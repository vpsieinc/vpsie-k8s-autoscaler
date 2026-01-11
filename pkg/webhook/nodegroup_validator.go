package webhook

import (
	"fmt"
	"regexp"

	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
)

const (
	// RequiredNamespace is the only namespace where NodeGroup and VPSieNode resources can be created
	RequiredNamespace = "kube-system"
)

// Package-level compiled regular expressions for validation
var (
	// validDatacenterRegex validates datacenter format (alphanumeric, hyphens, underscores)
	validDatacenterRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

	// validOfferingIDRegex validates offering ID format (alphanumeric, hyphens)
	validOfferingIDRegex = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

	// validKubernetesVersionRegex validates semantic version format (v1.28.0, v1.29.1-rc.0, etc.)
	validKubernetesVersionRegex = regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`)

	// validOSImageRegex validates OS image ID format (alphanumeric, underscores, dots, hyphens)
	validOSImageRegex = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

	// labelKeyRegex validates Kubernetes label key format
	labelKeyRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-_.]*[a-zA-Z0-9])?/)?[a-zA-Z0-9]([a-zA-Z0-9-_.]*[a-zA-Z0-9])?$`)

	// labelValueRegex validates Kubernetes label value format
	labelValueRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-_.]*[a-zA-Z0-9])?$`)
)

// NodeGroupValidator validates NodeGroup resources
type NodeGroupValidator struct {
	logger *zap.Logger
}

// NewNodeGroupValidator creates a new NodeGroup validator
func NewNodeGroupValidator(logger *zap.Logger) *NodeGroupValidator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &NodeGroupValidator{
		logger: logger,
	}
}

// Validate validates a NodeGroup resource
func (v *NodeGroupValidator) Validate(ng *autoscalerv1alpha1.NodeGroup, operation admissionv1.Operation) error {
	v.logger.Debug("validating NodeGroup",
		zap.String("name", ng.Name),
		zap.String("namespace", ng.Namespace),
		zap.String("operation", string(operation)))

	// Common validations for CREATE and UPDATE
	if operation == admissionv1.Create || operation == admissionv1.Update {
		// Validate namespace (must be kube-system)
		if err := v.validateNamespace(ng); err != nil {
			return err
		}

		// Validate min/max nodes
		if err := v.validateNodeCount(ng); err != nil {
			return err
		}

		// Validate datacenter
		if err := v.validateDatacenter(ng); err != nil {
			return err
		}

		// Validate offering IDs
		if err := v.validateOfferingIDs(ng); err != nil {
			return err
		}

		// Validate Kubernetes version
		if err := v.validateKubernetesVersion(ng); err != nil {
			return err
		}

		// Validate OS image
		if err := v.validateOSImage(ng); err != nil {
			return err
		}

		// Validate scale-up policy
		if err := v.validateScaleUpPolicy(ng); err != nil {
			return err
		}

		// Validate scale-down policy
		if err := v.validateScaleDownPolicy(ng); err != nil {
			return err
		}

		// Validate labels
		if err := v.validateLabels(ng); err != nil {
			return err
		}

		// Validate taints
		if err := v.validateTaints(ng); err != nil {
			return err
		}
	}

	// UPDATE-specific validations can be added here if needed in the future

	return nil
}

// validateNodeCount validates min and max node counts
func (v *NodeGroupValidator) validateNodeCount(ng *autoscalerv1alpha1.NodeGroup) error {
	if ng.Spec.MinNodes < 0 {
		return fmt.Errorf("spec.minNodes must be >= 0, got %d", ng.Spec.MinNodes)
	}

	if ng.Spec.MaxNodes < 0 {
		return fmt.Errorf("spec.maxNodes must be >= 0, got %d", ng.Spec.MaxNodes)
	}

	if ng.Spec.MinNodes > ng.Spec.MaxNodes {
		return fmt.Errorf("spec.minNodes (%d) cannot be greater than spec.maxNodes (%d)",
			ng.Spec.MinNodes, ng.Spec.MaxNodes)
	}

	// Reasonable upper limit to prevent misconfiguration
	const maxNodesLimit = 1000
	if ng.Spec.MaxNodes > maxNodesLimit {
		return fmt.Errorf("spec.maxNodes (%d) exceeds maximum allowed value of %d",
			ng.Spec.MaxNodes, maxNodesLimit)
	}

	return nil
}

// validateNamespace validates that the NodeGroup is in the kube-system namespace
func (v *NodeGroupValidator) validateNamespace(ng *autoscalerv1alpha1.NodeGroup) error {
	if ng.Namespace != RequiredNamespace {
		metrics.WebhookNamespaceValidationRejectionsTotal.WithLabelValues("NodeGroup", ng.Namespace).Inc()
		return fmt.Errorf("NodeGroup resources must be created in the %q namespace, got %q",
			RequiredNamespace, ng.Namespace)
	}
	return nil
}

// validateDatacenter validates the datacenter field
func (v *NodeGroupValidator) validateDatacenter(ng *autoscalerv1alpha1.NodeGroup) error {
	if ng.Spec.DatacenterID == "" {
		return fmt.Errorf("spec.datacenterID is required and cannot be empty")
	}

	// Validate datacenter format (alphanumeric, hyphens, underscores)
	if !validDatacenterRegex.MatchString(ng.Spec.DatacenterID) {
		return fmt.Errorf("spec.datacenterID '%s' contains invalid characters (only alphanumeric, hyphens, and underscores allowed)",
			ng.Spec.DatacenterID)
	}

	return nil
}

// validateOfferingIDs validates the offering IDs list
func (v *NodeGroupValidator) validateOfferingIDs(ng *autoscalerv1alpha1.NodeGroup) error {
	if len(ng.Spec.OfferingIDs) == 0 {
		return fmt.Errorf("spec.offeringIds must contain at least one offering ID")
	}

	// Validate each offering ID
	for i, offeringID := range ng.Spec.OfferingIDs {
		if offeringID == "" {
			return fmt.Errorf("spec.offeringIds[%d] cannot be empty", i)
		}

		// Basic format validation (alphanumeric, hyphens)
		if !validOfferingIDRegex.MatchString(offeringID) {
			return fmt.Errorf("spec.offeringIds[%d] '%s' contains invalid characters",
				i, offeringID)
		}
	}

	// Check for duplicates
	seen := make(map[string]bool)
	for i, offeringID := range ng.Spec.OfferingIDs {
		if seen[offeringID] {
			return fmt.Errorf("spec.offeringIds[%d] '%s' is duplicated", i, offeringID)
		}
		seen[offeringID] = true
	}

	return nil
}

// validateKubernetesVersion validates the Kubernetes version
func (v *NodeGroupValidator) validateKubernetesVersion(ng *autoscalerv1alpha1.NodeGroup) error {
	if ng.Spec.KubernetesVersion == "" {
		return fmt.Errorf("spec.kubernetesVersion is required and cannot be empty")
	}

	// Validate semantic version format (v1.28.0, v1.29.1-rc.0, etc.)
	if !validKubernetesVersionRegex.MatchString(ng.Spec.KubernetesVersion) {
		return fmt.Errorf("spec.kubernetesVersion '%s' is not a valid semantic version (expected format: v1.28.0)",
			ng.Spec.KubernetesVersion)
	}

	return nil
}

// validateOSImage validates the OS image ID format if provided
func (v *NodeGroupValidator) validateOSImage(ng *autoscalerv1alpha1.NodeGroup) error {
	// OSImageID is optional - VPSie API will automatically select an appropriate OS image
	if ng.Spec.OSImageID == "" {
		return nil
	}

	// Basic format validation if provided
	if !validOSImageRegex.MatchString(ng.Spec.OSImageID) {
		return fmt.Errorf("spec.osImageId '%s' contains invalid characters", ng.Spec.OSImageID)
	}

	return nil
}

// validateScaleUpPolicy validates the scale-up policy
func (v *NodeGroupValidator) validateScaleUpPolicy(ng *autoscalerv1alpha1.NodeGroup) error {
	policy := ng.Spec.ScaleUpPolicy

	// Validate stabilization window
	if policy.StabilizationWindowSeconds < 0 {
		return fmt.Errorf("spec.scaleUpPolicy.stabilizationWindowSeconds must be >= 0, got %d",
			policy.StabilizationWindowSeconds)
	}

	// Reasonable upper limit (30 minutes)
	if policy.StabilizationWindowSeconds > 1800 {
		return fmt.Errorf("spec.scaleUpPolicy.stabilizationWindowSeconds (%d) exceeds maximum of 1800 seconds",
			policy.StabilizationWindowSeconds)
	}

	// Validate CPU threshold
	if policy.CPUThreshold < 0 || policy.CPUThreshold > 100 {
		return fmt.Errorf("spec.scaleUpPolicy.cpuThreshold must be between 0 and 100, got %d",
			policy.CPUThreshold)
	}

	// Validate memory threshold
	if policy.MemoryThreshold < 0 || policy.MemoryThreshold > 100 {
		return fmt.Errorf("spec.scaleUpPolicy.memoryThreshold must be between 0 and 100, got %d",
			policy.MemoryThreshold)
	}

	return nil
}

// validateScaleDownPolicy validates the scale-down policy
func (v *NodeGroupValidator) validateScaleDownPolicy(ng *autoscalerv1alpha1.NodeGroup) error {
	policy := ng.Spec.ScaleDownPolicy

	// Validate cooldown period
	if policy.CooldownSeconds < 0 {
		return fmt.Errorf("spec.scaleDownPolicy.cooldownSeconds must be >= 0, got %d",
			policy.CooldownSeconds)
	}

	// Reasonable upper limit (1 hour)
	if policy.CooldownSeconds > 3600 {
		return fmt.Errorf("spec.scaleDownPolicy.cooldownSeconds (%d) exceeds maximum of 3600 seconds",
			policy.CooldownSeconds)
	}

	// Validate stabilization window
	if policy.StabilizationWindowSeconds < 0 {
		return fmt.Errorf("spec.scaleDownPolicy.stabilizationWindowSeconds must be >= 0, got %d",
			policy.StabilizationWindowSeconds)
	}

	// Reasonable upper limit (1 hour)
	if policy.StabilizationWindowSeconds > 3600 {
		return fmt.Errorf("spec.scaleDownPolicy.stabilizationWindowSeconds (%d) exceeds maximum of 3600 seconds",
			policy.StabilizationWindowSeconds)
	}

	// Validate CPU threshold
	if policy.CPUThreshold < 0 || policy.CPUThreshold > 100 {
		return fmt.Errorf("spec.scaleDownPolicy.cpuThreshold must be between 0 and 100, got %d",
			policy.CPUThreshold)
	}

	// Validate memory threshold
	if policy.MemoryThreshold < 0 || policy.MemoryThreshold > 100 {
		return fmt.Errorf("spec.scaleDownPolicy.memoryThreshold must be between 0 and 100, got %d",
			policy.MemoryThreshold)
	}

	return nil
}

// validateLabels validates node labels
func (v *NodeGroupValidator) validateLabels(ng *autoscalerv1alpha1.NodeGroup) error {
	for key, value := range ng.Spec.Labels {
		// Validate key format
		if !labelKeyRegex.MatchString(key) {
			return fmt.Errorf("spec.labels key '%s' is not a valid Kubernetes label key", key)
		}

		// Validate key length
		if len(key) > 253 {
			return fmt.Errorf("spec.labels key '%s' exceeds maximum length of 253 characters", key)
		}

		// Validate value format (empty values are allowed)
		if value != "" && !labelValueRegex.MatchString(value) {
			return fmt.Errorf("spec.labels value '%s' for key '%s' is not a valid Kubernetes label value",
				value, key)
		}

		// Validate value length
		if len(value) > 63 {
			return fmt.Errorf("spec.labels value for key '%s' exceeds maximum length of 63 characters", key)
		}

		// Check for reserved prefixes
		if hasReservedPrefix(key) {
			return fmt.Errorf("spec.labels key '%s' uses reserved Kubernetes prefix (kubernetes.io/ or k8s.io/)", key)
		}
	}

	return nil
}

// validateTaints validates node taints
func (v *NodeGroupValidator) validateTaints(ng *autoscalerv1alpha1.NodeGroup) error {
	// Valid taint effects
	validEffects := map[string]bool{
		"NoSchedule":       true,
		"PreferNoSchedule": true,
		"NoExecute":        true,
	}

	for i, taint := range ng.Spec.Taints {
		// Validate key
		if taint.Key == "" {
			return fmt.Errorf("spec.taints[%d].key cannot be empty", i)
		}

		// Validate effect
		if !validEffects[string(taint.Effect)] {
			return fmt.Errorf("spec.taints[%d].effect '%s' is not valid (must be NoSchedule, PreferNoSchedule, or NoExecute)",
				i, taint.Effect)
		}

		// Check for reserved prefixes
		if hasReservedPrefix(taint.Key) {
			return fmt.Errorf("spec.taints[%d].key '%s' uses reserved Kubernetes prefix", i, taint.Key)
		}
	}

	return nil
}

// hasReservedPrefix checks if a label or taint key uses a reserved Kubernetes prefix
func hasReservedPrefix(key string) bool {
	reservedPrefixes := []string{
		"kubernetes.io/",
		"k8s.io/",
	}
	for _, prefix := range reservedPrefixes {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}
