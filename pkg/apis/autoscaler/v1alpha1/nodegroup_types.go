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

	// OfferingIDs is a list of allowed VPSie offering/boxsize IDs for this node group
	// The autoscaler will choose the most cost-effective offering that satisfies resource requirements
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:Required
	OfferingIDs []string `json:"offeringIDs"`

	// OSImageID is the VPSie OS image ID to use for new nodes
	// +kubebuilder:validation:Required
	OSImageID string `json:"osImageID"`

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

	// UserData is cloud-init user data to configure new nodes
	// This should include the script to join the node to the Kubernetes cluster
	// +optional
	UserData string `json:"userData,omitempty"`

	// Tags are key-value pairs to tag VPSie instances for organization and billing
	// +optional
	Tags []string `json:"tags,omitempty"`

	// Notes are additional notes to attach to VPSie instances
	// +optional
	Notes string `json:"notes,omitempty"`
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
