# VPSie Mock API Server Documentation

## Overview

The MockVPSieServer is a comprehensive HTTP mock server that simulates the VPSie API for integration testing. It provides full lifecycle management, state transitions, error injection, and advanced testing capabilities.

## Features

### Core Capabilities

1. **OAuth 2.0 Authentication**
   - `/oauth/token` - Standard OAuth token endpoint
   - `/auth/from/api` - Legacy authentication endpoint
   - Token expiration and refresh support
   - Client credentials and refresh token grant types

2. **VM Management**
   - Create, Read, Update, Delete operations
   - Automatic state transitions (provisioning → running → ready)
   - Resource tracking and quota management
   - Custom VM configurations based on offerings

3. **Resource Endpoints**
   - `/v2/offerings` - Instance types and pricing
   - `/v2/datacenters` - Available regions and zones

4. **Advanced Testing Features**
   - Rate limiting simulation (429 responses)
   - Configurable network latency with variance
   - Error injection (specific scenarios and random errors)
   - Request tracking and logging
   - Custom handler injection for special test cases

## Usage

### Basic Setup

```go
import (
    "github.com/vpsie/vpsie-k8s-autoscaler/test/integration"
)

func TestMyFeature(t *testing.T) {
    // Create and start mock server
    server := integration.NewMockVPSieServer()
    defer server.Close()

    // Get server URL for client configuration
    apiURL := server.URL()

    // Get auth token for requests
    token := server.AuthToken

    // Configure your VPSie client to use mock server
    client := vpsieclient.NewClient(apiURL, token)
}
```

### Authentication Testing

```go
// Test OAuth flow
server := integration.NewMockVPSieServer()
defer server.Close()

// Expire token to test refresh
server.ExpireToken()

// Generate new token
newToken := server.RefreshAuthToken()

// Test with expired token (will fail)
resp := makeRequest(server.URL(), expiredToken)
// Returns 401 Unauthorized

// Test with new token (will succeed)
resp = makeRequest(server.URL(), newToken)
// Returns 200 OK
```

### VM Lifecycle Testing

```go
server := integration.NewMockVPSieServer()
defer server.Close()

// Configure fast state transitions for testing
server.StateTransitions = []VMStateTransition{
    {FromState: "provisioning", ToState: "running", Duration: 2 * time.Second},
    {FromState: "running", ToState: "ready", Duration: 1 * time.Second},
}

// Create VM - starts in "provisioning" state
vm := createVM(server, "test-vm")

// Wait for automatic transition to "running"
time.Sleep(3 * time.Second)

// Check VM status
vm = getVM(server, vm.ID)
assert.Equal(t, "running", vm.Status)

// Wait for final transition to "ready"
time.Sleep(2 * time.Second)

vm = getVM(server, vm.ID)
assert.Equal(t, "ready", vm.Status)
```

### Error Injection

```go
server := integration.NewMockVPSieServer()
defer server.Close()

// Inject specific error scenario
server.SetErrorScenario(ErrorScenario{
    Endpoint:   "/v2/vms",
    Method:     "POST",
    StatusCode: 503,
    Message:    "Service temporarily unavailable",
    ErrorCode:  "ServiceUnavailable",
    Permanent:  false, // Error clears after first occurrence
})

// First request fails with 503
resp1 := createVM(server, "vm-1")
// Returns 503 Service Unavailable

// Second request succeeds (non-permanent error cleared)
resp2 := createVM(server, "vm-2")
// Returns 201 Created

// Enable random error injection
server.InjectErrors = true
server.ErrorRate = 0.5 // 50% chance of error

// Requests will randomly fail
for i := 0; i < 10; i++ {
    resp := makeRequest(server)
    // ~50% will return errors (500, 503, 504, 400)
}
```

### Rate Limiting

```go
server := integration.NewMockVPSieServer()
defer server.Close()

// Set aggressive rate limit
server.SetRateLimit(5) // 5 requests per minute

// Make rapid requests
for i := 0; i < 10; i++ {
    resp := makeRequest(server)
    if resp.StatusCode == 429 {
        // Rate limit exceeded
        fmt.Println("Rate limited!")
        break
    }
}
```

### Quota Management

```go
server := integration.NewMockVPSieServer()
defer server.Close()

// Set VM quota
server.SetQuotaLimit(3)

// Create VMs up to quota
vm1 := createVM(server, "vm-1") // Success
vm2 := createVM(server, "vm-2") // Success
vm3 := createVM(server, "vm-3") // Success
vm4 := createVM(server, "vm-4") // Fails - quota exceeded (403)

// Delete a VM to free quota
deleteVM(server, vm1.ID)

// Now creation succeeds again
vm5 := createVM(server, "vm-5") // Success
```

