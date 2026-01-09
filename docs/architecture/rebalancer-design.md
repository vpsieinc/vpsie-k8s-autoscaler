# Rebalancer Design

## Overview

The Rebalancer component optimizes node allocation by replacing older, more expensive nodes with newer, more cost-effective alternatives while maintaining workload availability.

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                         Rebalancer                            │
│                                                              │
│  ┌───────────────┐  ┌───────────────┐  ┌───────────────┐   │
│  │   Analyzer    │──│    Planner    │──│   Executor    │   │
│  └───────┬───────┘  └───────────────┘  └───────┬───────┘   │
│          │                                      │           │
│          │  Candidates                          │  Actions  │
│          ▼                                      ▼           │
│  ┌───────────────┐                      ┌───────────────┐   │
│  │ Safety Checks │                      │   Rollback    │   │
│  └───────────────┘                      └───────────────┘   │
│                                                              │
└──────────────────────────────────────────────────────────────┘
```

## Components

### Analyzer

Identifies nodes eligible for rebalancing:

```go
type Analyzer struct {
    client          client.Client
    metricsClient   metricsv1beta1.MetricsV1beta1Interface
    costCalculator  *cost.Calculator
}

func (a *Analyzer) IdentifyCandidates(ctx context.Context, ng *NodeGroup) ([]*RebalanceCandidate, error) {
    // 1. Get current nodes
    // 2. Calculate cost of each node
    // 3. Compare to optimal offering
    // 4. Filter by savings threshold
    // 5. Apply safety checks
    return candidates, nil
}
```

**Candidate Selection Criteria:**
- Node cost exceeds optimal by configured threshold
- Node has been running for minimum duration
- Better offering available in same datacenter
- Not recently provisioned or rebalanced

### Planner

Creates migration plans for identified candidates:

```go
type RebalancePlan struct {
    ID          string
    NodeGroup   string
    Candidates  []*RebalanceCandidate
    Strategy    RebalanceStrategy
    Steps       []PlanStep
    CreatedAt   time.Time
}

type RebalanceStrategy string

const (
    StrategyRolling   RebalanceStrategy = "rolling"
    StrategySurge     RebalanceStrategy = "surge"
    StrategyBlueGreen RebalanceStrategy = "blue-green"
)
```

**Strategies:**

| Strategy | Description | Risk | Speed |
|----------|-------------|------|-------|
| Rolling | Replace one node at a time | Low | Slow |
| Surge | Provision new nodes first, then remove old | Medium | Medium |
| Blue-Green | Create complete new set, switch traffic | High | Fast |

### Executor

Executes the migration plan:

```go
type Executor struct {
    client      client.Client
    vpsieClient *vpsieclient.Client
    drain       *DrainManager
}

