# NodeGroup Isolation Design Document

## Overview

This design document specifies the implementation of NodeGroup isolation for the VPSie Kubernetes Autoscaler. The feature ensures the autoscaler only manages NodeGroups explicitly marked with a managed label (`autoscaler.vpsie.com/managed=true`), ignoring all external or manually-created NodeGroups. This provides multi-tenant safety and enables dynamic NodeGroup creation when no suitable managed NodeGroup exists for pending pods.

## Design Summary (Meta)

```yaml
design_type: "extension"
risk_level: "medium"
main_constraints:
  - "Clean slate migration - only NEW NodeGroups with label are managed"
  - "Must not break existing NodeGroups (they simply become unmanaged)"
  - "All operations must filter by managed label: scale-up, scale-down, rebalancing, status"
biggest_risks:
  - "Label not applied to NodeGroup CR itself, only to child VPSieNodes"
  - "Dynamic NodeGroup creation logic may select wrong NodeGroup type"
  - "Rebalancer uses different label selector (vpsie.io/nodegroup vs autoscaler.vpsie.com/nodegroup)"
unknowns:
  - "Dynamic NodeGroup naming strategy for auto-created groups"
  - "Template source for dynamically created NodeGroups"
```

## Background and Context

### Prerequisite ADRs

- None required - this is a filtering/isolation feature, not an architecture change

### Agreement Checklist

#### Scope
- [x] All autoscaler operations filter by managed label: scale-up, scale-down, rebalancing, status updates
- [x] NodeGroup CR itself receives the managed label (not just child VPSieNodes)
- [x] Dynamic NodeGroup creation when no suitable managed NodeGroup exists for pending pods
- [x] Label filtering at controller, scaler, rebalancer, and event watcher layers

#### Non-Scope (Explicitly not changing)
- [x] Existing NodeGroups without the label continue to exist but are ignored
- [x] No migration tooling - clean slate approach
- [x] No changes to VPSie API integration
- [x] No changes to node draining, provisioning, or termination logic

#### Constraints
- [x] Parallel operation: Yes - managed and unmanaged NodeGroups can coexist
- [x] Backward compatibility: No - existing NodeGroups become unmanaged by design
- [x] Performance measurement: Not required - minimal overhead from label selector filtering

### Problem to Solve

The current autoscaler manages ALL NodeGroup CRs in the cluster, regardless of their origin. This creates problems in multi-tenant or shared clusters where:

1. Multiple teams may have their own NodeGroups that should not be managed by the autoscaler
2. Manually-created NodeGroups for testing or special purposes get unexpectedly scaled
3. No clear ownership boundary between autoscaler-managed and external resources

### Current Challenges

1. **Label Mismatch**: The label `autoscaler.vpsie.com/managed=true` exists in `GetNodeGroupLabels()` but is only applied to VPSieNode CRs, not to the parent NodeGroup CR itself
2. **Inconsistent Label Selectors**: The rebalancer uses `vpsie.io/nodegroup` while the scaler uses `autoscaler.vpsie.com/nodegroup`
3. **No Filtering**: The EventWatcher's `GetNodeGroups()` returns ALL NodeGroups without filtering
4. **No Dynamic Creation**: If no suitable managed NodeGroup exists for a pending pod, the pod remains unscheduled

### Requirements

#### Functional Requirements

- FR1: Autoscaler must filter NodeGroups by `autoscaler.vpsie.com/managed=true` label
- FR2: All operations (scale-up, scale-down, rebalancing, status) must respect the filter
- FR3: System must dynamically create a managed NodeGroup when no suitable one exists for pending pods
- FR4: Matching criteria for "suitable" NodeGroup: compatible KubeSizeID, OfferingIDs, datacenter

#### Non-Functional Requirements

- **Performance**: Label selector filtering adds negligible overhead (native Kubernetes indexing)
- **Scalability**: Works correctly with hundreds of NodeGroups (filtered at API level)
- **Reliability**: No change to existing reliability characteristics
- **Maintainability**: Centralized filtering logic, single point of label definition

## Acceptance Criteria (AC) - EARS Format

### FR1: NodeGroup Label Filtering

- [ ] **When** the NodeGroup controller reconciles, the system shall only process NodeGroups with label `autoscaler.vpsie.com/managed=true`
- [ ] **When** listing NodeGroups for any operation, the system shall apply the managed label selector
- [ ] **If** a NodeGroup lacks the managed label, **then** the system shall skip it without error

