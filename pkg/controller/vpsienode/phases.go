package vpsienode

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

const (
	// DefaultRequeueAfter is the default requeue time
	DefaultRequeueAfter = 30 * time.Second

	// FastRequeueAfter is used when actively polling for status
	FastRequeueAfter = 10 * time.Second

	// ProvisioningTimeout is the maximum time to wait for VPS provisioning
	ProvisioningTimeout = 10 * time.Minute

	// JoiningTimeout is the maximum time to wait for node to join cluster
	JoiningTimeout = 15 * time.Minute
)

// PhaseHandler handles a specific phase of the VPSieNode lifecycle
type PhaseHandler interface {
	// Handle processes the current phase and returns the next phase
	Handle(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error)
}

// PhaseTransition represents a transition from one phase to another
type PhaseTransition struct {
	From    v1alpha1.VPSieNodePhase
	To      v1alpha1.VPSieNodePhase
	Handler PhaseHandler
}

// StateMachine manages the phase transitions for VPSieNode
type StateMachine struct {
	handlers map[v1alpha1.VPSieNodePhase]PhaseHandler
}

// NewStateMachine creates a new state machine with the given handlers
func NewStateMachine(
	provisioner *Provisioner,
	joiner *Joiner,
	terminator *Terminator,
	failedNodeTTL time.Duration,
	k8sClient client.Client,
) *StateMachine {
	sm := &StateMachine{
		handlers: make(map[v1alpha1.VPSieNodePhase]PhaseHandler),
	}

	// Register phase handlers
	sm.handlers[v1alpha1.VPSieNodePhasePending] = &PendingPhaseHandler{provisioner: provisioner}
	sm.handlers[v1alpha1.VPSieNodePhaseProvisioning] = &ProvisioningPhaseHandler{provisioner: provisioner}
	sm.handlers[v1alpha1.VPSieNodePhaseProvisioned] = &ProvisionedPhaseHandler{joiner: joiner}
	sm.handlers[v1alpha1.VPSieNodePhaseJoining] = &JoiningPhaseHandler{joiner: joiner}
	sm.handlers[v1alpha1.VPSieNodePhaseReady] = &ReadyPhaseHandler{joiner: joiner}
	sm.handlers[v1alpha1.VPSieNodePhaseTerminating] = &TerminatingPhaseHandler{terminator: terminator}
	sm.handlers[v1alpha1.VPSieNodePhaseDeleting] = &DeletingPhaseHandler{terminator: terminator}
	sm.handlers[v1alpha1.VPSieNodePhaseFailed] = &FailedPhaseHandler{ttl: failedNodeTTL, client: k8sClient}

	return sm
}

// Handle processes the current phase of the VPSieNode
func (sm *StateMachine) Handle(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	// Get the handler for the current phase
	handler, exists := sm.handlers[vn.Status.Phase]
	if !exists {
		return ctrl.Result{}, fmt.Errorf("no handler for phase %s", vn.Status.Phase)
	}

	// Execute the handler
	return handler.Handle(ctx, vn, logger)
}

// SetPhase sets the phase of the VPSieNode
func SetPhase(vn *v1alpha1.VPSieNode, phase v1alpha1.VPSieNodePhase, reason, message string) {
	vn.Status.Phase = phase

	// Set phase-specific timestamps
	now := metav1Now()
	switch phase {
	case v1alpha1.VPSieNodePhaseProvisioning:
		if vn.Status.CreatedAt == nil {
			vn.Status.CreatedAt = &now
		}
	case v1alpha1.VPSieNodePhaseProvisioned:
		if vn.Status.ProvisionedAt == nil {
			vn.Status.ProvisionedAt = &now
		}
	case v1alpha1.VPSieNodePhaseReady:
		if vn.Status.JoinedAt == nil {
			vn.Status.JoinedAt = &now
		}
		if vn.Status.ReadyAt == nil {
			vn.Status.ReadyAt = &now
		}
	case v1alpha1.VPSieNodePhaseTerminating:
		if vn.Status.TerminatingAt == nil {
			vn.Status.TerminatingAt = &now
		}
		// Note: DeletedAt is NOT set here - it's set by DeleteVPS() after successful deletion
	}
}

// PendingPhaseHandler handles the Pending phase
// Transitions: Pending → Provisioning
type PendingPhaseHandler struct {
	provisioner *Provisioner
}

// Handle implements PhaseHandler
func (h *PendingPhaseHandler) Handle(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Handling Pending phase", zap.String("vpsienode", vn.Name))

	// Transition to Provisioning phase
	SetPhase(vn, v1alpha1.VPSieNodePhaseProvisioning, ReasonProvisioning, "Starting VPS provisioning")
	SetVPSReadyCondition(vn, false, ReasonProvisioning, "VPS provisioning started")

	return ctrl.Result{Requeue: true}, nil
}

