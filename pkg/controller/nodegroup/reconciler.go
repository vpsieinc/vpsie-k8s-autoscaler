package nodegroup

import (
	"context"
	"crypto/rand"
	"fmt"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

const (
	// DefaultRequeueAfter is the default requeue time
	DefaultRequeueAfter = 30 * time.Second

	// FastRequeueAfter is used when operations are in progress
	FastRequeueAfter = 10 * time.Second
)

// reconcile handles the main reconciliation logic
func (r *NodeGroupReconciler) reconcile(ctx context.Context, ng *v1alpha1.NodeGroup, logger *zap.Logger) (ctrl.Result, error) {
	// Validate the NodeGroup spec
	if err := ValidateNodeGroupSpec(ng); err != nil {
		logger.Error("NodeGroup spec validation failed", zap.Error(err))
		r.Recorder.Event(ng, corev1.EventTypeWarning, "ValidationFailed",
			fmt.Sprintf("Spec validation failed: %v", err))

		// Create patch BEFORE modifications for proper optimistic locking
		patch := client.MergeFrom(ng.DeepCopy())
		SetErrorCondition(ng, true, ReasonValidationFailed, err.Error())
		SetReadyCondition(ng, false, ReasonValidationFailed, "Spec validation failed")

		// Update status with optimistic locking
		if statusErr := r.Status().Patch(ctx, ng, patch); statusErr != nil {
			if apierrors.IsConflict(statusErr) {
				logger.Info("Status update conflict, will retry")
				return ctrl.Result{Requeue: true}, nil
			}
			logger.Error("Failed to update status", zap.Error(statusErr))
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{}, err
	}

	// Ensure VPSie node group exists on the platform
	if ng.Status.VPSieGroupID == 0 && r.VPSieClient != nil {
		result, err := r.ensureVPSieNodeGroup(ctx, ng, logger)
		if err != nil {
			return result, err
		}
		if result.Requeue || result.RequeueAfter > 0 {
			return result, nil
		}
	}

	// Create patch BEFORE any status modifications for proper optimistic locking
	// MergeFrom captures the original state, and Patch computes the diff to the modified state
	patch := client.MergeFrom(ng.DeepCopy())

	// Update conditions for reconciliation start
	UpdateConditionsForReconcile(ng)

	// List existing VPSieNodes for this NodeGroup
	vpsieNodes, err := r.listVPSieNodesForNodeGroup(ctx, ng)
	if err != nil {
		logger.Error("Failed to list VPSieNodes", zap.Error(err))

		// Create patch BEFORE modifications for proper optimistic locking
		patch := client.MergeFrom(ng.DeepCopy())
		SetErrorCondition(ng, true, ReasonKubernetesAPIError, fmt.Sprintf("Failed to list VPSieNodes: %v", err))

		// Update status with optimistic locking
		if statusErr := r.Status().Patch(ctx, ng, patch); statusErr != nil {
			if apierrors.IsConflict(statusErr) {
				logger.Info("Status update conflict, will retry")
				return ctrl.Result{Requeue: true}, nil
			}
			logger.Error("Failed to update status", zap.Error(statusErr))
			return ctrl.Result{}, statusErr
		}

		return ctrl.Result{}, err
	}

	logger.Info("Found VPSieNodes",
		zap.Int("count", len(vpsieNodes)),
	)

	// Update status with current state BEFORE creating patch
	if err := UpdateNodeGroupStatus(ctx, r.Client, ng, vpsieNodes); err != nil {
		logger.Error("Failed to update NodeGroup status", zap.Error(err))
		return ctrl.Result{}, err
	}

	// Calculate desired nodes
	desired := CalculateDesiredNodes(ng)
	if ng.Status.DesiredNodes != desired {
		SetDesiredNodes(ng, desired)
		logger.Info("Updated desired node count",
			zap.Int32("desired", desired),
			zap.Int32("current", ng.Status.CurrentNodes),
		)
	}

	// Determine if scaling is needed
	needsScaleUp := NeedsScaleUp(ng)
	needsScaleDown := NeedsScaleDown(ng)

	var result ctrl.Result
	var reconcileErr error

	if needsScaleUp {
		logger.Info("Scaling up",
			zap.Int32("current", ng.Status.CurrentNodes),
			zap.Int32("desired", ng.Status.DesiredNodes),
		)
		nodesToAdd := ng.Status.DesiredNodes - ng.Status.CurrentNodes
		r.Recorder.Eventf(ng, corev1.EventTypeNormal, "ScalingUp",
			"Scaling up from %d to %d nodes (+%d nodes)", ng.Status.CurrentNodes, ng.Status.DesiredNodes, nodesToAdd)
		result, reconcileErr = r.reconcileScaleUp(ctx, ng, vpsieNodes, logger)
	} else if needsScaleDown {
		logger.Info("Scaling down",
			zap.Int32("current", ng.Status.CurrentNodes),
			zap.Int32("desired", ng.Status.DesiredNodes),
		)
		nodesToRemove := ng.Status.CurrentNodes - ng.Status.DesiredNodes
		r.Recorder.Eventf(ng, corev1.EventTypeNormal, "ScalingDown",
			"Scaling down from %d to %d nodes (-%d nodes)", ng.Status.CurrentNodes, ng.Status.DesiredNodes, nodesToRemove)
		result, reconcileErr = r.reconcileScaleDown(ctx, ng, vpsieNodes, logger)
	} else {
		// No explicit scaling needed - check if utilization-based scale-down should trigger
		if r.ScaleDownManager != nil && ng.Spec.ScaleDownPolicy.Enabled &&
			ng.Status.CurrentNodes > ng.Spec.MinNodes {
			// Evaluate if we should scale down based on utilization
			if shouldScaleDown, nodesToRemove := r.evaluateUtilizationBasedScaleDown(ctx, ng, logger); shouldScaleDown {
				logger.Info("Utilization-based scale-down triggered",
					zap.Int32("current", ng.Status.CurrentNodes),
					zap.Int("nodesToRemove", nodesToRemove),
				)
				// Reduce DesiredNodes to trigger scale-down
				newDesired := ng.Status.CurrentNodes - int32(nodesToRemove)
				if newDesired < ng.Spec.MinNodes {
					newDesired = ng.Spec.MinNodes
				}
				SetDesiredNodes(ng, newDesired)
				r.Recorder.Eventf(ng, corev1.EventTypeNormal, "ScalingDown",
					"Utilization-based scale-down: reducing from %d to %d nodes (-%d nodes)",
					ng.Status.CurrentNodes, newDesired, nodesToRemove)
				result, reconcileErr = r.reconcileScaleDown(ctx, ng, vpsieNodes, logger)
				needsScaleDown = true // For condition update below
			} else {
				logger.Debug("No utilization-based scale-down needed",
					zap.Int32("current", ng.Status.CurrentNodes),
					zap.Int32("min", ng.Spec.MinNodes),
				)
				result = ctrl.Result{RequeueAfter: DefaultRequeueAfter}
			}
		} else {
			logger.Info("No scaling needed",
				zap.Int32("current", ng.Status.CurrentNodes),
				zap.Int32("desired", ng.Status.DesiredNodes),
				zap.Int32("ready", ng.Status.ReadyNodes),
			)
			result = ctrl.Result{RequeueAfter: DefaultRequeueAfter}
		}
	}

	// Update conditions after scaling decision
	if needsScaleUp {
		UpdateConditionsAfterScale(ng, "up")
	} else if needsScaleDown {
		UpdateConditionsAfterScale(ng, "down")
	} else {
		UpdateConditionsAfterScale(ng, "")
	}

	// Clear error condition if no error occurred
	if reconcileErr == nil {
		SetErrorCondition(ng, false, ReasonReconciling, "")
	}

	// Apply status changes with optimistic locking (patch was created before modifications)
	if err := r.Status().Patch(ctx, ng, patch); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Status update conflict, will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error("Failed to update status", zap.Error(err))
		return ctrl.Result{}, err
	}

	logger.Info("Reconciliation complete",
		zap.Bool("requeue", result.Requeue || result.RequeueAfter > 0),
		zap.Duration("requeueAfter", result.RequeueAfter),
	)

	return result, reconcileErr
}

// reconcileScaleUp handles scaling up the NodeGroup
// Uses sequential scaling: only creates one node at a time and waits for it to be Ready
// before creating additional nodes. This prevents over-provisioning and respects cluster limits.
func (r *NodeGroupReconciler) reconcileScaleUp(
	ctx context.Context,
	ng *v1alpha1.NodeGroup,
	vpsieNodes []v1alpha1.VPSieNode,
	logger *zap.Logger,
) (ctrl.Result, error) {
	// Calculate how many nodes to add
	nodesToAdd := CalculateNodesToAdd(ng)
	if nodesToAdd <= 0 {
		logger.Info("No nodes to add")
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, nil
	}

	// Sequential scaling: check if any nodes are still in transition (not yet Ready)
	// If so, wait for them to be Ready before creating more nodes
	nodesInTransition := CountNodesInTransition(vpsieNodes)
	if nodesInTransition > 0 {
		logger.Info("Waiting for nodes in transition to be Ready before scaling up",
			zap.Int("nodesInTransition", nodesInTransition),
			zap.Int32("totalNodesToAdd", nodesToAdd),
			zap.Int32("currentNodes", ng.Status.CurrentNodes),
			zap.Int32("readyNodes", ng.Status.ReadyNodes),
			zap.Int32("desiredNodes", ng.Status.DesiredNodes),
		)
		// Requeue with fast interval to check when nodes become Ready
		return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
	}

	// Sequential scaling: only create ONE node at a time
	// After this node becomes Ready, the reconciler will run again and create the next one
	logger.Info("Creating new VPSieNode (sequential scaling: 1 at a time)",
		zap.Int32("remainingToAdd", nodesToAdd),
		zap.Int32("currentNodes", ng.Status.CurrentNodes),
		zap.Int32("desiredNodes", ng.Status.DesiredNodes),
	)

	vpsieNode := r.buildVPSieNode(ng)

	// Set owner reference
	if err := controllerutil.SetControllerReference(ng, vpsieNode, r.Scheme); err != nil {
		logger.Error("Failed to set owner reference", zap.Error(err))
		SetErrorCondition(ng, true, ReasonKubernetesAPIError, fmt.Sprintf("Failed to set owner reference: %v", err))
		return ctrl.Result{}, err
	}

	// Create the VPSieNode
	if err := r.Create(ctx, vpsieNode); err != nil {
		logger.Error("Failed to create VPSieNode",
			zap.String("vpsienode", vpsieNode.Name),
			zap.Error(err),
		)
		SetErrorCondition(ng, true, ReasonNodeProvisioningFailed, fmt.Sprintf("Failed to create VPSieNode: %v", err))
		return ctrl.Result{}, err
	}

	logger.Info("Created VPSieNode, waiting for it to be Ready before creating more",
		zap.String("vpsienode", vpsieNode.Name),
		zap.Int32("remainingAfterThis", nodesToAdd-1),
	)

	// Requeue to check progress - the next reconcile will create another node
	// once this one is Ready
	return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
}

// reconcileScaleDown handles scaling down the NodeGroup using intelligent scale-down
func (r *NodeGroupReconciler) reconcileScaleDown(
	ctx context.Context,
	ng *v1alpha1.NodeGroup,
	vpsieNodes []v1alpha1.VPSieNode,
	logger *zap.Logger,
) (ctrl.Result, error) {
	// Use ScaleDownManager if available for intelligent scale-down
	if r.ScaleDownManager != nil {
		return r.reconcileIntelligentScaleDown(ctx, ng, vpsieNodes, logger)
	}

	// Fallback to simple scale-down if ScaleDownManager not available
	return r.reconcileSimpleScaleDown(ctx, ng, vpsieNodes, logger)
}

// reconcileIntelligentScaleDown uses ScaleDownManager for safe, utilization-based scale-down
func (r *NodeGroupReconciler) reconcileIntelligentScaleDown(
	ctx context.Context,
	ng *v1alpha1.NodeGroup,
	vpsieNodes []v1alpha1.VPSieNode,
	logger *zap.Logger,
) (ctrl.Result, error) {
	logger.Info("Using intelligent scale-down based on node utilization")

	// Identify underutilized nodes
	candidates, err := r.ScaleDownManager.IdentifyUnderutilizedNodes(ctx, ng)
	if err != nil {
		logger.Error("Failed to identify underutilized nodes", zap.Error(err))
		SetErrorCondition(ng, true, ReasonScaleDownFailed, fmt.Sprintf("Failed to identify candidates: %v", err))
		return ctrl.Result{}, err
	}

	if len(candidates) == 0 {
		logger.Info("No underutilized nodes found for scale-down")
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, nil
	}

	logger.Info("Found scale-down candidates",
		zap.Int("candidateCount", len(candidates)),
	)

	// Perform intelligent scale-down with safety checks (drains nodes)
	if err := r.ScaleDownManager.ScaleDown(ctx, ng, candidates); err != nil {
		logger.Error("Intelligent scale-down failed", zap.Error(err))
		SetErrorCondition(ng, true, ReasonScaleDownFailed, fmt.Sprintf("Scale-down failed: %v", err))
		return ctrl.Result{}, err
	}

	// After successful drain, delete the corresponding VPSieNode CRs
	// The VPSieNode controller will handle VM termination and K8s node deletion

	// Build maps for O(1) lookup instead of O(n*m) nested loops
	// Map by Status.NodeName (set when node joins K8s cluster)
	vpsieNodeByNodeName := make(map[string]*v1alpha1.VPSieNode)
	// Fallback map by Spec.NodeName (hostname from VPSie)
	vpsieNodeBySpecName := make(map[string]*v1alpha1.VPSieNode)
	// Fallback map by Status.Hostname (VPSie hostname)
	vpsieNodeByHostname := make(map[string]*v1alpha1.VPSieNode)

	for i := range vpsieNodes {
		vn := &vpsieNodes[i]
		if vn.Status.NodeName != "" {
			vpsieNodeByNodeName[vn.Status.NodeName] = vn
		}
		if vn.Spec.NodeName != "" {
			vpsieNodeBySpecName[vn.Spec.NodeName] = vn
		}
		if vn.Status.Hostname != "" {
			vpsieNodeByHostname[vn.Status.Hostname] = vn
		}
	}

	logger.Debug("Built VPSieNode lookup maps",
		zap.Int("byNodeName", len(vpsieNodeByNodeName)),
		zap.Int("bySpecName", len(vpsieNodeBySpecName)),
		zap.Int("byHostname", len(vpsieNodeByHostname)),
	)

	deletedCount := 0
	var deletionErrors []error
	for _, candidate := range candidates {
		nodeName := candidate.Node.Name

		// Find the VPSieNode CR for this node using multiple lookup strategies
		vn, ok := vpsieNodeByNodeName[nodeName]
		if !ok {
			// Fallback: try Spec.NodeName
			vn, ok = vpsieNodeBySpecName[nodeName]
		}
		if !ok {
			// Fallback: try Status.Hostname
			vn, ok = vpsieNodeByHostname[nodeName]
		}
		if !ok {
			// CRITICAL: Log when we can't find a VPSieNode for a drained node
			logger.Error("CRITICAL: Cannot find VPSieNode for drained node - VPS will NOT be deleted!",
				zap.String("drainedNodeName", nodeName),
				zap.Int("totalVPSieNodes", len(vpsieNodes)),
			)
			// Record metric for this failure
			metrics.ScaleDownErrorsTotal.WithLabelValues(
				ng.Name,
				ng.Namespace,
				"vpsienode_not_found",
			).Inc()
			continue
		}

		logger.Info("Deleting VPSieNode after successful drain",
			zap.String("vpsienode", vn.Name),
			zap.String("nodeName", candidate.Node.Name),
		)

		if err := r.Delete(ctx, vn); err != nil {
			logger.Error("Failed to delete VPSieNode",
				zap.String("vpsienode", vn.Name),
				zap.Error(err),
			)
			deletionErrors = append(deletionErrors, fmt.Errorf("delete VPSieNode %s: %w", vn.Name, err))
			// Continue with other nodes - don't fail entire operation
			continue
		}

		deletedCount++
	}

	// Log warning if some deletions failed
	if len(deletionErrors) > 0 {
		logger.Warn("Some VPSieNode deletions failed during scale-down",
			zap.Int("failedCount", len(deletionErrors)),
			zap.Errors("errors", deletionErrors),
		)
	}

	logger.Info("Intelligent scale-down completed",
		zap.Int("nodesDrained", len(candidates)),
		zap.Int("vpsieNodesDeleted", deletedCount),
		zap.Int("deletionsFailed", len(deletionErrors)),
	)

	// Requeue to verify scale-down progress
	return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
}

// reconcileSimpleScaleDown is the fallback simple scale-down (original implementation)
func (r *NodeGroupReconciler) reconcileSimpleScaleDown(
	ctx context.Context,
	ng *v1alpha1.NodeGroup,
	vpsieNodes []v1alpha1.VPSieNode,
	logger *zap.Logger,
) (ctrl.Result, error) {
	logger.Warn("Using simple scale-down (ScaleDownManager not available)")

	// Calculate how many nodes to remove
	nodesToRemove := CalculateNodesToRemove(ng)
	if nodesToRemove <= 0 {
		logger.Info("No nodes to remove")
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, nil
	}

	logger.Info("Removing VPSieNodes",
		zap.Int32("count", nodesToRemove),
	)

	// Find nodes to delete (prefer nodes that are not ready)
	nodesToDelete := selectNodesToDelete(vpsieNodes, int(nodesToRemove))

	// Delete selected nodes
	for _, vn := range nodesToDelete {
		logger.Info("Deleting VPSieNode",
			zap.String("vpsienode", vn.Name),
			zap.String("phase", string(vn.Status.Phase)),
		)

		if err := r.Delete(ctx, &vn); err != nil {
			logger.Error("Failed to delete VPSieNode",
				zap.String("vpsienode", vn.Name),
				zap.Error(err),
			)
			SetErrorCondition(ng, true, ReasonKubernetesAPIError, fmt.Sprintf("Failed to delete VPSieNode: %v", err))
			return ctrl.Result{}, err
		}
	}

	// Requeue to check progress
	return ctrl.Result{RequeueAfter: FastRequeueAfter}, nil
}

// evaluateUtilizationBasedScaleDown checks if scale-down should be triggered based on node utilization.
// Returns true and the number of nodes to remove if scale-down is warranted.
func (r *NodeGroupReconciler) evaluateUtilizationBasedScaleDown(
	ctx context.Context,
	ng *v1alpha1.NodeGroup,
	logger *zap.Logger,
) (bool, int) {
	if r.ScaleDownManager == nil {
		return false, 0
	}

	// Check cooldown period from last scale action
	if ng.Status.LastScaleDownTime != nil {
		cooldown := time.Duration(ng.Spec.ScaleDownPolicy.CooldownSeconds) * time.Second
		if time.Since(ng.Status.LastScaleDownTime.Time) < cooldown {
			logger.Debug("Within scale-down cooldown period",
				zap.Duration("cooldown", cooldown),
				zap.Duration("elapsed", time.Since(ng.Status.LastScaleDownTime.Time)),
			)
			return false, 0
		}
	}

	// Also check cooldown from last scale-up (stabilization)
	if ng.Status.LastScaleTime != nil {
		stabilization := time.Duration(ng.Spec.ScaleDownPolicy.StabilizationWindowSeconds) * time.Second
		if time.Since(ng.Status.LastScaleTime.Time) < stabilization {
			logger.Debug("Within stabilization window after scale-up",
				zap.Duration("stabilization", stabilization),
				zap.Duration("elapsed", time.Since(ng.Status.LastScaleTime.Time)),
			)
			return false, 0
		}
	}

	// Identify underutilized nodes
	candidates, err := r.ScaleDownManager.IdentifyUnderutilizedNodes(ctx, ng)
	if err != nil {
		logger.Error("Failed to identify underutilized nodes for evaluation", zap.Error(err))
		return false, 0
	}

	if len(candidates) == 0 {
		logger.Debug("No underutilized nodes found")
		return false, 0
	}

	// Determine how many nodes can be removed while staying above MinNodes
	currentNodes := int(ng.Status.CurrentNodes)
	minNodes := int(ng.Spec.MinNodes)
	maxRemovable := currentNodes - minNodes

	if maxRemovable <= 0 {
		logger.Debug("At minimum nodes, cannot scale down",
			zap.Int("current", currentNodes),
			zap.Int("min", minNodes),
		)
		return false, 0
	}

	// Only remove underutilized nodes up to the max removable
	nodesToRemove := len(candidates)
	if nodesToRemove > maxRemovable {
		nodesToRemove = maxRemovable
	}

	logger.Info("Utilization-based scale-down evaluation complete",
		zap.Int("underutilizedCandidates", len(candidates)),
		zap.Int("nodesToRemove", nodesToRemove),
		zap.Int("currentNodes", currentNodes),
		zap.Int("minNodes", minNodes),
	)

	return true, nodesToRemove
}

// buildVPSieNode creates a new VPSieNode spec for the NodeGroup
func (r *NodeGroupReconciler) buildVPSieNode(ng *v1alpha1.NodeGroup) *v1alpha1.VPSieNode {
	// Generate unique name
	name := fmt.Sprintf("%s-%s", ng.Name, generateRandomSuffix())

	// Select instance type (use first offering for now)
	instanceType := ng.Spec.OfferingIDs[0]
	if ng.Spec.PreferredInstanceType != "" {
		instanceType = ng.Spec.PreferredInstanceType
	}

	// Build VPSieNode
	vpsieNode := &v1alpha1.VPSieNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ng.Namespace,
			Labels:    GetNodeGroupLabels(ng),
		},
		Spec: v1alpha1.VPSieNodeSpec{
			VPSieInstanceID:    0, // Will be set by VPSieNode controller
			InstanceType:       instanceType,
			NodeGroupName:      ng.Name,
			DatacenterID:       ng.Spec.DatacenterID,
			ResourceIdentifier: ng.Spec.ResourceIdentifier,
			Project:            ng.Spec.Project,
			OSImageID:          ng.Spec.OSImageID,
			KubernetesVersion:  ng.Spec.KubernetesVersion,
			SSHKeyIDs:          ng.Spec.SSHKeyIDs,
			VPSieGroupID:       ng.Status.VPSieGroupID, // Pass VPSie node group ID for API
			// Note: UserData/cloud-init support removed in v0.6.0
			// Node configuration is now handled entirely via VPSie API
		},
	}

	return vpsieNode
}

