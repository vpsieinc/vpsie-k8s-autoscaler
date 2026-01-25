# Task: ESDS-003 - Implement tolerationsTolerateTaints and Pass AC1 Tests

Metadata:
- Dependencies: ESDS-002 (tolerationMatches, tolerationMatchesTaint)
- Provides: Complete toleration matching capability; AC1 tests pass (Green state)
- Size: Small (1 file)

## Implementation Content
Implement the main toleration checking function and make AC1 tests pass:
1. `tolerationsTolerateTaints(tolerations []corev1.Toleration, taints []corev1.Taint) bool`
2. Remove `t.Skip()` from AC1 tests and verify they pass

## Target Files
- [x] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety.go`
- [x] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety_test.go`

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase
- [x] Verify ESDS-002 functions exist (`tolerationMatches`, `tolerationMatchesTaint`)
- [x] Remove `t.Skip()` from AC1 test cases in `TestTolerationMatching`
- [x] Run tests to confirm failure: `go test ./pkg/scaler -run TestTolerationMatching -v`

### 2. Green Phase
- [x] Add `tolerationsTolerateTaints` function after `tolerationMatchesTaint`:
  ```go
  // tolerationsTolerateTaints checks if tolerations cover all taints with NoSchedule/NoExecute effect.
  // Only hard constraints (NoSchedule, NoExecute) are checked.
  // PreferNoSchedule is a soft constraint and is ignored.
  func tolerationsTolerateTaints(tolerations []corev1.Toleration, taints []corev1.Taint) bool
  ```
- [x] Implement `tolerationsTolerateTaints`:
  - Iterate through taints
  - Skip PreferNoSchedule taints (soft constraint)
  - For NoSchedule and NoExecute taints, verify at least one toleration matches
  - Return false immediately if any hard-constraint taint is not tolerated
  - Return true if all hard-constraint taints are tolerated
- [x] Run AC1 tests: `go test ./pkg/scaler -run TestTolerationMatching -v`
- [x] Fix any failing assertions

### 3. Refactor Phase
- [x] Ensure function is well-documented with Kubernetes semantics
- [x] Verify performance (O(t*n) where t=taints, n=tolerations)
- [x] Run all safety tests: `go test ./pkg/scaler/... -v`

## Completion Criteria
- [x] `tolerationsTolerateTaints` correctly identifies toleration/taint mismatches
- [x] All 3 AC1 unit tests pass (Green state)
- [x] Wildcard toleration (empty key + Exists) matches any taint
- [x] PreferNoSchedule taints are correctly ignored
- [x] Test coverage for toleration functions: `go test ./pkg/scaler -cover -run TestTolerationMatching`
- [x] Operation verified (L2: Test Operation Verification)

## Implementation Reference

From Design Doc lines 313-329:
```go
func tolerationsTolerateTaints(tolerations []corev1.Toleration, taints []corev1.Taint) bool {
    for _, taint := range taints {
        // Only check hard constraints (NoSchedule, NoExecute)
        // PreferNoSchedule is soft - ignored for hard scheduling decisions
        if taint.Effect != corev1.TaintEffectNoSchedule &&
           taint.Effect != corev1.TaintEffectNoExecute {
            continue
        }

        // Check if any toleration matches this taint
        if !tolerationMatchesTaint(tolerations, &taint) {
            return false
        }
    }
    return true
}
```

## Test Verification

After implementation, these tests should pass:
```bash
go test ./pkg/scaler -run TestTolerationMatching -v
```

Expected output:
```
=== RUN   TestTolerationMatching
=== RUN   TestTolerationMatching/AC1:_Scale-down_blocked_-_pod_tolerates_taint_but_no_remaining_node_has_it
--- PASS: TestTolerationMatching/AC1:_Scale-down_blocked...
=== RUN   TestTolerationMatching/AC1:_Scale-down_allowed_-_remaining_node_has_matching_taint
--- PASS: TestTolerationMatching/AC1:_Scale-down_allowed...
=== RUN   TestTolerationMatching/AC1:_Wildcard_toleration_matches_any_taint
--- PASS: TestTolerationMatching/AC1:_Wildcard_toleration...
--- PASS: TestTolerationMatching
```

## Notes
- Impact scope: `pkg/scaler/safety.go`, `pkg/scaler/safety_test.go`
- Constraints: Must maintain backward compatibility - no changes to existing functions
- This task completes Phase 1 (AC1) of the work plan
