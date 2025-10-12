package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// ============================================================================
// Test Helpers
// ============================================================================

// createTestSecret creates a Kubernetes secret for testing with OAuth credentials
func createTestSecret(name, namespace, clientID, clientSecret, url string) *corev1.Secret {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{},
	}

	if clientID != "" {
		secret.Data[SecretClientIDKey] = []byte(clientID)
	}

	if clientSecret != "" {
		secret.Data[SecretClientSecretKey] = []byte(clientSecret)
	}

	if url != "" {
		secret.Data[SecretURLKey] = []byte(url)
	}

	return secret
}

// createTestServer creates a test HTTP server for mocking VPSie API with authentication
func createTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock VPSie authentication endpoint
		if r.URL.Path == TokenEndpoint {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(TokenResponse{
				Token:     "mock-access-token-" + time.Now().Format("20060102150405"),
				ExpiresIn: 3600,
			})
			return
		}
		// Delegate to custom handler for API endpoints
		handler(w, r)
	}))
}

// ============================================================================
// Kubernetes Secret Tests
// ============================================================================

func TestNewClient_Success(t *testing.T) {
	// Create fake Kubernetes client with secret
	secret := createTestSecret(
		DefaultSecretName,
		DefaultSecretNamespace,
		"test-client-id",
		"test-client-secret",
		"https://api.vpsie.test/v2",
	)

	fakeClient := fake.NewSimpleClientset(secret)

	// Create VPSie client
	ctx := context.Background()
	client, err := NewClient(ctx, fakeClient, nil)

	// Assertions
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "test-client-id", client.clientID)
	assert.Equal(t, "test-client-secret", client.clientSecret)
	assert.NotEmpty(t, client.accessToken, "access token should be obtained")
	assert.Equal(t, "https://api.vpsie.test/v2", client.baseURL)
	assert.Equal(t, "vpsie-k8s-autoscaler/1.0", client.userAgent)
}

func TestNewClient_WithCustomOptions(t *testing.T) {
	secret := createTestSecret(
		"custom-secret",
		"custom-namespace",
		"custom-client-id",
		"custom-client-secret",
		"https://custom.api/v2",
	)

	fakeClient := fake.NewSimpleClientset(secret)

	ctx := context.Background()
	client, err := NewClient(ctx, fakeClient, &ClientOptions{
		SecretName:      "custom-secret",
		SecretNamespace: "custom-namespace",
		RateLimit:       200,
		Timeout:         60 * time.Second,
		UserAgent:       "custom-agent/2.0",
	})

	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "custom-client-id", client.clientID)
	assert.Equal(t, "custom-client-secret", client.clientSecret)
	assert.NotEmpty(t, client.accessToken)
	assert.Equal(t, "https://custom.api/v2", client.baseURL)
	assert.Equal(t, "custom-agent/2.0", client.userAgent)
}

func TestNewClient_DefaultURL(t *testing.T) {
	// Secret without URL - should use default
	secret := createTestSecret(
		DefaultSecretName,
		DefaultSecretNamespace,
		"test-client-id",
		"test-client-secret",
		"", // No URL
	)

	fakeClient := fake.NewSimpleClientset(secret)

	ctx := context.Background()
	client, err := NewClient(ctx, fakeClient, nil)

	require.NoError(t, err)
	assert.Equal(t, DefaultAPIEndpoint, client.baseURL)
}

func TestNewClient_SecretNotFound(t *testing.T) {
	// Create fake client without the secret
	fakeClient := fake.NewSimpleClientset()

	ctx := context.Background()
	client, err := NewClient(ctx, fakeClient, nil)

	// Should return SecretError
	assert.Nil(t, client)
	assert.Error(t, err)

	var secretErr *SecretError
	require.True(t, errors.As(err, &secretErr), "error should be SecretError")
	assert.Equal(t, DefaultSecretName, secretErr.SecretName)
	assert.Equal(t, DefaultSecretNamespace, secretErr.SecretNamespace)
}

