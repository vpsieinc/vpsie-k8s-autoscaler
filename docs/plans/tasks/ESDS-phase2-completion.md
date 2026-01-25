# Phase 2 Completion: Enhanced canPodsBeRescheduled (AC1-AC4)

## Phase Summary
- Phase: 2 - Enhanced canPodsBeRescheduled
- Purpose: Implement per-pod scheduling simulation with full constraint checking
- Prerequisite Tasks: ESDS-005, ESDS-006, ESDS-007, ESDS-008

## Task Completion Checklist

### ESDS-005: Implement matchesNodeAffinity + matchesNodeSelectorTerms
- [ ] Completed: `matchesNodeAffinity` function implemented
- [ ] Completed: `matchesNodeSelectorTerms` function implemented
- [ ] Completed: `matchNodeSelectorRequirement` function implemented
- [ ] Verified: Only checks RequiredDuringScheduling (hard constraint)
- [ ] Verified: Supports In, NotIn, Exists, DoesNotExist operators

### ESDS-006: Implement hasPodAntiAffinityViolation
- [ ] Completed: `hasPodAntiAffinityViolation` function implemented
- [ ] Completed: `matchesPodAffinityTerm` function implemented
- [ ] Verified: Only checks Required anti-affinity (hard constraint)
- [ ] Verified: Handles topology key matching

### ESDS-007: Implement findSchedulableNode + buildNodePodsCache
- [ ] Completed: `findSchedulableNode` function implemented
- [ ] Completed: `buildNodePodsCache` method implemented
- [ ] Verified: Constraint check order matches Design Doc
- [ ] Verified: Uses existing MatchesNodeSelector helper

### ESDS-008: Refactor canPodsBeRescheduled + Green AC2/AC3/AC4 Tests
- [ ] Completed: canPodsBeRescheduled refactored
- [ ] Completed: t.Skip() removed from AC2/AC3/AC4 tests
- [ ] Completed: Blocking messages include pod name and constraint type
- [ ] Verified: All 6 new unit tests pass

## E2E Verification Procedures

### Unit Test Verification (L2)
```bash
# Run nodeSelector tests (AC2)
go test ./pkg/scaler -run TestNodeSelectorInCanPodsBeRescheduled -v

# Run anti-affinity tests (AC3)
go test ./pkg/scaler -run TestAntiAffinityVerification -v

# Run blocking message tests (AC4)
go test ./pkg/scaler -run TestClearBlockingMessages -v

# Run all Phase 2 tests together
go test ./pkg/scaler -run "TestNodeSelector|TestAntiAffinity|TestClearBlockingMessages" -v

# Run all scaler tests to verify no regression
go test ./pkg/scaler/... -v

# Verify test coverage
go test ./pkg/scaler -cover
```

### Expected Results
- All 6 new unit tests pass:
  - "AC2: Scale-down blocked - no remaining node has required label"
  - "AC2: Scale-down allowed - remaining node has required label"
  - "AC3: Scale-down blocked - would violate pod anti-affinity"
  - "AC3: Scale-down allowed - anti-affinity not violated"
  - "AC4: Blocking message includes pod name and constraint type"
- Plus AC1 tests continue to pass (3 tests)
- TestBackwardCompatibility passes
- No regressions in existing tests

### Performance Verification
```bash
# Verify constraint check order in code matches Design Doc
# tolerations -> nodeSelector -> affinity -> anti-affinity
grep -n "tolerationsTolerateTaints\|MatchesNodeSelector\|matchesNodeAffinity\|hasPodAntiAffinityViolation" pkg/scaler/safety.go
```

## Phase Completion Criteria
- [ ] All 6 new unit tests pass (AC2: 2, AC3: 2, AC4: 1, plus implicit AC1 integration)
- [ ] `findSchedulableNode` checks all constraints in performance-optimized order
- [ ] Blocking reason messages include pod name, namespace, and constraint type
- [ ] Existing `MatchesNodeSelector` helper is now actively used
- [ ] TestBackwardCompatibility passes (AC6 verified)
- [ ] Test resolution progress: 9/12 unit tests resolved

## Notes
- This phase completes AC2, AC3, AC4 of the Design Doc
- The optimistic cache update prevents anti-affinity violations
- Performance target: < 5 seconds for 100 nodes, 1000 pods
