# VPSie Kubernetes Autoscaler - Example Configurations

This directory contains example YAML manifests for the VPSie Kubernetes Autoscaler custom resources.

## NodeGroup Examples

NodeGroup resources define logical groups of Kubernetes worker nodes with shared properties and scaling policies.

### General Purpose NodeGroup
**File:** `nodegroup-general-purpose.yaml`

A balanced node group for stateless applications with:
- Min: 2 nodes, Max: 10 nodes
- Instance types: small-2cpu-4gb, medium-4cpu-8gb
- Moderate scaling thresholds (80% CPU/memory for scale-up, 50% for scale-down)
- 10-minute stabilization before scale-down

**Use cases:**
- Web applications
- API services
- Microservices
- General stateless workloads

**Apply:**
```bash
kubectl apply -f nodegroup-general-purpose.yaml
```

### High Memory NodeGroup
**File:** `nodegroup-high-memory.yaml`

A specialized node group for memory-intensive workloads with:
- Min: 1 node, Max: 5 nodes
- Instance types: large-8cpu-16gb, xlarge-16cpu-32gb
- Taints to reserve nodes for specific workloads
- More aggressive scale-up (70% CPU, 75% memory)
- Conservative scale-down (15-minute stabilization)

**Use cases:**
- In-memory databases (Redis, Memcached)
- Data processing (Spark, Hadoop)
- Caching layers
- Analytics workloads

**Apply:**
```bash
kubectl apply -f nodegroup-high-memory.yaml
```

**Important:** Pods must tolerate the taint to schedule on these nodes:
```yaml
tolerations:
  - key: workload-type
    operator: Equal
    value: memory-intensive
    effect: NoSchedule
```

### Spot/Batch Instances NodeGroup
**File:** `nodegroup-spot-instances.yaml`

A cost-optimized node group for fault-tolerant batch workloads with:
- Min: 0 nodes (can scale to zero), Max: 20 nodes
- Instance types: micro-1cpu-1gb, small-2cpu-4gb
- Very aggressive scaling (15s scale-up window, 5min scale-down)
- Low utilization thresholds (20% for scale-down)
- Taints to prevent non-batch workloads

**Use cases:**
- Batch processing
- CI/CD jobs
- Data pipelines
- ML training (non-critical)
- Queue workers

**Apply:**
```bash
kubectl apply -f nodegroup-spot-instances.yaml
```

**Important:** Pods must tolerate the taints:
```yaml
tolerations:
  - key: workload-type
    operator: Equal
    value: batch
    effect: NoSchedule
  - key: preemptible
    operator: Equal
    value: "true"
    effect: PreferNoSchedule
```

## VPSieNode Example

**File:** `vpsienode-example.yaml`

VPSieNode resources represent individual VPSie VPS instances that are part of the Kubernetes cluster. These are typically created and managed automatically by the autoscaler controller.

**Note:** You usually don't need to create VPSieNode resources manually. This example is provided for reference to understand the resource structure and status information.

## Prerequisites

Before applying these examples:

1. **Install the CRDs:**
   ```bash
   kubectl apply -f ../crds/
   ```

2. **Configure VPSie credentials:**
   Create a secret with your VPSie API credentials:
   ```bash
   kubectl create secret generic vpsie-credentials \
     --namespace=kube-system \
     --from-literal=clientId=your-client-id \
     --from-literal=clientSecret=your-client-secret
   ```

3. **Update the examples:**
   - Replace `datacenterID` with actual VPSie datacenter IDs
   - Replace `offeringIDs` with actual VPSie offering/boxsize IDs
   - Replace `osImageID` with actual VPSie OS image IDs
   - Update the `userData` section with your cluster's kubeadm join command

4. **Deploy the autoscaler controller:**
   ```bash
   kubectl apply -f ../manifests/controller.yaml
   ```

## Checking NodeGroup Status

After creating a NodeGroup, check its status:

```bash
# List all NodeGroups
kubectl get nodegroups -n kube-system

# Get detailed information
kubectl describe nodegroup general-purpose -n kube-system

# Watch NodeGroup status
kubectl get nodegroups -n kube-system -w
```

Example output:
```
NAME              MIN   MAX   DESIRED   CURRENT   READY   AGE
general-purpose   2     10    3         3         3       5m
high-memory       1     5     1         1         1       5m
spot-instances    0     20    0         0         0       5m
```

