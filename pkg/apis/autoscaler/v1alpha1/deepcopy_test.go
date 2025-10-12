package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeGroup_DeepCopy(t *testing.T) {
	now := metav1.Now()
	original := &NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup",
			Namespace: "kube-system",
			Labels: map[string]string{
				"environment": "production",
			},
		},
		Spec: NodeGroupSpec{
			MinNodes:     2,
			MaxNodes:     10,
			DatacenterID: "us-east-1",
			OfferingIDs:  []string{"small-2cpu-4gb", "medium-4cpu-8gb"},
			OSImageID:    "ubuntu-22.04-lts",
			Labels: map[string]string{
				"workload-type": "general",
			},
			Taints: []corev1.Taint{
				{
					Key:    "test",
					Value:  "value",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
			ScaleUpPolicy: ScaleUpPolicy{
				Enabled:      true,
				CPUThreshold: 80,
			},
			ScaleDownPolicy: ScaleDownPolicy{
				Enabled:      true,
				CPUThreshold: 50,
			},
			SSHKeyIDs: []string{"key-1", "key-2"},
			Tags:      []string{"tag-1", "tag-2"},
		},
		Status: NodeGroupStatus{
			CurrentNodes: 3,
			DesiredNodes: 3,
			ReadyNodes:   3,
			Nodes: []NodeInfo{
				{
					NodeName:     "node-1",
					VPSID:        1001,
					InstanceType: "small-2cpu-4gb",
					Status:       "Ready",
					IPAddress:    "192.0.2.10",
				},
			},
			Conditions: []NodeGroupCondition{
				{
					Type:               NodeGroupReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "AllReady",
					Message:            "All nodes ready",
				},
			},
			LastScaleTime: &now,
		},
	}

	// Create deep copy
	copied := original.DeepCopy()

	// Verify it's a different object
	assert.NotSame(t, original, copied)

	// Verify values are equal
	assert.Equal(t, original.Name, copied.Name)
	assert.Equal(t, original.Namespace, copied.Namespace)
	assert.Equal(t, original.Spec.MinNodes, copied.Spec.MinNodes)
	assert.Equal(t, original.Spec.MaxNodes, copied.Spec.MaxNodes)
	assert.Equal(t, original.Spec.DatacenterID, copied.Spec.DatacenterID)
	assert.Equal(t, original.Status.CurrentNodes, copied.Status.CurrentNodes)

	// Verify slices are deep copied (different memory addresses)
	assert.NotSame(t, &original.Spec.OfferingIDs, &copied.Spec.OfferingIDs)
	assert.NotSame(t, &original.Spec.Taints, &copied.Spec.Taints)
	assert.NotSame(t, &original.Status.Nodes, &copied.Status.Nodes)
	assert.NotSame(t, &original.Status.Conditions, &copied.Status.Conditions)

	// Verify maps are deep copied
	assert.NotSame(t, &original.Spec.Labels, &copied.Spec.Labels)

	// Modify original and verify copied is unchanged
	original.Spec.MinNodes = 5
	original.Spec.OfferingIDs = append(original.Spec.OfferingIDs, "new-offering")
	original.Spec.Labels["new-label"] = "new-value"

	assert.Equal(t, int32(2), copied.Spec.MinNodes)
	assert.Len(t, copied.Spec.OfferingIDs, 2)
	assert.NotContains(t, copied.Spec.Labels, "new-label")
}

func TestNodeGroup_DeepCopyInto(t *testing.T) {
	original := &NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup",
			Namespace: "kube-system",
		},
		Spec: NodeGroupSpec{
			MinNodes:     2,
			MaxNodes:     10,
			DatacenterID: "us-east-1",
			OfferingIDs:  []string{"small-2cpu-4gb"},
			OSImageID:    "ubuntu-22.04-lts",
		},
	}

	target := &NodeGroup{}
	original.DeepCopyInto(target)

	assert.Equal(t, original.Name, target.Name)
	assert.Equal(t, original.Spec.MinNodes, target.Spec.MinNodes)
	assert.Equal(t, original.Spec.DatacenterID, target.Spec.DatacenterID)

	// Verify it's a deep copy
	original.Spec.MinNodes = 5
	assert.Equal(t, int32(2), target.Spec.MinNodes)
}

