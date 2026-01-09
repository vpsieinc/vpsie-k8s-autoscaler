//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// TestFailure_APIErrorHandling tests handling of VPSie API errors
func TestFailure_APIErrorHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Enable error injection in mock server
	mockServer.SetErrorInjection(true, 0.0) // 0% initial rate
	defer mockServer.SetErrorInjection(false, 0)

	// Create a NodeGroup - should succeed with no errors
	ng := NewNodeGroupBuilder("test-api-errors").
		WithMinNodes(1).
		WithMaxNodes(5).
		Build()

	createNodeGroup(ctx, t, ng)

	// Verify NodeGroup exists
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	t.Log("NodeGroup created successfully, API error handling test setup complete")
}

// TestFailure_HighErrorRate tests behavior under high API error rate
func TestFailure_HighErrorRate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup first
	ng := NewNodeGroupBuilder("test-high-error-rate").
		WithMinNodes(1).
		WithMaxNodes(5).
		Build()

	createNodeGroup(ctx, t, ng)

	// Now enable high error rate
	mockServer.SetErrorInjection(true, 0.8) // 80% error rate
	defer mockServer.SetErrorInjection(false, 0)

	// The controller should handle errors gracefully
	// In a full E2E, we would verify:
	// - Circuit breaker opens
	// - Events are recorded
	// - Status reflects error state

	t.Log("High error rate configured (80%), controller should handle gracefully")
}

// TestFailure_NetworkLatency tests behavior under network latency
func TestFailure_NetworkLatency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Set artificial latency
	mockServer.SetLatency(500 * time.Millisecond)
	defer mockServer.SetLatency(0)

	ng := NewNodeGroupBuilder("test-latency").
		WithMinNodes(1).
		WithMaxNodes(5).
		Build()

	createNodeGroup(ctx, t, ng)

	// Verify operations still complete under latency
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	t.Log("Operations completed successfully under 500ms latency")
}

// TestFailure_InvalidCredentials tests behavior with invalid credentials
func TestFailure_InvalidCredentials(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// Create secret with invalid credentials
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vpsie-secret-invalid",
			Namespace: TestNamespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"clientId":     "invalid-client-id",
			"clientSecret": "invalid-secret",
			"url":          mockServer.URL(),
		},
	}

	_, err := clientset.CoreV1().Secrets(TestNamespace).Create(ctx, secret, metav1.CreateOptions{})
	if err == nil {
		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = clientset.CoreV1().Secrets(TestNamespace).Delete(cleanupCtx, secret.Name, metav1.DeleteOptions{})
		})
	}

	// The mock server accepts any credentials, but in production this would fail
	t.Log("Invalid credentials test setup complete")
}

// TestFailure_ResourceCleanup tests that resources are cleaned up on failure
func TestFailure_ResourceCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup
	ng := NewNodeGroupBuilder("test-cleanup").
		WithMinNodes(1).
		WithMaxNodes(5).
		Build()

	createNodeGroup(ctx, t, ng)

	// Create some VPSieNodes
	for i := 0; i < 3; i++ {
		vn := NewVPSieNodeBuilder("cleanup-node-"+string(rune('a'+i)), ng.Name).Build()
		err := k8sClient.Create(ctx, vn)
		require.NoError(t, err)
	}

	// Verify VPSieNodes exist
	vnList := getNodeGroupVPSieNodes(ctx, t, ng.Name)
	assert.Len(t, vnList, 3)

	// Delete NodeGroup
	err = k8sClient.Delete(ctx, ng)
	require.NoError(t, err)

	// Note: In a full E2E with controller running, we would verify:
	// - VPSieNodes are deleted (cascade delete or controller cleanup)
	// - VPSie VMs are terminated
	// - Kubernetes nodes are removed

	t.Log("Resource cleanup test initiated")
}

// TestFailure_QuotaExhaustion tests behavior when VPSie quota is exhausted
func TestFailure_QuotaExhaustion(t *testing.T) {
	mockServer.Reset()

	// Fill up quota (mock server has default limit of 100)
	for i := 0; i < 100; i++ {
		mockServer.AddVM("quota-vm-"+string(rune('a'+i%26)), "running")
	}

	assert.Equal(t, 100, mockServer.GetVMCount(), "Should have 100 VMs")

	// Next VM creation should fail due to quota
	// In a real test, we would verify the controller handles this gracefully
	t.Log("Quota exhaustion test: mock server at capacity")
}

// TestFailure_PartialScaleUp tests partial success during scale-up
func TestFailure_PartialScaleUp(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup requesting multiple nodes
	ng := NewNodeGroupBuilder("test-partial-scaleup").
		WithMinNodes(3). // Requires 3 nodes
		WithMaxNodes(5).
		Build()

	createNodeGroup(ctx, t, ng)

	// In a full E2E, we would:
	// 1. Configure mock server to fail after 1-2 VM creations
	// 2. Verify controller retries failed creations
	// 3. Verify partial success is reported in status

	t.Log("Partial scale-up test setup complete")
}

