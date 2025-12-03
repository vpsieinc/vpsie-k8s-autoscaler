package cost

import (
	"context"
	"fmt"
	"sort"
	"time"

	v1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// Optimizer analyzes NodeGroups and recommends cost optimizations
type Optimizer struct {
	calculator *Calculator
	analyzer   *Analyzer
	client     client.VPSieClient
}

// NewOptimizer creates a new cost optimizer
func NewOptimizer(calculator *Calculator, analyzer *Analyzer, client client.VPSieClient) *Optimizer {
	return &Optimizer{
		calculator: calculator,
		analyzer:   analyzer,
		client:     client,
	}
}

// AnalyzeOptimizations identifies optimization opportunities for a NodeGroup
func (o *Optimizer) AnalyzeOptimizations(ctx context.Context, nodeGroup *v1alpha1.NodeGroup) (*OptimizationReport, error) {
	if nodeGroup == nil {
		return nil, fmt.Errorf("nodeGroup cannot be nil")
	}

	// Get current cost
	currentCost, err := o.calculator.CalculateNodeGroupCost(ctx, nodeGroup)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate current cost: %w", err)
	}

	// Get utilization analysis
	utilizationAnalysis, err := o.analyzer.AnalyzeUtilization(ctx, nodeGroup)
	if err != nil {
		// If no utilization data, still continue with basic analysis
		utilizationAnalysis = nil
	}

	var opportunities []Opportunity
	var totalSavings float64

	// Check for downsizing opportunities
	if utilizationAnalysis != nil && utilizationAnalysis.AverageUtilization.CPUPercent < 30 &&
		utilizationAnalysis.AverageUtilization.MemoryPercent < 40 {
		opp, err := o.analyzeDownsizing(ctx, nodeGroup, currentCost, utilizationAnalysis)
		if err == nil && opp != nil {
			opportunities = append(opportunities, *opp)
			totalSavings += opp.MonthlySavings
		}
	}

	// Check for right-sizing opportunities
	if utilizationAnalysis != nil {
		opp, err := o.analyzeRightSizing(ctx, nodeGroup, currentCost, utilizationAnalysis)
		if err == nil && opp != nil {
			opportunities = append(opportunities, *opp)
			totalSavings += opp.MonthlySavings
		}
	}

	// Check for category optimization
	opp, err := o.analyzeCategoryOptimization(ctx, nodeGroup, currentCost, utilizationAnalysis)
	if err == nil && opp != nil {
		opportunities = append(opportunities, *opp)
		totalSavings += opp.MonthlySavings
	}

	// Check for consolidation opportunities
	if currentCost.TotalNodes > 2 && utilizationAnalysis != nil &&
		utilizationAnalysis.AverageUtilization.CPUPercent < 50 {
		opp, err := o.analyzeConsolidation(ctx, nodeGroup, currentCost, utilizationAnalysis)
		if err == nil && opp != nil {
			opportunities = append(opportunities, *opp)
			totalSavings += opp.MonthlySavings
		}
	}

	// Sort opportunities by savings (descending)
	sort.Slice(opportunities, func(i, j int) bool {
		return opportunities[i].MonthlySavings > opportunities[j].MonthlySavings
	})

	// Determine recommended action
	recommendedAction := "no_action"
	if len(opportunities) > 0 {
		if totalSavings > 100 {
			recommendedAction = "immediate_action_recommended"
		} else if totalSavings > 50 {
			recommendedAction = "action_recommended"
		} else if totalSavings > 10 {
			recommendedAction = "consider_optimization"
		}
	}

	return &OptimizationReport{
		NodeGroupName:     nodeGroup.Name,
		Namespace:         nodeGroup.Namespace,
		CurrentCost:       *currentCost,
		Opportunities:     opportunities,
		PotentialSavings:  totalSavings,
		RecommendedAction: recommendedAction,
		GeneratedAt:       time.Now(),
	}, nil
}

