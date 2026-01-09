package audit

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/vpsie/vpsie-k8s-autoscaler/internal/logging"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
)

// AuditEvent represents a structured audit log entry
type AuditEvent struct {
	// Timestamp is when the event occurred
	Timestamp time.Time `json:"timestamp"`

	// EventType is the type of event (from events.go)
	EventType EventType `json:"eventType"`

	// Category groups related events
	Category EventCategory `json:"category"`

	// Severity indicates the importance level
	Severity EventSeverity `json:"severity"`

	// RequestID correlates the event with a specific request
	RequestID string `json:"requestId,omitempty"`

	// Actor identifies who/what initiated the action
	Actor string `json:"actor,omitempty"`

	// Resource identifies the affected resource
	Resource *ResourceInfo `json:"resource,omitempty"`

	// Details contains event-specific information
	Details map[string]interface{} `json:"details,omitempty"`

	// Outcome indicates success or failure
	Outcome string `json:"outcome,omitempty"`

	// Message is a human-readable description
	Message string `json:"message,omitempty"`

	// Duration is how long the operation took (for completed operations)
	Duration time.Duration `json:"duration,omitempty"`
}

// ResourceInfo identifies an affected resource
type ResourceInfo struct {
	// Kind is the resource type (NodeGroup, VPSieNode, Node, etc.)
	Kind string `json:"kind"`

	// Name is the resource name
	Name string `json:"name"`

	// Namespace is the resource namespace (if applicable)
	Namespace string `json:"namespace,omitempty"`

	// UID is the resource UID (if available)
	UID string `json:"uid,omitempty"`
}

// AuditLogger handles audit event logging
type AuditLogger struct {
	logger        *zap.Logger
	enabled       bool
	mu            sync.RWMutex
	defaultActor  string
	eventSinks    []EventSink
}

// EventSink defines an interface for custom audit event destinations
type EventSink interface {
	// Write sends an audit event to the sink
	Write(event *AuditEvent) error

	// Close closes the sink
	Close() error
}

// AuditLoggerConfig configures the audit logger
type AuditLoggerConfig struct {
	// Enabled controls whether audit logging is active
	Enabled bool

	// Logger is the underlying zap logger
	Logger *zap.Logger

	// DefaultActor is the default actor if not specified
	DefaultActor string

	// EventSinks are additional destinations for audit events
	EventSinks []EventSink
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(config *AuditLoggerConfig) *AuditLogger {
	if config == nil {
		config = &AuditLoggerConfig{
			Enabled: true,
			Logger:  zap.NewNop(),
		}
	}

	logger := config.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	return &AuditLogger{
		logger:       logger.Named("audit"),
		enabled:      config.Enabled,
		defaultActor: config.DefaultActor,
		eventSinks:   config.EventSinks,
	}
}

// Log records an audit event
func (a *AuditLogger) Log(ctx context.Context, event *AuditEvent) {
	a.mu.RLock()
	enabled := a.enabled
	a.mu.RUnlock()

	if !enabled {
		return
	}

	// Fill in defaults
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Category == "" {
		event.Category = GetCategory(event.EventType)
	}
	if event.Severity == "" {
		event.Severity = GetSeverity(event.EventType)
	}
	if event.RequestID == "" {
		event.RequestID = logging.GetRequestID(ctx)
	}
	if event.Actor == "" {
		event.Actor = a.defaultActor
	}

	// Log the event
	fields := a.buildFields(event)
	switch event.Severity {
	case SeverityCritical:
		a.logger.Error(event.Message, fields...)
	case SeverityError:
		a.logger.Error(event.Message, fields...)
	case SeverityWarning:
		a.logger.Warn(event.Message, fields...)
	default:
		a.logger.Info(event.Message, fields...)
	}

	// Update metrics
	metrics.AuditEventsTotal.WithLabelValues(
		string(event.EventType),
		string(event.Category),
		string(event.Severity),
	).Inc()

	// Send to additional sinks
	for _, sink := range a.eventSinks {
		if err := sink.Write(event); err != nil {
			a.logger.Warn("Failed to write audit event to sink",
				zap.Error(err),
				zap.String("eventType", string(event.EventType)),
			)
		}
	}
}

// buildFields converts an AuditEvent to zap fields
func (a *AuditLogger) buildFields(event *AuditEvent) []zapcore.Field {
	fields := []zapcore.Field{
		zap.Time("timestamp", event.Timestamp),
		zap.String("eventType", string(event.EventType)),
		zap.String("category", string(event.Category)),
		zap.String("severity", string(event.Severity)),
	}

	if event.RequestID != "" {
		fields = append(fields, zap.String("requestId", event.RequestID))
	}
	if event.Actor != "" {
		fields = append(fields, zap.String("actor", event.Actor))
	}
	if event.Outcome != "" {
		fields = append(fields, zap.String("outcome", event.Outcome))
	}
	if event.Duration > 0 {
		fields = append(fields, zap.Duration("duration", event.Duration))
	}
	if event.Resource != nil {
		fields = append(fields, zap.Object("resource", zapResourceInfo{event.Resource}))
	}
	if len(event.Details) > 0 {
		// Serialize details to JSON for structured logging
		detailsJSON, _ := json.Marshal(event.Details)
		fields = append(fields, zap.String("details", string(detailsJSON)))
	}

	return fields
}

// zapResourceInfo wraps ResourceInfo for zap marshaling
type zapResourceInfo struct {
	*ResourceInfo
}

func (r zapResourceInfo) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("kind", r.Kind)
	enc.AddString("name", r.Name)
	if r.Namespace != "" {
		enc.AddString("namespace", r.Namespace)
	}
	if r.UID != "" {
		enc.AddString("uid", r.UID)
	}
	return nil
}

