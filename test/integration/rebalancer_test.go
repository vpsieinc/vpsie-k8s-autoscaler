//go:build integration
// +build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/rebalancer"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/cost"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// TestRebalancerAnalyzerIntegration tests the rebalancer analyzer with real cluster state
func TestRebalancerAnalyzerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start mock VPSie server
	mockServer := NewMockVPSieServer()
	mockServer.Start()
	defer mockServer.Stop()

	// Create VPSie client
	vpsieClient, err := client.NewClientWithCredentials(
		mockServer.URL,
		"test-token",
		nil,
	)
	require.NoError(t, err, "Failed to create VPSie client")
	defer vpsieClient.Close()

	// Create cost optimizer
	calculator := cost.NewCalculator(vpsieClient)
	optimizer := cost.NewOptimizer(calculator, vpsieClient)

	t.Run("Analyze cluster health for rebalancing", func(t *testing.T) {
		analyzer := rebalancer.NewAnalyzer(clientset, optimizer, nil)

		// Create test NodeGroup
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "analyzer-test-ng",
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     2,
				MaxNodes:     10,
				DatacenterID: "dc1",
				OfferingIDs:  []string{"offering-standard-2-4", "offering-standard-4-8"},
			},
		}

		err := k8sClient.Create(ctx, nodeGroup)
		require.NoError(t, err, "Failed to create NodeGroup")
		defer k8sClient.Delete(ctx, nodeGroup)

		// Analyze the NodeGroup
		analysis, err := analyzer.AnalyzeNodeGroup(ctx, nodeGroup)
		require.NoError(t, err, "Failed to analyze NodeGroup")
		assert.NotNil(t, analysis, "Analysis should not be nil")
		assert.Equal(t, nodeGroup.Name, analysis.NodeGroupName, "NodeGroup name should match")
		assert.NotNil(t, analysis.SafetyChecks, "Safety checks should not be nil")
	})

	t.Run("Respect PodDisruptionBudgets during analysis", func(t *testing.T) {
		analyzer := rebalancer.NewAnalyzer(clientset, optimizer, &rebalancer.AnalyzerConfig{
			RespectPDBs: true,
		})

		// Create test namespace for this test
		testNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "pdb-test",
			},
		}
		err := k8sClient.Create(ctx, testNS)
		if err != nil {
			require.True(t, k8serrors.IsAlreadyExists(err), "Namespace creation failed with non-exists error")
		}
		defer k8sClient.Delete(ctx, testNS)

		// Create PDB
		minAvailable := intstr.FromInt(2)
		pdb := &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pdb",
				Namespace: "pdb-test",
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable: &minAvailable,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "test-app",
					},
				},
			},
		}
		err = k8sClient.Create(ctx, pdb)
		require.NoError(t, err, "Failed to create PDB")
		defer k8sClient.Delete(ctx, pdb)

		// Create NodeGroup
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "pdb-test-ng",
				Namespace: "pdb-test",
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     3,
				MaxNodes:     10,
				DatacenterID: "dc1",
				OfferingIDs:  []string{"offering-standard-2-4"},
			},
		}

		err = k8sClient.Create(ctx, nodeGroup)
		require.NoError(t, err, "Failed to create NodeGroup")
		defer k8sClient.Delete(ctx, nodeGroup)

		// Analyze should respect PDB
		analysis, err := analyzer.AnalyzeNodeGroup(ctx, nodeGroup)
		require.NoError(t, err, "Failed to analyze NodeGroup with PDB")
		assert.NotNil(t, analysis, "Analysis should not be nil")

		// Verify PDB was considered in safety checks
		pdbCheckFound := false
		for _, check := range analysis.SafetyChecks {
			if check.Category == rebalancer.SafetyCheckPodDisruption {
				pdbCheckFound = true
				break
			}
		}
		assert.True(t, pdbCheckFound, "PDB safety check should be performed")
	})

	t.Run("Skip nodes with local storage when configured", func(t *testing.T) {
		analyzer := rebalancer.NewAnalyzer(clientset, optimizer, &rebalancer.AnalyzerConfig{
			SkipNodesWithLocalStorage: true,
		})

		// Create NodeGroup
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-storage-test-ng",
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     2,
				MaxNodes:     10,
				DatacenterID: "dc1",
				OfferingIDs:  []string{"offering-standard-2-4"},
			},
		}

		err := k8sClient.Create(ctx, nodeGroup)
		require.NoError(t, err, "Failed to create NodeGroup")
		defer k8sClient.Delete(ctx, nodeGroup)

		// Analyze
		analysis, err := analyzer.AnalyzeNodeGroup(ctx, nodeGroup)
		require.NoError(t, err, "Failed to analyze NodeGroup")
		assert.NotNil(t, analysis, "Analysis should not be nil")

		// Nodes with local storage should not be in candidates
		for _, candidate := range analysis.CandidateNodes {
			assert.False(t, candidate.HasLocalStorage, "Nodes with local storage should be skipped")
		}
	})

	t.Run("Maintenance window scheduling", func(t *testing.T) {
		// Configure analyzer with maintenance window
		now := time.Now()
		currentDay := now.Weekday().String()

		analyzer := rebalancer.NewAnalyzer(clientset, optimizer, &rebalancer.AnalyzerConfig{
			MaintenanceWindows: []rebalancer.MaintenanceWindow{
				{
					Start: "00:00",
					End:   "23:59",
					Days:  []string{currentDay}, // Today
				},
			},
		})

		// Create NodeGroup
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "maintenance-test-ng",
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     1,
				MaxNodes:     5,
				DatacenterID: "dc1",
				OfferingIDs:  []string{"offering-standard-2-4"},
			},
		}

		err := k8sClient.Create(ctx, nodeGroup)
		require.NoError(t, err, "Failed to create NodeGroup")
		defer k8sClient.Delete(ctx, nodeGroup)

		// Analyze during maintenance window
		analysis, err := analyzer.AnalyzeNodeGroup(ctx, nodeGroup)
		require.NoError(t, err, "Failed to analyze NodeGroup")
		assert.NotNil(t, analysis, "Analysis should not be nil")

		// Should pass timing safety check since we're in the window
		timingCheckPassed := false
		for _, check := range analysis.SafetyChecks {
			if check.Category == rebalancer.SafetyCheckTiming && check.Status == rebalancer.SafetyCheckPassed {
				timingCheckPassed = true
				break
			}
		}
		assert.True(t, timingCheckPassed, "Timing check should pass during maintenance window")
	})
}

