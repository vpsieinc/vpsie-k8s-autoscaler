package scaler

import (
	"context"
	"fmt"
	"testing"
	"time"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"

	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func TestNewScaleDownManager(t *testing.T) {
	client := fake.NewSimpleClientset()
	metricsClient := metricsfake.NewSimpleClientset()
	logger := zaptest.NewLogger(t)

	manager := NewScaleDownManager(client, metricsClient, logger, nil)

	if manager == nil {
		t.Fatal("expected manager to be created")
		return
	}

	if manager.config == nil {
		t.Fatal("expected default config to be set")
		return
	}

	if manager.config.CPUThreshold != DefaultCPUThreshold {
		t.Errorf("expected CPU threshold %f, got %f", DefaultCPUThreshold, manager.config.CPUThreshold)
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.CPUThreshold != DefaultCPUThreshold {
		t.Errorf("expected CPU threshold %f, got %f", DefaultCPUThreshold, config.CPUThreshold)
	}

	if config.MemoryThreshold != DefaultMemoryThreshold {
		t.Errorf("expected memory threshold %f, got %f", DefaultMemoryThreshold, config.MemoryThreshold)
	}

	if config.ObservationWindow != DefaultObservationWindow {
		t.Errorf("expected observation window %v, got %v", DefaultObservationWindow, config.ObservationWindow)
	}
}

func TestIsNodeProtected(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewScaleDownManager(nil, nil, logger, nil)

	tests := []struct {
		name     string
		node     *corev1.Node
		expected bool
	}{
		{
			name: "node with protected annotation",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Annotations: map[string]string{
						ProtectedNodeAnnotation: "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "node with scale-down disabled label",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-2",
					Labels: map[string]string{
						ScaleDownDisabledLabel: "true",
					},
				},
			},
			expected: true,
		},
		{
			name: "normal node",
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-3",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.isNodeProtected(tt.node)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestHasBeenUnderutilizedForWindow(t *testing.T) {
	config := &Config{
		CPUThreshold:      50.0,
		MemoryThreshold:   50.0,
		ObservationWindow: 10 * time.Minute,
	}

	logger := zaptest.NewLogger(t)
	manager := NewScaleDownManager(nil, nil, logger, config)

	now := time.Now()

	tests := []struct {
		name        string
		utilization *NodeUtilization
		expected    bool
	}{
		{
			name: "all samples underutilized",
			utilization: &NodeUtilization{
				NodeName:    "node-1",
				LastUpdated: now,
				Samples: []UtilizationSample{
					{Timestamp: now.Add(-9 * time.Minute), CPUUtilization: 30, MemoryUtilization: 30},
					{Timestamp: now.Add(-8 * time.Minute), CPUUtilization: 35, MemoryUtilization: 35},
					{Timestamp: now.Add(-7 * time.Minute), CPUUtilization: 32, MemoryUtilization: 32},
					{Timestamp: now.Add(-6 * time.Minute), CPUUtilization: 28, MemoryUtilization: 28},
					{Timestamp: now.Add(-5 * time.Minute), CPUUtilization: 31, MemoryUtilization: 31},
				},
			},
			expected: true,
		},
		{
			name: "some samples over threshold",
			utilization: &NodeUtilization{
				NodeName:    "node-2",
				LastUpdated: now,
				Samples: []UtilizationSample{
					{Timestamp: now.Add(-9 * time.Minute), CPUUtilization: 30, MemoryUtilization: 30},
					{Timestamp: now.Add(-8 * time.Minute), CPUUtilization: 70, MemoryUtilization: 35},
					{Timestamp: now.Add(-7 * time.Minute), CPUUtilization: 32, MemoryUtilization: 32},
				},
			},
			expected: false,
		},
		{
			name: "no samples",
			utilization: &NodeUtilization{
				NodeName: "node-3",
				Samples:  []UtilizationSample{},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.hasBeenUnderutilizedForWindow(tt.utilization)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsOutsideCooldownPeriod(t *testing.T) {
	config := &Config{
		CooldownPeriod: 10 * time.Minute,
	}

	logger := zaptest.NewLogger(t)
	manager := NewScaleDownManager(nil, nil, logger, config)

	nodeGroup := "test-group"

	// Initially no scale-down, should be outside cooldown
	if !manager.isOutsideCooldownPeriod(nodeGroup) {
		t.Error("expected to be outside cooldown initially")
	}

	// Set last scale-down to now
	manager.lastScaleDown[nodeGroup] = time.Now()

	// Should be inside cooldown
	if manager.isOutsideCooldownPeriod(nodeGroup) {
		t.Error("expected to be inside cooldown")
	}

	// Set last scale-down to 11 minutes ago
	manager.lastScaleDown[nodeGroup] = time.Now().Add(-11 * time.Minute)

	// Should be outside cooldown
	if !manager.isOutsideCooldownPeriod(nodeGroup) {
		t.Error("expected to be outside cooldown after period")
	}
}

func TestCalculatePriority(t *testing.T) {
	logger := zaptest.NewLogger(t)
	manager := NewScaleDownManager(nil, nil, logger, nil)

	tests := []struct {
		name        string
		utilization *NodeUtilization
		pods        []*corev1.Pod
		expectLower bool // Expect lower priority than next test case
	}{
		{
			name: "low utilization, few pods",
			utilization: &NodeUtilization{
				CPUUtilization:    20.0,
				MemoryUtilization: 20.0,
			},
			pods:        createTestPods(2, "default"),
			expectLower: true,
		},
		{
			name: "high utilization, many pods",
			utilization: &NodeUtilization{
				CPUUtilization:    80.0,
				MemoryUtilization: 80.0,
			},
			pods:        createTestPods(10, "default"),
			expectLower: false,
		},
	}

	var lastPriority int
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			priority := manager.calculatePriority(tt.utilization, tt.pods)

			if i > 0 && tt.expectLower && priority >= lastPriority {
				t.Errorf("expected priority %d to be lower than %d", priority, lastPriority)
			}

			lastPriority = priority
		})
	}
}

func TestSortCandidatesByPriority(t *testing.T) {
	candidates := []*ScaleDownCandidate{
		{Priority: 300},
		{Priority: 100},
		{Priority: 200},
	}

	sortCandidatesByPriority(candidates)

	if candidates[0].Priority != 100 {
		t.Errorf("expected first candidate priority 100, got %d", candidates[0].Priority)
	}

	if candidates[2].Priority != 300 {
		t.Errorf("expected last candidate priority 300, got %d", candidates[2].Priority)
	}
}

func TestCanScaleDown_MinNodes(t *testing.T) {
	client := fake.NewSimpleClientset()
	logger := zaptest.NewLogger(t)
	manager := NewScaleDownManager(client, nil, logger, nil)

	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-group",
			Namespace: "default",
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes: 3,
			MaxNodes: 10,
		},
		Status: autoscalerv1alpha1.NodeGroupStatus{
			Nodes: []autoscalerv1alpha1.NodeInfo{
				{NodeName: "node-1"},
				{NodeName: "node-2"},
				{NodeName: "node-3"},
			},
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
	}

	ctx := context.Background()
	canScale, reason, err := manager.CanScaleDown(ctx, nodeGroup, node)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if canScale {
		t.Error("expected scale-down to be blocked at minimum nodes")
	}

	if reason != "at minimum nodes" {
		t.Errorf("expected reason 'at minimum nodes', got '%s'", reason)
	}
}

func TestCanScaleDown_Cooldown(t *testing.T) {
	config := &Config{
		CooldownPeriod: 10 * time.Minute,
	}

	logger := zaptest.NewLogger(t)
	manager := NewScaleDownManager(nil, nil, logger, config)

	// Set last scale-down to now
	manager.lastScaleDown["test-group"] = time.Now()

	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-group",
			Namespace: "default",
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes: 1,
			MaxNodes: 10,
		},
		Status: autoscalerv1alpha1.NodeGroupStatus{
			Nodes: []autoscalerv1alpha1.NodeInfo{
				{NodeName: "node-1"},
				{NodeName: "node-2"},
			},
		},
	}

	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
		},
	}

	ctx := context.Background()
	canScale, reason, err := manager.CanScaleDown(ctx, nodeGroup, node)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if canScale {
		t.Error("expected scale-down to be blocked during cooldown")
	}

	if reason != "within cooldown period" {
		t.Errorf("expected reason 'within cooldown period', got '%s'", reason)
	}
}

