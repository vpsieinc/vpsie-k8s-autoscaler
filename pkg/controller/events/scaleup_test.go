package events

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeClient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// TestMakeScaleUpDecision tests scale-up decision making
func TestMakeScaleUpDecision(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fakeClient.NewClientBuilder().WithScheme(scheme).Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	analyzer := NewResourceAnalyzer(logger)

	tests := []struct {
		name             string
		nodeGroup        *v1alpha1.NodeGroup
		match            NodeGroupMatch
		canScale         bool
		expectedDecision bool
		expectedNodes    int32
	}{
		{
			name: "Scale up from 2 to 3 nodes",
			nodeGroup: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ng-1",
					Namespace: "default",
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes:              1,
					MaxNodes:              10,
					OfferingIDs:           []string{"offering-1"},
					PreferredInstanceType: "offering-1",
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 2,
				},
			},
			match: NodeGroupMatch{
				NodeGroup: &v1alpha1.NodeGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "ng-1",
						Namespace: "default",
					},
					Spec: v1alpha1.NodeGroupSpec{
						MinNodes:              1,
						MaxNodes:              10,
						OfferingIDs:           []string{"offering-1"},
						PreferredInstanceType: "offering-1",
					},
					Status: v1alpha1.NodeGroupStatus{
						DesiredNodes: 2,
					},
				},
				MatchingPods: []*corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}},
				},
				Deficit: ResourceDeficit{
					CPU:    resource.MustParse("4000m"),
					Memory: resource.MustParse("8Gi"),
					Pods:   2,
				},
			},
			canScale:         true,
			expectedDecision: true,
			expectedNodes:    3, // 2 current + 1 new (based on 4 CPU / 4 CPU per instance)
		},
		{
			name: "At max capacity, no scale-up",
			nodeGroup: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ng-1",
					Namespace: "default",
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes:    1,
					MaxNodes:    5,
					OfferingIDs: []string{"offering-1"},
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 5,
				},
			},
			match: NodeGroupMatch{
				NodeGroup: &v1alpha1.NodeGroup{
					Status: v1alpha1.NodeGroupStatus{
						DesiredNodes: 5,
					},
					Spec: v1alpha1.NodeGroupSpec{
						MaxNodes: 5,
					},
				},
				MatchingPods: []*corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
				},
				Deficit: ResourceDeficit{
					CPU:    resource.MustParse("2000m"),
					Memory: resource.MustParse("4Gi"),
					Pods:   1,
				},
			},
			canScale:         true,
			expectedDecision: false,
		},
		{
			name: "In cooldown period, no scale-up",
			nodeGroup: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ng-1",
					Namespace: "default",
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes:    1,
					MaxNodes:    10,
					OfferingIDs: []string{"offering-1"},
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 2,
				},
			},
			match: NodeGroupMatch{
				NodeGroup: &v1alpha1.NodeGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ng-1",
					},
					Status: v1alpha1.NodeGroupStatus{
						DesiredNodes: 2,
					},
					Spec: v1alpha1.NodeGroupSpec{
						MaxNodes:    10,
						OfferingIDs: []string{"offering-1"},
					},
				},
				MatchingPods: []*corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
				},
				Deficit: ResourceDeficit{
					CPU:    resource.MustParse("2000m"),
					Memory: resource.MustParse("4Gi"),
					Pods:   1,
				},
			},
			canScale:         false, // Cooldown active
			expectedDecision: false,
		},
		{
			name: "Respect max capacity limit",
			nodeGroup: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ng-1",
					Namespace: "default",
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes:    1,
					MaxNodes:    5,
					OfferingIDs: []string{"offering-1"},
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 3,
				},
			},
			match: NodeGroupMatch{
				NodeGroup: &v1alpha1.NodeGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "ng-1",
					},
					Status: v1alpha1.NodeGroupStatus{
						DesiredNodes: 3,
					},
					Spec: v1alpha1.NodeGroupSpec{
						MaxNodes:    5,
						OfferingIDs: []string{"offering-1"},
					},
				},
				MatchingPods: []*corev1.Pod{
					{ObjectMeta: metav1.ObjectMeta{Name: "pod-1"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "pod-2"}},
					{ObjectMeta: metav1.ObjectMeta{Name: "pod-3"}},
				},
				Deficit: ResourceDeficit{
					CPU:    resource.MustParse("12000m"), // Would need 3 nodes
					Memory: resource.MustParse("24Gi"),
					Pods:   3,
				},
			},
			canScale:         true,
			expectedDecision: true,
			expectedNodes:    5, // Limited by maxNodes (3 + 3 = 6, but max is 5)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh watcher and controller for each test to avoid cooldown state pollution
			watcher := NewEventWatcher(k8sClient, clientset, logger, nil)
			controller := NewScaleUpController(k8sClient, analyzer, watcher, logger)

			// Set up cooldown state
			if !tt.canScale {
				watcher.RecordScaleEvent(tt.nodeGroup.Name)
			}

			decision, err := controller.makeScaleUpDecision(context.Background(), tt.match)
			require.NoError(t, err)

			if tt.expectedDecision {
				require.NotNil(t, decision, "Expected a scale-up decision")
				assert.Equal(t, tt.expectedNodes, decision.DesiredNodes)
				assert.Equal(t, tt.nodeGroup.Name, decision.NodeGroup.Name)
			} else {
				assert.Nil(t, decision, "Expected no scale-up decision")
			}
		})
	}
}

