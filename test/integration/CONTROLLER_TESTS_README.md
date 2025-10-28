# Controller Runtime Integration Tests

## Overview

This document describes the integration tests for the VPSie Kubernetes Node Autoscaler controller runtime, including health checks, metrics, and end-to-end reconciliation tests.

## Test Coverage

### 1. TestHealthEndpoints_Integration

Tests the controller's health check endpoints to ensure proper liveness and readiness reporting.

**Features Tested:**
- `/healthz` - Liveness probe endpoint
- `/readyz` - Readiness probe endpoint
- `/ping` - Simple ping endpoint
- Health status during graceful shutdown

**Test Flow:**
1. Starts mock VPSie server
2. Creates VPSie secret with mock server credentials
3. Launches controller process in background
4. Waits for controller to become healthy
5. Tests each health endpoint
6. Initiates graceful shutdown
7. Verifies health status changes during shutdown

**Expected Results:**
- `/healthz` returns 200 OK with body "ok"
- `/readyz` returns 200 OK when controller is ready
- `/ping` returns 200 OK with body "pong"
- During shutdown, `/readyz` may return 503 Service Unavailable

### 2. TestMetricsEndpoint_Integration

Validates Prometheus metrics exposure and updates.

**Features Tested:**
- Prometheus metrics format validation
- All 22 registered metrics presence
- Metrics updates when resources change
- Specific metric value validation

**Metrics Verified:**
```
vpsie_autoscaler_nodegroup_desired_nodes
vpsie_autoscaler_nodegroup_current_nodes
vpsie_autoscaler_nodegroup_ready_nodes
vpsie_autoscaler_nodegroup_min_nodes
vpsie_autoscaler_nodegroup_max_nodes
vpsie_autoscaler_vpsienode_phase_transitions_total
vpsie_autoscaler_vpsienode_current_phase
vpsie_autoscaler_controller_reconcile_duration_seconds
vpsie_autoscaler_controller_reconcile_errors_total
vpsie_autoscaler_controller_reconcile_total
vpsie_autoscaler_vpsie_api_requests_total
vpsie_autoscaler_vpsie_api_request_duration_seconds
vpsie_autoscaler_vpsie_api_errors_total
vpsie_autoscaler_scaling_decisions_total
vpsie_autoscaler_scale_up_operations_total
vpsie_autoscaler_scale_down_operations_total
vpsie_autoscaler_pods_unschedulable
vpsie_autoscaler_pods_pending
vpsie_autoscaler_provisioning_duration_seconds
vpsie_autoscaler_termination_duration_seconds
vpsie_autoscaler_event_emissions_total
```

**Test Flow:**
1. Starts controller with metrics on port 18082
2. Verifies `/metrics` endpoint returns Prometheus format
3. Checks all 22 metrics are exposed
4. Creates a NodeGroup resource
5. Validates metrics update with correct values
6. Verifies reconciliation counter increases

### 3. TestControllerReconciliation_Integration

End-to-end test of controller reconciliation logic with mock VPSie API.

**Features Tested:**
- NodeGroup creation triggers VPSieNode creation
- VPSie API integration (VM creation)
- VM state transitions (provisioning → running → ready)
- Scale-up operations
- Resource cleanup on NodeGroup deletion

**Test Flow:**

#### Phase 1: Create with minNodes
1. Create NodeGroup with minNodes=2
2. Verify controller creates 2 VPSieNode resources
3. Confirm mock VPSie API receives 2 VM creation requests
4. Check VPSieNodes have correct specs

#### Phase 2: VM State Transitions
1. Wait for mock server VM state transitions
2. Verify controller detects state changes
3. Check VPSieNode status updates accordingly

#### Phase 3: Scale Up
1. Update NodeGroup minNodes from 2 to 3
2. Verify controller creates additional VPSieNode
3. Confirm additional VM creation request

#### Phase 4: Cleanup
1. Delete NodeGroup
2. Verify all VPSieNodes are deleted
3. Confirm VMs are deleted in mock server

## Prerequisites

### 1. Kubernetes Cluster

The tests require a real Kubernetes cluster with:
- CRDs installed (NodeGroup, VPSieNode)
- Test namespace created
- Appropriate RBAC permissions

Default configuration:
```yaml
kubeconfig: /Users/zozo/.kube/config-new-test
namespace: vpsie-autoscaler-test
```

### 2. Controller Binary

The controller binary must be built before running tests:
```bash
make build
```

The tests will automatically build the binary if it doesn't exist at `bin/vpsie-autoscaler`.

### 3. Test Dependencies

Required Go packages:
```go
github.com/stretchr/testify
k8s.io/client-go
sigs.k8s.io/controller-runtime
```

## Running the Tests

### Run All Integration Tests
```bash
make test-integration
```

### Run Specific Test
```bash
# Health endpoints test
go test -v -tags=integration ./test/integration -run TestHealthEndpoints_Integration -timeout 60s

# Metrics test
go test -v -tags=integration ./test/integration -run TestMetricsEndpoint_Integration -timeout 60s

# Reconciliation test
go test -v -tags=integration ./test/integration -run TestControllerReconciliation_Integration -timeout 90s
```

