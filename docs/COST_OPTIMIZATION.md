# Cost Optimization Engine

## Overview

The Cost Optimization Engine analyzes NodeGroup resource utilization and recommends cost-effective instance type changes to reduce infrastructure costs while maintaining performance and availability.

## Architecture

### Components

```
┌─────────────────────────────────────────────────────────────┐
│                  Cost Optimization Engine                    │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────────┐  ┌──────────────────┐  ┌────────────┐ │
│  │  Cost Calculator │  │   Cost Analyzer  │  │ Cost       │ │
│  │                  │  │                  │  │ Optimizer  │ │
│  │ • Price lookup   │  │ • Track costs    │  │ • Analyze  │ │
│  │ • Cost calc      │  │ • Trend analysis │  │ • Recommend│ │
│  │ • Comparison     │  │ • Forecast       │  │ • Simulate │ │
│  └──────────────────┘  └──────────────────┘  └────────────┘ │
│           │                     │                    │       │
│           └─────────────────────┴────────────────────┘       │
│                              │                               │
└──────────────────────────────┼───────────────────────────────┘
                               │
                    ┌──────────┴─────────┐
                    │  NodeGroup Controller│
                    └────────────────────┘
```

### 1. Cost Calculator (`pkg/vpsie/cost/calculator.go`)

**Responsibilities:**
- Fetch offering prices from VPSie API
- Calculate costs for individual instances (hourly, daily, monthly)
- Calculate aggregate costs for NodeGroups
- Compare costs between different offerings
- Calculate potential savings

**Key Functions:**
```go
// GetOfferingCost returns the cost for a specific offering
GetOfferingCost(ctx, offeringID string) (*OfferingCost, error)

// CalculateNodeGroupCost returns current cost for a NodeGroup
CalculateNodeGroupCost(ctx, nodeGroup *NodeGroup) (*NodeGroupCost, error)

// CompareOfferings compares costs between multiple offerings
CompareOfferings(offerings []string) (*CostComparison, error)

// CalculateSavings estimates savings from switching instance types
CalculateSavings(current, proposed *NodeGroupCost) (*SavingsAnalysis, error)
```

### 2. Cost Analyzer (`pkg/vpsie/cost/analyzer.go`)

**Responsibilities:**
- Track historical costs over time
- Analyze cost trends and patterns
- Identify cost anomalies and spikes
- Forecast future costs based on trends
- Generate cost reports

**Key Functions:**
```go
// RecordCost records a cost snapshot
RecordCost(ctx, snapshot *CostSnapshot) error

// GetCostTrend returns cost trend over time period
GetCostTrend(ctx, nodeGroup string, period time.Duration) (*CostTrend, error)

// AnalyzeUtilization analyzes resource utilization vs cost
AnalyzeUtilization(ctx, nodeGroup *NodeGroup) (*UtilizationAnalysis, error)

// ForecastCost forecasts costs based on historical data
ForecastCost(ctx, nodeGroup string, horizon time.Duration) (*CostForecast, error)
```

### 3. Cost Optimizer (`pkg/vpsie/cost/optimizer.go`)

**Responsibilities:**
- Analyze current node configurations and costs
- Identify optimization opportunities
- Recommend instance type changes
- Simulate cost impact of changes
- Generate optimization reports

**Key Functions:**
```go
// AnalyzeOptimizations identifies optimization opportunities
AnalyzeOptimizations(ctx, nodeGroup *NodeGroup) (*OptimizationReport, error)

// RecommendInstanceType recommends optimal instance type
RecommendInstanceType(ctx, requirements ResourceRequirements) (*Recommendation, error)

// SimulateOptimization simulates cost impact of optimization
SimulateOptimization(ctx, optimization *Optimization) (*SimulationResult, error)

// ApplyOptimization applies an optimization to a NodeGroup
ApplyOptimization(ctx, nodeGroup *NodeGroup, optimization *Optimization) error
```

