package scaler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
)

// TestIsDaemonSetPod tests the isDaemonSetPod helper function
func TestIsDaemonSetPod(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "DaemonSet pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "fluentd-abc123",
					Namespace: "kube-system",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "DaemonSet",
							Name: "fluentd",
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Deployment pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-abc123",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "nginx-deployment-abc123",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "StatefulSet pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mysql-0",
					Namespace: "database",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "StatefulSet",
							Name: "mysql",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Pod without owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "standalone-pod",
					Namespace: "default",
				},
			},
			expected: false,
		},
		{
			name: "Multiple owners including DaemonSet",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-owner-pod",
					Namespace: "kube-system",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "some-rs",
						},
						{
							Kind: "DaemonSet",
							Name: "node-exporter",
						},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isDaemonSetPod(tt.pod)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsStaticPod tests the isStaticPod helper function
func TestIsStaticPod(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name: "Static pod with Node owner",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-apiserver-master",
					Namespace: "kube-system",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "Node",
							Name: "master-node",
						},
					},
				},
			},
			expected: true,
		},
		{
			name: "Static pod with config.source annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "etcd-master",
					Namespace: "kube-system",
					Annotations: map[string]string{
						"kubernetes.io/config.source": "file",
					},
				},
			},
			expected: true,
		},
		{
			name: "Static pod with config.mirror annotation",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "kube-scheduler-master",
					Namespace: "kube-system",
					Annotations: map[string]string{
						"kubernetes.io/config.mirror": "abc123",
					},
				},
			},
			expected: true,
		},
		{
			name: "Regular pod",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "nginx-abc123",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							Kind: "ReplicaSet",
							Name: "nginx-rs",
						},
					},
				},
			},
			expected: false,
		},
		{
			name: "Pod with other annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "app-pod",
					Namespace: "production",
					Annotations: map[string]string{
						"app.kubernetes.io/version": "v1.0.0",
					},
				},
			},
			expected: false,
		},
		{
			name: "Pod with nil annotations",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-annotations-pod",
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

// TestIsNodeDraining tests the IsNodeDraining function
func TestIsNodeDraining(t *testing.T) {
	tests := []struct {
		name     string
		node     *corev1.Node
		expected bool
	}{
		{
			name: "Node is draining",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Annotations: map[string]string{
						DrainStatusAnnotation: "draining",
					},
				},
			},
			expected: true,
		},
		{
			name: "Node drain completed",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-2",
					Annotations: map[string]string{
						DrainStatusAnnotation: "complete",
					},
				},
			},
			expected: false,
		},
		{
			name: "Node drain failed",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-3",
					Annotations: map[string]string{
						DrainStatusAnnotation: "failed",
					},
				},
			},
			expected: false,
		},
		{
			name: "Node without drain status",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-4",
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "Node with nil annotations",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-5",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsNodeDraining(tt.node)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetNodeDrainDuration tests the GetNodeDrainDuration function
func TestGetNodeDrainDuration(t *testing.T) {
	now := time.Now()
	fiveMinutesAgo := now.Add(-5 * time.Minute)
	oneHourAgo := now.Add(-1 * time.Hour)

	tests := []struct {
		name            string
		node            *corev1.Node
		expectedMin     time.Duration
		expectedMax     time.Duration
		expectZero      bool
	}{
		{
			name: "Node draining for 5 minutes",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Annotations: map[string]string{
						DrainStartTimeAnnotation: fiveMinutesAgo.Format(time.RFC3339),
					},
				},
			},
			expectedMin: 4*time.Minute + 50*time.Second,
			expectedMax: 5*time.Minute + 10*time.Second,
			expectZero:  false,
		},
		{
			name: "Node draining for 1 hour",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-2",
					Annotations: map[string]string{
						DrainStartTimeAnnotation: oneHourAgo.Format(time.RFC3339),
					},
				},
			},
			expectedMin: 59*time.Minute + 50*time.Second,
			expectedMax: 60*time.Minute + 10*time.Second,
			expectZero:  false,
		},
		{
			name: "Node without drain start time",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-3",
					Annotations: map[string]string{
						"other-annotation": "value",
					},
				},
			},
			expectZero: true,
		},
		{
			name: "Node with nil annotations",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-4",
				},
			},
			expectZero: true,
		},
		{
			name: "Node with invalid time format",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-5",
					Annotations: map[string]string{
						DrainStartTimeAnnotation: "invalid-time",
					},
				},
			},
			expectZero: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetNodeDrainDuration(tt.node)

			if tt.expectZero {
				assert.Equal(t, time.Duration(0), result)
			} else {
				assert.GreaterOrEqual(t, result, tt.expectedMin, "Duration should be at least %v", tt.expectedMin)
				assert.LessOrEqual(t, result, tt.expectedMax, "Duration should be at most %v", tt.expectedMax)
			}
		})
	}
}