// TestRebalancerPlannerIntegration tests the rebalancer planner
func TestRebalancerPlannerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	t.Run("Create rebalance plan with batching", func(t *testing.T) {
		planner := rebalancer.NewPlanner(&rebalancer.PlannerConfig{
			BatchSize:     1,
			MaxConcurrent: 2,
		})

		// Create NodeGroup
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "planner-test-ng",
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     3,
				MaxNodes:     10,
				DatacenterID: "dc1",
				OfferingIDs:  []string{"offering-standard-2-4"},
			},
		}

		// Create analysis with multiple candidates
		analysis := &rebalancer.RebalanceAnalysis{
			NodeGroupName: nodeGroup.Name,
			Namespace:     nodeGroup.Namespace,
			TotalNodes:    5,
			CandidateNodes: []rebalancer.CandidateNode{
				{
					NodeName:        "node-1",
					CurrentOffering: "offering-standard-4-8",
					TargetOffering:  "offering-standard-2-4",
					SafeToRebalance: true,
				},
				{
					NodeName:        "node-2",
					CurrentOffering: "offering-standard-4-8",
					TargetOffering:  "offering-standard-2-4",
					SafeToRebalance: true,
				},
				{
					NodeName:        "node-3",
					CurrentOffering: "offering-standard-4-8",
					TargetOffering:  "offering-standard-2-4",
					SafeToRebalance: true,
				},
			},
			Optimization: &cost.Opportunity{
				Type:           cost.OptimizationRightSize,
				MonthlySavings: 150.0,
			},
		}

		// Create plan
		plan, err := planner.CreateRebalancePlan(ctx, analysis, nodeGroup)
		require.NoError(t, err, "Failed to create rebalance plan")
		assert.NotNil(t, plan, "Plan should not be nil")
		assert.Equal(t, nodeGroup.Name, plan.NodeGroupName, "NodeGroup name should match")
		assert.NotEmpty(t, plan.Batches, "Plan should have batches")
		assert.Equal(t, rebalancer.StrategyRolling, plan.Strategy, "Should use rolling strategy")
		assert.NotNil(t, plan.RollbackPlan, "Should have rollback plan")
	})

	t.Run("Plan with estimated duration", func(t *testing.T) {
		planner := rebalancer.NewPlanner(&rebalancer.PlannerConfig{
			BatchSize:        2,
			DrainTimeout:     5 * time.Minute,
			ProvisionTimeout: 10 * time.Minute,
		})

		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "duration-test-ng",
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     2,
				MaxNodes:     10,
				DatacenterID: "dc1",
				OfferingIDs:  []string{"offering-standard-2-4"},
			},
		}

		analysis := &rebalancer.RebalanceAnalysis{
			NodeGroupName: nodeGroup.Name,
			Namespace:     nodeGroup.Namespace,
			TotalNodes:    4,
			CandidateNodes: []rebalancer.CandidateNode{
				{NodeName: "node-1", SafeToRebalance: true},
				{NodeName: "node-2", SafeToRebalance: true},
			},
			Optimization: &cost.Opportunity{
				Type:           cost.OptimizationRightSize,
				MonthlySavings: 100.0,
			},
		}

		plan, err := planner.CreateRebalancePlan(ctx, analysis, nodeGroup)
		require.NoError(t, err, "Failed to create plan")
		assert.NotNil(t, plan, "Plan should not be nil")
		assert.Greater(t, plan.EstimatedDuration, time.Duration(0), "Should have estimated duration")
	})
}