// Enable enables audit logging
func (a *AuditLogger) Enable() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.enabled = true
}

// Disable disables audit logging
func (a *AuditLogger) Disable() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.enabled = false
}

// IsEnabled returns whether audit logging is enabled
func (a *AuditLogger) IsEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.enabled
}

// Close closes all event sinks
func (a *AuditLogger) Close() error {
	for _, sink := range a.eventSinks {
		if err := sink.Close(); err != nil {
			a.logger.Warn("Failed to close audit event sink", zap.Error(err))
		}
	}
	return nil
}

// Helper methods for common audit events

// LogNodeProvisioned logs a node provisioning event
func (a *AuditLogger) LogNodeProvisioned(ctx context.Context, nodeName, nodeGroupName, namespace string, vpsieID int, duration time.Duration) {
	a.Log(ctx, &AuditEvent{
		EventType: EventNodeProvisioned,
		Message:   "Node provisioned successfully",
		Outcome:   "success",
		Duration:  duration,
		Resource: &ResourceInfo{
			Kind:      "VPSieNode",
			Name:      nodeName,
			Namespace: namespace,
		},
		Details: map[string]interface{}{
			"nodeGroup": nodeGroupName,
			"vpsieId":   vpsieID,
		},
	})
}

// LogNodeProvisionFailed logs a failed node provisioning event
func (a *AuditLogger) LogNodeProvisionFailed(ctx context.Context, nodeName, nodeGroupName, namespace, reason string) {
	a.Log(ctx, &AuditEvent{
		EventType: EventNodeProvisionFailed,
		Message:   "Node provisioning failed",
		Outcome:   "failure",
		Resource: &ResourceInfo{
			Kind:      "VPSieNode",
			Name:      nodeName,
			Namespace: namespace,
		},
		Details: map[string]interface{}{
			"nodeGroup": nodeGroupName,
			"reason":    reason,
		},
	})
}

// LogNodeTerminated logs a node termination event
func (a *AuditLogger) LogNodeTerminated(ctx context.Context, nodeName, nodeGroupName, namespace string, vpsieID int) {
	a.Log(ctx, &AuditEvent{
		EventType: EventNodeTerminated,
		Message:   "Node terminated successfully",
		Outcome:   "success",
		Resource: &ResourceInfo{
			Kind:      "VPSieNode",
			Name:      nodeName,
			Namespace: namespace,
		},
		Details: map[string]interface{}{
			"nodeGroup": nodeGroupName,
			"vpsieId":   vpsieID,
		},
	})
}

// LogScaleUp logs a scale up event
func (a *AuditLogger) LogScaleUp(ctx context.Context, nodeGroupName, namespace string, fromCount, toCount int32, outcome string) {
	eventType := EventScaleUpCompleted
	if outcome != "success" {
		eventType = EventScaleUpFailed
	}
	a.Log(ctx, &AuditEvent{
		EventType: eventType,
		Message:   "NodeGroup scaled up",
		Outcome:   outcome,
		Resource: &ResourceInfo{
			Kind:      "NodeGroup",
			Name:      nodeGroupName,
			Namespace: namespace,
		},
		Details: map[string]interface{}{
			"fromCount": fromCount,
			"toCount":   toCount,
			"nodesAdded": toCount - fromCount,
		},
	})
}