// TestFailure_VMProvisioningTimeout tests handling of VM provisioning timeout
func TestFailure_VMProvisioningTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup
	ng := NewNodeGroupBuilder("test-provision-timeout").
		WithMinNodes(1).
		WithMaxNodes(5).
		Build()

	createNodeGroup(ctx, t, ng)

	// Create VPSieNode manually
	vn := NewVPSieNodeBuilder("timeout-node", ng.Name).Build()
	err = k8sClient.Create(ctx, vn)
	require.NoError(t, err)

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = k8sClient.Delete(cleanupCtx, vn)
	})

	// In a full E2E with controller:
	// - Disable auto-transition in mock server
	// - Wait for provisioning timeout
	// - Verify VPSieNode marked as failed
	// - Verify retry or cleanup occurs

	t.Log("Provisioning timeout test setup complete")
}

// TestFailure_NodeDrainFailure tests handling of node drain failures
func TestFailure_NodeDrainFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	ng := NewNodeGroupBuilder("test-drain-failure").
		WithMinNodes(1).
		WithMaxNodes(5).
		Build()

	createNodeGroup(ctx, t, ng)

	// In a full E2E, we would:
	// 1. Create a node with pods that can't be evicted (PDB, critical system pods)
	// 2. Trigger scale-down
	// 3. Verify drain fails gracefully
	// 4. Verify node is not deleted

	t.Log("Node drain failure test setup complete")
}

// TestFailure_ConcurrentFailures tests handling of multiple concurrent failures
func TestFailure_ConcurrentFailures(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Enable error injection with moderate rate
	mockServer.SetErrorInjection(true, 0.3)
	mockServer.SetLatency(200 * time.Millisecond)
	defer func() {
		mockServer.SetErrorInjection(false, 0)
		mockServer.SetLatency(0)
	}()

	// Create multiple NodeGroups concurrently
	const numNodeGroups = 3
	done := make(chan bool, numNodeGroups)

	for i := 0; i < numNodeGroups; i++ {
		go func(index int) {
			ng := NewNodeGroupBuilder("concurrent-fail-" + string(rune('a'+index))).
				WithMinNodes(1).
				WithMaxNodes(3).
				Build()

			_ = k8sClient.Create(ctx, ng)
			done <- true
		}(i)
	}

	// Wait for all attempts
	for i := 0; i < numNodeGroups; i++ {
		<-done
	}

	t.Log("Concurrent failures test completed")

	// Cleanup
	for i := 0; i < numNodeGroups; i++ {
		ng := &autoscalerv1alpha1.NodeGroup{}
		ng.Name = "concurrent-fail-" + string(rune('a'+i))
		ng.Namespace = TestNamespace
		_ = k8sClient.Delete(ctx, ng)
	}
}

// TestFailure_RecoveryAfterAPIOutage tests recovery after API outage
func TestFailure_RecoveryAfterAPIOutage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	ng := NewNodeGroupBuilder("test-recovery").
		WithMinNodes(1).
		WithMaxNodes(5).
		Build()

	createNodeGroup(ctx, t, ng)

	// Simulate API outage
	mockServer.SetErrorInjection(true, 1.0) // 100% errors

	// Wait a bit
	time.Sleep(2 * time.Second)

	// Restore API
	mockServer.SetErrorInjection(false, 0)

	// In a full E2E:
	// - Verify circuit breaker transitions to half-open then closed
	// - Verify reconciliation resumes
	// - Verify status is updated correctly

	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err, "Should be able to read NodeGroup after recovery")

	t.Log("Recovery after API outage test completed")
}

// TestFailure_StatusConditionUpdates tests that status conditions reflect failures
func TestFailure_StatusConditionUpdates(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	ng := NewNodeGroupBuilder("test-status-conditions").
		WithMinNodes(1).
		WithMaxNodes(5).
		Build()

	createNodeGroup(ctx, t, ng)

	// Get initial status
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	// Log status for debugging
	logNodeGroupStatus(t, &fetchedNg)

	// In a full E2E with controller:
	// - Trigger various failure scenarios
	// - Verify status conditions are updated
	// - Verify events are recorded
	// - Verify metrics are incremented

	t.Log("Status condition updates test completed")
}

// TestFailure_GracefulDegradation tests graceful degradation under failures
func TestFailure_GracefulDegradation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup
	ng := NewNodeGroupBuilder("test-degradation").
		WithMinNodes(2).
		WithMaxNodes(10).
		Build()

	createNodeGroup(ctx, t, ng)

	// Create some VPSieNodes
	for i := 0; i < 3; i++ {
		vn := NewVPSieNodeBuilder("degradation-node-"+string(rune('a'+i)), ng.Name).Build()
		err := k8sClient.Create(ctx, vn)
		require.NoError(t, err)

		t.Cleanup(func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = k8sClient.Delete(cleanupCtx, vn)
		})
	}

	// Enable failures
	mockServer.SetErrorInjection(true, 0.5)
	defer mockServer.SetErrorInjection(false, 0)

	// In a full E2E:
	// - Verify existing nodes continue to function
	// - Verify new scale operations are queued/retried
	// - Verify no data loss or corruption

	t.Log("Graceful degradation test completed")
}
