# Rebalancer Rollback Runbook

## Symptom
Rebalancer operation has failed mid-execution and a rollback is needed, or a rollback has been triggered automatically.

## Alert
- `RebalancerRollbackTriggered` (critical)
- `RebalancerOperationFailed` (critical)

## Impact
- Nodes may be in inconsistent state
- Some nodes cordoned but not drained
- New nodes provisioned but old ones not terminated
- Workload disruption possible

## Diagnosis Steps

### 1. Check Rebalancer Status
```bash
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep rebalancer
```

Key metrics:
- `rebalancer_operations_total{result="failure"}` - failed operations
- `rebalancer_operations_total{operation="rollback"}` - rollback count

### 2. Check Controller Logs
```bash
kubectl logs -n kube-system deployment/vpsie-autoscaler --tail=500 | grep -i "rebalanc\|rollback\|migration"
```

### 3. Check Node Status
```bash
# List cordoned nodes
kubectl get nodes | grep SchedulingDisabled

# Check node conditions
kubectl describe nodes | grep -A 5 "Conditions"
```

### 4. Check VPSieNodes
```bash
kubectl get vpsienodes -A -o wide
kubectl describe vpsienodes -A | grep -E "Phase|Reason"
```

### 5. Check Workload Status
```bash
kubectl get pods -A -o wide | grep -v Running
```

## Common Causes

### 1. VM Provisioning Failed
**Symptoms:** Replacement node not created, old node still running

**Diagnosis:**
```bash
kubectl get vpsienodes -A | grep -E "Provisioning|Failed"
```

**Resolution:**
- Delete failed VPSieNode
- Uncordon old node
- Retry operation later

### 2. Drain Failed
**Symptoms:** Pods still running on cordoned node

**Diagnosis:**
```bash
kubectl get pods -A -o wide --field-selector spec.nodeName=<cordoned-node>
```

**Resolution:**
```bash
# Check PDBs
kubectl get pdb -A

# Force drain if safe
kubectl drain <node> --ignore-daemonsets --delete-emptydir-data --force
```

### 3. Workload Migration Failed
**Symptoms:** Pods failing to schedule on new nodes

**Diagnosis:**
```bash
kubectl get events -A | grep -i "failed\|error" | head -20
kubectl describe pod <pending-pod> -n <namespace>
```

**Resolution:**
- Check new node capacity
- Verify node labels/taints
- Check pod affinity rules

### 4. Timeout
**Symptoms:** Operation exceeded time limit

**Diagnosis:**
```bash
kubectl logs -n kube-system deployment/vpsie-autoscaler | grep -i "timeout"
```

**Resolution:**
- Increase timeout configuration
- Break into smaller batches
- Address underlying slow operations

## Manual Rollback Steps

### 1. Identify Affected Nodes
```bash
# List all cordoned nodes
kubectl get nodes | grep SchedulingDisabled

# List VPSieNodes involved
kubectl get vpsienodes -A -l rebalance-operation=in-progress
```

### 2. Uncordon Old Nodes
```bash
# Uncordon each affected node
kubectl uncordon <node-name>

# Verify
kubectl get nodes | grep <node-name>
```

### 3. Terminate New Nodes (if provisioned)
```bash
# Find newly provisioned VPSieNodes
kubectl get vpsienodes -A --sort-by=.metadata.creationTimestamp | tail -5

# Delete new nodes that haven't received workloads
kubectl delete vpsienode <new-node-name> -n <namespace>
```

### 4. Verify Workload Health
```bash
# Check all pods are running
kubectl get pods -A | grep -v Running

# Verify no pending pods
kubectl get pods -A --field-selector status.phase=Pending
```

### 5. Clear Rebalance State
```bash
# Remove rebalance labels
kubectl label vpsienodes -A rebalance-operation-
kubectl label nodes -l rebalance-operation rebalance-operation-
```

## Automatic Rollback Behavior

The rebalancer automatically triggers rollback when:
1. New node provisioning fails
2. Drain timeout exceeded
3. Workload health check fails
4. Cluster health degrades

### Rollback Actions
1. Uncordon all previously cordoned nodes
2. Terminate newly provisioned nodes (if any)
3. Verify cluster returns to pre-operation state
4. Log rollback event for audit

## Prevention

### Pre-flight Checks
- Ensure sufficient capacity before starting
- Verify PDBs won't block drain
- Check cluster health status

### Safe Defaults
```yaml
# Rebalancer configuration
rebalancer:
  strategy: rolling      # Safer than surge
  maxConcurrent: 1       # One node at a time
  drainTimeout: 300s     # Allow time for graceful drain
  healthCheckInterval: 10s
  rollbackOnFailure: true
```

### Maintenance Windows
- Schedule rebalancing during low-traffic periods
- Notify teams before major rebalancing operations

## Post-Rollback Actions

1. **Verify Cluster Health**
   ```bash
   kubectl get nodes -o wide
   kubectl get pods -A | grep -v Running | head
   ```

2. **Check Cost Impact**
   - Review VPSie console for any orphaned resources
   - Verify billing for terminated nodes

3. **Document Incident**
   - Record what triggered the rollback
   - Note any manual interventions

4. **Plan Retry**
   - Address root cause before retrying
   - Consider smaller batch sizes
   - Schedule during lower-risk window

## Escalation
1. Collect rebalancer logs
2. Export node and VPSieNode states
3. Document current cluster state
4. Contact platform team with incident details
