# Node Rebalancer Architecture

## Overview

The Node Rebalancer is responsible for safely replacing nodes in a NodeGroup with optimized instance types to reduce costs or improve performance. It works in conjunction with the Cost Optimizer to apply recommendations with zero downtime.

## Architecture

```
┌────────────────────────────────────────────────────────────┐
│                    Node Rebalancer                          │
├────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐     │
│  │   Analyzer   │  │   Planner    │  │   Executor   │     │
│  │              │  │              │  │              │     │
│  │ • Identify   │  │ • Create     │  │ • Provision  │     │
│  │   candidates │  │   migration  │  │   new nodes  │     │
│  │ • Safety     │  │   plan       │  │ • Cordon &   │     │
│  │   checks     │  │ • Batch      │  │   drain old  │     │
│  │ • Priority   │  │   nodes      │  │ • Terminate  │     │
│  │   scoring    │  │ • Rollback   │  │ • Verify     │     │
│  └──────────────┘  └──────────────┘  └──────────────┘     │
│         │                 │                  │             │
│         └─────────────────┴──────────────────┘             │
│                          │                                 │
└──────────────────────────┼─────────────────────────────────┘
                           │
              ┌────────────┴─────────────┐
              │  NodeGroup Controller    │
              └──────────────────────────┘
```

## Components

### 1. Analyzer

**Responsibilities:**
- Identify nodes that are candidates for rebalancing
- Perform safety checks before rebalancing
- Calculate priority scores for rebalancing order
- Validate optimization opportunities

**Key Functions:**
```go
// AnalyzeRebalanceOpportunities identifies which nodes should be rebalanced
AnalyzeRebalanceOpportunities(ctx, nodeGroup) (*RebalanceAnalysis, error)

// ValidateRebalanceSafety checks if rebalancing is safe
ValidateRebalanceSafety(ctx, nodeGroup, nodes) (*SafetyCheck, error)

// CalculateRebalancePriority determines order of node replacement
CalculateRebalancePriority(nodes []*Node, optimization) ([]PriorityNode, error)
```

### 2. Planner

**Responsibilities:**
- Create detailed migration plans
- Batch nodes for gradual replacement
- Define rollback procedures
- Estimate time and resource requirements

**Key Functions:**
```go
// CreateRebalancePlan creates a detailed plan for node rebalancing
CreateRebalancePlan(ctx, analysis *RebalanceAnalysis, optimization) (*RebalancePlan, error)

// BatchNodes groups nodes into batches for gradual replacement
BatchNodes(nodes []Node, batchSize int) ([]NodeBatch, error)

// CreateRollbackPlan defines how to revert if rebalancing fails
CreateRollbackPlan(plan *RebalancePlan) (*RollbackPlan, error)
```

### 3. Executor

**Responsibilities:**
- Execute rebalance plans
- Provision new nodes
- Safely drain old nodes
- Monitor progress and health
- Handle failures and rollbacks

**Key Functions:**
```go
// ExecuteRebalance executes a rebalance plan
ExecuteRebalance(ctx, plan *RebalancePlan) (*RebalanceResult, error)

// ProvisionNode provisions a new node with the target instance type
ProvisionNode(ctx, spec *NodeSpec) (*Node, error)

// DrainNode safely drains workloads from a node
DrainNode(ctx, node *Node) error

// TerminateNode terminates an old node after draining
TerminateNode(ctx, node *Node) error

// Rollback reverts a failed rebalancing operation
Rollback(ctx, plan *RebalancePlan, state *ExecutionState) error
```

## Data Structures

### Rebalance Analysis
```go
type RebalanceAnalysis struct {
    NodeGroupName      string
    Namespace          string
    TotalNodes         int32
    CandidateNodes     []CandidateNode
    Optimization       *Opportunity
    SafetyChecks       []SafetyCheck
    RecommendedAction  string
    Priority           RebalancePriority
    EstimatedDuration  time.Duration
    AnalyzedAt         time.Time
}

type CandidateNode struct {
    NodeName           string
    VPSID              int
    CurrentOffering    string
    TargetOffering     string
    Age                time.Duration
    Workloads          []Workload
    PriorityScore      float64
    SafeToRebalance    bool
    RebalanceReason    string
}
```

