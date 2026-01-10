package events

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
)

// DynamicNodeGroupCreator creates NodeGroups dynamically when no suitable managed NodeGroup exists.
// Created NodeGroups are always marked with the managed label (autoscaler.vpsie.com/managed=true)
// to ensure they are processed by the autoscaler.
type DynamicNodeGroupCreator struct {
	client   client.Client
	logger   *zap.Logger
	template *NodeGroupTemplate
}

// NodeGroupTemplate provides default values for dynamically created NodeGroups
type NodeGroupTemplate struct {
	// Namespace is the namespace where NodeGroups will be created
	Namespace string

	// MinNodes is the minimum number of nodes (default: 1)
	MinNodes int32

	// MaxNodes is the maximum number of nodes (default: 10)
	MaxNodes int32

	// DefaultOfferingIDs are the VPSie offering IDs to use if not specified
	DefaultOfferingIDs []string

	// DefaultDatacenterID is the datacenter to use if not specified
	DefaultDatacenterID string

	// ResourceIdentifier is the VPSie Kubernetes cluster identifier
	ResourceIdentifier string

	// Project is the VPSie project ID
	Project string

	// OSImageID is the VPSie OS image ID to use for new nodes
	OSImageID string

	// KubernetesVersion is the Kubernetes version to install on new nodes
	KubernetesVersion string
}

// DefaultNodeGroupTemplate returns a template with sensible defaults
func DefaultNodeGroupTemplate() *NodeGroupTemplate {
	return &NodeGroupTemplate{
		Namespace:           "default",
		MinNodes:            1,
		MaxNodes:            10,
		DefaultOfferingIDs:  []string{},
		DefaultDatacenterID: "",
		ResourceIdentifier:  "",
		Project:             "",
		OSImageID:           "",
		KubernetesVersion:   "",
	}
}

// NewDynamicNodeGroupCreator creates a new DynamicNodeGroupCreator
func NewDynamicNodeGroupCreator(
	client client.Client,
	logger *zap.Logger,
	template *NodeGroupTemplate,
) *DynamicNodeGroupCreator {
	if template == nil {
		template = DefaultNodeGroupTemplate()
	}

	return &DynamicNodeGroupCreator{
		client:   client,
		logger:   logger.Named("dynamic-nodegroup-creator"),
		template: template,
	}
}

// FindSuitableNodeGroup finds a managed NodeGroup that can satisfy the pod's requirements.
// Returns nil if no suitable NodeGroup exists.
func (c *DynamicNodeGroupCreator) FindSuitableNodeGroup(
	ctx context.Context,
	pod *corev1.Pod,
	nodeGroups []v1alpha1.NodeGroup,
) *v1alpha1.NodeGroup {
	for i := range nodeGroups {
		ng := &nodeGroups[i]

		// Skip unmanaged NodeGroups (defense in depth - caller should already filter)
		if !v1alpha1.IsManagedNodeGroup(ng) {
			continue
		}

		// Check if NodeGroup can accommodate the pod
		if c.nodeGroupMatchesPod(ng, pod) {
			return ng
		}
	}

	return nil
}

// nodeGroupMatchesPod checks if a NodeGroup can satisfy a pod's scheduling requirements
func (c *DynamicNodeGroupCreator) nodeGroupMatchesPod(ng *v1alpha1.NodeGroup, pod *corev1.Pod) bool {
	// Check if NodeGroup has capacity
	if ng.Status.DesiredNodes >= ng.Spec.MaxNodes {
		return false
	}

	// Check node selector requirements
	if len(pod.Spec.NodeSelector) > 0 {
		// Pod has node selector - NodeGroup must have matching labels
		if len(ng.Spec.Labels) == 0 {
			return false
		}
		for key, value := range pod.Spec.NodeSelector {
			if ngValue, exists := ng.Spec.Labels[key]; !exists || ngValue != value {
				return false
			}
		}
	} else {
		// Pod has no node selector - only match generic NodeGroups (no labels)
		if len(ng.Spec.Labels) > 0 {
			return false
		}
	}

	// Check taints/tolerations
	if len(ng.Spec.Taints) > 0 {
		if !c.podToleratesTaints(pod, ng.Spec.Taints) {
			return false
		}
	}

	return true
}

// podToleratesTaints checks if a pod tolerates all the given taints
func (c *DynamicNodeGroupCreator) podToleratesTaints(pod *corev1.Pod, taints []corev1.Taint) bool {
	for _, taint := range taints {
		tolerated := false
		for _, toleration := range pod.Spec.Tolerations {
			if toleration.ToleratesTaint(&taint) {
				tolerated = true
				break
			}
		}
		if !tolerated {
			return false
		}
	}
	return true
}

// ValidateTemplate checks if the template has all required fields for creating NodeGroups.
// Returns an error if required fields are missing.
func (c *DynamicNodeGroupCreator) ValidateTemplate() error {
	if c.template.DefaultDatacenterID == "" {
		return fmt.Errorf("template validation failed: DefaultDatacenterID is required")
	}
	if len(c.template.DefaultOfferingIDs) == 0 {
		return fmt.Errorf("template validation failed: at least one DefaultOfferingID is required")
	}
	if c.template.ResourceIdentifier == "" {
		return fmt.Errorf("template validation failed: ResourceIdentifier is required")
	}
	return nil
}

