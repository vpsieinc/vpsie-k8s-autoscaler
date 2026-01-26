package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestMetricsNamespace(t *testing.T) {
	if Namespace != "vpsie_autoscaler" {
		t.Errorf("expected namespace 'vpsie_autoscaler', got %s", Namespace)
	}
}

// =============================================================================
// NodeGroup Metrics Tests
// =============================================================================

func TestNodeGroupDesiredNodes(t *testing.T) {
	ResetMetrics()

	NodeGroupDesiredNodes.WithLabelValues("test-ng", "kube-system").Set(5)

	metric := &dto.Metric{}
	err := NodeGroupDesiredNodes.WithLabelValues("test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Gauge.GetValue() != 5 {
		t.Errorf("expected value 5, got %f", metric.Gauge.GetValue())
	}
}

func TestNodeGroupCurrentNodes(t *testing.T) {
	ResetMetrics()

	NodeGroupCurrentNodes.WithLabelValues("test-ng", "kube-system").Set(3)

	metric := &dto.Metric{}
	err := NodeGroupCurrentNodes.WithLabelValues("test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Gauge.GetValue() != 3 {
		t.Errorf("expected value 3, got %f", metric.Gauge.GetValue())
	}
}

func TestNodeGroupReadyNodes(t *testing.T) {
	ResetMetrics()

	NodeGroupReadyNodes.WithLabelValues("test-ng", "kube-system").Set(2)

	metric := &dto.Metric{}
	err := NodeGroupReadyNodes.WithLabelValues("test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Gauge.GetValue() != 2 {
		t.Errorf("expected value 2, got %f", metric.Gauge.GetValue())
	}
}

func TestNodeGroupMinMaxNodes(t *testing.T) {
	ResetMetrics()

	NodeGroupMinNodes.WithLabelValues("test-ng", "kube-system").Set(1)
	NodeGroupMaxNodes.WithLabelValues("test-ng", "kube-system").Set(10)

	minMetric := &dto.Metric{}
	maxMetric := &dto.Metric{}

	err := NodeGroupMinNodes.WithLabelValues("test-ng", "kube-system").Write(minMetric)
	if err != nil {
		t.Fatalf("unexpected error writing min: %v", err)
	}
	err = NodeGroupMaxNodes.WithLabelValues("test-ng", "kube-system").Write(maxMetric)
	if err != nil {
		t.Fatalf("unexpected error writing max: %v", err)
	}

	if minMetric.Gauge.GetValue() != 1 {
		t.Errorf("expected min value 1, got %f", minMetric.Gauge.GetValue())
	}
	if maxMetric.Gauge.GetValue() != 10 {
		t.Errorf("expected max value 10, got %f", maxMetric.Gauge.GetValue())
	}
}

// =============================================================================
// VPSieNode Metrics Tests
// =============================================================================

func TestVPSieNodePhase(t *testing.T) {
	ResetMetrics()

	phases := []string{"Pending", "Provisioning", "Ready", "Deleting", "Failed"}
	for _, phase := range phases {
		VPSieNodePhase.WithLabelValues(phase, "test-ng", "kube-system").Set(1)
	}

	for _, phase := range phases {
		metric := &dto.Metric{}
		err := VPSieNodePhase.WithLabelValues(phase, "test-ng", "kube-system").Write(metric)
		if err != nil {
			t.Fatalf("unexpected error for phase %s: %v", phase, err)
		}
		if metric.Gauge.GetValue() != 1 {
			t.Errorf("expected value 1 for phase %s, got %f", phase, metric.Gauge.GetValue())
		}
	}
}

