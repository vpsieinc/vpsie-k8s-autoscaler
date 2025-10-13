package events

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
	"k8s.io/client-go/kubernetes/fake"
	fakeClient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// TestParseConstraint tests constraint parsing from event messages
func TestParseConstraint(t *testing.T) {
	tests := []struct {
		name       string
		message    string
		constraint ResourceConstraint
	}{
		{
			name:       "Insufficient CPU",
			message:    "0/3 nodes are available: 3 Insufficient cpu.",
			constraint: ConstraintCPU,
		},
		{
			name:       "Insufficient memory",
			message:    "0/3 nodes are available: 3 Insufficient memory.",
			constraint: ConstraintMemory,
		},
		{
			name:       "Too many pods",
			message:    "0/3 nodes are available: 3 Too many pods.",
			constraint: ConstraintPods,
		},
		{
			name:       "Mixed message with CPU",
			message:    "0/5 nodes are available: 2 node(s) had taints, 3 Insufficient cpu.",
			constraint: ConstraintCPU,
		},
		{
			name:       "Case insensitive CPU",
			message:    "Insufficient CPU available",
			constraint: ConstraintCPU,
		},
		{
			name:       "Case insensitive memory",
			message:    "INSUFFICIENT MEMORY",
			constraint: ConstraintMemory,
		},
		{
			name:       "Unknown constraint",
			message:    "0/3 nodes are available: 3 node(s) had taints that the pod didn't tolerate.",
			constraint: ConstraintUnknown,
		},
		{
			name:       "Empty message",
			message:    "",
			constraint: ConstraintUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseConstraint(tt.message)
			assert.Equal(t, tt.constraint, result)
		})
	}
}

// TestFilterRecentEvents tests event filtering by time
func TestFilterRecentEvents(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fakeClient.NewClientBuilder().WithScheme(scheme).Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	watcher := NewEventWatcher(k8sClient, clientset, logger, nil)
	watcher.stabilizationWindow = 60 * time.Second

	now := time.Now()

	events := []SchedulingEvent{
		{
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-1"},
			},
			Timestamp:  now.Add(-30 * time.Second), // Recent
			Constraint: ConstraintCPU,
		},
		{
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-2"},
			},
			Timestamp:  now.Add(-70 * time.Second), // Too old
			Constraint: ConstraintMemory,
		},
		{
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{Name: "pod-3"},
			},
			Timestamp:  now.Add(-10 * time.Second), // Recent
			Constraint: ConstraintCPU,
		},
	}

	filtered := watcher.filterRecentEvents(events)

	assert.Len(t, filtered, 2, "Should filter out old events")
	assert.Equal(t, "pod-1", filtered[0].Pod.Name)
	assert.Equal(t, "pod-3", filtered[1].Pod.Name)
}

// TestCanScale tests cooldown period enforcement
func TestCanScale(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fakeClient.NewClientBuilder().WithScheme(scheme).Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	watcher := NewEventWatcher(k8sClient, clientset, logger, nil)
	watcher.stabilizationWindow = 60 * time.Second

	// No previous scale event
	assert.True(t, watcher.CanScale("ng-1"), "Should be able to scale with no history")

	// Record a scale event
	watcher.RecordScaleEvent("ng-1")

	// Immediately after, should not be able to scale
	assert.False(t, watcher.CanScale("ng-1"), "Should not scale immediately after previous scale")

	// Simulate time passing by manually setting last scale time
	watcher.lastScaleTimeMu.Lock()
	watcher.lastScaleTime["ng-1"] = time.Now().Add(-70 * time.Second)
	watcher.lastScaleTimeMu.Unlock()

	// After cooldown, should be able to scale
	assert.True(t, watcher.CanScale("ng-1"), "Should be able to scale after cooldown")

	// Different NodeGroup should not be affected
	assert.True(t, watcher.CanScale("ng-2"), "Different NodeGroup should not be affected")
}

// TestRecordScaleEvent tests scale event recording
func TestRecordScaleEvent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fakeClient.NewClientBuilder().WithScheme(scheme).Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	watcher := NewEventWatcher(k8sClient, clientset, logger, nil)

	watcher.RecordScaleEvent("ng-1")

	watcher.lastScaleTimeMu.RLock()
	lastScale, exists := watcher.lastScaleTime["ng-1"]
	watcher.lastScaleTimeMu.RUnlock()

	assert.True(t, exists, "Scale event should be recorded")
	assert.True(t, time.Since(lastScale) < time.Second, "Timestamp should be recent")
}

// TestGetPendingPods tests pending pod retrieval
func TestGetPendingPods(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	// Create some pods
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

	pendingPod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pending-pod-1",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "", // Not scheduled
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
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	deletingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "deleting-pod",
			Namespace:         "default",
			DeletionTimestamp: &metav1.Time{Time: time.Now()},
		},
		Spec: corev1.PodSpec{
			NodeName: "",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending,
		},
	}

	k8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(runningPod, pendingPod1, pendingPod2, deletingPod).
		Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	watcher := NewEventWatcher(k8sClient, clientset, logger, nil)

	pendingPods, err := watcher.GetPendingPods(context.Background())
	require.NoError(t, err)

	assert.Len(t, pendingPods, 2, "Should find 2 pending unscheduled pods")

	podNames := make(map[string]bool)
	for _, pod := range pendingPods {
		podNames[pod.Name] = true
	}

	assert.True(t, podNames["pending-pod-1"])
	assert.True(t, podNames["pending-pod-2"])
	assert.False(t, podNames["running-pod"], "Running pod should not be included")
	assert.False(t, podNames["deleting-pod"], "Deleting pod should not be included")
}

// TestGetNodeGroups tests NodeGroup retrieval
func TestGetNodeGroups(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	ng1 := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ng-1",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes: 1,
			MaxNodes: 10,
		},
	}

	ng2 := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ng-2",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes: 2,
			MaxNodes: 20,
		},
	}

	k8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(ng1, ng2).
		Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	watcher := NewEventWatcher(k8sClient, clientset, logger, nil)

	nodeGroups, err := watcher.GetNodeGroups(context.Background())
	require.NoError(t, err)

	assert.Len(t, nodeGroups, 2, "Should find 2 NodeGroups")

	ngNames := make(map[string]bool)
	for _, ng := range nodeGroups {
		ngNames[ng.Name] = true
	}

	assert.True(t, ngNames["ng-1"])
	assert.True(t, ngNames["ng-2"])
}

// TestEventWatcherScaleUpHandler tests scale-up handler invocation
func TestEventWatcherScaleUpHandler(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	k8sClient := fakeClient.NewClientBuilder().WithScheme(scheme).Build()
	clientset := fake.NewSimpleClientset()
	logger := zap.NewNop()

	var handlerCalled bool
	var handlerEvents []SchedulingEvent

	handler := func(ctx context.Context, events []SchedulingEvent) error {
		handlerCalled = true
		handlerEvents = events
		return nil
	}

	watcher := NewEventWatcher(k8sClient, clientset, logger, handler)
	watcher.stabilizationWindow = 1 * time.Second // Short window for testing

	// Add some events to buffer
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
	}

	watcher.eventBuffer = []SchedulingEvent{
		{
			Pod:        pod,
			Timestamp:  time.Now(),
			Constraint: ConstraintCPU,
			Message:    "Insufficient cpu",
		},
	}

	// Process events
	watcher.processEvents(context.Background())

	assert.True(t, handlerCalled, "Scale-up handler should be called")
	assert.Len(t, handlerEvents, 1, "Handler should receive events")
	assert.Equal(t, "test-pod", handlerEvents[0].Pod.Name)
}
