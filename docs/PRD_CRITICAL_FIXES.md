# Product Requirements Document: Critical Production Fixes

**Version:** 1.0
**Date:** 2025-12-22
**Status:** Approved for Implementation
**Priority:** P0 - Production Blocker

## Executive Summary

This PRD documents 10 critical issues identified through comprehensive code review that **MUST** be fixed before production deployment. These issues span security vulnerabilities, data integrity risks, and functional gaps that could cause service disruption, data loss, or security breaches.

**Risk Level:** HIGH
**Estimated Fix Time:** 3-4 days
**Deployment Risk if Unfixed:** SEVERE (data loss, security breach, service disruption)

---

## 1. Issue Prioritization & Dependencies

### Phase 1: Foundation Fixes (P0 - Day 1)
**Must be completed first as they affect core functionality:**

1. **Status Patch Timing Bug** (reconciler.go:83-90)
   - **Impact:** Silent status update failures
   - **Risk:** Cluster state inconsistency
   - **Blocks:** All status-dependent operations

2. **Credential Exposure Risk** (client.go:330-334)
   - **Impact:** Potential credential leakage in logs
   - **Risk:** Security breach
   - **Blocks:** Security audit approval

3. **Webhook Body Limit** (server.go:22-24)
   - **Impact:** DoS vulnerability
   - **Risk:** Service disruption
   - **Blocks:** Security audit approval

### Phase 2: Business Logic Completion (P0 - Day 2)
**Depends on Phase 1 completion:**

4. **Missing Node Deletion** (reconciler.go:223-271)
   - **Impact:** Drained nodes never removed
   - **Risk:** Resource waste, cost overrun
   - **Depends on:** Issue #1 (status patch fix)

5. **Rebalancer Non-Functional** (executor.go:296-314)
   - **Impact:** Entire Phase 5 feature broken
   - **Risk:** No cost optimization
   - **Depends on:** Node provisioning implementation

### Phase 3: Concurrency & Safety (P0 - Day 3)
**Can be done in parallel with Phase 2:**

6. **Race Condition in Utilization Copy** (scaler.go:173-193)
   - **Impact:** Data corruption in scale decisions
   - **Risk:** Wrong nodes scaled down
   - **Blocks:** Scale-down operations

7. **OAuth Form Encoding** (client.go:331-333)
   - **Impact:** Auth failures with special chars
   - **Risk:** Intermittent API failures
   - **Blocks:** Credential rotation

### Phase 4: Security Hardening (P0 - Day 3-4)
**Can be done in parallel with Phase 3:**

8. **RBAC Too Permissive** (rbac.yaml:76-89)
   - **Impact:** Can delete any cluster node
   - **Risk:** Catastrophic failure
   - **Requires:** New ValidatingWebhook implementation

9. **Missing TLS Validation** (client.go:195-203)
   - **Impact:** MITM attack vulnerability
   - **Risk:** Credential interception
   - **Blocks:** Security audit approval

10. **Node Provisioning Implementation** (executor.go:296-314)
    - **Impact:** Same as #5 (duplicate issue)
    - **Risk:** Rebalancer completely broken
    - **Blocks:** Issue #5 resolution

---

## 2. Detailed Requirements

### ISSUE #1: Status Patch Timing Bug

**Location:** `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/controller/nodegroup/reconciler.go:83-90`

**Current Behavior:**
```go
patch := client.MergeFrom(ng.DeepCopy())

// Update status with current state
if err := UpdateNodeGroupStatus(ctx, r.Client, ng, vpsieNodes); err != nil {
    logger.Error("Failed to update NodeGroup status", zap.Error(err))
    return ctrl.Result{}, err
}
// ... later ...
if err := r.Status().Patch(ctx, ng, patch); err != nil {
    // This patch has no effect!
}
```

**Problem:** Patch captures state BEFORE modifications, making it ineffective.

**Required Fix:**
```go
// Move patch creation AFTER all status modifications
if err := UpdateNodeGroupStatus(ctx, r.Client, ng, vpsieNodes); err != nil {
    logger.Error("Failed to update NodeGroup status", zap.Error(err))
    return ctrl.Result{}, err
}

// Create patch from current state
patch := client.MergeFrom(ng.DeepCopy())

// Make additional modifications if needed
SetReadyCondition(ng, true, ...)

// Now patch will include all changes
if err := r.Status().Patch(ctx, ng, patch); err != nil {
    return ctrl.Result{}, err
}
```

