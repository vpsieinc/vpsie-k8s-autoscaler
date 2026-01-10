//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// NodeGroupBuilder provides a fluent interface for building NodeGroup test objects
type NodeGroupBuilder struct {
	ng *autoscalerv1alpha1.NodeGroup
}

// NewNodeGroupBuilder creates a new NodeGroupBuilder with defaults
func NewNodeGroupBuilder(name string) *NodeGroupBuilder {
	return &NodeGroupBuilder{
		ng: &autoscalerv1alpha1.NodeGroup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: TestNamespace,
			},
			Spec: autoscalerv1alpha1.NodeGroupSpec{
				MinNodes:     1,
				MaxNodes:     5,
				DatacenterID: "dc-test-1",
				OfferingIDs:  []string{"small-2cpu-4gb"},
				OSImageID:    "ubuntu-22.04",
				ScaleUpPolicy: autoscalerv1alpha1.ScaleUpPolicy{
					Enabled:                    true,
					StabilizationWindowSeconds: 60,
					CPUThreshold:               80,
					MemoryThreshold:            80,
					MaxNodesPerScale:           2,
					CooldownSeconds:            120,
				},
				ScaleDownPolicy: autoscalerv1alpha1.ScaleDownPolicy{
					Enabled:                    true,
					StabilizationWindowSeconds: 300,
					CPUThreshold:               50,
					MemoryThreshold:            50,
					MaxNodesPerScale:           1,
					CooldownSeconds:            300,
				},
			},
		},
	}
}

// WithMinNodes sets the minimum node count
func (b *NodeGroupBuilder) WithMinNodes(min int32) *NodeGroupBuilder {
	b.ng.Spec.MinNodes = min
	return b
}

// WithMaxNodes sets the maximum node count
func (b *NodeGroupBuilder) WithMaxNodes(max int32) *NodeGroupBuilder {
	b.ng.Spec.MaxNodes = max
	return b
}

// WithDatacenter sets the datacenter ID
func (b *NodeGroupBuilder) WithDatacenter(dc string) *NodeGroupBuilder {
	b.ng.Spec.DatacenterID = dc
	return b
}

// WithOfferings sets the offering IDs
func (b *NodeGroupBuilder) WithOfferings(offerings ...string) *NodeGroupBuilder {
	b.ng.Spec.OfferingIDs = offerings
	return b
}

// WithLabels sets node labels
func (b *NodeGroupBuilder) WithLabels(labels map[string]string) *NodeGroupBuilder {
	b.ng.Spec.Labels = labels
	return b
}

// WithScaleUpThresholds sets scale-up CPU and memory thresholds
func (b *NodeGroupBuilder) WithScaleUpThresholds(cpu, memory int32) *NodeGroupBuilder {
	b.ng.Spec.ScaleUpPolicy.CPUThreshold = cpu
	b.ng.Spec.ScaleUpPolicy.MemoryThreshold = memory
	return b
}

// WithScaleDownThresholds sets scale-down CPU and memory thresholds
func (b *NodeGroupBuilder) WithScaleDownThresholds(cpu, memory int32) *NodeGroupBuilder {
	b.ng.Spec.ScaleDownPolicy.CPUThreshold = cpu
	b.ng.Spec.ScaleDownPolicy.MemoryThreshold = memory
	return b
}

// Build returns the constructed NodeGroup
func (b *NodeGroupBuilder) Build() *autoscalerv1alpha1.NodeGroup {
	return b.ng
}

// createNodeGroup creates a NodeGroup and waits for it to be ready
func createNodeGroup(ctx context.Context, t *testing.T, ng *autoscalerv1alpha1.NodeGroup) *autoscalerv1alpha1.NodeGroup {
	t.Helper()

	err := k8sClient.Create(ctx, ng)
	require.NoError(t, err, "Failed to create NodeGroup")

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = k8sClient.Delete(cleanupCtx, ng)
	})

	return ng
}

// waitForNodeGroupCondition waits for a NodeGroup to reach a specific condition
func waitForNodeGroupCondition(ctx context.Context, t *testing.T, name string, conditionFn func(*autoscalerv1alpha1.NodeGroup) bool) *autoscalerv1alpha1.NodeGroup {
	t.Helper()

	var ng autoscalerv1alpha1.NodeGroup
	err := wait.PollUntilContextTimeout(ctx, PollInterval, DefaultTimeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: TestNamespace}, &ng); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return conditionFn(&ng), nil
	})
	require.NoError(t, err, "Timeout waiting for NodeGroup condition")

	return &ng
}

