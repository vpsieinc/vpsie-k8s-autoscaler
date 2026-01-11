package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NodeGroupSpec defines the desired state of NodeGroup
type NodeGroupSpec struct {
	// MinNodes is the minimum number of nodes in this group
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Required
	MinNodes int32 `json:"minNodes"`

	// MaxNodes is the maximum number of nodes in this group
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Required
	MaxNodes int32 `json:"maxNodes"`

	// DatacenterID is the VPSie datacenter ID where nodes will be created
	// +kubebuilder:validation:Required
	DatacenterID string `json:"datacenterID"`

	// ResourceIdentifier is the VPSie Kubernetes cluster identifier
	// Required for the VPSie Kubernetes apps API to know which cluster to add nodes to
	// +kubebuilder:validation:Required
	ResourceIdentifier string `json:"resourceIdentifier"`

	// Project is the VPSie project ID
	// Required for the VPSie Kubernetes apps API
	// +kubebuilder:validation:Required
	Project string `json:"project"`

	// OfferingIDs is a list of allowed VPSie offering/boxsize IDs for this node group
	// The autoscaler will choose the most cost-effective offering that satisfies resource requirements
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:Required
	OfferingIDs []string `json:"offeringIDs"`

	// OSImageID is the VPSie OS image ID to use for new nodes
	// Optional: VPSie API will automatically select an appropriate OS image if not specified
	// +kubebuilder:validation:Optional
	OSImageID string `json:"osImageID,omitempty"`

	// KubernetesVersion is the Kubernetes version to install on new nodes (e.g., "v1.28.0", "v1.29.1")
	// Must be within ±1 minor version of the control plane
	// +kubebuilder:validation:Pattern=`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`
	// +kubebuilder:validation:Required
	KubernetesVersion string `json:"kubernetesVersion"`

	// PreferredInstanceType is the preferred offering ID to use when multiple options are available
	// +optional
	PreferredInstanceType string `json:"preferredInstanceType,omitempty"`

	// AllowMixedInstances allows the node group to contain nodes with different instance types
	// +kubebuilder:default=true
	// +optional
	AllowMixedInstances bool `json:"allowMixedInstances,omitempty"`

	// Labels are the Kubernetes labels to apply to nodes in this group
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Taints are the Kubernetes taints to apply to nodes in this group
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`

	// ScaleUpPolicy defines when and how to scale up the node group
	// +optional
	ScaleUpPolicy ScaleUpPolicy `json:"scaleUpPolicy,omitempty"`

	// ScaleDownPolicy defines when and how to scale down the node group
	// +optional
	ScaleDownPolicy ScaleDownPolicy `json:"scaleDownPolicy,omitempty"`

	// SSHKeyIDs is a list of VPSie SSH key IDs to install on new nodes
	// +optional
	SSHKeyIDs []string `json:"sshKeyIDs,omitempty"`

	// Tags are key-value pairs to tag VPSie instances for organization and billing
	// +optional
	Tags []string `json:"tags,omitempty"`

	// Notes are additional notes to attach to VPSie instances
	// +optional
	Notes string `json:"notes,omitempty"`

	// KubeSizeID is the VPSie Kubernetes size/package ID for nodes in this group
	// Get available values from VPSie API /k8s/offers endpoint
	// +kubebuilder:validation:Minimum=1
	// +optional
	KubeSizeID int `json:"kubeSizeID,omitempty"`

	// SpotConfig defines spot instance configuration for cost savings
	// +optional
	SpotConfig *SpotInstanceConfig `json:"spotConfig,omitempty"`

	// MultiRegion enables multi-region/datacenter distribution for high availability
	// +optional
	MultiRegion *MultiRegionConfig `json:"multiRegion,omitempty"`

	// CostOptimization defines cost optimization settings for this NodeGroup
	// +optional
	CostOptimization *CostOptimizationConfig `json:"costOptimization,omitempty"`
}

// ScaleUpPolicy defines the scale-up behavior for a NodeGroup
type ScaleUpPolicy struct {
	// StabilizationWindowSeconds is the time to wait before scaling up after conditions are met
	// This prevents flapping when resource usage fluctuates
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=60
	// +optional
	StabilizationWindowSeconds int32 `json:"stabilizationWindowSeconds,omitempty"`

	// CPUThreshold is the CPU utilization percentage that triggers scale-up
	// Scale up when average CPU usage across nodes exceeds this threshold
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=80
	// +optional
	CPUThreshold int32 `json:"cpuThreshold,omitempty"`

	// MemoryThreshold is the memory utilization percentage that triggers scale-up
	// Scale up when average memory usage across nodes exceeds this threshold
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=80
	// +optional
	MemoryThreshold int32 `json:"memoryThreshold,omitempty"`

	// Enabled controls whether automatic scale-up is enabled
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`
}

