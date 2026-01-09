# VPSie API Errors Runbook

## Symptom
VPSie API requests are failing, causing autoscaler operations to fail. May see increased `vpsie_api_errors_total` metric.

## Alert
- `VPSieAPIHighErrorRate` (critical)
- `VPSieAPICircuitBreakerOpen` (critical)

## Impact
- Unable to provision new nodes
- Unable to terminate nodes
- Autoscaler operations halted
- Cluster scaling suspended

## Diagnosis Steps

### 1. Check API Error Metrics
```bash
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep "vpsie_api"
```

Key metrics to examine:
- `vpsie_api_errors_total` - errors by type
- `vpsie_api_requests_total` - request counts
- `vpsie_api_circuit_breaker_state` - circuit breaker status

### 2. Check Controller Logs
```bash
kubectl logs -n kube-system deployment/vpsie-autoscaler -f --tail=200 | grep -i "api\|error\|failed\|vpsie"
```

### 3. Check Circuit Breaker State
```bash
# Check if circuit breaker is open
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep circuit_breaker_state
```

### 4. Test API Connectivity
```bash
# Test from within the cluster
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -v https://api.vpsie.com/v2/health
```

### 5. Check VPSie Status
Visit https://status.vpsie.com or check programmatically:
```bash
curl -s https://status.vpsie.com/api/v2/status.json | jq '.status'
```

## Error Types and Solutions

### 1. Authentication Errors (401)
**Symptoms:** `vpsie_api_errors_total{error_type="unauthorized"}` increasing

**Diagnosis:**
```bash
# Verify secret exists
kubectl get secret vpsie-secret -n kube-system

# Check credential keys
kubectl get secret vpsie-secret -n kube-system -o jsonpath='{.data}' | jq 'keys'
```

**Resolution:**
```bash
# Update credentials
kubectl create secret generic vpsie-secret \
  --from-literal=clientId='<new-client-id>' \
  --from-literal=clientSecret='<new-client-secret>' \
  -n kube-system \
  --dry-run=client -o yaml | kubectl apply -f -

# Restart controller to pick up new credentials
kubectl rollout restart deployment/vpsie-autoscaler -n kube-system
```

### 2. Rate Limiting (429)
**Symptoms:** `vpsie_api_errors_total{error_type="rate_limited"}` increasing

**Diagnosis:**
```bash
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep rate_limit
```

**Resolution:**
- Wait for rate limit window to pass
- Reduce scaling frequency
- Contact VPSie for rate limit increase
- Check for runaway reconciliation loops

### 3. Server Errors (500)
**Symptoms:** `vpsie_api_errors_total{error_type="server_error"}` increasing

**Diagnosis:**
- Check VPSie status page
- Review specific operation that failed

**Resolution:**
- Wait for VPSie to resolve
- Retry failed operations manually if urgent
- Circuit breaker will automatically retry after timeout

### 4. Not Found (404)
**Symptoms:** `vpsie_api_errors_total{error_type="not_found"}` increasing

**Diagnosis:**
```bash
# Check for invalid resource IDs in NodeGroup
kubectl get nodegroups -A -o yaml | grep -E "offeringID|datacenterID|osImageID"
```

**Resolution:**
- Verify offering IDs are valid
- Check datacenter availability
- Update NodeGroup with correct IDs

### 5. Forbidden (403)
**Symptoms:** `vpsie_api_errors_total{error_type="forbidden"}` increasing

**Diagnosis:**
- Check account permissions in VPSie console
- Verify API client has required scopes

**Resolution:**
- Update API client permissions in VPSie
- Generate new credentials with proper scopes

## Circuit Breaker Recovery

When circuit breaker is open:

### Check State
```bash
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep circuit_breaker
```

### Wait for Auto-Recovery
- Circuit breaker will enter half-open state after timeout (default: 30s)
- Test requests will be attempted
- Success leads to closed state

### Manual Recovery
```bash
# Restart controller to reset circuit breaker
kubectl rollout restart deployment/vpsie-autoscaler -n kube-system
```

## Resolution Steps

### Immediate Actions
1. Check VPSie API status
2. Verify credentials are valid
3. Check rate limit status
4. Review recent changes to NodeGroup configs

### Credential Rotation
```bash
# Get new credentials from VPSie console
# Update secret
kubectl create secret generic vpsie-secret \
  --from-literal=clientId='<client-id>' \
  --from-literal=clientSecret='<client-secret>' \
  -n kube-system \
  --dry-run=client -o yaml | kubectl apply -f -
```

### Force Reconciliation
```bash
# Trigger reconciliation after fixing issues
kubectl annotate nodegroup <name> reconcile-trigger=$(date +%s) --overwrite
```

## Prevention
- Monitor `VPSieAPIHighErrorRate` alert
- Set up credential rotation automation
- Review rate limits during capacity planning
- Configure appropriate circuit breaker settings

## Escalation
1. Collect error logs: `kubectl logs -n kube-system deployment/vpsie-autoscaler > api-errors.txt`
2. Export metrics snapshot
3. Check VPSie status page
4. If VPSie issue: contact VPSie support
5. If credential issue: contact security team