### Network Latency Simulation

```go
server := integration.NewMockVPSieServer()
defer server.Close()

// Set base latency with variance
server.Latency = 200 * time.Millisecond
server.LatencyVariance = 100 * time.Millisecond

// Requests will have latency between 100-300ms
start := time.Now()
makeRequest(server)
duration := time.Since(start)
// duration will be 100-300ms
```

### Request Tracking

```go
server := integration.NewMockVPSieServer()
defer server.Close()

// Enable request logging
server.LogRequests = true

// Make various requests
createVM(server, "vm-1")
listVMs(server)
deleteVM(server, 1000)

// Check request counts
fmt.Printf("POST /v2/vms: %d requests\n", server.GetRequestCount("/v2/vms"))
fmt.Printf("GET /v2/vms: %d requests\n", server.GetRequestCount("/v2/vms"))
fmt.Printf("Total requests: %d\n", server.GetTotalRequests())

// Get detailed request log
log := server.GetRequestLog()
for _, entry := range log {
    fmt.Printf("%s %s at %s\n", entry.Method, entry.Path, entry.Timestamp)
}

// Reset counters
server.ResetRequestCounts()
server.ClearRequestLog()
```

### Custom Handlers

```go
server := integration.NewMockVPSieServer()
defer server.Close()

// Add custom handler for specific test scenario
server.CustomHandlers["POST /v2/vms"] = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    // Custom logic for this test
    w.WriteHeader(http.StatusConflict)
    json.NewEncoder(w).Encode(map[string]string{
        "error": "CustomError",
        "message": "Special test condition",
    })
})

// Request will use custom handler
resp := createVM(server, "test-vm")
// Returns 409 Conflict with custom error
```

## Test Scenarios

### 1. Successful VM Creation and Lifecycle

```go
func TestVMLifecycle(t *testing.T) {
    server := integration.NewMockVPSieServer()
    defer server.Close()

    // Create VM
    vm := createVM(server, "test-vm")
    assert.NotNil(t, vm)
    assert.Equal(t, "provisioning", vm.Status)

    // Get VM
    retrieved := getVM(server, vm.ID)
    assert.Equal(t, vm.ID, retrieved.ID)

    // Delete VM
    err := deleteVM(server, vm.ID)
    assert.NoError(t, err)

    // Verify deletion
    _, err = getVM(server, vm.ID)
    assert.Error(t, err) // Should return 404
}
```

### 2. Authentication Failure

```go
func TestAuthenticationFailure(t *testing.T) {
    server := integration.NewMockVPSieServer()
    defer server.Close()

    // Use invalid token
    invalidToken := "invalid-token"

    req, _ := http.NewRequest("GET", server.URL()+"/v2/vms", nil)
    req.Header.Set("Authorization", "Bearer "+invalidToken)

    resp, _ := http.DefaultClient.Do(req)
    assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
```

### 3. Rate Limiting

```go
func TestRateLimiting(t *testing.T) {
    server := integration.NewMockVPSieServer()
    defer server.Close()

    server.SetRateLimit(3)

    hitLimit := false
    for i := 0; i < 10; i++ {
        resp := makeRequest(server)
        if resp.StatusCode == http.StatusTooManyRequests {
            hitLimit = true
            break
        }
    }

    assert.True(t, hitLimit, "Should hit rate limit")
}
```

### 4. VM Provisioning Failure

```go
func TestVMProvisioningFailure(t *testing.T) {
    server := integration.NewMockVPSieServer()
    defer server.Close()

    // Inject provisioning error
    server.SetErrorScenario(ErrorScenario{
        Endpoint:   "/v2/vms",
        Method:     "POST",
        StatusCode: 500,
        Message:    "Failed to provision VM",
        ErrorCode:  "ProvisioningError",
        Permanent:  true,
    })

    vm := createVM(server, "test-vm")
    assert.Nil(t, vm, "VM creation should fail")
}
```

### 5. Quota Exceeded

```go
func TestQuotaExceeded(t *testing.T) {
    server := integration.NewMockVPSieServer()
    defer server.Close()

    server.SetQuotaLimit(1)

    vm1 := createVM(server, "vm-1")
    assert.NotNil(t, vm1)

    vm2 := createVM(server, "vm-2")
    assert.Nil(t, vm2, "Should fail due to quota")
}
```

### 6. Network Timeout

```go
func TestNetworkTimeout(t *testing.T) {
    server := integration.NewMockVPSieServer()
    defer server.Close()

    // Set very high latency to simulate timeout
    server.Latency = 10 * time.Second

    client := &http.Client{
        Timeout: 1 * time.Second,
    }

    req, _ := http.NewRequest("GET", server.URL()+"/v2/vms", nil)
    _, err := client.Do(req)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "timeout")
}
```