### FR2: All Operations Filter by Label

- [ ] **When** ScaleDownManager identifies underutilized nodes, the system shall only consider nodes from managed NodeGroups
- [ ] **When** EventWatcher detects pending pods, the system shall only match against managed NodeGroups
- [ ] **When** Rebalancer analyzes optimization opportunities, the system shall only analyze managed NodeGroups
- [ ] **While** status is being updated, the system shall only update managed NodeGroups

### FR3: Dynamic NodeGroup Creation

- [ ] **When** pending pods cannot be scheduled and no suitable managed NodeGroup exists, the system shall create a new managed NodeGroup
- [ ] **When** creating a dynamic NodeGroup, the system shall apply label `autoscaler.vpsie.com/managed=true`
- [ ] **If** a suitable managed NodeGroup already exists, **then** the system shall use it instead of creating a new one

### FR4: NodeGroup Suitability Matching

- [ ] The system shall consider a NodeGroup suitable if it matches the pod's resource requirements
- [ ] The system shall consider a NodeGroup suitable if its datacenter matches the cluster's primary datacenter
- [ ] **When** multiple suitable NodeGroups exist, the system shall prefer NodeGroups with available capacity

## Existing Codebase Analysis

### Implementation Path Mapping

| Type | Path | Description |
|------|------|-------------|
| Existing | `pkg/controller/nodegroup/status.go:199-204` | `GetNodeGroupLabels()` - applies managed label to VPSieNodes only |
| Existing | `pkg/controller/nodegroup/controller.go:127-142` | `secretToNodeGroups()` - lists ALL NodeGroups without filter |
| Existing | `pkg/scaler/scaler.go:458-477` | `getNodeGroupNodes()` - filters nodes by `autoscaler.vpsie.com/nodegroup` |
| Existing | `pkg/events/watcher.go:428-436` | `GetNodeGroups()` - returns ALL NodeGroups without filter |
| Existing | `pkg/events/analyzer.go:119-140` | `FindMatchingNodeGroups()` - no managed filter |
| Existing | `pkg/rebalancer/analyzer.go:421-471` | `getNodeGroupNodes()` - uses `vpsie.io/nodegroup` label |
| New | `pkg/controller/nodegroup/filter.go` | Centralized filtering utilities |
| New | `pkg/events/creator.go` | Dynamic NodeGroup creation logic |

### Similar Functionality Search Results

**Found Pattern**: The `GetNodeGroupLabels()` function at `pkg/controller/nodegroup/status.go:199-204` already defines the managed label:

```go
func GetNodeGroupLabels(ng *v1alpha1.NodeGroup) map[string]string {
    labels := make(map[string]string)
    labels[GetNodeGroupNameLabel()] = ng.Name
    labels["autoscaler.vpsie.com/managed"] = "true"
    return labels
}
```

**Decision**: Extend this existing pattern. The label value and key are already correct, but they need to be:
1. Applied to the NodeGroup CR itself (not just VPSieNodes)
2. Used as a filter selector in all listing operations

### Integration Points (Include even for new implementations)

| Component | Integration Target | Impact Level |
|-----------|-------------------|--------------|
| NodeGroup Controller | Reconciler entry point | High - controls all processing |
| ScaleDownManager | `getNodeGroupNodes()` | Medium - already filters by nodegroup name |
| EventWatcher | `GetNodeGroups()` | High - affects scale-up decisions |
| ResourceAnalyzer | `FindMatchingNodeGroups()` | High - affects NodeGroup selection |
| Rebalancer Analyzer | `getNodeGroupNodes()` | Medium - uses different label namespace |

## Design

### Change Impact Map

```yaml
Change Target: NodeGroup filtering across all components
Direct Impact:
  - pkg/controller/nodegroup/controller.go (add label selector to List calls)
  - pkg/controller/nodegroup/status.go (add helper for managed label check)
  - pkg/events/watcher.go (filter GetNodeGroups by managed label)
  - pkg/events/analyzer.go (filter in FindMatchingNodeGroups)
  - pkg/events/scaleup.go (add dynamic NodeGroup creation)
  - pkg/rebalancer/analyzer.go (normalize label selector)
Indirect Impact:
  - Existing NodeGroups become unmanaged (desired behavior)
  - Metrics may change due to reduced scope
No Ripple Effect:
  - VPSie API client
  - Node provisioning/termination logic
  - Drain operations
  - Webhook validators
```

