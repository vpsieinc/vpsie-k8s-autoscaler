# API Reference

## Custom Resource Definitions

### NodeGroup

**API Group:** `autoscaler.vpsie.com/v1alpha1`

**Kind:** `NodeGroup`

**Description:** Manages a logical group of Kubernetes nodes backed by VPSie VPS instances with autoscaling policies.

#### Spec

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `minNodes` | `int32` | Yes | - | Minimum number of nodes in the group. Must be >= 0. |
| `maxNodes` | `int32` | Yes | - | Maximum number of nodes in the group. Must be >= 1 and >= minNodes. |
| `datacenterID` | `string` | Yes | - | VPSie datacenter ID where nodes will be provisioned. |
| `offeringIDs` | `[]string` | Yes | - | List of acceptable VPSie instance type IDs. Controller selects the most cost-effective option. |
| `osImageID` | `string` | Yes | - | VPSie OS image ID to use for provisioned nodes. |
| `scaleUpPolicy` | `ScaleUpPolicy` | No | See below | Configuration for scale-up behavior. |
| `scaleDownPolicy` | `ScaleDownPolicy` | No | See below | Configuration for scale-down behavior. |
| `labels` | `map[string]string` | No | `{}` | Kubernetes labels to apply to provisioned nodes. |
| `taints` | `[]corev1.Taint` | No | `[]` | Kubernetes taints to apply to provisioned nodes for workload isolation. |
| `sshKeyIDs` | `[]string` | No | `[]` | VPSie SSH key IDs to add to provisioned instances. |
| `tags` | `[]string` | No | `[]` | VPSie tags to apply to provisioned instances for organization. |
| `userData` | `string` | No | `""` | Cloud-init user data script for node initialization. |
| `preferredInstanceType` | `string` | No | `""` | Preferred instance type from offeringIDs. Used when multiple types satisfy demand. |
| `allowMixedInstances` | `bool` | No | `false` | Allow using different instance types within the same NodeGroup. |

##### ScaleUpPolicy

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | `bool` | No | `true` | Enable automatic scale-up. |
| `stabilizationWindowSeconds` | `int32` | No | `60` | Time to wait before scaling up again after a scale-up event. |
| `cpuThreshold` | `int32` | No | `80` | CPU utilization percentage threshold (0-100) to trigger scale-up. |
| `memoryThreshold` | `int32` | No | `80` | Memory utilization percentage threshold (0-100) to trigger scale-up. |

##### ScaleDownPolicy

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | `bool` | No | `true` | Enable automatic scale-down. |
| `stabilizationWindowSeconds` | `int32` | No | `600` | Time to wait before scaling down (default 10 minutes). |
| `cpuThreshold` | `int32` | No | `50` | CPU utilization percentage threshold (0-100) below which scale-down is considered. |
| `memoryThreshold` | `int32` | No | `50` | Memory utilization percentage threshold (0-100) below which scale-down is considered. |
| `unneededTime` | `int32` | No | `600` | Time in seconds a node must be underutilized before removal (default 10 minutes). |

#### Status

| Field | Type | Description |
|-------|------|-------------|
| `currentNodes` | `int32` | Actual number of nodes currently in the group. |
| `desiredNodes` | `int32` | Target number of nodes based on scaling decisions. |
| `readyNodes` | `int32` | Number of nodes that are Ready and accepting workloads. |
| `nodes` | `[]NodeInfo` | Details about individual nodes in the group. |
| `conditions` | `[]NodeGroupCondition` | Current state conditions. |
| `lastScaleTime` | `*metav1.Time` | Timestamp of the most recent scaling action. |

##### NodeInfo

| Field | Type | Description |
|-------|------|-------------|
| `nodeName` | `string` | Kubernetes node name. |
| `vpsID` | `int` | VPSie VPS instance ID. |
| `instanceType` | `string` | VPSie instance type (e.g., "small-2cpu-4gb"). |
| `status` | `string` | Current status (Pending, Provisioning, Ready, Terminating, Failed). |
| `ipAddress` | `string` | Primary IP address. |

##### NodeGroupCondition

| Field | Type | Description |
|-------|------|-------------|
| `type` | `string` | Condition type: `Ready`, `Scaling`, `Error`, `AtMinCapacity`, `AtMaxCapacity`. |
| `status` | `corev1.ConditionStatus` | Status: `True`, `False`, or `Unknown`. |
| `lastTransitionTime` | `metav1.Time` | Last time the condition transitioned. |
| `reason` | `string` | Machine-readable reason for the condition. |
| `message` | `string` | Human-readable message with details. |

#### Example

