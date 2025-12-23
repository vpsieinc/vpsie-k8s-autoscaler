package webhook

import (
	"fmt"
	"regexp"

	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// VPSieNodeValidator validates VPSieNode resources
type VPSieNodeValidator struct {
	logger *zap.Logger
}

// NewVPSieNodeValidator creates a new VPSieNode validator
func NewVPSieNodeValidator(logger *zap.Logger) *VPSieNodeValidator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &VPSieNodeValidator{
		logger: logger,
	}
}

// Validate validates a VPSieNode resource
func (v *VPSieNodeValidator) Validate(vn *autoscalerv1alpha1.VPSieNode, operation admissionv1.Operation) error {
	v.logger.Debug("validating VPSieNode",
		zap.String("name", vn.Name),
		zap.String("namespace", vn.Namespace),
		zap.String("operation", string(operation)))

	// Common validations for CREATE and UPDATE
	if operation == admissionv1.Create || operation == admissionv1.Update {
		// Validate NodeGroup reference
		if err := v.validateNodeGroupRef(vn); err != nil {
			return err
		}

		// Validate datacenter
		if err := v.validateDatacenter(vn); err != nil {
			return err
		}

		// Validate offering ID
		if err := v.validateOfferingID(vn); err != nil {
			return err
		}

		// Validate Kubernetes version
		if err := v.validateKubernetesVersion(vn); err != nil {
			return err
		}

		// Validate OS image
		if err := v.validateOSImage(vn); err != nil {
			return err
		}

		// Validate SSH key IDs
		if err := v.validateSSHKeyIDs(vn); err != nil {
			return err
		}

		// User data/cloud-init validation was removed in v0.6.0
		// Node configuration is now handled entirely by VPSie API
	}

	// UPDATE-specific validations
	if operation == admissionv1.Update {
		// Validate immutable fields
		if err := v.validateImmutableFields(vn); err != nil {
			return err
		}
	}

	return nil
}

// validateNodeGroupRef validates the NodeGroup reference
func (v *VPSieNodeValidator) validateNodeGroupRef(vn *autoscalerv1alpha1.VPSieNode) error {
	if vn.Spec.NodeGroupName == "" {
		return fmt.Errorf("spec.nodeGroupName is required and cannot be empty")
	}

	// Validate Kubernetes resource name format
	validName := regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)
	if !validName.MatchString(vn.Spec.NodeGroupName) {
		return fmt.Errorf("spec.nodeGroupName '%s' is not a valid Kubernetes resource name",
			vn.Spec.NodeGroupName)
	}

	// Validate length
	if len(vn.Spec.NodeGroupName) > 253 {
		return fmt.Errorf("spec.nodeGroupName '%s' exceeds maximum length of 253 characters",
			vn.Spec.NodeGroupName)
	}

	return nil
}

// validateDatacenter validates the datacenter field
func (v *VPSieNodeValidator) validateDatacenter(vn *autoscalerv1alpha1.VPSieNode) error {
	if vn.Spec.DatacenterID == "" {
		return fmt.Errorf("spec.datacenterID is required and cannot be empty")
	}

	// Validate datacenter format
	validDatacenter := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validDatacenter.MatchString(vn.Spec.DatacenterID) {
		return fmt.Errorf("spec.datacenterID '%s' contains invalid characters", vn.Spec.DatacenterID)
	}

	return nil
}

// validateOfferingID validates the offering ID (instance type)
func (v *VPSieNodeValidator) validateOfferingID(vn *autoscalerv1alpha1.VPSieNode) error {
	if vn.Spec.InstanceType == "" {
		return fmt.Errorf("spec.instanceType is required and cannot be empty")
	}

	// Basic format validation
	validInstanceType := regexp.MustCompile(`^[a-zA-Z0-9-]+$`)
	if !validInstanceType.MatchString(vn.Spec.InstanceType) {
		return fmt.Errorf("spec.instanceType '%s' contains invalid characters", vn.Spec.InstanceType)
	}

	return nil
}

// validateKubernetesVersion validates the Kubernetes version
func (v *VPSieNodeValidator) validateKubernetesVersion(vn *autoscalerv1alpha1.VPSieNode) error {
	if vn.Spec.KubernetesVersion == "" {
		return fmt.Errorf("spec.kubernetesVersion is required and cannot be empty")
	}

	// Validate semantic version format
	validVersion := regexp.MustCompile(`^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[a-zA-Z0-9.-]+)?(\+[a-zA-Z0-9.-]+)?$`)
	if !validVersion.MatchString(vn.Spec.KubernetesVersion) {
		return fmt.Errorf("spec.kubernetesVersion '%s' is not a valid semantic version",
			vn.Spec.KubernetesVersion)
	}

	return nil
}

// validateOSImage validates the OS image ID
func (v *VPSieNodeValidator) validateOSImage(vn *autoscalerv1alpha1.VPSieNode) error {
	if vn.Spec.OSImageID == "" {
		return fmt.Errorf("spec.osImageId is required and cannot be empty")
	}

	// Basic format validation
	validOSImage := regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)
	if !validOSImage.MatchString(vn.Spec.OSImageID) {
		return fmt.Errorf("spec.osImageId '%s' contains invalid characters", vn.Spec.OSImageID)
	}

	return nil
}

// validateSSHKeyIDs validates SSH key IDs (optional field)
func (v *VPSieNodeValidator) validateSSHKeyIDs(vn *autoscalerv1alpha1.VPSieNode) error {
	// SSH keys are optional
	if len(vn.Spec.SSHKeyIDs) == 0 {
		return nil
	}

	// Validate each SSH key ID
	for i, keyID := range vn.Spec.SSHKeyIDs {
		if keyID == "" {
			return fmt.Errorf("spec.sshKeyIds[%d] cannot be empty", i)
		}

		// Basic format validation (alphanumeric, hyphens, underscores)
		validKeyID := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
		if !validKeyID.MatchString(keyID) {
			return fmt.Errorf("spec.sshKeyIds[%d] '%s' contains invalid characters", i, keyID)
		}
	}

	// Check for duplicates
	seen := make(map[string]bool)
	for i, keyID := range vn.Spec.SSHKeyIDs {
		if seen[keyID] {
			return fmt.Errorf("spec.sshKeyIds[%d] '%s' is duplicated", i, keyID)
		}
		seen[keyID] = true
	}

	return nil
}

// validateInstanceConfiguration validates instance configuration
// (User data/cloud-init support was removed in v0.6.0)
func (v *VPSieNodeValidator) validateInstanceConfiguration(vn *autoscalerv1alpha1.VPSieNode) error {
	// Instance configuration is now handled entirely by VPSie API
	// No additional validation needed beyond CRD schema validation
	return nil
}

// validateImmutableFields validates that immutable fields haven't changed
func (v *VPSieNodeValidator) validateImmutableFields(vn *autoscalerv1alpha1.VPSieNode) error {
	// For UPDATE operations, we would need the old object to compare
	// This is a placeholder - in a full implementation, you would:
	// 1. Get the old object from the admission request
	// 2. Compare immutable fields (NodeGroupName, Datacenter, OfferingID, etc.)
	// 3. Return error if any immutable fields changed

	// Example immutable fields:
	// - NodeGroupName (can't move a node to a different group)
	// - Datacenter (can't move a node to a different datacenter)
	// - OfferingID (can't change instance type after creation)

	// Note: This would require updating the Validate signature to accept
	// both old and new objects, or updating the server to pass the old object

	return nil
}