// CreateNodeGroupForPod creates a new NodeGroup to satisfy the pod's requirements.
// The created NodeGroup is always marked with the managed label.
// Returns an error if the template is not properly configured.
func (c *DynamicNodeGroupCreator) CreateNodeGroupForPod(
	ctx context.Context,
	pod *corev1.Pod,
	namespace string,
) (*v1alpha1.NodeGroup, error) {
	// Validate template before creating NodeGroup
	if err := c.ValidateTemplate(); err != nil {
		return nil, err
	}

	if namespace == "" {
		namespace = c.template.Namespace
	}

	// Generate unique name
	name := c.generateNodeGroupName()

	c.logger.Info("Creating dynamic NodeGroup for pod",
		zap.String("nodeGroup", name),
		zap.String("pod", pod.Name),
		zap.String("namespace", namespace),
	)

	// Build NodeGroup spec based on pod requirements
	spec := c.buildNodeGroupSpec(pod)

	ng := &v1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				v1alpha1.ManagedLabelKey: v1alpha1.ManagedLabelValue,
			},
		},
		Spec: spec,
	}

	// Create the NodeGroup
	if err := c.client.Create(ctx, ng); err != nil {
		metrics.DynamicNodeGroupCreationsTotal.WithLabelValues("failure", namespace).Inc()
		return nil, fmt.Errorf("failed to create NodeGroup: %w", err)
	}

	metrics.DynamicNodeGroupCreationsTotal.WithLabelValues("success", namespace).Inc()

	c.logger.Info("Created dynamic NodeGroup",
		zap.String("nodeGroup", name),
		zap.String("namespace", namespace),
		zap.Int32("minNodes", spec.MinNodes),
		zap.Int32("maxNodes", spec.MaxNodes),
	)

	return ng, nil
}

// generateNodeGroupName generates a unique name for a dynamically created NodeGroup.
// Uses UnixNano timestamp to prevent collisions when multiple NodeGroups are created
// within the same second.
func (c *DynamicNodeGroupCreator) generateNodeGroupName() string {
	timestamp := time.Now().UnixNano()
	datacenter := c.template.DefaultDatacenterID
	if datacenter == "" {
		datacenter = "default"
	}
	// Use last 10 digits of nanoseconds for reasonable uniqueness while keeping name short
	return fmt.Sprintf("auto-%s-%d", datacenter, timestamp%10000000000)
}

// buildNodeGroupSpec builds a NodeGroup spec based on pod requirements
func (c *DynamicNodeGroupCreator) buildNodeGroupSpec(pod *corev1.Pod) v1alpha1.NodeGroupSpec {
	spec := v1alpha1.NodeGroupSpec{
		MinNodes:           c.template.MinNodes,
		MaxNodes:           c.template.MaxNodes,
		OfferingIDs:        c.template.DefaultOfferingIDs,
		DatacenterID:       c.template.DefaultDatacenterID,
		ResourceIdentifier: c.template.ResourceIdentifier,
		Project:            c.template.Project,
		OSImageID:          c.template.OSImageID,
		KubernetesVersion:  c.template.KubernetesVersion,
	}

	// Copy node selector labels to NodeGroup spec
	if len(pod.Spec.NodeSelector) > 0 {
		spec.Labels = make(map[string]string)
		for key, value := range pod.Spec.NodeSelector {
			spec.Labels[key] = value
		}
	}

	// Extract tolerations that might indicate required taints
	// Note: We only add taints for explicitly requested tolerations, not wildcard ones
	taints := c.extractRequiredTaints(pod.Spec.Tolerations)
	if len(taints) > 0 {
		spec.Taints = taints
	}

	return spec
}

// extractRequiredTaints extracts taints from pod tolerations that indicate explicit taint requirements.
// This is a heuristic - not all tolerations indicate a desire for the taint.
func (c *DynamicNodeGroupCreator) extractRequiredTaints(tolerations []corev1.Toleration) []corev1.Taint {
	taints := make([]corev1.Taint, 0)

	for _, toleration := range tolerations {
		// Skip empty/wildcard tolerations
		if toleration.Key == "" {
			continue
		}

		// Skip common system tolerations
		if isSystemToleration(toleration.Key) {
			continue
		}

		// Convert toleration to taint
		taint := corev1.Taint{
			Key:    toleration.Key,
			Effect: toleration.Effect,
		}

		// Only add value if operator is Equal
		if toleration.Operator == corev1.TolerationOpEqual {
			taint.Value = toleration.Value
		}

		taints = append(taints, taint)
	}

	return taints
}

// isSystemToleration checks if a toleration key is a common system toleration
func isSystemToleration(key string) bool {
	systemKeys := []string{
		"node.kubernetes.io/not-ready",
		"node.kubernetes.io/unreachable",
		"node.kubernetes.io/memory-pressure",
		"node.kubernetes.io/disk-pressure",
		"node.kubernetes.io/pid-pressure",
		"node.kubernetes.io/unschedulable",
		"node.kubernetes.io/network-unavailable",
		"node.cloudprovider.kubernetes.io/uninitialized",
	}

	for _, sysKey := range systemKeys {
		if key == sysKey {
			return true
		}
	}
	return false
}

// SetTemplate updates the template used for creating NodeGroups
func (c *DynamicNodeGroupCreator) SetTemplate(template *NodeGroupTemplate) {
	if template != nil {
		c.template = template
	}
}
