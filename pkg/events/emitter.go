package events

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/record"

	autoscalermetrics "github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
)

const (
	// Event types
	EventTypeNormal  = corev1.EventTypeNormal
	EventTypeWarning = corev1.EventTypeWarning

	// Event reasons for NodeGroup
	ReasonScaleUpTriggered   = "ScaleUpTriggered"
	ReasonScaleDownTriggered = "ScaleDownTriggered"
	ReasonScaleUpCompleted   = "ScaleUpCompleted"
	ReasonScaleDownCompleted = "ScaleDownCompleted"
	ReasonScaleUpFailed      = "ScaleUpFailed"
	ReasonScaleDownFailed    = "ScaleDownFailed"
	ReasonNodeGroupUpdated   = "NodeGroupUpdated"
	ReasonNodeGroupError     = "NodeGroupError"

	// Event reasons for VPSieNode
	ReasonNodeProvisioning       = "NodeProvisioning"
	ReasonNodeProvisioningFailed = "NodeProvisioningFailed"
	ReasonNodeProvisioned        = "NodeProvisioned"
	ReasonNodeJoining            = "NodeJoining"
	ReasonNodeJoinFailed         = "NodeJoinFailed"
	ReasonNodeReady              = "NodeReady"
	ReasonNodeTerminating        = "NodeTerminating"
	ReasonNodeTerminationFailed  = "NodeTerminationFailed"
	ReasonNodeTerminated         = "NodeTerminated"
	ReasonNodeDraining           = "NodeDraining"
	ReasonNodeDrainFailed        = "NodeDrainFailed"
	ReasonNodeDrained            = "NodeDrained"
	ReasonVPSCreated             = "VPSCreated"
	ReasonVPSCreateFailed        = "VPSCreateFailed"
	ReasonVPSDeleted             = "VPSDeleted"
	ReasonVPSDeleteFailed        = "VPSDeleteFailed"

	// Event reasons for unschedulable pods
	ReasonUnschedulablePods = "UnschedulablePods"
)

// EventEmitter handles Kubernetes event emission
type EventEmitter struct {
	recorder record.EventRecorder
}

// NewEventEmitter creates a new event emitter
func NewEventEmitter(clientset kubernetes.Interface, scheme *runtime.Scheme) *EventEmitter {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartRecordingToSink(
		&corev1.EventSinkImpl{
			Interface: clientset.CoreV1().Events(""),
		},
	)

	recorder := eventBroadcaster.NewRecorder(
		scheme,
		corev1.EventSource{Component: "vpsie-autoscaler"},
	)

	return &EventEmitter{
		recorder: recorder,
	}
}

// emitEvent is a helper to emit an event and record metrics
func (e *EventEmitter) emitEvent(object runtime.Object, eventType, reason, message string) {
	e.recorder.Event(object, eventType, reason, message)

	// Record metric
	objectKind := "Unknown"
	if obj, ok := object.(metav1.Object); ok {
		objectKind = obj.GetObjectKind().GroupVersionKind().Kind
		if objectKind == "" {
			// Fallback to type name
			objectKind = fmt.Sprintf("%T", object)
		}
	}
	autoscalermetrics.RecordEventEmitted(eventType, reason, objectKind)
}

// EmitScaleUpTriggered emits an event when scale-up is triggered
func (e *EventEmitter) EmitScaleUpTriggered(object runtime.Object, currentNodes, desiredNodes int32, reason string) {
	message := fmt.Sprintf("Scale-up triggered: %d -> %d nodes. Reason: %s", currentNodes, desiredNodes, reason)
	e.emitEvent(object, EventTypeNormal, ReasonScaleUpTriggered, message)
}

// EmitScaleUpCompleted emits an event when scale-up is completed
func (e *EventEmitter) EmitScaleUpCompleted(object runtime.Object, nodesAdded int32) {
	message := fmt.Sprintf("Scale-up completed: added %d nodes", nodesAdded)
	e.emitEvent(object, EventTypeNormal, ReasonScaleUpCompleted, message)
}

// EmitScaleUpFailed emits an event when scale-up fails
func (e *EventEmitter) EmitScaleUpFailed(object runtime.Object, err error) {
	message := fmt.Sprintf("Scale-up failed: %v", err)
	e.emitEvent(object, EventTypeWarning, ReasonScaleUpFailed, message)
}

// EmitScaleDownTriggered emits an event when scale-down is triggered
func (e *EventEmitter) EmitScaleDownTriggered(object runtime.Object, currentNodes, desiredNodes int32, reason string) {
	message := fmt.Sprintf("Scale-down triggered: %d -> %d nodes. Reason: %s", currentNodes, desiredNodes, reason)
	e.emitEvent(object, EventTypeNormal, ReasonScaleDownTriggered, message)
}

// EmitScaleDownCompleted emits an event when scale-down is completed
func (e *EventEmitter) EmitScaleDownCompleted(object runtime.Object, nodesRemoved int32) {
	message := fmt.Sprintf("Scale-down completed: removed %d nodes", nodesRemoved)
	e.emitEvent(object, EventTypeNormal, ReasonScaleDownCompleted, message)
}

// EmitScaleDownFailed emits an event when scale-down fails
func (e *EventEmitter) EmitScaleDownFailed(object runtime.Object, err error) {
	message := fmt.Sprintf("Scale-down failed: %v", err)
	e.emitEvent(object, EventTypeWarning, ReasonScaleDownFailed, message)
}

// EmitNodeProvisioning emits an event when node provisioning starts
func (e *EventEmitter) EmitNodeProvisioning(object runtime.Object, instanceType string) {
	message := fmt.Sprintf("Provisioning node with instance type: %s", instanceType)
	e.emitEvent(object, EventTypeNormal, ReasonNodeProvisioning, message)
}

