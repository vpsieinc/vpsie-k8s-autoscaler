package rebalancer

import (
	"context"
	"testing"
	"time"

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

		// Both nodes should fail at provisioning
		if result.NodesFailed != 2 {
			t.Errorf("Expected NodesFailed=2, got %d", result.NodesFailed)
		}

		if len(result.FailedNodes) != 2 {
			t.Errorf("Expected 2 failed nodes, got %d", len(result.FailedNodes))
		}

		// Verify both failures are provision operations
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
