# Upgrade Guide

This guide covers upgrading the VPSie Kubernetes Node Autoscaler to a new version.

## Before You Begin

### Pre-Upgrade Checklist

1. **Review the release notes** for breaking changes
2. **Check Kubernetes compatibility** for the new version
3. **Verify cluster health** before starting
4. **Create backups** of custom resources

### Backup Procedure

1. **Backup CRD resources**:
   ```bash
   kubectl get nodegroup -A -o yaml > backup-nodegroups.yaml
   kubectl get vpsienode -A -o yaml > backup-vpsienodes.yaml
   kubectl get autoscalerconfig -o yaml > backup-autoscalerconfig.yaml
   ```

2. **Backup the VPSie secret** (for reference):
   ```bash
   kubectl get secret vpsie-secret -n kube-system -o yaml > backup-vpsie-secret.yaml
   ```

3. **Note current version**:
   ```bash
   kubectl get deployment vpsie-autoscaler -n kube-system -o jsonpath='{.spec.template.spec.containers[0].image}'
   ```

## Upgrade Methods

### Method 1: Helm Upgrade (Recommended)

```bash
# Update Helm repository
helm repo update

# Check current release
helm list -n kube-system | grep vpsie-autoscaler

# Perform upgrade
helm upgrade vpsie-autoscaler vpsie/vpsie-autoscaler \
  --namespace kube-system \
  --reuse-values

# Or with specific version
helm upgrade vpsie-autoscaler vpsie/vpsie-autoscaler \
  --namespace kube-system \
  --version X.Y.Z \
  --reuse-values
```

### Method 2: kubectl Upgrade

1. **Update CRDs first** (if schema changed):
   ```bash
   kubectl apply -f deploy/crds/
   ```

2. **Update the controller deployment**:
   ```bash
   kubectl apply -f deploy/manifests/
   ```

   Or update the image directly:
   ```bash
   kubectl set image deployment/vpsie-autoscaler \
     vpsie-autoscaler=ghcr.io/vpsieinc/vpsie-autoscaler:vX.Y.Z \
     -n kube-system
   ```

## CRD Schema Changes

When upgrading between versions with CRD schema changes:

### Non-Breaking Changes (Adding Fields)

These are handled automatically:
```bash
kubectl apply -f deploy/crds/
```

### Breaking Changes (Field Removal/Type Change)

1. **Apply new CRDs with replace**:
   ```bash
   kubectl replace -f deploy/crds/
   ```

2. **If resources fail validation**, update them:
   ```bash
   kubectl get nodegroup -A -o yaml | \
     sed 's/oldFieldName/newFieldName/g' | \
     kubectl apply -f -
   ```

### Migration Scripts

For complex migrations, the release may include migration scripts:
```bash
./scripts/migrate-vX.Y.Z.sh
```

## Rollback Procedure

If the upgrade causes issues, rollback to the previous version:

### Helm Rollback

```bash
# List revision history
helm history vpsie-autoscaler -n kube-system

# Rollback to previous revision
helm rollback vpsie-autoscaler -n kube-system

# Or rollback to specific revision
helm rollback vpsie-autoscaler 2 -n kube-system
```

### kubectl Rollback

```bash
# Rollback deployment
kubectl rollout undo deployment/vpsie-autoscaler -n kube-system

# Or to specific revision
kubectl rollout undo deployment/vpsie-autoscaler -n kube-system --to-revision=2
```

### CRD Rollback

If you need to rollback CRDs (dangerous - may lose data):

1. **Backup current resources**:
   ```bash
   kubectl get nodegroup -A -o yaml > emergency-backup-nodegroups.yaml
   ```

2. **Apply old CRDs**:
   ```bash
   git checkout v1.0.0 -- deploy/crds/
   kubectl apply -f deploy/crds/
   ```

3. **Reapply resources** (may need manual adjustment):
   ```bash
   kubectl apply -f emergency-backup-nodegroups.yaml
   ```

## Verification Steps

After upgrade, verify the system is working correctly:

1. **Check deployment status**:
   ```bash
   kubectl rollout status deployment/vpsie-autoscaler -n kube-system
   ```

2. **Verify new version**:
   ```bash
   kubectl get deployment vpsie-autoscaler -n kube-system \
     -o jsonpath='{.spec.template.spec.containers[0].image}'
   ```

3. **Check controller logs**:
   ```bash
   kubectl logs -n kube-system -l app=vpsie-autoscaler --tail=100
   ```

4. **Verify health endpoints**:
   ```bash
   kubectl exec -n kube-system deploy/vpsie-autoscaler -- curl -s localhost:8081/healthz
   kubectl exec -n kube-system deploy/vpsie-autoscaler -- curl -s localhost:8081/readyz
   ```

5. **Check CRD status**:
   ```bash
   kubectl get crd nodegroups.autoscaler.vpsie.com -o jsonpath='{.status.storedVersions}'
   ```

6. **Verify NodeGroups are reconciling**:
   ```bash
   kubectl get nodegroup -A -o wide
   ```

7. **Check metrics are being collected**:
   ```bash
   kubectl exec -n kube-system deploy/vpsie-autoscaler -- \
     curl -s localhost:8080/metrics | grep vpsie_nodegroup
   ```

## Version-Specific Notes

### Upgrading to v1.x from v0.x

- New CRD fields require running `kubectl apply -f deploy/crds/`
- Leader election is now enabled by default
- Webhook validation requires TLS certificates

### Upgrading to v2.x from v1.x

- Check release notes for specific migration requirements
- AutoscalerConfig CRD may have new required fields

## Troubleshooting Upgrades

### Controller Fails to Start

```bash
# Check for CRD validation errors
kubectl logs -n kube-system -l app=vpsie-autoscaler --previous

# Verify CRDs are installed correctly
kubectl get crd | grep vpsie
```

### Resources in Unknown State

```bash
# Check resource status
kubectl describe nodegroup -A

# Force reconciliation by adding annotation
kubectl annotate nodegroup <name> -n <namespace> \
  reconcile.autoscaler.vpsie.com/trigger=$(date +%s) --overwrite
```

### Leader Election Issues

```bash
# Check leader election lease
kubectl get lease -n kube-system | grep vpsie

# Delete lease to trigger re-election (use with caution)
kubectl delete lease vpsie-autoscaler-leader -n kube-system
```

## Best Practices

1. **Test in staging first**: Always test upgrades in a non-production environment
2. **Upgrade during low-traffic periods**: Minimize impact of any issues
3. **Monitor closely after upgrade**: Watch logs and metrics for anomalies
4. **Keep backups**: Maintain backups of CRD resources before each upgrade
5. **Read release notes**: Check for deprecations and breaking changes
6. **Plan rollback**: Know how to rollback before you upgrade

## See Also

- [Installation Guide](installation.md)
- [Architecture](../ARCHITECTURE.md)
- [Release Notes](https://github.com/vpsieinc/vpsie-k8s-autoscaler/releases)
