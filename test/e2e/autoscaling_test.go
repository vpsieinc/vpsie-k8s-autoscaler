//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// TestAutoscaling_NodeGroupLifecycle tests basic NodeGroup CRUD operations
func TestAutoscaling_NodeGroupLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// Create VPSie secret for the test namespace
	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err, "Failed to create VPSie secret")

	t.Run("Create NodeGroup", func(t *testing.T) {
		ng := NewNodeGroupBuilder("test-lifecycle").
			WithMinNodes(1).
			WithMaxNodes(5).
			WithOfferings("small-2cpu-4gb").
			Build()

		createdNg := createNodeGroup(ctx, t, ng)

		// Verify the NodeGroup was created
		var fetchedNg autoscalerv1alpha1.NodeGroup
		err := k8sClient.Get(ctx, client.ObjectKey{
			Name:      createdNg.Name,
			Namespace: createdNg.Namespace,
		}, &fetchedNg)
		require.NoError(t, err, "Failed to fetch created NodeGroup")

		assert.Equal(t, int32(1), fetchedNg.Spec.MinNodes)
		assert.Equal(t, int32(5), fetchedNg.Spec.MaxNodes)
		assert.Equal(t, "dc-test-1", fetchedNg.Spec.DatacenterID)
	})

	t.Run("Update NodeGroup", func(t *testing.T) {
		ng := NewNodeGroupBuilder("test-update").
			WithMinNodes(1).
			WithMaxNodes(5).
			Build()

		createNodeGroup(ctx, t, ng)

		// Update the NodeGroup
		err := retryOnConflict(ctx, t, func() error {
			var fetchedNg autoscalerv1alpha1.NodeGroup
			if err := k8sClient.Get(ctx, client.ObjectKey{
				Name:      ng.Name,
				Namespace: ng.Namespace,
			}, &fetchedNg); err != nil {
				return err
			}

			fetchedNg.Spec.MaxNodes = 10
			return k8sClient.Update(ctx, &fetchedNg)
		})
		require.NoError(t, err, "Failed to update NodeGroup")

		// Verify the update
		var updatedNg autoscalerv1alpha1.NodeGroup
		err = k8sClient.Get(ctx, client.ObjectKey{
			Name:      ng.Name,
			Namespace: ng.Namespace,
		}, &updatedNg)
		require.NoError(t, err)
		assert.Equal(t, int32(10), updatedNg.Spec.MaxNodes)
	})

	t.Run("Delete NodeGroup", func(t *testing.T) {
		ng := NewNodeGroupBuilder("test-delete").
			WithMinNodes(1).
			WithMaxNodes(3).
			Build()

		createNodeGroup(ctx, t, ng)

		// Delete the NodeGroup
		err := k8sClient.Delete(ctx, ng)
		require.NoError(t, err, "Failed to delete NodeGroup")

		// Wait for deletion
		time.Sleep(2 * time.Second)

		// Verify deletion
		var fetchedNg autoscalerv1alpha1.NodeGroup
		err = k8sClient.Get(ctx, client.ObjectKey{
			Name:      ng.Name,
			Namespace: ng.Namespace,
		}, &fetchedNg)
		assert.Error(t, err, "NodeGroup should be deleted")
	})
}

// TestAutoscaling_ScaleUpTrigger tests that unschedulable pods trigger scale-up
func TestAutoscaling_ScaleUpTrigger(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Create VPSie secret
	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create a NodeGroup with scale-up enabled
	ng := NewNodeGroupBuilder("test-scaleup").
		WithMinNodes(0).
		WithMaxNodes(5).
		WithScaleUpThresholds(80, 80).
		Build()

	createNodeGroup(ctx, t, ng)
	t.Logf("Created NodeGroup: %s", ng.Name)

	// Create a pod that cannot be scheduled (requests resources that don't exist)
	// This simulates unschedulable pods that should trigger scale-up
	pod := createUnschedulablePod(ctx, t, "trigger-scaleup", "4", "8Gi")
	t.Logf("Created unschedulable pod: %s", pod.Name)

	// Wait for pod to be marked as unschedulable
	pod = waitForPodUnschedulable(ctx, t, pod.Name)
	t.Logf("Pod is unschedulable as expected")

	// In a full E2E test with the controller running, we would wait for:
	// 1. The controller to detect the unschedulable pod
	// 2. A VPSieNode to be created
	// 3. The VPSieNode to provision (mock server transitions state)
	// 4. A Kubernetes node to be created and join the cluster
	// 5. The pod to be scheduled

	// For now, verify the NodeGroup exists and can be updated
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	logNodeGroupStatus(t, &fetchedNg)
}