func TestNewClient_MissingClientIDKey(t *testing.T) {
	// Secret without clientId key
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultSecretName,
			Namespace: DefaultSecretNamespace,
		},
		Data: map[string][]byte{
			// Missing clientId key
			SecretClientSecretKey: []byte("test-client-secret"),
			SecretURLKey:          []byte("https://api.vpsie.test/v2"),
		},
	}

	fakeClient := fake.NewSimpleClientset(secret)

	ctx := context.Background()
	client, err := NewClient(ctx, fakeClient, nil)

	assert.Nil(t, client)
	assert.Error(t, err)

	var secretErr *SecretError
	require.True(t, errors.As(err, &secretErr))
	assert.Contains(t, secretErr.Reason, "does not contain 'clientId' key")
}

func TestNewClient_MissingClientSecretKey(t *testing.T) {
	// Secret without clientSecret key
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultSecretName,
			Namespace: DefaultSecretNamespace,
		},
		Data: map[string][]byte{
			SecretClientIDKey: []byte("test-client-id"),
			// Missing clientSecret key
			SecretURLKey: []byte("https://api.vpsie.test/v2"),
		},
	}

	fakeClient := fake.NewSimpleClientset(secret)

	ctx := context.Background()
	client, err := NewClient(ctx, fakeClient, nil)

	assert.Nil(t, client)
	assert.Error(t, err)

	var secretErr *SecretError
	require.True(t, errors.As(err, &secretErr))
	assert.Contains(t, secretErr.Reason, "does not contain 'clientSecret' key")
}

func TestNewClient_EmptyClientID(t *testing.T) {
	// Secret with empty clientId
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultSecretName,
			Namespace: DefaultSecretNamespace,
		},
		Data: map[string][]byte{
			SecretClientIDKey:     []byte(""), // Explicitly empty clientId
			SecretClientSecretKey: []byte("test-client-secret"),
			SecretURLKey:          []byte("https://api.vpsie.test/v2"),
		},
	}

	fakeClient := fake.NewSimpleClientset(secret)

	ctx := context.Background()
	client, err := NewClient(ctx, fakeClient, nil)

	assert.Nil(t, client)
	assert.Error(t, err)

	var secretErr *SecretError
	require.True(t, errors.As(err, &secretErr))
	assert.Contains(t, secretErr.Reason, "is empty")
}

func TestNewClient_EmptyClientSecret(t *testing.T) {
	// Secret with empty clientSecret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultSecretName,
			Namespace: DefaultSecretNamespace,
		},
		Data: map[string][]byte{
			SecretClientIDKey:     []byte("test-client-id"),
			SecretClientSecretKey: []byte(""), // Explicitly empty clientSecret
			SecretURLKey:          []byte("https://api.vpsie.test/v2"),
		},
	}

	fakeClient := fake.NewSimpleClientset(secret)

	ctx := context.Background()
	client, err := NewClient(ctx, fakeClient, nil)

	assert.Nil(t, client)
	assert.Error(t, err)

	var secretErr *SecretError
	require.True(t, errors.As(err, &secretErr))
	assert.Contains(t, secretErr.Reason, "is empty")
}

func TestNewClient_URLTrimming(t *testing.T) {
	tests := []struct {
		name        string
		inputURL    string
		expectedURL string
	}{
		{
			name:        "URL with trailing slash",
			inputURL:    "https://api.vpsie.test/v2/",
			expectedURL: "https://api.vpsie.test/v2",
		},
		{
			name:        "URL with multiple trailing slashes",
			inputURL:    "https://api.vpsie.test/v2///",
			expectedURL: "https://api.vpsie.test/v2",
		},
		{
			name:        "URL without trailing slash",
			inputURL:    "https://api.vpsie.test/v2",
			expectedURL: "https://api.vpsie.test/v2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := createTestSecret(
				DefaultSecretName,
				DefaultSecretNamespace,
				"test-client-id",
				"test-client-secret",
				tt.inputURL,
			)

			fakeClient := fake.NewSimpleClientset(secret)

			ctx := context.Background()
			client, err := NewClient(ctx, fakeClient, nil)

			require.NoError(t, err)
			assert.Equal(t, tt.expectedURL, client.baseURL)
		})
	}
}