// TestExecuteScaleUp tests scale-up execution
func TestExecuteScaleUp(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ng-1",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes:    1,
			MaxNodes:    10,
			OfferingIDs: []string{"offering-1"},
		},
		Status: v1alpha1.NodeGroupStatus{
			DesiredNodes: 2,
		},
	}

	k8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ng).
		WithStatusSubresource(&v1alpha1.NodeGroup{}).
		Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	analyzer := NewResourceAnalyzer(logger)
	watcher := NewEventWatcher(k8sClient, clientset, logger, nil)
	controller := NewScaleUpController(k8sClient, analyzer, watcher, logger)

	decision := ScaleUpDecision{
		NodeGroup:    ng,
		CurrentNodes: 2,
		DesiredNodes: 4,
		NodesToAdd:   2,
		InstanceType: "offering-1",
		MatchingPods: 5,
		Deficit: ResourceDeficit{
			CPU:    resource.MustParse("8000m"),
			Memory: resource.MustParse("16Gi"),
			Pods:   5,
		},
		Reason: "Scaling up to accommodate 5 pending pods",
	}

	err := controller.executeScaleUp(context.Background(), decision)
	require.NoError(t, err)

	// Verify NodeGroup was updated
	updatedNG := &v1alpha1.NodeGroup{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, updatedNG)
	require.NoError(t, err)

	assert.Equal(t, int32(4), updatedNG.Status.DesiredNodes)

	// Verify cooldown was recorded
	assert.False(t, watcher.CanScale(ng.Name), "Should be in cooldown period")
}

// TestHandleScaleUp tests the complete scale-up flow
func TestHandleScaleUp(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create test pods
	pendingPod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pending-pod-1",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "", // Not scheduled
			NodeSelector: map[string]string{
				"env": "production",
			},
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	pendingPod2 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pending-pod-2",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "", // Not scheduled
			NodeSelector: map[string]string{
				"env": "production",
			},
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2000m"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	runningPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "running-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	// Create NodeGroup
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ng-prod",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes: 1,
			MaxNodes: 10,
			Labels: map[string]string{
				"env": "production",
			},
			OfferingIDs:           []string{"offering-1"},
			PreferredInstanceType: "offering-1",
		},
		Status: v1alpha1.NodeGroupStatus{
			DesiredNodes: 2,
		},
	}

	k8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pendingPod1, pendingPod2, runningPod, ng).
		WithStatusSubresource(&v1alpha1.NodeGroup{}).
		Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	analyzer := NewResourceAnalyzer(logger)
	watcher := NewEventWatcher(k8sClient, clientset, logger, nil)
	controller := NewScaleUpController(k8sClient, analyzer, watcher, logger)

	// Create scheduling events
	events := []SchedulingEvent{
		{
			Pod:        pendingPod1,
			Constraint: ConstraintCPU,
			Message:    "Insufficient cpu",
		},
		{
			Pod:        pendingPod2,
			Constraint: ConstraintMemory,
			Message:    "Insufficient memory",
		},
	}

	err := controller.HandleScaleUp(context.Background(), events)
	require.NoError(t, err)

	// Verify NodeGroup was scaled up
	updatedNG := &v1alpha1.NodeGroup{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, updatedNG)
	require.NoError(t, err)

	assert.Greater(t, updatedNG.Status.DesiredNodes, int32(2), "NodeGroup should be scaled up")
}