func TestVPSieNodePhaseTransitions(t *testing.T) {
	ResetMetrics()

	VPSieNodePhaseTransitions.WithLabelValues("Pending", "Provisioning", "test-ng", "kube-system").Inc()
	VPSieNodePhaseTransitions.WithLabelValues("Provisioning", "Ready", "test-ng", "kube-system").Inc()

	metric := &dto.Metric{}
	err := VPSieNodePhaseTransitions.WithLabelValues("Pending", "Provisioning", "test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

// =============================================================================
// Controller Metrics Tests
// =============================================================================

func TestControllerReconcileDuration(t *testing.T) {
	ResetMetrics()

	ControllerReconcileDuration.WithLabelValues("nodegroup").Observe(0.5)
	ControllerReconcileDuration.WithLabelValues("nodegroup").Observe(1.0)
	ControllerReconcileDuration.WithLabelValues("nodegroup").Observe(0.25)

	metric := &dto.Metric{}
	err := ControllerReconcileDuration.WithLabelValues("nodegroup").(prometheus.Histogram).Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Histogram.GetSampleCount() != 3 {
		t.Errorf("expected 3 samples, got %d", metric.Histogram.GetSampleCount())
	}
	expectedSum := 1.75 // 0.5 + 1.0 + 0.25
	if metric.Histogram.GetSampleSum() != expectedSum {
		t.Errorf("expected sum %f, got %f", expectedSum, metric.Histogram.GetSampleSum())
	}
}

func TestControllerReconcileErrors(t *testing.T) {
	ResetMetrics()

	ControllerReconcileErrors.WithLabelValues("nodegroup", "api_error").Inc()
	ControllerReconcileErrors.WithLabelValues("nodegroup", "api_error").Inc()
	ControllerReconcileErrors.WithLabelValues("nodegroup", "validation_error").Inc()

	metric := &dto.Metric{}
	err := ControllerReconcileErrors.WithLabelValues("nodegroup", "api_error").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 2 {
		t.Errorf("expected value 2, got %f", metric.Counter.GetValue())
	}
}

func TestControllerReconcileTotal(t *testing.T) {
	ResetMetrics()

	ControllerReconcileTotal.WithLabelValues("nodegroup", "success").Inc()
	ControllerReconcileTotal.WithLabelValues("nodegroup", "success").Inc()
	ControllerReconcileTotal.WithLabelValues("nodegroup", "requeue").Inc()

	metric := &dto.Metric{}
	err := ControllerReconcileTotal.WithLabelValues("nodegroup", "success").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 2 {
		t.Errorf("expected value 2, got %f", metric.Counter.GetValue())
	}
}

// =============================================================================
// VPSie API Metrics Tests
// =============================================================================

func TestVPSieAPIRequests(t *testing.T) {
	ResetMetrics()

	VPSieAPIRequests.WithLabelValues("CreateVM", "200").Inc()
	VPSieAPIRequests.WithLabelValues("CreateVM", "200").Inc()
	VPSieAPIRequests.WithLabelValues("CreateVM", "500").Inc()

	metric := &dto.Metric{}
	err := VPSieAPIRequests.WithLabelValues("CreateVM", "200").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 2 {
		t.Errorf("expected value 2, got %f", metric.Counter.GetValue())
	}
}

func TestVPSieAPIRequestDuration(t *testing.T) {
	ResetMetrics()

	VPSieAPIRequestDuration.WithLabelValues("CreateVM").Observe(0.5)
	VPSieAPIRequestDuration.WithLabelValues("CreateVM").Observe(1.2)

	metric := &dto.Metric{}
	err := VPSieAPIRequestDuration.WithLabelValues("CreateVM").(prometheus.Histogram).Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Histogram.GetSampleCount() != 2 {
		t.Errorf("expected 2 samples, got %d", metric.Histogram.GetSampleCount())
	}
}

func TestVPSieAPIErrors(t *testing.T) {
	ResetMetrics()

	VPSieAPIErrors.WithLabelValues("CreateVM", "rate_limited").Inc()
	VPSieAPIErrors.WithLabelValues("CreateVM", "timeout").Inc()
	VPSieAPIErrors.WithLabelValues("CreateVM", "timeout").Inc()

	metric := &dto.Metric{}
	err := VPSieAPIErrors.WithLabelValues("CreateVM", "timeout").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 2 {
		t.Errorf("expected value 2, got %f", metric.Counter.GetValue())
	}
}

func TestVPSieAPIRateLimiting(t *testing.T) {
	ResetMetrics()

	VPSieAPIRateLimitedTotal.WithLabelValues("CreateVM").Inc()
	VPSieAPIRateLimitWaitDuration.WithLabelValues("CreateVM").Observe(0.1)

	metric := &dto.Metric{}
	err := VPSieAPIRateLimitedTotal.WithLabelValues("CreateVM").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

func TestVPSieAPICircuitBreaker(t *testing.T) {
	ResetMetrics()

	VPSieAPICircuitBreakerState.WithLabelValues("closed").Set(1)
	VPSieAPICircuitBreakerState.WithLabelValues("open").Set(0)
	VPSieAPICircuitBreakerState.WithLabelValues("half-open").Set(0)

	metric := &dto.Metric{}
	err := VPSieAPICircuitBreakerState.WithLabelValues("closed").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Gauge.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Gauge.GetValue())
	}
}

func TestVPSieAPICircuitBreakerStateChanges(t *testing.T) {
	ResetMetrics()

	VPSieAPICircuitBreakerStateChanges.WithLabelValues("closed", "open").Inc()
	VPSieAPICircuitBreakerStateChanges.WithLabelValues("open", "half-open").Inc()
	VPSieAPICircuitBreakerStateChanges.WithLabelValues("half-open", "closed").Inc()

	metric := &dto.Metric{}
	err := VPSieAPICircuitBreakerStateChanges.WithLabelValues("closed", "open").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

// =============================================================================
// Scaling Metrics Tests
// =============================================================================

func TestScaleUpTotal(t *testing.T) {
	ResetMetrics()

	ScaleUpTotal.WithLabelValues("test-ng", "kube-system").Inc()
	ScaleUpTotal.WithLabelValues("test-ng", "kube-system").Inc()

	metric := &dto.Metric{}
	err := ScaleUpTotal.WithLabelValues("test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 2 {
		t.Errorf("expected value 2, got %f", metric.Counter.GetValue())
	}
}

func TestScaleDownTotal(t *testing.T) {
	ResetMetrics()

	ScaleDownTotal.WithLabelValues("test-ng", "kube-system").Inc()

	metric := &dto.Metric{}
	err := ScaleDownTotal.WithLabelValues("test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

func TestScaleUpNodesAdded(t *testing.T) {
	ResetMetrics()

	ScaleUpNodesAdded.WithLabelValues("test-ng", "kube-system").Observe(2)
	ScaleUpNodesAdded.WithLabelValues("test-ng", "kube-system").Observe(3)

	metric := &dto.Metric{}
	err := ScaleUpNodesAdded.WithLabelValues("test-ng", "kube-system").(prometheus.Histogram).Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Histogram.GetSampleCount() != 2 {
		t.Errorf("expected 2 samples, got %d", metric.Histogram.GetSampleCount())
	}
	if metric.Histogram.GetSampleSum() != 5 {
		t.Errorf("expected sum 5, got %f", metric.Histogram.GetSampleSum())
	}
}

func TestScaleDownNodesRemoved(t *testing.T) {
	ResetMetrics()

	ScaleDownNodesRemoved.WithLabelValues("test-ng", "kube-system").Observe(1)

	metric := &dto.Metric{}
	err := ScaleDownNodesRemoved.WithLabelValues("test-ng", "kube-system").(prometheus.Histogram).Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Histogram.GetSampleCount() != 1 {
		t.Errorf("expected 1 sample, got %d", metric.Histogram.GetSampleCount())
	}
}

func TestScaleDownErrorsTotal(t *testing.T) {
	ResetMetrics()

	ScaleDownErrorsTotal.WithLabelValues("test-ng", "kube-system", "drain_timeout").Inc()

	metric := &dto.Metric{}
	err := ScaleDownErrorsTotal.WithLabelValues("test-ng", "kube-system", "drain_timeout").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

// =============================================================================
// Pod Metrics Tests
// =============================================================================

func TestUnschedulablePodsTotal(t *testing.T) {
	ResetMetrics()

	UnschedulablePodsTotal.WithLabelValues("insufficient_cpu", "default").Inc()
	UnschedulablePodsTotal.WithLabelValues("insufficient_memory", "default").Inc()

	metric := &dto.Metric{}
	err := UnschedulablePodsTotal.WithLabelValues("insufficient_cpu", "default").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

func TestPendingPodsGauge(t *testing.T) {
	ResetMetrics()

	PendingPodsGauge.WithLabelValues("default").Set(5)
	PendingPodsGauge.WithLabelValues("default").Set(3) // Update

	metric := &dto.Metric{}
	err := PendingPodsGauge.WithLabelValues("default").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Gauge.GetValue() != 3 {
		t.Errorf("expected value 3, got %f", metric.Gauge.GetValue())
	}
}

// =============================================================================
// Node Lifecycle Metrics Tests
// =============================================================================

func TestNodeProvisioningDuration(t *testing.T) {
	ResetMetrics()

	NodeProvisioningDuration.WithLabelValues("test-ng", "kube-system").Observe(120) // 2 minutes

	metric := &dto.Metric{}
	err := NodeProvisioningDuration.WithLabelValues("test-ng", "kube-system").(prometheus.Histogram).Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Histogram.GetSampleCount() != 1 {
		t.Errorf("expected 1 sample, got %d", metric.Histogram.GetSampleCount())
	}
}

func TestNodeTerminationDuration(t *testing.T) {
	ResetMetrics()

	NodeTerminationDuration.WithLabelValues("test-ng", "kube-system").Observe(30)

	metric := &dto.Metric{}
	err := NodeTerminationDuration.WithLabelValues("test-ng", "kube-system").(prometheus.Histogram).Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Histogram.GetSampleCount() != 1 {
		t.Errorf("expected 1 sample, got %d", metric.Histogram.GetSampleCount())
	}
}

func TestNodeDrainDuration(t *testing.T) {
	ResetMetrics()

	NodeDrainDuration.WithLabelValues("test-ng", "kube-system", "success").Observe(45)
	NodeDrainDuration.WithLabelValues("test-ng", "kube-system", "timeout").Observe(300)

	metric := &dto.Metric{}
	err := NodeDrainDuration.WithLabelValues("test-ng", "kube-system", "success").(prometheus.Histogram).Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Histogram.GetSampleCount() != 1 {
		t.Errorf("expected 1 sample, got %d", metric.Histogram.GetSampleCount())
	}
}

func TestNodeDrainPodsEvicted(t *testing.T) {
	ResetMetrics()

	NodeDrainPodsEvicted.WithLabelValues("test-ng", "kube-system").Observe(10)

	metric := &dto.Metric{}
	err := NodeDrainPodsEvicted.WithLabelValues("test-ng", "kube-system").(prometheus.Histogram).Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Histogram.GetSampleSum() != 10 {
		t.Errorf("expected sum 10, got %f", metric.Histogram.GetSampleSum())
	}
}

// =============================================================================
// Safety Metrics Tests
// =============================================================================

func TestScaleDownBlockedTotal(t *testing.T) {
	ResetMetrics()

	ScaleDownBlockedTotal.WithLabelValues("test-ng", "kube-system", "pdb").Inc()
	ScaleDownBlockedTotal.WithLabelValues("test-ng", "kube-system", "cooldown").Inc()

	metric := &dto.Metric{}
	err := ScaleDownBlockedTotal.WithLabelValues("test-ng", "kube-system", "pdb").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

func TestSafetyCheckFailuresTotal(t *testing.T) {
	ResetMetrics()

	SafetyCheckFailuresTotal.WithLabelValues("pdb", "test-ng", "kube-system").Inc()
	SafetyCheckFailuresTotal.WithLabelValues("affinity", "test-ng", "kube-system").Inc()

	metric := &dto.Metric{}
	err := SafetyCheckFailuresTotal.WithLabelValues("pdb", "test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

// =============================================================================
// Events Metrics Tests
// =============================================================================

func TestEventsEmitted(t *testing.T) {
	ResetMetrics()

	EventsEmitted.WithLabelValues("Normal", "ScaleUp", "NodeGroup").Inc()
	EventsEmitted.WithLabelValues("Warning", "ScaleDownBlocked", "NodeGroup").Inc()

	metric := &dto.Metric{}
	err := EventsEmitted.WithLabelValues("Normal", "ScaleUp", "NodeGroup").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

// =============================================================================
// Webhook Metrics Tests
// =============================================================================

func TestWebhookValidationDuration(t *testing.T) {
	ResetMetrics()

	WebhookValidationDuration.WithLabelValues("nodegroup", "create", "allowed").Observe(0.001)
	WebhookValidationDuration.WithLabelValues("nodegroup", "create", "denied").Observe(0.002)

	metric := &dto.Metric{}
	err := WebhookValidationDuration.WithLabelValues("nodegroup", "create", "allowed").(prometheus.Histogram).Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if metric.Histogram.GetSampleCount() != 1 {
		t.Errorf("expected 1 sample, got %d", metric.Histogram.GetSampleCount())
	}
}

func TestWebhookNamespaceValidationRejectionsTotal(t *testing.T) {
	ResetMetrics()

	WebhookNamespaceValidationRejectionsTotal.WithLabelValues("NodeGroup", "default").Inc()

	metric := &dto.Metric{}
	err := WebhookNamespaceValidationRejectionsTotal.WithLabelValues("NodeGroup", "default").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

// =============================================================================
// Rebalancer Metrics Tests
// =============================================================================

func TestRebalancerOperationsTotal(t *testing.T) {
	ResetMetrics()

	RebalancerOperationsTotal.WithLabelValues("test-ng", "kube-system", "analyze", "success").Inc()
	RebalancerOperationsTotal.WithLabelValues("test-ng", "kube-system", "execute", "failure").Inc()

	metric := &dto.Metric{}
	err := RebalancerOperationsTotal.WithLabelValues("test-ng", "kube-system", "analyze", "success").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

func TestRebalancerNodesReplacedTotal(t *testing.T) {
	ResetMetrics()

	RebalancerNodesReplacedTotal.WithLabelValues("test-ng", "kube-system", "rolling").Inc()

	metric := &dto.Metric{}
	err := RebalancerNodesReplacedTotal.WithLabelValues("test-ng", "kube-system", "rolling").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

func TestRebalancerCostSavingsTotal(t *testing.T) {
	ResetMetrics()

	RebalancerCostSavingsTotal.WithLabelValues("test-ng", "kube-system").Add(10.50)

	metric := &dto.Metric{}
	err := RebalancerCostSavingsTotal.WithLabelValues("test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 10.50 {
		t.Errorf("expected value 10.50, got %f", metric.Counter.GetValue())
	}
}

// =============================================================================
// Utilization Metrics Tests
// =============================================================================

func TestNodeUtilizationCPU(t *testing.T) {
	ResetMetrics()

	NodeUtilizationCPU.WithLabelValues("node-1", "test-ng", "kube-system").Set(75.5)

	metric := &dto.Metric{}
	err := NodeUtilizationCPU.WithLabelValues("node-1", "test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Gauge.GetValue() != 75.5 {
		t.Errorf("expected value 75.5, got %f", metric.Gauge.GetValue())
	}
}

func TestNodeUtilizationMemory(t *testing.T) {
	ResetMetrics()

	NodeUtilizationMemory.WithLabelValues("node-1", "test-ng", "kube-system").Set(60.0)

	metric := &dto.Metric{}
	err := NodeUtilizationMemory.WithLabelValues("node-1", "test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Gauge.GetValue() != 60.0 {
		t.Errorf("expected value 60.0, got %f", metric.Gauge.GetValue())
	}
}

// =============================================================================
// Cost Metrics Tests
// =============================================================================

func TestNodeGroupCostCurrent(t *testing.T) {
	ResetMetrics()

	NodeGroupCostCurrent.WithLabelValues("test-ng", "kube-system").Set(12.50)

	metric := &dto.Metric{}
	err := NodeGroupCostCurrent.WithLabelValues("test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Gauge.GetValue() != 12.50 {
		t.Errorf("expected value 12.50, got %f", metric.Gauge.GetValue())
	}
}

func TestCostSavingsEstimatedMonthly(t *testing.T) {
	ResetMetrics()

	CostSavingsEstimatedMonthly.WithLabelValues("test-ng", "kube-system", "scale_down").Set(150.00)

	metric := &dto.Metric{}
	err := CostSavingsEstimatedMonthly.WithLabelValues("test-ng", "kube-system", "scale_down").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Gauge.GetValue() != 150.00 {
		t.Errorf("expected value 150.00, got %f", metric.Gauge.GetValue())
	}
}

// =============================================================================
// Credential Metrics Tests
// =============================================================================

func TestCredentialMetrics(t *testing.T) {
	// Note: Counter metrics without Vec don't have Reset(), so we just test they work
	CredentialRotationAttempts.Inc()
	CredentialRotationSuccesses.Inc()
	CredentialRotationFailures.Inc()
	CredentialRotationDuration.Observe(0.5)
	CredentialExpiresAt.Set(1700000000)
	CredentialValid.Set(1)

	// Verify CredentialExpiresAt
	metric := &dto.Metric{}
	err := CredentialExpiresAt.Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Gauge.GetValue() != 1700000000 {
		t.Errorf("expected value 1700000000, got %f", metric.Gauge.GetValue())
	}
}

// =============================================================================
// Audit Metrics Tests
// =============================================================================

func TestAuditEventsTotal(t *testing.T) {
	ResetMetrics()

	AuditEventsTotal.WithLabelValues("NodeProvisioned", "scaling", "info").Inc()
	AuditEventsTotal.WithLabelValues("ScaleUpTriggered", "scaling", "info").Inc()

	metric := &dto.Metric{}
	err := AuditEventsTotal.WithLabelValues("NodeProvisioned", "scaling", "info").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

// =============================================================================
// Event Buffer Metrics Tests
// =============================================================================

func TestEventBufferMetrics(t *testing.T) {
	ResetMetrics()

	EventBufferSize.Set(50)
	EventBufferDropped.Inc()

	sizeMetric := &dto.Metric{}
	err := EventBufferSize.Write(sizeMetric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sizeMetric.Gauge.GetValue() != 50 {
		t.Errorf("expected value 50, got %f", sizeMetric.Gauge.GetValue())
	}
}

// =============================================================================
// Scale Up Decision Metrics Tests
// =============================================================================

func TestScaleUpDecisionsTotal(t *testing.T) {
	ResetMetrics()

	ScaleUpDecisionsTotal.WithLabelValues("test-ng", "kube-system", "executed").Inc()
	ScaleUpDecisionsTotal.WithLabelValues("test-ng", "kube-system", "skipped_cooldown").Inc()

	metric := &dto.Metric{}
	err := ScaleUpDecisionsTotal.WithLabelValues("test-ng", "kube-system", "executed").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

func TestScaleUpDecisionNodesRequested(t *testing.T) {
	ResetMetrics()

	ScaleUpDecisionNodesRequested.WithLabelValues("test-ng", "kube-system").Observe(3)

	metric := &dto.Metric{}
	err := ScaleUpDecisionNodesRequested.WithLabelValues("test-ng", "kube-system").(prometheus.Histogram).Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Histogram.GetSampleCount() != 1 {
		t.Errorf("expected 1 sample, got %d", metric.Histogram.GetSampleCount())
	}
}

// =============================================================================
// Discovery Metrics Tests
// =============================================================================

func TestVPSieNodeDiscoveryMetrics(t *testing.T) {
	ResetMetrics()

	VPSieNodeDiscoveryDuration.Observe(2.5)
	VPSieNodeDiscoveryStrategyUsed.WithLabelValues("ip_matching").Inc()
	VPSieNodeDiscoveryFailuresTotal.WithLabelValues("timeout").Inc()

	strategyMetric := &dto.Metric{}
	err := VPSieNodeDiscoveryStrategyUsed.WithLabelValues("ip_matching").Write(strategyMetric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strategyMetric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", strategyMetric.Counter.GetValue())
	}
}

// =============================================================================
// TTL Deletion Metrics Tests
// =============================================================================

func TestVPSieNodeTTLDeletionsTotal(t *testing.T) {
	ResetMetrics()

	VPSieNodeTTLDeletionsTotal.WithLabelValues("test-ng", "kube-system").Inc()

	metric := &dto.Metric{}
	err := VPSieNodeTTLDeletionsTotal.WithLabelValues("test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

// =============================================================================
// Dynamic NodeGroup Metrics Tests
// =============================================================================

func TestDynamicNodeGroupCreationsTotal(t *testing.T) {
	ResetMetrics()

	DynamicNodeGroupCreationsTotal.WithLabelValues("success", "kube-system").Inc()
	DynamicNodeGroupCreationsTotal.WithLabelValues("failure", "kube-system").Inc()

	metric := &dto.Metric{}
	err := DynamicNodeGroupCreationsTotal.WithLabelValues("success", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}

// =============================================================================
// Queue Depth Metrics Tests
// =============================================================================

func TestReconciliationQueueDepth(t *testing.T) {
	ResetMetrics()

	ReconciliationQueueDepth.WithLabelValues("nodegroup").Set(5)
	ReconciliationQueueDepth.WithLabelValues("vpsienode").Set(3)

	metric := &dto.Metric{}
	err := ReconciliationQueueDepth.WithLabelValues("nodegroup").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Gauge.GetValue() != 5 {
		t.Errorf("expected value 5, got %f", metric.Gauge.GetValue())
	}
}

// =============================================================================
// ResetMetrics Tests
// =============================================================================

func TestResetMetrics(t *testing.T) {
	// Set some metrics
	NodeGroupDesiredNodes.WithLabelValues("test-ng", "kube-system").Set(10)
	ScaleUpTotal.WithLabelValues("test-ng", "kube-system").Inc()

	// Reset
	ResetMetrics()

	// Verify reset for gauge
	metric := &dto.Metric{}
	err := NodeGroupDesiredNodes.WithLabelValues("test-ng", "kube-system").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// After Reset(), the metric is cleared so writing gives us a fresh metric with default value
	// However, calling WithLabelValues after Reset() creates a new metric with value 0
	if metric.Gauge.GetValue() != 0 {
		t.Errorf("expected value 0 after reset, got %f", metric.Gauge.GetValue())
	}
}

// =============================================================================
// Scaling Decisions Metrics Tests
// =============================================================================

func TestScalingDecisionsTotal(t *testing.T) {
	ResetMetrics()

	ScalingDecisionsTotal.WithLabelValues("test-ng", "kube-system", "scale_up", "pending_pods").Inc()
	ScalingDecisionsTotal.WithLabelValues("test-ng", "kube-system", "scale_down", "underutilized").Inc()
	ScalingDecisionsTotal.WithLabelValues("test-ng", "kube-system", "no_action", "cooldown").Inc()

	metric := &dto.Metric{}
	err := ScalingDecisionsTotal.WithLabelValues("test-ng", "kube-system", "scale_up", "pending_pods").Write(metric)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if metric.Counter.GetValue() != 1 {
		t.Errorf("expected value 1, got %f", metric.Counter.GetValue())
	}
}
