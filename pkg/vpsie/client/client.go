package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/vpsie/vpsie-k8s-autoscaler/internal/logging"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
)

const (
	// DefaultSecretName is the default name of the Kubernetes secret containing VPSie credentials
	DefaultSecretName = "vpsie-secret"

	// DefaultSecretNamespace is the default namespace of the Kubernetes secret
	DefaultSecretNamespace = "kube-system"

	// DefaultAPIEndpoint is the default VPSie API endpoint
	DefaultAPIEndpoint = "https://api.vpsie.com/v2"

	// DefaultTimeout is the default HTTP client timeout
	DefaultTimeout = 30 * time.Second

	// DefaultRateLimit is the default rate limit (requests per minute)
	DefaultRateLimit = 100

	// DefaultTokenRefreshBuffer is the time before expiration to refresh the token
	DefaultTokenRefreshBuffer = 5 * time.Minute

	// SecretClientIDKey is the key name for the OAuth client ID in the secret
	SecretClientIDKey = "clientId"

	// SecretClientSecretKey is the key name for the OAuth client secret in the secret
	SecretClientSecretKey = "clientSecret"

	// SecretURLKey is the key name for the API URL in the secret (optional)
	SecretURLKey = "url"

	// TokenEndpoint is the VPSie authentication endpoint path
	TokenEndpoint = "/auth/from/api"
)

// Client represents a VPSie API client
type Client struct {
	httpClient     *http.Client
	rateLimiter    *rate.Limiter
	circuitBreaker *CircuitBreaker
	baseURL        string
	clientID       string
	clientSecret   string
	accessToken    string
	tokenExpiresAt time.Time
	userAgent      string
	logger         *zap.Logger
	mu             sync.RWMutex
}

// ClientOptions represents options for creating a new Client
type ClientOptions struct {
	// SecretName is the name of the Kubernetes secret containing VPSie credentials
	SecretName string

	// SecretNamespace is the namespace of the Kubernetes secret
	SecretNamespace string

	// HTTPClient is a custom HTTP client to use (optional)
	HTTPClient *http.Client

	// Timeout is the HTTP client timeout
	Timeout time.Duration

	// RateLimit is the maximum number of requests per minute
	RateLimit int

	// UserAgent is the user agent string to use in requests
	UserAgent string

	// TokenRefreshBuffer is the time before expiration to refresh the token
	TokenRefreshBuffer time.Duration

	// Logger is the logger to use (optional, defaults to no-op logger)
	Logger *zap.Logger
}

// TokenResponse represents the authentication response from the VPSie API
type TokenResponse struct {
	AccessToken  AccessTokenInfo  `json:"accessToken"`
	RefreshToken RefreshTokenInfo `json:"refreshToken"`
	Error        bool             `json:"error,omitempty"`
	Message      string           `json:"message,omitempty"`
	Code         int              `json:"code,omitempty"`
}

// AccessTokenInfo contains the access token details
type AccessTokenInfo struct {
	Token   string `json:"token"`
	Expires string `json:"expires"`
}

// RefreshTokenInfo contains the refresh token details
type RefreshTokenInfo struct {
	Token   string `json:"token"`
	Expires string `json:"expires"`
}

