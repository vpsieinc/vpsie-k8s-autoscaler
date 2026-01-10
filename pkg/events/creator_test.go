package events

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

func TestNewDynamicNodeGroupCreator(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := zap.NewNop()

	t.Run("Creates with default template", func(t *testing.T) {
		creator := NewDynamicNodeGroupCreator(fakeClient, logger, nil)
		if creator == nil {
			t.Fatal("Expected creator to be created")
		}
		if creator.template == nil {
			t.Fatal("Expected default template to be set")
		}
		if creator.template.MinNodes != 1 {
			t.Errorf("Expected MinNodes=1, got %d", creator.template.MinNodes)
		}
		if creator.template.MaxNodes != 10 {
			t.Errorf("Expected MaxNodes=10, got %d", creator.template.MaxNodes)
		}
	})

	t.Run("Creates with custom template", func(t *testing.T) {
		template := &NodeGroupTemplate{
			MinNodes:            2,
			MaxNodes:            20,
			DefaultDatacenterID: "dc-1",
		}
		creator := NewDynamicNodeGroupCreator(fakeClient, logger, template)
		if creator.template.MinNodes != 2 {
			t.Errorf("Expected MinNodes=2, got %d", creator.template.MinNodes)
		}
		if creator.template.MaxNodes != 20 {
			t.Errorf("Expected MaxNodes=20, got %d", creator.template.MaxNodes)
		}
	})
}

func TestFindSuitableNodeGroup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := zap.NewNop()
	creator := NewDynamicNodeGroupCreator(fakeClient, logger, nil)
	ctx := context.Background()

	t.Run("Returns nil when no NodeGroups exist", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		}
		result := creator.FindSuitableNodeGroup(ctx, pod, []v1alpha1.NodeGroup{})
		if result != nil {
			t.Error("Expected nil when no NodeGroups exist")
		}
	})

	t.Run("Returns nil for unmanaged NodeGroups", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		}
		nodeGroups := []v1alpha1.NodeGroup{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "unmanaged-ng",
					// No managed label
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 1,
					MaxNodes: 10,
				},
			},
		}
		result := creator.FindSuitableNodeGroup(ctx, pod, nodeGroups)
		if result != nil {
			t.Error("Expected nil for unmanaged NodeGroup")
		}
	})

	t.Run("Returns managed NodeGroup with capacity", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		}
		nodeGroups := []v1alpha1.NodeGroup{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "managed-ng",
					Labels: map[string]string{
						v1alpha1.ManagedLabelKey: v1alpha1.ManagedLabelValue,
					},
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 1,
					MaxNodes: 10,
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 2,
				},
			},
		}
		result := creator.FindSuitableNodeGroup(ctx, pod, nodeGroups)
		if result == nil {
			t.Fatal("Expected to find suitable NodeGroup")
		}
		if result.Name != "managed-ng" {
			t.Errorf("Expected managed-ng, got %s", result.Name)
		}
	})

	t.Run("Skips NodeGroup at max capacity", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		}
		nodeGroups := []v1alpha1.NodeGroup{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "full-ng",
					Labels: map[string]string{
						v1alpha1.ManagedLabelKey: v1alpha1.ManagedLabelValue,
					},
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 1,
					MaxNodes: 5,
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 5, // At max
				},
			},
		}
		result := creator.FindSuitableNodeGroup(ctx, pod, nodeGroups)
		if result != nil {
			t.Error("Expected nil for NodeGroup at max capacity")
		}
	})

	t.Run("Matches pod with node selector", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Spec: corev1.PodSpec{
				NodeSelector: map[string]string{
					"gpu": "true",
				},
			},
		}
		nodeGroups := []v1alpha1.NodeGroup{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "generic-ng",
					Labels: map[string]string{
						v1alpha1.ManagedLabelKey: v1alpha1.ManagedLabelValue,
					},
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 1,
					MaxNodes: 10,
					// No labels - should not match pod with node selector
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gpu-ng",
					Labels: map[string]string{
						v1alpha1.ManagedLabelKey: v1alpha1.ManagedLabelValue,
					},
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 1,
					MaxNodes: 10,
					Labels: map[string]string{
						"gpu": "true",
					},
				},
			},
		}
		result := creator.FindSuitableNodeGroup(ctx, pod, nodeGroups)
		if result == nil {
			t.Fatal("Expected to find suitable NodeGroup")
		}
		if result.Name != "gpu-ng" {
			t.Errorf("Expected gpu-ng, got %s", result.Name)
		}
	})

	t.Run("Pod without selector only matches generic NodeGroup", func(t *testing.T) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			// No node selector
		}
		nodeGroups := []v1alpha1.NodeGroup{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "labeled-ng",
					Labels: map[string]string{
						v1alpha1.ManagedLabelKey: v1alpha1.ManagedLabelValue,
					},
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 1,
					MaxNodes: 10,
					Labels: map[string]string{
						"special": "true",
					},
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name: "generic-ng",
					Labels: map[string]string{
						v1alpha1.ManagedLabelKey: v1alpha1.ManagedLabelValue,
					},
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 1,
					MaxNodes: 10,
					// No labels
				},
			},
		}
		result := creator.FindSuitableNodeGroup(ctx, pod, nodeGroups)
		if result == nil {
			t.Fatal("Expected to find suitable NodeGroup")
		}
		if result.Name != "generic-ng" {
			t.Errorf("Expected generic-ng, got %s", result.Name)
		}
	})
}

