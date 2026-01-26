package nodegroup

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vpsie/vpsie-k8s-autoscaler/internal/logging"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/scaler"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/tracing"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

const (
	// ControllerName is the name of the NodeGroup controller
	ControllerName = "nodegroup-controller"

	// FinalizerName is the finalizer added to NodeGroups
	FinalizerName = "nodegroup.autoscaler.vpsie.com/finalizer"

	// DefaultMaxConcurrentReconciles is the default number of concurrent reconciles
	DefaultMaxConcurrentReconciles = 1
)

// ScaleDownManagerInterface defines the interface for scale-down operations
type ScaleDownManagerInterface interface {
	IdentifyUnderutilizedNodes(ctx context.Context, ng *v1alpha1.NodeGroup) ([]*scaler.ScaleDownCandidate, error)
	ScaleDown(ctx context.Context, ng *v1alpha1.NodeGroup, candidates []*scaler.ScaleDownCandidate) error
	UpdateNodeUtilization(ctx context.Context) error
	// GetMaxNodesPerScaleDown returns the maximum number of nodes that can be scaled down
	// in a single operation. This is a safety limit to prevent aggressive scale-down.
	GetMaxNodesPerScaleDown() int
}

// NodeGroupReconciler reconciles a NodeGroup object
type NodeGroupReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	VPSieClient      *vpsieclient.Client
	ScaleDownManager ScaleDownManagerInterface
	Logger           *zap.Logger
	Recorder         record.EventRecorder

	// Secret watching for credential rotation
	SecretName        string // Name of the secret containing VPSie credentials
	SecretNamespace   string // Namespace of the secret
	credentialsHash   string // Hash of current credentials for change detection
	credentialsHashMu sync.RWMutex
}

// SetupWithManager sets up the controller with the Manager
func (r *NodeGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Initialize event recorder if not already set
	if r.Recorder == nil {
		r.Recorder = mgr.GetEventRecorderFor(ControllerName)
	}

	// Initialize credentials hash from current VPSie client
	if r.VPSieClient != nil {
		r.credentialsHash = r.VPSieClient.GetCredentialsHash()
	}

	// Build the controller
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NodeGroup{}).
		Owns(&v1alpha1.VPSieNode{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: DefaultMaxConcurrentReconciles,
		})

	// Add secret watcher for credential rotation if configured
	if r.SecretName != "" && r.SecretNamespace != "" {
		r.Logger.Info("Setting up secret watcher for credential rotation",
			zap.String("secretName", r.SecretName),
			zap.String("secretNamespace", r.SecretNamespace),
		)

		// Watch for changes to the VPSie credentials secret
		builder = builder.Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.secretToNodeGroups),
		)
	}

	return builder.Complete(r)
}

// secretToNodeGroups maps secret events to NodeGroup reconcile requests
// This enables credential rotation when the vpsie-secret is updated
func (r *NodeGroupReconciler) secretToNodeGroups(ctx context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}

	// Only watch the specific VPSie credentials secret
	if secret.Name != r.SecretName || secret.Namespace != r.SecretNamespace {
		return nil
	}

	r.Logger.Info("VPSie credentials secret changed, triggering credential rotation check",
		zap.String("secretName", secret.Name),
		zap.String("secretNamespace", secret.Namespace),
	)

	// Trigger credential rotation check with its own timeout context
	// Use context.Background() as parent since the operation should complete
	// regardless of the parent context's lifecycle, with a 30-second timeout
	// to prevent indefinite hangs
	go func() {
		rotationCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		r.checkAndRotateCredentials(rotationCtx, secret)
	}()

	// Get all managed NodeGroups to trigger reconciliation
	// Only managed NodeGroups need credential rotation since they're the only ones we control
	var nodeGroupList v1alpha1.NodeGroupList
	if err := r.List(ctx, &nodeGroupList, ManagedLabelSelector()); err != nil {
		r.Logger.Error("Failed to list NodeGroups for secret change", zap.Error(err))
		return nil
	}

	// Queue all NodeGroups for reconciliation
	requests := make([]reconcile.Request, 0, len(nodeGroupList.Items))
	for _, ng := range nodeGroupList.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      ng.Name,
				Namespace: ng.Namespace,
			},
		})
	}

	return requests
}

