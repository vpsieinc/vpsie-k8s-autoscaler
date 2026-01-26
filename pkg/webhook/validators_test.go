package webhook

import (
	"testing"

	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// =============================================================================
// NodeGroupValidator Tests
// =============================================================================

func TestNewNodeGroupValidator(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := zap.NewNop()
		v := NewNodeGroupValidator(logger)
		if v == nil {
			t.Fatal("expected validator to be created")
		}
	})

	t.Run("with nil logger", func(t *testing.T) {
		v := NewNodeGroupValidator(nil)
		if v == nil {
			t.Fatal("expected validator to be created with nop logger")
		}
	})
}

func TestNodeGroupValidator_ValidateNamespace(t *testing.T) {
	v := NewNodeGroupValidator(zap.NewNop())

	tests := []struct {
		name      string
		namespace string
		wantErr   bool
	}{
		{
			name:      "valid kube-system namespace",
			namespace: "kube-system",
			wantErr:   false,
		},
		{
			name:      "invalid default namespace",
			namespace: "default",
			wantErr:   true,
		},
		{
			name:      "invalid custom namespace",
			namespace: "my-namespace",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodegroup",
					Namespace: tt.namespace,
				},
				Spec: validNodeGroupSpec(),
			}
			err := v.Validate(ng, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNamespace() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeGroupValidator_ValidateNodeCount(t *testing.T) {
	v := NewNodeGroupValidator(zap.NewNop())

	tests := []struct {
		name     string
		minNodes int32
		maxNodes int32
		wantErr  bool
	}{
		{
			name:     "valid min/max",
			minNodes: 1,
			maxNodes: 5,
			wantErr:  false,
		},
		{
			name:     "valid min equals max",
			minNodes: 3,
			maxNodes: 3,
			wantErr:  false,
		},
		{
			name:     "valid zero min",
			minNodes: 0,
			maxNodes: 10,
			wantErr:  false,
		},
		{
			name:     "invalid min greater than max",
			minNodes: 10,
			maxNodes: 5,
			wantErr:  true,
		},
		{
			name:     "invalid negative min",
			minNodes: -1,
			maxNodes: 5,
			wantErr:  true,
		},
		{
			name:     "invalid negative max",
			minNodes: 0,
			maxNodes: -1,
			wantErr:  true,
		},
		{
			name:     "invalid max exceeds limit",
			minNodes: 0,
			maxNodes: 1001,
			wantErr:  true,
		},
		{
			name:     "valid max at limit",
			minNodes: 0,
			maxNodes: 1000,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodegroup",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					MinNodes:          tt.minNodes,
					MaxNodes:          tt.maxNodes,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					KubernetesVersion: "v1.28.0",
				},
			}
			err := v.Validate(ng, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNodeCount() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeGroupValidator_ValidateDatacenter(t *testing.T) {
	v := NewNodeGroupValidator(zap.NewNop())

	tests := []struct {
		name         string
		datacenterID string
		wantErr      bool
	}{
		{
			name:         "valid datacenter",
			datacenterID: "us-east-1",
			wantErr:      false,
		},
		{
			name:         "valid datacenter with underscore",
			datacenterID: "us_east_1",
			wantErr:      false,
		},
		{
			name:         "valid datacenter alphanumeric",
			datacenterID: "dc123",
			wantErr:      false,
		},
		{
			name:         "empty datacenter",
			datacenterID: "",
			wantErr:      true,
		},
		{
			name:         "invalid datacenter with special chars",
			datacenterID: "us-east@1",
			wantErr:      true,
		},
		{
			name:         "invalid datacenter with spaces",
			datacenterID: "us east 1",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodegroup",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					MinNodes:          1,
					MaxNodes:          5,
					DatacenterID:      tt.datacenterID,
					OfferingIDs:       []string{"offering-1"},
					KubernetesVersion: "v1.28.0",
				},
			}
			err := v.Validate(ng, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDatacenter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeGroupValidator_ValidateOfferingIDs(t *testing.T) {
	v := NewNodeGroupValidator(zap.NewNop())

	tests := []struct {
		name        string
		offeringIDs []string
		wantErr     bool
	}{
		{
			name:        "valid single offering",
			offeringIDs: []string{"offering-1"},
			wantErr:     false,
		},
		{
			name:        "valid multiple offerings",
			offeringIDs: []string{"offering-1", "offering-2", "offering-3"},
			wantErr:     false,
		},
		{
			name:        "empty offerings list",
			offeringIDs: []string{},
			wantErr:     true,
		},
		{
			name:        "nil offerings list",
			offeringIDs: nil,
			wantErr:     true,
		},
		{
			name:        "offering with empty string",
			offeringIDs: []string{"offering-1", "", "offering-3"},
			wantErr:     true,
		},
		{
			name:        "duplicate offerings",
			offeringIDs: []string{"offering-1", "offering-2", "offering-1"},
			wantErr:     true,
		},
		{
			name:        "invalid offering format",
			offeringIDs: []string{"offering@invalid"},
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodegroup",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					MinNodes:          1,
					MaxNodes:          5,
					DatacenterID:      "dc-1",
					OfferingIDs:       tt.offeringIDs,
					KubernetesVersion: "v1.28.0",
				},
			}
			err := v.Validate(ng, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOfferingIDs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeGroupValidator_ValidateKubernetesVersion(t *testing.T) {
	v := NewNodeGroupValidator(zap.NewNop())

	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{
			name:    "valid version",
			version: "v1.28.0",
			wantErr: false,
		},
		{
			name:    "valid version with patch",
			version: "v1.29.1",
			wantErr: false,
		},
		{
			name:    "valid version with prerelease",
			version: "v1.29.0-rc.0",
			wantErr: false,
		},
		{
			name:    "valid version with build metadata",
			version: "v1.29.0+build.123",
			wantErr: false,
		},
		{
			name:    "empty version",
			version: "",
			wantErr: true,
		},
		{
			name:    "invalid version without v prefix",
			version: "1.28.0",
			wantErr: true,
		},
		{
			name:    "invalid version format",
			version: "v1.28",
			wantErr: true,
		},
		{
			name:    "invalid version random string",
			version: "latest",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodegroup",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					MinNodes:          1,
					MaxNodes:          5,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					KubernetesVersion: tt.version,
				},
			}
			err := v.Validate(ng, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateKubernetesVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeGroupValidator_ValidateOSImage(t *testing.T) {
	v := NewNodeGroupValidator(zap.NewNop())

	tests := []struct {
		name      string
		osImageID string
		wantErr   bool
	}{
		{
			name:      "valid os image",
			osImageID: "ubuntu-22.04",
			wantErr:   false,
		},
		{
			name:      "valid os image with underscore",
			osImageID: "ubuntu_22_04",
			wantErr:   false,
		},
		{
			name:      "empty os image (optional)",
			osImageID: "",
			wantErr:   false,
		},
		{
			name:      "invalid os image with special chars",
			osImageID: "ubuntu@22.04",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodegroup",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					MinNodes:          1,
					MaxNodes:          5,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					KubernetesVersion: "v1.28.0",
					OSImageID:         tt.osImageID,
				},
			}
			err := v.Validate(ng, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOSImage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeGroupValidator_ValidateScaleUpPolicy(t *testing.T) {
	v := NewNodeGroupValidator(zap.NewNop())

	tests := []struct {
		name    string
		policy  autoscalerv1alpha1.ScaleUpPolicy
		wantErr bool
	}{
		{
			name: "valid policy",
			policy: autoscalerv1alpha1.ScaleUpPolicy{
				Enabled:                    true,
				CPUThreshold:               80,
				MemoryThreshold:            80,
				StabilizationWindowSeconds: 60,
			},
			wantErr: false,
		},
		{
			name: "valid policy with zero thresholds",
			policy: autoscalerv1alpha1.ScaleUpPolicy{
				Enabled:         true,
				CPUThreshold:    0,
				MemoryThreshold: 0,
			},
			wantErr: false,
		},
		{
			name: "valid policy with max thresholds",
			policy: autoscalerv1alpha1.ScaleUpPolicy{
				Enabled:         true,
				CPUThreshold:    100,
				MemoryThreshold: 100,
			},
			wantErr: false,
		},
		{
			name: "invalid negative stabilization window",
			policy: autoscalerv1alpha1.ScaleUpPolicy{
				Enabled:                    true,
				StabilizationWindowSeconds: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid stabilization window exceeds max",
			policy: autoscalerv1alpha1.ScaleUpPolicy{
				Enabled:                    true,
				StabilizationWindowSeconds: 1801,
			},
			wantErr: true,
		},
		{
			name: "invalid CPU threshold exceeds 100",
			policy: autoscalerv1alpha1.ScaleUpPolicy{
				Enabled:      true,
				CPUThreshold: 101,
			},
			wantErr: true,
		},
		{
			name: "invalid memory threshold exceeds 100",
			policy: autoscalerv1alpha1.ScaleUpPolicy{
				Enabled:         true,
				MemoryThreshold: 101,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodegroup",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					MinNodes:          1,
					MaxNodes:          5,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					KubernetesVersion: "v1.28.0",
					ScaleUpPolicy:     tt.policy,
				},
			}
			err := v.Validate(ng, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateScaleUpPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeGroupValidator_ValidateScaleDownPolicy(t *testing.T) {
	v := NewNodeGroupValidator(zap.NewNop())

	tests := []struct {
		name    string
		policy  autoscalerv1alpha1.ScaleDownPolicy
		wantErr bool
	}{
		{
			name: "valid policy",
			policy: autoscalerv1alpha1.ScaleDownPolicy{
				Enabled:                    true,
				CPUThreshold:               30,
				MemoryThreshold:            30,
				CooldownSeconds:            300,
				StabilizationWindowSeconds: 600,
			},
			wantErr: false,
		},
		{
			name: "invalid negative cooldown",
			policy: autoscalerv1alpha1.ScaleDownPolicy{
				Enabled:         true,
				CooldownSeconds: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid cooldown exceeds max",
			policy: autoscalerv1alpha1.ScaleDownPolicy{
				Enabled:         true,
				CooldownSeconds: 3601,
			},
			wantErr: true,
		},
		{
			name: "invalid stabilization window exceeds max",
			policy: autoscalerv1alpha1.ScaleDownPolicy{
				Enabled:                    true,
				StabilizationWindowSeconds: 3601,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodegroup",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					MinNodes:          1,
					MaxNodes:          5,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					KubernetesVersion: "v1.28.0",
					ScaleDownPolicy:   tt.policy,
				},
			}
			err := v.Validate(ng, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateScaleDownPolicy() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeGroupValidator_ValidateLabels(t *testing.T) {
	v := NewNodeGroupValidator(zap.NewNop())

	tests := []struct {
		name    string
		labels  map[string]string
		wantErr bool
	}{
		{
			name:    "nil labels",
			labels:  nil,
			wantErr: false,
		},
		{
			name:    "empty labels",
			labels:  map[string]string{},
			wantErr: false,
		},
		{
			name: "valid simple labels",
			labels: map[string]string{
				"app":         "myapp",
				"environment": "production",
			},
			wantErr: false,
		},
		{
			name: "valid label with prefix",
			labels: map[string]string{
				"example.com/my-label": "value",
			},
			wantErr: false,
		},
		{
			name: "valid empty value",
			labels: map[string]string{
				"app": "",
			},
			wantErr: false,
		},
		{
			name: "invalid kubernetes.io prefix",
			labels: map[string]string{
				"kubernetes.io/hostname": "node1",
			},
			wantErr: true,
		},
		{
			name: "invalid k8s.io prefix",
			labels: map[string]string{
				"k8s.io/arch": "amd64",
			},
			wantErr: true,
		},
		{
			name: "invalid value exceeds length",
			labels: map[string]string{
				"app": "this-is-a-very-long-label-value-that-exceeds-the-maximum-length-of-63-characters",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodegroup",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					MinNodes:          1,
					MaxNodes:          5,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					KubernetesVersion: "v1.28.0",
					Labels:            tt.labels,
				},
			}
			err := v.Validate(ng, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLabels() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeGroupValidator_ValidateTaints(t *testing.T) {
	v := NewNodeGroupValidator(zap.NewNop())

	tests := []struct {
		name    string
		taints  []corev1.Taint
		wantErr bool
	}{
		{
			name:    "nil taints",
			taints:  nil,
			wantErr: false,
		},
		{
			name:    "empty taints",
			taints:  []corev1.Taint{},
			wantErr: false,
		},
		{
			name: "valid NoSchedule taint",
			taints: []corev1.Taint{
				{
					Key:    "dedicated",
					Value:  "gpu",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
			wantErr: false,
		},
		{
			name: "valid PreferNoSchedule taint",
			taints: []corev1.Taint{
				{
					Key:    "special",
					Value:  "true",
					Effect: corev1.TaintEffectPreferNoSchedule,
				},
			},
			wantErr: false,
		},
		{
			name: "valid NoExecute taint",
			taints: []corev1.Taint{
				{
					Key:    "maintenance",
					Value:  "true",
					Effect: corev1.TaintEffectNoExecute,
				},
			},
			wantErr: false,
		},
		{
			name: "empty taint key",
			taints: []corev1.Taint{
				{
					Key:    "",
					Value:  "value",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid taint effect",
			taints: []corev1.Taint{
				{
					Key:    "key",
					Value:  "value",
					Effect: "InvalidEffect",
				},
			},
			wantErr: true,
		},
		{
			name: "reserved kubernetes.io prefix",
			taints: []corev1.Taint{
				{
					Key:    "kubernetes.io/taint",
					Value:  "value",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &autoscalerv1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-nodegroup",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.NodeGroupSpec{
					MinNodes:          1,
					MaxNodes:          5,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					KubernetesVersion: "v1.28.0",
					Taints:            tt.taints,
				},
			}
			err := v.Validate(ng, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTaints() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNodeGroupValidator_Operations(t *testing.T) {
	v := NewNodeGroupValidator(zap.NewNop())
	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup",
			Namespace: "kube-system",
		},
		Spec: validNodeGroupSpec(),
	}

	tests := []struct {
		name      string
		operation admissionv1.Operation
		wantErr   bool
	}{
		{
			name:      "create operation",
			operation: admissionv1.Create,
			wantErr:   false,
		},
		{
			name:      "update operation",
			operation: admissionv1.Update,
			wantErr:   false,
		},
		{
			name:      "delete operation",
			operation: admissionv1.Delete,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(ng, tt.operation)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// VPSieNodeValidator Tests
// =============================================================================

func TestNewVPSieNodeValidator(t *testing.T) {
	t.Run("with logger", func(t *testing.T) {
		logger := zap.NewNop()
		v := NewVPSieNodeValidator(logger)
		if v == nil {
			t.Fatal("expected validator to be created")
		}
	})

	t.Run("with nil logger", func(t *testing.T) {
		v := NewVPSieNodeValidator(nil)
		if v == nil {
			t.Fatal("expected validator to be created with nop logger")
		}
	})
}

func TestVPSieNodeValidator_ValidateNamespace(t *testing.T) {
	v := NewVPSieNodeValidator(zap.NewNop())

	tests := []struct {
		name      string
		namespace string
		wantErr   bool
	}{
		{
			name:      "valid kube-system namespace",
			namespace: "kube-system",
			wantErr:   false,
		},
		{
			name:      "invalid default namespace",
			namespace: "default",
			wantErr:   true,
		},
		{
			name:      "invalid custom namespace",
			namespace: "my-namespace",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vn := &autoscalerv1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vpsienode",
					Namespace: tt.namespace,
				},
				Spec: validVPSieNodeSpec(),
			}
			err := v.Validate(vn, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNamespace() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVPSieNodeValidator_ValidateNodeGroupRef(t *testing.T) {
	v := NewVPSieNodeValidator(zap.NewNop())

	tests := []struct {
		name          string
		nodeGroupName string
		wantErr       bool
	}{
		{
			name:          "valid nodegroup name",
			nodeGroupName: "my-nodegroup",
			wantErr:       false,
		},
		{
			name:          "valid nodegroup name with dots",
			nodeGroupName: "my.nodegroup.test",
			wantErr:       false,
		},
		{
			name:          "empty nodegroup name",
			nodeGroupName: "",
			wantErr:       true,
		},
		{
			name:          "invalid nodegroup name with uppercase",
			nodeGroupName: "MyNodeGroup",
			wantErr:       true,
		},
		{
			name:          "invalid nodegroup name starting with hyphen",
			nodeGroupName: "-nodegroup",
			wantErr:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vn := &autoscalerv1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vpsienode",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.VPSieNodeSpec{
					NodeGroupName:     tt.nodeGroupName,
					DatacenterID:      "dc-1",
					InstanceType:      "standard-2",
					KubernetesVersion: "v1.28.0",
				},
			}
			err := v.Validate(vn, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateNodeGroupRef() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVPSieNodeValidator_ValidateDatacenter(t *testing.T) {
	v := NewVPSieNodeValidator(zap.NewNop())

	tests := []struct {
		name         string
		datacenterID string
		wantErr      bool
	}{
		{
			name:         "valid datacenter",
			datacenterID: "us-east-1",
			wantErr:      false,
		},
		{
			name:         "empty datacenter",
			datacenterID: "",
			wantErr:      true,
		},
		{
			name:         "invalid datacenter with special chars",
			datacenterID: "us-east@1",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vn := &autoscalerv1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vpsienode",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.VPSieNodeSpec{
					NodeGroupName:     "my-nodegroup",
					DatacenterID:      tt.datacenterID,
					InstanceType:      "standard-2",
					KubernetesVersion: "v1.28.0",
				},
			}
			err := v.Validate(vn, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDatacenter() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVPSieNodeValidator_ValidateOfferingID(t *testing.T) {
	v := NewVPSieNodeValidator(zap.NewNop())

	tests := []struct {
		name         string
		instanceType string
		wantErr      bool
	}{
		{
			name:         "valid instance type",
			instanceType: "standard-2",
			wantErr:      false,
		},
		{
			name:         "empty instance type",
			instanceType: "",
			wantErr:      true,
		},
		{
			name:         "invalid instance type with special chars",
			instanceType: "standard@2",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vn := &autoscalerv1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vpsienode",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.VPSieNodeSpec{
					NodeGroupName:     "my-nodegroup",
					DatacenterID:      "dc-1",
					InstanceType:      tt.instanceType,
					KubernetesVersion: "v1.28.0",
				},
			}
			err := v.Validate(vn, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOfferingID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVPSieNodeValidator_ValidateKubernetesVersion(t *testing.T) {
	v := NewVPSieNodeValidator(zap.NewNop())

	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{
			name:    "valid version",
			version: "v1.28.0",
			wantErr: false,
		},
		{
			name:    "empty version",
			version: "",
			wantErr: true,
		},
		{
			name:    "invalid version without v prefix",
			version: "1.28.0",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vn := &autoscalerv1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vpsienode",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.VPSieNodeSpec{
					NodeGroupName:     "my-nodegroup",
					DatacenterID:      "dc-1",
					InstanceType:      "standard-2",
					KubernetesVersion: tt.version,
				},
			}
			err := v.Validate(vn, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateKubernetesVersion() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVPSieNodeValidator_ValidateOSImage(t *testing.T) {
	v := NewVPSieNodeValidator(zap.NewNop())

	tests := []struct {
		name      string
		osImageID string
		wantErr   bool
	}{
		{
			name:      "valid os image",
			osImageID: "ubuntu-22.04",
			wantErr:   false,
		},
		{
			name:      "empty os image (optional)",
			osImageID: "",
			wantErr:   false,
		},
		{
			name:      "invalid os image with special chars",
			osImageID: "ubuntu@22.04",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vn := &autoscalerv1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vpsienode",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.VPSieNodeSpec{
					NodeGroupName:     "my-nodegroup",
					DatacenterID:      "dc-1",
					InstanceType:      "standard-2",
					KubernetesVersion: "v1.28.0",
					OSImageID:         tt.osImageID,
				},
			}
			err := v.Validate(vn, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOSImage() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVPSieNodeValidator_ValidateSSHKeyIDs(t *testing.T) {
	v := NewVPSieNodeValidator(zap.NewNop())

	tests := []struct {
		name      string
		sshKeyIDs []string
		wantErr   bool
	}{
		{
			name:      "nil ssh keys (optional)",
			sshKeyIDs: nil,
			wantErr:   false,
		},
		{
			name:      "empty ssh keys (optional)",
			sshKeyIDs: []string{},
			wantErr:   false,
		},
		{
			name:      "valid single ssh key",
			sshKeyIDs: []string{"key-123"},
			wantErr:   false,
		},
		{
			name:      "valid multiple ssh keys",
			sshKeyIDs: []string{"key-1", "key-2", "key-3"},
			wantErr:   false,
		},
		{
			name:      "empty string in ssh keys",
			sshKeyIDs: []string{"key-1", "", "key-3"},
			wantErr:   true,
		},
		{
			name:      "duplicate ssh keys",
			sshKeyIDs: []string{"key-1", "key-2", "key-1"},
			wantErr:   true,
		},
		{
			name:      "invalid ssh key format",
			sshKeyIDs: []string{"key@invalid"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vn := &autoscalerv1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vpsienode",
					Namespace: "kube-system",
				},
				Spec: autoscalerv1alpha1.VPSieNodeSpec{
					NodeGroupName:     "my-nodegroup",
					DatacenterID:      "dc-1",
					InstanceType:      "standard-2",
					KubernetesVersion: "v1.28.0",
					SSHKeyIDs:         tt.sshKeyIDs,
				},
			}
			err := v.Validate(vn, admissionv1.Create)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSSHKeyIDs() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVPSieNodeValidator_Operations(t *testing.T) {
	v := NewVPSieNodeValidator(zap.NewNop())
	vn := &autoscalerv1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vpsienode",
			Namespace: "kube-system",
		},
		Spec: validVPSieNodeSpec(),
	}

	tests := []struct {
		name      string
		operation admissionv1.Operation
		wantErr   bool
	}{
		{
			name:      "create operation",
			operation: admissionv1.Create,
			wantErr:   false,
		},
		{
			name:      "update operation",
			operation: admissionv1.Update,
			wantErr:   false,
		},
		{
			name:      "delete operation",
			operation: admissionv1.Delete,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(vn, tt.operation)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// =============================================================================
// Helper Tests
// =============================================================================

func TestHasReservedPrefix(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		expected bool
	}{
		{
			name:     "kubernetes.io prefix",
			key:      "kubernetes.io/hostname",
			expected: true,
		},
		{
			name:     "k8s.io prefix",
			key:      "k8s.io/arch",
			expected: true,
		},
		{
			name:     "custom prefix",
			key:      "example.com/my-label",
			expected: false,
		},
		{
			name:     "no prefix",
			key:      "app",
			expected: false,
		},
		{
			name:     "kubernetes in key but not prefix",
			key:      "my-kubernetes-label",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasReservedPrefix(tt.key)
			if result != tt.expected {
				t.Errorf("hasReservedPrefix(%q) = %v, expected %v", tt.key, result, tt.expected)
			}
		})
	}
}

// =============================================================================
// Test Helpers
// =============================================================================

func validNodeGroupSpec() autoscalerv1alpha1.NodeGroupSpec {
	return autoscalerv1alpha1.NodeGroupSpec{
		MinNodes:          1,
		MaxNodes:          5,
		DatacenterID:      "dc-1",
		OfferingIDs:       []string{"offering-1"},
		KubernetesVersion: "v1.28.0",
	}
}

func validVPSieNodeSpec() autoscalerv1alpha1.VPSieNodeSpec {
	return autoscalerv1alpha1.VPSieNodeSpec{
		NodeGroupName:     "my-nodegroup",
		DatacenterID:      "dc-1",
		InstanceType:      "standard-2",
		KubernetesVersion: "v1.28.0",
	}
}
