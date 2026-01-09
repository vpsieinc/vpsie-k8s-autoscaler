# High API Error Rate Runbook

## Symptom
VPSie API error rate exceeds threshold, triggering alerts.

## Alert
- `VPSieAPIHighErrorRate` (critical) - >10% error rate for 5 minutes

## Impact
- Autoscaling operations degraded or failing
- Node provisioning delays
- Cost optimization features disabled

## Quick Reference

### Check Current Error Rate
```bash
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep -E "vpsie_api_requests_total|vpsie_api_errors_total"
```

### Check Circuit Breaker
```bash
kubectl exec -n kube-system deployment/vpsie-autoscaler -- \
  curl -s localhost:8080/metrics | grep circuit_breaker_state
```

## Immediate Actions

1. **Check VPSie Status**: https://status.vpsie.com
2. **Check Error Types**: See which errors are dominant
3. **Check Credentials**: Verify `vpsie-secret` is valid
4. **Check Rate Limits**: Look for 429 errors

## Error Type Resolution

| Error Type | Metric Label | Resolution |
|------------|--------------|------------|
| Authentication | `unauthorized` | Rotate credentials |
| Rate Limited | `rate_limited` | Wait or request quota increase |
| Not Found | `not_found` | Check resource IDs in NodeGroup |
| Server Error | `server_error` | Wait for VPSie recovery |
| Forbidden | `forbidden` | Check account permissions |

## Related Runbooks
- [VPSie API Errors](vpsie-api-errors.md) - Detailed troubleshooting
- [Scale-Up Failure](scale-up-failure.md) - If scaling is affected
