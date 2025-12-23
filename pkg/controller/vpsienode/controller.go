package vpsienode

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

const (
	// ControllerName is the name of the VPSieNode controller
	ControllerName = "vpsienode-controller"

	// FinalizerName is the finalizer added to VPSieNodes
	FinalizerName = "vpsienode.autoscaler.vpsie.com/finalizer"

	// DefaultMaxConcurrentReconciles is the default number of concurrent reconciles
	DefaultMaxConcurrentReconciles = 3
)

// VPSieNodeReconciler reconciles a VPSieNode object
type VPSieNodeReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	VPSieClient  VPSieClientInterface
	Logger       *zap.Logger
	Recorder     record.EventRecorder
	stateMachine *StateMachine
	provisioner  *Provisioner
	joiner       *Joiner
	drainer      *Drainer
	terminator   *Terminator
}

// NewVPSieNodeReconciler creates a new VPSieNodeReconciler
func NewVPSieNodeReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	vpsieClient VPSieClientInterface,
	logger *zap.Logger,
	sshKeyIDs []string,
) *VPSieNodeReconciler {
	provisioner := NewProvisioner(vpsieClient, sshKeyIDs)
	joiner := NewJoiner(client, provisioner)
	drainer := NewDrainer(client)
	terminator := NewTerminator(drainer, provisioner)
	stateMachine := NewStateMachine(provisioner, joiner, terminator)

	return &VPSieNodeReconciler{
		Client:       client,
		Scheme:       scheme,
		VPSieClient:  vpsieClient,
		Logger:       logger.Named(ControllerName),
		stateMachine: stateMachine,
		provisioner:  provisioner,
		joiner:       joiner,
		drainer:      drainer,
		terminator:   terminator,
	}
}

// SetupWithManager sets up the controller with the Manager
func (r *VPSieNodeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize event recorder if not already set
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor(ControllerName)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.VPSieNode{}).
		Owns(&corev1.Node{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: DefaultMaxConcurrentReconciles,
		}).
		Complete(r)
}

// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=vpsienodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=vpsienodes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=vpsienodes/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *VPSieNodeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Logger.With(
		zap.String("namespace", req.Namespace),
		zap.String("name", req.Name),
	)

	logger.Debug("Reconciling VPSieNode")

	// Fetch the VPSieNode instance
	vn := &v1alpha1.VPSieNode{}
	if err := r.Get(ctx, req.NamespacedName, vn); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("VPSieNode not found, probably deleted")
			return ctrl.Result{}, nil
		}
		logger.Error("Failed to get VPSieNode", zap.Error(err))
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !vn.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, vn, logger)
	}

	// Add finalizer if not present
	if !containsString(vn.Finalizers, FinalizerName) {
		vn.Finalizers = append(vn.Finalizers, FinalizerName)
		if err := r.Update(ctx, vn); err != nil {
			logger.Error("Failed to add finalizer", zap.Error(err))
			return ctrl.Result{}, err
		}
		logger.Info("Added finalizer to VPSieNode")
		return ctrl.Result{Requeue: true}, nil
	}

	// Initialize phase if not set
	if vn.Status.Phase == "" {
		// Use optimistic locking with patch to prevent conflicts
		patch := client.MergeFrom(vn.DeepCopy())
		vn.Status.Phase = v1alpha1.VPSieNodePhasePending
		vn.Status.ObservedGeneration = vn.Generation
		r.Recorder.Event(vn, corev1.EventTypeNormal, "Initializing",
			"VPSieNode created and entering Pending phase")
		if err := r.Status().Patch(ctx, vn, patch); err != nil {
			if apierrors.IsConflict(err) {
				logger.Info("Status update conflict, will retry")
				return ctrl.Result{Requeue: true}, nil
			}
			logger.Error("Failed to initialize phase", zap.Error(err))
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Track if spec changed (VPS ID is the main indicator)
	originalVPSID := vn.Spec.VPSieInstanceID

	// Reconcile the VPSieNode through the state machine
	result, err := r.reconcile(ctx, vn, logger)

	// Update spec only if it was modified (e.g., VPS ID was set)
	if vn.Spec.VPSieInstanceID != originalVPSID {
		if updateErr := r.Update(ctx, vn); updateErr != nil {
			logger.Error("Failed to update spec", zap.Error(updateErr))
			if err == nil {
				return ctrl.Result{}, updateErr
			}
			return result, fmt.Errorf("reconcile error: %w, spec update error: %v", err, updateErr)
		}
	}

	// Always update status with optimistic locking
	patch := client.MergeFrom(vn.DeepCopy())
	vn.Status.ObservedGeneration = vn.Generation
	if statusErr := r.Status().Patch(ctx, vn, patch); statusErr != nil {
		if apierrors.IsConflict(statusErr) {
			logger.Info("Status update conflict, will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error("Failed to update status", zap.Error(statusErr))
		if err == nil {
			return ctrl.Result{}, statusErr
		}
		// Return both errors
		return result, fmt.Errorf("reconcile error: %w, status update error: %v", err, statusErr)
	}

	return result, err
}

// reconcile handles the main reconciliation logic
func (r *VPSieNodeReconciler) reconcile(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Processing VPSieNode",
		zap.String("phase", string(vn.Status.Phase)),
		zap.Int("vpsID", vn.Spec.VPSieInstanceID),
	)

	// Clear any previous errors if we're not in Failed phase
	if vn.Status.Phase != v1alpha1.VPSieNodePhaseFailed {
		ClearError(vn)
	}

	// Execute the state machine for the current phase
	result, err := r.stateMachine.Handle(ctx, vn, logger)
	if err != nil {
		logger.Error("Phase handler error",
			zap.String("phase", string(vn.Status.Phase)),
			zap.Error(err),
		)
		r.Recorder.Eventf(vn, corev1.EventTypeWarning, "PhaseFailed",
			"Failed in %s phase: %v", vn.Status.Phase, err)
		// Don't set phase to Failed here, let the phase handler decide
		return result, err
	}

	logger.Debug("Phase handler completed",
		zap.String("phase", string(vn.Status.Phase)),
		zap.Bool("requeue", result.Requeue || result.RequeueAfter > 0),
		zap.Duration("requeueAfter", result.RequeueAfter),
	)

	return result, nil
}

// reconcileDelete handles VPSieNode deletion
func (r *VPSieNodeReconciler) reconcileDelete(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Handling VPSieNode deletion")

	if !containsString(vn.Finalizers, FinalizerName) {
		logger.Info("Finalizer already removed, nothing to do")
		return ctrl.Result{}, nil
	}

	// Transition to Terminating phase if not already there
	if vn.Status.Phase != v1alpha1.VPSieNodePhaseTerminating &&
		vn.Status.Phase != v1alpha1.VPSieNodePhaseDeleting {
		patch := client.MergeFrom(vn.DeepCopy())
		SetPhase(vn, v1alpha1.VPSieNodePhaseTerminating, ReasonTerminating, "VPSieNode is being deleted")
		if err := r.Status().Patch(ctx, vn, patch); err != nil {
			if apierrors.IsConflict(err) {
				logger.Info("Status update conflict, will retry")
				return ctrl.Result{Requeue: true}, nil
			}
			logger.Error("Failed to update phase to Terminating", zap.Error(err))
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Use state machine to handle Terminating and Deleting phases until VPS is deleted
	if (vn.Status.Phase == v1alpha1.VPSieNodePhaseTerminating ||
		vn.Status.Phase == v1alpha1.VPSieNodePhaseDeleting) &&
		vn.Status.DeletedAt == nil {
		result, err := r.stateMachine.Handle(ctx, vn, logger)
		if err != nil {
			logger.Error("Failed to handle deletion phase",
				zap.String("phase", string(vn.Status.Phase)),
				zap.Error(err),
			)
		}

		// Update status after state machine handling
		patch := client.MergeFrom(vn.DeepCopy())
		vn.Status.ObservedGeneration = vn.Generation
		if statusErr := r.Status().Patch(ctx, vn, patch); statusErr != nil {
			if apierrors.IsConflict(statusErr) {
				logger.Info("Status update conflict, will retry")
				return ctrl.Result{Requeue: true}, nil
			}
			logger.Error("Failed to update status during deletion", zap.Error(statusErr))
			if err == nil {
				return ctrl.Result{}, statusErr
			}
		}

		// If DeletedAt is not set, continue processing
		if vn.Status.DeletedAt == nil {
			return result, err
		}
	}

	// At this point, VPS should be deleted, remove finalizer
	vn.Finalizers = removeString(vn.Finalizers, FinalizerName)
	if err := r.Update(ctx, vn); err != nil {
		logger.Error("Failed to remove finalizer", zap.Error(err))
		return ctrl.Result{}, err
	}

	logger.Info("Successfully removed finalizer, VPSieNode will be deleted")
	return ctrl.Result{}, nil
}

// containsString checks if a slice contains a string
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// removeString removes a string from a slice
func removeString(slice []string, s string) []string {
	var result []string
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
