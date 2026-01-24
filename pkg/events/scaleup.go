package events

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/tracing"
)

// ScaleUpDecision represents a decision to scale up a NodeGroup
type ScaleUpDecision struct {
	NodeGroup    *v1alpha1.NodeGroup
	CurrentNodes int32
	DesiredNodes int32
	NodesToAdd   int32
	InstanceType string
	MatchingPods int
	Deficit      ResourceDeficit
	Reason       string
}

// ScaleUpController handles scale-up decisions and executions
type ScaleUpController struct {
	client   client.Client
	analyzer *ResourceAnalyzer
	watcher  *EventWatcher
	creator  *DynamicNodeGroupCreator
	logger   *zap.Logger
}

// NewScaleUpController creates a new scale-up controller
func NewScaleUpController(
	client client.Client,
	analyzer *ResourceAnalyzer,
	watcher *EventWatcher,
	creator *DynamicNodeGroupCreator,
	logger *zap.Logger,
) *ScaleUpController {
	return &ScaleUpController{
		client:   client,
		analyzer: analyzer,
		watcher:  watcher,
		creator:  creator,
		logger:   logger.Named("scale-up-controller"),
	}
}

// SetWatcher sets the EventWatcher reference (for deferred initialization)
func (c *ScaleUpController) SetWatcher(watcher *EventWatcher) {
	c.watcher = watcher
}

// HandleScaleUp processes scheduling events and makes scale-up decisions
func (c *ScaleUpController) HandleScaleUp(ctx context.Context, events []SchedulingEvent) error {
	// Start Sentry transaction for tracing
	ctx, span := tracing.StartTransaction(ctx, "ScaleUpController.HandleScaleUp", "scaler.scale_up")
	if span != nil {
		span.SetTag("event_count", fmt.Sprintf("%d", len(events)))
		defer span.Finish()
	}

	c.logger.Info("Handling scale-up request",
		zap.Int("eventCount", len(events)),
	)

	// Get all pending pods
	pendingPods, err := c.watcher.GetPendingPods(ctx)
	if err != nil {
		return fmt.Errorf("failed to get pending pods: %w", err)
	}

	if len(pendingPods) == 0 {
		c.logger.Debug("No pending pods, skipping scale-up")
		return nil
	}

	c.logger.Info("Found pending pods",
		zap.Int("count", len(pendingPods)),
	)

	// Get all managed NodeGroups
	nodeGroups, err := c.watcher.GetNodeGroups(ctx)
	if err != nil {
		return fmt.Errorf("failed to get NodeGroups: %w", err)
	}

	// Find matching NodeGroups
	matches := c.analyzer.FindMatchingNodeGroups(pendingPods, nodeGroups)

	// If no suitable NodeGroup exists, try to create one dynamically
	if len(matches) == 0 && c.creator != nil {
		c.logger.Info("No suitable managed NodeGroups found, attempting dynamic creation",
			zap.Int("pendingPods", len(pendingPods)),
		)

		// Try to create a NodeGroup for the first pending pod
		// (additional pods with similar requirements will be handled by the new NodeGroup)
		ng, err := c.createNodeGroupForPendingPods(ctx, pendingPods)
		if err != nil {
			c.logger.Error("Failed to create dynamic NodeGroup",
				zap.Error(err),
			)
			return nil
		}

		if ng != nil {
			c.logger.Info("Created dynamic NodeGroup",
				zap.String("nodeGroup", ng.Name),
				zap.String("namespace", ng.Namespace),
			)

			// Add the new NodeGroup to the list and re-find matches
			nodeGroups = append(nodeGroups, *ng)
			matches = c.analyzer.FindMatchingNodeGroups(pendingPods, nodeGroups)
		}
	}

	if len(matches) == 0 {
		c.logger.Warn("No NodeGroups match pending pod requirements (after dynamic creation attempt)")
		return nil
	}

	c.logger.Info("Found NodeGroup matches",
		zap.Int("matchCount", len(matches)),
	)

	// Make scale-up decisions for each matching NodeGroup
	decisions := make([]ScaleUpDecision, 0)
	for _, match := range matches {
		decision, err := c.makeScaleUpDecision(ctx, match)
		if err != nil {
			c.logger.Error("Failed to make scale-up decision",
				zap.String("nodeGroup", match.NodeGroup.Name),
				zap.Error(err),
			)
			continue
		}

		if decision != nil {
			decisions = append(decisions, *decision)
		}
	}

	if len(decisions) == 0 {
		c.logger.Info("No scale-up decisions made")
		return nil
	}

	// Execute scale-up decisions
	for _, decision := range decisions {
		if err := c.executeScaleUp(ctx, decision); err != nil {
			c.logger.Error("Failed to execute scale-up",
				zap.String("nodeGroup", decision.NodeGroup.Name),
				zap.Error(err),
			)
			// Continue with other decisions
		}
	}

	return nil
}

