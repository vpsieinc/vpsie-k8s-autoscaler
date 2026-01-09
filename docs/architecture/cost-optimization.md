# Cost Optimization

## Overview

The VPSie Autoscaler includes several cost optimization features that help minimize infrastructure spend while maintaining workload performance.

## Cost Calculation

### Offering Cost Model

```go
type OfferingCost struct {
    OfferingID    string
    HourlyCost    float64
    MonthlyCost   float64
    CPUCores      int
    MemoryGB      int
    StorageGB     int
    CostPerCPU    float64  // Normalized cost per CPU
    CostPerMemGB  float64  // Normalized cost per GB memory
}
```

### Cost Calculator

```go
type Calculator struct {
    client     *vpsieclient.Client
    cache      map[string]*OfferingCost
    cacheTTL   time.Duration
    mu         sync.RWMutex
}

func (c *Calculator) GetOfferingCost(ctx context.Context, offeringID string) (*OfferingCost, error) {
    // Check cache
    if cached := c.getFromCache(offeringID); cached != nil {
        return cached, nil
    }

    // Fetch from API
    offering, err := c.client.GetOffering(ctx, offeringID)
    if err != nil {
        return nil, err
    }

    cost := &OfferingCost{
        OfferingID:   offering.ID,
        HourlyCost:   offering.PriceHourly,
        MonthlyCost:  offering.PriceMonthly,
        CPUCores:     offering.CPU,
        MemoryGB:     offering.Memory / 1024,
        StorageGB:    offering.Storage,
        CostPerCPU:   offering.PriceHourly / float64(offering.CPU),
        CostPerMemGB: offering.PriceHourly / float64(offering.Memory/1024),
    }

    c.setInCache(offeringID, cost)
    return cost, nil
}
```

## Cost Optimization Strategies

### 1. Right-Sizing

Automatically selects the most cost-effective instance type for workload requirements:

```go
func (c *Calculator) FindOptimalOffering(
    ctx context.Context,
    requiredCPU int,
    requiredMemGB int,
    availableOfferings []string,
) (*Offering, float64, error) {
    var bestOffering *Offering
    var bestCost = math.MaxFloat64

    for _, offeringID := range availableOfferings {
        offering, _ := c.client.GetOffering(ctx, offeringID)

        // Skip if doesn't meet requirements
        if offering.CPU < requiredCPU || offering.Memory/1024 < requiredMemGB {
            continue
        }

        cost, _ := c.GetOfferingCost(ctx, offeringID)
        if cost.HourlyCost < bestCost {
            bestCost = cost.HourlyCost
            bestOffering = offering
        }
    }

    return bestOffering, bestCost, nil
}
```

### 2. Scale-Down for Cost Savings

Aggressive scale-down identifies underutilized capacity:

```go
func (s *ScaleDownManager) CalculatePotentialSavings(
    ctx context.Context,
    ng *NodeGroup,
) (float64, error) {
    candidates, _ := s.IdentifyUnderutilizedNodes(ctx, ng)

    var totalSavings float64
    for _, candidate := range candidates {
        cost, _ := s.costCalculator.GetOfferingCost(ctx, candidate.OfferingID)
        // Monthly savings = hourly cost * 24 * 30
        totalSavings += cost.HourlyCost * 720
    }

    return totalSavings, nil
}
```

### 3. Rebalancing for Better Pricing

Replaces nodes when better-priced offerings become available:

```go
func (a *Analyzer) FindRebalanceCandidates(
    ctx context.Context,
    ng *NodeGroup,
) ([]*RebalanceCandidate, error) {
    var candidates []*RebalanceCandidate

    nodes := a.getNodesForNodeGroup(ctx, ng)
    for _, node := range nodes {
        currentCost, _ := a.costCalculator.GetOfferingCost(ctx, node.OfferingID)

        // Find best alternative
        optimal, optimalCost, _ := a.costCalculator.FindOptimalOffering(
            ctx,
            node.CPUCores,
            node.MemoryGB,
            ng.Spec.OfferingIDs,
        )

        // Calculate savings percentage
        savingsPercent := (currentCost.HourlyCost - optimalCost) / currentCost.HourlyCost * 100

        if savingsPercent > a.config.SavingsThreshold {
            candidates = append(candidates, &RebalanceCandidate{
                Node:           node,
                CurrentCost:    currentCost.HourlyCost,
                OptimalCost:    optimalCost,
                SavingsPercent: savingsPercent,
                Recommendation: optimal.ID,
            })
        }
    }

    return candidates, nil
}
```

