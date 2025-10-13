package nodegroup

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
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

// NodeGroupReconciler reconciles a NodeGroup object
type NodeGroupReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	VPSieClient *vpsieclient.Client
	Logger      *zap.Logger
}

// SetupWithManager sets up the controller with the Manager
func (r *NodeGroupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.NodeGroup{}).
		Owns(&v1alpha1.VPSieNode{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: DefaultMaxConcurrentReconciles,
		}).
		Complete(r)
}

// NewNodeGroupReconciler creates a new NodeGroupReconciler
func NewNodeGroupReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	vpsieClient *vpsieclient.Client,
	logger *zap.Logger,
) *NodeGroupReconciler {
	return &NodeGroupReconciler{
		Client:      client,
		Scheme:      scheme,
		VPSieClient: vpsieClient,
		Logger:      logger.Named(ControllerName),
	}
}

// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=nodegroups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=nodegroups/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=nodegroups/finalizers,verbs=update
// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=vpsienodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaler.vpsie.com,resources=vpsienodes/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *NodeGroupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Logger.With(
		zap.String("namespace", req.Namespace),
		zap.String("name", req.Name),
	)

	logger.Info("Reconciling NodeGroup")

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
		return ctrl.Result{RequeueAfter: 5 * 1000000000}, nil // 5 seconds
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

// listVPSieNodesForNodeGroup lists all VPSieNodes belonging to a NodeGroup
func (r *NodeGroupReconciler) listVPSieNodesForNodeGroup(ctx context.Context, ng *v1alpha1.NodeGroup) ([]v1alpha1.VPSieNode, error) {
	var vpsieNodeList v1alpha1.VPSieNodeList
	labels := GetNodeGroupLabels(ng)

	if err := r.List(ctx, &vpsieNodeList, client.MatchingLabels(labels)); err != nil {
		return nil, fmt.Errorf("failed to list VPSieNodes: %w", err)
	}

	return vpsieNodeList.Items, nil
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
