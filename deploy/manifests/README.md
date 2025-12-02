# VPSie Kubernetes Autoscaler - Deployment Manifests

This directory contains Kubernetes deployment manifests for the VPSie Kubernetes Autoscaler controller.

## Files

- **`rbac.yaml`** - ServiceAccount, ClusterRole, and ClusterRoleBinding for the controller
- **`deployment.yaml`** - Deployment manifest with resource limits and security context
- **`service.yaml`** - Service for metrics and health endpoints
- **`kustomization.yaml`** - Kustomize configuration for easy deployment

## Prerequisites

1. **VPSie API Credentials**: Create a Kubernetes secret with your VPSie API credentials:

```bash
kubectl create secret generic vpsie-secret \
  --from-literal=clientId='your-vpsie-client-id' \
  --from-literal=clientSecret='your-vpsie-client-secret' \
  --from-literal=url='https://api.vpsie.com/v2' \
  -n kube-system
```

2. **Custom Resource Definitions (CRDs)**: Install the CRDs before deploying the controller:

```bash
kubectl apply -f ../crds/
```

## Deployment Options

### Option 1: Using kubectl

Deploy all manifests directly:

```bash
kubectl apply -f rbac.yaml
kubectl apply -f service.yaml
kubectl apply -f deployment.yaml
```

### Option 2: Using Kustomize

Deploy using kustomize (recommended):

```bash
kubectl apply -k .
```

Or with a specific image tag:

```bash
kustomize edit set image vpsie/k8s-autoscaler:v1.0.0
kubectl apply -k .
```

## Resource Limits

The deployment includes production-ready resource requests and limits:

### Requests (Guaranteed Resources)
- **CPU**: 100m (0.1 cores) - Minimum guaranteed CPU
- **Memory**: 128Mi - Minimum guaranteed memory

### Limits (Maximum Resources)
- **CPU**: 500m (0.5 cores) - Maximum CPU usage
- **Memory**: 512Mi - Maximum memory usage

These values are appropriate for:
- Small to medium clusters (< 100 nodes)
- Moderate autoscaling activity
- Standard reconciliation loops

### Adjusting Resources

For larger clusters or higher activity, increase limits in `deployment.yaml`:

```yaml
resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: 1000m  # 1 CPU core
    memory: 1Gi
```

## Health and Metrics

The deployment exposes two endpoints:

### Health Endpoints
- **Liveness**: `http://:8081/healthz` - Container health
- **Readiness**: `http://:8081/readyz` - Ready to serve traffic
- **Ping**: `http://:8081/ping` - Basic connectivity check

### Metrics Endpoint
- **Prometheus metrics**: `http://:8080/metrics`
- Automatically scraped if Prometheus is configured (see annotations)

Example Prometheus ServiceMonitor:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: vpsie-autoscaler
  namespace: kube-system
spec:
  selector:
    matchLabels:
      app: vpsie-autoscaler
  endpoints:
  - port: metrics
    interval: 30s
```

## Security

The deployment includes production security best practices:

- **Non-root user**: Runs as UID 65532
- **Read-only root filesystem**: Container filesystem is read-only
- **No privilege escalation**: `allowPrivilegeEscalation: false`
- **Dropped capabilities**: All Linux capabilities dropped
- **Security context**: Minimal permissions

## Leader Election

The controller supports leader election for high availability:

- Enabled by default with `--leader-elect=true`
- Uses Kubernetes Lease objects for coordination
- Safe to run multiple replicas (only one active at a time)

To enable HA, increase replicas in `deployment.yaml`:

```yaml
spec:
  replicas: 3  # Run 3 instances for HA
```

## Troubleshooting

### Check Pod Status

```bash
kubectl get pods -n kube-system -l app=vpsie-autoscaler
```

### View Logs

```bash
kubectl logs -n kube-system -l app=vpsie-autoscaler --tail=100 -f
```

### Check Metrics

```bash
kubectl port-forward -n kube-system svc/vpsie-autoscaler-metrics 8080:8080
curl http://localhost:8080/metrics
```

### Common Issues

1. **Secret not found**:
   ```
   Error: secret "vpsie-secret" not found
   ```
   Solution: Create the VPSie credentials secret (see Prerequisites)

2. **CRDs not installed**:
   ```
   Error: no matches for kind "NodeGroup"
   ```
   Solution: Install CRDs first: `kubectl apply -f ../crds/`

3. **Insufficient permissions**:
   ```
   Error: forbidden: User cannot create resource
   ```
   Solution: Ensure RBAC is applied: `kubectl apply -f rbac.yaml`

## Monitoring

Key metrics to monitor:

- `vpsie_autoscaler_nodegroup_current_nodes` - Current nodes per NodeGroup
- `vpsie_autoscaler_nodegroup_desired_nodes` - Desired nodes per NodeGroup
- `vpsie_autoscaler_vpsie_api_rate_limited_total` - API rate limiting events
- `vpsie_autoscaler_scale_down_errors_total` - Scale-down failures
- `vpsie_autoscaler_controller_reconcile_duration_seconds` - Reconciliation performance

## Uninstalling

To remove the autoscaler:

```bash
kubectl delete -k .
# or
kubectl delete -f deployment.yaml -f service.yaml -f rbac.yaml
```

To also remove CRDs (this will delete all NodeGroups and VPSieNodes):

```bash
kubectl delete -f ../crds/
```
