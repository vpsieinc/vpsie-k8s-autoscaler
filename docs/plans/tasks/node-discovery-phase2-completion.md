# Phase 2 Completion: Discovery Core Implementation

## Phase Overview

**Purpose**: Implement the core Discoverer component with VPS ID discovery and K8s node matching logic
**Tasks Included**: Task 02, Task 03

## Task Completion Checklist

- [ ] Task 02: Discoverer Component Core completed
  - [ ] `Discoverer` struct created with all fields
  - [ ] `NewDiscoverer()` constructor implemented
  - [ ] `DiscoverVPSID()` method implemented
  - [ ] `matchesHostnamePattern()` helper implemented
  - [ ] `findK8sNodeByIP()` method implemented
  - [ ] `isNodeClaimedByOther()` method implemented

- [ ] Task 03: Discoverer Unit Tests completed
  - [ ] All 15 test cases implemented
  - [ ] Tests follow AAA pattern
  - [ ] Test helpers extracted
  - [ ] All tests pass
  - [ ] Tests pass with race detector

## E2E Verification Procedures

### Discoverer Functionality Tests

```bash
# Run all discoverer tests
go test ./pkg/controller/vpsienode/... -run TestDiscover -v

# Run with race detection
go test -race ./pkg/controller/vpsienode/... -run TestDiscover -v
```

### Test Coverage Check

```bash
# Generate coverage report
go test ./pkg/controller/vpsienode/... -coverprofile=coverage.out -run TestDiscover

# Check coverage for discoverer.go
go tool cover -func=coverage.out | grep discoverer

# Target: >= 85% coverage for discoverer.go
```

### Specific Scenario Verification

1. **Timeout Handling Test**:
   ```bash
   go test ./pkg/controller/vpsienode -run TestDiscoverVPSID_Timeout -v
   ```
   - Verify: Returns `timedOut=true` when `CreatedAt > 15 minutes ago`

2. **Hostname Matching Test**:
   ```bash
   go test ./pkg/controller/vpsienode -run TestMatchesHostnamePattern -v
   ```
   - Verify: "my-node" matches "my-node-k8s-worker"
   - Verify: "my-node" does NOT match "other-node"

3. **IP Matching Test**:
   ```bash
   go test ./pkg/controller/vpsienode -run TestFindK8sNodeByIP -v
   ```
   - Verify: Discovers VPS when K8s node has matching IP

4. **Claim Checking Test**:
   ```bash
   go test ./pkg/controller/vpsienode -run TestIsNodeClaimedByOther -v
   ```
   - Verify: Returns true when node is claimed by different VPSieNode

## Phase Completion Criteria

- [ ] Discoverer component created with all methods
- [ ] Hostname pattern matching implemented
- [ ] IP-based K8s node matching implemented
- [ ] Timeout handling implemented (15 minute default)
- [ ] Unit tests cover discovery success/failure/timeout scenarios
- [ ] Test coverage >= 85% for discoverer.go
- [ ] Tests pass with race detector

## Acceptance Criteria Mapping

| Design Doc AC | Test/Verification |
|--------------|-------------------|
| D1: VPS ID discovered within 15 minutes | TestDiscoverVPSID_Timeout |
| D4: Discovery uses hostname pattern matching | TestMatchesHostnamePattern_* |
| D5: Discovery uses IP address as fallback | TestDiscoverVPSID_Success_ByIP |
| D6: VPSieNode transitions to Failed on timeout | TestDiscoverVPSID_Timeout returns timedOut=true |
| D9: Discovery works for concurrent VPSieNodes | TestIsNodeClaimedByOther_* |

## Notes

- Discoverer is implemented but not yet integrated into provisioning flow
- Phase 3 will integrate Discoverer into Provisioner and Controller
- All unit tests must pass before proceeding to Phase 3
