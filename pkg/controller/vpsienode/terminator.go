package vpsienode

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

const (
	// MaxRetries is the maximum number of retries for deletion operations
	MaxRetries = 3

	// RetryDelay is the delay between retries
	RetryDelay = 5 * time.Second
)

// Terminator handles the complete termination flow for VPSieNodes
type Terminator struct {
	drainer     *Drainer
	provisioner *Provisioner
}

// NewTerminator creates a new Terminator
func NewTerminator(drainer *Drainer, provisioner *Provisioner) *Terminator {
	return &Terminator{
		drainer:     drainer,
		provisioner: provisioner,
	}
}

// InitiateTermination initiates the termination process
// This is called when the VPSieNode enters the Terminating phase
func (t *Terminator) InitiateTermination(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Initiating node termination",
		zap.String("vpsienode", vn.Name),
		zap.String("phase", string(vn.Status.Phase)),
	)

	// Set terminating timestamp
	now := metav1.Now()
	if vn.Status.TerminatingAt == nil {
		vn.Status.TerminatingAt = &now
	}

	// Update status to indicate termination has started
	SetPhase(vn, v1alpha1.VPSieNodePhaseTerminating, ReasonTerminating, "Starting node termination")

	// Requeue immediately to start the deletion process
	return ctrl.Result{Requeue: true}, nil
}

// DrainAndDelete drains the node and deletes it from Kubernetes
// This is called during the Terminating → Deleting transition
func (t *Terminator) DrainAndDelete(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Draining and deleting node",
		zap.String("vpsienode", vn.Name),
		zap.String("nodeName", vn.Status.NodeName),
	)

	nodeName := vn.Status.NodeName
	if nodeName == "" {
		nodeName = vn.Spec.NodeName
	}

	// Step 1: Drain the node if it exists in Kubernetes
	if nodeName != "" {
		logger.Info("Draining node", zap.String("node", nodeName))
		if err := t.drainer.DrainNode(ctx, nodeName, logger); err != nil {
			logger.Error("Failed to drain node",
				zap.String("node", nodeName),
				zap.Error(err),
			)
			RecordError(vn, ReasonDrainFailed, fmt.Sprintf("Failed to drain node: %v", err))
			// Continue with deletion even if drain fails after recording error
			// The node might already be gone or unreachable
		} else {
			logger.Info("Successfully drained node", zap.String("node", nodeName))
		}

		// Step 2: Delete the Kubernetes Node object
		logger.Info("Deleting Kubernetes Node", zap.String("node", nodeName))
		if err := t.drainer.DeleteNode(ctx, vn, logger); err != nil {
			logger.Error("Failed to delete Kubernetes Node",
				zap.String("node", nodeName),
				zap.Error(err),
			)
			RecordError(vn, ReasonNodeDeleteFailed, fmt.Sprintf("Failed to delete Node: %v", err))
			// Continue with VPS deletion even if Node deletion fails
		} else {
			logger.Info("Successfully deleted Kubernetes Node", zap.String("node", nodeName))
		}
	}

	// Transition to Deleting phase
	SetPhase(vn, v1alpha1.VPSieNodePhaseDeleting, ReasonDeleting, "Deleting VPS")

	// Requeue immediately to delete VPS
	return ctrl.Result{Requeue: true}, nil
}

