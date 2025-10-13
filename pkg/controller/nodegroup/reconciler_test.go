package nodegroup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

func TestBuildVPSieNode(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			DatacenterID: "dc-1",
			OfferingIDs:  []string{"offering-1", "offering-2"},
			OSImageID:    "ubuntu-22.04",
		},
	}

	r := &NodeGroupReconciler{}
	vn := r.buildVPSieNode(ng)

	assert.NotNil(t, vn)
	assert.Equal(t, "default", vn.Namespace)
	assert.Contains(t, vn.Name, "test-ng-")
	assert.Equal(t, "test-ng", vn.Labels[GetNodeGroupNameLabel()])
	assert.Equal(t, "true", vn.Labels["autoscaler.vpsie.com/managed"])
	assert.Equal(t, "offering-1", vn.Spec.InstanceType)
	assert.Equal(t, "test-ng", vn.Spec.NodeGroupName)
	assert.Equal(t, "dc-1", vn.Spec.DatacenterID)
}

func TestBuildVPSieNodeWithPreferredType(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			DatacenterID:          "dc-1",
			OfferingIDs:           []string{"offering-1", "offering-2"},
			PreferredInstanceType: "offering-2",
			OSImageID:             "ubuntu-22.04",
		},
	}

	r := &NodeGroupReconciler{}
	vn := r.buildVPSieNode(ng)

	assert.Equal(t, "offering-2", vn.Spec.InstanceType)
}

func TestSelectNodesToDelete(t *testing.T) {
	tests := []struct {
		name       string
		nodes      []v1alpha1.VPSieNode
		count      int
		expectLen  int
		expectNonReady bool
	}{
		{
			name: "select fewer than available",
			nodes: []v1alpha1.VPSieNode{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
					Status:     v1alpha1.VPSieNodeStatus{Phase: v1alpha1.VPSieNodePhaseReady},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
					Status:     v1alpha1.VPSieNodeStatus{Phase: v1alpha1.VPSieNodePhaseReady},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node-3"},
					Status:     v1alpha1.VPSieNodeStatus{Phase: v1alpha1.VPSieNodePhaseReady},
				},
			},
			count:     2,
			expectLen: 2,
		},
		{
			name: "select all nodes",
			nodes: []v1alpha1.VPSieNode{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
					Status:     v1alpha1.VPSieNodeStatus{Phase: v1alpha1.VPSieNodePhaseReady},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
					Status:     v1alpha1.VPSieNodeStatus{Phase: v1alpha1.VPSieNodePhaseReady},
				},
			},
			count:     2,
			expectLen: 2,
		},
		{
			name: "prioritize not-ready nodes",
			nodes: []v1alpha1.VPSieNode{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node-ready"},
					Status:     v1alpha1.VPSieNodeStatus{Phase: v1alpha1.VPSieNodePhaseReady},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node-pending"},
					Status:     v1alpha1.VPSieNodeStatus{Phase: v1alpha1.VPSieNodePhasePending},
				},
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node-provisioning"},
					Status:     v1alpha1.VPSieNodeStatus{Phase: v1alpha1.VPSieNodePhaseProvisioning},
				},
			},
			count:          2,
			expectLen:      2,
			expectNonReady: true,
		},
		{
			name: "request more than available",
			nodes: []v1alpha1.VPSieNode{
				{
					ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
					Status:     v1alpha1.VPSieNodeStatus{Phase: v1alpha1.VPSieNodePhaseReady},
				},
			},
			count:     5,
			expectLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := selectNodesToDelete(tt.nodes, tt.count)
			assert.Len(t, result, tt.expectLen)

			if tt.expectNonReady {
				// Verify that at least one selected node is not ready
				hasNonReady := false
				for _, node := range result {
					if node.Status.Phase != v1alpha1.VPSieNodePhaseReady {
						hasNonReady = true
						break
					}
				}
				assert.True(t, hasNonReady, "Expected at least one non-ready node to be selected")
			}
		})
	}
}

func TestGenerateRandomSuffix(t *testing.T) {
	suffix1 := generateRandomSuffix()
	suffix2 := generateRandomSuffix()

	assert.NotEmpty(t, suffix1)
	assert.NotEmpty(t, suffix2)
	// Suffixes should usually be different (though not guaranteed)
	// Just verify they're generated
}

func TestContainsString(t *testing.T) {
	slice := []string{"a", "b", "c"}

	assert.True(t, containsString(slice, "a"))
	assert.True(t, containsString(slice, "b"))
	assert.True(t, containsString(slice, "c"))
	assert.False(t, containsString(slice, "d"))
	assert.False(t, containsString([]string{}, "a"))
}

func TestRemoveString(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		remove   string
		expected []string
	}{
		{
			name:     "remove existing string",
			slice:    []string{"a", "b", "c"},
			remove:   "b",
			expected: []string{"a", "c"},
		},
		{
			name:     "remove non-existing string",
			slice:    []string{"a", "b", "c"},
			remove:   "d",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "remove from empty slice",
			slice:    []string{},
			remove:   "a",
			expected: nil,
		},
		{
			name:     "remove only element",
			slice:    []string{"a"},
			remove:   "a",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeString(tt.slice, tt.remove)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestScalingScenarios(t *testing.T) {
	tests := []struct {
		name           string
		minNodes       int32
		maxNodes       int32
		currentNodes   int32
		desiredNodes   int32
		needsScaleUp   bool
		needsScaleDown bool
		canScaleUp     bool
		canScaleDown   bool
	}{
		{
			name:           "at desired capacity",
			minNodes:       2,
			maxNodes:       10,
			currentNodes:   5,
			desiredNodes:   5,
			needsScaleUp:   false,
			needsScaleDown: false,
			canScaleUp:     true,
			canScaleDown:   true,
		},
		{
			name:           "needs scale up",
			minNodes:       2,
			maxNodes:       10,
			currentNodes:   3,
			desiredNodes:   7,
			needsScaleUp:   true,
			needsScaleDown: false,
			canScaleUp:     true,
			canScaleDown:   true,
		},
		{
			name:           "needs scale down",
			minNodes:       2,
			maxNodes:       10,
			currentNodes:   7,
			desiredNodes:   4,
			needsScaleUp:   false,
			needsScaleDown: true,
			canScaleUp:     true,
			canScaleDown:   true,
		},
		{
			name:           "at min capacity",
			minNodes:       2,
			maxNodes:       10,
			currentNodes:   2,
			desiredNodes:   2,
			needsScaleUp:   false,
			needsScaleDown: false,
			canScaleUp:     true,
			canScaleDown:   false,
		},
		{
			name:           "at max capacity",
			minNodes:       2,
			maxNodes:       10,
			currentNodes:   10,
			desiredNodes:   10,
			needsScaleUp:   false,
			needsScaleDown: false,
			canScaleUp:     false,
			canScaleDown:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &v1alpha1.NodeGroup{
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: tt.minNodes,
					MaxNodes: tt.maxNodes,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: tt.currentNodes,
					DesiredNodes: tt.desiredNodes,
				},
			}

			assert.Equal(t, tt.needsScaleUp, NeedsScaleUp(ng), "NeedsScaleUp")
			assert.Equal(t, tt.needsScaleDown, NeedsScaleDown(ng), "NeedsScaleDown")
			assert.Equal(t, tt.canScaleUp, CanScaleUp(ng), "CanScaleUp")
			assert.Equal(t, tt.canScaleDown, CanScaleDown(ng), "CanScaleDown")
		})
	}
}
