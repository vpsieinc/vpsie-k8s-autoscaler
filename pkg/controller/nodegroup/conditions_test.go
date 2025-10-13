package nodegroup

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

func TestSetCondition(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
	}

	// Set a new condition
	SetCondition(ng, v1alpha1.NodeGroupReady, corev1.ConditionTrue, ReasonReconciling, "Test message")

	assert.Len(t, ng.Status.Conditions, 1)
	assert.Equal(t, v1alpha1.NodeGroupReady, ng.Status.Conditions[0].Type)
	assert.Equal(t, corev1.ConditionTrue, ng.Status.Conditions[0].Status)
	assert.Equal(t, ReasonReconciling, ng.Status.Conditions[0].Reason)
	assert.Equal(t, "Test message", ng.Status.Conditions[0].Message)

	// Update existing condition
	SetCondition(ng, v1alpha1.NodeGroupReady, corev1.ConditionFalse, ReasonValidationFailed, "Updated message")

	assert.Len(t, ng.Status.Conditions, 1)
	assert.Equal(t, corev1.ConditionFalse, ng.Status.Conditions[0].Status)
	assert.Equal(t, ReasonValidationFailed, ng.Status.Conditions[0].Reason)
	assert.Equal(t, "Updated message", ng.Status.Conditions[0].Message)

	// Add different condition
	SetCondition(ng, v1alpha1.NodeGroupScaling, corev1.ConditionTrue, ReasonScalingUp, "Scaling")

	assert.Len(t, ng.Status.Conditions, 2)
}

func TestRemoveCondition(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
	}

	SetCondition(ng, v1alpha1.NodeGroupReady, corev1.ConditionTrue, ReasonReconciling, "Ready")
	SetCondition(ng, v1alpha1.NodeGroupScaling, corev1.ConditionTrue, ReasonScalingUp, "Scaling")

	assert.Len(t, ng.Status.Conditions, 2)

	RemoveCondition(ng, v1alpha1.NodeGroupScaling)

	assert.Len(t, ng.Status.Conditions, 1)
	assert.Equal(t, v1alpha1.NodeGroupReady, ng.Status.Conditions[0].Type)
}

func TestGetCondition(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
	}

	// Non-existent condition
	cond := GetCondition(ng, v1alpha1.NodeGroupReady)
	assert.Nil(t, cond)

	// Existing condition
	SetCondition(ng, v1alpha1.NodeGroupReady, corev1.ConditionTrue, ReasonReconciling, "Ready")
	cond = GetCondition(ng, v1alpha1.NodeGroupReady)

	assert.NotNil(t, cond)
	assert.Equal(t, v1alpha1.NodeGroupReady, cond.Type)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
}

func TestIsConditionTrue(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
	}

	// Non-existent condition
	assert.False(t, IsConditionTrue(ng, v1alpha1.NodeGroupReady))

	// Condition set to False
	SetCondition(ng, v1alpha1.NodeGroupReady, corev1.ConditionFalse, ReasonReconciling, "Not ready")
	assert.False(t, IsConditionTrue(ng, v1alpha1.NodeGroupReady))

	// Condition set to True
	SetCondition(ng, v1alpha1.NodeGroupReady, corev1.ConditionTrue, ReasonReconciling, "Ready")
	assert.True(t, IsConditionTrue(ng, v1alpha1.NodeGroupReady))
}

func TestIsConditionFalse(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
	}

	// Non-existent condition
	assert.False(t, IsConditionFalse(ng, v1alpha1.NodeGroupReady))

	// Condition set to True
	SetCondition(ng, v1alpha1.NodeGroupReady, corev1.ConditionTrue, ReasonReconciling, "Ready")
	assert.False(t, IsConditionFalse(ng, v1alpha1.NodeGroupReady))

	// Condition set to False
	SetCondition(ng, v1alpha1.NodeGroupReady, corev1.ConditionFalse, ReasonReconciling, "Not ready")
	assert.True(t, IsConditionFalse(ng, v1alpha1.NodeGroupReady))
}

