package rebalancer

import (
	"time"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/cost"
	corev1 "k8s.io/api/core/v1"
)

// RebalanceAnalysis contains the results of analyzing rebalancing opportunities
type RebalanceAnalysis struct {
	NodeGroupName     string
	Namespace         string
	TotalNodes        int32
	CandidateNodes    []CandidateNode
	Optimization      *cost.Opportunity
	SafetyChecks      []SafetyCheck
	RecommendedAction RecommendedAction
	Priority          RebalancePriority
	EstimatedDuration time.Duration
	AnalyzedAt        time.Time
}

// CandidateNode represents a node that is a candidate for rebalancing
type CandidateNode struct {
	NodeName        string
	VPSID           int
	CurrentOffering string
	TargetOffering  string
	Age             time.Duration
	Workloads       []Workload
	PriorityScore   float64
	SafeToRebalance bool
	RebalanceReason string
}

// Workload represents a workload running on a node
type Workload struct {
	Name             string
	Namespace        string
	Kind             string // Deployment, StatefulSet, DaemonSet, etc.
	Replicas         int32
	HasPDB           bool
	HasLocalStorage  bool
	IsCritical       bool
	CanEvict         bool
	EvictionEstimate time.Duration
}

// SafetyCheck represents a single safety check result
type SafetyCheck struct {
	Category  SafetyCheckCategory
	Status    SafetyCheckStatus
	Message   string
	Details   map[string]interface{}
	CheckedAt time.Time
}

// SafetyCheckCategory defines the category of safety check
type SafetyCheckCategory string

const (
	SafetyCheckClusterHealth    SafetyCheckCategory = "cluster_health"
	SafetyCheckNodeGroupHealth  SafetyCheckCategory = "nodegroup_health"
	SafetyCheckPodDisruption    SafetyCheckCategory = "pod_disruption"
	SafetyCheckResourceCapacity SafetyCheckCategory = "resource_capacity"
	SafetyCheckTiming           SafetyCheckCategory = "timing"
)

// SafetyCheckStatus defines the status of a safety check
type SafetyCheckStatus string

const (
	SafetyCheckPassed SafetyCheckStatus = "passed"
	SafetyCheckFailed SafetyCheckStatus = "failed"
	SafetyCheckWarn   SafetyCheckStatus = "warning"
)

// RecommendedAction defines what action should be taken
type RecommendedAction string

const (
	ActionProceed     RecommendedAction = "proceed"
	ActionPostpone    RecommendedAction = "postpone"
	ActionReject      RecommendedAction = "reject"
	ActionNeedsReview RecommendedAction = "needs_review"
)

// RebalancePriority defines the priority of rebalancing
type RebalancePriority string

const (
	PriorityHigh   RebalancePriority = "high"
	PriorityMedium RebalancePriority = "medium"
	PriorityLow    RebalancePriority = "low"
)

// PriorityNode represents a node with its priority score for rebalancing order
type PriorityNode struct {
	Node          *CandidateNode
	PriorityScore float64
	Reason        string
}

// RebalancePlan contains the detailed plan for rebalancing
type RebalancePlan struct {
	ID                string
	NodeGroupName     string
	Namespace         string
	Optimization      *cost.Opportunity
	Batches           []NodeBatch
	TotalNodes        int32
	Strategy          RebalanceStrategy
	MaxConcurrent     int32
	RollbackPlan      *RollbackPlan
	EstimatedDuration time.Duration
	CreatedAt         time.Time
}

// NodeBatch represents a batch of nodes to be rebalanced together
type NodeBatch struct {
	BatchNumber       int
	Nodes             []CandidateNode
	EstimatedDuration time.Duration
	DependsOn         []int // Previous batches that must complete
}

// RebalanceStrategy defines the strategy for rebalancing
type RebalanceStrategy string

const (
	StrategyRolling   RebalanceStrategy = "rolling"
	StrategySurge     RebalanceStrategy = "surge"
	StrategyBlueGreen RebalanceStrategy = "blue-green"
)

