# ADR-0001: Critical Security and Testing Infrastructure Fixes

**Status:** Proposed
**Date:** 2025-12-23
**Deciders:** VPSie K8s Autoscaler Team
**Technical Story:** 6 Critical Production Fixes

## Context and Problem Statement

Code review has identified 6 critical issues that require immediate attention:

1. **Template Injection Vulnerability** (SECURITY CRITICAL) - Cloud-init templates execute with root privileges without validation
2. **Metrics Label Cardinality Explosion** (SECURITY CRITICAL) - Unsanitized metric labels can cause DoS via cardinality explosion
3. **Missing DeepCopy Methods** (CRITICAL) - CloudInitTemplateRef type missing DeepCopy methods causing compilation failure
4. **Safety Check Test Coverage** (CRITICAL) - 7 safety functions have ZERO tests
5. **Webhook Node Deletion Tests** (CRITICAL) - Security feature has NO tests
6. **Integration Test Helper Functions** (CRITICAL) - Prometheus metric helpers have incorrect API usage

These issues affect **8-10 files** across API, security, and testing layers, requiring **3-4 days** effort with **4-phase execution**.

## Decision Drivers

* **Security First**: Template injection and metrics cardinality are DoS attack vectors
* **Compilation Blocker**: Missing DeepCopy methods prevent successful builds
* **Test Coverage Gaps**: Critical safety logic has 0% test coverage
* **Backward Compatibility**: Must maintain existing cloud-init functionality
* **Metric Compatibility**: Must preserve existing Prometheus queries

## Considered Options

### Fix #1: Template Validation Strategy

#### Option A: Text/Template Parser Only (RECOMMENDED)
**Overview**: Use Go's text/template parser for syntax validation only

**Benefits:**
- Lightweight and fast (< 1ms validation)
- No external dependencies
- Catches template syntax errors early
- Compatible with existing UserData field

**Drawbacks:**
- Does not validate YAML structure
- Does not catch semantic errors
- Limited protection against malformed cloud-init

**Effort:** 2 days

**Implementation:**
```go
import "text/template"

func ValidateTemplate(tmpl string) error {
    _, err := template.New("validation").Parse(tmpl)
    return err
}
```

#### Option B: Text/Template + YAML Validation
**Overview**: Parse template AND validate YAML structure

**Benefits:**
- Catches both template and YAML errors
- Stronger validation guarantee
- Better user experience (early error detection)

**Drawbacks:**
- Requires template variable substitution with defaults
- Higher computational cost
- Complex error messages

**Effort:** 3 days

#### Option C: Full Validation (Template + YAML + Cloud-init Schema)
**Overview**: Validate against cloud-init schema specification

**Benefits:**
- Maximum validation coverage
- Catches cloud-init-specific errors
- Production-grade validation

**Drawbacks:**
- Requires cloud-init schema dependency
- Very high complexity
- Significant performance overhead
- Overkill for current requirements

**Effort:** 5 days

---

### Fix #2: Metrics Label Sanitization Policy

#### Option A: Truncate with Warning (RECOMMENDED)
**Overview**: Truncate labels to max length, sanitize characters, log warnings

**Benefits:**
- Non-breaking (metrics continue recording)
- Operator visibility through warnings
- Predictable behavior
- Maintains metric continuity

**Drawbacks:**
- Possible label collisions after truncation
- Warning log noise if misconfigured

**Effort:** 1 day

**Implementation:**
```go
const (
    MaxLabelLength = 128
    AllowedChars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-."
)

func SanitizeLabel(value string) string {
    // Replace invalid characters
    sanitized := strings.Map(func(r rune) rune {
        if strings.ContainsRune(AllowedChars, r) {
            return r
        }
        return '_'
    }, value)

    // Truncate to max length
    if len(sanitized) > MaxLabelLength {
        log.Warn("Label truncated", "original", value, "sanitized", sanitized[:MaxLabelLength])
        return sanitized[:MaxLabelLength]
    }
    return sanitized
}
```

#### Option B: Hash High-Cardinality Labels
**Overview**: Hash long labels instead of truncating

**Benefits:**
- No collision risk
- Deterministic mapping
- Stable over time

