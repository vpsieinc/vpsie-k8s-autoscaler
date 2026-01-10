package nodegroup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

func TestValidateNodeGroupSpec(t *testing.T) {
	tests := []struct {
		name    string
		ng      *v1alpha1.NodeGroup
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid spec",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes:          2,
					MaxNodes:          10,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					OSImageID:         "ubuntu-22.04",
					KubernetesVersion: "v1.28.0",
				},
			},
			wantErr: false,
		},
		{
			name: "negative minNodes",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes:          -1,
					MaxNodes:          10,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					OSImageID:         "ubuntu-22.04",
					KubernetesVersion: "v1.28.0",
				},
			},
			wantErr: true,
			errMsg:  "minNodes must be >= 0",
		},
		{
			name: "zero maxNodes",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes:          0,
					MaxNodes:          0,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					OSImageID:         "ubuntu-22.04",
					KubernetesVersion: "v1.28.0",
				},
			},
			wantErr: true,
			errMsg:  "maxNodes must be >= 1",
		},
		{
			name: "minNodes > maxNodes",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes:          10,
					MaxNodes:          5,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					OSImageID:         "ubuntu-22.04",
					KubernetesVersion: "v1.28.0",
				},
			},
			wantErr: true,
			errMsg:  "minNodes (10) must be <= maxNodes (5)",
		},
		{
			name: "empty datacenterID",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes:          2,
					MaxNodes:          10,
					DatacenterID:      "",
					OfferingIDs:       []string{"offering-1"},
					OSImageID:         "ubuntu-22.04",
					KubernetesVersion: "v1.28.0",
				},
			},
			wantErr: true,
			errMsg:  "datacenterID is required",
		},
		{
			name: "empty offeringIDs",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes:          2,
					MaxNodes:          10,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{},
					OSImageID:         "ubuntu-22.04",
					KubernetesVersion: "v1.28.0",
				},
			},
			wantErr: true,
			errMsg:  "at least one offeringID is required",
		},
		{
			name: "empty osImageID",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes:          2,
					MaxNodes:          10,
					DatacenterID:      "dc-1",
					OfferingIDs:       []string{"offering-1"},
					OSImageID:         "",
					KubernetesVersion: "v1.28.0",
				},
			},
			wantErr: true,
			errMsg:  "osImageID is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNodeGroupSpec(tt.ng)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCalculateDesiredNodes(t *testing.T) {
	tests := []struct {
		name     string
		ng       *v1alpha1.NodeGroup
		expected int32
	}{
		{
			name: "unset desired defaults to min",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 2,
					MaxNodes: 10,
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 0,
				},
			},
			expected: 2,
		},
		{
			name: "desired within bounds",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 2,
					MaxNodes: 10,
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 5,
				},
			},
			expected: 5,
		},
		{
			name: "desired below min gets clamped",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 2,
					MaxNodes: 10,
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 1,
				},
			},
			expected: 2,
		},
		{
			name: "desired above max gets clamped",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 2,
					MaxNodes: 10,
				},
				Status: v1alpha1.NodeGroupStatus{
					DesiredNodes: 15,
				},
			},
			expected: 10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateDesiredNodes(tt.ng)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNeedsScaleUp(t *testing.T) {
	tests := []struct {
		name     string
		ng       *v1alpha1.NodeGroup
		expected bool
	}{
		{
			name: "current < desired and current < max",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MaxNodes: 10,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 3,
					DesiredNodes: 5,
				},
			},
			expected: true,
		},
		{
			name: "current == desired",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MaxNodes: 10,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 5,
					DesiredNodes: 5,
				},
			},
			expected: false,
		},
		{
			name: "current at max",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MaxNodes: 5,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 5,
					DesiredNodes: 10,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NeedsScaleUp(tt.ng)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNeedsScaleDown(t *testing.T) {
	tests := []struct {
		name     string
		ng       *v1alpha1.NodeGroup
		expected bool
	}{
		{
			name: "current > desired and current > min",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 2,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 5,
					DesiredNodes: 3,
				},
			},
			expected: true,
		},
		{
			name: "current == desired",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 2,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 5,
					DesiredNodes: 5,
				},
			},
			expected: false,
		},
		{
			name: "current at min",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 5,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 5,
					DesiredNodes: 2,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NeedsScaleDown(tt.ng)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateNodesToAdd(t *testing.T) {
	tests := []struct {
		name     string
		ng       *v1alpha1.NodeGroup
		expected int32
	}{
		{
			name: "can add all needed nodes",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MaxNodes: 10,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 3,
					DesiredNodes: 5,
				},
			},
			expected: 2,
		},
		{
			name: "limited by max capacity",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MaxNodes: 7,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 5,
					DesiredNodes: 10,
				},
			},
			expected: 2,
		},
		{
			name: "no nodes needed",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MaxNodes: 10,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 5,
					DesiredNodes: 5,
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateNodesToAdd(tt.ng)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCalculateNodesToRemove(t *testing.T) {
	tests := []struct {
		name     string
		ng       *v1alpha1.NodeGroup
		expected int32
	}{
		{
			name: "can remove all excess nodes",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 2,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 7,
					DesiredNodes: 5,
				},
			},
			expected: 2,
		},
		{
			name: "limited by min capacity",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 5,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 7,
					DesiredNodes: 2,
				},
			},
			expected: 2,
		},
		{
			name: "no nodes to remove",
			ng: &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 2,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 5,
					DesiredNodes: 5,
				},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateNodesToRemove(tt.ng)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsScaling(t *testing.T) {
	tests := []struct {
		name     string
		ng       *v1alpha1.NodeGroup
		expected bool
	}{
		{
			name: "is scaling",
			ng: &v1alpha1.NodeGroup{
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 3,
					DesiredNodes: 5,
				},
			},
			expected: true,
		},
		{
			name: "not scaling",
			ng: &v1alpha1.NodeGroup{
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 5,
					DesiredNodes: 5,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsScaling(tt.ng)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsReady(t *testing.T) {
	tests := []struct {
		name     string
		ng       *v1alpha1.NodeGroup
		expected bool
	}{
		{
			name: "all ready",
			ng: &v1alpha1.NodeGroup{
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 5,
					DesiredNodes: 5,
					ReadyNodes:   5,
				},
			},
			expected: true,
		},
		{
			name: "not all ready",
			ng: &v1alpha1.NodeGroup{
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 5,
					DesiredNodes: 5,
					ReadyNodes:   3,
				},
			},
			expected: false,
		},
		{
			name: "scaling in progress",
			ng: &v1alpha1.NodeGroup{
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: 3,
					DesiredNodes: 5,
					ReadyNodes:   3,
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsReady(tt.ng)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetNodeGroupLabels(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ng",
		},
	}

	labels := GetNodeGroupLabels(ng)

	assert.NotNil(t, labels)
	assert.Equal(t, "test-ng", labels[GetNodeGroupNameLabel()])
	assert.Equal(t, "true", labels["autoscaler.vpsie.com/managed"])
}

func TestSetDesiredNodes(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-ng",
		},
		Status: v1alpha1.NodeGroupStatus{
			CurrentNodes: 3,
			DesiredNodes: 3,
		},
	}

	// No change
	SetDesiredNodes(ng, 3)
	assert.Nil(t, ng.Status.LastScaleTime)

	// Scale up
	SetDesiredNodes(ng, 5)
	assert.Equal(t, int32(5), ng.Status.DesiredNodes)
	assert.NotNil(t, ng.Status.LastScaleTime)
	assert.NotNil(t, ng.Status.LastScaleUpTime)

	// Scale down
	ng.Status.CurrentNodes = 5
	SetDesiredNodes(ng, 3)
	assert.Equal(t, int32(3), ng.Status.DesiredNodes)
	assert.NotNil(t, ng.Status.LastScaleDownTime)
}

func TestIsManagedNodeGroup(t *testing.T) {
	tests := []struct {
		name     string
		ng       *v1alpha1.NodeGroup
		expected bool
	}{
		{
			name: "NodeGroup with managed label set to true",
			ng: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "managed-ng",
					Labels: map[string]string{
						ManagedLabelKey: ManagedLabelValue,
					},
				},
			},
			expected: true,
		},
		{
			name: "NodeGroup with managed label set to false",
			ng: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "unmanaged-ng",
					Labels: map[string]string{
						ManagedLabelKey: "false",
					},
				},
			},
			expected: false,
		},
		{
			name: "NodeGroup without managed label",
			ng: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-label-ng",
					Labels: map[string]string{
						"other-label": "value",
					},
				},
			},
			expected: false,
		},
		{
			name: "NodeGroup with nil labels map",
			ng: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "nil-labels-ng",
					Labels: nil,
				},
			},
			expected: false,
		},
		{
			name: "NodeGroup with empty labels map",
			ng: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "empty-labels-ng",
					Labels: map[string]string{},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsManagedNodeGroup(tt.ng)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetNodeGroupManaged(t *testing.T) {
	tests := []struct {
		name           string
		ng             *v1alpha1.NodeGroup
		expectedLabels map[string]string
	}{
		{
			name: "NodeGroup with nil labels",
			ng: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "nil-labels-ng",
					Labels: nil,
				},
			},
			expectedLabels: map[string]string{
				ManagedLabelKey: ManagedLabelValue,
			},
		},
		{
			name: "NodeGroup with existing labels",
			ng: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "existing-labels-ng",
					Labels: map[string]string{
						"existing-label": "value",
					},
				},
			},
			expectedLabels: map[string]string{
				"existing-label": "value",
				ManagedLabelKey:  ManagedLabelValue,
			},
		},
		{
			name: "NodeGroup already managed - idempotent",
			ng: &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name: "already-managed-ng",
					Labels: map[string]string{
						ManagedLabelKey: ManagedLabelValue,
					},
				},
			},
			expectedLabels: map[string]string{
				ManagedLabelKey: ManagedLabelValue,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetNodeGroupManaged(tt.ng)
			assert.Equal(t, tt.expectedLabels, tt.ng.Labels)
			// Verify idempotency by calling again
			SetNodeGroupManaged(tt.ng)
			assert.Equal(t, tt.expectedLabels, tt.ng.Labels)
		})
	}
}

func TestManagedLabelSelector(t *testing.T) {
	selector := ManagedLabelSelector()
	assert.NotNil(t, selector)
	assert.Equal(t, ManagedLabelValue, selector[ManagedLabelKey])
}

func TestLabelConstants(t *testing.T) {
	// Verify constants are exported and have expected values
	assert.Equal(t, "autoscaler.vpsie.com/managed", ManagedLabelKey)
	assert.Equal(t, "true", ManagedLabelValue)
	assert.Equal(t, "autoscaler.vpsie.com/nodegroup", NodeGroupNameLabelKey)
}
