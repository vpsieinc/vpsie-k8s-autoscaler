# Work Plan: Remove Cloud-Init, Use VPSie API for Node Configuration

**Date:** 2025-12-23
**Priority:** Critical (Architectural correction)
**Estimated Effort:** 4-6 hours

## Context

The current implementation incorrectly assumes nodes are configured via cloud-init templates. However, **nodes are actually configured via QEMU agent through the VPSie API**, which handles all node configuration automatically. This requires removing cloud-init related code and documentation.

## Impact Analysis

### Files to Remove
1. `deploy/examples/cloud-init/` directory (3 files)
   - `example-nodegroup-with-template.yaml`
   - `gpu-template.yaml`
   - `default-template.yaml`

### Files to Modify

#### CRD Changes (pkg/apis/autoscaler/v1alpha1/nodegroup_types.go)
**Remove:**
- Line 69-73: `UserData` field (deprecated cloud-init field)
- Line 75-79: `CloudInitTemplate` field
- Line 81-84: `CloudInitTemplateRef` field
- Line 86-97: `CloudInitVariables` field
- Line 295-310: `CloudInitTemplateRef` type definition

**Impact:** Breaking change for existing NodeGroup resources using these fields

#### Provisioner Changes (pkg/controller/vpsienode/provisioner.go)
**Remove:**
- Line 20-21: `cloudInitTemplate` field from Provisioner struct
- Line 27: `cloudInitTemplate` parameter from NewProvisioner()
- Line 30: `cloudInitTemplate` assignment
- Line 66: `UserData` field from CreateVPSRequest
- Line 240-268: `generateCloudInit()` function (entire function)

**Simplify:** VPS creation relies entirely on VPSie API's QEMU agent configuration

#### Controller Options (pkg/controller/options.go)
**Remove:** Any cloud-init template configuration options

#### VPSie Client (pkg/vpsie/client/client.go)
**Remove:** `UserData` field from CreateVPSRequest type

#### Documentation Updates
**Modify:**
- `docs/API.md` - Remove cloud-init references
- `docs/PRD.md` - Update node provisioning description
- `CHANGELOG.md` - Add breaking change note
- `CurrentState.md` - Update architecture description
- `README.md` - Update provisioning flow description

#### CRD Manifests (Auto-generated)
**Regenerate:** After CRD type changes
- `deploy/crds/autoscaler.vpsie.com_nodegroups.yaml`

### Critical Fixes Impact

**Fix #1 (Template Injection Vulnerability) becomes OBSOLETE** - No template execution means no injection risk

**Remaining Critical Fixes:**
- Fix #2: Label sanitization (STILL NEEDED)
- Fix #3: DeepCopy generation (STILL NEEDED - will regenerate without cloud-init fields)
- Fix #4: Safety check tests (STILL NEEDED)
- Fix #5: Webhook deletion tests (STILL NEEDED)
- Fix #6: Integration test helpers (STILL NEEDED)

## Implementation Plan

### Phase 1: CRD Updates (1 hour)
1. Remove cloud-init fields from `nodegroup_types.go`
2. Run `make generate` to regenerate DeepCopy methods and CRD manifests
3. Verify CRD manifests no longer contain cloud-init fields
4. Run `make test` to identify broken tests

### Phase 2: Provisioner Refactoring (1 hour)
1. Remove cloud-init template from Provisioner struct
2. Remove `generateCloudInit()` function
3. Update `NewProvisioner()` signature
4. Remove `UserData` from CreateVPSRequest
5. Update all callsites in controller
6. Run unit tests for provisioner

### Phase 3: Controller Updates (30 minutes)
1. Remove cloud-init template options from controller configuration
2. Update controller initialization to not pass cloud-init template
3. Verify controller compiles and runs

### Phase 4: Documentation Cleanup (1 hour)
1. Remove `deploy/examples/cloud-init/` directory
2. Update API documentation
3. Update PRD and architecture docs
4. Add migration guide for users with existing cloud-init configurations
5. Update CHANGELOG with breaking change notice

### Phase 5: Critical Fixes Update (30 minutes)
1. Update ADR and Design Doc to remove Fix #1 (template validation)
2. Update work plan to reflect 5 remaining fixes instead of 6
3. Verify all remaining fixes are still applicable

### Phase 6: Testing and Verification (1 hour)
1. Run `make test` - all unit tests pass
2. Run `make build` - binary compiles successfully
3. Run `make manifests` - CRD manifests generate correctly
4. Manual verification: Deploy updated CRDs to test cluster
5. Verify existing NodeGroups without cloud-init fields work correctly

## Breaking Changes and Migration

### For Existing Users

**If you have NodeGroups using cloud-init fields:**

```yaml
# OLD (will be rejected after upgrade)
apiVersion: autoscaler.vpsie.com/v1alpha1
kind: NodeGroup
metadata:
  name: my-nodegroup
spec:
  userData: |
    #!/bin/bash
    kubeadm join ...
  # OR
  cloudInitTemplate: |
    #!/bin/bash
    ...
  # OR
  cloudInitTemplateRef:
    name: my-template
```

```yaml
# NEW (VPSie API handles configuration automatically)
apiVersion: autoscaler.vpsie.com/v1alpha1
kind: NodeGroup
metadata:
  name: my-nodegroup
spec:
  # Remove all cloud-init fields
  # VPSie API with QEMU agent configures nodes automatically
  # Based on osImageID and kubernetesVersion
```

**Migration Steps:**
1. Remove `userData`, `cloudInitTemplate`, `cloudInitTemplateRef`, and `cloudInitVariables` from all NodeGroup resources
2. Apply updated CRDs: `kubectl apply -f deploy/crds/`
3. Restart autoscaler controller to pick up new CRD version
4. Verify nodes provision correctly via VPSie API

## Acceptance Criteria

- [ ] No cloud-init fields in `nodegroup_types.go`
- [ ] `CloudInitTemplateRef` type removed
- [ ] Provisioner no longer accepts or uses cloud-init template
- [ ] `UserData` removed from CreateVPSRequest
- [ ] `deploy/examples/cloud-init/` directory deleted
- [ ] Documentation updated to reflect VPSie API configuration
- [ ] CRD manifests regenerated without cloud-init fields
- [ ] All unit tests pass
- [ ] Controller compiles successfully
- [ ] CHANGELOG includes breaking change notice
- [ ] Migration guide added for existing users

## Risk Mitigation

**Risk:** Existing NodeGroups using cloud-init fields will break

**Mitigation:**
1. Add prominent breaking change notice in CHANGELOG
2. Create migration guide with kubectl commands
3. Consider adding validation webhook to reject NodeGroups with cloud-init fields with helpful error message
4. Version bump to v0.6.0 to signal breaking change

**Risk:** VPSie API may not support all configuration that cloud-init did

**Mitigation:**
1. Document supported configuration via VPSie API
2. Identify any gaps and file feature requests with VPSie
3. Ensure KubernetesVersion field is properly passed to VPSie API

## Post-Implementation

1. Update critical fixes documentation (ADR, Design Doc) to remove template validation
2. Proceed with remaining 5 critical fixes
3. Test end-to-end node provisioning with VPSie API configuration
4. Update integration tests to verify QEMU agent configuration

---

## Approval Required

**Before proceeding, please confirm:**
1. Is this the correct understanding of VPSie API's QEMU agent configuration?
2. Should we keep `userData` field as deprecated but non-functional for backward compatibility, or remove completely?
3. What Kubernetes version should we bump to for this breaking change? (suggest v0.6.0)
4. Are there any VPSie API configuration options we should add to replace cloud-init functionality?

