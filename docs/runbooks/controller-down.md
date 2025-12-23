# Runbook: Controller Down

**Last Updated:** 2025-12-22
**Alert Severity:** Critical
**Component:** controller
**Team:** platform

---

## Alert Overview

**Alert Name:** `ControllerDown`

**Summary:** The VPSie Autoscaler controller has stopped reconciling NodeGroups, indicating it may have crashed or become unresponsive.

**Prometheus Expression:**
```promql
absent(controller_reconcile_total{controller="nodegroup"}) == 1
or
rate(controller_reconcile_total{controller="nodegroup"}[10m]) == 0
```

**Threshold:** No reconciliation activity for 10 minutes

**Duration:** Fires immediately after 10-minute window

---

## Impact Assessment

### Severity: Critical

**Immediate Impact:**
- **No automatic scaling operations** - Cluster capacity will not adjust to workload demands
- **Pending pods will remain unscheduled** - New workloads cannot get resources
- **Manual intervention required** - Operators must scale nodes manually or restart controller

**Potential Escalation:**
- **Service outages** if traffic spikes and nodes aren't scaled up
- **Resource waste** if nodes aren't scaled down during low traffic
- **Cascading failures** if dependent services can't get capacity

**Affected Components:**
- NodeGroup reconciliation loop
- VPSieNode provisioning
- Scale-up and scale-down operations
- Cost optimization and rebalancing

---

## Prerequisites

Before starting troubleshooting, ensure you have:

- [ ] kubectl access to the cluster
- [ ] Access to Grafana dashboards
- [ ] Access to Prometheus queries
- [ ] Ability to restart deployments
- [ ] Autoscaler logs access

**Required Tools:**
```bash
kubectl
```

---

## Diagnostic Steps

### Step 1: Verify Alert Status

```bash
# Check if alert is still firing
kubectl get pods -n kube-system -l app=vpsie-autoscaler

# Quick status check
POD=$(kubectl get pods -n kube-system -l app=vpsie-autoscaler -o jsonpath='{.items[0].metadata.name}')
echo "Controller pod: $POD"
kubectl get pod -n kube-system $POD
```

### Step 2: Check Pod Health

```bash
# Get pod status and restart count
kubectl get pods -n kube-system -l app=vpsie-autoscaler -o wide

# Check pod events
kubectl describe pod -n kube-system -l app=vpsie-autoscaler | grep -A 20 Events

# Check for recent restarts
kubectl get pods -n kube-system -l app=vpsie-autoscaler -o json | \
  jq '.items[].status.containerStatuses[] | {name: .name, restartCount: .restartCount, state: .state}'
```

**Expected Output (Healthy):**
```
NAME                                READY   STATUS    RESTARTS   AGE
vpsie-autoscaler-5d4f8b9c7d-abc123   1/1     Running   0          5d
```

**Problem Indicators:**
- `Status: CrashLoopBackOff` → Pod is crash looping
- `Status: OOMKilled` → Pod ran out of memory
- `Status: Error` → Pod failed to start
- `RESTARTS: > 10` → Repeated crashes

### Step 3: Analyze Logs

```bash
# Get recent logs (last 100 lines)
kubectl logs -n kube-system -l app=vpsie-autoscaler --tail=100

# Get logs from previous instance (if restarted)
kubectl logs -n kube-system -l app=vpsie-autoscaler --previous --tail=200

# Follow logs in real-time
kubectl logs -n kube-system -l app=vpsie-autoscaler -f
```

**Look for:**
- Panic stack traces
- "out of memory" errors
- API connection failures
- Deadlock indicators
- Continuous error loops

### Step 4: Check Resource Usage

```bash
# Check current resource usage
kubectl top pod -n kube-system -l app=vpsie-autoscaler

# Check resource limits
kubectl get pod -n kube-system -l app=vpsie-autoscaler -o json | \
  jq '.items[].spec.containers[] | {name: .name, resources: .resources}'
```

