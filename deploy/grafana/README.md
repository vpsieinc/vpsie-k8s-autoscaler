# VPSie Kubernetes Autoscaler - Grafana Dashboard

This directory contains the pre-built Grafana dashboard for monitoring the VPSie Kubernetes Autoscaler.

## Dashboard Overview

The dashboard provides comprehensive observability across 12 panels:

### Row 1: NodeGroup Overview
- **Current Nodes** - Real-time node count
- **Desired Nodes** - Target node count
- **Ready Nodes** - Healthy node count
- **Min/Max Limits** - Scaling boundaries

### Row 2: Scaling Activity
- **Scale Operations Over Time** - Visualizes scale-up and scale-down events

### Row 3: Node Provisioning Performance
- **Provisioning Duration Heatmap** - Distribution of provisioning times
- **Provisioning Percentiles** - P50/P95/P99 latencies

### Row 4: API & Controller Health
- **VPSie API Metrics** - Request rate, error rate, and latency
- **Controller Performance** - Reconciliation duration and error rate

### Row 5: VPSieNode Lifecycle
- **Phase Distribution** - Stacked area chart showing node phases over time

### Row 6: Safety & Blocking
- **Scale-Down Blocked by Reason** - Table showing what's preventing scale-down
- **Safety Check Failures** - Breakdown of failed safety checks

## Prerequisites

1. **Grafana** - Version 9.0 or later
2. **Prometheus Datasource** - Configured and scraping autoscaler metrics
3. **Metrics Endpoint** - Autoscaler must be exposing metrics on `/metrics`

## Installation

### Method 1: Import via Grafana UI

1. Open Grafana and navigate to **Dashboards** → **Import**
2. Click **Upload JSON file**
3. Select `autoscaler-dashboard.json` from this directory
4. Select your Prometheus datasource when prompted
5. Click **Import**

### Method 2: Import via API

```bash
# Set your Grafana URL and API key
GRAFANA_URL="http://grafana.example.com"
GRAFANA_API_KEY="your-api-key"

# Import the dashboard
curl -X POST \
  -H "Authorization: Bearer ${GRAFANA_API_KEY}" \
  -H "Content-Type: application/json" \
  -d @autoscaler-dashboard.json \
  "${GRAFANA_URL}/api/dashboards/db"
```

### Method 3: Kubernetes ConfigMap (GitOps)

```bash
# Create ConfigMap from dashboard JSON
kubectl create configmap vpsie-autoscaler-dashboard \
  --from-file=autoscaler-dashboard.json \
  -n monitoring

# Label for Grafana sidecar discovery (if using grafana-operator or similar)
kubectl label configmap vpsie-autoscaler-dashboard \
  grafana_dashboard=1 \
  -n monitoring
```

If using the Grafana Operator, create a `GrafanaDashboard` resource:

```yaml
apiVersion: grafana.integreatly.org/v1beta1
kind: GrafanaDashboard
metadata:
  name: vpsie-autoscaler
  namespace: monitoring
spec:
  instanceSelector:
    matchLabels:
      dashboards: "grafana"
  json: |
    <paste contents of autoscaler-dashboard.json>
```

## Configuration

### Dashboard Variables

The dashboard includes two template variables for filtering:

- **namespace** - Filter by Kubernetes namespace
  - Type: Query variable
  - Query: `label_values(nodegroup_desired_nodes, namespace)`
  - Multi-select: No
  - Include All: Yes

- **nodegroup** - Filter by NodeGroup name
  - Type: Query variable
  - Query: `label_values(nodegroup_desired_nodes{namespace="$namespace"}, nodegroup)`
  - Multi-select: Yes
  - Include All: Yes

### Annotations

**Scale Events** - Automatically annotates the dashboard when scale operations occur:
- Expression: `changes(nodegroup_desired_nodes{namespace="$namespace", nodegroup="$nodegroup"}[$__interval])`
- Shows: When desired nodes count changes

### Refresh Rate

- **Default**: 30 seconds
- **Configurable**: Use the refresh dropdown in the top-right corner

## Panel Details

### 1. Current/Desired/Ready Nodes (Gauges)
- **Metrics**:
  - `nodegroup_current_nodes`
  - `nodegroup_desired_nodes`
  - `nodegroup_ready_nodes`
