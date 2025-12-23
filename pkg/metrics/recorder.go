package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// RecordNodeGroupMetrics records all metrics for a NodeGroup
func RecordNodeGroupMetrics(ng *v1alpha1.NodeGroup) {
	nodegroup, _ := SanitizeLabel(ng.Name)
	namespace, _ := SanitizeLabel(ng.Namespace)

	labels := prometheus.Labels{
		"nodegroup": nodegroup,
		"namespace": namespace,
	}

	NodeGroupDesiredNodes.With(labels).Set(float64(ng.Status.DesiredNodes))
	NodeGroupCurrentNodes.With(labels).Set(float64(ng.Status.CurrentNodes))
	NodeGroupReadyNodes.With(labels).Set(float64(ng.Status.ReadyNodes))
	NodeGroupMinNodes.With(labels).Set(float64(ng.Spec.MinNodes))
	NodeGroupMaxNodes.With(labels).Set(float64(ng.Spec.MaxNodes))
}

// RecordVPSieNodePhase records the phase of a VPSieNode
func RecordVPSieNodePhase(vn *v1alpha1.VPSieNode, phase v1alpha1.VPSieNodePhase, value float64) {
	phaseStr, _ := SanitizeLabel(string(phase))
	nodegroup, _ := SanitizeLabel(vn.Spec.NodeGroupName)
	namespace, _ := SanitizeLabel(vn.Namespace)

	labels := prometheus.Labels{
		"phase":     phaseStr,
		"nodegroup": nodegroup,
		"namespace": namespace,
	}
	VPSieNodePhase.With(labels).Set(value)
}

// RecordPhaseTransition records a phase transition for a VPSieNode
func RecordPhaseTransition(vn *v1alpha1.VPSieNode, fromPhase, toPhase v1alpha1.VPSieNodePhase) {
	fromPhaseStr, _ := SanitizeLabel(string(fromPhase))
	toPhaseStr, _ := SanitizeLabel(string(toPhase))
	nodegroup, _ := SanitizeLabel(vn.Spec.NodeGroupName)
	namespace, _ := SanitizeLabel(vn.Namespace)

	labels := prometheus.Labels{
		"from_phase": fromPhaseStr,
		"to_phase":   toPhaseStr,
		"nodegroup":  nodegroup,
		"namespace":  namespace,
	}
	VPSieNodePhaseTransitions.With(labels).Inc()
}

// RecordReconcileDuration records the duration of a reconciliation
func RecordReconcileDuration(controller string, duration time.Duration) {
	controllerSan, _ := SanitizeLabel(controller)
	ControllerReconcileDuration.WithLabelValues(controllerSan).Observe(duration.Seconds())
}

// RecordReconcileError records a reconciliation error
func RecordReconcileError(controller string, errorType string) {
	controllerSan, _ := SanitizeLabel(controller)
	errorTypeSan, _ := SanitizeLabel(errorType)
	ControllerReconcileErrors.WithLabelValues(controllerSan, errorTypeSan).Inc()
}

// RecordReconcileResult records the result of a reconciliation
func RecordReconcileResult(controller string, result string) {
	controllerSan, _ := SanitizeLabel(controller)
	resultSan, _ := SanitizeLabel(result)
	ControllerReconcileTotal.WithLabelValues(controllerSan, resultSan).Inc()
}

// RecordAPIRequest records a VPSie API request
func RecordAPIRequest(method string, status string, duration time.Duration) {
	methodSan, _ := SanitizeLabel(method)
	statusSan, _ := SanitizeLabel(status)
	VPSieAPIRequests.WithLabelValues(methodSan, statusSan).Inc()
	VPSieAPIRequestDuration.WithLabelValues(methodSan).Observe(duration.Seconds())
}

// RecordAPIError records a VPSie API error
func RecordAPIError(method string, errorType string) {
	methodSan, _ := SanitizeLabel(method)
	errorTypeSan, _ := SanitizeLabel(errorType)
	VPSieAPIErrors.WithLabelValues(methodSan, errorTypeSan).Inc()
}

// RecordScaleUp records a scale-up operation
func RecordScaleUp(nodeGroup, namespace string, nodesAdded int32) {
	nodeGroupSan, _ := SanitizeLabel(nodeGroup)
	namespaceSan, _ := SanitizeLabel(namespace)

	labels := prometheus.Labels{
		"nodegroup": nodeGroupSan,
		"namespace": namespaceSan,
	}
	ScaleUpTotal.With(labels).Inc()
	ScaleUpNodesAdded.With(labels).Observe(float64(nodesAdded))
}

// RecordScaleDown records a scale-down operation
func RecordScaleDown(nodeGroup, namespace string, nodesRemoved int32) {
	nodeGroupSan, _ := SanitizeLabel(nodeGroup)
	namespaceSan, _ := SanitizeLabel(namespace)

	labels := prometheus.Labels{
		"nodegroup": nodeGroupSan,
		"namespace": namespaceSan,
	}
	ScaleDownTotal.With(labels).Inc()
	ScaleDownNodesRemoved.With(labels).Observe(float64(nodesRemoved))
}

// RecordUnschedulablePod records an unschedulable pod
func RecordUnschedulablePod(constraint, namespace string) {
	constraintSan, _ := SanitizeLabel(constraint)
	namespaceSan, _ := SanitizeLabel(namespace)
	UnschedulablePodsTotal.WithLabelValues(constraintSan, namespaceSan).Inc()
}

// RecordPendingPods records the current number of pending pods
func RecordPendingPods(namespace string, count int) {
	namespaceSan, _ := SanitizeLabel(namespace)
	PendingPodsGauge.WithLabelValues(namespaceSan).Set(float64(count))
}

// RecordNodeProvisioningDuration records the time taken to provision a node
func RecordNodeProvisioningDuration(nodeGroup, namespace string, duration time.Duration) {
	nodeGroupSan, _ := SanitizeLabel(nodeGroup)
	namespaceSan, _ := SanitizeLabel(namespace)

	labels := prometheus.Labels{
		"nodegroup": nodeGroupSan,
		"namespace": namespaceSan,
	}
	NodeProvisioningDuration.With(labels).Observe(duration.Seconds())
}

// RecordNodeTerminationDuration records the time taken to terminate a node
func RecordNodeTerminationDuration(nodeGroup, namespace string, duration time.Duration) {
	nodeGroupSan, _ := SanitizeLabel(nodeGroup)
	namespaceSan, _ := SanitizeLabel(namespace)

	labels := prometheus.Labels{
		"nodegroup": nodeGroupSan,
		"namespace": namespaceSan,
	}
	NodeTerminationDuration.With(labels).Observe(duration.Seconds())
}

// RecordEventEmitted records a Kubernetes event emission
func RecordEventEmitted(eventType, reason, objectKind string) {
	eventTypeSan, _ := SanitizeLabel(eventType)
	reasonSan, _ := SanitizeLabel(reason)
	objectKindSan, _ := SanitizeLabel(objectKind)
	EventsEmitted.WithLabelValues(eventTypeSan, reasonSan, objectKindSan).Inc()
}