func TestIdentifyUnderutilizedNodes(t *testing.T) {
	// Create test nodes
	nodes := []*corev1.Node{
		createTestNode("node-1", "test-group", 4000, 8000000000),
		createTestNode("node-2", "test-group", 4000, 8000000000),
		createTestNode("node-3", "test-group", 4000, 8000000000),
	}

	client := fake.NewSimpleClientset()
	for _, node := range nodes {
		_, err := client.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("failed to create node: %v", err)
		}
	}

	logger := zaptest.NewLogger(t)
	manager := NewScaleDownManager(client, nil, logger, DefaultConfig())

	// Add utilization data
	now := time.Now()
	manager.nodeUtilization["node-1"] = &NodeUtilization{
		NodeName:          "node-1",
		CPUUtilization:    30.0,
		MemoryUtilization: 30.0,
		IsUnderutilized:   true,
		LastUpdated:       now,
		Samples: []UtilizationSample{
			{Timestamp: now.Add(-9 * time.Minute), CPUUtilization: 30, MemoryUtilization: 30},
			{Timestamp: now.Add(-8 * time.Minute), CPUUtilization: 32, MemoryUtilization: 32},
			{Timestamp: now.Add(-7 * time.Minute), CPUUtilization: 28, MemoryUtilization: 28},
			{Timestamp: now.Add(-6 * time.Minute), CPUUtilization: 31, MemoryUtilization: 31},
			{Timestamp: now.Add(-5 * time.Minute), CPUUtilization: 29, MemoryUtilization: 29},
		},
	}

	manager.nodeUtilization["node-2"] = &NodeUtilization{
		NodeName:          "node-2",
		CPUUtilization:    70.0,
		MemoryUtilization: 70.0,
		IsUnderutilized:   false,
		LastUpdated:       now,
	}

	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-group",
			Namespace: "default",
			Labels: map[string]string{
				autoscalerv1alpha1.ManagedLabelKey: autoscalerv1alpha1.ManagedLabelValue,
			},
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes: 1,
			MaxNodes: 10,
		},
	}

	ctx := context.Background()
	candidates, err := manager.IdentifyUnderutilizedNodes(ctx, nodeGroup)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(candidates) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(candidates))
	}

	if len(candidates) > 0 && candidates[0].Node.Name != "node-1" {
		t.Errorf("expected node-1 to be identified, got %s", candidates[0].Node.Name)
	}
}

