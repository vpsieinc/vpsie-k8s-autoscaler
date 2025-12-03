package rebalancer

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	v1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Planner creates detailed migration plans for rebalancing operations
type Planner struct {
	config *PlannerConfig
}

// NewPlanner creates a new rebalance planner
func NewPlanner(config *PlannerConfig) *Planner {
	if config == nil {
		config = &PlannerConfig{
			BatchSize:        1,
			MaxConcurrent:    2,
			DrainTimeout:     5 * time.Minute,
			ProvisionTimeout: 10 * time.Minute,
		}
	}

	return &Planner{
		config: config,
	}
}

// CreateRebalancePlan creates a detailed plan for node rebalancing
func (p *Planner) CreateRebalancePlan(ctx context.Context, analysis *RebalanceAnalysis, nodeGroup *v1alpha1.NodeGroup) (*RebalancePlan, error) {
	logger := log.FromContext(ctx)
	logger.Info("Creating rebalance plan",
		"nodeGroup", analysis.NodeGroupName,
		"candidates", len(analysis.CandidateNodes))

	// Determine strategy from NodeGroup spec
	strategy := p.determineStrategy(nodeGroup)

	// Create batches of nodes
	batches, err := p.createBatches(analysis.CandidateNodes, strategy)
	if err != nil {
		return nil, fmt.Errorf("failed to create batches: %w", err)
	}

	// Estimate total duration
	totalDuration := p.estimateTotalDuration(batches, strategy)

	plan := &RebalancePlan{
		ID:                uuid.New().String(),
		NodeGroupName:     analysis.NodeGroupName,
		Namespace:         analysis.Namespace,
		Optimization:      analysis.Optimization,
		Batches:           batches,
		TotalNodes:        int32(len(analysis.CandidateNodes)),
		Strategy:          strategy,
		MaxConcurrent:     int32(p.config.MaxConcurrent),
		EstimatedDuration: totalDuration,
		CreatedAt:         time.Now(),
	}

	// Create rollback plan
	rollbackPlan, err := p.createRollbackPlan(plan)
	if err != nil {
		return nil, fmt.Errorf("failed to create rollback plan: %w", err)
	}
	plan.RollbackPlan = rollbackPlan

	logger.Info("Rebalance plan created",
		"planID", plan.ID,
		"strategy", strategy,
		"batches", len(batches),
		"estimatedDuration", totalDuration)

	return plan, nil
}

// BatchNodes groups nodes into batches for gradual replacement
func (p *Planner) BatchNodes(nodes []CandidateNode, batchSize int) ([]NodeBatch, error) {
	if batchSize <= 0 {
		batchSize = p.config.BatchSize
	}

	batches := make([]NodeBatch, 0)
	batchNumber := 0

	for i := 0; i < len(nodes); i += batchSize {
		end := i + batchSize
		if end > len(nodes) {
			end = len(nodes)
		}

		batchNodes := nodes[i:end]
		estimatedDuration := p.estimateBatchDuration(batchNodes)

		batch := NodeBatch{
			BatchNumber:       batchNumber,
			Nodes:             batchNodes,
			EstimatedDuration: estimatedDuration,
		}

		// Each batch depends on previous batch completing
		if batchNumber > 0 {
			batch.DependsOn = []int{batchNumber - 1}
		}

		batches = append(batches, batch)
		batchNumber++
	}

	return batches, nil
}

// CreateRollbackPlan defines how to revert if rebalancing fails
func (p *Planner) createRollbackPlan(plan *RebalancePlan) (*RollbackPlan, error) {
	rollback := &RollbackPlan{
		Steps:           make([]RollbackStep, 0),
		AutoRollback:    true,
		RollbackTimeout: 30 * time.Minute,
	}

	// Step 1: Pause execution
	rollback.Steps = append(rollback.Steps, RollbackStep{
		Order:       1,
		Description: "Pause rebalancing execution",
		Action:      "pause_execution",
	})

	// Step 2: Uncordon old nodes
	rollback.Steps = append(rollback.Steps, RollbackStep{
		Order:       2,
		Description: "Uncordon old nodes that were cordoned",
		Action:      "uncordon_old_nodes",
	})

	// Step 3: Terminate new nodes
	rollback.Steps = append(rollback.Steps, RollbackStep{
		Order:       3,
		Description: "Terminate newly provisioned nodes",
		Action:      "terminate_new_nodes",
	})

	// Step 4: Verify workloads
	rollback.Steps = append(rollback.Steps, RollbackStep{
		Order:       4,
		Description: "Verify workloads are running on old nodes",
		Action:      "verify_workloads",
	})

	// Step 5: Update status
	rollback.Steps = append(rollback.Steps, RollbackStep{
		Order:       5,
		Description: "Update NodeGroup status to reflect rollback",
		Action:      "update_status",
	})

	return rollback, nil
}

// Strategy-specific planning

