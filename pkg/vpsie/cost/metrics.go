package cost

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for cost optimization
type Metrics struct {
	// Cost metrics
	NodeGroupCostHourly  *prometheus.GaugeVec
	NodeGroupCostMonthly *prometheus.GaugeVec
	CostPerNode          *prometheus.GaugeVec
	CostPerCPUCore       *prometheus.GaugeVec
	CostPerGBMemory      *prometheus.GaugeVec

	// Optimization metrics
	OptimizationOpportunities *prometheus.GaugeVec
	PotentialSavingsMonthly   *prometheus.GaugeVec
	OptimizationsApplied      *prometheus.CounterVec
	OptimizationsFailed       *prometheus.CounterVec
	SavingsRealizedMonthly    *prometheus.GaugeVec

	// Utilization metrics
	ResourceUtilizationCPU    *prometheus.GaugeVec
	ResourceUtilizationMemory *prometheus.GaugeVec
	ResourceUtilizationDisk   *prometheus.GaugeVec
	EfficiencyScore           *prometheus.GaugeVec
	WasteEstimateMonthly      *prometheus.GaugeVec

	// Trend metrics
	CostTrend         *prometheus.GaugeVec
	CostChangePercent *prometheus.GaugeVec

	// Analysis metrics
	SnapshotsRecorded       *prometheus.CounterVec
	AnalysisExecuted        *prometheus.CounterVec
	AnalysisErrors          *prometheus.CounterVec
	AnalysisDurationSeconds *prometheus.HistogramVec
}

// NewMetrics creates and registers all cost optimization metrics
func NewMetrics(registry prometheus.Registerer) *Metrics {
	if registry == nil {
		registry = prometheus.DefaultRegisterer
	}

	factory := promauto.With(registry)

	return &Metrics{
		// Cost metrics
		NodeGroupCostHourly: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_nodegroup_cost_hourly",
				Help: "Current hourly cost of the NodeGroup in USD",
			},
			[]string{"nodegroup", "namespace", "datacenter"},
		),

		NodeGroupCostMonthly: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_nodegroup_cost_monthly",
				Help: "Current monthly cost of the NodeGroup in USD (730 hours)",
			},
			[]string{"nodegroup", "namespace", "datacenter"},
		),

		CostPerNode: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_nodegroup_cost_per_node",
				Help: "Average cost per node in the NodeGroup (USD/hour)",
			},
			[]string{"nodegroup", "namespace"},
		),

		CostPerCPUCore: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_cost_per_cpu_core",
				Help: "Cost per CPU core in the NodeGroup (USD/month)",
			},
			[]string{"nodegroup", "namespace", "offering"},
		),

		CostPerGBMemory: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_cost_per_gb_memory",
				Help: "Cost per GB of memory in the NodeGroup (USD/month)",
			},
			[]string{"nodegroup", "namespace", "offering"},
		),

		// Optimization metrics
		OptimizationOpportunities: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_cost_optimization_opportunities",
				Help: "Number of cost optimization opportunities identified",
			},
			[]string{"nodegroup", "namespace", "type"},
		),

		PotentialSavingsMonthly: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_cost_potential_savings_monthly",
				Help: "Potential monthly cost savings from all opportunities (USD)",
			},
			[]string{"nodegroup", "namespace"},
		),

		OptimizationsApplied: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_cost_optimizations_applied_total",
				Help: "Total number of cost optimizations successfully applied",
			},
			[]string{"nodegroup", "namespace", "type"},
		),

		OptimizationsFailed: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_cost_optimizations_failed_total",
				Help: "Total number of cost optimizations that failed",
			},
			[]string{"nodegroup", "namespace", "type", "reason"},
		),

		SavingsRealizedMonthly: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_cost_savings_realized_monthly",
				Help: "Actual monthly cost savings realized from optimizations (USD)",
			},
			[]string{"nodegroup", "namespace"},
		),

		// Utilization metrics
		ResourceUtilizationCPU: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_resource_utilization_cpu_percent",
				Help: "Average CPU utilization percentage for the NodeGroup",
			},
			[]string{"nodegroup", "namespace"},
		),

		ResourceUtilizationMemory: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_resource_utilization_memory_percent",
				Help: "Average memory utilization percentage for the NodeGroup",
			},
			[]string{"nodegroup", "namespace"},
		),

		ResourceUtilizationDisk: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_resource_utilization_disk_percent",
				Help: "Average disk utilization percentage for the NodeGroup",
			},
			[]string{"nodegroup", "namespace"},
		),

		EfficiencyScore: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_resource_efficiency_score",
				Help: "Resource efficiency score (0-100, higher is better)",
			},
			[]string{"nodegroup", "namespace"},
		),

		WasteEstimateMonthly: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_cost_waste_estimate_monthly",
				Help: "Estimated monthly waste from unused resources (USD)",
			},
			[]string{"nodegroup", "namespace"},
		),

		// Trend metrics
		CostTrend: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_cost_trend",
				Help: "Cost trend direction (1=increasing, 0=stable, -1=decreasing, 2=volatile)",
			},
			[]string{"nodegroup", "namespace"},
		),

		CostChangePercent: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "vpsie_cost_change_percent",
				Help: "Cost change percentage over the analysis period",
			},
			[]string{"nodegroup", "namespace", "period"},
		),

		// Analysis metrics
		SnapshotsRecorded: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_cost_snapshots_recorded_total",
				Help: "Total number of cost snapshots recorded",
			},
			[]string{"nodegroup", "namespace"},
		),

		AnalysisExecuted: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_cost_analysis_executed_total",
				Help: "Total number of cost analyses executed",
			},
			[]string{"nodegroup", "namespace", "type"},
		),

		AnalysisErrors: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "vpsie_cost_analysis_errors_total",
				Help: "Total number of cost analysis errors",
			},
			[]string{"nodegroup", "namespace", "type", "error"},
		),

		AnalysisDurationSeconds: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "vpsie_cost_analysis_duration_seconds",
				Help:    "Duration of cost analysis operations in seconds",
				Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"nodegroup", "namespace", "type"},
		),
	}
}

