//go:build integration
// +build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// TestMockVPSieServer_OAuthAuthentication tests OAuth token endpoint
func TestMockVPSieServer_OAuthAuthentication(t *testing.T) {
	server := NewMockVPSieServer()
	defer server.Close()

	tests := []struct {
		name         string
		grantType    string
		clientID     string
		clientSecret string
		expectError  bool
	}{
		{
			name:         "Valid client credentials",
			grantType:    "client_credentials",
			clientID:     "test-client",
			clientSecret: "test-secret",
			expectError:  false,
		},
		{
			name:         "Missing client ID",
			grantType:    "client_credentials",
			clientID:     "",
			clientSecret: "test-secret",
			expectError:  true,
		},
		{
			name:         "Unsupported grant type",
			grantType:    "password",
			clientID:     "test-client",
			clientSecret: "test-secret",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqBody := map[string]string{
				"grant_type":    tt.grantType,
				"client_id":     tt.clientID,
				"client_secret": tt.clientSecret,
			}

			body, _ := json.Marshal(reqBody)
			resp, err := http.Post(server.URL()+"/oauth/token", "application/json", bytes.NewBuffer(body))
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			if tt.expectError && resp.StatusCode == http.StatusOK {
				t.Errorf("Expected error but got success")
			} else if !tt.expectError && resp.StatusCode != http.StatusOK {
				t.Errorf("Expected success but got status %d", resp.StatusCode)
			}

			if !tt.expectError {
				var tokenResp map[string]interface{}
				json.NewDecoder(resp.Body).Decode(&tokenResp)

				if tokenResp["access_token"] == nil {
					t.Error("Missing access_token in response")
				}
				if tokenResp["refresh_token"] == nil {
					t.Error("Missing refresh_token in response")
				}
			}
		})
	}
}

// TestMockVPSieServer_TokenExpiration tests token expiration handling
func TestMockVPSieServer_TokenExpiration(t *testing.T) {
	server := NewMockVPSieServer()
	defer server.Close()

	// Get initial token
	token := server.AuthToken

	// Create VM with valid token
	vm := createTestVM(t, server, token, "test-vm-1", false)
	if vm == nil {
		t.Fatal("Failed to create VM with valid token")
	}

	// Expire the token
	server.ExpireToken()

	// Try to create VM with expired token
	vm = createTestVM(t, server, token, "test-vm-2", true)
	if vm != nil {
		t.Error("Should not create VM with expired token")
	}

	// Refresh token
	newToken := server.RefreshAuthToken()
	if newToken == token {
		t.Error("New token should be different from expired token")
	}

	// Create VM with new token
	vm = createTestVM(t, server, newToken, "test-vm-3", false)
	if vm == nil {
		t.Fatal("Failed to create VM with new token")
	}
}

