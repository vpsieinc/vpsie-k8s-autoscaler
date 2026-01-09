# Provisioning Failed Runbook

## Symptom
VPSieNode resources are stuck in `Provisioning` phase or transitioning to `Failed` phase.

## Alert
- `VPSieNodeProvisioningFailed` (critical)
- `VPSieNodeProvisioningTimeout` (warning)

## Impact
- New nodes not joining cluster
- Scale-up operations failing
- Workload scheduling delayed

## Diagnosis Steps

### 1. Check VPSieNode Status
```bash
# List all VPSieNodes with phase
kubectl get vpsienodes -A -o wide

# Get detailed status
kubectl describe vpsienode <name> -n <namespace>
```

### 2. Check VPSieNode Events
```bash
kubectl get events --field-selector involvedObject.name=<vpsienode-name> -n <namespace>
```

### 3. Check Controller Logs
```bash
kubectl logs -n kube-system deployment/vpsie-autoscaler --tail=200 | grep -i "<vpsienode-name>\|provision\|failed"
```

### 4. Check VPSie Console
- Log into VPSie console
- Check if VM was created
- Check VM status (running, stopped, error)

### 5. Check Node Registration
```bash
# If VM was created, check if node registered
kubectl get nodes | grep <expected-node-name>
```

## Common Causes

### 1. VM Creation Failed
**Symptoms:** No VPSieInstanceID in VPSieNode, creation error in logs

**Diagnosis:**
```bash
kubectl get vpsienode <name> -o jsonpath='{.spec.vpsieInstanceID}'
# Returns 0 if not created
```

**Resolution:**
- Check VPSie API errors (see vpsie-api-errors.md)
- Verify offering/datacenter/image IDs are valid
- Check account quotas

### 2. VM Created but Not Registering
**Symptoms:** VPSieInstanceID populated, but node not in cluster

**Diagnosis:**
```bash
# Check VPSieNode has instance ID
kubectl get vpsienode <name> -o jsonpath='{.spec.vpsieInstanceID}'

# Check nodes
kubectl get nodes -o wide
```

**Resolution:**
- SSH into VM (via VPSie console)
- Check kubelet status: `systemctl status kubelet`
- Check cloud-init logs: `cat /var/log/cloud-init-output.log`
- Check network connectivity to API server

### 3. Timeout During Provisioning
**Symptoms:** VPSieNode stuck in Provisioning for extended period

**Diagnosis:**
```bash
# Check provisioning start time
kubectl get vpsienode <name> -o jsonpath='{.status.provisioningStartTime}'

# Calculate duration
```

**Resolution:**
```bash
# Delete stuck VPSieNode (will terminate VM and recreate)
kubectl delete vpsienode <name> -n <namespace>
```

### 4. Invalid Kubernetes Version
**Symptoms:** Node fails to join with version mismatch errors

**Diagnosis:**
```bash
# Check configured version
kubectl get nodegroup <ng-name> -o jsonpath='{.spec.kubernetesVersion}'

# Check cluster version
kubectl version --short
```

**Resolution:**
- Update NodeGroup `kubernetesVersion` to match cluster
- Ensure OS image supports target K8s version

### 5. SSH Key Issues
**Symptoms:** Cannot access VM for debugging

**Diagnosis:**
```bash
# Check SSH keys configured
kubectl get nodegroup <ng-name> -o jsonpath='{.spec.sshKeyIDs}'
```

**Resolution:**
- Verify SSH key IDs exist in VPSie account
- Update NodeGroup with valid SSH key IDs

## Resolution Steps

### Delete and Recreate
```bash
# Delete failed VPSieNode
kubectl delete vpsienode <name> -n <namespace>

# NodeGroup controller will create a new one
# Monitor new VPSieNode
kubectl get vpsienodes -w
```

### Manual VM Cleanup
If VM exists in VPSie but VPSieNode deleted:
1. Log into VPSie console
2. Find orphaned VM
3. Terminate VM manually
4. Wait for autoscaler to create new node

### Force NodeGroup Reconciliation
```bash
kubectl annotate nodegroup <name> reconcile-trigger=$(date +%s) --overwrite
```

### Debug Node Registration
If VM is running but not joining:

```bash
# Get VM IP from VPSie console
# SSH to VM
ssh root@<vm-ip>

# Check kubelet status
systemctl status kubelet
journalctl -u kubelet -n 100

# Check cloud-init
cat /var/log/cloud-init-output.log | tail -100

# Test API server connectivity
curl -k https://<api-server>:6443/healthz
```

## Prevention
- Monitor `VPSieNodeProvisioningFailed` alert
- Set appropriate provisioning timeouts
- Validate NodeGroup configuration before applying
- Test OS images in staging before production
- Monitor `node_provisioning_duration_seconds` for anomalies

## Escalation
1. Collect VPSieNode status: `kubectl get vpsienode <name> -o yaml`
2. Collect controller logs
3. Check VPSie console for VM status
4. If VPSie VM issue: contact VPSie support
5. If cluster issue: contact platform team