// NewClient creates a new VPSie API client by reading credentials from a Kubernetes secret
func NewClient(ctx context.Context, clientset kubernetes.Interface, opts *ClientOptions) (*Client, error) {
	if opts == nil {
		opts = &ClientOptions{}
	}

	// Set defaults
	if opts.SecretName == "" {
		opts.SecretName = DefaultSecretName
	}
	if opts.SecretNamespace == "" {
		opts.SecretNamespace = DefaultSecretNamespace
	}
	if opts.Timeout == 0 {
		opts.Timeout = DefaultTimeout
	}
	if opts.RateLimit == 0 {
		opts.RateLimit = DefaultRateLimit
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "vpsie-k8s-autoscaler/1.0"
	}

	// Read credentials from Kubernetes secret
	secret, err := clientset.CoreV1().Secrets(opts.SecretNamespace).Get(ctx, opts.SecretName, metav1.GetOptions{})
	if err != nil {
		return nil, NewSecretError(opts.SecretName, opts.SecretNamespace, "failed to get secret", err)
	}

	// Extract client ID from secret
	clientIDBytes, ok := secret.Data[SecretClientIDKey]
	if !ok {
		return nil, NewSecretError(opts.SecretName, opts.SecretNamespace,
			fmt.Sprintf("secret does not contain '%s' key", SecretClientIDKey), nil)
	}
	clientID := string(clientIDBytes)
	if clientID == "" {
		return nil, NewSecretError(opts.SecretName, opts.SecretNamespace,
			fmt.Sprintf("secret key '%s' is empty", SecretClientIDKey), nil)
	}

	// Extract client secret from secret
	clientSecretBytes, ok := secret.Data[SecretClientSecretKey]
	if !ok {
		return nil, NewSecretError(opts.SecretName, opts.SecretNamespace,
			fmt.Sprintf("secret does not contain '%s' key", SecretClientSecretKey), nil)
	}
	clientSecret := string(clientSecretBytes)
	if clientSecret == "" {
		return nil, NewSecretError(opts.SecretName, opts.SecretNamespace,
			fmt.Sprintf("secret key '%s' is empty", SecretClientSecretKey), nil)
	}

	// Extract API URL from secret (optional, use default if not provided)
	baseURL := DefaultAPIEndpoint
	if urlBytes, ok := secret.Data[SecretURLKey]; ok && len(urlBytes) > 0 {
		baseURL = string(urlBytes)
	}

	// Validate base URL
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil, NewConfigError("api_url", "API URL cannot be empty")
	}

	// Create HTTP client if not provided
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: opts.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	}

	// Create rate limiter (convert requests per minute to requests per second)
	rps := float64(opts.RateLimit) / 60.0
	rateLimiter := rate.NewLimiter(rate.Limit(rps), opts.RateLimit)

	// Get logger (default to no-op if not provided)
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// Create circuit breaker for fault tolerance
	circuitBreaker := NewCircuitBreaker(DefaultCircuitBreakerConfig(), logger.Named("circuit-breaker"))

	// Create client instance
	client := &Client{
		httpClient:     httpClient,
		rateLimiter:    rateLimiter,
		circuitBreaker: circuitBreaker,
		baseURL:        baseURL,
		clientID:       clientID,
		clientSecret:   clientSecret,
		userAgent:      opts.UserAgent,
		logger:         logger.Named("vpsie-client"),
	}

	// Obtain initial access token
	if err := client.refreshToken(ctx); err != nil {
		return nil, fmt.Errorf("failed to obtain initial access token: %w", err)
	}

	return client, nil
}

// NewClientWithCredentials creates a new VPSie API client with explicit OAuth credentials (for testing)
func NewClientWithCredentials(baseURL, clientID, clientSecret string, opts *ClientOptions) (*Client, error) {
	ctx := context.Background()
	return NewClientWithCredentialsAndContext(ctx, baseURL, clientID, clientSecret, opts)
}

// NewClientWithCredentialsAndContext creates a new VPSie API client with explicit OAuth credentials and context
func NewClientWithCredentialsAndContext(ctx context.Context, baseURL, clientID, clientSecret string, opts *ClientOptions) (*Client, error) {
	if opts == nil {
		opts = &ClientOptions{}
	}

	if clientID == "" {
		return nil, NewConfigError("client_id", "Client ID cannot be empty")
	}

	if clientSecret == "" {
		return nil, NewConfigError("client_secret", "Client secret cannot be empty")
	}

	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = DefaultAPIEndpoint
	}

	// Set defaults
	if opts.Timeout == 0 {
		opts.Timeout = DefaultTimeout
	}
	if opts.RateLimit == 0 {
		opts.RateLimit = DefaultRateLimit
	}
	if opts.UserAgent == "" {
		opts.UserAgent = "vpsie-k8s-autoscaler/1.0"
	}

	// Create HTTP client if not provided
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: opts.Timeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		}
	}

	// Create rate limiter
	rps := float64(opts.RateLimit) / 60.0
	rateLimiter := rate.NewLimiter(rate.Limit(rps), opts.RateLimit)

	// Get logger (default to no-op if not provided)
	logger := opts.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	// Create circuit breaker for fault tolerance
	circuitBreaker := NewCircuitBreaker(DefaultCircuitBreakerConfig(), logger.Named("circuit-breaker"))

	// Create client instance
	client := &Client{
		httpClient:     httpClient,
		rateLimiter:    rateLimiter,
		circuitBreaker: circuitBreaker,
		baseURL:        baseURL,
		clientID:       clientID,
		clientSecret:   clientSecret,
		userAgent:      opts.UserAgent,
		logger:         logger.Named("vpsie-client"),
	}

	// Obtain initial access token
	if err := client.refreshToken(ctx); err != nil {
		return nil, fmt.Errorf("failed to obtain initial access token: %w", err)
	}

	return client, nil
}

