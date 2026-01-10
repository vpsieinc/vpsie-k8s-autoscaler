# Task: Phase 1 - Interface Extension

Metadata:
- Phase: 1 (Foundation)
- Dependencies: None
- Provides: Extended VPSieClientInterface and MockVPSieClient
- Size: Small (2 files)
- Verification Level: L3 (Build Success)

## Implementation Content

Extend the `VPSieClientInterface` to support listing K8s node groups for VPS discovery. This is the foundational change required before implementing the Discoverer component.

The `ListK8sNodeGroups()` method already exists in `pkg/vpsie/client/client.go` but is not exposed through the controller's interface abstraction.

## Target Files

- [ ] `pkg/controller/vpsienode/vpsie_interface.go` (MODIFY: add method signature)
- [ ] `pkg/controller/vpsienode/mock_vpsie_client.go` (MODIFY: add mock implementation)

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase

- [ ] Add `ListK8sNodeGroups()` method signature to `VPSieClientInterface`
- [ ] Run `make build` - expect compilation failure (MockVPSieClient doesn't implement new method)
- [ ] Confirm the error message indicates missing method implementation

### 2. Green Phase

- [ ] Add `ListK8sNodeGroupsFunc` field to `MockVPSieClient` struct:
  ```go
  // ListK8sNodeGroupsFunc allows custom behavior for ListK8sNodeGroups
  ListK8sNodeGroupsFunc func(ctx context.Context, clusterIdentifier string) ([]vpsieclient.K8sNodeGroup, error)
  ```
- [ ] Implement `ListK8sNodeGroups()` method on `MockVPSieClient`:
  ```go
  func (m *MockVPSieClient) ListK8sNodeGroups(ctx context.Context, clusterIdentifier string) ([]vpsieclient.K8sNodeGroup, error) {
      m.mu.Lock()
      defer m.mu.Unlock()
      m.CallCounts["ListK8sNodeGroups"]++
      if m.ListK8sNodeGroupsFunc != nil {
          return m.ListK8sNodeGroupsFunc(ctx, clusterIdentifier)
      }
      return nil, nil
  }
  ```
- [ ] Run `make build` - expect success

### 3. Refactor Phase

- [ ] Verify interface documentation comments are clear
- [ ] Ensure mock follows existing patterns (thread-safe, call counting)
- [ ] Run `make lint` and `make fmt` - fix any issues

## Code Specifications

### Interface Extension (vpsie_interface.go)

Add to `VPSieClientInterface`:

```go
type VPSieClientInterface interface {
    // ... existing methods ...

    // ListK8sNodeGroups lists all node groups for a VPSie managed Kubernetes cluster
    // Returns the node groups with their numeric IDs and node counts
    // Used by Discoverer to find VPS nodes created via async provisioning
    ListK8sNodeGroups(ctx context.Context, clusterIdentifier string) ([]vpsieclient.K8sNodeGroup, error)
}
```

### Mock Implementation (mock_vpsie_client.go)

Add field to `MockVPSieClient` struct:

```go
// ListK8sNodeGroupsFunc allows custom behavior for ListK8sNodeGroups
ListK8sNodeGroupsFunc func(ctx context.Context, clusterIdentifier string) ([]vpsieclient.K8sNodeGroup, error)
```

Add method to `MockVPSieClient`:

```go
// ListK8sNodeGroups mocks listing K8s node groups
func (m *MockVPSieClient) ListK8sNodeGroups(ctx context.Context, clusterIdentifier string) ([]vpsieclient.K8sNodeGroup, error) {
    m.mu.Lock()
    defer m.mu.Unlock()

    m.CallCounts["ListK8sNodeGroups"]++

    // Use custom function if provided
    if m.ListK8sNodeGroupsFunc != nil {
        return m.ListK8sNodeGroupsFunc(ctx, clusterIdentifier)
    }

    // Default: return empty list
    return nil, nil
}
```

## Completion Criteria

- [ ] `VPSieClientInterface` includes `ListK8sNodeGroups()` method signature
- [ ] `MockVPSieClient` implements `ListK8sNodeGroups()` with function hook support
- [ ] Interface compliance verified: `var _ VPSieClientInterface = (*vpsieclient.Client)(nil)` compiles
- [ ] `make build` succeeds
- [ ] `make lint` passes
- [ ] `make test` passes (existing tests still work)

## Operational Verification Commands

```bash
# Verify build succeeds
make build

# Verify lint passes
make lint

# Verify existing tests still pass
make test

# Verify interface compliance (should be implicit via existing var _ line)
go build ./pkg/controller/vpsienode/...
```

## Acceptance Criteria Mapping

| Design Doc AC | Coverage |
|--------------|----------|
| D8: Max 1 API call per VPSieNode per reconcile | Interface supports batch query via ListK8sNodeGroups |

## Notes

- Impact scope: Interface layer only - no behavioral changes
- The actual `ListK8sNodeGroups()` implementation exists in `pkg/vpsie/client/client.go` (line 1499)
- This task sets up the abstraction layer for the Discoverer to use
