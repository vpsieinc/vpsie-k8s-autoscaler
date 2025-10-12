package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestVPSieNode_Creation(t *testing.T) {
	vn := &VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vpsie-node-1001",
			Namespace: "kube-system",
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
	}

	assert.Equal(t, "vpsie-node-1001", vn.Name)
	assert.Equal(t, "kube-system", vn.Namespace)
	assert.Equal(t, 1001, vn.Spec.VPSieInstanceID)
	assert.Equal(t, "small-2cpu-4gb", vn.Spec.InstanceType)
	assert.Equal(t, "general-purpose", vn.Spec.NodeGroupName)
	assert.Equal(t, "us-east-1", vn.Spec.DatacenterID)
	assert.Equal(t, "192.0.2.10", vn.Spec.IPAddress)
	assert.Equal(t, "2001:db8::1", vn.Spec.IPv6Address)
}

func TestVPSieNode_Phases(t *testing.T) {
	tests := []struct {
		name  string
		phase VPSieNodePhase
	}{
		{"Pending phase", VPSieNodePhasePending},
		{"Provisioning phase", VPSieNodePhaseProvisioning},
		{"Provisioned phase", VPSieNodePhaseProvisioned},
		{"Joining phase", VPSieNodePhaseJoining},
		{"Ready phase", VPSieNodePhaseReady},
		{"Terminating phase", VPSieNodePhaseTerminating},
		{"Deleting phase", VPSieNodePhaseDeleting},
		{"Failed phase", VPSieNodePhaseFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vn := &VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-node",
					Namespace: "kube-system",
				},
				Status: VPSieNodeStatus{
					Phase: tt.phase,
				},
			}

			assert.Equal(t, tt.phase, vn.Status.Phase)
		})
	}
}

func TestVPSieNode_StatusWithResources(t *testing.T) {
	now := metav1.Now()

	vn := &VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vpsie-node-2000",
			Namespace: "kube-system",
		},
		Spec: VPSieNodeSpec{
			VPSieInstanceID: 2000,
			InstanceType:    "large-8cpu-16gb",
			NodeGroupName:   "high-memory",
			DatacenterID:    "us-west-2",
		},
		Status: VPSieNodeStatus{
			Phase:       VPSieNodePhaseReady,
			NodeName:    "vpsie-node-2000",
			VPSieStatus: "running",
			Hostname:    "vpsie-node-2000.example.com",
			Resources: NodeResources{
				CPU:         8,
				MemoryMB:    16384,
				DiskGB:      200,
				BandwidthGB: 4000,
			},
			CreatedAt:     &now,
			ProvisionedAt: &now,
			JoinedAt:      &now,
			ReadyAt:       &now,
		},
	}

	assert.Equal(t, VPSieNodePhaseReady, vn.Status.Phase)
	assert.Equal(t, "vpsie-node-2000", vn.Status.NodeName)
	assert.Equal(t, "running", vn.Status.VPSieStatus)
	assert.Equal(t, 8, vn.Status.Resources.CPU)
	assert.Equal(t, 16384, vn.Status.Resources.MemoryMB)
	assert.Equal(t, 200, vn.Status.Resources.DiskGB)
	assert.Equal(t, 4000, vn.Status.Resources.BandwidthGB)
	assert.NotNil(t, vn.Status.CreatedAt)
	assert.NotNil(t, vn.Status.ProvisionedAt)
	assert.NotNil(t, vn.Status.JoinedAt)
	assert.NotNil(t, vn.Status.ReadyAt)
}

