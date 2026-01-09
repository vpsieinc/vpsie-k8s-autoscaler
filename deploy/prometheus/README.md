# VPSie Kubernetes Autoscaler - Prometheus Alert Rules

This directory contains production-ready Prometheus alert rules for monitoring the VPSie Kubernetes Autoscaler.

## Overview

The alert rules provide proactive monitoring across three severity levels:

- **CRITICAL** (4 alerts) - Require immediate attention, may impact service availability
- **WARNING** (8 alerts) - Should be investigated soon, may lead to issues if unaddressed

## Alert Summary

### Critical Alerts

| Alert | Threshold | Duration | Impact |
|-------|-----------|----------|--------|
| **HighVPSieAPIErrorRate** | >10% error rate | 5 minutes | Scaling operations failing |
| **ControllerDown** | No reconciliation | 10 minutes | No automatic scaling |
| **NodeProvisioningFailed** | >3 failures | 15 minutes | Cannot add capacity |
| **NodeGroupAtMaxCapacity** | At max with pending pods | 30 minutes | Service degradation |

### Warning Alerts

| Alert | Threshold | Duration | Impact |
|-------|-----------|----------|--------|
| **SlowNodeProvisioning** | P95 > 10 minutes | 15 minutes | Slow scaling response |
| **HighControllerReconcileDuration** | P99 > 30 seconds | 15 minutes | Delayed scaling decisions |
| **VPSieAPIRateLimited** | Any rate limiting | 5 minutes | Throttled operations |
| **FrequentScaleDownBlocking** | >10 blocks/30min | 30 minutes | Inefficient resource usage |
| **HighSafetyCheckFailureRate** | >0.1 failures/sec | 15 minutes | Configuration issues |
| **NodesStuckDraining** | Any nodes draining | 30 minutes | Resource stuck |
| **RebalancingFailed** | Any failures | Immediate | Cost optimization disabled |
| **UnschedulablePodsExtended** | Any pending pods | 15 minutes | Service capacity issues |

## Installation

### Prerequisites

1. **Prometheus Operator** - For managing PrometheusRule CRDs
2. **Alertmanager** - For routing and notifications (optional but recommended)
3. **Autoscaler Metrics** - Ensure autoscaler is exposing metrics

### Method 1: Using Prometheus Operator

```bash
# Apply the alert rules as a PrometheusRule CRD
kubectl apply -f - <<EOF
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: vpsie-autoscaler-alerts
  namespace: monitoring
  labels:
    prometheus: kube-prometheus
    role: alert-rules
spec:
  $(cat alerts.yaml | yq '.data."alerts.yaml"')
EOF
```

### Method 2: Using ConfigMap

```bash
# Create the ConfigMap
kubectl apply -f alerts.yaml

# Add to Prometheus configuration
kubectl edit configmap prometheus-config -n monitoring
```

Add to the Prometheus config:

```yaml
rule_files:
  - /etc/prometheus/rules/*.yaml

# Mount the ConfigMap
volumes:
  - name: alert-rules
    configMap:
      name: vpsie-autoscaler-alerts
volumeMounts:
  - name: alert-rules
    mountPath: /etc/prometheus/rules
```

### Method 3: Direct Prometheus Configuration

```yaml
# prometheus.yml
rule_files:
  - /etc/prometheus/rules/vpsie-autoscaler.yaml
```

Copy `alerts.yaml` to `/etc/prometheus/rules/` and reload Prometheus.

## Verification

### Check Alert Rules Loaded

```bash
# Via Prometheus UI
# Navigate to: http://prometheus:9090/rules

# Via promtool
promtool check rules alerts.yaml

# Via kubectl (if using Prometheus Operator)
kubectl get prometheusrules -n monitoring vpsie-autoscaler-alerts -o yaml
```

### Test Alert Queries

```bash
# Check if metrics are available
curl http://prometheus:9090/api/v1/query?query=nodegroup_current_nodes

# Simulate alert conditions (for testing)
kubectl scale deployment my-app --replicas=100  # Trigger capacity alert
```

## Configuration

### Alertmanager Integration

Configure Alertmanager to route VPSie autoscaler alerts:

```yaml
# alertmanager.yml
route:
  receiver: 'default'
  routes:
    - match:
        component: vpsie-autoscaler
      receiver: 'platform-team'
      continue: true
      routes:
        - match:
            severity: critical
          receiver: 'pagerduty-critical'
        - match:
            severity: warning
          receiver: 'slack-warnings'

receivers:
  - name: 'pagerduty-critical'
    pagerduty_configs:
      - service_key: '<your-pagerduty-key>'
        description: '{{ .GroupLabels.alertname }}: {{ .Annotations.summary }}'

  - name: 'slack-warnings'
    slack_configs:
      - api_url: '<your-slack-webhook>'
        channel: '#autoscaler-alerts'
        title: '{{ .GroupLabels.alertname }}'
        text: '{{ .Annotations.description }}'
```