// selectNodesToDelete selects which nodes should be deleted during scale-down
// Prioritizes nodes that are not ready
func selectNodesToDelete(vpsieNodes []v1alpha1.VPSieNode, count int) []v1alpha1.VPSieNode {
	if count >= len(vpsieNodes) {
		return vpsieNodes
	}

	// Sort nodes by priority (not ready first, then oldest)
	var notReady []v1alpha1.VPSieNode
	var ready []v1alpha1.VPSieNode

	for i := range vpsieNodes {
		vn := vpsieNodes[i]
		if vn.Status.Phase != v1alpha1.VPSieNodePhaseReady {
			notReady = append(notReady, vn)
		} else {
			ready = append(ready, vn)
		}
	}

	var result []v1alpha1.VPSieNode

	// First, select not-ready nodes
	for i := 0; i < count && i < len(notReady); i++ {
		result = append(result, notReady[i])
	}

	// If we need more, select from ready nodes (oldest first)
	remaining := count - len(result)
	for i := 0; i < remaining && i < len(ready); i++ {
		result = append(result, ready[i])
	}

	return result
}

// generateRandomSuffix generates a cryptographically secure random suffix for resource names
// Returns an 8-character hexadecimal string (2^32 possible values, extremely low collision probability)
func generateRandomSuffix() string {
	b := make([]byte, 4) // 4 bytes = 8 hex characters
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based if crypto/rand fails (should never happen)
		return fmt.Sprintf("%x", time.Now().UnixNano()%0xFFFFFFFF)
	}
	return fmt.Sprintf("%x", b)
}