// checkAndRotateCredentials checks if credentials have changed and rotates them
func (r *NodeGroupReconciler) checkAndRotateCredentials(ctx context.Context, secret *corev1.Secret) {
	if r.VPSieClient == nil {
		return
	}

	// Check for simple token authentication first
	if tokenBytes, ok := secret.Data[vpsieclient.SecretTokenKey]; ok && len(tokenBytes) > 0 {
		r.checkAndRotateToken(ctx, string(tokenBytes))
		return
	}

	// Fall back to OAuth credentials
	clientID, ok := secret.Data[vpsieclient.SecretClientIDKey]
	if !ok || len(clientID) == 0 {
		r.Logger.Error("Secret missing credentials (need 'token' or 'clientId'/'clientSecret')")
		return
	}

	clientSecret, ok := secret.Data[vpsieclient.SecretClientSecretKey]
	if !ok || len(clientSecret) == 0 {
		r.Logger.Error("Secret missing clientSecret key")
		return
	}

	// Check if credentials have actually changed
	r.credentialsHashMu.RLock()
	currentHash := r.credentialsHash
	r.credentialsHashMu.RUnlock()

	// Calculate new hash (simple change detection)
	newHash := vpsieclient.CalculateCredentialsHash(string(clientID), string(clientSecret))
	if newHash == currentHash {
		r.Logger.Debug("Credentials unchanged, skipping rotation")
		return
	}

	r.Logger.Info("Credentials changed, attempting rotation")
	metrics.CredentialRotationAttempts.Inc()

	// Attempt to rotate credentials
	startTime := time.Now()
	if err := r.VPSieClient.UpdateCredentials(ctx, string(clientID), string(clientSecret)); err != nil {
		r.Logger.Error("Credential rotation failed", zap.Error(err))
		metrics.CredentialRotationFailures.Inc()
		return
	}

	// Update stored hash on success
	r.credentialsHashMu.Lock()
	r.credentialsHash = newHash
	r.credentialsHashMu.Unlock()

	duration := time.Since(startTime)
	r.Logger.Info("Credential rotation successful",
		zap.Duration("duration", duration),
	)
	metrics.CredentialRotationSuccesses.Inc()
	metrics.CredentialRotationDuration.Observe(duration.Seconds())
}

// checkAndRotateToken checks if the simple API token has changed and updates it
func (r *NodeGroupReconciler) checkAndRotateToken(ctx context.Context, newToken string) {
	// Check if token has actually changed
	r.credentialsHashMu.RLock()
	currentHash := r.credentialsHash
	r.credentialsHashMu.RUnlock()

	// Calculate new hash for token
	newHash := vpsieclient.CalculateCredentialsHash(newToken, "")
	if newHash == currentHash {
		r.Logger.Debug("Token unchanged, skipping rotation")
		return
	}

	r.Logger.Info("Token changed, updating")
	metrics.CredentialRotationAttempts.Inc()

	startTime := time.Now()
	if err := r.VPSieClient.UpdateToken(newToken); err != nil {
		r.Logger.Error("Token update failed", zap.Error(err))
		metrics.CredentialRotationFailures.Inc()
		return
	}

	// Update stored hash on success
	r.credentialsHashMu.Lock()
	r.credentialsHash = newHash
	r.credentialsHashMu.Unlock()

	duration := time.Since(startTime)
	r.Logger.Info("Token update successful",
		zap.Duration("duration", duration),
	)
	metrics.CredentialRotationSuccesses.Inc()
	metrics.CredentialRotationDuration.Observe(duration.Seconds())
}