// DeleteVPS deletes the VPS instance from VPSie
// This is called during the Deleting phase
func (t *Terminator) DeleteVPS(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	// Determine if we can identify the VPS for deletion
	// For K8s-managed nodes, VPSieInstanceID may be 0 but we can still delete via hostname lookup
	hostname := vn.Status.Hostname
	if hostname == "" {
		hostname = vn.Spec.NodeName
	}
	if hostname == "" {
		hostname = vn.Status.NodeName
	}

	canDeleteViaK8sAPI := vn.Spec.ResourceIdentifier != "" && hostname != ""
	canDeleteViaVMAPI := vn.Spec.VPSieInstanceID != 0

	if !canDeleteViaK8sAPI && !canDeleteViaVMAPI {
		logger.Info("No VPS ID or K8s identifiers available, skipping VPS deletion",
			zap.String("vpsienode", vn.Name),
			zap.String("resourceIdentifier", vn.Spec.ResourceIdentifier),
			zap.String("hostname", hostname),
		)
		// No VPS to delete, mark termination as complete
		now := metav1.Now()
		vn.Status.DeletedAt = &now
		return ctrl.Result{}, nil
	}

	logger.Info("Deleting VPS",
		zap.String("vpsienode", vn.Name),
		zap.Int("vpsID", vn.Spec.VPSieInstanceID),
		zap.String("resourceIdentifier", vn.Spec.ResourceIdentifier),
		zap.String("hostname", hostname),
	)

	// Delete VPS with retries
	var lastErr error
	for attempt := 1; attempt <= MaxRetries; attempt++ {
		err := t.provisioner.Delete(ctx, vn, logger)
		if err == nil {
			logger.Info("Successfully deleted VPS",
				zap.String("vpsienode", vn.Name),
				zap.Int("vpsID", vn.Spec.VPSieInstanceID),
			)
			now := metav1.Now()
			vn.Status.DeletedAt = &now
			return ctrl.Result{}, nil
		}

		// Check if VPS not found (already deleted)
		if vpsieclient.IsNotFound(err) {
			logger.Info("VPS not found, already deleted",
				zap.String("vpsienode", vn.Name),
				zap.Int("vpsID", vn.Spec.VPSieInstanceID),
			)
			now := metav1.Now()
			vn.Status.DeletedAt = &now
			return ctrl.Result{}, nil
		}

		lastErr = err
		logger.Warn("Failed to delete VPS, will retry",
			zap.String("vpsienode", vn.Name),
			zap.Int("vpsID", vn.Spec.VPSieInstanceID),
			zap.Int("attempt", attempt),
			zap.Int("maxRetries", MaxRetries),
			zap.Error(err),
		)

		if attempt < MaxRetries {
			// Wait before retry
			select {
			case <-ctx.Done():
				return ctrl.Result{}, ctx.Err()
			case <-time.After(RetryDelay):
				// Continue to next attempt
			}
		}
	}

	// All retries exhausted
	logger.Error("Failed to delete VPS after all retries",
		zap.String("vpsienode", vn.Name),
		zap.Int("vpsID", vn.Spec.VPSieInstanceID),
		zap.Int("retries", MaxRetries),
		zap.Error(lastErr),
	)

	RecordError(vn, ReasonVPSDeleteFailed, fmt.Sprintf("Failed to delete VPS after %d retries: %v", MaxRetries, lastErr))
	SetPhase(vn, v1alpha1.VPSieNodePhaseFailed, ReasonVPSDeleteFailed, "Failed to delete VPS")

	// Return error to trigger requeue with backoff
	return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, fmt.Errorf("failed to delete VPS: %w", lastErr)
}

// CheckDrainStatus checks if the node has been successfully drained
func (t *Terminator) CheckDrainStatus(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (bool, error) {
	nodeName := vn.Status.NodeName
	if nodeName == "" {
		nodeName = vn.Spec.NodeName
	}

	if nodeName == "" {
		// No node to check
		return true, nil
	}

	// Check if there are any pods still on the node
	pods, err := t.drainer.getPodsOnNode(ctx, nodeName, logger)
	if err != nil {
		return false, fmt.Errorf("failed to list pods on node: %w", err)
	}

	// Filter to pods that should have been evicted
	remainingPods := t.drainer.filterPodsToEvict(pods, logger)

	if len(remainingPods) > 0 {
		logger.Debug("Pods still remaining on node",
			zap.String("node", nodeName),
			zap.Int("count", len(remainingPods)),
		)
		return false, nil
	}

	return true, nil
}

// GetDrainProgress returns the drain progress as a percentage
func (t *Terminator) GetDrainProgress(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (int, error) {
	nodeName := vn.Status.NodeName
	if nodeName == "" {
		nodeName = vn.Spec.NodeName
	}

	if nodeName == "" {
		return 100, nil
	}

	pods, err := t.drainer.getPodsOnNode(ctx, nodeName, logger)
	if err != nil {
		return 0, fmt.Errorf("failed to list pods on node: %w", err)
	}

	totalPods := len(t.drainer.filterPodsToEvict(pods, logger))
	if totalPods == 0 {
		return 100, nil
	}

	// Simple progress estimation (not accurate but useful for status)
	// In reality, we'd need to track initial pod count
	return 50, nil
}
