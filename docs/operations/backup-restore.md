# Backup and Restore Guide

This guide covers backup and restore procedures for VPSie Autoscaler Custom Resource Definitions (CRDs) and resources.

## Overview

The VPSie Autoscaler uses two Custom Resource types:
- **NodeGroup**: Defines node pool configurations (min/max nodes, offerings, datacenter)
- **VPSieNode**: Represents individual VPSie VPS instances managed by the autoscaler

Regular backups ensure you can recover from:
- Accidental deletion of resources
- Cluster migrations
- Disaster recovery scenarios
- CRD upgrades gone wrong

## Prerequisites

- `kubectl` configured with cluster access
- Sufficient RBAC permissions to read/write CRDs and resources
- Bash shell (Linux, macOS, or WSL)

## Backup Procedures

### Basic Backup

Create a backup of all NodeGroups and VPSieNodes across all namespaces:

```bash
./scripts/backup-crds.sh
```

Output is saved to `./backups/<timestamp>/` with a `.tar.gz` archive.

### Namespace-Specific Backup

Backup resources from a specific namespace only:

```bash
./scripts/backup-crds.sh -n production
```

### Include Credentials

To include the `vpsie-secret` in the backup (contains API credentials):

```bash
./scripts/backup-crds.sh -s
```

**Security Warning**: The secrets file contains sensitive credentials. Store backups securely and consider encrypting them.

### Custom Output Location

```bash
./scripts/backup-crds.sh -o /path/to/backups/my-backup
```

### Full Example

```bash
# Production backup with secrets
./scripts/backup-crds.sh \
  --namespace production \
  --output ./backups/prod-$(date +%Y%m%d) \
  --include-secrets
```

## Backup Contents

A typical backup directory contains:

```
backups/20240115-120000/
├── backup-metadata.json    # Backup metadata (timestamp, cluster, options)
├── crds/                   # CRD definitions
│   ├── nodegroups.autoscaler.vpsie.com.yaml
│   └── vpsienodes.autoscaler.vpsie.com.yaml
├── nodegroups.yaml         # NodeGroup resources
├── vpsienodes.yaml         # VPSieNode resources
└── secrets.yaml            # (Optional) vpsie-secret
```

The archive `vpsie-autoscaler-backup-<timestamp>.tar.gz` is also created.

## Restore Procedures

### Pre-Restore Checklist

1. Verify target cluster connectivity: `kubectl cluster-info`
2. Check current context: `kubectl config current-context`
3. Ensure you have admin permissions
4. Review backup metadata to confirm source

### Dry Run

Always test with dry-run first:

```bash
./scripts/restore-crds.sh --dry-run ./backups/20240115-120000
```

Or from an archive:

```bash
./scripts/restore-crds.sh --dry-run vpsie-autoscaler-backup-20240115-120000.tar.gz
```

### Full Restore

Restore CRDs and resources:

```bash
./scripts/restore-crds.sh ./backups/20240115-120000
```

### Restore to Different Namespace

Override the original namespace:

```bash
./scripts/restore-crds.sh -n staging ./backups/20240115-120000
```

### CRDs Only

Restore only the CRD definitions (useful for fresh clusters):

```bash
./scripts/restore-crds.sh --crds-only ./backups/20240115-120000
```

### Resources Only

Skip CRD restore (CRDs already exist):

```bash
./scripts/restore-crds.sh --resources-only ./backups/20240115-120000
```

### Restore with Secrets

```bash
./scripts/restore-crds.sh --include-secrets ./backups/20240115-120000
```

**Warning**: This overwrites existing `vpsie-secret`.

### Non-Interactive Restore

For automation/scripts:

```bash
./scripts/restore-crds.sh --force ./backups/20240115-120000
```

## Disaster Recovery Scenarios

### Scenario 1: Accidental Resource Deletion

If a NodeGroup was accidentally deleted:

```bash
# Restore just resources (CRDs still exist)
./scripts/restore-crds.sh --resources-only ./backups/latest
```

### Scenario 2: Fresh Cluster Migration

Migrating to a new cluster:

```bash
# 1. On old cluster: Create backup with secrets
./scripts/backup-crds.sh -s -o ./migration-backup

# 2. Transfer backup to new cluster

# 3. On new cluster: Full restore
./scripts/restore-crds.sh --include-secrets ./migration-backup
```

### Scenario 3: CRD Upgrade Rollback

If a CRD upgrade causes issues:

```bash
# Restore previous CRD definitions
./scripts/restore-crds.sh --crds-only ./backups/pre-upgrade

# Verify
kubectl get crd nodegroups.autoscaler.vpsie.com -o yaml | head -20
```

