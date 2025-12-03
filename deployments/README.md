# VPSie Kubernetes Autoscaler - Kustomize Deployment

This directory contains Kustomize-based deployment manifests for the VPSie Kubernetes Autoscaler.

## Structure

```
deployments/
├── base/                  # Base manifests
│   ├── namespace.yaml
│   ├── serviceaccount.yaml
│   ├── clusterrole.yaml
│   ├── clusterrolebinding.yaml
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── poddisruptionbudget.yaml
│   ├── servicemonitor.yaml
│   └── kustomization.yaml
└── overlays/             # Environment-specific overlays
    ├── dev/
    │   └── kustomization.yaml
    ├── staging/
    │   └── kustomization.yaml
    └── production/
        ├── kustomization.yaml
        └── resourcequota.yaml
```

## Prerequisites

1. **Kubernetes cluster** (v1.24+)
2. **kubectl** CLI tool
3. **kustomize** (v4.0+) or kubectl with built-in kustomize support
4. **VPSie API credentials** (Client ID and Client Secret)

## Quick Start

### 1. Create VPSie Secret

First, create a secret containing your VPSie API credentials:

```bash
kubectl create namespace vpsie-system

kubectl create secret generic vpsie-secret \
  --from-literal=clientId='YOUR_CLIENT_ID' \
  --from-literal=clientSecret='YOUR_CLIENT_SECRET' \
  --from-literal=url='https://api.vpsie.com/v2' \
  -n vpsie-system
```

### 2. Install CRDs

Install the Custom Resource Definitions:

```bash
kubectl apply -f ../../deploy/crds/
```

### 3. Deploy Using Kustomize

Choose the appropriate environment and deploy:

#### Development

```bash
kubectl apply -k deployments/overlays/dev/
```

#### Staging

```bash
kubectl apply -k deployments/overlays/staging/
```

#### Production

```bash
kubectl apply -k deployments/overlays/production/
```

### 4. Verify Deployment

```bash
# Check deployment status
kubectl get deployments -n vpsie-system

# Check pods
kubectl get pods -n vpsie-system

# Check logs
kubectl logs -f -n vpsie-system -l app.kubernetes.io/name=vpsie-autoscaler

# Check controller health
kubectl get --raw /api/v1/namespaces/vpsie-system/services/vpsie-autoscaler-metrics:health/proxy/healthz
```

## Environment Configurations

### Development

- **Replicas**: 1
- **Log Level**: debug
- **Log Format**: console
- **Leader Election**: disabled
- **Sync Period**: 15s
- **Cost Optimization**: conservative (1h interval)
- **Rebalancing**: disabled
- **Spot Instances**: disabled
- **Resource Requests**: 50m CPU, 64Mi memory
- **Resource Limits**: 200m CPU, 256Mi memory

### Staging

- **Replicas**: 2
- **Log Level**: info
- **Log Format**: json
- **Leader Election**: enabled
- **Sync Period**: 30s
- **Cost Optimization**: auto (12h interval)
- **Rebalancing**: enabled (12h interval, max 1 concurrent)
- **Spot Instances**: enabled
- **Resource Requests**: 75m CPU, 96Mi memory
- **Resource Limits**: 300m CPU, 384Mi memory

### Production

- **Replicas**: 3
- **Log Level**: info
- **Log Format**: json
- **Leader Election**: enabled
- **Sync Period**: 30s
- **Cost Optimization**: auto (24h interval)
- **Rebalancing**: enabled (24h interval, max 2 concurrent)
- **Spot Instances**: enabled
- **Resource Requests**: 100m CPU, 128Mi memory
- **Resource Limits**: 500m CPU, 512Mi memory
- **Node Affinity**: Prefers control-plane nodes
- **PDB minAvailable**: 2

## Customization

### Overriding Image Tag

```bash
kubectl kustomize deployments/overlays/production/ | \
  kubectl patch --local -f - -p '{"spec":{"template":{"spec":{"containers":[{"name":"controller","image":"ghcr.io/vpsie/vpsie-k8s-autoscaler:v0.5.0"}]}}}}' --type=strategic -o yaml | \
  kubectl apply -f -
```

