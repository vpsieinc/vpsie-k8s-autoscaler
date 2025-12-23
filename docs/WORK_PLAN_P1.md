# Work Plan: P1 Nice-to-Have Features Implementation

**Document Version:** 1.0
**Date:** 2025-12-22
**Status:** READY FOR IMPLEMENTATION
**Total Effort:** 50-55 hours (adjusted from ADR review)
**Timeline:** 2 sprints (10 working days)

---

## Executive Summary

This work plan provides a detailed task breakdown for implementing 9 P1 (High Priority, Low Effort) features that enhance operational excellence, developer experience, and production readiness of the VPSie Kubernetes Autoscaler.

**Key Principles:**
- **Test-Driven Development:** Integration tests created alongside implementation
- **Atomic Tasks:** Each task is 2-4 hours, independently testable
- **Sequential Dependencies:** Clear task ordering to prevent blocking
- **Backward Compatibility:** All changes preserve existing functionality
- **Incremental Delivery:** Features can be merged independently

**Priority Order (Dependency-Driven):**
1. **P1.9 - Missing Metrics** (Foundation for P1.1) - FIRST
2. **P1.1 - Grafana Dashboard** (Uses metrics from P1.9)
3. **P1.2 - Prometheus Alerts** (Uses metrics from P1.9)
4. **P1.3 - Cloud-Init Templates**
5. **P1.4 - SSH Key Management**
6. **P1.8 - Sample Storage Optimization**
7. **P1.5 - Configuration Consolidation**
8. **P1.6 - Documentation Reorganization**
9. **P1.7 - Script Consolidation**

---

## Table of Contents