// determineStrategy determines the best rebalancing strategy
func (p *Planner) determineStrategy(nodeGroup *v1alpha1.NodeGroup) RebalanceStrategy {
	// TODO: Add Rebalancing field to NodeGroup CRD spec to allow configuration
	// Check if NodeGroup has rebalancing configuration
	// if nodeGroup.Spec.Rebalancing != nil && nodeGroup.Spec.Rebalancing.Strategy != "" {
	// 	switch nodeGroup.Spec.Rebalancing.Strategy {
	// 	case "rolling":
	// 		return StrategyRolling
	// 	case "surge":
	// 		return StrategySurge
	// 	case "blue-green":
	// 		return StrategyBlueGreen
	// 	}
	// }

	// Default to rolling for safety
	return StrategyRolling
}

// createBatches creates batches based on strategy
func (p *Planner) createBatches(candidates []CandidateNode, strategy RebalanceStrategy) ([]NodeBatch, error) {
	switch strategy {
	case StrategyRolling:
		return p.createRollingBatches(candidates)
	case StrategySurge:
		return p.createSurgeBatches(candidates)
	case StrategyBlueGreen:
		return p.createBlueGreenBatches(candidates)
	default:
		return p.createRollingBatches(candidates)
	}
}

// createRollingBatches creates batches for rolling replacement
func (p *Planner) createRollingBatches(candidates []CandidateNode) ([]NodeBatch, error) {
	// Rolling: Replace nodes one-by-one or in small batches
	return p.BatchNodes(candidates, p.config.BatchSize)
}

// createSurgeBatches creates batches for surge replacement
func (p *Planner) createSurgeBatches(candidates []CandidateNode) ([]NodeBatch, error) {
	// Surge: Two batches - provision all new nodes, then drain all old nodes
	batches := make([]NodeBatch, 2)

	// Batch 0: Provision all new nodes (parallel)
	batches[0] = NodeBatch{
		BatchNumber:       0,
		Nodes:             candidates,
		EstimatedDuration: p.config.ProvisionTimeout,
		DependsOn:         []int{},
	}

	// Batch 1: Drain all old nodes (after new nodes are ready)
	batches[1] = NodeBatch{
		BatchNumber:       1,
		Nodes:             candidates,
		EstimatedDuration: p.config.DrainTimeout * time.Duration(len(candidates)),
		DependsOn:         []int{0},
	}

	return batches, nil
}

// createBlueGreenBatches creates batches for blue-green replacement
func (p *Planner) createBlueGreenBatches(candidates []CandidateNode) ([]NodeBatch, error) {
	// Blue-Green: Similar to surge but with explicit phases
	batches := make([]NodeBatch, 3)

	// Batch 0: Provision complete "green" set
	batches[0] = NodeBatch{
		BatchNumber:       0,
		Nodes:             candidates,
		EstimatedDuration: p.config.ProvisionTimeout * 2, // More time for complete set
		DependsOn:         []int{},
	}

	// Batch 1: Switch traffic (cordon blue nodes)
	batches[1] = NodeBatch{
		BatchNumber:       1,
		Nodes:             candidates,
		EstimatedDuration: 1 * time.Minute, // Quick cordon operation
		DependsOn:         []int{0},
	}

	// Batch 2: Drain and remove "blue" set
	batches[2] = NodeBatch{
		BatchNumber:       2,
		Nodes:             candidates,
		EstimatedDuration: p.config.DrainTimeout * time.Duration(len(candidates)),
		DependsOn:         []int{1},
	}

	return batches, nil
}

// Duration estimation

// estimateTotalDuration estimates total time for the plan
func (p *Planner) estimateTotalDuration(batches []NodeBatch, strategy RebalanceStrategy) time.Duration {
	var total time.Duration

	switch strategy {
	case StrategyRolling:
		// Sequential batches
		for _, batch := range batches {
			total += batch.EstimatedDuration
		}
	case StrategySurge, StrategyBlueGreen:
		// Some parallel, some sequential
		for _, batch := range batches {
			// Only add if batch has no dependencies or depends on completed batches
			total += batch.EstimatedDuration
		}
	}

	// Add 20% buffer for safety
	total = time.Duration(float64(total) * 1.2)

	return total
}

// estimateBatchDuration estimates time for a single batch
func (p *Planner) estimateBatchDuration(nodes []CandidateNode) time.Duration {
	// Base estimate: provision time + drain time + verify time
	baseTime := p.config.ProvisionTimeout + p.config.DrainTimeout + (1 * time.Minute)

	// Adjust for number of nodes in batch
	nodeCount := len(nodes)
	if nodeCount <= 1 {
		return baseTime
	}

	// If multiple nodes, assume some parallelism up to MaxConcurrent
	concurrent := p.config.MaxConcurrent
	if nodeCount < concurrent {
		concurrent = nodeCount
	}

	// Calculate parallel time
	parallelTime := baseTime * time.Duration(nodeCount) / time.Duration(concurrent)

	return parallelTime
}

