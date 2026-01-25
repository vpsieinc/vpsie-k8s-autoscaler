# Task: ESDS-009 - Test Skeletons for Same-NodeGroup Protection

Metadata:
- Dependencies: None (can run parallel with ESDS-001, ESDS-004)
- Provides: Test skeletons for AC5 (3 tests) - Red state verification
- Size: Small (1 file)

## Implementation Content
Complete the test skeleton implementations for same-nodegroup protection in `executor_test.go`. The test skeletons already exist with `t.Skip()` - this task fills in the test data and assertions while keeping the skip to maintain Red state.

Existing skeletons in `TestSameNodeGroupProtection`:
1. "AC5: Termination skipped when same nodegroup with same offering"
2. "AC5: Termination proceeds when different nodegroup"
3. "AC5: Termination proceeds when same nodegroup but different offering (right-sizing)"

## Target Files
- [x] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/rebalancer/executor_test.go`

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase
- [x] Read existing `TestSameNodeGroupProtection` function (lines 260-357)
- [x] Complete "AC5: Termination skipped when same nodegroup with same offering" test:
  - Create node in nodegroup "test-ng" with label `autoscaler.vpsie.com/nodegroup=test-ng`
  - Create RebalancePlan with NodeGroupName: "test-ng"
  - Create CandidateNode with CurrentOffering == TargetOffering
  - Add assertions for VPSie API NOT called
  - Add assertions for result.NodesFailed == 0
  - Add assertions for skip log message
  - Keep `t.Skip()` at the end
- [x] Complete "AC5: Termination proceeds when different nodegroup" test:
  - Create node in nodegroup "source-ng"
  - Create RebalancePlan with NodeGroupName: "target-ng" (different)
  - Add assertions for normal execution flow attempted
  - Keep `t.Skip()` at the end
- [x] Complete "AC5: Same nodegroup but different offering" test:
  - Create node in nodegroup "test-ng"
  - Create RebalancePlan with NodeGroupName: "test-ng"
  - Create CandidateNode with CurrentOffering != TargetOffering
  - Add assertions for normal execution flow attempted (right-sizing)
  - Keep `t.Skip()` at the end
- [x] Run tests and confirm they compile but skip: `go test ./pkg/rebalancer -run TestSameNodeGroupProtection -v`

### 2. Green Phase
- [x] N/A - This is Phase 0, tests remain in Red state

### 3. Refactor Phase
- [x] N/A - This is Phase 0

## Completion Criteria
- [x] All 3 test cases have concrete test data (nodes, plans, candidates)
- [x] All 3 test cases have concrete assertions
- [x] All 3 test cases end with `t.Skip("Skeleton: Implementation required - ...")`
- [x] Tests compile successfully: `go build ./pkg/rebalancer/...`
- [x] Tests skip with expected messages
- [x] Operation verified (L3: Build Success Verification)

## Code Template - AC5 Skip Test

```go
t.Run("AC5: Termination skipped when same nodegroup with same offering", func(t *testing.T) {
    // Arrange:
    node := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: "test-node-1",
            Labels: map[string]string{
                "autoscaler.vpsie.com/nodegroup": "test-ng",
            },
        },
        Spec: corev1.NodeSpec{Unschedulable: false},
        Status: corev1.NodeStatus{
            Conditions: []corev1.NodeCondition{
                {Type: corev1.NodeReady, Status: corev1.ConditionTrue},
            },
        },
    }

    fakeClient := fake.NewSimpleClientset(node)
    executor := NewExecutor(fakeClient, nil, &ExecutorConfig{
        DrainTimeout:        5 * time.Minute,
        ProvisionTimeout:    10 * time.Minute,
        HealthCheckInterval: 10 * time.Second,
        MaxRetries:          3,
    })

    plan := &RebalancePlan{
        ID:            "test-plan-same-ng",
        NodeGroupName: "test-ng", // SAME as node's nodegroup
        Namespace:     "default",
        Strategy:      StrategyRolling,
        Batches: []NodeBatch{
            {
                BatchNumber: 1,
                Nodes: []CandidateNode{
                    {
                        NodeName:        "test-node-1",
                        CurrentOffering: "offering-standard-2-4",
                        TargetOffering:  "offering-standard-2-4", // SAME as current
                    },
                },
            },
        },
    }

    state := &ExecutionState{
        PlanID:           plan.ID,
        Status:           StatusInProgress,
        CurrentBatch:     0,
        CompletedNodes:   make([]string, 0),
        FailedNodes:      make([]NodeFailure, 0),
        ProvisionedNodes: make([]string, 0),
        StartedAt:        time.Now(),
    }

    // Act:
    // result, err := executor.executeRollingBatch(context.TODO(), plan, &plan.Batches[0], state)

    // Assert:
    // require.NoError(t, err)
    // assert.Equal(t, int32(0), result.NodesFailed, "No nodes should fail - operation skipped")
    // assert.Equal(t, int32(0), result.NodesRebalanced, "No nodes should be rebalanced - operation skipped")
    // Verify VPSie API not called (mock verification)
    // Verify log contains "Skipping termination: same nodegroup"

    _ = executor
    _ = plan
    _ = state
    t.Skip("Skeleton: Implementation required - same-nodegroup guard clause in executeRollingBatch")
})
```

## Code Template - AC5 Different NodeGroup Test

```go
t.Run("AC5: Termination proceeds when different nodegroup", func(t *testing.T) {
    // Arrange:
    node := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: "test-node-1",
            Labels: map[string]string{
                "autoscaler.vpsie.com/nodegroup": "source-ng", // Source nodegroup
            },
        },
        Spec: corev1.NodeSpec{Unschedulable: false},
        Status: corev1.NodeStatus{
            Conditions: []corev1.NodeCondition{
                {Type: corev1.NodeReady, Status: corev1.ConditionTrue},
            },
        },
    }

    fakeClient := fake.NewSimpleClientset(node)
    executor := NewExecutor(fakeClient, nil, &ExecutorConfig{
        DrainTimeout:        5 * time.Minute,
        ProvisionTimeout:    10 * time.Minute,
        HealthCheckInterval: 10 * time.Second,
        MaxRetries:          3,
    })

    plan := &RebalancePlan{
        ID:            "test-plan-diff-ng",
        NodeGroupName: "target-ng", // DIFFERENT from node's nodegroup
        Namespace:     "default",
        Strategy:      StrategyRolling,
        Batches: []NodeBatch{
            {
                BatchNumber: 1,
                Nodes: []CandidateNode{
                    {
                        NodeName:        "test-node-1",
                        CurrentOffering: "offering-standard-2-4",
                        TargetOffering:  "offering-standard-4-8",
                    },
                },
            },
        },
    }

    state := &ExecutionState{
        PlanID:           plan.ID,
        Status:           StatusInProgress,
        CurrentBatch:     0,
        CompletedNodes:   make([]string, 0),
        FailedNodes:      make([]NodeFailure, 0),
        ProvisionedNodes: make([]string, 0),
        StartedAt:        time.Now(),
    }

    // Act:
    // result, err := executor.executeRollingBatch(context.TODO(), plan, &plan.Batches[0], state)

    // Assert:
    // The execution should proceed (and fail at provisioning since it's not implemented)
    // This verifies the guard clause does NOT block different-nodegroup operations
    // require.NoError(t, err)
    // assert.Equal(t, int32(1), result.NodesFailed, "Should fail at provisioning")
    // Verify the failure is at "provision" stage, not blocked by same-nodegroup check

    _ = executor
    _ = plan
    _ = state
    t.Skip("Skeleton: Implementation required - same-nodegroup guard clause in executeRollingBatch")
})
```

## Notes
- Impact scope: Test file only - no production code changes
- Constraints: Maintain existing test structure and imports
- These tests verify AC5 which is independent of AC1-AC4 and can be implemented in parallel
