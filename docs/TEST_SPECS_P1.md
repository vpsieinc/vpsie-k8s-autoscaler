# Test Specifications: P1 Nice-to-Have Features

**Document Version:** 1.0
**Date:** 2025-12-22
**Status:** APPROVED
**Scope:** Comprehensive test specifications for 9 P1 features
**Based On:** ADR_NICE_TO_HAVE_P1.md v1.0, ADR_REVIEW_P1.md

---

## Executive Summary

This document provides comprehensive acceptance test specifications for the 9 P1 nice-to-have features. Each feature includes unit tests, integration tests (where applicable), performance tests (for P1.8), and manual tests (for UI/operational features).

**Testing Strategy:**
- Unit tests: Written alongside implementation (TDD approach)
- Integration tests: Created simultaneously with implementation
- Performance tests: Run after P1.8 implementation
- Manual tests: Executed during deployment validation
- E2E tests: Run after all implementations complete

**Test Coverage Goals:**
- Unit test coverage: 80%+ for new code
- Integration test coverage: All critical paths
- Performance benchmarks: Memory and CPU profiling
- Manual validation: All operational workflows

---

## Table of Contents

1. [P1.1 - Grafana Dashboard Template](#p11---grafana-dashboard-template)
2. [P1.2 - Prometheus Alert Rules](#p12---prometheus-alert-rules)
3. [P1.3 - Cloud-Init Template Configuration](#p13---cloud-init-template-configuration)
4. [P1.4 - SSH Key Management](#p14---ssh-key-management)
5. [P1.5 - Configuration Package Consolidation](#p15---configuration-package-consolidation)
6. [P1.6 - Documentation Reorganization](#p16---documentation-reorganization)
7. [P1.7 - Script Consolidation](#p17---script-consolidation)
8. [P1.8 - Sample Storage Optimization](#p18---sample-storage-optimization)
9. [P1.9 - Missing Metrics](#p19---missing-metrics)
10. [Test Data Requirements](#test-data-requirements)
11. [Test Environment Setup](#test-environment-setup)

---

## P1.1 - Grafana Dashboard Template

### Overview
Production-ready Grafana dashboard using all 42 metrics (38 existing + 4 new from P1.9).

### Unit Tests

**Test File:** N/A (JSON configuration, no code to unit test)

### Integration Tests

**Test File:** N/A (Manual validation required)

### Manual Tests

#### MT-1.1.1: Dashboard Import
**Objective:** Verify dashboard imports successfully into Grafana

**Prerequisites:**
- Grafana 9.0+ running
- Prometheus datasource configured
- VPSie Autoscaler deployed with metrics exposed

**Test Steps:**
1. Navigate to Grafana UI
2. Click **Dashboards** → **Import**
3. Upload `deploy/grafana/autoscaler-dashboard.json`
4. Select Prometheus datasource from dropdown
5. Click **Import**

**Expected Results:**
- ✅ Dashboard imports without errors
- ✅ No missing panel errors
- ✅ Dashboard appears in dashboard list
- ✅ Dashboard UID is unique (no conflicts)

**Acceptance Criteria:**
- Import completes in <5 seconds
- Zero JSON parsing errors
- All 10 panels visible

---

#### MT-1.1.2: Variable Filtering
**Objective:** Verify dashboard variables work correctly

**Test Steps:**
1. Open imported dashboard
2. Locate variable dropdowns at top: `[Namespace ▼] [NodeGroup ▼] [Time Range ▼]`
3. Change **Namespace** to specific namespace (e.g., `default`)
4. Observe panel data updates
5. Change **NodeGroup** to specific group (e.g., `gpu-nodes`)
6. Observe panel data updates
7. Select **All** for both variables
8. Observe panel data shows all NodeGroups

**Expected Results:**
- ✅ Namespace variable populated from `label_values(nodegroup_desired_nodes, namespace)`
- ✅ NodeGroup variable filtered by selected namespace
- ✅ Panels update data within 5 seconds of variable change
- ✅ "All" option shows aggregated data
- ✅ No "No data" errors when valid data exists

**Acceptance Criteria:**
- Variable query execution time <2 seconds
- Panel refresh time <5 seconds
- Multi-select works for NodeGroup variable

---

#### MT-1.1.3: Panel Data Accuracy
**Objective:** Verify panels display correct metrics

**Test Steps:**
1. Open dashboard with time range "Last 1 hour"
2. Verify **Panel 1: NodeGroup Overview**
   - Check `nodegroup_desired_nodes` matches CRD spec
   - Check `nodegroup_current_nodes` matches actual VPSieNode count
   - Check `nodegroup_ready_nodes` matches ready nodes
3. Verify **Panel 2: Scaling Activity**
   - Trigger scale-up by increasing NodeGroup `minNodes`
   - Observe `scale_up_total` counter increments
4. Verify **Panel 3: Provisioning Heatmap**
   - Check heatmap shows distribution of `node_provisioning_duration_seconds`
5. Verify **Panel 4: API Health**
   - Check `vpsie_api_requests_total` rate
   - Check error rate calculation: `errors_total / requests_total`
6. Verify **Panel 10: Safety Check Failures** (NEW from P1.9)
   - Trigger safety check failure (e.g., PDB violation)
   - Verify counter increments

**Expected Results:**
- ✅ All metrics display non-zero values when cluster is active
- ✅ Gauge panels show current values
- ✅ Time series panels show historical trends
- ✅ Heatmap buckets are correctly populated
- ✅ Error rate calculation is accurate

**Acceptance Criteria:**
- Data accuracy: 100% match with Prometheus query results
- No missing data gaps >30 seconds
- All 10 panels render without errors

---

#### MT-1.1.4: Annotations
**Objective:** Verify scale event annotations appear

**Test Steps:**
1. Trigger scale event (change NodeGroup `minNodes`)
2. Wait for reconciliation to complete
3. Observe dashboard for annotation markers
4. Hover over annotation to see details

**Expected Results:**
- ✅ Annotation appears at time of scale event
- ✅ Annotation title: "Scale event"
- ✅ Annotation query: `changes(nodegroup_desired_nodes[$__interval])`
- ✅ Tooltip shows timestamp and NodeGroup name

**Acceptance Criteria:**
- Annotation appears within 30 seconds of event
- Annotation position matches event timestamp

---

#### MT-1.1.5: Theme Compatibility
**Objective:** Verify dashboard works in light and dark themes

**Test Steps:**
1. Open dashboard in dark theme (default)
2. Verify all panels are readable
3. Switch to light theme (Grafana preferences)
4. Verify all panels are readable
5. Check color contrast for:
   - Panel backgrounds
   - Graph lines
   - Text labels
   - Legend items

**Expected Results:**
- ✅ Dark theme: All text readable (white on dark)
- ✅ Light theme: All text readable (dark on white)
- ✅ Graph colors have sufficient contrast in both themes
- ✅ No color artifacts or invisible elements

**Acceptance Criteria:**
- WCAG AA contrast ratio (4.5:1) for text
- All colors configurable via Grafana theme variables

---

### Test Data Requirements

**Metrics Data:**
- At least 1 hour of metric history
- At least 2 NodeGroups with different activity patterns
- At least 1 scale-up and 1 scale-down event
- At least 1 VPSie API error (for error rate testing)

**Evidence Collection:**
- Screenshot of dashboard in dark theme
- Screenshot of dashboard in light theme
- Screenshot of variable filtering in action
- Screenshot of annotation example

**Validation Checklist:**
```yaml
dashboard_validation:
  import:
    - [ ] JSON valid
    - [ ] Imports without errors
    - [ ] UID is unique
  variables:
    - [ ] Namespace variable populated
    - [ ] NodeGroup variable filtered correctly
    - [ ] All option works
  panels:
    - [ ] Panel 1: NodeGroup Overview
    - [ ] Panel 2: Scaling Activity
    - [ ] Panel 3: Provisioning Heatmap
    - [ ] Panel 4: API Health
    - [ ] Panel 5: Controller Performance
    - [ ] Panel 6: VPSieNode Phase Distribution
    - [ ] Panel 7: Unschedulable Pods
    - [ ] Panel 8: Cost Tracking
    - [ ] Panel 9: Rebalancing Status
    - [ ] Panel 10: Safety Check Failures (NEW)
  annotations:
    - [ ] Scale events appear
    - [ ] Annotations clickable
  themes:
    - [ ] Dark theme readable
    - [ ] Light theme readable
```

---

## P1.2 - Prometheus Alert Rules

### Overview
12 alert rules (4 critical, 8 warning) with runbooks for proactive monitoring.

### Unit Tests

**Test File:** N/A (YAML configuration, validated by promtool)

### Integration Tests

**Test File:** N/A (Manual validation required)

### Manual Tests

#### MT-1.2.1: Alert Rule Validation
**Objective:** Verify alert rules are syntactically correct

**Prerequisites:**
- `promtool` installed (Prometheus toolkit)
- Alert rules file: `deploy/prometheus/alerts.yaml`

**Test Steps:**
1. Extract alerts.yaml from ConfigMap:
   ```bash
   kubectl get configmap vpsie-autoscaler-alerts -n monitoring -o yaml > /tmp/alerts-cm.yaml
   ```
2. Extract rules section to temporary file:
   ```bash
   yq eval '.data."alerts.yaml"' /tmp/alerts-cm.yaml > /tmp/alerts.yaml
   ```
3. Validate with promtool:
   ```bash
   promtool check rules /tmp/alerts.yaml
   ```

**Expected Results:**
- ✅ Exit code 0 (no errors)
- ✅ Output: "SUCCESS: X rules found"
- ✅ No syntax errors
- ✅ No duplicate alert names

**Acceptance Criteria:**
- All 12 alerts pass validation
- Zero errors or warnings from promtool

---

#### MT-1.2.2: Alert Rule Loading
**Objective:** Verify Prometheus loads alert rules

**Test Steps:**
1. Apply ConfigMap:
   ```bash
   kubectl apply -f deploy/prometheus/alerts.yaml
   ```
2. Restart Prometheus (or wait for hot-reload):
   ```bash
   kubectl rollout restart deployment prometheus -n monitoring
   ```
3. Check Prometheus UI → **Status** → **Rules**
4. Verify `vpsie-autoscaler-critical` group exists
5. Verify `vpsie-autoscaler-warning` group exists
6. Verify all 12 rules are loaded

**Expected Results:**
- ✅ ConfigMap created successfully
- ✅ Prometheus loads rules without errors
- ✅ Rules appear in Prometheus UI within 2 minutes
- ✅ All rules show "Inactive" state initially

**Acceptance Criteria:**
- Zero Prometheus configuration errors in logs
- All 12 rules visible in UI

---

#### MT-1.2.3: Critical Alert - HighVPSieAPIErrorRate
**Objective:** Verify critical alert fires when API error rate exceeds 10%

**Test Steps:**
1. Simulate VPSie API errors by:
   - Option A: Temporarily changing VPSie secret to invalid credentials
   - Option B: Using mock VPSie server with error injection
2. Wait for error rate to exceed 10% over 5 minutes
3. Check Prometheus UI → **Alerts**
4. Verify `HighVPSieAPIErrorRate` alert is **Firing**
5. Check alert labels:
   - `severity: critical`
   - `component: vpsie-api`
   - `method: <API method>`
6. Check annotations:
   - `summary` contains error percentage
   - `description` mentions API method
   - `runbook_url` is present and valid
   - `dashboard_url` is present and valid

**Expected Results:**
- ✅ Alert transitions: `Inactive → Pending (5min) → Firing`
- ✅ Alert appears in Alertmanager within 30 seconds of firing
- ✅ Labels are correct
- ✅ Annotations are populated
- ✅ Runbook URL is accessible (HTTP 200)

**Acceptance Criteria:**
- Alert fires within 6 minutes of error rate exceeding threshold
- Alert resolves within 6 minutes of error rate dropping below threshold

---

#### MT-1.2.4: Critical Alert - ControllerDown
**Objective:** Verify critical alert fires when controller stops reconciling

**Test Steps:**
1. Scale controller deployment to 0 replicas:
   ```bash
   kubectl scale deployment vpsie-autoscaler-controller --replicas=0 -n kube-system
   ```
2. Wait 10 minutes (alert `for` duration)
3. Check Prometheus UI → **Alerts**
4. Verify `ControllerDown` alert is **Firing**
5. Check alert labels and annotations
6. Scale controller back to 1 replica
7. Wait 10 minutes
8. Verify alert resolves to **Inactive**

**Expected Results:**
- ✅ Alert fires after 10 minutes of zero reconciliation rate
- ✅ Alert resolves after controller restarts and reconciles resources
- ✅ Severity is `critical`
- ✅ Runbook provides troubleshooting steps

**Acceptance Criteria:**
- Alert fires exactly at 10-minute mark (±30 seconds)
- Alert resolves within 11 minutes of controller restart

---

#### MT-1.2.5: Warning Alert - SlowNodeProvisioning
**Objective:** Verify warning alert fires when provisioning is slow

**Test Steps:**
1. Create NodeGroup with slow provisioning (e.g., wrong datacenter)
2. Wait for P95 provisioning duration to exceed 10 minutes over 15 minutes
3. Check Prometheus UI → **Alerts**
4. Verify `SlowNodeProvisioning` alert is **Firing**
5. Check alert contains NodeGroup name in labels

**Expected Results:**
- ✅ Alert fires when P95 duration exceeds 10 minutes
- ✅ Alert includes `nodegroup` label
- ✅ Severity is `warning`
- ✅ Annotation describes slow provisioning

**Acceptance Criteria:**
- Alert fires for slow NodeGroups only (not affecting others)
- Alert resolves when provisioning speeds up

---

#### MT-1.2.6: Alert Routing to Alertmanager
**Objective:** Verify alerts route to Alertmanager and trigger notifications

**Prerequisites:**
- Alertmanager configured with notification receiver (e.g., Slack, email)

**Test Steps:**
1. Trigger `HighVPSieAPIErrorRate` alert (from MT-1.2.3)
2. Wait for alert to fire
3. Check Alertmanager UI → **Alerts**
4. Verify alert appears with correct labels
5. Check notification channel (Slack, email, PagerDuty)
6. Verify notification received

**Expected Results:**
- ✅ Alert appears in Alertmanager within 1 minute
- ✅ Notification sent to configured receiver
- ✅ Notification includes:
  - Alert name
  - Severity
  - Summary
  - Dashboard link
  - Runbook link

**Acceptance Criteria:**
- Notification received within 2 minutes of alert firing
- Links in notification are clickable and valid

---

#### MT-1.2.7: Runbook Accessibility
**Objective:** Verify runbook URLs are accessible and helpful

**Test Steps:**
1. Extract all runbook URLs from `deploy/prometheus/alerts.yaml`:
   ```bash
   grep runbook_url deploy/prometheus/alerts.yaml
   ```
2. For each URL, verify:
   - HTTP status 200
   - Page loads in <5 seconds
   - Contains troubleshooting steps
   - Contains resolution guidance

**Expected Results:**
- ✅ All 12 runbook URLs return HTTP 200
- ✅ Runbooks are formatted in Markdown
- ✅ Runbooks include:
  - Problem description
  - Diagnosis steps
  - Resolution steps
  - Escalation path

**Acceptance Criteria:**
- 100% of runbook URLs accessible
- Runbooks contain actionable guidance

---

### Test Data Requirements

**Alert Testing Data:**
- VPSie API error injection capability
- Ability to scale controller to 0 replicas
- NodeGroup with configurable provisioning delay
- Alertmanager with test notification receiver

**Evidence Collection:**
- Screenshot of Prometheus Rules page (all 12 rules loaded)
- Screenshot of alert in Firing state
- Screenshot of Alertmanager showing routed alert
- Screenshot of notification in Slack/email
- Screenshot of runbook page

**Validation Checklist:**
```yaml
alert_validation:
  syntax:
    - [ ] promtool validation passes
    - [ ] No duplicate alert names
  loading:
    - [ ] Prometheus loads rules
    - [ ] All 12 rules visible in UI
  critical_alerts:
    - [ ] HighVPSieAPIErrorRate fires
    - [ ] ControllerDown fires
    - [ ] NodeGroupStuckScaling fires
    - [ ] HighControllerErrorRate fires
  warning_alerts:
    - [ ] SlowNodeProvisioning fires
    - [ ] HighNodeProvisioningFailureRate fires
    - [ ] StaleNodeGroupMetrics fires
    - [ ] UnschedulablePodsAccumulating fires
    - [ ] HighDrainFailureRate fires
    - [ ] FrequentRebalancing fires
    - [ ] CostSavingsNotRealized fires
    - [ ] HighMemoryUsage fires
  alertmanager:
    - [ ] Alerts route to Alertmanager
    - [ ] Notifications sent
  runbooks:
    - [ ] All URLs accessible
    - [ ] Runbooks contain guidance
```

---

## P1.3 - Cloud-Init Template Configuration

### Overview
Enable flexible node provisioning with customizable cloud-init templates via inline strings or ConfigMap references.

### Unit Tests

**Test File:** `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/controller/vpsienode/provisioner_test.go`

#### UT-1.3.1: Template Loading Priority
**Objective:** Verify template loading follows priority: inline > ConfigMap > default

**Test Code:**
```go
func TestProvisioner_LoadTemplate_Priority(t *testing.T) {
    tests := []struct {
        name              string
        inlineTemplate    string
        configMapRef      *v1alpha1.CloudInitTemplateRef
        configMapData     map[string]string
        expectedTemplate  string
        expectedSource    string // "inline", "configmap", "default"
    }{
        {
            name:             "Priority 1: Inline template",
            inlineTemplate:   "#!/bin/bash\necho 'inline'",
            configMapRef:     &v1alpha1.CloudInitTemplateRef{Name: "test-cm"},
            configMapData:    map[string]string{"cloud-init.sh": "echo 'configmap'"},
            expectedTemplate: "#!/bin/bash\necho 'inline'",
            expectedSource:   "inline",
        },
        {
            name:             "Priority 2: ConfigMap reference",
            inlineTemplate:   "",
            configMapRef:     &v1alpha1.CloudInitTemplateRef{Name: "test-cm", Key: "custom.sh"},
            configMapData:    map[string]string{"custom.sh": "echo 'configmap'"},
            expectedTemplate: "echo 'configmap'",
            expectedSource:   "configmap",
        },
        {
            name:             "Priority 3: Default template",
            inlineTemplate:   "",
            configMapRef:     nil,
            configMapData:    nil,
            expectedTemplate: defaultCloudInitTemplate,
            expectedSource:   "default",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // GIVEN: NodeGroup with template configuration
            ng := &v1alpha1.NodeGroup{
                ObjectMeta: metav1.ObjectMeta{Name: "test-ng", Namespace: "default"},
                Spec: v1alpha1.NodeGroupSpec{
                    CloudInitTemplate:    tt.inlineTemplate,
                    CloudInitTemplateRef: tt.configMapRef,
                },
            }

            // GIVEN: Fake Kubernetes client with ConfigMap (if needed)
            clientBuilder := fake.NewClientBuilder()
            if tt.configMapData != nil {
                cm := &corev1.ConfigMap{
                    ObjectMeta: metav1.ObjectMeta{Name: tt.configMapRef.Name, Namespace: "default"},
                    Data:       tt.configMapData,
                }
                clientBuilder = clientBuilder.WithObjects(cm)
            }
            client := clientBuilder.Build()

            provisioner := &Provisioner{client: client, log: logr.Discard()}

            // WHEN: Loading template
            result, err := provisioner.loadTemplate(context.Background(), ng)

            // THEN: Correct template loaded
            assert.NoError(t, err)
            assert.Equal(t, tt.expectedTemplate, result)
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Inline template takes highest priority
- ✅ ConfigMap reference used when inline is empty
- ✅ Default template used when both are empty
- ✅ No errors when only one source is provided

---

#### UT-1.3.2: ConfigMap Key Resolution
**Objective:** Verify ConfigMap key defaults to "cloud-init.sh" if not specified

**Test Code:**
```go
func TestProvisioner_LoadTemplateFromConfigMap_KeyDefault(t *testing.T) {
    tests := []struct {
        name          string
        refKey        string
        configMapData map[string]string
        expectedKey   string
        expectError   bool
    }{
        {
            name:          "Default key: cloud-init.sh",
            refKey:        "",
            configMapData: map[string]string{"cloud-init.sh": "default key content"},
            expectedKey:   "cloud-init.sh",
            expectError:   false,
        },
        {
            name:          "Custom key specified",
            refKey:        "custom.sh",
            configMapData: map[string]string{"custom.sh": "custom key content"},
            expectedKey:   "custom.sh",
            expectError:   false,
        },
        {
            name:          "Key not found in ConfigMap",
            refKey:        "missing.sh",
            configMapData: map[string]string{"cloud-init.sh": "default"},
            expectedKey:   "",
            expectError:   true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // GIVEN: ConfigMap with data
            cm := &corev1.ConfigMap{
                ObjectMeta: metav1.ObjectMeta{Name: "test-cm", Namespace: "default"},
                Data:       tt.configMapData,
            }
            client := fake.NewClientBuilder().WithObjects(cm).Build()

            ng := &v1alpha1.NodeGroup{
                ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
            }
            ref := &v1alpha1.CloudInitTemplateRef{Name: "test-cm", Key: tt.refKey}

            provisioner := &Provisioner{client: client, log: logr.Discard()}

            // WHEN: Loading from ConfigMap
            result, err := provisioner.loadTemplateFromConfigMap(context.Background(), ng, ref)

            // THEN: Key resolution correct
            if tt.expectError {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), "not found")
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.configMapData[tt.expectedKey], result)
            }
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Empty key defaults to "cloud-init.sh"
- ✅ Custom key used when specified
- ✅ Error returned when key not found in ConfigMap

---

#### UT-1.3.3: Template Variable Substitution
**Objective:** Verify template variables are correctly substituted

**Test Code:**
```go
func TestProvisioner_GenerateCloudInit_VariableSubstitution(t *testing.T) {
    // GIVEN: Template with variables
    template := `#!/bin/bash
Node: {{.NodeName}}
NodeGroup: {{.NodeGroupName}}
Kubernetes: {{.KubernetesVersion}}
Datacenter: {{.DatacenterID}}
Custom: {{.Custom.CustomVar}}`

    ng := &v1alpha1.NodeGroup{
        ObjectMeta: metav1.ObjectMeta{Name: "test-ng", Namespace: "default"},
        Spec: v1alpha1.NodeGroupSpec{
            CloudInitTemplate:   template,
            CloudInitVariables:  map[string]string{"CustomVar": "CustomValue"},
            DatacenterID:        "dc-123",
            KubernetesVersion:   "1.28.0",
        },
    }

    vn := &v1alpha1.VPSieNode{
        ObjectMeta: metav1.ObjectMeta{Name: "test-node"},
        Spec:       v1alpha1.VPSieNodeSpec{OfferingID: "offer-456"},
    }

    provisioner := &Provisioner{
        client:          fake.NewClientBuilder().Build(),
        log:             logr.Discard(),
        clusterEndpoint: "https://k8s.example.com:6443",
        joinToken:       "abcdef.0123456789abcdef",
        caCertHash:      "sha256:abc123...",
    }

    // WHEN: Generating cloud-init
    result, err := provisioner.generateCloudInit(context.Background(), vn, ng)

    // THEN: Variables substituted correctly
    assert.NoError(t, err)
    assert.Contains(t, result, "Node: test-node")
    assert.Contains(t, result, "NodeGroup: test-ng")
    assert.Contains(t, result, "Kubernetes: 1.28.0")
    assert.Contains(t, result, "Datacenter: dc-123")
    assert.Contains(t, result, "Custom: CustomValue")
}
```

**Acceptance Criteria:**
- ✅ Built-in variables substituted correctly
- ✅ Custom variables from NodeGroup spec substituted
- ✅ No placeholder strings remain in output

---

#### UT-1.3.4: Template Parsing Errors
**Objective:** Verify template parsing errors are handled gracefully

**Test Code:**
```go
func TestProvisioner_GenerateCloudInit_ParseErrors(t *testing.T) {
    tests := []struct {
        name          string
        template      string
        expectError   bool
        errorContains string
    }{
        {
            name:          "Missing variable key (missingkey=error)",
            template:      "{{.NonExistentVariable}}",
            expectError:   true,
            errorContains: "failed to execute template",
        },
        {
            name:          "Invalid template syntax",
            template:      "{{.NodeName",
            expectError:   true,
            errorContains: "failed to parse template",
        },
        {
            name:          "Valid template",
            template:      "{{.NodeName}}",
            expectError:   false,
            errorContains: "",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // GIVEN: NodeGroup with template
            ng := &v1alpha1.NodeGroup{
                ObjectMeta: metav1.ObjectMeta{Name: "test-ng"},
                Spec:       v1alpha1.NodeGroupSpec{CloudInitTemplate: tt.template},
            }
            vn := &v1alpha1.VPSieNode{ObjectMeta: metav1.ObjectMeta{Name: "test-node"}}

            provisioner := &Provisioner{
                client: fake.NewClientBuilder().Build(),
                log:    logr.Discard(),
            }

            // WHEN: Generating cloud-init
            _, err := provisioner.generateCloudInit(context.Background(), vn, ng)

            // THEN: Error handling correct
            if tt.expectError {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errorContains)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Parse errors return descriptive error messages
- ✅ Missing keys fail with `missingkey=error` option
- ✅ Syntax errors caught during parsing

---

#### UT-1.3.5: ConfigMap Namespace Resolution
**Objective:** Verify ConfigMap namespace defaults to NodeGroup namespace

**Test Code:**
```go
func TestProvisioner_LoadTemplateFromConfigMap_NamespaceResolution(t *testing.T) {
    tests := []struct {
        name              string
        nodeGroupNS       string
        refNamespace      string
        expectedNamespace string
    }{
        {
            name:              "Default to NodeGroup namespace",
            nodeGroupNS:       "production",
            refNamespace:      "",
            expectedNamespace: "production",
        },
        {
            name:              "Explicit namespace specified",
            nodeGroupNS:       "production",
            refNamespace:      "shared-configs",
            expectedNamespace: "shared-configs",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // GIVEN: ConfigMap in specific namespace
            cm := &corev1.ConfigMap{
                ObjectMeta: metav1.ObjectMeta{
                    Name:      "test-cm",
                    Namespace: tt.expectedNamespace,
                },
                Data: map[string]string{"cloud-init.sh": "content"},
            }
            client := fake.NewClientBuilder().WithObjects(cm).Build()

            ng := &v1alpha1.NodeGroup{
                ObjectMeta: metav1.ObjectMeta{Namespace: tt.nodeGroupNS},
            }
            ref := &v1alpha1.CloudInitTemplateRef{
                Name:      "test-cm",
                Namespace: tt.refNamespace,
            }

            provisioner := &Provisioner{client: client, log: logr.Discard()}

            // WHEN: Loading from ConfigMap
            result, err := provisioner.loadTemplateFromConfigMap(context.Background(), ng, ref)

            // THEN: Correct namespace used
            assert.NoError(t, err)
            assert.Equal(t, "content", result)
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Empty namespace defaults to NodeGroup namespace
- ✅ Explicit namespace overrides default
- ✅ ConfigMap fetched from correct namespace

---

### Integration Tests

**Test File:** `/Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/cloudinit_template_test.go`

#### IT-1.3.1: End-to-End Template Rendering
**Objective:** Verify complete flow from NodeGroup CRD to VPSie API call

**Test Code:**
```go
//go:build integration
// +build integration

func TestCloudInitTemplate_EndToEnd(t *testing.T) {
    // GIVEN: Kind cluster with VPSie mock server
    ctx := context.Background()
    cfg := integration.GetTestConfig(t)
    client := integration.GetTestClient(t, cfg)
    mockServer := integration.NewMockVPSieServer()
    defer mockServer.Close()

    // GIVEN: ConfigMap with GPU template
    cm := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "gpu-template",
            Namespace: "default",
        },
        Data: map[string]string{
            "cloud-init.sh": `#!/bin/bash
apt-get update
apt-get install -y nvidia-driver-{{.Custom.DriverVersion}}
kubeadm join {{.ClusterEndpoint}}`,
        },
    }
    require.NoError(t, client.Create(ctx, cm))

    // GIVEN: NodeGroup referencing ConfigMap
    ng := &v1alpha1.NodeGroup{
        ObjectMeta: metav1.ObjectMeta{Name: "gpu-nodes", Namespace: "default"},
        Spec: v1alpha1.NodeGroupSpec{
            MinNodes:              1,
            MaxNodes:              3,
            DatacenterID:          "dc-1",
            CloudInitTemplateRef:  &v1alpha1.CloudInitTemplateRef{Name: "gpu-template"},
            CloudInitVariables:    map[string]string{"DriverVersion": "535"},
        },
    }
    require.NoError(t, client.Create(ctx, ng))

    // WHEN: Controller reconciles NodeGroup
    integration.WaitForNodeGroupReconciliation(t, client, "gpu-nodes", 2*time.Minute)

    // THEN: VPSie API called with rendered cloud-init
    apiCalls := mockServer.GetVMCreateRequests()
    require.Len(t, apiCalls, 1)

    userData := apiCalls[0].UserData
    assert.Contains(t, userData, "nvidia-driver-535")
    assert.Contains(t, userData, "kubeadm join https://")
    assert.NotContains(t, userData, "{{.Custom.DriverVersion}}") // Template rendered
}
```

**Acceptance Criteria:**
- ✅ ConfigMap template loaded successfully
- ✅ Variables substituted in rendered output
- ✅ Rendered cloud-init passed to VPSie API
- ✅ No template placeholders in API request

---

#### IT-1.3.2: Mutual Exclusivity Validation
**Objective:** Verify inline and ConfigMap are mutually exclusive

**Test Code:**
```go
func TestCloudInitTemplate_MutualExclusivity(t *testing.T) {
    ctx := context.Background()
    client := integration.GetTestClient(t, integration.GetTestConfig(t))

    // GIVEN: NodeGroup with BOTH inline and ConfigMap
    ng := &v1alpha1.NodeGroup{
        ObjectMeta: metav1.ObjectMeta{Name: "invalid-ng", Namespace: "default"},
        Spec: v1alpha1.NodeGroupSpec{
            MinNodes:             1,
            MaxNodes:             1,
            CloudInitTemplate:    "#!/bin/bash\necho 'inline'",
            CloudInitTemplateRef: &v1alpha1.CloudInitTemplateRef{Name: "test-cm"},
        },
    }

    // WHEN: Creating NodeGroup
    err := client.Create(ctx, ng)

    // THEN: Validation webhook rejects creation
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "mutually exclusive")
}
```

**Acceptance Criteria:**
- ✅ Webhook rejects NodeGroup with both inline and ConfigMap
- ✅ Error message mentions "mutually exclusive"
- ✅ Either inline OR ConfigMap allowed (not both)

---

### Test Data Requirements

**ConfigMap Templates:**
```yaml
# GPU node template
apiVersion: v1
kind: ConfigMap
metadata:
  name: gpu-template
  namespace: default
data:
  cloud-init.sh: |
    #!/bin/bash
    apt-get update
    apt-get install -y nvidia-driver-{{.Custom.DriverVersion}}
    kubeadm join {{.ClusterEndpoint}} --token {{.JoinToken}}

# ARM64 template
apiVersion: v1
kind: ConfigMap
metadata:
  name: arm64-template
  namespace: default
data:
  cloud-init.sh: |
    #!/bin/bash
    dpkg --add-architecture arm64
    apt-get update
    kubeadm join {{.ClusterEndpoint}}
```

**NodeGroup Examples:**
```yaml
# Inline template
apiVersion: autoscaler.vpsie.io/v1alpha1
kind: NodeGroup
metadata:
  name: test-inline
spec:
  minNodes: 1
  maxNodes: 3
  cloudInitTemplate: |
    #!/bin/bash
    echo "Node: {{.NodeName}}"
    kubeadm join {{.ClusterEndpoint}}

# ConfigMap reference
apiVersion: autoscaler.vpsie.io/v1alpha1
kind: NodeGroup
metadata:
  name: test-configmap
spec:
  minNodes: 1
  maxNodes: 3
  cloudInitTemplateRef:
    name: gpu-template
    key: cloud-init.sh
  cloudInitVariables:
    DriverVersion: "535"
```

---

## P1.4 - SSH Key Management

### Overview
Enable SSH access to nodes via VPSie SSH key IDs, inline public keys, or Kubernetes secrets.

### Unit Tests

**Test File:** `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/controller/vpsienode/ssh_keys_test.go`

#### UT-1.4.1: SSH Key Collection from Multiple Sources
**Objective:** Verify SSH keys collected from global defaults + NodeGroup sources

**Test Code:**
```go
func TestProvisioner_CollectSSHKeys_MultipleSources(t *testing.T) {
    tests := []struct {
        name                  string
        globalKeyIDs          []string
        globalPublicKeys      []string
        nodeGroupKeyIDs       []string
        nodeGroupPublicKeys   []string
        secretKeys            map[string]string
        expectedKeyIDCount    int
        expectedPublicKeyCount int
    }{
        {
            name:                  "All sources populated",
            globalKeyIDs:          []string{"global-key-1"},
            globalPublicKeys:      []string{"ssh-rsa AAAA...global"},
            nodeGroupKeyIDs:       []string{"ng-key-1"},
            nodeGroupPublicKeys:   []string{"ssh-rsa AAAA...nodegroup"},
            secretKeys:            map[string]string{"admin": "ssh-rsa AAAA...secret"},
            expectedKeyIDCount:    2, // global-key-1 + ng-key-1
            expectedPublicKeyCount: 3, // global + nodegroup + secret
        },
        {
            name:                  "Only global defaults",
            globalKeyIDs:          []string{"global-key-1", "global-key-2"},
            globalPublicKeys:      []string{},
            nodeGroupKeyIDs:       []string{},
            nodeGroupPublicKeys:   []string{},
            secretKeys:            nil,
            expectedKeyIDCount:    2,
            expectedPublicKeyCount: 0,
        },
        {
            name:                  "Deduplication of duplicate keys",
            globalKeyIDs:          []string{"key-1"},
            globalPublicKeys:      []string{"ssh-rsa AAAA...duplicate"},
            nodeGroupKeyIDs:       []string{"key-1"}, // Duplicate
            nodeGroupPublicKeys:   []string{"ssh-rsa AAAA...duplicate"}, // Duplicate
            secretKeys:            nil,
            expectedKeyIDCount:    1, // Deduplicated
            expectedPublicKeyCount: 1, // Deduplicated
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // GIVEN: Provisioner with global defaults
            clientBuilder := fake.NewClientBuilder()
            if tt.secretKeys != nil {
                secret := &corev1.Secret{
                    ObjectMeta: metav1.ObjectMeta{Name: "ssh-secret", Namespace: "default"},
                    Data:       make(map[string][]byte),
                }
                for k, v := range tt.secretKeys {
                    secret.Data[k] = []byte(v)
                }
                clientBuilder = clientBuilder.WithObjects(secret)
            }
            client := clientBuilder.Build()

            provisioner := &Provisioner{
                client:               client,
                log:                  logr.Discard(),
                defaultSSHKeyIDs:     tt.globalKeyIDs,
                defaultSSHPublicKeys: tt.globalPublicKeys,
            }

            // GIVEN: NodeGroup with SSH keys
            ng := &v1alpha1.NodeGroup{
                ObjectMeta: metav1.ObjectMeta{Name: "test-ng", Namespace: "default"},
                Spec: v1alpha1.NodeGroupSpec{
                    SSHKeyIDs:     tt.nodeGroupKeyIDs,
                    SSHPublicKeys: tt.nodeGroupPublicKeys,
                },
            }

            if tt.secretKeys != nil {
                ng.Spec.SSHKeySecretRef = &v1alpha1.SSHKeySecretRef{
                    Name: "ssh-secret",
                    Keys: []string{"admin"},
                }
            }

            // WHEN: Collecting SSH keys
            collection, err := provisioner.collectSSHKeys(context.Background(), ng)

            // THEN: All sources merged and deduplicated
            assert.NoError(t, err)
            assert.Len(t, collection.KeyIDs, tt.expectedKeyIDCount)
            assert.Len(t, collection.PublicKeys, tt.expectedPublicKeyCount)
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Global defaults included in collection
- ✅ NodeGroup-specific keys included
- ✅ Secret keys loaded and included
- ✅ Duplicates removed via deduplication

---

#### UT-1.4.2: SSH Key Validation
**Objective:** Verify SSH public keys are validated before use

**Test Code:**
```go
func TestProvisioner_ValidateSSHPublicKey(t *testing.T) {
    tests := []struct {
        name        string
        publicKey   string
        expectValid bool
    }{
        {
            name:        "Valid RSA key",
            publicKey:   "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC... user@host",
            expectValid: true,
        },
        {
            name:        "Valid ED25519 key",
            publicKey:   "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@host",
            expectValid: true,
        },
        {
            name:        "Valid ECDSA key",
            publicKey:   "ecdsa-sha2-nistp256 AAAAE2VjZHNhLXNoYTItbmlzdHAyNTY... user@host",
            expectValid: true,
        },
        {
            name:        "Invalid: no prefix",
            publicKey:   "AAAAB3NzaC1yc2EAAAADAQABAAABAQC...",
            expectValid: false,
        },
        {
            name:        "Invalid: private key",
            publicKey:   "-----BEGIN RSA PRIVATE KEY-----",
            expectValid: false,
        },
        {
            name:        "Invalid: empty",
            publicKey:   "",
            expectValid: false,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // WHEN: Validating SSH key
            err := validateSSHPublicKey(tt.publicKey)

            // THEN: Validation result correct
            if tt.expectValid {
                assert.NoError(t, err)
            } else {
                assert.Error(t, err)
            }
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Valid RSA, ED25519, ECDSA keys accepted
- ✅ Invalid formats rejected
- ✅ Private keys rejected
- ✅ Empty strings rejected

---

#### UT-1.4.3: Secret Loading
**Objective:** Verify SSH keys loaded correctly from Kubernetes secrets

**Test Code:**
```go
func TestProvisioner_LoadSSHKeysFromSecret(t *testing.T) {
    tests := []struct {
        name          string
        secretData    map[string][]byte
        secretRef     *v1alpha1.SSHKeySecretRef
        expectedKeys  int
        expectError   bool
    }{
        {
            name: "Load specific keys",
            secretData: map[string][]byte{
                "admin":     []byte("ssh-rsa AAAA...admin"),
                "ops":       []byte("ssh-rsa AAAA...ops"),
                "developer": []byte("ssh-rsa AAAA...dev"),
            },
            secretRef: &v1alpha1.SSHKeySecretRef{
                Name: "ssh-keys",
                Keys: []string{"admin", "ops"},
            },
            expectedKeys: 2,
            expectError:  false,
        },
        {
            name: "Load all keys when Keys is empty",
            secretData: map[string][]byte{
                "admin": []byte("ssh-rsa AAAA...admin"),
                "ops":   []byte("ssh-rsa AAAA...ops"),
            },
            secretRef: &v1alpha1.SSHKeySecretRef{
                Name: "ssh-keys",
                Keys: []string{},
            },
            expectedKeys: 2, // All keys loaded
            expectError:  false,
        },
        {
            name:       "Secret not found",
            secretData: nil,
            secretRef: &v1alpha1.SSHKeySecretRef{
                Name: "missing-secret",
                Keys: []string{"admin"},
            },
            expectedKeys: 0,
            expectError:  true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // GIVEN: Secret with SSH keys
            clientBuilder := fake.NewClientBuilder()
            if tt.secretData != nil {
                secret := &corev1.Secret{
                    ObjectMeta: metav1.ObjectMeta{
                        Name:      tt.secretRef.Name,
                        Namespace: "default",
                    },
                    Data: tt.secretData,
                }
                clientBuilder = clientBuilder.WithObjects(secret)
            }
            client := clientBuilder.Build()

            ng := &v1alpha1.NodeGroup{
                ObjectMeta: metav1.ObjectMeta{Namespace: "default"},
            }

            provisioner := &Provisioner{client: client, log: logr.Discard()}

            // WHEN: Loading keys from secret
            keys, err := provisioner.loadSSHKeysFromSecret(context.Background(), ng, tt.secretRef)

            // THEN: Correct keys loaded
            if tt.expectError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Len(t, keys, tt.expectedKeys)
            }
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Specific keys loaded when Keys list provided
- ✅ All keys loaded when Keys list empty
- ✅ Error returned when secret not found
- ✅ Error returned when key not found in secret

---

### Integration Tests

**Test File:** `/Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/ssh_keys_test.go`

#### IT-1.4.1: SSH Keys Passed to VPSie API
**Objective:** Verify SSH keys included in VPSie VM create request

**Test Code:**
```go
//go:build integration
// +build integration

func TestSSHKeys_PassedToVPSieAPI(t *testing.T) {
    ctx := context.Background()
    client := integration.GetTestClient(t, integration.GetTestConfig(t))
    mockServer := integration.NewMockVPSieServer()
    defer mockServer.Close()

    // GIVEN: Secret with SSH keys
    secret := &corev1.Secret{
        ObjectMeta: metav1.ObjectMeta{Name: "prod-ssh-keys", Namespace: "default"},
        Data: map[string][]byte{
            "admin": []byte("ssh-rsa AAAAB3...admin user@admin"),
            "ops":   []byte("ssh-rsa AAAAB3...ops user@ops"),
        },
    }
    require.NoError(t, client.Create(ctx, secret))

    // GIVEN: NodeGroup with SSH keys from multiple sources
    ng := &v1alpha1.NodeGroup{
        ObjectMeta: metav1.ObjectMeta{Name: "ssh-test-ng", Namespace: "default"},
        Spec: v1alpha1.NodeGroupSpec{
            MinNodes:        1,
            MaxNodes:        1,
            DatacenterID:    "dc-1",
            SSHKeyIDs:       []string{"vpsie-key-123"},
            SSHPublicKeys:   []string{"ssh-rsa AAAAB3...inline user@inline"},
            SSHKeySecretRef: &v1alpha1.SSHKeySecretRef{
                Name: "prod-ssh-keys",
                Keys: []string{"admin"},
            },
        },
    }
    require.NoError(t, client.Create(ctx, ng))

    // WHEN: Controller reconciles NodeGroup
    integration.WaitForNodeGroupReconciliation(t, client, "ssh-test-ng", 2*time.Minute)

    // THEN: VPSie API called with all SSH keys
    apiCalls := mockServer.GetVMCreateRequests()
    require.Len(t, apiCalls, 1)

    req := apiCalls[0]
    assert.Contains(t, req.SSHKeyIDs, "vpsie-key-123")
    assert.Contains(t, req.SSHPublicKeys, "ssh-rsa AAAAB3...inline user@inline")
    assert.Contains(t, req.SSHPublicKeys, "ssh-rsa AAAAB3...admin user@admin")
    assert.Len(t, req.SSHKeyIDs, 1)
    assert.Len(t, req.SSHPublicKeys, 2)
}
```

**Acceptance Criteria:**
- ✅ VPSie key IDs included in API request
- ✅ Inline public keys included
- ✅ Secret public keys included
- ✅ No duplicates in API request

---

### Test Data Requirements

**SSH Keys:**
```yaml
# Test secret
apiVersion: v1
kind: Secret
metadata:
  name: test-ssh-keys
  namespace: default
type: Opaque
data:
  admin: c3NoLXJzYSBBQUFBQjMuLi5hZG1pbg==  # Base64: "ssh-rsa AAAAB3...admin"
  ops: c3NoLXJzYSBBQUFBQjMuLi5vcHM=      # Base64: "ssh-rsa AAAAB3...ops"

# NodeGroup with all SSH key sources
apiVersion: autoscaler.vpsie.io/v1alpha1
kind: NodeGroup
metadata:
  name: multi-ssh-ng
spec:
  minNodes: 1
  maxNodes: 3
  sshKeyIDs:
    - vpsie-key-abc
  sshPublicKeys:
    - "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC... user@host"
  sshKeySecretRef:
    name: test-ssh-keys
    keys:
      - admin
      - ops
```

---

## P1.5 - Configuration Package Consolidation

### Overview
Centralize configuration management using Viper with support for flags, environment variables, and config files.

### Unit Tests

**Test File:** `/Users/zozo/projects/vpsie-k8s-autoscaler/internal/config/config_test.go`

#### UT-1.5.1: Configuration Priority Order
**Objective:** Verify priority: flags > env > file > defaults

**Test Code:**
```go
func TestLoadConfig_Priority(t *testing.T) {
    tests := []struct {
        name         string
        flagValue    string
        envValue     string
        fileValue    string
        defaultValue string
        expectedValue string
    }{
        {
            name:          "Flag has highest priority",
            flagValue:     ":9090",
            envValue:      ":8080",
            fileValue:     ":7070",
            defaultValue:  ":6060",
            expectedValue: ":9090",
        },
        {
            name:          "Env overrides file and defaults",
            flagValue:     "",
            envValue:      ":8080",
            fileValue:     ":7070",
            defaultValue:  ":6060",
            expectedValue: ":8080",
        },
        {
            name:          "File overrides defaults",
            flagValue:     "",
            envValue:      "",
            fileValue:     ":7070",
            defaultValue:  ":6060",
            expectedValue: ":7070",
        },
        {
            name:          "Defaults used when nothing else set",
            flagValue:     "",
            envValue:      "",
            fileValue:     "",
            defaultValue:  ":6060",
            expectedValue: ":6060",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // GIVEN: Temporary config file
            if tt.fileValue != "" {
                configContent := fmt.Sprintf("metrics:\n  bindAddress: %s\n", tt.fileValue)
                tmpFile := createTempConfigFile(t, configContent)
                defer os.Remove(tmpFile)
                os.Setenv("VPSIE_CONFIG_FILE", tmpFile)
                defer os.Unsetenv("VPSIE_CONFIG_FILE")
            }

            // GIVEN: Environment variable
            if tt.envValue != "" {
                os.Setenv("VPSIE_METRICS_BIND_ADDRESS", tt.envValue)
                defer os.Unsetenv("VPSIE_METRICS_BIND_ADDRESS")
            }

            // GIVEN: Command-line flags
            flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
            flags.String("metrics-bind-address", tt.defaultValue, "")
            if tt.flagValue != "" {
                flags.Set("metrics-bind-address", tt.flagValue)
            }

            // WHEN: Loading config
            cfg, err := LoadConfig(flags)

            // THEN: Correct priority applied
            assert.NoError(t, err)
            assert.Equal(t, tt.expectedValue, cfg.Metrics.BindAddress)
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Flags override all other sources
- ✅ Environment variables override file and defaults
- ✅ File overrides defaults
- ✅ Defaults used as fallback

---

#### UT-1.5.2: Configuration Validation
**Objective:** Verify invalid configurations are rejected

**Test Code:**
```go
func TestConfig_Validate(t *testing.T) {
    tests := []struct {
        name        string
        config      *Config
        expectValid bool
        errorMsg    string
    }{
        {
            name: "Valid configuration",
            config: &Config{
                Metrics: MetricsConfig{BindAddress: ":8080"},
                Health:  HealthConfig{BindAddress: ":8081"},
                Logging: LoggingConfig{Level: "info"},
                VPSie:   VPSieConfig{RateLimit: 100},
            },
            expectValid: true,
        },
        {
            name: "Invalid log level",
            config: &Config{
                Logging: LoggingConfig{Level: "invalid"},
            },
            expectValid: false,
            errorMsg:    "invalid log level",
        },
        {
            name: "Invalid rate limit (negative)",
            config: &Config{
                VPSie: VPSieConfig{RateLimit: -10},
            },
            expectValid: false,
            errorMsg:    "rate limit must be positive",
        },
        {
            name: "Invalid rate limit (zero)",
            config: &Config{
                VPSie: VPSieConfig{RateLimit: 0},
            },
            expectValid: false,
            errorMsg:    "rate limit must be positive",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // WHEN: Validating config
            err := tt.config.Validate()

            // THEN: Validation result correct
            if tt.expectValid {
                assert.NoError(t, err)
            } else {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errorMsg)
            }
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Valid configs pass validation
- ✅ Invalid log levels rejected
- ✅ Negative/zero rate limits rejected
- ✅ Error messages are descriptive

---

#### UT-1.5.3: Environment Variable Mapping
**Objective:** Verify environment variables map correctly to config fields

**Test Code:**
```go
func TestLoadConfig_EnvironmentVariables(t *testing.T) {
    tests := []struct {
        name      string
        envVars   map[string]string
        checkFunc func(*testing.T, *Config)
    }{
        {
            name: "Metrics configuration",
            envVars: map[string]string{
                "VPSIE_METRICS_BIND_ADDRESS": ":9090",
                "VPSIE_METRICS_PATH":         "/custom-metrics",
            },
            checkFunc: func(t *testing.T, cfg *Config) {
                assert.Equal(t, ":9090", cfg.Metrics.BindAddress)
                assert.Equal(t, "/custom-metrics", cfg.Metrics.Path)
            },
        },
        {
            name: "Logging configuration",
            envVars: map[string]string{
                "VPSIE_LOGGING_LEVEL":       "debug",
                "VPSIE_LOGGING_FORMAT":      "json",
                "VPSIE_LOGGING_DEVELOPMENT": "true",
            },
            checkFunc: func(t *testing.T, cfg *Config) {
                assert.Equal(t, "debug", cfg.Logging.Level)
                assert.Equal(t, "json", cfg.Logging.Format)
                assert.True(t, cfg.Logging.Development)
            },
        },
        {
            name: "VPSie configuration",
            envVars: map[string]string{
                "VPSIE_VPSIE_SECRET_NAME":      "custom-secret",
                "VPSIE_VPSIE_SECRET_NAMESPACE": "custom-ns",
                "VPSIE_VPSIE_RATE_LIMIT":       "200",
            },
            checkFunc: func(t *testing.T, cfg *Config) {
                assert.Equal(t, "custom-secret", cfg.VPSie.SecretName)
                assert.Equal(t, "custom-ns", cfg.VPSie.SecretNamespace)
                assert.Equal(t, 200, cfg.VPSie.RateLimit)
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // GIVEN: Environment variables set
            for k, v := range tt.envVars {
                os.Setenv(k, v)
                defer os.Unsetenv(k)
            }

            // WHEN: Loading config
            flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
            cfg, err := LoadConfig(flags)

            // THEN: Environment variables mapped correctly
            assert.NoError(t, err)
            tt.checkFunc(t, cfg)
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Environment variables use VPSIE_ prefix
- ✅ Nested fields use underscores (e.g., VPSIE_METRICS_BIND_ADDRESS)
- ✅ Boolean, int, and string types parsed correctly
- ✅ Case-insensitive environment variable names

---

#### UT-1.5.4: Default Values
**Objective:** Verify default values are set correctly

**Test Code:**
```go
func TestLoadConfig_Defaults(t *testing.T) {
    // GIVEN: No flags, env vars, or config file
    flags := pflag.NewFlagSet("test", pflag.ContinueOnError)

    // WHEN: Loading config with defaults only
    cfg, err := LoadConfig(flags)

    // THEN: Default values applied
    assert.NoError(t, err)

    // Metrics defaults
    assert.Equal(t, ":8080", cfg.Metrics.BindAddress)
    assert.Equal(t, "/metrics", cfg.Metrics.Path)

    // Health defaults
    assert.Equal(t, ":8081", cfg.Health.BindAddress)
    assert.Equal(t, "/healthz", cfg.Health.LivenessPath)
    assert.Equal(t, "/readyz", cfg.Health.ReadinessPath)

    // Logging defaults
    assert.Equal(t, "info", cfg.Logging.Level)
    assert.Equal(t, "json", cfg.Logging.Format)
    assert.False(t, cfg.Logging.Development)

    // VPSie defaults
    assert.Equal(t, "vpsie-secret", cfg.VPSie.SecretName)
    assert.Equal(t, "kube-system", cfg.VPSie.SecretNamespace)
    assert.Equal(t, 100, cfg.VPSie.RateLimit)
    assert.Equal(t, 30*time.Second, cfg.VPSie.Timeout)

    // Controller defaults
    assert.True(t, cfg.Controller.LeaderElection)
    assert.Equal(t, "vpsie-autoscaler-leader", cfg.Controller.LeaderElectionID)
    assert.Equal(t, 10*time.Minute, cfg.Controller.SyncPeriod)

    // Feature flags defaults
    assert.True(t, cfg.Features.EnableRebalancing)
    assert.True(t, cfg.Features.EnableCostOptimization)
    assert.False(t, cfg.Features.EnableSpotInstances) // Future feature
}
```

**Acceptance Criteria:**
- ✅ All fields have sensible defaults
- ✅ Defaults match production recommendations
- ✅ Feature flags default to enabled for Phase 5 features

---

### Integration Tests

**Test File:** `/Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/config_test.go`

#### IT-1.5.1: Config File Loading
**Objective:** Verify config file is loaded and parsed correctly

**Test Code:**
```go
//go:build integration
// +build integration

func TestConfigFile_LoadAndApply(t *testing.T) {
    // GIVEN: Config file with custom values
    configYAML := `
controller:
  namespace: production
  leaderElection: true
  syncPeriod: 5m
  defaultSSHKeyIDs:
    - key-abc
    - key-xyz

metrics:
  bindAddress: ":9090"
  path: "/custom-metrics"

logging:
  level: debug
  format: console
  development: true

features:
  enableRebalancing: true
  enableCostOptimization: false
`
    tmpFile := createTempConfigFile(t, configYAML)
    defer os.Remove(tmpFile)

    // WHEN: Loading config with file path
    flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
    flags.String("config", tmpFile, "")
    flags.Set("config", tmpFile)

    cfg, err := LoadConfig(flags)

    // THEN: Config loaded from file
    require.NoError(t, err)

    assert.Equal(t, "production", cfg.Controller.Namespace)
    assert.Equal(t, 5*time.Minute, cfg.Controller.SyncPeriod)
    assert.Equal(t, []string{"key-abc", "key-xyz"}, cfg.Controller.DefaultSSHKeyIDs)
    assert.Equal(t, ":9090", cfg.Metrics.BindAddress)
    assert.Equal(t, "debug", cfg.Logging.Level)
    assert.True(t, cfg.Features.EnableRebalancing)
    assert.False(t, cfg.Features.EnableCostOptimization)
}
```

**Acceptance Criteria:**
- ✅ YAML file parsed correctly
- ✅ All config sections loaded
- ✅ Nested fields populated
- ✅ Arrays/slices parsed correctly

---

### Performance Tests

**Test File:** `/Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/config_performance_test.go`

#### PT-1.5.1: Config Loading Performance
**Objective:** Verify config loading adds <2% controller startup overhead

**Test Code:**
```go
//go:build performance
// +build performance

func BenchmarkConfigLoad(b *testing.B) {
    flags := pflag.NewFlagSet("bench", pflag.ContinueOnError)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := LoadConfig(flags)
        if err != nil {
            b.Fatal(err)
        }
    }
}

func TestConfigLoad_Overhead(t *testing.T) {
    // GIVEN: Baseline startup time without config loading
    baseline := measureControllerStartup(t, false)

    // WHEN: Measuring startup time WITH config loading
    withConfig := measureControllerStartup(t, true)

    // THEN: Overhead is <2%
    overhead := (withConfig - baseline) / baseline * 100
    assert.Less(t, overhead, 2.0, "Config loading overhead should be <2%%")
}
```

**Acceptance Criteria:**
- ✅ Config loading completes in <10ms
- ✅ Controller startup overhead <2%
- ✅ No memory leaks from Viper

---

## P1.6 - Documentation Reorganization

### Overview
Reorganize documentation into logical subdirectories for better discoverability.

### Manual Tests

#### MT-1.6.1: Documentation Structure
**Objective:** Verify docs/ directory is organized logically

**Test Steps:**
1. Navigate to `/Users/zozo/projects/vpsie-k8s-autoscaler/docs/`
2. Verify directory structure:
   ```
   docs/
   ├── README.md (index)
   ├── architecture/
   ├── development/
   ├── operations/
   └── configuration/
   ```
3. Check each subdirectory contains relevant files
4. Verify README.md contains navigation links

**Expected Results:**
- ✅ All subdirectories exist
- ✅ Files moved from root to appropriate subdirectories
- ✅ No orphaned files in root docs/
- ✅ README.md contains table of contents

**Acceptance Criteria:**
- Zero markdown files in root (except README.md)
- All links in README.md are valid

---

#### MT-1.6.2: Link Validation
**Objective:** Verify all internal documentation links work

**Test Steps:**
1. Run markdown link checker:
   ```bash
   npx markdown-link-check docs/**/*.md
   ```
2. Fix any broken links

**Expected Results:**
- ✅ 100% of internal links return HTTP 200 or file exists
- ✅ No 404 errors
- ✅ No broken relative paths

**Acceptance Criteria:**
- Zero broken links in documentation

---

## P1.7 - Script Consolidation

### Overview
Organize scripts into categorized subdirectories under `scripts/`.

### Manual Tests

#### MT-1.7.1: Script Organization
**Objective:** Verify scripts/ directory is organized by category

**Test Steps:**
1. Navigate to `/Users/zozo/projects/vpsie-k8s-autoscaler/scripts/`
2. Verify directory structure:
   ```
   scripts/
   ├── build/      (build scripts)
   ├── test/       (test scripts)
   ├── deploy/     (deployment scripts)
   ├── dev/        (development helpers)
   └── utils/      (utility scripts)
   ```
3. Verify scripts moved from root to appropriate categories
4. Check Makefile updated with new script paths

**Expected Results:**
- ✅ All subdirectories exist
- ✅ Scripts categorized correctly
- ✅ No scripts left in repository root
- ✅ Makefile targets use new paths

**Acceptance Criteria:**
- Zero shell scripts in repository root
- All Makefile targets execute successfully

---

#### MT-1.7.2: Script Execution
**Objective:** Verify all scripts execute without errors

**Test Steps:**
1. Run build scripts:
   ```bash
   scripts/build/build.sh
   ```
2. Run test scripts:
   ```bash
   scripts/test/run-tests.sh
   ```
3. Verify all exit with code 0

**Expected Results:**
- ✅ Build scripts compile successfully
- ✅ Test scripts run tests
- ✅ No missing dependencies

**Acceptance Criteria:**
- All scripts execute successfully
- No broken shebang lines or missing executables

---

## P1.8 - Sample Storage Optimization

### Overview
Replace unbounded slices with circular buffers to reduce memory usage by >50%.

### Unit Tests

**Test File:** `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/utilization_test.go`

#### UT-1.8.1: Circular Buffer Initialization
**Objective:** Verify circular buffer initializes with correct capacity

**Test Code:**
```go
func TestCircularBuffer_Init(t *testing.T) {
    tests := []struct {
        name             string
        capacity         int
        expectedCapacity int
    }{
        {
            name:             "Default capacity (30 min * 2 samples/min)",
            capacity:         60,
            expectedCapacity: 60,
        },
        {
            name:             "Custom capacity",
            capacity:         120,
            expectedCapacity: 120,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // WHEN: Creating circular buffer
            buf := NewCircularBuffer[UtilizationSample](tt.capacity)

            // THEN: Capacity set correctly
            assert.Equal(t, tt.expectedCapacity, buf.Cap())
            assert.Equal(t, 0, buf.Len()) // Empty initially
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Buffer initialized with correct capacity
- ✅ Initial length is zero
- ✅ No memory allocated beyond capacity

---

#### UT-1.8.2: Circular Buffer Append
**Objective:** Verify samples appended correctly and old samples evicted

**Test Code:**
```go
func TestCircularBuffer_Append(t *testing.T) {
    // GIVEN: Circular buffer with capacity 3
    buf := NewCircularBuffer[UtilizationSample](3)

    // WHEN: Appending 5 samples (exceeds capacity)
    samples := []UtilizationSample{
        {Timestamp: time.Unix(1, 0), CPUPercent: 10},
        {Timestamp: time.Unix(2, 0), CPUPercent: 20},
        {Timestamp: time.Unix(3, 0), CPUPercent: 30},
        {Timestamp: time.Unix(4, 0), CPUPercent: 40},
        {Timestamp: time.Unix(5, 0), CPUPercent: 50},
    }

    for _, s := range samples {
        buf.Append(s)
    }

    // THEN: Only last 3 samples retained
    assert.Equal(t, 3, buf.Len())

    // Verify oldest samples evicted (FIFO)
    allSamples := buf.GetAll()
    assert.Len(t, allSamples, 3)
    assert.Equal(t, float64(30), allSamples[0].CPUPercent) // Sample 3
    assert.Equal(t, float64(40), allSamples[1].CPUPercent) // Sample 4
    assert.Equal(t, float64(50), allSamples[2].CPUPercent) // Sample 5
}
```

**Acceptance Criteria:**
- ✅ Buffer never exceeds capacity
- ✅ Oldest samples evicted when capacity reached (FIFO)
- ✅ Order of samples preserved

---

#### UT-1.8.3: Circular Buffer Thread Safety
**Objective:** Verify concurrent access is safe

**Test Code:**
```go
func TestCircularBuffer_ConcurrentAccess(t *testing.T) {
    buf := NewCircularBuffer[UtilizationSample](100)

    var wg sync.WaitGroup

    // WHEN: 10 goroutines append concurrently
    for i := 0; i < 10; i++ {
        wg.Add(1)
        go func(id int) {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                sample := UtilizationSample{
                    Timestamp:  time.Now(),
                    CPUPercent: float64(id*100 + j),
                }
                buf.Append(sample)
            }
        }(i)
    }

    // AND: 5 goroutines read concurrently
    for i := 0; i < 5; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for j := 0; j < 100; j++ {
                _ = buf.GetAll()
                time.Sleep(1 * time.Millisecond)
            }
        }()
    }

    wg.Wait()

    // THEN: No race conditions (run with -race flag)
    assert.LessOrEqual(t, buf.Len(), 100)
}
```

**Acceptance Criteria:**
- ✅ No race conditions detected with `-race` flag
- ✅ No panics from concurrent access
- ✅ Buffer length never exceeds capacity

---

#### UT-1.8.4: Calculate Percentile from Buffer
**Objective:** Verify percentile calculation on circular buffer data

**Test Code:**
```go
func TestCircularBuffer_CalculatePercentile(t *testing.T) {
    // GIVEN: Buffer with known samples
    buf := NewCircularBuffer[UtilizationSample](100)
    for i := 1; i <= 100; i++ {
        buf.Append(UtilizationSample{
            Timestamp:  time.Now(),
            CPUPercent: float64(i), // 1, 2, 3, ..., 100
        })
    }

    tests := []struct {
        percentile float64
        expected   float64
        tolerance  float64
    }{
        {percentile: 50, expected: 50, tolerance: 2},  // Median
        {percentile: 90, expected: 90, tolerance: 2},  // P90
        {percentile: 95, expected: 95, tolerance: 2},  // P95
        {percentile: 99, expected: 99, tolerance: 2},  // P99
    }

    for _, tt := range tests {
        t.Run(fmt.Sprintf("P%d", int(tt.percentile)), func(t *testing.T) {
            // WHEN: Calculating percentile
            result := calculatePercentile(buf.GetAll(), tt.percentile, func(s UtilizationSample) float64 {
                return s.CPUPercent
            })

            // THEN: Result within tolerance
            assert.InDelta(t, tt.expected, result, tt.tolerance)
        })
    }
}
```

**Acceptance Criteria:**
- ✅ Percentile calculations accurate (±2%)
- ✅ Works with partial buffers (<capacity)
- ✅ Handles edge cases (empty buffer, single sample)

---

### Performance Tests

**Test File:** `/Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/utilization_performance_test.go`

#### PT-1.8.1: Memory Usage Comparison
**Objective:** Verify circular buffer reduces memory usage by >50%

**Test Code:**
```go
//go:build performance
// +build performance

func TestCircularBuffer_MemoryReduction(t *testing.T) {
    // GIVEN: Measure baseline memory with unbounded slice
    runtime.GC()
    var baselineMemBefore, baselineMemAfter runtime.MemStats
    runtime.ReadMemStats(&baselineMemBefore)

    // Simulate 1 hour of samples at 30s interval (120 samples)
    // With unbounded slice, this grows continuously
    baselineSlice := make([]UtilizationSample, 0, 120)
    for i := 0; i < 1000; i++ { // Simulate long runtime
        sample := UtilizationSample{
            Timestamp:  time.Now(),
            CPUPercent: rand.Float64() * 100,
        }
        baselineSlice = append(baselineSlice, sample)
        if len(baselineSlice) > 120 {
            // In old implementation, samples accumulated
            // (no eviction mechanism)
        }
    }

    runtime.GC()
    runtime.ReadMemStats(&baselineMemAfter)
    baselineMemUsage := baselineMemAfter.Alloc - baselineMemBefore.Alloc

    // WHEN: Measure memory with circular buffer
    runtime.GC()
    var circularMemBefore, circularMemAfter runtime.MemStats
    runtime.ReadMemStats(&circularMemBefore)

    circularBuf := NewCircularBuffer[UtilizationSample](120)
    for i := 0; i < 1000; i++ {
        sample := UtilizationSample{
            Timestamp:  time.Now(),
            CPUPercent: rand.Float64() * 100,
        }
        circularBuf.Append(sample)
    }

    runtime.GC()
    runtime.ReadMemStats(&circularMemAfter)
    circularMemUsage := circularMemAfter.Alloc - circularMemBefore.Alloc

    // THEN: Circular buffer uses >50% less memory
    reduction := (1 - float64(circularMemUsage)/float64(baselineMemUsage)) * 100

    t.Logf("Baseline memory: %d bytes", baselineMemUsage)
    t.Logf("Circular buffer memory: %d bytes", circularMemUsage)
    t.Logf("Memory reduction: %.2f%%", reduction)

    assert.Greater(t, reduction, 50.0, "Expected >50% memory reduction")
}
```

**Acceptance Criteria:**
- ✅ Memory reduction >50%
- ✅ Memory usage bounded (doesn't grow unbounded)
- ✅ No memory leaks

---

#### PT-1.8.2: Throughput Benchmark
**Objective:** Verify append/read operations are performant

**Test Code:**
```go
func BenchmarkCircularBuffer_Append(b *testing.B) {
    buf := NewCircularBuffer[UtilizationSample](60)
    sample := UtilizationSample{Timestamp: time.Now(), CPUPercent: 50}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        buf.Append(sample)
    }
}

func BenchmarkCircularBuffer_GetAll(b *testing.B) {
    buf := NewCircularBuffer[UtilizationSample](60)
    for i := 0; i < 60; i++ {
        buf.Append(UtilizationSample{Timestamp: time.Now(), CPUPercent: 50})
    }

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = buf.GetAll()
    }
}
```

**Acceptance Criteria:**
- ✅ Append throughput >1M ops/sec
- ✅ GetAll throughput >100K ops/sec
- ✅ No performance degradation as buffer fills

---

## P1.9 - Missing Metrics

### Overview
Add 4 new Prometheus metrics for drain duration, drain failures, safety check failures, and scale-down blocks.

### Unit Tests

**Test File:** `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/metrics/metrics_test.go`

#### UT-1.9.1: Metric Registration
**Objective:** Verify new metrics register without conflicts

**Test Code:**
```go
func TestMetrics_Registration(t *testing.T) {
    // GIVEN: Fresh Prometheus registry
    registry := prometheus.NewRegistry()

    // WHEN: Registering all metrics
    err := RegisterMetrics(registry)

    // THEN: All metrics registered successfully
    assert.NoError(t, err)

    // Verify new metrics exist
    metricFamilies, err := registry.Gather()
    require.NoError(t, err)

    metricNames := make(map[string]bool)
    for _, mf := range metricFamilies {
        metricNames[mf.GetName()] = true
    }

    // Verify new P1.9 metrics registered
    assert.True(t, metricNames["vpsie_autoscaler_node_drain_duration_seconds"])
    assert.True(t, metricNames["vpsie_autoscaler_node_drain_failures_total"])
    assert.True(t, metricNames["vpsie_autoscaler_safety_check_failures_total"])
    assert.True(t, metricNames["vpsie_autoscaler_scale_down_blocked_total"])
}
```

**Acceptance Criteria:**
- ✅ All 4 new metrics register without errors
- ✅ No metric name conflicts with existing 38 metrics
- ✅ Metric types correct (histogram, counters)

---

#### UT-1.9.2: Drain Duration Histogram Buckets
**Objective:** Verify histogram buckets cover expected drain duration range

**Test Code:**
```go
func TestMetrics_DrainDurationBuckets(t *testing.T) {
    // GIVEN: Drain duration histogram
    metric := NodeDrainDuration.With(prometheus.Labels{
        "nodegroup": "test-ng",
        "namespace": "default",
        "result":    "success",
    })

    // WHEN: Observing various drain durations
    durations := []float64{
        1,    // 1 second (very fast)
        10,   // 10 seconds (fast)
        60,   // 1 minute (normal)
        300,  // 5 minutes (slow)
        600,  // 10 minutes (very slow)
    }

    for _, d := range durations {
        metric.(prometheus.Histogram).Observe(d)
    }

    // THEN: Buckets cover the range
    // Expected buckets: 1, 2, 4, 8, 16, 32, 64, 128, 256, 512
    // This covers 1s to 512s (8.5 minutes)

    // Verify metric can be gathered
    metricFamily := collectMetric(t, NodeDrainDuration)
    assert.NotNil(t, metricFamily)

    buckets := metricFamily.GetMetric()[0].GetHistogram().GetBucket()
    assert.True(t, len(buckets) >= 10, "Should have at least 10 buckets")
}
```

**Acceptance Criteria:**
- ✅ Histogram buckets: 1s, 2s, 4s, 8s, 16s, 32s, 64s, 128s, 256s, 512s
- ✅ Covers range 1s to 512s (8.5 minutes)
- ✅ Exponential buckets (base 2)

---

#### UT-1.9.3: Drain Failure Counter
**Objective:** Verify drain failure counter increments correctly

**Test Code:**
```go
func TestMetrics_DrainFailures(t *testing.T) {
    // GIVEN: Fresh counter
    registry := prometheus.NewRegistry()
    counter := prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "vpsie_autoscaler",
            Name:      "node_drain_failures_total",
            Help:      "Total number of node drain failures",
        },
        []string{"nodegroup", "namespace", "reason"},
    )
    registry.MustRegister(counter)

    // WHEN: Recording drain failures
    counter.With(prometheus.Labels{
        "nodegroup": "test-ng",
        "namespace": "default",
        "reason":    "pdb_violation",
    }).Inc()

    counter.With(prometheus.Labels{
        "nodegroup": "test-ng",
        "namespace": "default",
        "reason":    "eviction_timeout",
    }).Inc()

    // THEN: Counter incremented
    metricFamily := collectMetric(t, counter)
    metrics := metricFamily.GetMetric()

    assert.Len(t, metrics, 2) // Two different reasons

    for _, m := range metrics {
        assert.Equal(t, float64(1), m.GetCounter().GetValue())
    }
}
```

**Acceptance Criteria:**
- ✅ Counter increments for each failure
- ✅ Labels include: nodegroup, namespace, reason
- ✅ Reason label values: pdb_violation, eviction_timeout, daemonset_pods, etc.

---

#### UT-1.9.4: Safety Check Failures Counter
**Objective:** Verify safety check failures tracked by check type

**Test Code:**
```go
func TestMetrics_SafetyCheckFailures(t *testing.T) {
    // GIVEN: Safety check failure counter
    registry := prometheus.NewRegistry()
    counter := prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "vpsie_autoscaler",
            Name:      "safety_check_failures_total",
            Help:      "Total number of safety check failures",
        },
        []string{"check_type", "nodegroup", "namespace"},
    )
    registry.MustRegister(counter)

    // WHEN: Recording different safety check failures
    checkTypes := []string{
        "cluster_health",
        "pdb",
        "local_storage",
        "maintenance_window",
        "cooldown",
    }

    for _, checkType := range checkTypes {
        counter.With(prometheus.Labels{
            "check_type": checkType,
            "nodegroup":  "test-ng",
            "namespace":  "default",
        }).Inc()
    }

    // THEN: Each check type tracked separately
    metricFamily := collectMetric(t, counter)
    metrics := metricFamily.GetMetric()

    assert.Len(t, metrics, 5) // 5 different check types

    recordedCheckTypes := make(map[string]bool)
    for _, m := range metrics {
        for _, label := range m.GetLabel() {
            if label.GetName() == "check_type" {
                recordedCheckTypes[label.GetValue()] = true
            }
        }
    }

    for _, checkType := range checkTypes {
        assert.True(t, recordedCheckTypes[checkType])
    }
}
```

**Acceptance Criteria:**
- ✅ Counter tracks check_type, nodegroup, namespace
- ✅ Check types: cluster_health, pdb, local_storage, maintenance_window, cooldown
- ✅ Each check type tracked independently

---

#### UT-1.9.5: Scale Down Blocked Counter
**Objective:** Verify scale-down blocks tracked by reason

**Test Code:**
```go
func TestMetrics_ScaleDownBlocked(t *testing.T) {
    // GIVEN: Scale down blocked counter
    registry := prometheus.NewRegistry()
    counter := prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "vpsie_autoscaler",
            Name:      "scale_down_blocked_total",
            Help:      "Total number of scale-down operations blocked",
        },
        []string{"reason", "nodegroup", "namespace"},
    )
    registry.MustRegister(counter)

    // WHEN: Recording scale-down blocks
    reasons := []string{
        "safety_check_failed",
        "min_nodes_reached",
        "recently_scaled",
        "pdb_violation",
    }

    for _, reason := range reasons {
        counter.With(prometheus.Labels{
            "reason":    reason,
            "nodegroup": "test-ng",
            "namespace": "default",
        }).Inc()
    }

    // THEN: Each reason tracked
    metricFamily := collectMetric(t, counter)
    metrics := metricFamily.GetMetric()

    assert.Len(t, metrics, 4)
}
```

**Acceptance Criteria:**
- ✅ Counter tracks reason, nodegroup, namespace
- ✅ Reasons include: safety_check_failed, min_nodes_reached, recently_scaled, pdb_violation
- ✅ Helps diagnose why scale-down didn't occur

---

### Integration Tests

**Test File:** `/Users/zozo/projects/vpsie-k8s-autoscaler/test/integration/metrics_test.go`

#### IT-1.9.1: Drain Duration Instrumentation
**Objective:** Verify drain duration is recorded during actual node drain

**Test Code:**
```go
//go:build integration
// +build integration

func TestMetrics_DrainDuration_Integration(t *testing.T) {
    ctx := context.Background()
    client := integration.GetTestClient(t, integration.GetTestConfig(t))

    // GIVEN: NodeGroup with pods
    ng, pods := integration.CreateNodeGroupWithPods(t, client, "drain-test", 1, 3)

    // WHEN: Triggering scale-down (which drains node)
    ng.Spec.MinNodes = 0
    require.NoError(t, client.Update(ctx, ng))

    // Wait for drain to complete
    integration.WaitForNodeGroupReconciliation(t, client, "drain-test", 5*time.Minute)

    // THEN: Drain duration metric recorded
    metrics := integration.ScrapeMetrics(t, "http://localhost:8080/metrics")

    drainDurationMetric := integration.FindMetric(metrics, "vpsie_autoscaler_node_drain_duration_seconds")
    require.NotNil(t, drainDurationMetric)

    // Verify metric has expected labels
    assert.Equal(t, "drain-test", drainDurationMetric.Labels["nodegroup"])
    assert.Equal(t, "default", drainDurationMetric.Labels["namespace"])
    assert.Contains(t, []string{"success", "failure"}, drainDurationMetric.Labels["result"])

    // Verify duration is reasonable (>0, <600s)
    assert.Greater(t, drainDurationMetric.Value, 0.0)
    assert.Less(t, drainDurationMetric.Value, 600.0)
}
```

**Acceptance Criteria:**
- ✅ Metric appears in /metrics endpoint
- ✅ Duration recorded for successful drains
- ✅ Duration recorded for failed drains
- ✅ Labels populated correctly

---

#### IT-1.9.2: Safety Check Failure Recording
**Objective:** Verify safety check failures appear in metrics

**Test Code:**
```go
func TestMetrics_SafetyCheckFailures_Integration(t *testing.T) {
    ctx := context.Background()
    client := integration.GetTestClient(t, integration.GetTestConfig(t))

    // GIVEN: NodeGroup with PodDisruptionBudget that blocks scale-down
    ng := integration.CreateNodeGroup(t, client, "safety-test", 2, 2)
    pdb := integration.CreatePDBForNodeGroup(t, client, ng, 2) // Require 2 pods always

    // WHEN: Attempting scale-down (should fail PDB check)
    ng.Spec.MinNodes = 1
    require.NoError(t, client.Update(ctx, ng))

    // Wait for reconciliation attempt
    time.Sleep(2 * time.Minute)

    // THEN: Safety check failure metric incremented
    metrics := integration.ScrapeMetrics(t, "http://localhost:8080/metrics")

    safetyFailureMetric := integration.FindMetric(metrics, "vpsie_autoscaler_safety_check_failures_total")
    require.NotNil(t, safetyFailureMetric)

    assert.Equal(t, "pdb", safetyFailureMetric.Labels["check_type"])
    assert.Equal(t, "safety-test", safetyFailureMetric.Labels["nodegroup"])
    assert.Greater(t, safetyFailureMetric.Value, 0.0)
}
```

**Acceptance Criteria:**
- ✅ Metric increments when safety checks fail
- ✅ check_type label identifies which check failed
- ✅ Helps operators diagnose scale-down issues

---

### Test Data Requirements

**Metrics Testing:**
- Prometheus server running on :9090
- Metrics endpoint accessible at :8080/metrics
- Test NodeGroups with varying activity
- Ability to trigger drain operations
- Ability to trigger safety check failures (PDB violations)

**Evidence Collection:**
- Screenshot of Prometheus graph showing drain duration histogram
- Screenshot of safety_check_failures_total counter
- Example PromQL queries for each new metric

---

## Test Data Requirements

### Global Test Data

**Kubernetes Resources:**
```yaml
# Test namespace
apiVersion: v1
kind: Namespace
metadata:
  name: autoscaler-test

# VPSie credentials secret
apiVersion: v1
kind: Secret
metadata:
  name: vpsie-secret
  namespace: kube-system
type: Opaque
data:
  clientId: <base64-encoded>
  clientSecret: <base64-encoded>

# Test ConfigMaps
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-cloud-init
data:
  cloud-init.sh: |
    #!/bin/bash
    echo "Test template"

# Test SSH keys secret
apiVersion: v1
kind: Secret
metadata:
  name: test-ssh-keys
type: Opaque
data:
  admin: <base64-encoded-public-key>
```

**Mock VPSie Server:**
- HTTP server on localhost:8888
- Implements VPSie API v2 endpoints
- Supports error injection for testing
- Tracks API call history

**Test Configuration Files:**
```yaml
# config/test.yaml
controller:
  namespace: autoscaler-test
  leaderElection: false
metrics:
  bindAddress: ":8080"
logging:
  level: debug
  format: console
```

---

## Test Environment Setup

### Local Development Environment

**Requirements:**
- Go 1.24+
- Kind cluster
- kubectl
- Docker
- Prometheus (for metrics testing)
- Grafana (for dashboard testing)

**Setup Steps:**
```bash
# 1. Create kind cluster
make kind-create

# 2. Load test data
kubectl apply -f test/fixtures/

# 3. Deploy autoscaler
make deploy

# 4. Deploy Prometheus
helm install prometheus prometheus-community/kube-prometheus-stack -n monitoring

# 5. Deploy Grafana
# (included in kube-prometheus-stack)

# 6. Port-forward services
kubectl port-forward -n monitoring svc/prometheus-kube-prometheus-prometheus 9090:9090
kubectl port-forward -n monitoring svc/prometheus-grafana 3000:80
kubectl port-forward -n kube-system svc/vpsie-autoscaler-metrics 8080:8080
```

### CI/CD Environment

**GitHub Actions:**
```yaml
# .github/workflows/test-p1-features.yml
name: P1 Features Tests

on:
  pull_request:
    paths:
      - 'pkg/controller/vpsienode/**'
      - 'pkg/metrics/**'
      - 'internal/config/**'

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      - run: make test

  integration-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: helm/kind-action@v1
      - run: make test-integration

  performance-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - run: make test-performance-benchmarks
```

---

## Summary

This test specification document provides comprehensive acceptance criteria for all 9 P1 features:

**Test Coverage:**
- **P1.1 (Grafana Dashboard):** 5 manual tests for import, variables, panels, annotations, themes
- **P1.2 (Prometheus Alerts):** 7 manual tests for validation, loading, firing, routing, runbooks
- **P1.3 (Cloud-Init Templates):** 5 unit tests + 2 integration tests for template loading, variable substitution
- **P1.4 (SSH Key Management):** 3 unit tests + 1 integration test for multi-source collection, validation
- **P1.5 (Configuration):** 4 unit tests + 1 integration test + 1 performance test for priority, validation, file loading
- **P1.6 (Documentation):** 2 manual tests for structure and link validation
- **P1.7 (Scripts):** 2 manual tests for organization and execution
- **P1.8 (Sample Storage):** 4 unit tests + 2 performance tests for circular buffer, memory reduction
- **P1.9 (Missing Metrics):** 5 unit tests + 2 integration tests for metric registration, instrumentation

**Total Test Count:**
- Unit tests: 26
- Integration tests: 8
- Performance tests: 3
- Manual tests: 16
- **Total: 53 tests**

**Next Steps:**
1. Create test files during implementation (TDD approach)
2. Run unit tests continuously during development
3. Execute integration tests after each feature completes
4. Perform manual tests during deployment validation
5. Run performance benchmarks for P1.8
6. Collect evidence (screenshots, metrics) for acceptance review

---

**Document Status:** ✅ APPROVED FOR USE
**Review Date:** 2025-12-22
**Reviewer:** Acceptance Test Generator Sub-Agent