### Run with Coverage
```bash
go test -v -tags=integration -cover ./test/integration
```

## Test Configuration

### Port Configuration

Each test uses unique ports to avoid conflicts:

| Test | Metrics Port | Health Port |
|------|-------------|-------------|
| TestHealthEndpoints | 18080 | 18081 |
| TestMetricsEndpoint | 18082 | 18083 |
| TestControllerReconciliation | 18084 | 18085 |

### Timeouts

Default timeouts for various operations:

| Operation | Timeout |
|-----------|---------|
| Controller startup | 30s |
| Health check polling | 30s |
| Graceful shutdown | 30s |
| VM state transitions | 10s |
| Resource creation/deletion | 30s |

### Mock Server Configuration

The mock VPSie server provides:
- OAuth authentication endpoint
- VM CRUD operations
- Automatic state transitions
- Request counting for assertions

Default VM state transitions:
```go
StateTransitions: []VMStateTransition{
    {FromState: "provisioning", ToState: "running", Duration: 5 * time.Second},
    {FromState: "running", ToState: "ready", Duration: 3 * time.Second},
}
```

## Helper Functions

### Controller Process Management

```go
// Start controller in background
controller, err := startControllerInBackground(
    metricsPort,
    healthPort,
    secretName,
    secretNamespace,
)

// Check controller health
if controller.IsHealthy() {
    // Controller is running and healthy
}

// Graceful shutdown
err := controller.Stop()

// Get controller logs
stdout, stderr, err := controller.GetLogs()
```

### Mock Server Usage

```go
// Create mock server
mockServer := NewMockVPSieServer()
defer mockServer.Close()

// Configure state transitions
mockServer.StateTransitions = []VMStateTransition{
    {FromState: "provisioning", ToState: "running", Duration: 5 * time.Second},
}

// Get request counts
count := mockServer.GetRequestCount("/v2/vms")

// Set VM status manually
mockServer.SetVMStatus(vmID, "running")
```

## Troubleshooting

### Common Issues

#### 1. Controller fails to start

**Symptoms:**
- Test timeout waiting for controller to be healthy
- Error: "controller did not become healthy"

**Solutions:**
- Check controller binary exists: `ls bin/vpsie-autoscaler`
- Verify kubeconfig is valid: `kubectl get nodes`
- Check controller logs: `controller.GetLogs()`

#### 2. CRDs not found

**Symptoms:**
- Error creating NodeGroup/VPSieNode resources
- "no matches for kind" error

**Solutions:**
```bash
# Install CRDs
kubectl apply -f deploy/crds/

# Verify CRDs
kubectl get crd nodegroups.autoscaler.vpsie.com
kubectl get crd vpsienodes.autoscaler.vpsie.com
```

#### 3. Port conflicts

**Symptoms:**
- "bind: address already in use" error
- Controller fails to start

**Solutions:**
- Check for processes using the ports:
```bash
lsof -i :18080-18085
```
- Kill conflicting processes or use different ports

#### 4. Mock server connection issues

**Symptoms:**
- VPSie API errors in controller logs
- "connection refused" errors

**Solutions:**
- Verify mock server is running
- Check VPSie secret contains correct URL:
```bash
kubectl get secret -n vpsie-autoscaler-test vpsie-test-secret -o yaml
```

### Debug Mode

Enable debug logging for more information:

```go
// Controller is started with debug logging by default
cmd := exec.Command(
    binaryPath,
    "--log-level", "debug",
    // other flags...
)
```

Check controller logs:
```go
stdout, stderr, err := controller.GetLogs()
fmt.Println("STDOUT:", stdout)
fmt.Println("STDERR:", stderr)
```

## CI/CD Integration

### GitHub Actions

Example workflow for running integration tests:

```yaml
name: Integration Tests

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  integration:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.25'

    - name: Create kind cluster
      run: |
        kind create cluster
        kubectl apply -f deploy/crds/

    - name: Build controller
      run: make build

    - name: Run integration tests
      run: make test-integration
      env:
        KUBECONFIG: ${{ env.KUBECONFIG }}
```

### Local Testing with kind

```bash
# Create kind cluster
kind create cluster --name autoscaler-test

# Install CRDs
kubectl apply -f deploy/crds/

# Run tests
KUBECONFIG=~/.kube/config make test-integration

# Cleanup
kind delete cluster --name autoscaler-test
```

## Best Practices

1. **Test Isolation**: Each test uses unique ports and resource names
2. **Cleanup**: Always defer cleanup of resources and processes
3. **Timeouts**: Use appropriate timeouts for operations
4. **Assertions**: Verify both positive and negative cases
5. **Logging**: Capture and check logs on failure

## Contributing

When adding new integration tests:

1. Use unique port ranges to avoid conflicts
2. Follow the existing test structure
3. Add helper functions to `test_helpers.go`
4. Document test scenarios and expected outcomes
5. Ensure tests are idempotent and can run repeatedly

## Related Documentation

- [Mock VPSie Server README](MOCK_SERVER_README.md)
- [Integration Test Summary](README.md)
- [Main Project README](../../README.md)