### Architecture Overview

```
                    ┌─────────────────────────────────────────────┐
                    │              Kubernetes API                  │
                    │  (NodeGroup CRs with managed label filter)  │
                    └──────────────────┬──────────────────────────┘
                                       │
                    ┌──────────────────┴──────────────────────────┐
                    │         Label Selector Filter               │
                    │    autoscaler.vpsie.com/managed=true        │
                    └──────────────────┬──────────────────────────┘
                                       │
        ┌──────────────────┬───────────┴───────────┬──────────────────┐
        │                  │                       │                  │
        ▼                  ▼                       ▼                  ▼
┌───────────────┐  ┌───────────────┐     ┌───────────────┐  ┌───────────────┐
│   NodeGroup   │  │ ScaleDown     │     │  EventWatcher │  │  Rebalancer   │
│  Controller   │  │   Manager     │     │ (Scale-Up)    │  │   Analyzer    │
└───────────────┘  └───────────────┘     └───────────────┘  └───────────────┘
                                                │
                                                ▼
                                         ┌────────────────┐
                                         │ Dynamic NG     │
                                         │ Creator        │
                                         │ (when needed)  │
                                         └────────────────┘
```

### Data Flow

```
1. Scale-Up Flow (with Dynamic NodeGroup Creation):

   Pending Pod Event
         │
         ▼
   EventWatcher.handleEvent()
         │
         ▼
   GetNodeGroups() ──filter──> Only managed NodeGroups
         │
         ▼
   FindMatchingNodeGroups()
         │
    ┌────┴────┐
    │ Match?  │
    └────┬────┘
    Yes  │   No
    ┌────┘   └────┐
    ▼            ▼
   Scale      DynamicNodeGroupCreator
   Existing   │
   NG         ▼
             Create new NodeGroup with:
             - autoscaler.vpsie.com/managed=true label
             - Derived spec from default template
             │
             ▼
             Scale new NodeGroup

2. Scale-Down Flow:

   ScaleDownManager.IdentifyUnderutilizedNodes()
         │
         ▼
   getNodeGroupNodes() ──filter──> Nodes from managed NodeGroups only
         │
         ▼
   Continue with standard scale-down logic

3. Rebalancer Flow:

   Analyzer.AnalyzeRebalanceOpportunities()
         │
         ▼
   Verify NodeGroup has managed label
         │
         ▼
   getNodeGroupNodes() ──filter──> Nodes with autoscaler.vpsie.com/nodegroup
         │
         ▼
   Continue with standard rebalance logic
```

### Integration Points List

| Integration Point | Location | Old Implementation | New Implementation | Switching Method |
|-------------------|----------|-------------------|-------------------|------------------|
| NodeGroup List | `controller.go:127` | `r.List(ctx, &nodeGroupList)` | `r.List(ctx, &nodeGroupList, client.MatchingLabels{ManagedLabel: "true"})` | Direct code change |
| GetNodeGroups | `watcher.go:429` | `w.client.List(ctx, nodeGroupList)` | Add `client.MatchingLabels` filter | Direct code change |
| FindMatchingNodeGroups | `analyzer.go:121` | Iterate all NodeGroups | Add managed label check in loop | Direct code change |
| Rebalancer nodes | `analyzer.go:423` | `vpsie.io/nodegroup` selector | `autoscaler.vpsie.com/nodegroup` selector | Direct code change |

### Main Components

#### Component 1: Centralized Label Constants

- **Responsibility**: Define label keys and values in a single location
- **Interface**: Exported constants for use across packages
- **Dependencies**: None

```go
// pkg/controller/nodegroup/labels.go
const (
    // ManagedLabelKey is the label key indicating a NodeGroup is managed by the autoscaler
    ManagedLabelKey = "autoscaler.vpsie.com/managed"

    // ManagedLabelValue is the value indicating the NodeGroup is managed
    ManagedLabelValue = "true"

    // NodeGroupNameLabelKey is the label key for the NodeGroup name
    NodeGroupNameLabelKey = "autoscaler.vpsie.com/nodegroup"
)

// IsManagedNodeGroup checks if a NodeGroup has the managed label
func IsManagedNodeGroup(ng *v1alpha1.NodeGroup) bool {
    if ng.Labels == nil {
        return false
    }
    return ng.Labels[ManagedLabelKey] == ManagedLabelValue
}

// ManagedLabelSelector returns the label selector for managed NodeGroups
func ManagedLabelSelector() client.MatchingLabels {
    return client.MatchingLabels{ManagedLabelKey: ManagedLabelValue}
}
```