### Adding Custom Patches

Create a `patches/` directory in your overlay:

```yaml
# deployments/overlays/production/patches/custom-resources.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vpsie-autoscaler-controller
spec:
  template:
    spec:
      containers:
      - name: controller
        resources:
          requests:
            cpu: "200m"
            memory: "256Mi"
```

Then reference it in `kustomization.yaml`:

```yaml
patches:
  - path: patches/custom-resources.yaml
```

### Enabling ServiceMonitor

If you have Prometheus Operator installed:

1. Edit `deployments/base/kustomization.yaml`
2. Uncomment the servicemonitor.yaml line:
   ```yaml
   resources:
     - servicemonitor.yaml
   ```

### Using External Secrets

For production, consider using external secret management:

1. **Sealed Secrets**:
   ```bash
   kubeseal --format yaml < vpsie-secret.yaml > sealed-secret.yaml
   kubectl apply -f sealed-secret.yaml
   ```

2. **External Secrets Operator**:
   ```yaml
   apiVersion: external-secrets.io/v1beta1
   kind: ExternalSecret
   metadata:
     name: vpsie-secret
   spec:
     secretStoreRef:
       name: aws-secretsmanager
       kind: SecretStore
     target:
       name: vpsie-secret
     data:
       - secretKey: clientId
         remoteRef:
           key: vpsie-credentials
           property: clientId
       - secretKey: clientSecret
         remoteRef:
           key: vpsie-credentials
           property: clientSecret
   ```

## Monitoring

### Prometheus Metrics

The autoscaler exposes metrics on port 8080:

- `vpsie_nodegroup_*` - NodeGroup metrics
- `vpsie_vpsienode_*` - VPSieNode metrics
- `vpsie_cost_*` - Cost optimization metrics
- `vpsie_rebalancing_*` - Rebalancing metrics

Access metrics:

```bash
kubectl port-forward -n vpsie-system svc/vpsie-autoscaler-metrics 8080:8080
curl http://localhost:8080/metrics
```

### Health Endpoints

```bash
# Liveness probe
curl http://localhost:8081/healthz

# Readiness probe
curl http://localhost:8081/readyz
```

## Upgrading

### Rolling Upgrade

```bash
# Update the image tag in the overlay
kubectl kustomize deployments/overlays/production/ | kubectl apply -f -

# Watch the rollout
kubectl rollout status deployment/vpsie-autoscaler-controller -n vpsie-system
```

### Rollback

```bash
kubectl rollout undo deployment/vpsie-autoscaler-controller -n vpsie-system
```

## Troubleshooting

### Check Controller Logs

```bash
kubectl logs -f -n vpsie-system -l app.kubernetes.io/name=vpsie-autoscaler --tail=100
```

### Check Events

```bash
kubectl get events -n vpsie-system --sort-by='.lastTimestamp'
```

### Check CRD Status

```bash
kubectl get nodegroups -A
kubectl describe nodegroup <name> -n <namespace>
```

### Common Issues

1. **Controller not starting**:
   - Check secret exists: `kubectl get secret vpsie-secret -n vpsie-system`
   - Verify credentials are correct
   - Check RBAC permissions

2. **Leader election errors**:
   - Ensure only one namespace is used
   - Check lease resource: `kubectl get leases -n vpsie-system`

3. **API rate limiting**:
   - Adjust `VPSIE_RATE_LIMIT` in ConfigMap
   - Increase sync period

## Uninstalling

```bash
# Delete the deployment
kubectl delete -k deployments/overlays/production/

# Delete CRDs (this will delete all NodeGroups and VPSieNodes!)
kubectl delete -f ../../deploy/crds/

# Delete the namespace
kubectl delete namespace vpsie-system
```

## Additional Resources

- [Kustomize Documentation](https://kustomize.io/)
- [VPSie API Documentation](https://api.vpsie.com/docs)
- [Project Documentation](../../docs/)