// ProvisioningPhaseHandler handles the Provisioning phase
// Transitions: Provisioning → Provisioned (on success) or Provisioning → Failed (on error/timeout)
type ProvisioningPhaseHandler struct {
	provisioner *Provisioner
}

// Handle implements PhaseHandler
func (h *ProvisioningPhaseHandler) Handle(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Handling Provisioning phase", zap.String("vpsienode", vn.Name))

	// Check for timeout
	if vn.Status.CreatedAt != nil {
		elapsed := time.Since(vn.Status.CreatedAt.Time)
		if elapsed > ProvisioningTimeout {
			logger.Warn("Provisioning timeout exceeded",
				zap.String("vpsienode", vn.Name),
				zap.Duration("elapsed", elapsed),
			)
			SetPhase(vn, v1alpha1.VPSieNodePhaseFailed, ReasonProvisioningTimeout, "VPS provisioning timeout exceeded")
			RecordError(vn, ReasonProvisioningTimeout, fmt.Sprintf("Provisioning timeout exceeded (%v)", elapsed))
			return ctrl.Result{}, nil
		}
	}

	// Provision the VPS
	result, err := h.provisioner.Provision(ctx, vn, logger)
	if err != nil {
		logger.Error("Failed to provision VPS",
			zap.String("vpsienode", vn.Name),
			zap.Error(err),
		)

		// Check if this is a terminal error (e.g., cluster capacity limit reached)
		// Terminal errors won't be resolved by retrying, so fail immediately
		if vpsieclient.IsTerminalError(err) {
			logger.Error("Terminal provisioning error - failing immediately without retry",
				zap.String("vpsienode", vn.Name),
				zap.Error(err),
			)
			SetPhase(vn, v1alpha1.VPSieNodePhaseFailed, ReasonCapacityLimitReached, "Cluster capacity limit reached")
			RecordError(vn, ReasonCapacityLimitReached, err.Error())
			// Return nil error so the status update persists; the phase is now Failed
			return ctrl.Result{}, nil
		}

		RecordError(vn, ReasonVPSieAPIError, err.Error())
		return result, err
	}

	return result, nil
}

// ProvisionedPhaseHandler handles the Provisioned phase
// Transitions: Provisioned → Joining
type ProvisionedPhaseHandler struct {
	joiner *Joiner
}

// Handle implements PhaseHandler
func (h *ProvisionedPhaseHandler) Handle(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Handling Provisioned phase", zap.String("vpsienode", vn.Name))

	// Prepare for node joining
	if err := h.joiner.PrepareJoin(ctx, vn, logger); err != nil {
		logger.Error("Failed to prepare node joining",
			zap.String("vpsienode", vn.Name),
			zap.Error(err),
		)
		RecordError(vn, ReasonFailed, err.Error())
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, err
	}

	// Transition to Joining phase
	SetPhase(vn, v1alpha1.VPSieNodePhaseJoining, ReasonJoining, "Node is joining the cluster")
	SetNodeJoinedCondition(vn, false, ReasonJoining, "Node joining started")

	return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
}

// JoiningPhaseHandler handles the Joining phase
// Transitions: Joining → Ready (on success) or Joining → Failed (on timeout)
type JoiningPhaseHandler struct {
	joiner *Joiner
}

// Handle implements PhaseHandler
func (h *JoiningPhaseHandler) Handle(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Handling Joining phase", zap.String("vpsienode", vn.Name))

	// Check for timeout
	if vn.Status.ProvisionedAt != nil {
		elapsed := time.Since(vn.Status.ProvisionedAt.Time)
		if elapsed > JoiningTimeout {
			logger.Warn("Joining timeout exceeded",
				zap.String("vpsienode", vn.Name),
				zap.Duration("elapsed", elapsed),
			)
			SetPhase(vn, v1alpha1.VPSieNodePhaseFailed, ReasonJoiningTimeout, "Node joining timeout exceeded")
			RecordError(vn, ReasonJoiningTimeout, fmt.Sprintf("Node joining timeout exceeded (%v)", elapsed))
			return ctrl.Result{}, nil
		}
	}

	// Check if node has joined
	result, err := h.joiner.CheckJoinStatus(ctx, vn, logger)
	if err != nil {
		logger.Error("Failed to check node join status",
			zap.String("vpsienode", vn.Name),
			zap.Error(err),
		)
		RecordError(vn, ReasonFailed, err.Error())
		return result, err
	}

	return result, nil
}

