package rebalancer

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for node rebalancing
type Metrics struct {
	// Rebalancing operations
	PlansCreated  *prometheus.CounterVec
	PlansExecuted *prometheus.CounterVec
	PlansFailed   *prometheus.CounterVec
	PlansDuration *prometheus.HistogramVec

	// Node operations
	NodesProvisioned *prometheus.CounterVec
	NodesDrained     *prometheus.CounterVec
	NodesTerminated  *prometheus.CounterVec
	NodesFailed      *prometheus.CounterVec

	// Safety checks
	SafetyChecksPassed *prometheus.CounterVec
	SafetyChecksFailed *prometheus.CounterVec
	RollbacksExecuted  *prometheus.CounterVec

	// Progress metrics
	CurrentProgress         *prometheus.GaugeVec
	CurrentBatch            *prometheus.GaugeVec
	EstimatedCompletionTime *prometheus.GaugeVec

	// Batch metrics
	BatchesExecuted *prometheus.CounterVec
	BatchesFailed   *prometheus.CounterVec
	BatchDuration   *prometheus.HistogramVec

	// Operation duration metrics
	ProvisionDuration *prometheus.HistogramVec
	DrainDuration     *prometheus.HistogramVec
	TerminateDuration *prometheus.HistogramVec

	// Savings metrics
	SavingsRealized *prometheus.GaugeVec
}

// NewMetrics creates and registers all rebalancing metrics
func NewMetrics(registry prometheus.Registerer) *Metrics {
	if registry == nil {
		registry = prometheus.DefaultRegisterer
	}

	factory := promauto.With(registry)

	return &Metrics{
		// Rebalancing operations
		PlansCreated: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_plans_created_total",
				Help: "Total number of rebalancing plans created",
			},
			[]string{"nodegroup", "namespace", "strategy"},
		),

		PlansExecuted: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_plans_executed_total",
				Help: "Total number of rebalancing plans successfully executed",
			},
			[]string{"nodegroup", "namespace", "strategy"},
		),

		PlansFailed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_plans_failed_total",
				Help: "Total number of rebalancing plans that failed",
			},
			[]string{"nodegroup", "namespace", "strategy", "reason"},
		),

		PlansDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "vpsie_rebalancing_duration_seconds",
				Help:    "Duration of rebalancing operations in seconds",
				Buckets: []float64{60, 300, 600, 1200, 1800, 3600, 7200}, // 1m to 2h
			},
			[]string{"nodegroup", "namespace", "strategy"},
		),

		// Node operations
		NodesProvisioned: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_nodes_provisioned_total",
				Help: "Total number of nodes provisioned during rebalancing",
			},
			[]string{"nodegroup", "namespace", "offering"},
		),

		NodesDrained: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_nodes_drained_total",
				Help: "Total number of nodes successfully drained",
			},
			[]string{"nodegroup", "namespace"},
		),

		NodesTerminated: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_nodes_terminated_total",
				Help: "Total number of nodes terminated during rebalancing",
			},
			[]string{"nodegroup", "namespace"},
		),

		NodesFailed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_nodes_failed_total",
				Help: "Total number of node operations that failed",
			},
			[]string{"nodegroup", "namespace", "operation", "reason"},
		),

		// Safety checks
		SafetyChecksPassed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_safety_checks_passed_total",
				Help: "Total number of safety checks that passed",
			},
			[]string{"nodegroup", "namespace", "category"},
		),

		SafetyChecksFailed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_safety_checks_failed_total",
				Help: "Total number of safety checks that failed",
			},
			[]string{"nodegroup", "namespace", "category", "reason"},
		),

		RollbacksExecuted: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_rollbacks_executed_total",
				Help: "Total number of rollbacks executed",
			},
			[]string{"nodegroup", "namespace", "reason"},
		),

		// Progress metrics
		CurrentProgress: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_rebalancing_progress_percent",
				Help: "Current progress of rebalancing operation (0-100)",
			},
			[]string{"nodegroup", "namespace", "plan_id"},
		),

		CurrentBatch: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_rebalancing_current_batch",
				Help: "Current batch number being executed",
			},
			[]string{"nodegroup", "namespace", "plan_id"},
		),

		EstimatedCompletionTime: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_rebalancing_estimated_completion_seconds",
				Help: "Estimated seconds until rebalancing completion",
			},
			[]string{"nodegroup", "namespace", "plan_id"},
		),

		// Batch metrics
		BatchesExecuted: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_batches_executed_total",
				Help: "Total number of batches successfully executed",
			},
			[]string{"nodegroup", "namespace"},
		),

		BatchesFailed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_rebalancing_batches_failed_total",
				Help: "Total number of batches that failed",
			},
			[]string{"nodegroup", "namespace", "reason"},
		),

		BatchDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "vpsie_rebalancing_batch_duration_seconds",
				Help:    "Duration of batch execution in seconds",
				Buckets: []float64{30, 60, 120, 300, 600, 900}, // 30s to 15m
			},
			[]string{"nodegroup", "namespace"},
		),

		// Operation duration metrics
		ProvisionDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "vpsie_rebalancing_provision_duration_seconds",
				Help:    "Duration of node provisioning operations in seconds",
				Buckets: []float64{10, 30, 60, 120, 300, 600}, // 10s to 10m
			},
			[]string{"nodegroup", "namespace", "offering"},
		),

		DrainDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "vpsie_rebalancing_drain_duration_seconds",
				Help:    "Duration of node draining operations in seconds",
				Buckets: []float64{10, 30, 60, 120, 300, 600}, // 10s to 10m
			},
			[]string{"nodegroup", "namespace"},
		),

		TerminateDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "vpsie_rebalancing_terminate_duration_seconds",
				Help:    "Duration of node termination operations in seconds",
				Buckets: []float64{1, 5, 10, 30, 60}, // 1s to 1m
			},
			[]string{"nodegroup", "namespace"},
		),

		// Savings metrics
		SavingsRealized: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_rebalancing_savings_realized_monthly",
				Help: "Monthly cost savings realized from rebalancing (USD)",
			},
			[]string{"nodegroup", "namespace"},
		),
	}
}