func TestNodeGroup_DeepCopyObject(t *testing.T) {
	original := &NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup",
			Namespace: "kube-system",
		},
		Spec: NodeGroupSpec{
			MinNodes:     2,
			MaxNodes:     10,
			DatacenterID: "us-east-1",
			OfferingIDs:  []string{"small-2cpu-4gb"},
			OSImageID:    "ubuntu-22.04-lts",
		},
	}

	obj := original.DeepCopyObject()
	assert.NotNil(t, obj)

	copied, ok := obj.(*NodeGroup)
	assert.True(t, ok)
	assert.Equal(t, original.Name, copied.Name)
	assert.Equal(t, original.Spec.MinNodes, copied.Spec.MinNodes)
}

func TestNodeGroupList_DeepCopy(t *testing.T) {
	original := &NodeGroupList{
		Items: []NodeGroup{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ng-1",
					Namespace: "kube-system",
				},
				Spec: NodeGroupSpec{
					MinNodes:     2,
					MaxNodes:     10,
					DatacenterID: "us-east-1",
					OfferingIDs:  []string{"small-2cpu-4gb"},
					OSImageID:    "ubuntu-22.04-lts",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ng-2",
					Namespace: "kube-system",
				},
				Spec: NodeGroupSpec{
					MinNodes:     1,
					MaxNodes:     5,
					DatacenterID: "us-west-2",
					OfferingIDs:  []string{"large-8cpu-16gb"},
					OSImageID:    "ubuntu-22.04-lts",
				},
			},
		},
	}

	copied := original.DeepCopy()

	assert.NotSame(t, original, copied)
	assert.Len(t, copied.Items, 2)
	assert.Equal(t, original.Items[0].Name, copied.Items[0].Name)
	assert.Equal(t, original.Items[1].Name, copied.Items[1].Name)

	// Modify original
	original.Items[0].Spec.MinNodes = 10
	assert.Equal(t, int32(2), copied.Items[0].Spec.MinNodes)
}

func TestVPSieNode_DeepCopy(t *testing.T) {
	now := metav1.Now()
	original := &VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vpsie-node-1001",
			Namespace: "kube-system",
			Labels: map[string]string{
				"nodegroup": "general-purpose",
			},
		},
		Spec: VPSieNodeSpec{
			VPSieInstanceID: 1001,
			InstanceType:    "small-2cpu-4gb",
			NodeGroupName:   "general-purpose",
			DatacenterID:    "us-east-1",
			NodeName:        "vpsie-node-1001",
			IPAddress:       "192.0.2.10",
			IPv6Address:     "2001:db8::1",
		},
		Status: VPSieNodeStatus{
			Phase:       VPSieNodePhaseReady,
			NodeName:    "vpsie-node-1001",
			VPSieStatus: "running",
			Hostname:    "vpsie-node-1001.example.com",
			Resources: NodeResources{
				CPU:      2,
				MemoryMB: 4096,
				DiskGB:   80,
			},
			CreatedAt: &now,
			ReadyAt:   &now,
			Conditions: []VPSieNodeCondition{
				{
					Type:               VPSieNodeConditionNodeReady,
					Status:             "True",
					LastTransitionTime: now,
					Reason:             "NodeReady",
					Message:            "Node is ready",
				},
			},
		},
	}

	copied := original.DeepCopy()

	assert.NotSame(t, original, copied)
	assert.Equal(t, original.Name, copied.Name)
	assert.Equal(t, original.Spec.VPSieInstanceID, copied.Spec.VPSieInstanceID)
	assert.Equal(t, original.Spec.InstanceType, copied.Spec.InstanceType)
	assert.Equal(t, original.Status.Phase, copied.Status.Phase)
	assert.Equal(t, original.Status.Resources.CPU, copied.Status.Resources.CPU)

	// Verify conditions are deep copied
	assert.NotSame(t, &original.Status.Conditions, &copied.Status.Conditions)

	// Modify original
	original.Spec.VPSieInstanceID = 9999
	original.Status.Resources.CPU = 10
	assert.Equal(t, 1001, copied.Spec.VPSieInstanceID)
	assert.Equal(t, 2, copied.Status.Resources.CPU)
}

func TestVPSieNode_DeepCopyInto(t *testing.T) {
	original := &VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vpsie-node-1001",
			Namespace: "kube-system",
		},
		Spec: VPSieNodeSpec{
			VPSieInstanceID: 1001,
			InstanceType:    "small-2cpu-4gb",
			NodeGroupName:   "general-purpose",
			DatacenterID:    "us-east-1",
		},
		Status: VPSieNodeStatus{
			Phase: VPSieNodePhaseReady,
		},
	}

	target := &VPSieNode{}
	original.DeepCopyInto(target)

	assert.Equal(t, original.Name, target.Name)
	assert.Equal(t, original.Spec.VPSieInstanceID, target.Spec.VPSieInstanceID)
	assert.Equal(t, original.Status.Phase, target.Status.Phase)

	// Verify deep copy
	original.Spec.VPSieInstanceID = 9999
	assert.Equal(t, 1001, target.Spec.VPSieInstanceID)
}

