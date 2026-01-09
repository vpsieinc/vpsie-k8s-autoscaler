# Scale-Down Blocked Runbook

## Symptom
Scale-down operations are consistently blocked by safety checks.

## Alert
- `ScaleDownBlockedPersistent` (warning)

## Quick Check
```bash
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep scale_down_blocked_total
```

## Block Reasons

| Reason | Description | Resolution |
|--------|-------------|------------|
| `pdb` | PodDisruptionBudget violation | Increase replicas or adjust PDB |
| `affinity` | Pod affinity rules prevent eviction | Review affinity rules |
| `capacity` | Would drop below minNodes | Increase minNodes or add more nodes |
| `cooldown` | Scale-down cooldown active | Wait for cooldown to expire |
| `local_storage` | Pods with local storage | Use PVCs instead of emptyDir |
| `system_pods` | Critical system pods | Use daemonset ignore flag |

## Diagnosis
```bash
# Check which safety check is blocking
kubectl logs -n kube-system deployment/vpsie-autoscaler | grep -i "blocked\|safety"

# Check PDBs
kubectl get pdb -A -o wide

# Check pods on target nodes
kubectl get pods -A -o wide --field-selector spec.nodeName=<node>
```

## Resolution by Reason

### PDB Blocking
```bash
# Check current PDB state
kubectl describe pdb <name> -n <namespace>

# Options:
# 1. Scale up the deployment
# 2. Temporarily adjust minAvailable
# 3. Wait for enough replicas
```

### Local Storage
```bash
# Find pods with local storage
kubectl get pods -o json | jq '.items[] | select(.spec.volumes[]?.emptyDir) | .metadata.name'

# Consider migrating to PVCs
```

### Cooldown
```bash
# Check cooldown config
kubectl get nodegroup <name> -o jsonpath='{.spec.scaleDownCooldown}'

# Wait or reduce cooldown period
```

## Related
- [Scale-Down Stuck](scale-down-stuck.md) - For when scale-down is completely stalled
