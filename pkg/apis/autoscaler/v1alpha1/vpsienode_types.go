package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VPSieNodeSpec defines the desired state of VPSieNode
type VPSieNodeSpec struct {
	// VPSieInstanceID is the VPSie VPS instance ID
	// +kubebuilder:validation:Required
	VPSieInstanceID int `json:"vpsieInstanceID"`

	// InstanceType is the VPSie offering/boxsize ID used for this node
	// +kubebuilder:validation:Required
	InstanceType string `json:"instanceType"`

	// NodeGroupName is the name of the NodeGroup this node belongs to
	// +kubebuilder:validation:Required
	NodeGroupName string `json:"nodeGroupName"`

	// DatacenterID is the VPSie datacenter ID where this node is located
	// +kubebuilder:validation:Required
	DatacenterID string `json:"datacenterID"`

	// NodeName is the desired Kubernetes node name
	// If not specified, it will be automatically generated
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// IPAddress is the primary IP address of the VPS
	// +optional
	IPAddress string `json:"ipAddress,omitempty"`

	// IPv6Address is the IPv6 address of the VPS
	// +optional
	IPv6Address string `json:"ipv6Address,omitempty"`
}

// VPSieNodeStatus defines the observed state of VPSieNode
type VPSieNodeStatus struct {
	// Phase represents the current phase of the node lifecycle
	Phase VPSieNodePhase `json:"phase"`

	// NodeName is the actual Kubernetes node name once the node has joined
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// VPSieStatus is the status from VPSie API (running, stopped, suspended, etc.)
	// +optional
	VPSieStatus string `json:"vpsieStatus,omitempty"`

	// Hostname is the hostname of the VPS
	// +optional
	Hostname string `json:"hostname,omitempty"`

	// Resources contains the actual resource capacity of the node
	// +optional
	Resources NodeResources `json:"resources,omitempty"`

	// CreatedAt is when the VPS was created
	// +optional
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`

	// ProvisionedAt is when the VPS was fully provisioned and running
	// +optional
	ProvisionedAt *metav1.Time `json:"provisionedAt,omitempty"`

	// JoinedAt is when the node successfully joined the Kubernetes cluster
	// +optional
	JoinedAt *metav1.Time `json:"joinedAt,omitempty"`

	// ReadyAt is when the node became ready to accept workloads
	// +optional
	ReadyAt *metav1.Time `json:"readyAt,omitempty"`

	// TerminatingAt is when node termination was initiated
	// +optional
	TerminatingAt *metav1.Time `json:"terminatingAt,omitempty"`

	// DeletedAt is when the VPS was deleted
	// +optional
	DeletedAt *metav1.Time `json:"deletedAt,omitempty"`

	// Conditions represent the latest available observations of the node's state
	// +optional
	Conditions []VPSieNodeCondition `json:"conditions,omitempty"`

	// LastError is the last error encountered while managing this node
	// +optional
	LastError string `json:"lastError,omitempty"`

	// ObservedGeneration is the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// VPSieNodePhase represents the lifecycle phase of a VPSieNode
type VPSieNodePhase string

const (
	// VPSieNodePhasePending indicates the node resource has been created but VPS provisioning hasn't started
	VPSieNodePhasePending VPSieNodePhase = "Pending"

	// VPSieNodePhaseProvisioning indicates the VPS is being created on VPSie
	VPSieNodePhaseProvisioning VPSieNodePhase = "Provisioning"

	// VPSieNodePhaseProvisioned indicates the VPS is running but hasn't joined Kubernetes yet
	VPSieNodePhaseProvisioned VPSieNodePhase = "Provisioned"

	// VPSieNodePhaseJoining indicates the node is in the process of joining the Kubernetes cluster
	VPSieNodePhaseJoining VPSieNodePhase = "Joining"

	// VPSieNodePhaseReady indicates the node has joined and is ready to accept workloads
	VPSieNodePhaseReady VPSieNodePhase = "Ready"

	// VPSieNodePhaseTerminating indicates the node is being removed from the cluster
	VPSieNodePhaseTerminating VPSieNodePhase = "Terminating"

	// VPSieNodePhaseDeleting indicates the VPS is being deleted on VPSie
	VPSieNodePhaseDeleting VPSieNodePhase = "Deleting"

	// VPSieNodePhaseFailed indicates the node failed to provision or join the cluster
	VPSieNodePhaseFailed VPSieNodePhase = "Failed"
)

// NodeResources contains the resource capacity of a node
type NodeResources struct {
	// CPU is the number of CPU cores
	CPU int `json:"cpu"`

	// MemoryMB is the amount of memory in megabytes
	MemoryMB int `json:"memoryMB"`

	// DiskGB is the disk size in gigabytes
	DiskGB int `json:"diskGB"`

	// BandwidthGB is the network bandwidth allocation in gigabytes
	// +optional
	BandwidthGB int `json:"bandwidthGB,omitempty"`
}

// VPSieNodeConditionType represents the type of condition
type VPSieNodeConditionType string

const (
	// VPSieNodeConditionVPSReady indicates the VPS is running on VPSie
	VPSieNodeConditionVPSReady VPSieNodeConditionType = "VPSReady"

	// VPSieNodeConditionNodeJoined indicates the node has joined the Kubernetes cluster
	VPSieNodeConditionNodeJoined VPSieNodeConditionType = "NodeJoined"

	// VPSieNodeConditionNodeReady indicates the Kubernetes node is ready
	VPSieNodeConditionNodeReady VPSieNodeConditionType = "NodeReady"

	// VPSieNodeConditionError indicates an error has occurred
	VPSieNodeConditionError VPSieNodeConditionType = "Error"
)

// VPSieNodeCondition describes the state of a VPSieNode at a certain point
type VPSieNodeCondition struct {
	// Type of condition
	Type VPSieNodeConditionType `json:"type"`

	// Status of the condition (True, False, Unknown)
	Status string `json:"status"` // Using string instead of corev1.ConditionStatus to avoid import

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
// +kubebuilder:resource:scope=Namespaced,shortName=vn;vns
// +kubebuilder:printcolumn:name="VPS ID",type=integer,JSONPath=`.spec.vpsieInstanceID`,description="VPSie instance ID"
// +kubebuilder:printcolumn:name="Node",type=string,JSONPath=`.status.nodeName`,description="Kubernetes node name"
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`,description="Node phase"
// +kubebuilder:printcolumn:name="Instance Type",type=string,JSONPath=`.spec.instanceType`,description="VPSie instance type"
// +kubebuilder:printcolumn:name="NodeGroup",type=string,JSONPath=`.spec.nodeGroupName`,description="NodeGroup name"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VPSieNode is the Schema for the vpsienodes API
// It represents a single VPSie VPS instance that is part of a Kubernetes cluster
type VPSieNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VPSieNodeSpec   `json:"spec,omitempty"`
	Status VPSieNodeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VPSieNodeList contains a list of VPSieNode
type VPSieNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VPSieNode `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VPSieNode{}, &VPSieNodeList{})
}