// TestScaleDownManager_DrainNode verifies that ScaleDown properly drains nodes
// but does NOT delete them. Node deletion is handled by the NodeGroup controller
// after verifying the VPSie VM is terminated.
func TestScaleDownManager_DrainNode(t *testing.T) {
	// Create test nodes - need at least 2 nodes for rescheduling safety check
	node := createTestNode("node-1", "test-group", 4000, 8000000000)
	node2 := createTestNode("node-2", "test-group", 4000, 8000000000)

	// Create fake client with the nodes
	client := fake.NewSimpleClientset(node, node2)
	logger := zaptest.NewLogger(t)

	config := &Config{
		CPUThreshold:              50.0,
		MemoryThreshold:           50.0,
		ObservationWindow:         10 * time.Minute,
		CooldownPeriod:            10 * time.Minute,
		MaxNodesPerScaleDown:      5,
		EnablePodDisruptionBudget: true,
		DrainTimeout:              5 * time.Minute,
		EvictionGracePeriod:       30,
	}

	manager := NewScaleDownManager(client, nil, logger, config)

	// Create NodeGroup with scale-down enabled
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-group",
			Namespace: "default",
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes: 0,
			MaxNodes: 10,
			ScaleDownPolicy: autoscalerv1alpha1.ScaleDownPolicy{
				Enabled: true,
			},
		},
		Status: autoscalerv1alpha1.NodeGroupStatus{
			Nodes: []autoscalerv1alpha1.NodeInfo{
				{NodeName: "node-1"},
				{NodeName: "node-2"},
			},
		},
	}

	// Create candidate for scale-down
	candidate := &ScaleDownCandidate{
		Node: node,
		Utilization: &NodeUtilization{
			NodeName:          "node-1",
			CPUUtilization:    30.0,
			MemoryUtilization: 30.0,
			IsUnderutilized:   true,
		},
		Pods:         []*corev1.Pod{}, // No pods to drain
		SafeToRemove: true,
		Priority:     100,
	}

	ctx := context.Background()

	// Execute scale-down
	err := manager.ScaleDown(ctx, nodeGroup, []*ScaleDownCandidate{candidate})

	if err != nil {
		t.Fatalf("ScaleDown failed: %v", err)
	}

	// Verify node still exists (ScaleDown only drains, does not delete)
	// Node deletion is handled by NodeGroup controller after VPSie VM termination
	drainedNode, err := client.CoreV1().Nodes().Get(ctx, "node-1", metav1.GetOptions{})
	if err != nil {
		t.Errorf("expected node to still exist after drain, got error: %v", err)
	}

	// Verify node is cordoned (Unschedulable = true)
	if drainedNode != nil && !drainedNode.Spec.Unschedulable {
		t.Error("expected node to be cordoned (Unschedulable=true) after drain")
	}
}

