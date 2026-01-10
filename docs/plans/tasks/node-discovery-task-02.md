# Task: Phase 2a - Discoverer Component Core

Metadata:
- Phase: 2 (Discovery Core)
- Dependencies: Task 01 (Interface Extension)
- Provides: `pkg/controller/vpsienode/discoverer.go`
- Size: Small (1 file)
- Verification Level: L2 (Test Operation)

## Implementation Content

Create the core Discoverer component that handles VPS ID discovery and K8s node matching for async provisioning. This component will:

1. Discover VPS IDs after async provisioning by querying VPSie API
2. Match VPS to VPSieNode using hostname pattern (primary) and IP (fallback)
3. Handle discovery timeouts (15 minutes default)
4. Check if K8s nodes are already claimed by other VPSieNodes

## Target Files

- [ ] `pkg/controller/vpsienode/discoverer.go` (NEW)

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase

- [ ] Create empty `discoverer.go` file with package declaration
- [ ] Write stub interfaces/signatures based on Design Doc Section 9.1
- [ ] Create minimal test file `discoverer_test.go` with one failing test:
  ```go
  func TestDiscoverVPSID_Success_ByHostname(t *testing.T) {
      // Will fail - DiscoverVPSID not implemented
  }
  ```
- [ ] Run test - confirm failure

### 2. Green Phase

- [ ] Implement `Discoverer` struct with dependencies:
  ```go
  type Discoverer struct {
      vpsieClient VPSieClientInterface
      k8sClient   client.Client
      logger      *zap.Logger
  }
  ```
- [ ] Implement `NewDiscoverer()` constructor
- [ ] Implement `DiscoverVPSID()` method:
  - Check timeout using `vn.Status.CreatedAt`
  - List all VMs via VPSie API
  - Filter candidates (running status)
  - Sort by creation time (newest first)
  - Try hostname pattern matching first
  - Try IP-based K8s node matching as fallback
- [ ] Implement `matchesHostnamePattern()` helper function
- [ ] Implement `findK8sNodeByIP()` method
- [ ] Implement `isNodeClaimedByOther()` method
- [ ] Run test - confirm pass

### 3. Refactor Phase

- [ ] Add comprehensive logging with context (vpsienode name, cluster, groupID)
- [ ] Ensure error handling follows Design Doc Section 11
- [ ] Add godoc comments for all exported functions
- [ ] Run `make lint` and `make fmt`

## Code Specifications

### Constants

```go
// DiscoveryTimeout is the maximum time to wait for VPS discovery
const DiscoveryTimeout = 15 * time.Minute
```

### Discoverer Struct

```go
// Discoverer handles VPS and K8s node discovery for async provisioning
type Discoverer struct {
    vpsieClient VPSieClientInterface
    k8sClient   client.Client
    logger      *zap.Logger
}
```

### NewDiscoverer Constructor

```go
// NewDiscoverer creates a new Discoverer
func NewDiscoverer(vpsieClient VPSieClientInterface, k8sClient client.Client, logger *zap.Logger) *Discoverer {
    return &Discoverer{
        vpsieClient: vpsieClient,
        k8sClient:   k8sClient,
        logger:      logger.Named("discoverer"),
    }
}
```

### DiscoverVPSID Method

```go
// DiscoverVPSID attempts to discover the VPS ID for a VPSieNode
// that was created via async provisioning (creation-requested=true, VPSieInstanceID=0)
//
// Strategy:
// 1. List all VMs from VPSie API
// 2. Filter to running VMs
// 3. Sort by creation time (newest first)
// 4. Match by hostname pattern (VPSieNode name as prefix)
// 5. Fallback: Match by IP if K8s node exists with matching IP
//
// Returns:
//   - *vpsieclient.VPS: Discovered VPS (nil if not found)
//   - bool: Whether discovery timed out
//   - error: API errors
func (d *Discoverer) DiscoverVPSID(ctx context.Context, vn *v1alpha1.VPSieNode) (*vpsieclient.VPS, bool, error)
```

### matchesHostnamePattern Helper

```go
// matchesHostnamePattern checks if the VPS hostname matches the VPSieNode name pattern
// VPSie typically appends a suffix to the node name when creating nodes
// e.g., VPSieNode "my-nodegroup-abc123" might create VPS "my-nodegroup-abc123-k8s-worker"
func matchesHostnamePattern(vpsieNodeName, vpsHostname string) bool
```

### findK8sNodeByIP Method

```go
// findK8sNodeByIP finds a Kubernetes node by its IP address
func (d *Discoverer) findK8sNodeByIP(ctx context.Context, ip string) (*corev1.Node, error)
```

### isNodeClaimedByOther Method

```go
// isNodeClaimedByOther checks if a K8s node is already associated with another VPSieNode
// by checking the autoscaler.vpsie.com/vpsienode label
func (d *Discoverer) isNodeClaimedByOther(ctx context.Context, node *corev1.Node, currentVN *v1alpha1.VPSieNode) bool
```

## Completion Criteria

- [ ] `Discoverer` struct created with all required fields
- [ ] `NewDiscoverer()` constructor implemented
- [ ] `DiscoverVPSID()` method implemented with:
  - [ ] Timeout checking (15 minute default)
  - [ ] VPS listing via API
  - [ ] Candidate filtering (running status)
  - [ ] Hostname pattern matching
  - [ ] IP-based K8s node matching fallback
- [ ] `matchesHostnamePattern()` helper implemented
- [ ] `findK8sNodeByIP()` method implemented
- [ ] `isNodeClaimedByOther()` method implemented
- [ ] Code compiles: `go build ./pkg/controller/vpsienode/...`
- [ ] Basic test passes (at least one test case)

## Acceptance Criteria Mapping

| Design Doc AC | Implementation |
|--------------|----------------|
| D1: VPS ID discovered within 15 minutes | Timeout check in DiscoverVPSID() |
| D4: Discovery uses hostname pattern matching | matchesHostnamePattern() |
| D5: Discovery uses IP address as fallback | findK8sNodeByIP() in DiscoverVPSID() |
| D6: VPSieNode transitions to Failed on timeout | Returns timedOut=true |
| D9: Discovery works for concurrent VPSieNodes | isNodeClaimedByOther() |

## Notes

- Impact scope: New file only - no modifications to existing code
- The Discoverer is designed to be injected into Provisioner (Task 04)
- Uses controller-runtime client for K8s API access
- Uses existing VPSieClientInterface for VPSie API access
- Thread-safe for concurrent reconciles (no shared mutable state)