// ============================================================================
// NewClientWithCredentials Tests
// ============================================================================

func TestNewClientWithCredentials_Success(t *testing.T) {
	client, err := NewClientWithCredentials(
		"https://api.vpsie.test/v2",
		"test-client-id",
		"test-client-secret",
		&ClientOptions{
			RateLimit: 50,
			UserAgent: "test-agent",
		},
	)

	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, "test-client-id", client.clientID)
	assert.Equal(t, "test-client-secret", client.clientSecret)
	assert.NotEmpty(t, client.accessToken)
	assert.Equal(t, "https://api.vpsie.test/v2", client.baseURL)
	assert.Equal(t, "test-agent", client.userAgent)
}

func TestNewClientWithCredentials_EmptyClientID(t *testing.T) {
	client, err := NewClientWithCredentials(
		"https://api.vpsie.test/v2",
		"", // Empty client ID
		"test-client-secret",
		nil,
	)

	assert.Nil(t, client)
	assert.Error(t, err)

	var configErr *ConfigError
	require.True(t, errors.As(err, &configErr))
	assert.Equal(t, "client_id", configErr.Field)
}

func TestNewClientWithCredentials_EmptyClientSecret(t *testing.T) {
	client, err := NewClientWithCredentials(
		"https://api.vpsie.test/v2",
		"test-client-id",
		"", // Empty client secret
		nil,
	)

	assert.Nil(t, client)
	assert.Error(t, err)

	var configErr *ConfigError
	require.True(t, errors.As(err, &configErr))
	assert.Equal(t, "client_secret", configErr.Field)
}

func TestNewClientWithCredentials_DefaultURL(t *testing.T) {
	client, err := NewClientWithCredentials(
		"", // Empty URL
		"test-client-id",
		"test-client-secret",
		nil,
	)

	require.NoError(t, err)
	assert.Equal(t, DefaultAPIEndpoint, client.baseURL)
}

// ============================================================================
// HTTP Request Building Tests
// ============================================================================

func TestClient_RequestHeaders(t *testing.T) {
	// Create test server to inspect request
	var capturedReq *http.Request
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		capturedReq = r
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ListVPSResponse{Data: []VPS{}})
	})
	defer server.Close()

	// Create client
	client, err := NewClientWithCredentials(
		server.URL,
		"test-client-id",
		"test-client-secret",
		&ClientOptions{
			UserAgent: "test-user-agent/1.0",
		},
	)
	require.NoError(t, err)

	// Make request
	ctx := context.Background()
	_, err = client.ListVMs(ctx)
	require.NoError(t, err)

	// Verify headers
	assert.Equal(t, "Bearer test-bearer-token", capturedReq.Header.Get("Authorization"))
	assert.Equal(t, "test-user-agent/1.0", capturedReq.Header.Get("User-Agent"))
	assert.Equal(t, "application/json", capturedReq.Header.Get("Accept"))
}

func TestClient_URLConstruction(t *testing.T) {
	tests := []struct {
		name         string
		baseURL      string
		operation    func(*Client) error
		expectedPath string
	}{
		{
			name:    "ListVMs",
			baseURL: "https://api.test",
			operation: func(c *Client) error {
				_, err := c.ListVMs(context.Background())
				return err
			},
			expectedPath: "/vms",
		},
		{
			name:    "GetVM",
			baseURL: "https://api.test",
			operation: func(c *Client) error {
				_, err := c.GetVM(context.Background(), "vm-123")
				return err
			},
			expectedPath: "/vms/vm-123",
		},
		{
			name:    "DeleteVM",
			baseURL: "https://api.test",
			operation: func(c *Client) error {
				return c.DeleteVM(context.Background(), "vm-456")
			},
			expectedPath: "/vms/vm-456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedPath string
			server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
				capturedPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)

				// Return appropriate response
				if r.Method == http.MethodGet && r.URL.Path == "/vms" {
					_ = json.NewEncoder(w).Encode(ListVPSResponse{Data: []VPS{}})
				} else if r.Method == http.MethodGet {
					_ = json.NewEncoder(w).Encode(VPS{ID: "vm-123"})
				}
			})
			defer server.Close()

			client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
			require.NoError(t, err)

			_ = tt.operation(client)
			assert.Equal(t, tt.expectedPath, capturedPath)
		})
	}
}