// TestMockVPSieServer_VMLifecycle tests VM lifecycle operations
func TestMockVPSieServer_VMLifecycle(t *testing.T) {
	server := NewMockVPSieServer()
	defer server.Close()

	token := server.AuthToken

	// Create a VM
	vmReq := vpsieclient.CreateVPSRequest{
		Name:         "test-vm",
		Hostname:     "test.example.com",
		OfferingID:   "medium-4cpu-8gb",
		DatacenterID: "dc-us-east-1",
		OSImageID:    "ubuntu-22.04",
		Tags:         []string{"test", "integration"},
		Notes:        "Test VM for integration testing",
	}

	body, _ := json.Marshal(vmReq)
	req, _ := http.NewRequest("POST", server.URL()+"/v2/vms", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to create VM: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("Expected status 201, got %d", resp.StatusCode)
	}

	var createResp struct {
		Data vpsieclient.VPS `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&createResp)
	vmID := createResp.Data.ID

	// Verify VM is in provisioning state
	if createResp.Data.Status != "provisioning" {
		t.Errorf("Expected status 'provisioning', got %s", createResp.Data.Status)
	}

	// Get VM details
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/v2/vms/%d", server.URL(), vmID), nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to get VM: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// List all VMs
	req, _ = http.NewRequest("GET", server.URL()+"/v2/vms", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to list VMs: %v", err)
	}
	defer resp.Body.Close()

	var listResp struct {
		Data []vpsieclient.VPS `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&listResp)

	if len(listResp.Data) != 1 {
		t.Errorf("Expected 1 VM, got %d", len(listResp.Data))
	}

	// Delete VM
	req, _ = http.NewRequest("DELETE", fmt.Sprintf("%s/v2/vms/%d", server.URL(), vmID), nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to delete VM: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// Verify VM is deleted
	req, _ = http.NewRequest("GET", fmt.Sprintf("%s/v2/vms/%d", server.URL(), vmID), nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("Failed to get VM: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected status 404 for deleted VM, got %d", resp.StatusCode)
	}
}

// TestMockVPSieServer_StateTransitions tests automatic VM state transitions
func TestMockVPSieServer_StateTransitions(t *testing.T) {
	server := NewMockVPSieServer()
	defer server.Close()

	// Configure fast transitions for testing
	server.StateTransitions = []VMStateTransition{
		{FromState: "provisioning", ToState: "running", Duration: 2 * time.Second},
		{FromState: "running", ToState: "ready", Duration: 1 * time.Second},
	}

	token := server.AuthToken

	// Create a VM
	vm := createTestVM(t, server, token, "test-transition-vm", false)
	if vm == nil {
		t.Fatal("Failed to create VM")
	}

	// Initial state should be provisioning
	if vm.Status != "provisioning" {
		t.Errorf("Expected initial status 'provisioning', got %s", vm.Status)
	}

	// Wait for first transition
	time.Sleep(3 * time.Second)

	// Get VM and check state
	vm, err := getVM(server, token, vm.ID)
	if err != nil {
		t.Fatalf("Failed to get VM: %v", err)
	}

	if vm.Status != "running" {
		t.Errorf("Expected status 'running' after transition, got %s", vm.Status)
	}

	// Wait for second transition
	time.Sleep(2 * time.Second)

	// Get VM and check final state
	vm, err = getVM(server, token, vm.ID)
	if err != nil {
		t.Fatalf("Failed to get VM: %v", err)
	}

	if vm.Status != "ready" {
		t.Errorf("Expected status 'ready' after final transition, got %s", vm.Status)
	}
}

// TestMockVPSieServer_QuotaLimits tests quota enforcement
func TestMockVPSieServer_QuotaLimits(t *testing.T) {
	server := NewMockVPSieServer()
	defer server.Close()

	// Set quota limit
	server.SetQuotaLimit(2)
	token := server.AuthToken

	// Create first VM (should succeed)
	vm1 := createTestVM(t, server, token, "vm-1", false)
	if vm1 == nil {
		t.Fatal("Failed to create first VM")
	}

	// Create second VM (should succeed)
	vm2 := createTestVM(t, server, token, "vm-2", false)
	if vm2 == nil {
		t.Fatal("Failed to create second VM")
	}

	// Create third VM (should fail due to quota)
	vm3 := createTestVM(t, server, token, "vm-3", true)
	if vm3 != nil {
		t.Error("Should not create VM when quota is exceeded")
	}

	// Delete one VM
	deleteVM(t, server, token, vm1.ID)

	// Now creation should succeed again
	vm4 := createTestVM(t, server, token, "vm-4", false)
	if vm4 == nil {
		t.Fatal("Failed to create VM after freeing quota")
	}
}

// TestMockVPSieServer_RateLimiting tests rate limiting
func TestMockVPSieServer_RateLimiting(t *testing.T) {
	server := NewMockVPSieServer()
	defer server.Close()

	// Set very low rate limit
	server.SetRateLimit(3)
	token := server.AuthToken

	hitRateLimit := false

	// Make requests until we hit rate limit
	for i := 0; i < 10; i++ {
		req, _ := http.NewRequest("GET", server.URL()+"/v2/vms", nil)
		req.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}
		resp.Body.Close()

		if resp.StatusCode == http.StatusTooManyRequests {
			hitRateLimit = true
			break
		}
	}

	if !hitRateLimit {
		t.Error("Expected to hit rate limit but didn't")
	}
}

// TestMockVPSieServer_ErrorInjection tests error injection scenarios
func TestMockVPSieServer_ErrorInjection(t *testing.T) {
	server := NewMockVPSieServer()
	defer server.Close()

	token := server.AuthToken

	// Test specific error scenario
	server.SetErrorScenario(ErrorScenario{
		Endpoint:   "/v2/vms",
		Method:     "POST",
		StatusCode: 503,
		Message:    "Service temporarily unavailable",
		ErrorCode:  "ServiceUnavailable",
		Permanent:  false,
	})

	// First request should fail
	vm := createTestVM(t, server, token, "test-vm", true)
	if vm != nil {
		t.Error("Expected VM creation to fail with injected error")
	}

	// Second request should succeed (non-permanent error)
	vm = createTestVM(t, server, token, "test-vm", false)
	if vm == nil {
		t.Fatal("Expected VM creation to succeed after non-permanent error")
	}

	// Test random error injection
	server.InjectErrors = true
	server.ErrorRate = 1.0 // 100% error rate

	vm = createTestVM(t, server, token, "test-vm-2", true)
	if vm != nil {
		t.Error("Expected VM creation to fail with random error injection")
	}
}