// RollbackPlan defines how to revert if rebalancing fails
type RollbackPlan struct {
	Steps           []RollbackStep
	AutoRollback    bool
	RollbackTimeout time.Duration
}

// RollbackStep defines a single rollback step
type RollbackStep struct {
	Order       int
	Description string
	Action      string
}

// ExecutionState tracks the state of rebalancing execution
type ExecutionState struct {
	PlanID           string
	Status           ExecutionStatus
	CurrentBatch     int
	CompletedNodes   []string
	FailedNodes      []NodeFailure
	ProvisionedNodes []string
	StartedAt        time.Time
	CompletedAt      *time.Time
	Errors           []error
}

// ExecutionStatus defines the status of rebalancing execution
type ExecutionStatus string

const (
	StatusPending     ExecutionStatus = "pending"
	StatusInProgress  ExecutionStatus = "in_progress"
	StatusPaused      ExecutionStatus = "paused"
	StatusCompleted   ExecutionStatus = "completed"
	StatusFailed      ExecutionStatus = "failed"
	StatusRollingBack ExecutionStatus = "rolling_back"
)

// NodeFailure represents a failed node operation
type NodeFailure struct {
	NodeName  string
	Operation string
	Error     error
	Timestamp time.Time
}

// RebalanceResult contains the results of a rebalancing operation
type RebalanceResult struct {
	PlanID          string
	Status          ExecutionStatus
	NodesRebalanced int32
	NodesFailed     int32
	Duration        time.Duration
	SavingsRealized float64
	Errors          []error
}

// Node represents a Kubernetes node with VPSie metadata
type Node struct {
	Name       string
	VPSID      int
	OfferingID string
	Status     corev1.NodeConditionType
	Age        time.Duration
	Pods       []*corev1.Pod
	Cordoned   bool
	Draining   bool
}

// NodeSpec represents the specification for provisioning a new node
type NodeSpec struct {
	NodeGroupName string
	Namespace     string
	OfferingID    string
	DatacenterID  string
	OSImageID     string
	SSHKeyIDs     []string
	Labels        map[string]string
	Taints        []corev1.Taint
	UserData      string
}

// AnalyzerConfig contains configuration for the rebalance analyzer
type AnalyzerConfig struct {
	// MinHealthyPercent is the minimum percentage of healthy nodes required
	MinHealthyPercent int

	// SkipNodesWithLocalStorage skips nodes with local storage
	SkipNodesWithLocalStorage bool

	// RespectPDBs honors PodDisruptionBudgets
	RespectPDBs bool

	// CooldownPeriod is the minimum time between rebalancing operations
	CooldownPeriod time.Duration

	// MaintenanceWindows defines allowed time windows for rebalancing
	MaintenanceWindows []MaintenanceWindow
}

// MaintenanceWindow defines a time window for allowed operations
type MaintenanceWindow struct {
	Start string   // HH:MM format
	End   string   // HH:MM format
	Days  []string // monday, tuesday, etc.
}

// PlannerConfig contains configuration for the rebalance planner
type PlannerConfig struct {
	// BatchSize is the number of nodes per batch
	BatchSize int

	// MaxConcurrent is the maximum number of nodes to rebalance concurrently
	MaxConcurrent int

	// DrainTimeout is the maximum time to drain a node
	DrainTimeout time.Duration

	// ProvisionTimeout is the maximum time to provision a node
	ProvisionTimeout time.Duration
}

// ExecutorConfig contains configuration for the rebalance executor
type ExecutorConfig struct {
	// DrainTimeout is the maximum time to drain a node
	DrainTimeout time.Duration

	// ProvisionTimeout is the maximum time to provision a node
	ProvisionTimeout time.Duration

	// HealthCheckInterval is the interval between health checks
	HealthCheckInterval time.Duration

	// MaxRetries is the maximum number of retries for failed operations
	MaxRetries int
}
