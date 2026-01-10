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

	// VPSieAPICircuitBreakerState tracks the current state of the circuit breaker
	VPSieAPICircuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_circuit_breaker_state",
			Help:      "Current state of VPSie API circuit breaker (1=active, 0=inactive)",
		},
		[]string{"state"}, // closed, open, half-open
	)

	// VPSieAPICircuitBreakerOpened tracks how many times requests were blocked by open circuit
	VPSieAPICircuitBreakerOpened = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_circuit_breaker_opened_total",
			Help:      "Total number of requests blocked by open circuit breaker",
		},
		[]string{},
	)

	// VPSieAPICircuitBreakerStateChanges tracks state transitions
	VPSieAPICircuitBreakerStateChanges = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_circuit_breaker_state_changes_total",
			Help:      "Total number of circuit breaker state changes",
		},
		[]string{"from_state", "to_state"},
	)

	// VPSieAPICircuitBreakerHalfOpenAttempts tracks half-open test request attempts
	VPSieAPICircuitBreakerHalfOpenAttempts = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_circuit_breaker_half_open_attempts_total",
			Help:      "Total number of test requests attempted in half-open state",
		},
	)

	// VPSieAPICircuitBreakerHalfOpenSuccesses tracks successful half-open test requests
	VPSieAPICircuitBreakerHalfOpenSuccesses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_circuit_breaker_half_open_successes_total",
			Help:      "Total number of successful test requests in half-open state",
		},
	)

	// VPSieAPICircuitBreakerHalfOpenFailures tracks failed half-open test requests
	VPSieAPICircuitBreakerHalfOpenFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_circuit_breaker_half_open_failures_total",
			Help:      "Total number of failed test requests in half-open state",
		},
	)

	// VPSieAPICircuitBreakerHalfOpenRejected tracks requests rejected in half-open due to max concurrent limit
	VPSieAPICircuitBreakerHalfOpenRejected = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "vpsie_api_circuit_breaker_half_open_rejected_total",
			Help:      "Total number of requests rejected in half-open state due to concurrent limit",
		},
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

	// ScaleDownBlockedTotal tracks the number of scale-down operations blocked by safety checks
	ScaleDownBlockedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "scale_down_blocked_total",
			Help:      "Total number of scale-down operations blocked by safety checks",
		},
		[]string{"nodegroup", "namespace", "reason"}, // reason: pdb, affinity, capacity, cooldown
	)

	// SafetyCheckFailuresTotal tracks the number of safety check failures by type
	SafetyCheckFailuresTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "safety_check_failures_total",
			Help:      "Total number of safety check failures by type",
		},
		[]string{"check_type", "nodegroup", "namespace"}, // check_type: pdb, affinity, capacity
	)

	// NodeDrainDuration tracks the time taken to drain a node
	NodeDrainDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "node_drain_duration_seconds",
			Help:      "Time taken to drain a node during scale-down",
			Buckets:   prometheus.ExponentialBuckets(1, 2, 12), // 1s to ~68 minutes
		},
		[]string{"nodegroup", "namespace", "result"}, // result: success, timeout, error
	)

	// NodeDrainPodsEvicted tracks the number of pods evicted during node drain
	NodeDrainPodsEvicted = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "node_drain_pods_evicted",
			Help:      "Number of pods evicted during node drain",
			Buckets:   prometheus.LinearBuckets(0, 5, 20), // 0 to 95 pods
		},
		[]string{"nodegroup", "namespace"},
	)

	// Phase 2 Enhanced Metrics

	// ReconciliationQueueDepth tracks the current depth of the reconciliation queue
	ReconciliationQueueDepth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "reconciliation_queue_depth",
			Help:      "Current depth of the reconciliation queue per controller",
		},
		[]string{"controller"},
	)

	// ScalingDecisionsTotal tracks scaling decisions made by the autoscaler
	ScalingDecisionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "scaling_decisions_total",
			Help:      "Total number of scaling decisions made",
		},
		[]string{"nodegroup", "namespace", "decision", "reason"},
		// decision: scale_up, scale_down, no_action
		// reason: underutilized, pending_pods, manual, cooldown, min_nodes, max_nodes
	)

	// WebhookValidationDuration tracks the duration of webhook validation requests
	WebhookValidationDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "webhook_validation_duration_seconds",
			Help:      "Duration of webhook validation requests",
			Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 12), // 0.1ms to ~400ms
		},
		[]string{"resource", "operation", "result"},
		// resource: nodegroup, vpsienode
		// operation: create, update, delete
		// result: allowed, denied, error
	)

	// CostSavingsEstimatedMonthly tracks estimated monthly cost savings
	CostSavingsEstimatedMonthly = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "cost_savings_estimated_monthly",
			Help:      "Estimated monthly cost savings in USD from autoscaling optimizations",
		},
		[]string{"nodegroup", "namespace", "source"},
		// source: scale_down, right_sizing, rebalancing
	)

	// RebalancerOperationsTotal tracks rebalancer operations
	RebalancerOperationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "rebalancer_operations_total",
			Help:      "Total number of rebalancer operations",
		},
		[]string{"nodegroup", "namespace", "operation", "result"},
		// operation: analyze, plan, execute, rollback
		// result: success, failure, skipped
	)

	// RebalancerNodesReplacedTotal tracks nodes replaced by the rebalancer
	RebalancerNodesReplacedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "rebalancer_nodes_replaced_total",
			Help:      "Total number of nodes replaced by the rebalancer",
		},
		[]string{"nodegroup", "namespace", "strategy"},
		// strategy: rolling, surge, blue_green
	)

	// RebalancerCostSavingsTotal tracks cumulative cost savings from rebalancing
	RebalancerCostSavingsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "rebalancer_cost_savings_total",
			Help:      "Cumulative cost savings in USD from rebalancing operations",
		},
		[]string{"nodegroup", "namespace"},
	)

	// NodeUtilizationCPU tracks current CPU utilization per node
	NodeUtilizationCPU = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "node_utilization_cpu_percent",
			Help:      "Current CPU utilization percentage per node",
		},
		[]string{"node", "nodegroup", "namespace"},
	)

	// NodeUtilizationMemory tracks current memory utilization per node
	NodeUtilizationMemory = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "node_utilization_memory_percent",
			Help:      "Current memory utilization percentage per node",
		},
		[]string{"node", "nodegroup", "namespace"},
	)

	// NodeGroupCostCurrent tracks the current hourly cost of a node group
	NodeGroupCostCurrent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "nodegroup_cost_hourly",
			Help:      "Current hourly cost of a NodeGroup in USD",
		},
		[]string{"nodegroup", "namespace"},
	)

	// Phase 5 Security Metrics - Credential Rotation

	// CredentialRotationAttempts tracks the total number of credential rotation attempts
	CredentialRotationAttempts = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "credential_rotation_attempts_total",
			Help:      "Total number of credential rotation attempts",
		},
	)

	// CredentialRotationSuccesses tracks successful credential rotations
	CredentialRotationSuccesses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "credential_rotation_successes_total",
			Help:      "Total number of successful credential rotations",
		},
	)

	// CredentialRotationFailures tracks failed credential rotations
	CredentialRotationFailures = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "credential_rotation_failures_total",
			Help:      "Total number of failed credential rotations",
		},
	)

	// CredentialRotationDuration tracks the duration of credential rotation operations
	CredentialRotationDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "credential_rotation_duration_seconds",
			Help:      "Duration of credential rotation operations",
			Buckets:   prometheus.ExponentialBuckets(0.1, 2, 10), // 100ms to ~51s
		},
	)

	// CredentialExpiresAt tracks when the current access token expires (Unix timestamp)
	CredentialExpiresAt = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "credential_expires_at_timestamp",
			Help:      "Unix timestamp of when the current access token expires",
		},
	)

	// CredentialValid tracks whether the current credentials are valid (1=valid, 0=invalid)
	CredentialValid = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "credential_valid",
			Help:      "Whether the current credentials are valid (1=valid, 0=invalid)",
		},
	)

	// AuditEventsTotal tracks the total number of audit events by type
	AuditEventsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "audit_events_total",
			Help:      "Total number of audit events by type, category, and severity",
		},
		[]string{"event_type", "category", "severity"},
	)

	// Dynamic NodeGroup Metrics

	// DynamicNodeGroupCreationsTotal tracks dynamic NodeGroup creation attempts
	DynamicNodeGroupCreationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "dynamic_nodegroup_creations_total",
			Help:      "Total number of dynamic NodeGroup creation attempts",
		},
		[]string{"result", "namespace"}, // result: success, failure
	)

	// EventWatcher Metrics

	// EventBufferSize tracks the current size of the event buffer
	EventBufferSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: Namespace,
			Name:      "event_buffer_size",
			Help:      "Current number of scheduling events in the buffer",
		},
	)

	// EventBufferDropped tracks the number of events dropped due to buffer overflow
	EventBufferDropped = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "event_buffer_dropped_total",
			Help:      "Total number of events dropped due to buffer overflow",
		},
	)

	// ScaleUpDecisionsTotal tracks scale-up decisions made
	ScaleUpDecisionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "scale_up_decisions_total",
			Help:      "Total number of scale-up decisions made",
		},
		[]string{"nodegroup", "namespace", "result"}, // result: executed, skipped_cooldown, skipped_max_capacity
	)

	// ScaleUpDecisionNodesRequested tracks nodes requested in scale-up decisions
	ScaleUpDecisionNodesRequested = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: Namespace,
			Name:      "scale_up_decision_nodes_requested",
			Help:      "Number of nodes requested in scale-up decisions",
			Buckets:   prometheus.LinearBuckets(1, 1, 10), // 1 to 10 nodes
		},
		[]string{"nodegroup", "namespace"},
	)

	// WebhookNamespaceValidationRejectionsTotal tracks namespace validation rejections in webhooks
	WebhookNamespaceValidationRejectionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: Namespace,
			Name:      "webhook_namespace_validation_rejections_total",
			Help:      "Total number of webhook namespace validation rejections",
		},
		[]string{"resource_type", "namespace"},
		// resource_type: NodeGroup, VPSieNode
		// namespace: the rejected namespace
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
		VPSieAPICircuitBreakerState,
		VPSieAPICircuitBreakerOpened,
		VPSieAPICircuitBreakerStateChanges,
		VPSieAPICircuitBreakerHalfOpenAttempts,
		VPSieAPICircuitBreakerHalfOpenSuccesses,
		VPSieAPICircuitBreakerHalfOpenFailures,
		VPSieAPICircuitBreakerHalfOpenRejected,
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
		ScaleDownBlockedTotal,
		SafetyCheckFailuresTotal,
		NodeDrainDuration,
		NodeDrainPodsEvicted,
		// Phase 2 Enhanced Metrics
		ReconciliationQueueDepth,
		ScalingDecisionsTotal,
		WebhookValidationDuration,
		CostSavingsEstimatedMonthly,
		RebalancerOperationsTotal,
		RebalancerNodesReplacedTotal,
		RebalancerCostSavingsTotal,
		NodeUtilizationCPU,
		NodeUtilizationMemory,
		NodeGroupCostCurrent,
		// Phase 5 Security Metrics - Credential Rotation
		CredentialRotationAttempts,
		CredentialRotationSuccesses,
		CredentialRotationFailures,
		CredentialRotationDuration,
		CredentialExpiresAt,
		CredentialValid,
		// Phase 5 Security Metrics - Audit Logging
		AuditEventsTotal,
		// Dynamic NodeGroup and Event Watcher Metrics
		DynamicNodeGroupCreationsTotal,
		EventBufferSize,
		EventBufferDropped,
		ScaleUpDecisionsTotal,
		ScaleUpDecisionNodesRequested,
		// Webhook Metrics
		WebhookNamespaceValidationRejectionsTotal,
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
	VPSieAPICircuitBreakerState.Reset()
	VPSieAPICircuitBreakerOpened.Reset()
	VPSieAPICircuitBreakerStateChanges.Reset()
	// Note: Counter metrics don't have Reset() - they use Add() and are reset by recreating
	// VPSieAPICircuitBreakerHalfOpenAttempts, etc. are Counters, not CounterVecs
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
	ScaleDownBlockedTotal.Reset()
	SafetyCheckFailuresTotal.Reset()
	NodeDrainDuration.Reset()
	NodeDrainPodsEvicted.Reset()
	// Phase 2 Enhanced Metrics
	ReconciliationQueueDepth.Reset()
	ScalingDecisionsTotal.Reset()
	WebhookValidationDuration.Reset()
	CostSavingsEstimatedMonthly.Reset()
	RebalancerOperationsTotal.Reset()
	RebalancerNodesReplacedTotal.Reset()
	RebalancerCostSavingsTotal.Reset()
	NodeUtilizationCPU.Reset()
	NodeUtilizationMemory.Reset()
	NodeGroupCostCurrent.Reset()
	// Dynamic NodeGroup and Event Watcher Metrics
	DynamicNodeGroupCreationsTotal.Reset()
	ScaleUpDecisionsTotal.Reset()
	ScaleUpDecisionNodesRequested.Reset()
	// Webhook Metrics
	WebhookNamespaceValidationRejectionsTotal.Reset()
}
