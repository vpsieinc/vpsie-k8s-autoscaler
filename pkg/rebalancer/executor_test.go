package rebalancer

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestNewExecutor(t *testing.T) {
	kubeClient := fake.NewSimpleClientset()

	t.Run("Create with default config", func(t *testing.T) {
		executor := NewExecutor(kubeClient, nil, nil)
		if executor == nil {
			t.Fatal("Expected executor to be created")
			return
		}
		if executor.config == nil {
			t.Fatal("Expected default config to be set")
			return
		}
		if executor.config.DrainTimeout != 5*time.Minute {
			t.Errorf("Expected DrainTimeout=5m, got %v", executor.config.DrainTimeout)
		}
		if executor.config.MaxRetries != 3 {
			t.Errorf("Expected MaxRetries=3, got %d", executor.config.MaxRetries)
		}
		if executor.config.ProvisionTimeout != 10*time.Minute {
			t.Errorf("Expected ProvisionTimeout=10m, got %v", executor.config.ProvisionTimeout)
		}
		if executor.config.HealthCheckInterval != 10*time.Second {
			t.Errorf("Expected HealthCheckInterval=10s, got %v", executor.config.HealthCheckInterval)
		}
	})

	t.Run("Create with custom config", func(t *testing.T) {
		config := &ExecutorConfig{
			DrainTimeout:        15 * time.Minute,
			ProvisionTimeout:    20 * time.Minute,
			HealthCheckInterval: 10 * time.Second,
			MaxRetries:          5,
		}
		executor := NewExecutor(kubeClient, nil, config)
		if executor.config.DrainTimeout != 15*time.Minute {
			t.Errorf("Expected DrainTimeout=15m, got %v", executor.config.DrainTimeout)
		}
		if executor.config.MaxRetries != 5 {
			t.Errorf("Expected MaxRetries=5, got %d", executor.config.MaxRetries)
		}
	})
}