**Red Flags:**
- Memory usage near limit → OOMKill imminent
- CPU throttling → Slow reconciliation
- Disk pressure → Log volume issues

### Step 5: Verify Kubernetes API Connectivity

```bash
# Test from controller pod (if running)
kubectl exec -n kube-system -l app=vpsie-autoscaler -- \
  curl -k https://kubernetes.default.svc/api/v1/namespaces

# Check service account permissions
kubectl auth can-i --list --as=system:serviceaccount:kube-system:vpsie-autoscaler
```

### Step 6: Check Leader Election (if enabled)

```bash
# Check leader election ConfigMap or Lease
kubectl get lease -n kube-system vpsie-autoscaler-leader-election -o yaml

# Look for holder identity and transition times
kubectl get lease -n kube-system vpsie-autoscaler-leader-election -o json | \
  jq '{holderIdentity: .spec.holderIdentity, renewTime: .spec.renewTime}'
```

---

## Resolution Steps

### Quick Fix: Restart Controller

```bash
# Restart the controller deployment
kubectl rollout restart deployment/vpsie-autoscaler -n kube-system

# Watch the restart process
kubectl rollout status deployment/vpsie-autoscaler -n kube-system

# Verify new pod is running
kubectl get pods -n kube-system -l app=vpsie-autoscaler
```

**When to use:** When logs show a transient issue or deadlock

**Impact:** Brief interruption (<30s) in scaling operations during restart

### Permanent Fix Based on Root Cause

#### Option 1: OOMKill (Memory Exhaustion)

**Symptoms:**
- Pod status shows `OOMKilled`
- Logs show memory allocation failures
- kubectl top shows memory usage near limit

**Resolution:**
```bash
# Increase memory limits
kubectl edit deployment vpsie-autoscaler -n kube-system

# Update resources section:
#   resources:
#     requests:
#       memory: 512Mi      # Increase from 256Mi
#     limits:
#       memory: 1Gi        # Increase from 512Mi

# Apply and wait for rollout
kubectl rollout status deployment/vpsie-autoscaler -n kube-system
```

**Verification:**
```bash
# Monitor new pod memory usage
watch kubectl top pod -n kube-system -l app=vpsie-autoscaler
```

#### Option 2: API Connection Failures

**Symptoms:**
- Logs show "connection refused" or "timeout" errors
- Cannot reach Kubernetes API from pod
- Network policy issues

**Resolution:**
```bash
# Check network policies
kubectl get networkpolicy -n kube-system

# Verify service account token
kubectl get secret -n kube-system | grep vpsie-autoscaler

# Test API connectivity from pod
kubectl run -it --rm debug --image=curlimages/curl --restart=Never -- \
  curl -k https://kubernetes.default.svc/api/v1/namespaces

# If network policy is blocking, update it
kubectl edit networkpolicy -n kube-system vpsie-autoscaler-network-policy
```

#### Option 3: VPSie API Credentials Invalid

**Symptoms:**
- Logs show "unauthorized" or "403 Forbidden"
- VPSie API errors in every reconciliation
- Secret may have been updated incorrectly

**Resolution:**
```bash
# Verify secret exists and has correct keys
kubectl get secret -n kube-system vpsie-secret -o jsonpath='{.data}' | jq

# Check secret values (base64 decode)
kubectl get secret -n kube-system vpsie-secret -o jsonpath='{.data.clientId}' | base64 -d
kubectl get secret -n kube-system vpsie-secret -o jsonpath='{.data.clientSecret}' | base64 -d

# Update secret if credentials changed
kubectl create secret generic vpsie-secret \
  --from-literal=clientId='new-client-id' \
  --from-literal=clientSecret='new-client-secret' \
  --dry-run=client -o yaml | kubectl apply -n kube-system -f -

# Restart controller to pick up new credentials
kubectl rollout restart deployment/vpsie-autoscaler -n kube-system
```

#### Option 4: Deadlock or Infinite Loop

