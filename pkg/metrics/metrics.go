package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// Namespace is the metrics namespace for the autoscaler
	Namespace = "vpsie_autoscaler"
)

var (
	// NodeGroupDesiredNodes tracks the desired number of nodes in a NodeGroup
	NodeGroupDesiredNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "nodegroup_desired_nodes",
			Help:      "Desired number of nodes in a NodeGroup",
		},
		[]string{"nodegroup", "namespace"},
	)

	// NodeGroupCurrentNodes tracks the current number of nodes in a NodeGroup
	NodeGroupCurrentNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "nodegroup_current_nodes",
			Help:      "Current number of nodes in a NodeGroup",
		},
		[]string{"nodegroup", "namespace"},
	)

	// NodeGroupReadyNodes tracks the number of ready nodes in a NodeGroup
	NodeGroupReadyNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "nodegroup_ready_nodes",
			Help:      "Number of ready nodes in a NodeGroup",
		},
		[]string{"nodegroup", "namespace"},
	)

	// NodeGroupMinNodes tracks the minimum nodes configuration
	NodeGroupMinNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "nodegroup_min_nodes",
			Help:      "Minimum number of nodes configured for a NodeGroup",
		},
		[]string{"nodegroup", "namespace"},
	)

	// NodeGroupMaxNodes tracks the maximum nodes configuration
	NodeGroupMaxNodes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "nodegroup_max_nodes",
			Help:      "Maximum number of nodes configured for a NodeGroup",
		},
		[]string{"nodegroup", "namespace"},
	)

	// VPSieNodePhase tracks the number of VPSieNodes in each phase
	VPSieNodePhase = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "vpsienode_phase",
			Help:      "Number of VPSieNodes in each phase",
		},
		[]string{"phase", "nodegroup", "namespace"},
	)

	// ControllerReconcileDuration tracks the time taken by controller reconciliation
	ControllerReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "controller_reconcile_duration_seconds",
			Help:      "Time taken by controller reconciliation",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to 16s
		},
		[]string{"controller"},
	)

	// ControllerReconcileErrors tracks the number of reconciliation errors
	ControllerReconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "controller_reconcile_errors_total",
			Help:      "Total number of controller reconciliation errors",
		},
		[]string{"controller", "error_type"},
	)

	// ControllerReconcileTotal tracks the total number of reconciliations
	ControllerReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "controller_reconcile_total",
			Help:      "Total number of controller reconciliations",
		},
		[]string{"controller", "result"},
	)

	// VPSieAPIRequests tracks the number of VPSie API requests
	VPSieAPIRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_requests_total",
			Help:      "Total number of VPSie API requests",
		},
		[]string{"method", "status"},
	)

	// VPSieAPIRequestDuration tracks the duration of VPSie API requests
	VPSieAPIRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_request_duration_seconds",
			Help:      "Duration of VPSie API requests",
			Buckets:   prometheus.ExponentialBuckets(0.01, 2, 12), // 10ms to 40s
		},
		[]string{"method"},
	)

	// VPSieAPIErrors tracks API errors by type
	VPSieAPIErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_errors_total",
			Help:      "Total number of VPSie API errors by type",
		},
		[]string{"method", "error_type"},
	)

	// VPSieAPIRateLimitedTotal tracks the number of times API requests were rate limited
	VPSieAPIRateLimitedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_rate_limited_total",
			Help:      "Total number of times VPSie API requests were rate limited",
		},
		[]string{"method"},
	)

	// VPSieAPIRateLimitWaitDuration tracks the time spent waiting for rate limiter
	VPSieAPIRateLimitWaitDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_rate_limit_wait_duration_seconds",
			Help:      "Time spent waiting for VPSie API rate limiter",
			Buckets:   prometheus.ExponentialBuckets(0.001, 2, 12), // 1ms to 4s
		},
		[]string{"method"},
	)

	// ScaleUpTotal tracks the number of scale-up operations
	ScaleUpTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "scale_up_total",
			Help:      "Total number of scale-up operations",
		},
		[]string{"nodegroup", "namespace"},
	)

	// ScaleDownTotal tracks the number of scale-down operations
	ScaleDownTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "scale_down_total",
			Help:      "Total number of scale-down operations",
		},
		[]string{"nodegroup", "namespace"},
	)

	// ScaleUpNodesAdded tracks the number of nodes added in scale-up operations
	ScaleUpNodesAdded = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "scale_up_nodes_added",
			Help:      "Number of nodes added in scale-up operations",
			Buckets:   prometheus.LinearBuckets(1, 1, 10), // 1 to 10 nodes
		},
		[]string{"nodegroup", "namespace"},
	)

	// ScaleDownNodesRemoved tracks the number of nodes removed in scale-down operations
	ScaleDownNodesRemoved = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "scale_down_nodes_removed",
			Help:      "Number of nodes removed in scale-down operations",
			Buckets:   prometheus.LinearBuckets(1, 1, 10), // 1 to 10 nodes
		},
		[]string{"nodegroup", "namespace"},
	)

	// ScaleDownErrorsTotal tracks the number of scale-down errors by type
	ScaleDownErrorsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "scale_down_errors_total",
			Help:      "Total number of scale-down errors by type",
		},
		[]string{"nodegroup", "namespace", "error_type"},
	)

	// UnschedulablePodsTotal tracks the number of unschedulable pods detected
	UnschedulablePodsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "unschedulable_pods_total",
			Help:      "Total number of unschedulable pods detected",
		},
		[]string{"constraint", "namespace"},
	)

	// PendingPodsGauge tracks the current number of pending unschedulable pods
	PendingPodsGauge = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "pending_pods_current",
			Help:      "Current number of pending unschedulable pods",
		},
		[]string{"namespace"},
	)

	// NodeProvisioningDuration tracks the time taken to provision a node
	NodeProvisioningDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "node_provisioning_duration_seconds",
			Help:      "Time taken to provision a node from pending to ready",
			Buckets:   prometheus.ExponentialBuckets(10, 2, 10), // 10s to ~170 minutes
		},
		[]string{"nodegroup", "namespace"},
	)

	// NodeTerminationDuration tracks the time taken to terminate a node
	NodeTerminationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "node_termination_duration_seconds",
			Help:      "Time taken to terminate a node",
			Buckets:   prometheus.ExponentialBuckets(1, 2, 12), // 1s to ~68 minutes
		},
		[]string{"nodegroup", "namespace"},
	)

	// VPSieNodePhaseTransitions tracks phase transitions
	VPSieNodePhaseTransitions = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "vpsienode_phase_transitions_total",
			Help:      "Total number of VPSieNode phase transitions",
		},
		[]string{"from_phase", "to_phase", "nodegroup", "namespace"},
	)

	// EventsEmitted tracks Kubernetes events emitted by the controller
	EventsEmitted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "events_emitted_total",
			Help:      "Total number of Kubernetes events emitted",
		},
		[]string{"event_type", "reason", "object_kind"},
	)
)

