# Overall Design Document: Node Discovery Implementation

Generation Date: 2025-01-09
Target Plan Document: node-discovery-workplan.md
Related Design Doc: docs/designs/node-discovery.md

## Project Overview

### Purpose and Goals

Implement a Node Discovery mechanism that resolves critical gaps in the VPSie K8s Autoscaler where:
1. VPS ID=0 after async provisioning prevents proper VPSieNode lifecycle management
2. Missing K8s node labels break NodeGroup status tracking
3. ScaleDownManager cannot identify nodes for removal

### Background and Context

When `AddK8sSlaveToGroup()` is called, the VPSie API returns `data: true` without a node ID (async provisioning). This causes VPSieNodes to get stuck in `Provisioning` phase with `VPSieInstanceID=0`. The solution implements VPS ID discovery by querying VPSie API and matching K8s nodes using IP (primary) and hostname (fallback).

## Task Division Design

### Division Policy

**Strategy: Vertical Slice (Feature-driven)** with **Foundation-first ordering**

- Phase 1 creates the interface foundation (must be completed before any discovery logic)
- Phase 2 implements the Discoverer component in isolation with full unit test coverage
- Phase 3 integrates into the existing controller flow
- Phase 4 validates all acceptance criteria

**Verifiability Levels:**
- Task 01 (Interface): L3 - Build Success Verification
- Task 02 (Discoverer Core): L2 - Test Operation Verification
- Task 03 (Discoverer Tests): L2 - Test Operation Verification
- Task 04 (Provisioner Integration): L2 - Test Operation Verification
- Task 05 (Joiner Enhancement): L1 - Functional Operation Verification

### Inter-task Relationship Map

```
Task 01: Extend VPSieClientInterface + Update MockVPSieClient
  |
  v (interface ready)
Task 02: Create Discoverer component with core methods
  |
  v (discoverer implemented)
Task 03: Comprehensive unit tests for Discoverer
  |
  v (tests verified)
Task 04: Inject Discoverer into Provisioner, add discovery to createVPS
  |
  v (provisioner integrated)
Task 05: Enhance Joiner IP-first matching + Integration tests
  |
  v (all components integrated)
Phase 4: Quality Assurance (all 27 acceptance criteria verified)
```

### Interface Change Impact Analysis

| Existing Interface | New Interface | Conversion Required | Corresponding Task |
|-------------------|---------------|---------------------|-------------------|
| `VPSieClientInterface` (7 methods) | + `ListK8sNodeGroups()` | Yes (add method) | Task 01 |
| `MockVPSieClient` | + `ListK8sNodeGroupsFunc` | Yes (add mock field) | Task 01 |
| `Provisioner` struct | + `discoverer` field | Yes (add field) | Task 04 |
| `Provisioner.createVPS()` | + discovery call | Internal change | Task 04 |
| `Joiner.findKubernetesNode()` | IP-first ordering | Logic reorder | Task 05 |

### Common Processing Points

1. **IP-based node matching**: Both Discoverer and Joiner need to find K8s nodes by IP
   - Discoverer: `findK8sNodeByIP()` method
   - Joiner: Already has `findNodeByIP()` method
   - Design: Discoverer gets its own implementation to avoid circular dependency

2. **Hostname pattern matching**: Used in Discoverer for VPS-to-VPSieNode correlation
   - `matchesHostnamePattern()` helper function

3. **Timeout checking**: Both discovery and joining have 15-minute timeouts
   - Constant `DiscoveryTimeout = 15 * time.Minute`
   - Existing `ProvisioningTimeout` for provisioning phase

## Implementation Considerations

### Principles to Maintain Throughout

1. **Conservative Discovery Scope**: Only VPSieNodes with `creation-requested=true` annotation trigger discovery
2. **IP-First Matching Strategy**: More reliable than hostname for K8s node correlation
3. **Fail on Timeout**: VPSieNodes that cannot be discovered within 15 minutes transition to Failed phase
4. **No CRD Changes**: All required fields already exist in VPSieNode spec/status

### Risks and Countermeasures

- **Risk**: VPSie API rate limiting during discovery polling
  - **Countermeasure**: Max 1 API call per VPSieNode per reconcile cycle; reuse existing rate limiter

- **Risk**: Race condition between discovery and node join
  - **Countermeasure**: IP-first matching allows correlation even without VPS ID

- **Risk**: Multiple VPSieNodes provisioning concurrently
  - **Countermeasure**: `isNodeClaimedByOther()` method prevents duplicate assignment

### Impact Scope Management

**Allowed Change Scope:**
- `pkg/controller/vpsienode/vpsie_interface.go` - Add interface method
- `pkg/controller/vpsienode/mock_vpsie_client.go` - Add mock implementation
- `pkg/controller/vpsienode/discoverer.go` - NEW FILE
- `pkg/controller/vpsienode/discoverer_test.go` - NEW FILE
- `pkg/controller/vpsienode/provisioner.go` - Add discoverer field and discovery call
- `pkg/controller/vpsienode/controller.go` - Create and inject discoverer
- `pkg/controller/vpsienode/joiner.go` - Reorder matching strategy

**No-Change Areas:**
- CRD definitions (`pkg/apis/autoscaler/v1alpha1/types.go`)
- NodeGroup controller (`pkg/controller/nodegroup/`)
- VPSie API client (`pkg/vpsie/client/client.go`)
- ScaleDownManager (`pkg/scaler/`)

## Files Summary

### New Files (2)
- `pkg/controller/vpsienode/discoverer.go`
- `pkg/controller/vpsienode/discoverer_test.go`

### Modified Files (5)
- `pkg/controller/vpsienode/vpsie_interface.go` - ADD method signature
- `pkg/controller/vpsienode/mock_vpsie_client.go` - ADD mock implementation
- `pkg/controller/vpsienode/provisioner.go` - ADD discoverer field, discovery logic
- `pkg/controller/vpsienode/controller.go` - CREATE and inject discoverer
- `pkg/controller/vpsienode/joiner.go` - REORDER IP-first matching

## Task Summary

| Task | Phase | Size | Files | Verification Level |
|------|-------|------|-------|-------------------|
| Task 01 | Phase 1 | Small (2 files) | vpsie_interface.go, mock_vpsie_client.go | L3: Build Success |
| Task 02 | Phase 2 | Small (1 file) | discoverer.go | L2: Test Operation |
| Task 03 | Phase 2 | Small (1 file) | discoverer_test.go | L2: Test Operation |
| Task 04 | Phase 3 | Medium (2 files) | provisioner.go, controller.go | L2: Test Operation |
| Task 05 | Phase 3 | Small (1 file) | joiner.go | L1: Functional Operation |

## Acceptance Criteria Reference

Total: 27 acceptance criteria from Design Doc Section 13

- VPS Discovery (D1-D9): 9 criteria
- K8s Node Matching (M1-M4): 4 criteria
- Label Application (L1-L5): 5 criteria
- NodeGroup Status (S1-S3): 3 criteria
- Error Handling (E1-E4): 4 criteria
- Performance (P1-P2): 2 criteria implied
