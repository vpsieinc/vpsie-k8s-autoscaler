package client

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

	// MaxResponseBodySize is the maximum size of HTTP response bodies (10MB)
	// This prevents DoS attacks from malicious or compromised APIs
	MaxResponseBodySize = 10 * 1024 * 1024

	// SecretClientIDKey is the key name for the OAuth client ID in the secret
	SecretClientIDKey = "clientId"

	// SecretClientSecretKey is the key name for the OAuth client secret in the secret
	SecretClientSecretKey = "clientSecret"

	// SecretTokenKey is the key name for a simple API token in the secret (alternative to OAuth)
	SecretTokenKey = "token"

	// SecretURLKey is the key name for the API URL in the secret (optional)
	SecretURLKey = "url"

	// TokenEndpoint is the VPSie authentication endpoint path
	TokenEndpoint = "/auth/from/api"

	// DefaultMaxRetries is the default maximum number of retries for transient errors
	DefaultMaxRetries = 3

	// DefaultInitialBackoff is the initial backoff duration for retries
	DefaultInitialBackoff = 100 * time.Millisecond

	// DefaultMaxBackoff is the maximum backoff duration between retries
	DefaultMaxBackoff = 30 * time.Second

	// DefaultBackoffMultiplier is the multiplier for exponential backoff
	DefaultBackoffMultiplier = 2.0

	// DefaultJitterFactor is the maximum jitter as a fraction of backoff (0.0-1.0)
	DefaultJitterFactor = 0.2
)

// RetryConfig configures the retry behavior with exponential backoff
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (0 = no retries)
	MaxRetries int

	// InitialBackoff is the initial backoff duration
	InitialBackoff time.Duration

	// MaxBackoff is the maximum backoff duration
	MaxBackoff time.Duration

	// BackoffMultiplier is the multiplier for exponential backoff
	BackoffMultiplier float64

	// JitterFactor is the maximum jitter as a fraction of backoff (0.0-1.0)
	JitterFactor float64

	// RetryableStatusCodes are HTTP status codes that should trigger a retry
	RetryableStatusCodes []int
}

// DefaultRetryConfig returns the default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        DefaultMaxRetries,
		InitialBackoff:    DefaultInitialBackoff,
		MaxBackoff:        DefaultMaxBackoff,
		BackoffMultiplier: DefaultBackoffMultiplier,
		JitterFactor:      DefaultJitterFactor,
		// Retry on server errors and rate limiting
		RetryableStatusCodes: []int{
			http.StatusTooManyRequests,
			http.StatusInternalServerError,
			http.StatusBadGateway,
			http.StatusServiceUnavailable,
			http.StatusGatewayTimeout,
		},
	}
}

// Client represents a VPSie API client
type Client struct {
	httpClient     *http.Client
	rateLimiter    *rate.Limiter
	circuitBreaker *CircuitBreaker
	retryConfig    RetryConfig
	baseURL        string
	clientID       string
	clientSecret   string
	accessToken    string
	tokenExpiresAt time.Time
	userAgent      string
	logger         *zap.Logger
	mu             sync.RWMutex
	// useSimpleToken indicates whether to use simple token auth instead of OAuth
	useSimpleToken bool
}