func TestVPSieNode_Conditions(t *testing.T) {
	now := metav1.Now()

	tests := []struct {
		name          string
		conditionType VPSieNodeConditionType
		status        string
		reason        string
		message       string
	}{
		{
			name:          "VPSReady condition",
			conditionType: VPSieNodeConditionVPSReady,
			status:        "True",
			reason:        "VPSRunning",
			message:       "VPS is running on VPSie",
		},
		{
			name:          "NodeJoined condition",
			conditionType: VPSieNodeConditionNodeJoined,
			status:        "True",
			reason:        "NodeJoinedCluster",
			message:       "Node successfully joined Kubernetes cluster",
		},
		{
			name:          "NodeReady condition",
			conditionType: VPSieNodeConditionNodeReady,
			status:        "True",
			reason:        "NodeReady",
			message:       "Kubernetes node is ready to accept workloads",
		},
		{
			name:          "Error condition",
			conditionType: VPSieNodeConditionError,
			status:        "True",
			reason:        "ProvisioningFailed",
			message:       "Failed to provision VPS on VPSie",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			condition := VPSieNodeCondition{
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

func TestVPSieNode_WithMultipleConditions(t *testing.T) {
	now := metav1.Now()

	vn := &VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vpsie-node-3000",
			Namespace: "kube-system",
		},
		Status: VPSieNodeStatus{
			Phase: VPSieNodePhaseReady,
			Conditions: []VPSieNodeCondition{
				{
					Type:               VPSieNodeConditionVPSReady,
					Status:             "True",
					LastTransitionTime: now,
					Reason:             "VPSRunning",
					Message:            "VPS is running",
				},
				{
					Type:               VPSieNodeConditionNodeJoined,
					Status:             "True",
					LastTransitionTime: now,
					Reason:             "NodeJoined",
					Message:            "Node joined cluster",
				},
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

	assert.Len(t, vn.Status.Conditions, 3)
	assert.Equal(t, VPSieNodeConditionVPSReady, vn.Status.Conditions[0].Type)
	assert.Equal(t, VPSieNodeConditionNodeJoined, vn.Status.Conditions[1].Type)
	assert.Equal(t, VPSieNodeConditionNodeReady, vn.Status.Conditions[2].Type)
}

func TestVPSieNode_LifecycleTimestamps(t *testing.T) {
	created := metav1.Now()
	provisioned := metav1.NewTime(created.Add(3 * 60 * 1000000000)) // +3 minutes
	joined := metav1.NewTime(created.Add(5 * 60 * 1000000000))      // +5 minutes
	ready := metav1.NewTime(created.Add(6 * 60 * 1000000000))       // +6 minutes

	vn := &VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vpsie-node-4000",
			Namespace: "kube-system",
		},
		Status: VPSieNodeStatus{
			Phase:         VPSieNodePhaseReady,
			CreatedAt:     &created,
			ProvisionedAt: &provisioned,
			JoinedAt:      &joined,
			ReadyAt:       &ready,
		},
	}

	assert.NotNil(t, vn.Status.CreatedAt)
	assert.NotNil(t, vn.Status.ProvisionedAt)
	assert.NotNil(t, vn.Status.JoinedAt)
	assert.NotNil(t, vn.Status.ReadyAt)
	assert.True(t, vn.Status.ProvisionedAt.After(vn.Status.CreatedAt.Time))
	assert.True(t, vn.Status.JoinedAt.After(vn.Status.ProvisionedAt.Time))
	assert.True(t, vn.Status.ReadyAt.After(vn.Status.JoinedAt.Time))
}

func TestVPSieNode_TerminatingState(t *testing.T) {
	now := metav1.Now()

	vn := &VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vpsie-node-5000",
			Namespace: "kube-system",
		},
		Status: VPSieNodeStatus{
			Phase:         VPSieNodePhaseTerminating,
			TerminatingAt: &now,
			Conditions: []VPSieNodeCondition{
				{
					Type:               VPSieNodeConditionNodeReady,
					Status:             "False",
					LastTransitionTime: now,
					Reason:             "NodeTerminating",
					Message:            "Node is being terminated",
				},
			},
		},
	}

	assert.Equal(t, VPSieNodePhaseTerminating, vn.Status.Phase)
	assert.NotNil(t, vn.Status.TerminatingAt)
	assert.Equal(t, "False", vn.Status.Conditions[0].Status)
}

func TestVPSieNode_FailedState(t *testing.T) {
	now := metav1.Now()

	vn := &VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vpsie-node-6000",
			Namespace: "kube-system",
		},
		Status: VPSieNodeStatus{
			Phase:     VPSieNodePhaseFailed,
			LastError: "Failed to provision VPS: VPSie API returned 500 Internal Server Error",
			Conditions: []VPSieNodeCondition{
				{
					Type:               VPSieNodeConditionError,
					Status:             "True",
					LastTransitionTime: now,
					Reason:             "ProvisioningFailed",
					Message:            "VPSie API error during provisioning",
				},
			},
		},
	}

	assert.Equal(t, VPSieNodePhaseFailed, vn.Status.Phase)
	assert.Contains(t, vn.Status.LastError, "Failed to provision")
	assert.Contains(t, vn.Status.LastError, "500 Internal Server Error")
	assert.Len(t, vn.Status.Conditions, 1)
	assert.Equal(t, VPSieNodeConditionError, vn.Status.Conditions[0].Type)
}

func TestNodeResources_Fields(t *testing.T) {
	resources := NodeResources{
		CPU:         4,
		MemoryMB:    8192,
		DiskGB:      100,
		BandwidthGB: 2000,
	}

	assert.Equal(t, 4, resources.CPU)
	assert.Equal(t, 8192, resources.MemoryMB)
	assert.Equal(t, 100, resources.DiskGB)
	assert.Equal(t, 2000, resources.BandwidthGB)
}

func TestVPSieNode_WithoutOptionalFields(t *testing.T) {
	// Test VPSieNode with minimal required fields
	vn := &VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minimal-node",
			Namespace: "kube-system",
		},
		Spec: VPSieNodeSpec{
			VPSieInstanceID: 7000,
			InstanceType:    "small-2cpu-4gb",
			NodeGroupName:   "general-purpose",
			DatacenterID:    "us-east-1",
			// Optional fields not set: NodeName, IPAddress, IPv6Address
		},
		Status: VPSieNodeStatus{
			Phase: VPSieNodePhasePending,
			// Optional fields not set
		},
	}

	assert.Equal(t, 7000, vn.Spec.VPSieInstanceID)
	assert.Equal(t, VPSieNodePhasePending, vn.Status.Phase)
	assert.Empty(t, vn.Spec.NodeName)
	assert.Empty(t, vn.Spec.IPAddress)
	assert.Empty(t, vn.Spec.IPv6Address)
	assert.Nil(t, vn.Status.CreatedAt)
	assert.Empty(t, vn.Status.Conditions)
}

func TestVPSieNodeList_Creation(t *testing.T) {
	vnList := &VPSieNodeList{
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

	assert.Len(t, vnList.Items, 2)
	assert.Equal(t, "vn-1", vnList.Items[0].Name)
	assert.Equal(t, "vn-2", vnList.Items[1].Name)
	assert.Equal(t, 1001, vnList.Items[0].Spec.VPSieInstanceID)
	assert.Equal(t, 1002, vnList.Items[1].Spec.VPSieInstanceID)
}

func TestVPSieNode_ObservedGeneration(t *testing.T) {
	vn := &VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "vpsie-node-8000",
			Namespace:  "kube-system",
			Generation: 5,
		},
		Status: VPSieNodeStatus{
			Phase:              VPSieNodePhaseReady,
			ObservedGeneration: 5,
		},
	}

	assert.Equal(t, int64(5), vn.Generation)
	assert.Equal(t, int64(5), vn.Status.ObservedGeneration)
}