**Symptoms:**
- Pod is running but no reconciliation activity
- CPU usage is 100% or 0%
- Logs show repeated attempts at same operation
- No progress for extended period

**Resolution:**
```bash
# Get stack traces (if supported by controller)
kubectl exec -n kube-system -l app=vpsie-autoscaler -- \
  curl localhost:8080/debug/pprof/goroutine?debug=2

# Force restart
kubectl delete pod -n kube-system -l app=vpsie-autoscaler

# Watch for new pod to start
kubectl get pods -n kube-system -l app=vpsie-autoscaler -w
```

**Post-resolution:**
- Review code for deadlock patterns
- Add timeout safeguards
- Improve error handling

---

## Verification

### Confirm Resolution

```bash
# 1. Check pod is running and healthy
kubectl get pods -n kube-system -l app=vpsie-autoscaler
# Expected: STATUS=Running, READY=1/1, RESTARTS=0 (or low number)

# 2. Verify reconciliation is happening
kubectl logs -n kube-system -l app=vpsie-autoscaler --tail=50 | grep "Reconciling"
# Expected: Regular reconciliation log entries

# 3. Check Prometheus metric
curl -s 'http://prometheus:9090/api/v1/query?query=rate(controller_reconcile_total{controller="nodegroup"}[5m])' | \
  jq '.data.result[].value[1]'
# Expected: Non-zero value (e.g., "0.05" = 3 reconciliations/minute)

# 4. Verify alert has cleared
curl -s 'http://prometheus:9090/api/v1/alerts' | \
  jq '.data.alerts[] | select(.labels.alertname=="ControllerDown")'
# Expected: Empty or state="inactive"
```

### Success Criteria

- [ ] Controller pod is `Running` with `READY=1/1`
- [ ] Reconciliation logs appearing every 30-60 seconds
- [ ] `controller_reconcile_total` metric increasing
- [ ] No errors in last 10 minutes of logs
- [ ] ControllerDown alert cleared in Prometheus
- [ ] Scaling operations functioning (test with a scale-up/down)

---

## Prevention

### Short-term Mitigations

1. **Increase Resource Limits**
   - Action: Raise memory/CPU requests and limits by 50%
   - Timeline: Immediate (if OOMKill was root cause)
   - Command:
     ```bash
     kubectl patch deployment vpsie-autoscaler -n kube-system --type='json' \
       -p='[{"op": "replace", "path": "/spec/template/spec/containers/0/resources/limits/memory", "value":"1Gi"}]'
     ```

2. **Add Liveness/Readiness Probes**
   - Action: Configure probes to auto-restart unresponsive controller
   - Timeline: Next deployment
   - Example:
     ```yaml
     livenessProbe:
       httpGet:
         path: /healthz
         port: 8080
       initialDelaySeconds: 30
       periodSeconds: 10
       failureThreshold: 3
     readinessProbe:
       httpGet:
         path: /readyz
         port: 8080
       initialDelaySeconds: 10
       periodSeconds: 5
     ```

3. **Enable Debug Logging Temporarily**
   - Action: Set log level to debug to catch issues early
   - Timeline: Until root cause fully understood
   - Command:
     ```bash
     kubectl set env deployment/vpsie-autoscaler -n kube-system LOG_LEVEL=debug
     ```

### Long-term Solutions

1. **Implement Circuit Breakers**
   - Description: Add circuit breakers for VPSie API calls to prevent cascading failures
   - Effort: 2-3 days
   - Priority: High

2. **Add Reconciliation Timeout**
   - Description: Enforce max reconciliation duration to prevent deadlocks
   - Effort: 1 day
   - Priority: High

3. **Improve Error Handling**
   - Description: Add exponential backoff and better error categorization
   - Effort: 3-5 days
   - Priority: Medium

4. **Add Profiling Endpoint**
   - Description: Expose pprof endpoint for runtime debugging
   - Effort: 1 day
   - Priority: Medium

### Monitoring Improvements