// analyzeDownsizing checks if we can downsize to smaller instances
func (o *Optimizer) analyzeDownsizing(ctx context.Context, nodeGroup *v1alpha1.NodeGroup,
	currentCost *NodeGroupCost, utilization *UtilizationAnalysis) (*Opportunity, error) {

	// Get all available offerings
	offerings, err := o.client.ListOfferings(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list offerings: %w", err)
	}

	// Get current offering details
	var currentOffering *client.Offering
	for offeringID := range currentCost.InstanceTypes {
		for i := range offerings {
			if offerings[i].ID == offeringID {
				currentOffering = &offerings[i]
				break
			}
		}
		if currentOffering != nil {
			break
		}
	}

	if currentOffering == nil {
		return nil, fmt.Errorf("current offering not found")
	}

	// Find smaller offerings with similar specs but lower cost
	var candidates []client.Offering
	for _, offering := range offerings {
		// Must be smaller (lower price)
		if offering.Price >= currentOffering.Price {
			continue
		}

		// Must have at least 60% of current resources to handle peak
		minCPU := int(float64(currentOffering.CPU) * 0.6)
		minMemory := int(float64(currentOffering.RAM) * 0.6)

		if offering.CPU >= minCPU && offering.RAM >= minMemory && offering.Available {
			// Check if it meets utilization requirements
			cpuHeadroom := (float64(offering.CPU) / float64(currentOffering.CPU)) * 100
			memoryHeadroom := (float64(offering.RAM) / float64(currentOffering.RAM)) * 100

			// Ensure peak utilization won't exceed 85%
			projectedCPU := utilization.PeakUtilization.CPUPercent * (100 / cpuHeadroom)
			projectedMemory := utilization.PeakUtilization.MemoryPercent * (100 / memoryHeadroom)

			if projectedCPU < 85 && projectedMemory < 85 {
				candidates = append(candidates, offering)
			}
		}
	}

	if len(candidates) == 0 {
		return nil, nil // No downsizing opportunity
	}

	// Select the best candidate (cheapest that meets requirements)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Price < candidates[j].Price
	})

	bestCandidate := candidates[0]
	savings := (currentOffering.Price - bestCandidate.Price) * float64(currentCost.TotalNodes)

	return &Opportunity{
		Type:                OptimizationDownsize,
		Description:         fmt.Sprintf("Downsize from %s to %s", currentOffering.Name, bestCandidate.Name),
		CurrentOffering:     currentOffering.ID,
		RecommendedOffering: bestCandidate.ID,
		MonthlySavings:      savings,
		AnnualSavings:       savings * 12,
		ConfidenceScore:     0.8,
		Risk:                RiskLow,
		PerformanceImpact:   "Minimal - sufficient headroom for peak usage",
		Implementation:      "Gradually replace nodes one at a time with new instance type",
	}, nil
}