```yaml
apiVersion: autoscaler.vpsie.com/v1alpha1
kind: NodeGroup
metadata:
  name: production-workers
  namespace: kube-system
spec:
  minNodes: 3
  maxNodes: 20
  datacenterID: "dc-us-east-1"
  offeringIDs:
    - "small-2cpu-4gb"
    - "medium-4cpu-8gb"
    - "large-8cpu-16gb"
  osImageID: "ubuntu-22.04-lts"
  preferredInstanceType: "medium-4cpu-8gb"
  allowMixedInstances: true

  scaleUpPolicy:
    enabled: true
    stabilizationWindowSeconds: 60
    cpuThreshold: 80
    memoryThreshold: 80

  scaleDownPolicy:
    enabled: true
    stabilizationWindowSeconds: 600
    cpuThreshold: 50
    memoryThreshold: 50
    unneededTime: 600

  labels:
    workload-type: "general"
    environment: "production"
    managed-by: "vpsie-autoscaler"

  taints: []

  sshKeyIDs:
    - "ssh-key-123"

  tags:
    - "production"
    - "autoscaled"

status:
  currentNodes: 5
  desiredNodes: 5
  readyNodes: 5
  nodes:
    - nodeName: "vpsie-prod-workers-abc123"
      vpsID: 1001
      instanceType: "medium-4cpu-8gb"
      status: "Ready"
      ipAddress: "192.0.2.10"
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2025-10-12T10:00:00Z"
      reason: "AllNodesReady"
      message: "All nodes are ready and operational"
  lastScaleTime: "2025-10-12T09:45:00Z"
```

---

### VPSieNode

**API Group:** `autoscaler.vpsie.com/v1alpha1`

**Kind:** `VPSieNode`

**Description:** Represents a single VPSie VPS instance and tracks its lifecycle as a Kubernetes node.

#### Spec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `vpsieInstanceID` | `int` | Yes | VPSie VPS instance ID (immutable after creation). |
| `instanceType` | `string` | Yes | VPSie instance type (e.g., "small-2cpu-4gb"). |
| `nodeGroupName` | `string` | Yes | Name of the parent NodeGroup. |
| `datacenterID` | `string` | Yes | VPSie datacenter ID where the instance is located. |
| `nodeName` | `string` | No | Kubernetes node name (set after cluster join). |
| `ipAddress` | `string` | No | Primary IPv4 address. |
| `ipv6Address` | `string` | No | IPv6 address (if available). |

#### Status

| Field | Type | Description |
|-------|------|-------------|
| `phase` | `VPSieNodePhase` | Current lifecycle phase (see phases below). |
| `nodeName` | `string` | Kubernetes node name after registration. |
| `vpsieStatus` | `string` | Raw VPSie API status (e.g., "running", "stopped"). |
| `hostname` | `string` | Instance hostname. |
| `resources` | `NodeResources` | Allocated resources (CPU, memory, disk, bandwidth). |
| `conditions` | `[]VPSieNodeCondition` | Detailed status conditions. |
| `createdAt` | `*metav1.Time` | Timestamp when VPSieNode was created. |
| `provisionedAt` | `*metav1.Time` | Timestamp when VPS became running. |
| `joinedAt` | `*metav1.Time` | Timestamp when node joined Kubernetes cluster. |
| `readyAt` | `*metav1.Time` | Timestamp when node became Ready. |
| `terminatingAt` | `*metav1.Time` | Timestamp when termination began. |
| `lastError` | `string` | Most recent error message (for troubleshooting). |
| `observedGeneration` | `int64` | Generation of spec that was last processed. |

##### VPSieNodePhase

The `phase` field can have one of these values:

| Phase | Description |
|-------|-------------|
| `Pending` | VPSieNode resource created, awaiting provisioning. |
| `Provisioning` | VPS creation in progress on VPSie platform. |
| `Provisioned` | VPS is running, awaiting Kubernetes cluster join. |
| `Joining` | Node is joining the Kubernetes cluster. |
| `Ready` | Node is fully operational and accepting workloads. |
| `Terminating` | Node is being gracefully shut down. |
| `Deleting` | VPS deletion in progress. |
| `Failed` | Unrecoverable error occurred. |

##### NodeResources

| Field | Type | Description |
|-------|------|-------------|
| `cpu` | `int` | Number of CPU cores. |
| `memoryMB` | `int` | Memory in megabytes. |
| `diskGB` | `int` | Disk size in gigabytes. |
| `bandwidthGB` | `int` | Monthly bandwidth allocation in gigabytes. |

##### VPSieNodeCondition