// SpotInstanceConfig defines configuration for spot instances
type SpotInstanceConfig struct {
	// Enabled controls whether spot instances are used
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// MaxSpotPercentage is the maximum percentage of nodes that can be spot instances
	// This ensures some on-demand capacity for stability
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=80
	// +optional
	MaxSpotPercentage int32 `json:"maxSpotPercentage,omitempty"`

	// FallbackToOnDemand controls whether to fall back to on-demand if spot unavailable
	// +kubebuilder:default=true
	// +optional
	FallbackToOnDemand bool `json:"fallbackToOnDemand,omitempty"`

	// InterruptionGracePeriod is the time before spot termination to drain workloads
	// +kubebuilder:default="120s"
	// +optional
	InterruptionGracePeriod string `json:"interruptionGracePeriod,omitempty"`

	// AllowedInterruptionRate is the maximum interruption rate per hour
	// Helps avoid offerings with high interruption rates
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=20
	// +optional
	AllowedInterruptionRate int32 `json:"allowedInterruptionRate,omitempty"`
}

// MultiRegionConfig defines multi-region distribution configuration
type MultiRegionConfig struct {
	// Enabled controls whether multi-region distribution is active
	// +kubebuilder:default=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// DatacenterIDs is a list of datacenter IDs to distribute nodes across
	// If empty, uses the primary DatacenterID from NodeGroupSpec
	// +optional
	DatacenterIDs []string `json:"datacenterIDs,omitempty"`

	// DistributionStrategy defines how to distribute nodes across regions
	// Values: "balanced", "weighted", "primary-backup"
	// +kubebuilder:default="balanced"
	// +optional
	DistributionStrategy string `json:"distributionStrategy,omitempty"`

	// WeightedDistribution defines custom weights for each datacenter
	// Only used when DistributionStrategy is "weighted"
	// Key is datacenter ID, value is weight (higher = more nodes)
	// +optional
	WeightedDistribution map[string]int32 `json:"weightedDistribution,omitempty"`

	// PrimaryDatacenter is the primary datacenter for "primary-backup" strategy
	// +optional
	PrimaryDatacenter string `json:"primaryDatacenter,omitempty"`

	// MinNodesPerRegion is the minimum number of nodes per region
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	MinNodesPerRegion int32 `json:"minNodesPerRegion,omitempty"`
}

// CostOptimizationConfig defines cost optimization settings for a NodeGroup
type CostOptimizationConfig struct {
	// Enabled controls whether cost optimization is active for this NodeGroup
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// Strategy defines the optimization strategy
	// Values: "auto", "manual", "aggressive", "conservative"
	// +kubebuilder:default="auto"
	// +optional
	Strategy string `json:"strategy,omitempty"`

	// OptimizationInterval is the minimum time between optimization actions
	// +kubebuilder:default="24h"
	// +optional
	OptimizationInterval string `json:"optimizationInterval,omitempty"`

	// MinMonthlySavings is the minimum monthly savings required to apply optimization
	// +kubebuilder:default=10.0
	// +kubebuilder:validation:Type=number
	// +optional
	MinMonthlySavings float64 `json:"minMonthlySavings,omitempty"`

	// MaxPerformanceImpact is the maximum acceptable performance reduction percentage
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=5
	// +optional
	MaxPerformanceImpact int32 `json:"maxPerformanceImpact,omitempty"`

	// RequireApproval controls whether optimizations require manual approval
	// +kubebuilder:default=false
	// +optional
	RequireApproval bool `json:"requireApproval,omitempty"`
}

// ScaleDownPolicy defines the scale-down behavior for a NodeGroup
type ScaleDownPolicy struct {
	// StabilizationWindowSeconds is the time to wait before scaling down after conditions are met
	// This prevents premature scale-down and gives time for resource usage patterns to stabilize
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=600
	// +optional
	StabilizationWindowSeconds int32 `json:"stabilizationWindowSeconds,omitempty"`

	// CPUThreshold is the CPU utilization percentage below which scale-down is considered
	// Scale down when node CPU usage is below this threshold for the stabilization window
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=50
	// +optional
	CPUThreshold int32 `json:"cpuThreshold,omitempty"`

	// MemoryThreshold is the memory utilization percentage below which scale-down is considered
	// Scale down when node memory usage is below this threshold for the stabilization window
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=50
	// +optional
	MemoryThreshold int32 `json:"memoryThreshold,omitempty"`

	// Enabled controls whether automatic scale-down is enabled
	// +kubebuilder:default=true
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// CooldownSeconds is the time to wait after a scale-up before allowing scale-down
	// This prevents scaling down immediately after scaling up
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=600
	// +optional
	CooldownSeconds int32 `json:"cooldownSeconds,omitempty"`
}

// InstanceTypeInfo contains information about a VPSie offering/instance type
type InstanceTypeInfo struct {
	// OfferingID is the VPSie offering ID
	OfferingID string `json:"offeringID"`

	// CPU is the number of CPU cores
	CPU int `json:"cpu"`

	// MemoryMB is the amount of memory in megabytes
	MemoryMB int `json:"memoryMB"`

	// DiskGB is the disk size in gigabytes
	DiskGB int `json:"diskGB"`
}