// makeScaleUpDecision creates a scale-up decision for a NodeGroup match
func (c *ScaleUpController) makeScaleUpDecision(
	ctx context.Context,
	match NodeGroupMatch,
) (*ScaleUpDecision, error) {
	ng := match.NodeGroup

	// Check if NodeGroup can be scaled
	if !c.watcher.CanScale(ng.Name) {
		c.logger.Debug("NodeGroup is in cooldown period",
			zap.String("nodeGroup", ng.Name),
		)
		metrics.ScaleUpDecisionsTotal.WithLabelValues(ng.Name, ng.Namespace, "skipped_cooldown").Inc()
		return nil, nil
	}

	// Check if already at max capacity
	if ng.Status.DesiredNodes >= ng.Spec.MaxNodes {
		c.logger.Debug("NodeGroup is at max capacity",
			zap.String("nodeGroup", ng.Name),
			zap.Int32("desiredNodes", ng.Status.DesiredNodes),
			zap.Int32("maxNodes", ng.Spec.MaxNodes),
		)
		metrics.ScaleUpDecisionsTotal.WithLabelValues(ng.Name, ng.Namespace, "skipped_max_capacity").Inc()
		return nil, nil
	}

	// Select instance type
	instanceType, err := c.analyzer.SelectInstanceType(ng, match.Deficit)
	if err != nil {
		return nil, fmt.Errorf("failed to select instance type: %w", err)
	}

	// Get instance type info (for now, use default values)
	// TODO: Fetch actual instance type info from VPSie API
	instanceInfo := v1alpha1.InstanceTypeInfo{
		OfferingID: instanceType,
		CPU:        4,    // Default
		MemoryMB:   8192, // Default
		DiskGB:     80,   // Default
	}

	// Estimate nodes needed
	nodesNeeded := c.analyzer.EstimateNodesNeeded(match.Deficit, instanceInfo)

	// Account for nodes already being provisioned (not yet ready)
	// These nodes will accommodate some of the pending pods once ready
	nodesBeingProvisioned := ng.Status.DesiredNodes - ng.Status.CurrentNodes
	if nodesBeingProvisioned < 0 {
		nodesBeingProvisioned = 0
	}

	// Only add nodes beyond what's already being provisioned
	actualNodesNeeded := int32(nodesNeeded) - nodesBeingProvisioned
	if actualNodesNeeded <= 0 {
		c.logger.Debug("Nodes already being provisioned will satisfy demand",
			zap.String("nodeGroup", ng.Name),
			zap.Int("nodesNeeded", nodesNeeded),
			zap.Int32("nodesBeingProvisioned", nodesBeingProvisioned),
		)
		metrics.ScaleUpDecisionsTotal.WithLabelValues(ng.Name, ng.Namespace, "skipped_provisioning").Inc()
		return nil, nil
	}

	// Calculate actual nodes to add (respect max capacity)
	availableCapacity := ng.Spec.MaxNodes - ng.Status.DesiredNodes
	nodesToAdd := actualNodesNeeded
	if nodesToAdd > availableCapacity {
		nodesToAdd = availableCapacity
	}

	if nodesToAdd <= 0 {
		return nil, nil
	}

	desiredNodes := ng.Status.DesiredNodes + nodesToAdd

	c.logger.Info("Scale-up decision made",
		zap.String("nodeGroup", ng.Name),
		zap.Int32("currentNodes", ng.Status.CurrentNodes),
		zap.Int32("desiredNodes", desiredNodes),
		zap.Int32("nodesToAdd", nodesToAdd),
		zap.Int32("nodesBeingProvisioned", nodesBeingProvisioned),
		zap.String("instanceType", instanceType),
		zap.Int("matchingPods", len(match.MatchingPods)),
	)

	// Emit metrics for scale-up decision
	metrics.ScaleUpDecisionsTotal.WithLabelValues(ng.Name, ng.Namespace, "executed").Inc()
	metrics.ScaleUpDecisionNodesRequested.WithLabelValues(ng.Name, ng.Namespace).Observe(float64(nodesToAdd))

	return &ScaleUpDecision{
		NodeGroup:    ng,
		CurrentNodes: ng.Status.DesiredNodes,
		DesiredNodes: desiredNodes,
		NodesToAdd:   nodesToAdd,
		InstanceType: instanceType,
		MatchingPods: len(match.MatchingPods),
		Deficit:      match.Deficit,
		Reason:       fmt.Sprintf("Scaling up to accommodate %d pending pods", len(match.MatchingPods)),
	}, nil
}