## Data Structures

### Cost Information
```go
type OfferingCost struct {
    OfferingID   string
    HourlyCost   float64
    DailyCost    float64
    MonthlyCost  float64
    Currency     string
    Specs        ResourceSpecs
    LastUpdated  time.Time
}

type NodeGroupCost struct {
    NodeGroupName  string
    TotalNodes     int32
    CostPerNode    float64
    TotalHourly    float64
    TotalDaily     float64
    TotalMonthly   float64
    InstanceTypes  map[string]int32  // offeringID -> count
    LastUpdated    time.Time
}

type CostSnapshot struct {
    Timestamp     time.Time
    NodeGroupName string
    Cost          NodeGroupCost
    Utilization   ResourceUtilization
}
```

### Optimization
```go
type OptimizationReport struct {
    NodeGroupName      string
    CurrentCost        NodeGroupCost
    Opportunities      []Opportunity
    PotentialSavings   float64
    RecommendedAction  string
    GeneratedAt        time.Time
}

type Opportunity struct {
    Type              OptimizationType
    Description       string
    CurrentOffering   string
    RecommendedOffering string
    AffectedNodes     []string
    MonthlySavings    float64
    ConfidenceScore   float64
    Risk              RiskLevel
}

type OptimizationType string

const (
    Downsize          OptimizationType = "downsize"
    RightSize         OptimizationType = "rightsize"
    ChangeCategory    OptimizationType = "change_category"
    ConsolidateNodes  OptimizationType = "consolidate"
)

type Recommendation struct {
    OfferingID      string
    Rationale       string
    ExpectedSavings float64
    PerformanceImpact string
    Confidence      float64
}
```

### Trend Analysis
```go
type CostTrend struct {
    NodeGroupName string
    StartTime     time.Time
    EndTime       time.Time
    DataPoints    []CostDataPoint
    AverageCost   float64
    MinCost       float64
    MaxCost       float64
    Trend         string  // "increasing", "decreasing", "stable"
    ChangePercent float64
}

type CostDataPoint struct {
    Timestamp time.Time
    Cost      float64
    NodeCount int32
}
```

## Optimization Strategies

### 1. Right-Sizing
- Analyze actual resource usage (CPU, memory, disk)
- Compare with instance specifications
- Recommend smaller instances if consistently under-utilized
- Recommend larger instances if frequently over-utilized

**Triggers:**
- CPU utilization < 30% for 7 days → Consider downsize
- CPU utilization > 80% for 3 days → Consider upsize
- Memory utilization < 40% for 7 days → Consider downsize

### 2. Instance Category Optimization
- Switch between standard, high-memory, compute-optimized offerings
- Based on workload characteristics

**Examples:**
- High memory usage → Recommend high-memory instances
- High CPU usage → Recommend compute-optimized instances
- Balanced usage → Recommend standard instances

### 3. Node Consolidation
- Identify NodeGroups with low utilization
- Recommend consolidating workloads onto fewer, larger nodes
- Balance cost savings vs fault tolerance

### 4. Spot Instance Usage
- Identify workloads suitable for spot instances
- Recommend spot instances for fault-tolerant workloads
- Calculate potential savings (typically 60-80% off on-demand price)

## Metrics

The cost optimization engine exposes the following Prometheus metrics:

```
# Current costs
vpsie_nodegroup_cost_hourly{nodegroup, datacenter}
vpsie_nodegroup_cost_monthly{nodegroup, datacenter}

# Optimization opportunities
vpsie_cost_optimization_opportunities{nodegroup, type}
vpsie_cost_potential_savings_monthly{nodegroup}

# Utilization efficiency
vpsie_cost_per_cpu_core{nodegroup, offering}
vpsie_cost_per_gb_memory{nodegroup, offering}
vpsie_resource_utilization_score{nodegroup}

# Optimization actions
vpsie_cost_optimizations_applied_total{nodegroup, type}
vpsie_cost_savings_realized_monthly{nodegroup}
```

