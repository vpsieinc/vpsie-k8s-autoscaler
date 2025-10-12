# Product Requirements Document (PRD)
# VPSie Kubernetes Autoscaler

**Version:** 1.1.0
**Last Updated:** 2025-10-12
**Status:** Draft

## 1. Overview

The VPSie Kubernetes Autoscaler is a custom cluster autoscaler that dynamically manages Kubernetes node pools by provisioning and deprovisioning VPSie VPS instances based on cluster resource demands and cost optimization goals.

### 1.1 Purpose

Enable automatic scaling of Kubernetes worker nodes on VPSie infrastructure to:
- Ensure pods can always be scheduled when resources are needed
- Minimize infrastructure costs by removing underutilized nodes
- Provide fine-grained control over node provisioning policies
- Optimize for cost efficiency while maintaining application availability

### 1.2 Target Users

- DevOps engineers running Kubernetes clusters on VPSie
- Platform teams managing multi-tenant Kubernetes environments
- Startups and SMBs seeking cost-effective Kubernetes infrastructure

## 2. Core Requirements

### 2.1 Event-Driven Autoscaling ⭐ NEW

The autoscaler must implement event-driven scaling logic that responds to real-time cluster conditions:

#### 2.1.1 Scale-Up (Adding Nodes)

**Trigger Mechanism:**
- Watch Kubernetes events for pod scheduling failures
- Monitor for events indicating "insufficient resources" (CPU, memory, or other resources)
- Specifically watch for:
  - `FailedScheduling` events with reason containing "Insufficient cpu", "Insufficient memory", or "Insufficient pods"
  - Pods in `Pending` state with resource constraint conditions
  - Node pressure events (MemoryPressure, DiskPressure, PIDPressure)

**Scaling Logic:**
- When resource shortage events are detected:
  1. Calculate the total resource deficit (sum of unschedulable pod requests)
  2. Determine the optimal VPSie instance type(s) to satisfy the demand
  3. Consider NodeGroup-specific constraints (min/max nodes, allowed instance types)
  4. Trigger creation of new VPS instances via VPSie API
  5. Wait for nodes to join the cluster and become Ready
  6. Verify that pending pods can be scheduled

**Response Time:**
- Begin node provisioning within 30 seconds of detecting resource shortage
- Target: Nodes ready and available within 5 minutes of provisioning request

#### 2.1.2 Scale-Down (Removing Nodes)

**Pre-Removal Safety Checks:**
- Before removing any node, the autoscaler MUST verify:
  1. **Resource Availability:** Sufficient free resources exist on remaining nodes to accommodate all pods from the node being removed
  2. **Pod Eviction Safety:** All pods on the target node can be safely evicted (respect PodDisruptionBudgets)
  3. **System Pods:** Critical system pods (kube-system, monitoring) can be rescheduled
  4. **Stateful Workloads:** Pods with PersistentVolumes have alternative scheduling options
  5. **Node Affinity/Anti-Affinity:** Pod placement constraints can still be satisfied after removal

**Elimination Process:**
1. Identify underutilized nodes (low CPU/memory usage over observation period)
2. Check if free capacity exists on other nodes to accommodate workloads
3. Cordon the node (mark as unschedulable)
4. Drain the node gracefully:
   - Respect pod termination grace periods
   - Wait for PodDisruptionBudgets
   - Fail-safe: If drain fails, uncordon the node and abort removal
5. Delete the node from Kubernetes
6. Terminate the VPS instance via VPSie API
7. Verify successful cleanup

**Scale-Down Constraints:**
- Minimum observation period: 10 minutes of sustained low utilization
- Default utilization threshold: <50% CPU and <50% memory
- Respect NodeGroup minimum node count
- Never scale down during active scaling-up operations
- Implement scale-down cooldown period (default: 10 minutes after last scale-up)

### 2.2 Custom Resource Definitions (CRDs)

#### 2.2.1 NodeGroup CRD

Defines a logical group of nodes with shared properties and scaling policies.