// RegisterMetrics registers all metrics with the controller-runtime metrics registry
func RegisterMetrics() {
	metrics.Registry.MustRegister(
		NodeGroupDesiredNodes,
		NodeGroupCurrentNodes,
		NodeGroupReadyNodes,
		NodeGroupMinNodes,
		NodeGroupMaxNodes,
		VPSieNodePhase,
		ControllerReconcileDuration,
		ControllerReconcileErrors,
		ControllerReconcileTotal,
		VPSieAPIRequests,
		VPSieAPIRequestDuration,
		VPSieAPIErrors,
		VPSieAPIRateLimitedTotal,
		VPSieAPIRateLimitWaitDuration,
		ScaleUpTotal,
		ScaleDownTotal,
		ScaleUpNodesAdded,
		ScaleDownNodesRemoved,
		ScaleDownErrorsTotal,
		UnschedulablePodsTotal,
		PendingPodsGauge,
		NodeProvisioningDuration,
		NodeTerminationDuration,
		VPSieNodePhaseTransitions,
		EventsEmitted,
	)
}

// ResetMetrics resets all metrics (useful for testing)
func ResetMetrics() {
	NodeGroupDesiredNodes.Reset()
	NodeGroupCurrentNodes.Reset()
	NodeGroupReadyNodes.Reset()
	NodeGroupMinNodes.Reset()
	NodeGroupMaxNodes.Reset()
	VPSieNodePhase.Reset()
	ControllerReconcileDuration.Reset()
	ControllerReconcileErrors.Reset()
	ControllerReconcileTotal.Reset()
	VPSieAPIRequests.Reset()
	VPSieAPIRequestDuration.Reset()
	VPSieAPIErrors.Reset()
	VPSieAPIRateLimitedTotal.Reset()
	VPSieAPIRateLimitWaitDuration.Reset()
	ScaleUpTotal.Reset()
	ScaleDownTotal.Reset()
	ScaleUpNodesAdded.Reset()
	ScaleDownNodesRemoved.Reset()
	ScaleDownErrorsTotal.Reset()
	UnschedulablePodsTotal.Reset()
	PendingPodsGauge.Reset()
	NodeProvisioningDuration.Reset()
	NodeTerminationDuration.Reset()
	VPSieNodePhaseTransitions.Reset()
	EventsEmitted.Reset()
}