### Customizing Thresholds

Edit the alert expressions in `alerts.yaml`:

```yaml
# Example: Increase API error rate threshold from 10% to 20%
- alert: HighVPSieAPIErrorRate
  expr: |
    (
      sum(rate(vpsie_api_errors_total[5m])) by (method)
      /
      sum(rate(vpsie_api_requests_total[5m])) by (method)
    ) > 0.20  # Changed from 0.10
```

### Adding Custom Labels

Add labels to alerts for custom routing:

```yaml
labels:
  severity: critical
  component: vpsie-api
  team: platform
  environment: production  # Add custom label
  slack_channel: autoscaler  # Add routing label
```

## Alert Details

### CRITICAL-1: HighVPSieAPIErrorRate

**Trigger:** VPSie API error rate >10% for 5 minutes

**Common Causes:**
- VPSie API outage or maintenance
- Invalid or expired API credentials
- Rate limiting or quota exhaustion
- Network connectivity issues

**Runbook:** [high-api-error-rate.md](../../docs/runbooks/high-api-error-rate.md)

**Actions:**
1. Check VPSie API status page
2. Verify API credentials in `vpsie-secret`
3. Review autoscaler logs: `kubectl logs -n kube-system -l app=vpsie-autoscaler`
4. Test API connectivity manually

### CRITICAL-2: ControllerDown

**Trigger:** No reconciliation activity for 10 minutes

**Common Causes:**
- Controller pod crashed or OOMKilled
- Deadlock in reconciliation loop
- Unable to connect to Kubernetes API
- Leader election issues (if enabled)

**Runbook:** [controller-down.md](../../docs/runbooks/controller-down.md)

**Actions:**
1. Check pod status: `kubectl get pods -n kube-system -l app=vpsie-autoscaler`
2. Review pod logs: `kubectl logs -n kube-system -l app=vpsie-autoscaler --tail=200`
3. Check resource usage: `kubectl top pod -n kube-system -l app=vpsie-autoscaler`
4. Restart if necessary: `kubectl rollout restart deployment/vpsie-autoscaler -n kube-system`

### CRITICAL-3: NodeProvisioningFailed

**Trigger:** >3 node provisioning failures in 15 minutes

**Common Causes:**
- VPSie datacenter capacity exhausted
- Invalid offering or datacenter ID in NodeGroup
- Insufficient VPSie account balance
- Cloud-init script errors

**Runbook:** [provisioning-failed.md](../../docs/runbooks/provisioning-failed.md)

**Actions:**
1. Check failed VPSieNodes: `kubectl get vpsienodes -A | grep Failed`
2. Review failure reasons: `kubectl describe vpsienode <name>`
3. Verify VPSie account balance and quotas
4. Check offering availability in target datacenter

### CRITICAL-4: NodeGroupAtMaxCapacity

**Trigger:** At max_nodes with pending pods for 30 minutes

**Common Causes:**
- Legitimate capacity limits reached
- Pod resource requests too large for any node
- Affinity/anti-affinity preventing scheduling
- max_nodes set too low

**Runbook:** [max-capacity.md](../../docs/runbooks/max-capacity.md)

**Actions:**
1. Review pending pods: `kubectl get pods -A | grep Pending`
2. Check pod events: `kubectl describe pod <name>`
3. Consider increasing max_nodes if justified
4. Review pod resource requests and scheduling constraints

### WARNING-1: SlowNodeProvisioning

**Trigger:** P95 provisioning time >10 minutes for 15 minutes

**Common Causes:**
- VPSie API slow response times
- Cloud-init script taking too long
- Node registration delays
- Network latency

**Runbook:** [slow-provisioning.md](../../docs/runbooks/slow-provisioning.md)

**Actions:**
1. Check VPSie API latency metrics
2. Review cloud-init script complexity
3. Monitor node registration events
4. Consider simpler provisioning workflows

### WARNING-2: HighControllerReconcileDuration

**Trigger:** P99 reconciliation time >30 seconds for 15 minutes

**Common Causes:**
- High VPSie API latency
- Large number of NodeGroups to reconcile
- Kubernetes API slowness
- Controller resource contention

**Runbook:** [slow-reconciliation.md](../../docs/runbooks/slow-reconciliation.md)

**Actions:**
1. Check controller resource limits
2. Review number of NodeGroups
3. Monitor VPSie API latency
4. Consider increasing controller resources

### WARNING-3: VPSieAPIRateLimited

**Trigger:** Any rate limiting detected for 5 minutes

**Common Causes:**
- Too many API calls in short period
- Rate limit configuration too aggressive
- VPSie account limits
- Multiple autoscalers sharing credentials

**Runbook:** [rate-limited.md](../../docs/runbooks/rate-limited.md)

**Actions:**
1. Review rate limit configuration
2. Check for multiple autoscaler instances
3. Contact VPSie support for quota increase
4. Consider batching operations

