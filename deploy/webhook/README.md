# VPSie Kubernetes Autoscaler - Validation Webhook

This directory contains Kubernetes manifests for deploying the VPSie Autoscaler validation webhook.

## Overview

The validation webhook provides comprehensive validation for NodeGroup and VPSieNode custom resources before they are persisted to the Kubernetes API server. This ensures that invalid configurations are rejected early, preventing runtime errors and improving cluster stability.

## Validation Rules

### NodeGroup Validation

The webhook validates the following for NodeGroup resources:

**Node Count Validation:**
- `minNodes` must be >= 0
- `maxNodes` must be >= 0
- `minNodes` cannot be greater than `maxNodes`
- `maxNodes` cannot exceed 1000 (reasonable upper limit)

**Datacenter Validation:**
- Required field, cannot be empty
- Must contain only alphanumeric characters, hyphens, and underscores

**Offering IDs Validation:**
- Must contain at least one offering ID
- No empty offering IDs allowed
- No duplicate offering IDs
- Only alphanumeric characters and hyphens allowed

**Kubernetes Version Validation:**
- Required field, cannot be empty
- Must follow semantic versioning format (e.g., v1.28.0, v1.29.1-rc.0)

**OS Image Validation:**
- Required field, cannot be empty
- Only alphanumeric characters, dots, hyphens, and underscores allowed

**Scale-Up Policy Validation:**
- `cooldownSeconds`: 0-3600 seconds (max 1 hour)
- `stabilizationWindowSeconds`: 0-1800 seconds (max 30 minutes)
- `maxNodesPerScale`: 1-100 nodes

**Scale-Down Policy Validation:**
- `cooldownSeconds`: 0-3600 seconds (max 1 hour)
- `stabilizationWindowSeconds`: 0-3600 seconds (max 1 hour)
- `maxNodesPerScale`: 1-100 nodes
- `cpuThreshold`: 0-100 percent
- `memoryThreshold`: 0-100 percent

**Labels Validation:**
- Keys and values must follow Kubernetes label naming conventions
- Key length <= 253 characters
- Value length <= 63 characters
- Reserved prefixes (kubernetes.io/, k8s.io/) are not allowed

**Taints Validation:**
- Effect must be: NoSchedule, PreferNoSchedule, or NoExecute
- Keys cannot use reserved Kubernetes prefixes

### VPSieNode Validation

The webhook validates the following for VPSieNode resources:

**NodeGroup Reference Validation:**
- Required field, cannot be empty
- Must be a valid Kubernetes resource name

**Datacenter Validation:**
- Required field, cannot be empty
- Only alphanumeric characters, hyphens, and underscores allowed

**Offering ID Validation:**
- Required field, cannot be empty
- Only alphanumeric characters and hyphens allowed

**Kubernetes Version Validation:**
- Required field, cannot be empty
- Must follow semantic versioning format

**OS Image Validation:**
- Required field, cannot be empty
- Only alphanumeric characters, dots, hyphens, and underscores allowed

**SSH Key IDs Validation (Optional):**
- No empty key IDs
- No duplicate key IDs
- Only alphanumeric characters, hyphens, and underscores allowed

**User Data Validation (Optional):**
- Maximum size: 64KB

## Prerequisites

1. **Kubernetes cluster**: Version 1.16+
2. **kubectl**: Configured to access your cluster
3. **openssl**: For generating TLS certificates
4. **CRDs**: NodeGroup and VPSieNode CRDs must be installed

## Deployment Steps

### 1. Generate TLS Certificates

The webhook requires TLS certificates for secure communication with the Kubernetes API server.

```bash
# Make the script executable
chmod +x generate-certs.sh

# Generate certificates and create Kubernetes secret
./generate-certs.sh
```

The script will:
- Generate a self-signed CA certificate
- Generate a server certificate for the webhook
- Create a Kubernetes secret `vpsie-autoscaler-webhook-certs` in the `kube-system` namespace
- Update the `ValidatingWebhookConfiguration` with the CA bundle

### 2. Deploy RBAC Resources

```bash
kubectl apply -f rbac.yaml
```

This creates:
- ServiceAccount: `vpsie-autoscaler-webhook`
- ClusterRole: Permissions to read NodeGroups, VPSieNodes, and Nodes
- ClusterRoleBinding: Binds the ClusterRole to the ServiceAccount

### 3. Deploy Webhook Server

```bash
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
```

This creates:
- Deployment: 2 replicas of the webhook server with anti-affinity
- Service: ClusterIP service for webhook and health endpoints

### 4. Deploy Webhook Configuration

```bash
kubectl apply -f webhook-configuration.yaml
```

This creates:
- ValidatingWebhookConfiguration: Configures Kubernetes to call the webhook for NodeGroup and VPSieNode validation

### 5. Verify Deployment

```bash
# Check webhook pods
kubectl get pods -n kube-system -l app=vpsie-autoscaler-webhook

# Check webhook service
kubectl get svc -n kube-system vpsie-autoscaler-webhook

# Check webhook configuration
kubectl get validatingwebhookconfiguration vpsie-autoscaler-webhook
```

## Testing the Webhook

### Test Valid NodeGroup

```bash
cat <<EOF | kubectl apply -f -
apiVersion: autoscaler.vpsie.com/v1alpha1
kind: NodeGroup
metadata:
  name: test-nodegroup
  namespace: default
spec:
  minNodes: 1
  maxNodes: 5
  datacenter: us-west-1
  offeringIds:
    - vpsie-m1-small
  kubernetesVersion: v1.28.0
  osImageId: ubuntu-22.04
  scaleUpPolicy:
    cooldownSeconds: 300
    stabilizationWindowSeconds: 60
    maxNodesPerScale: 3
  scaleDownPolicy:
    cooldownSeconds: 600
    stabilizationWindowSeconds: 300
    maxNodesPerScale: 2
    cpuThreshold: 50
    memoryThreshold: 50
EOF
```

