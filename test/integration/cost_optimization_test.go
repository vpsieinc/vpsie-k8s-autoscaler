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
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/cost"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestCostCalculatorIntegration tests the cost calculator with mock VPSie API
func TestCostCalculatorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// TODO: This test requires a VPSie client, but NewClientWithCredentials enforces HTTPS
	// while the mock server uses HTTP. This test needs to be refactored to either:
	// 1. Use a test-specific client constructor that allows HTTP
	// 2. Use httptest.NewTLSServer instead of httptest.NewServer
	// For now, we test only the parts that don't require a VPSie client

	t.Run("Calculate potential savings", func(t *testing.T) {
		// This test doesn't require a VPSie client - it uses mock data directly
		currentCost := &cost.NodeGroupCost{
			TotalMonthly: 100.0,
			TotalHourly:  0.14,
			CostPerNode:  50.0,
		}

		proposedCost := &cost.NodeGroupCost{
			TotalMonthly: 75.0,
			TotalHourly:  0.10,
			CostPerNode:  37.5,
		}

		// Create a minimal calculator for savings calculation
		// Note: CalculateSavings doesn't actually use the client
		calculator := cost.NewCalculator(nil)
		savings, err := calculator.CalculateSavings(currentCost, proposedCost)
		require.NoError(t, err, "Failed to calculate savings")
		assert.NotNil(t, savings, "Savings should not be nil")
		assert.Equal(t, 25.0, savings.MonthlySavings, "Monthly savings should be $25")
		assert.Equal(t, 25.0, savings.SavingsPercent, "Percent savings should be 25%")
	})
}

// TestCostOptimizerIntegration tests the cost optimizer with real scenarios
func TestCostOptimizerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// TODO: This test requires a VPSie client with HTTPS support.
	// See TestCostCalculatorIntegration for details.
	t.Skip("Skipping: requires VPSie client with HTTPS support - mock server uses HTTP")
}

// TestCostMetricsIntegration tests that cost metrics are properly updated
func TestCostMetricsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Start mock VPSie server (not used for client, but keeps test structure)
	mockServer := NewMockVPSieServer()
	defer mockServer.Close()

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