// analyzeRightSizing checks if we need to upsize or downsize based on utilization
func (o *Optimizer) analyzeRightSizing(ctx context.Context, nodeGroup *v1alpha1.NodeGroup,
	currentCost *NodeGroupCost, utilization *UtilizationAnalysis) (*Opportunity, error) {

	// Calculate resource requirements based on peak + 20% headroom
	targetCPUUsage := utilization.PeakUtilization.CPUPercent * 1.2
	targetMemoryUsage := utilization.PeakUtilization.MemoryPercent * 1.2

	// If current usage is within 60-80% range, no action needed
	if targetCPUUsage >= 60 && targetCPUUsage <= 80 &&
		targetMemoryUsage >= 60 && targetMemoryUsage <= 80 {
		return nil, nil
	}

	// Get current offering
	var currentOfferingID string
	for offeringID := range currentCost.InstanceTypes {
		currentOfferingID = offeringID
		break
	}

	currentOfferingCost, err := o.calculator.GetOfferingCost(ctx, currentOfferingID)
	if err != nil {
		return nil, err
	}

	// Calculate required resources
	requiredCPU := int(float64(currentOfferingCost.Specs.CPU) * (targetCPUUsage / 100))
	requiredMemory := int(float64(currentOfferingCost.Specs.MemoryMB) * (targetMemoryUsage / 100))

	// Find best match
	recommendation, err := o.calculator.FindCheapestOffering(ctx, ResourceRequirements{
		MinCPU:      requiredCPU,
		MinMemoryMB: requiredMemory,
		MinDiskGB:   currentOfferingCost.Specs.DiskGB,
	}, nodeGroup.Spec.OfferingIDs)

	if err != nil || recommendation.OfferingID == currentOfferingID {
		return nil, nil // No better option
	}

	newOfferingCost, err := o.calculator.GetOfferingCost(ctx, recommendation.OfferingID)
	if err != nil {
		return nil, err
	}

	savings := (currentOfferingCost.MonthlyCost - newOfferingCost.MonthlyCost) * float64(currentCost.TotalNodes)

	optimizationType := OptimizationRightSize
	if newOfferingCost.MonthlyCost > currentOfferingCost.MonthlyCost {
		optimizationType = OptimizationUpsize
		savings = -savings // Negative savings (cost increase)
	}

	risk := RiskLow
	if optimizationType == OptimizationUpsize {
		risk = RiskMedium
	}

	return &Opportunity{
		Type:                optimizationType,
		Description:         fmt.Sprintf("Right-size from %s to %s based on utilization patterns", currentOfferingCost.Name, newOfferingCost.Name),
		CurrentOffering:     currentOfferingID,
		RecommendedOffering: recommendation.OfferingID,
		MonthlySavings:      savings,
		AnnualSavings:       savings * 12,
		ConfidenceScore:     recommendation.Confidence,
		Risk:                risk,
		PerformanceImpact:   recommendation.PerformanceImpact,
		Implementation:      "Rolling update: replace nodes gradually to minimize disruption",
	}, nil
}

// analyzeCategoryOptimization checks if switching instance categories could save costs
func (o *Optimizer) analyzeCategoryOptimization(ctx context.Context, nodeGroup *v1alpha1.NodeGroup,
	currentCost *NodeGroupCost, utilization *UtilizationAnalysis) (*Opportunity, error) {

	if utilization == nil {
		return nil, nil
	}

	// Determine workload type
	avgCPU := utilization.AverageUtilization.CPUPercent
	avgMemory := utilization.AverageUtilization.MemoryPercent

	// If CPU-heavy, check compute-optimized instances
	// If memory-heavy, check memory-optimized instances
	targetCategory := ""
	if avgCPU > avgMemory*1.5 {
		targetCategory = "compute-optimized"
	} else if avgMemory > avgCPU*1.5 {
		targetCategory = "high-memory"
	} else {
		return nil, nil // Balanced workload, no category change needed
	}

	// Find offerings in target category
	offerings, err := o.client.ListOfferings(ctx, nil)
	if err != nil {
		return nil, err
	}

	var currentOfferingID string
	for offeringID := range currentCost.InstanceTypes {
		currentOfferingID = offeringID
		break
	}

	currentOffering, err := o.calculator.GetOfferingCost(ctx, currentOfferingID)
	if err != nil {
		return nil, err
	}

	// Find best match in target category
	var bestCandidate *client.Offering
	var bestSavings float64

	for i := range offerings {
		offering := &offerings[i]
		if offering.Category != targetCategory || !offering.Available {
			continue
		}

		// Must meet resource requirements
		if offering.CPU < currentOffering.Specs.CPU || offering.RAM < currentOffering.Specs.MemoryMB {
			continue
		}

		savings := (currentOffering.MonthlyCost - offering.Price) * float64(currentCost.TotalNodes)
		if savings > bestSavings {
			bestSavings = savings
			bestCandidate = offering
		}
	}

	if bestCandidate == nil || bestSavings <= 0 {
		return nil, nil
	}

	return &Opportunity{
		Type:                OptimizationChangeCategory,
		Description:         fmt.Sprintf("Switch to %s instance type for workload characteristics", targetCategory),
		CurrentOffering:     currentOfferingID,
		RecommendedOffering: bestCandidate.ID,
		MonthlySavings:      bestSavings,
		AnnualSavings:       bestSavings * 12,
		ConfidenceScore:     0.75,
		Risk:                RiskMedium,
		PerformanceImpact:   "May improve performance for workload type",
		Implementation:      "Test with single node first, then roll out gradually",
	}, nil
}