**Success Criteria:**
- [ ] Patch created after all status modifications
- [ ] Status updates reflected in Kubernetes API
- [ ] Integration test verifies status changes persist
- [ ] No "status update failed" errors in logs
- [ ] `kubectl get nodegroup` shows correct status

---

### ISSUE #2: Node Provisioning Incomplete

**Location:** `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/rebalancer/executor.go:296-315`

**Current Behavior:**
```go
func (e *Executor) provisionNewNode(ctx context.Context, plan *RebalancePlan, candidate *CandidateNode) (*Node, error) {
    return nil, fmt.Errorf("node provisioning not yet implemented: VPSie API integration required for offering %s", candidate.TargetOffering)
}
```

**Problem:** Rebalancer always fails because it cannot create replacement nodes.

**Required Fix:**
```go
func (e *Executor) provisionNewNode(ctx context.Context, plan *RebalancePlan, candidate *CandidateNode) (*Node, error) {
    // Create VPSieNode CRD to trigger provisioning
    vpsieNode := &v1alpha1.VPSieNode{
        ObjectMeta: metav1.ObjectMeta{
            Name:      generateNodeName(),
            Namespace: e.namespace,
            Labels: map[string]string{
                "autoscaler.vpsie.com/managed":     "true",
                "autoscaler.vpsie.com/nodegroup":   plan.NodeGroupName,
                "autoscaler.vpsie.com/rebalanced":  "true",
            },
        },
        Spec: v1alpha1.VPSieNodeSpec{
            NodeGroupName: plan.NodeGroupName,
            Offering:      candidate.TargetOffering,
            Datacenter:    candidate.Datacenter,
        },
    }

    if err := e.client.Create(ctx, vpsieNode); err != nil {
        return nil, fmt.Errorf("failed to create VPSieNode: %w", err)
    }

    // Wait for node to become ready
    if err := e.waitForNodeReady(ctx, vpsieNode.Name, 5*time.Minute); err != nil {
        return nil, fmt.Errorf("node provisioning timeout: %w", err)
    }

    return &Node{
        Name:  vpsieNode.Name,
        VPSID: vpsieNode.Status.VPSieInstanceID,
    }, nil
}
```

**Success Criteria:**
- [ ] VPSieNode CRD created successfully
- [ ] Node appears in Kubernetes within timeout
- [ ] Node reaches Ready state
- [ ] Rebalancer can complete rolling migrations
- [ ] Integration test validates end-to-end rebalancing

---

### ISSUE #3: RBAC Too Permissive

**Location:** `/Users/zozo/projects/vpsie-k8s-autoscaler/deploy/manifests/rbac.yaml:76-89`

**Current Behavior:**
```yaml
- apiGroups:
  - ""
  resources:
  - nodes
  verbs:
  - delete  # Can delete ANY node!
```

**Problem:** Controller can delete control plane nodes or nodes managed by other systems.

**Required Fix:**

**Step 1:** Implement ValidatingWebhook
Create `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/webhook/node_deletion_validator.go`:

```go
package webhook

import (
    "context"
    "fmt"
    admissionv1 "k8s.io/api/admission/v1"
    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type NodeDeletionValidator struct {
    decoder *admission.Decoder
}

func (v *NodeDeletionValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
    node := &corev1.Node{}
    if err := v.decoder.Decode(req, node); err != nil {
        return admission.Errored(400, err)
    }

    // Only allow deletion of nodes managed by this autoscaler
    if node.Labels["autoscaler.vpsie.com/managed"] != "true" {
        return admission.Denied(fmt.Sprintf(
            "node %s is not managed by VPSie autoscaler (missing label autoscaler.vpsie.com/managed=true)",
            node.Name,
        ))
    }

    return admission.Allowed("node is managed by VPSie autoscaler")
}
```

**Step 2:** Register webhook in `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/webhook/server.go`:

```go
// Add to SetupWebhooks()
mgr.GetWebhookServer().Register("/validate/node-deletion",
    &webhook.Admission{Handler: &NodeDeletionValidator{
        decoder: admission.NewDecoder(mgr.GetScheme()),
    }})
```