func TestCreateNodeGroupForPod(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	logger := zap.NewNop()
	ctx := context.Background()

	t.Run("Creates NodeGroup with managed label", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		template := &NodeGroupTemplate{
			Namespace:           "test-ns",
			MinNodes:            1,
			MaxNodes:            5,
			DefaultDatacenterID: "dc-test",
			DefaultOfferingIDs:  []string{"offering-1"},
			ResourceIdentifier:  "test-cluster",
		}
		creator := NewDynamicNodeGroupCreator(fakeClient, logger, template)

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		}

		ng, err := creator.CreateNodeGroupForPod(ctx, pod, "test-ns")
		if err != nil {
			t.Fatalf("Failed to create NodeGroup: %v", err)
		}

		// Verify managed label
		if ng.Labels[v1alpha1.ManagedLabelKey] != v1alpha1.ManagedLabelValue {
			t.Error("Expected managed label to be set")
		}

		// Verify name format
		if !strings.HasPrefix(ng.Name, "auto-dc-test-") {
			t.Errorf("Expected name to start with 'auto-dc-test-', got %s", ng.Name)
		}

		// Verify spec
		if ng.Spec.MinNodes != 1 {
			t.Errorf("Expected MinNodes=1, got %d", ng.Spec.MinNodes)
		}
		if ng.Spec.MaxNodes != 5 {
			t.Errorf("Expected MaxNodes=5, got %d", ng.Spec.MaxNodes)
		}
	})

	t.Run("Copies pod node selector to NodeGroup labels", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		template := &NodeGroupTemplate{
			Namespace:           "default",
			MinNodes:            1,
			MaxNodes:            10,
			DefaultDatacenterID: "dc-default",
			DefaultOfferingIDs:  []string{"offering-1"},
			ResourceIdentifier:  "test-cluster",
		}
		creator := NewDynamicNodeGroupCreator(fakeClient, logger, template)

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Spec: corev1.PodSpec{
				NodeSelector: map[string]string{
					"gpu":  "nvidia",
					"tier": "high",
				},
			},
		}

		ng, err := creator.CreateNodeGroupForPod(ctx, pod, "default")
		if err != nil {
			t.Fatalf("Failed to create NodeGroup: %v", err)
		}

		// Verify labels copied
		if ng.Spec.Labels["gpu"] != "nvidia" {
			t.Errorf("Expected gpu=nvidia label, got %v", ng.Spec.Labels)
		}
		if ng.Spec.Labels["tier"] != "high" {
			t.Errorf("Expected tier=high label, got %v", ng.Spec.Labels)
		}
	})

	t.Run("Extracts taints from pod tolerations", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		template := &NodeGroupTemplate{
			Namespace:           "default",
			MinNodes:            1,
			MaxNodes:            10,
			DefaultDatacenterID: "dc-default",
			DefaultOfferingIDs:  []string{"offering-1"},
			ResourceIdentifier:  "test-cluster",
		}
		creator := NewDynamicNodeGroupCreator(fakeClient, logger, template)

		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
			Spec: corev1.PodSpec{
				Tolerations: []corev1.Toleration{
					{
						Key:      "dedicated",
						Operator: corev1.TolerationOpEqual,
						Value:    "ml-workload",
						Effect:   corev1.TaintEffectNoSchedule,
					},
					// System tolerations should be skipped
					{
						Key:      "node.kubernetes.io/not-ready",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					},
				},
			},
		}

		ng, err := creator.CreateNodeGroupForPod(ctx, pod, "default")
		if err != nil {
			t.Fatalf("Failed to create NodeGroup: %v", err)
		}

		// Should have the custom taint
		if len(ng.Spec.Taints) != 1 {
			t.Errorf("Expected 1 taint, got %d", len(ng.Spec.Taints))
		}
		if len(ng.Spec.Taints) > 0 {
			if ng.Spec.Taints[0].Key != "dedicated" {
				t.Errorf("Expected taint key 'dedicated', got %s", ng.Spec.Taints[0].Key)
			}
			if ng.Spec.Taints[0].Value != "ml-workload" {
				t.Errorf("Expected taint value 'ml-workload', got %s", ng.Spec.Taints[0].Value)
			}
		}
	})
}