// analyzeConsolidation checks if we can consolidate onto fewer, larger nodes
func (o *Optimizer) analyzeConsolidation(ctx context.Context, nodeGroup *v1alpha1.NodeGroup,
	currentCost *NodeGroupCost, utilization *UtilizationAnalysis) (*Opportunity, error) {

	if currentCost.TotalNodes < 3 {
		return nil, nil // Need at least 3 nodes to consolidate
	}

	// Calculate total resources needed
	var currentOfferingID string
	for offeringID := range currentCost.InstanceTypes {
		currentOfferingID = offeringID
		break
	}

	currentOffering, err := o.calculator.GetOfferingCost(ctx, currentOfferingID)
	if err != nil {
		return nil, err
	}

	// Total resources across all nodes
	totalCPU := currentOffering.Specs.CPU * int(currentCost.TotalNodes)
	totalMemory := currentOffering.Specs.MemoryMB * int(currentCost.TotalNodes)

	// Account for utilization - we only need to support peak + 20%
	requiredCPU := int(float64(totalCPU) * (utilization.PeakUtilization.CPUPercent / 100) * 1.2)
	requiredMemory := int(float64(totalMemory) * (utilization.PeakUtilization.MemoryPercent / 100) * 1.2)

	// Find larger instances that can fit the workload
	offerings, err := o.client.ListOfferings(ctx, nil)
	if err != nil {
		return nil, err
	}

	type consolidationOption struct {
		offering  client.Offering
		nodeCount int32
		savings   float64
	}

	var options []consolidationOption

	for _, offering := range offerings {
		if !offering.Available {
			continue
		}

		// Prevent division by zero
		if offering.CPU == 0 || offering.RAM == 0 {
			continue
		}

		// Calculate how many nodes we'd need
		nodesForCPU := (requiredCPU + offering.CPU - 1) / offering.CPU
		nodesForMemory := (requiredMemory + offering.RAM - 1) / offering.RAM
		requiredNodes := nodesForCPU
		if nodesForMemory > requiredNodes {
			requiredNodes = nodesForMemory
		}

		// Must reduce node count
		if int32(requiredNodes) >= currentCost.TotalNodes {
			continue
		}

		// Calculate savings
		newCost := offering.Price * float64(requiredNodes)
		oldCost := currentCost.TotalMonthly
		savings := oldCost - newCost

		if savings > 0 {
			options = append(options, consolidationOption{
				offering:  offering,
				nodeCount: int32(requiredNodes),
				savings:   savings,
			})
		}
	}

	if len(options) == 0 {
		return nil, nil
	}

	// Select best option (most savings)
	sort.Slice(options, func(i, j int) bool {
		return options[i].savings > options[j].savings
	})

	best := options[0]

	// Higher risk with fewer nodes
	risk := RiskMedium
	if best.nodeCount == 1 {
		risk = RiskHigh
	}

	return &Opportunity{
		Type:                OptimizationConsolidateNodes,
		Description:         fmt.Sprintf("Consolidate from %d x %s to %d x %s nodes", currentCost.TotalNodes, currentOffering.Name, best.nodeCount, best.offering.Name),
		CurrentOffering:     currentOfferingID,
		RecommendedOffering: best.offering.ID,
		MonthlySavings:      best.savings,
		AnnualSavings:       best.savings * 12,
		ConfidenceScore:     0.7,
		Risk:                risk,
		PerformanceImpact:   "Fewer nodes may impact fault tolerance",
		Implementation:      "Gradually scale up new nodes, then drain and remove old nodes",
	}, nil
}

