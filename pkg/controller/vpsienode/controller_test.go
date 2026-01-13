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
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// TestPendingPhaseTransition tests Pending → Provisioning transition
func TestPendingPhaseTransition(t *testing.T) {
	// Setup
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
			InstanceType:  "offering-1",
			NodeGroupName: "test-ng",
			DatacenterID:  "dc-1",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhasePending,
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vn).WithStatusSubresource(vn).Build()
	mockVPSie := NewMockVPSieClient()
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
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator, 24*time.Hour, client)
	reconciler.provisioner = provisioner
	reconciler.joiner = joiner

	// Test: Reconcile should transition from Pending to Provisioning
	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.Requeue)

	// Verify phase transition
	err = client.Get(context.Background(), req.NamespacedName, vn)
	require.NoError(t, err, "Failed to get VPSieNode after reconcile")
	assert.Equal(t, v1alpha1.VPSieNodePhaseProvisioning, vn.Status.Phase)
	assert.NotNil(t, vn.Status.CreatedAt)
	assert.True(t, IsConditionFalse(vn, v1alpha1.VPSieNodeConditionVPSReady))
}

// TestProvisioningPhaseTransition tests Provisioning → Provisioned transition
func TestProvisioningPhaseTransition(t *testing.T) {
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
			InstanceType:       "offering-1",
			NodeGroupName:      "test-ng",
			DatacenterID:       "dc-1",
			VPSieGroupID:       1,
			ResourceIdentifier: "test-cluster",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseProvisioning,
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vn).WithStatusSubresource(vn).Build()
	mockVPSie := NewMockVPSieClient()
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
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator, 24*time.Hour, client)
	reconciler.provisioner = provisioner
	reconciler.joiner = joiner

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// First reconcile: Creates VPS (stays in Provisioning)
	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)

	// Verify VPS was created via AddK8sSlaveToGroup API
	_ = client.Get(context.Background(), req.NamespacedName, vn)
	assert.NotEqual(t, 0, vn.Spec.VPSieInstanceID)
	assert.NotEmpty(t, vn.Spec.IPAddress)
	assert.Equal(t, 1, mockVPSie.GetCallCount("AddK8sSlaveToGroup"))

	// Update VPS status to "running"
	err = mockVPSie.UpdateVMStatus(vn.Spec.VPSieInstanceID, "running")
	require.NoError(t, err)

	// Second reconcile: VPS is running, transitions to Provisioned
	result, err = reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)

	// Verify phase transition
	_ = client.Get(context.Background(), req.NamespacedName, vn)
	assert.Equal(t, v1alpha1.VPSieNodePhaseProvisioned, vn.Status.Phase)
	assert.NotNil(t, vn.Status.ProvisionedAt)
	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionVPSReady))
	assert.Equal(t, "running", vn.Status.VPSieStatus)
}

// TestProvisioningPhaseWithExistingVPS tests Provisioning phase when VPS already exists
func TestProvisioningPhaseWithExistingVPS(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	vpsID := 1234

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-vn",
			Namespace:  "default",
			Finalizers: []string{FinalizerName},
		},
		Spec: v1alpha1.VPSieNodeSpec{
			VPSieInstanceID: vpsID,
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseProvisioning,
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vn).WithStatusSubresource(vn).Build()
	mockVPSie := NewMockVPSieClient()

	// Pre-create VPS in mock
	mockVPSie.VMs[vpsID] = &vpsieclient.VPS{
		ID:          vpsID,
		Name:        "test-vn",
		Hostname:    "test-vn",
		Status:      "running",
		CPU:         4,
		RAM:         8192,
		Disk:        80,
		IPAddress:   "10.0.0.10",
		IPv6Address: "2001:db8::10",
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
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator, 24*time.Hour, client)
	reconciler.provisioner = provisioner
	reconciler.joiner = joiner

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Reconcile: Should check existing VPS and transition to Provisioned
	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)

	// Verify no new VPS was created
	assert.Equal(t, 0, mockVPSie.GetCallCount("CreateVM"))
	assert.Equal(t, 1, mockVPSie.GetCallCount("GetVM"))

	// Verify phase transition
	_ = client.Get(context.Background(), req.NamespacedName, vn)
	assert.Equal(t, v1alpha1.VPSieNodePhaseProvisioned, vn.Status.Phase)
	assert.NotNil(t, vn.Status.ProvisionedAt)
}

// TestProvisionedPhaseTransition tests Provisioned → Joining transition
func TestProvisionedPhaseTransition(t *testing.T) {
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
			VPSieInstanceID: 1000,
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
			NodeName:        "test-node",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseProvisioned,
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vn).WithStatusSubresource(vn).Build()
	mockVPSie := NewMockVPSieClient()
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
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator, 24*time.Hour, client)
	reconciler.provisioner = provisioner
	reconciler.joiner = joiner

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Reconcile: Should transition from Provisioned to Joining
	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)

	// Verify phase transition
	_ = client.Get(context.Background(), req.NamespacedName, vn)
	assert.Equal(t, v1alpha1.VPSieNodePhaseJoining, vn.Status.Phase)
	assert.True(t, IsConditionFalse(vn, v1alpha1.VPSieNodeConditionNodeJoined))
}