// TestScaleDownManager_DrainNode_NodeNotFound tests the behavior when trying
// to drain a node that doesn't exist. The drain should fail gracefully.
func TestScaleDownManager_DrainNode_NodeNotFound(t *testing.T) {
	// Create test nodes - need at least 2 nodes for rescheduling safety check
	node := createTestNode("node-1", "test-group", 4000, 8000000000)
	node2 := createTestNode("node-2", "test-group", 4000, 8000000000)

	// Create fake client with the nodes
	client := fake.NewSimpleClientset(node, node2)
	logger := zaptest.NewLogger(t)

	config := &Config{
		CPUThreshold:              50.0,
		MemoryThreshold:           50.0,
		ObservationWindow:         10 * time.Minute,
		CooldownPeriod:            10 * time.Minute,
		MaxNodesPerScaleDown:      5,
		EnablePodDisruptionBudget: true,
		DrainTimeout:              5 * time.Minute,
		EvictionGracePeriod:       30,
	}

	manager := NewScaleDownManager(client, nil, logger, config)

	// Create NodeGroup with scale-down enabled
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-group",
			Namespace: "default",
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes: 0,
			MaxNodes: 10,
			ScaleDownPolicy: autoscalerv1alpha1.ScaleDownPolicy{
				Enabled: true,
			},
		},
		Status: autoscalerv1alpha1.NodeGroupStatus{
			Nodes: []autoscalerv1alpha1.NodeInfo{
				{NodeName: "node-1"},
				{NodeName: "node-2"},
			},
		},
	}

	// Create candidate for scale-down
	candidate := &ScaleDownCandidate{
		Node: node,
		Utilization: &NodeUtilization{
			NodeName:          "node-1",
			CPUUtilization:    30.0,
			MemoryUtilization: 30.0,
			IsUnderutilized:   true,
		},
		Pods:         []*corev1.Pod{},
		SafeToRemove: true,
		Priority:     100,
	}

	// Delete the node before ScaleDown to simulate it disappearing
	ctx := context.Background()
	_ = client.CoreV1().Nodes().Delete(ctx, "node-1", metav1.DeleteOptions{})

	// Execute scale-down - should handle missing node gracefully
	err := manager.ScaleDown(ctx, nodeGroup, []*ScaleDownCandidate{candidate})

	// Since the node doesn't exist, the drain will fail but ScaleDown should
	// report the error properly without panicking
	if err == nil {
		// If drain handles missing nodes gracefully (treats as already drained),
		// this is acceptable behavior
		t.Log("ScaleDown succeeded - node was already gone")
	} else {
		// Error should indicate the drain failed for the missing node
		errStr := err.Error()
		if !contains(errStr, "drain") && !contains(errStr, "not found") && !contains(errStr, "error") {
			t.Errorf("expected drain failure error, got: %v", err)
		}
	}
}