// TestRebalancerExecutorIntegration tests the executor (basic tests only)
func TestRebalancerExecutorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start mock VPSie server
	mockServer := NewMockVPSieServer()
	mockServer.Start()
	defer mockServer.Stop()

	// Create VPSie client
	vpsieClient, err := client.NewClientWithCredentials(
		mockServer.URL,
		"test-token",
		nil,
	)
	require.NoError(t, err, "Failed to create VPSie client")
	defer vpsieClient.Close()

	t.Run("Create executor with configuration", func(t *testing.T) {
		executor := rebalancer.NewExecutor(clientset, vpsieClient, &rebalancer.ExecutorConfig{
			DrainTimeout:        5 * time.Minute,
			ProvisionTimeout:    10 * time.Minute,
			HealthCheckInterval: 10 * time.Second,
			MaxRetries:          3,
		})

		assert.NotNil(t, executor, "Executor should be created")
	})

	// Note: Full execution tests require complex setup with actual nodes
	// and are better suited for E2E tests
}

// TestRebalancerMetricsIntegration tests that rebalancing metrics are updated
func TestRebalancerMetricsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	t.Run("Verify rebalancing events are created", func(t *testing.T) {
		// Create NodeGroup
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "events-test-ng",
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     1,
				MaxNodes:     5,
				DatacenterID: "dc1",
				OfferingIDs:  []string{"offering-standard-2-4"},
			},
		}

		err := k8sClient.Create(ctx, nodeGroup)
		require.NoError(t, err, "Failed to create NodeGroup")
		defer k8sClient.Delete(ctx, nodeGroup)

		// Wait a bit for potential events
		time.Sleep(2 * time.Second)

		// Query events related to the NodeGroup
		events, err := clientset.CoreV1().Events(testNamespace).List(ctx, metav1.ListOptions{
			FieldSelector: "involvedObject.name=" + nodeGroup.Name,
		})
		require.NoError(t, err, "Failed to list events")

		// Note: In a full test, we would trigger rebalancing and verify events
		// For now, we just verify we can query events
		assert.NotNil(t, events, "Events list should not be nil")
	})
}

// TestRebalancerEndToEnd tests a complete rebalancing workflow
func TestRebalancerEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start mock VPSie server
	mockServer := NewMockVPSieServer()
	mockServer.Start()
	defer mockServer.Stop()

	// Create VPSie client
	vpsieClient, err := client.NewClientWithCredentials(
		mockServer.URL,
		"test-token",
		nil,
	)
	require.NoError(t, err, "Failed to create VPSie client")
	defer vpsieClient.Close()

	t.Run("Complete rebalancing workflow", func(t *testing.T) {
		// Create cost optimizer
		calculator := cost.NewCalculator(vpsieClient)
		optimizer := cost.NewOptimizer(calculator, vpsieClient)

		// Create analyzer
		analyzer := rebalancer.NewAnalyzer(clientset, optimizer, nil)

		// Create planner
		planner := rebalancer.NewPlanner(nil)

		// Create NodeGroup
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "e2e-test-ng",
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     2,
				MaxNodes:     10,
				DatacenterID: "dc1",
				OfferingIDs:  []string{"offering-standard-2-4", "offering-standard-4-8"},
			},
		}

		err := k8sClient.Create(ctx, nodeGroup)
		require.NoError(t, err, "Failed to create NodeGroup")
		defer k8sClient.Delete(ctx, nodeGroup)

		// Step 1: Analyze
		analysis, err := analyzer.AnalyzeNodeGroup(ctx, nodeGroup)
		require.NoError(t, err, "Failed to analyze NodeGroup")
		assert.NotNil(t, analysis, "Analysis should not be nil")

		// Step 2: Create plan (if there are candidates)
		if len(analysis.CandidateNodes) > 0 && analysis.RecommendedAction == rebalancer.ActionProceed {
			plan, err := planner.CreateRebalancePlan(ctx, analysis, nodeGroup)
			require.NoError(t, err, "Failed to create plan")
			assert.NotNil(t, plan, "Plan should not be nil")
			assert.NotEmpty(t, plan.ID, "Plan should have ID")

			// Step 3: Executor would execute the plan
			// (Skipped in integration tests - requires real nodes)
		}
	})
}