## Configuration

Cost optimization can be configured per NodeGroup:

```yaml
apiVersion: autoscaler.vpsie.io/v1alpha1
kind: NodeGroup
metadata:
  name: my-nodegroup
spec:
  # ... existing fields ...
  costOptimization:
    enabled: true
    strategy: auto  # auto, manual, aggressive, conservative

    # Minimum time between optimizations
    optimizationInterval: 24h

    # Thresholds for optimization
    thresholds:
      cpuUtilization: 30  # Downsize if < 30% for period
      memoryUtilization: 40
      evaluationPeriod: 7d

    # Constraints
    constraints:
      minMonthlySavings: 10.00  # Only apply if saves at least $10/month
      maxPerformanceImpact: 5   # Max 5% performance reduction
      requireApproval: false    # Auto-apply or require approval

    # Notifications
    notifications:
      slack: "#cost-alerts"
      email: ["ops@example.com"]
```

## Integration with Node Rebalancer

The Cost Optimizer works with the Node Rebalancer to apply optimizations:

1. **Cost Optimizer** identifies optimization opportunities
2. **Node Rebalancer** creates a safe migration plan
3. **Node Rebalancer** executes node replacements with zero downtime
4. **Cost Analyzer** tracks realized savings

## Implementation Phases

### Phase 1: Cost Calculator (Week 1)
- Implement basic cost calculation
- Fetch offering prices from VPSie API
- Calculate NodeGroup costs
- Add cost metrics

### Phase 2: Cost Analyzer (Week 2)
- Implement historical cost tracking
- Add trend analysis
- Implement utilization analysis
- Add forecasting

### Phase 3: Cost Optimizer (Week 3)
- Implement optimization analysis
- Add recommendation engine
- Implement simulation
- Add optimization reports

### Phase 4: Integration & Testing (Week 4)
- Integrate with NodeGroup controller
- Integration with node rebalancer
- Comprehensive testing
- Documentation

## Safety & Best Practices

1. **Never optimize without data** - Require minimum 24h of metrics
2. **Gradual changes** - Optimize one node at a time
3. **Rollback capability** - Track previous configurations
4. **Approval workflow** - Support manual approval for aggressive optimizations
5. **Impact analysis** - Always simulate before applying
6. **Cost-performance balance** - Don't sacrifice performance for minimal savings

## Example Usage

```go
// Initialize cost optimizer
optimizer := cost.NewOptimizer(vpsieClient, metricsRecorder)

// Analyze a NodeGroup for optimization opportunities
report, err := optimizer.AnalyzeOptimizations(ctx, nodeGroup)
if err != nil {
    log.Error("Failed to analyze optimizations", "error", err)
    return err
}

// Check if we have opportunities
if len(report.Opportunities) > 0 {
    log.Info("Found optimization opportunities",
        "count", len(report.Opportunities),
        "potential_savings", report.PotentialSavings)

    // Pick the best opportunity
    best := report.Opportunities[0]

    // Simulate the optimization
    simulation, err := optimizer.SimulateOptimization(ctx, &cost.Optimization{
        NodeGroupName: nodeGroup.Name,
        Opportunity:   best,
    })

    if simulation.IsViable() {
        // Apply via rebalancer
        if err := rebalancer.ApplyOptimization(ctx, simulation); err != nil {
            log.Error("Failed to apply optimization", "error", err)
        }
    }
}
```

## Future Enhancements

1. **Reserved Instances** - Support for reserved instance purchasing
2. **Multi-cloud cost comparison** - Compare costs across cloud providers
3. **Budget alerts** - Alert when costs exceed budgets
4. **Cost allocation** - Break down costs by team, project, or workload
5. **ML-based forecasting** - Use machine learning for better predictions
6. **Automated optimization** - Fully automated optimization with ML