// waitForNodeGroupDesiredNodes waits for NodeGroup to have desired node count
func waitForNodeGroupDesiredNodes(ctx context.Context, t *testing.T, name string, count int32) *autoscalerv1alpha1.NodeGroup {
	t.Helper()

	return waitForNodeGroupCondition(ctx, t, name, func(ng *autoscalerv1alpha1.NodeGroup) bool {
		return ng.Status.DesiredNodes == count
	})
}

// waitForNodeGroupCurrentNodes waits for NodeGroup to have current node count
func waitForNodeGroupCurrentNodes(ctx context.Context, t *testing.T, name string, count int32) *autoscalerv1alpha1.NodeGroup {
	t.Helper()

	return waitForNodeGroupCondition(ctx, t, name, func(ng *autoscalerv1alpha1.NodeGroup) bool {
		return ng.Status.CurrentNodes == count
	})
}

// waitForNodeGroupReadyNodes waits for NodeGroup to have ready node count
func waitForNodeGroupReadyNodes(ctx context.Context, t *testing.T, name string, count int32) *autoscalerv1alpha1.NodeGroup {
	t.Helper()

	return waitForNodeGroupCondition(ctx, t, name, func(ng *autoscalerv1alpha1.NodeGroup) bool {
		return ng.Status.ReadyNodes == count
	})
}

// VPSieNodeBuilder provides a fluent interface for building VPSieNode test objects
type VPSieNodeBuilder struct {
	vn *autoscalerv1alpha1.VPSieNode
}

// NewVPSieNodeBuilder creates a new VPSieNodeBuilder with defaults
func NewVPSieNodeBuilder(name, nodeGroupName string) *VPSieNodeBuilder {
	return &VPSieNodeBuilder{
		vn: &autoscalerv1alpha1.VPSieNode{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: TestNamespace,
				Labels: map[string]string{
					"autoscaler.vpsie.com/nodegroup": nodeGroupName,
				},
			},
			Spec: autoscalerv1alpha1.VPSieNodeSpec{
				NodeGroupName: nodeGroupName,
				DatacenterID:  "dc-test-1",
				OfferingID:    "small-2cpu-4gb",
				OSImageID:     "ubuntu-22.04",
			},
		},
	}
}

// WithOffering sets the offering ID
func (b *VPSieNodeBuilder) WithOffering(offering string) *VPSieNodeBuilder {
	b.vn.Spec.OfferingID = offering
	return b
}

// Build returns the constructed VPSieNode
func (b *VPSieNodeBuilder) Build() *autoscalerv1alpha1.VPSieNode {
	return b.vn
}

// waitForVPSieNodePhase waits for a VPSieNode to reach a specific phase
func waitForVPSieNodePhase(ctx context.Context, t *testing.T, name string, phase autoscalerv1alpha1.VPSieNodePhase) *autoscalerv1alpha1.VPSieNode {
	t.Helper()

	var vn autoscalerv1alpha1.VPSieNode
	err := wait.PollUntilContextTimeout(ctx, PollInterval, DefaultTimeout, true, func(ctx context.Context) (bool, error) {
		if err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: TestNamespace}, &vn); err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return vn.Status.Phase == phase, nil
	})
	require.NoError(t, err, "Timeout waiting for VPSieNode phase %s", phase)

	return &vn
}

// createUnschedulablePod creates a pod that requests more resources than available
// This triggers scale-up in the autoscaler
func createUnschedulablePod(ctx context.Context, t *testing.T, name string, cpuRequest, memoryRequest string) *corev1.Pod {
	t.Helper()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: TestNamespace,
			Labels: map[string]string{
				"app":                     "e2e-test",
				"e2e.vpsie.com/test-type": "scale-trigger",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "nginx:alpine",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(cpuRequest),
							corev1.ResourceMemory: resource.MustParse(memoryRequest),
						},
					},
				},
			},
			// Use node selector that matches autoscaler-managed nodes
			NodeSelector: map[string]string{
				"autoscaler.vpsie.com/managed": "true",
			},
		},
	}

	_, err := clientset.CoreV1().Pods(TestNamespace).Create(ctx, pod, metav1.CreateOptions{})
	require.NoError(t, err, "Failed to create unschedulable pod")

	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = clientset.CoreV1().Pods(TestNamespace).Delete(cleanupCtx, name, metav1.DeleteOptions{})
	})

	return pod
}