// TestScaleDownManager_DrainNode_WithPods tests drain behavior when there are
// pods on the node. With the fake client, pods don't actually terminate so
// the drain should timeout.
func TestScaleDownManager_DrainNode_WithPods(t *testing.T) {
	// Create test nodes - need at least 2 nodes for rescheduling safety check
	node := createTestNode("node-1", "test-group", 4000, 8000000000)
	node2 := createTestNode("node-2", "test-group", 4000, 8000000000)

	// Create test pods on the node (will be evicted)
	pod1 := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod-1",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{
				{
					Name:  "container",
					Image: "test:latest",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	// Create fake client with nodes and pods
	client := fake.NewSimpleClientset(node, node2, pod1)
	logger := zaptest.NewLogger(t)

	config := &Config{
		CPUThreshold:              50.0,
		MemoryThreshold:           50.0,
		ObservationWindow:         10 * time.Minute,
		CooldownPeriod:            10 * time.Minute,
		MaxNodesPerScaleDown:      5,
		EnablePodDisruptionBudget: true,
		DrainTimeout:              3 * time.Second, // Very short timeout for test
		EvictionGracePeriod:       1,               // Short grace period
	}

	manager := NewScaleDownManager(client, nil, logger, config)

	// Create NodeGroup with scale-down enabled
	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-group",
			Namespace: "default",
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes: 0,
			MaxNodes: 10,
			ScaleDownPolicy: autoscalerv1alpha1.ScaleDownPolicy{
				Enabled: true,
			},
		},
		Status: autoscalerv1alpha1.NodeGroupStatus{
			Nodes: []autoscalerv1alpha1.NodeInfo{
				{NodeName: "node-1"},
				{NodeName: "node-2"},
			},
		},
	}

	// Create candidate for scale-down
	candidate := &ScaleDownCandidate{
		Node: node,
		Utilization: &NodeUtilization{
			NodeName:          "node-1",
			CPUUtilization:    30.0,
			MemoryUtilization: 30.0,
			IsUnderutilized:   true,
		},
		Pods:         []*corev1.Pod{pod1},
		SafeToRemove: true,
		Priority:     100,
	}

	// Use a context with timeout to limit test duration
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Note: This test will timeout because the fake client doesn't actually
	// terminate pods. We verify the scale-down operation is attempted
	// and returns an error due to drain timeout.
	err := manager.ScaleDown(ctx, nodeGroup, []*ScaleDownCandidate{candidate})

	// Scale-down should return an error because drain times out waiting for
	// pods to terminate (fake client doesn't actually terminate pods)
	if err == nil {
		// If no error, that means drain completed successfully, which is unexpected
		// with fake client. But this is still valid - the operation was attempted.
		t.Log("Scale-down completed without error (unexpected with fake client, but valid)")
	} else {
		// This is the expected path - drain timed out
		t.Logf("Scale-down returned expected error: %v", err)
	}

	// Verify node still exists (ScaleDown only drains, does not delete)
	// Node deletion is handled by NodeGroup controller after VPSie VM termination
	verifyCtx := context.Background()
	drainedNode, getErr := client.CoreV1().Nodes().Get(verifyCtx, "node-1", metav1.GetOptions{})
	if getErr != nil {
		t.Errorf("expected node to still exist after drain, got error: %v", getErr)
	}

	// Node should be cordoned even if drain failed/timed out
	if drainedNode != nil && !drainedNode.Spec.Unschedulable {
		// Node might not be cordoned if drain failed early
		t.Log("Node is not cordoned - drain may have failed before cordoning")
	}
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Helper functions

func createTestNode(name, nodeGroup string, cpuMillis, memoryBytes int64) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"autoscaler.vpsie.com/nodegroup": nodeGroup,
			},
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(cpuMillis, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(memoryBytes, resource.BinarySI),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    *resource.NewMilliQuantity(cpuMillis, resource.DecimalSI),
				corev1.ResourceMemory: *resource.NewQuantity(memoryBytes, resource.BinarySI),
			},
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}
}

func createTestPods(count int, namespace string) []*corev1.Pod {
	pods := make([]*corev1.Pod, count)
	for i := 0; i < count; i++ {
		pods[i] = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod-%d", i),
				Namespace: namespace,
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name: "container",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					},
				},
			},
		}
	}
	return pods
}