## Checking VPSieNode Status

View VPSieNode resources (individual nodes):

```bash
# List all VPSieNodes
kubectl get vpsienodes -n kube-system

# Short form
kubectl get vn -n kube-system

# Get detailed information
kubectl describe vpsienode vpsie-node-6266 -n kube-system
```

Example output:
```
NAME              VPS ID   NODE                PHASE   INSTANCE TYPE      NODEGROUP          AGE
vpsie-node-6266   6266     vpsie-node-6266     Ready   small-2cpu-4gb     general-purpose    10m
vpsie-node-6267   6267     vpsie-node-6267     Ready   small-2cpu-4gb     general-purpose    10m
vpsie-node-6268   6268     vpsie-node-6268     Ready   large-8cpu-16gb    high-memory        8m
```

## Scaling Behavior

### Automatic Scale-Up

The autoscaler will automatically add nodes when:
1. Pods fail to schedule due to insufficient resources (event-driven)
2. Average node CPU/memory exceeds the scale-up thresholds
3. Current node count is below the maximum

### Automatic Scale-Down

The autoscaler will automatically remove nodes when:
1. Node utilization is below scale-down thresholds for the stabilization period
2. All pods on the node can be safely rescheduled on other nodes
3. Current node count is above the minimum
4. Sufficient time has passed since the last scale-up (cooldown period)

### Manual Scaling

You can temporarily adjust the desired number of nodes by editing the NodeGroup:

```bash
# Edit NodeGroup
kubectl edit nodegroup general-purpose -n kube-system

# Or scale via patch
kubectl patch nodegroup general-purpose -n kube-system \
  --type=merge \
  -p '{"spec":{"minNodes":5}}'
```

## Monitoring and Troubleshooting

### View Autoscaler Logs

```bash
# Get autoscaler controller pod
kubectl get pods -n kube-system -l app=vpsie-autoscaler

# View logs
kubectl logs -n kube-system -l app=vpsie-autoscaler --tail=100 -f
```

### Check Events

```bash
# NodeGroup events
kubectl get events -n kube-system --field-selector involvedObject.name=general-purpose

# VPSieNode events
kubectl get events -n kube-system --field-selector involvedObject.name=vpsie-node-6266

# All autoscaler-related events
kubectl get events -n kube-system | grep -i autoscaler
```

### Common Issues

**Nodes not joining cluster:**
- Verify the `userData` script includes correct kubeadm join command
- Check VPSie VPS console for bootstrap script errors
- Verify network connectivity between VPS and control plane

**Scale-up not triggering:**
- Check if NodeGroup is at maximum capacity
- Verify scale-up policy is enabled
- Check autoscaler controller logs for errors
- Verify VPSie API credentials are correct

**Scale-down too aggressive:**
- Increase `stabilizationWindowSeconds` in `scaleDownPolicy`
- Increase `cpuThreshold` and `memoryThreshold` in `scaleDownPolicy`
- Increase `cooldownSeconds` to wait longer after scale-up

**Pods evicted unexpectedly:**
- Set PodDisruptionBudgets for critical applications
- Use node affinity to prevent pods from scheduling on spot/preemptible nodes
- Disable scale-down temporarily: set `scaleDownPolicy.enabled: false`

## Best Practices

1. **Start Conservative:** Begin with higher scale-down thresholds and longer stabilization windows, then adjust based on observed behavior.

2. **Use Multiple NodeGroups:** Separate workloads by resource requirements (general, memory-intensive, CPU-intensive, batch).

3. **Set PodDisruptionBudgets:** Protect critical applications from unexpected evictions during scale-down.

4. **Use Node Affinity:** Direct pods to appropriate node groups using node selectors and affinity rules.

5. **Monitor Costs:** Track VPSie spending and adjust instance types or scaling policies to optimize costs.

6. **Test Scale-Down:** Verify that scale-down works correctly before enabling in production.

7. **Set Resource Requests:** Always set CPU and memory requests on pods so the autoscaler can make informed scaling decisions.

## Further Reading

- [VPSie Autoscaler Documentation](../../docs/README.md)
- [Product Requirements Document](../../docs/PRD.md)
- [Architecture Overview](../../docs/ARCHITECTURE.md)
- [Kubernetes Cluster Autoscaler Concepts](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler)
