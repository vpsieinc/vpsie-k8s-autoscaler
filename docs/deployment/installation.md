# Installation Guide

This guide covers the installation and initial configuration of the VPSie Kubernetes Node Autoscaler.

## Prerequisites

Before installing the autoscaler, ensure you have:

1. **Kubernetes Cluster**: Version 1.24 or later
2. **VPSie Account**: With API credentials (OAuth client ID and secret)
3. **kubectl**: Configured to access your cluster
4. **Helm 3**: For Helm-based installation (recommended)

## Installation Methods

### Method 1: Helm Installation (Recommended)

1. **Add the Helm repository** (if published):
   ```bash
   helm repo add vpsie https://charts.vpsie.com
   helm repo update
   ```

2. **Create the VPSie credentials secret**:
   ```bash
   kubectl create secret generic vpsie-secret \
     --from-literal=clientId='your-oauth-client-id' \
     --from-literal=clientSecret='your-oauth-client-secret' \
     -n kube-system
   ```

3. **Install the autoscaler**:
   ```bash
   helm install vpsie-autoscaler vpsie/vpsie-autoscaler \
     --namespace kube-system \
     --set resourceIdentifier='your-cluster-uuid'
   ```

### Method 2: Manual Installation with kubectl

1. **Create the VPSie credentials secret**:
   ```bash
   kubectl create secret generic vpsie-secret \
     --from-literal=clientId='your-oauth-client-id' \
     --from-literal=clientSecret='your-oauth-client-secret' \
     -n kube-system
   ```

2. **Apply the CRDs**:
   ```bash
   kubectl apply -f deploy/crds/
   ```

3. **Apply the manifests**:
   ```bash
   kubectl apply -f deploy/manifests/
   ```

## Configuration

### VPSie Secret Configuration

The `vpsie-secret` in `kube-system` namespace requires:

| Key | Required | Description |
|-----|----------|-------------|
| `clientId` | Yes | VPSie OAuth client ID |
| `clientSecret` | Yes | VPSie OAuth client secret |
| `url` | No | API endpoint (defaults to https://api.vpsie.com/v2) |
| `resourceIdentifier` | No | VPSie cluster UUID (auto-discovered if not set) |
| `datacenterId` | No | VPSie datacenter UUID (auto-discovered if not set) |
| `projectId` | No | VPSie project UUID (auto-discovered if not set) |

### Controller Configuration

Key controller flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--metrics-addr` | `:8080` | Metrics server bind address |
| `--health-addr` | `:8081` | Health probe bind address |
| `--enable-leader-election` | `true` | Enable leader election for HA |
| `--log-level` | `info` | Log level (debug, info, warn, error) |
| `--resource-identifier` | (auto) | VPSie cluster UUID |

## Verifying Installation

1. **Check controller deployment**:
   ```bash
   kubectl get deployment -n kube-system vpsie-autoscaler
   kubectl logs -n kube-system -l app=vpsie-autoscaler -f
   ```

2. **Check health endpoints**:
   ```bash
   kubectl exec -n kube-system deploy/vpsie-autoscaler -- curl -s localhost:8081/healthz
   kubectl exec -n kube-system deploy/vpsie-autoscaler -- curl -s localhost:8081/readyz
   ```

3. **Verify CRDs are installed**:
   ```bash
   kubectl get crd nodegroups.autoscaler.vpsie.com
   kubectl get crd vpsienodes.autoscaler.vpsie.com
   kubectl get crd autoscalerconfigs.autoscaler.vpsie.com
   ```

## First Scaling Test

1. **Create a test NodeGroup**:
   ```yaml
   apiVersion: autoscaler.vpsie.com/v1alpha1
   kind: NodeGroup
   metadata:
     name: test-nodegroup
     namespace: kube-system
     labels:
       autoscaler.vpsie.com/managed: "true"
   spec:
     minNodes: 1
     maxNodes: 5
     kubernetesVersion: "v1.28.0"
     resourceIdentifier: "your-cluster-uuid"
     datacenterID: "your-datacenter-uuid"
     offeringIDs:
       - "offering-uuid"
     scaleUpPolicy:
       enabled: true
       cpuThreshold: 80
       memoryThreshold: 80
     scaleDownPolicy:
       enabled: true
       cpuThreshold: 30
       memoryThreshold: 30
   ```

2. **Apply the NodeGroup**:
   ```bash
   kubectl apply -f test-nodegroup.yaml
   ```

3. **Monitor the NodeGroup status**:
   ```bash
   kubectl get nodegroup test-nodegroup -n kube-system -o yaml
   ```

4. **Trigger scale-up by creating resource-hungry pods**:
   ```yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: stress-test
   spec:
     replicas: 10
     selector:
       matchLabels:
         app: stress-test
     template:
       metadata:
         labels:
           app: stress-test
       spec:
         containers:
         - name: stress
           image: nginx
           resources:
             requests:
               cpu: "500m"
               memory: "512Mi"
   ```

5. **Watch for new nodes being provisioned**:
   ```bash
   kubectl get nodes -w
   kubectl get vpsienodes -n kube-system -w
   ```

6. **Clean up test resources**:
   ```bash
   kubectl delete deployment stress-test
   kubectl delete nodegroup test-nodegroup -n kube-system
   ```

## Troubleshooting

### Common Issues

1. **VPSie API authentication failures**:
   - Verify `clientId` and `clientSecret` in the secret
   - Check if OAuth credentials have the required permissions

2. **Auto-discovery failures**:
   - Set `resourceIdentifier` explicitly in the secret or via flag
   - Ensure the cluster exists in VPSie API

3. **Nodes not scaling up**:
   - Check controller logs for pending pod detection
   - Verify NodeGroup configuration matches cluster settings

### Useful Commands

```bash
# View controller logs
kubectl logs -n kube-system -l app=vpsie-autoscaler -f

# Check NodeGroup status
kubectl describe nodegroup -n kube-system

# View VPSieNode resources
kubectl get vpsienodes -n kube-system -o wide

# Check metrics
kubectl exec -n kube-system deploy/vpsie-autoscaler -- curl -s localhost:8080/metrics | grep vpsie
```

## Next Steps

- [Upgrading](upgrading.md) - Upgrade procedures
- [Architecture](../ARCHITECTURE.md) - System architecture details
- [Runbooks](../runbooks/) - Operational procedures
