package webhook

import (
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
)

// NodeDeletionValidatorInterface defines the interface for node deletion validation
type NodeDeletionValidatorInterface interface {
	ValidateDelete(node *corev1.Node) error
}

// NodeDeletionValidator validates node deletion requests
// This addresses Fix #8: RBAC Protection - only allows deletion of autoscaler-managed nodes
type NodeDeletionValidator struct {
	logger *zap.Logger
}

// NewNodeDeletionValidator creates a new node deletion validator
func NewNodeDeletionValidator(logger *zap.Logger) *NodeDeletionValidator {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &NodeDeletionValidator{
		logger: logger,
	}
}

// ValidateDelete validates a node deletion request
// Only nodes with label "autoscaler.vpsie.com/managed=true" can be deleted
func (v *NodeDeletionValidator) ValidateDelete(node *corev1.Node) error {
	v.logger.Debug("validating node deletion",
		zap.String("node", node.Name))

	// Check if node has the managed label
	if node.Labels == nil {
		return fmt.Errorf("node %s does not have any labels - not managed by VPSie autoscaler", node.Name)
	}

	managedLabel, exists := node.Labels["autoscaler.vpsie.com/managed"]
	if !exists {
		return fmt.Errorf("node %s is not managed by VPSie autoscaler (missing label autoscaler.vpsie.com/managed)", node.Name)
	}

	if managedLabel != "true" {
		return fmt.Errorf("node %s has invalid managed label value '%s' (expected 'true')", node.Name, managedLabel)
	}

	v.logger.Debug("node deletion validated successfully",
		zap.String("node", node.Name))

	return nil
}
