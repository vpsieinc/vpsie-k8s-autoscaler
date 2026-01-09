# Scale-Down Stuck Runbook

## Symptom
NodeGroup shows `DesiredNodes < CurrentNodes` but nodes are not being removed, or nodes remain cordoned without deletion.

## Alert
- `VPSieAutoscalerScaleDownStalled` (warning)

## Impact
- Unnecessary infrastructure costs
- Resource inefficiency
- Potential cluster capacity issues

## Diagnosis Steps

### 1. Check Scale-Down Status
```bash
kubectl get nodegroups -A -o wide
kubectl describe nodegroup <name> -n <namespace>
```

Look for:
- `Status.CurrentNodes` vs `Status.DesiredNodes`
- Scaling conditions
- Recent events mentioning blocked scale-down

### 2. Check for Blocked Scale-Down
```bash
# Check scale-down blocked metric
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep scale_down_blocked_total
```

### 3. Check Node Drain Status
```bash
# List cordoned nodes
kubectl get nodes | grep SchedulingDisabled

# Check drain status
kubectl describe node <node-name> | grep -A 10 "Taints"
```

### 4. Check PodDisruptionBudgets
```bash
kubectl get pdb -A
kubectl describe pdb <name> -n <namespace>
```

### 5. Check Controller Logs
```bash
kubectl logs -n kube-system deployment/vpsie-autoscaler -f --tail=200 | grep -i "scale.down\|drain\|blocked\|pdb"
```

## Common Causes

### 1. PodDisruptionBudget Blocking
**Symptoms:** Logs show "PDB violation", `scale_down_blocked{reason="pdb"}`

**Diagnosis:**
```bash
# Check PDB status
kubectl get pdb -A -o wide

# Check disruptions allowed
kubectl describe pdb <name> -n <namespace> | grep "Allowed disruptions"
```

**Resolution:**
- Wait for more replicas to become available
- Temporarily adjust PDB `minAvailable`
- Scale up deployment before scale-down

### 2. Pods with Local Storage
**Symptoms:** Logs show "local storage", pods with emptyDir/hostPath

**Diagnosis:**
```bash
# Find pods with local storage on target node
kubectl get pods -A -o wide --field-selector spec.nodeName=<node-name>
kubectl describe pod <pod-name> -n <namespace> | grep -A 5 "Volumes"
```

**Resolution:**
- Move data or use PVC instead of emptyDir
- Set `--skip-nodes-with-local-storage=true` if acceptable
- Manually drain with `--force` flag

### 3. System Pods
**Symptoms:** DaemonSet pods blocking drain

**Resolution:**
```bash
# Drain with ignore daemonsets
kubectl drain <node-name> --ignore-daemonsets --delete-emptydir-data
```

### 4. Cooldown Period Active
**Symptoms:** Logs show "cooldown", recent scaling activity

**Diagnosis:**
```bash
# Check last scale event time
kubectl describe nodegroup <name> | grep "Last Scale"

# Check cooldown config
kubectl get nodegroup <name> -o jsonpath='{.spec.scaleDownCooldown}'
```

**Resolution:**
- Wait for cooldown to expire
- Adjust `scaleDownCooldown` in NodeGroup spec if appropriate

### 5. Minimum Nodes Constraint
**Symptoms:** `CurrentNodes == MinNodes`

**Diagnosis:**
```bash
kubectl get nodegroup <name> -o jsonpath='{.spec.minNodes}'
kubectl get nodegroup <name> -o jsonpath='{.status.currentNodes}'
```

**Resolution:**
- Reduce `minNodes` if appropriate
- This is expected behavior when at minimum capacity

### 6. VM Deletion Failed
**Symptoms:** VPSieNode deleted but VM still running

**Diagnosis:**
```bash
# Check VPSie console for orphaned VMs
# Check finalizer status
kubectl get vpsienode <name> -o jsonpath='{.metadata.finalizers}'
```

**Resolution:**
```bash
# Manually delete VM in VPSie console, then remove finalizer
kubectl patch vpsienode <name> -p '{"metadata":{"finalizers":[]}}' --type=merge
```

## Resolution Steps

### Manual Drain
```bash
# Cordon node
kubectl cordon <node-name>

# Drain with options
kubectl drain <node-name> \
  --ignore-daemonsets \
  --delete-emptydir-data \
  --grace-period=60 \
  --timeout=5m
```

### Force Scale-Down (Use with Caution)
```bash
# Delete specific VPSieNode
kubectl delete vpsienode <name> -n <namespace>

# If stuck with finalizer
kubectl patch vpsienode <name> -p '{"metadata":{"finalizers":[]}}' --type=merge
```

### Unblock PDB
```bash
# Temporarily increase replicas
kubectl scale deployment <name> --replicas=<higher-count>

# After drain completes, restore original count
```

## Prevention
- Configure appropriate PDB `minAvailable` values
- Set realistic `scaleDownCooldown` periods
- Monitor `scale_down_blocked_total` metric
- Use cluster-autoscaler-safe annotations on critical pods

## Escalation
If unable to resolve:
1. Collect controller logs with scale-down context
2. Export PDB configurations
3. Document which pods are blocking drain
4. Contact platform team with diagnosis information