| Field | Type | Description |
|-------|------|-------------|
| `type` | `string` | Condition type: `VPSReady`, `NodeJoined`, `NodeReady`, `Error`. |
| `status` | `string` | Status: `True`, `False`, or `Unknown`. |
| `lastTransitionTime` | `metav1.Time` | Last time the condition transitioned. |
| `reason` | `string` | Machine-readable reason for the condition. |
| `message` | `string` | Human-readable message with details. |

#### Example

```yaml
apiVersion: autoscaler.vpsie.com/v1alpha1
kind: VPSieNode
metadata:
  name: vpsie-prod-workers-abc123
  namespace: kube-system
  labels:
    nodegroup: production-workers
spec:
  vpsieInstanceID: 1001
  instanceType: "medium-4cpu-8gb"
  nodeGroupName: "production-workers"
  datacenterID: "dc-us-east-1"
  nodeName: "vpsie-prod-workers-abc123"
  ipAddress: "192.0.2.10"
  ipv6Address: "2001:db8::1"

status:
  phase: Ready
  nodeName: "vpsie-prod-workers-abc123"
  vpsieStatus: "running"
  hostname: "vpsie-prod-workers-abc123.vpsie.local"
  resources:
    cpu: 4
    memoryMB: 8192
    diskGB: 100
    bandwidthGB: 2000
  conditions:
    - type: VPSReady
      status: "True"
      lastTransitionTime: "2025-10-12T10:00:00Z"
      reason: "VPSRunning"
      message: "VPS is running on VPSie platform"
    - type: NodeJoined
      status: "True"
      lastTransitionTime: "2025-10-12T10:03:00Z"
      reason: "NodeJoinedCluster"
      message: "Node successfully joined Kubernetes cluster"
    - type: NodeReady
      status: "True"
      lastTransitionTime: "2025-10-12T10:05:00Z"
      reason: "NodeReady"
      message: "Kubernetes node is ready to accept workloads"
  createdAt: "2025-10-12T09:55:00Z"
  provisionedAt: "2025-10-12T10:00:00Z"
  joinedAt: "2025-10-12T10:03:00Z"
  readyAt: "2025-10-12T10:05:00Z"
  observedGeneration: 1
```

---

## kubectl Commands

### NodeGroup Operations

```bash
# List all NodeGroups
kubectl get nodegroups -n kube-system
kubectl get ng -n kube-system  # short name

# Get detailed information
kubectl describe nodegroup production-workers -n kube-system

# View NodeGroup YAML
kubectl get nodegroup production-workers -n kube-system -o yaml

# Edit NodeGroup (e.g., change min/max nodes)
kubectl edit nodegroup production-workers -n kube-system

# Delete NodeGroup (will terminate all nodes)
kubectl delete nodegroup production-workers -n kube-system

# Watch NodeGroup status changes
kubectl get ng -n kube-system -w
```

### VPSieNode Operations

```bash
# List all VPSieNodes
kubectl get vpsienodes -n kube-system
kubectl get vn -n kube-system  # short name

# Filter by NodeGroup
kubectl get vn -n kube-system -l nodegroup=production-workers

# Get detailed information
kubectl describe vpsienode vpsie-prod-workers-abc123 -n kube-system

# View VPSieNode YAML
kubectl get vn vpsie-prod-workers-abc123 -n kube-system -o yaml

# Watch VPSieNode phase transitions
kubectl get vn -n kube-system -w

# View VPSieNodes in specific phase
kubectl get vn -n kube-system --field-selector status.phase=Provisioning
```

### Custom Columns

```bash
# NodeGroup with custom columns
kubectl get ng -n kube-system \
  -o custom-columns=\
NAME:.metadata.name,\
MIN:.spec.minNodes,\
MAX:.spec.maxNodes,\
CURRENT:.status.currentNodes,\
READY:.status.readyNodes,\
DESIRED:.status.desiredNodes

# VPSieNode with custom columns
kubectl get vn -n kube-system \
  -o custom-columns=\
NAME:.metadata.name,\
PHASE:.status.phase,\
NODE:.status.nodeName,\
VPS-ID:.spec.vpsieInstanceID,\
TYPE:.spec.instanceType,\
IP:.spec.ipAddress
```

---

## Annotations and Labels

### NodeGroup Annotations

| Annotation | Description | Example |
|------------|-------------|---------|
| `autoscaler.vpsie.com/pause` | Pause autoscaling for this NodeGroup | `"true"` |
| `autoscaler.vpsie.com/min-nodes-override` | Temporarily override minNodes | `"5"` |
| `autoscaler.vpsie.com/max-nodes-override` | Temporarily override maxNodes | `"15"` |

### NodeGroup Labels

| Label | Description | Example |
|-------|-------------|---------|
| `autoscaler.vpsie.com/nodegroup` | NodeGroup name (automatically added to nodes) | `"production-workers"` |
| `environment` | Environment identifier | `"production"` |
| `workload-type` | Type of workload | `"general"` |

