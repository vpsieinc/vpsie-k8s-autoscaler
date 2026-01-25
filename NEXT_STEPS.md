# Next Steps - VPSie Kubernetes Autoscaler

**Last Updated:** 2025-10-12
**Current Version:** v0.1.0-alpha (Foundation Complete)
**Target Version:** v0.2.0-alpha (Controller Implementation)

## Current Status

### ✅ Phase 1 Complete: Foundation & Infrastructure

1. **VPSie API Client** (`pkg/vpsie/client/`)
   - ✅ VPSie custom authentication with automatic token refresh
   - ✅ VM lifecycle operations (List, Create, Get, Delete)
   - ✅ Comprehensive error handling with wrapped errors
   - ✅ Thread-safe token management with mutex
   - ✅ 36 tests with 85.3% coverage

2. **Custom Resource Definitions** (`pkg/apis/autoscaler/v1alpha1/`)
   - ✅ NodeGroup CRD for managing logical node groups
   - ✅ VPSieNode CRD for tracking individual VPS instances (8 lifecycle phases)
   - ✅ Full kubebuilder markers and OpenAPI v3 validation
   - ✅ 38 tests with 100% pass rate
   - ✅ Generated CRD manifests and DeepCopy methods

3. **Documentation**
   - ✅ Product Requirements Document (PRD) - 429 lines
   - ✅ API Reference (docs/API.md) - 497 lines
   - ✅ Development Guide (docs/DEVELOPMENT.md) - 505 lines
   - ✅ OAuth Migration Guide (OAUTH_MIGRATION.md) - 315 lines
   - ✅ Example configurations for different workload types
   - ✅ Comprehensive README with usage instructions

4. **CI/CD & Infrastructure**
   - ✅ GitHub Actions CI workflow (test, lint, build, verify-crds)
   - ✅ Docker multi-arch builds (linux/amd64, linux/arm64)
   - ✅ Automated image publishing to ghcr.io/vpsieinc/vpsie-k8s-autoscaler
   - ✅ 72 total tests with 81.5% overall coverage
   - ✅ Pre-commit hooks for code quality
   - ✅ All workflows passing with zero failures

**Foundation Status:** ✅ **COMPLETE** - Ready for controller implementation!

5. **Observability Framework** (`pkg/metrics/`, `pkg/logging/`, `pkg/events/`)
   - ✅ Prometheus metrics package with 22 metrics (311 lines)
     - NodeGroup metrics (desired/current/ready/min/max nodes)
     - VPSieNode metrics (phase tracking, transitions, provisioning/termination duration)
     - Controller metrics (reconcile duration/errors/totals)
     - VPSie API metrics (requests/duration/errors)
     - Scaling metrics (scale-up/down operations and node counts)
     - Pod metrics (unschedulable/pending counts)
     - Event emission metrics
   - ✅ Structured logging package with zap (234 lines)
     - Request ID tracking (UUID v4)
     - Scaling decision logging
     - API operation logging
     - Node lifecycle logging
     - Phase transition logging
     - Reconciliation logging
   - ✅ Kubernetes event emitter (308 lines)
     - 20+ event types for NodeGroup and VPSieNode
     - Automatic metrics recording for all events
   - ✅ VPSie API client fully instrumented with metrics and logging
   - ⏳ Controller integration pending (NodeGroup, VPSieNode, Event Watcher)
   - ✅ Comprehensive documentation (OBSERVABILITY.md - 409 lines)

**Observability Status:** ✅ **FRAMEWORK COMPLETE** - Ready for controller integration!

## Phase 2: Controller Implementation

### Overview

Implement the core Kubernetes controller that watches NodeGroup and VPSieNode resources and reconciles desired state with actual infrastructure.

### Priority 1: Controller Scaffold (Week 1)

**Goal:** Create the basic controller structure with proper Kubernetes client integration.

#### Tasks:

1. **Controller Manager Setup**
   - [ ] Create `pkg/controller/manager.go` - Controller manager initialization
   - [ ] Implement Kubernetes client-go integration
   - [ ] Set up controller-runtime manager with leader election
   - [ ] Add graceful shutdown handling (SIGTERM, SIGINT)
   - [ ] Implement health check endpoints (`/healthz`, `/readyz`)

   **Files to create:**
   ```
   pkg/controller/
   ├── manager.go           # Manager initialization
   ├── options.go           # Configuration options
   └── health.go            # Health check handlers
   ```

   **Key components:**
   ```go
   // pkg/controller/manager.go
   type ControllerManager struct {
       client        kubernetes.Interface
       vpsieClient   *vpsie.Client
       mgr           ctrl.Manager
       stopCh        <-chan struct{}
   }

   func NewManager(config *rest.Config, opts Options) (*ControllerManager, error)
   func (m *ControllerManager) Start(ctx context.Context) error
   ```

2. **Update Main Controller Binary**
   - [ ] Update `cmd/controller/main.go` to use controller-runtime
   - [ ] Add flags for controller configuration
   - [ ] Implement proper structured logging
   - [ ] Add metrics server initialization

   **Configuration flags:**
   - `--kubeconfig` - Path to kubeconfig (optional, uses in-cluster if not set)
   - `--metrics-addr` - Metrics server bind address (default: `:8080`)
   - `--health-addr` - Health probe bind address (default: `:8081`)
   - `--leader-election` - Enable leader election (default: `true`)
   - `--sync-period` - Resync period for controllers (default: `10m`)
   - `--vpsie-secret` - Name of VPSie credentials secret (default: `vpsie-secret`)
   - `--vpsie-namespace` - Namespace of VPSie secret (default: `kube-system`)

3. **Write Tests**
   - [ ] Unit tests for manager initialization
   - [ ] Tests for configuration parsing
   - [ ] Tests for graceful shutdown
   - [ ] Integration test scaffold

   **Expected:** 15+ tests with 80%+ coverage

### Priority 2: NodeGroup Controller (Week 2-3)

**Goal:** Implement the NodeGroup reconciliation loop.

#### Tasks:

1. **NodeGroup Controller Structure**
   - [ ] Create `pkg/controller/nodegroup/controller.go`
   - [ ] Implement reconciliation loop
   - [ ] Set up watches for NodeGroup resources
   - [ ] Add finalizer handling for cleanup

   **Files to create:**
   ```
   pkg/controller/nodegroup/
   ├── controller.go        # Main reconciliation logic
   ├── reconciler.go        # Reconcile implementation
   ├── status.go            # Status update helpers
   ├── conditions.go        # Condition management
   └── controller_test.go   # Controller tests
   ```

2. **Reconciliation Logic**
   - [ ] Implement spec validation
   - [ ] Create/update VPSieNode resources based on desired count
   - [ ] Update NodeGroup status with current state
   - [ ] Manage NodeGroup conditions (Ready, Scaling, Error, AtMinCapacity, AtMaxCapacity)
   - [ ] Handle NodeGroup deletion (finalizers)

   **Key reconciliation flow:**
   ```
   1. Validate NodeGroup spec (min <= max, valid IDs, etc.)
   2. List existing VPSieNodes for this NodeGroup
   3. Calculate desired vs actual node count
   4. If scale-up needed:
      - Create new VPSieNode resources
      - Update NodeGroup status (Scaling condition)
   5. If scale-down needed:
      - Mark VPSieNodes for deletion (see Phase 3)
      - Update NodeGroup status
   6. Update NodeGroup status with current state
   7. Requeue if needed (scale operations in progress)
   ```

3. **Status Management**
   - [ ] Implement status update helpers
   - [ ] Add condition management utilities
   - [ ] Track lastScaleTime
   - [ ] Update node list in status

   **Status fields to maintain:**
   ```yaml
   status:
     currentNodes: 3
     desiredNodes: 5
     readyNodes: 3
     nodes:
       - nodeName: "node-1"
         vpsID: 1001
         instanceType: "small-2cpu-4gb"
         status: "Ready"
         ipAddress: "192.0.2.10"
     conditions:
       - type: Ready
         status: "False"
         reason: Scaling
         message: "Scaling from 3 to 5 nodes"
     lastScaleTime: "2025-10-12T10:00:00Z"
   ```