// TestJoiningPhaseTransitionWithNode tests Joining → Ready transition when node exists
func TestJoiningPhaseTransitionWithNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	nodeName := "test-node"

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-vn",
			Namespace:  "default",
			Finalizers: []string{FinalizerName},
		},
		Spec: v1alpha1.VPSieNodeSpec{
			VPSieInstanceID: 1000,
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
			NodeName:        nodeName,
			IPAddress:       "10.0.0.10",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseJoining,
		},
	}

	// Create a Kubernetes Node that matches
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeName,
		},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: "10.0.0.10",
				},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vn, node).WithStatusSubresource(vn).Build()
	mockVPSie := NewMockVPSieClient()
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
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator, 24*time.Hour, client)
	reconciler.provisioner = provisioner
	reconciler.joiner = joiner

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Reconcile: Should find the node and transition to Ready
	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)

	// Verify phase transition
	_ = client.Get(context.Background(), req.NamespacedName, vn)
	assert.Equal(t, v1alpha1.VPSieNodePhaseReady, vn.Status.Phase)
	assert.Equal(t, nodeName, vn.Status.NodeName)
	assert.NotNil(t, vn.Status.JoinedAt)
	assert.NotNil(t, vn.Status.ReadyAt)
	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionNodeJoined))
	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionNodeReady))

	// Verify labels were applied to the node
	_ = client.Get(context.Background(), types.NamespacedName{Name: nodeName}, node)
	assert.Equal(t, "true", node.Labels["autoscaler.vpsie.com/managed"])
	assert.Equal(t, "test-ng", node.Labels["autoscaler.vpsie.com/nodegroup"])
	assert.Equal(t, "test-vn", node.Labels["autoscaler.vpsie.com/vpsienode"])
}

// TestJoiningPhaseWaitingForNode tests Joining phase when node hasn't appeared yet
func TestJoiningPhaseWaitingForNode(t *testing.T) {
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
			VPSieInstanceID: 1000,
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
			NodeName:        "test-node",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseJoining,
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vn).WithStatusSubresource(vn).Build()
	mockVPSie := NewMockVPSieClient()
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
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator, 24*time.Hour, client)
	reconciler.provisioner = provisioner
	reconciler.joiner = joiner

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Reconcile: Node not found, should stay in Joining phase
	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0)

	// Verify still in Joining phase
	_ = client.Get(context.Background(), req.NamespacedName, vn)
	assert.Equal(t, v1alpha1.VPSieNodePhaseJoining, vn.Status.Phase)
	assert.Empty(t, vn.Status.NodeName)
}

// TestProvisioningTimeout tests timeout handling in Provisioning phase
func TestProvisioningTimeout(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Set CreatedAt to more than 10 minutes ago
	createdAt := metav1.NewTime(time.Now().Add(-11 * time.Minute))

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-vn",
			Namespace:  "default",
			Finalizers: []string{FinalizerName},
		},
		Spec: v1alpha1.VPSieNodeSpec{
			VPSieInstanceID: 1000,
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase:     v1alpha1.VPSieNodePhaseProvisioning,
			CreatedAt: &createdAt,
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vn).WithStatusSubresource(vn).Build()
	mockVPSie := NewMockVPSieClient()

	// Add VPS that's still in provisioning state
	mockVPSie.VMs[1000] = &vpsieclient.VPS{
		ID:       1000,
		Name:     "test-vn",
		Hostname: "test-vn",
		Status:   "provisioning",
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
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator, 24*time.Hour, client)
	reconciler.provisioner = provisioner
	reconciler.joiner = joiner

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Reconcile: Should timeout and transition to Failed
	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Verify transitioned to Failed phase
	_ = client.Get(context.Background(), req.NamespacedName, vn)
	assert.Equal(t, v1alpha1.VPSieNodePhaseFailed, vn.Status.Phase)
	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionError))
	assert.NotEmpty(t, vn.Status.LastError)
	assert.Contains(t, vn.Status.LastError, "timeout")
}

// TestJoiningTimeout tests timeout handling in Joining phase
func TestJoiningTimeout(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Set ProvisionedAt to more than 15 minutes ago
	provisionedAt := metav1.NewTime(time.Now().Add(-16 * time.Minute))

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-vn",
			Namespace:  "default",
			Finalizers: []string{FinalizerName},
		},
		Spec: v1alpha1.VPSieNodeSpec{
			VPSieInstanceID: 1000,
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
			NodeName:        "test-node",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase:         v1alpha1.VPSieNodePhaseJoining,
			ProvisionedAt: &provisionedAt,
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vn).WithStatusSubresource(vn).Build()
	mockVPSie := NewMockVPSieClient()
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
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator, 24*time.Hour, client)
	reconciler.provisioner = provisioner
	reconciler.joiner = joiner

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Reconcile: Should timeout and transition to Failed
	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Verify transitioned to Failed phase
	_ = client.Get(context.Background(), req.NamespacedName, vn)
	assert.Equal(t, v1alpha1.VPSieNodePhaseFailed, vn.Status.Phase)
	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionError))
	assert.NotEmpty(t, vn.Status.LastError)
	assert.Contains(t, vn.Status.LastError, "timeout")
}

// TestVPSNotFoundError tests handling when VPS is not found
func TestVPSNotFoundError(t *testing.T) {
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
			VPSieInstanceID: 9999, // Non-existent VPS
			InstanceType:    "offering-1",
			NodeGroupName:   "test-ng",
			DatacenterID:    "dc-1",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseProvisioning,
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(vn).WithStatusSubresource(vn).Build()
	mockVPSie := NewMockVPSieClient()
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
	reconciler.stateMachine = NewStateMachine(provisioner, joiner, terminator, 24*time.Hour, client)
	reconciler.provisioner = provisioner
	reconciler.joiner = joiner

	req := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Reconcile: Should detect VPS not found and transition to Failed
	result, err := reconciler.Reconcile(context.Background(), req)
	require.NoError(t, err)
	assert.False(t, result.Requeue)

	// Verify transitioned to Failed phase
	_ = client.Get(context.Background(), req.NamespacedName, vn)
	assert.Equal(t, v1alpha1.VPSieNodePhaseFailed, vn.Status.Phase)
	assert.NotEmpty(t, vn.Status.LastError)
}