// NodeGroupReconcilerOptions contains optional configuration for the NodeGroupReconciler
type NodeGroupReconcilerOptions struct {
	// SecretName is the name of the Kubernetes secret containing VPSie credentials
	// If set, the controller will watch for changes and rotate credentials automatically
	SecretName string

	// SecretNamespace is the namespace of the Kubernetes secret
	SecretNamespace string
}

// NewNodeGroupReconciler creates a new NodeGroupReconciler
func NewNodeGroupReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	vpsieClient *vpsieclient.Client,
	logger *zap.Logger,
	scaleDownManager ScaleDownManagerInterface,
) *NodeGroupReconciler {
	return NewNodeGroupReconcilerWithOptions(client, scheme, vpsieClient, logger, scaleDownManager, nil)
}

// NewNodeGroupReconcilerWithOptions creates a new NodeGroupReconciler with additional options
func NewNodeGroupReconcilerWithOptions(
	client client.Client,
	scheme *runtime.Scheme,
	vpsieClient *vpsieclient.Client,
	logger *zap.Logger,
	scaleDownManager ScaleDownManagerInterface,
	opts *NodeGroupReconcilerOptions,
) *NodeGroupReconciler {
	r := &NodeGroupReconciler{
		Client:           client,
		Scheme:           scheme,
		VPSieClient:      vpsieClient,
		ScaleDownManager: scaleDownManager,
		Logger:           logger.Named(ControllerName),
	}

	if opts != nil {
		r.SecretName = opts.SecretName
		r.SecretNamespace = opts.SecretNamespace
	}

	// Set default secret location if not specified
	if r.SecretName == "" {
		r.SecretName = vpsieclient.DefaultSecretName
	}
	if r.SecretNamespace == "" {
		r.SecretNamespace = vpsieclient.DefaultSecretNamespace
	}

	return r
}

// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=nodegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=nodegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=nodegroups/finalizers,verbs=update
// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=vpsienodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=vpsienodes/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *NodeGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Add correlation ID for request tracing
	ctx = logging.WithRequestID(ctx)

	// Start Sentry transaction for tracing
	ctx, span := tracing.StartTransaction(ctx, "NodeGroupReconciler.Reconcile", "controller.reconcile")
	if span != nil {
		span.SetTag("controller", ControllerName)
		span.SetTag("resource.name", req.Name)
		span.SetTag("resource.namespace", req.Namespace)
		defer span.Finish()
	}

	logger := logging.WithRequestIDField(ctx, r.Logger.With(
		zap.String("namespace", req.Namespace),
		zap.String("name", req.Name),
	))

	logger.Info("Reconciling NodeGroup", zap.String("requestID", logging.GetRequestID(ctx)))

	// Fetch the NodeGroup instance
	ng := &v1alpha1.NodeGroup{}
	if err := r.Get(ctx, req.NamespacedName, ng); err != nil {
		if client.IgnoreNotFound(err) == nil {
			logger.Info("NodeGroup not found, probably deleted")
			return ctrl.Result{}, nil
		}
		logger.Error("Failed to get NodeGroup", zap.Error(err))
		return ctrl.Result{}, err
	}

	// NodeGroup isolation: Skip NodeGroups not managed by the autoscaler.
	// Only NodeGroups with the managed label (autoscaler.vpsie.com/managed=true) are processed.
	// This prevents the autoscaler from interfering with externally created or static NodeGroups.
	if !IsManagedNodeGroup(ng) {
		logger.Debug("Skipping unmanaged NodeGroup",
			zap.String("nodegroup", ng.Name),
			zap.Any("labels", ng.Labels),
		)
		// Do not requeue - only process if the NodeGroup is updated to add the managed label
		return ctrl.Result{}, nil
	}

	// Handle deletion
	if !ng.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, ng, logger)
	}

	// Add finalizer if not present
	if !containsString(ng.Finalizers, FinalizerName) {
		ng.Finalizers = append(ng.Finalizers, FinalizerName)
		if err := r.Update(ctx, ng); err != nil {
			logger.Error("Failed to add finalizer", zap.Error(err))
			return ctrl.Result{}, err
		}
		logger.Info("Added finalizer to NodeGroup")
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile the NodeGroup
	return r.reconcile(ctx, ng, logger)
}