// SimulateOptimization simulates the impact of applying an optimization
func (o *Optimizer) SimulateOptimization(ctx context.Context, optimization *Optimization) (*SimulationResult, error) {
	if optimization == nil {
		return nil, fmt.Errorf("optimization cannot be nil")
	}

	// Get the recommended offering details
	newOffering, err := o.calculator.GetOfferingCost(ctx, optimization.Opportunity.RecommendedOffering)
	if err != nil {
		return nil, fmt.Errorf("failed to get new offering cost: %w", err)
	}

	// Calculate performance impact
	currentOffering, err := o.calculator.GetOfferingCost(ctx, optimization.Opportunity.CurrentOffering)
	if err != nil {
		return nil, fmt.Errorf("failed to get current offering cost: %w", err)
	}

	// Calculate percentage changes (prevent division by zero)
	var cpuChange, memoryChange, diskChange int
	if currentOffering.Specs.CPU > 0 {
		cpuChange = ((newOffering.Specs.CPU - currentOffering.Specs.CPU) * 100) / currentOffering.Specs.CPU
	}
	if currentOffering.Specs.MemoryMB > 0 {
		memoryChange = ((newOffering.Specs.MemoryMB - currentOffering.Specs.MemoryMB) * 100) / currentOffering.Specs.MemoryMB
	}
	if currentOffering.Specs.DiskGB > 0 {
		diskChange = ((newOffering.Specs.DiskGB - currentOffering.Specs.DiskGB) * 100) / currentOffering.Specs.DiskGB
	}

	impact := "negligible"
	if cpuChange < -20 || memoryChange < -20 {
		impact = "moderate"
	}
	if cpuChange < -40 || memoryChange < -40 {
		impact = "significant"
	}

	performanceImpact := PerformanceImpact{
		CPUChange:    cpuChange,
		MemoryChange: memoryChange,
		DiskChange:   diskChange,
		Impact:       impact,
	}

	// Create migration plan
	migrationPlan := &MigrationPlan{
		TotalNodes:    1, // Start with 1 node for testing
		NodesPerBatch: 1,
		EstimatedTime: 10 * time.Minute,
		RequiresDrain: true,
		RollbackPlan:  "Keep old nodes running until new nodes are verified healthy",
		Steps: []MigrationStep{
			{
				StepNumber:  1,
				Description: "Provision new node with recommended instance type",
				Duration:    5 * time.Minute,
			},
			{
				StepNumber:  2,
				Description: "Wait for node to join cluster and become ready",
				Duration:    3 * time.Minute,
			},
			{
				StepNumber:  3,
				Description: "Drain and remove one old node",
				Duration:    2 * time.Minute,
			},
		},
	}

	// Check constraints
	var violatedConstraints []string
	isViable := true

	// Check minimum savings constraint (assume $10 minimum)
	if optimization.Opportunity.MonthlySavings < 10 {
		violatedConstraints = append(violatedConstraints, "Monthly savings below minimum threshold ($10)")
		isViable = false
	}

	// Check performance impact
	if performanceImpact.Impact == "significant" {
		violatedConstraints = append(violatedConstraints, "Performance impact exceeds acceptable threshold")
		isViable = false
	}

	return &SimulationResult{
		Optimization:        optimization.Opportunity,
		EstimatedSavings:    optimization.Opportunity.MonthlySavings,
		EstimatedRisk:       optimization.Opportunity.Risk,
		PerformanceImpact:   performanceImpact,
		MigrationPlan:       migrationPlan,
		IsViable:            isViable,
		ViolatedConstraints: violatedConstraints,
		SimulatedAt:         time.Now(),
	}, nil
}

// RecommendInstanceType recommends the optimal instance type for given requirements
func (o *Optimizer) RecommendInstanceType(ctx context.Context, requirements ResourceRequirements) (*Recommendation, error) {
	return o.calculator.FindCheapestOffering(ctx, requirements, nil)
}