// refreshToken obtains a new access token using client credentials
func (c *Client) refreshToken(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Prepare form data (application/x-www-form-urlencoded)
	formData := fmt.Sprintf("clientId=%s&clientSecret=%s",
		c.clientID,
		c.clientSecret,
	)

	// Build token URL
	tokenURL := c.baseURL + TokenEndpoint

	// Create HTTP request (without using doRequest to avoid circular dependency)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(formData))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	// Perform request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to perform token request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read token response: %w", err)
	}

	// Parse token response
	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	// Check for API errors
	if tokenResp.Error {
		return fmt.Errorf("token request failed: %s (code: %d)", tokenResp.Message, tokenResp.Code)
	}

	// Check for non-200 status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Validate response
	if tokenResp.AccessToken.Token == "" {
		return fmt.Errorf("token response missing access token")
	}

	// Update client state
	c.accessToken = tokenResp.AccessToken.Token

	// Parse expiration time
	if tokenResp.AccessToken.Expires != "" {
		expiresAt, err := time.Parse(time.RFC3339, tokenResp.AccessToken.Expires)
		if err == nil {
			c.tokenExpiresAt = expiresAt
		} else {
			// If parsing fails, default to 1 hour from now
			c.tokenExpiresAt = time.Now().Add(1 * time.Hour)
		}
	} else {
		// Default to 1 hour if not specified
		c.tokenExpiresAt = time.Now().Add(1 * time.Hour)
	}

	return nil
}

// ensureValidToken checks if the token is still valid and refreshes if needed
func (c *Client) ensureValidToken(ctx context.Context) error {
	c.mu.RLock()
	needsRefresh := time.Now().Add(DefaultTokenRefreshBuffer).After(c.tokenExpiresAt)
	c.mu.RUnlock()

	if needsRefresh {
		return c.refreshToken(ctx)
	}

	return nil
}

