# Task: ESDS-001 - Test Skeletons for Toleration Matching

Metadata:
- Dependencies: None
- Provides: Test skeletons for AC1 (3 tests) - Red state verification
- Size: Small (1 file)

## Implementation Content
Complete the test skeleton implementations for toleration matching in `safety_test.go`. The test skeletons already exist with `t.Skip()` - this task fills in the test data and assertions while keeping the skip to maintain Red state.

The existing skeletons in `TestTolerationMatching`:
1. "AC1: Scale-down blocked - pod tolerates taint but no remaining node has it"
2. "AC1: Scale-down allowed - remaining node has matching taint"
3. "AC1: Wildcard toleration matches any taint"

## Target Files
- [x] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety_test.go`

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase
- [x] Read existing `TestTolerationMatching` function (lines 1107-1181)
- [x] Verify test function structure follows table-driven test pattern
- [x] Complete "AC1: Scale-down blocked" test case:
  - Create pod with toleration: `{Key: "gpu", Value: "true", Effect: NoSchedule, Operator: Equal}`
  - Create node to remove with taint: `{Key: "gpu", Value: "true", Effect: NoSchedule}`
  - Create remaining node WITHOUT gpu taint
  - Add assertions for `tolerationsTolerateTaints(pod.Tolerations, remainingNode.Taints) == false`
  - Keep `t.Skip()` at the end
- [x] Complete "AC1: Scale-down allowed" test case:
  - Create pod with same toleration
  - Create remaining node WITH matching taint
  - Add assertions for `tolerationsTolerateTaints(pod.Tolerations, remainingNode.Taints) == true`
  - Keep `t.Skip()` at the end
- [x] Complete "AC1: Wildcard toleration" test case:
  - Create pod with wildcard toleration: `{Key: "", Operator: Exists, Effect: ""}`
  - Create node with any taint: `{Key: "special", Value: "value", Effect: NoSchedule}`
  - Add assertions for `tolerationMatches(&toleration, &taint) == true`
  - Keep `t.Skip()` at the end
- [x] Run tests and confirm they compile but skip: `go test ./pkg/scaler -run TestTolerationMatching -v`

### 2. Green Phase
- [x] N/A - This is Phase 0, tests remain in Red state

### 3. Refactor Phase
- [x] N/A - This is Phase 0

## Completion Criteria
- [x] All 3 test cases have concrete test data (pods, nodes, taints, tolerations)
- [x] All 3 test cases have concrete assertions
- [x] All 3 test cases end with `t.Skip("Skeleton: Implementation required - ...")`
- [x] Tests compile successfully: `go build ./pkg/scaler/...`
- [x] Tests skip with expected messages: `go test ./pkg/scaler -run TestTolerationMatching -v`
- [x] Operation verified (L3: Build Success Verification)

## Code Template

```go
t.Run("AC1: Scale-down blocked - pod tolerates taint but no remaining node has it", func(t *testing.T) {
    // Arrange:
    pod := &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "gpu-workload",
            Namespace: "default",
        },
        Spec: corev1.PodSpec{
            Tolerations: []corev1.Toleration{
                {
                    Key:      "gpu",
                    Value:    "true",
                    Effect:   corev1.TaintEffectNoSchedule,
                    Operator: corev1.TolerationOpEqual,
                },
            },
        },
    }

    remainingNode := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: "worker-no-gpu",
        },
        Spec: corev1.NodeSpec{
            // No taints - this node does NOT have gpu taint
            Taints: []corev1.Taint{},
        },
    }

    // Act:
    // result := tolerationsTolerateTaints(pod.Spec.Tolerations, remainingNode.Spec.Taints)

    // Assert:
    // assert.True(t, result, "Pod should tolerate node without gpu taint since no NoSchedule/NoExecute taints")
    // NOTE: The actual test will verify that pod CANNOT be scheduled on remaining node
    // because the pod requires a node with gpu=true:NoSchedule taint (inverse logic to verify)

    t.Skip("Skeleton: Implementation required - tolerationsTolerateTaints function")
})
```

## Notes
- Impact scope: Test file only - no production code changes
- Constraints: Maintain existing test structure and imports
- The test skeletons already exist in the file - this task completes them with concrete data