## API Reference

### MockVPSieServer Methods

| Method | Description |
|--------|-------------|
| `NewMockVPSieServer()` | Creates and starts a new mock server |
| `URL()` | Returns the base URL of the mock server |
| `Close()` | Shuts down the mock server |
| `SetVMStatus(vmID, status)` | Manually set VM status |
| `GetVM(vmID)` | Get VM by ID |
| `SetQuotaLimit(limit)` | Set maximum number of VMs |
| `SetRateLimit(limit)` | Set requests per minute limit |
| `ExpireToken()` | Expire the current auth token |
| `RefreshAuthToken()` | Generate a new auth token |
| `SetErrorScenario(scenario)` | Add error injection scenario |
| `ClearErrorScenarios()` | Remove all error scenarios |
| `GetRequestCount(endpoint)` | Get request count for endpoint |
| `GetTotalRequests()` | Get total request count |
| `ResetRequestCounts()` | Reset all counters |
| `GetRequestLog()` | Get detailed request log |
| `ClearRequestLog()` | Clear request log |

### Configuration Options

| Field | Type | Description |
|-------|------|-------------|
| `AuthToken` | string | Current authentication token |
| `TokenExpiry` | time.Time | Token expiration time |
| `RefreshToken` | string | Refresh token for OAuth flow |
| `InjectErrors` | bool | Enable random error injection |
| `ErrorRate` | float64 | Random error probability (0.0-1.0) |
| `ErrorScenarios` | []ErrorScenario | Specific error scenarios |
| `Latency` | time.Duration | Base network latency |
| `LatencyVariance` | time.Duration | Random latency variance |
| `QuotaLimit` | int | Maximum VMs allowed |
| `RateLimit` | int | Requests per minute limit |
| `StateTransitions` | []VMStateTransition | VM state transitions |
| `AutoTransition` | bool | Enable automatic state transitions |
| `LogRequests` | bool | Enable request logging |
| `CustomHandlers` | map[string]http.HandlerFunc | Custom endpoint handlers |

## Best Practices

1. **Always defer Close()**: Ensure the mock server is properly shut down
   ```go
   server := NewMockVPSieServer()
   defer server.Close()
   ```

2. **Reset state between tests**: Clear errors and counters for test isolation
   ```go
   server.ClearErrorScenarios()
   server.ResetRequestCounts()
   ```

3. **Use appropriate timeouts**: When testing state transitions
   ```go
   server.StateTransitions = []VMStateTransition{
       {FromState: "provisioning", ToState: "running", Duration: 100 * time.Millisecond},
   }
   ```

4. **Validate error responses**: Check both status code and error message
   ```go
   if resp.StatusCode != http.StatusOK {
       var errResp ErrorResponse
       json.NewDecoder(resp.Body).Decode(&errResp)
       t.Errorf("Request failed: %s", errResp.Message)
   }
   ```

5. **Use table-driven tests**: For comprehensive scenario coverage
   ```go
   tests := []struct {
       name        string
       scenario    ErrorScenario
       expectError bool
   }{
       // Test cases...
   }
   ```

## Integration with VPSie Client

The mock server is designed to work seamlessly with the actual VPSie client:

```go
// Create mock server
mockServer := integration.NewMockVPSieServer()
defer mockServer.Close()

// Configure VPSie client to use mock server
client := &vpsieclient.Client{
    BaseURL: mockServer.URL(),
    Token:   mockServer.AuthToken,
}

// Use client normally - it will interact with mock server
vms, err := client.ListVMs(context.Background())
```

## Troubleshooting

### Common Issues

1. **Port conflicts**: The mock server uses a random port. If you need a specific port:
   ```go
   // Create custom server
   mux := http.NewServeMux()
   // ... configure routes
   server := httptest.NewServer(mux)
   ```

2. **State transition timing**: Adjust durations for faster tests:
   ```go
   server.StateTransitions = []VMStateTransition{
       {FromState: "provisioning", ToState: "running", Duration: 10 * time.Millisecond},
   }
   ```

3. **Concurrent access**: The mock server is thread-safe, but be aware of race conditions in tests:
   ```go
   // Use sync.WaitGroup for concurrent tests
   var wg sync.WaitGroup
   for i := 0; i < 10; i++ {
       wg.Add(1)
       go func() {
           defer wg.Done()
           createVM(server, fmt.Sprintf("vm-%d", i))
       }()
   }
   wg.Wait()
   ```

## Examples

See `mock_vpsie_server_test.go` for comprehensive examples of all features.