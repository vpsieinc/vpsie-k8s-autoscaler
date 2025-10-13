package vpsienode

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// TestDrainNode_Success tests successful node draining
func TestDrainNode_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

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
		WithObjects(node).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()

	drainer := NewDrainer(client)
	logger := zap.NewNop()

	err := drainer.DrainNode(context.Background(), "test-node", logger)
	require.NoError(t, err)

	// Verify node was cordoned
	updatedNode := &corev1.Node{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-node"}, updatedNode)
	require.NoError(t, err)
	assert.True(t, updatedNode.Spec.Unschedulable, "Node should be cordoned")
}

// TestCordonNode tests cordoning a node
func TestCordonNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

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
		WithObjects(node).
		Build()

	drainer := NewDrainer(client)
	logger := zap.NewNop()

	err := drainer.cordonNode(context.Background(), "test-node", logger)
	require.NoError(t, err)

	// Verify node is now unschedulable
	updatedNode := &corev1.Node{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-node"}, updatedNode)
	require.NoError(t, err)
	assert.True(t, updatedNode.Spec.Unschedulable)
}

// TestCordonNode_AlreadyCordoned tests cordoning an already cordoned node
func TestCordonNode_AlreadyCordoned(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true, // Already cordoned
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node).
		Build()

	drainer := NewDrainer(client)
	logger := zap.NewNop()

	err := drainer.cordonNode(context.Background(), "test-node", logger)
	require.NoError(t, err, "Should succeed when node is already cordoned")
}

// TestCordonNode_NotFound tests cordoning a non-existent node
func TestCordonNode_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	drainer := NewDrainer(client)
	logger := zap.NewNop()

	err := drainer.cordonNode(context.Background(), "non-existent-node", logger)
	require.NoError(t, err, "Should succeed when node doesn't exist")
}

// TestUncordonNode tests uncordoning a node (rollback)
func TestUncordonNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
		Spec: corev1.NodeSpec{
			Unschedulable: true, // Cordoned
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node).
		Build()

	drainer := NewDrainer(client)
	logger := zap.NewNop()

	err := drainer.uncordonNode(context.Background(), "test-node", logger)
	require.NoError(t, err)

	// Verify node is now schedulable
	updatedNode := &corev1.Node{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-node"}, updatedNode)
	require.NoError(t, err)
	assert.False(t, updatedNode.Spec.Unschedulable, "Node should be uncordoned")
}

// TestGetPodsOnNode tests listing pods on a node
func TestGetPodsOnNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-1",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
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
	}

	pod3 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pod-3",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "other-node", // Different node
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pod1, pod2, pod3).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			pod := obj.(*corev1.Pod)
			return []string{pod.Spec.NodeName}
		}).
		Build()

	drainer := NewDrainer(client)
	logger := zap.NewNop()

	pods, err := drainer.getPodsOnNode(context.Background(), "test-node", logger)
	require.NoError(t, err)
	assert.Len(t, pods, 2, "Should find 2 pods on test-node")
}

// TestFilterPodsToEvict tests filtering of pods to evict
func TestFilterPodsToEvict(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	logger := zap.NewNop()
	drainer := &Drainer{}

	tests := []struct {
		name     string
		pod      corev1.Pod
		shouldEvict bool
	}{
		{
			name: "regular pod should be evicted",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "regular-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeName: "test-node",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			shouldEvict: true,
		},
		{
			name: "DaemonSet pod should not be evicted",
			pod: corev1.Pod{
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
			},
			shouldEvict: false,
		},
		{
			name: "static pod should not be evicted",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "static-pod",
					Namespace: "kube-system",
					Annotations: map[string]string{
						"kubernetes.io/config.mirror": "true",
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "test-node",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			shouldEvict: false,
		},
		{
			name: "succeeded pod should not be evicted",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "succeeded-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeName: "test-node",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodSucceeded,
				},
			},
			shouldEvict: false,
		},
		{
			name: "failed pod should not be evicted",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "failed-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					NodeName: "test-node",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodFailed,
				},
			},
			shouldEvict: false,
		},
		{
			name: "terminating pod should not be evicted",
			pod: corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "terminating-pod",
					Namespace:         "default",
					DeletionTimestamp: &metav1.Time{},
				},
				Spec: corev1.PodSpec{
					NodeName: "test-node",
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			},
			shouldEvict: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := drainer.filterPodsToEvict([]corev1.Pod{tt.pod}, logger)
			if tt.shouldEvict {
				assert.Len(t, filtered, 1, "Pod should be included in eviction list")
			} else {
				assert.Len(t, filtered, 0, "Pod should not be included in eviction list")
			}
		})
	}
}

// TestIsDaemonSetPod tests DaemonSet pod detection
func TestIsDaemonSetPod(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "pod with DaemonSet owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "DaemonSet", Name: "test-ds"},
					},
				},
			},
			expected: true,
		},
		{
			name: "pod with ReplicaSet owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "test-rs"},
					},
				},
			},
			expected: false,
		},
		{
			name: "pod with no owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDaemonSetPod(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsStaticPod tests static pod detection
func TestIsStaticPod(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "pod with mirror annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"kubernetes.io/config.mirror": "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "kube-system pod with no owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
				},
			},
			expected: true,
		},
		{
			name: "kube-system pod with owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "kube-system",
					OwnerReferences: []metav1.OwnerReference{
						{Kind: "ReplicaSet", Name: "test-rs"},
					},
				},
			},
			expected: false,
		},
		{
			name: "default namespace pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isStaticPod(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDeleteNode tests deleting a Kubernetes Node object
func TestDeleteNode(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
		Spec: v1alpha1.VPSieNodeSpec{
			NodeName: "test-node",
		},
		Status: v1alpha1.VPSieNodeStatus{
			NodeName: "test-node",
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(node).
		Build()

	drainer := NewDrainer(client)
	logger := zap.NewNop()

	err := drainer.DeleteNode(context.Background(), vn, logger)
	require.NoError(t, err)

	// Verify node was deleted
	deletedNode := &corev1.Node{}
	err = client.Get(context.Background(), types.NamespacedName{Name: "test-node"}, deletedNode)
	assert.Error(t, err, "Node should be deleted")
}

// TestDeleteNode_NotFound tests deleting a non-existent node
func TestDeleteNode_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
		Spec: v1alpha1.VPSieNodeSpec{
			NodeName: "non-existent-node",
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	drainer := NewDrainer(client)
	logger := zap.NewNop()

	err := drainer.DeleteNode(context.Background(), vn, logger)
	require.NoError(t, err, "Should succeed when node doesn't exist")
}

// TestDeleteNode_NoNodeName tests deleting when no node name is set
func TestDeleteNode_NoNodeName(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
		Spec:   v1alpha1.VPSieNodeSpec{},
		Status: v1alpha1.VPSieNodeStatus{},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	drainer := NewDrainer(client)
	logger := zap.NewNop()

	err := drainer.DeleteNode(context.Background(), vn, logger)
	require.NoError(t, err, "Should succeed when no node name is set")
}
