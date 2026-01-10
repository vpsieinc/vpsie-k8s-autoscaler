# Phase 3 Completion: Controller Integration

## Phase Overview

**Purpose**: Integrate Discoverer into the provisioning flow and enhance node matching
**Tasks Included**: Task 04, Task 05

## Task Completion Checklist

- [ ] Task 04: Provisioner Integration completed
  - [ ] Discoverer injected into Provisioner
  - [ ] `createVPS()` calls `DiscoverVPSID()` appropriately
  - [ ] VPSieNode spec updated with discovered VPS info
  - [ ] Timeout handling marks VPSieNode as Failed

- [ ] Task 05: Joiner Enhancement completed
  - [ ] IP-first matching strategy implemented
  - [ ] Debug logging added for matching strategies
  - [ ] Unit tests for IP-first matching pass

## E2E Verification Procedures

### Full Controller Test Suite

```bash
# Run all vpsienode controller tests
go test ./pkg/controller/vpsienode/... -v

# Run with race detection
go test -race ./pkg/controller/vpsienode/... -v
```

### Provisioner Discovery Flow

```bash
# Test discovery integration
go test ./pkg/controller/vpsienode -run TestProvisioner -v
```

**Verify**:
1. Create VPSieNode with `creation-requested=true`, `VPSieInstanceID=0`
2. Mock `ListVMs` to return matching VPS
3. Assert `VPSieNode.Spec.VPSieInstanceID` updated after reconcile

### Timeout Flow

```bash
# Test timeout handling
go test ./pkg/controller/vpsienode -run TestProvisioner_CreateVPS_WithDiscovery_Timeout -v
```

**Verify**:
1. Create VPSieNode with `CreatedAt > 15 minutes ago`
2. Assert error returned (triggers Failed phase transition)

### Joiner IP-First Matching

```bash
# Test IP-first matching
go test ./pkg/controller/vpsienode -run TestJoiner -v
```

**Verify**:
1. VPSieNode with `IPAddress="10.0.0.5"`
2. K8s node with different name but `InternalIP="10.0.0.5"`
3. Assert node found by IP

### Integration Tests (if implemented)

```bash
# Run integration tests
go test -tags=integration ./test/integration -run TestNodeDiscovery -v
```

## Phase Completion Criteria

- [ ] Discoverer injected into Provisioner during controller construction
- [ ] `createVPS()` calls `DiscoverVPSID()` when:
  - `creation-requested=true` annotation present
  - `VPSieInstanceID=0`
- [ ] VPSieNode spec updated with discovered VPS information:
  - `Spec.VPSieInstanceID`
  - `Spec.IPAddress`
  - `Spec.IPv6Address`
  - `Status.Hostname`
  - `Status.VPSieStatus`
  - `Status.Resources`
- [ ] Joiner uses IP-first matching strategy
- [ ] Timeout marks VPSieNode as Failed with "DiscoveryTimeout" reason
- [ ] All tests pass

## Acceptance Criteria Mapping

| Design Doc AC | Test/Verification |
|--------------|-------------------|
| D2: VPSieNode.Spec.VPSieInstanceID updated | TestProvisioner_CreateVPS_WithDiscovery_Success |
| D3: VPSieNode.Spec.IPAddress updated | TestProvisioner_CreateVPS_WithDiscovery_Success |
| D7: Discovery completes within API timeout | Existing client timeout (30s) |
| M1: K8s node matched by IP (primary) | TestJoiner_FindKubernetesNode_IPFirst |
| M2: K8s node matched by hostname (fallback) | TestJoiner_FindKubernetesNode_FallbackToHostname |
| M3: VPSieNode.Status.NodeName updated | Existing CheckJoinStatus() |
| E1-E3: Error handling | TestProvisioner_CreateVPS_WithDiscovery_* |

## Notes

- This phase completes the core implementation
- Phase 4 will verify all 27 acceptance criteria
- All tests must pass before proceeding to Phase 4