func TestSetReadyCondition(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
	}

	SetReadyCondition(ng, true, ReasonReconciling, "Ready")
	assert.True(t, IsConditionTrue(ng, v1alpha1.NodeGroupReady))

	SetReadyCondition(ng, false, ReasonReconciling, "Not ready")
	assert.True(t, IsConditionFalse(ng, v1alpha1.NodeGroupReady))
}

func TestSetScalingCondition(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
	}

	SetScalingCondition(ng, true, ReasonScalingUp, "Scaling up")
	assert.True(t, IsConditionTrue(ng, v1alpha1.NodeGroupScaling))

	SetScalingCondition(ng, false, ReasonScalingComplete, "Scaling complete")
	assert.True(t, IsConditionFalse(ng, v1alpha1.NodeGroupScaling))
}

func TestSetErrorCondition(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
	}

	SetErrorCondition(ng, true, ReasonVPSieAPIError, "API error")
	assert.True(t, IsConditionTrue(ng, v1alpha1.NodeGroupError))

	SetErrorCondition(ng, false, ReasonReconciling, "No error")
	assert.True(t, IsConditionFalse(ng, v1alpha1.NodeGroupError))
}

func TestUpdateConditionsForReconcile(t *testing.T) {
	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ng",
			Namespace: "default",
		},
		Spec: v1alpha1.NodeGroupSpec{
			MinNodes: 2,
			MaxNodes: 10,
		},
		Status: v1alpha1.NodeGroupStatus{
			CurrentNodes: 2,
		},
	}

	UpdateConditionsForReconcile(ng)

	// Should clear error condition
	assert.True(t, IsConditionFalse(ng, v1alpha1.NodeGroupError))

	// Should set AtMinCapacity
	assert.True(t, IsConditionTrue(ng, v1alpha1.NodeGroupAtMinCapacity))
	assert.False(t, IsConditionTrue(ng, v1alpha1.NodeGroupAtMaxCapacity))
}

func TestUpdateConditionsAfterScale(t *testing.T) {
	tests := []struct {
		name           string
		currentNodes   int32
		desiredNodes   int32
		readyNodes     int32
		scaleDirection string
		expectScaling  bool
		expectReady    bool
	}{
		{
			name:           "scaling up in progress",
			currentNodes:   2,
			desiredNodes:   5,
			readyNodes:     2,
			scaleDirection: "up",
			expectScaling:  true,
			expectReady:    false,
		},
		{
			name:           "scaling complete and ready",
			currentNodes:   5,
			desiredNodes:   5,
			readyNodes:     5,
			scaleDirection: "",
			expectScaling:  false,
			expectReady:    true,
		},
		{
			name:           "scaling complete but not all ready",
			currentNodes:   5,
			desiredNodes:   5,
			readyNodes:     3,
			scaleDirection: "",
			expectScaling:  false,
			expectReady:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ng := &v1alpha1.NodeGroup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ng",
					Namespace: "default",
				},
				Spec: v1alpha1.NodeGroupSpec{
					MinNodes: 2,
					MaxNodes: 10,
				},
				Status: v1alpha1.NodeGroupStatus{
					CurrentNodes: tt.currentNodes,
					DesiredNodes: tt.desiredNodes,
					ReadyNodes:   tt.readyNodes,
				},
			}

			UpdateConditionsAfterScale(ng, tt.scaleDirection)

			if tt.expectScaling {
				assert.True(t, IsConditionTrue(ng, v1alpha1.NodeGroupScaling), "Expected scaling condition to be true")
			} else {
				assert.True(t, IsConditionFalse(ng, v1alpha1.NodeGroupScaling), "Expected scaling condition to be false")
			}

			if tt.expectReady {
				assert.True(t, IsConditionTrue(ng, v1alpha1.NodeGroupReady), "Expected ready condition to be true")
			} else {
				assert.True(t, IsConditionFalse(ng, v1alpha1.NodeGroupReady), "Expected ready condition to be false")
			}
		})
	}
}