// EmitNodeProvisioningFailed emits an event when node provisioning fails
func (e *EventEmitter) EmitNodeProvisioningFailed(object runtime.Object, err error, reason string) {
	message := fmt.Sprintf("Node provisioning failed: %s. Error: %v", reason, err)
	e.emitEvent(object, EventTypeWarning, ReasonNodeProvisioningFailed, message)
}

// EmitNodeProvisioned emits an event when node provisioning completes
func (e *EventEmitter) EmitNodeProvisioned(object runtime.Object, vpsID string) {
	message := fmt.Sprintf("Node provisioned successfully. VPS ID: %s", vpsID)
	e.emitEvent(object, EventTypeNormal, ReasonNodeProvisioned, message)
}

// EmitNodeJoining emits an event when node starts joining the cluster
func (e *EventEmitter) EmitNodeJoining(object runtime.Object, nodeName string) {
	message := fmt.Sprintf("Node joining cluster: %s", nodeName)
	e.emitEvent(object, EventTypeNormal, ReasonNodeJoining, message)
}

// EmitNodeJoinFailed emits an event when node join fails
func (e *EventEmitter) EmitNodeJoinFailed(object runtime.Object, err error) {
	message := fmt.Sprintf("Node join failed: %v", err)
	e.emitEvent(object, EventTypeWarning, ReasonNodeJoinFailed, message)
}

// EmitNodeReady emits an event when node becomes ready
func (e *EventEmitter) EmitNodeReady(object runtime.Object, nodeName string) {
	message := fmt.Sprintf("Node is ready: %s", nodeName)
	e.emitEvent(object, EventTypeNormal, ReasonNodeReady, message)
}

// EmitNodeTerminating emits an event when node termination starts
func (e *EventEmitter) EmitNodeTerminating(object runtime.Object, reason string) {
	message := fmt.Sprintf("Terminating node. Reason: %s", reason)
	e.emitEvent(object, EventTypeNormal, ReasonNodeTerminating, message)
}

// EmitNodeTerminationFailed emits an event when node termination fails
func (e *EventEmitter) EmitNodeTerminationFailed(object runtime.Object, err error, reason string) {
	message := fmt.Sprintf("Node termination failed: %s. Error: %v", reason, err)
	e.emitEvent(object, EventTypeWarning, ReasonNodeTerminationFailed, message)
}

// EmitNodeTerminated emits an event when node termination completes
func (e *EventEmitter) EmitNodeTerminated(object runtime.Object) {
	message := "Node terminated successfully"
	e.emitEvent(object, EventTypeNormal, ReasonNodeTerminated, message)
}

// EmitNodeDraining emits an event when node draining starts
func (e *EventEmitter) EmitNodeDraining(object runtime.Object, nodeName string, podCount int) {
	message := fmt.Sprintf("Draining node %s (%d pods)", nodeName, podCount)
	e.emitEvent(object, EventTypeNormal, ReasonNodeDraining, message)
}

// EmitNodeDrainFailed emits an event when node drain fails
func (e *EventEmitter) EmitNodeDrainFailed(object runtime.Object, nodeName string, err error) {
	message := fmt.Sprintf("Failed to drain node %s: %v", nodeName, err)
	e.emitEvent(object, EventTypeWarning, ReasonNodeDrainFailed, message)
}

// EmitNodeDrained emits an event when node drain completes
func (e *EventEmitter) EmitNodeDrained(object runtime.Object, nodeName string) {
	message := fmt.Sprintf("Node drained successfully: %s", nodeName)
	e.emitEvent(object, EventTypeNormal, ReasonNodeDrained, message)
}

// EmitVPSCreated emits an event when VPS is created
func (e *EventEmitter) EmitVPSCreated(object runtime.Object, vpsID string) {
	message := fmt.Sprintf("VPS created: %s", vpsID)
	e.emitEvent(object, EventTypeNormal, ReasonVPSCreated, message)
}

// EmitVPSCreateFailed emits an event when VPS creation fails
func (e *EventEmitter) EmitVPSCreateFailed(object runtime.Object, err error) {
	message := fmt.Sprintf("VPS creation failed: %v", err)
	e.emitEvent(object, EventTypeWarning, ReasonVPSCreateFailed, message)
}

// EmitVPSDeleted emits an event when VPS is deleted
func (e *EventEmitter) EmitVPSDeleted(object runtime.Object, vpsID string) {
	message := fmt.Sprintf("VPS deleted: %s", vpsID)
	e.emitEvent(object, EventTypeNormal, ReasonVPSDeleted, message)
}

// EmitVPSDeleteFailed emits an event when VPS deletion fails
func (e *EventEmitter) EmitVPSDeleteFailed(object runtime.Object, vpsID string, err error) {
	message := fmt.Sprintf("VPS deletion failed: %s. Error: %v", vpsID, err)
	e.emitEvent(object, EventTypeWarning, ReasonVPSDeleteFailed, message)
}

// EmitUnschedulablePods emits an event for unschedulable pods
func (e *EventEmitter) EmitUnschedulablePods(ctx context.Context, object runtime.Object, podCount int, constraint string) {
	message := fmt.Sprintf("Detected %d unschedulable pods (constraint: %s)", podCount, constraint)
	e.emitEvent(object, EventTypeWarning, ReasonUnschedulablePods, message)
}

// EmitNodeGroupError emits a generic NodeGroup error event
func (e *EventEmitter) EmitNodeGroupError(object runtime.Object, err error) {
	message := fmt.Sprintf("NodeGroup error: %v", err)
	e.emitEvent(object, EventTypeWarning, ReasonNodeGroupError, message)
}