1. [Sprint Organization](#sprint-organization)
2. [Task Breakdown by Feature](#task-breakdown-by-feature)
3. [Gantt Timeline](#gantt-timeline)
4. [Dependencies Matrix](#dependencies-matrix)
5. [Testing Strategy](#testing-strategy)
6. [Rollback Procedures](#rollback-procedures)
7. [Quality Gates](#quality-gates)
8. [Task Reference](#task-reference)

---

## Sprint Organization

### Sprint 1 (Week 1) - Core Features
**Goal:** Observability, provisioning flexibility, and performance
**Duration:** 5 days (40 hours)
**Deliverables:**
- Complete observability stack (metrics, dashboard, alerts)
- Flexible node provisioning (cloud-init, SSH keys)
- Memory optimization

**Features:**
- P1.9 - Missing Metrics (5h)
- P1.1 - Grafana Dashboard (6h)
- P1.2 - Prometheus Alerts (4h)
- P1.3 - Cloud-Init Templates (6h)
- P1.4 - SSH Key Management (4h)
- P1.8 - Sample Storage Optimization (4h)
- Integration testing & documentation (11h)

### Sprint 2 (Week 2) - Code Quality & Documentation
**Goal:** Maintainability, developer experience, consistency
**Duration:** 5 days (40 hours)
**Deliverables:**
- Centralized configuration
- Organized documentation
- Consolidated scripts

**Features:**
- P1.5 - Configuration Consolidation (7h)
- P1.6 - Documentation Reorganization (6h)
- P1.7 - Script Consolidation (3h)
- Final integration testing (8h)
- Release preparation (6h)

---

## Task Breakdown by Feature

### P1.9 - Missing Metrics (5 hours) - PRIORITY 1

**Objective:** Add 4 missing metrics for complete observability before dashboard/alerts creation

#### P1.9-T1: Define New Metrics (1h)
**Description:** Add metric definitions to pkg/metrics/metrics.go

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/metrics/metrics.go`

**Changes:**
```go
// Add 4 new metrics:
// 1. ScaleDownBlockedTotal - Counter for blocked scale-down operations
// 2. SafetyCheckFailuresTotal - Counter for safety check failures
// 3. NodeDrainDuration - Histogram for drain duration
// 4. NodeDrainPodsEvicted - Histogram for pods evicted during drain
```

**Dependencies:** None

**Acceptance Criteria:**
- [ ] 4 new metric variables defined
- [ ] Proper labels defined (nodegroup, namespace, reason, status, check_type)
- [ ] Appropriate metric types (Counter, Histogram)
- [ ] Buckets configured for histograms
- [ ] Metrics registered in init() function
- [ ] go fmt passes
- [ ] go vet passes

**Testing:** Unit test for metric registration

**Estimated Effort:** 1 hour

---

#### P1.9-T2: Create Metric Recorder Functions (1h)
**Description:** Add helper functions in pkg/metrics/recorder.go

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/metrics/recorder.go`

**Changes:**
```go
// Add recorder functions:
// - RecordScaleDownBlocked(nodeGroup, namespace, reason string)
// - RecordSafetyCheckFailure(checkType, nodeGroup, namespace string)
// - RecordNodeDrain(nodeGroup, namespace, status string, duration float64, podCount int)
```

**Dependencies:** P1.9-T1

**Acceptance Criteria:**
- [ ] 3 new recorder functions implemented
- [ ] Functions properly increment/observe metrics
- [ ] Error handling for invalid inputs
- [ ] Unit tests for each recorder function
- [ ] Code coverage > 80%

**Testing:** Unit tests in recorder_test.go

**Estimated Effort:** 1 hour

---

#### P1.9-T3: Instrument ScaleDownManager (1.5h)
**Description:** Add metric calls to pkg/scaler/scaler.go

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/scaler.go`

**Changes:**
- Record ScaleDownBlockedTotal when scale-down is prevented
- Record SafetyCheckFailuresTotal categorized by check type
- Add metric calls in validateSafeRemoval(), ScaleDown()

**Dependencies:** P1.9-T2

**Acceptance Criteria:**
- [ ] Metrics recorded for all blocked scale-downs
- [ ] Safety check failures categorized (PDB, local storage, maintenance window, cooldown, cluster health)
- [ ] Existing tests still pass
- [ ] New integration test validates metrics increment

**Testing:**
- Unit tests: Verify metric calls with mock recorder
- Integration test: test/integration/metrics_test.go

**Estimated Effort:** 1.5 hours

---

#### P1.9-T4: Instrument Node Drainer (1h)
**Description:** Add drain metrics to pkg/scaler/drain.go

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/drain.go`

**Changes:**
- Measure drain duration with timer
- Count evicted pods
- Record NodeDrainDuration and NodeDrainPodsEvicted

**Dependencies:** P1.9-T2

**Acceptance Criteria:**
- [ ] Drain start/end time tracked
- [ ] Pod eviction count tracked
- [ ] Metrics recorded with success/failure status
- [ ] Existing drain tests pass
- [ ] New test validates drain metrics

**Testing:**
- Unit test: Verify drain metrics in drain_test.go
- Integration test: test/integration/metrics_test.go

**Estimated Effort:** 1 hour

---

#### P1.9-T5: Integration Test for Missing Metrics (0.5h)
**Description:** Create integration test validating new metrics

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/metrics_test.go`

**Test Cases:**
1. TestMetrics_ScaleDownBlocked - Trigger blocked scale-down, verify counter
2. TestMetrics_SafetyCheckFailures - Trigger safety check failures, verify counters
3. TestMetrics_NodeDrain - Drain node, verify duration and pod count histograms
4. TestMetrics_Registration - Verify all 4 metrics are registered

**Dependencies:** P1.9-T3, P1.9-T4

**Acceptance Criteria:**
- [ ] All 4 test cases pass
- [ ] Metrics scrape endpoint returns new metrics
- [ ] Metric labels are correct
- [ ] Test runs in <30 seconds

**Testing:** Run with `make test-integration`

**Estimated Effort:** 0.5 hours

---

### P1.1 - Grafana Dashboard Template (6 hours) - PRIORITY 2

**Objective:** Create production-ready Grafana dashboard using all 42 metrics (38 existing + 4 from P1.9)

#### P1.1-T1: Create Dashboard JSON Structure (1.5h)
**Description:** Create base Grafana dashboard JSON with layout and variables

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/deploy/grafana/autoscaler-dashboard.json`

**Structure:**
- Dashboard metadata (title, UID, version)
- Template variables (namespace, nodegroup, time range)
- 10 panel rows
- Annotations configuration

**Dependencies:** P1.9-T5 (metrics must exist)

**Acceptance Criteria:**
- [ ] Valid Grafana 9.0+ JSON format
- [ ] Unique dashboard UID: vpsie-autoscaler
- [ ] Variables: namespace, nodegroup (multi-select)
- [ ] Time picker enabled
- [ ] Auto-refresh: 30s
- [ ] JSON validates with `promtool`

**Testing:** Manual - import into Grafana

**Estimated Effort:** 1.5 hours

---

#### P1.1-T2: Create Dashboard Panels - Rows 1-3 (1.5h)
**Description:** Implement panels for NodeGroup overview, scaling activity, provisioning performance

**Panels:**
1. **Row 1:** NodeGroup Overview (4 gauges)
   - Current Nodes (nodegroup_current_nodes)
   - Desired Nodes (nodegroup_desired_nodes)
   - Ready Nodes (nodegroup_ready_nodes)
   - Min/Max Limits (nodegroup_min_nodes, nodegroup_max_nodes)

2. **Row 2:** Scaling Activity (time series)
   - Scale Up Total (rate(scale_up_total))
   - Scale Down Total (rate(scale_down_total))

3. **Row 3:** Provisioning Performance (2 panels)
   - Provisioning Duration (P50/P95/P99 from node_provisioning_duration_seconds)
   - Provisioning Heatmap (heatmap of node_provisioning_duration_seconds)

**Dependencies:** P1.1-T1

**Acceptance Criteria:**
- [ ] 7 panels render without errors
- [ ] PromQL queries are correct
- [ ] Panels respond to variable changes
- [ ] Legends are clear and descriptive
- [ ] Color schemes are theme-compatible

**Testing:** Manual - verify in Grafana

**Estimated Effort:** 1.5 hours

---

#### P1.1-T3: Create Dashboard Panels - Rows 4-7 (1.5h)
**Description:** Implement panels for API health, controller performance, node phases, cost tracking

**Panels:**
4. **Row 4:** API & Controller Health (2 graphs)
   - VPSie API Metrics (request rate, error rate, latency)
   - Controller Performance (reconcile duration, error rate)

5. **Row 5:** VPSieNode Phase Distribution (stacked area)
   - Phase counts over time (vpsienode_phase)

6. **Row 6:** Unschedulable Pods & Safety (2 panels)
   - Pending Pods by Constraint (bar chart)
   - Scale-Down Blocked by Reason (table) ← NEW metric from P1.9

7. **Row 7:** Cost Tracking (2 panels)
   - Monthly Cost (gauge + trend)
   - Savings Opportunities (stat)

**Dependencies:** P1.1-T2

**Acceptance Criteria:**
- [ ] All 8 panels functional
- [ ] New P1.9 metrics utilized in Panel 6
- [ ] Phase 5 cost metrics displayed
- [ ] Stacked area chart shows all phases
- [ ] Table sorts by count descending

**Testing:** Manual - verify all panels show data

**Estimated Effort:** 1.5 hours

---

#### P1.1-T4: Add Annotations and Documentation (1h)
**Description:** Configure annotations and create setup documentation

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/deploy/grafana/README.md`
- `/Users/zozo/projects/vpsie-k8s-autoscaler/docs/operations/grafana-setup.md`

**Annotations:**
- Scale events (changes in nodegroup_desired_nodes)
- Rebalance events (from rebalancer metrics)

**Documentation:**
- Import instructions
- Variable configuration
- Customization guide
- Troubleshooting

**Dependencies:** P1.1-T3

**Acceptance Criteria:**
- [ ] Annotations appear on dashboard
- [ ] README.md with import steps
- [ ] grafana-setup.md with screenshots
- [ ] All links are valid
- [ ] Documentation tested by following steps

**Testing:** Manual - follow documentation to import

**Estimated Effort:** 1 hour

---

#### P1.1-T5: Dashboard Screenshot and Validation (0.5h)
**Description:** Capture screenshots and validate dashboard

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/docs/operations/screenshots/grafana-dashboard.png`

**Tasks:**
- Import dashboard into test Grafana instance
- Verify all 10 panels render
- Test variable filtering
- Capture screenshot for documentation
- Test in light and dark themes

**Dependencies:** P1.1-T4

**Acceptance Criteria:**
- [ ] Dashboard imports successfully
- [ ] All panels show data (or "No data" with explanation)
- [ ] Variables work correctly
- [ ] Screenshot saved at 1920x1080
- [ ] Both themes tested

**Testing:** Manual validation checklist from TEST_SPECS_P1.md MT-1.1

**Estimated Effort:** 0.5 hours

---

### P1.2 - Prometheus Alert Rules (4 hours) - PRIORITY 3

**Objective:** Create 12 production-ready alert rules (4 critical, 8 warning) with runbooks

#### P1.2-T1: Create Alert Rules YAML (1.5h)
**Description:** Define 12 alert rules in Prometheus format

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/deploy/prometheus/alerts.yaml`

**Alert Rules:**

**Critical (4):**
1. HighVPSieAPIErrorRate - API errors > 10% for 5m
2. ControllerDown - No reconciliation for 10m
3. NodeGroupStuckScaling - At max capacity with pending pods for 30m
4. HighControllerErrorRate - Controller errors > 5% for 10m

**Warning (8):**
5. SlowNodeProvisioning - P95 duration > 10 minutes
6. HighNodeProvisioningFailureRate - Failures > 20% for 15m
7. StaleNodeGroupMetrics - Metrics not updated for 15m
8. UnschedulablePodsAccumulating - >10 pending pods for 15m
9. HighDrainFailureRate - Drain failures > 10% for 10m
10. FrequentRebalancing - >3 rebalances in 1h
11. CostSavingsNotRealized - Savings >20% available for 24h
12. HighMemoryUsage - Controller memory >500MB for 10m

**Dependencies:** P1.9-T5 (new metrics available)

**Acceptance Criteria:**
- [ ] All 12 alerts defined
- [ ] Labels: severity, component
- [ ] Annotations: summary, description, runbook_url, dashboard_url
- [ ] For durations appropriate to alert type
- [ ] Valid Prometheus PromQL expressions
- [ ] `promtool check rules` passes

**Testing:** `promtool check rules alerts.yaml`

**Estimated Effort:** 1.5 hours

---

#### P1.2-T2: Create Runbooks (1.5h)
**Description:** Write troubleshooting runbooks for all 12 alerts

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/docs/operations/runbooks.md`

**Runbook Structure (per alert):**
1. Alert Name and Severity
2. Problem Description
3. Impact Assessment
4. Diagnosis Steps
5. Resolution Steps
6. Prevention Guidance
7. Escalation Path

**Dependencies:** P1.2-T1

**Acceptance Criteria:**
- [ ] Runbook for each of 12 alerts
- [ ] Clear step-by-step instructions
- [ ] Example commands included
- [ ] Links to relevant documentation
- [ ] Markdown formatting
- [ ] TOC for navigation

**Testing:** Manual review - follow runbook steps

**Estimated Effort:** 1.5 hours

---

#### P1.2-T3: Create Alerting Guide (0.5h)
**Description:** Document how to configure and customize alerts

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/docs/operations/alerting-guide.md`

**Content:**
- Alert overview and philosophy
- Installation instructions
- Alertmanager integration
- Alert routing configuration
- Customization examples
- Testing alerts
- Silencing and inhibition

**Dependencies:** P1.2-T1

**Acceptance Criteria:**
- [ ] Step-by-step setup guide
- [ ] Alertmanager configuration example
- [ ] Example routes for different severities
- [ ] Test procedure documented
- [ ] Customization examples

**Testing:** Follow guide to set up alerts

**Estimated Effort:** 0.5 hours

---

#### P1.2-T4: Validate Alerts with promtool (0.5h)
**Description:** Test alert rules and create test cases

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/deploy/prometheus/alerts-test.yaml`

**Tasks:**
- Validate syntax with promtool
- Create unit tests for alert expressions
- Document expected firing conditions
- Test alert routing (if Alertmanager available)

**Dependencies:** P1.2-T1

**Acceptance Criteria:**
- [ ] `promtool check rules` passes with 0 errors
- [ ] `promtool test rules` passes (if tests defined)
- [ ] All PromQL expressions valid
- [ ] No duplicate alert names
- [ ] All annotations populated

**Testing:** Automated with promtool

**Estimated Effort:** 0.5 hours

---

### P1.3 - Cloud-Init Template Configuration (6 hours) - PRIORITY 4

**Objective:** Make cloud-init templates configurable per NodeGroup

#### P1.3-T1: Add CRD Fields for Cloud-Init (1h)
**Description:** Extend NodeGroupSpec with cloud-init configuration

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1/nodegroup_types.go`

**Changes:**
```go
type NodeGroupSpec struct {
    // ... existing fields ...

    // CloudInit configuration
    CloudInitTemplate string `json:"cloudInitTemplate,omitempty"`
    CloudInitVariables map[string]string `json:"cloudInitVariables,omitempty"`
    CloudInitTemplateRef *CloudInitTemplateRef `json:"cloudInitTemplateRef,omitempty"`
}

type CloudInitTemplateRef struct {
    Name string `json:"name"` // ConfigMap name
    Key  string `json:"key"`  // Key in ConfigMap
}
```

**Dependencies:** None

**Acceptance Criteria:**
- [ ] Fields added to NodeGroupSpec
- [ ] JSON tags correct
- [ ] Optional fields (omitempty)
- [ ] Comments added
- [ ] CRD validation added (maxLength for template)
- [ ] `make generate` runs successfully
- [ ] CRD manifests regenerated

**Testing:** Unit test for CRD validation

**Estimated Effort:** 1 hour

---

#### P1.3-T2: Implement Template Engine (2h)
**Description:** Add template rendering logic to provisioner

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/controller/vpsienode/provisioner.go`

**Changes:**
- Import text/template
- Create generateCloudInit() function
- Support inline templates (CloudInitTemplate)
- Support ConfigMap references (CloudInitTemplateRef)
- Merge built-in variables with custom variables
- Template validation

**Built-in Variables:**
- ClusterEndpoint
- JoinToken
- CACertHash
- NodeName
- NodeGroupName
- DatacenterID

**Dependencies:** P1.3-T1

**Acceptance Criteria:**
- [ ] Template parsing and execution works
- [ ] ConfigMap loading works
- [ ] Variable merging works (custom overrides built-in)
- [ ] Template errors are caught and reported
- [ ] Default template used if none specified
- [ ] Unit tests for template engine
- [ ] Code coverage > 80%

**Testing:** Unit tests in provisioner_test.go

**Estimated Effort:** 2 hours

---

#### P1.3-T3: Create Example Templates (1h)
**Description:** Provide example cloud-init templates

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/deploy/examples/cloud-init/gpu-node-template.yaml`
- `/Users/zozo/projects/vpsie-k8s-autoscaler/deploy/examples/cloud-init/arm64-node-template.yaml`
- `/Users/zozo/projects/vpsie-k8s-autoscaler/deploy/examples/cloud-init/custom-packages-template.yaml`
- `/Users/zozo/projects/vpsie-k8s-autoscaler/deploy/examples/nodegroup-with-cloudinit.yaml`

**Templates:**
1. GPU node - Install NVIDIA drivers, CUDA toolkit
2. ARM64 node - ARM-specific package setup
3. Custom packages - Install additional packages (monitoring agents, etc.)

**Dependencies:** P1.3-T2

**Acceptance Criteria:**
- [ ] 3 example templates created
- [ ] Templates use template variables correctly
- [ ] Example NodeGroup CRD with template reference
- [ ] ConfigMap example for template storage
- [ ] All examples tested (syntax valid)

**Testing:** Manual - apply examples to test cluster

**Estimated Effort:** 1 hour

---

#### P1.3-T4: Integration Test for Cloud-Init (1.5h)
**Description:** Create integration test for cloud-init rendering

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/cloudinit_template_test.go`

**Test Cases:**
1. TestCloudInit_InlineTemplate - Render inline template
2. TestCloudInit_ConfigMapReference - Load template from ConfigMap
3. TestCloudInit_VariableSubstitution - Verify variables are substituted
4. TestCloudInit_DefaultTemplate - Use default when none specified
5. TestCloudInit_InvalidTemplate - Handle template errors gracefully

**Dependencies:** P1.3-T2

**Acceptance Criteria:**
- [ ] All 5 test cases pass
- [ ] Test creates ConfigMap with template
- [ ] Test verifies rendered cloud-init contains variables
- [ ] Test cleans up resources
- [ ] Test runs in <60 seconds

**Testing:** Run with `make test-integration`

**Estimated Effort:** 1.5 hours

---

#### P1.3-T5: Documentation for Cloud-Init (0.5h)
**Description:** Document cloud-init template feature

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/docs/configuration/cloud-init.md`

**Content:**
- Feature overview
- Template syntax (Go text/template)
- Available variables
- Inline template example
- ConfigMap reference example
- Best practices
- Troubleshooting

**Dependencies:** P1.3-T3

**Acceptance Criteria:**
- [ ] Clear explanation of feature
- [ ] All variables documented
- [ ] 3+ examples
- [ ] Troubleshooting section
- [ ] Links to example templates

**Testing:** Manual review

**Estimated Effort:** 0.5 hours

---

### P1.4 - SSH Key Management (4 hours) - PRIORITY 5

**Objective:** Enable SSH key injection into provisioned nodes

#### P1.4-T1: Add CRD Fields for SSH Keys (0.5h)
**Description:** Extend NodeGroupSpec with SSH key configuration

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1/nodegroup_types.go`

**Changes:**
```go
type NodeGroupSpec struct {
    // ... existing fields ...

    // SSH key configuration
    SSHKeyIDs []string `json:"sshKeyIds,omitempty"`
    SSHPublicKeys []string `json:"sshPublicKeys,omitempty"`
    SSHKeySecretRef *SSHKeySecretRef `json:"sshKeySecretRef,omitempty"`
}

type SSHKeySecretRef struct {
    Name string `json:"name"` // Secret name
    Keys []string `json:"keys"` // Keys within secret
}
```

**Dependencies:** None

**Acceptance Criteria:**
- [ ] Fields added to NodeGroupSpec
- [ ] JSON tags correct
- [ ] CRD validation (maxItems)
- [ ] `make generate` succeeds
- [ ] CRD manifests updated

**Testing:** Unit test for CRD validation

**Estimated Effort:** 0.5 hours

---

#### P1.4-T2: Add Controller Options for Global SSH Keys (0.5h)
**Description:** Add global SSH key configuration to controller

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/cmd/controller/main.go`
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/controller/nodegroup/controller.go`

**Changes:**
```go
type ControllerOptions struct {
    // ... existing fields ...

    // Global SSH keys applied to all nodes
    DefaultSSHKeyIDs []string
}
```

**Command-line flags:**
- `--default-ssh-key-ids` (comma-separated list)

**Dependencies:** None

**Acceptance Criteria:**
- [ ] DefaultSSHKeyIDs field added
- [ ] CLI flag added
- [ ] Flag parsing works
- [ ] Help text updated
- [ ] Default value: empty list

**Testing:** Unit test for flag parsing

**Estimated Effort:** 0.5 hours

---

#### P1.4-T3: Implement SSH Key Collection (1.5h)
**Description:** Collect SSH keys from multiple sources and merge

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/controller/vpsienode/provisioner.go`

**Changes:**
- Load SSH keys from Secret (if SSHKeySecretRef specified)
- Merge keys: global + NodeGroup-specific + secret-based
- Deduplicate key IDs
- Add to VPSie VM creation request

**Dependencies:** P1.4-T1, P1.4-T2

**Acceptance Criteria:**
- [ ] Secret loading works (handles missing secrets gracefully)
- [ ] Key merging works (global + NodeGroup + secret)
- [ ] Deduplication works
- [ ] Keys passed to VPSie API client
- [ ] Unit tests for key collection logic
- [ ] Code coverage > 80%

**Testing:** Unit tests in provisioner_test.go

**Estimated Effort:** 1.5 hours

---

#### P1.4-T4: Update VPSie Client for SSH Keys (0.5h)
**Description:** Add SSH key fields to VPSie API client

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/vpsie/client/types.go`

**Changes:**
```go
type VMCreateRequest struct {
    // ... existing fields ...

    SSHKeyIDs     []string `json:"ssh_key_ids,omitempty"`
    SSHPublicKeys []string `json:"ssh_public_keys,omitempty"`
}
```

**Dependencies:** None

**Acceptance Criteria:**
- [ ] Fields added to VMCreateRequest
- [ ] JSON tags match VPSie API expectations
- [ ] Fields properly serialized in API calls
- [ ] Existing tests still pass

**Testing:** Unit test for VMCreateRequest serialization

**Estimated Effort:** 0.5 hours

---

#### P1.4-T5: Integration Test for SSH Keys (1h)
**Description:** Create integration test for SSH key injection

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/ssh_key_test.go`

**Test Cases:**
1. TestSSHKeys_GlobalKeys - Verify global keys applied to all nodes
2. TestSSHKeys_NodeGroupKeys - Verify NodeGroup-specific keys
3. TestSSHKeys_SecretReference - Load keys from Kubernetes secret
4. TestSSHKeys_Merging - Verify all sources merged correctly
5. TestSSHKeys_Deduplication - Verify duplicate IDs removed

**Dependencies:** P1.4-T3, P1.4-T4

**Acceptance Criteria:**
- [ ] All 5 test cases pass
- [ ] Test creates Secret with SSH keys
- [ ] Test verifies VPSie API receives correct keys
- [ ] Test cleans up resources
- [ ] Test runs in <60 seconds

**Testing:** Run with `make test-integration`

**Estimated Effort:** 1 hour

---

### P1.8 - Sample Storage Optimization (4 hours) - PRIORITY 6

**Objective:** Optimize memory usage in utilization sample storage using circular buffer

#### P1.8-T1: Implement Circular Buffer (1.5h)
**Description:** Replace slice-based sample storage with circular buffer

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/utilization.go`

**Changes:**
- Modify NodeUtilization struct to use circular buffer
- Implement AddSample() with circular buffer logic
- Implement GetSamples() to return samples in correct order
- Set maxSamples based on retention period (default 12 samples for 1h)

**Dependencies:** None

**Acceptance Criteria:**
- [ ] Circular buffer implemented correctly
- [ ] No slice allocations after warmup
- [ ] Sample order preserved (oldest to newest)
- [ ] Memory usage constant after warmup
- [ ] Existing tests pass
- [ ] Unit tests for circular buffer logic
- [ ] Code coverage > 90%

**Testing:** Unit tests in utilization_test.go

**Estimated Effort:** 1.5 hours

---

#### P1.8-T2: Create Memory Pool (0.5h)
**Description:** Add sync.Pool for sample slice reuse

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/pool.go`

**Changes:**
```go
var samplePool = sync.Pool{
    New: func() interface{} {
        return make([]UtilizationSample, 0, 12)
    },
}

func getSampleSlice() []UtilizationSample
func putSampleSlice(s []UtilizationSample)
```

**Dependencies:** P1.8-T1

**Acceptance Criteria:**
- [ ] Pool created with appropriate capacity
- [ ] Get/Put functions work correctly
- [ ] Slices are reset before returning to pool
- [ ] Unit test validates pool behavior

**Testing:** Unit test in pool_test.go

**Estimated Effort:** 0.5 hours

---

#### P1.8-T3: Create Benchmarks (1h)
**Description:** Benchmark memory usage before/after optimization

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/utilization_bench_test.go`

**Benchmarks:**
1. BenchmarkAddSample_SliceAllocation (baseline)
2. BenchmarkAddSample_CircularBuffer (optimized)
3. BenchmarkGetSamples_SliceAllocation (baseline)
4. BenchmarkGetSamples_CircularBuffer (optimized)
5. BenchmarkMemoryGrowth_1Hour (simulate 1 hour of samples)

**Dependencies:** P1.8-T1, P1.8-T2

**Acceptance Criteria:**
- [ ] Benchmarks run successfully
- [ ] Memory allocation reduced by >50%
- [ ] CPU performance impact <5%
- [ ] Benchmark results documented
- [ ] go test -bench passes

**Testing:** Run with `go test -bench=. -benchmem`

**Estimated Effort:** 1 hour

---

#### P1.8-T4: Performance Validation (1h)
**Description:** Run performance tests and document results

**Tasks:**
- Run benchmarks on representative workload
- Profile memory usage with pprof
- Document before/after metrics
- Verify no regressions in other areas

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/docs/performance/sample-storage-optimization.md`

**Dependencies:** P1.8-T3

**Acceptance Criteria:**
- [ ] Benchmark results show >50% memory reduction
- [ ] pprof confirms reduced allocations
- [ ] No CPU performance regression
- [ ] All existing tests pass
- [ ] Performance report created

**Testing:** Performance benchmarks + memory profiling

**Estimated Effort:** 1 hour

---

### P1.5 - Configuration Package Consolidation (7 hours) - PRIORITY 7

**Objective:** Centralize configuration management into internal/config package

#### P1.5-T1: Create Config Package Structure (1h)
**Description:** Create centralized configuration package

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/internal/config/config.go`
- `/Users/zozo/projects/vpsie-k8s-autoscaler/internal/config/defaults.go`
- `/Users/zozo/projects/vpsie-k8s-autoscaler/internal/config/validation.go`

**Structure:**
```go
type Config struct {
    Controller ControllerConfig
    VPSie      VPSieConfig
    Metrics    MetricsConfig
    Health     HealthConfig
    Logging    LoggingConfig
    Features   FeatureFlags
}
```

**Dependencies:** None

**Acceptance Criteria:**
- [ ] Config structs defined
- [ ] Default values function
- [ ] Validation function
- [ ] YAML/JSON tags
- [ ] Comments for all fields

**Testing:** Unit test for config structure

**Estimated Effort:** 1 hour

---

#### P1.5-T2: Implement Config Loaders (1.5h)
**Description:** Load configuration from multiple sources

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/internal/config/config.go`

**Loaders:**
1. LoadFromFile(path string) - Load from YAML file
2. LoadFromEnv() - Load from environment variables
3. LoadFromFlags(cmd *cobra.Command) - Load from CLI flags
4. Load() - Merge all sources (file → env → flags)

**Priority:** Flags > Env > File > Defaults

**Dependencies:** P1.5-T1

**Acceptance Criteria:**
- [ ] All loaders implemented
- [ ] Priority order correct
- [ ] Validation on load
- [ ] Clear error messages
- [ ] Unit tests for each loader
- [ ] Code coverage > 85%

**Testing:** Unit tests in config_test.go

**Estimated Effort:** 1.5 hours

---

#### P1.5-T3: Create Example Config File (0.5h)
**Description:** Create example configuration file

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/config/config.example.yaml`

**Content:** All configuration options with comments

**Dependencies:** P1.5-T1

**Acceptance Criteria:**
- [ ] All fields documented
- [ ] Default values shown
- [ ] Examples for common scenarios
- [ ] Valid YAML syntax
- [ ] Loads successfully with LoadFromFile()

**Testing:** Manual - load and validate example config

**Estimated Effort:** 0.5 hours

---

#### P1.5-T4: Update Controller to Use Config Package (2h)
**Description:** Refactor main.go to use centralized config

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/cmd/controller/main.go`

**Changes:**
- Replace individual flags with config.Load()
- Pass Config to controllers
- Remove duplicated configuration code
- Add --config-file flag

**Dependencies:** P1.5-T2

**Acceptance Criteria:**
- [ ] All flags mapped to config fields
- [ ] Config file support works
- [ ] Environment variable support works
- [ ] Backward compatible with existing flags
- [ ] Help text updated
- [ ] Existing tests pass

**Testing:** Integration test for config loading

**Estimated Effort:** 2 hours

---

#### P1.5-T5: Consolidate Logging Packages (1.5h)
**Description:** Remove duplicate logging packages

**Tasks:**
1. Move pkg/logging/ to internal/logging/
2. Remove pkg/log/ (unused)
3. Update all imports
4. Ensure consistency

**Files Modified:**
- All files importing pkg/logging or pkg/log

**Files Removed:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/log/` (entire directory)

**Files Moved:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/logging/` → `internal/logging/`

**Dependencies:** None (can run in parallel with P1.5-T4)

**Acceptance Criteria:**
- [ ] pkg/log/ deleted
- [ ] pkg/logging/ moved to internal/logging/
- [ ] All imports updated
- [ ] All tests pass
- [ ] No compilation errors
- [ ] go fmt passes

**Testing:** Full test suite

**Estimated Effort:** 1.5 hours

---

#### P1.5-T6: Documentation for Configuration (0.5h)
**Description:** Document configuration system

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/docs/configuration/config-reference.md`

**Content:**
- Configuration overview
- Loading order (defaults → file → env → flags)
- All configuration fields reference
- Environment variable names
- Example configurations
- Migration guide from old flags

**Dependencies:** P1.5-T4

**Acceptance Criteria:**
- [ ] All fields documented
- [ ] Environment variable names listed
- [ ] Migration guide for existing users
- [ ] Examples for common scenarios

**Testing:** Manual review

**Estimated Effort:** 0.5 hours

---

### P1.6 - Documentation Reorganization (6 hours) - PRIORITY 8

**Objective:** Reorganize documentation into logical directory structure

#### P1.6-T1: Create Documentation Structure (0.5h)
**Description:** Create new docs/ directory structure

**Directories Created:**
```
docs/
├── README.md
├── architecture/
├── development/
├── operations/
├── configuration/
├── api/
├── history/
│   ├── phases/
│   ├── reviews/
│   └── migrations/
└── prd/
```

**Dependencies:** None

**Acceptance Criteria:**
- [ ] All directories created
- [ ] .gitkeep files added (if needed)
- [ ] Directory structure matches ADR

**Testing:** Manual - verify structure

**Estimated Effort:** 0.5 hours

---

#### P1.6-T2: Move and Categorize Documents (2h)
**Description:** Move all markdown files from root to appropriate docs/ subdirectories

**File Moves:**

**Architecture:**
- CONTROLLER_STARTUP_FLOW.md → docs/architecture/controller-flow.md
- COST_OPTIMIZATION.md → docs/architecture/cost-optimization.md
- REBALANCER_ARCHITECTURE.md → docs/architecture/rebalancer.md

**Development:**
- DEVELOPMENT.md → docs/development/getting-started.md
- TEST_SUITE_COMPLETE.md → docs/development/testing.md
- INTEGRATION_TESTS_SUMMARY.md → docs/development/integration-tests.md

**Operations:**
- OBSERVABILITY.md → docs/operations/observability.md

**History:**
- PHASE5_SUMMARY.md → docs/history/phases/phase5-summary.md
- CODE_REVIEW_DETAILED.md → docs/history/reviews/code-review.md
- ARCHITECTURE_REVIEW_REPORT.md → docs/history/reviews/architecture-review.md

**PRD:**
- PRD.md → docs/prd/original.md
- PRD_CRITICAL_FIXES.md → docs/prd/critical-fixes.md
- PRD_NICE_TO_HAVE_FEATURES.md → docs/prd/nice-to-have.md

**Dependencies:** P1.6-T1

**Acceptance Criteria:**
- [ ] All markdown files moved
- [ ] Root directory has ≤3 markdown files (README.md, CLAUDE.md, LICENSE)
- [ ] No broken file references
- [ ] Git history preserved (use `git mv`)

**Testing:** Verify no broken links

**Estimated Effort:** 2 hours

---

#### P1.6-T3: Update Internal Links (1.5h)
**Description:** Fix all internal links in moved documents

**Tasks:**
- Find all markdown links: `grep -r "\.md" docs/`
- Update relative paths
- Update absolute paths in README.md
- Add redirects if needed

**Dependencies:** P1.6-T2

**Acceptance Criteria:**
- [ ] All internal links work
- [ ] No 404s when following links
- [ ] Links use relative paths where possible
- [ ] README.md links updated

**Testing:** Manual - click all links in documentation

**Estimated Effort:** 1.5 hours

---

#### P1.6-T4: Create Documentation Index (1h)
**Description:** Create master documentation index

**Files Created:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/docs/README.md`

**Content:**
- Documentation overview
- Quick start links
- Directory guide
- Search tips
- Contribution guidelines

**Dependencies:** P1.6-T3

**Acceptance Criteria:**
- [ ] Clear navigation structure
- [ ] Links to all major documents
- [ ] Categorized by audience (operator, developer, contributor)
- [ ] Search-friendly organization

**Testing:** Manual review

**Estimated Effort:** 1 hour

---

#### P1.6-T5: Update Main README (0.5h)
**Description:** Update root README.md to reference new docs structure

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/README.md`

**Changes:**
- Add "Documentation" section
- Link to docs/README.md
- Remove outdated references
- Add badges (if applicable)

**Dependencies:** P1.6-T4

**Acceptance Criteria:**
- [ ] Clear link to documentation
- [ ] Quick start preserved
- [ ] Badges updated (if applicable)
- [ ] No broken links

**Testing:** Manual review

**Estimated Effort:** 0.5 hours

---

#### P1.6-T6: Validation and Cleanup (0.5h)
**Description:** Final validation and cleanup

**Tasks:**
- Verify all links work
- Check for orphaned files
- Validate markdown syntax
- Remove old documentation files from root
- Update .gitignore if needed

**Dependencies:** P1.6-T5

**Acceptance Criteria:**
- [ ] Zero orphaned files
- [ ] All links validated
- [ ] Root directory clean
- [ ] No markdown syntax errors

**Testing:** Automated link checker + manual review

**Estimated Effort:** 0.5 hours

---

### P1.7 - Script Consolidation (3 hours) - PRIORITY 9

**Objective:** Consolidate all scripts into organized scripts/ directory

#### P1.7-T1: Create Scripts Directory Structure (0.5h)
**Description:** Create organized scripts directory

**Directories Created:**
```
scripts/
├── build/
├── test/
├── deploy/
├── dev/
└── utils/
```

**Dependencies:** None

**Acceptance Criteria:**
- [ ] All directories created
- [ ] README.md in scripts/ describing structure
- [ ] .gitkeep files if needed

**Testing:** Manual - verify structure

**Estimated Effort:** 0.5 hours

---

#### P1.7-T2: Move and Rename Scripts (1h)
**Description:** Move all scripts from root to scripts/ subdirectories

**File Moves:**

**Build:**
- build.sh → scripts/build/build.sh

**Test:**
- run-tests.sh → scripts/test/run-unit-tests.sh
- test-scaler.sh → scripts/test/test-scaler.sh
- scripts/verify-scaledown-integration.sh → scripts/test/verify-scaledown.sh
- scripts/verify-integration.sh → scripts/test/verify-integration.sh

**Utils:**
- fix-gomod.sh → scripts/utils/fix-gomod.sh
- fix-logging.sh → scripts/utils/fix-logging.sh

**Dependencies:** P1.7-T1

**Acceptance Criteria:**
- [ ] All scripts moved
- [ ] Root directory has no .sh files
- [ ] Execute permissions preserved (chmod +x)
- [ ] Git history preserved (use `git mv`)
- [ ] Scripts still work after move

**Testing:** Execute each script from new location

**Estimated Effort:** 1 hour

---

#### P1.7-T3: Update Makefile References (0.5h)
**Description:** Update Makefile to reference new script paths

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/Makefile`

**Changes:**
- Update all script paths
- Verify all make targets work
- Add comments for script locations

**Dependencies:** P1.7-T2

**Acceptance Criteria:**
- [ ] All make targets work
- [ ] Script paths correct
- [ ] No broken references
- [ ] `make test` still works
- [ ] `make build` still works

**Testing:** Run all make targets

**Estimated Effort:** 0.5 hours

---

#### P1.7-T4: Update Documentation (0.5h)
**Description:** Update documentation referencing scripts

**Files Modified:**
- `/Users/zozo/projects/vpsie-k8s-autoscaler/CLAUDE.md`
- `/Users/zozo/projects/vpsie-k8s-autoscaler/docs/development/getting-started.md`
- Any other docs referencing scripts

**Changes:**
- Update script paths
- Add scripts/ directory reference
- Update build/test instructions

**Dependencies:** P1.7-T2

**Acceptance Criteria:**
- [ ] All script references updated
- [ ] No broken links
- [ ] Instructions still accurate

**Testing:** Manual review + follow instructions

**Estimated Effort:** 0.5 hours

---

#### P1.7-T5: Validation (0.5h)
**Description:** Final validation of script consolidation

**Tasks:**
- Execute all scripts from new locations
- Run full test suite
- Verify make targets
- Check for missed scripts

**Dependencies:** P1.7-T3, P1.7-T4

**Acceptance Criteria:**
- [ ] All scripts execute successfully
- [ ] Zero scripts in root directory
- [ ] All make targets work
- [ ] Documentation accurate

**Testing:** Comprehensive script execution test

**Estimated Effort:** 0.5 hours

---

## Gantt Timeline

```
Sprint 1 (Week 1) - Days 1-5
═══════════════════════════════════════════════════════════════════

Day 1 (8h) - Metrics & Dashboard Foundation
├── P1.9-T1: Define New Metrics (1h)
├── P1.9-T2: Create Recorder Functions (1h)
├── P1.9-T3: Instrument ScaleDownManager (1.5h)
├── P1.9-T4: Instrument Node Drainer (1h)
├── P1.9-T5: Integration Test (0.5h)
├── P1.1-T1: Dashboard JSON Structure (1.5h)
└── P1.1-T2: Dashboard Panels Rows 1-3 (1.5h)

Day 2 (8h) - Dashboard & Alerts
├── P1.1-T3: Dashboard Panels Rows 4-7 (1.5h)
├── P1.1-T4: Annotations & Documentation (1h)
├── P1.1-T5: Dashboard Screenshot (0.5h)
├── P1.2-T1: Create Alert Rules (1.5h)
├── P1.2-T2: Create Runbooks (1.5h)
├── P1.2-T3: Alerting Guide (0.5h)
└── P1.2-T4: Validate Alerts (0.5h)
└── Buffer (0.5h)

Day 3 (8h) - Cloud-Init Templates
├── P1.3-T1: Add CRD Fields (1h)
├── P1.3-T2: Template Engine (2h)
├── P1.3-T3: Example Templates (1h)
├── P1.3-T4: Integration Test (1.5h)
├── P1.3-T5: Documentation (0.5h)
└── Buffer (2h)

Day 4 (8h) - SSH Keys & Sample Optimization
├── P1.4-T1: Add CRD Fields (0.5h)
├── P1.4-T2: Controller Options (0.5h)
├── P1.4-T3: SSH Key Collection (1.5h)
├── P1.4-T4: Update VPSie Client (0.5h)
├── P1.4-T5: Integration Test (1h)
├── P1.8-T1: Circular Buffer (1.5h)
├── P1.8-T2: Memory Pool (0.5h)
└── Buffer (1.5h)

Day 5 (8h) - Sample Optimization & Sprint 1 Wrap
├── P1.8-T3: Benchmarks (1h)
├── P1.8-T4: Performance Validation (1h)
├── Sprint 1 Integration Testing (3h)
├── Sprint 1 Documentation Review (1h)
├── Code Review Prep (1h)
└── Buffer (1h)

Sprint 2 (Week 2) - Days 6-10
═══════════════════════════════════════════════════════════════════

Day 6 (8h) - Configuration Package
├── P1.5-T1: Config Package Structure (1h)
├── P1.5-T2: Config Loaders (1.5h)
├── P1.5-T3: Example Config File (0.5h)
├── P1.5-T4: Update Controller (2h)
├── P1.5-T5: Consolidate Logging (1.5h) [Parallel]
└── Buffer (1.5h)

Day 7 (8h) - Configuration & Documentation Start
├── P1.5-T6: Config Documentation (0.5h)
├── P1.6-T1: Doc Structure (0.5h)
├── P1.6-T2: Move Documents (2h)
├── P1.6-T3: Update Links (1.5h)
└── Buffer (3.5h)

Day 8 (8h) - Documentation & Scripts
├── P1.6-T4: Documentation Index (1h)
├── P1.6-T5: Update Main README (0.5h)
├── P1.6-T6: Validation (0.5h)
├── P1.7-T1: Scripts Structure (0.5h)
├── P1.7-T2: Move Scripts (1h)
├── P1.7-T3: Update Makefile (0.5h)
├── P1.7-T4: Update Documentation (0.5h)
├── P1.7-T5: Validation (0.5h)
└── Buffer (3h)

Day 9 (8h) - Integration Testing & Bug Fixes
├── Full Integration Test Suite (4h)
├── Bug Fixes from Testing (2h)
├── Performance Testing (1h)
└── Buffer (1h)

Day 10 (8h) - Final Validation & Release Prep
├── Code Review Revisions (2h)
├── Final Documentation Pass (2h)
├── Release Notes (1h)
├── CHANGELOG.md Update (1h)
├── Tag Release (0.5h)
└── Retrospective (1.5h)
```

---

## Dependencies Matrix

```
┌─────────┬──────────────────────────────────────────────────────┐
│ Feature │ Dependencies                                         │
├─────────┼──────────────────────────────────────────────────────┤
│ P1.9    │ None (FIRST - Foundation for others)                │
│ P1.1    │ P1.9 (needs new metrics)                            │
│ P1.2    │ P1.9 (needs new metrics for alerts)                 │
│ P1.3    │ None (independent)                                   │
│ P1.4    │ None (independent)                                   │
│ P1.8    │ None (independent)                                   │
│ P1.5    │ None (independent)                                   │
│ P1.6    │ None (can start early, completes after P1.5)        │
│ P1.7    │ None (can start early)                              │
└─────────┴──────────────────────────────────────────────────────┘

Task-Level Dependencies:
═══════════════════════

P1.9: Linear dependency chain
  T1 → T2 → [T3, T4] → T5

P1.1: Linear with P1.9 dependency
  P1.9-T5 → T1 → T2 → T3 → T4 → T5

P1.2: Linear with P1.9 dependency
  P1.9-T5 → T1 → [T2, T3, T4]

P1.3: Linear
  T1 → T2 → T3 → T4 → T5

P1.4: Linear
  T1 → T2 → T3 → T4 → T5

P1.8: Linear
  T1 → T2 → T3 → T4

P1.5: Parallel possible
  T1 → T2 → [T3, T4, T5] → T6

P1.6: Linear
  T1 → T2 → T3 → T4 → T5 → T6

P1.7: Linear
  T1 → T2 → [T3, T4] → T5
```

---

## Testing Strategy

### Test Types by Feature

| Feature | Unit Tests | Integration Tests | Performance Tests | Manual Tests |
|---------|-----------|-------------------|-------------------|--------------|
| P1.9 | metrics_test.go, recorder_test.go | metrics_test.go | N/A | N/A |
| P1.1 | N/A | N/A | N/A | MT-1.1.1 to MT-1.1.5 |
| P1.2 | N/A | N/A | N/A | MT-1.2.1 to MT-1.2.7 |
| P1.3 | provisioner_test.go | cloudinit_template_test.go | N/A | Apply examples |
| P1.4 | provisioner_test.go, types_test.go | ssh_key_test.go | N/A | Verify SSH access |
| P1.8 | utilization_test.go, pool_test.go | N/A | utilization_bench_test.go | pprof analysis |
| P1.5 | config_test.go | config_test.go | N/A | Load test configs |
| P1.6 | N/A | N/A | N/A | Link validation |
| P1.7 | N/A | N/A | N/A | Execute all scripts |

### Integration Test Files (Created Simultaneously)

1. **test/integration/metrics_test.go** (P1.9)
   - TestMetrics_ScaleDownBlocked
   - TestMetrics_SafetyCheckFailures
   - TestMetrics_NodeDrain
   - TestMetrics_Registration

2. **test/integration/cloudinit_template_test.go** (P1.3)
   - TestCloudInit_InlineTemplate
   - TestCloudInit_ConfigMapReference
   - TestCloudInit_VariableSubstitution
   - TestCloudInit_DefaultTemplate
   - TestCloudInit_InvalidTemplate

3. **test/integration/ssh_key_test.go** (P1.4)
   - TestSSHKeys_GlobalKeys
   - TestSSHKeys_NodeGroupKeys
   - TestSSHKeys_SecretReference
   - TestSSHKeys_Merging
   - TestSSHKeys_Deduplication

4. **test/integration/config_test.go** (P1.5)
   - TestConfig_LoadFromFile
   - TestConfig_LoadFromEnv
   - TestConfig_LoadFromFlags
   - TestConfig_PriorityOrder
   - TestConfig_Validation

### Test Execution Order

**Sprint 1:**
1. P1.9 unit tests (Day 1)
2. P1.9 integration tests (Day 1)
3. P1.3 unit tests (Day 3)
4. P1.3 integration tests (Day 3)
5. P1.4 unit tests (Day 4)
6. P1.4 integration tests (Day 4)
7. P1.8 unit tests (Day 4)
8. P1.8 performance tests (Day 5)
9. **Sprint 1 integration test suite** (Day 5)

**Sprint 2:**
10. P1.5 unit tests (Day 6)
11. P1.5 integration tests (Day 7)
12. P1.6 validation (Day 8)
13. P1.7 validation (Day 8)
14. **Full integration test suite** (Day 9)
15. **Performance regression tests** (Day 9)

### Test Coverage Goals

- **Unit Test Coverage:** 80%+ for all new code
- **Integration Test Coverage:** All critical paths
- **Performance Benchmarks:** Baseline + optimized comparison
- **Manual Test Completion:** 100% of manual test cases

---

## Rollback Procedures

### Sprint 1 Rollback

**If critical issues found after Sprint 1 merge:**

1. **Immediate Rollback (Option 1):**
   ```bash
   # Revert merge commit
   git revert <merge-commit-sha>
   git push origin main

   # Redeploy previous version
   kubectl rollout undo deployment vpsie-autoscaler-controller -n kube-system
   ```

2. **Selective Feature Disable (Option 2):**
   ```bash
   # Remove Grafana dashboard
   kubectl delete configmap vpsie-autoscaler-dashboard -n monitoring

   # Remove Prometheus alerts
   kubectl delete configmap vpsie-autoscaler-alerts -n monitoring

   # Disable new metrics (feature flag)
   kubectl set env deployment/vpsie-autoscaler-controller ENABLE_NEW_METRICS=false
   ```

3. **CRD Rollback (if P1.3 or P1.4 cause issues):**
   ```bash
   # Reapply old CRD version
   kubectl apply -f deploy/crds/autoscaler.vpsie.io_nodegroups_v1alpha1_old.yaml

   # Note: Existing resources with new fields will retain them (backward compatible)
   ```

**Rollback Validation:**
- [ ] All NodeGroups reconciling normally
- [ ] No controller errors in logs
- [ ] VPSieNodes provisioning successfully
- [ ] Metrics endpoint returns 38 original metrics (if P1.9 rolled back)

### Sprint 2 Rollback

**If critical issues found after Sprint 2 merge:**

1. **Configuration Rollback (P1.5):**
   ```bash
   # Revert to flag-based configuration
   git revert <config-consolidation-commit-sha>

   # Update deployment to use old flags
   kubectl set env deployment/vpsie-autoscaler-controller --from-literal=... (old flags)
   ```

2. **Documentation Rollback (P1.6):**
   ```bash
   # Revert documentation moves
   git revert <doc-reorganization-commit-sha>

   # No deployment impact - docs only
   ```

3. **Script Rollback (P1.7):**
   ```bash
   # Revert script moves
   git revert <script-consolidation-commit-sha>

   # Update CI/CD pipelines if needed
   ```

**Rollback Validation:**
- [ ] Controller starts successfully
- [ ] Configuration loads correctly
- [ ] Build scripts work
- [ ] Tests run successfully

### Feature-Specific Rollback

**Per-Feature Rollback Commands:**

| Feature | Rollback Command | Impact |
|---------|-----------------|--------|
| P1.9 | Revert commits P1.9-T1 to P1.9-T5 | 4 metrics missing, dashboard/alerts degraded |
| P1.1 | Delete Grafana ConfigMap | No dashboard, manual dashboards unaffected |
| P1.2 | Delete Prometheus alerts ConfigMap | No alerts, manual alerts unaffected |
| P1.3 | Revert CRD, revert provisioner changes | Cloud-init templates ignored, default used |
| P1.4 | Revert CRD, revert provisioner changes | SSH keys not injected, manual SSH key setup needed |
| P1.8 | Revert utilization.go, pool.go | Higher memory usage, no functional change |
| P1.5 | Revert config package, restore flags | Flag-based config, no config file support |
| P1.6 | Revert git mv commands | Docs in old locations |
| P1.7 | Revert git mv commands | Scripts in old locations |

---

## Quality Gates

### Before Merging Each Feature

**Code Quality:**
- [ ] `go fmt` passes with no changes
- [ ] `go vet` passes with 0 warnings
- [ ] `golangci-lint run` passes (or exceptions documented)
- [ ] No new TODO comments without issue tracker reference
- [ ] Code coverage ≥80% for new code

**Testing:**
- [ ] All unit tests pass
- [ ] All integration tests pass (if applicable)
- [ ] Performance tests pass (if applicable)
- [ ] Manual tests completed (checklist signed off)
- [ ] No regressions in existing tests

**Documentation:**
- [ ] Code comments for public functions
- [ ] User-facing documentation updated
- [ ] Examples provided (if applicable)
- [ ] CHANGELOG.md updated
- [ ] Migration guide provided (if breaking changes)

**Review:**
- [ ] Code review completed (2+ approvals)
- [ ] Architecture review (for P1.3, P1.4, P1.5)
- [ ] Security review (for P1.4 SSH keys)
- [ ] All review comments addressed

**Integration:**
- [ ] Feature branch rebased on latest main
- [ ] Merge conflicts resolved
- [ ] CI/CD pipeline passes
- [ ] Staging deployment successful

### Before Sprint 1 Completion

**Functional:**
- [ ] All 6 Sprint 1 features merged
- [ ] Integration test suite passes (all features combined)
- [ ] Grafana dashboard displays data correctly
- [ ] Prometheus alerts validate with promtool
- [ ] Cloud-init templates render correctly
- [ ] SSH keys inject into VMs
- [ ] Memory optimization shows >50% reduction

**Performance:**
- [ ] Controller startup time <10 seconds
- [ ] Reconciliation latency P95 <1 second
- [ ] Memory usage stable over 24 hours
- [ ] No CPU regression (±5%)

**Operations:**
- [ ] Deployment guide tested
- [ ] Rollback procedure tested
- [ ] Monitoring dashboards functional
- [ ] Alert routing tested (if Alertmanager available)

**Documentation:**
- [ ] All new docs reviewed
- [ ] Links validated
- [ ] Screenshots current
- [ ] Runbooks accurate

### Before Sprint 2 Completion

**Functional:**
- [ ] All 3 Sprint 2 features merged
- [ ] Configuration package loads from all sources
- [ ] Documentation reorganization complete
- [ ] Scripts consolidated and functional

**Integration:**
- [ ] Full regression test suite passes
- [ ] All 9 P1 features work together
- [ ] No conflicts between features
- [ ] Configuration migrations work

**Release Readiness:**
- [ ] Release notes complete
- [ ] CHANGELOG.md updated
- [ ] Version tag created (e.g., v0.6.0-alpha)
- [ ] Helm chart updated
- [ ] Docker image built and pushed

**Stakeholder Approval:**
- [ ] Tech lead review
- [ ] Product owner sign-off
- [ ] Documentation team review (if applicable)

---

## Task Reference

### Complete Task List (27 tasks)

#### P1.9 - Missing Metrics (5 tasks)
- [x] P1.9-T1: Define New Metrics (1h)
- [x] P1.9-T2: Create Metric Recorder Functions (1h)
- [x] P1.9-T3: Instrument ScaleDownManager (1.5h)
- [x] P1.9-T4: Instrument Node Drainer (1h)
- [x] P1.9-T5: Integration Test (0.5h)

#### P1.1 - Grafana Dashboard (5 tasks)
- [x] P1.1-T1: Create Dashboard JSON Structure (1.5h)
- [x] P1.1-T2: Create Dashboard Panels Rows 1-3 (1.5h)
- [x] P1.1-T3: Create Dashboard Panels Rows 4-7 (1.5h)
- [x] P1.1-T4: Add Annotations and Documentation (1h)
- [x] P1.1-T5: Dashboard Screenshot and Validation (0.5h)

#### P1.2 - Prometheus Alerts (4 tasks)
- [x] P1.2-T1: Create Alert Rules YAML (1.5h)
- [x] P1.2-T2: Create Runbooks (1.5h)
- [x] P1.2-T3: Create Alerting Guide (0.5h)
- [x] P1.2-T4: Validate Alerts (0.5h)

#### P1.3 - Cloud-Init Templates (5 tasks)
- [x] P1.3-T1: Add CRD Fields (1h)
- [x] P1.3-T2: Implement Template Engine (2h)
- [x] P1.3-T3: Create Example Templates (1h)
- [x] P1.3-T4: Integration Test (1.5h)
- [x] P1.3-T5: Documentation (0.5h)

#### P1.4 - SSH Key Management (5 tasks)
- [x] P1.4-T1: Add CRD Fields (0.5h)
- [x] P1.4-T2: Add Controller Options (0.5h)
- [x] P1.4-T3: Implement SSH Key Collection (1.5h)
- [x] P1.4-T4: Update VPSie Client (0.5h)
- [x] P1.4-T5: Integration Test (1h)

#### P1.8 - Sample Storage Optimization (4 tasks)
- [x] P1.8-T1: Implement Circular Buffer (1.5h)
- [x] P1.8-T2: Create Memory Pool (0.5h)
- [x] P1.8-T3: Create Benchmarks (1h)
- [x] P1.8-T4: Performance Validation (1h)

#### P1.5 - Configuration Consolidation (6 tasks)
- [x] P1.5-T1: Create Config Package Structure (1h)
- [x] P1.5-T2: Implement Config Loaders (1.5h)
- [x] P1.5-T3: Create Example Config File (0.5h)
- [x] P1.5-T4: Update Controller (2h)
- [x] P1.5-T5: Consolidate Logging Packages (1.5h)
- [x] P1.5-T6: Documentation (0.5h)

#### P1.6 - Documentation Reorganization (6 tasks)
- [x] P1.6-T1: Create Documentation Structure (0.5h)
- [x] P1.6-T2: Move and Categorize Documents (2h)
- [x] P1.6-T3: Update Internal Links (1.5h)
- [x] P1.6-T4: Create Documentation Index (1h)
- [x] P1.6-T5: Update Main README (0.5h)
- [x] P1.6-T6: Validation and Cleanup (0.5h)

#### P1.7 - Script Consolidation (5 tasks)
- [x] P1.7-T1: Create Scripts Directory Structure (0.5h)
- [x] P1.7-T2: Move and Rename Scripts (1h)
- [x] P1.7-T3: Update Makefile References (0.5h)
- [x] P1.7-T4: Update Documentation (0.5h)
- [x] P1.7-T5: Validation (0.5h)

**Total Tasks:** 45 tasks
**Total Estimated Effort:** 50 hours (with buffer: 55 hours)

---

## Success Criteria

### Sprint 1 Success Criteria

**Observability:**
- [ ] 4 new metrics exposed and scraping
- [ ] Grafana dashboard imports and displays all 10 panels
- [ ] Prometheus alerts load and validate with promtool
- [ ] All 12 runbooks accessible and complete

**Provisioning Flexibility:**
- [ ] Cloud-init templates configurable per NodeGroup
- [ ] Template variables substitute correctly
- [ ] SSH keys inject into provisioned VMs
- [ ] Example templates work end-to-end

**Performance:**
- [ ] Sample storage memory usage reduced by >50%
- [ ] No CPU performance regression (<5% variance)
- [ ] Controller memory stable over 24 hours

**Quality:**
- [ ] All unit tests pass (80%+ coverage)
- [ ] All integration tests pass
- [ ] All performance benchmarks pass
- [ ] Zero regressions in existing functionality

### Sprint 2 Success Criteria

**Configuration:**
- [ ] Configuration loads from file, env, flags
- [ ] Priority order correct (flags > env > file > defaults)
- [ ] Backward compatible with existing deployments
- [ ] Single logging package (internal/logging)

**Documentation:**
- [ ] All docs moved to docs/ directory
- [ ] Root directory has ≤3 markdown files
- [ ] All internal links work
- [ ] Documentation index complete

**Scripts:**
- [ ] All scripts in scripts/ directory
- [ ] Root directory has 0 .sh files
- [ ] All make targets work
- [ ] Scripts execute from new locations

**Quality:**
- [ ] Full regression test suite passes
- [ ] No broken links in documentation
- [ ] All scripts functional
- [ ] Zero compilation errors

### Overall P1 Success Criteria

**Deliverables:**
- [ ] All 9 P1 features implemented
- [ ] 45 tasks completed
- [ ] 50-55 hours actual effort
- [ ] 2 sprints duration

**Quality Metrics:**
- [ ] Test coverage >80%
- [ ] Zero regressions
- [ ] Zero P1 bugs in production
- [ ] Documentation coverage 100%

**Business Outcomes:**
- [ ] Mean time to debug (MTTD) reduced by 50%
- [ ] Developer onboarding time reduced from 4h to 1h
- [ ] Operational workflows improved
- [ ] Production-ready observability

---

## Implementation Notes

### Best Practices

1. **Test-Driven Development:**
   - Write integration tests simultaneously with implementation
   - Don't wait until feature completion to test
   - Use table-driven tests for multiple scenarios

2. **Incremental Commits:**
   - Commit after each task completion
   - Use conventional commit messages (feat:, fix:, docs:, etc.)
   - Reference task IDs in commit messages (e.g., "feat(metrics): add scale-down blocked metric (P1.9-T1)")

3. **Code Review:**
   - Submit PRs per feature (not per task)
   - Include test results in PR description
   - Tag reviewers based on expertise area

4. **Documentation:**
   - Update documentation as you implement (not after)
   - Include screenshots for UI features
   - Test all examples before committing

5. **CRD Changes:**
   - Always run `make generate` after modifying CRD types
   - Test CRD manifests apply successfully
   - Document migration path for existing resources

### Common Pitfalls to Avoid

1. **Metrics:**
   - Don't forget to register new metrics in init()
   - Use appropriate metric types (Counter vs. Gauge vs. Histogram)
   - Include all required labels

2. **Templates:**
   - Validate template syntax before rendering
   - Handle missing variables gracefully
   - Escape user input to prevent injection

3. **Configuration:**
   - Maintain backward compatibility with existing flags
   - Validate configuration early (fail fast)
   - Provide clear error messages for invalid config

4. **Documentation:**
   - Use relative links for internal docs
   - Test all examples before documenting
   - Keep screenshots up-to-date

5. **Scripts:**
   - Preserve execute permissions (git add --chmod=+x)
   - Test scripts from different working directories
   - Use absolute paths or $( cd "$(dirname "$0")" && pwd )

---

## Appendix: File Paths Reference

### Files Created (New)

**Deploy:**
- /Users/zozo/projects/vpsie-k8s-autoscaler/deploy/grafana/autoscaler-dashboard.json
- /Users/zozo/projects/vpsie-k8s-autoscaler/deploy/grafana/README.md
- /Users/zozo/projects/vpsie-k8s-autoscaler/deploy/prometheus/alerts.yaml
- /Users/zozo/projects/vpsie-k8s-autoscaler/deploy/examples/cloud-init/gpu-node-template.yaml
- /Users/zozo/projects/vpsie-k8s-autoscaler/deploy/examples/cloud-init/arm64-node-template.yaml
- /Users/zozo/projects/vpsie-k8s-autoscaler/deploy/examples/cloud-init/custom-packages-template.yaml
- /Users/zozo/projects/vpsie-k8s-autoscaler/deploy/examples/nodegroup-with-cloudinit.yaml
- /Users/zozo/projects/vpsie-k8s-autoscaler/deploy/examples/nodegroup-with-ssh-keys.yaml

**Internal:**
- /Users/zozo/projects/vpsie-k8s-autoscaler/internal/config/config.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/internal/config/config_test.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/internal/config/defaults.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/internal/config/validation.go

**Package:**
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/pool.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/pool_test.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/utilization_bench_test.go

**Tests:**
- /Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/metrics_test.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/cloudinit_template_test.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/ssh_key_test.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/config_test.go

**Documentation:**
- /Users/zozo/projects/vpsie-k8s-autoscaler/docs/operations/grafana-setup.md
- /Users/zozo/projects/vpsie-k8s-autoscaler/docs/operations/alerting-guide.md
- /Users/zozo/projects/vpsie-k8s-autoscaler/docs/operations/runbooks.md
- /Users/zozo/projects/vpsie-k8s-autoscaler/docs/configuration/cloud-init.md
- /Users/zozo/projects/vpsie-k8s-autoscaler/docs/configuration/ssh-keys.md
- /Users/zozo/projects/vpsie-k8s-autoscaler/docs/configuration/config-reference.md
- /Users/zozo/projects/vpsie-k8s-autoscaler/docs/performance/sample-storage-optimization.md
- /Users/zozo/projects/vpsie-k8s-autoscaler/docs/README.md

**Config:**
- /Users/zozo/projects/vpsie-k8s-autoscaler/config/config.example.yaml

### Files Modified

**Core:**
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/metrics/metrics.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/metrics/recorder.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/scaler.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/drain.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/utilization.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/controller/vpsienode/provisioner.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/vpsie/client/types.go

**CRD:**
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1/nodegroup_types.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/deploy/crds/autoscaler.vpsie.io_nodegroups.yaml (regenerated)

**Controller:**
- /Users/zozo/projects/vpsie-k8s-autoscaler/cmd/controller/main.go
- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/controller/nodegroup/controller.go

**Build:**
- /Users/zozo/projects/vpsie-k8s-autoscaler/Makefile

**Documentation:**
- /Users/zozo/projects/vpsie-k8s-autoscaler/README.md
- /Users/zozo/projects/vpsie-k8s-autoscaler/CLAUDE.md

### Files Moved

**Scripts:**
- build.sh → scripts/build/build.sh
- run-tests.sh → scripts/test/run-unit-tests.sh
- test-scaler.sh → scripts/test/test-scaler.sh
- fix-gomod.sh → scripts/utils/fix-gomod.sh
- fix-logging.sh → scripts/utils/fix-logging.sh
- scripts/verify-scaledown-integration.sh → scripts/test/verify-scaledown.sh
- scripts/verify-integration.sh → scripts/test/verify-integration.sh

**Logging:**
- pkg/logging/ → internal/logging/

**Documentation:** (20+ files - see P1.6-T2 for complete list)

### Files Removed

- /Users/zozo/projects/vpsie-k8s-autoscaler/pkg/log/ (entire directory - unused)

---

**Document Status:** READY FOR IMPLEMENTATION
**Last Updated:** 2025-12-22
**Next Action:** Begin P1.9-T1 (Define New Metrics)