// ReadyPhaseHandler handles the Ready phase
// Node is operational and ready to accept workloads
type ReadyPhaseHandler struct {
	joiner *Joiner
}

// Handle implements PhaseHandler
func (h *ReadyPhaseHandler) Handle(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Debug("Handling Ready phase", zap.String("vpsienode", vn.Name))

	// Monitor node health
	result, err := h.joiner.MonitorNode(ctx, vn, logger)
	if err != nil {
		logger.Error("Failed to monitor node",
			zap.String("vpsienode", vn.Name),
			zap.Error(err),
		)
		return result, err
	}

	return result, nil
}

// TerminatingPhaseHandler handles the Terminating phase
// Transitions: Terminating → Deleting
type TerminatingPhaseHandler struct {
	terminator *Terminator
}

// Handle implements PhaseHandler
func (h *TerminatingPhaseHandler) Handle(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Handling Terminating phase", zap.String("vpsienode", vn.Name))

	// Drain the node and delete it from Kubernetes
	return h.terminator.DrainAndDelete(ctx, vn, logger)
}

// DeletingPhaseHandler handles the Deleting phase
// Deletes the VPS from VPSie
type DeletingPhaseHandler struct {
	terminator *Terminator
}

// Handle implements PhaseHandler
func (h *DeletingPhaseHandler) Handle(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Handling Deleting phase", zap.String("vpsienode", vn.Name))

	// Delete the VPS with retries
	return h.terminator.DeleteVPS(ctx, vn, logger)
}

// FailedPhaseHandler handles the Failed phase
// Node remains in Failed state until TTL expires or manually deleted
type FailedPhaseHandler struct {
	ttl    time.Duration
	client client.Client
}

// Handle implements PhaseHandler
func (h *FailedPhaseHandler) Handle(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Handling Failed phase", zap.String("vpsienode", vn.Name))

	// If TTL is disabled (0), do nothing - node remains in Failed state
	if h.ttl <= 0 {
		logger.Debug("TTL-based cleanup disabled for failed VPSieNodes")
		return ctrl.Result{}, nil
	}

	// Determine when the node entered Failed state
	// Use Error condition's LastTransitionTime, or fall back to creation timestamp
	var failedAt time.Time
	errorCond := GetCondition(vn, v1alpha1.VPSieNodeConditionError)
	if errorCond != nil && !errorCond.LastTransitionTime.IsZero() {
		failedAt = errorCond.LastTransitionTime.Time
	} else {
		// Fall back to creation time if no Error condition exists
		failedAt = vn.CreationTimestamp.Time
	}

	elapsed := time.Since(failedAt)
	remaining := h.ttl - elapsed

	logger.Debug("Checking TTL for failed VPSieNode",
		zap.String("vpsienode", vn.Name),
		zap.Duration("ttl", h.ttl),
		zap.Duration("elapsed", elapsed),
		zap.Duration("remaining", remaining),
	)

	// If TTL has not expired, requeue after the remaining time
	if remaining > 0 {
		logger.Info("VPSieNode TTL not expired, will check again",
			zap.String("vpsienode", vn.Name),
			zap.Duration("checkAfter", remaining),
		)
		return ctrl.Result{RequeueAfter: remaining}, nil
	}

	// TTL expired - delete the VPSieNode
	logger.Warn("VPSieNode TTL expired, initiating deletion",
		zap.String("vpsienode", vn.Name),
		zap.Duration("ttl", h.ttl),
		zap.Duration("inFailedState", elapsed),
		zap.String("lastError", vn.Status.LastError),
	)

	// Record metric for TTL-based deletion
	nodeGroupName := vn.Labels["autoscaler.vpsie.com/nodegroup"]
	if nodeGroupName == "" {
		nodeGroupName = "unknown"
	}
	metrics.VPSieNodeTTLDeletionsTotal.WithLabelValues(nodeGroupName, vn.Namespace).Inc()

	// Delete the VPSieNode CR - the finalizer will handle VPS cleanup
	if err := h.client.Delete(ctx, vn); err != nil {
		logger.Error("Failed to delete VPSieNode after TTL expiration",
			zap.String("vpsienode", vn.Name),
			zap.Error(err),
		)
		// Requeue to retry deletion
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, err
	}

	logger.Info("VPSieNode deleted due to TTL expiration",
		zap.String("vpsienode", vn.Name),
	)

	// No requeue needed - the delete will trigger a new reconciliation via the watch
	return ctrl.Result{}, nil
}

// metav1Now is a helper for creating metav1.Time
func metav1Now() metav1.Time {
	return metav1.Now()
}