- **Use Case**: At-a-glance cluster health

### 2. Scaling Activity (Time Series)
- **Metrics**:
  - `rate(scale_up_total[$__rate_interval])`
  - `rate(scale_down_total[$__rate_interval])`
- **Use Case**: Understand scaling patterns and frequency

### 3. Provisioning Duration Heatmap
- **Metric**: `node_provisioning_duration_seconds_bucket`
- **Use Case**: Identify slow provisioning periods

### 4. Provisioning Percentiles
- **Metrics**: P50, P95, P99 of `node_provisioning_duration_seconds`
- **Use Case**: SLO tracking for provisioning speed

### 5. VPSie API Metrics
- **Metrics**:
  - Request rate: `rate(vpsie_api_requests_total[$__rate_interval])`
  - Error rate: `rate(vpsie_api_errors_total) / rate(vpsie_api_requests_total)`
  - Latency P95: `histogram_quantile(0.95, vpsie_api_request_duration_seconds_bucket)`
- **Use Case**: Monitor VPSie API health and quota usage

### 6. Controller Performance
- **Metrics**:
  - Reconcile duration: P50, P95 of `controller_reconcile_duration_seconds`
  - Error rate: `rate(controller_reconcile_errors_total)`
- **Use Case**: Detect controller slowdowns or errors

### 7. VPSieNode Phase Distribution
- **Metric**: `vpsienode_phase` (grouped by phase)
- **Use Case**: Track node lifecycle states

### 8. Scale-Down Blocked by Reason (Table)
- **Metric**: `scale_down_blocked_total` (grouped by reason)
- **Use Case**: Understand why nodes aren't scaling down

### 9. Safety Check Failures (Table)
- **Metric**: `safety_check_failures_total` (grouped by check_type)
- **Use Case**: Identify recurring safety violations

## Troubleshooting

### Dashboard shows "No data"

1. **Check Prometheus scraping**:
   ```bash
   kubectl get servicemonitor -n kube-system vpsie-autoscaler -o yaml
   ```

2. **Verify metrics endpoint**:
   ```bash
   kubectl port-forward -n kube-system svc/vpsie-autoscaler-metrics 8080:8080
   curl http://localhost:8080/metrics | grep nodegroup_
   ```

3. **Check datasource in Grafana**:
   - Go to **Configuration** → **Data Sources**
   - Test the Prometheus connection
   - Verify the URL is correct

### Variables not populating

1. **Check metric label cardinality**:
   ```promql
   count(count by (namespace) (nodegroup_desired_nodes))
   ```

2. **Verify namespace/nodegroup labels exist**:
   ```promql
   nodegroup_desired_nodes{namespace="default"}
   ```

### Panels showing errors

1. **Check metric names** - Ensure autoscaler version matches dashboard expectations
2. **Review Prometheus queries** - Click panel → Edit → check query syntax
3. **Validate time ranges** - Some queries require sufficient historical data

## Customization

### Adding Custom Panels

1. Click **Add panel** → **Add a new panel**
2. Configure your visualization
3. Add queries using available metrics (see [Metrics Guide](../../docs/metrics.md))
4. Save the dashboard

### Modifying Thresholds

Edit panel → Field → Thresholds:
- Green: Normal operation
- Yellow: Warning (e.g., >80% capacity)
- Red: Critical (e.g., at max nodes)

### Exporting Modified Dashboard

1. Click **Dashboard settings** (gear icon)
2. Go to **JSON Model**
3. Copy the JSON
4. Save to `autoscaler-dashboard.json`

## Available Metrics

For a complete list of available metrics, see:
- [Metrics Documentation](../../docs/metrics.md)
- [Prometheus Endpoint](http://autoscaler:8080/metrics)

## Related Documentation

- [Architecture Decision Record - Grafana Dashboard](../../docs/ADR_NICE_TO_HAVE_P1.md#31-grafana-dashboard-template)
- [Prometheus Alert Rules](../prometheus/README.md)
- [Operator Guide](../../docs/operator-guide.md)

## Support

For issues or questions:
- [GitHub Issues](https://github.com/vpsie/vpsie-k8s-autoscaler/issues)
- [Documentation](https://docs.vpsie.com/autoscaler)

## License

This dashboard is part of the VPSie Kubernetes Autoscaler project and shares the same license.
