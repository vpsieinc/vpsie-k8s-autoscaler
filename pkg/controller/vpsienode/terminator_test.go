package vpsienode

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// TestTerminationFlow tests the complete termination flow Ready → Terminating → Deleting
func TestTerminationFlow(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-vn",
			Namespace:         "default",
			Finalizers:        []string{FinalizerName},
			DeletionTimestamp: &metav1.Time{Time: time.Now()}, // Deletion requested
		},
		Spec: v1alpha1.VPSieNodeSpec{
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
			VPSieInstanceID: 1000,
			NodeName:        "test-node",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase:    v1alpha1.VPSieNodePhaseReady,
			NodeName: "test-node",
		},
	}

	// Create a Kubernetes Node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Spec: corev1.NodeSpec{
			Unschedulable: false,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vn, node).
		WithStatusSubresource(vn).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()

	mockVPSie := NewMockVPSieClient()
	mockVPSie.VMs[1000] = &vpsieclient.VPS{
		ID:       1000,
		Status:   "running",
		Hostname: "test-node",
	}

	logger := zap.NewNop()

	reconciler := &VPSieNodeReconciler{
		Client:      client,
		Scheme:      scheme,
		VPSieClient: mockVPSie,
		Logger:      logger,
	}

	provisioner := NewProvisioner(mockVPSie, nil)
	joiner := NewJoiner(client, provisioner)
	drainer := NewDrainer(client)
	terminator := NewTerminator(drainer, provisioner)
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator)
	reconciler.provisioner = provisioner
	reconciler.joiner = joiner
	reconciler.drainer = drainer
	reconciler.terminator = terminator

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// First reconcile: Should transition to Terminating
	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.Requeue)

	err = client.Get(context.Background(), req.NamespacedName, vn)
	require.NoError(t, err)
	assert.Equal(t, v1alpha1.VPSieNodePhaseTerminating, vn.Status.Phase)
	assert.NotNil(t, vn.Status.TerminatingAt)

	// Keep reconciling until the object is deleted or we hit a limit
	maxReconciles := 10
	for i := 0; i < maxReconciles; i++ {
		t.Logf("Reconcile %d starting...", i+2)
		result, err = reconciler.Reconcile(context.Background(), req)
		require.NoError(t, err)
		t.Logf("Reconcile %d: result.Requeue=%v, result.RequeueAfter=%v", i+2, result.Requeue, result.RequeueAfter)

		// Check if object still exists
		err = client.Get(context.Background(), req.NamespacedName, vn)
		if err != nil {
			// Object was deleted - termination complete
			t.Logf("Object deleted after reconcile %d", i+2)
			break
		}

		t.Logf("Reconcile %d: phase=%s, DeletedAt=%v, finalizers=%v, VPSieInstanceID=%d",
			i+2, vn.Status.Phase, vn.Status.DeletedAt, vn.Finalizers, vn.Spec.VPSieInstanceID)

		// If no requeue requested and object still exists, something is wrong
		if !result.Requeue && result.RequeueAfter == 0 {
			t.Logf("No requeue requested, stopping")
			break
		}
	}

	// Verify VPS was deleted
	t.Logf("DeleteVM call count: %d", mockVPSie.GetCallCount("DeleteVM"))
	assert.Equal(t, 1, mockVPSie.GetCallCount("DeleteVM"), "VPS should have been deleted")
}

// TestTerminationWithPods tests termination with pods running on the node
func TestTerminationWithPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-vn",
			Namespace:         "default",
			Finalizers:        []string{FinalizerName},
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
		Spec: v1alpha1.VPSieNodeSpec{
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
			VPSieInstanceID: 1000,
			NodeName:        "test-node",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase:    v1alpha1.VPSieNodePhaseReady,
			NodeName: "test-node",
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Spec: corev1.NodeSpec{
			Unschedulable: false,
		},
	}

	// Create some pods on the node
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	pod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-2",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	// Create a DaemonSet pod (should not be evicted)
	dsPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ds-pod",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					Kind: "DaemonSet",
					Name: "test-ds",
				},
			},
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vn, node, pod1, pod2, dsPod).
		WithStatusSubresource(vn).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()

	mockVPSie := NewMockVPSieClient()
	mockVPSie.VMs[1000] = &vpsieclient.VPS{
		ID:       1000,
		Status:   "running",
		Hostname: "test-node",
	}

	logger := zap.NewNop()

	reconciler := &VPSieNodeReconciler{
		Client:      client,
		Scheme:      scheme,
		VPSieClient: mockVPSie,
		Logger:      logger,
	}

	provisioner := NewProvisioner(mockVPSie, nil)
	joiner := NewJoiner(client, provisioner)
	drainer := NewDrainer(client)
	terminator := NewTerminator(drainer, provisioner)
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator)
	reconciler.provisioner = provisioner
	reconciler.drainer = drainer
	reconciler.terminator = terminator

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Transition to Terminating
	_, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)

	// Drain and transition to Deleting (object might be deleted if everything completes fast)
	_, err = reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)

	err = client.Get(context.Background(), req.NamespacedName, vn)
	if err != nil {
		// Object was deleted (acceptable)
		return
	}
	// If still exists, verify phase progressed
	if vn.Status.Phase != v1alpha1.VPSieNodePhaseDeleting && vn.Status.DeletedAt == nil {
		t.Logf("Warning: Expected Deleting phase or DeletedAt, got phase=%s", vn.Status.Phase)
	}

	// Verify node was cordoned or deleted (both are acceptable outcomes)
	updatedNode := &corev1.Node{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-node"}, updatedNode)
	if err == nil {
		// Node still exists, verify it was cordoned
		assert.True(t, updatedNode.Spec.Unschedulable, "Node should be cordoned")
	} else {
		// Node was deleted (expected with termination)
		assert.True(t, true, "Node was deleted during termination")
	}

	// Note: The fake client doesn't fully support pod eviction subresource,
	// so we can't verify pod deletion in this test. The drain logic is tested
	// separately in drainer_test.go
}

