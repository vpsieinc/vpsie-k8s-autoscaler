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

	"golang.org/x/time/rate"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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

	// SecretTokenKey is the key name for the API token in the secret
	SecretTokenKey = "token"

	// SecretURLKey is the key name for the API URL in the secret
	SecretURLKey = "url"
)

// Client represents a VPSie API client
type Client struct {
	httpClient  *http.Client
	rateLimiter *rate.Limiter
	baseURL     string
	token       string
	userAgent   string
	mu          sync.RWMutex
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

	// Extract token from secret
	tokenBytes, ok := secret.Data[SecretTokenKey]
	if !ok {
		return nil, NewSecretError(opts.SecretName, opts.SecretNamespace,
			fmt.Sprintf("secret does not contain '%s' key", SecretTokenKey), nil)
	}
	token := string(tokenBytes)
	if token == "" {
		return nil, NewSecretError(opts.SecretName, opts.SecretNamespace,
			fmt.Sprintf("secret key '%s' is empty", SecretTokenKey), nil)
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

	return &Client{
		httpClient:  httpClient,
		rateLimiter: rateLimiter,
		baseURL:     baseURL,
		token:       token,
		userAgent:   opts.UserAgent,
	}, nil
}

// NewClientWithCredentials creates a new VPSie API client with explicit credentials (for testing)
func NewClientWithCredentials(baseURL, token string, opts *ClientOptions) (*Client, error) {
	if opts == nil {
		opts = &ClientOptions{}
	}

	if token == "" {
		return nil, NewConfigError("token", "API token cannot be empty")
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

	return &Client{
		httpClient:  httpClient,
		rateLimiter: rateLimiter,
		baseURL:     baseURL,
		token:       token,
		userAgent:   opts.UserAgent,
	}, nil
}

// doRequest performs an HTTP request with authentication and rate limiting
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	// Wait for rate limiter
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter error: %w", err)
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
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", c.userAgent)
	c.mu.RUnlock()

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	// Perform request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform request: %w", err)
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

// UpdateCredentials updates the API credentials (useful for credential rotation)
func (c *Client) UpdateCredentials(token, baseURL string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if token == "" {
		return NewConfigError("token", "API token cannot be empty")
	}

	c.token = token

	if baseURL != "" {
		c.baseURL = strings.TrimRight(baseURL, "/")
	}

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
// This method performs a GET request to /vms and returns all VPS instances.
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

	// Perform GET request to /vms endpoint
	if err := c.get(ctx, "/vms", &response); err != nil {
		return nil, fmt.Errorf("failed to list VMs: %w", err)
	}

	return response.Data, nil
}

// CreateVM creates a new VPS instance with the specified configuration.
//
// This method performs a POST request to /vms with the provided configuration.
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

	// Perform POST request to /vms endpoint
	if err := c.post(ctx, "/vms", req, &vps); err != nil {
		return nil, fmt.Errorf("failed to create VM '%s': %w", req.Name, err)
	}

	return &vps, nil
}

// GetVM retrieves detailed information about a specific VPS instance.
//
// This method performs a GET request to /vms/{id} and returns the full VPS details
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
func (c *Client) GetVM(ctx context.Context, vmID string) (*VPS, error) {
	if vmID == "" {
		return nil, NewConfigError("vm_id", "VM ID is required")
	}

	var vps VPS

	// Perform GET request to /vms/{id} endpoint
	path := fmt.Sprintf("/vms/%s", vmID)
	if err := c.get(ctx, path, &vps); err != nil {
		return nil, fmt.Errorf("failed to get VM '%s': %w", vmID, err)
	}

	return &vps, nil
}

// DeleteVM deletes a VPS instance.
//
// This method performs a DELETE request to /vms/{id} to permanently delete the VPS.
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
func (c *Client) DeleteVM(ctx context.Context, vmID string) error {
	if vmID == "" {
		return NewConfigError("vm_id", "VM ID is required")
	}

	// Perform DELETE request to /vms/{id} endpoint
	path := fmt.Sprintf("/vms/%s", vmID)
	err := c.delete(ctx, path)

	// If the VM is not found (404), consider it a success since the VM is already deleted
	// This makes the deletion operation idempotent
	if err != nil && IsNotFound(err) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("failed to delete VM '%s': %w", vmID, err)
	}

	return nil
}
