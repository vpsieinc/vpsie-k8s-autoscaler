package vpsienode

import (
	"context"
	"fmt"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/vpsie/vpsie-k8s-autoscaler/internal/logging"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/tracing"
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
	Scheme        *runtime.Scheme
	VPSieClient   VPSieClientInterface
	Logger        *zap.Logger
	Recorder      record.EventRecorder
	FailedNodeTTL time.Duration
	stateMachine  *StateMachine
	provisioner   *Provisioner
	joiner        *Joiner
	drainer       *Drainer
	terminator    *Terminator
}

// NewVPSieNodeReconciler creates a new VPSieNodeReconciler
func NewVPSieNodeReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	vpsieClient VPSieClientInterface,
	logger *zap.Logger,
	sshKeyIDs []string,
	failedNodeTTL time.Duration,
) *VPSieNodeReconciler {
	provisioner := NewProvisioner(vpsieClient, sshKeyIDs)
	joiner := NewJoiner(client, provisioner)
	drainer := NewDrainer(client)
	terminator := NewTerminator(drainer, provisioner)
	stateMachine := NewStateMachine(provisioner, joiner, terminator, failedNodeTTL, client)

	// Create and inject discoverer for async VPS ID discovery
	discoverer := NewDiscoverer(vpsieClient, client, logger)
	provisioner.SetDiscoverer(discoverer)

	return &VPSieNodeReconciler{
		Client:        client,
		Scheme:        scheme,
		VPSieClient:   vpsieClient,
		Logger:        logger.Named(ControllerName),
		FailedNodeTTL: failedNodeTTL,
		stateMachine:  stateMachine,
		provisioner:   provisioner,
		joiner:        joiner,
		drainer:       drainer,
		terminator:    terminator,
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
	// Add correlation ID for request tracing
	ctx = logging.WithRequestID(ctx)

	// Start Sentry transaction for tracing
	ctx, span := tracing.StartTransaction(ctx, "VPSieNodeReconciler.Reconcile", "controller.reconcile")
	if span != nil {
		span.SetTag("controller", ControllerName)
		span.SetTag("resource.name", req.Name)
		span.SetTag("resource.namespace", req.Namespace)
		defer span.Finish()
	}

	// Helper to capture errors to Sentry
	captureError := func(err error, operation string) {
		if span != nil {
			span.Status = sentry.SpanStatusInternalError
		}
		tracing.CaptureError(err, map[string]string{
			"controller": ControllerName,
			"resource":   req.Name,
			"namespace":  req.Namespace,
			"operation":  operation,
		})
	}

	logger := logging.WithRequestIDField(ctx, r.Logger.With(
		zap.String("namespace", req.Namespace),
		zap.String("name", req.Name),
	))

	logger.Debug("Reconciling VPSieNode", zap.String("requestID", logging.GetRequestID(ctx)))

	// Fetch the VPSieNode instance
	vn := &v1alpha1.VPSieNode{}
	if err := r.Get(ctx, req.NamespacedName, vn); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("VPSieNode not found, probably deleted")
			return ctrl.Result{}, nil
		}
		logger.Error("Failed to get VPSieNode", zap.Error(err))
		captureError(err, "get_vpsienode")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !vn.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, vn, logger, captureError)
	}

	// Add finalizer if not present
	if !containsString(vn.Finalizers, FinalizerName) {
		vn.Finalizers = append(vn.Finalizers, FinalizerName)
		if err := r.Update(ctx, vn); err != nil {
			logger.Error("Failed to add finalizer", zap.Error(err))
			captureError(err, "add_finalizer")
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
		if r.Recorder != nil {
			r.Recorder.Event(vn, corev1.EventTypeNormal, "Initializing",
				"VPSieNode created and entering Pending phase")
		}
		if err := r.Status().Patch(ctx, vn, patch); err != nil {
			if apierrors.IsConflict(err) {
				logger.Info("Status update conflict, will retry")
				return ctrl.Result{Requeue: true}, nil
			}
			logger.Error("Failed to initialize phase", zap.Error(err))
			captureError(err, "initialize_phase")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Capture original state BEFORE reconcile modifies vn
	// This is critical for the patch to detect status changes
	originalVPSID := vn.Spec.VPSieInstanceID
	originalNodeName := vn.Spec.NodeName
	originalIPAddress := vn.Spec.IPAddress
	originalCreationRequested := vn.Annotations != nil && vn.Annotations[AnnotationCreationRequested] == "true"
	originalState := vn.DeepCopy()

	// Reconcile the VPSieNode through the state machine
	result, err := r.reconcile(ctx, vn, logger)

	// Check if object metadata or spec was modified
	specOrMetadataChanged := vn.Spec.VPSieInstanceID != originalVPSID ||
		vn.Spec.NodeName != originalNodeName ||
		vn.Spec.IPAddress != originalIPAddress
	creationRequestedChanged := (vn.Annotations != nil && vn.Annotations[AnnotationCreationRequested] == "true") != originalCreationRequested

	// Capture current status before Update() to preserve it.
	// The fake client's status subresource behavior resets in-memory status
	// after Update(). In production, the subsequent Status().Patch() is
	// authoritative, but this prevents test failures and ensures status
	// changes made during reconcile are not lost between Update() and Patch().
	currentStatus := vn.Status.DeepCopy()

	// Update object if spec or annotations were modified
	if specOrMetadataChanged || creationRequestedChanged {
		if updateErr := r.Update(ctx, vn); updateErr != nil {
			logger.Error("Failed to update object", zap.Error(updateErr))
			if err == nil {
				return ctrl.Result{}, updateErr
			}
			return result, fmt.Errorf("reconcile error: %w, object update error: %v", err, updateErr)
		}
		// Restore status after Update() - see comment above for explanation.
		vn.Status = *currentStatus
	}

	// Always update status with optimistic locking
	// Use originalState captured BEFORE reconcile to detect all status changes
	patch := client.MergeFrom(originalState)
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
		if r.Recorder != nil {
			r.Recorder.Eventf(vn, corev1.EventTypeWarning, "PhaseFailed",
				"Failed in %s phase: %v", vn.Status.Phase, err)
		}
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
func (r *VPSieNodeReconciler) reconcileDelete(ctx context.Context, vn *v1alpha1.VPSieNode, logger *zap.Logger, captureError func(error, string)) (ctrl.Result, error) {
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
			captureError(err, "update_phase_terminating")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Use state machine to handle Terminating and Deleting phases until VPS is deleted
	if (vn.Status.Phase == v1alpha1.VPSieNodePhaseTerminating ||
		vn.Status.Phase == v1alpha1.VPSieNodePhaseDeleting) &&
		vn.Status.DeletedAt == nil {
		// Capture original state BEFORE state machine modifies vn
		originalState := vn.DeepCopy()

		result, err := r.stateMachine.Handle(ctx, vn, logger)
		if err != nil {
			logger.Error("Failed to handle deletion phase",
				zap.String("phase", string(vn.Status.Phase)),
				zap.Error(err),
			)
			captureError(err, "state_machine_delete")
		}

		// Update status after state machine handling
		// Use originalState captured BEFORE state machine to detect all status changes
		patch := client.MergeFrom(originalState)
		vn.Status.ObservedGeneration = vn.Generation
		if statusErr := r.Status().Patch(ctx, vn, patch); statusErr != nil {
			if apierrors.IsConflict(statusErr) {
				logger.Info("Status update conflict, will retry")
				return ctrl.Result{Requeue: true}, nil
			}
			logger.Error("Failed to update status during deletion", zap.Error(statusErr))
			captureError(statusErr, "update_status_deletion")
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
		captureError(err, "remove_finalizer")
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
