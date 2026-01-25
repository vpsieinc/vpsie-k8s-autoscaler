# Overall Design Document: Enhanced Scale-Down Safety

Generation Date: 2026-01-11
Target Plan Document: enhanced-scale-down-safety-plan.md

## Project Overview

### Purpose and Goals
Enhance the VPSie Kubernetes Autoscaler's scale-down safety mechanism to perform per-pod scheduling simulation before node removal. The current implementation only checks aggregate resource capacity, which can lead to workload disruption when pods with special scheduling constraints (tolerations, nodeSelector, affinity) cannot be rescheduled to remaining nodes.

### Background and Context
The existing `canPodsBeRescheduled()` function in `pkg/scaler/safety.go`:
1. Only checks aggregate CPU/memory capacity with 20% buffer
2. Does NOT check per-pod tolerations against node taints
3. Does NOT verify per-pod nodeSelector matches (helper exists but unused: `MatchesNodeSelector`)
4. Does NOT simulate actual scheduling per pod

Additionally, the rebalancer's `TerminateNode()` lacks protection against same-nodegroup operations that cause unnecessary churn.

## Task Division Design

### Division Policy
Tasks are divided following a **Vertical Slice** approach based on technical dependencies:

1. **Foundation First**: Toleration matching is foundational - required by `findSchedulableNode`
2. **Incremental Enhancement**: Each phase builds on previous functionality
3. **Independent Rebalancer**: Same-nodegroup protection can be implemented in parallel

Verifiability levels:
- All implementation tasks use **L2 (Unit Test Verification)** - new tests added and passing
- Phase completion tasks use **L2 (Integration Test Verification)**

### Inter-task Relationship Map
```
ESDS-001: Test skeletons for tolerationMatching
    |
    v
ESDS-002: Implement tolerationMatches + tolerationMatchesTaint
    |
    v
ESDS-003: Implement tolerationsTolerateTaints + Green AC1 tests
    |
    v
ESDS-004: Test skeletons for nodeSelector/affinity
    |
    v
ESDS-005: Implement matchesNodeAffinity + matchesNodeSelectorTerms
    |
    v
ESDS-006: Implement hasPodAntiAffinityViolation + matchesPodAffinityTerm
    |
    v
ESDS-007: Implement findSchedulableNode + buildNodePodsCache
    |
    v
ESDS-008: Refactor canPodsBeRescheduled + Green AC2/AC3/AC4 tests
    |
    |
    +-------> ESDS-009: Test skeletons for same-nodegroup (parallel)
                  |
                  v
              ESDS-010: Implement getNodeGroupFromNode + guard clauses + Green AC5 tests

Phase Completions:
- ESDS-phase0-completion: After ESDS-001, ESDS-004, ESDS-009
- ESDS-phase1-completion: After ESDS-003
- ESDS-phase2-completion: After ESDS-008
- ESDS-phase3-completion: After ESDS-010
- ESDS-phase4-completion: Quality Assurance (AC6)
```

### Interface Change Impact Analysis
| Existing Interface | New Interface | Conversion Required | Corresponding Task |
|-------------------|---------------|-------------------|-------------------|
| `canPodsBeRescheduled(ctx, pods)` | `canPodsBeRescheduled(ctx, pods)` | None (signature unchanged) | ESDS-008 |
| `MatchesNodeSelector(node, pod)` | `MatchesNodeSelector(node, pod)` | None (now actively used) | ESDS-007 |
| `HasNodeSelector(pod)` | `HasNodeSelector(pod)` | None (now used internally) | ESDS-007 |
| `HasNodeAffinity(pod)` | `HasNodeAffinity(pod)` | None (now used internally) | ESDS-005 |
| `executeRollingBatch(ctx, plan, batch, state)` | `executeRollingBatch(ctx, plan, batch, state)` | None (guard added) | ESDS-010 |

### Common Processing Points
- **Existing helpers to reuse**: `MatchesNodeSelector`, `HasNodeSelector`, `HasNodeAffinity`, `isSkippableDaemonSetPod`
- **New shared functions**: `tolerationMatches`, `tolerationMatchesTaint`, `tolerationsTolerateTaints`
- **Caching strategy**: `buildNodePodsCache` creates one-time pod cache per `canPodsBeRescheduled` call

## Implementation Considerations

