# Graceful Shutdown and Signal Handling Integration Tests

## Overview

This document describes the comprehensive integration tests for graceful shutdown and signal handling in the VPSie Kubernetes Node Autoscaler controller.

## Test Coverage

### 1. TestGracefulShutdown_Integration

Tests complete graceful shutdown flow with SIGTERM signal.

**Features Tested:**
- Controller shutdown with active resources
- Health endpoint status during shutdown
- New work rejection during shutdown
- Existing reconciliation completion
- Shutdown timeout compliance (30 seconds)
- Resource cleanup and no leaks

**Test Flow:**
1. Start controller with mock VPSie server
2. Create NodeGroup with 2 minimum nodes
3. Wait for VPSieNodes to be created
4. Verify controller is actively reconciling
5. Send SIGTERM signal
6. Verify shutdown behaviors:
   - Health endpoints reflect shutdown state
   - Controller stops accepting new work
   - Existing reconciliations complete
   - Controller exits within 30 seconds
   - No resource leaks (logs cleaned up, process terminated)

**Key Assertions:**
```go
// Health during shutdown
- /readyz returns 503 Service Unavailable
- /healthz returns 200 OK or 503

// New work rejection
- New NodeGroups are not processed
- No VPSieNodes created for new resources

// Graceful completion
- Reconciliation counters continue incrementing
- No significant error increase

// Clean shutdown
- Process exits within 30 seconds
- Log files are cleaned up
- Process is not running after exit
```

### 2. TestSignalHandling_MultipleSignals

Tests handling of multiple signals and different signal types.

**Test Scenarios:**

#### Multiple SIGTERM
- First SIGTERM triggers graceful shutdown
- Second SIGTERM forces immediate exit
- Controller exits quickly after second signal

#### SIGINT (Ctrl+C)
- Simulates user pressing Ctrl+C
- Triggers graceful shutdown
- Controller exits within timeout

#### SIGQUIT
- Tests SIGQUIT signal handling
- Triggers graceful shutdown
- Controller exits cleanly

**Test Flow for Each Signal:**
1. Start fresh controller instance
2. Wait for controller to be healthy
3. Send specific signal
4. Verify appropriate shutdown behavior
5. Confirm clean exit

**Port Configuration:**
| Test Case | Metrics Port | Health Port |
|-----------|-------------|-------------|
| Multiple SIGTERM | 18088 | 18089 |
| SIGINT | 18090 | 18091 |
| SIGQUIT | 18092 | 18093 |

### 3. TestShutdownWithActiveReconciliation

Tests shutdown behavior during active reconciliation operations.

**Features Tested:**
- Shutdown during long-running reconciliation
- Reconciliation completion before exit
- Status updates saved correctly
- Final state consistency

**Test Setup:**
- Mock server with 3-second latency
- NodeGroup with 5 minimum nodes (triggers long reconciliation)
- Large instance type for complex provisioning

**Test Flow:**
1. Start controller with slow mock VPSie server
2. Create NodeGroup triggering long reconciliation
3. Wait for reconciliation to start
4. Send SIGTERM during active reconciliation
5. Verify behaviors:
   - Reconciliations complete
   - Status is saved
   - Graceful shutdown occurs
   - Final state is consistent

**Key Validations:**

#### Reconciliation Completion
```go
// Metrics show reconciliation completed
finalReconcileCount >= initialReconcileCount

// Error count doesn't spike
errorIncrease < 5
```

#### Status Persistence
```go
// VPSieNode statuses are saved
- Phase is set and valid
- VPS instance IDs are preserved
- At least some nodes have status
```

#### Final State Consistency
```go
// NodeGroup status matches reality
- CurrentNodes ≤ MinNodes
- Actual VPSieNode count ≤ requested

// No orphaned resources
- All VPSieNodes belong to valid NodeGroups
```

## Signal Types and Expected Behaviors

| Signal | Description | Expected Behavior |
|--------|-------------|-------------------|
| SIGTERM | Termination request | Graceful shutdown with 30s timeout |
| SIGINT | Interrupt (Ctrl+C) | Graceful shutdown with 30s timeout |
| SIGQUIT | Quit signal | Graceful shutdown with 30s timeout |
| SIGKILL | Force kill | Immediate termination (not tested) |

## Shutdown Phases

### Phase 1: Shutdown Initiated
- Signal received by controller
- Shutdown flag set internally
- Health endpoints start reporting degraded status

### Phase 2: Stop Accepting Work
- New reconciliation requests rejected
- Watch events ignored
- API requests return errors

### Phase 3: Complete Active Work
- Ongoing reconciliations allowed to complete
- Status updates persisted to Kubernetes
- Metrics flushed

### Phase 4: Resource Cleanup
- Connections closed gracefully
- Temporary files removed
- Goroutines terminated

### Phase 5: Process Exit
- Exit with appropriate code
- Parent process notified

## Helper Functions

### Process Management

```go
// Send signal to controller
err := sendSignal(controller, syscall.SIGTERM)

// Wait for shutdown with timeout
err := waitForShutdown(controller, 30*time.Second)

// Check if process is running
running := isProcessRunning(controller.PID)
```