### Scenario 4: Cluster Disaster Recovery

Complete cluster loss:

```bash
# 1. Rebuild cluster

# 2. Restore from offsite backup
./scripts/restore-crds.sh --include-secrets /mnt/backup/vpsie-autoscaler-backup-latest.tar.gz

# 3. Verify autoscaler deployment
kubectl get pods -n kube-system | grep vpsie

# 4. Check resources
kubectl get nodegroups --all-namespaces
kubectl get vpsienodes --all-namespaces
```

## Automated Backups

### CronJob Backup

Create a Kubernetes CronJob for automated backups:

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: vpsie-autoscaler-backup
  namespace: kube-system
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: vpsie-autoscaler-backup
          containers:
          - name: backup
            image: bitnami/kubectl:latest
            command:
            - /bin/bash
            - -c
            - |
              BACKUP_DIR="/backups/$(date +%Y%m%d-%H%M%S)"
              mkdir -p "$BACKUP_DIR/crds"

              # Backup CRDs
              kubectl get crd nodegroups.autoscaler.vpsie.com -o yaml > "$BACKUP_DIR/crds/nodegroups.yaml"
              kubectl get crd vpsienodes.autoscaler.vpsie.com -o yaml > "$BACKUP_DIR/crds/vpsienodes.yaml"

              # Backup resources
              kubectl get nodegroups --all-namespaces -o yaml > "$BACKUP_DIR/nodegroups.yaml"
              kubectl get vpsienodes --all-namespaces -o yaml > "$BACKUP_DIR/vpsienodes.yaml"

              # Create archive
              tar -czf "/backups/vpsie-backup-$(date +%Y%m%d).tar.gz" -C "$BACKUP_DIR" .

              # Cleanup old backups (keep 30 days)
              find /backups -name "*.tar.gz" -mtime +30 -delete
            volumeMounts:
            - name: backup-storage
              mountPath: /backups
          restartPolicy: OnFailure
          volumes:
          - name: backup-storage
            persistentVolumeClaim:
              claimName: vpsie-backup-pvc
```

### Required RBAC for Backup Job

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: vpsie-autoscaler-backup
  namespace: kube-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: vpsie-autoscaler-backup
rules:
- apiGroups: ["autoscaler.vpsie.com"]
  resources: ["nodegroups", "vpsienodes"]
  verbs: ["get", "list"]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: ["get", "list"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vpsie-autoscaler-backup
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: vpsie-autoscaler-backup
subjects:
- kind: ServiceAccount
  name: vpsie-autoscaler-backup
  namespace: kube-system
```

## Best Practices

1. **Regular Backups**: Schedule daily backups, retain for 30 days minimum
2. **Before Upgrades**: Always backup before upgrading CRDs or the autoscaler
3. **Offsite Storage**: Copy backups to cloud storage (S3, GCS, Azure Blob)
4. **Test Restores**: Periodically test restore to a staging cluster
5. **Encrypt Secrets**: If backing up secrets, encrypt the backup archive
6. **Version Control**: Consider storing CRD YAML in git for version history
7. **Documentation**: Record restore procedures in your runbooks

## Encryption (Recommended)

Encrypt backups containing secrets:

```bash
# Backup and encrypt
./scripts/backup-crds.sh -s -o ./backup-temp
gpg --symmetric --cipher-algo AES256 ./backup-temp/../*.tar.gz
rm -rf ./backup-temp ./backup-temp/../*.tar.gz

# Decrypt and restore
gpg --decrypt backup.tar.gz.gpg > backup.tar.gz
./scripts/restore-crds.sh backup.tar.gz
```

## Troubleshooting

### "CRD not found" during backup

The autoscaler CRDs are not installed. Install them first:

```bash
kubectl apply -f deploy/crds/
```

### "Cannot connect to cluster" error

Check your kubeconfig:

```bash
kubectl config current-context
kubectl cluster-info
```

### Restore fails with validation errors

The backup may contain invalid resources. Check:

```bash
kubectl apply --dry-run=server -f backup/nodegroups.yaml
```

### Resources exist after restore shows success

The restore uses `kubectl apply`, which updates existing resources. Use `kubectl delete` first for a clean restore:

```bash
kubectl delete nodegroups --all --all-namespaces
./scripts/restore-crds.sh ./backup
```

## Related Documentation

- [Operational Runbooks](../runbooks/)
- [VPSie API Errors Runbook](../runbooks/vpsie-api-errors.md)
- [Architecture Overview](../architecture/overview.md)
