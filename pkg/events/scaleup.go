package events

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
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
	logger   *zap.Logger
}

// NewScaleUpController creates a new scale-up controller
func NewScaleUpController(
	client client.Client,
	analyzer *ResourceAnalyzer,
	watcher *EventWatcher,
	logger *zap.Logger,
) *ScaleUpController {
	return &ScaleUpController{
		client:   client,
		analyzer: analyzer,
		watcher:  watcher,
		logger:   logger.Named("scale-up-controller"),
	}
}

// SetWatcher sets the EventWatcher reference (for deferred initialization)
func (c *ScaleUpController) SetWatcher(watcher *EventWatcher) {
	c.watcher = watcher
}

// HandleScaleUp processes scheduling events and makes scale-up decisions
func (c *ScaleUpController) HandleScaleUp(ctx context.Context, events []SchedulingEvent) error {
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

	// Get all NodeGroups
	nodeGroups, err := c.watcher.GetNodeGroups(ctx)
	if err != nil {
		return fmt.Errorf("failed to get NodeGroups: %w", err)
	}

	if len(nodeGroups) == 0 {
		c.logger.Warn("No NodeGroups available for scale-up")
		return nil
	}

	// Find matching NodeGroups
	matches := c.analyzer.FindMatchingNodeGroups(pendingPods, nodeGroups)
	if len(matches) == 0 {
		c.logger.Warn("No NodeGroups match pending pod requirements")
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
		return nil, nil
	}

	// Check if already at max capacity
	if ng.Status.DesiredNodes >= ng.Spec.MaxNodes {
		c.logger.Debug("NodeGroup is at max capacity",
			zap.String("nodeGroup", ng.Name),
			zap.Int32("desiredNodes", ng.Status.DesiredNodes),
			zap.Int32("maxNodes", ng.Spec.MaxNodes),
		)
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

	// Calculate actual nodes to add (respect max capacity)
	availableCapacity := ng.Spec.MaxNodes - ng.Status.DesiredNodes
	nodesToAdd := int32(nodesNeeded)
	if nodesToAdd > availableCapacity {
		nodesToAdd = availableCapacity
	}

	if nodesToAdd <= 0 {
		return nil, nil
	}

	desiredNodes := ng.Status.DesiredNodes + nodesToAdd

	c.logger.Info("Scale-up decision made",
		zap.String("nodeGroup", ng.Name),
		zap.Int32("currentNodes", ng.Status.DesiredNodes),
		zap.Int32("desiredNodes", desiredNodes),
		zap.Int32("nodesToAdd", nodesToAdd),
		zap.String("instanceType", instanceType),
		zap.Int("matchingPods", len(match.MatchingPods)),
	)

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