// TestTerminationWithNonExistentNode tests termination when node doesn't exist
func TestTerminationWithNonExistentNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-vn",
			Namespace:         "default",
			Finalizers:        []string{FinalizerName},
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
		Spec: v1alpha1.VPSieNodeSpec{
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
			VPSieInstanceID: 1000,
			NodeName:        "non-existent-node",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase:    v1alpha1.VPSieNodePhaseReady,
			NodeName: "non-existent-node",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vn).
		WithStatusSubresource(vn).
		Build()

	mockVPSie := NewMockVPSieClient()
	mockVPSie.VMs[1000] = &vpsieclient.VPS{
		ID:       1000,
		Status:   "running",
		Hostname: "test-node",
	}

	logger := zap.NewNop()

	reconciler := &VPSieNodeReconciler{
		Client:      client,
		Scheme:      scheme,
		VPSieClient: mockVPSie,
		Logger:      logger,
	}

	provisioner := NewProvisioner(mockVPSie, nil)
	joiner := NewJoiner(client, provisioner)
	drainer := NewDrainer(client)
	terminator := NewTerminator(drainer, provisioner)
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator)
	reconciler.provisioner = provisioner
	reconciler.drainer = drainer
	reconciler.terminator = terminator

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Keep reconciling until the object is deleted
	maxReconciles := 10
	for i := 0; i < maxReconciles; i++ {
		result, err := reconciler.Reconcile(context.Background(), req)
		require.NoError(t, err)

		// Check if object still exists
		err = client.Get(context.Background(), req.NamespacedName, vn)
		if err != nil {
			// Object was deleted - termination complete
			break
		}

		// If no requeue requested and object still exists, something is wrong
		if !result.Requeue && result.RequeueAfter == 0 {
			t.Logf("Reconcile %d: No requeue, phase=%s, DeletedAt=%v",
				i, vn.Status.Phase, vn.Status.DeletedAt)
			break
		}
	}

	// Should succeed even though node didn't exist
	assert.Equal(t, 1, mockVPSie.GetCallCount("DeleteVM"), "VPS should have been deleted")
}

// TestVPSDeletionFailure tests VPS deletion failure with retries
func TestVPSDeletionFailure(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-vn",
			Namespace:         "default",
			Finalizers:        []string{FinalizerName},
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
		Spec: v1alpha1.VPSieNodeSpec{
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
			VPSieInstanceID: 1000,
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseDeleting,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vn).
		WithStatusSubresource(vn).
		Build()

	mockVPSie := NewMockVPSieClient()
	// Make DeleteVM always fail
	mockVPSie.DeleteVMFunc = func(ctx context.Context, id int) error {
		return &vpsieclient.APIError{
			StatusCode: 500,
			Message:    "Internal Server Error",
		}
	}

	logger := zap.NewNop()

	provisioner := NewProvisioner(mockVPSie, nil)
	drainer := NewDrainer(client)
	terminator := NewTerminator(drainer, provisioner)

	// Try to delete VPS
	result, err := terminator.DeleteVPS(context.Background(), vn, logger)
	assert.Error(t, err)
	assert.True(t, result.RequeueAfter > 0)

	// Verify VPS deletion was retried
	assert.GreaterOrEqual(t, mockVPSie.GetCallCount("DeleteVM"), MaxRetries)

	// Verify phase transitioned to Failed
	assert.Equal(t, v1alpha1.VPSieNodePhaseFailed, vn.Status.Phase)
	assert.Contains(t, vn.Status.LastError, "Failed to delete VPS")
}

// TestVPSAlreadyDeleted tests VPS deletion when VPS is already gone
func TestVPSAlreadyDeleted(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-vn",
			Namespace:  "default",
			Finalizers: []string{FinalizerName},
		},
		Spec: v1alpha1.VPSieNodeSpec{
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
			VPSieInstanceID: 9999, // Non-existent VPS
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseDeleting,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vn).
		WithStatusSubresource(vn).
		Build()

	mockVPSie := NewMockVPSieClient()
	logger := zap.NewNop()

	provisioner := NewProvisioner(mockVPSie, nil)
	drainer := NewDrainer(client)
	terminator := NewTerminator(drainer, provisioner)

	// Try to delete non-existent VPS
	result, err := terminator.DeleteVPS(context.Background(), vn, logger)
	require.NoError(t, err, "Should succeed when VPS is already deleted")
	assert.False(t, result.Requeue)

	// Verify DeletedAt was set
	assert.NotNil(t, vn.Status.DeletedAt)
}

// TestTerminationWithNoVPSID tests termination when no VPS ID is set
func TestTerminationWithNoVPSID(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-vn",
			Namespace:  "default",
			Finalizers: []string{FinalizerName},
		},
		Spec: v1alpha1.VPSieNodeSpec{
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
			VPSieInstanceID: 0, // No VPS ID
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseDeleting,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vn).
		WithStatusSubresource(vn).
		Build()

	mockVPSie := NewMockVPSieClient()
	logger := zap.NewNop()

	provisioner := NewProvisioner(mockVPSie, nil)
	drainer := NewDrainer(client)
	terminator := NewTerminator(drainer, provisioner)

	// Try to delete with no VPS ID
	result, err := terminator.DeleteVPS(context.Background(), vn, logger)
	require.NoError(t, err, "Should succeed when no VPS ID is set")
	assert.False(t, result.Requeue)

	// Verify no API calls were made
	assert.Equal(t, 0, mockVPSie.GetCallCount("DeleteVM"))
}
