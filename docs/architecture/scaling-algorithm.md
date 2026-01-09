# Scaling Algorithm

## Overview

The VPSie Autoscaler uses a combination of reactive and proactive scaling strategies to maintain optimal cluster capacity while minimizing costs.

## Scale-Up Algorithm

### Trigger Conditions

Scale-up is triggered when:
1. **Pending pods exist** that cannot be scheduled due to insufficient resources
2. **Current nodes < Desired nodes** (gap exists)
3. **Current nodes < Min nodes** (below minimum)

### Scale-Up Decision Flow

```
┌─────────────────────────────────────┐
│        Reconciliation Loop          │
└─────────────────┬───────────────────┘
                  │
                  ▼
┌─────────────────────────────────────┐
│    Check Current vs Desired Nodes   │
│    CurrentNodes < DesiredNodes?     │
└─────────────────┬───────────────────┘
                  │
          ┌───────┴───────┐
          │ Yes           │ No
          ▼               ▼
┌─────────────────┐  ┌────────────────┐
│ Calculate nodes │  │ Check pending  │
│ to add          │  │ pods           │
└────────┬────────┘  └───────┬────────┘
         │                   │
         │                   ▼
         │           ┌──────────────────┐
         │           │ Unschedulable    │
         │           │ pods exist?      │
         │           └───────┬──────────┘
         │                   │
         │           ┌───────┴───────┐
         │           │ Yes           │ No
         │           ▼               ▼
         │    ┌─────────────┐  ┌─────────────┐
         │    │ Increase    │  │ No action   │
         │    │ desired     │  │ needed      │
         │    │ count       │  └─────────────┘
         │    └──────┬──────┘
         │           │
         └─────┬─────┘
               │
               ▼
┌─────────────────────────────────────┐
│     Create VPSieNode Resources      │
│     (up to nodesToAdd count)        │
└─────────────────────────────────────┘
```

### Nodes to Add Calculation

```go
func CalculateNodesToAdd(ng *NodeGroup) int32 {
    nodesToAdd := ng.Status.DesiredNodes - ng.Status.CurrentNodes

    // Respect max nodes limit
    maxAddable := ng.Spec.MaxNodes - ng.Status.CurrentNodes
    if nodesToAdd > maxAddable {
        nodesToAdd = maxAddable
    }

    // Respect batch size if configured
    if ng.Spec.ScaleUpBatchSize > 0 && nodesToAdd > ng.Spec.ScaleUpBatchSize {
        nodesToAdd = ng.Spec.ScaleUpBatchSize
    }

    return nodesToAdd
}
```

### Instance Type Selection

When multiple offering IDs are configured:
1. Use `preferredInstanceType` if specified and valid
2. Otherwise, select first available offering from `offeringIDs`
3. Validate offering exists in target datacenter

## Scale-Down Algorithm

### Trigger Conditions

Scale-down is triggered when:
1. **Current nodes > Desired nodes** (overprovisioned)
2. **Underutilized nodes exist** (CPU/Memory below threshold)
3. **Cooldown period has elapsed** since last scaling

### Scale-Down Decision Flow

```
┌─────────────────────────────────────┐
│   Identify Scale-Down Candidates    │
└─────────────────┬───────────────────┘
                  │
                  ▼
┌─────────────────────────────────────┐
│   For each node, check utilization: │
│   - CPU utilization < threshold     │
│   - Memory utilization < threshold  │
└─────────────────┬───────────────────┘
                  │
                  ▼
┌─────────────────────────────────────┐
│   Apply Safety Checks:              │
│   1. PodDisruptionBudget            │
│   2. Local storage pods             │
│   3. System pods                    │
│   4. Affinity rules                 │
│   5. Minimum nodes constraint       │
│   6. Cooldown period                │
└─────────────────┬───────────────────┘
                  │
          ┌───────┴───────┐
          │ Pass          │ Fail
          ▼               ▼
┌─────────────────┐  ┌────────────────┐
│ Drain and       │  │ Skip candidate │
│ terminate       │  │ log reason     │
└─────────────────┘  └────────────────┘
```

