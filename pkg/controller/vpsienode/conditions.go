package vpsienode

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// Condition reasons for VPSieNode
const (
	// ReasonProvisioning indicates VPS is being provisioned
	ReasonProvisioning = "Provisioning"

	// ReasonProvisioned indicates VPS is provisioned and running
	ReasonProvisioned = "Provisioned"

	// ReasonJoining indicates node is joining the cluster
	ReasonJoining = "Joining"

	// ReasonJoined indicates node has joined the cluster
	ReasonJoined = "Joined"

	// ReasonReady indicates node is ready to accept workloads
	ReasonReady = "Ready"

	// ReasonTerminating indicates node is being terminated
	ReasonTerminating = "Terminating"

	// ReasonDeleting indicates VPS is being deleted
	ReasonDeleting = "Deleting"

	// ReasonFailed indicates an error occurred
	ReasonFailed = "Failed"

	// ReasonVPSieAPIError indicates an error calling VPSie API
	ReasonVPSieAPIError = "VPSieAPIError"

	// ReasonProvisioningTimeout indicates provisioning took too long
	ReasonProvisioningTimeout = "ProvisioningTimeout"

	// ReasonCapacityLimitReached indicates cluster capacity limit was reached
	// This is a terminal error that won't be resolved by retrying
	ReasonCapacityLimitReached = "CapacityLimitReached"

	// ReasonJoiningTimeout indicates node joining took too long
	ReasonJoiningTimeout = "JoiningTimeout"

	// ReasonNodeNotFound indicates Kubernetes node was not found
	ReasonNodeNotFound = "NodeNotFound"

	// ReasonDrainFailed indicates node drain failed
	ReasonDrainFailed = "DrainFailed"

	// ReasonNodeDeleteFailed indicates Kubernetes Node deletion failed
	ReasonNodeDeleteFailed = "NodeDeleteFailed"

	// ReasonVPSDeleteFailed indicates VPS deletion failed
	ReasonVPSDeleteFailed = "VPSDeleteFailed"

	// ReasonTTLExpired indicates the VPSieNode was deleted due to TTL expiration
	ReasonTTLExpired = "TTLExpired"
)

// SetCondition sets or updates a condition on the VPSieNode
func SetCondition(vn *v1alpha1.VPSieNode, condType v1alpha1.VPSieNodeConditionType, status corev1.ConditionStatus, reason, message string) {
	now := metav1.Now()

	// Find existing condition
	for i := range vn.Status.Conditions {
		if vn.Status.Conditions[i].Type == condType {
			cond := &vn.Status.Conditions[i]

			// Update the condition
			oldStatus := cond.Status
			cond.Status = string(status)
			cond.Reason = reason
			cond.Message = message
			cond.LastUpdateTime = now

			// Only update transition time if status actually changed
			if oldStatus != string(status) {
				cond.LastTransitionTime = now
			}
			return
		}
	}

	// Condition doesn't exist, add it
	vn.Status.Conditions = append(vn.Status.Conditions, v1alpha1.VPSieNodeCondition{
		Type:               condType,
		Status:             string(status),
		LastTransitionTime: now,
		LastUpdateTime:     now,
		Reason:             reason,
		Message:            message,
	})
}

// GetCondition returns the condition with the specified type
func GetCondition(vn *v1alpha1.VPSieNode, condType v1alpha1.VPSieNodeConditionType) *v1alpha1.VPSieNodeCondition {
	for i := range vn.Status.Conditions {
		if vn.Status.Conditions[i].Type == condType {
			return &vn.Status.Conditions[i]
		}
	}
	return nil
}

// IsConditionTrue returns true if the condition is present and set to True
func IsConditionTrue(vn *v1alpha1.VPSieNode, condType v1alpha1.VPSieNodeConditionType) bool {
	cond := GetCondition(vn, condType)
	return cond != nil && cond.Status == string(corev1.ConditionTrue)
}

// IsConditionFalse returns true if the condition is present and set to False
func IsConditionFalse(vn *v1alpha1.VPSieNode, condType v1alpha1.VPSieNodeConditionType) bool {
	cond := GetCondition(vn, condType)
	return cond != nil && cond.Status == string(corev1.ConditionFalse)
}

// RemoveCondition removes a condition from the VPSieNode
func RemoveCondition(vn *v1alpha1.VPSieNode, condType v1alpha1.VPSieNodeConditionType) {
	var newConditions []v1alpha1.VPSieNodeCondition
	for i := range vn.Status.Conditions {
		if vn.Status.Conditions[i].Type != condType {
			newConditions = append(newConditions, vn.Status.Conditions[i])
		}
	}
	vn.Status.Conditions = newConditions
}

// SetVPSReadyCondition sets the VPSReady condition
func SetVPSReadyCondition(vn *v1alpha1.VPSieNode, ready bool, reason, message string) {
	status := corev1.ConditionTrue
	if !ready {
		status = corev1.ConditionFalse
	}
	SetCondition(vn, v1alpha1.VPSieNodeConditionVPSReady, status, reason, message)
}

// SetNodeJoinedCondition sets the NodeJoined condition
func SetNodeJoinedCondition(vn *v1alpha1.VPSieNode, joined bool, reason, message string) {
	status := corev1.ConditionTrue
	if !joined {
		status = corev1.ConditionFalse
	}
	SetCondition(vn, v1alpha1.VPSieNodeConditionNodeJoined, status, reason, message)
}

// SetNodeReadyCondition sets the NodeReady condition
func SetNodeReadyCondition(vn *v1alpha1.VPSieNode, ready bool, reason, message string) {
	status := corev1.ConditionTrue
	if !ready {
		status = corev1.ConditionFalse
	}
	SetCondition(vn, v1alpha1.VPSieNodeConditionNodeReady, status, reason, message)
}

// SetErrorCondition sets the Error condition
func SetErrorCondition(vn *v1alpha1.VPSieNode, hasError bool, reason, message string) {
	status := corev1.ConditionTrue
	if !hasError {
		status = corev1.ConditionFalse
	}
	SetCondition(vn, v1alpha1.VPSieNodeConditionError, status, reason, message)
}

// ClearError clears the error condition and last error message
func ClearError(vn *v1alpha1.VPSieNode) {
	SetErrorCondition(vn, false, "", "")
	vn.Status.LastError = ""
}

// RecordError records an error in the status
func RecordError(vn *v1alpha1.VPSieNode, reason, message string) {
	SetErrorCondition(vn, true, reason, message)
	vn.Status.LastError = message
}
