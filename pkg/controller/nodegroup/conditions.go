package nodegroup

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// ConditionReason represents reasons for condition transitions
const (
	// ReasonReconciling indicates the NodeGroup is being reconciled
	ReasonReconciling = "Reconciling"

	// ReasonValidationFailed indicates the NodeGroup spec validation failed
	ReasonValidationFailed = "ValidationFailed"

	// ReasonScalingUp indicates the NodeGroup is scaling up
	ReasonScalingUp = "ScalingUp"

	// ReasonScalingDown indicates the NodeGroup is scaling down
	ReasonScalingDown = "ScalingDown"

	// ReasonScalingComplete indicates scaling operation completed
	ReasonScalingComplete = "ScalingComplete"

	// ReasonMinCapacity indicates the NodeGroup is at minimum capacity
	ReasonMinCapacity = "AtMinimumCapacity"

	// ReasonMaxCapacity indicates the NodeGroup is at maximum capacity
	ReasonMaximumCapacity = "AtMaximumCapacity"

	// ReasonVPSieAPIError indicates an error communicating with VPSie API
	ReasonVPSieAPIError = "VPSieAPIError"

	// ReasonKubernetesAPIError indicates an error communicating with Kubernetes API
	ReasonKubernetesAPIError = "KubernetesAPIError"

	// ReasonNodeProvisioningFailed indicates node provisioning failed
	ReasonNodeProvisioningFailed = "NodeProvisioningFailed"

	// ReasonScaleDownFailed indicates scale-down operation failed
	ReasonScaleDownFailed = "ScaleDownFailed"
)

// SetCondition sets a condition on the NodeGroup status
func SetCondition(ng *v1alpha1.NodeGroup, conditionType v1alpha1.NodeGroupConditionType, status corev1.ConditionStatus, reason, message string) {
	now := metav1.Now()

	// Find existing condition
	var existingCondition *v1alpha1.NodeGroupCondition
	for i := range ng.Status.Conditions {
		if ng.Status.Conditions[i].Type == conditionType {
			existingCondition = &ng.Status.Conditions[i]
			break
		}
	}

	if existingCondition != nil {
		// Update existing condition
		if existingCondition.Status != status {
			existingCondition.LastTransitionTime = now
		}
		existingCondition.Status = status
		existingCondition.Reason = reason
		existingCondition.Message = message
		existingCondition.LastUpdateTime = now
	} else {
		// Add new condition
		newCondition := v1alpha1.NodeGroupCondition{
			Type:               conditionType,
			Status:             status,
			LastTransitionTime: now,
			LastUpdateTime:     now,
			Reason:             reason,
			Message:            message,
		}
		ng.Status.Conditions = append(ng.Status.Conditions, newCondition)
	}
}

// RemoveCondition removes a condition from the NodeGroup status
func RemoveCondition(ng *v1alpha1.NodeGroup, conditionType v1alpha1.NodeGroupConditionType) {
	var newConditions []v1alpha1.NodeGroupCondition
	for _, condition := range ng.Status.Conditions {
		if condition.Type != conditionType {
			newConditions = append(newConditions, condition)
		}
	}
	ng.Status.Conditions = newConditions
}

// GetCondition returns a condition from the NodeGroup status
func GetCondition(ng *v1alpha1.NodeGroup, conditionType v1alpha1.NodeGroupConditionType) *v1alpha1.NodeGroupCondition {
	for i := range ng.Status.Conditions {
		if ng.Status.Conditions[i].Type == conditionType {
			return &ng.Status.Conditions[i]
		}
	}
	return nil
}

// IsConditionTrue checks if a condition is set to True
func IsConditionTrue(ng *v1alpha1.NodeGroup, conditionType v1alpha1.NodeGroupConditionType) bool {
	condition := GetCondition(ng, conditionType)
	return condition != nil && condition.Status == corev1.ConditionTrue
}