func TestGenerateNodeGroupName(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := zap.NewNop()

	t.Run("Generates unique names with datacenter", func(t *testing.T) {
		template := &NodeGroupTemplate{
			DefaultDatacenterID: "us-east-1",
		}
		creator := NewDynamicNodeGroupCreator(fakeClient, logger, template)

		name := creator.generateNodeGroupName()
		if !strings.HasPrefix(name, "auto-us-east-1-") {
			t.Errorf("Expected name to start with 'auto-us-east-1-', got %s", name)
		}
	})

	t.Run("Uses default when no datacenter", func(t *testing.T) {
		template := &NodeGroupTemplate{
			DefaultDatacenterID: "",
		}
		creator := NewDynamicNodeGroupCreator(fakeClient, logger, template)

		name := creator.generateNodeGroupName()
		if !strings.HasPrefix(name, "auto-default-") {
			t.Errorf("Expected name to start with 'auto-default-', got %s", name)
		}
	})
}

func TestIsSystemToleration(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"node.kubernetes.io/not-ready", true},
		{"node.kubernetes.io/unreachable", true},
		{"node.kubernetes.io/memory-pressure", true},
		{"node.kubernetes.io/disk-pressure", true},
		{"custom-taint", false},
		{"gpu", false},
		{"dedicated", false},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			result := isSystemToleration(tc.key)
			if result != tc.expected {
				t.Errorf("isSystemToleration(%s) = %v, expected %v", tc.key, result, tc.expected)
			}
		})
	}
}

func TestValidateTemplate(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	logger := zap.NewNop()

	t.Run("Valid template passes validation", func(t *testing.T) {
		template := &NodeGroupTemplate{
			DefaultDatacenterID: "dc-1",
			DefaultOfferingIDs:  []string{"offering-1"},
			ResourceIdentifier:  "test-cluster",
		}
		creator := NewDynamicNodeGroupCreator(fakeClient, logger, template)

		err := creator.ValidateTemplate()
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})

	t.Run("Missing DatacenterID fails validation", func(t *testing.T) {
		template := &NodeGroupTemplate{
			DefaultOfferingIDs: []string{"offering-1"},
			ResourceIdentifier: "test-cluster",
		}
		creator := NewDynamicNodeGroupCreator(fakeClient, logger, template)

		err := creator.ValidateTemplate()
		if err == nil {
			t.Error("Expected error for missing DatacenterID")
		}
	})

	t.Run("Missing OfferingIDs fails validation", func(t *testing.T) {
		template := &NodeGroupTemplate{
			DefaultDatacenterID: "dc-1",
			ResourceIdentifier:  "test-cluster",
		}
		creator := NewDynamicNodeGroupCreator(fakeClient, logger, template)

		err := creator.ValidateTemplate()
		if err == nil {
			t.Error("Expected error for missing OfferingIDs")
		}
	})

	t.Run("Missing ResourceIdentifier fails validation", func(t *testing.T) {
		template := &NodeGroupTemplate{
			DefaultDatacenterID: "dc-1",
			DefaultOfferingIDs:  []string{"offering-1"},
		}
		creator := NewDynamicNodeGroupCreator(fakeClient, logger, template)

		err := creator.ValidateTemplate()
		if err == nil {
			t.Error("Expected error for missing ResourceIdentifier")
		}
	})
}
