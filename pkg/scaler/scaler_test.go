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
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	metricsfake "k8s.io/metrics/pkg/client/clientset/versioned/fake"
)

func TestNewScaleDownManager(t *testing.T) {
	client := fake.NewSimpleClientset()
	metricsClient := metricsfake.NewSimpleClientset()
	logger := zaptest.NewLogger(t)

	manager := NewScaleDownManager(client, metricsClient, logger, nil)

	if manager == nil {
		t.Fatal("expected manager to be created")
	}

	if manager.config == nil {
		t.Fatal("expected default config to be set")
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
				NodeName: "node-1",
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
				NodeName: "node-2",
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