func TestClient_RateLimiting(t *testing.T) {
	requestCount := 0
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ListVPSResponse{Data: []VPS{}})
	})
	defer server.Close()

	// Create client with rate limit
	client, err := NewClientWithCredentials(
		server.URL,
		"test-client-id",
		"test-client-secret",
		&ClientOptions{
			RateLimit: 100, // 100 per minute
		},
	)
	require.NoError(t, err)

	ctx := context.Background()

	// Make multiple requests
	for i := 0; i < 3; i++ {
		_, err = client.ListVMs(ctx)
		require.NoError(t, err)
	}

	// Should have made 3 requests
	assert.Equal(t, 3, requestCount)

	// Verify rate limiter exists and is configured
	assert.NotNil(t, client.rateLimiter)
	// Note: Token bucket algorithm allows bursts, so exact timing tests are unreliable.
	// The important thing is that the rate limiter is configured and will enforce limits
	// when sustained traffic exceeds the rate.
}

func TestClient_ContextCancellation(t *testing.T) {
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	// Create context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Request should fail due to cancelled context
	_, err = client.ListVMs(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestClient_ContextTimeout(t *testing.T) {
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow response
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Request should fail due to timeout
	_, err = client.ListVMs(ctx)
	assert.Error(t, err)
}

// ============================================================================
// VPS Operation Tests - ListVMs
// ============================================================================

func TestListVMs_Success(t *testing.T) {
	expectedVMs := []VPS{
		{
			ID:        "vm-1",
			Name:      "test-vm-1",
			Status:    "running",
			CPU:       2,
			RAM:       4096,
			Disk:      50,
			IPAddress: "192.168.1.10",
		},
		{
			ID:        "vm-2",
			Name:      "test-vm-2",
			Status:    "stopped",
			CPU:       4,
			RAM:       8192,
			Disk:      100,
			IPAddress: "192.168.1.11",
		},
	}

	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/vms", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ListVPSResponse{
			Data: expectedVMs,
			Pagination: Pagination{
				Total:       2,
				Count:       2,
				CurrentPage: 1,
				TotalPages:  1,
			},
		})
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	ctx := context.Background()
	vms, err := client.ListVMs(ctx)

	require.NoError(t, err)
	assert.Len(t, vms, 2)
	assert.Equal(t, "vm-1", vms[0].ID)
	assert.Equal(t, "test-vm-1", vms[0].Name)
	assert.Equal(t, "running", vms[0].Status)
	assert.Equal(t, "vm-2", vms[1].ID)
}

func TestListVMs_EmptyList(t *testing.T) {
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ListVPSResponse{
			Data:       []VPS{},
			Pagination: Pagination{Total: 0},
		})
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	vms, err := client.ListVMs(context.Background())

	require.NoError(t, err)
	assert.Empty(t, vms)
}

func TestListVMs_APIError(t *testing.T) {
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req-123")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid API token",
			Code:    401,
		})
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	vms, err := client.ListVMs(context.Background())

	assert.Nil(t, vms)
	assert.Error(t, err)

	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 401, apiErr.StatusCode)
	assert.Equal(t, "req-123", apiErr.RequestID)
	assert.True(t, apiErr.IsUnauthorized())
}

// ============================================================================
// VPS Operation Tests - CreateVM
// ============================================================================