// doRequest performs an HTTP request with authentication and rate limiting
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	// Track metrics
	startTime := time.Now()
	requestID := logging.GetRequestID(ctx)

	// Add request ID to logger if available
	logger := logging.WithRequestIDField(ctx, c.logger)

	// Log API call (debug level)
	logging.LogAPICall(logger, method, path, requestID)

	// Ensure we have a valid access token
	if err := c.ensureValidToken(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure valid token: %w", err)
	}

	// Wait for rate limiter and record metrics
	rateLimitStart := time.Now()
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}
	rateLimitWait := time.Since(rateLimitStart)
	metrics.VPSieAPIRateLimitWaitDuration.WithLabelValues(method).Observe(rateLimitWait.Seconds())

	// If we waited more than 10ms, we were rate limited
	if rateLimitWait > 10*time.Millisecond {
		metrics.VPSieAPIRateLimitedTotal.WithLabelValues(method).Inc()
	}

	// Build URL
	url := c.baseURL + path

	// Marshal request body if provided
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	c.mu.RLock()
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("User-Agent", c.userAgent)
	c.mu.RUnlock()

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	// Perform request with circuit breaker protection
	var resp *http.Response
	cbErr := c.circuitBreaker.Call(func() error {
		var err error
		resp, err = c.httpClient.Do(req)
		return err
	})
	duration := time.Since(startTime)

	if cbErr != nil {
		// Check if circuit breaker is open
		if cbErr == ErrCircuitOpen {
			metrics.RecordAPIError(method, "circuit_open")
			metrics.RecordAPIRequest(method, "error", duration)
			logging.LogAPIError(logger, method, path, 0, cbErr, requestID)
			return nil, fmt.Errorf("circuit breaker is open: %w", cbErr)
		}
		// Regular request error
		metrics.RecordAPIError(method, "request_failed")
		metrics.RecordAPIRequest(method, "error", duration)
		logging.LogAPIError(logger, method, path, 0, cbErr, requestID)
		return nil, fmt.Errorf("failed to perform request: %w", cbErr)
	}

	// Log response (debug level)
	logging.LogAPIResponse(logger, method, path, resp.StatusCode, duration.String(), requestID)

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()

		// Record error metrics
		status := fmt.Sprintf("%d", resp.StatusCode)
		metrics.RecordAPIRequest(method, status, duration)

		// Determine error type for metrics
		var errorType string
		switch {
		case resp.StatusCode == http.StatusUnauthorized:
			errorType = "unauthorized"
		case resp.StatusCode == http.StatusForbidden:
			errorType = "forbidden"
		case resp.StatusCode == http.StatusNotFound:
			errorType = "not_found"
		case resp.StatusCode == http.StatusTooManyRequests:
			errorType = "rate_limited"
		case resp.StatusCode >= 500:
			errorType = "server_error"
		default:
			errorType = "client_error"
		}
		metrics.RecordAPIError(method, errorType)

		// If we get 401 Unauthorized, try refreshing the token once
		if resp.StatusCode == http.StatusUnauthorized {
			if refreshErr := c.refreshToken(ctx); refreshErr == nil {
				// Token refreshed successfully, retry the request
				return c.doRequestWithToken(ctx, method, path, body)
			}
		}

		// Try to parse error response
		var errResp ErrorResponse
		bodyBytes, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Message != "" {
			requestID := resp.Header.Get("X-Request-ID")
			apiErr := NewAPIErrorWithRequestID(resp.StatusCode, errResp.Error, errResp.Message, requestID)
			logging.LogAPIError(logger, method, path, resp.StatusCode, apiErr, requestID)
			return nil, apiErr
		}

		// Fallback to generic error
		requestID := resp.Header.Get("X-Request-ID")
		apiErr := NewAPIErrorWithRequestID(resp.StatusCode, http.StatusText(resp.StatusCode), string(bodyBytes), requestID)
		logging.LogAPIError(logger, method, path, resp.StatusCode, apiErr, requestID)
		return nil, apiErr
	}

	// Record success metrics
	status := fmt.Sprintf("%d", resp.StatusCode)
	metrics.RecordAPIRequest(method, status, duration)

	return resp, nil
}

// doRequestWithToken performs an HTTP request with the current token (no retry on 401)
func (c *Client) doRequestWithToken(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	// Wait for rate limiter and record metrics
	rateLimitStart := time.Now()
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
	}
	rateLimitWait := time.Since(rateLimitStart)
	metrics.VPSieAPIRateLimitWaitDuration.WithLabelValues(method).Observe(rateLimitWait.Seconds())

	// If we waited more than 10ms, we were rate limited
	if rateLimitWait > 10*time.Millisecond {
		metrics.VPSieAPIRateLimitedTotal.WithLabelValues(method).Inc()
	}

	// Build URL
	url := c.baseURL + path

	// Marshal request body if provided
	var bodyReader io.Reader
	if body != nil {
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	c.mu.RLock()
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("User-Agent", c.userAgent)
	c.mu.RUnlock()

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	// Perform request with circuit breaker protection
	var resp *http.Response
	cbErr := c.circuitBreaker.Call(func() error {
		var err error
		resp, err = c.httpClient.Do(req)
		return err
	})
	if cbErr != nil {
		// Check if circuit breaker is open
		if cbErr == ErrCircuitOpen {
			return nil, fmt.Errorf("circuit breaker is open: %w", cbErr)
		}
		return nil, fmt.Errorf("failed to perform request: %w", cbErr)
	}

	// Check for HTTP errors
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()

		// Try to parse error response
		var errResp ErrorResponse
		bodyBytes, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(bodyBytes, &errResp); err == nil && errResp.Message != "" {
			requestID := resp.Header.Get("X-Request-ID")
			return nil, NewAPIErrorWithRequestID(resp.StatusCode, errResp.Error, errResp.Message, requestID)
		}

		// Fallback to generic error
		requestID := resp.Header.Get("X-Request-ID")
		return nil, NewAPIErrorWithRequestID(resp.StatusCode, http.StatusText(resp.StatusCode), string(bodyBytes), requestID)
	}

	return resp, nil
}

