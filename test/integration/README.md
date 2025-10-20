# Integration Tests

Integration tests for VPSie Kubernetes Node Autoscaler using a real Kubernetes cluster.

## Overview

These integration tests verify the autoscaler's functionality against a live Kubernetes cluster, including:
- Custom Resource Definition (CRD) operations
- NodeGroup and VPSieNode lifecycle management
- Configuration validation
- Status updates and condition management

## Prerequisites

### 1. Kubernetes Cluster

You need access to a Kubernetes cluster with:
- Kubernetes 1.24+
- kubectl configured with cluster access
- Sufficient permissions to create/delete namespaces and custom resources

### 2. Installed CRDs

The autoscaler CRDs must be installed on the cluster:

```bash
kubectl apply -f deploy/crds/ --kubeconfig=/path/to/kubeconfig
```

Verify CRDs are installed:

```bash
kubectl get crds | grep autoscaler.vpsie.com
```

Expected output:
```
nodegroups.autoscaler.vpsie.com
vpsienodes.autoscaler.vpsie.com
```

### 3. Test Cluster Configuration

The integration tests are configured to use a specific kubeconfig file. Update the `testKubeconfig` constant in `controller_integration_test.go` if needed:

```go
const (
    testKubeconfig = "/Users/zozo/.kube/config-new-test"
    testNamespace = "vpsie-autoscaler-test"
)
```

## Running Integration Tests

### Using Make

```bash
# Run integration tests
make test-integration

# Run all tests (unit + integration)
make test-all
```

### Using go test

```bash
# Run all integration tests
go test -v -tags=integration ./test/integration/

# Run specific test
go test -v -tags=integration ./test/integration/ -run TestNodeGroup_CRUD

# Run with timeout
go test -v -tags=integration -timeout 5m ./test/integration/
```

## Test Structure

### TestMain

Sets up the test environment:
- Loads kubeconfig from test cluster
- Creates controller-runtime client
- Creates test namespace
- Verifies CRDs are installed
- Cleans up test namespace after tests complete

### Test Cases

#### ✅ Implemented Tests

1. **TestNodeGroup_CRUD** - Tests NodeGroup custom resource operations
   - Create NodeGroup
   - Read NodeGroup
   - Update NodeGroup (modify min/max nodes)
   - Delete NodeGroup
   - Verify deletion

2. **TestVPSieNode_CRUD** - Tests VPSieNode custom resource operations
   - Create VPSieNode
   - Read VPSieNode
   - Update status (phase, conditions)
   - Delete VPSieNode
   - Verify deletion

3. **TestConfigurationValidation_Integration** - Tests controller options validation
   - Invalid metrics address
   - Same metrics and health address
   - Valid configuration

#### 🚧 Skipped Tests (Future Implementation)

1. **TestControllerManager_Integration** - Requires VPSie API credentials
2. **TestHealthEndpoints_Integration** - Requires running controller
3. **TestMetricsEndpoint_Integration** - Requires running controller
4. **TestLeaderElection_Integration** - Requires multiple controller instances
5. **TestControllerReconciliation_Integration** - Requires VPSie API mock
6. **TestGracefulShutdown_Integration** - Requires running controller

## Test Results

Latest test run:
```
=== Results ===
✅ TestNodeGroup_CRUD (0.83s)
✅ TestVPSieNode_CRUD (0.69s)
✅ TestConfigurationValidation_Integration (0.00s)
⏭️  6 tests skipped

Total: 2.746s
Status: PASS
```

## Test Namespace

Tests run in an isolated namespace: `vpsie-autoscaler-test`

- Created automatically during TestMain setup
- Cleaned up automatically after tests complete
- All test resources are created in this namespace

## Troubleshooting

### CRDs Not Found

```
Error: CRDs not installed
Solution: kubectl apply -f deploy/crds/ --kubeconfig=/path/to/kubeconfig
```

### Cannot Connect to Cluster

```
Error: Failed to connect to cluster
Solution: Verify kubeconfig path in controller_integration_test.go
Check: kubectl cluster-info --kubeconfig=/path/to/kubeconfig
```

### Permission Denied

```
Error: Failed to create test namespace
Solution: Ensure your kubeconfig has sufficient permissions:
  - Create/delete namespaces
  - Create/update/delete NodeGroups and VPSieNodes
  - Update resource status
```

### Tests Timeout

```
Error: Test timeout
Solution: Increase timeout with -timeout flag:
go test -v -tags=integration -timeout 10m ./test/integration/
```

## Adding New Integration Tests

### 1. Create Test Function

```go
func TestMyFeature_Integration(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
    defer cancel()

    // Your test code here
}
```

### 2. Use Existing Clients

- `k8sClient` - controller-runtime client for CRD operations
- `clientset` - Kubernetes clientset for core resources
- `cfg` - rest.Config for creating custom clients

### 3. Clean Up Resources

Always clean up resources created during tests:

```go
defer func() {
    _ = k8sClient.Delete(ctx, resource)
}()
```

### 4. Use Test Namespace

Create all test resources in `testNamespace`:

```go
resource.Namespace = testNamespace
```

## CI/CD Integration

Integration tests can be run in CI/CD pipelines with:

```yaml
# Example GitHub Actions workflow
- name: Run Integration Tests
  run: |
    kubectl apply -f deploy/crds/
    make test-integration
  env:
    KUBECONFIG: /path/to/kubeconfig
```

## Next Steps

Phase 3 implementation priorities:

1. **VPSie API Mocking**
   - Create mock VPSie API server
   - Implement common scenarios (create VM, delete VM, failures)
   - Enable TestControllerReconciliation_Integration

2. **Controller Testing**
   - Start controller in background during tests
   - Test health and metrics endpoints
   - Test graceful shutdown

3. **Leader Election Testing**
   - Run multiple controller instances
   - Verify leader election behavior
   - Test leader handoff

## References

- [controller-runtime Testing](https://book.kubebuilder.io/reference/testing)
- [Kubernetes Integration Tests](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-testing/integration-tests.md)
- [Project Documentation](../../README.md)