func TestCreateVM_Success(t *testing.T) {
	req := CreateVPSRequest{
		Name:         "test-vm",
		Hostname:     "test-vm.example.com",
		OfferingID:   "offering-123",
		DatacenterID: "dc-456",
		OSImageID:    "ubuntu-22.04",
		SSHKeyIDs:    []string{"key-789"},
		Tags:         []string{"test", "k8s"},
	}

	expectedVM := VPS{
		ID:           "vm-new-123",
		Name:         "test-vm",
		Hostname:     "test-vm.example.com",
		Status:       "creating",
		CPU:          2,
		RAM:          4096,
		Disk:         50,
		IPAddress:    "192.168.1.100",
		OfferingID:   "offering-123",
		DatacenterID: "dc-456",
		Tags:         []string{"test", "k8s"},
	}

	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/vms", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		// Verify request body
		var receivedReq CreateVPSRequest
		err := json.NewDecoder(r.Body).Decode(&receivedReq)
		require.NoError(t, err)
		assert.Equal(t, req.Name, receivedReq.Name)
		assert.Equal(t, req.OfferingID, receivedReq.OfferingID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(expectedVM)
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	vm, err := client.CreateVM(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "vm-new-123", vm.ID)
	assert.Equal(t, "test-vm", vm.Name)
	assert.Equal(t, "creating", vm.Status)
	assert.Equal(t, "192.168.1.100", vm.IPAddress)
}

func TestCreateVM_DefaultHostname(t *testing.T) {
	req := CreateVPSRequest{
		Name:         "auto-hostname-vm",
		OfferingID:   "offering-123",
		DatacenterID: "dc-456",
		OSImageID:    "ubuntu-22.04",
		// Hostname not provided
	}

	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var receivedReq CreateVPSRequest
		_ = json.NewDecoder(r.Body).Decode(&receivedReq)

		// Hostname should default to name
		assert.Equal(t, "auto-hostname-vm", receivedReq.Hostname)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(VPS{ID: "vm-123", Name: "auto-hostname-vm"})
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	_, err = client.CreateVM(context.Background(), req)
	require.NoError(t, err)
}

func TestCreateVM_ValidationErrors(t *testing.T) {
	tests := []struct {
		name          string
		req           CreateVPSRequest
		expectedErr   string
		expectedField string
	}{
		{
			name: "missing name",
			req: CreateVPSRequest{
				OfferingID:   "offering-123",
				DatacenterID: "dc-456",
				OSImageID:    "ubuntu-22.04",
			},
			expectedErr:   "name",
			expectedField: "name",
		},
		{
			name: "missing offering ID",
			req: CreateVPSRequest{
				Name:         "test-vm",
				DatacenterID: "dc-456",
				OSImageID:    "ubuntu-22.04",
			},
			expectedErr:   "offering_id",
			expectedField: "offering_id",
		},
		{
			name: "missing datacenter ID",
			req: CreateVPSRequest{
				Name:       "test-vm",
				OfferingID: "offering-123",
				OSImageID:  "ubuntu-22.04",
			},
			expectedErr:   "datacenter_id",
			expectedField: "datacenter_id",
		},
		{
			name: "missing OS image ID",
			req: CreateVPSRequest{
				Name:         "test-vm",
				OfferingID:   "offering-123",
				DatacenterID: "dc-456",
			},
			expectedErr:   "os_image_id",
			expectedField: "os_image_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClientWithCredentials("https://api.test", "test-client-id", "test-client-secret", nil)
			require.NoError(t, err)

			vm, err := client.CreateVM(context.Background(), tt.req)

			assert.Nil(t, vm)
			assert.Error(t, err)

			var configErr *ConfigError
			require.True(t, errors.As(err, &configErr), "error should be ConfigError")
			assert.Equal(t, tt.expectedField, configErr.Field)
			assert.Contains(t, configErr.Reason, "required")
		})
	}
}

// ============================================================================
// VPS Operation Tests - GetVM
// ============================================================================

func TestGetVM_Success(t *testing.T) {
	expectedVM := VPS{
		ID:           "vm-123",
		Name:         "test-vm",
		Status:       "running",
		CPU:          4,
		RAM:          8192,
		Disk:         100,
		IPAddress:    "192.168.1.50",
		OfferingID:   "offering-123",
		DatacenterID: "dc-456",
	}

	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/vms/vm-123", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(expectedVM)
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	vm, err := client.GetVM(context.Background(), "vm-123")

	require.NoError(t, err)
	assert.Equal(t, "vm-123", vm.ID)
	assert.Equal(t, "test-vm", vm.Name)
	assert.Equal(t, "running", vm.Status)
	assert.Equal(t, 4, vm.CPU)
}