### Rebalance Plan
```go
type RebalancePlan struct {
    ID                 string
    NodeGroupName      string
    Namespace          string
    Optimization       *Opportunity
    Batches            []NodeBatch
    TotalNodes         int32
    Strategy           RebalanceStrategy
    MaxConcurrent      int32
    RollbackPlan       *RollbackPlan
    EstimatedDuration  time.Duration
    CreatedAt          time.Time
}

type NodeBatch struct {
    BatchNumber        int
    Nodes              []CandidateNode
    EstimatedDuration  time.Duration
    DependsOn          []int  // Previous batches that must complete
}
```

### Execution State
```go
type ExecutionState struct {
    PlanID             string
    Status             ExecutionStatus
    CurrentBatch       int
    CompletedNodes     []string
    FailedNodes        []NodeFailure
    ProvisionedNodes   []string
    StartedAt          time.Time
    CompletedAt        *time.Time
    Errors             []error
}
```

## Rebalancing Strategies

### 1. Rolling Replacement
- **Description**: Replace nodes one-by-one or in small batches
- **Use Case**: Production environments requiring zero downtime
- **Risk**: Low
- **Duration**: Longer (gradual)

```
[Old Node 1] [Old Node 2] [Old Node 3]
     ↓
[New Node 1] [Old Node 2] [Old Node 3]  <- Provision & wait
     ↓
[New Node 1] [Old Node 2] [Old Node 3]  <- Drain old node 1
     ↓
[New Node 1] [Old Node 2] [Old Node 3]  <- Terminate old node 1
     ↓
[New Node 1] [New Node 2] [Old Node 3]  <- Repeat
```

### 2. Surge Replacement
- **Description**: Provision all new nodes first, then drain old ones
- **Use Case**: When capacity is critical
- **Risk**: Medium (temporary over-provisioning)
- **Duration**: Faster

```
[Old 1] [Old 2] [Old 3]
     ↓
[Old 1] [Old 2] [Old 3] [New 1] [New 2] [New 3]  <- Provision all
     ↓
[New 1] [New 2] [New 3]  <- Drain and terminate all old
```

### 3. Blue-Green
- **Description**: Create complete new node group, switch traffic, remove old
- **Use Case**: Major upgrades or high-risk changes
- **Risk**: High (requires double capacity temporarily)
- **Duration**: Medium

## Safety Checks

Before rebalancing, the Analyzer performs these safety checks:

1. **Cluster Health**
   - All control plane components healthy
   - No ongoing disruptions
   - Sufficient capacity for draining

2. **NodeGroup Health**
   - Minimum nodes requirement met
   - No recent scaling events
   - No nodes already draining

3. **PodDisruptionBudgets**
   - All PDBs can be satisfied
   - No critical workloads would be disrupted
   - Sufficient replicas available

4. **Resource Availability**
   - Target instance types available in VPSie
   - Sufficient quota for new nodes
   - Network/storage capacity available

5. **Timing Constraints**
   - No recent optimizations (cooldown period)
   - Outside of maintenance windows (if configured)
   - Not during peak traffic hours (if configured)

## Rebalancing Flow

### Phase 1: Analysis
1. Cost Optimizer identifies optimization opportunity
2. Analyzer validates safety of rebalancing
3. Analyzer identifies candidate nodes
4. Analyzer calculates priority scores

### Phase 2: Planning
1. Planner creates detailed rebalance plan
2. Planner batches nodes for gradual replacement
3. Planner defines rollback procedures
4. Plan is reviewed (manual approval if required)

### Phase 3: Execution
1. **For each batch:**
   - Provision new nodes with target instance type
   - Wait for new nodes to become Ready
   - Cordon old nodes (prevent new pods)
   - Drain old nodes (evict pods gracefully)
   - Wait for pods to reschedule
   - Verify workloads are healthy
   - Terminate old nodes
   - Update NodeGroup status

2. **Monitoring:**
   - Track progress of each batch
   - Monitor pod health during draining
   - Watch for failed provisioning
   - Detect stuck drains

3. **Failure Handling:**
   - Pause execution on failure
   - Log detailed error information
   - Execute rollback if necessary
   - Alert operators

### Phase 4: Verification
1. Verify all new nodes are Ready
2. Verify all workloads are healthy
3. Verify cost savings realized
4. Update metrics and status
5. Record outcome for future optimizations

## Configuration