#### Component 2: Dynamic NodeGroup Creator

- **Responsibility**: Create new managed NodeGroups when no suitable one exists
- **Interface**: `CreateNodeGroupForPod(ctx, pod, template) (*NodeGroup, error)`
- **Dependencies**: Kubernetes client, NodeGroup template/defaults

```go
// pkg/events/creator.go
type DynamicNodeGroupCreator struct {
    client          client.Client
    defaultTemplate *v1alpha1.NodeGroupSpec
    logger          *zap.Logger
}

// CreateNodeGroupForPod creates a new managed NodeGroup suitable for the given pod
func (c *DynamicNodeGroupCreator) CreateNodeGroupForPod(
    ctx context.Context,
    pod *corev1.Pod,
    namespace string,
) (*v1alpha1.NodeGroup, error)
```

### Contract Definitions

```go
// ManagedNodeGroupFilter is the interface for filtering managed NodeGroups
type ManagedNodeGroupFilter interface {
    // FilterManaged returns only NodeGroups with the managed label
    FilterManaged(nodeGroups []v1alpha1.NodeGroup) []v1alpha1.NodeGroup

    // IsManagedNodeGroup checks if a single NodeGroup is managed
    IsManagedNodeGroup(ng *v1alpha1.NodeGroup) bool
}

// DynamicCreator is the interface for creating NodeGroups dynamically
type DynamicCreator interface {
    // CreateNodeGroupForPod creates a new managed NodeGroup for a pending pod
    CreateNodeGroupForPod(ctx context.Context, pod *corev1.Pod, namespace string) (*v1alpha1.NodeGroup, error)

    // FindSuitableNodeGroup finds an existing managed NodeGroup suitable for the pod
    FindSuitableNodeGroup(ctx context.Context, pod *corev1.Pod, nodeGroups []v1alpha1.NodeGroup) *v1alpha1.NodeGroup
}
```

### Data Contract

#### NodeGroup Label Contract

```yaml
Input:
  Type: NodeGroup CR
  Preconditions:
    - Must have valid ObjectMeta
    - Labels map may be nil or populated
  Validation: Check Labels[autoscaler.vpsie.com/managed] == "true"

Output:
  Type: bool (is managed)
  Guarantees: Deterministic, no side effects
  On Error: Return false (safe default - do not manage)

Invariants:
  - Label key and value are constants (not configurable)
  - Missing label means "not managed"
  - Label value must be exactly "true" (not "yes", "1", etc.)
```

#### Dynamic NodeGroup Creation Contract

```yaml
Input:
  Type: (context, *corev1.Pod, string namespace)
  Preconditions:
    - Pod has valid resource requests
    - Namespace exists
    - Default template is configured
  Validation: Pod must be in Pending phase with no node assigned

Output:
  Type: (*v1alpha1.NodeGroup, error)
  Guarantees:
    - Created NodeGroup has managed label
    - Created NodeGroup has unique name
    - Created NodeGroup has valid spec for the pod
  On Error: Return (nil, error) with descriptive message

Invariants:
  - Created NodeGroups always have autoscaler.vpsie.com/managed=true
  - Created NodeGroups have a predictable naming pattern
```

### Integration Boundary Contracts

```yaml
Boundary Name: NodeGroup Controller -> Kubernetes API
  Input: Label selector filter for List operations
  Output: Only NodeGroups with managed=true label (sync)
  On Error: Return empty list, log error, requeue

Boundary Name: EventWatcher -> ScaleUpController
  Input: Filtered list of managed NodeGroups
  Output: Scale-up decision or dynamic creation request (sync)
  On Error: Log and skip this reconcile cycle

Boundary Name: DynamicCreator -> Kubernetes API
  Input: NodeGroup CR with managed label
  Output: Created NodeGroup (sync)
  On Error: Return error, scale-up will retry on next cycle
```