// IsConditionFalse checks if a condition is set to False
func IsConditionFalse(ng *v1alpha1.NodeGroup, conditionType v1alpha1.NodeGroupConditionType) bool {
	condition := GetCondition(ng, conditionType)
	return condition != nil && condition.Status == corev1.ConditionFalse
}

// SetReadyCondition sets the Ready condition based on the current state
func SetReadyCondition(ng *v1alpha1.NodeGroup, ready bool, reason, message string) {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	SetCondition(ng, v1alpha1.NodeGroupReady, status, reason, message)
}

// SetScalingCondition sets the Scaling condition
func SetScalingCondition(ng *v1alpha1.NodeGroup, scaling bool, reason, message string) {
	status := corev1.ConditionFalse
	if scaling {
		status = corev1.ConditionTrue
	}
	SetCondition(ng, v1alpha1.NodeGroupScaling, status, reason, message)
}

// SetErrorCondition sets the Error condition
func SetErrorCondition(ng *v1alpha1.NodeGroup, hasError bool, reason, message string) {
	status := corev1.ConditionFalse
	if hasError {
		status = corev1.ConditionTrue
	}
	SetCondition(ng, v1alpha1.NodeGroupError, status, reason, message)
}

// SetAtMinCapacityCondition sets the AtMinCapacity condition
func SetAtMinCapacityCondition(ng *v1alpha1.NodeGroup, atMin bool, message string) {
	status := corev1.ConditionFalse
	if atMin {
		status = corev1.ConditionTrue
	}
	SetCondition(ng, v1alpha1.NodeGroupAtMinCapacity, status, ReasonMinCapacity, message)
}

// SetAtMaxCapacityCondition sets the AtMaxCapacity condition
func SetAtMaxCapacityCondition(ng *v1alpha1.NodeGroup, atMax bool, message string) {
	status := corev1.ConditionFalse
	if atMax {
		status = corev1.ConditionTrue
	}
	SetCondition(ng, v1alpha1.NodeGroupAtMaxCapacity, status, ReasonMaximumCapacity, message)
}

// UpdateConditionsForReconcile updates conditions at the start of reconciliation
func UpdateConditionsForReconcile(ng *v1alpha1.NodeGroup) {
	// Clear error condition at the start of reconciliation
	SetErrorCondition(ng, false, ReasonReconciling, "Reconciliation in progress")

	// Update capacity conditions
	atMin := ng.Status.CurrentNodes <= ng.Spec.MinNodes
	atMax := ng.Status.CurrentNodes >= ng.Spec.MaxNodes

	SetAtMinCapacityCondition(ng, atMin, "")
	SetAtMaxCapacityCondition(ng, atMax, "")
}

// UpdateConditionsAfterScale updates conditions after a scaling operation
func UpdateConditionsAfterScale(ng *v1alpha1.NodeGroup, scaleDirection string) {
	// Update scaling condition
	scaling := ng.Status.CurrentNodes != ng.Status.DesiredNodes
	if scaling {
		message := ""
		if scaleDirection == "up" {
			message = "Scaling up nodes"
		} else if scaleDirection == "down" {
			message = "Scaling down nodes"
		}
		SetScalingCondition(ng, true, ReasonReconciling, message)
	} else {
		SetScalingCondition(ng, false, ReasonScalingComplete, "Desired node count reached")
	}

	// Update ready condition
	ready := ng.Status.ReadyNodes == ng.Status.DesiredNodes && !scaling
	if ready {
		SetReadyCondition(ng, true, ReasonScalingComplete, "All nodes are ready")
	} else {
		SetReadyCondition(ng, false, ReasonReconciling, "Waiting for nodes to be ready")
	}

	// Update capacity conditions
	atMin := ng.Status.CurrentNodes <= ng.Spec.MinNodes
	atMax := ng.Status.CurrentNodes >= ng.Spec.MaxNodes

	SetAtMinCapacityCondition(ng, atMin, "")
	SetAtMaxCapacityCondition(ng, atMax, "")
}