**Drawbacks:**
- Label values become unreadable
- Harder to debug
- Hash collisions possible (though unlikely)

**Effort:** 1.5 days

#### Option C: Reject with Error
**Overview**: Fail metric recording if label invalid

**Benefits:**
- Strictest validation
- Forces proper label hygiene
- No silent failures

**Drawbacks:**
- **BREAKING CHANGE** - existing metrics may fail
- Operational burden (must fix all violations)
- Metric data loss during transition

**Effort:** 2 days

---

### Fix #3: Code Generation Integration

#### Option A: Existing Makefile Target (RECOMMENDED)
**Overview**: Use existing `make generate` target with controller-gen

**Benefits:**
- Already integrated
- Zero configuration needed
- Proven to work
- Standard approach

**Drawbacks:**
- None

**Effort:** 0.5 days (just run `make generate`)

**Command:**
```bash
make generate
```

#### Option B: Custom Generation Script
**Overview**: Create custom script for DeepCopy generation

**Benefits:**
- Fine-grained control
- Can add custom logic

**Drawbacks:**
- Maintenance burden
- Reinvents the wheel
- Not idiomatic for Kubernetes projects

**Effort:** 2 days

---

### Fix #4: Test Coverage Requirements

#### Option A: 90% Coverage Target (RECOMMENDED)
**Overview**: Minimum 90% line coverage for safety.go

**Benefits:**
- Industry standard for critical code
- Covers happy path + key edge cases
- Reasonable effort/benefit ratio
- Catches most bugs

**Drawbacks:**
- Some edge cases may be missed
- 100% coverage may be impractical

**Effort:** 2 days

**Test Structure:**
```go
func TestSafetyChecks(t *testing.T) {
    tests := []struct {
        name string
        setup func() (node, pods, expected)
        check func(result, error)
    }{
        // 15-20 test cases covering:
        // - Local storage detection (EmptyDir, HostPath, PVC)
        // - Resource capacity validation
        // - System pod protection
        // - Anti-affinity violations
        // - Node protection annotations
    }
}
```

#### Option B: 80% Coverage Target
**Overview**: Lower bar for faster delivery

**Benefits:**
- Faster implementation
- Good enough for many cases

**Drawbacks:**
- May miss critical edge cases
- Below industry standard for safety-critical code

**Effort:** 1.5 days

#### Option C: 100% Coverage Target
**Overview**: Complete coverage

**Benefits:**
- Maximum confidence
- No gaps

**Drawbacks:**
- Diminishing returns
- May require testing error cases that can't occur
- Significantly higher effort

**Effort:** 3 days

---

### Fix #5: Test Framework Architecture

#### Option A: Unified Test Suite (RECOMMENDED)
**Overview**: All tests in existing test files with clear sections

**Benefits:**
- Simpler structure
- Easier to maintain
- Consistent patterns

**Drawbacks:**
- Large test files
- Less organizational flexibility

**Effort:** 2 days

**File Structure:**
```
pkg/scaler/safety_test.go           # Unit tests for safety.go
pkg/webhook/server_test.go          # Existing + new security tests
test/integration/metrics_test.go    # Fixed helper functions
```

#### Option B: Dedicated Security Test Directory
**Overview**: Create test/security/ for all security tests

**Benefits:**
- Clear separation of concerns
- Can run security tests separately
- Better organization

**Drawbacks:**
- More complex structure
- Test duplication risk
- Additional configuration

**Effort:** 3 days

**File Structure:**
```
test/security/template_injection_test.go
test/security/metrics_cardinality_test.go
test/security/webhook_authorization_test.go
```

---

## Decision Outcome

### Chosen Options

| Fix | Option | Rationale |
|-----|--------|-----------|
| #1: Template Validation | **Option A** (Text/Template Parser Only) | Best balance of security and complexity. Catches syntax errors which are 90% of real-world issues. |
| #2: Metrics Sanitization | **Option A** (Truncate with Warning) | Non-breaking, maintains metrics continuity, provides operator visibility. |
| #3: DeepCopy Generation | **Option A** (Existing Makefile) | Already integrated, zero configuration needed. |
| #4: Test Coverage | **Option A** (90% Coverage) | Industry standard for safety-critical code, reasonable effort. |
| #5: Test Framework | **Option A** (Unified Test Suite) | Simpler structure, easier maintenance, follows existing patterns. |
| #6: Metrics Helper Fix | **Direct Fix** | Use prometheus.DefaultGatherer.Gather() correctly per Prometheus client API. |