// ValidatePlan validates that a plan is safe to execute
func (p *Planner) ValidatePlan(plan *RebalancePlan, nodeGroup *v1alpha1.NodeGroup) error {
	// Check batch dependencies are valid
	for _, batch := range plan.Batches {
		for _, dep := range batch.DependsOn {
			if dep >= batch.BatchNumber {
				return fmt.Errorf("batch %d depends on later batch %d", batch.BatchNumber, dep)
			}
			if dep < 0 || dep >= len(plan.Batches) {
				return fmt.Errorf("batch %d has invalid dependency %d", batch.BatchNumber, dep)
			}
		}
	}

	// Check that total nodes doesn't exceed limits
	if plan.TotalNodes > nodeGroup.Spec.MaxNodes {
		return fmt.Errorf("plan would create %d nodes but max is %d", plan.TotalNodes, nodeGroup.Spec.MaxNodes)
	}

	// Check strategy is valid
	validStrategies := map[RebalanceStrategy]bool{
		StrategyRolling:   true,
		StrategySurge:     true,
		StrategyBlueGreen: true,
	}

	if !validStrategies[plan.Strategy] {
		return fmt.Errorf("invalid strategy: %s", plan.Strategy)
	}

	// Check rollback plan exists
	if plan.RollbackPlan == nil {
		return fmt.Errorf("plan must include rollback plan")
	}

	return nil
}

// OptimizePlan optimizes a plan for efficiency
func (p *Planner) OptimizePlan(plan *RebalancePlan) (*RebalancePlan, error) {
	// Create a copy to avoid modifying original
	optimized := *plan

	// Optimization 1: Merge small batches if possible
	if len(plan.Batches) > 1 {
		optimized.Batches = p.mergeBatches(plan.Batches, p.config.MaxConcurrent)
	}

	// Optimization 2: Adjust timing based on node characteristics
	for i := range optimized.Batches {
		optimized.Batches[i].EstimatedDuration = p.adjustDurationForWorkloads(
			optimized.Batches[i],
		)
	}

	// Recalculate total duration
	optimized.EstimatedDuration = p.estimateTotalDuration(optimized.Batches, optimized.Strategy)

	return &optimized, nil
}

// mergeBatches merges small batches to reduce total time
func (p *Planner) mergeBatches(batches []NodeBatch, maxConcurrent int) []NodeBatch {
	merged := make([]NodeBatch, 0)
	currentBatch := NodeBatch{
		BatchNumber: 0,
		Nodes:       make([]CandidateNode, 0),
	}

	for _, batch := range batches {
		// If adding this batch would exceed max concurrent, start new batch
		if len(currentBatch.Nodes)+len(batch.Nodes) > maxConcurrent {
			if len(currentBatch.Nodes) > 0 {
				currentBatch.EstimatedDuration = p.estimateBatchDuration(currentBatch.Nodes)
				merged = append(merged, currentBatch)
			}
			currentBatch = NodeBatch{
				BatchNumber: len(merged),
				Nodes:       batch.Nodes,
			}
		} else {
			currentBatch.Nodes = append(currentBatch.Nodes, batch.Nodes...)
		}
	}

	// Add final batch
	if len(currentBatch.Nodes) > 0 {
		currentBatch.EstimatedDuration = p.estimateBatchDuration(currentBatch.Nodes)
		merged = append(merged, currentBatch)
	}

	return merged
}

// adjustDurationForWorkloads adjusts duration based on workload characteristics
func (p *Planner) adjustDurationForWorkloads(batch NodeBatch) time.Duration {
	duration := batch.EstimatedDuration

	// Count nodes with complex workloads
	complexWorkloads := 0
	for _, node := range batch.Nodes {
		for _, workload := range node.Workloads {
			if workload.HasPDB || workload.HasLocalStorage || workload.IsCritical {
				complexWorkloads++
			}
		}
	}

	// Add extra time for complex workloads (30 seconds per complex workload)
	if complexWorkloads > 0 {
		duration += time.Duration(complexWorkloads) * 30 * time.Second
	}

	return duration
}

// GetBatchByNumber retrieves a specific batch by number
func (p *Planner) GetBatchByNumber(plan *RebalancePlan, batchNumber int) (*NodeBatch, error) {
	for i := range plan.Batches {
		if plan.Batches[i].BatchNumber == batchNumber {
			return &plan.Batches[i], nil
		}
	}
	return nil, fmt.Errorf("batch %d not found in plan %s", batchNumber, plan.ID)
}

// CanExecuteBatch checks if a batch can be executed (dependencies satisfied)
func (p *Planner) CanExecuteBatch(plan *RebalancePlan, batchNumber int, completedBatches []int) (bool, error) {
	batch, err := p.GetBatchByNumber(plan, batchNumber)
	if err != nil {
		return false, err
	}

	// Check all dependencies are in completed list
	for _, dep := range batch.DependsOn {
		found := false
		for _, completed := range completedBatches {
			if dep == completed {
				found = true
				break
			}
		}
		if !found {
			return false, fmt.Errorf("batch %d depends on incomplete batch %d", batchNumber, dep)
		}
	}

	return true, nil
}