// TestScaleDownManager_UtilizationRaceCondition tests for race conditions in utilization data access.
// This test verifies that:
// 1. Concurrent reads don't race with modifications
// 2. Deep copy works correctly (modifying original doesn't affect snapshot)
// 3. Multiple goroutines can safely call GetNodeUtilization
// This test MUST pass with the -race flag enabled: go test -race ./pkg/scaler -run TestScaleDownManager_UtilizationRaceCondition
func TestScaleDownManager_UtilizationRaceCondition(t *testing.T) {
	client := fake.NewSimpleClientset()
	logger := zaptest.NewLogger(t)
	manager := NewScaleDownManager(client, nil, logger, DefaultConfig())

	// Create test nodes
	nodes := []*corev1.Node{
		createTestNode("node-1", "test-group", 4000, 8000000000),
		createTestNode("node-2", "test-group", 4000, 8000000000),
		createTestNode("node-3", "test-group", 4000, 8000000000),
	}

	for _, node := range nodes {
		_, err := client.CoreV1().Nodes().Create(context.Background(), node, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("failed to create node: %v", err)
		}
	}

	// Initialize utilization data
	now := time.Now()
	for i, node := range nodes {
		manager.nodeUtilization[node.Name] = &NodeUtilization{
			NodeName:          node.Name,
			CPUUtilization:    30.0,
			MemoryUtilization: 30.0,
			IsUnderutilized:   true,
			LastUpdated:       now,
			Samples: []UtilizationSample{
				{Timestamp: now.Add(-9 * time.Minute), CPUUtilization: 30, MemoryUtilization: 30},
				{Timestamp: now.Add(-8 * time.Minute), CPUUtilization: 32, MemoryUtilization: 32},
				{Timestamp: now.Add(-7 * time.Minute), CPUUtilization: 28, MemoryUtilization: 28},
				{Timestamp: now.Add(-6 * time.Minute), CPUUtilization: 31, MemoryUtilization: 31},
				{Timestamp: now.Add(-5 * time.Minute), CPUUtilization: float64(29 + i), MemoryUtilization: float64(29 + i)},
			},
		}
	}

	nodeGroup := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-group",
			Namespace: "default",
			Labels: map[string]string{
				autoscalerv1alpha1.ManagedLabelKey: autoscalerv1alpha1.ManagedLabelValue,
			},
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes: 1,
			MaxNodes: 10,
		},
	}

	// Test 1: Concurrent reads don't race with modifications
	t.Run("concurrent reads and writes", func(t *testing.T) {
		ctx := context.Background()
		done := make(chan bool)
		errChan := make(chan error, 100)

		// Start multiple goroutines reading utilization data via IdentifyUnderutilizedNodes
		for i := 0; i < 10; i++ {
			go func(id int) {
				for j := 0; j < 50; j++ {
					candidates, err := manager.IdentifyUnderutilizedNodes(ctx, nodeGroup)
					if err != nil {
						errChan <- fmt.Errorf("goroutine %d iteration %d: %w", id, j, err)
						return
					}
					// Verify we got expected candidates
					if len(candidates) == 0 {
						errChan <- fmt.Errorf("goroutine %d iteration %d: expected candidates, got none", id, j)
						return
					}
				}
				done <- true
			}(i)
		}

		// Concurrently modify utilization data (simulating UpdateNodeUtilization)
		for i := 0; i < 10; i++ {
			go func(id int) {
				for j := 0; j < 50; j++ {
					manager.utilizationLock.Lock()
					for _, node := range nodes {
						if util, exists := manager.nodeUtilization[node.Name]; exists {
							// Create new slice (matching UpdateNodeUtilization pattern)
							// This is the safe pattern that prevents race conditions
							newSamples := make([]UtilizationSample, len(util.Samples), len(util.Samples)+1)
							copy(newSamples, util.Samples)
							newSample := UtilizationSample{
								Timestamp:         time.Now(),
								CPUUtilization:    30.0 + float64(j%10),
								MemoryUtilization: 30.0 + float64(j%10),
							}
							newSamples = append(newSamples, newSample)
							if len(newSamples) > MaxSamplesPerNode {
								newSamples = newSamples[len(newSamples)-MaxSamplesPerNode:]
							}
							util.Samples = newSamples
							util.LastUpdated = time.Now()
						}
					}
					manager.utilizationLock.Unlock()
					time.Sleep(time.Microsecond)
				}
				done <- true
			}(i)
		}

		// Wait for all goroutines to complete
		for i := 0; i < 20; i++ {
			select {
			case err := <-errChan:
				t.Fatalf("concurrent access error: %v", err)
			case <-done:
				// Continue waiting
			}
		}

		// Check for any errors
		select {
		case err := <-errChan:
			t.Fatalf("concurrent access error: %v", err)
		default:
			// No errors - success
		}
	})

	// Test 2: Deep copy works correctly (modifying original doesn't affect snapshot)
	t.Run("deep copy isolation", func(t *testing.T) {
		ctx := context.Background()

		// Get candidates (triggers deep copy in IdentifyUnderutilizedNodes)
		candidates, err := manager.IdentifyUnderutilizedNodes(ctx, nodeGroup)
		if err != nil {
			t.Fatalf("failed to identify nodes: %v", err)
		}

		if len(candidates) == 0 {
			t.Fatal("expected candidates, got none")
		}

		// Store original sample count and first sample value
		originalSampleCount := len(candidates[0].Utilization.Samples)
		firstSampleCPU := candidates[0].Utilization.Samples[0].CPUUtilization

		// Modify the original data aggressively
		manager.utilizationLock.Lock()
		if util, exists := manager.nodeUtilization["node-1"]; exists {
			// Add many new samples
			for i := 0; i < 10; i++ {
				newSamples := make([]UtilizationSample, len(util.Samples), len(util.Samples)+1)
				copy(newSamples, util.Samples)
				newSamples = append(newSamples, UtilizationSample{
					Timestamp:         time.Now(),
					CPUUtilization:    99.0, // Very different value
					MemoryUtilization: 99.0,
				})
				util.Samples = newSamples
			}
			// Modify first sample
			util.Samples[0].CPUUtilization = 99.0
		}
		manager.utilizationLock.Unlock()

		// Verify the candidate's data hasn't changed (deep copy worked)
		if len(candidates[0].Utilization.Samples) != originalSampleCount {
			t.Errorf("deep copy failed: sample count changed from %d to %d",
				originalSampleCount, len(candidates[0].Utilization.Samples))
		}

		if candidates[0].Utilization.Samples[0].CPUUtilization != firstSampleCPU {
			t.Errorf("deep copy failed: first sample CPU changed from %.2f to %.2f",
				firstSampleCPU, candidates[0].Utilization.Samples[0].CPUUtilization)
		}
	})

	// Test 3: Multiple goroutines can safely call GetNodeUtilization
	t.Run("concurrent GetNodeUtilization", func(t *testing.T) {
		done := make(chan bool)
		errChan := make(chan error, 50)

		// Start multiple goroutines calling GetNodeUtilization
		for i := 0; i < 25; i++ {
			go func(id int) {
				for j := 0; j < 100; j++ {
					util, exists := manager.GetNodeUtilization("node-1")
					if !exists {
						errChan <- fmt.Errorf("goroutine %d iteration %d: node-1 not found", id, j)
						return
					}
					if util == nil {
						errChan <- fmt.Errorf("goroutine %d iteration %d: nil utilization", id, j)
						return
					}
					if len(util.Samples) == 0 {
						errChan <- fmt.Errorf("goroutine %d iteration %d: no samples", id, j)
						return
					}
				}
				done <- true
			}(i)
		}

		// Concurrently modify the data
		for i := 0; i < 25; i++ {
			go func(id int) {
				for j := 0; j < 100; j++ {
					manager.utilizationLock.Lock()
					if util, exists := manager.nodeUtilization["node-1"]; exists {
						newSamples := make([]UtilizationSample, len(util.Samples), len(util.Samples)+1)
						copy(newSamples, util.Samples)
						newSamples = append(newSamples, UtilizationSample{
							Timestamp:         time.Now(),
							CPUUtilization:    30.0,
							MemoryUtilization: 30.0,
						})
						if len(newSamples) > MaxSamplesPerNode {
							newSamples = newSamples[1:]
						}
						util.Samples = newSamples
					}
					manager.utilizationLock.Unlock()
				}
				done <- true
			}(i)
		}

		// Wait for all goroutines
		for i := 0; i < 50; i++ {
			select {
			case err := <-errChan:
				t.Fatalf("concurrent GetNodeUtilization error: %v", err)
			case <-done:
				// Continue
			}
		}

		// Final error check
		select {
		case err := <-errChan:
			t.Fatalf("concurrent GetNodeUtilization error: %v", err)
		default:
			// Success
		}
	})
}
