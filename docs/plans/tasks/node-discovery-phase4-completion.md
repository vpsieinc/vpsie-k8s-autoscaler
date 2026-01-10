# Phase 4 Completion: Quality Assurance

## Phase Overview

**Purpose**: Verify all acceptance criteria, ensure test coverage, and update documentation
**Tasks Included**: Quality assurance verification (no new code)

## Task Completion Checklist

- [ ] All Design Doc acceptance criteria verified (27 criteria)
- [ ] Full test suite passes with race detection
- [ ] Test coverage >= 80% for modified files
- [ ] Zero linting errors
- [ ] Build succeeds
- [ ] Documentation updated

## E2E Verification Procedures

### Full Acceptance Criteria Checklist

**VPS Discovery (D1-D9)**:
- [ ] D1: VPS ID discovered within 15 minutes of async provisioning
- [ ] D2: VPSieNode.Spec.VPSieInstanceID updated after discovery
- [ ] D3: VPSieNode.Spec.IPAddress updated after discovery
- [ ] D4: Discovery uses hostname pattern matching
- [ ] D5: Discovery uses IP address as fallback
- [ ] D6: VPSieNode transitions to Failed on timeout
- [ ] D7: Discovery completes within API timeout (30s)
- [ ] D8: Max 1 API call per VPSieNode per reconcile
- [ ] D9: Discovery works for concurrent VPSieNodes

**K8s Node Matching (M1-M4)**:
- [ ] M1: K8s node matched by IP address (primary)
- [ ] M2: K8s node matched by hostname (fallback)
- [ ] M3: VPSieNode.Status.NodeName updated after match
- [ ] M4: Matching works for nodes joined before discovery

**Label Application (L1-L5)**:
- [ ] L1: Label `autoscaler.vpsie.com/managed=true` applied
- [ ] L2: Label `autoscaler.vpsie.com/nodegroup` applied
- [ ] L3: Label `autoscaler.vpsie.com/vpsienode` applied
- [ ] L4: Labels applied atomically with optimistic locking
- [ ] L5: Conflict handling with retry

**NodeGroup Status (S1-S3)**:
- [ ] S1: CurrentNodes reflects actual VPSieNode count
- [ ] S2: ReadyNodes reflects Ready phase VPSieNodes
- [ ] S3: Status updates after discovery completes

**Error Handling (E1-E4)**:
- [ ] E1: API errors logged with context
- [ ] E2: Transient errors trigger retry
- [ ] E3: Timeout errors mark VPSieNode Failed
- [ ] E4: Failed VPSieNodes require manual cleanup

### Quality Check Commands

```bash
# Full test suite with race detection
make test

# Coverage report for vpsienode controller
go test ./pkg/controller/vpsienode/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep -E "discoverer|provisioner|joiner"

# Target: >= 80% coverage

# Linting
make lint

# Code formatting
make fmt

# Build verification
make build
```

### Documentation Update Check

- [ ] `CLAUDE.md` updated if architecture changes warrant
- [ ] `docs/architecture/overview.md` updated with discovery flow (if applicable)
- [ ] Inline code documentation complete (godoc comments)

## Phase Completion Criteria

- [ ] All 27 acceptance criteria from Design Doc verified
- [ ] Test coverage >= 80% for modified files
- [ ] Zero linting errors
- [ ] Build succeeds
- [ ] Documentation updated (if needed)

## Final Verification Script

```bash
#!/bin/bash
set -e

echo "=== Phase 4: Quality Assurance Verification ==="

echo "1. Build verification..."
make build

echo "2. Lint verification..."
make lint

echo "3. Format check..."
make fmt

echo "4. Full test suite with race detection..."
make test

echo "5. Coverage check..."
go test ./pkg/controller/vpsienode/... -coverprofile=coverage.out
go tool cover -func=coverage.out | grep -E "discoverer|provisioner|joiner" | while read line; do
    coverage=$(echo $line | awk '{print $NF}' | tr -d '%')
    if (( $(echo "$coverage < 80" | bc -l) )); then
        echo "WARNING: Coverage below 80%: $line"
    fi
done

echo "=== All checks passed ==="
```

## Notes

- This phase is verification only - no new code should be written
- If any criteria fail, return to relevant task and fix
- Document any deviations from Design Doc in this file
- All verifications must pass before considering implementation complete