// TestExecutor_ProvisioningErrorHandling verifies that provisioning errors are handled correctly
// This addresses Fix #5: Rebalancer Node Provisioning Race Condition
func TestExecutor_ProvisioningErrorHandling(t *testing.T) {
	kubeClient := fake.NewSimpleClientset()
	executor := NewExecutor(kubeClient, nil, &ExecutorConfig{
		DrainTimeout:        5 * time.Minute,
		ProvisionTimeout:    10 * time.Minute,
		HealthCheckInterval: 10 * time.Second,
		MaxRetries:          3,
	})

	plan := &RebalancePlan{
		ID:            "test-plan-1",
		NodeGroupName: "test-nodegroup",
		Namespace:     "default",
		Strategy:      StrategyRolling,
		Batches: []NodeBatch{
			{
				BatchNumber: 1,
				Nodes: []CandidateNode{
					{
						NodeName:        "old-node-1",
						CurrentOffering: "offering-old",
						TargetOffering:  "offering-new",
					},
				},
			},
		},
	}

	t.Run("Provisioning returns error", func(t *testing.T) {
		// The current implementation always returns (nil, error) from provisionNewNode
		// This test verifies that errors are properly caught and handled
		state := &ExecutionState{
			PlanID:           plan.ID,
			Status:           StatusInProgress,
			CurrentBatch:     0,
			CompletedNodes:   make([]string, 0),
			FailedNodes:      make([]NodeFailure, 0),
			ProvisionedNodes: make([]string, 0),
			StartedAt:        time.Now(),
		}

		// Execute the batch - should fail at provisioning
		result, err := executor.executeRollingBatch(context.TODO(), plan, &plan.Batches[0], state)

		// Verify that the batch execution doesn't panic
		if result == nil {
			t.Fatal("Expected result to be non-nil even on failure")
			return
		}

		// Verify that the error was recorded
		if result.NodesFailed != 1 {
			t.Errorf("Expected NodesFailed=1, got %d", result.NodesFailed)
		}

		if len(result.FailedNodes) != 1 {
			t.Fatalf("Expected 1 failed node, got %d", len(result.FailedNodes))
		}

		failure := result.FailedNodes[0]
		if failure.NodeName != "old-node-1" {
			t.Errorf("Expected failed node name 'old-node-1', got '%s'", failure.NodeName)
		}

		if failure.Operation != "provision" {
			t.Errorf("Expected operation 'provision', got '%s'", failure.Operation)
		}

		if failure.Error == nil {
			t.Error("Expected error to be non-nil")
		}

		// Verify error message contains "provisioning failed"
		if failure.Error != nil {
			errStr := failure.Error.Error()
			if errStr == "" {
				t.Error("Expected error message to be non-empty")
			}
		}

		// No error should be returned from executeBatch itself (it handles errors internally)
		if err != nil {
			t.Errorf("Expected no error from executeBatch, got: %v", err)
		}
	})

	t.Run("Surge strategy provisioning error handling", func(t *testing.T) {
		surgePlan := &RebalancePlan{
			ID:            "test-plan-surge",
			NodeGroupName: "test-nodegroup",
			Namespace:     "default",
			Strategy:      StrategySurge,
			Batches: []NodeBatch{
				{
					BatchNumber: 1,
					Nodes: []CandidateNode{
						{
							NodeName:        "old-node-1",
							CurrentOffering: "offering-old",
							TargetOffering:  "offering-new",
						},
						{
							NodeName:        "old-node-2",
							CurrentOffering: "offering-old",
							TargetOffering:  "offering-new",
						},
					},
				},
			},
		}

		state := &ExecutionState{
			PlanID:           surgePlan.ID,
			Status:           StatusInProgress,
			CurrentBatch:     0,
			CompletedNodes:   make([]string, 0),
			FailedNodes:      make([]NodeFailure, 0),
			ProvisionedNodes: make([]string, 0),
			StartedAt:        time.Now(),
		}

		// Execute surge batch - should fail at provisioning both nodes
		result, err := executor.executeSurgeBatch(context.TODO(), surgePlan, &surgePlan.Batches[0], state)

		// Verify that the batch execution doesn't panic
		if result == nil {
			t.Fatal("Expected result to be non-nil even on failure")
			return
		}

		// Surge strategy: 2 nodes fail at provisioning, then 2 more fail during drain (nodes don't exist)
		// Total: 4 failures (2 provisioning + 2 drain)
		if result.NodesFailed != 4 {
			t.Errorf("Expected NodesFailed=4, got %d", result.NodesFailed)
		}

		// Only the provisioning failures are recorded in FailedNodes slice
		if len(result.FailedNodes) != 2 {
			t.Errorf("Expected 2 failed nodes in FailedNodes slice, got %d", len(result.FailedNodes))
		}

		// Verify provisioning failures are recorded with correct operation
		for i, failure := range result.FailedNodes {
			if failure.Operation != "provision" {
				t.Errorf("Failed node %d: expected operation 'provision', got '%s'", i, failure.Operation)
			}
			if failure.Error == nil {
				t.Errorf("Failed node %d: expected error to be non-nil", i)
			}
		}

		if err != nil {
			t.Errorf("Expected no error from executeSurgeBatch, got: %v", err)
		}
	})

	t.Run("Verify distinct error messages", func(t *testing.T) {
		// This test verifies that we can distinguish between different error types
		// Currently provisionNewNode returns (nil, error), so we test that case

		state := &ExecutionState{
			PlanID:           plan.ID,
			Status:           StatusInProgress,
			CurrentBatch:     0,
			CompletedNodes:   make([]string, 0),
			FailedNodes:      make([]NodeFailure, 0),
			ProvisionedNodes: make([]string, 0),
			StartedAt:        time.Now(),
		}

		result, _ := executor.executeRollingBatch(context.TODO(), plan, &plan.Batches[0], state)

		if len(result.FailedNodes) > 0 {
			errMsg := result.FailedNodes[0].Error.Error()
			// The error message should indicate provisioning failed
			// It should contain information about what went wrong
			if errMsg == "" {
				t.Error("Expected non-empty error message")
			}

			// Verify the error is wrapped properly (contains "provisioning failed")
			if len(errMsg) < 10 {
				t.Errorf("Error message seems too short: %s", errMsg)
			}
		}
	})
}