### Utilization Thresholds

```go
type ScaleDownConfig struct {
    // CPUUtilizationThreshold is the CPU threshold below which
    // a node is considered underutilized (default: 50%)
    CPUUtilizationThreshold float64

    // MemoryUtilizationThreshold is the memory threshold below which
    // a node is considered underutilized (default: 50%)
    MemoryUtilizationThreshold float64

    // UnderutilizationDuration is how long a node must be
    // underutilized before becoming a candidate (default: 10m)
    UnderutilizationDuration time.Duration
}
```

### Candidate Selection

Nodes are prioritized for scale-down in this order:
1. **Not ready nodes** - Unhealthy nodes first
2. **Oldest nodes** - Older nodes before newer ones
3. **Most underutilized** - Lowest utilization first

```go
func selectNodesToDelete(nodes []VPSieNode, count int) []VPSieNode {
    // Sort by priority
    sort.Slice(nodes, func(i, j int) bool {
        // Not ready nodes first
        if nodes[i].Status.Phase != Ready && nodes[j].Status.Phase == Ready {
            return true
        }
        // Then by creation time (oldest first)
        return nodes[i].CreationTimestamp.Before(&nodes[j].CreationTimestamp)
    })

    return nodes[:min(count, len(nodes))]
}
```

### Safety Checks

#### PodDisruptionBudget Validation
```go
func (p *PolicyEngine) checkPDBs(ctx context.Context, node string) error {
    pods := listPodsOnNode(node)
    for _, pod := range pods {
        pdbs := findPDBsForPod(pod)
        for _, pdb := range pdbs {
            if pdb.Status.DisruptionsAllowed == 0 {
                return ErrPDBViolation
            }
        }
    }
    return nil
}
```

#### Local Storage Check
```go
func hasLocalStorage(pod *v1.Pod) bool {
    for _, vol := range pod.Spec.Volumes {
        if vol.EmptyDir != nil || vol.HostPath != nil {
            return true
        }
    }
    return false
}
```

## Cooldown Periods

### Scale-Up Cooldown
- Default: 0 (no cooldown)
- Prevents rapid scale-up oscillation
- Configured per NodeGroup

### Scale-Down Cooldown
- Default: 10 minutes
- Prevents premature scale-down after scale-up
- Allows time for pod scheduling stabilization

```go
func isInCooldown(ng *NodeGroup, lastScaleTime time.Time) bool {
    cooldown := ng.Spec.ScaleDownCooldown
    if cooldown == 0 {
        cooldown = DefaultScaleDownCooldown
    }
    return time.Since(lastScaleTime) < cooldown
}
```

## Desired Nodes Calculation

```go
func CalculateDesiredNodes(ng *NodeGroup) int32 {
    // Start with current + pending pods that need resources
    desired := ng.Status.CurrentNodes + calculateRequiredForPending(ng)

    // Clamp to min/max bounds
    if desired < ng.Spec.MinNodes {
        desired = ng.Spec.MinNodes
    }
    if desired > ng.Spec.MaxNodes {
        desired = ng.Spec.MaxNodes
    }

    return desired
}
```

## Scaling Events

The autoscaler emits Kubernetes events for visibility:

| Event | Type | Description |
|-------|------|-------------|
| `ScalingUp` | Normal | NodeGroup scaling up |
| `ScalingDown` | Normal | NodeGroup scaling down |
| `ScaleUpFailed` | Warning | Scale-up operation failed |
| `ScaleDownBlocked` | Warning | Scale-down blocked by safety check |

## Metrics

Key metrics for monitoring scaling:

- `scaling_decisions_total{decision,reason}` - Scaling decisions count
- `scale_up_total` - Total scale-up operations
- `scale_down_total` - Total scale-down operations
- `scale_down_blocked_total{reason}` - Blocked scale-down count
- `node_provisioning_duration_seconds` - Time to provision nodes
- `node_drain_duration_seconds` - Time to drain nodes
