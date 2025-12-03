package rebalancer

import (
	"context"
	"fmt"

	v1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/record"
)

// EventRecorder handles recording Kubernetes events for rebalancing operations
type EventRecorder struct {
	recorder    record.EventRecorder
	broadcaster record.EventBroadcaster
}

// NewEventRecorder creates a new event recorder for rebalancing
func NewEventRecorder(kubeClient kubernetes.Interface) *EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{
		Interface: kubeClient.CoreV1().Events(""),
	})

	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{
		Component: "vpsie-rebalancer",
	})

	return &EventRecorder{
		recorder:    recorder,
		broadcaster: eventBroadcaster,
	}
}

// Shutdown stops the event broadcaster and cleans up resources
func (e *EventRecorder) Shutdown() {
	if e.broadcaster != nil {
		e.broadcaster.Shutdown()
	}
}

// Event types
const (
	// Plan events
	EventPlanCreated   = "PlanCreated"
	EventPlanStarted   = "PlanStarted"
	EventPlanCompleted = "PlanCompleted"
	EventPlanFailed    = "PlanFailed"

	// Safety check events
	EventSafetyCheckPassed = "SafetyCheckPassed"
	EventSafetyCheckFailed = "SafetyCheckFailed"

	// Node events
	EventNodeProvisioning = "NodeProvisioning"
	EventNodeProvisioned  = "NodeProvisioned"
	EventNodeDraining     = "NodeDraining"
	EventNodeDrained      = "NodeDrained"
	EventNodeTerminating  = "NodeTerminating"
	EventNodeTerminated   = "NodeTerminated"
	EventNodeFailed       = "NodeFailed"

	// Batch events
	EventBatchStarted   = "BatchStarted"
	EventBatchCompleted = "BatchCompleted"
	EventBatchFailed    = "BatchFailed"

	// Rollback events
	EventRollbackStarted   = "RollbackStarted"
	EventRollbackCompleted = "RollbackCompleted"
	EventRollbackFailed    = "RollbackFailed"

	// Savings events
	EventSavingsRealized = "SavingsRealized"
)

// RecordPlanCreated records a plan creation event
func (e *EventRecorder) RecordPlanCreated(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, plan *RebalancePlan) {
	message := fmt.Sprintf("Rebalancing plan %s created with strategy %s for %d nodes",
		plan.ID, plan.Strategy, plan.TotalNodes)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventPlanCreated, message)
}

// RecordPlanStarted records a plan execution start event
func (e *EventRecorder) RecordPlanStarted(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, plan *RebalancePlan) {
	message := fmt.Sprintf("Started executing rebalancing plan %s with %d batches (estimated duration: %s)",
		plan.ID, len(plan.Batches), plan.EstimatedDuration)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventPlanStarted, message)
}

// RecordPlanCompleted records a successful plan completion event
func (e *EventRecorder) RecordPlanCompleted(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, result *RebalanceResult) {
	message := fmt.Sprintf("Rebalancing plan %s completed successfully. Nodes rebalanced: %d, Duration: %s, Savings: $%.2f/month",
		result.PlanID, result.NodesRebalanced, result.Duration, result.SavingsRealized)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventPlanCompleted, message)
}

// RecordPlanFailed records a failed plan execution event
func (e *EventRecorder) RecordPlanFailed(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, planID string, err error) {
	message := fmt.Sprintf("Rebalancing plan %s failed: %v", planID, err)
	e.recorder.Event(nodeGroup, corev1.EventTypeWarning, EventPlanFailed, message)
}

// RecordSafetyCheckPassed records a passed safety check event
func (e *EventRecorder) RecordSafetyCheckPassed(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, check *SafetyCheck) {
	message := fmt.Sprintf("Safety check passed: %s - %s", check.Category, check.Message)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventSafetyCheckPassed, message)
}

// RecordSafetyCheckFailed records a failed safety check event
func (e *EventRecorder) RecordSafetyCheckFailed(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, check *SafetyCheck) {
	message := fmt.Sprintf("Safety check failed: %s - %s", check.Category, check.Message)
	e.recorder.Event(nodeGroup, corev1.EventTypeWarning, EventSafetyCheckFailed, message)
}

// RecordNodeProvisioning records a node provisioning start event
func (e *EventRecorder) RecordNodeProvisioning(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, nodeName, offering string) {
	message := fmt.Sprintf("Provisioning new node %s with offering %s", nodeName, offering)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventNodeProvisioning, message)
}

