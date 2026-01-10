# Task: Phase 3b - Joiner Enhancement and Integration Tests

Metadata:
- Phase: 3 (Controller Integration)
- Dependencies: Task 04 (Provisioner Integration)
- Provides: IP-first matching strategy, integration tests
- Size: Small (1 file + test additions)
- Verification Level: L1 (Functional Operation)

## Implementation Content

Enhance the Joiner component to prioritize IP-based matching over hostname matching. This improves reliability of K8s node discovery since IP addresses are more stable identifiers than hostnames in cloud environments.

Also create integration tests that verify the full discovery flow from async provisioning through K8s node joining.

## Target Files

- [ ] `pkg/controller/vpsienode/joiner.go` (MODIFY: reorder matching strategy)
- [ ] `pkg/controller/vpsienode/joiner_test.go` (MODIFY: add tests for IP-first matching)
- [ ] `test/integration/node_discovery_test.go` (NEW: integration tests - optional)

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase

- [ ] Write test for IP-first matching:
  ```go
  func TestJoiner_FindKubernetesNode_IPFirst(t *testing.T) {
      // VPSieNode has both NodeName and IPAddress
      // K8s node exists with matching IP but different name
      // Should find node by IP first
  }
  ```
- [ ] Run test - may pass or fail depending on current implementation order

### 2. Green Phase

#### 2.1 Update findKubernetesNode method

Reorder the matching strategies in `findKubernetesNode()` to try IP first:

```go
// findKubernetesNode finds the Kubernetes Node corresponding to the VPSieNode
// Uses IP-first matching strategy for reliability
func (j *Joiner) findKubernetesNode(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (*corev1.Node, error) {
    // Strategy 1: Try finding by IP address first (most reliable)
    if vn.Spec.IPAddress != "" {
        node, err := j.findNodeByIP(ctx, vn.Spec.IPAddress)
        if err == nil && node != nil {
            logger.Debug("Found node by IP address",
                zap.String("ip", vn.Spec.IPAddress),
                zap.String("nodeName", node.Name),
            )
            return node, nil
        }
        if err != nil && !errors.IsNotFound(err) {
            logger.Debug("Error finding node by IP", zap.Error(err))
        }
    }

    // Strategy 2: Try finding by exact node name
    if vn.Spec.NodeName != "" {
        node := &corev1.Node{}
        err := j.client.Get(ctx, types.NamespacedName{Name: vn.Spec.NodeName}, node)
        if err == nil {
            return node, nil
        }
        if !errors.IsNotFound(err) {
            return nil, err
        }
    }

    // Strategy 3: Try finding by hostname
    if vn.Status.Hostname != "" {
        node, err := j.findNodeByHostname(ctx, vn.Status.Hostname)
        if err == nil && node != nil {
            return node, nil
        }
        if err != nil && !errors.IsNotFound(err) {
            logger.Debug("Error finding node by hostname", zap.Error(err))
        }
    }

    return nil, errors.NewNotFound(corev1.Resource("node"), vn.Spec.NodeName)
}
```

- [ ] Run tests - confirm pass

### 3. Refactor Phase

- [ ] Add debug logging for each matching strategy attempt
- [ ] Ensure consistent error handling
- [ ] Run `make lint` and `make fmt`

## Test Cases

### Joiner Tests (add to joiner_test.go)

```go
TestJoiner_FindKubernetesNode_IPFirst
// VPSieNode: IPAddress="10.0.0.5", NodeName="wrong-name"
// K8s Node: name="correct-name", InternalIP="10.0.0.5"
// Should find node by IP and return "correct-name"

TestJoiner_FindKubernetesNode_FallbackToName
// VPSieNode: IPAddress="" (empty), NodeName="my-node"
// K8s Node: name="my-node"
// Should find node by name

TestJoiner_FindKubernetesNode_FallbackToHostname
// VPSieNode: IPAddress="" (empty), NodeName="" (empty), Hostname="my-hostname"
// K8s Node: name="my-hostname"
// Should find node by hostname

TestJoiner_FindKubernetesNode_IPNotFound_FallsBack
// VPSieNode: IPAddress="10.0.0.99" (no match), NodeName="my-node"
// K8s Node: name="my-node", InternalIP="10.0.0.1"
// Should not find by IP, then find by name
```

### Integration Tests (optional: test/integration/node_discovery_test.go)

```go
//go:build integration

TestAsyncProvisioning_DiscoverySuccess
// 1. Create NodeGroup
// 2. VPSieNode created with creation-requested=true, VPSieInstanceID=0
// 3. Mock VPSie API returns VPS with matching hostname
// 4. Reconcile runs discovery
// 5. Assert VPSieNode has discovered VPS ID
// 6. K8s node joins cluster
// 7. Assert VPSieNode reaches Ready state

TestAsyncProvisioning_DiscoveryTimeout
// 1. Create VPSieNode with old CreatedAt (> 15 min)
// 2. Reconcile runs discovery
// 3. Assert VPSieNode transitions to Failed phase

TestAsyncProvisioning_DiscoveryRetry
// 1. Create VPSieNode
// 2. First reconcile: VPS not found
// 3. Second reconcile: VPS appears in API
// 4. Assert VPSieNode has discovered VPS ID

TestJoiner_IPFirstMatching
// 1. VPSieNode with IPAddress set
// 2. K8s node exists with different name but matching IP
// 3. Assert joiner finds correct node

TestFullProvisioningFlow_WithDiscovery
// End-to-end test of:
// 1. NodeGroup scales up
// 2. VPSieNode created
// 3. AddK8sSlaveToGroup called (returns ID=0)
// 4. Discovery finds VPS
// 5. K8s node joins
// 6. Labels applied
// 7. VPSieNode Ready
// 8. NodeGroup status updated
```

## Completion Criteria

- [ ] `findKubernetesNode()` tries IP matching first, before node name
- [ ] Debug logging added for each matching strategy
- [ ] Unit tests pass for IP-first matching
- [ ] Existing joiner tests still pass
- [ ] `make build` succeeds
- [ ] `make test` passes
- [ ] `make lint` passes

## Operational Verification

```bash
# Verify build
make build

# Run joiner tests
go test ./pkg/controller/vpsienode -run TestJoiner -v

# Run full controller tests
go test ./pkg/controller/vpsienode/... -v

# Full test suite
make test

# Integration tests (if implemented)
go test -tags=integration ./test/integration -run TestNodeDiscovery -v
```

## Acceptance Criteria Mapping

| Design Doc AC | Implementation |
|--------------|----------------|
| M1: K8s node matched by IP (primary) | IP matching is first strategy |
| M2: K8s node matched by hostname (fallback) | Hostname is third strategy |
| M3: VPSieNode.Status.NodeName updated | Already handled by CheckJoinStatus() |
| M4: Matching works for nodes joined before discovery | IP-first ensures match regardless of timing |

## Notes

- Impact scope: Changes matching order only - existing behavior preserved
- The current implementation already has IP matching, just not as first strategy
- This change improves reliability for async provisioning scenarios where IP is known before hostname
- Integration tests are optional but recommended for verifying full flow
- Labels are already applied by existing `applyNodeConfiguration()` method (L1-L5 criteria)