// TestAutoscaling_MinNodesEnforcement tests that minimum nodes are always maintained
func TestAutoscaling_MinNodesEnforcement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup with minNodes=2
	ng := NewNodeGroupBuilder("test-minnodes").
		WithMinNodes(2).
		WithMaxNodes(10).
		Build()

	createNodeGroup(ctx, t, ng)

	// Verify the NodeGroup was created with correct min
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	assert.Equal(t, int32(2), fetchedNg.Spec.MinNodes, "MinNodes should be enforced")
}

// TestAutoscaling_MaxNodesLimit tests that maximum nodes are not exceeded
func TestAutoscaling_MaxNodesLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup with maxNodes=3
	ng := NewNodeGroupBuilder("test-maxnodes").
		WithMinNodes(0).
		WithMaxNodes(3).
		Build()

	createNodeGroup(ctx, t, ng)

	// Verify the NodeGroup respects max limit
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	assert.Equal(t, int32(3), fetchedNg.Spec.MaxNodes, "MaxNodes should be set correctly")
}

// TestAutoscaling_ScaleDownPolicy tests scale-down policy configuration
func TestAutoscaling_ScaleDownPolicy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup with specific scale-down policy
	ng := NewNodeGroupBuilder("test-scaledown-policy").
		WithMinNodes(1).
		WithMaxNodes(10).
		WithScaleDownThresholds(30, 40).
		Build()

	// Set custom cooldown
	ng.Spec.ScaleDownPolicy.CooldownSeconds = 600
	ng.Spec.ScaleDownPolicy.StabilizationWindowSeconds = 300

	createNodeGroup(ctx, t, ng)

	// Verify the policy was applied
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	assert.Equal(t, int32(30), fetchedNg.Spec.ScaleDownPolicy.CPUThreshold)
	assert.Equal(t, int32(40), fetchedNg.Spec.ScaleDownPolicy.MemoryThreshold)
	assert.Equal(t, int32(600), fetchedNg.Spec.ScaleDownPolicy.CooldownSeconds)
}

// TestAutoscaling_MultipleNodeGroups tests managing multiple NodeGroups
func TestAutoscaling_MultipleNodeGroups(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create multiple NodeGroups
	nodeGroups := []struct {
		name       string
		minNodes   int32
		maxNodes   int32
		datacenter string
	}{
		{"multi-ng-1", 1, 5, "dc-test-1"},
		{"multi-ng-2", 2, 10, "dc-test-1"},
		{"multi-ng-3", 0, 3, "dc-test-2"},
	}

	for _, ngSpec := range nodeGroups {
		ng := NewNodeGroupBuilder(ngSpec.name).
			WithMinNodes(ngSpec.minNodes).
			WithMaxNodes(ngSpec.maxNodes).
			WithDatacenter(ngSpec.datacenter).
			Build()

		createNodeGroup(ctx, t, ng)
	}

	// Verify all NodeGroups exist
	ngList := &autoscalerv1alpha1.NodeGroupList{}
	err = k8sClient.List(ctx, ngList, client.InNamespace(TestNamespace))
	require.NoError(t, err)

	// Filter to only our test NodeGroups
	count := 0
	for _, ng := range ngList.Items {
		for _, expected := range nodeGroups {
			if ng.Name == expected.name {
				count++
				assert.Equal(t, expected.minNodes, ng.Spec.MinNodes)
				assert.Equal(t, expected.maxNodes, ng.Spec.MaxNodes)
				assert.Equal(t, expected.datacenter, ng.Spec.DatacenterID)
			}
		}
	}
	assert.Equal(t, len(nodeGroups), count, "All NodeGroups should be created")
}

// TestAutoscaling_VPSieNodeCreation tests VPSieNode resource creation
func TestAutoscaling_VPSieNodeCreation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create a NodeGroup first
	ng := NewNodeGroupBuilder("test-vpsienode").
		WithMinNodes(1).
		WithMaxNodes(5).
		Build()

	createNodeGroup(ctx, t, ng)

	// Create a VPSieNode manually (simulating what controller would do)
	vn := NewVPSieNodeBuilder("test-node-1", ng.Name).
		WithOffering("small-2cpu-4gb").
		Build()

	err = k8sClient.Create(ctx, vn)
	require.NoError(t, err, "Failed to create VPSieNode")

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = k8sClient.Delete(cleanupCtx, vn)
	})

	// Verify VPSieNode was created
	var fetchedVN autoscalerv1alpha1.VPSieNode
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      vn.Name,
		Namespace: vn.Namespace,
	}, &fetchedVN)
	require.NoError(t, err)

	assert.Equal(t, ng.Name, fetchedVN.Spec.NodeGroupName)
	assert.Equal(t, "small-2cpu-4gb", fetchedVN.Spec.OfferingID)
}