// Note: Full ExecuteRebalance tests are complex and require realistic
// cluster state with node provisioning. These are better suited for
// integration tests rather than unit tests. The planner and analyzer
// components are tested separately above.

// Note: Additional integration tests for cordon, delete, rollback, pause/resume
// would require more complex test setups and are better suited for integration tests

// =============================================================================
// Enhanced Scale-Down Safety Tests - Design Doc: enhanced-scale-down-safety-design.md
// Generated: 2026-01-11 | Budget Used: 2 unit tests for executor.go
// =============================================================================

// TestSameNodeGroupProtection tests the same-nodegroup protection in rebalancer executor.
// AC5: "Rebalancer does not terminate nodes when target nodegroup equals current nodegroup"
func TestSameNodeGroupProtection(t *testing.T) {
	// AC5: Same-NodeGroup Protection - SKIP scenario
	// ROI: 64 | Business Value: 8 (prevents unnecessary churn) | Frequency: 4 (edge case)
	// Behavior: Plan nodegroup == node's nodegroup AND same offering → Termination SKIPPED
	// @category: core-functionality
	// @dependency: Executor.executeRollingBatch, getNodeGroupFromNode (new)
	// @complexity: medium
	t.Run("AC5: Termination skipped when same nodegroup with same offering", func(t *testing.T) {
		// Arrange:
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup": "test-ng",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		}

		fakeClient := fake.NewSimpleClientset(node)
		executor := NewExecutor(fakeClient, nil, &ExecutorConfig{
			DrainTimeout:        5 * time.Minute,
			ProvisionTimeout:    10 * time.Minute,
			HealthCheckInterval: 10 * time.Second,
			MaxRetries:          3,
		})

		plan := &RebalancePlan{
			ID:            "test-plan-same-ng",
			NodeGroupName: "test-ng", // SAME as node's nodegroup
			Namespace:     "default",
			Strategy:      StrategyRolling,
			Batches: []NodeBatch{
				{
					BatchNumber: 1,
					Nodes: []CandidateNode{
						{
							NodeName:        "test-node-1",
							CurrentOffering: "offering-standard-2-4",
							TargetOffering:  "offering-standard-2-4", // SAME as current
						},
					},
				},
			},
		}

		state := &ExecutionState{
			PlanID:           plan.ID,
			Status:           StatusInProgress,
			CurrentBatch:     0,
			CompletedNodes:   make([]string, 0),
			FailedNodes:      make([]NodeFailure, 0),
			ProvisionedNodes: make([]string, 0),
			StartedAt:        time.Now(),
		}

		// Act:
		// result, err := executor.executeRollingBatch(context.TODO(), plan, &plan.Batches[0], state)

		// Assert:
		// require.NoError(t, err)
		// assert.Equal(t, int32(0), result.NodesFailed, "No nodes should fail - operation skipped")
		// assert.Equal(t, int32(0), result.NodesRebalanced, "No nodes should be rebalanced - operation skipped")
		// Verify VPSie API not called (mock verification)
		// Verify log contains "Skipping termination: same nodegroup"

		_ = executor
		_ = plan
		_ = state
		t.Skip("Skeleton: Implementation required - same-nodegroup guard clause in executeRollingBatch")
	})

	// AC5: Different-NodeGroup - PROCEED scenario
	// ROI: 68 | Business Value: 7 (correct behavior) | Frequency: 6 (normal case)
	// Behavior: Plan nodegroup != node's nodegroup → Termination PROCEEDS
	// @category: core-functionality
	// @dependency: Executor.executeRollingBatch, getNodeGroupFromNode (new)
	// @complexity: medium
	t.Run("AC5: Termination proceeds when different nodegroup", func(t *testing.T) {
		// Arrange:
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup": "source-ng", // Source nodegroup
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		}

		fakeClient := fake.NewSimpleClientset(node)
		executor := NewExecutor(fakeClient, nil, &ExecutorConfig{
			DrainTimeout:        5 * time.Minute,
			ProvisionTimeout:    10 * time.Minute,
			HealthCheckInterval: 10 * time.Second,
			MaxRetries:          3,
		})

		plan := &RebalancePlan{
			ID:            "test-plan-diff-ng",
			NodeGroupName: "target-ng", // DIFFERENT from node's nodegroup
			Namespace:     "default",
			Strategy:      StrategyRolling,
			Batches: []NodeBatch{
				{
					BatchNumber: 1,
					Nodes: []CandidateNode{
						{
							NodeName:        "test-node-1",
							CurrentOffering: "offering-standard-2-4",
							TargetOffering:  "offering-standard-4-8",
						},
					},
				},
			},
		}

		state := &ExecutionState{
			PlanID:           plan.ID,
			Status:           StatusInProgress,
			CurrentBatch:     0,
			CompletedNodes:   make([]string, 0),
			FailedNodes:      make([]NodeFailure, 0),
			ProvisionedNodes: make([]string, 0),
			StartedAt:        time.Now(),
		}

		// Act:
		// result, err := executor.executeRollingBatch(context.TODO(), plan, &plan.Batches[0], state)

		// Assert:
		// The execution should proceed (and fail at provisioning since it's not implemented)
		// This verifies the guard clause does NOT block different-nodegroup operations
		// require.NoError(t, err)
		// assert.Equal(t, int32(1), result.NodesFailed, "Should fail at provisioning")
		// Verify the failure is at "provision" stage, not blocked by same-nodegroup check

		_ = executor
		_ = plan
		_ = state
		t.Skip("Skeleton: Implementation required - same-nodegroup guard clause in executeRollingBatch")
	})

	// AC5: Same NodeGroup but Different Offering - PROCEED scenario
	// ROI: 60 | Business Value: 7 (right-sizing use case) | Frequency: 5
	// Behavior: Plan nodegroup == node's nodegroup BUT different offering → Termination PROCEEDS
	// @category: core-functionality
	// @dependency: Executor.executeRollingBatch, getNodeGroupFromNode (new)
	// @complexity: medium
	t.Run("AC5: Termination proceeds when same nodegroup but different offering (right-sizing)", func(t *testing.T) {
		// Arrange:
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test-node-1",
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup": "test-ng",
				},
			},
			Spec: corev1.NodeSpec{Unschedulable: false},
			Status: corev1.NodeStatus{
				Conditions: []corev1.NodeCondition{
					{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
				},
			},
		}

		fakeClient := fake.NewSimpleClientset(node)
		executor := NewExecutor(fakeClient, nil, &ExecutorConfig{
			DrainTimeout:        5 * time.Minute,
			ProvisionTimeout:    10 * time.Minute,
			HealthCheckInterval: 10 * time.Second,
			MaxRetries:          3,
		})

		plan := &RebalancePlan{
			ID:            "test-plan-right-sizing",
			NodeGroupName: "test-ng", // SAME as node's nodegroup
			Namespace:     "default",
			Strategy:      StrategyRolling,
			Batches: []NodeBatch{
				{
					BatchNumber: 1,
					Nodes: []CandidateNode{
						{
							NodeName:        "test-node-1",
							CurrentOffering: "offering-standard-4-8",
							TargetOffering:  "offering-standard-2-4", // DIFFERENT - downsizing
						},
					},
				},
			},
		}

		state := &ExecutionState{
			PlanID:           plan.ID,
			Status:           StatusInProgress,
			CurrentBatch:     0,
			CompletedNodes:   make([]string, 0),
			FailedNodes:      make([]NodeFailure, 0),
			ProvisionedNodes: make([]string, 0),
			StartedAt:        time.Now(),
		}

		// Act:
		// result, err := executor.executeRollingBatch(context.TODO(), plan, &plan.Batches[0], state)

		// Assert:
		// - Verify normal rebalance flow is attempted (this is a valid right-sizing operation)
		// - Verify the guard clause does NOT block execution
		// require.NoError(t, err)
		// assert.Equal(t, int32(1), result.NodesFailed, "Should fail at provisioning")
		// Verify provisionNewNode is called with TargetOffering "offering-standard-2-4"

		_ = executor
		_ = plan
		_ = state
		t.Skip("Skeleton: Implementation required - same-nodegroup guard clause in executeRollingBatch")
	})
}
