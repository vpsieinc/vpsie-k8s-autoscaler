# Task: ESDS-010 - Implement Same-NodeGroup Protection and Pass AC5 Tests

Metadata:
- Dependencies: ESDS-009 (test skeletons exist)
- Provides: Complete same-nodegroup protection; AC5 tests pass (Green state)
- Size: Small (2 files)

## Implementation Content
Implement the same-nodegroup protection guard clause:
1. `getNodeGroupFromNode(ctx context.Context, nodeName string) string` - Get nodegroup label from node
2. Guard clause in `executeRollingBatch` to skip same-nodegroup with same-offering operations
3. Same guard in `executeSurgeBatch` for consistency
4. Remove `t.Skip()` from AC5 tests

## Target Files
- [ ] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/rebalancer/executor.go`
- [ ] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/rebalancer/executor_test.go`

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase
- [ ] Verify ESDS-009 test skeletons exist
- [ ] Remove `t.Skip()` from AC5 test cases in `TestSameNodeGroupProtection`
- [ ] Run tests to confirm failure: `go test ./pkg/rebalancer -run TestSameNodeGroupProtection -v`

### 2. Green Phase
- [ ] Add `getNodeGroupFromNode` method to Executor:
  ```go
  // getNodeGroupFromNode retrieves the nodegroup label from a node.
  // Returns empty string on error (fail-safe - allows operation to proceed).
  func (e *Executor) getNodeGroupFromNode(ctx context.Context, nodeName string) string
  ```
- [ ] Implement `getNodeGroupFromNode`:
  - Get node from Kubernetes API
  - Return value of label `autoscaler.vpsie.com/nodegroup`
  - Return empty string on error (fail-safe)
- [ ] Add guard clause to `executeRollingBatch` (after line 157, before "Step 1: Provision"):
  ```go
  // Check if this is a same-nodegroup "rebalance" with same offering (no-op)
  currentNodeGroup := e.getNodeGroupFromNode(ctx, candidate.NodeName)
  if currentNodeGroup == plan.NodeGroupName && candidate.CurrentOffering == candidate.TargetOffering {
      logger.Info("Skipping termination: same nodegroup and offering",
          "nodeName", candidate.NodeName,
          "nodeGroup", plan.NodeGroupName,
          "offering", candidate.CurrentOffering)
      continue
  }
  ```
- [ ] Add same guard to `executeSurgeBatch` (after line 252, before Phase 1 provisioning):
  ```go
  // Filter out same-nodegroup same-offering candidates before provisioning
  var candidatesToProcess []CandidateNode
  for _, candidate := range batch.Nodes {
      currentNodeGroup := e.getNodeGroupFromNode(ctx, candidate.NodeName)
      if currentNodeGroup == plan.NodeGroupName && candidate.CurrentOffering == candidate.TargetOffering {
          logger.Info("Skipping termination: same nodegroup and offering",
              "nodeName", candidate.NodeName,
              "nodeGroup", plan.NodeGroupName,
              "offering", candidate.CurrentOffering)
          continue
      }
      candidatesToProcess = append(candidatesToProcess, candidate)
  }
  ```
- [ ] Run AC5 tests: `go test ./pkg/rebalancer -run TestSameNodeGroupProtection -v`

### 3. Refactor Phase
- [ ] Ensure logging is consistent between rolling and surge strategies
- [ ] Run all rebalancer tests: `go test ./pkg/rebalancer/... -v`

## Completion Criteria
- [ ] All 3 AC5 unit tests pass (Green state)
- [ ] Same-nodegroup with same offering skipped silently with info log
- [ ] Same-nodegroup with different offering proceeds (right-sizing use case)
- [ ] Different nodegroup always proceeds
- [ ] VPSie API is not called for skipped nodes (verified in tests)
- [ ] Guard clause added to both `executeRollingBatch` and `executeSurgeBatch`
- [ ] Operation verified (L2: Test Operation Verification)

## Implementation Reference

From Design Doc lines 432-458:
```go
// In executeRollingBatch, before calling TerminateNode
func (e *Executor) executeRollingBatch(ctx context.Context, plan *RebalancePlan, batch *NodeBatch, state *ExecutionState) (*batchResult, error) {
    // ... existing code ...

    for _, candidate := range batch.Nodes {
        // NEW: Check if this is a same-nodegroup "rebalance" (no-op)
        currentNodeGroup := e.getNodeGroupFromNode(ctx, candidate.NodeName)
        if currentNodeGroup == plan.NodeGroupName && candidate.CurrentOffering == candidate.TargetOffering {
            logger.Info("Skipping termination: same nodegroup and offering",
                "nodeName", candidate.NodeName,
                "nodeGroup", plan.NodeGroupName,
                "offering", candidate.CurrentOffering)
            continue
        }

        // ... rest of existing code ...
    }
}

func (e *Executor) getNodeGroupFromNode(ctx context.Context, nodeName string) string {
    node, err := e.kubeClient.CoreV1().Nodes().Get(ctx, nodeName, metav1.GetOptions{})
    if err != nil {
        return ""
    }
    return node.Labels["autoscaler.vpsie.com/nodegroup"]
}
```

## Test Verification

After implementation, these tests should pass:
```bash
go test ./pkg/rebalancer -run TestSameNodeGroupProtection -v
```

Expected output:
```
=== RUN   TestSameNodeGroupProtection
=== RUN   TestSameNodeGroupProtection/AC5:_Termination_skipped_when_same_nodegroup_with_same_offering
--- PASS: TestSameNodeGroupProtection/AC5:_Termination_skipped...
=== RUN   TestSameNodeGroupProtection/AC5:_Termination_proceeds_when_different_nodegroup
--- PASS: TestSameNodeGroupProtection/AC5:_Termination_proceeds_when_different...
=== RUN   TestSameNodeGroupProtection/AC5:_Termination_proceeds_when_same_nodegroup_but_different_offering_(right-sizing)
--- PASS: TestSameNodeGroupProtection/AC5:_Termination_proceeds...
--- PASS: TestSameNodeGroupProtection
```

## Notes
- Impact scope: `pkg/rebalancer/executor.go` (add guard), `pkg/rebalancer/executor_test.go` (enable tests)
- Constraints: Must not break existing rebalancer functionality
- This task completes Phase 3 (AC5) of the work plan
- The guard uses `continue` to skip silently, not `return` to fail
