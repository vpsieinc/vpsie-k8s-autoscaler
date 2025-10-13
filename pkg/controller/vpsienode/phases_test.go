package vpsienode

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

func TestSetPhase(t *testing.T) {
	tests := []struct {
		name        string
		phase       v1alpha1.VPSieNodePhase
		checkField  func(*v1alpha1.VPSieNode) bool
		description string
	}{
		{
			name:  "Provisioning phase sets CreatedAt",
			phase: v1alpha1.VPSieNodePhaseProvisioning,
			checkField: func(vn *v1alpha1.VPSieNode) bool {
				return vn.Status.CreatedAt != nil
			},
			description: "CreatedAt should be set",
		},
		{
			name:  "Provisioned phase sets ProvisionedAt",
			phase: v1alpha1.VPSieNodePhaseProvisioned,
			checkField: func(vn *v1alpha1.VPSieNode) bool {
				return vn.Status.ProvisionedAt != nil
			},
			description: "ProvisionedAt should be set",
		},
		{
			name:  "Ready phase sets JoinedAt and ReadyAt",
			phase: v1alpha1.VPSieNodePhaseReady,
			checkField: func(vn *v1alpha1.VPSieNode) bool {
				return vn.Status.JoinedAt != nil && vn.Status.ReadyAt != nil
			},
			description: "JoinedAt and ReadyAt should be set",
		},
		{
			name:  "Terminating phase sets TerminatingAt",
			phase: v1alpha1.VPSieNodePhaseTerminating,
			checkField: func(vn *v1alpha1.VPSieNode) bool {
				return vn.Status.TerminatingAt != nil
			},
			description: "TerminatingAt should be set",
		},
		{
			name:  "Deleting phase sets DeletedAt",
			phase: v1alpha1.VPSieNodePhaseDeleting,
			checkField: func(vn *v1alpha1.VPSieNode) bool {
				return vn.Status.DeletedAt != nil
			},
			description: "DeletedAt should be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vn := &v1alpha1.VPSieNode{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-vn",
					Namespace: "default",
				},
			}

			SetPhase(vn, tt.phase, "TestReason", "Test message")

			assert.Equal(t, tt.phase, vn.Status.Phase)
			assert.True(t, tt.checkField(vn), tt.description)
		})
	}
}

func TestSetPhase_DoesNotOverwriteExistingTimestamps(t *testing.T) {
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
	}

	// Set Provisioning phase, which sets CreatedAt
	SetPhase(vn, v1alpha1.VPSieNodePhaseProvisioning, "TestReason", "Test message")
	firstCreatedAt := vn.Status.CreatedAt

	// Set Provisioning phase again
	SetPhase(vn, v1alpha1.VPSieNodePhaseProvisioning, "TestReason", "Test message")
	secondCreatedAt := vn.Status.CreatedAt

	// CreatedAt should not be overwritten
	assert.Equal(t, firstCreatedAt, secondCreatedAt)
}

func TestNewStateMachine(t *testing.T) {
	// Create a mock provisioner, joiner, drainer, and terminator
	provisioner := &Provisioner{}
	joiner := &Joiner{}
	terminator := &Terminator{}

	sm := NewStateMachine(provisioner, joiner, terminator)

	assert.NotNil(t, sm)
	assert.NotNil(t, sm.handlers)

	// Verify all phases have handlers
	expectedPhases := []v1alpha1.VPSieNodePhase{
		v1alpha1.VPSieNodePhasePending,
		v1alpha1.VPSieNodePhaseProvisioning,
		v1alpha1.VPSieNodePhaseProvisioned,
		v1alpha1.VPSieNodePhaseJoining,
		v1alpha1.VPSieNodePhaseReady,
		v1alpha1.VPSieNodePhaseTerminating,
		v1alpha1.VPSieNodePhaseDeleting,
		v1alpha1.VPSieNodePhaseFailed,
	}

	for _, phase := range expectedPhases {
		handler, exists := sm.handlers[phase]
		assert.True(t, exists, "Handler should exist for phase %s", phase)
		assert.NotNil(t, handler, "Handler should not be nil for phase %s", phase)
	}
}

func TestMetav1Now(t *testing.T) {
	now := metav1Now()
	assert.NotNil(t, now)
	assert.False(t, now.IsZero())
}