## Cost Metrics

### Prometheus Metrics

```go
// Current cost tracking
NodeGroupCostCurrent = prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Namespace: "vpsie_autoscaler",
        Name:      "nodegroup_cost_hourly",
        Help:      "Current hourly cost of a NodeGroup in USD",
    },
    []string{"nodegroup", "namespace"},
)

// Estimated savings
CostSavingsEstimatedMonthly = prometheus.NewGaugeVec(
    prometheus.GaugeOpts{
        Namespace: "vpsie_autoscaler",
        Name:      "cost_savings_estimated_monthly",
        Help:      "Estimated monthly cost savings in USD",
    },
    []string{"nodegroup", "namespace", "source"},
)

// Cumulative savings from rebalancing
RebalancerCostSavingsTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Namespace: "vpsie_autoscaler",
        Name:      "rebalancer_cost_savings_total",
        Help:      "Cumulative cost savings in USD from rebalancing",
    },
    []string{"nodegroup", "namespace"},
)
```

### Grafana Dashboard Panels

1. **Current Cluster Cost**
   - Sum of all NodeGroup hourly costs
   - Projected monthly cost

2. **Cost Savings**
   - Savings from scale-down operations
   - Savings from rebalancing
   - Total cumulative savings

3. **Cost Efficiency**
   - Cost per allocated CPU
   - Cost per allocated GB memory
   - Utilization-adjusted cost

## Cost Reporting

### Per-NodeGroup Cost

```go
func (r *CostReporter) GetNodeGroupCost(ng *NodeGroup) *NodeGroupCostReport {
    nodes := r.getNodesForNodeGroup(ng)

    var hourlyTotal float64
    for _, node := range nodes {
        cost, _ := r.calculator.GetOfferingCost(ctx, node.OfferingID)
        hourlyTotal += cost.HourlyCost
    }

    return &NodeGroupCostReport{
        NodeGroup:    ng.Name,
        Namespace:    ng.Namespace,
        NodeCount:    len(nodes),
        HourlyCost:   hourlyTotal,
        DailyCost:    hourlyTotal * 24,
        MonthlyCost:  hourlyTotal * 720,
    }
}
```

### Savings Estimation

```go
func (r *CostReporter) EstimatePotentialSavings(ng *NodeGroup) *SavingsReport {
    // Scale-down savings
    scaleDownSavings := r.calculateScaleDownSavings(ng)

    // Right-sizing savings
    rightSizingSavings := r.calculateRightSizingSavings(ng)

    // Rebalancing savings
    rebalanceSavings := r.calculateRebalanceSavings(ng)

    return &SavingsReport{
        ScaleDown:    scaleDownSavings,
        RightSizing:  rightSizingSavings,
        Rebalancing:  rebalanceSavings,
        TotalMonthly: scaleDownSavings + rightSizingSavings + rebalanceSavings,
    }
}
```

## Configuration

### NodeGroup Cost Settings

```yaml
apiVersion: autoscaler.vpsie.com/v1alpha1
kind: NodeGroup
metadata:
  name: production-workers
spec:
  # Cost-optimized instance selection
  offeringIDs:
    - "offering-small-2cpu-4gb"
    - "offering-medium-4cpu-8gb"
    - "offering-large-8cpu-16gb"

  # Prefer smaller, cheaper instances
  preferredInstanceType: "offering-small-2cpu-4gb"

  # Cost optimization settings
  costOptimization:
    enabled: true
    rebalanceEnabled: true
    savingsThreshold: 10  # 10% savings required for rebalance
```

### Global Cost Settings

```yaml
# Helm values
costOptimization:
  enabled: true
  offeringCacheTTL: 1h
  savingsAlertThreshold: 100  # Alert if potential savings > $100/month
```

## Best Practices

1. **Use Multiple Offering IDs**
   - Provide range of instance sizes
   - Allow autoscaler to pick optimal size

2. **Enable Rebalancing**
   - Catch pricing changes
   - Migrate to better instances automatically

3. **Monitor Cost Metrics**
   - Set up cost alerts
   - Review monthly reports

4. **Set Appropriate Thresholds**
   - Balance cost savings vs. operational overhead
   - Consider maintenance windows for rebalancing

5. **Right-Size Before Scaling**
   - Use metrics to determine actual resource needs
   - Avoid over-provisioning