func TestGetVM_NotFound(t *testing.T) {
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "Not Found",
			Message: "VM not found",
			Code:    404,
		})
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	vm, err := client.GetVM(context.Background(), "vm-nonexistent")

	assert.Nil(t, vm)
	assert.Error(t, err)
	assert.True(t, IsNotFound(err))
}

func TestGetVM_EmptyID(t *testing.T) {
	client, err := NewClientWithCredentials("https://api.test", "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	vm, err := client.GetVM(context.Background(), "")

	assert.Nil(t, vm)
	assert.Error(t, err)

	var configErr *ConfigError
	require.True(t, errors.As(err, &configErr))
	assert.Equal(t, "vm_id", configErr.Field)
}

// ============================================================================
// VPS Operation Tests - DeleteVM
// ============================================================================

func TestDeleteVM_Success(t *testing.T) {
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/vms/vm-123", r.URL.Path)

		w.WriteHeader(http.StatusNoContent)
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	err = client.DeleteVM(context.Background(), "vm-123")
	assert.NoError(t, err)
}

func TestDeleteVM_AlreadyDeleted(t *testing.T) {
	// Server returns 404 - VM already deleted
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "Not Found",
			Message: "VM not found",
			Code:    404,
		})
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	// 404 should be treated as success (idempotent)
	err = client.DeleteVM(context.Background(), "vm-already-deleted")
	assert.NoError(t, err, "DeleteVM should treat 404 as success")
}

func TestDeleteVM_Conflict(t *testing.T) {
	// Server returns 409 - VM is running, can't delete
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "Conflict",
			Message: "VM must be stopped before deletion",
			Code:    409,
		})
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	err = client.DeleteVM(context.Background(), "vm-running")
	assert.Error(t, err)

	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 409, apiErr.StatusCode)
}

func TestDeleteVM_EmptyID(t *testing.T) {
	client, err := NewClientWithCredentials("https://api.test", "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	err = client.DeleteVM(context.Background(), "")
	assert.Error(t, err)

	var configErr *ConfigError
	require.True(t, errors.As(err, &configErr))
	assert.Equal(t, "vm_id", configErr.Field)
}

// ============================================================================
// Error Handling Tests
// ============================================================================

func TestAPIError_Parsing(t *testing.T) {
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req-abc-123")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid parameters provided",
			Code:    400,
		})
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	_, err = client.ListVMs(context.Background())

	require.Error(t, err)

	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.Equal(t, 400, apiErr.StatusCode)
	assert.Equal(t, "Bad Request", apiErr.Message)
	assert.Equal(t, "Invalid parameters provided", apiErr.Details)
	assert.Equal(t, "req-abc-123", apiErr.RequestID)
}

func TestAPIError_RateLimit(t *testing.T) {
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "Rate Limit Exceeded",
			Message: "Too many requests",
			Code:    429,
		})
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	_, err = client.ListVMs(context.Background())

	require.Error(t, err)
	assert.True(t, IsRateLimited(err))

	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.True(t, apiErr.IsRateLimited())
}

func TestAPIError_ServerError(t *testing.T) {
	server := createTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	})
	defer server.Close()

	client, err := NewClientWithCredentials(server.URL, "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	_, err = client.ListVMs(context.Background())

	require.Error(t, err)

	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.True(t, apiErr.IsServerError())
	assert.Equal(t, 500, apiErr.StatusCode)
}

// ============================================================================
// UpdateCredentials Tests
// ============================================================================
// Note: UpdateCredentials removed in OAuth implementation - tokens are auto-refreshed
// ============================================================================
// Helper Method Tests
// ============================================================================

func TestGetBaseURL(t *testing.T) {
	client, err := NewClientWithCredentials("https://api.test/v2", "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	assert.Equal(t, "https://api.test/v2", client.GetBaseURL())
}

func TestSetUserAgent(t *testing.T) {
	client, err := NewClientWithCredentials("https://api.test", "test-client-id", "test-client-secret", nil)
	require.NoError(t, err)

	client.SetUserAgent("new-agent/2.0")
	assert.Equal(t, "new-agent/2.0", client.userAgent)
}