func TestVPSieNode_DeepCopyObject(t *testing.T) {
	original := &VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vpsie-node-1001",
			Namespace: "kube-system",
		},
		Spec: VPSieNodeSpec{
			VPSieInstanceID: 1001,
			InstanceType:    "small-2cpu-4gb",
			NodeGroupName:   "general-purpose",
			DatacenterID:    "us-east-1",
		},
	}

	obj := original.DeepCopyObject()
	assert.NotNil(t, obj)

	copied, ok := obj.(*VPSieNode)
	assert.True(t, ok)
	assert.Equal(t, original.Name, copied.Name)
	assert.Equal(t, original.Spec.VPSieInstanceID, copied.Spec.VPSieInstanceID)
}

func TestVPSieNodeList_DeepCopy(t *testing.T) {
	original := &VPSieNodeList{
		Items: []VPSieNode{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vn-1",
					Namespace: "kube-system",
				},
				Spec: VPSieNodeSpec{
					VPSieInstanceID: 1001,
					InstanceType:    "small-2cpu-4gb",
					NodeGroupName:   "general-purpose",
					DatacenterID:    "us-east-1",
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "vn-2",
					Namespace: "kube-system",
				},
				Spec: VPSieNodeSpec{
					VPSieInstanceID: 1002,
					InstanceType:    "large-8cpu-16gb",
					NodeGroupName:   "high-memory",
					DatacenterID:    "us-west-2",
				},
			},
		},
	}

	copied := original.DeepCopy()

	assert.NotSame(t, original, copied)
	assert.Len(t, copied.Items, 2)
	assert.Equal(t, original.Items[0].Name, copied.Items[0].Name)
	assert.Equal(t, original.Items[1].Name, copied.Items[1].Name)

	// Modify original
	original.Items[0].Spec.VPSieInstanceID = 9999
	assert.Equal(t, 1001, copied.Items[0].Spec.VPSieInstanceID)
}

func TestNodeGroup_NilDeepCopy(t *testing.T) {
	var ng *NodeGroup = nil
	copied := ng.DeepCopy()
	assert.Nil(t, copied)
}

func TestVPSieNode_NilDeepCopy(t *testing.T) {
	var vn *VPSieNode = nil
	copied := vn.DeepCopy()
	assert.Nil(t, copied)
}

func TestScaleUpPolicy_DeepCopy(t *testing.T) {
	original := ScaleUpPolicy{
		Enabled:                    true,
		StabilizationWindowSeconds: 60,
		CPUThreshold:               80,
		MemoryThreshold:            80,
	}

	// DeepCopy is generated for the parent struct, test through NodeGroupSpec
	ng := &NodeGroup{
		Spec: NodeGroupSpec{
			MinNodes:      2,
			MaxNodes:      10,
			DatacenterID:  "us-east-1",
			OfferingIDs:   []string{"small"},
			OSImageID:     "ubuntu",
			ScaleUpPolicy: original,
		},
	}

	copied := ng.DeepCopy()

	assert.Equal(t, original.Enabled, copied.Spec.ScaleUpPolicy.Enabled)
	assert.Equal(t, original.CPUThreshold, copied.Spec.ScaleUpPolicy.CPUThreshold)

	// Modify original
	ng.Spec.ScaleUpPolicy.CPUThreshold = 90
	assert.Equal(t, int32(80), copied.Spec.ScaleUpPolicy.CPUThreshold)
}

func TestNodeResources_DeepCopy(t *testing.T) {
	original := NodeResources{
		CPU:         4,
		MemoryMB:    8192,
		DiskGB:      100,
		BandwidthGB: 2000,
	}

	// DeepCopy is generated for the parent struct, test through VPSieNodeStatus
	vn := &VPSieNode{
		Status: VPSieNodeStatus{
			Phase:     VPSieNodePhaseReady,
			Resources: original,
		},
	}

	copied := vn.DeepCopy()

	assert.Equal(t, original.CPU, copied.Status.Resources.CPU)
	assert.Equal(t, original.MemoryMB, copied.Status.Resources.MemoryMB)

	// Modify original
	vn.Status.Resources.CPU = 16
	assert.Equal(t, 4, copied.Status.Resources.CPU)
}