// get performs a GET request
func (c *Client) get(ctx context.Context, path string, result interface{}) error {
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// post performs a POST request
func (c *Client) post(ctx context.Context, path string, body, result interface{}) error {
	resp, err := c.doRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if result != nil {
		if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// delete performs a DELETE request
func (c *Client) delete(ctx context.Context, path string) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

// GetBaseURL returns the current base URL
func (c *Client) GetBaseURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.baseURL
}

// SetUserAgent sets the user agent string
func (c *Client) SetUserAgent(userAgent string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userAgent = userAgent
}

// ============================================================================
// VPS Lifecycle Operations
// ============================================================================

// ListVMs retrieves a list of all VPS instances associated with the account.
//
// This method performs a GET request to /vm and returns all VPS instances.
// The context can be used to cancel the request or set a timeout.
//
// Example usage:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	defer cancel()
//	vms, err := client.ListVMs(ctx)
//	if err != nil {
//	    log.Fatalf("failed to list VMs: %v", err)
//	}
//	for _, vm := range vms {
//	    fmt.Printf("VM: %s (ID: %s, Status: %s)\n", vm.Name, vm.ID, vm.Status)
//	}
//
// Returns:
//   - []VPS: A slice of VPS instances
//   - error: An error if the request fails, is rate limited, or the API returns an error
func (c *Client) ListVMs(ctx context.Context) ([]VPS, error) {
	var response ListVPSResponse

	// Perform GET request to /vm endpoint
	if err := c.get(ctx, "/vm", &response); err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}

	return response.Data, nil
}

// CreateVM creates a new VPS instance with the specified configuration.
//
// This method performs a POST request to /vm with the provided configuration.
// The request includes CPU, RAM, disk size, datacenter, OS image, and other parameters.
// The context can be used to cancel the request or set a timeout.
//
// The CreateVPSRequest must contain at minimum:
//   - Name: A unique name for the VPS
//   - OfferingID: The ID of the instance type/plan
//   - DatacenterID: The ID of the datacenter/region
//   - OSImageID: The ID of the operating system image
//
// Optional parameters include:
//   - Hostname: Custom hostname (defaults to name if not provided)
//   - SSHKeyIDs: SSH keys to install for root access
//   - Password: Root password (if SSH keys not provided)
//   - Notes: Descriptive notes about the VPS
//   - Tags: Tags for organization and filtering
//   - UserData: Cloud-init user data for initial configuration
//
// Example usage:
//
//	req := client.CreateVPSRequest{
//	    Name:         "my-k8s-node",
//	    Hostname:     "node-1.cluster.local",
//	    OfferingID:   "offering-123",
//	    DatacenterID: "dc-us-east-1",
//	    OSImageID:    "ubuntu-22.04",
//	    SSHKeyIDs:    []string{"key-456"},
//	    Tags:         []string{"kubernetes", "worker"},
//	    UserData:     base64.StdEncoding.EncodeToString([]byte(cloudInitScript)),
//	}
//	vm, err := client.CreateVM(ctx, req)
//	if err != nil {
//	    log.Fatalf("failed to create VM: %v", err)
//	}
//	fmt.Printf("Created VM: %s (ID: %s)\n", vm.Name, vm.ID)
//
// Returns:
//   - *VPS: The created VPS instance with full details including ID and IP addresses
//   - error: An error if validation fails, the request fails, or the API returns an error
func (c *Client) CreateVM(ctx context.Context, req CreateVPSRequest) (*VPS, error) {
	// Validate required fields
	if req.Name == "" {
		return nil, NewConfigError("name", "VM name is required")
	}
	if req.OfferingID == "" {
		return nil, NewConfigError("offering_id", "offering ID is required")
	}
	if req.DatacenterID == "" {
		return nil, NewConfigError("datacenter_id", "datacenter ID is required")
	}
	if req.OSImageID == "" {
		return nil, NewConfigError("os_image_id", "OS image ID is required")
	}

	// Set hostname to name if not provided
	if req.Hostname == "" {
		req.Hostname = req.Name
	}

	var vps VPS

	// Perform POST request to /vm endpoint
	if err := c.post(ctx, "/vm", req, &vps); err != nil {
		return nil, fmt.Errorf("failed to create VM '%s': %w", req.Name, err)
	}

	return &vps, nil
}

// GetVM retrieves detailed information about a specific VPS instance.
//
// This method performs a GET request to /vm/{id} and returns the full VPS details
// including status, IP addresses, resource allocation, and metadata.
// The context can be used to cancel the request or set a timeout.
//
// Example usage:
//
//	vm, err := client.GetVM(ctx, "vm-123")
//	if err != nil {
//	    if client.IsNotFound(err) {
//	        log.Printf("VM not found")
//	        return
//	    }
//	    log.Fatalf("failed to get VM: %v", err)
//	}
//	fmt.Printf("VM Status: %s, IP: %s\n", vm.Status, vm.IPAddress)
//
// Returns:
//   - *VPS: The VPS instance with full details
//   - error: An error if the VM is not found, the request fails, or the API returns an error
//     Use IsNotFound(err) to check if the error is a 404 Not Found error
func (c *Client) GetVM(ctx context.Context, vmID int) (*VPS, error) {
	if vmID == 0 {
		return nil, NewConfigError("vm_id", "VM ID is required")
	}

	var vps VPS

	// Perform GET request to /vm/{id} endpoint
	path := fmt.Sprintf("/vm/%d", vmID)
	if err := c.get(ctx, path, &vps); err != nil {
		return nil, fmt.Errorf("failed to get VM %d: %w", vmID, err)
	}

	return &vps, nil
}

// DeleteVM deletes a VPS instance.
//
// This method performs a DELETE request to /vm/{id} to permanently delete the VPS.
// The deletion is asynchronous - the API will accept the request and the VPS will be
// deleted in the background. The context can be used to cancel the request or set a timeout.
//
// Important behaviors:
//   - If the VM is not found (404 error), this method treats it as success since the
//     desired state (VM deleted) has been achieved. This makes the operation idempotent.
//   - The VM must be stopped before deletion, or the API may return an error
//   - Deletion is permanent and cannot be undone
//   - All data on the VM will be lost
//
// Example usage:
//
//	// Simple deletion
//	if err := client.DeleteVM(ctx, "vm-123"); err != nil {
//	    log.Fatalf("failed to delete VM: %v", err)
//	}
//	fmt.Println("VM deleted successfully")
//
//	// With proper error handling
//	err := client.DeleteVM(ctx, vmID)
//	if err != nil {
//	    if apiErr, ok := err.(*client.APIError); ok && apiErr.StatusCode == 409 {
//	        log.Printf("VM is still running, stopping first...")
//	        // Stop VM first, then retry deletion
//	    } else {
//	        log.Fatalf("failed to delete VM: %v", err)
//	    }
//	}
//
// Returns:
//   - error: An error if the request fails or the API returns an error (except 404)
//     Returns nil if the VM is successfully deleted or already doesn't exist (404)
func (c *Client) DeleteVM(ctx context.Context, vmID int) error {
	if vmID == 0 {
		return NewConfigError("vm_id", "VM ID is required")
	}

	// Perform DELETE request to /vm/{id} endpoint
	path := fmt.Sprintf("/vm/%d", vmID)
	err := c.delete(ctx, path)

	// If the VM is not found (404), consider it a success since the VM is already deleted
	// This makes the deletion operation idempotent
	if err != nil && IsNotFound(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to delete VM %d: %w", vmID, err)
	}

	return nil
}

// Close cleans up client resources including HTTP connections and logger buffers.
// This method should be called when the client is no longer needed to prevent resource leaks.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Close idle HTTP connections to free resources
	if transport, ok := c.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}

	// Sync logger to flush any buffered log entries
	// Ignore sync errors as they're common on stdout/stderr and typically benign
	_ = c.logger.Sync()

	return nil
}