**Spec Fields:**
```yaml
apiVersion: autoscaler.vpsie.com/v1alpha1
kind: NodeGroup
metadata:
  name: general-purpose-nodes
  namespace: kube-system
spec:
  # Scaling bounds
  minNodes: 2
  maxNodes: 10

  # VPSie instance configuration
  datacenterID: "us-east-1"
  offeringIDs:
    - "small-2cpu-4gb"
    - "medium-4cpu-8gb"
  osImageID: "ubuntu-22.04-lts"

  # Node labels and taints
  labels:
    nodegroup: general-purpose
    workload-type: stateless
  taints:
    - key: workload-type
      value: general-purpose
      effect: NoSchedule

  # Scaling policies
  scaleUpPolicy:
    stabilizationWindowSeconds: 60  # Wait before scaling up
    cpuThreshold: 80  # Scale up if CPU > 80%
    memoryThreshold: 80  # Scale up if memory > 80%

  scaleDownPolicy:
    stabilizationWindowSeconds: 600  # Wait 10min before scaling down
    cpuThreshold: 50  # Scale down if CPU < 50%
    memoryThreshold: 50  # Scale down if memory < 50%
    enabled: true

  # Cost optimization
  preferredInstanceType: "small-2cpu-4gb"  # Prefer smaller instances
  allowMixedInstances: true  # Allow multiple instance types in group
```

**Status Fields:**
```yaml
status:
  currentNodes: 3
  desiredNodes: 3
  readyNodes: 3
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2025-10-12T10:30:00Z"
      reason: AllNodesReady
      message: "All nodes in group are ready"
  nodeList:
    - nodeName: vpsie-node-001
      vpsID: 6266
      instanceType: small-2cpu-4gb
      status: Ready
      createdAt: "2025-10-12T09:00:00Z"
```

### 2.3 VPSie API Integration

#### 2.3.1 Required VPSie API Operations

- **Authentication:** OAuth 2.0 with clientId/clientSecret
- **List VMs:** GET `/vm` - retrieve all VPS instances
- **Create VM:** POST `/vm` - provision new VPS instance
- **Get VM:** GET `/vm/{id}` - retrieve specific VPS details
- **Delete VM:** DELETE `/vm/{id}` - terminate VPS instance
- **Health Check:** Validate API connectivity and credentials

#### 2.3.2 VPSie Credentials Management

Credentials stored in Kubernetes Secret:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: vpsie-credentials
  namespace: kube-system
type: Opaque
stringData:
  clientId: "your-oauth-client-id"
  clientSecret: "your-oauth-client-secret"