// ensureVPSieNodeGroup ensures the node group exists on VPSie platform
// Creates the node group if it doesn't exist and stores the numeric group ID in status
func (r *NodeGroupReconciler) ensureVPSieNodeGroup(ctx context.Context, ng *v1alpha1.NodeGroup, logger *zap.Logger) (ctrl.Result, error) {
	logger.Info("Ensuring node group exists on VPSie platform",
		zap.String("nodegroup", ng.Name),
		zap.String("cluster", ng.Spec.ResourceIdentifier),
		zap.Int("kubeSizeID", ng.Spec.KubeSizeID),
	)

	// Validate required fields for VPSie node group creation
	if ng.Spec.KubeSizeID == 0 {
		logger.Error("KubeSizeID is required to create node group on VPSie")
		r.Recorder.Event(ng, corev1.EventTypeWarning, "ValidationFailed",
			"KubeSizeID is required to create node group on VPSie platform")
		SetErrorCondition(ng, true, ReasonValidationFailed, "KubeSizeID is required")
		return ctrl.Result{}, fmt.Errorf("kubeSizeID is required")
	}

	if ng.Spec.ResourceIdentifier == "" {
		logger.Error("ResourceIdentifier is required to create node group on VPSie")
		r.Recorder.Event(ng, corev1.EventTypeWarning, "ValidationFailed",
			"ResourceIdentifier (cluster ID) is required to create node group on VPSie platform")
		SetErrorCondition(ng, true, ReasonValidationFailed, "ResourceIdentifier is required")
		return ctrl.Result{}, fmt.Errorf("resourceIdentifier is required")
	}

	// First, check if node group already exists on VPSie by listing groups
	groups, err := r.VPSieClient.ListK8sNodeGroups(ctx, ng.Spec.ResourceIdentifier)
	if err != nil {
		logger.Error("Failed to list node groups from VPSie",
			zap.String("cluster", ng.Spec.ResourceIdentifier),
			zap.Error(err),
		)
		r.Recorder.Eventf(ng, corev1.EventTypeWarning, "VPSieAPIError",
			"Failed to list node groups from VPSie: %v", err)
		SetErrorCondition(ng, true, ReasonVPSieAPIError, fmt.Sprintf("Failed to list VPSie node groups: %v", err))
		return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, err
	}

	// Check if group already exists by name
	var numericGroupID int
	for _, group := range groups {
		if group.GroupName == ng.Name {
			numericGroupID = group.ID
			logger.Info("Found existing node group on VPSie platform",
				zap.String("nodegroup", ng.Name),
				zap.Int("vpsieGroupID", numericGroupID),
			)
			break
		}
	}

	// If group doesn't exist, create it
	if numericGroupID == 0 {
		logger.Info("Creating node group on VPSie platform",
			zap.String("nodegroup", ng.Name),
			zap.Int("kubeSizeID", ng.Spec.KubeSizeID),
		)

		// Create node group on VPSie
		req := vpsieclient.CreateK8sNodeGroupRequest{
			ClusterIdentifier: ng.Spec.ResourceIdentifier,
			GroupName:         ng.Name,
			KubeSizeID:        ng.Spec.KubeSizeID,
		}

		_, err := r.VPSieClient.CreateK8sNodeGroup(ctx, req)
		if err != nil {
			logger.Error("Failed to create node group on VPSie",
				zap.String("nodegroup", ng.Name),
				zap.Error(err),
			)
			r.Recorder.Eventf(ng, corev1.EventTypeWarning, "VPSieAPIError",
				"Failed to create node group on VPSie: %v", err)
			SetErrorCondition(ng, true, ReasonVPSieAPIError, fmt.Sprintf("Failed to create VPSie node group: %v", err))
			return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, err
		}

		// Fetch the list again to get the numeric ID
		groups, err = r.VPSieClient.ListK8sNodeGroups(ctx, ng.Spec.ResourceIdentifier)
		if err != nil {
			logger.Error("Failed to list node groups after creation",
				zap.String("cluster", ng.Spec.ResourceIdentifier),
				zap.Error(err),
			)
			return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, err
		}

		// Find the newly created group
		for _, group := range groups {
			if group.GroupName == ng.Name {
				numericGroupID = group.ID
				break
			}
		}

		if numericGroupID == 0 {
			logger.Error("Created node group but could not find its numeric ID",
				zap.String("nodegroup", ng.Name),
			)
			return ctrl.Result{RequeueAfter: DefaultRequeueAfter}, fmt.Errorf("could not find numeric ID for created node group")
		}

		logger.Info("Created node group on VPSie platform",
			zap.String("nodegroup", ng.Name),
			zap.Int("vpsieGroupID", numericGroupID),
		)

		r.Recorder.Eventf(ng, corev1.EventTypeNormal, "VPSieNodeGroupCreated",
			"Created node group %s on VPSie platform (ID: %d)", ng.Name, numericGroupID)
	}

	// Update status with numeric VPSie group ID
	patch := client.MergeFrom(ng.DeepCopy())
	ng.Status.VPSieGroupID = numericGroupID

	if err := r.Status().Patch(ctx, ng, patch); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Status update conflict after setting VPSie node group ID, will retry")
			return ctrl.Result{Requeue: true}, nil
		}
		logger.Error("Failed to update status with VPSie group ID", zap.Error(err))
		return ctrl.Result{}, err
	}

	// Requeue to continue with normal reconciliation
	return ctrl.Result{Requeue: true}, nil
}
