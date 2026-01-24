package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AutoscalerConfigSpec defines the desired configuration for the VPSie autoscaler.
// This CRD serves as the single source of truth for autoscaler configuration,
// including defaults for dynamically created NodeGroups.
type AutoscalerConfigSpec struct {
	// NodeGroupDefaults contains default values for dynamically created NodeGroups.
	// These values are used when the autoscaler creates a new NodeGroup for unschedulable pods.
	// +optional
	NodeGroupDefaults NodeGroupDefaults `json:"nodeGroupDefaults,omitempty"`

	// GlobalSettings contains cluster-wide autoscaler settings
	// +optional
	GlobalSettings GlobalAutoscalerSettings `json:"globalSettings,omitempty"`
}

// NodeGroupDefaults defines default values for dynamically created NodeGroups
type NodeGroupDefaults struct {
	// MinNodes is the minimum number of nodes for new NodeGroups
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=1
	// +optional
	MinNodes int32 `json:"minNodes,omitempty"`

	// MaxNodes is the maximum number of nodes for new NodeGroups
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=10
	// +optional
	MaxNodes int32 `json:"maxNodes,omitempty"`

	// Namespace is the namespace where NodeGroups will be created
	// +kubebuilder:default="default"
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// DatacenterID is the default VPSie datacenter ID
	// +optional
	DatacenterID string `json:"datacenterID,omitempty"`

	// OfferingIDs is a list of allowed VPSie offering/boxsize IDs
	// +optional
	OfferingIDs []string `json:"offeringIDs,omitempty"`

	// ResourceIdentifier is the VPSie Kubernetes cluster identifier
	// +optional
	ResourceIdentifier string `json:"resourceIdentifier,omitempty"`

	// Project is the VPSie project ID
	// +optional
	Project string `json:"project,omitempty"`

	// OSImageID is the VPSie OS image ID for new nodes
	// +optional
	OSImageID string `json:"osImageID,omitempty"`

	// KubernetesVersion is the Kubernetes version for new nodes
	// +optional
	KubernetesVersion string `json:"kubernetesVersion,omitempty"`

	// KubeSizeID is the default VPSie Kubernetes size/package ID
	// +optional
	KubeSizeID int `json:"kubeSizeID,omitempty"`

	// Labels are additional labels to apply to all dynamically created nodes
	// These are merged with any labels derived from pod NodeSelectors
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Taints are default taints to apply to nodes in dynamic NodeGroups
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`

	// ScaleUpPolicy defines the default scale-up behavior
	// +optional
	ScaleUpPolicy ScaleUpPolicy `json:"scaleUpPolicy,omitempty"`

	// ScaleDownPolicy defines the default scale-down behavior
	// +optional
	ScaleDownPolicy ScaleDownPolicy `json:"scaleDownPolicy,omitempty"`

	// CostOptimization defines default cost optimization settings
	// +optional
	CostOptimization *CostOptimizationConfig `json:"costOptimization,omitempty"`

	// SSHKeyIDs is a list of VPSie SSH key IDs for new nodes
	// +optional
	SSHKeyIDs []string `json:"sshKeyIDs,omitempty"`

	// Tags are key-value pairs to tag VPSie instances
	// +optional
	Tags []string `json:"tags,omitempty"`

	// Notes are additional notes for VPSie instances
	// +optional
	Notes string `json:"notes,omitempty"`

	// SpotConfig defines default spot instance configuration
	// +optional
	SpotConfig *SpotInstanceConfig `json:"spotConfig,omitempty"`
}

// GlobalAutoscalerSettings contains cluster-wide autoscaler configuration
type GlobalAutoscalerSettings struct {
	// ScaleUpCooldownSeconds is the minimum time between scale-up operations (cluster-wide)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=60
	// +optional
	ScaleUpCooldownSeconds int32 `json:"scaleUpCooldownSeconds,omitempty"`

	// ScaleDownCooldownSeconds is the minimum time between scale-down operations (cluster-wide)
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=300
	// +optional
	ScaleDownCooldownSeconds int32 `json:"scaleDownCooldownSeconds,omitempty"`

	// MaxConcurrentScaleUps is the maximum number of nodes that can be provisioned simultaneously
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	MaxConcurrentScaleUps int32 `json:"maxConcurrentScaleUps,omitempty"`

	// MaxConcurrentScaleDowns is the maximum number of nodes that can be terminated simultaneously
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	// +optional
	MaxConcurrentScaleDowns int32 `json:"maxConcurrentScaleDowns,omitempty"`

	// MaxClusterWorkers is the maximum total number of worker nodes allowed across all NodeGroups.
	// This is a cluster-wide limit that prevents the autoscaler from exceeding VPSie cluster capacity.
	// Set to 0 for unlimited (not recommended).
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=10
	// +optional
	MaxClusterWorkers int32 `json:"maxClusterWorkers,omitempty"`

	// UnschedulablePodGracePeriodSeconds is how long to wait for a pod to be scheduled
	// before considering it for scale-up
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=30
	// +optional
	UnschedulablePodGracePeriodSeconds int32 `json:"unschedulablePodGracePeriodSeconds,omitempty"`

	// EnableDynamicNodeGroupCreation controls whether the autoscaler can create new NodeGroups
	// +kubebuilder:default=true
	// +optional
	EnableDynamicNodeGroupCreation bool `json:"enableDynamicNodeGroupCreation,omitempty"`

	// EnableRebalancing controls whether the autoscaler can rebalance nodes for cost optimization
	// +kubebuilder:default=true
	// +optional
	EnableRebalancing bool `json:"enableRebalancing,omitempty"`

	// NodeReadyTimeoutSeconds is how long to wait for a new node to become ready
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:default=600
	// +optional
	NodeReadyTimeoutSeconds int32 `json:"nodeReadyTimeoutSeconds,omitempty"`

	// PodEvictionTimeoutSeconds is the maximum time to wait for pods to be evicted during scale-down
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=120
	// +optional
	PodEvictionTimeoutSeconds int32 `json:"podEvictionTimeoutSeconds,omitempty"`
}

// AutoscalerConfigStatus defines the observed state of AutoscalerConfig
type AutoscalerConfigStatus struct {
	// ObservedGeneration is the generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastUpdated is when the configuration was last applied
	// +optional
	LastUpdated *metav1.Time `json:"lastUpdated,omitempty"`

	// Active indicates whether this configuration is currently active
	// +optional
	Active bool `json:"active,omitempty"`

	// Message provides additional information about the configuration status
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=asc
// +kubebuilder:printcolumn:name="MaxWorkers",type=integer,JSONPath=`.spec.globalSettings.maxClusterWorkers`,description="Max cluster workers"
// +kubebuilder:printcolumn:name="MinNodes",type=integer,JSONPath=`.spec.nodeGroupDefaults.minNodes`,description="Default min nodes"
// +kubebuilder:printcolumn:name="MaxNodes",type=integer,JSONPath=`.spec.nodeGroupDefaults.maxNodes`,description="Default max nodes"
// +kubebuilder:printcolumn:name="DynamicCreation",type=boolean,JSONPath=`.spec.globalSettings.enableDynamicNodeGroupCreation`,description="Dynamic NodeGroup creation enabled"
// +kubebuilder:printcolumn:name="Active",type=boolean,JSONPath=`.status.active`,description="Configuration active"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// AutoscalerConfig is the Schema for the autoscalerconfigs API.
// It provides cluster-wide configuration for the VPSie autoscaler,
// including defaults for dynamically created NodeGroups.
// Only one AutoscalerConfig should exist per cluster (named "default").
type AutoscalerConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   AutoscalerConfigSpec   `json:"spec,omitempty"`
	Status AutoscalerConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// AutoscalerConfigList contains a list of AutoscalerConfig
type AutoscalerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []AutoscalerConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&AutoscalerConfig{}, &AutoscalerConfigList{})
}
