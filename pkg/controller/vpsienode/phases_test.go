package vpsienode

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

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
		// Note: DeletedAt is NOT set when entering Deleting phase
		// It's set by DeleteVPS() after successful VPS deletion
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

	// Create a fake client
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	sm := NewStateMachine(provisioner, joiner, terminator, 30*time.Minute, fakeClient)

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

func TestFailedPhaseHandler_TTLDisabled(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	// TTL disabled (0)
	handler := &FailedPhaseHandler{
		ttl:    0,
		client: fakeClient,
	}

	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseFailed,
		},
	}

	result, err := handler.Handle(context.Background(), vn, logger)

	require.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Zero(t, result.RequeueAfter, "Should not requeue when TTL is disabled")
}

func TestFailedPhaseHandler_TTLNotExpired(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	// Create VPSieNode that just entered Failed state
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseFailed,
			Conditions: []v1alpha1.VPSieNodeCondition{
				{
					Type:               v1alpha1.VPSieNodeConditionError,
					Status:             string(corev1.ConditionTrue),
					LastTransitionTime: metav1.Now(), // Just now
					Reason:             "TestError",
					Message:            "Test error",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vn).
		Build()

	// TTL of 30 minutes - node just entered Failed state, so TTL should not be expired
	handler := &FailedPhaseHandler{
		ttl:    30 * time.Minute,
		client: fakeClient,
	}

	result, err := handler.Handle(context.Background(), vn, logger)

	require.NoError(t, err)
	assert.True(t, result.RequeueAfter > 0, "Should requeue when TTL not expired")
	assert.True(t, result.RequeueAfter <= 30*time.Minute, "RequeueAfter should be <= TTL")
}

func TestFailedPhaseHandler_TTLExpired(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	// Create VPSieNode that has been in Failed state for longer than TTL
	failedTime := metav1.NewTime(time.Now().Add(-1 * time.Hour)) // 1 hour ago
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vn",
			Namespace: "default",
			Labels: map[string]string{
				"autoscaler.vpsie.com/nodegroup": "test-ng",
			},
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase:     v1alpha1.VPSieNodePhaseFailed,
			LastError: "Provisioning timeout",
			Conditions: []v1alpha1.VPSieNodeCondition{
				{
					Type:               v1alpha1.VPSieNodeConditionError,
					Status:             string(corev1.ConditionTrue),
					LastTransitionTime: failedTime,
					Reason:             "ProvisioningTimeout",
					Message:            "Provisioning timeout exceeded",
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vn).
		Build()

	// TTL of 30 minutes - node has been in Failed state for 1 hour
	handler := &FailedPhaseHandler{
		ttl:    30 * time.Minute,
		client: fakeClient,
	}

	result, err := handler.Handle(context.Background(), vn, logger)

	require.NoError(t, err)
	assert.False(t, result.Requeue)
	assert.Zero(t, result.RequeueAfter, "Should not requeue after deletion")

	// Verify the VPSieNode was deleted by trying to get it
	var deletedVn v1alpha1.VPSieNode
	getErr := fakeClient.Get(context.Background(), types.NamespacedName{
		Name:      vn.Name,
		Namespace: vn.Namespace,
	}, &deletedVn)
	assert.Error(t, getErr, "VPSieNode should be deleted")
}

func TestFailedPhaseHandler_FallbackToCreationTimestamp(t *testing.T) {
	logger := zaptest.NewLogger(t)
	scheme := runtime.NewScheme()
	_ = v1alpha1.AddToScheme(scheme)

	// Create VPSieNode without Error condition - should fall back to CreationTimestamp
	creationTime := metav1.NewTime(time.Now().Add(-2 * time.Hour)) // 2 hours ago
	vn := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-vn",
			Namespace:         "default",
			CreationTimestamp: creationTime,
		},
		Status: v1alpha1.VPSieNodeStatus{
			Phase: v1alpha1.VPSieNodePhaseFailed,
			// No Error condition
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(vn).
		Build()

	// TTL of 30 minutes - node was created 2 hours ago
	handler := &FailedPhaseHandler{
		ttl:    30 * time.Minute,
		client: fakeClient,
	}

	result, err := handler.Handle(context.Background(), vn, logger)

	require.NoError(t, err)
	// Should have triggered deletion since 2h > 30m TTL
	assert.False(t, result.Requeue)
	assert.Zero(t, result.RequeueAfter)
}