// RecordCost records cost metrics for a NodeGroup
func (m *Metrics) RecordCost(nodeGroup, namespace, datacenter string, cost *NodeGroupCost) {
	m.NodeGroupCostHourly.WithLabelValues(nodeGroup, namespace, datacenter).Set(cost.TotalHourly)
	m.NodeGroupCostMonthly.WithLabelValues(nodeGroup, namespace, datacenter).Set(cost.TotalMonthly)
	m.CostPerNode.WithLabelValues(nodeGroup, namespace).Set(cost.CostPerNode)
}

// RecordUtilization records utilization metrics
func (m *Metrics) RecordUtilization(nodeGroup, namespace string, utilization ResourceUtilization, efficiency float64) {
	m.ResourceUtilizationCPU.WithLabelValues(nodeGroup, namespace).Set(utilization.CPUPercent)
	m.ResourceUtilizationMemory.WithLabelValues(nodeGroup, namespace).Set(utilization.MemoryPercent)
	m.ResourceUtilizationDisk.WithLabelValues(nodeGroup, namespace).Set(utilization.DiskPercent)
	m.EfficiencyScore.WithLabelValues(nodeGroup, namespace).Set(efficiency)
}

// RecordOptimizationOpportunities records optimization opportunity metrics
func (m *Metrics) RecordOptimizationOpportunities(nodeGroup, namespace string, report *OptimizationReport) {
	// Count opportunities by type
	oppCounts := make(map[OptimizationType]int)
	for _, opp := range report.Opportunities {
		oppCounts[opp.Type]++
	}

	// Record counts
	for oppType, count := range oppCounts {
		m.OptimizationOpportunities.WithLabelValues(nodeGroup, namespace, string(oppType)).Set(float64(count))
	}

	// Record potential savings
	m.PotentialSavingsMonthly.WithLabelValues(nodeGroup, namespace).Set(report.PotentialSavings)
}

// RecordOptimizationApplied records a successful optimization
func (m *Metrics) RecordOptimizationApplied(nodeGroup, namespace string, optimizationType OptimizationType) {
	m.OptimizationsApplied.WithLabelValues(nodeGroup, namespace, string(optimizationType)).Inc()
}

// RecordOptimizationFailed records a failed optimization
func (m *Metrics) RecordOptimizationFailed(nodeGroup, namespace string, optimizationType OptimizationType, reason string) {
	m.OptimizationsFailed.WithLabelValues(nodeGroup, namespace, string(optimizationType), reason).Inc()
}

// RecordTrend records cost trend metrics
func (m *Metrics) RecordTrend(nodeGroup, namespace string, trend *CostTrend) {
	// Convert trend direction to numeric value
	trendValue := 0.0
	switch trend.Trend {
	case TrendIncreasing:
		trendValue = 1.0
	case TrendDecreasing:
		trendValue = -1.0
	case TrendStable:
		trendValue = 0.0
	case TrendVolatile:
		trendValue = 2.0
	}

	m.CostTrend.WithLabelValues(nodeGroup, namespace).Set(trendValue)

	period := trend.EndTime.Sub(trend.StartTime).String()
	m.CostChangePercent.WithLabelValues(nodeGroup, namespace, period).Set(trend.ChangePercent)
}

// RecordSnapshot increments snapshot counter
func (m *Metrics) RecordSnapshot(nodeGroup, namespace string) {
	m.SnapshotsRecorded.WithLabelValues(nodeGroup, namespace).Inc()
}

// RecordAnalysis records analysis execution metrics
func (m *Metrics) RecordAnalysis(nodeGroup, namespace, analysisType string, duration float64) {
	m.AnalysisExecuted.WithLabelValues(nodeGroup, namespace, analysisType).Inc()
	m.AnalysisDurationSeconds.WithLabelValues(nodeGroup, namespace, analysisType).Observe(duration)
}

// RecordAnalysisError records analysis error metrics
func (m *Metrics) RecordAnalysisError(nodeGroup, namespace, analysisType, errorType string) {
	m.AnalysisErrors.WithLabelValues(nodeGroup, namespace, analysisType, errorType).Inc()
}

// RecordWaste records waste estimate metrics
func (m *Metrics) RecordWaste(nodeGroup, namespace string, wasteAmount float64) {
	m.WasteEstimateMonthly.WithLabelValues(nodeGroup, namespace).Set(wasteAmount)
}

// RecordSavingsRealized records realized savings
func (m *Metrics) RecordSavingsRealized(nodeGroup, namespace string, savings float64) {
	m.SavingsRealizedMonthly.WithLabelValues(nodeGroup, namespace).Set(savings)
}
