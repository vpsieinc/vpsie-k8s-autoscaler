# Task: Phase 2b - Discoverer Unit Tests

Metadata:
- Phase: 2 (Discovery Core)
- Dependencies: Task 02 (Discoverer Core)
- Provides: Comprehensive unit test coverage for Discoverer
- Size: Small (1 file)
- Verification Level: L2 (Test Operation)

## Implementation Content

Create comprehensive unit tests for the Discoverer component. Tests should cover success scenarios, failure scenarios, edge cases, and concurrency handling as specified in Design Doc Section 12.

## Target Files

- [ ] `pkg/controller/vpsienode/discoverer_test.go` (NEW)

## Implementation Steps (TDD: Red-Green-Refactor)

### 1. Red Phase

- [ ] Create test file structure with table-driven tests
- [ ] Write all test case stubs from Work Plan Section Phase 2
- [ ] Run tests - confirm all fail (except those already passing from Task 02)

### 2. Green Phase

- [ ] Implement each test case:
  - Discovery success scenarios (hostname, IP)
  - Discovery failure scenarios (timeout, no candidates, API error)
  - Helper function tests (hostname pattern, K8s node by IP)
  - Claim checking tests
- [ ] Use MockVPSieClient for VPSie API mocking
- [ ] Use fake.NewClientBuilder() for K8s API mocking
- [ ] Run tests - all pass

### 3. Refactor Phase

- [ ] Extract common test setup to helper functions
- [ ] Ensure test names follow naming convention
- [ ] Add test documentation
- [ ] Run with race detector: `go test -race ./pkg/controller/vpsienode/... -run TestDiscover`

## Test Cases (from Work Plan)

### Discovery Success Scenarios

```go
TestDiscoverVPSID_Success_ByHostname
// VPSieNode "my-node" matches VPS hostname "my-node-k8s-worker"
// Should return the VPS with correct ID

TestDiscoverVPSID_Success_ByIP
// VPSieNode has no hostname match but K8s node exists with matching IP
// Should return the VPS

TestDiscoverVPSID_MultipleCandidates_SelectsNewest
// Multiple VPSs match, should select most recently created
// Verifies sorting by creation time
```

### Discovery Failure Scenarios

```go
TestDiscoverVPSID_Timeout
// VPSieNode.Status.CreatedAt > 15 minutes ago
// Should return timedOut=true

TestDiscoverVPSID_NoCandidates
// No running VPSs in API response
// Should return nil, false, nil

TestDiscoverVPSID_APIError
// VPSie API returns error
// Should propagate error

TestDiscoverVPSID_SkipsNonRunningVMs
// VPSs with status != "running" should be skipped
```

### Hostname Pattern Tests

```go
TestMatchesHostnamePattern_ExactPrefix
// "my-node" matches "my-node-k8s-worker"

TestMatchesHostnamePattern_NoMatch
// "my-node" does NOT match "other-node"

TestMatchesHostnamePattern_ShorterHostname
// "my-long-node-name" does NOT match "my-long"
// Hostname must be >= VPSieNode name length
```

### K8s Node Matching Tests

```go
TestFindK8sNodeByIP_Found
// K8s node exists with matching InternalIP
// Should return the node

TestFindK8sNodeByIP_NotFound
// No K8s node has the IP
// Should return nil, nil

TestFindK8sNodeByIP_MatchesExternalIP
// K8s node with matching ExternalIP
// Should return the node
```

### Claim Checking Tests

```go
TestIsNodeClaimedByOther_Claimed
// Node has label autoscaler.vpsie.com/vpsienode=other-vn
// Should return true

TestIsNodeClaimedByOther_NotClaimed
// Node has no VPSieNode label
// Should return false

TestIsNodeClaimedByOther_SameVPSieNode
// Node has label pointing to current VPSieNode
// Should return false (same node, not "other")
```

## Test Setup Helpers

```go
// newTestDiscoverer creates a Discoverer with mock dependencies
func newTestDiscoverer(t *testing.T, mockVPSie *MockVPSieClient, k8sObjs ...client.Object) *Discoverer

// newTestVPSieNode creates a VPSieNode for testing with common defaults
func newTestVPSieNode(name string, opts ...vpsieNodeOption) *v1alpha1.VPSieNode

// newTestK8sNode creates a K8s Node for testing
func newTestK8sNode(name, ip string) *corev1.Node

// vpsieNodeOption allows customizing test VPSieNodes
type vpsieNodeOption func(*v1alpha1.VPSieNode)

func withCreatedAt(t time.Time) vpsieNodeOption
func withAnnotation(key, value string) vpsieNodeOption
func withVPSieGroupID(id int) vpsieNodeOption
```

## Test Code Pattern

```go
func TestDiscoverVPSID_Success_ByHostname(t *testing.T) {
    // Arrange
    mockClient := NewMockVPSieClient()
    mockClient.ListVMsFunc = func(ctx context.Context) ([]vpsieclient.VPS, error) {
        return []vpsieclient.VPS{
            {
                ID:        12345,
                Hostname:  "my-node-k8s-worker",
                IPAddress: "10.0.0.5",
                Status:    "running",
                CreatedAt: time.Now(),
            },
        }, nil
    }

    k8sClient := fake.NewClientBuilder().Build()
    discoverer := NewDiscoverer(mockClient, k8sClient, zap.NewNop())

    vn := &v1alpha1.VPSieNode{
        ObjectMeta: metav1.ObjectMeta{
            Name: "my-node",
        },
        Status: v1alpha1.VPSieNodeStatus{
            CreatedAt: &metav1.Time{Time: time.Now()},
        },
    }

    // Act
    vps, timedOut, err := discoverer.DiscoverVPSID(context.Background(), vn)

    // Assert
    assert.NoError(t, err)
    assert.False(t, timedOut)
    assert.NotNil(t, vps)
    assert.Equal(t, 12345, vps.ID)
    assert.Equal(t, "my-node-k8s-worker", vps.Hostname)
}
```

## Completion Criteria

- [ ] All 15 test cases from Work Plan implemented
- [ ] Tests use table-driven pattern where appropriate
- [ ] Each test follows AAA pattern (Arrange-Act-Assert)
- [ ] Test helpers extracted for common setup
- [ ] All tests pass: `go test ./pkg/controller/vpsienode/... -run TestDiscover -v`
- [ ] Tests pass with race detector: `go test -race ./pkg/controller/vpsienode/... -run TestDiscover`
- [ ] Test coverage >= 85% for discoverer.go

## Coverage Verification

```bash
# Run tests with coverage
go test ./pkg/controller/vpsienode/... -coverprofile=coverage.out -run TestDiscover -v

# Check coverage for discoverer.go
go tool cover -func=coverage.out | grep discoverer

# Target: >= 85% coverage
```

## Acceptance Criteria Mapping

| Design Doc AC | Test Cases |
|--------------|------------|
| D1: VPS ID discovered within 15 minutes | TestDiscoverVPSID_Timeout |
| D4: Hostname pattern matching | TestMatchesHostnamePattern_* |
| D5: IP address fallback | TestFindK8sNodeByIP_*, TestDiscoverVPSID_Success_ByIP |
| D6: Timeout -> Failed | TestDiscoverVPSID_Timeout |
| D9: Concurrent VPSieNodes | TestIsNodeClaimedByOther_* |

## Notes

- Use `fake.NewClientBuilder()` from controller-runtime for K8s API mocking
- Use existing `MockVPSieClient` for VPSie API mocking
- Tests should be independent and not share state
- Each test should complete in < 100ms (unit test performance target)
