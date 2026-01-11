# Temporary Workarounds

This document tracks temporary workarounds implemented due to external limitations that should be removed once the limitations are fixed.

## VPSie Node Group Size Limitation

**Status:** Active workaround
**Added:** 2026-01-11
**File:** `pkg/events/creator.go` - `SelectOptimalKubeSizeID()`

### Issue

VPSie API currently does not allow multiple node groups with the same `kubeSizeID` (BoxsizeID) in a Kubernetes cluster. Attempting to create a second node group with a size that's already in use returns:

```
Group GRP-Advanced-c745 has same selected Size, please select another size
```

### Workaround

The `SelectOptimalKubeSizeID()` function now:

1. Fetches existing VPSie node groups via `ListK8sNodeGroups()`
2. Collects the `BoxsizeID` values that are already in use
3. Filters out these sizes when selecting the optimal KubeSizeID for a new node group
4. Falls back to the next available size if the optimal size is already in use

### Code Location

```go
// pkg/events/creator.go

// Get existing node groups to find sizes already in use (VPSie limitation workaround)
usedSizes := make(map[int]bool)
if c.template.ResourceIdentifier != "" {
    existingGroups, err := c.vpsieClient.ListK8sNodeGroups(ctx, c.template.ResourceIdentifier)
    // ... filter out used sizes
}
```

### When to Remove

Remove this workaround when VPSie fixes their API to allow multiple node groups with the same size. After removal:

1. Remove the `ListK8sNodeGroups()` call in `SelectOptimalKubeSizeID()`
2. Remove the `usedSizes` filtering logic
3. Simplify the selection to just pick the optimal size based on pod resources
4. Update this document to mark the workaround as removed

### Impact

- Slight performance overhead due to extra API call to list existing node groups
- If all sizes are in use, node group creation will fail with an error message
- Users may get larger (more expensive) nodes than optimal if smaller sizes are already in use
