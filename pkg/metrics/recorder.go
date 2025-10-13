package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// RecordNodeGroupMetrics records all metrics for a NodeGroup
func RecordNodeGroupMetrics(ng *v1alpha1.NodeGroup) {
	labels := prometheus.Labels{
		"nodegroup": ng.Name,
		"namespace": ng.Namespace,
	}

	NodeGroupDesiredNodes.With(labels).Set(float64(ng.Status.DesiredNodes))
	NodeGroupCurrentNodes.With(labels).Set(float64(ng.Status.CurrentNodes))
	NodeGroupReadyNodes.With(labels).Set(float64(ng.Status.ReadyNodes))
	NodeGroupMinNodes.With(labels).Set(float64(ng.Spec.MinNodes))
	NodeGroupMaxNodes.With(labels).Set(float64(ng.Spec.MaxNodes))
}

// RecordVPSieNodePhase records the phase of a VPSieNode
func RecordVPSieNodePhase(vn *v1alpha1.VPSieNode, phase v1alpha1.VPSieNodePhase, value float64) {
	labels := prometheus.Labels{
		"phase":     string(phase),
		"nodegroup": vn.Spec.NodeGroupName,
		"namespace": vn.Namespace,
	}
	VPSieNodePhase.With(labels).Set(value)
}

// RecordPhaseTransition records a phase transition for a VPSieNode
func RecordPhaseTransition(vn *v1alpha1.VPSieNode, fromPhase, toPhase v1alpha1.VPSieNodePhase) {
	labels := prometheus.Labels{
		"from_phase": string(fromPhase),
		"to_phase":   string(toPhase),
		"nodegroup":  vn.Spec.NodeGroupName,
		"namespace":  vn.Namespace,
	}
	VPSieNodePhaseTransitions.With(labels).Inc()
}

// RecordReconcileDuration records the duration of a reconciliation
func RecordReconcileDuration(controller string, duration time.Duration) {
	ControllerReconcileDuration.WithLabelValues(controller).Observe(duration.Seconds())
}

// RecordReconcileError records a reconciliation error
func RecordReconcileError(controller string, errorType string) {
	ControllerReconcileErrors.WithLabelValues(controller, errorType).Inc()
}

// RecordReconcileResult records the result of a reconciliation
func RecordReconcileResult(controller string, result string) {
	ControllerReconcileTotal.WithLabelValues(controller, result).Inc()
}

// RecordAPIRequest records a VPSie API request
func RecordAPIRequest(method string, status string, duration time.Duration) {
	VPSieAPIRequests.WithLabelValues(method, status).Inc()
	VPSieAPIRequestDuration.WithLabelValues(method).Observe(duration.Seconds())
}

// RecordAPIError records a VPSie API error
func RecordAPIError(method string, errorType string) {
	VPSieAPIErrors.WithLabelValues(method, errorType).Inc()
}

// RecordScaleUp records a scale-up operation
func RecordScaleUp(nodeGroup, namespace string, nodesAdded int32) {
	labels := prometheus.Labels{
		"nodegroup": nodeGroup,
		"namespace": namespace,
	}
	ScaleUpTotal.With(labels).Inc()
	ScaleUpNodesAdded.With(labels).Observe(float64(nodesAdded))
}

// RecordScaleDown records a scale-down operation
func RecordScaleDown(nodeGroup, namespace string, nodesRemoved int32) {
	labels := prometheus.Labels{
		"nodegroup": nodeGroup,
		"namespace": namespace,
	}
	ScaleDownTotal.With(labels).Inc()
	ScaleDownNodesRemoved.With(labels).Observe(float64(nodesRemoved))
}

// RecordUnschedulablePod records an unschedulable pod
func RecordUnschedulablePod(constraint, namespace string) {
	UnschedulablePodsTotal.WithLabelValues(constraint, namespace).Inc()
}

// RecordPendingPods records the current number of pending pods
func RecordPendingPods(namespace string, count int) {
	PendingPodsGauge.WithLabelValues(namespace).Set(float64(count))
}

// RecordNodeProvisioningDuration records the time taken to provision a node
func RecordNodeProvisioningDuration(nodeGroup, namespace string, duration time.Duration) {
	labels := prometheus.Labels{
		"nodegroup": nodeGroup,
		"namespace": namespace,
	}
	NodeProvisioningDuration.With(labels).Observe(duration.Seconds())
}

// RecordNodeTerminationDuration records the time taken to terminate a node
func RecordNodeTerminationDuration(nodeGroup, namespace string, duration time.Duration) {
	labels := prometheus.Labels{
		"nodegroup": nodeGroup,
		"namespace": namespace,
	}
	NodeTerminationDuration.With(labels).Observe(duration.Seconds())
}

// RecordEventEmitted records a Kubernetes event emission
func RecordEventEmitted(eventType, reason, objectKind string) {
	EventsEmitted.WithLabelValues(eventType, reason, objectKind).Inc()
}