// NodeGroupStatus defines the observed state of NodeGroup
type NodeGroupStatus struct {
	// CurrentNodes is the actual number of nodes currently in the group
	CurrentNodes int32 `json:"currentNodes"`

	// DesiredNodes is the number of nodes the autoscaler wants to maintain
	DesiredNodes int32 `json:"desiredNodes"`

	// ReadyNodes is the number of nodes that are ready to accept workloads
	ReadyNodes int32 `json:"readyNodes"`

	// VPSieGroupID is the numeric VPSie node group ID created on VPSie platform
	// This is the numeric ID returned by ListK8sNodeGroups, used for adding nodes
	// +optional
	VPSieGroupID int `json:"vpsieGroupID,omitempty"`

	// Nodes is a list of nodes in this group with their details
	// +optional
	Nodes []NodeInfo `json:"nodes,omitempty"`

	// Conditions represent the latest available observations of the NodeGroup's state
	// +optional
	Conditions []NodeGroupCondition `json:"conditions,omitempty"`

	// LastScaleTime is the timestamp of the last scaling operation
	// +optional
	LastScaleTime *metav1.Time `json:"lastScaleTime,omitempty"`

	// LastScaleUpTime is the timestamp of the last scale-up operation
	// +optional
	LastScaleUpTime *metav1.Time `json:"lastScaleUpTime,omitempty"`

	// LastScaleDownTime is the timestamp of the last scale-down operation
	// +optional
	LastScaleDownTime *metav1.Time `json:"lastScaleDownTime,omitempty"`

	// ObservedGeneration is the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// NodeInfo contains information about a node in the NodeGroup
type NodeInfo struct {
	// NodeName is the Kubernetes node name
	NodeName string `json:"nodeName"`

	// VPSID is the VPSie instance ID
	VPSID int `json:"vpsID"`

	// InstanceType is the VPSie offering/boxsize ID
	InstanceType string `json:"instanceType"`

	// Status is the current status of the node (Provisioning, Ready, Terminating, etc.)
	Status string `json:"status"`

	// CreatedAt is when the node was created
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// ReadyAt is when the node became ready
	// +optional
	ReadyAt *metav1.Time `json:"readyAt,omitempty"`

	// IPAddress is the primary IP address of the node
	// +optional
	IPAddress string `json:"ipAddress,omitempty"`
}

// NodeGroupConditionType represents the type of condition
type NodeGroupConditionType string

const (
	// NodeGroupReady indicates the node group is healthy and operating normally
	NodeGroupReady NodeGroupConditionType = "Ready"

	// NodeGroupScaling indicates the node group is currently scaling
	NodeGroupScaling NodeGroupConditionType = "Scaling"

	// NodeGroupError indicates the node group has encountered an error
	NodeGroupError NodeGroupConditionType = "Error"

	// NodeGroupAtMinCapacity indicates the node group is at minimum capacity
	NodeGroupAtMinCapacity NodeGroupConditionType = "AtMinCapacity"

	// NodeGroupAtMaxCapacity indicates the node group is at maximum capacity
	NodeGroupAtMaxCapacity NodeGroupConditionType = "AtMaxCapacity"
)

// NodeGroupCondition describes the state of a NodeGroup at a certain point
type NodeGroupCondition struct {
	// Type of condition
	Type NodeGroupConditionType `json:"type"`

	// Status of the condition (True, False, Unknown)
	Status corev1.ConditionStatus `json:"status"`

	// LastTransitionTime is the last time the condition transitioned from one status to another
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// LastUpdateTime is the last time this condition was updated
	// +optional
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`

	// Reason is a one-word CamelCase reason for the condition's last transition
	// +optional
	Reason string `json:"reason,omitempty"`

	// Message is a human-readable message indicating details about last transition
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=ng;ngs
// +kubebuilder:printcolumn:name="Min",type=integer,JSONPath=`.spec.minNodes`,description="Minimum nodes"
// +kubebuilder:printcolumn:name="Max",type=integer,JSONPath=`.spec.maxNodes`,description="Maximum nodes"
// +kubebuilder:printcolumn:name="Desired",type=integer,JSONPath=`.status.desiredNodes`,description="Desired nodes"
// +kubebuilder:printcolumn:name="Current",type=integer,JSONPath=`.status.currentNodes`,description="Current nodes"
// +kubebuilder:printcolumn:name="Ready",type=integer,JSONPath=`.status.readyNodes`,description="Ready nodes"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// NodeGroup is the Schema for the nodegroups API
type NodeGroup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NodeGroupSpec   `json:"spec,omitempty"`
	Status NodeGroupStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// NodeGroupList contains a list of NodeGroup
type NodeGroupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NodeGroup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NodeGroup{}, &NodeGroupList{})
}