// TestMockVPSieServer_RequestTracking tests request counting and logging
func TestMockVPSieServer_RequestTracking(t *testing.T) {
	server := NewMockVPSieServer()
	defer server.Close()

	server.LogRequests = true
	token := server.AuthToken

	// Make various requests
	createTestVM(t, server, token, "vm-1", false)
	getVM(server, token, 1000)
	listVMs(server, token)

	// Check request counts
	if count := server.GetRequestCount("/v2/vms"); count != 2 { // POST and GET
		t.Errorf("Expected 2 requests to /v2/vms, got %d", count)
	}

	if count := server.GetRequestCount("/v2/vms/1000"); count != 1 {
		t.Errorf("Expected 1 request to /v2/vms/1000, got %d", count)
	}

	// Check total request count
	totalRequests := server.GetTotalRequests()
	if totalRequests < 3 {
		t.Errorf("Expected at least 3 total requests, got %d", totalRequests)
	}

	// Check request log
	log := server.GetRequestLog()
	if len(log) < 3 {
		t.Errorf("Expected at least 3 entries in request log, got %d", len(log))
	}

	// Verify log entries have required fields
	for _, entry := range log {
		if entry.Method == "" {
			t.Error("Request log entry missing method")
		}
		if entry.Path == "" {
			t.Error("Request log entry missing path")
		}
		if entry.Timestamp.IsZero() {
			t.Error("Request log entry missing timestamp")
		}
	}
}

// TestMockVPSieServer_Latency tests network latency simulation
func TestMockVPSieServer_Latency(t *testing.T) {
	server := NewMockVPSieServer()
	defer server.Close()

	// Set latency
	server.Latency = 100 * time.Millisecond
	server.LatencyVariance = 50 * time.Millisecond

	token := server.AuthToken

	start := time.Now()
	listVMs(server, token)
	duration := time.Since(start)

	// Request should take at least the minimum latency (100ms - 50ms = 50ms)
	if duration < 50*time.Millisecond {
		t.Errorf("Request completed too quickly: %v", duration)
	}

	// Request should not take more than max latency (100ms + 50ms = 150ms) + buffer
	if duration > 200*time.Millisecond {
		t.Errorf("Request took too long: %v", duration)
	}
}

// TestMockVPSieServer_CustomHandlers tests custom request handlers
func TestMockVPSieServer_CustomHandlers(t *testing.T) {
	server := NewMockVPSieServer()
	defer server.Close()

	// Set custom handler for a specific endpoint
	customCalled := false
	server.CustomHandlers["GET /v2/custom"] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		customCalled = true
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "custom response",
		})
	})

	// Call custom endpoint
	resp, err := http.Get(server.URL() + "/v2/custom")
	if err != nil {
		t.Fatalf("Failed to call custom endpoint: %v", err)
	}
	defer resp.Body.Close()

	if !customCalled {
		t.Error("Custom handler was not called")
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}

// Helper functions

func createTestVM(t *testing.T, server *MockVPSieServer, token, name string, expectError bool) *vpsieclient.VPS {
	vmReq := vpsieclient.CreateVPSRequest{
		Name:         name,
		Hostname:     name + ".example.com",
		OfferingID:   "small-2cpu-4gb",
		DatacenterID: "dc-us-east-1",
	}

	body, _ := json.Marshal(vmReq)
	req, _ := http.NewRequest("POST", server.URL()+"/v2/vms", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		if !expectError {
			t.Fatalf("Failed to create VM: %v", err)
		}
		return nil
	}
	defer resp.Body.Close()

	if expectError && resp.StatusCode == http.StatusCreated {
		t.Error("Expected VM creation to fail but it succeeded")
		return nil
	}

	if !expectError && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status 201, got %d: %s", resp.StatusCode, string(body))
		return nil
	}

	if expectError {
		return nil
	}

	var createResp struct {
		Data vpsieclient.VPS `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&createResp)
	return &createResp.Data
}

func getVM(server *MockVPSieServer, token string, vmID int) (*vpsieclient.VPS, error) {
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/v2/vms/%d", server.URL(), vmID), nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var vmResp struct {
		Data vpsieclient.VPS `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&vmResp)
	return &vmResp.Data, nil
}

func deleteVM(t *testing.T, server *MockVPSieServer, token string, vmID int) {
	req, _ := http.NewRequest("DELETE", fmt.Sprintf("%s/v2/vms/%d", server.URL(), vmID), nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Failed to delete VM: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for delete, got %d", resp.StatusCode)
	}
}

func listVMs(server *MockVPSieServer, token string) ([]vpsieclient.VPS, error) {
	req, _ := http.NewRequest("GET", server.URL()+"/v2/vms", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var listResp struct {
		Data []vpsieclient.VPS `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&listResp)
	return listResp.Data, nil
}