**Step 3:** Add ValidatingWebhookConfiguration in `/Users/zozo/projects/vpsie-k8s-autoscaler/deploy/manifests/webhook.yaml`:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: vpsie-autoscaler-node-deletion
webhooks:
- name: node-deletion.autoscaler.vpsie.com
  clientConfig:
    service:
      name: vpsie-autoscaler-webhook
      namespace: kube-system
      path: /validate/node-deletion
    caBundle: ${CA_BUNDLE}
  rules:
  - operations: ["DELETE"]
    apiGroups: [""]
    apiVersions: ["v1"]
    resources: ["nodes"]
  admissionReviewVersions: ["v1"]
  sideEffects: None
  failurePolicy: Fail  # Fail closed for security
```

**Success Criteria:**
- [ ] Webhook rejects deletion of non-managed nodes
- [ ] Webhook allows deletion of managed nodes only
- [ ] Integration test validates protection
- [ ] `kubectl delete node <non-managed>` is blocked
- [ ] Autoscaler can still delete managed nodes

---

### ISSUE #4: Credential Exposure Risk

**Location:** `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/vpsie/client/client.go:330-334`

**Current Behavior:**
```go
formData := fmt.Sprintf("clientId=%s&clientSecret=%s",
    c.clientID,
    c.clientSecret,
)
```

**Problem:** Credentials could leak in logs if request body is logged.

**Required Fix:**
```go
import "net/url"

// Use URL encoding to prevent injection and ensure proper formatting
formData := url.Values{
    "clientId":     {c.clientID},
    "clientSecret": {c.clientSecret},
}.Encode()