// RecordPlanCreated records a plan creation
func (m *Metrics) RecordPlanCreated(nodeGroup, namespace, strategy string) {
	m.PlansCreated.WithLabelValues(nodeGroup, namespace, strategy).Inc()
}

// RecordPlanExecuted records a successful plan execution
func (m *Metrics) RecordPlanExecuted(nodeGroup, namespace, strategy string, duration float64) {
	m.PlansExecuted.WithLabelValues(nodeGroup, namespace, strategy).Inc()
	m.PlansDuration.WithLabelValues(nodeGroup, namespace, strategy).Observe(duration)
}

// RecordPlanFailed records a failed plan execution
func (m *Metrics) RecordPlanFailed(nodeGroup, namespace, strategy, reason string) {
	m.PlansFailed.WithLabelValues(nodeGroup, namespace, strategy, reason).Inc()
}

// RecordNodeProvisioned records a successful node provisioning
func (m *Metrics) RecordNodeProvisioned(nodeGroup, namespace, offering string, duration float64) {
	m.NodesProvisioned.WithLabelValues(nodeGroup, namespace, offering).Inc()
	m.ProvisionDuration.WithLabelValues(nodeGroup, namespace, offering).Observe(duration)
}

// RecordNodeDrained records a successful node drain
func (m *Metrics) RecordNodeDrained(nodeGroup, namespace string, duration float64) {
	m.NodesDrained.WithLabelValues(nodeGroup, namespace).Inc()
	m.DrainDuration.WithLabelValues(nodeGroup, namespace).Observe(duration)
}

// RecordNodeTerminated records a successful node termination
func (m *Metrics) RecordNodeTerminated(nodeGroup, namespace string, duration float64) {
	m.NodesTerminated.WithLabelValues(nodeGroup, namespace).Inc()
	m.TerminateDuration.WithLabelValues(nodeGroup, namespace).Observe(duration)
}

// RecordNodeFailed records a failed node operation
func (m *Metrics) RecordNodeFailed(nodeGroup, namespace, operation, reason string) {
	m.NodesFailed.WithLabelValues(nodeGroup, namespace, operation, reason).Inc()
}

// RecordSafetyCheckPassed records a passed safety check
func (m *Metrics) RecordSafetyCheckPassed(nodeGroup, namespace, category string) {
	m.SafetyChecksPassed.WithLabelValues(nodeGroup, namespace, category).Inc()
}

// RecordSafetyCheckFailed records a failed safety check
func (m *Metrics) RecordSafetyCheckFailed(nodeGroup, namespace, category, reason string) {
	m.SafetyChecksFailed.WithLabelValues(nodeGroup, namespace, category, reason).Inc()
}

// RecordRollback records a rollback execution
func (m *Metrics) RecordRollback(nodeGroup, namespace, reason string) {
	m.RollbacksExecuted.WithLabelValues(nodeGroup, namespace, reason).Inc()
}

// UpdateProgress updates the current progress of a rebalancing operation
func (m *Metrics) UpdateProgress(nodeGroup, namespace, planID string, percent float64) {
	m.CurrentProgress.WithLabelValues(nodeGroup, namespace, planID).Set(percent)
}

// UpdateCurrentBatch updates the current batch number
func (m *Metrics) UpdateCurrentBatch(nodeGroup, namespace, planID string, batchNumber int) {
	m.CurrentBatch.WithLabelValues(nodeGroup, namespace, planID).Set(float64(batchNumber))
}

// UpdateEstimatedCompletion updates the estimated completion time
func (m *Metrics) UpdateEstimatedCompletion(nodeGroup, namespace, planID string, seconds float64) {
	m.EstimatedCompletionTime.WithLabelValues(nodeGroup, namespace, planID).Set(seconds)
}

// RecordBatchExecuted records a successful batch execution
func (m *Metrics) RecordBatchExecuted(nodeGroup, namespace string, duration float64) {
	m.BatchesExecuted.WithLabelValues(nodeGroup, namespace).Inc()
	m.BatchDuration.WithLabelValues(nodeGroup, namespace).Observe(duration)
}

// RecordBatchFailed records a failed batch execution
func (m *Metrics) RecordBatchFailed(nodeGroup, namespace, reason string) {
	m.BatchesFailed.WithLabelValues(nodeGroup, namespace, reason).Inc()
}

// RecordSavingsRealized records the cost savings realized from rebalancing
func (m *Metrics) RecordSavingsRealized(nodeGroup, namespace string, monthlySavings float64) {
	m.SavingsRealized.WithLabelValues(nodeGroup, namespace).Set(monthlySavings)
}

// ClearProgressMetrics clears progress metrics after rebalancing completes
func (m *Metrics) ClearProgressMetrics(nodeGroup, namespace, planID string) {
	m.CurrentProgress.DeleteLabelValues(nodeGroup, namespace, planID)
	m.CurrentBatch.DeleteLabelValues(nodeGroup, namespace, planID)
	m.EstimatedCompletionTime.DeleteLabelValues(nodeGroup, namespace, planID)
}
