# Scale-Up Failure Runbook

## Symptom
NodeGroup shows `DesiredNodes > CurrentNodes` but new nodes are not being provisioned, or VPSieNodes are stuck in `Pending` or `Provisioning` phase.

## Alert
- `VPSieAutoscalerScaleUpFailure` (critical)

## Impact
- Workloads cannot be scheduled
- Application degradation or outages
- Pending pods accumulating

## Diagnosis Steps

### 1. Check NodeGroup Status
```bash
kubectl get nodegroups -A -o wide
kubectl describe nodegroup <name> -n <namespace>
```

Look for:
- `Status.CurrentNodes` vs `Status.DesiredNodes`
- Recent events showing failures
- Condition `Ready=False`

### 2. Check VPSieNode Status
```bash
kubectl get vpsienodes -A -o wide
kubectl describe vpsienode <name> -n <namespace>
```

Look for:
- Nodes stuck in `Pending` or `Provisioning` phase
- `ProvisionError` in conditions
- Missing `VPSieInstanceID`

### 3. Check Controller Logs
```bash
kubectl logs -n kube-system deployment/vpsie-autoscaler -f --tail=100 | grep -i "error\|failed\|provision"
```

### 4. Check VPSie API Health
```bash
# Check circuit breaker state
kubectl exec -n kube-system deployment/vpsie-autoscaler -- curl localhost:8080/metrics | grep circuit_breaker

# Check API error rate
kubectl exec -n kube-system deployment/vpsie-autoscaler -- curl localhost:8080/metrics | grep vpsie_api_errors_total
```

### 5. Verify VPSie Account
- Check VPSie console for quota limits
- Verify datacenter availability
- Check offering availability

## Common Causes

### 1. VPSie API Errors
**Symptoms:** `VPSieAPIErrors` metric increasing, error logs mentioning API failures

**Resolution:**
```bash
# Check VPSie API status
curl -s https://status.vpsie.com/api/v2/status.json

# Verify credentials
kubectl get secret vpsie-secret -n kube-system -o jsonpath='{.data.clientId}' | base64 -d
```

### 2. Quota Exceeded
**Symptoms:** 403/429 errors in logs, "quota exceeded" messages

**Resolution:**
- Request quota increase from VPSie
- Delete unused VMs in VPSie console
- Reduce `maxNodes` in NodeGroup spec

### 3. Invalid Configuration
**Symptoms:** Validation errors in events, immediate failures

**Resolution:**
```bash
# Check NodeGroup spec
kubectl get nodegroup <name> -o yaml

# Verify offering exists
kubectl logs -n kube-system deployment/vpsie-autoscaler | grep "offering"
```

Fix invalid:
- `offeringIDs` - verify offering IDs exist
- `datacenterID` - verify datacenter is available
- `osImageID` - verify OS image exists

### 4. Circuit Breaker Open
**Symptoms:** `circuit breaker is open` in logs, `circuit_breaker_state{state="open"}=1`

**Resolution:**
- Wait for circuit breaker timeout (default: 30s)
- Check underlying VPSie API issues
- Check `VPSieAPICircuitBreakerStateChanges` metric for patterns

### 5. Network Issues
**Symptoms:** Connection timeouts, TLS errors

**Resolution:**
```bash
# Test connectivity from controller pod
kubectl exec -n kube-system deployment/vpsie-autoscaler -- curl -v https://api.vpsie.com/v2/health
```

## Resolution Steps

### Immediate Mitigation
1. Check VPSie API status
2. Verify credentials are valid
3. Check for quota issues
4. Review recent configuration changes

### For Stuck VPSieNodes
```bash
# Delete stuck VPSieNode (controller will recreate)
kubectl delete vpsienode <stuck-node-name> -n <namespace>

# Force reconciliation
kubectl annotate nodegroup <name> reconcile-trigger=$(date +%s) --overwrite
```

### For Configuration Issues
```bash
# Fix NodeGroup configuration
kubectl edit nodegroup <name> -n <namespace>

# Verify changes applied
kubectl describe nodegroup <name> -n <namespace>
```

## Prevention
- Set up monitoring for `VPSieAutoscalerScaleUpFailure` alert
- Configure PodDisruptionBudgets for critical workloads
- Regular credential rotation testing
- Quota monitoring dashboards

## Escalation
If unable to resolve:
1. Collect controller logs: `kubectl logs -n kube-system deployment/vpsie-autoscaler > autoscaler-logs.txt`
2. Export NodeGroup/VPSieNode status
3. Open ticket with VPSie support if API issues
4. Contact platform team with collected logs