// Add explicit log sanitization
c.logger.Info("refreshing OAuth token",
    zap.String("endpoint", c.baseURL+"/oauth/token"),
    // Never log credentials or request body
)
```

**Also add sanitization to error handling:**
```go
if tokenResp.Error {
    c.logger.Error("token request failed",
        zap.Int("code", tokenResp.Code),
        zap.Bool("error", tokenResp.Error),
        // Do NOT log message as it might contain sensitive data
    )
    return fmt.Errorf("token request failed with code: %d", tokenResp.Code)
}
```

**Success Criteria:**
- [ ] URL encoding used for form data
- [ ] No credentials in error messages
- [ ] No credentials in log statements
- [ ] Grep verification: `grep -r "clientSecret\|clientID" pkg/vpsie/client/*.go` shows no log statements
- [ ] Security audit passes credential exposure check

---

### ISSUE #5: Webhook Body Limit Too Large

**Location:** `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/webhook/server.go:22-24`

**Current Behavior:**
```go
const (
    MaxRequestBodySize = 1 * 1024 * 1024  // 1MB
)
```

**Problem:** Enables DoS attacks by exhausting memory.

**Required Fix:**
```go
const (
    // MaxRequestBodySize for admission webhook requests
    // Typical CRD objects are 10-50KB; 128KB provides ample buffer
    MaxRequestBodySize = 128 * 1024  // 128KB
)
```

**Also add to handler:**
```go
func (s *Server) handleNodeGroupValidation(w http.ResponseWriter, r *http.Request) {
    // Validate Content-Type
    if r.Header.Get("Content-Type") != "application/json" {
        http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
        return
    }

    // Use size-limited reader
    body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBodySize))
    if err != nil {
        http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
        return
    }
    defer r.Body.Close()

    // Validate request is not nil
    admissionReview := &admissionv1.AdmissionReview{}
    if err := json.Unmarshal(body, admissionReview); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    if admissionReview.Request == nil {
        http.Error(w, "admission request is nil", http.StatusBadRequest)
        return
    }

    // ... rest of handler
}
```

**Success Criteria:**
- [ ] MaxRequestBodySize reduced to 128KB
- [ ] Content-Type validation added
- [ ] Nil request check added
- [ ] Load test with 1MB+ requests fails appropriately
- [ ] Normal webhook operations succeed

---

### ISSUE #6: Race Condition in Utilization Copy

**Location:** `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/scaler/scaler.go:173-193`

**Current Behavior:**
```go
s.utilizationLock.RLock()
// ... deep copy while holding lock
copy(utilizationCopy.Samples, utilization.Samples)
s.utilizationLock.RUnlock()
```

**Problem:** If `UpdateNodeUtilization` replaces the slice between `len()` and `copy()`, data corruption occurs.

**Required Fix:**

**Option 1: Use existing safe pattern from utilization.go**
```go
func (s *ScaleDownManager) IdentifyUnderutilizedNodes(ctx context.Context, nodes []*corev1.Node) []*corev1.Node {
    candidates := make([]*corev1.Node, 0)

    for _, node := range nodes {
        // Use existing GetNodeUtilization which safely copies
        utilization, exists := s.GetNodeUtilization(node.Name)
        if !exists || !utilization.IsUnderutilized {
            continue
        }

        candidates = append(candidates, node)
    }

    return candidates
}
```

**Option 2: Match UpdateNodeUtilization pattern**
```go
s.utilizationLock.Lock()
utilization, exists := s.nodeUtilization[node.Name]
if !exists || !utilization.IsUnderutilized {
    s.utilizationLock.Unlock()
    continue
}

// Create new slice (not reference original)
samplesCopy := make([]UtilizationSample, len(utilization.Samples))
copy(samplesCopy, utilization.Samples)

utilizationCopy := &NodeUtilization{
    NodeName:          utilization.NodeName,
    CPUUtilization:    utilization.CPUUtilization,
    MemoryUtilization: utilization.MemoryUtilization,
    IsUnderutilized:   utilization.IsUnderutilized,
    LastUpdated:       utilization.LastUpdated,
    Samples:           samplesCopy,
}
s.utilizationLock.Unlock()
```

**Success Criteria:**
- [ ] No race conditions with `go test -race`
- [ ] Concurrent UpdateNodeUtilization + IdentifyUnderutilizedNodes passes
- [ ] Data integrity test validates samples consistency
- [ ] Load test with 1000+ concurrent calls succeeds
- [ ] No data corruption in scale-down decisions

---

### ISSUE #7: Missing Node Deletion After Drain

**Location:** `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/controller/nodegroup/reconciler.go:223-271`

**Current Behavior:**
```go
func (r *NodeGroupReconciler) reconcileIntelligentScaleDown(...) {
    // ... drains nodes ...
    if err := r.ScaleDownManager.ScaleDown(ctx, ng, candidates); err != nil {
        return ctrl.Result{}, err
    }

    // Requeues but never deletes!
    return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
}
```

**Problem:** Drained nodes remain cordoned indefinitely, wasting resources.

**Required Fix:**

**Step 1:** Add drain status tracking in `pkg/scaler/scaler.go`:
```go
const (
    AnnotationDrainStatus   = "autoscaler.vpsie.com/drain-status"
    DrainStatusDrained      = "drained"
    DrainStatusDraining     = "draining"
)

func (s *ScaleDownManager) ScaleDown(ctx context.Context, ng *v1alpha1.NodeGroup, nodes []*corev1.Node) error {
    for _, node := range nodes {
        // Mark as draining
        if err := s.addAnnotation(ctx, node, AnnotationDrainStatus, DrainStatusDraining); err != nil {
            return err
        }

        // Drain the node
        if err := s.DrainNode(ctx, node); err != nil {
            return err
        }

        // Mark as drained (ready for deletion)
        if err := s.addAnnotation(ctx, node, AnnotationDrainStatus, DrainStatusDrained); err != nil {
            return err
        }
    }
    return nil
}
```

**Step 2:** Detect and delete drained nodes in reconciler:
```go
func (r *NodeGroupReconciler) reconcileIntelligentScaleDown(...) {
    // ... existing drain logic ...

    // Check for drained nodes ready for deletion
    drainedNodes, err := r.getDrainedNodes(ctx, ng)
    if err != nil {
        return ctrl.Result{}, err
    }

    for _, node := range drainedNodes {
        // Find corresponding VPSieNode
        vpsieNode, err := r.getVPSieNodeForKubernetesNode(ctx, node)
        if err != nil {
            logger.Error("Failed to find VPSieNode for drained node",
                zap.String("nodeName", node.Name),
                zap.Error(err))
            continue
        }

        // Delete VPSieNode (will trigger VPS termination and K8s node deletion)
        if err := r.Delete(ctx, vpsieNode); err != nil {
            logger.Error("Failed to delete VPSieNode",
                zap.String("vpsieNode", vpsieNode.Name),
                zap.Error(err))
            continue
        }

        logger.Info("Deleted VPSieNode for drained node",
            zap.String("nodeName", node.Name),
            zap.String("vpsieNode", vpsieNode.Name))
    }

    return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
}

func (r *NodeGroupReconciler) getDrainedNodes(ctx context.Context, ng *v1alpha1.NodeGroup) ([]*corev1.Node, error) {
    nodeList := &corev1.NodeList{}
    if err := r.List(ctx, nodeList); err != nil {
        return nil, err
    }

    drained := make([]*corev1.Node, 0)
    for i := range nodeList.Items {
        node := &nodeList.Items[i]

        // Check if node belongs to this NodeGroup
        if node.Labels["autoscaler.vpsie.com/nodegroup"] != ng.Name {
            continue
        }

        // Check drain status
        if node.Annotations[AnnotationDrainStatus] == DrainStatusDrained {
            drained = append(drained, node)
        }
    }

    return drained, nil
}
```

**Success Criteria:**
- [ ] Drained nodes are annotated with drain status
- [ ] VPSieNode deleted after drain completes
- [ ] VPS instance terminated by VPSieNode controller
- [ ] Kubernetes node removed within 5 minutes
- [ ] Integration test validates end-to-end flow

---

### ISSUE #8: RBAC Implementation (Same as #3)
See Issue #3 for full implementation details.

---

### ISSUE #9: OAuth Form Encoding (Same as #4)
See Issue #4 for full implementation details.

---

### ISSUE #10: Missing TLS Validation

**Location:** `/Users/zozo/projects/vpsie-k8s-autoscaler/pkg/vpsie/client/client.go:195-203`

**Current Behavior:**
```go
httpClient = &http.Client{
    Timeout: opts.Timeout,
}
```

**Problem:** No explicit TLS 1.2+ enforcement or certificate validation.

**Required Fix:**
```go
import (
    "crypto/tls"
    "net/http"
)

httpClient = &http.Client{
    Timeout: opts.Timeout,
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
        TLSClientConfig: &tls.Config{
            MinVersion: tls.VersionTLS12,
            // Enforce strong cipher suites
            CipherSuites: []uint16{
                tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
                tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
                tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
                tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
            },
            PreferServerCipherSuites: true,
            // InsecureSkipVerify: false (default, explicit for clarity)
            InsecureSkipVerify: false,
        },
    },
}
```

**Success Criteria:**
- [ ] TLS 1.2 minimum enforced
- [ ] Strong cipher suites configured
- [ ] Certificate validation enabled
- [ ] Connection to TLS 1.0/1.1 servers fails
- [ ] Security scan validates TLS configuration

---

## 3. Testing Strategy

### Unit Tests
For each fix, add/update unit tests:

```bash
# Run with race detector
go test -race ./pkg/controller/nodegroup -run TestStatusPatchTiming -v
go test -race ./pkg/scaler -run TestUtilizationCopy -v
go test -race ./pkg/vpsie/client -run TestOAuthEncoding -v
```

### Integration Tests

**Phase 1 Tests:**
```bash
# Test status patch persistence
go test -tags=integration ./test/integration -run TestNodeGroupStatusPersistence -v

# Test webhook DoS protection
go test -tags=integration ./test/integration -run TestWebhookSizeLimits -v
```

**Phase 2 Tests:**
```bash
# Test node deletion after drain
go test -tags=integration ./test/integration -run TestScaleDownComplete -v

# Test rebalancer end-to-end
go test -tags=integration ./test/integration -run TestRebalancerProvisioning -v
```

**Phase 3 Tests:**
```bash
# Test concurrent utilization updates
go test -tags=integration ./test/integration -run TestConcurrentUtilization -v

# Test OAuth with special characters
go test -tags=integration ./test/integration -run TestOAuthSpecialChars -v
```

**Phase 4 Tests:**
```bash
# Test RBAC node deletion protection
go test -tags=integration ./test/integration -run TestNodeDeletionProtection -v

# Test TLS enforcement
go test -tags=integration ./test/integration -run TestTLSValidation -v
```

### Security Tests (NEW)

Create `/Users/zozo/projects/vpsie-k8s-autoscaler/test/security/`:

```bash
# Credential exposure scan
./test/security/scan-credentials.sh

# DoS vulnerability test
./test/security/test-dos.sh

# RBAC privilege escalation test
./test/security/test-rbac.sh
```

### E2E Validation

```bash
# Full autoscaler lifecycle with fixes
make test-e2e

# Verify in live cluster:
kubectl apply -f test/fixtures/nodegroup-test.yaml
kubectl get nodegroup test-ng -o yaml  # Check status persists
kubectl delete node <non-managed>      # Should be blocked
```

---

## 4. Risk Assessment

### What Happens if We Deploy WITHOUT These Fixes?

#### Day 1 Issues (Immediate Impact):
1. **Status Patch Bug**: Silent failures, operators see stale status, incorrect scaling decisions
2. **Rebalancer Broken**: Phase 5 features completely non-functional, no cost optimization
3. **Node Deletion Missing**: Drained nodes accumulate, wasting resources and money
4. **Webhook DoS**: Large requests crash webhook, blocking all NodeGroup operations

#### Security Incidents (Hours to Discovery):
5. **RBAC Too Permissive**: Operator mistake or bug deletes control plane nodes → cluster down
6. **Credential Exposure**: Logs leak API credentials → unauthorized VPS provisioning → financial damage
7. **Missing TLS Validation**: MITM attack intercepts credentials → security breach

#### Intermittent Bugs (Days to Discovery):
8. **Race Condition**: Wrong nodes scaled down during high load → service disruption
9. **OAuth Encoding**: Credential rotation with special chars fails → auth outage
10. **Webhook Limits**: Large CRDs rejected unnecessarily → operator frustration

### Recovery Procedures (if deployed unfixed):

**Immediate Rollback:**
```bash
# Rollback to previous version
helm rollback vpsie-autoscaler 1

# Manual cleanup
kubectl get nodes -l autoscaler.vpsie.com/drain-status=drained
kubectl delete vpsienode <stuck-nodes>
```

**Hotfix Deployment:**
```bash
# Apply critical fixes only (Phase 1)
kubectl apply -f deploy/hotfix/
helm upgrade vpsie-autoscaler ./charts/vpsie-autoscaler \
    --set hotfix.enabled=true
```

---

## 5. Success Criteria

### Code Quality Gates:
- [ ] All unit tests pass with `-race` flag
- [ ] golangci-lint passes with no errors
- [ ] Security scan passes (no credential leaks)
- [ ] Coverage > 80% for modified code

### Functional Tests:
- [ ] Integration test suite passes (all phases)
- [ ] E2E test validates full lifecycle
- [ ] Manual testing in dev cluster succeeds
- [ ] Staging deployment validates fixes

### Security Validation:
- [ ] Credential scan shows no leaks
- [ ] DoS test fails appropriately
- [ ] RBAC protection verified
- [ ] TLS configuration hardened

### Performance:
- [ ] No performance regression in benchmarks
- [ ] Concurrent operations handle 1000+ nodes
- [ ] Memory usage stable under load

### Documentation:
- [ ] Architecture review updated (ADR)
- [ ] CHANGELOG.md updated with fixes
- [ ] Migration guide for RBAC changes
- [ ] Security audit report completed

---

## 6. Rollout Plan

### Development (Days 1-3):
1. Implement fixes in order (Phase 1 → 2 → 3 → 4)
2. Run tests after each fix
3. Commit per fix with quality validation

### Testing (Days 4-5):
1. Integration test full suite
2. Security audit
3. Load testing
4. Staging deployment

### Production (Week 1-4):
1. **Week 1**: Deploy to 1 non-critical cluster
2. **Week 2**: Monitor metrics, expand to 3 clusters
3. **Week 3**: Deploy to 50% of clusters
4. **Week 4**: Full rollout

### Monitoring:
Track these metrics post-deployment:
- `autoscaler_status_update_failures_total` (should drop to 0)
- `autoscaler_node_deletion_duration_seconds` (should be < 5min)
- `autoscaler_rebalance_success_total` (should increase from 0)
- `webhook_request_errors_total{reason="size_limit"}` (should stay low)

---

## 7. Appendix: Implementation Checklist

### Pre-Implementation:
- [ ] Branch created: `fix/critical-production-issues`
- [ ] PRD reviewed and approved
- [ ] Test plan documented
- [ ] Rollback procedure documented

### During Implementation:
- [ ] Fix implemented per phase
- [ ] Unit tests added/updated
- [ ] Integration tests pass
- [ ] Quality check passes (golangci-lint, race detector)
- [ ] Commit with detailed message

### Post-Implementation:
- [ ] All 10 fixes deployed to staging
- [ ] Security audit completed
- [ ] E2E tests pass in staging
- [ ] Documentation updated
- [ ] PR created with full test results
- [ ] Code review completed
- [ ] Approved for production rollout

---

**Document Status:** READY FOR IMPLEMENTATION
**Next Step:** Execute solution-designer agent for technical architecture

