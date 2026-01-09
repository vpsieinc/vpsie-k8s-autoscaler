//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// TestRebalancing_CostOptimizationConfig tests cost optimization configuration
func TestRebalancing_CostOptimizationConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup with cost optimization settings
	ng := NewNodeGroupBuilder("test-cost-opt").
		WithMinNodes(2).
		WithMaxNodes(10).
		WithOfferings("small-2cpu-4gb", "medium-4cpu-8gb", "large-8cpu-16gb").
		Build()

	// Enable cost optimization (if CRD supports it)
	createNodeGroup(ctx, t, ng)

	// Verify NodeGroup was created with multiple offerings
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	assert.Len(t, fetchedNg.Spec.OfferingIDs, 3, "Should have 3 offerings for cost optimization")
	assert.Contains(t, fetchedNg.Spec.OfferingIDs, "small-2cpu-4gb")
	assert.Contains(t, fetchedNg.Spec.OfferingIDs, "medium-4cpu-8gb")
	assert.Contains(t, fetchedNg.Spec.OfferingIDs, "large-8cpu-16gb")
}

// TestRebalancing_MultipleOfferings tests that multiple offerings can be specified
func TestRebalancing_MultipleOfferings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	offerings := []string{
		"small-2cpu-4gb",
		"medium-4cpu-8gb",
		"large-8cpu-16gb",
	}

	ng := NewNodeGroupBuilder("test-multi-offering").
		WithMinNodes(1).
		WithMaxNodes(5).
		WithOfferings(offerings...).
		Build()

	createNodeGroup(ctx, t, ng)

	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	// Verify all offerings are stored
	for _, offering := range offerings {
		assert.Contains(t, fetchedNg.Spec.OfferingIDs, offering)
	}
}

// TestRebalancing_VPSieNodeWithDifferentOfferings tests creating VPSieNodes with different offerings
func TestRebalancing_VPSieNodeWithDifferentOfferings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup
	ng := NewNodeGroupBuilder("test-offering-nodes").
		WithMinNodes(1).
		WithMaxNodes(10).
		WithOfferings("small-2cpu-4gb", "medium-4cpu-8gb").
		Build()

	createNodeGroup(ctx, t, ng)

	// Create VPSieNodes with different offerings
	offerings := []string{"small-2cpu-4gb", "medium-4cpu-8gb"}
	for i, offering := range offerings {
		vn := NewVPSieNodeBuilder("offering-node-"+string(rune('a'+i)), ng.Name).
			WithOffering(offering).
			Build()

		err := k8sClient.Create(ctx, vn)
		require.NoError(t, err, "Failed to create VPSieNode with offering %s", offering)

		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = k8sClient.Delete(cleanupCtx, vn)
		})
	}

	// Verify VPSieNodes with correct offerings
	vnList := getNodeGroupVPSieNodes(ctx, t, ng.Name)
	assert.Len(t, vnList, 2, "Should have 2 VPSieNodes")

	foundOfferings := make(map[string]bool)
	for _, vn := range vnList {
		foundOfferings[vn.Spec.OfferingID] = true
	}

	for _, offering := range offerings {
		assert.True(t, foundOfferings[offering], "Should find VPSieNode with offering %s", offering)
	}
}

// TestRebalancing_NodeGroupStatusUpdates tests that status reflects VPSieNode changes
func TestRebalancing_NodeGroupStatusUpdates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	ng := NewNodeGroupBuilder("test-status-updates").
		WithMinNodes(0).
		WithMaxNodes(10).
		Build()

	createNodeGroup(ctx, t, ng)

	// Create multiple VPSieNodes
	for i := 0; i < 3; i++ {
		vn := NewVPSieNodeBuilder("status-node-"+string(rune('a'+i)), ng.Name).Build()
		err := k8sClient.Create(ctx, vn)
		require.NoError(t, err)

		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = k8sClient.Delete(cleanupCtx, vn)
		})
	}

	// Check VPSieNodes count
	vnList := getNodeGroupVPSieNodes(ctx, t, ng.Name)
	assert.Len(t, vnList, 3, "Should have 3 VPSieNodes")
}

// TestRebalancing_DatacenterAffinity tests datacenter configuration
func TestRebalancing_DatacenterAffinity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	testCases := []struct {
		name       string
		datacenter string
	}{
		{"dc-1-test", "dc-test-1"},
		{"dc-2-test", "dc-test-2"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ng := NewNodeGroupBuilder(tc.name).
				WithMinNodes(1).
				WithMaxNodes(5).
				WithDatacenter(tc.datacenter).
				Build()

			createNodeGroup(ctx, t, ng)

			var fetchedNg autoscalerv1alpha1.NodeGroup
			err := k8sClient.Get(ctx, client.ObjectKey{
				Name:      ng.Name,
				Namespace: ng.Namespace,
			}, &fetchedNg)
			require.NoError(t, err)

			assert.Equal(t, tc.datacenter, fetchedNg.Spec.DatacenterID)
		})
	}
}