### Error Handling

| Error Scenario | Handling Strategy |
|----------------|-------------------|
| No managed NodeGroups exist | Log warning, skip scale-up (or trigger dynamic creation) |
| Dynamic creation fails | Log error, return error to caller, retry on next cycle |
| Label selector returns empty | Treated as "no managed NodeGroups", not an error |
| NodeGroup has malformed labels | Treated as unmanaged, log debug message |

### Logging and Monitoring

**Log Events**:
- INFO: "Filtering NodeGroups by managed label, found N managed NodeGroups"
- INFO: "Creating dynamic NodeGroup for pending pod"
- WARN: "No managed NodeGroups available for scale-up"
- DEBUG: "Skipping unmanaged NodeGroup: {name}"

**Metrics**:
- `autoscaler_nodegroups_total{managed="true|false"}` - Count of managed vs unmanaged
- `autoscaler_dynamic_nodegroup_created_total` - Dynamic creations
- `autoscaler_nodegroup_filter_skipped_total` - NodeGroups skipped due to no label

## Implementation Plan

### Implementation Approach

**Selected Approach**: Vertical Slice (Feature-driven)

**Selection Reason**: Each component (controller, scaler, watcher, rebalancer) can be modified independently. Changes are isolated and can be verified with component-specific tests. The feature delivers immediate value once any single component is updated.

### Technical Dependencies and Implementation Order

#### Required Implementation Order

1. **Centralized Label Constants and Helper Functions**
   - Technical Reason: All other components depend on consistent label definitions
   - Dependent Elements: Controller, Scaler, Watcher, Rebalancer

2. **NodeGroup Controller Filter**
   - Technical Reason: Core reconciliation loop must filter first to prevent processing unmanaged NodeGroups
   - Prerequisites: Label constants

3. **EventWatcher and ResourceAnalyzer Filter**
   - Technical Reason: Scale-up decisions depend on filtered NodeGroup list
   - Prerequisites: Label constants

4. **Dynamic NodeGroup Creator**
   - Technical Reason: Requires filtered NodeGroup list to determine if creation is needed
   - Prerequisites: EventWatcher filter complete

5. **ScaleDownManager Filter**
   - Technical Reason: Can be done in parallel with EventWatcher since they are independent paths
   - Prerequisites: Label constants

6. **Rebalancer Label Normalization**
   - Technical Reason: Lowest priority, can be done last
   - Prerequisites: Label constants

### Integration Points

**Integration Point 1: Label Helper -> All Components**
- Components: `labels.go` -> Controller, Scaler, Watcher, Rebalancer
- Verification: Unit tests for label matching logic

**Integration Point 2: Controller Filter -> Reconciliation**
- Components: NodeGroupReconciler -> Label filter
- Verification: Integration test with mixed managed/unmanaged NodeGroups

**Integration Point 3: EventWatcher Filter -> Scale-Up**
- Components: GetNodeGroups -> FindMatchingNodeGroups -> ScaleUpController
- Verification: E2E test with pending pods and managed NodeGroup

**Integration Point 4: Dynamic Creator -> NodeGroup Creation**
- Components: ScaleUpController -> DynamicNodeGroupCreator -> Kubernetes API
- Verification: E2E test with pending pods and no suitable managed NodeGroup

### Migration Strategy

**Clean Slate Approach**:

1. Existing NodeGroups: No changes, they become "unmanaged" and are ignored
2. New NodeGroups: Must be created with `autoscaler.vpsie.com/managed=true` label
3. No migration tooling required
4. Documentation update to inform users of the new requirement

**Example of creating a managed NodeGroup**:
```yaml
apiVersion: autoscaler.vpsie.com/v1alpha1
kind: NodeGroup
metadata:
  name: my-nodegroup
  labels:
    autoscaler.vpsie.com/managed: "true"  # Required for autoscaler management
spec:
  minNodes: 1
  maxNodes: 10
  # ... rest of spec
```

## Test Strategy

### Basic Test Design Policy

Each acceptance criterion maps to at least one test case. Tests verify observable behavior, not implementation details.

### Unit Tests

