package rebalancer

import (
	"context"
	"testing"
	"time"

	v1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/cost"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNewPlanner(t *testing.T) {
	t.Run("Create with default config", func(t *testing.T) {
		planner := NewPlanner(nil)
		if planner == nil {
			t.Fatal("Expected planner to be created")
			return
		}
		if planner.config == nil {
			t.Fatal("Expected default config to be set")
			return
		}
		if planner.config.BatchSize != 1 {
			t.Errorf("Expected BatchSize=1, got %d", planner.config.BatchSize)
		}
		if planner.config.MaxConcurrent != 2 {
			t.Errorf("Expected MaxConcurrent=2, got %d", planner.config.MaxConcurrent)
		}
		if planner.config.DrainTimeout != 5*time.Minute {
			t.Errorf("Expected DrainTimeout=5m, got %v", planner.config.DrainTimeout)
		}
		if planner.config.ProvisionTimeout != 10*time.Minute {
			t.Errorf("Expected ProvisionTimeout=10m, got %v", planner.config.ProvisionTimeout)
		}
	})

	t.Run("Create with custom config", func(t *testing.T) {
		config := &PlannerConfig{
			BatchSize:        3,
			MaxConcurrent:    5,
			DrainTimeout:     15 * time.Minute,
			ProvisionTimeout: 20 * time.Minute,
		}
		planner := NewPlanner(config)
		if planner.config.BatchSize != 3 {
			t.Errorf("Expected BatchSize=3, got %d", planner.config.BatchSize)
		}
		if planner.config.MaxConcurrent != 5 {
			t.Errorf("Expected MaxConcurrent=5, got %d", planner.config.MaxConcurrent)
		}
	})
}

func TestCreateRebalancePlan(t *testing.T) {
	ctx := context.Background()
	planner := NewPlanner(nil)
	nodeGroup := createTestNodeGroup("test-ng", "default")

	t.Run("Create plan with single candidate", func(t *testing.T) {
		analysis := &RebalanceAnalysis{
			NodeGroupName: "test-ng",
			Namespace:     "default",
			TotalNodes:    3,
			CandidateNodes: []CandidateNode{
				{
					NodeName:        "node-1",
					CurrentOffering: "offering-1",
					TargetOffering:  "offering-2",
					SafeToRebalance: true,
				},
			},
			Optimization: &cost.Opportunity{
				Type:           cost.OptimizationRightSize,
				MonthlySavings: 50,
			},
		}

		plan, err := planner.CreateRebalancePlan(ctx, analysis, nodeGroup)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if plan == nil {
			t.Fatal("Expected plan to be created")
			return
		}
		if plan.NodeGroupName != "test-ng" {
			t.Errorf("Expected NodeGroupName='test-ng', got %s", plan.NodeGroupName)
		}
		if len(plan.Batches) == 0 {
			t.Error("Expected at least one batch")
		}
		if plan.Strategy != StrategyRolling {
			t.Errorf("Expected StrategyRolling, got %s", plan.Strategy)
		}
		if plan.RollbackPlan == nil {
			t.Error("Expected rollback plan to be created")
		}
	})

	t.Run("Create plan with multiple candidates", func(t *testing.T) {
		analysis := &RebalanceAnalysis{
			NodeGroupName: "test-ng",
			Namespace:     "default",
			TotalNodes:    5,
			CandidateNodes: []CandidateNode{
				{NodeName: "node-1", SafeToRebalance: true},
				{NodeName: "node-2", SafeToRebalance: true},
				{NodeName: "node-3", SafeToRebalance: true},
			},
			Optimization: &cost.Opportunity{
				Type:           cost.OptimizationRightSize,
				MonthlySavings: 150,
			},
		}

		plan, err := planner.CreateRebalancePlan(ctx, analysis, nodeGroup)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(plan.Batches) == 0 {
			t.Fatal("Expected batches to be created")
		}

		totalNodes := 0
		for _, batch := range plan.Batches {
			totalNodes += len(batch.Nodes)
		}
		if totalNodes != 3 {
			t.Errorf("Expected 3 total nodes across batches, got %d", totalNodes)
		}
	})

	t.Run("No candidates to rebalance", func(t *testing.T) {
		analysis := &RebalanceAnalysis{
			NodeGroupName:  "test-ng",
			Namespace:      "default",
			TotalNodes:     3,
			CandidateNodes: []CandidateNode{},
			Optimization:   &cost.Opportunity{},
		}

		plan, err := planner.CreateRebalancePlan(ctx, analysis, nodeGroup)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if plan == nil {
			t.Fatal("Expected plan to be created")
			return
		}
		if len(plan.Batches) != 0 {
			t.Errorf("Expected 0 batches for no candidates, got %d", len(plan.Batches))
		}
	})
}

func TestDetermineStrategy(t *testing.T) {
	planner := NewPlanner(nil)

	t.Run("Default to rolling strategy", func(t *testing.T) {
		nodeGroup := createTestNodeGroup("test-ng", "default")
		strategy := planner.determineStrategy(nodeGroup)
		if strategy != StrategyRolling {
			t.Errorf("Expected StrategyRolling, got %s", strategy)
		}
	})
}

// Note: createBatches, prioritizeNodes, createRollbackPlan, and estimateDuration
// are private methods and are tested indirectly through CreateRebalancePlan

// Helper function
func createTestNodeGroup(name, namespace string) *v1alpha1.NodeGroup {
	return &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes:          1,
			MaxNodes:          10,
			DatacenterID:      "dc-1",
			OfferingIDs:       []string{"offering-1", "offering-2"},
			OSImageID:         "ubuntu-22.04",
			KubernetesVersion: "v1.28.0",
		},
	}
}