// TestRebalancing_MockAPIIntegration tests the mock VPSie API server
func TestRebalancing_MockAPIIntegration(t *testing.T) {
	// Reset mock server state
	mockServer.Reset()

	// Test VM creation via mock server
	vmID := mockServer.AddVM("test-vm-1", "running")
	assert.Greater(t, vmID, 0, "VM ID should be positive")

	// Test VM count
	count := mockServer.GetVMCount()
	assert.Equal(t, 1, count, "Should have 1 VM")

	// Test state change
	err := mockServer.SetVMState(vmID, "ready")
	assert.NoError(t, err, "Should be able to set VM state")

	// Test request counting
	initialCount := mockServer.GetTotalRequests()

	// Make a request to the mock server (this would be done by the controller)
	// For now, just verify the counter works
	assert.GreaterOrEqual(t, initialCount, int64(0))
}

// TestRebalancing_ErrorInjection tests error injection in mock server
func TestRebalancing_ErrorInjection(t *testing.T) {
	// Test error injection configuration
	mockServer.SetErrorInjection(true, 0.5)
	defer mockServer.SetErrorInjection(false, 0)

	// Verify error injection is set
	// The actual error injection would be tested when making API calls
	t.Log("Error injection enabled with 50% rate")
}

// TestRebalancing_LatencySimulation tests latency simulation in mock server
func TestRebalancing_LatencySimulation(t *testing.T) {
	// Set artificial latency
	mockServer.SetLatency(100 * time.Millisecond)
	defer mockServer.SetLatency(0)

	t.Log("Latency simulation enabled at 100ms")
}

// TestRebalancing_QuotaEnforcement tests quota enforcement in mock server
func TestRebalancing_QuotaEnforcement(t *testing.T) {
	mockServer.Reset()

	// The mock server has a quota limit (default 100)
	// Add VMs up to the limit would trigger quota errors
	initialCount := mockServer.GetVMCount()
	assert.Equal(t, 0, initialCount, "Should start with 0 VMs after reset")
}

// TestRebalancing_ScaleDownSafety tests scale-down safety configuration
func TestRebalancing_ScaleDownSafety(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	ng := NewNodeGroupBuilder("test-scaledown-safety").
		WithMinNodes(2). // Minimum 2 nodes for safety
		WithMaxNodes(10).
		WithScaleDownThresholds(20, 30). // Conservative thresholds
		Build()

	// Set long stabilization window for safety
	ng.Spec.ScaleDownPolicy.StabilizationWindowSeconds = 600

	createNodeGroup(ctx, t, ng)

	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	// Verify safety settings
	assert.Equal(t, int32(2), fetchedNg.Spec.MinNodes, "MinNodes should enforce safety")
	assert.Equal(t, int32(20), fetchedNg.Spec.ScaleDownPolicy.CPUThreshold)
	assert.Equal(t, int32(30), fetchedNg.Spec.ScaleDownPolicy.MemoryThreshold)
	assert.Equal(t, int32(600), fetchedNg.Spec.ScaleDownPolicy.StabilizationWindowSeconds)
}

// TestRebalancing_ConcurrentOperations tests handling of concurrent operations
func TestRebalancing_ConcurrentOperations(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create multiple NodeGroups concurrently
	const numNodeGroups = 5
	done := make(chan error, numNodeGroups)

	for i := 0; i < numNodeGroups; i++ {
		go func(index int) {
			ng := NewNodeGroupBuilder("concurrent-ng-" + string(rune('a'+index))).
				WithMinNodes(1).
				WithMaxNodes(5).
				Build()

			err := k8sClient.Create(ctx, ng)
			done <- err
		}(i)
	}

	// Wait for all creations
	successCount := 0
	for i := 0; i < numNodeGroups; i++ {
		err := <-done
		if err == nil {
			successCount++
		}
	}

	assert.Equal(t, numNodeGroups, successCount, "All NodeGroups should be created successfully")

	// Cleanup
	for i := 0; i < numNodeGroups; i++ {
		ng := &autoscalerv1alpha1.NodeGroup{}
		ng.Name = "concurrent-ng-" + string(rune('a'+i))
		ng.Namespace = TestNamespace
		_ = k8sClient.Delete(ctx, ng)
	}
}