```yaml
apiVersion: autoscaler.vpsie.io/v1alpha1
kind: NodeGroup
metadata:
  name: my-nodegroup
spec:
  # ... other fields ...

  rebalancing:
    enabled: true
    strategy: "rolling"                # rolling, surge, blue-green
    maxConcurrent: 2                   # Max nodes to rebalance at once
    batchSize: 1                       # Nodes per batch
    drainTimeout: "300s"               # Max time to drain a node
    provisionTimeout: "600s"           # Max time to provision a node
    cooldownPeriod: "1h"               # Min time between rebalancing
    requireApproval: false             # Auto-execute or require approval

    # Safety settings
    minHealthyPercent: 75              # Keep at least 75% nodes healthy
    skipNodesWithLocalStorage: true    # Don't rebalance nodes with local data
    respectPDBs: true                  # Honor PodDisruptionBudgets

    # Scheduling
    maintenanceWindows:
      - start: "02:00"
        end: "04:00"
        days: ["saturday", "sunday"]
```

## Metrics

```
# Rebalancing operations
vpsie_rebalancing_plans_created_total
vpsie_rebalancing_plans_executed_total
vpsie_rebalancing_plans_failed_total
vpsie_rebalancing_duration_seconds

# Node operations
vpsie_rebalancing_nodes_provisioned_total
vpsie_rebalancing_nodes_drained_total
vpsie_rebalancing_nodes_terminated_total
vpsie_rebalancing_nodes_failed_total

# Safety metrics
vpsie_rebalancing_safety_checks_passed_total
vpsie_rebalancing_safety_checks_failed_total
vpsie_rebalancing_rollbacks_executed_total

# Progress metrics
vpsie_rebalancing_progress_percent
vpsie_rebalancing_current_batch
vpsie_rebalancing_estimated_completion_seconds
```

## Error Handling

### Provisioning Failures
- **Cause**: VPSie API errors, quota limits, unavailable instances
- **Action**: Retry with exponential backoff, fall back to alternative offerings
- **Rollback**: Not needed (no old nodes affected)

### Drain Failures
- **Cause**: Stuck pods, PDB violations, timeout
- **Action**: Force drain after timeout, skip problematic pods
- **Rollback**: Uncordon node, cancel rebalancing

### Node Join Failures
- **Cause**: Network issues, authentication, Kubernetes version mismatch
- **Action**: Retry join, check logs, terminate and reprovision
- **Rollback**: Keep old nodes, terminate new nodes

### Workload Failures
- **Cause**: Pods not starting on new nodes, health check failures
- **Action**: Investigate pod events, check resource constraints
- **Rollback**: Restore old nodes, reschedule pods back

## Best Practices

1. **Start Small**: Begin with batch size of 1, increase gradually
2. **Monitor Closely**: Watch metrics and logs during first rebalancing
3. **Test in Staging**: Always test rebalancing in non-production first
4. **Use Approval Workflow**: Require manual approval for production initially
5. **Respect Peak Hours**: Avoid rebalancing during peak traffic
6. **Honor PDBs**: Always respect PodDisruptionBudgets
7. **Set Timeouts**: Configure reasonable drain and provision timeouts
8. **Plan Rollback**: Always have a rollback plan ready
9. **Document Changes**: Log all rebalancing operations for audit
10. **Gradual Rollout**: Start with conservative settings, tune over time

## Integration with Cost Optimizer

```go
// Example: Cost Optimizer triggers rebalancing
report, _ := optimizer.AnalyzeOptimizations(ctx, nodeGroup)
if len(report.Opportunities) > 0 {
    top := report.Opportunities[0]

    // Analyze rebalance feasibility
    analysis, _ := rebalancer.Analyze(ctx, nodeGroup, top)

    if analysis.SafeToRebalance {
        // Create plan
        plan, _ := rebalancer.Plan(ctx, analysis)

        // Execute (or queue for approval)
        if !nodeGroup.Spec.Rebalancing.RequireApproval {
            result, _ := rebalancer.Execute(ctx, plan)
            log.Info("Rebalancing completed", "savings", result.SavingsRealized)
        } else {
            log.Info("Rebalancing plan created, awaiting approval", "planID", plan.ID)
        }
    }
}
```

## Future Enhancements

1. **Predictive Rebalancing**: Schedule based on forecasted workload
2. **Machine Learning**: Learn optimal batch sizes and timing
3. **Multi-Cluster**: Coordinate rebalancing across clusters
4. **Cost-Aware Scheduling**: Prefer rebalancing during off-peak pricing
5. **Automated Rollback**: Detect issues and rollback automatically
6. **Progressive Delivery**: Canary-style rebalancing with automatic validation
