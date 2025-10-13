package vpsienode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

func TestSetCondition(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Set a new condition
	SetCondition(vn, v1alpha1.VPSieNodeConditionVPSReady, corev1.ConditionTrue, ReasonProvisioned, "VPS is ready")

	assert.Len(t, vn.Status.Conditions, 1)
	assert.Equal(t, v1alpha1.VPSieNodeConditionVPSReady, vn.Status.Conditions[0].Type)
	assert.Equal(t, string(corev1.ConditionTrue), vn.Status.Conditions[0].Status)
	assert.Equal(t, ReasonProvisioned, vn.Status.Conditions[0].Reason)
	assert.Equal(t, "VPS is ready", vn.Status.Conditions[0].Message)

	// Update existing condition
	SetCondition(vn, v1alpha1.VPSieNodeConditionVPSReady, corev1.ConditionFalse, ReasonProvisioning, "VPS is provisioning")

	assert.Len(t, vn.Status.Conditions, 1)
	assert.Equal(t, string(corev1.ConditionFalse), vn.Status.Conditions[0].Status)
	assert.Equal(t, ReasonProvisioning, vn.Status.Conditions[0].Reason)

	// Add different condition
	SetCondition(vn, v1alpha1.VPSieNodeConditionNodeJoined, corev1.ConditionTrue, ReasonJoined, "Node joined")

	assert.Len(t, vn.Status.Conditions, 2)
}

func TestGetCondition(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Non-existent condition
	cond := GetCondition(vn, v1alpha1.VPSieNodeConditionVPSReady)
	assert.Nil(t, cond)

	// Existing condition
	SetCondition(vn, v1alpha1.VPSieNodeConditionVPSReady, corev1.ConditionTrue, ReasonProvisioned, "VPS is ready")
	cond = GetCondition(vn, v1alpha1.VPSieNodeConditionVPSReady)

	assert.NotNil(t, cond)
	assert.Equal(t, v1alpha1.VPSieNodeConditionVPSReady, cond.Type)
	assert.Equal(t, string(corev1.ConditionTrue), cond.Status)
}

func TestIsConditionTrue(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Non-existent condition
	assert.False(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionVPSReady))

	// Condition set to False
	SetCondition(vn, v1alpha1.VPSieNodeConditionVPSReady, corev1.ConditionFalse, ReasonProvisioning, "Not ready")
	assert.False(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionVPSReady))

	// Condition set to True
	SetCondition(vn, v1alpha1.VPSieNodeConditionVPSReady, corev1.ConditionTrue, ReasonProvisioned, "Ready")
	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionVPSReady))
}

func TestIsConditionFalse(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Non-existent condition
	assert.False(t, IsConditionFalse(vn, v1alpha1.VPSieNodeConditionVPSReady))

	// Condition set to True
	SetCondition(vn, v1alpha1.VPSieNodeConditionVPSReady, corev1.ConditionTrue, ReasonProvisioned, "Ready")
	assert.False(t, IsConditionFalse(vn, v1alpha1.VPSieNodeConditionVPSReady))

	// Condition set to False
	SetCondition(vn, v1alpha1.VPSieNodeConditionVPSReady, corev1.ConditionFalse, ReasonProvisioning, "Not ready")
	assert.True(t, IsConditionFalse(vn, v1alpha1.VPSieNodeConditionVPSReady))
}

func TestRemoveCondition(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	SetCondition(vn, v1alpha1.VPSieNodeConditionVPSReady, corev1.ConditionTrue, ReasonProvisioned, "Ready")
	SetCondition(vn, v1alpha1.VPSieNodeConditionNodeJoined, corev1.ConditionTrue, ReasonJoined, "Joined")

	assert.Len(t, vn.Status.Conditions, 2)

	RemoveCondition(vn, v1alpha1.VPSieNodeConditionVPSReady)

	assert.Len(t, vn.Status.Conditions, 1)
	assert.Equal(t, v1alpha1.VPSieNodeConditionNodeJoined, vn.Status.Conditions[0].Type)
}

func TestSetVPSReadyCondition(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	SetVPSReadyCondition(vn, true, ReasonProvisioned, "VPS is ready")
	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionVPSReady))

	SetVPSReadyCondition(vn, false, ReasonProvisioning, "VPS is not ready")
	assert.True(t, IsConditionFalse(vn, v1alpha1.VPSieNodeConditionVPSReady))
}

func TestSetNodeJoinedCondition(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	SetNodeJoinedCondition(vn, true, ReasonJoined, "Node has joined")
	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionNodeJoined))

	SetNodeJoinedCondition(vn, false, ReasonJoining, "Node is joining")
	assert.True(t, IsConditionFalse(vn, v1alpha1.VPSieNodeConditionNodeJoined))
}

func TestSetNodeReadyCondition(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	SetNodeReadyCondition(vn, true, ReasonReady, "Node is ready")
	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionNodeReady))

	SetNodeReadyCondition(vn, false, ReasonJoining, "Node is not ready")
	assert.True(t, IsConditionFalse(vn, v1alpha1.VPSieNodeConditionNodeReady))
}

func TestSetErrorCondition(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	SetErrorCondition(vn, true, ReasonFailed, "Something went wrong")
	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionError))

	SetErrorCondition(vn, false, "", "")
	assert.True(t, IsConditionFalse(vn, v1alpha1.VPSieNodeConditionError))
}

func TestClearError(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
		Status: v1alpha1.VPSieNodeStatus{
			LastError: "Some error",
		},
	}

	SetErrorCondition(vn, true, ReasonFailed, "Error occurred")
	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionError))
	assert.NotEmpty(t, vn.Status.LastError)

	ClearError(vn)
	assert.True(t, IsConditionFalse(vn, v1alpha1.VPSieNodeConditionError))
	assert.Empty(t, vn.Status.LastError)
}

func TestRecordError(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	RecordError(vn, ReasonVPSieAPIError, "API call failed")

	assert.True(t, IsConditionTrue(vn, v1alpha1.VPSieNodeConditionError))
	assert.Equal(t, "API call failed", vn.Status.LastError)

	cond := GetCondition(vn, v1alpha1.VPSieNodeConditionError)
	assert.NotNil(t, cond)
	assert.Equal(t, ReasonVPSieAPIError, cond.Reason)
	assert.Equal(t, "API call failed", cond.Message)
}