// TestFilterPodsForEviction tests the filterPodsForEviction method
func TestFilterPodsForEviction(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := &ScaleDownManager{
		logger: logger.Sugar(),
	}

	tests := []struct {
		name          string
		pods          []*corev1.Pod
		expectedCount int
		expectedNames []string
	}{
		{
			name: "Filter out DaemonSet pods",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app-pod",
						Namespace: "default",
						OwnerReferences: []metav1.OwnerReference{
							{Kind: "ReplicaSet", Name: "app-rs"},
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "daemon-pod",
						Namespace: "kube-system",
						OwnerReferences: []metav1.OwnerReference{
							{Kind: "DaemonSet", Name: "fluentd"},
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
			},
			expectedCount: 1,
			expectedNames: []string{"app-pod"},
		},
		{
			name: "Filter out static pods",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app-pod",
						Namespace: "default",
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "kube-apiserver",
						Namespace: "kube-system",
						Annotations: map[string]string{
							"kubernetes.io/config.mirror": "abc123",
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
			},
			expectedCount: 1,
			expectedNames: []string{"app-pod"},
		},
		{
			name: "Filter out completed/failed pods",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "running-pod",
						Namespace: "default",
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "succeeded-pod",
						Namespace: "default",
					},
					Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "failed-pod",
						Namespace: "default",
					},
					Status: corev1.PodStatus{Phase: corev1.PodFailed},
				},
			},
			expectedCount: 1,
			expectedNames: []string{"running-pod"},
		},
		{
			name: "All pods should be evicted",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app-1",
						Namespace: "default",
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "app-2",
						Namespace: "production",
					},
					Status: corev1.PodStatus{Phase: corev1.PodPending},
				},
			},
			expectedCount: 2,
			expectedNames: []string{"app-1", "app-2"},
		},
		{
			name:          "Empty pod list",
			pods:          []*corev1.Pod{},
			expectedCount: 0,
			expectedNames: []string{},
		},
		{
			name: "All pods should be filtered",
			pods: []*corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "daemon-pod",
						Namespace: "kube-system",
						OwnerReferences: []metav1.OwnerReference{
							{Kind: "DaemonSet", Name: "fluentd"},
						},
					},
					Status: corev1.PodStatus{Phase: corev1.PodRunning},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "succeeded-job",
						Namespace: "batch",
					},
					Status: corev1.PodStatus{Phase: corev1.PodSucceeded},
				},
			},
			expectedCount: 0,
			expectedNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.filterPodsForEviction(tt.pods)

			assert.Equal(t, tt.expectedCount, len(result), "Expected %d pods after filtering", tt.expectedCount)

			resultNames := make([]string, len(result))
			for i, pod := range result {
				resultNames[i] = pod.Name
			}

			for _, expectedName := range tt.expectedNames {
				assert.Contains(t, resultNames, expectedName, "Expected pod %s in filtered results", expectedName)
			}
		})
	}
}

// TestCordonNode tests the cordonNode method
func TestCordonNode(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name           string
		node           *corev1.Node
		expectChange   bool
		expectError    bool
	}{
		{
			name: "Cordon schedulable node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
				},
				Spec: corev1.NodeSpec{
					Unschedulable: false,
				},
			},
			expectChange: true,
			expectError:  false,
		},
		{
			name: "Already cordoned node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-2",
				},
				Spec: corev1.NodeSpec{
					Unschedulable: true,
				},
			},
			expectChange: false,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{tt.node}
			fakeClient := fake.NewSimpleClientset(objects...)

			manager := &ScaleDownManager{
				client: fakeClient,
				logger: logger.Sugar(),
			}

			err := manager.cordonNode(ctx, tt.node)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify node state
				updatedNode, err := fakeClient.CoreV1().Nodes().Get(ctx, tt.node.Name, metav1.GetOptions{})
				require.NoError(t, err)

				if tt.expectChange {
					assert.True(t, updatedNode.Spec.Unschedulable, "Node should be cordoned")
				} else {
					assert.True(t, updatedNode.Spec.Unschedulable, "Node should remain cordoned")
				}
			}
		})
	}
}