4. **Write Tests**
   - [ ] Test reconciliation with various NodeGroup configurations
   - [ ] Test scale-up scenarios
   - [ ] Test scale-down scenarios (stub for now)
   - [ ] Test error handling
   - [ ] Test finalizer handling
   - [ ] Test status updates

   **Expected:** 25+ tests with 80%+ coverage

### Priority 3: VPSieNode Controller (Week 3-4)

**Goal:** Implement VPSieNode lifecycle management (provisioning, joining, ready, terminating).

#### Tasks:

1. **VPSieNode Controller Structure**
   - [ ] Create `pkg/controller/vpsienode/controller.go`
   - [ ] Implement state machine for 8 lifecycle phases
   - [ ] Set up watches for VPSieNode and Node resources
   - [ ] Add finalizer handling

   **Files to create:**
   ```
   pkg/controller/vpsienode/
   ├── controller.go        # Main reconciliation logic
   ├── reconciler.go        # Reconcile implementation
   ├── phases.go            # Phase transition logic
   ├── provisioner.go       # VPS provisioning logic
   ├── joiner.go            # Node joining logic (cloud-init, etc.)
   ├── terminator.go        # Node termination logic
   ├── status.go            # Status update helpers
   └── controller_test.go   # Controller tests
   ```

2. **Phase State Machine**

   Implement transitions between 8 phases:

   - [ ] **Pending → Provisioning**: Create VPS via VPSie API
   - [ ] **Provisioning → Provisioned**: Wait for VPS to be running
   - [ ] **Provisioned → Joining**: Wait for node to register with Kubernetes
   - [ ] **Joining → Ready**: Wait for node Ready condition
   - [ ] **Ready → Terminating**: Handle deletion request
   - [ ] **Terminating → Deleting**: Drain node, delete from Kubernetes
   - [ ] **Deleting → (deleted)**: Delete VPS, remove VPSieNode
   - [ ] **Any → Failed**: Handle errors and failures

   **Phase transition logic:**
   ```go
   // pkg/controller/vpsienode/phases.go
   type PhaseHandler interface {
       Handle(ctx context.Context, vn *v1alpha1.VPSieNode) (Result, error)
   }

   type PendingHandler struct { /* ... */ }      // Create VPS
   type ProvisioningHandler struct { /* ... */ }  // Wait for running
   type ProvisionedHandler struct { /* ... */ }   // Wait for node join
   type JoiningHandler struct { /* ... */ }       // Wait for ready
   type ReadyHandler struct { /* ... */ }         // Monitor health
   type TerminatingHandler struct { /* ... */ }   // Drain node
   type DeletingHandler struct { /* ... */ }      // Delete VPS
   ```

3. **VPS Provisioning**
   - [ ] Implement `provisioner.go` for VPS creation
   - [ ] Generate cloud-init user data for node bootstrapping
   - [ ] Include kubeadm join command in user data
   - [ ] Set VPSieNode status fields (vpsieInstanceID, ipAddress, etc.)
   - [ ] Handle provisioning errors

   **Cloud-init template:**
   ```yaml
   #cloud-config
   runcmd:
     - curl -fsSL https://get.k8s.io | bash
     - kubeadm join <cluster-endpoint> --token <token> \
         --discovery-token-ca-cert-hash sha256:<hash>
     - systemctl enable kubelet
     - systemctl start kubelet
   ```

4. **Node Joining Logic**
   - [ ] Watch for Kubernetes Node resource with matching name/IP
   - [ ] Update VPSieNode status when node registers
   - [ ] Set joinedAt timestamp
   - [ ] Apply labels from NodeGroup spec to Kubernetes node
   - [ ] Apply taints from NodeGroup spec to Kubernetes node

5. **Node Termination**
   - [ ] Implement graceful node drain (respect PodDisruptionBudgets)
   - [ ] Delete node from Kubernetes
   - [ ] Delete VPS via VPSie API
   - [ ] Handle termination errors (retry logic)