### WARNING-4: FrequentScaleDownBlocking

**Trigger:** >10 scale-down blocks in 30 minutes (excluding cooldown)

**Common Causes:**
- Pods using local storage (EmptyDir, HostPath)
- Restrictive PodDisruptionBudgets
- Nodes with protection annotations
- Anti-affinity rules preventing rescheduling

**Runbook:** [scale-down-blocked.md](../../docs/runbooks/scale-down-blocked.md)

**Actions:**
1. Review blocking reasons in Grafana dashboard
2. Check pod volume usage
3. Audit PodDisruptionBudgets
4. Review node protection annotations

### WARNING-5: HighSafetyCheckFailureRate

**Trigger:** >0.1 safety check failures/sec for 15 minutes

**Common Causes:**
- Workloads not designed for scale-down
- Cluster capacity constraints
- Configuration issues
- System pods without replicas

**Runbook:** [safety-check-failures.md](../../docs/runbooks/safety-check-failures.md)

**Actions:**
1. Check safety check types failing
2. Review workload configurations
3. Ensure critical pods have multiple replicas
4. Verify cluster has sufficient spare capacity

### WARNING-6: NodesStuckDraining

**Trigger:** Nodes in Draining phase for >30 minutes

**Common Causes:**
- PodDisruptionBudgets blocking eviction
- Pods with long termination grace periods
- Pods stuck in Terminating state
- Finalizers preventing deletion

**Runbook:** [stuck-draining.md](../../docs/runbooks/stuck-draining.md)

**Actions:**
1. List draining nodes: `kubectl get vpsienodes -A | grep Draining`
2. Check pods on draining nodes
3. Review PDBs: `kubectl get pdb -A`
4. Force delete stuck pods if necessary

### WARNING-7: RebalancingFailed

**Trigger:** Any rebalancing failures in 1 hour

**Common Causes:**
- Target datacenter capacity exhausted
- Invalid target offering configuration
- Pod scheduling constraints
- Insufficient permissions

**Runbook:** [rebalancing-failed.md](../../docs/runbooks/rebalancing-failed.md)

**Actions:**
1. Review rebalancer logs
2. Check rebalancing plan execution
3. Verify target offering availability
4. Review pod affinity rules

### WARNING-8: UnschedulablePodsExtended

**Trigger:** Pods pending due to constraints for >15 minutes

**Common Causes:**
- Insufficient CPU/memory capacity
- NodeGroup at max_nodes
- Resource requests too large
- Custom resource (GPU) not available

**Runbook:** [unschedulable-pods.md](../../docs/runbooks/unschedulable-pods.md)

**Actions:**
1. Check pending pod constraints
2. Verify NodeGroup can scale up
3. Review pod resource requests
4. Check node offerings meet requirements

## Silencing Alerts

### Temporary Silence (Alertmanager)

```bash
# Silence all autoscaler alerts for 2 hours
amtool silence add component=vpsie-autoscaler --duration=2h --comment="Maintenance window"

# Silence specific alert
amtool silence add alertname=HighVPSieAPIErrorRate --duration=1h --comment="Known VPSie API issue"
```

### Permanent Disable (Prometheus)

Comment out the alert in `alerts.yaml` and reload Prometheus.

## Testing Alerts

### Manual Alert Simulation

```bash
# Trigger HighVPSieAPIErrorRate (requires test environment)
# Stop VPSie API mock server to cause errors

# Trigger SlowNodeProvisioning
# Add artificial delay to cloud-init script

# Trigger NodeGroupAtMaxCapacity
kubectl scale deployment test-app --replicas=1000
# Set NodeGroup max_nodes to current count
```

### Alert Testing Tool

```bash
# Use amtool to test alert routing
amtool alert add alertname=HighVPSieAPIErrorRate severity=critical component=vpsie-api
```

## Maintenance

### Regular Review

- **Weekly**: Review alert frequency and adjust thresholds
- **Monthly**: Update runbook URLs and action steps
- **Quarterly**: Audit alert coverage for new features

### Metrics to Monitor

- Alert firing frequency
- Mean time to acknowledge (MTTA)
- Mean time to resolve (MTTR)
- False positive rate

## Related Documentation

- [Grafana Dashboard](../grafana/README.md)
- [Metrics Guide](../../docs/metrics.md)
- [Runbook Templates](../../docs/runbooks/)
- [Operator Guide](../../docs/operator-guide.md)
- [ADR - Prometheus Alerts](../../docs/ADR_NICE_TO_HAVE_P1.md#32-prometheus-alert-rules)

## Support

For issues with alerts:
- [GitHub Issues](https://github.com/vpsie/vpsie-k8s-autoscaler/issues)
- [Slack Channel](#autoscaler-alerts)
- [Documentation](https://docs.vpsie.com/autoscaler)

## License

This alert configuration is part of the VPSie Kubernetes Autoscaler project and shares the same license.