```

#### 2.3.3 Rate Limiting and Error Handling

- Implement exponential backoff for failed API calls
- Respect VPSie API rate limits (detect 429 responses)
- Cache API responses where appropriate (datacenters, offerings)
- Implement circuit breaker pattern for API unavailability

### 2.4 Controller Architecture

#### 2.4.1 Control Loop

The controller runs a continuous reconciliation loop:
1. Watch for Kubernetes events (pod scheduling failures, node pressure)
2. Monitor NodeGroup CRD changes
3. Fetch current cluster state (nodes, pods, resource usage)
4. Calculate desired state based on:
   - Resource demand from pending pods
   - Current resource utilization on existing nodes
   - NodeGroup scaling policies
   - Cost optimization preferences
5. Execute scaling actions (create/delete VPS instances)
6. Update NodeGroup status

**Reconciliation Frequency:**
- Event-driven: Immediate response to pod scheduling failures
- Periodic: Every 30 seconds for general health checks
- Metrics-based: Every 60 seconds for utilization-based scaling

#### 2.4.2 Node Lifecycle Management

**Node Provisioning:**
1. Determine required instance type from NodeGroup spec
2. Call VPSie API to create VPS instance
3. Wait for VPS to be running and accessible
4. Wait for node to join cluster (via kubelet)
5. Wait for node to become Ready
6. Add node to NodeGroup status

**Node Deprovisioning:**
1. Identify node candidate for removal (low utilization)
2. Verify safe removal conditions (see 2.1.2)
3. Cordon node to prevent new pod scheduling
4. Drain node (evict all pods gracefully)
5. Delete node from Kubernetes
6. Call VPSie API to delete VPS instance
7. Update NodeGroup status

### 2.5 Observability

#### 2.5.1 Metrics (Prometheus)

Export the following metrics:
- `vpsie_autoscaler_nodegroup_nodes{nodegroup, state}` - Node count by state
- `vpsie_autoscaler_nodegroup_desired_nodes{nodegroup}` - Desired node count
- `vpsie_autoscaler_scale_up_total{nodegroup}` - Total scale-up operations
- `vpsie_autoscaler_scale_down_total{nodegroup}` - Total scale-down operations
- `vpsie_autoscaler_scale_up_failures_total{nodegroup}` - Failed scale-ups
- `vpsie_autoscaler_scale_down_failures_total{nodegroup}` - Failed scale-downs
- `vpsie_autoscaler_vpsie_api_requests_total{method, status}` - API call metrics
- `vpsie_autoscaler_vpsie_api_latency_seconds{method}` - API latency
- `vpsie_autoscaler_unschedulable_pods{nodegroup}` - Pending pod count
- `vpsie_autoscaler_node_utilization{nodegroup, resource}` - CPU/memory utilization

#### 2.5.2 Logging

- Structured logging (JSON format)
- Log levels: DEBUG, INFO, WARN, ERROR
- Key log events:
  - Scaling decisions and rationale
  - VPSie API calls and responses
  - Node lifecycle events
  - Error conditions and retry attempts
  - Event-driven triggers (resource shortage detected)

#### 2.5.3 Events

Emit Kubernetes events for:
- Scale-up triggered
- Node provisioning started/completed/failed
- Scale-down triggered
- Node drain started/completed/failed
- VPSie API errors
- Configuration errors

## 3. Non-Functional Requirements

### 3.1 Performance

- **Scaling Response Time:** Begin scaling within 30 seconds of triggering condition
- **Node Provisioning Time:** VPS ready within 5 minutes (VPSie-dependent)
- **Node Ready Time:** Node joins cluster within 2 minutes of VPS ready
- **API Latency:** VPSie API calls complete within 10 seconds (p95)
- **Controller CPU:** <100m CPU under normal load
- **Controller Memory:** <256Mi memory under normal load

### 3.2 Reliability

- **Controller Availability:** Support high-availability deployment (2+ replicas with leader election)
- **Crash Recovery:** Gracefully recover from controller restarts
- **State Consistency:** Maintain consistent view of cluster state
- **Idempotency:** All operations must be idempotent
- **Error Recovery:** Automatic retry with exponential backoff

### 3.3 Security

- **Credentials:** Store VPSie API credentials in Kubernetes Secrets
- **RBAC:** Minimal RBAC permissions (list/watch/create/delete nodes and pods)
- **Network:** Support network policies for controller pod
- **Audit:** Log all scaling actions for audit trail

### 3.4 Scalability

- Support up to 100 NodeGroups per cluster
- Support up to 500 nodes per NodeGroup
- Support clusters with up to 1000 total nodes

## 4. Implementation Phases

### Phase 1: Core Infrastructure ✅ COMPLETE
- [x] VPSie API client implementation
- [x] Authentication and token management
- [x] Basic VM operations (List, Create, Get, Delete)
- [x] Error handling and rate limiting
- [x] Unit tests with 85%+ coverage

### Phase 2: NodeGroup CRD and Controller 🚧 IN PROGRESS
- [ ] Define NodeGroup CRD schema
- [ ] Generate CRD client code
- [ ] Implement NodeGroup controller scaffolding
- [ ] Implement leader election for HA
- [ ] Basic reconciliation loop

### Phase 3: Event-Driven Scaling ⭐ CURRENT FOCUS
- [ ] Implement Kubernetes event watcher
- [ ] Detect pod scheduling failures due to insufficient resources
- [ ] Calculate resource deficit from unschedulable pods
- [ ] Implement scale-up decision logic
- [ ] Trigger VPS provisioning via VPSie API
- [ ] Track node join and readiness

### Phase 4: Safe Scale-Down
- [ ] Implement node utilization monitoring
- [ ] Implement resource availability checker
- [ ] Implement safe node drain logic
- [ ] Respect PodDisruptionBudgets
- [ ] Implement scale-down cooldown periods
- [ ] Implement node elimination workflow

### Phase 5: Cost Optimization
- [ ] Implement instance type selection algorithm
- [ ] Support mixed instance types per NodeGroup
- [ ] Implement cost-aware scaling decisions
- [ ] Support spot/preemptible instances (if VPSie supports)

### Phase 6: Observability
- [ ] Implement Prometheus metrics
- [ ] Implement structured logging
- [ ] Emit Kubernetes events for scaling actions
- [ ] Create Grafana dashboards
- [ ] Create runbooks and alerts

### Phase 7: Production Readiness
- [ ] End-to-end integration tests
- [ ] Chaos testing and failure scenarios
- [ ] Performance benchmarking
- [ ] Security audit
- [ ] Documentation (user guide, operator guide, troubleshooting)

## 5. Success Criteria

### 5.1 Functional Success

- ✅ Automatically scale up when pods cannot be scheduled due to resource constraints
- ✅ Automatically scale down when nodes are underutilized and safe to remove
- ✅ Respect NodeGroup min/max bounds
- ✅ Support multiple NodeGroups with different policies
- ✅ Gracefully handle VPSie API failures

### 5.2 Operational Success

- Pod scheduling success rate > 99%
- Scale-up response time < 30 seconds (p95)
- Node ready time < 7 minutes (p95) including VPSie provisioning
- Zero unintended node terminations
- Controller uptime > 99.9%

### 5.3 Cost Efficiency

- Average node utilization > 60%
- Reduce infrastructure costs by 20-40% vs. static provisioning
- Zero over-provisioning incidents (pending pods due to scale-down)

## 6. Dependencies

### 6.1 External Dependencies

- **VPSie API:** Stable v2 API with documented endpoints
- **Kubernetes:** 1.24+ cluster
- **controller-runtime:** v0.19+ for CRD controllers
- **client-go:** v0.31+ for Kubernetes API client

### 6.2 Internal Dependencies

- VPSie API client package (pkg/vpsie/client) ✅ Complete
- NodeGroup CRD definition (api/v1alpha1) 🚧 Pending
- Controller manager scaffold (cmd/controller) ✅ Complete

## 7. Open Questions

1. **Node Bootstrapping:** How should nodes be configured to join the cluster?
   - Use cloud-init with kubeadm join command?
   - Use custom AMI/image with pre-configured kubelet?

2. **VPSie Metadata:** Can we tag VPS instances with Kubernetes metadata (nodegroup, cluster-id)?

3. **Network Configuration:** How are pod network CIDR and service CIDR configured on new nodes?

4. **Storage:** Does VPSie support persistent volume provisioning?

5. **Spot Instances:** Does VPSie offer spot/preemptible instances for cost savings?

## 8. Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|-----------|
| VPSie API unavailability | High | Implement retry logic, circuit breaker, degrade gracefully |
| Slow VPS provisioning | Medium | Set realistic timeout expectations, alert on slow provisioning |
| Node fails to join cluster | High | Implement health checks, automatic cleanup of failed nodes |
| Over-aggressive scale-down | Critical | Implement conservative thresholds, mandatory stabilization windows |
| Accidental node deletion | Critical | Require multiple safety checks, implement dry-run mode |
| Race conditions in scaling | Medium | Implement proper locking, idempotent operations |
| Cost overruns | High | Implement hard limits on NodeGroup maxNodes, alerting on costs |

## 9. Future Enhancements

- **Multi-cloud support:** Extend to support other cloud providers
- **GPU node support:** Support GPU-enabled VPS instances
- **Predictive scaling:** Use historical data to predict resource needs
- **Vertical pod autoscaling integration:** Coordinate with VPA for right-sizing
- **Custom metrics:** Support scaling based on custom application metrics
- **Scheduled scaling:** Support time-based scaling policies
- **Budget management:** Integrate with cost tracking and budget enforcement

## 10. References

- [Kubernetes Cluster Autoscaler](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler)
- [VPSie API Documentation](https://vpsie.com/api/v2/docs) (assumed)
- [Kubernetes Node Lifecycle](https://kubernetes.io/docs/concepts/architecture/nodes/)
- [PodDisruptionBudget Best Practices](https://kubernetes.io/docs/tasks/run-application/configure-pdb/)