Expected result: ✅ NodeGroup created successfully

### Test Invalid NodeGroup (minNodes > maxNodes)

```bash
cat <<EOF | kubectl apply -f -
apiVersion: autoscaler.vpsie.com/v1alpha1
kind: NodeGroup
metadata:
  name: invalid-nodegroup
  namespace: default
spec:
  minNodes: 10
  maxNodes: 5
  datacenter: us-west-1
  offeringIds:
    - vpsie-m1-small
  kubernetesVersion: v1.28.0
  osImageId: ubuntu-22.04
  scaleUpPolicy:
    cooldownSeconds: 300
    stabilizationWindowSeconds: 60
    maxNodesPerScale: 3
  scaleDownPolicy:
    cooldownSeconds: 600
    stabilizationWindowSeconds: 300
    maxNodesPerScale: 2
    cpuThreshold: 50
    memoryThreshold: 50
EOF
```

Expected result: ❌ Error: `spec.minNodes (10) cannot be greater than spec.maxNodes (5)`

### Test Invalid Kubernetes Version

```bash
cat <<EOF | kubectl apply -f -
apiVersion: autoscaler.vpsie.com/v1alpha1
kind: NodeGroup
metadata:
  name: invalid-version
  namespace: default
spec:
  minNodes: 1
  maxNodes: 5
  datacenter: us-west-1
  offeringIds:
    - vpsie-m1-small
  kubernetesVersion: invalid-version
  osImageId: ubuntu-22.04
  scaleUpPolicy:
    cooldownSeconds: 300
    stabilizationWindowSeconds: 60
    maxNodesPerScale: 3
  scaleDownPolicy:
    cooldownSeconds: 600
    stabilizationWindowSeconds: 300
    maxNodesPerScale: 2
    cpuThreshold: 50
    memoryThreshold: 50
EOF
```

Expected result: ❌ Error: `spec.kubernetesVersion 'invalid-version' is not a valid semantic version`

## Troubleshooting

### Webhook Not Called

If validation is not being performed:

1. Check webhook configuration:
   ```bash
   kubectl get validatingwebhookconfiguration vpsie-autoscaler-webhook -o yaml
   ```

2. Verify `caBundle` is set correctly

3. Check webhook service endpoints:
   ```bash
   kubectl get endpoints -n kube-system vpsie-autoscaler-webhook
   ```

### Webhook Timeout

If you see timeout errors:

1. Check webhook pod logs:
   ```bash
   kubectl logs -n kube-system -l app=vpsie-autoscaler-webhook
   ```

2. Verify webhook pods are running:
   ```bash
   kubectl get pods -n kube-system -l app=vpsie-autoscaler-webhook
   ```

3. Check network connectivity between API server and webhook

### Certificate Errors

If you see TLS/certificate errors:

1. Verify secret exists:
   ```bash
   kubectl get secret -n kube-system vpsie-autoscaler-webhook-certs
   ```

2. Check certificate validity:
   ```bash
   kubectl get secret -n kube-system vpsie-autoscaler-webhook-certs -o jsonpath='{.data.tls\.crt}' | base64 -d | openssl x509 -noout -text
   ```

3. Regenerate certificates if needed:
   ```bash
   ./generate-certs.sh
   kubectl rollout restart deployment -n kube-system vpsie-autoscaler-webhook
   ```

### Failure Policy

The webhook is configured with `failurePolicy: Fail`, which means:
- If the webhook is unavailable, resource creation/updates will be rejected
- This prevents invalid resources from being created during webhook downtime

To change to a more permissive policy (allow on failure):
```bash
kubectl patch validatingwebhookconfiguration vpsie-autoscaler-webhook \
  --type='json' \
  -p='[{"op": "replace", "path": "/webhooks/0/failurePolicy", "value": "Ignore"}]'
```

## Monitoring

### Health Checks

The webhook exposes health endpoints:

- **Liveness**: `http://:9444/healthz`
- **Readiness**: `http://:9444/readyz`

Check health status:
```bash
kubectl port-forward -n kube-system svc/vpsie-autoscaler-webhook 9444:9444
curl http://localhost:9444/healthz
curl http://localhost:9444/readyz
```

### Logs

View webhook logs:
```bash
# All webhook pods
kubectl logs -n kube-system -l app=vpsie-autoscaler-webhook --tail=100 -f

# Specific pod
kubectl logs -n kube-system <pod-name> --tail=100 -f
```

## Uninstalling

To remove the validation webhook:

```bash
# Delete webhook configuration (this stops validation)
kubectl delete validatingwebhookconfiguration vpsie-autoscaler-webhook

# Delete webhook server
kubectl delete -f deployment.yaml
kubectl delete -f service.yaml

# Delete RBAC
kubectl delete -f rbac.yaml

# Delete certificates secret
kubectl delete secret -n kube-system vpsie-autoscaler-webhook-certs
```

## Security Considerations

- **TLS Communication**: All communication between API server and webhook is encrypted with TLS
- **Non-root User**: Webhook runs as non-root user (UID 65532)
- **Read-only Filesystem**: Container filesystem is read-only except for mounted volumes
- **Dropped Capabilities**: All Linux capabilities are dropped
- **Resource Limits**: CPU and memory limits prevent resource exhaustion
- **RBAC**: Minimal permissions (read-only access to NodeGroups, VPSieNodes, and Nodes)

## Performance

- **Timeout**: 10 seconds per validation request
- **Concurrency**: 2 replicas with anti-affinity for high availability
- **Resource Usage**: Minimal (50m CPU, 64Mi memory per replica)