// TestHandleScaleUpNoPendingPods tests scale-up with no pending pods
func TestHandleScaleUpNoPendingPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	runningPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "running-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	k8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(runningPod).
		Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	analyzer := NewResourceAnalyzer(logger)
	watcher := NewEventWatcher(k8sClient, clientset, logger, nil)
	controller := NewScaleUpController(k8sClient, analyzer, watcher, logger)

	events := []SchedulingEvent{}

	err := controller.HandleScaleUp(context.Background(), events)
	require.NoError(t, err)

	// No scaling should occur
}

// TestHandleScaleUpNoMatchingNodeGroups tests scale-up with no matching NodeGroups
func TestHandleScaleUpNoMatchingNodeGroups(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	pendingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pending-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "",
			NodeSelector: map[string]string{
				"env": "staging",
			},
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1000m"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ng-prod",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes: 1,
			MaxNodes: 10,
			Labels: map[string]string{
				"env": "production", // Does not match pod
			},
			OfferingIDs: []string{"offering-1"},
		},
		Status: v1alpha1.NodeGroupStatus{
			DesiredNodes: 2,
		},
	}

	k8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pendingPod, ng).
		WithStatusSubresource(&v1alpha1.NodeGroup{}).
		Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	analyzer := NewResourceAnalyzer(logger)
	watcher := NewEventWatcher(k8sClient, clientset, logger, nil)
	controller := NewScaleUpController(k8sClient, analyzer, watcher, logger)

	events := []SchedulingEvent{
		{
			Pod:        pendingPod,
			Constraint: ConstraintCPU,
			Message:    "Insufficient cpu",
		},
	}

	err := controller.HandleScaleUp(context.Background(), events)
	require.NoError(t, err)

	// Verify no scale-up occurred
	updatedNG := &v1alpha1.NodeGroup{}
	err = k8sClient.Get(context.Background(), client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, updatedNG)
	require.NoError(t, err)

	assert.Equal(t, int32(2), updatedNG.Status.DesiredNodes, "NodeGroup should not be scaled")
}

// TestGetScaleUpDecisions tests the GetScaleUpDecisions method (for testing)
func TestGetScaleUpDecisions(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	pendingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pending-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "",
			NodeSelector: map[string]string{
				"env": "production",
			},
			Containers: []corev1.Container{
				{
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2000m"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ng-prod",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes: 1,
			MaxNodes: 10,
			Labels: map[string]string{
				"env": "production",
			},
			OfferingIDs:           []string{"offering-1"},
			PreferredInstanceType: "offering-1",
		},
		Status: v1alpha1.NodeGroupStatus{
			DesiredNodes: 2,
		},
	}

	k8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pendingPod, ng).
		Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	analyzer := NewResourceAnalyzer(logger)
	watcher := NewEventWatcher(k8sClient, clientset, logger, nil)
	controller := NewScaleUpController(k8sClient, analyzer, watcher, logger)

	events := []SchedulingEvent{
		{
			Pod:        pendingPod,
			Constraint: ConstraintCPU,
			Message:    "Insufficient cpu",
		},
	}

	decisions, err := controller.GetScaleUpDecisions(context.Background(), events)
	require.NoError(t, err)

	require.Len(t, decisions, 1, "Should have one scale-up decision")
	assert.Equal(t, "ng-prod", decisions[0].NodeGroup.Name)
	assert.Greater(t, decisions[0].DesiredNodes, int32(2))
	assert.Equal(t, "offering-1", decisions[0].InstanceType)
}
