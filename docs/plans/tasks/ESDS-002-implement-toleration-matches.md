# Task: ESDS-002 - Implement tolerationMatches and tolerationMatchesTaint

Metadata:
- Dependencies: ESDS-001 (test skeletons exist)
- Provides: Foundation toleration matching functions for ESDS-003
- Size: Small (1 file)

## Implementation Content
Implement the core toleration matching functions per Kubernetes official documentation:
1. `tolerationMatches(toleration *corev1.Toleration, taint *corev1.Taint) bool` - Single toleration/taint match
2. `tolerationMatchesTaint(tolerations []corev1.Toleration, taint *corev1.Taint) bool` - Find any matching toleration

These are pure functions with no external dependencies.

## Target Files
- [x] `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/safety.go`

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase
- [x] Verify ESDS-001 test skeletons exist and compile
- [x] Read Design Doc toleration matching algorithm (lines 308-367)
- [x] Write a simple unit test for `tolerationMatches` edge cases:
  - Empty key with Exists operator (wildcard) -> true
  - Matching key/value/effect with Equal operator -> true
  - Mismatched key -> false
  - Mismatched effect -> false
  - Exists operator ignores value -> true
- [x] Run test to confirm failure: `go test ./pkg/scaler -run TestTolerationMatches -v`

### 2. Green Phase
- [x] Add `tolerationMatches` function after line 501 (after `MatchesNodeSelector`):
  ```go
  // tolerationMatches checks if a toleration matches a taint.
  // Per Kubernetes documentation:
  // - Empty key with Exists operator matches all taints (wildcard)
  // - Key must match
  // - Effect must match (empty toleration effect matches all effects)
  // - Operator: Exists matches any value, Equal requires value match
  func tolerationMatches(toleration *corev1.Toleration, taint *corev1.Taint) bool
  ```
- [x] Implement `tolerationMatches`:
  - Handle empty key with Exists operator (wildcard) -> return true
  - Check key match
  - Check effect match (empty toleration effect matches all)
  - Handle Equal and Exists operators
- [x] Add `tolerationMatchesTaint` function:
  ```go
  // tolerationMatchesTaint checks if any toleration in the list matches the taint.
  func tolerationMatchesTaint(tolerations []corev1.Toleration, taint *corev1.Taint) bool
  ```
- [x] Implement `tolerationMatchesTaint`:
  - Iterate tolerations and return true if any matches
- [x] Run tests: `go test ./pkg/scaler -run TestTolerationMatches -v`

### 3. Refactor Phase
- [x] Ensure code follows existing style in safety.go
- [x] Add inline comments explaining Kubernetes semantics
- [x] Verify no duplicate logic with existing code

## Completion Criteria
- [x] `tolerationMatches` handles all operator types (Equal, Exists, default)
- [x] `tolerationMatches` handles empty key wildcard
- [x] `tolerationMatches` handles empty effect (matches all)
- [x] `tolerationMatchesTaint` finds first match in list
- [x] New unit tests for edge cases pass
- [x] Existing tests still pass: `go test ./pkg/scaler/... -v`
- [x] Operation verified (L2: Test Operation Verification)

## Implementation Reference

From Design Doc lines 340-367:
```go
func tolerationMatches(toleration *corev1.Toleration, taint *corev1.Taint) bool {
    // Empty key with Exists operator matches all taints
    if toleration.Key == "" && toleration.Operator == corev1.TolerationOpExists {
        return true
    }

    // Key must match
    if toleration.Key != taint.Key {
        return false
    }

    // Effect must match (empty toleration effect matches all effects)
    if toleration.Effect != "" && toleration.Effect != taint.Effect {
        return false
    }

    // Operator-based value matching
    switch toleration.Operator {
    case corev1.TolerationOpExists:
        // Exists operator matches any value
        return true
    case corev1.TolerationOpEqual, "":
        // Equal operator (or default) requires value match
        return toleration.Value == taint.Value
    }

    return false
}
```

## Notes
- Impact scope: `pkg/scaler/safety.go` only
- Constraints: Must follow Kubernetes official toleration matching semantics
- Reference: https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/
