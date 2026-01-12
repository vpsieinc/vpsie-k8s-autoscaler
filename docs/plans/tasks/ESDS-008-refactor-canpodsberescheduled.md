# Task: ESDS-008 - Refactor canPodsBeRescheduled and Pass AC2/AC3/AC4 Tests

Metadata:
- Dependencies: ESDS-007 (findSchedulableNode, buildNodePodsCache)
- Provides: Enhanced canPodsBeRescheduled with per-pod scheduling simulation; AC2/AC3/AC4 tests pass
- Size: Medium (2 files)

## Implementation Content
Refactor the existing `canPodsBeRescheduled` function to use per-pod scheduling simulation:
1. Enhance `canPodsBeRescheduled` to iterate pods and call `findSchedulableNode`
2. Generate clear blocking messages (AC4)
3. Update node pods cache optimistically after each successful match
4. Remove `t.Skip()` from AC2/AC3/AC4 tests

## Target Files
- [ ] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety.go`
- [ ] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety_test.go`

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase
- [ ] Remove `t.Skip()` from AC2 tests in `TestNodeSelectorInCanPodsBeRescheduled`
- [ ] Remove `t.Skip()` from AC3 tests in `TestAntiAffinityVerification`
- [ ] Remove `t.Skip()` from AC4 test in `TestClearBlockingMessages`
- [ ] Run tests to confirm failure: `go test ./pkg/scaler -run "TestNodeSelector|TestAntiAffinity|TestClearBlockingMessages" -v`

### 2. Green Phase
- [ ] Refactor `canPodsBeRescheduled` (lines 212-275):
  ```go
  func (s *ScaleDownManager) canPodsBeRescheduled(ctx context.Context, pods []*corev1.Pod) (bool, string, error) {
      // Get all nodes except the one being removed
      nodeList, err := s.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
      if err != nil {
          return false, "", fmt.Errorf("failed to list nodes: %w", err)
      }

      // Filter to ready, schedulable nodes
      var remainingNodes []*corev1.Node
      for i := range nodeList.Items {
          node := &nodeList.Items[i]
          if node.Spec.Unschedulable {
              continue
          }
          if !isNodeReady(node) {
              continue
          }
          remainingNodes = append(remainingNodes, node)
      }

      if len(remainingNodes) == 0 {
          return false, "no available nodes for rescheduling", nil
      }

      // Build node pods cache for anti-affinity checks
      nodePodsCache, err := s.buildNodePodsCache(ctx, remainingNodes)
      if err != nil {
          return false, "", fmt.Errorf("failed to build node pods cache: %w", err)
      }

      // Check each pod can be scheduled somewhere
      for _, pod := range pods {
          // Skip DaemonSet pods - recreated automatically
          if s.isSkippableDaemonSetPod(pod) {
              continue
          }

          schedulable, targetNode := findSchedulableNode(pod, remainingNodes, nodePodsCache)
          if !schedulable {
              return false, fmt.Sprintf("pod %s/%s cannot be rescheduled: no suitable node found",
                  pod.Namespace, pod.Name), nil
          }

          // Optimistically add pod to target node's cache for subsequent anti-affinity checks
          nodePodsCache[targetNode.Name] = append(nodePodsCache[targetNode.Name], pod)
      }

      return true, "", nil
  }
  ```
- [ ] Ensure blocking message format matches AC4:
  - Include pod namespace/name: "pod myapp/web-abc123"
  - Include constraint description: "no suitable node found"
- [ ] Run AC2/AC3/AC4 tests: `go test ./pkg/scaler -run "TestNodeSelector|TestAntiAffinity|TestClearBlockingMessages" -v`

### 3. Refactor Phase
- [ ] Keep existing aggregate capacity check as additional safeguard (optional)
- [ ] Add debug logging for per-pod scheduling decisions
- [ ] Ensure backward compatibility with TestBackwardCompatibility
- [ ] Run all safety tests: `go test ./pkg/scaler/... -v`

## Completion Criteria
- [ ] All 5 new unit tests pass (AC2: 2, AC3: 2, AC4: 1)
- [ ] `findSchedulableNode` checks all constraints in performance-optimized order
- [ ] Blocking reason messages include pod name, namespace, and constraint type
- [ ] Existing `MatchesNodeSelector` helper is now actively used
- [ ] `TestBackwardCompatibility` still passes (AC6)
- [ ] Existing `TestIsSafeToRemove` tests pass unchanged
- [ ] Operation verified (L2: Test Operation Verification)

## Implementation Reference

From Design Doc lines 243-277:
```go
func (s *ScaleDownManager) canPodsBeRescheduled(ctx context.Context, pods []*corev1.Pod) (bool, string, error) {
    allNodes, err := s.client.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
    if err != nil {
        return false, "", fmt.Errorf("failed to list nodes: %w", err)
    }

    remainingNodes := filterReadySchedulableNodes(allNodes)
    if len(remainingNodes) == 0 {
        return false, "no available nodes for rescheduling", nil
    }

    nodePodsCache := buildNodePodsCache(ctx, remainingNodes)

    for _, pod := range pods {
        if isSkippableDaemonSetPod(pod) {
            continue
        }

        schedulable, node := findSchedulableNode(pod, remainingNodes, nodePodsCache)
        if !schedulable {
            return false, fmt.Sprintf("pod %s/%s cannot be rescheduled: no suitable node found",
                pod.Namespace, pod.Name), nil
        }

        nodePodsCache[node.Name] = append(nodePodsCache[node.Name], pod)
    }

    return true, "", nil
}
```

## Test Verification

After implementation, these tests should pass:
```bash
go test ./pkg/scaler -run "TestNodeSelector|TestAntiAffinity|TestClearBlockingMessages" -v
```

Expected output:
```
=== RUN   TestNodeSelectorInCanPodsBeRescheduled
=== RUN   TestNodeSelectorInCanPodsBeRescheduled/AC2:_Scale-down_blocked_-_no_remaining_node_has_required_label
--- PASS: TestNodeSelectorInCanPodsBeRescheduled/AC2:_Scale-down_blocked...
=== RUN   TestNodeSelectorInCanPodsBeRescheduled/AC2:_Scale-down_allowed_-_remaining_node_has_required_label
--- PASS: TestNodeSelectorInCanPodsBeRescheduled/AC2:_Scale-down_allowed...
--- PASS: TestNodeSelectorInCanPodsBeRescheduled
=== RUN   TestAntiAffinityVerification
=== RUN   TestAntiAffinityVerification/AC3:_Scale-down_blocked_-_would_violate_pod_anti-affinity
--- PASS: TestAntiAffinityVerification/AC3:_Scale-down_blocked...
=== RUN   TestAntiAffinityVerification/AC3:_Scale-down_allowed_-_anti-affinity_not_violated
--- PASS: TestAntiAffinityVerification/AC3:_Scale-down_allowed...
--- PASS: TestAntiAffinityVerification
=== RUN   TestClearBlockingMessages
=== RUN   TestClearBlockingMessages/AC4:_Blocking_message_includes_pod_name_and_constraint_type
--- PASS: TestClearBlockingMessages/AC4:_Blocking_message...
--- PASS: TestClearBlockingMessages
```

## Notes
- Impact scope: `pkg/scaler/safety.go` (refactor), `pkg/scaler/safety_test.go` (enable tests)
- Constraints: Must maintain backward compatibility with existing behavior
- This task completes Phase 2 (AC2, AC3, AC4) of the work plan
- The optimistic cache update prevents anti-affinity violations when multiple pods from same node need to be rescheduled