// reconcileDelete handles NodeGroup deletion
func (r *NodeGroupReconciler) reconcileDelete(ctx context.Context, ng *v1alpha1.NodeGroup, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Handling NodeGroup deletion")

	if !containsString(ng.Finalizers, FinalizerName) {
		logger.Info("Finalizer already removed, nothing to do")
		return ctrl.Result{}, nil
	}

	// List all VPSieNodes for this NodeGroup
	vpsieNodes, err := r.listVPSieNodesForNodeGroup(ctx, ng)
	if err != nil {
		logger.Error("Failed to list VPSieNodes", zap.Error(err))
		return ctrl.Result{}, err
	}

	// Delete all VPSieNodes
	for i := range vpsieNodes {
		vn := &vpsieNodes[i]
		if vn.DeletionTimestamp.IsZero() {
			logger.Info("Deleting VPSieNode",
				zap.String("vpsienode", vn.Name),
			)
			if err := r.Delete(ctx, vn); err != nil {
				logger.Error("Failed to delete VPSieNode",
					zap.String("vpsienode", vn.Name),
					zap.Error(err),
				)
				return ctrl.Result{}, err
			}
		}
	}

	// Wait for all VPSieNodes to be deleted
	if len(vpsieNodes) > 0 {
		logger.Info("Waiting for VPSieNodes to be deleted",
			zap.Int("count", len(vpsieNodes)),
		)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// Remove finalizer
	ng.Finalizers = removeString(ng.Finalizers, FinalizerName)
	if err := r.Update(ctx, ng); err != nil {
		logger.Error("Failed to remove finalizer", zap.Error(err))
		return ctrl.Result{}, err
	}

	logger.Info("Successfully removed finalizer, NodeGroup will be deleted")
	return ctrl.Result{}, nil
}

// listVPSieNodesForNodeGroup lists all VPSieNodes belonging to a NodeGroup.
// IMPORTANT: This function filters out VPSieNodes that have deletion timestamps,
// as those are in the process of being deleted and should not be counted toward
// CurrentNodes. This prevents duplicate scale-down operations when a VPSieNode
// CR still exists (finalizers running) but the actual node is being terminated.
func (r *NodeGroupReconciler) listVPSieNodesForNodeGroup(ctx context.Context, ng *v1alpha1.NodeGroup) ([]v1alpha1.VPSieNode, error) {
	var vpsieNodeList v1alpha1.VPSieNodeList
	labels := GetNodeGroupLabels(ng)

	if err := r.List(ctx, &vpsieNodeList, client.MatchingLabels(labels)); err != nil {
		return nil, fmt.Errorf("failed to list VPSieNodes: %w", err)
	}

	// Filter out VPSieNodes that are being deleted (have deletion timestamps)
	// These nodes should not be counted toward CurrentNodes as they are in the
	// process of termination. Including them causes duplicate scale-down operations:
	// 1. First reconcile: deletes VPSieNode A, pod restarts
	// 2. Second reconcile: sees VPSieNode A (with deletion timestamp) + VPSieNode B
	// 3. CurrentNodes = 2 (incorrect), triggers another scale-down
	// 4. But K8s Node A is gone, so only Node B is found as candidate
	// 5. Node B gets deleted → BOTH nodes gone!
	activeNodes := make([]v1alpha1.VPSieNode, 0, len(vpsieNodeList.Items))
	for _, vn := range vpsieNodeList.Items {
		if vn.DeletionTimestamp == nil {
			activeNodes = append(activeNodes, vn)
		}
	}

	return activeNodes, nil
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