// ClientOptions represents options for creating a new Client
type ClientOptions struct {
	// SecretName is the name of the Kubernetes secret containing VPSie credentials
	SecretName string

	// SecretNamespace is the namespace of the Kubernetes secret
	SecretNamespace string

	// HTTPClient is a custom HTTP client to use (optional)
	HTTPClient *http.Client

	// HTTPTransport is a custom HTTP transport to use (optional)
	// This is useful for adding tracing/monitoring middleware
	HTTPTransport http.RoundTripper

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

	// RetryConfig configures retry behavior with exponential backoff
	// If nil, DefaultRetryConfig() is used
	RetryConfig *RetryConfig

	// CircuitBreakerConfig configures the circuit breaker
	// If nil, DefaultCircuitBreakerConfig() is used
	CircuitBreakerConfig *CircuitBreakerConfig
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

	// Check for simple token authentication first (preferred for simplicity)
	var clientID, clientSecret, simpleToken string
	var useSimpleToken bool

	if tokenBytes, ok := secret.Data[SecretTokenKey]; ok && len(tokenBytes) > 0 {
		// Use simple token authentication
		simpleToken = string(tokenBytes)
		useSimpleToken = true
	} else {
		// Fall back to OAuth credentials
		clientIDBytes, ok := secret.Data[SecretClientIDKey]
		if !ok {
			return nil, NewSecretError(opts.SecretName, opts.SecretNamespace,
				fmt.Sprintf("secret must contain either '%s' (simple token) or '%s'/'%s' (OAuth)",
					SecretTokenKey, SecretClientIDKey, SecretClientSecretKey), nil)
		}
		clientID = string(clientIDBytes)
		if clientID == "" {
			return nil, NewSecretError(opts.SecretName, opts.SecretNamespace,
				fmt.Sprintf("secret key '%s' is empty", SecretClientIDKey), nil)
		}

		clientSecretBytes, ok := secret.Data[SecretClientSecretKey]
		if !ok {
			return nil, NewSecretError(opts.SecretName, opts.SecretNamespace,
				fmt.Sprintf("secret does not contain '%s' key", SecretClientSecretKey), nil)
		}
		clientSecret = string(clientSecretBytes)
		if clientSecret == "" {
			return nil, NewSecretError(opts.SecretName, opts.SecretNamespace,
				fmt.Sprintf("secret key '%s' is empty", SecretClientSecretKey), nil)
		}
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

	// Enforce HTTPS to prevent credentials from being sent over unencrypted connections
	if !strings.HasPrefix(baseURL, "https://") {
		return nil, NewConfigError("api_url", fmt.Sprintf("API URL must use HTTPS, got: %s", baseURL))
	}

	// Create HTTP client if not provided
	httpClient := opts.HTTPClient
	if httpClient == nil {
		// Create base transport with TLS configuration
		baseTransport := &http.Transport{
			// TLS configuration - enforce TLS 1.2+ with strong cipher suites
			// This addresses Fix #9: TLS Validation
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12, // Disables TLS 1.0 and 1.1
				CipherSuites: []uint16{
					// ECDHE provides forward secrecy, AES-GCM is authenticated encryption
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				},
				PreferServerCipherSuites: true,
				InsecureSkipVerify:       false, // Explicit for security audits
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		}

		// Use custom transport if provided (e.g., for tracing), wrapping the base transport
		var transport http.RoundTripper = baseTransport
		if opts.HTTPTransport != nil {
			transport = opts.HTTPTransport
		}

		httpClient = &http.Client{
			Timeout:   opts.Timeout,
			Transport: transport,
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
	cbConfig := DefaultCircuitBreakerConfig()
	if opts.CircuitBreakerConfig != nil {
		cbConfig = *opts.CircuitBreakerConfig
	}
	circuitBreaker := NewCircuitBreaker(cbConfig, logger.Named("circuit-breaker"))

	// Create retry config
	retryConfig := DefaultRetryConfig()
	if opts.RetryConfig != nil {
		retryConfig = *opts.RetryConfig
	}

	// Create client instance
	client := &Client{
		httpClient:     httpClient,
		rateLimiter:    rateLimiter,
		circuitBreaker: circuitBreaker,
		retryConfig:    retryConfig,
		baseURL:        baseURL,
		clientID:       clientID,
		clientSecret:   clientSecret,
		userAgent:      opts.UserAgent,
		logger:         logger.Named("vpsie-client"),
		useSimpleToken: useSimpleToken,
	}

	// Set up authentication
	if useSimpleToken {
		// Simple token auth - use token directly, no expiration
		client.accessToken = simpleToken
		client.tokenExpiresAt = time.Now().Add(365 * 24 * time.Hour) // Far future
		client.logger.Info("Using simple token authentication")
	} else {
		// OAuth - obtain initial access token
		if err := client.refreshToken(ctx); err != nil {
			return nil, fmt.Errorf("failed to obtain initial access token: %w", err)
		}
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

	// Enforce HTTPS to prevent credentials from being sent over unencrypted connections
	if !strings.HasPrefix(baseURL, "https://") {
		return nil, NewConfigError("api_url", fmt.Sprintf("API URL must use HTTPS, got: %s", baseURL))
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
		// Create base transport with TLS configuration
		baseTransport := &http.Transport{
			// TLS configuration - enforce TLS 1.2+ with strong cipher suites
			// This addresses Fix #9: TLS Validation
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12, // Disables TLS 1.0 and 1.1
				CipherSuites: []uint16{
					// ECDHE provides forward secrecy, AES-GCM is authenticated encryption
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				},
				PreferServerCipherSuites: true,
				InsecureSkipVerify:       false, // Explicit for security audits
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		}

		// Use custom transport if provided (e.g., for tracing), wrapping the base transport
		var transport http.RoundTripper = baseTransport
		if opts.HTTPTransport != nil {
			transport = opts.HTTPTransport
		}

		httpClient = &http.Client{
			Timeout:   opts.Timeout,
			Transport: transport,
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
	cbConfig := DefaultCircuitBreakerConfig()
	if opts.CircuitBreakerConfig != nil {
		cbConfig = *opts.CircuitBreakerConfig
	}
	circuitBreaker := NewCircuitBreaker(cbConfig, logger.Named("circuit-breaker"))

	// Create retry config
	retryConfig := DefaultRetryConfig()
	if opts.RetryConfig != nil {
		retryConfig = *opts.RetryConfig
	}

	// Create client instance
	client := &Client{
		httpClient:     httpClient,
		rateLimiter:    rateLimiter,
		circuitBreaker: circuitBreaker,
		retryConfig:    retryConfig,
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

	// Prepare form data (application/x-www-form-urlencoded) with proper URL encoding
	formData := url.Values{
		"clientId":     {c.clientID},
		"clientSecret": {c.clientSecret},
	}.Encode()

	// Build token URL
	tokenURL := c.baseURL + TokenEndpoint

	// Log token refresh without exposing credentials
	c.logger.Info("refreshing OAuth token",
		zap.String("endpoint", tokenURL),
	)

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

	// Read response body with size limit to prevent DoS attacks
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
	if err != nil {
		return fmt.Errorf("failed to read token response: %w", err)
	}

	// Parse token response
	var tokenResp TokenResponse
	if err := json.Unmarshal(bodyBytes, &tokenResp); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	// Check for API errors - do not log message as it may contain credentials
	if tokenResp.Error {
		c.logger.Error("token request failed",
			zap.Int("code", tokenResp.Code),
			zap.Bool("error", tokenResp.Error),
		)
		return fmt.Errorf("token request failed with code: %d", tokenResp.Code)
	}

	// Check for non-200 status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token request failed with status %d", resp.StatusCode)
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

	c.logger.Info("OAuth token refreshed successfully")

	return nil
}

// ensureValidToken checks if the token is still valid and refreshes if needed
func (c *Client) ensureValidToken(ctx context.Context) error {
	c.mu.RLock()
	useSimpleToken := c.useSimpleToken
	needsRefresh := time.Now().Add(DefaultTokenRefreshBuffer).After(c.tokenExpiresAt)
	c.mu.RUnlock()

	// Simple token auth doesn't need refresh
	if useSimpleToken {
		return nil
	}

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

		// Try to parse error response with size limit
		var errResp ErrorResponse
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
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

		// Try to parse error response with size limit
		var errResp ErrorResponse
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
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
		// Limit response body size to prevent DoS attacks
		limitedReader := io.LimitReader(resp.Body, MaxResponseBodySize)
		if err := json.NewDecoder(limitedReader).Decode(result); err != nil {
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
		// Limit response body size to prevent DoS attacks
		limitedReader := io.LimitReader(resp.Body, MaxResponseBodySize)
		if err := json.NewDecoder(limitedReader).Decode(result); err != nil {
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

// UpdateCredentials updates the OAuth credentials for the client.
// This method is thread-safe and can be used for credential rotation.
// It validates the new credentials by attempting to obtain an access token
// before committing the change. If validation fails, the old credentials are retained.
//
// This method is typically called when:
//   - The vpsie-secret Kubernetes Secret is updated
//   - Credentials need to be rotated for security compliance
//   - An external credential management system pushes new credentials
//
// Example usage:
//
//	// Watch for secret changes
//	if secretChanged {
//	    newClientID := string(secret.Data["clientId"])
//	    newClientSecret := string(secret.Data["clientSecret"])
//	    if err := client.UpdateCredentials(ctx, newClientID, newClientSecret); err != nil {
//	        log.Error("credential rotation failed", "error", err)
//	    } else {
//	        log.Info("credentials rotated successfully")
//	    }
//	}
//
// Returns:
//   - error: An error if the new credentials are invalid or token refresh fails
func (c *Client) UpdateCredentials(ctx context.Context, clientID, clientSecret string) error {
	if clientID == "" {
		return NewConfigError("client_id", "Client ID cannot be empty")
	}
	if clientSecret == "" {
		return NewConfigError("client_secret", "Client secret cannot be empty")
	}

	c.mu.Lock()
	// Store old credentials in case we need to rollback
	oldClientID := c.clientID
	oldClientSecret := c.clientSecret
	oldAccessToken := c.accessToken
	oldTokenExpiresAt := c.tokenExpiresAt

	// Update to new credentials
	c.clientID = clientID
	c.clientSecret = clientSecret
	// Clear token to force refresh
	c.accessToken = ""
	c.tokenExpiresAt = time.Time{}
	c.mu.Unlock()

	c.logger.Info("attempting credential rotation",
		zap.Bool("credentials_changed", oldClientID != clientID),
	)

	// Attempt to get a new token with the new credentials
	if err := c.refreshToken(ctx); err != nil {
		// Rollback to old credentials
		c.mu.Lock()
		c.clientID = oldClientID
		c.clientSecret = oldClientSecret
		c.accessToken = oldAccessToken
		c.tokenExpiresAt = oldTokenExpiresAt
		c.mu.Unlock()

		c.logger.Error("credential rotation failed, rolled back to previous credentials",
			zap.Error(err),
		)

		return fmt.Errorf("failed to validate new credentials: %w", err)
	}

	c.logger.Info("credential rotation successful")

	return nil
}

// UpdateToken updates the simple API token for the client.
// This method is thread-safe and can be used for token rotation with simple token auth.
// Unlike UpdateCredentials, this doesn't validate the token immediately.
func (c *Client) UpdateToken(token string) error {
	if token == "" {
		return NewConfigError("token", "Token cannot be empty")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.accessToken = token
	c.tokenExpiresAt = time.Now().Add(365 * 24 * time.Hour) // Far future for simple tokens
	c.useSimpleToken = true

	c.logger.Info("simple token updated")

	return nil
}

// IsSimpleTokenAuth returns whether the client is using simple token authentication
func (c *Client) IsSimpleTokenAuth() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.useSimpleToken
}

// CalculateCredentialsHash calculates a SHA-256 hash of the given credentials for change detection.
// This is useful for comparing credentials without storing them in plain text.
// The hash does not expose the actual credentials.
// This function is exported to allow credential change detection before updating the client.
func CalculateCredentialsHash(clientID, clientSecret string) string {
	combined := clientID + ":" + clientSecret
	hash := sha256.Sum256([]byte(combined))
	return hex.EncodeToString(hash[:])
}

// GetCredentialsHash returns a hash of the current credentials for change detection.
// This is useful for comparing against a stored hash to detect when credentials have changed.
// The hash does not expose the actual credentials.
func (c *Client) GetCredentialsHash() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.useSimpleToken {
		// For simple token auth, hash the token itself
		return CalculateCredentialsHash(c.accessToken, "")
	}
	return CalculateCredentialsHash(c.clientID, c.clientSecret)
}

// IsTokenValid returns whether the current access token is valid.
// This is useful for health checks and monitoring.
func (c *Client) IsTokenValid() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.accessToken == "" {
		return false
	}
	// Simple token auth is always valid (doesn't expire)
	if c.useSimpleToken {
		return true
	}
	return time.Now().Add(DefaultTokenRefreshBuffer).Before(c.tokenExpiresAt)
}

// GetTokenExpiresAt returns when the current access token expires.
// This is useful for monitoring and alerting on token expiration.
func (c *Client) GetTokenExpiresAt() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tokenExpiresAt
}

// GetCircuitBreakerStats returns the current circuit breaker statistics.
// This is useful for monitoring the health of the VPSie API connection.
func (c *Client) GetCircuitBreakerStats() CircuitBreakerStats {
	if c.circuitBreaker == nil {
		return CircuitBreakerStats{}
	}
	return c.circuitBreaker.GetStats()
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

// ============================================================================
// Kubernetes Node Operations (VPSie Kubernetes Apps API)
// ============================================================================

// AddK8sNode adds a new worker node to a VPSie managed Kubernetes cluster.
// This uses the /apps/v2 API which has a different schema than the regular /vm API.
//
// The request requires:
//   - ResourceIdentifier: The VPSie Kubernetes cluster identifier
//   - ProjectID: The VPSie project ID
//   - DatacenterID: The datacenter where the node will be created
//   - BoxsizeID: The instance type/offering ID for the node
//
// Returns a VPS struct with the created node information.
func (c *Client) AddK8sNode(ctx context.Context, req AddK8sNodeRequest) (*VPS, error) {
	// Validate required fields
	if req.ResourceIdentifier == "" {
		return nil, NewConfigError("resource_identifier", "Resource identifier is required")
	}
	if req.ProjectID == "" {
		return nil, NewConfigError("project_id", "Project ID is required")
	}
	if req.DatacenterID == "" {
		return nil, NewConfigError("datacenter_id", "Datacenter ID is required")
	}
	if req.Hostname == "" {
		return nil, NewConfigError("hostname", "Hostname is required")
	}

	var response AddK8sNodeResponse

	// The /apps/v2 API uses /vm endpoint but with different fields than regular VM API
	if err := c.post(ctx, "/vm", req, &response); err != nil {
		return nil, fmt.Errorf("failed to add K8s node: %w", err)
	}

	// Check for API error in response
	if response.Error {
		return nil, NewAPIError(response.Code, "AddK8sNodeFailed", response.Message)
	}

	// Convert response to VPS struct
	vps := &VPS{
		ID:       response.Data.ID,
		Status:   response.Data.Status,
		Hostname: response.Data.Hostname,
	}

	return vps, nil
}

// CreateK8sNodeGroup creates a new node group in a VPSie managed Kubernetes cluster.
// This must be called before adding nodes to a new node group.
//
// Required fields:
//   - ClusterIdentifier: The VPSie Kubernetes cluster ID
//   - GroupName: Name for the node group
//   - KubeSizeID: The Kubernetes size/package ID (get from ListK8sOffers)
func (c *Client) CreateK8sNodeGroup(ctx context.Context, req CreateK8sNodeGroupRequest) (string, error) {
	// Validate required fields
	if req.ClusterIdentifier == "" {
		return "", NewConfigError("cluster_identifier", "Cluster identifier is required")
	}
	if req.GroupName == "" {
		return "", NewConfigError("group_name", "Group name is required")
	}
	if req.KubeSizeID == 0 {
		return "", NewConfigError("kube_size_id", "Kube size ID is required")
	}

	var response CreateK8sNodeGroupResponse

	// The API docs show GET but with a request body - using POST as that's more RESTful
	// Endpoint: /k8s/cluster/add/group (per curl example in API docs)
	if err := c.post(ctx, "/k8s/cluster/add/group", req, &response); err != nil {
		return "", fmt.Errorf("failed to create K8s node group: %w", err)
	}

	// Check for API error in response
	if response.Error {
		return "", NewAPIError(response.Code, "CreateK8sNodeGroupFailed", response.Message)
	}

	return response.Data.GroupID, nil
}

// ListK8sOffers lists available Kubernetes node size/package offerings.
// Use this to get valid KubeSizeID values for CreateK8sNodeGroup.
func (c *Client) ListK8sOffers(ctx context.Context, dcIdentifier string) ([]K8sOffer, error) {
	if dcIdentifier == "" {
		return nil, NewConfigError("dc_identifier", "Datacenter identifier is required")
	}

	var response ListK8sOffersResponse

	// POST /k8s/offers with dcIdentifier
	reqBody := map[string]string{"dcIdentifier": dcIdentifier}
	if err := c.post(ctx, "/k8s/offers", reqBody, &response); err != nil {
		return nil, fmt.Errorf("failed to list K8s offers: %w", err)
	}

	if response.Error {
		return nil, NewAPIError(response.Code, "ListK8sOffersFailed", response.Message)
	}

	return response.Data, nil
}

// ListK8sNodeGroups lists all node groups for a VPSie managed Kubernetes cluster.
// Returns the node groups with their numeric IDs needed for adding nodes.
func (c *Client) ListK8sNodeGroups(ctx context.Context, clusterIdentifier string) ([]K8sNodeGroup, error) {
	if clusterIdentifier == "" {
		return nil, NewConfigError("cluster_identifier", "Cluster identifier is required")
	}

	var response ListK8sNodeGroupsResponse

	// GET /k8s/node/groups/byClusterId/{clusterIdentifier}
	endpoint := fmt.Sprintf("/k8s/node/groups/byClusterId/%s", clusterIdentifier)
	if err := c.get(ctx, endpoint, &response); err != nil {
		return nil, fmt.Errorf("failed to list K8s node groups: %w", err)
	}

	if response.Error {
		return nil, NewAPIError(response.Code, "ListK8sNodeGroupsFailed", response.Message)
	}

	return response.Data, nil
}

// AddK8sSlaveToGroup adds a slave/worker node to a specific node group in a VPSie managed Kubernetes cluster.
// The groupID must be the numeric ID from ListK8sNodeGroups, not the UUID identifier.
// Note: The API may return data as a boolean (true) on success, in which case we return a VPS
// with Status="provisioning" and ID=0, indicating the node creation was initiated but ID is not yet known.
func (c *Client) AddK8sSlaveToGroup(ctx context.Context, clusterIdentifier string, groupID int) (*VPS, error) {
	if clusterIdentifier == "" {
		return nil, NewConfigError("cluster_identifier", "Cluster identifier is required")
	}
	if groupID == 0 {
		return nil, NewConfigError("group_id", "Numeric group ID is required")
	}

	// Use a flexible response structure that can handle data as bool or object
	var response struct {
		Error   bool            `json:"error"`
		Code    int             `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data,omitempty"`
	}

	// POST /k8s/cluster/byId/{clusterIdentifier}/add/slave/group/{groupID}
	endpoint := fmt.Sprintf("/k8s/cluster/byId/%s/add/slave/group/%d", clusterIdentifier, groupID)

	if err := c.post(ctx, endpoint, nil, &response); err != nil {
		return nil, fmt.Errorf("failed to add K8s slave node: %w", err)
	}

	if response.Error {
		return nil, NewAPIError(response.Code, "AddK8sSlaveToGroupFailed", response.Message)
	}

	// Try to unmarshal data as an object first
	var nodeData struct {
		ID         int    `json:"id"`
		Identifier string `json:"identifier"` // VPSie node UUID for K8s API operations
		Status     string `json:"status"`
		Hostname   string `json:"hostname"`
	}
	if err := json.Unmarshal(response.Data, &nodeData); err == nil && (nodeData.ID != 0 || nodeData.Identifier != "") {
		return &VPS{
			ID:         nodeData.ID,
			Identifier: nodeData.Identifier,
			Hostname:   nodeData.Hostname,
			Status:     nodeData.Status,
		}, nil
	}

	// If data is not an object (likely boolean true), return a placeholder VPS
	// indicating the request was accepted but node creation is in progress
	return &VPS{
		ID:     0, // Will be populated later by polling
		Status: "provisioning",
	}, nil
}

// AddK8sSlave adds a slave/worker node to a VPSie managed Kubernetes cluster.
// This is a simpler API than AddK8sNode - it adds a node using the cluster's default configuration.
// Deprecated: Use AddK8sSlaveToGroup instead with a specific numeric group ID.
func (c *Client) AddK8sSlave(ctx context.Context, clusterIdentifier string, groupID string) error {
	if clusterIdentifier == "" {
		return NewConfigError("cluster_identifier", "Cluster identifier is required")
	}

	var response struct {
		Error   bool   `json:"error"`
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	// POST /k8s/cluster/byId/{identifier}/add/slave
	endpoint := fmt.Sprintf("/k8s/cluster/byId/%s/add/slave", clusterIdentifier)

	var reqBody interface{}
	if groupID != "" {
		reqBody = map[string]string{"groupId": groupID}
	}

	if err := c.post(ctx, endpoint, reqBody, &response); err != nil {
		return fmt.Errorf("failed to add K8s slave node: %w", err)
	}

	if response.Error {
		return NewAPIError(response.Code, "AddK8sSlaveFailed", response.Message)
	}

	return nil
}

// DeleteK8sNode deletes a worker node from a VPSie managed Kubernetes cluster.
// This uses the K8s-specific deletion API which properly removes the node from the cluster.
//
// The clusterIdentifier is the UUID of the VPSie K8s cluster.
// The nodeIdentifier is the UUID of the specific node to delete.
//
// Example usage:
//
//	err := client.DeleteK8sNode(ctx, "cluster-uuid", "node-uuid")
//	if err != nil {
//	    log.Fatalf("failed to delete K8s node: %v", err)
//	}
func (c *Client) DeleteK8sNode(ctx context.Context, clusterIdentifier, nodeIdentifier string) error {
	if clusterIdentifier == "" {
		return NewConfigError("cluster_identifier", "Cluster identifier is required")
	}
	if nodeIdentifier == "" {
		return NewConfigError("node_identifier", "Node identifier is required")
	}

	var response DeleteK8sNodeResponse

	// DELETE /k8s/cluster/byId/{clusterIdentifier}/delete/slave
	// with body {"identifier": "nodeIdentifier"}
	endpoint := fmt.Sprintf("/k8s/cluster/byId/%s/delete/slave", clusterIdentifier)
	reqBody := DeleteK8sNodeRequest{
		Identifier: nodeIdentifier,
	}

	if err := c.deleteWithBody(ctx, endpoint, reqBody, &response); err != nil {
		// If node not found, consider it already deleted
		if IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to delete K8s node: %w", err)
	}

	if response.Error {
		return NewAPIError(response.Code, "DeleteK8sNodeFailed", response.Message)
	}

	return nil
}

// deleteWithBody performs a DELETE request with a JSON body
func (c *Client) deleteWithBody(ctx context.Context, path string, body, result interface{}) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if result != nil {
		// Limit response body size to prevent DoS attacks
		limitedReader := io.LimitReader(resp.Body, MaxResponseBodySize)
		if err := json.NewDecoder(limitedReader).Decode(result); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// ============================================================================
// Kubernetes Cluster Discovery Operations
// ============================================================================

// ListK8sClusters lists all Kubernetes clusters associated with the account.
// This is used for auto-discovery of cluster configuration.
func (c *Client) ListK8sClusters(ctx context.Context) ([]K8sCluster, error) {
	var response ListK8sClustersResponse

	// GET /k8s/cluster/all - list all K8s clusters
	if err := c.get(ctx, "/k8s/cluster/all", &response); err != nil {
		return nil, fmt.Errorf("failed to list K8s clusters: %w", err)
	}

	if response.Error {
		return nil, NewAPIError(response.Code, "ListK8sClustersFailed", response.Message)
	}

	return response.Data, nil
}

// GetK8sCluster retrieves details of a specific Kubernetes cluster by identifier.
func (c *Client) GetK8sCluster(ctx context.Context, clusterIdentifier string) (*K8sCluster, error) {
	if clusterIdentifier == "" {
		return nil, NewConfigError("cluster_identifier", "Cluster identifier is required")
	}

	var response GetK8sClusterResponse

	// GET /k8s/cluster/byId/{identifier}
	endpoint := fmt.Sprintf("/k8s/cluster/byId/%s", clusterIdentifier)
	if err := c.get(ctx, endpoint, &response); err != nil {
		return nil, fmt.Errorf("failed to get K8s cluster: %w", err)
	}

	if response.Error {
		return nil, NewAPIError(response.Code, "GetK8sClusterFailed", response.Message)
	}

	return &response.Data, nil
}

// FindK8sClusterByName finds a Kubernetes cluster by its name.
// Returns nil if no cluster is found with the given name.
func (c *Client) FindK8sClusterByName(ctx context.Context, name string) (*K8sCluster, error) {
	if name == "" {
		return nil, NewConfigError("name", "Cluster name is required")
	}

	clusters, err := c.ListK8sClusters(ctx)
	if err != nil {
		return nil, err
	}

	for i := range clusters {
		if clusters[i].Name == name {
			return &clusters[i], nil
		}
	}

	return nil, nil // Not found
}

// GetK8sClusterInfo retrieves detailed cluster information including node list.
// This is useful for looking up node identifiers by hostname.
func (c *Client) GetK8sClusterInfo(ctx context.Context, clusterIdentifier string) (*K8sClusterInfo, error) {
	if clusterIdentifier == "" {
		return nil, NewConfigError("cluster_identifier", "Cluster identifier is required")
	}

	var response GetK8sClusterInfoResponse

	// GET /k8s/cluster/byId/{identifier} - gets cluster info including nodes array
	// Note: This endpoint is under /apps/v2 in VPSie API, but we use relative path from baseURL
	endpoint := fmt.Sprintf("/k8s/cluster/byId/%s", clusterIdentifier)

	c.logger.Debug("GetK8sClusterInfo: calling API",
		zap.String("endpoint", endpoint),
		zap.String("clusterIdentifier", clusterIdentifier),
	)

	if err := c.get(ctx, endpoint, &response); err != nil {
		return nil, fmt.Errorf("failed to get K8s cluster info: %w", err)
	}

	c.logger.Debug("GetK8sClusterInfo: API response",
		zap.Int("nodesCount", len(response.Data.Nodes)),
		zap.String("clusterName", response.Data.Name),
	)

	if response.Error {
		return nil, NewAPIError(response.Code, "GetK8sClusterInfoFailed", response.Message)
	}

	return &response.Data, nil
}

// FindK8sNodeIdentifier looks up a node's identifier by hostname in a cluster.
// This is used during node deletion when VPSieNodeIdentifier is not set.
// Returns empty string if node is not found.
func (c *Client) FindK8sNodeIdentifier(ctx context.Context, clusterIdentifier, hostname string) (string, error) {
	if clusterIdentifier == "" || hostname == "" {
		return "", nil
	}

	info, err := c.GetK8sClusterInfo(ctx, clusterIdentifier)
	if err != nil {
		// If cluster info endpoint doesn't exist, return empty without error
		// The caller will fall back to other deletion methods
		if IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to get cluster info: %w", err)
	}

	// Normalize hostname for case-insensitive comparison
	// VPSie API may return different casing than K8s node names
	hostnameNormalized := strings.ToLower(hostname)

	// Log what we received from the API
	c.logger.Debug("FindK8sNodeIdentifier: cluster info received",
		zap.Int("nodesCount", len(info.Nodes)),
		zap.Int("slavesCount", len(info.Slaves)),
		zap.Int("mastersCount", len(info.Masters)),
		zap.String("searchingFor", hostnameNormalized),
	)

	// Search in nodes array first (returned by /k8s/cluster/byId/{id} endpoint)
	for _, node := range info.Nodes {
		if strings.ToLower(node.Hostname) == hostnameNormalized {
			return node.Identifier, nil
		}
	}

	// Fallback: search in slaves (for /info endpoint compatibility)
	for _, node := range info.Slaves {
		if strings.ToLower(node.Hostname) == hostnameNormalized {
			return node.Identifier, nil
		}
	}

	// Fallback: search in masters
	for _, node := range info.Masters {
		if strings.ToLower(node.Hostname) == hostnameNormalized {
			return node.Identifier, nil
		}
	}

	return "", nil // Not found
}