// ============================================================================
// VPS Operations (Interface compatibility methods)
// ============================================================================

// ListVPS lists all VPS instances (delegates to ListVMs)
func (c *Client) ListVPS(ctx context.Context, opts *ListOptions) ([]VPS, error) {
	return c.ListVMs(ctx)
}

// GetVPS retrieves a specific VPS by ID (delegates to GetVM)
func (c *Client) GetVPS(ctx context.Context, id int) (*VPS, error) {
	return c.GetVM(ctx, id)
}

// CreateVPS creates a new VPS instance (delegates to CreateVM)
func (c *Client) CreateVPS(ctx context.Context, req *CreateVPSRequest) (*VPS, error) {
	return c.CreateVM(ctx, *req)
}

// DeleteVPS deletes a VPS instance (delegates to DeleteVM)
func (c *Client) DeleteVPS(ctx context.Context, id int) error {
	return c.DeleteVM(ctx, id)
}

// UpdateVPS updates a VPS instance configuration
func (c *Client) UpdateVPS(ctx context.Context, id int, req *UpdateVPSRequest) (*VPS, error) {
	if id == 0 {
		return nil, NewConfigError("vps_id", "VPS ID is required")
	}
	if req == nil {
		return nil, NewConfigError("request", "Update request is required")
	}

	var vps VPS
	path := fmt.Sprintf("/vm/%d", id)
	if err := c.post(ctx, path, req, &vps); err != nil {
		return nil, fmt.Errorf("failed to update VPS %d: %w", id, err)
	}

	return &vps, nil
}

