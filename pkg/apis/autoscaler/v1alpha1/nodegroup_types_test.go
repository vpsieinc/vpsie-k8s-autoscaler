package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestNodeGroup_Creation(t *testing.T) {
	ng := &NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup",
			Namespace: "kube-system",
		},
		Spec: NodeGroupSpec{
			MinNodes:     2,
			MaxNodes:     10,
			DatacenterID: "us-east-1",
			OfferingIDs:  []string{"small-2cpu-4gb", "medium-4cpu-8gb"},
			OSImageID:    "ubuntu-22.04-lts",
		},
	}

	assert.Equal(t, "test-nodegroup", ng.Name)
	assert.Equal(t, "kube-system", ng.Namespace)
	assert.Equal(t, int32(2), ng.Spec.MinNodes)
	assert.Equal(t, int32(10), ng.Spec.MaxNodes)
	assert.Equal(t, "us-east-1", ng.Spec.DatacenterID)
	assert.Len(t, ng.Spec.OfferingIDs, 2)
}

func TestNodeGroup_WithLabelsAndTaints(t *testing.T) {
	ng := &NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup",
			Namespace: "kube-system",
		},
		Spec: NodeGroupSpec{
			MinNodes:     1,
			MaxNodes:     5,
			DatacenterID: "us-west-2",
			OfferingIDs:  []string{"large-8cpu-16gb"},
			OSImageID:    "ubuntu-22.04-lts",
			Labels: map[string]string{
				"workload-type": "memory-intensive",
				"environment":   "production",
			},
			Taints: []corev1.Taint{
				{
					Key:    "workload-type",
					Value:  "memory-intensive",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
	}

	assert.Equal(t, "memory-intensive", ng.Spec.Labels["workload-type"])
	assert.Equal(t, "production", ng.Spec.Labels["environment"])
	assert.Len(t, ng.Spec.Taints, 1)
	assert.Equal(t, "workload-type", ng.Spec.Taints[0].Key)
	assert.Equal(t, corev1.TaintEffectNoSchedule, ng.Spec.Taints[0].Effect)
}

func TestNodeGroup_WithScalingPolicies(t *testing.T) {
	ng := &NodeGroup{
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
			ScaleUpPolicy: ScaleUpPolicy{
				Enabled:                    true,
				StabilizationWindowSeconds: 60,
				CPUThreshold:               80,
				MemoryThreshold:            80,
			},
			ScaleDownPolicy: ScaleDownPolicy{
				Enabled:                    true,
				StabilizationWindowSeconds: 600,
				CPUThreshold:               50,
				MemoryThreshold:            50,
				CooldownSeconds:            600,
			},
		},
	}

	assert.True(t, ng.Spec.ScaleUpPolicy.Enabled)
	assert.Equal(t, int32(60), ng.Spec.ScaleUpPolicy.StabilizationWindowSeconds)
	assert.Equal(t, int32(80), ng.Spec.ScaleUpPolicy.CPUThreshold)
	assert.Equal(t, int32(80), ng.Spec.ScaleUpPolicy.MemoryThreshold)

	assert.True(t, ng.Spec.ScaleDownPolicy.Enabled)
	assert.Equal(t, int32(600), ng.Spec.ScaleDownPolicy.StabilizationWindowSeconds)
	assert.Equal(t, int32(50), ng.Spec.ScaleDownPolicy.CPUThreshold)
	assert.Equal(t, int32(50), ng.Spec.ScaleDownPolicy.MemoryThreshold)
	assert.Equal(t, int32(600), ng.Spec.ScaleDownPolicy.CooldownSeconds)
}

func TestNodeGroup_WithUserData(t *testing.T) {
	userData := `#!/bin/bash
set -euo pipefail
apt-get update
kubeadm join control-plane:6443 --token xxx --discovery-token-ca-cert-hash sha256:yyy`

	ng := &NodeGroup{
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
			UserData:     userData,
			SSHKeyIDs:    []string{"ssh-key-1", "ssh-key-2"},
			Tags:         []string{"k8s-cluster", "production"},
			Notes:        "Test node group",
		},
	}

	assert.Contains(t, ng.Spec.UserData, "kubeadm join")
	assert.Len(t, ng.Spec.SSHKeyIDs, 2)
	assert.Len(t, ng.Spec.Tags, 2)
	assert.Equal(t, "Test node group", ng.Spec.Notes)
}

func TestNodeGroup_Status(t *testing.T) {
	now := metav1.Now()

	ng := &NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-nodegroup",
			Namespace: "kube-system",
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
					CreatedAt:    &now,
					IPAddress:    "192.0.2.10",
				},
				{
					NodeName:     "node-2",
					VPSID:        1002,
					InstanceType: "small-2cpu-4gb",
					Status:       "Ready",
					CreatedAt:    &now,
					IPAddress:    "192.0.2.11",
				},
				{
					NodeName:     "node-3",
					VPSID:        1003,
					InstanceType: "small-2cpu-4gb",
					Status:       "Ready",
					CreatedAt:    &now,
					IPAddress:    "192.0.2.12",
				},
			},
			Conditions: []NodeGroupCondition{
				{
					Type:               NodeGroupReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: now,
					Reason:             "AllNodesReady",
					Message:            "All nodes in group are ready",
				},
			},
			LastScaleTime: &now,
		},
	}

	assert.Equal(t, int32(3), ng.Status.CurrentNodes)
	assert.Equal(t, int32(3), ng.Status.DesiredNodes)
	assert.Equal(t, int32(3), ng.Status.ReadyNodes)
	assert.Len(t, ng.Status.Nodes, 3)
	assert.Equal(t, "node-1", ng.Status.Nodes[0].NodeName)
	assert.Equal(t, 1001, ng.Status.Nodes[0].VPSID)
	assert.Len(t, ng.Status.Conditions, 1)
	assert.Equal(t, NodeGroupReady, ng.Status.Conditions[0].Type)
}