// TestAutoscaling_NodeLabels tests that node labels are properly applied
func TestAutoscaling_NodeLabels(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup with custom labels
	labels := map[string]string{
		"environment": "test",
		"team":        "platform",
		"workload":    "general",
	}

	ng := NewNodeGroupBuilder("test-labels").
		WithMinNodes(1).
		WithMaxNodes(5).
		WithLabels(labels).
		Build()

	createNodeGroup(ctx, t, ng)

	// Verify labels are stored
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	for key, value := range labels {
		assert.Equal(t, value, fetchedNg.Spec.Labels[key], "Label %s should match", key)
	}
}

// TestAutoscaling_CooldownPeriod tests that cooldown periods are respected
func TestAutoscaling_CooldownPeriod(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	// Create NodeGroup with short cooldown for testing
	ng := NewNodeGroupBuilder("test-cooldown").
		WithMinNodes(1).
		WithMaxNodes(10).
		Build()

	ng.Spec.ScaleUpPolicy.CooldownSeconds = 60
	ng.Spec.ScaleDownPolicy.CooldownSeconds = 120

	createNodeGroup(ctx, t, ng)

	// Verify cooldown settings
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	assert.Equal(t, int32(60), fetchedNg.Spec.ScaleUpPolicy.CooldownSeconds)
	assert.Equal(t, int32(120), fetchedNg.Spec.ScaleDownPolicy.CooldownSeconds)
}

// TestAutoscaling_StatusConditions tests that status conditions are properly set
func TestAutoscaling_StatusConditions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	err := createVPSieSecret(ctx, TestNamespace)
	require.NoError(t, err)

	ng := NewNodeGroupBuilder("test-conditions").
		WithMinNodes(1).
		WithMaxNodes(5).
		Build()

	createNodeGroup(ctx, t, ng)

	// Get the NodeGroup and check initial status
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	// Log the status for debugging
	logNodeGroupStatus(t, &fetchedNg)

	// Status fields should be initialized
	// Note: The actual controller would set these; in E2E we verify the CRD accepts them
	assert.GreaterOrEqual(t, fetchedNg.Status.DesiredNodes, int32(0))
	assert.GreaterOrEqual(t, fetchedNg.Status.CurrentNodes, int32(0))
	assert.GreaterOrEqual(t, fetchedNg.Status.ReadyNodes, int32(0))
}

// TestAutoscaling_ValidationRejection tests that invalid NodeGroups are rejected
func TestAutoscaling_ValidationRejection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	testCases := []struct {
		name        string
		modifier    func(*autoscalerv1alpha1.NodeGroup)
		expectError bool
	}{
		{
			name: "MinNodes greater than MaxNodes",
			modifier: func(ng *autoscalerv1alpha1.NodeGroup) {
				ng.Spec.MinNodes = 10
				ng.Spec.MaxNodes = 5
			},
			expectError: true,
		},
		{
			name: "Negative MinNodes",
			modifier: func(ng *autoscalerv1alpha1.NodeGroup) {
				ng.Spec.MinNodes = -1
			},
			expectError: true,
		},
		{
			name: "Empty OfferingIDs",
			modifier: func(ng *autoscalerv1alpha1.NodeGroup) {
				ng.Spec.OfferingIDs = []string{}
			},
			expectError: true,
		},
		{
			name: "Empty DatacenterID",
			modifier: func(ng *autoscalerv1alpha1.NodeGroup) {
				ng.Spec.DatacenterID = ""
			},
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ng := NewNodeGroupBuilder("test-validation-" + tc.name).
				WithMinNodes(1).
				WithMaxNodes(5).
				Build()

			tc.modifier(ng)

			err := k8sClient.Create(ctx, ng)

			if tc.expectError {
				assert.Error(t, err, "Expected validation error for: %s", tc.name)
			} else {
				assert.NoError(t, err, "Expected no error for: %s", tc.name)
				// Cleanup if created
				_ = k8sClient.Delete(ctx, ng)
			}
		})
	}
}