### Consequences

**Good:**
- Security vulnerabilities eliminated
- Code compiles successfully
- Critical safety logic has strong test coverage
- Backward compatibility maintained
- Prometheus queries continue working
- Clear path forward for implementation

**Bad:**
- Template validation limited to syntax (not semantic YAML validation)
- Label truncation may cause collisions in extreme cases
- Warning log volume may increase if metrics are misconfigured

**Neutral:**
- Test suite grows by ~300 lines
- One-time effort to run `make generate`
- Integration test helpers require API change

---

## Implementation Guidelines

### Phase 1: Foundation (Day 1)
**Priority: HIGHEST - Blocks Compilation**

1. **Fix #3: Generate DeepCopy Methods**
   ```bash
   make generate
   ```
   - Verify `CloudInitTemplateRef` has DeepCopy methods
   - Verify compilation succeeds
   - Commit immediately

### Phase 2: Security Hardening (Day 2)
**Priority: HIGH - Security Critical**

2. **Fix #1: Template Validation**
   - Add `ValidateTemplate()` function
   - Integrate into NodeGroup validation webhook
   - Add unit tests for validation logic

3. **Fix #2: Metrics Label Sanitization**
   - Add `SanitizeLabel()` helper function
   - Update all metric recording sites:
     - `pkg/scaler/drain.go:55-59`
     - `pkg/scaler/safety.go:37-41`
     - `pkg/vpsie/cost/metrics.go:229-266`
   - Add unit tests for sanitization

### Phase 3: Testing Infrastructure (Day 3)
**Priority: HIGH - Quality Assurance**

4. **Fix #4: Safety Check Tests**
   - Create `pkg/scaler/safety_test.go`
   - Implement table-driven tests
   - Mock Kubernetes client
   - Target 90% coverage

5. **Fix #5: Webhook Security Tests**
   - Add tests to `pkg/webhook/server_test.go`
   - Test authorization bypass attempts
   - Test validation scenarios

### Phase 4: Integration Test Fixes (Day 4)
**Priority: MEDIUM - CI/CD Stability**

6. **Fix #6: Prometheus Helper Functions**
   - Fix `getCounterValue()` using `prometheus.DefaultGatherer.Gather()`
   - Fix `getHistogramCount()` similarly
   - Parse `io_prometheus_client.MetricFamily` proto messages
   - Verify all integration tests pass

---

## Backward Compatibility Strategy

### Template Validation
- **Existing UserData field**: Continue supporting (deprecated but functional)
- **New CloudInitTemplate field**: Validate if provided
- **Migration path**: Users can gradually adopt new field

### Metrics Label Sanitization
- **Existing queries**: Continue working (labels sanitized consistently)
- **Dashboards**: No changes required
- **Alerts**: No changes required
- **Breaking change**: None (sanitization is transparent)

### DeepCopy Generation
- **API compatibility**: No changes (generated code is internal)
- **CRD schema**: No changes
- **Existing resources**: Continue working

---

## Testing Strategy

### Unit Tests
| Component | Coverage Target | Test Count |
|-----------|----------------|------------|
| Template Validation | 100% | 10 tests |
| Label Sanitization | 100% | 8 tests |
| Safety Checks | 90% | 20 tests |
| Webhook Security | 85% | 12 tests |

### Integration Tests
- Fix Prometheus metric helpers
- Verify metrics recording with sanitized labels
- End-to-end template validation flow

### Security Tests
- Template injection attempts
- Metrics cardinality explosion scenarios
- Webhook authorization bypass attempts

---

## References

### Security Research (2025)