| Test Case | AC Reference | Description |
|-----------|--------------|-------------|
| `TestIsManagedNodeGroup_WithLabel` | FR1 | Returns true when label present |
| `TestIsManagedNodeGroup_WithoutLabel` | FR1 | Returns false when label missing |
| `TestIsManagedNodeGroup_NilLabels` | FR1 | Returns false when labels map is nil |
| `TestManagedLabelSelector` | FR1 | Returns correct MatchingLabels |
| `TestFilterManagedNodeGroups` | FR2 | Filters list correctly |

### Integration Tests

| Test Case | AC Reference | Description |
|-----------|--------------|-------------|
| `TestReconciler_SkipsUnmanagedNodeGroups` | FR1, FR2 | Controller ignores unmanaged |
| `TestScaleDownManager_FiltersByManagedLabel` | FR2 | Scaler only considers managed |
| `TestEventWatcher_ReturnsOnlyManagedNodeGroups` | FR2 | Watcher filters correctly |
| `TestRebalancer_UsesConsistentLabelSelector` | FR2 | Label namespace normalized |

### E2E Tests

| Test Case | AC Reference | Description |
|-----------|--------------|-------------|
| `TestScaleUp_WithManagedNodeGroup` | FR2 | Scales managed NodeGroup |
| `TestScaleUp_IgnoresUnmanagedNodeGroup` | FR2 | Does not scale unmanaged |
| `TestDynamicCreation_WhenNoSuitableExists` | FR3 | Creates new managed NodeGroup |
| `TestDynamicCreation_UsesExistingWhenSuitable` | FR3, FR4 | Prefers existing managed NodeGroup |

### Performance Tests

Not required - label selector filtering uses native Kubernetes indexing with O(1) complexity per label.

## Security Considerations

1. **Label Tampering**: Users could add the managed label to any NodeGroup. This is acceptable as it opts-in to autoscaler management.
2. **Resource Limits**: Dynamic NodeGroup creation should be rate-limited to prevent resource exhaustion.
3. **RBAC**: No changes required - existing permissions for NodeGroup CRs apply.

## Future Extensibility

1. **Namespace Isolation**: Could extend to support per-namespace managed labels
2. **Label Value Customization**: Could make label key/value configurable via CRD or ConfigMap
3. **Multi-Autoscaler Support**: Different autoscaler instances could use different label values

## Alternative Solutions

### Alternative 1: Annotation-Based Filtering

- **Overview**: Use annotation `autoscaler.vpsie.com/managed: "true"` instead of label
- **Advantages**: Keeps labels for organizational purposes only
- **Disadvantages**: Cannot use native Kubernetes label selectors, requires client-side filtering
- **Reason for Rejection**: Labels with selectors provide better API-level filtering performance

### Alternative 2: Opt-Out Instead of Opt-In

- **Overview**: Manage all NodeGroups by default, use label to exclude
- **Advantages**: Less change for existing users
- **Disadvantages**: Breaking change for users who don't want autoscaler management, less secure default
- **Reason for Rejection**: Opt-in is safer and more explicit

### Alternative 3: CRD Field Instead of Label

- **Overview**: Add `spec.managed: true` field to NodeGroup CRD
- **Advantages**: Type-safe, validated by webhook
- **Disadvantages**: Requires CRD schema change, migration, regeneration
- **Reason for Rejection**: Labels are the Kubernetes-native way to express cross-cutting concerns

## Risks and Mitigation

| Risk | Impact | Probability | Mitigation |
|------|--------|-------------|------------|
| Existing users don't add label to NodeGroups | High (autoscaler stops working for them) | High | Clear documentation, release notes, CLI warning |
| Dynamic creation creates too many NodeGroups | Medium | Low | Rate limiting, max NodeGroups per namespace |
| Label selector inconsistency (vpsie.io vs autoscaler.vpsie.com) | Medium | Medium | Normalize all selectors in this implementation |
| Rebalancer operates on wrong nodes due to label mismatch | High | Medium | Add managed label check in rebalancer entry point |

## References

- [Kubernetes Recommended Labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/)
- [Well-Known Labels, Annotations and Taints](https://kubernetes.io/docs/reference/labels-annotations-taints/)
- [Best Practices Guide for Kubernetes Labels and Annotations](https://komodor.com/blog/best-practices-guide-for-kubernetes-labels-and-annotations/)
- [Labels and Selectors](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/)

## Update History

| Date | Version | Changes | Author |
|------|---------|---------|--------|
| 2026-01-10 | 1.0 | Initial version | Claude |