// LogScaleDown logs a scale down event
func (a *AuditLogger) LogScaleDown(ctx context.Context, nodeGroupName, namespace string, fromCount, toCount int32, outcome string) {
	eventType := EventScaleDownCompleted
	if outcome != "success" {
		eventType = EventScaleDownFailed
	}
	a.Log(ctx, &AuditEvent{
		EventType: eventType,
		Message:   "NodeGroup scaled down",
		Outcome:   outcome,
		Resource: &ResourceInfo{
			Kind:      "NodeGroup",
			Name:      nodeGroupName,
			Namespace: namespace,
		},
		Details: map[string]interface{}{
			"fromCount":    fromCount,
			"toCount":      toCount,
			"nodesRemoved": fromCount - toCount,
		},
	})
}

// LogScaleDownBlocked logs when scale down is blocked by safety checks
func (a *AuditLogger) LogScaleDownBlocked(ctx context.Context, nodeGroupName, namespace, reason string) {
	a.Log(ctx, &AuditEvent{
		EventType: EventScaleDownBlocked,
		Message:   "Scale down blocked by safety check",
		Outcome:   "blocked",
		Resource: &ResourceInfo{
			Kind:      "NodeGroup",
			Name:      nodeGroupName,
			Namespace: namespace,
		},
		Details: map[string]interface{}{
			"reason": reason,
		},
	})
}

// LogCredentialRotated logs a credential rotation event
func (a *AuditLogger) LogCredentialRotated(ctx context.Context, outcome string, duration time.Duration) {
	eventType := EventCredentialRotated
	if outcome != "success" {
		eventType = EventCredentialRotationFailed
	}
	a.Log(ctx, &AuditEvent{
		EventType: eventType,
		Message:   "VPSie credentials rotated",
		Outcome:   outcome,
		Duration:  duration,
	})
}

// LogAPICall logs an API call event
func (a *AuditLogger) LogAPICall(ctx context.Context, method, path string, statusCode int, duration time.Duration, outcome string) {
	eventType := EventAPICallSuccess
	if outcome != "success" {
		eventType = EventAPICallFailed
	}
	a.Log(ctx, &AuditEvent{
		EventType: eventType,
		Message:   "VPSie API call",
		Outcome:   outcome,
		Duration:  duration,
		Details: map[string]interface{}{
			"method":     method,
			"path":       path,
			"statusCode": statusCode,
		},
	})
}

// LogRebalance logs a rebalance operation event
func (a *AuditLogger) LogRebalance(ctx context.Context, nodeGroupName, namespace, operation, outcome string, details map[string]interface{}) {
	var eventType EventType
	switch operation {
	case "analyze":
		eventType = EventRebalanceAnalyzed
	case "plan":
		eventType = EventRebalancePlanned
	case "execute":
		if outcome == "success" {
			eventType = EventRebalanceExecuted
		} else {
			eventType = EventRebalanceFailed
		}
	case "rollback":
		eventType = EventRebalanceRolledBack
	default:
		eventType = EventRebalanceExecuted
	}

	a.Log(ctx, &AuditEvent{
		EventType: eventType,
		Message:   "Rebalance operation",
		Outcome:   outcome,
		Resource: &ResourceInfo{
			Kind:      "NodeGroup",
			Name:      nodeGroupName,
			Namespace: namespace,
		},
		Details: details,
	})
}

// Global audit logger instance
var (
	globalAuditLogger   *AuditLogger
	globalAuditLoggerMu sync.RWMutex
)

// GetGlobalAuditLogger returns the global audit logger instance.
// If no logger has been set via SetGlobalAuditLogger, a default
// no-op logger is created and returned.
func GetGlobalAuditLogger() *AuditLogger {
	globalAuditLoggerMu.RLock()
	logger := globalAuditLogger
	globalAuditLoggerMu.RUnlock()

	if logger != nil {
		return logger
	}

	// Need to initialize - acquire write lock
	globalAuditLoggerMu.Lock()
	defer globalAuditLoggerMu.Unlock()

	// Double-check after acquiring write lock
	if globalAuditLogger != nil {
		return globalAuditLogger
	}

	globalAuditLogger = NewAuditLogger(nil)
	return globalAuditLogger
}

// SetGlobalAuditLogger sets the global audit logger instance.
// This is thread-safe and can be called concurrently with GetGlobalAuditLogger.
func SetGlobalAuditLogger(logger *AuditLogger) {
	globalAuditLoggerMu.Lock()
	defer globalAuditLoggerMu.Unlock()
	globalAuditLogger = logger
}