### Principles to Maintain Throughout
1. **Backward Compatibility**: Existing tests in `safety_test.go` must continue to pass
2. **Fail-Fast**: Return immediately on first non-schedulable pod (performance)
3. **Hard Constraints Only**: Check `RequiredDuringScheduling*` only, skip `Preferred*` (soft constraints)
4. **Constraint Check Order**: tolerations (O(n)) -> nodeSelector (O(1)) -> affinity (O(1)) -> anti-affinity (O(n*m))

### Risks and Countermeasures
- **Risk**: Performance degradation with large clusters (100+ nodes, 1000+ pods)
  **Countermeasure**: Fail-fast on first non-schedulable pod; cache node pods; short-circuit on node match

- **Risk**: Edge cases in Kubernetes toleration matching semantics
  **Countermeasure**: Follow Kubernetes official documentation precisely; test wildcard tolerations; test effect matching

- **Risk**: Existing safety_test.go tests could break
  **Countermeasure**: Run existing tests first; ensure backward compatibility (AC6)

### Impact Scope Management
- **Allowed change scope**: `pkg/scaler/safety.go`, `pkg/scaler/safety_test.go`, `pkg/rebalancer/executor.go`, `pkg/rebalancer/executor_test.go`
- **No-change areas**: scale-up path, VPSie API integration, NodeGroup reconciler, DrainNode behavior

## Task Summary

| Task ID | Description | Size | Verification |
|---------|-------------|------|--------------|
| ESDS-001 | Test skeletons for toleration matching (Phase 0) | Small (1 file) | L3 (compile) |
| ESDS-002 | Implement tolerationMatches + tolerationMatchesTaint | Small (1 file) | L2 (partial tests) |
| ESDS-003 | Implement tolerationsTolerateTaints + Green AC1 tests | Small (1 file) | L2 (AC1 tests pass) |
| ESDS-004 | Test skeletons for nodeSelector/affinity (Phase 0) | Small (1 file) | L3 (compile) |
| ESDS-005 | Implement matchesNodeAffinity + matchesNodeSelectorTerms | Small (1 file) | L2 (unit tests) |
| ESDS-006 | Implement hasPodAntiAffinityViolation | Small (1 file) | L2 (unit tests) |
| ESDS-007 | Implement findSchedulableNode + buildNodePodsCache | Small (1 file) | L2 (unit tests) |
| ESDS-008 | Refactor canPodsBeRescheduled + Green AC2/AC3/AC4 tests | Medium (2 files) | L2 (6 tests pass) |
| ESDS-009 | Test skeletons for same-nodegroup (Phase 0) | Small (1 file) | L3 (compile) |
| ESDS-010 | Implement same-nodegroup protection + Green AC5 tests | Small (2 files) | L2 (3 tests pass) |

## Acceptance Criteria Mapping

| AC | Description | Unit Tests | Integration Tests | Tasks |
|----|-------------|------------|-------------------|-------|
| AC1 | Toleration matching | 3 tests | 1 test | ESDS-001, ESDS-002, ESDS-003 |
| AC2 | NodeSelector matching | 2 tests | 1 test | ESDS-004, ESDS-007, ESDS-008 |
| AC3 | Anti-affinity verification | 2 tests | - | ESDS-004, ESDS-006, ESDS-008 |
| AC4 | Clear blocking messages | 1 test | - | ESDS-008 |
| AC5 | Same-nodegroup protection | 3 tests | 1 test | ESDS-009, ESDS-010 |
| AC6 | Backward compatibility | 1 test | 1 test | Phase 4 completion |
| **Total** | | **12 tests** | **4 tests** | |

## Files Modified

| File | Tasks | Changes | Lines (est.) |
|------|-------|---------|--------------|
| `pkg/scaler/safety.go` | ESDS-002,003,005,006,007,008 | Add toleration matching, findSchedulableNode, refactor canPodsBeRescheduled | +150 |
| `pkg/scaler/safety_test.go` | ESDS-001,004 | Complete 9 test skeletons | +200 |
| `pkg/rebalancer/executor.go` | ESDS-010 | Add getNodeGroupFromNode, guard clause | +30 |
| `pkg/rebalancer/executor_test.go` | ESDS-009 | Complete 3 test skeletons | +80 |

## Execution Order

1. **Phase 0 (Red State)**: ESDS-001, ESDS-004, ESDS-009 (can be parallel)
2. **Phase 1 (AC1)**: ESDS-002 -> ESDS-003
3. **Phase 2 (AC2-4)**: ESDS-005 -> ESDS-006 -> ESDS-007 -> ESDS-008
4. **Phase 3 (AC5)**: ESDS-010 (after ESDS-009)
5. **Phase 4 (AC6)**: Quality Assurance (phase4-completion)
