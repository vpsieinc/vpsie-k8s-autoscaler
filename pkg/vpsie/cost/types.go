package cost

import (
	"time"

	v1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// OfferingCost represents the cost of a VPSie offering
type OfferingCost struct {
	OfferingID  string
	Name        string
	HourlyCost  float64
	DailyCost   float64
	MonthlyCost float64
	Currency    string
	Specs       ResourceSpecs
	Category    string
	LastUpdated time.Time
}

// ResourceSpecs represents the resource specifications of an offering
type ResourceSpecs struct {
	CPU       int // Number of CPU cores
	MemoryMB  int // Memory in MB
	DiskGB    int // Disk in GB
	Bandwidth int // Bandwidth in GB
}

// NodeGroupCost represents the cost of a NodeGroup
type NodeGroupCost struct {
	NodeGroupName    string
	Namespace        string
	TotalNodes       int32
	CostPerNode      float64
	TotalHourly      float64
	TotalDaily       float64
	TotalMonthly     float64
	InstanceTypes    map[string]InstanceTypeCost // offeringID -> cost breakdown
	LastUpdated      time.Time
	EstimatedSavings float64 // Potential savings if optimized
}

// InstanceTypeCost represents cost for a specific instance type in the group
type InstanceTypeCost struct {
	OfferingID   string
	Count        int32
	HourlyEach   float64
	TotalHourly  float64
	TotalMonthly float64
}

// CostComparison compares costs between multiple offerings
type CostComparison struct {
	Offerings       []OfferingCost
	CheapestID      string
	MostExpensiveID string
	AverageCost     float64
	ComparedAt      time.Time
}

// CostSnapshot represents a point-in-time cost snapshot
type CostSnapshot struct {
	Timestamp       time.Time
	NodeGroupName   string
	Namespace       string
	Cost            NodeGroupCost
	Utilization     ResourceUtilization
	EfficiencyScore float64 // 0-100, higher is better
}

// ResourceUtilization represents resource utilization metrics
type ResourceUtilization struct {
	CPUPercent    float64
	MemoryPercent float64
	DiskPercent   float64
	NodeCount     int32
}

// SavingsAnalysis analyzes potential savings from optimization
type SavingsAnalysis struct {
	CurrentCost       NodeGroupCost
	ProposedCost      NodeGroupCost
	MonthlySavings    float64
	AnnualSavings     float64
	SavingsPercent    float64
	BreakEvenDays     int
	RecommendedAction string
	Confidence        float64 // 0-1
	GeneratedAt       time.Time
}

// OptimizationReport contains optimization opportunities for a NodeGroup
type OptimizationReport struct {
	NodeGroupName     string
	Namespace         string
	CurrentCost       NodeGroupCost
	Opportunities     []Opportunity
	PotentialSavings  float64
	RecommendedAction string
	GeneratedAt       time.Time
}

// Opportunity represents a cost optimization opportunity
type Opportunity struct {
	Type                OptimizationType
	Description         string
	CurrentOffering     string
	RecommendedOffering string
	AffectedNodes       []string
	MonthlySavings      float64
	AnnualSavings       float64
	ConfidenceScore     float64 // 0-1
	Risk                RiskLevel
	PerformanceImpact   string
	Implementation      string
}

// OptimizationType represents the type of optimization
type OptimizationType string

const (
	OptimizationDownsize         OptimizationType = "downsize"
	OptimizationRightSize        OptimizationType = "rightsize"
	OptimizationUpsize           OptimizationType = "upsize"
	OptimizationChangeCategory   OptimizationType = "change_category"
	OptimizationConsolidateNodes OptimizationType = "consolidate"
	OptimizationSpotInstances    OptimizationType = "use_spot_instances"
	OptimizationReserved         OptimizationType = "reserved_instances"
)

// RiskLevel represents the risk level of an optimization
type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

// Recommendation represents a recommendation for instance type selection
type Recommendation struct {
	OfferingID         string
	OfferingName       string
	Rationale          string
	ExpectedSavings    float64
	PerformanceImpact  string
	Confidence         float64
	AlternativeOptions []string
}

// Optimization represents an optimization to be applied
type Optimization struct {
	NodeGroupName     string
	Namespace         string
	Opportunity       Opportunity
	SimulationResults *SimulationResult
	ApprovedBy        string
	ApprovedAt        time.Time
}

// SimulationResult represents the result of simulating an optimization
type SimulationResult struct {
	Optimization        Opportunity
	EstimatedSavings    float64
	EstimatedRisk       RiskLevel
	PerformanceImpact   PerformanceImpact
	MigrationPlan       *MigrationPlan
	IsViable            bool
	ViolatedConstraints []string
	SimulatedAt         time.Time
}

// PerformanceImpact represents the estimated performance impact
type PerformanceImpact struct {
	CPUChange    int    // Percentage change in CPU capacity
	MemoryChange int    // Percentage change in memory capacity
	DiskChange   int    // Percentage change in disk capacity
	Impact       string // "negligible", "minor", "moderate", "significant"
}

// MigrationPlan represents a plan for migrating nodes
type MigrationPlan struct {
	TotalNodes    int32
	NodesPerBatch int32
	EstimatedTime time.Duration
	RequiresDrain bool
	RollbackPlan  string
	Steps         []MigrationStep
}

// MigrationStep represents a single step in a migration
type MigrationStep struct {
	StepNumber  int
	Description string
	NodeNames   []string
	Duration    time.Duration
}

// CostTrend represents cost trend over time
type CostTrend struct {
	NodeGroupName string
	Namespace     string
	StartTime     time.Time
	EndTime       time.Time
	DataPoints    []CostDataPoint
	AverageCost   float64
	MinCost       float64
	MaxCost       float64
	Trend         TrendDirection
	ChangePercent float64
	Forecast      *CostForecast
}

// TrendDirection represents the direction of a trend
type TrendDirection string

const (
	TrendIncreasing TrendDirection = "increasing"
	TrendDecreasing TrendDirection = "decreasing"
	TrendStable     TrendDirection = "stable"
	TrendVolatile   TrendDirection = "volatile"
)

// CostDataPoint represents a single data point in cost history
type CostDataPoint struct {
	Timestamp   time.Time
	HourlyCost  float64
	MonthlyCost float64
	NodeCount   int32
	Utilization ResourceUtilization
}

// CostForecast represents a cost forecast
type CostForecast struct {
	NodeGroupName   string
	Namespace       string
	ForecastHorizon time.Duration
	PredictedCost   float64
	ConfidenceLevel float64
	UpperBound      float64
	LowerBound      float64
	Assumptions     []string
	GeneratedAt     time.Time
}

// UtilizationAnalysis analyzes resource utilization vs cost
type UtilizationAnalysis struct {
	NodeGroupName      string
	Namespace          string
	AverageUtilization ResourceUtilization
	PeakUtilization    ResourceUtilization
	CostPerCPUCore     float64
	CostPerGBMemory    float64
	CostPerGBDisk      float64
	EfficiencyScore    float64 // 0-100, higher is better
	WasteEstimate      float64 // Monthly waste due to over-provisioning
	Recommendations    []string
	AnalyzedAt         time.Time
}

// CostOptimizationConfig represents configuration for cost optimization
type CostOptimizationConfig struct {
	Enabled              bool
	Strategy             OptimizationStrategy
	OptimizationInterval time.Duration
	Thresholds           OptimizationThresholds
	Constraints          OptimizationConstraints
	Notifications        NotificationConfig
}

// OptimizationStrategy represents the optimization strategy
type OptimizationStrategy string

const (
	StrategyAuto         OptimizationStrategy = "auto"
	StrategyManual       OptimizationStrategy = "manual"
	StrategyAggressive   OptimizationStrategy = "aggressive"
	StrategyConservative OptimizationStrategy = "conservative"
)

// OptimizationThresholds represents thresholds for optimization
type OptimizationThresholds struct {
	CPUUtilization     float64       // Downsize if below this %
	MemoryUtilization  float64       // Downsize if below this %
	EvaluationPeriod   time.Duration // How long to wait before optimizing
	MinSamplesRequired int           // Minimum samples needed for decision
}

// OptimizationConstraints represents constraints for optimization
type OptimizationConstraints struct {
	MinMonthlySavings       float64 // Minimum monthly savings to apply optimization
	MaxPerformanceImpact    int     // Maximum performance reduction % allowed
	RequireApproval         bool    // Whether manual approval is required
	AllowedOptimizations    []OptimizationType
	ForbiddenOptimizations  []OptimizationType
	MaxNodesPerOptimization int32 // Max nodes to optimize at once
}

// NotificationConfig represents notification configuration
type NotificationConfig struct {
	Enabled   bool
	Slack     string   // Slack channel
	Email     []string // Email addresses
	Webhook   string   // Webhook URL
	OnSavings float64  // Notify if savings exceed this amount
}

// ResourceRequirements represents resource requirements for instance selection
type ResourceRequirements struct {
	MinCPU       int
	MinMemoryMB  int
	MinDiskGB    int
	MinBandwidth int
	Workload     WorkloadType
	Priority     PriorityClass
}

// WorkloadType represents the type of workload
type WorkloadType string

const (
	WorkloadGeneral          WorkloadType = "general"
	WorkloadCPUIntensive     WorkloadType = "cpu_intensive"
	WorkloadMemoryIntensive  WorkloadType = "memory_intensive"
	WorkloadStorageIntensive WorkloadType = "storage_intensive"
	WorkloadBurstable        WorkloadType = "burstable"
)

// PriorityClass represents the priority class
type PriorityClass string

const (
	PriorityCost        PriorityClass = "cost"        // Optimize for cost
	PriorityPerformance PriorityClass = "performance" // Optimize for performance
	PriorityBalanced    PriorityClass = "balanced"    // Balance cost and performance
)

// NodeGroupWithCost combines NodeGroup with cost information
type NodeGroupWithCost struct {
	NodeGroup *v1alpha1.NodeGroup
	Cost      *NodeGroupCost
	Analysis  *UtilizationAnalysis
}
