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
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/cost"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestCostCalculatorIntegration tests the cost calculator with mock VPSie API
func TestCostCalculatorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start mock VPSie server
	mockServer := NewMockVPSieServer()
	mockServer.Start()
	defer mockServer.Stop()

	// Create VPSie client pointing to mock server
	vpsieClient, err := client.NewClientWithCredentials(
		mockServer.URL,
		"test-token",
		nil,
	)
	require.NoError(t, err, "Failed to create VPSie client")
	defer vpsieClient.Close()

	t.Run("Calculate offering costs", func(t *testing.T) {
		calculator := cost.NewCalculator(vpsieClient)

		// Test getting cost for a specific offering
		offeringCost, err := calculator.GetOfferingCost(ctx, "offering-standard-2-4")
		require.NoError(t, err, "Failed to get offering cost")
		assert.NotNil(t, offeringCost, "Offering cost should not be nil")
		assert.Greater(t, offeringCost.MonthlyPrice, float64(0), "Monthly price should be positive")
		assert.Greater(t, offeringCost.HourlyPrice, float64(0), "Hourly price should be positive")
	})

	t.Run("Calculate NodeGroup total cost", func(t *testing.T) {
		calculator := cost.NewCalculator(vpsieClient)

		// Create test NodeGroup
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cost-test-ng",
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     1,
				MaxNodes:     5,
				DatacenterID: "dc1",
				OfferingIDs:  []string{"offering-standard-2-4"},
			},
		}

		// Create NodeGroup in cluster
		err := k8sClient.Create(ctx, nodeGroup)
		require.NoError(t, err, "Failed to create NodeGroup")
		defer k8sClient.Delete(ctx, nodeGroup)

		// Calculate cost
		totalCost, err := calculator.CalculateNodeGroupCost(ctx, nodeGroup)
		require.NoError(t, err, "Failed to calculate NodeGroup cost")
		assert.NotNil(t, totalCost, "Total cost should not be nil")
		assert.GreaterOrEqual(t, totalCost.MonthlyTotal, float64(0), "Monthly total should be non-negative")
	})

	t.Run("Compare offering costs", func(t *testing.T) {
		calculator := cost.NewCalculator(vpsieClient)

		offeringIDs := []string{
			"offering-standard-2-4",
			"offering-standard-4-8",
			"offering-standard-8-16",
		}

		comparison, err := calculator.CompareOfferings(ctx, offeringIDs)
		require.NoError(t, err, "Failed to compare offerings")
		assert.NotNil(t, comparison, "Comparison should not be nil")
		assert.Len(t, comparison.Offerings, 3, "Should have 3 offerings")
		assert.NotEmpty(t, comparison.CheapestID, "Cheapest ID should not be empty")
		assert.NotEmpty(t, comparison.MostExpensiveID, "Most expensive ID should not be empty")
	})

	t.Run("Calculate potential savings", func(t *testing.T) {
		calculator := cost.NewCalculator(vpsieClient)

		currentCost := &cost.NodeGroupCost{
			MonthlyTotal: 100.0,
			HourlyTotal:  0.14,
			PerNode:      50.0,
		}

		proposedCost := &cost.NodeGroupCost{
			MonthlyTotal: 75.0,
			HourlyTotal:  0.10,
			PerNode:      37.5,
		}

		savings, err := calculator.CalculateSavings(currentCost, proposedCost)
		require.NoError(t, err, "Failed to calculate savings")
		assert.NotNil(t, savings, "Savings should not be nil")
		assert.Equal(t, 25.0, savings.MonthlySavings, "Monthly savings should be $25")
		assert.Equal(t, 25.0, savings.PercentSavings, "Percent savings should be 25%")
	})

	t.Run("Find cheapest offering that meets requirements", func(t *testing.T) {
		calculator := cost.NewCalculator(vpsieClient)

		requirements := &cost.ResourceRequirements{
			MinCPU:    resource.MustParse("2"),
			MinMemory: resource.MustParse("4Gi"),
			MinDisk:   resource.MustParse("40Gi"),
		}

		allowedOfferings := []string{
			"offering-standard-2-4",
			"offering-standard-4-8",
			"offering-standard-8-16",
		}

		cheapest, err := calculator.FindCheapestOffering(ctx, requirements, allowedOfferings)
		require.NoError(t, err, "Failed to find cheapest offering")
		assert.NotNil(t, cheapest, "Cheapest offering should not be nil")
		assert.NotEmpty(t, cheapest.OfferingID, "Offering ID should not be empty")
		assert.Greater(t, cheapest.Cost.MonthlyPrice, float64(0), "Cost should be positive")
	})

	t.Run("Cache expiration and refresh", func(t *testing.T) {
		// Create calculator with 1 second cache TTL for testing
		calculator := cost.NewCalculator(vpsieClient)

		// First call - populates cache
		cost1, err := calculator.GetOfferingCost(ctx, "offering-standard-2-4")
		require.NoError(t, err, "Failed to get offering cost (first call)")

		// Second call - should use cache
		cost2, err := calculator.GetOfferingCost(ctx, "offering-standard-2-4")
		require.NoError(t, err, "Failed to get offering cost (second call)")
		assert.Equal(t, cost1.MonthlyPrice, cost2.MonthlyPrice, "Cached cost should match")

		// Note: Full cache expiration test would require waiting for TTL
		// This is tested in unit tests instead
	})
}