func (e *Executor) Execute(ctx context.Context, plan *RebalancePlan) (*ExecutionState, error) {
    state := &ExecutionState{PlanID: plan.ID}

    for _, step := range plan.Steps {
        if err := e.executeStep(ctx, step, state); err != nil {
            return state, e.Rollback(ctx, plan, state)
        }
    }

    return state, nil
}
```

## Safety Checks

### Pre-flight Checks

Before starting rebalancing:

```go
func (a *Analyzer) performSafetyChecks(ctx context.Context, ng *NodeGroup) error {
    // 1. Cluster Health Check
    if err := a.checkClusterHealth(ctx); err != nil {
        return fmt.Errorf("cluster unhealthy: %w", err)
    }

    // 2. PDB Validation
    if err := a.checkPDBs(ctx, ng); err != nil {
        return fmt.Errorf("PDB check failed: %w", err)
    }

    // 3. Local Storage Check
    if err := a.checkLocalStorage(ctx, ng); err != nil {
        return fmt.Errorf("local storage check failed: %w", err)
    }

    // 4. Maintenance Window
    if !a.inMaintenanceWindow(ng) {
        return ErrNotInMaintenanceWindow
    }

    // 5. Cooldown Check
    if a.inCooldown(ng) {
        return ErrInCooldown
    }

    return nil
}
```

### Cluster Health Check

```go
func (a *Analyzer) checkClusterHealth(ctx context.Context) error {
    // Check node health
    nodes, _ := a.client.List(ctx, &v1.NodeList{})
    unhealthyCount := 0
    for _, node := range nodes.Items {
        if !isNodeReady(node) {
            unhealthyCount++
        }
    }

    // Fail if >20% nodes unhealthy
    if float64(unhealthyCount)/float64(len(nodes.Items)) > 0.2 {
        return ErrClusterUnhealthy
    }

    return nil
}
```

### PDB Validation

```go
func (a *Analyzer) checkPDBs(ctx context.Context, ng *NodeGroup) error {
    // List all PDBs in cluster
    pdbs, _ := a.client.List(ctx, &policyv1.PodDisruptionBudgetList{})

    for _, pdb := range pdbs.Items {
        if pdb.Status.DisruptionsAllowed == 0 {
            return fmt.Errorf("PDB %s/%s has no disruptions allowed",
                pdb.Namespace, pdb.Name)
        }
    }

    return nil
}
```

## Rollback Mechanism

### Automatic Rollback Triggers

Rollback is triggered when:
1. New node provisioning fails
2. Drain operation times out
3. Workload health check fails
4. Cluster health degrades

### Rollback Process

```go
func (e *Executor) Rollback(ctx context.Context, plan *RebalancePlan, state *ExecutionState) error {
    e.logger.Info("Initiating rollback", "planID", plan.ID)

    // 1. Uncordon old nodes
    for _, oldNode := range state.CordonedNodes {
        if err := e.UncordonNode(ctx, oldNode); err != nil {
            e.logger.Error("Failed to uncordon node", "node", oldNode.Name, "error", err)
        }
    }

    // 2. Terminate new nodes
    for _, newNode := range state.ProvisionedNodes {
        if err := e.TerminateNode(ctx, newNode); err != nil {
            e.logger.Error("Failed to terminate node", "node", newNode.Name, "error", err)
        }
    }

    // 3. Verify workload health
    if err := e.verifyWorkloadHealth(ctx); err != nil {
        return fmt.Errorf("workload health degraded after rollback: %w", err)
    }

    return nil
}
```

## Execution Strategies

### Rolling Strategy

Safest approach, replaces nodes one at a time:

```
Time ──────────────────────────────────────────────────────────►
     ┌─────────────────────────────────────────────────────────┐
     │ Node-1 │ Cordon → Drain → Terminate    │ New-1 Ready   │
     │ Node-2 │              │ Cordon → Drain → Terminate │ New-2 Ready
     │ Node-3 │                        │ Cordon → Drain → Terminate │ New-3
     └─────────────────────────────────────────────────────────┘
```

### Surge Strategy

Provisions new nodes first for faster migration:

```
Time ──────────────────────────────────────────────────────────►
     ┌─────────────────────────────────────────────────────────┐
     │ New-1, New-2, New-3 provisioning... │ Ready              │
     │                                      │ Migrate workloads  │
     │                                      │ Old-1,2,3 terminate│
     └─────────────────────────────────────────────────────────┘
```

## Metrics

| Metric | Description |
|--------|-------------|
| `rebalancer_operations_total{operation,result}` | Operation counts |
| `rebalancer_nodes_replaced_total{strategy}` | Nodes replaced by strategy |
| `rebalancer_cost_savings_total` | Cumulative USD saved |
| `rebalancer_rollback_total` | Rollback count |

## Configuration

```yaml
rebalancer:
  enabled: true
  strategy: rolling
  savingsThresholdPercent: 10    # Only rebalance if >10% savings
  minNodeAge: 24h                # Don't rebalance new nodes
  cooldownPeriod: 1h             # Wait between rebalances
  maxConcurrent: 1               # Max nodes rebalancing at once
  drainTimeout: 300s             # Drain timeout per node
  maintenanceWindows:            # Optional maintenance windows
    - "Sat 00:00-06:00"
    - "Sun 00:00-06:00"
```