6. **Write Tests**
   - [ ] Test each phase transition
   - [ ] Test VPS provisioning with mock VPSie client
   - [ ] Test node joining detection
   - [ ] Test termination flow
   - [ ] Test error handling and retries
   - [ ] Test finalizer cleanup

   **Expected:** 30+ tests with 80%+ coverage

### Priority 4: Event-Driven Scaling (Week 5-6)

**Goal:** Implement event-driven scale-up logic based on pod scheduling failures.

#### Tasks:

1. **Event Watcher**
   - [ ] Create `pkg/controller/events/watcher.go`
   - [ ] Watch for pod FailedScheduling events
   - [ ] Filter events for resource constraints (CPU, memory, pods)
   - [ ] Calculate resource deficit from unschedulable pods

   **Files to create:**
   ```
   pkg/controller/events/
   ├── watcher.go           # Event watching logic
   ├── analyzer.go          # Resource deficit calculation
   ├── scaleup.go           # Scale-up decision logic
   └── watcher_test.go      # Tests
   ```

2. **Scale-Up Logic**
   - [ ] Calculate total resource deficit (sum of pending pod requests)
   - [ ] Find NodeGroups that can satisfy the demand
   - [ ] Select optimal instance type(s) from offeringIDs
   - [ ] Respect NodeGroup max capacity
   - [ ] Trigger NodeGroup scale-up (increase desiredNodes)
   - [ ] Implement stabilization window (avoid rapid scaling)

   **Scale-up algorithm:**
   ```
   1. Aggregate resource requests from unschedulable pods
   2. For each NodeGroup:
      - Check if NodeGroup can satisfy demand (labels, taints match)
      - Calculate how many nodes needed
      - Check if within maxNodes limit
   3. Select optimal instance type:
      - Prefer instance types that exactly match demand
      - Consider cost (prefer smaller instances if possible)
      - Use PreferredInstanceType if specified
   4. Update NodeGroup.Status.DesiredNodes
   5. Wait for VPSieNodes to be provisioned
   ```

