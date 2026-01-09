# Slow Reconciliation Runbook

## Symptom
Controller reconciliation is taking longer than expected, causing delayed scaling responses.

## Alert
- `VPSieAutoscalerSlowReconciliation` (warning)
- `ReconciliationQueueDepthHigh` (warning)

## Impact
- Delayed scaling decisions
- Increased time to handle pending pods
- Potential workload disruption during high load

## Diagnosis Steps

### 1. Check Reconciliation Metrics
```bash
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep -E "reconcile|queue"
```

Key metrics:
- `controller_reconcile_duration_seconds` - reconciliation time
- `reconciliation_queue_depth` - pending reconciliations
- `controller_reconcile_total` - total reconciliations

### 2. Check Controller Resource Usage
```bash
kubectl top pod -n kube-system -l app=vpsie-autoscaler
```

### 3. Check Controller Logs
```bash
kubectl logs -n kube-system deployment/vpsie-autoscaler --tail=200 | grep -i "reconcil\|slow\|timeout"
```

### 4. Check NodeGroup Count
```bash
kubectl get nodegroups -A | wc -l
```

### 5. Check API Latency
```bash
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep vpsie_api_request_duration
```

## Common Causes

### 1. High NodeGroup Count
**Symptoms:** Many NodeGroups, high queue depth

**Diagnosis:**
```bash
kubectl get nodegroups -A | wc -l
```

**Resolution:**
- Consolidate NodeGroups where possible
- Increase controller replicas (if using leader election)
- Increase `MaxConcurrentReconciles` in controller config

### 2. VPSie API Latency
**Symptoms:** High `vpsie_api_request_duration_seconds`

**Diagnosis:**
```bash
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep api_request_duration | sort -t'=' -k2 -rn | head
```

**Resolution:**
- Check VPSie API status
- Review network connectivity
- Consider caching frequently accessed data

### 3. Resource Constraints
**Symptoms:** High CPU/memory usage, OOM kills

**Diagnosis:**
```bash
kubectl top pod -n kube-system -l app=vpsie-autoscaler
kubectl describe pod -n kube-system -l app=vpsie-autoscaler | grep -A 5 "Resources"
```

**Resolution:**
```yaml
# Increase resource limits
resources:
  limits:
    cpu: "1"
    memory: "512Mi"
  requests:
    cpu: "200m"
    memory: "256Mi"
```

### 4. Kubernetes API Server Slow
**Symptoms:** Slow list/watch operations

**Diagnosis:**
```bash
kubectl get --raw /metrics | grep apiserver_request_duration_seconds
```

**Resolution:**
- Reduce label selector scope
- Use resource version for efficient watches
- Check API server health

### 5. Large VPSieNode Lists
**Symptoms:** Slow when listing VPSieNodes

**Diagnosis:**
```bash
time kubectl get vpsienodes -A
```

**Resolution:**
- Ensure indexes are used for label selectors
- Clean up orphaned VPSieNodes
- Use namespace filtering

## Resolution Steps

### Increase Concurrency
```yaml
# In controller configuration
maxConcurrentReconciles: 3  # Default is 1
```

### Increase Resources
```bash
kubectl patch deployment vpsie-autoscaler -n kube-system -p '{
  "spec": {
    "template": {
      "spec": {
        "containers": [{
          "name": "controller",
          "resources": {
            "limits": {"cpu": "1", "memory": "512Mi"},
            "requests": {"cpu": "200m", "memory": "256Mi"}
          }
        }]
      }
    }
  }
}'
```

### Force Queue Clear
```bash
# Restart controller to clear queue
kubectl rollout restart deployment/vpsie-autoscaler -n kube-system
```

### Reduce Reconciliation Frequency
```yaml
# In NodeGroup spec
spec:
  reconcileInterval: 60s  # Increase from default
```

## Prevention
- Monitor `controller_reconcile_duration_seconds` P99
- Set up `ReconciliationQueueDepthHigh` alert
- Regular resource usage review
- Capacity planning for NodeGroup growth

## Escalation
1. Collect metrics snapshot
2. Export controller logs with timing info
3. Document NodeGroup count and sizes
4. Contact platform team with performance data
