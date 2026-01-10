# Task: Phase 3a - Provisioner Integration

Metadata:
- Phase: 3 (Controller Integration)
- Dependencies: Task 02 (Discoverer Core), Task 03 (Discoverer Tests)
- Provides: Discovery integrated into provisioning flow
- Size: Medium (2 files)
- Verification Level: L2 (Test Operation)

## Implementation Content

Integrate the Discoverer component into the Provisioner and Controller. This enables automatic VPS ID discovery when async provisioning returns ID=0.

Key changes:
1. Add `discoverer` field to Provisioner struct
2. Add `SetDiscoverer()` method for dependency injection
3. Update `createVPS()` to call discovery when creation-requested=true and VPSieInstanceID=0
4. Update controller to create and inject Discoverer

## Target Files

- [ ] `pkg/controller/vpsienode/provisioner.go` (MODIFY: add discoverer field and discovery logic)
- [ ] `pkg/controller/vpsienode/controller.go` (MODIFY: create and inject discoverer)

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase

- [ ] Write test for discovery integration:
  ```go
  func TestProvisioner_CreateVPS_WithDiscovery(t *testing.T) {
      // VPSieNode with creation-requested=true, VPSieInstanceID=0
      // Mock returns VPS with ID
      // Should update VPSieNode with discovered ID
  }
  ```
- [ ] Run test - expect failure (discoverer not integrated)

### 2. Green Phase

#### 2.1 Update Provisioner struct

Add field to Provisioner:
```go
type Provisioner struct {
    vpsieClient VPSieClientInterface
    sshKeyIDs   []string
    discoverer  *Discoverer  // NEW: Add discoverer reference
}
```

#### 2.2 Add SetDiscoverer method

```go
// SetDiscoverer sets the discoverer for async VPS ID discovery
func (p *Provisioner) SetDiscoverer(d *Discoverer) {
    p.discoverer = d
}
```

#### 2.3 Update createVPS method

Modify the `createVPS()` method (around line 48) to add discovery logic:

```go
// createVPS creates a new VPS instance via VPSie Kubernetes API
func (p *Provisioner) createVPS(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
    // Check if creation was already requested (to avoid duplicate API calls)
    if vn.Annotations != nil && vn.Annotations[AnnotationCreationRequested] == "true" {
        logger.Info("Node creation already requested, attempting discovery",
            zap.String("vpsienode", vn.Name),
        )

        // Attempt to discover the VPS ID
        if p.discoverer != nil {
            vps, timedOut, err := p.discoverer.DiscoverVPSID(ctx, vn)
            if err != nil {
                logger.Warn("VPS discovery failed", zap.Error(err))
                // Continue - will retry on next reconcile
            } else if timedOut {
                // Discovery timeout - mark as failed
                logger.Error("VPS discovery timeout exceeded",
                    zap.String("vpsienode", vn.Name),
                )
                return ctrl.Result{}, fmt.Errorf("VPS discovery timeout exceeded")
            } else if vps != nil {
                // Discovery successful - update VPSieNode
                logger.Info("VPS discovered successfully",
                    zap.Int("vpsID", vps.ID),
                    zap.String("hostname", vps.Hostname),
                    zap.String("ip", vps.IPAddress),
                )

                // Update VPSieNode with discovered VPS information
                vn.Spec.VPSieInstanceID = vps.ID
                vn.Spec.IPAddress = vps.IPAddress
                vn.Spec.IPv6Address = vps.IPv6Address
                vn.Status.Hostname = vps.Hostname
                vn.Status.VPSieStatus = vps.Status
                vn.Status.Resources = v1alpha1.NodeResources{
                    CPU:      vps.CPU,
                    MemoryMB: vps.RAM,
                    DiskGB:   vps.Disk,
                }

                // Continue with normal provisioning flow
                return p.checkVPSStatus(ctx, vn, logger)
            }
        }

        // VPS not discovered yet, keep waiting
        return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
    }

    // ... rest of existing createVPS logic (unchanged) ...
}
```

#### 2.4 Update Controller

Update `NewVPSieNodeReconciler` to create and inject Discoverer:

```go
func NewVPSieNodeReconciler(
    client client.Client,
    scheme *runtime.Scheme,
    vpsieClient VPSieClientInterface,
    logger *zap.Logger,
    sshKeyIDs []string,
) *VPSieNodeReconciler {
    provisioner := NewProvisioner(vpsieClient, sshKeyIDs)
    joiner := NewJoiner(client, provisioner)
    drainer := NewDrainer(client)
    terminator := NewTerminator(drainer, provisioner)
    stateMachine := NewStateMachine(provisioner, joiner, terminator)

    // NEW: Create and inject discoverer
    discoverer := NewDiscoverer(vpsieClient, client, logger)
    provisioner.SetDiscoverer(discoverer)

    return &VPSieNodeReconciler{
        Client:       client,
        Scheme:       scheme,
        VPSieClient:  vpsieClient,
        Logger:       logger.Named(ControllerName),
        stateMachine: stateMachine,
        provisioner:  provisioner,
        joiner:       joiner,
        drainer:      drainer,
        terminator:   terminator,
    }
}
```

- [ ] Run tests - confirm pass

### 3. Refactor Phase

- [ ] Ensure error messages are descriptive for debugging
- [ ] Add debug logging for discovery attempts
- [ ] Run `make lint` and `make fmt`
- [ ] Run full test suite: `make test`

## Completion Criteria

- [ ] Provisioner has `discoverer` field and `SetDiscoverer()` method
- [ ] `createVPS()` calls `DiscoverVPSID()` when appropriate:
  - `creation-requested=true` annotation present
  - `VPSieInstanceID=0`
- [ ] VPSieNode spec/status updated on successful discovery:
  - `Spec.VPSieInstanceID` = discovered VPS ID
  - `Spec.IPAddress` = VPS IP
  - `Spec.IPv6Address` = VPS IPv6
  - `Status.Hostname` = VPS hostname
  - `Status.VPSieStatus` = VPS status
  - `Status.Resources` = VPS resources
- [ ] Discovery timeout returns error (causes Failed phase transition)
- [ ] Controller creates and injects Discoverer
- [ ] `make build` succeeds
- [ ] `make test` passes
- [ ] `make lint` passes

## Test Cases

### Provisioner Tests (add to provisioner_test.go)

```go
TestProvisioner_CreateVPS_WithDiscovery_Success
// VPSieNode: creation-requested=true, VPSieInstanceID=0
// Mock Discoverer returns VPS with ID 12345
// Assert: VPSieNode.Spec.VPSieInstanceID = 12345

TestProvisioner_CreateVPS_WithDiscovery_Timeout
// VPSieNode: creation-requested=true, VPSieInstanceID=0, CreatedAt > 15min ago
// Mock Discoverer returns timedOut=true
// Assert: error returned (triggers Failed phase)

TestProvisioner_CreateVPS_WithDiscovery_NotFound
// VPSieNode: creation-requested=true, VPSieInstanceID=0
// Mock Discoverer returns nil (VPS not found yet)
// Assert: Requeue with FastRequeueAfter

TestProvisioner_CreateVPS_WithDiscovery_APIError
// VPSieNode: creation-requested=true, VPSieInstanceID=0
// Mock Discoverer returns error
// Assert: Requeue (will retry)

TestProvisioner_CreateVPS_NoDiscovererSet
// Provisioner without discoverer set
// VPSieNode: creation-requested=true, VPSieInstanceID=0
// Assert: Falls back to waiting (no panic)
```

## Operational Verification

```bash
# Verify build
make build

# Run provisioner tests
go test ./pkg/controller/vpsienode -run TestProvisioner -v

# Run all vpsienode controller tests
go test ./pkg/controller/vpsienode/... -v

# Full test suite
make test
```

## Acceptance Criteria Mapping

| Design Doc AC | Implementation |
|--------------|----------------|
| D2: VPSieNode.Spec.VPSieInstanceID updated | VPSieNode spec update in createVPS() |
| D3: VPSieNode.Spec.IPAddress updated | VPSieNode spec update in createVPS() |
| D6: Timeout -> Failed | Error returned triggers Failed phase in state machine |
| E2: Transient errors trigger retry | Discovery error -> requeue |
| E3: Timeout marks Failed | Discovery timeout returns error |

## Notes

- Impact scope: Modifies existing provisioning flow
- Discovery is only attempted when:
  - `creation-requested=true` annotation is present
  - `VPSieInstanceID=0` (not yet discovered)
- If discoverer is nil, falls back to existing wait behavior
- Error from discovery causes requeue (transient error handling)
- Timeout from discovery returns error (triggers Failed phase transition)