// executeScaleUp executes a scale-up decision by updating the NodeGroup
func (c *ScaleUpController) executeScaleUp(ctx context.Context, decision ScaleUpDecision) error {
	c.logger.Info("Executing scale-up",
		zap.String("nodeGroup", decision.NodeGroup.Name),
		zap.Int32("from", decision.CurrentNodes),
		zap.Int32("to", decision.DesiredNodes),
	)

	// Get the latest version of the NodeGroup
	ng := &v1alpha1.NodeGroup{}
	err := c.client.Get(ctx, client.ObjectKey{
		Name:      decision.NodeGroup.Name,
		Namespace: decision.NodeGroup.Namespace,
	}, ng)
	if err != nil {
		return fmt.Errorf("failed to get NodeGroup: %w", err)
	}

	// Check if another controller already scaled it
	if ng.Status.DesiredNodes >= decision.DesiredNodes {
		c.logger.Info("NodeGroup already scaled by another controller",
			zap.String("nodeGroup", ng.Name),
			zap.Int32("currentDesired", ng.Status.DesiredNodes),
		)
		// Still record the scale event to prevent repeated scale-up attempts
		// This is important when the NodeGroup reconciler sets DesiredNodes
		// before our scale decision is executed
		c.watcher.RecordScaleEvent(ng.Name)
		return nil
	}

	// Update desired nodes and last scale time
	ng.Status.DesiredNodes = decision.DesiredNodes
	now := metav1.Now()
	ng.Status.LastScaleTime = &now

	// Update status
	err = c.client.Status().Update(ctx, ng)
	if err != nil {
		return fmt.Errorf("failed to update NodeGroup status: %w", err)
	}

	// Record scale event for cooldown
	c.watcher.RecordScaleEvent(ng.Name)

	c.logger.Info("Scale-up executed successfully",
		zap.String("nodeGroup", ng.Name),
		zap.Int32("desiredNodes", ng.Status.DesiredNodes),
		zap.String("reason", decision.Reason),
	)

	return nil
}

// GetScaleUpDecisions returns scale-up decisions without executing them (for testing)
func (c *ScaleUpController) GetScaleUpDecisions(
	ctx context.Context,
	events []SchedulingEvent,
) ([]ScaleUpDecision, error) {
	// Get all pending pods
	pendingPods, err := c.watcher.GetPendingPods(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending pods: %w", err)
	}

	if len(pendingPods) == 0 {
		return nil, nil
	}

	// Get all NodeGroups
	nodeGroups, err := c.watcher.GetNodeGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get NodeGroups: %w", err)
	}

	if len(nodeGroups) == 0 {
		return nil, nil
	}

	// Find matching NodeGroups
	matches := c.analyzer.FindMatchingNodeGroups(pendingPods, nodeGroups)
	if len(matches) == 0 {
		return nil, nil
	}

	// Make scale-up decisions
	decisions := make([]ScaleUpDecision, 0)
	for _, match := range matches {
		decision, err := c.makeScaleUpDecision(ctx, match)
		if err != nil {
			c.logger.Error("Failed to make scale-up decision",
				zap.String("nodeGroup", match.NodeGroup.Name),
				zap.Error(err),
			)
			continue
		}

		if decision != nil {
			decisions = append(decisions, *decision)
		}
	}

	return decisions, nil
}

// SetCreator sets the DynamicNodeGroupCreator reference (for deferred initialization)
func (c *ScaleUpController) SetCreator(creator *DynamicNodeGroupCreator) {
	c.creator = creator
}

// createNodeGroupForPendingPods creates a dynamic NodeGroup for pending pods.
// It groups pods by their scheduling requirements and creates a NodeGroup for the first group.
func (c *ScaleUpController) createNodeGroupForPendingPods(
	ctx context.Context,
	pendingPods []corev1.Pod,
) (*v1alpha1.NodeGroup, error) {
	if c.creator == nil {
		return nil, fmt.Errorf("dynamic NodeGroup creator not configured")
	}

	if len(pendingPods) == 0 {
		return nil, nil
	}

	// Use the first pending pod as representative for the NodeGroup requirements
	// Pods with similar requirements will be able to schedule on the same NodeGroup
	pod := &pendingPods[0]

	c.logger.Info("Creating dynamic NodeGroup for pending pod",
		zap.String("pod", pod.Name),
		zap.String("namespace", pod.Namespace),
	)

	// Create the NodeGroup in the same namespace as the first pending pod
	// This ensures proper RBAC and resource isolation
	namespace := pod.Namespace
	if namespace == "" {
		namespace = "default"
	}

	ng, err := c.creator.CreateNodeGroupForPod(ctx, pod, namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to create NodeGroup for pod %s/%s: %w", pod.Namespace, pod.Name, err)
	}

	return ng, nil
}
