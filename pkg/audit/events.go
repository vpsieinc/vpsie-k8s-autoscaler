package audit

// EventType represents the type of audit event
type EventType string

const (
	// Node Lifecycle Events
	EventNodeProvisioned     EventType = "node.provisioned"
	EventNodeProvisionFailed EventType = "node.provision_failed"
	EventNodeTerminated      EventType = "node.terminated"
	EventNodeTerminateFailed EventType = "node.terminate_failed"
	EventNodeDrained         EventType = "node.drained"
	EventNodeDrainFailed     EventType = "node.drain_failed"
	EventNodeCordoned        EventType = "node.cordoned"
	EventNodeUncordoned      EventType = "node.uncordoned"

	// Scaling Events
	EventScaleUpInitiated   EventType = "scaling.up_initiated"
	EventScaleUpCompleted   EventType = "scaling.up_completed"
	EventScaleUpFailed      EventType = "scaling.up_failed"
	EventScaleDownInitiated EventType = "scaling.down_initiated"
	EventScaleDownCompleted EventType = "scaling.down_completed"
	EventScaleDownFailed    EventType = "scaling.down_failed"
	EventScaleDownBlocked   EventType = "scaling.down_blocked"

	// Rebalancing Events
	EventRebalanceAnalyzed   EventType = "rebalance.analyzed"
	EventRebalancePlanned    EventType = "rebalance.planned"
	EventRebalanceExecuted   EventType = "rebalance.executed"
	EventRebalanceFailed     EventType = "rebalance.failed"
	EventRebalanceRolledBack EventType = "rebalance.rolled_back"

	// Configuration Events
	EventNodeGroupCreated EventType = "config.nodegroup_created"
	EventNodeGroupUpdated EventType = "config.nodegroup_updated"
	EventNodeGroupDeleted EventType = "config.nodegroup_deleted"

	// Security Events
	EventCredentialRotated        EventType = "security.credential_rotated"
	EventCredentialRotationFailed EventType = "security.credential_rotation_failed"
	EventAuthenticationFailed     EventType = "security.authentication_failed"
	EventAPIRateLimited           EventType = "security.api_rate_limited"
	EventCircuitBreakerOpened     EventType = "security.circuit_breaker_opened"
	EventCircuitBreakerClosed     EventType = "security.circuit_breaker_closed"

	// API Events
	EventAPICallMade    EventType = "api.call_made"
	EventAPICallFailed  EventType = "api.call_failed"
	EventAPICallSuccess EventType = "api.call_success"

	// System Events
	EventControllerStarted       EventType = "system.controller_started"
	EventControllerStopped       EventType = "system.controller_stopped"
	EventLeaderElected           EventType = "system.leader_elected"
	EventLeaderLost              EventType = "system.leader_lost"
	EventReconciliationStarted   EventType = "system.reconciliation_started"
	EventReconciliationCompleted EventType = "system.reconciliation_completed"
	EventReconciliationFailed    EventType = "system.reconciliation_failed"
)

// EventSeverity represents the severity level of an audit event
type EventSeverity string

const (
	SeverityInfo     EventSeverity = "info"
	SeverityWarning  EventSeverity = "warning"
	SeverityError    EventSeverity = "error"
	SeverityCritical EventSeverity = "critical"
)

// EventCategory groups related event types
type EventCategory string

const (
	CategoryNode      EventCategory = "node"
	CategoryScaling   EventCategory = "scaling"
	CategoryRebalance EventCategory = "rebalance"
	CategoryConfig    EventCategory = "config"
	CategorySecurity  EventCategory = "security"
	CategoryAPI       EventCategory = "api"
	CategorySystem    EventCategory = "system"
)

// GetCategory returns the category for an event type
func GetCategory(eventType EventType) EventCategory {
	switch eventType {
	case EventNodeProvisioned, EventNodeProvisionFailed, EventNodeTerminated,
		EventNodeTerminateFailed, EventNodeDrained, EventNodeDrainFailed,
		EventNodeCordoned, EventNodeUncordoned:
		return CategoryNode
	case EventScaleUpInitiated, EventScaleUpCompleted, EventScaleUpFailed,
		EventScaleDownInitiated, EventScaleDownCompleted, EventScaleDownFailed,
		EventScaleDownBlocked:
		return CategoryScaling
	case EventRebalanceAnalyzed, EventRebalancePlanned, EventRebalanceExecuted,
		EventRebalanceFailed, EventRebalanceRolledBack:
		return CategoryRebalance
	case EventNodeGroupCreated, EventNodeGroupUpdated, EventNodeGroupDeleted:
		return CategoryConfig
	case EventCredentialRotated, EventCredentialRotationFailed,
		EventAuthenticationFailed, EventAPIRateLimited,
		EventCircuitBreakerOpened, EventCircuitBreakerClosed:
		return CategorySecurity
	case EventAPICallMade, EventAPICallFailed, EventAPICallSuccess:
		return CategoryAPI
	default:
		return CategorySystem
	}
}

// GetSeverity returns the default severity for an event type
func GetSeverity(eventType EventType) EventSeverity {
	switch eventType {
	// Critical events
	case EventNodeProvisionFailed, EventNodeTerminateFailed,
		EventScaleUpFailed, EventScaleDownFailed,
		EventRebalanceFailed, EventCredentialRotationFailed,
		EventAuthenticationFailed:
		return SeverityCritical

	// Error events
	case EventNodeDrainFailed, EventRebalanceRolledBack,
		EventReconciliationFailed, EventAPICallFailed:
		return SeverityError

	// Warning events
	case EventScaleDownBlocked, EventAPIRateLimited,
		EventCircuitBreakerOpened:
		return SeverityWarning

	// Info events (default)
	default:
		return SeverityInfo
	}
}