- [ ] Add alert for high restart count (>5 in 1 hour)
- [ ] Add alert for long reconciliation duration (P95 > 30s)
- [ ] Create dashboard for controller health metrics
- [ ] Monitor memory growth over time (potential leak detection)

---

## Escalation

### When to Escalate

Escalate if:
- [ ] Controller fails to start after 3 restart attempts
- [ ] Root cause is a code bug requiring immediate patch
- [ ] Issue impacts production customer workloads
- [ ] Multiple components are down simultaneously
- [ ] Restart cycle is accelerating (decreasing time between crashes)

### Escalation Contacts

- **Primary:** Platform Team Slack (#platform-oncall)
- **Secondary:** Senior Platform Engineer (@platform-lead)
- **Emergency:** Engineering Manager (only for critical customer impact)
- **Vendor Escalation:** VPSie Support (if API issues)

### Information to Provide

```markdown
**Incident Summary:**
- Alert: ControllerDown
- Duration: [X] minutes
- Impact: [Number] unschedulable pods, [Number] NodeGroups affected

**Investigation Timeline:**
- [Time]: Alert fired
- [Time]: Started investigation
- [Time]: Identified [symptom]
- [Time]: Attempted [fix]

**Current State:**
- Pod status: [Running | CrashLoopBackOff | etc.]
- Last 10 log lines: [paste]
- Resource usage: [paste kubectl top output]
- Recent changes: [any recent deployments or config changes]

**Grafana Link:**
https://grafana.example.com/d/vpsie-autoscaler?from=[alert_time]&to=now

**Prometheus Query Link:**
http://prometheus:9090/graph?g0.expr=controller_reconcile_total&g0.range_input=1h
```

---

## Related Documentation

- [Alert Definition](../../deploy/prometheus/alerts.yaml#L489-L504)
- [Controller Architecture](../architecture.md#controller)
- [Metrics Guide](../metrics.md#controller-metrics)
- [Grafana Dashboard](https://grafana.example.com/d/vpsie-autoscaler)
- [Deployment Guide](../deployment.md)

## Related Runbooks

- [Runbook: HighVPSieAPIErrorRate](./high-api-error-rate.md)
- [Runbook: NodeProvisioningFailed](./provisioning-failed.md)
- [Runbook: HighControllerReconcileDuration](./slow-reconciliation.md)

---

## Postmortem Example

```markdown
# Postmortem: ControllerDown - 2025-01-15

## Incident Timeline
- 14:23: ControllerDown alert fired
- 14:25: On-call engineer notified via PagerDuty
- 14:28: Investigation started
- 14:32: Root cause identified (OOMKill due to memory leak)
- 14:35: Memory limits increased from 512Mi to 1Gi
- 14:37: Controller restarted with new limits
- 14:40: Alert cleared, reconciliation resumed

## Root Cause
Memory leak in NodeGroup reconciliation loop caused controller to exhaust
memory and get OOMKilled repeatedly. Leak was introduced in commit abc123
where VPSieNode objects were cached but never evicted.

## Resolution
1. Immediate: Increased memory limits to 1Gi (provides 2 weeks of buffer)
2. Permanent: Fixed memory leak in PR #456, deployed in v0.5.1

## Action Items
- [x] Fix memory leak (PR #456) - Owner: @dev-team - Completed: 2025-01-16
- [ ] Add memory usage alerts (>80% of limit) - Owner: @platform - Due: 2025-01-20
- [ ] Implement cache eviction policy - Owner: @dev-team - Due: 2025-01-25
- [ ] Add memory profiling to CI/CD - Owner: @devops - Due: 2025-02-01

## Lessons Learned
- Resource limits should be monitored as closely as usage
- Caching without eviction is a recipe for memory leaks
- Need better testing for long-running controller scenarios
```

---

## Revision History

| Date | Author | Changes |
|------|--------|---------|
| 2025-12-22 | Platform Team | Initial runbook creation |