func TestNodeGroup_Conditions(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		name          string
		conditionType NodeGroupConditionType
		status        corev1.ConditionStatus
		reason        string
		message       string
	}{
		{
			name:          "Ready condition",
			conditionType: NodeGroupReady,
			status:        corev1.ConditionTrue,
			reason:        "AllNodesReady",
			message:       "All nodes are ready",
		},
		{
			name:          "Scaling condition",
			conditionType: NodeGroupScaling,
			status:        corev1.ConditionTrue,
			reason:        "ScalingUp",
			message:       "Adding 2 new nodes",
		},
		{
			name:          "Error condition",
			conditionType: NodeGroupError,
			status:        corev1.ConditionTrue,
			reason:        "VPSieAPIError",
			message:       "Failed to create VPS instance",
		},
		{
			name:          "AtMinCapacity condition",
			conditionType: NodeGroupAtMinCapacity,
			status:        corev1.ConditionTrue,
			reason:        "MinNodesReached",
			message:       "Cannot scale down below minimum",
		},
		{
			name:          "AtMaxCapacity condition",
			conditionType: NodeGroupAtMaxCapacity,
			status:        corev1.ConditionTrue,
			reason:        "MaxNodesReached",
			message:       "Cannot scale up above maximum",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := NodeGroupCondition{
				Type:               tt.conditionType,
				Status:             tt.status,
				LastTransitionTime: now,
				Reason:             tt.reason,
				Message:            tt.message,
			}

			assert.Equal(t, tt.conditionType, condition.Type)
			assert.Equal(t, tt.status, condition.Status)
			assert.Equal(t, tt.reason, condition.Reason)
			assert.Equal(t, tt.message, condition.Message)
		})
	}
}

func TestNodeGroup_MixedInstances(t *testing.T) {
	ng := &NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "mixed-nodegroup",
			Namespace: "kube-system",
		},
		Spec: NodeGroupSpec{
			MinNodes:              2,
			MaxNodes:              10,
			DatacenterID:          "us-east-1",
			OfferingIDs:           []string{"small-2cpu-4gb", "medium-4cpu-8gb", "large-8cpu-16gb"},
			OSImageID:             "ubuntu-22.04-lts",
			PreferredInstanceType: "small-2cpu-4gb",
			AllowMixedInstances:   true,
		},
	}

	assert.Len(t, ng.Spec.OfferingIDs, 3)
	assert.Equal(t, "small-2cpu-4gb", ng.Spec.PreferredInstanceType)
	assert.True(t, ng.Spec.AllowMixedInstances)
}

func TestNodeGroup_ScaleDownDisabled(t *testing.T) {
	ng := &NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-scaledown",
			Namespace: "kube-system",
		},
		Spec: NodeGroupSpec{
			MinNodes:     2,
			MaxNodes:     10,
			DatacenterID: "us-east-1",
			OfferingIDs:  []string{"small-2cpu-4gb"},
			OSImageID:    "ubuntu-22.04-lts",
			ScaleUpPolicy: ScaleUpPolicy{
				Enabled: true,
			},
			ScaleDownPolicy: ScaleDownPolicy{
				Enabled: false, // Scale-down disabled
			},
		},
	}

	assert.True(t, ng.Spec.ScaleUpPolicy.Enabled)
	assert.False(t, ng.Spec.ScaleDownPolicy.Enabled)
}

func TestNodeInfo_Fields(t *testing.T) {
	now := metav1.Now()

	nodeInfo := NodeInfo{
		NodeName:     "test-node-1",
		VPSID:        5000,
		InstanceType: "medium-4cpu-8gb",
		Status:       "Ready",
		CreatedAt:    &now,
		ReadyAt:      &now,
		IPAddress:    "192.0.2.100",
	}

	assert.Equal(t, "test-node-1", nodeInfo.NodeName)
	assert.Equal(t, 5000, nodeInfo.VPSID)
	assert.Equal(t, "medium-4cpu-8gb", nodeInfo.InstanceType)
	assert.Equal(t, "Ready", nodeInfo.Status)
	assert.Equal(t, "192.0.2.100", nodeInfo.IPAddress)
	assert.NotNil(t, nodeInfo.CreatedAt)
	assert.NotNil(t, nodeInfo.ReadyAt)
}

func TestNodeGroupList_Creation(t *testing.T) {
	ngList := &NodeGroupList{
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

	assert.Len(t, ngList.Items, 2)
	assert.Equal(t, "ng-1", ngList.Items[0].Name)
	assert.Equal(t, "ng-2", ngList.Items[1].Name)
}