// PerformVPSAction performs an action on a VPS (start, stop, restart, etc.)
func (c *Client) PerformVPSAction(ctx context.Context, id int, action *VPSAction) error {
	if id == 0 {
		return NewConfigError("vps_id", "VPS ID is required")
	}
	if action == nil {
		return NewConfigError("action", "VPS action is required")
	}

	path := fmt.Sprintf("/vm/%d/action", id)
	if err := c.post(ctx, path, action, nil); err != nil {
		return fmt.Errorf("failed to perform action on VPS %d: %w", id, err)
	}

	return nil
}

// ============================================================================
// Offering Operations
// ============================================================================

// ListOfferings retrieves a list of all available VPS offerings/plans
func (c *Client) ListOfferings(ctx context.Context, opts *ListOptions) ([]Offering, error) {
	var response ListOfferingsResponse

	if err := c.get(ctx, "/offerings", &response); err != nil {
		return nil, fmt.Errorf("failed to list offerings: %w", err)
	}

	return response.Data, nil
}

// GetOffering retrieves details of a specific offering by ID
func (c *Client) GetOffering(ctx context.Context, id string) (*Offering, error) {
	if id == "" {
		return nil, NewConfigError("offering_id", "Offering ID is required")
	}

	var offering Offering
	path := fmt.Sprintf("/offerings/%s", id)
	if err := c.get(ctx, path, &offering); err != nil {
		return nil, fmt.Errorf("failed to get offering %s: %w", id, err)
	}

	return &offering, nil
}

// ============================================================================
// Datacenter Operations
// ============================================================================

// ListDatacenters retrieves a list of all available datacenters
func (c *Client) ListDatacenters(ctx context.Context, opts *ListOptions) ([]Datacenter, error) {
	var response ListDatacentersResponse

	if err := c.get(ctx, "/datacenters", &response); err != nil {
		return nil, fmt.Errorf("failed to list datacenters: %w", err)
	}

	return response.Data, nil
}

// ============================================================================
// OS Image Operations
// ============================================================================

// ListOSImages retrieves a list of all available OS images
func (c *Client) ListOSImages(ctx context.Context, opts *ListOptions) ([]OSImage, error) {
	var response ListOSImagesResponse

	if err := c.get(ctx, "/images", &response); err != nil {
		return nil, fmt.Errorf("failed to list OS images: %w", err)
	}

	return response.Data, nil
}