// waitForPodCondition waits for a pod to reach a specific condition
func waitForPodCondition(ctx context.Context, t *testing.T, name string, conditionFn func(*corev1.Pod) bool) *corev1.Pod {
	t.Helper()

	var pod *corev1.Pod
	err := wait.PollUntilContextTimeout(ctx, PollInterval, DefaultTimeout, true, func(ctx context.Context) (bool, error) {
		var err error
		pod, err = clientset.CoreV1().Pods(TestNamespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return conditionFn(pod), nil
	})
	require.NoError(t, err, "Timeout waiting for pod condition")

	return pod
}

// waitForPodScheduled waits for a pod to be scheduled (not pending)
func waitForPodScheduled(ctx context.Context, t *testing.T, name string) *corev1.Pod {
	t.Helper()

	return waitForPodCondition(ctx, t, name, func(pod *corev1.Pod) bool {
		return pod.Spec.NodeName != "" || pod.Status.Phase != corev1.PodPending
	})
}

// waitForPodUnschedulable waits for a pod to have an unschedulable condition
func waitForPodUnschedulable(ctx context.Context, t *testing.T, name string) *corev1.Pod {
	t.Helper()

	return waitForPodCondition(ctx, t, name, func(pod *corev1.Pod) bool {
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodScheduled &&
				condition.Status == corev1.ConditionFalse &&
				condition.Reason == corev1.PodReasonUnschedulable {
				return true
			}
		}
		return false
	})
}

// getNodeGroupVPSieNodes returns all VPSieNodes belonging to a NodeGroup
func getNodeGroupVPSieNodes(ctx context.Context, t *testing.T, nodeGroupName string) []autoscalerv1alpha1.VPSieNode {
	t.Helper()

	vnList := &autoscalerv1alpha1.VPSieNodeList{}
	err := k8sClient.List(ctx, vnList, client.InNamespace(TestNamespace), client.MatchingLabels{
		"autoscaler.vpsie.com/nodegroup": nodeGroupName,
	})
	require.NoError(t, err, "Failed to list VPSieNodes")

	return vnList.Items
}

// assertNodeGroupStatus asserts various status fields on a NodeGroup
func assertNodeGroupStatus(t *testing.T, ng *autoscalerv1alpha1.NodeGroup, desired, current, ready int32) {
	t.Helper()

	if desired >= 0 {
		require.Equal(t, desired, ng.Status.DesiredNodes, "DesiredNodes mismatch")
	}
	if current >= 0 {
		require.Equal(t, current, ng.Status.CurrentNodes, "CurrentNodes mismatch")
	}
	if ready >= 0 {
		require.Equal(t, ready, ng.Status.ReadyNodes, "ReadyNodes mismatch")
	}
}

// assertEventExists checks if a Kubernetes event with the given reason exists
func assertEventExists(ctx context.Context, t *testing.T, namespace, reason string) {
	t.Helper()

	events, err := clientset.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	require.NoError(t, err, "Failed to list events")

	found := false
	for _, event := range events.Items {
		if event.Reason == reason {
			found = true
			break
		}
	}
	require.True(t, found, "Expected event with reason %s not found", reason)
}

// logNodeGroupStatus logs the current status of a NodeGroup for debugging
func logNodeGroupStatus(t *testing.T, ng *autoscalerv1alpha1.NodeGroup) {
	t.Helper()

	t.Logf("NodeGroup %s status: Desired=%d, Current=%d, Ready=%d, Phase=%s",
		ng.Name,
		ng.Status.DesiredNodes,
		ng.Status.CurrentNodes,
		ng.Status.ReadyNodes,
		ng.Status.Phase,
	)

	for _, condition := range ng.Status.Conditions {
		t.Logf("  Condition %s: %s (Reason: %s)",
			condition.Type,
			condition.Status,
			condition.Reason,
		)
	}
}

// retryOnConflict retries an operation if it fails due to a conflict
func retryOnConflict(ctx context.Context, t *testing.T, fn func() error) error {
	t.Helper()

	var lastErr error
	err := wait.PollUntilContextTimeout(ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		lastErr = fn()
		if lastErr == nil {
			return true, nil
		}
		if errors.IsConflict(lastErr) {
			return false, nil // Retry
		}
		return false, lastErr // Don't retry other errors
	})
	if err != nil && lastErr != nil {
		return fmt.Errorf("operation failed after retries: %v", lastErr)
	}
	return err
}