### Metrics Helpers

```go
// Get metric value during tests
value := getMetricValue(t, metricsAddr, "metric_name")

// Common metrics to check
- vpsie_autoscaler_controller_reconcile_total
- vpsie_autoscaler_controller_reconcile_errors_total
- vpsie_autoscaler_nodegroup_current_nodes
```

## Running the Tests

### Run All Shutdown Tests
```bash
go test -v -tags=integration ./test/integration \
  -run "Test.*Shutdown|Test.*Signal" -timeout 180s
```

### Run Individual Tests
```bash
# Graceful shutdown test
go test -v -tags=integration ./test/integration \
  -run TestGracefulShutdown_Integration -timeout 90s

# Signal handling test
go test -v -tags=integration ./test/integration \
  -run TestSignalHandling_MultipleSignals -timeout 60s

# Shutdown with active reconciliation
go test -v -tags=integration ./test/integration \
  -run TestShutdownWithActiveReconciliation -timeout 90s
```

### Debug Mode
```bash
# Enable verbose logging
CONTROLLER_LOG_LEVEL=debug go test -v -tags=integration \
  ./test/integration -run TestGracefulShutdown_Integration
```

## Troubleshooting

### Common Issues

#### 1. Controller doesn't shutdown gracefully

**Symptoms:**
- Test timeout waiting for shutdown
- Process still running after SIGTERM

**Solutions:**
- Check controller logs for blocked operations
- Verify signal is being received
- Check for deadlocks in reconciliation

#### 2. Health endpoints not updating

**Symptoms:**
- /readyz still returns 200 during shutdown
- Health checks don't reflect shutdown state

**Solutions:**
- Verify controller implements shutdown hooks
- Check health check implementation
- Ensure proper context cancellation

#### 3. Reconciliations not completing

**Symptoms:**
- Metrics show incomplete reconciliations
- VPSieNode status not saved

**Solutions:**
- Check reconciliation timeout settings
- Verify mock server is responding
- Check for errors in controller logs

#### 4. Resource leaks

**Symptoms:**
- Log files not cleaned up
- Processes remain after test

**Solutions:**
- Ensure defer statements for cleanup
- Check process group handling
- Verify Stop() method is called

### Debug Commands

```bash
# Check for zombie processes
ps aux | grep vpsie-autoscaler

# Check open files
lsof -p <PID>

# Monitor signals
strace -p <PID> -e signal

# Check port usage
netstat -tulpn | grep 180
```

## Best Practices

### 1. Test Isolation
- Each test uses unique ports
- Clean up resources in defer blocks
- Don't rely on test execution order

### 2. Timing Considerations
```go
// Allow time for shutdown to start
time.Sleep(2 * time.Second)

// Use Eventually for async operations
require.Eventually(t, func() bool {
    return controller.IsHealthy()
}, 30*time.Second, 1*time.Second)
```

### 3. Signal Testing
- Always start with fresh controller instance
- Test one signal type per subtest
- Verify process actually receives signal

### 4. Assertions
```go
// Use appropriate assertions
assert.NoError(t, err) // For operations that should succeed
require.NoError(t, err) // For critical setup steps
assert.Eventually() // For async state changes
```

## Integration with CI/CD

### GitHub Actions Example

```yaml
name: Shutdown Tests

on:
  push:
    branches: [main]
  pull_request:

jobs:
  shutdown-tests:
    runs-on: ubuntu-latest
    timeout-minutes: 10

    steps:
    - uses: actions/checkout@v3

    - name: Setup Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.25'

    - name: Create kind cluster
      run: |
        kind create cluster
        kubectl apply -f deploy/crds/

    - name: Build controller
      run: make build

    - name: Run shutdown tests
      run: |
        go test -v -tags=integration ./test/integration \
          -run "Test.*Shutdown|Test.*Signal" \
          -timeout 180s \
          -race

    - name: Upload logs on failure
      if: failure()
      uses: actions/upload-artifact@v3
      with:
        name: test-logs
        path: /tmp/controller-*.log
```

### Local Testing with kind

```bash
# Setup
kind create cluster --name shutdown-test
kubectl apply -f deploy/crds/

# Run tests with coverage
go test -v -tags=integration -cover \
  ./test/integration -run "Test.*Shutdown" \
  -coverprofile=shutdown.coverage

# Cleanup
kind delete cluster --name shutdown-test
```

## Performance Considerations

### Shutdown Timing

| Operation | Expected Duration |
|-----------|------------------|
| Signal delivery | < 100ms |
| Health status update | < 1s |
| Stop accepting work | < 1s |
| Complete reconciliation | < 10s (typical) |
| Resource cleanup | < 2s |
| Total shutdown | < 30s |

### Resource Usage

During shutdown:
- CPU usage may spike briefly (finalizing operations)
- Memory should not increase significantly
- Network connections should close gracefully
- File descriptors should be released

## Related Documentation

- [Controller Integration Tests](CONTROLLER_TESTS_README.md)
- [Test Helpers](test_helpers.go)
- [Main Integration Test README](README.md)
- [Mock VPSie Server](MOCK_SERVER_README.md)