// RecordNodeProvisioned records a successful node provisioning event
func (e *EventRecorder) RecordNodeProvisioned(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, nodeName string) {
	message := fmt.Sprintf("Node %s provisioned successfully", nodeName)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventNodeProvisioned, message)
}

// RecordNodeDraining records a node draining start event
func (e *EventRecorder) RecordNodeDraining(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, nodeName string, podCount int) {
	message := fmt.Sprintf("Draining node %s (%d pods)", nodeName, podCount)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventNodeDraining, message)
}

// RecordNodeDrained records a successful node drain event
func (e *EventRecorder) RecordNodeDrained(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, nodeName string) {
	message := fmt.Sprintf("Node %s drained successfully", nodeName)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventNodeDrained, message)
}

// RecordNodeTerminating records a node termination start event
func (e *EventRecorder) RecordNodeTerminating(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, nodeName string) {
	message := fmt.Sprintf("Terminating node %s", nodeName)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventNodeTerminating, message)
}

// RecordNodeTerminated records a successful node termination event
func (e *EventRecorder) RecordNodeTerminated(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, nodeName string) {
	message := fmt.Sprintf("Node %s terminated successfully", nodeName)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventNodeTerminated, message)
}

// RecordNodeFailed records a failed node operation event
func (e *EventRecorder) RecordNodeFailed(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, failure *NodeFailure) {
	message := fmt.Sprintf("Node operation failed: %s - %s: %v",
		failure.NodeName, failure.Operation, failure.Error)
	e.recorder.Event(nodeGroup, corev1.EventTypeWarning, EventNodeFailed, message)
}

// RecordBatchStarted records a batch execution start event
func (e *EventRecorder) RecordBatchStarted(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, batch *NodeBatch) {
	message := fmt.Sprintf("Started batch %d with %d nodes (estimated duration: %s)",
		batch.BatchNumber, len(batch.Nodes), batch.EstimatedDuration)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventBatchStarted, message)
}

// RecordBatchCompleted records a successful batch completion event
func (e *EventRecorder) RecordBatchCompleted(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, batchNumber int, nodesCompleted int) {
	message := fmt.Sprintf("Batch %d completed successfully (%d nodes rebalanced)",
		batchNumber, nodesCompleted)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventBatchCompleted, message)
}

// RecordBatchFailed records a failed batch execution event
func (e *EventRecorder) RecordBatchFailed(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, batchNumber int, err error) {
	message := fmt.Sprintf("Batch %d failed: %v", batchNumber, err)
	e.recorder.Event(nodeGroup, corev1.EventTypeWarning, EventBatchFailed, message)
}

// RecordRollbackStarted records a rollback start event
func (e *EventRecorder) RecordRollbackStarted(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, planID, reason string) {
	message := fmt.Sprintf("Starting rollback of plan %s: %s", planID, reason)
	e.recorder.Event(nodeGroup, corev1.EventTypeWarning, EventRollbackStarted, message)
}

// RecordRollbackCompleted records a successful rollback event
func (e *EventRecorder) RecordRollbackCompleted(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, planID string) {
	message := fmt.Sprintf("Rollback of plan %s completed successfully", planID)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventRollbackCompleted, message)
}

// RecordRollbackFailed records a failed rollback event
func (e *EventRecorder) RecordRollbackFailed(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, planID string, err error) {
	message := fmt.Sprintf("Rollback of plan %s failed: %v", planID, err)
	e.recorder.Event(nodeGroup, corev1.EventTypeWarning, EventRollbackFailed, message)
}

// RecordSavingsRealized records a cost savings event
func (e *EventRecorder) RecordSavingsRealized(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, monthlySavings float64) {
	message := fmt.Sprintf("Cost savings realized: $%.2f/month", monthlySavings)
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, EventSavingsRealized, message)
}

// RecordWarning records a generic warning event
func (e *EventRecorder) RecordWarning(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, reason, message string) {
	e.recorder.Event(nodeGroup, corev1.EventTypeWarning, reason, message)
}

// RecordInfo records a generic informational event
func (e *EventRecorder) RecordInfo(ctx context.Context, nodeGroup *v1alpha1.NodeGroup, reason, message string) {
	e.recorder.Event(nodeGroup, corev1.EventTypeNormal, reason, message)
}
