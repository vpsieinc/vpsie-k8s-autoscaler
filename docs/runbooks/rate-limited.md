# Rate Limited Runbook

## Symptom
VPSie API requests are being rate limited (429 responses).

## Alert
- `VPSieAPIRateLimited` (warning)

## Quick Check
```bash
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep rate_limit
```

## Diagnosis
```bash
# Check rate limit metrics
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep -E "rate_limit|api_requests"

# Check logs for rate limit errors
kubectl logs -n kube-system deployment/vpsie-autoscaler | grep -i "rate.limit\|429"
```

## Common Causes
1. **High scaling activity** - Many NodeGroups scaling simultaneously
2. **Runaway reconciliation** - Reconcile loop triggering too frequently
3. **Low quota** - VPSie account has limited API quota

## Resolution

### Short-term
- Wait for rate limit window to pass (usually 1 minute)
- Built-in rate limiter should prevent most issues

### Long-term
- Request quota increase from VPSie
- Consolidate NodeGroups to reduce API calls
- Increase reconciliation interval

### Check Rate Limiter Config
```bash
# Default is 100 requests/minute
# Can be adjusted in controller configuration
```

## Prevention
- Monitor `vpsie_api_rate_limited_total` metric
- Set up alerts before hitting limits
- Plan capacity for API calls during scaling events