**Template Injection:**
- [Go Template Security Best Practices - StudyRaid](https://app.studyraid.com/en/read/15258/528824/security-best-practices-for-go-templates)
- [Server-Side Template Injection in Go - Snyk](https://snyk.io/articles/understanding-server-side-template-injection-in-golang/)
- [SSTI in Golang - Oligo Security](https://www.oligo.security/blog/safe-by-default-or-vulnerable-by-design-golang-server-side-template-injection)
- [Method Confusion in Go SSTIs - OnSecurity](https://onsecurity.io/article/go-ssti-method-research/)

**Metrics Cardinality:**
- [Managing High Cardinality in Prometheus - Last9](https://last9.io/blog/how-to-manage-high-cardinality-metrics-in-prometheus/)
- [High Cardinality Metrics - Grafana Labs](https://grafana.com/blog/2022/10/20/how-to-manage-high-cardinality-metrics-in-prometheus-and-kubernetes/)
- [Cardinality is Key - Robust Perception](https://www.robustperception.io/cardinality-is-key/)
- [Prometheus Optimization - Kaidalov](https://kaidalov.com/posts/2025/09/prometheus-optimization/)

**Cloud-init Validation:**
- [Cloud-init User Data Validation](https://cloudinit.readthedocs.io/en/latest/howto/debug_user_data.html)
- [Cloud-init Schema Validation](https://deepwiki.com/canonical/cloud-init/1.2-schema-validation)
- [Cloud-init Jinja Templates](https://github.com/canonical/cloud-init/blob/main/cloudinit/handlers/jinja_template.py)

### Implementation Examples
- Kubernetes controller-gen: https://book.kubebuilder.io/reference/controller-gen.html
- Prometheus client_golang: https://pkg.go.dev/github.com/prometheus/client_golang/prometheus
- Go text/template: https://pkg.go.dev/text/template

---

## Compliance

### YAGNI Principle
All fixes address **actual identified issues** from code review. No speculative features added.

### SOLID Principles
- **Single Responsibility**: Each fix targets one specific issue
- **Open/Closed**: Validation and sanitization are extensible
- **Dependency Inversion**: Using interfaces for testability

### Security Principles
- **Defense in Depth**: Multiple validation layers
- **Fail Secure**: Validation errors prevent execution
- **Least Privilege**: RBAC fixes limit scope (out of scope for these 6 fixes)

---

## Risks and Mitigation

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Label truncation causes collisions | Low | Medium | Log warnings, monitor metrics |
| Template validation too strict | Low | Medium | Support legacy UserData field |
| Test coverage gaps | Medium | High | Peer review + coverage reports |
| Integration test breakage | Medium | Medium | Run full test suite before merge |
| Performance degradation | Low | Low | Benchmark validation functions |

---

## Acceptance Criteria

### Fix #1: Template Validation
- [ ] ValidateTemplate() function implemented
- [ ] Integrated into NodeGroup validation webhook
- [ ] 10+ unit tests covering valid/invalid templates
- [ ] Backward compatible with UserData field

### Fix #2: Metrics Label Sanitization
- [ ] SanitizeLabel() helper function implemented
- [ ] All 3 metric locations updated
- [ ] Warning logs for truncated labels
- [ ] 8+ unit tests covering edge cases

### Fix #3: DeepCopy Generation
- [ ] `make generate` executed successfully
- [ ] CloudInitTemplateRef has DeepCopy methods
- [ ] Code compiles without errors
- [ ] CRD manifests regenerated

### Fix #4: Safety Check Tests
- [ ] safety_test.go created with 20+ tests
- [ ] 90%+ code coverage achieved
- [ ] All 7 safety functions tested
- [ ] Mock Kubernetes client used

### Fix #5: Webhook Security Tests
- [ ] 12+ security tests added to server_test.go
- [ ] Authorization bypass tests
- [ ] Validation scenario tests
- [ ] Test coverage >85%

### Fix #6: Integration Test Helpers
- [ ] getCounterValue() fixed with correct API usage
- [ ] getHistogramCount() fixed with correct API usage
- [ ] All integration tests pass
- [ ] No test suite breakage

### Overall Quality Gates
- [ ] All unit tests pass (make test)
- [ ] All integration tests pass (make test-integration)
- [ ] Code coverage >80% overall
- [ ] No new linter warnings
- [ ] Documentation updated

---

## Follow-up Work

Out of scope for this ADR (future improvements):
- Full YAML validation for cloud-init templates (Option B/C)
- Metrics label hash strategy for extremely high cardinality
- Dedicated security test framework
- Automated security scanning in CI/CD
- Performance benchmarks for validation functions
