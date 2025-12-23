# Runbook Template: [Alert Name]

**Last Updated:** YYYY-MM-DD
**Alert Severity:** [Critical | Warning]
**Component:** [vpsie-api | controller | provisioner | scaler | rebalancer]
**Team:** platform

---

## Alert Overview

**Alert Name:** `[AlertName]`

**Summary:** [One-line description of what this alert indicates]

**Prometheus Expression:**
```promql
[Paste the alert expression from alerts.yaml]
```

**Threshold:** [Describe the threshold that triggers this alert]

**Duration:** [How long the condition must persist before firing]

---

## Impact Assessment

### Severity: [Critical | Warning]

**Immediate Impact:**
- [What is currently failing or degraded?]
- [What features/functionality are affected?]
- [Are users experiencing issues?]

**Potential Escalation:**
- [What will happen if not resolved quickly?]
- [What cascading failures might occur?]
- [When should this be escalated?]

**Affected Components:**
- [List Kubernetes components affected]
- [List external dependencies affected]
- [List user-facing services affected]

---

## Prerequisites

Before starting troubleshooting, ensure you have:

- [ ] kubectl access to the cluster
- [ ] Access to Grafana dashboards
- [ ] Access to Prometheus queries
- [ ] VPSie console access (if needed)
- [ ] Autoscaler logs access

**Required Tools:**
```bash
kubectl
promtool  # Optional
curl      # Optional
jq        # Optional
```

---

## Diagnostic Steps

### Step 1: Verify Alert Status

```bash
# Check if alert is still firing
curl -s 'http://prometheus:9090/api/v1/alerts' | jq '.data.alerts[] | select(.labels.alertname=="[AlertName]")'

# View in Prometheus UI
open http://prometheus:9090/alerts
```

### Step 2: Check [Component] Health

```bash
# [Component-specific health check command]
kubectl get pods -n kube-system -l app=vpsie-autoscaler

# Check logs
kubectl logs -n kube-system -l app=vpsie-autoscaler --tail=100
```

### Step 3: Analyze Metrics

```bash
# Query relevant metrics
curl -s 'http://prometheus:9090/api/v1/query?query=[metric_name]' | jq

# Check in Grafana
open https://grafana.example.com/d/vpsie-autoscaler
```

### Step 4: Identify Root Cause

**Common Causes:**

1. **[Cause 1]**
   - Symptoms: [What you'll see]
   - Verification: [How to confirm]
   - Fix: [What to do]

2. **[Cause 2]**
   - Symptoms: [What you'll see]
   - Verification: [How to confirm]
   - Fix: [What to do]

3. **[Cause 3]**
   - Symptoms: [What you'll see]
   - Verification: [How to confirm]
   - Fix: [What to do]

---

## Resolution Steps

### Quick Fix (If Applicable)

```bash
# Emergency mitigation command
[command to quickly mitigate the issue]
```

**When to use:** [Describe emergency scenarios]

**Impact:** [What this quick fix affects]

### Permanent Fix

#### Option 1: [Fix Description]

**When to use:** [Scenario where this applies]

**Steps:**
```bash
# Step 1: [Description]
[command]

# Step 2: [Description]
[command]

# Step 3: [Description]
[command]
```

**Verification:**
```bash
# Verify the fix worked
[verification command]
```

#### Option 2: [Alternative Fix]

**When to use:** [Scenario where this applies]

**Steps:**
```bash
# [Commands]
```

---

## Verification

### Confirm Resolution

```bash
# 1. Check alert has cleared
[command to verify alert is no longer firing]

# 2. Verify metrics are healthy
[command to check relevant metrics]

# 3. Test functionality
[command to test the fixed component]
```

### Success Criteria

- [ ] Alert is no longer firing in Prometheus
- [ ] [Metric] has returned to normal range
- [ ] [Component] is functioning correctly
- [ ] No related errors in logs for 10+ minutes

---

## Prevention

### Short-term Mitigations

1. **[Mitigation 1]**
   - Action: [What to do]
   - Timeline: [When to implement]

2. **[Mitigation 2]**
   - Action: [What to do]
   - Timeline: [When to implement]

### Long-term Solutions

1. **[Solution 1]**
   - Description: [What needs to change]
   - Effort: [Time/complexity estimate]
   - Priority: [High | Medium | Low]

2. **[Solution 2]**
   - Description: [What needs to change]
   - Effort: [Time/complexity estimate]
   - Priority: [High | Medium | Low]

### Monitoring Improvements

- [ ] Add additional metrics for early detection
- [ ] Adjust alert thresholds if false positive
- [ ] Create dashboard for this failure mode
- [ ] Document edge cases discovered

---

## Escalation

### When to Escalate

Escalate to senior engineer or oncall if:
- [ ] Alert persists after 30 minutes of troubleshooting
- [ ] Root cause is unclear or unfamiliar
- [ ] Fix requires dangerous operations (data deletion, etc.)
- [ ] Impact is expanding to other services

### Escalation Contacts

- **Primary:** [Team Slack channel] (#platform-oncall)
- **Secondary:** [Senior Engineer] (@platform-lead)
- **Emergency:** [Manager] (only for critical customer impact)

### Information to Provide

When escalating, include:
- Alert firing time and duration
- Steps already attempted
- Current error messages
- Recent changes to the system
- Grafana dashboard link with relevant time range

---

## Related Documentation

- [Alert Definition](../../deploy/prometheus/alerts.yaml)
- [Metrics Guide](../metrics.md)
- [Architecture Decision Record](../ADR_NICE_TO_HAVE_P1.md)
- [Grafana Dashboard](https://grafana.example.com/d/vpsie-autoscaler)
- [Operator Guide](../operator-guide.md)

## Related Runbooks

- [Runbook: Related Alert 1](./related-alert-1.md)
- [Runbook: Related Alert 2](./related-alert-2.md)

---

## Postmortem Template

After resolving the incident, create a postmortem:

```markdown
# Postmortem: [Alert Name] - [Date]

## Incident Timeline
- [Time]: Alert fired
- [Time]: Investigation started
- [Time]: Root cause identified
- [Time]: Fix applied
- [Time]: Alert cleared

## Root Cause
[Detailed explanation of what caused the alert]

## Resolution
[What was done to fix it]

## Action Items
- [ ] [Preventive action 1] - Owner: [Name] - Due: [Date]
- [ ] [Preventive action 2] - Owner: [Name] - Due: [Date]
```

---

## Revision History

| Date | Author | Changes |
|------|--------|---------|
| YYYY-MM-DD | [Name] | Initial runbook creation |
| YYYY-MM-DD | [Name] | Added section on [topic] after incident |