### VPSieNode Annotations

| Annotation | Description | Example |
|------------|-------------|---------|
| `autoscaler.vpsie.com/do-not-delete` | Prevent automatic deletion | `"true"` |
| `autoscaler.vpsie.com/drain-timeout` | Custom drain timeout in seconds | `"600"` |

### VPSieNode Labels

| Label | Description | Example |
|-------|-------------|---------|
| `nodegroup` | Parent NodeGroup name | `"production-workers"` |
| `autoscaler.vpsie.com/instance-type` | VPSie instance type | `"medium-4cpu-8gb"` |
| `autoscaler.vpsie.com/datacenter` | VPSie datacenter ID | `"dc-us-east-1"` |

---

## Validation Rules

### NodeGroup Validation

- `minNodes` must be >= 0
- `maxNodes` must be >= 1
- `maxNodes` must be >= `minNodes`
- `datacenterID` is required and must not be empty
- `offeringIDs` must contain at least one entry
- `osImageID` is required and must not be empty
- `cpuThreshold` and `memoryThreshold` must be between 0 and 100
- `stabilizationWindowSeconds` must be > 0

### VPSieNode Validation

- `vpsieInstanceID` must be > 0
- `instanceType` is required and must not be empty
- `nodeGroupName` is required and must not be empty
- `datacenterID` is required and must not be empty

---

## Events

The autoscaler emits Kubernetes events for important state changes:

### NodeGroup Events

| Reason | Type | Description |
|--------|------|-------------|
| `ScalingUp` | Normal | NodeGroup is increasing node count |
| `ScalingDown` | Normal | NodeGroup is decreasing node count |
| `ScaleLimitReached` | Warning | Cannot scale beyond maxNodes limit |
| `ProvisioningFailed` | Warning | Failed to provision new node |
| `InvalidConfiguration` | Warning | NodeGroup configuration is invalid |

### VPSieNode Events

| Reason | Type | Description |
|--------|------|-------------|
| `VPSCreated` | Normal | VPS successfully created on VPSie |
| `NodeJoined` | Normal | Node joined Kubernetes cluster |
| `NodeReady` | Normal | Node is ready to accept workloads |
| `Terminating` | Normal | Node termination initiated |
| `ProvisioningFailed` | Warning | Failed to create VPS |
| `JoinFailed` | Warning | Node failed to join cluster |
| `DrainFailed` | Warning | Failed to drain node before termination |

---

## Metrics (Planned)

The controller will expose Prometheus metrics:

```
# NodeGroup metrics
nodegroup_desired_nodes{nodegroup="production-workers"} 5
nodegroup_current_nodes{nodegroup="production-workers"} 5
nodegroup_ready_nodes{nodegroup="production-workers"} 5

# VPSieNode metrics
vpsienode_phase{phase="Ready"} 5
vpsienode_phase{phase="Provisioning"} 2

# Controller metrics
controller_reconcile_duration_seconds{controller="nodegroup"} 0.15
controller_reconcile_errors_total{controller="nodegroup"} 0

# VPSie API metrics
vpsie_api_requests_total{method="CreateVM",status="200"} 150
vpsie_api_request_duration_seconds{method="CreateVM"} 1.2
```

---

## Error Codes

Common error messages and their meanings:

| Error | Cause | Resolution |
|-------|-------|------------|
| `MinNodes exceeds MaxNodes` | Invalid NodeGroup spec | Fix spec: ensure minNodes <= maxNodes |
| `VPSie API authentication failed` | Invalid credentials | Check vpsie-secret configuration |
| `Datacenter not found` | Invalid datacenterID | Verify datacenter ID in VPSie console |
| `Instance type not available` | Invalid offeringID | Check available offerings in datacenter |
| `Maximum nodes reached` | At maxNodes limit | Increase maxNodes or wait for scale-down |
| `Node join timeout` | Node failed to join cluster | Check cloud-init logs, network connectivity |
| `Insufficient VPSie quota` | Account quota exceeded | Contact VPSie support to increase quota |

---

## Version History

| Version | Status | Changes |
|---------|--------|---------|
| v1alpha1 | Current | Initial API version with NodeGroup and VPSieNode CRDs |

---

## Future API Changes (Planned)

### v1alpha2 (Planned)

- Add `nodeSelector` field to NodeGroup for pod affinity
- Add `priority` field for NodeGroup scaling priority
- Add `spotInstances` support for cost optimization
- Add `networkConfig` for VPC and private networking

### v1beta1 (Planned)

- API stability guarantees
- Deprecation notices for removed fields
- Migration guide from v1alpha1