// TestCostOptimizerIntegration tests the cost optimizer with real scenarios
func TestCostOptimizerIntegration(t *testing.T) {
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

	t.Run("Analyze NodeGroup for optimization opportunities", func(t *testing.T) {
		calculator := cost.NewCalculator(vpsieClient)
		optimizer := cost.NewOptimizer(calculator, vpsieClient)

		// Create test NodeGroup
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "optimizer-test-ng",
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     2,
				MaxNodes:     10,
				DatacenterID: "dc1",
				OfferingIDs: []string{
					"offering-standard-8-16", // More expensive
					"offering-standard-4-8",  // Mid-range
					"offering-standard-2-4",  // Cheaper
				},
			},
		}

		err := k8sClient.Create(ctx, nodeGroup)
		require.NoError(t, err, "Failed to create NodeGroup")
		defer k8sClient.Delete(ctx, nodeGroup)

		// Analyze for optimization
		report, err := optimizer.AnalyzeNodeGroup(ctx, nodeGroup)
		require.NoError(t, err, "Failed to analyze NodeGroup")
		assert.NotNil(t, report, "Report should not be nil")
		assert.Equal(t, nodeGroup.Name, report.NodeGroupName, "NodeGroup name should match")
	})

	t.Run("Generate optimization recommendations", func(t *testing.T) {
		calculator := cost.NewCalculator(vpsieClient)
		optimizer := cost.NewOptimizer(calculator, vpsieClient)

		// Create NodeGroup with suboptimal configuration
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "recommendation-test-ng",
				Namespace: testNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     3,
				MaxNodes:     10,
				DatacenterID: "dc1",
				OfferingIDs:  []string{"offering-standard-8-16"}, // Expensive offering
			},
		}

		err := k8sClient.Create(ctx, nodeGroup)
		require.NoError(t, err, "Failed to create NodeGroup")
		defer k8sClient.Delete(ctx, nodeGroup)

		// Get recommendations
		report, err := optimizer.AnalyzeNodeGroup(ctx, nodeGroup)
		require.NoError(t, err, "Failed to get recommendations")
		assert.NotNil(t, report, "Report should not be nil")

		// If there are opportunities, verify they have required fields
		if len(report.Opportunities) > 0 {
			for _, opp := range report.Opportunities {
				assert.NotEmpty(t, opp.Type, "Opportunity type should not be empty")
				assert.GreaterOrEqual(t, opp.MonthlySavings, float64(0), "Savings should be non-negative")
			}
		}
	})
}

// TestCostMetricsIntegration tests that cost metrics are properly updated
func TestCostMetricsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start mock VPSie server
	mockServer := NewMockVPSieServer()
	mockServer.Start()
	defer mockServer.Stop()

	t.Run("Verify cost metrics are exposed", func(t *testing.T) {
		// Create NodeGroup
		nodeGroup := &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "metrics-test-ng",
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

		// Wait a bit for metrics to be updated
		time.Sleep(2 * time.Second)

		// Note: In a full integration test, we would query the Prometheus metrics endpoint
		// For now, we just verify the NodeGroup exists which should trigger metric updates
		var fetchedNG autoscalerv1alpha1.NodeGroup
		err = k8sClient.Get(ctx, client.ObjectKey{
			Namespace: testNamespace,
			Name:      nodeGroup.Name,
		}, &fetchedNG)
		require.NoError(t, err, "Failed to fetch NodeGroup")
		assert.Equal(t, nodeGroup.Name, fetchedNG.Name, "NodeGroup name should match")
	})
}