// TestUncordonNode tests the uncordonNode method
func TestUncordonNode(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name           string
		node           *corev1.Node
		expectChange   bool
		expectError    bool
	}{
		{
			name: "Uncordon cordoned node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
				},
				Spec: corev1.NodeSpec{
					Unschedulable: true,
				},
			},
			expectChange: true,
			expectError:  false,
		},
		{
			name: "Already schedulable node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-2",
				},
				Spec: corev1.NodeSpec{
					Unschedulable: false,
				},
			},
			expectChange: false,
			expectError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{tt.node}
			fakeClient := fake.NewSimpleClientset(objects...)

			manager := &ScaleDownManager{
				client: fakeClient,
				logger: logger.Sugar(),
			}

			err := manager.uncordonNode(ctx, tt.node)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify node state
				updatedNode, err := fakeClient.CoreV1().Nodes().Get(ctx, tt.node.Name, metav1.GetOptions{})
				require.NoError(t, err)

				assert.False(t, updatedNode.Spec.Unschedulable, "Node should be schedulable")
			}
		})
	}
}

// TestAnnotateNodeDrainStart tests the annotateNodeDrainStart method
func TestAnnotateNodeDrainStart(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name        string
		node        *corev1.Node
		expectError bool
	}{
		{
			name: "Node without existing annotations",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
				},
			},
			expectError: false,
		},
		{
			name: "Node with existing annotations",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-2",
					Annotations: map[string]string{
						"existing": "annotation",
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{tt.node}
			fakeClient := fake.NewSimpleClientset(objects...)

			manager := &ScaleDownManager{
				client: fakeClient,
				logger: logger.Sugar(),
			}

			// Truncate to second precision since RFC3339 format only has second precision
			beforeTime := time.Now().Truncate(time.Second)
			err := manager.annotateNodeDrainStart(ctx, tt.node)
			afterTime := time.Now().Add(time.Second).Truncate(time.Second) // Add 1s buffer for time drift

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify annotations
				updatedNode, err := fakeClient.CoreV1().Nodes().Get(ctx, tt.node.Name, metav1.GetOptions{})
				require.NoError(t, err)

				assert.NotNil(t, updatedNode.Annotations)
				assert.Equal(t, "draining", updatedNode.Annotations[DrainStatusAnnotation])

				// Check start time is within expected range
				startTimeStr := updatedNode.Annotations[DrainStartTimeAnnotation]
				startTime, err := time.Parse(time.RFC3339, startTimeStr)
				require.NoError(t, err)

				// Compare with second precision (RFC3339 only stores seconds)
				assert.True(t, !startTime.Before(beforeTime), "Start time should be after test start")
				assert.True(t, !startTime.After(afterTime), "Start time should be before test end")
			}
		})
	}
}

// TestAnnotateNodeDrainStatus tests the annotateNodeDrainStatus method
func TestAnnotateNodeDrainStatus(t *testing.T) {
	ctx := context.Background()
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name         string
		node         *corev1.Node
		status       string
		expectError  bool
	}{
		{
			name: "Set status to complete",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-1",
					Annotations: map[string]string{
						DrainStatusAnnotation: "draining",
					},
				},
			},
			status:      "complete",
			expectError: false,
		},
		{
			name: "Set status to failed",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-2",
					Annotations: map[string]string{
						DrainStatusAnnotation: "draining",
					},
				},
			},
			status:      "failed",
			expectError: false,
		},
		{
			name: "Set status on node without annotations",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "worker-3",
				},
			},
			status:      "timeout",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := []runtime.Object{tt.node}
			fakeClient := fake.NewSimpleClientset(objects...)

			manager := &ScaleDownManager{
				client: fakeClient,
				logger: logger.Sugar(),
			}

			err := manager.annotateNodeDrainStatus(ctx, tt.node, tt.status)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				// Verify annotations
				updatedNode, err := fakeClient.CoreV1().Nodes().Get(ctx, tt.node.Name, metav1.GetOptions{})
				require.NoError(t, err)

				assert.NotNil(t, updatedNode.Annotations)
				assert.Equal(t, tt.status, updatedNode.Annotations[DrainStatusAnnotation])
			}
		})
	}
}

// TestDrainConstants tests that drain constants are properly defined
func TestDrainConstants(t *testing.T) {
	assert.Equal(t, 5*time.Second, EvictionRetryInterval, "Eviction retry interval should be 5 seconds")
	assert.Equal(t, 12, MaxEvictionRetries, "Max eviction retries should be 12")
	assert.Equal(t, "autoscaler.vpsie.com/drain-start-time", DrainStartTimeAnnotation)
	assert.Equal(t, "autoscaler.vpsie.com/drain-status", DrainStatusAnnotation)
}