3. **Stabilization**
   - [ ] Implement stabilization window (don't scale too fast)
   - [ ] Track recent scale events
   - [ ] Prevent scale-up during cooldown period

4. **Write Tests**
   - [ ] Test event filtering
   - [ ] Test resource deficit calculation
   - [ ] Test scale-up decision logic
   - [ ] Test stabilization windows
   - [ ] Test multi-NodeGroup scenarios

   **Expected:** 20+ tests with 80%+ coverage

### Priority 5: Scale-Down Logic (Week 7)

**Goal:** Implement safe scale-down based on node utilization.

#### Tasks:

1. **Node Utilization Monitor**
   - [ ] Create `pkg/controller/scaledown/monitor.go`
   - [ ] Query node metrics (CPU, memory usage)
   - [ ] Track utilization over time (observation window)
   - [ ] Identify underutilized nodes

   **Files to create:**
   ```
   pkg/controller/scaledown/
   ├── monitor.go           # Utilization monitoring
   ├── analyzer.go          # Scale-down candidate selection
   ├── safety.go            # Pre-removal safety checks
   ├── drainer.go           # Node drain logic
   └── monitor_test.go      # Tests
   ```

2. **Scale-Down Safety Checks**
   - [ ] Verify sufficient capacity on remaining nodes
   - [ ] Check PodDisruptionBudgets
   - [ ] Ensure system pods can be rescheduled
   - [ ] Verify StatefulSet pods have alternatives
   - [ ] Check pod affinity/anti-affinity constraints

   **Safety check algorithm:**
   ```
   1. For each underutilized node:
      a. List all pods on the node
      b. Calculate total resource requests
      c. Check if other nodes have sufficient free capacity
      d. Verify PodDisruptionBudgets allow eviction
      e. Check if pods can satisfy affinity constraints elsewhere
      f. Mark node as safe for removal or skip
   ```

3. **Node Drain and Removal**
   - [ ] Cordon node (mark unschedulable)
   - [ ] Drain node gracefully with retries
   - [ ] Respect pod termination grace periods
   - [ ] Fail-safe: uncordon if drain fails
   - [ ] Delete node from Kubernetes
   - [ ] Update VPSieNode to Terminating phase
   - [ ] Delete VPS via VPSie API

4. **Write Tests**
   - [ ] Test utilization monitoring
   - [ ] Test safety checks (various scenarios)
   - [ ] Test drain logic
   - [ ] Test rollback on drain failure
   - [ ] Test scale-down constraints (minNodes, cooldown)

   **Expected:** 25+ tests with 80%+ coverage

## Phase 3: Deployment & Operations (Week 8-9)

### Priority 6: Deployment Manifests

#### Tasks:

1. **Kubernetes Manifests**
   - [ ] Create `deploy/controller/` directory
   - [ ] ServiceAccount, ClusterRole, ClusterRoleBinding
   - [ ] Deployment manifest for controller
   - [ ] Service for metrics and health endpoints
   - [ ] ConfigMap for controller configuration

   **RBAC permissions needed:**
   ```yaml
   # Minimal permissions for controller
   rules:
   - apiGroups: ["autoscaler.vpsie.com"]
     resources: ["nodegroups", "vpsienodes"]
     verbs: ["get", "list", "watch", "update", "patch"]
   - apiGroups: ["autoscaler.vpsie.com"]
     resources: ["nodegroups/status", "vpsienodes/status"]
     verbs: ["update", "patch"]
   - apiGroups: [""]
     resources: ["nodes", "pods", "events"]
     verbs: ["get", "list", "watch"]
   - apiGroups: [""]
     resources: ["nodes"]
     verbs: ["update", "patch", "delete"]
   - apiGroups: [""]
     resources: ["secrets"]
     verbs: ["get"]
   - apiGroups: [""]
     resources: ["pods/eviction"]
     verbs: ["create"]
   ```

2. **Helm Chart** (Optional but recommended)
   - [ ] Create `charts/vpsie-autoscaler/` directory
   - [ ] Chart.yaml with metadata
   - [ ] values.yaml with configurable options
   - [ ] Templates for all Kubernetes resources
   - [ ] README.md with installation instructions

3. **Kustomize Overlays** (Alternative to Helm)
   - [ ] Create `deploy/kustomize/base/`
   - [ ] Create overlays for different environments (dev, staging, prod)

### Priority 7: Observability ✅ Framework Complete

**Status:** Observability framework fully implemented and documented. Controller integration pending.

#### Completed Tasks:

1. **Metrics** ✅
   - ✅ Created `pkg/metrics/metrics.go` with 22 comprehensive metrics
   - ✅ Metrics registration with controller-runtime
   - ✅ All controller-specific metrics implemented:
     - ✅ `nodegroup_desired_nodes{nodegroup, namespace}`
     - ✅ `nodegroup_current_nodes{nodegroup, namespace}`
     - ✅ `nodegroup_ready_nodes{nodegroup, namespace}`
     - ✅ `nodegroup_min_nodes{nodegroup, namespace}`
     - ✅ `nodegroup_max_nodes{nodegroup, namespace}`
     - ✅ `vpsienode_phase{phase, nodegroup, namespace}`
     - ✅ `vpsienode_phase_transitions_total{from_phase, to_phase, nodegroup, namespace}`
     - ✅ `controller_reconcile_duration_seconds{controller}` (histogram)
     - ✅ `controller_reconcile_errors_total{controller, error_type}`
     - ✅ `controller_reconcile_total{controller, result}`
     - ✅ `vpsie_api_requests_total{method, status}`
     - ✅ `vpsie_api_request_duration_seconds{method}` (histogram)
     - ✅ `vpsie_api_errors_total{method, error_type}`
     - ✅ `scale_up_total{nodegroup, namespace}`
     - ✅ `scale_down_total{nodegroup, namespace}`
     - ✅ `scale_up_nodes_added{nodegroup, namespace}` (histogram)
     - ✅ `scale_down_nodes_removed{nodegroup, namespace}` (histogram)
     - ✅ `unschedulable_pods_total{constraint, namespace}`
     - ✅ `pending_pods_current{namespace}`
     - ✅ `events_emitted_total{event_type, reason, object_kind}`
   - ✅ Created `pkg/metrics/recorder.go` with helper functions for recording metrics

2. **Structured Logging** ✅
   - ✅ Created `pkg/logging/logger.go` with zap integration
   - ✅ Request ID tracking with UUID v4 (WithRequestID, GetRequestID)
   - ✅ Comprehensive logging functions:
     - ✅ LogScaleUpDecision / LogScaleDownDecision
     - ✅ LogAPICall / LogAPIResponse / LogAPIError
     - ✅ LogNodeProvisioningStart/Complete/Failed
     - ✅ LogNodeTerminationStart/Complete/Failed
     - ✅ LogPhaseTransition
     - ✅ LogUnschedulablePods
     - ✅ LogReconciliationStart/Complete/Error
   - ⏳ Log level configuration via flags (pending - will be added to controller main.go)

3. **Kubernetes Events** ✅
   - ✅ Created `pkg/events/emitter.go` with EventEmitter
   - ✅ 20+ event types for NodeGroup and VPSieNode lifecycle
   - ✅ Automatic metrics recording for all emitted events
   - ✅ Event reasons:
     - ✅ NodeGroup: ScaleUpTriggered/Completed/Failed, ScaleDownTriggered/Completed/Failed
     - ✅ VPSieNode: NodeProvisioning/Provisioned/ProvisioningFailed, NodeJoining/Ready/JoinFailed
     - ✅ VPSieNode: NodeTerminating/Terminated/TerminationFailed, NodeDraining/Drained/DrainFailed
     - ✅ VPS: VPSCreated/CreateFailed, VPSDeleted/DeleteFailed
     - ✅ Pods: UnschedulablePods

4. **VPSie API Client Integration** ✅
   - ✅ Fully instrumented with metrics (request duration, counts, errors)
   - ✅ Debug logging for all API calls and responses
   - ✅ Error logging with full context
   - ✅ Request ID tracking throughout request lifecycle

5. **Documentation** ✅
   - ✅ Created comprehensive OBSERVABILITY.md (409 lines)
   - ✅ Sample Prometheus queries and alerts
   - ✅ Recommended Grafana dashboard panels
   - ✅ Integration guide and best practices

#### Pending Tasks:

1. **Controller Integration** ⏳
   - [ ] Integrate metrics into NodeGroup controller (reconciliation, scaling operations)
   - [ ] Integrate metrics into VPSieNode controller (phase tracking, provisioning/termination duration)
   - [ ] Integrate metrics into Event Watcher (unschedulable pod detection)
   - [ ] Add event emission to controllers (scale-up/down, provisioning, termination)
   - [ ] Add structured logging to all controller operations

2. **Metrics Endpoint** ⏳
   - [ ] Expose Prometheus metrics endpoint in controller (via controller-runtime)
   - [ ] Add --metrics-addr flag to controller

3. **Dashboards** (Optional)
   - [ ] Create Grafana dashboard JSON (deploy/grafana/autoscaler-dashboard.json)
   - [ ] Create Prometheus alert rule templates (deploy/prometheus/alerts.yaml)

### Priority 8: Documentation

#### Tasks:

1. **Deployment Guide**
   - [ ] Create `docs/DEPLOYMENT.md`
   - [ ] Step-by-step installation instructions
   - [ ] Configuration options reference
   - [ ] Troubleshooting guide

2. **Operations Guide**
   - [ ] Create `docs/OPERATIONS.md`
   - [ ] How to monitor the controller
   - [ ] Common operational tasks
   - [ ] Disaster recovery procedures

3. **Architecture Documentation**
   - [ ] Create `docs/ARCHITECTURE.md`
   - [ ] System architecture diagram
   - [ ] Component interactions
   - [ ] Scaling decision flow charts

## Phase 4: Polish & Release (Week 10+)

### Priority 9: Edge Cases & Reliability

#### Tasks:

1. **Error Handling**
   - [ ] Implement retry logic with exponential backoff
   - [ ] Add circuit breaker for VPSie API calls
   - [ ] Handle partial failures gracefully

2. **Edge Case Testing**
   - [ ] NodeGroup deleted while nodes provisioning
   - [ ] VPSie API rate limiting
   - [ ] Network failures during provisioning
   - [ ] Controller restart during scale operations
   - [ ] Multiple controllers (leader election)

3. **E2E Tests**
   - [ ] Create `test/e2e/` directory
   - [ ] E2E test suite with real Kubernetes cluster (kind)
   - [ ] Mock VPSie API for E2E tests

### Priority 10: Release Preparation

#### Tasks:

1. **Versioning & Releases**
   - [ ] Set up semantic versioning
   - [ ] Create CHANGELOG.md
   - [ ] Tag releases in git

2. **CI/CD**
   - [ ] GitHub Actions for tests
   - [ ] Docker image building
   - [ ] Release automation

3. **Security**
   - [ ] Security audit of VPSie client
   - [ ] Dependency vulnerability scanning
   - [ ] RBAC permission review

## Testing Strategy

### Unit Tests
- Target: 80%+ coverage for all packages
- Mock external dependencies (VPSie API, Kubernetes API)
- Fast execution (<30 seconds total)

### Integration Tests
- Test controllers with fake Kubernetes API server
- Use envtest for CRD integration testing
- Test reconciliation loops end-to-end

### E2E Tests
- Full cluster testing with kind
- Mock VPSie API for reproducibility
- Test complete scaling scenarios

### Test Coverage Goals
- Phase 2: 80%+ coverage
- Phase 3: 75%+ coverage (deployment code)
- Phase 4: 85%+ coverage overall

## Dependencies to Add

```go
// go.mod additions needed for Phase 2
require (
    k8s.io/client-go v0.31.0
    k8s.io/apimachinery v0.31.0
    k8s.io/api v0.31.0
    sigs.k8s.io/controller-runtime v0.19.4  // Already added
    github.com/prometheus/client_golang v1.19.0
    go.uber.org/zap v1.27.0  // Structured logging
)
```

## Success Criteria

### Phase 2 Complete
- ✅ Controller can watch NodeGroup and VPSieNode resources
- ✅ NodeGroup reconciler can create VPSieNode resources
- ✅ VPSieNode reconciler can provision VPS instances
- ✅ Nodes join the Kubernetes cluster automatically
- ✅ All tests passing with 80%+ coverage

### Phase 3 Complete
- ✅ Event-driven scale-up works (responds to pending pods)
- ✅ Safe scale-down removes underutilized nodes
- ✅ Deployment manifests for production use
- ✅ Prometheus metrics exposed
- ✅ Documentation complete

### Phase 4 Complete
- ✅ E2E tests passing
- ✅ Docker images published
- ✅ v0.2.0-alpha release tagged
- ✅ Production-ready (beta quality)

## Timeline

**Total estimated time:** 10-12 weeks

- **Weeks 1-4:** Phase 2 (Controller Implementation)
- **Weeks 5-7:** Phase 3 continued (Scaling Logic)
- **Weeks 8-9:** Phase 3 (Deployment & Operations)
- **Week 10+:** Phase 4 (Polish & Release)

## Questions to Resolve

1. **Node Bootstrapping:** How will nodes join the cluster?
   - Option A: Pre-configured cloud-init with kubeadm join
   - Option B: Custom bootstrap script
   - **Decision needed:** Requires kubeadm token management

2. **VPS Naming Convention:**
   - Format: `vpsie-<nodegroup>-<random-suffix>`
   - Length constraints? VPSie API limits?

3. **Network Configuration:**
   - VPC support?
   - Private networking between nodes?
   - Load balancer integration?

4. **Cost Optimization:**
   - Should we implement instance type recommendation?
   - Support for mixed instance types per NodeGroup?

5. **Multi-Cluster:**
   - Support multiple clusters per VPSie account?
   - How to avoid conflicts?

## Resources

- **Kubernetes Controller Patterns:** https://kubernetes.io/docs/concepts/architecture/controller/
- **controller-runtime Book:** https://book.kubebuilder.io/
- **VPSie API Documentation:** https://api-docs.vpsie.com/
- **Cluster Autoscaler Design:** https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md

## Notes

- Prioritize correctness over performance initially
- Add comprehensive logging for debugging
- Document all assumptions and design decisions
- Keep the PRD updated as requirements evolve
