package audit

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// mockEventSink is a test implementation of EventSink
type mockEventSink struct {
	mu       sync.Mutex
	events   []*AuditEvent
	writeErr error
	closed   bool
}

func newMockEventSink() *mockEventSink {
	return &mockEventSink{
		events: make([]*AuditEvent, 0),
	}
}

func (m *mockEventSink) Write(event *AuditEvent) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.events = append(m.events, event)
	return nil
}

func (m *mockEventSink) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockEventSink) getEvents() []*AuditEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*AuditEvent, len(m.events))
	copy(result, m.events)
	return result
}

func (m *mockEventSink) setWriteError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeErr = err
}

func (m *mockEventSink) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func TestNewAuditLogger(t *testing.T) {
	t.Run("with nil config", func(t *testing.T) {
		logger := NewAuditLogger(nil)
		if logger == nil {
			t.Fatal("expected logger to be created")
		}
		if !logger.enabled {
			t.Error("expected logger to be enabled by default")
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		zapLogger := zap.NewNop()
		config := &AuditLoggerConfig{
			Enabled:      true,
			Logger:       zapLogger,
			DefaultActor: "test-actor",
		}
		logger := NewAuditLogger(config)
		if logger == nil {
			t.Fatal("expected logger to be created")
		}
		if logger.defaultActor != "test-actor" {
			t.Errorf("expected default actor 'test-actor', got '%s'", logger.defaultActor)
		}
	})

	t.Run("with disabled config", func(t *testing.T) {
		config := &AuditLoggerConfig{
			Enabled: false,
		}
		logger := NewAuditLogger(config)
		if logger.enabled {
			t.Error("expected logger to be disabled")
		}
	})
}

func TestAuditLogger_Log(t *testing.T) {
	core, recorded := observer.New(zapcore.InfoLevel)
	zapLogger := zap.New(core)

	sink := newMockEventSink()
	config := &AuditLoggerConfig{
		Enabled:      true,
		Logger:       zapLogger,
		DefaultActor: "test-controller",
		EventSinks:   []EventSink{sink},
	}
	logger := NewAuditLogger(config)

	ctx := context.Background()
	event := &AuditEvent{
		EventType: EventNodeProvisioned,
		Message:   "Test node provisioned",
		Outcome:   "success",
		Resource: &ResourceInfo{
			Kind:      "VPSieNode",
			Name:      "test-node",
			Namespace: "kube-system",
		},
	}

	logger.Log(ctx, event)

	// Check that event was logged
	logs := recorded.All()
	if len(logs) != 1 {
		t.Errorf("expected 1 log entry, got %d", len(logs))
	}

	// Check that event was sent to sink
	events := sink.getEvents()
	if len(events) != 1 {
		t.Errorf("expected 1 event in sink, got %d", len(events))
	}

	// Check that defaults were filled in
	if events[0].Timestamp.IsZero() {
		t.Error("expected timestamp to be set")
	}
	if events[0].Actor != "test-controller" {
		t.Errorf("expected actor 'test-controller', got '%s'", events[0].Actor)
	}
	if events[0].Category != CategoryNode {
		t.Errorf("expected category 'node', got '%s'", events[0].Category)
	}
	if events[0].Severity != SeverityInfo {
		t.Errorf("expected severity 'info', got '%s'", events[0].Severity)
	}
}

func TestAuditLogger_Log_Disabled(t *testing.T) {
	sink := newMockEventSink()
	config := &AuditLoggerConfig{
		Enabled:    false,
		EventSinks: []EventSink{sink},
	}
	logger := NewAuditLogger(config)

	ctx := context.Background()
	event := &AuditEvent{
		EventType: EventNodeProvisioned,
		Message:   "Test node provisioned",
	}

	logger.Log(ctx, event)

	// Event should not be sent to sink when disabled
	events := sink.getEvents()
	if len(events) != 0 {
		t.Errorf("expected 0 events when disabled, got %d", len(events))
	}
}

func TestAuditLogger_Log_SinkError(t *testing.T) {
	core, recorded := observer.New(zapcore.WarnLevel)
	zapLogger := zap.New(core)

	sink := newMockEventSink()
	sink.setWriteError(errors.New("sink error"))

	config := &AuditLoggerConfig{
		Enabled:    true,
		Logger:     zapLogger,
		EventSinks: []EventSink{sink},
	}
	logger := NewAuditLogger(config)

	ctx := context.Background()
	event := &AuditEvent{
		EventType: EventNodeProvisioned,
		Message:   "Test node provisioned",
	}

	// Should not panic even with sink error
	logger.Log(ctx, event)

	// Should have logged a warning about the sink error
	logs := recorded.FilterMessage("Failed to write audit event to sink").All()
	if len(logs) != 1 {
		t.Errorf("expected 1 warning log for sink error, got %d", len(logs))
	}
}

func TestAuditLogger_Log_Severities(t *testing.T) {
	tests := []struct {
		name      string
		eventType EventType
		severity  EventSeverity
	}{
		{"critical event", EventNodeProvisionFailed, SeverityCritical},
		{"error event", EventNodeDrainFailed, SeverityError},
		{"warning event", EventScaleDownBlocked, SeverityWarning},
		{"info event", EventNodeProvisioned, SeverityInfo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, _ := observer.New(zapcore.DebugLevel)
			zapLogger := zap.New(core)

			config := &AuditLoggerConfig{
				Enabled: true,
				Logger:  zapLogger,
			}
			logger := NewAuditLogger(config)

			ctx := context.Background()
			event := &AuditEvent{
				EventType: tt.eventType,
				Message:   "Test event",
			}

			logger.Log(ctx, event)

			if event.Severity != tt.severity {
				t.Errorf("expected severity %s, got %s", tt.severity, event.Severity)
			}
		})
	}
}

func TestAuditLogger_EnableDisable(t *testing.T) {
	config := &AuditLoggerConfig{
		Enabled: true,
	}
	logger := NewAuditLogger(config)

	if !logger.IsEnabled() {
		t.Error("expected logger to be enabled initially")
	}

	logger.Disable()
	if logger.IsEnabled() {
		t.Error("expected logger to be disabled after Disable()")
	}

	logger.Enable()
	if !logger.IsEnabled() {
		t.Error("expected logger to be enabled after Enable()")
	}
}

func TestAuditLogger_Close(t *testing.T) {
	sink1 := newMockEventSink()
	sink2 := newMockEventSink()

	config := &AuditLoggerConfig{
		Enabled:    true,
		EventSinks: []EventSink{sink1, sink2},
	}
	logger := NewAuditLogger(config)

	err := logger.Close()
	if err != nil {
		t.Errorf("expected no error on close, got %v", err)
	}

	if !sink1.isClosed() {
		t.Error("expected sink1 to be closed")
	}
	if !sink2.isClosed() {
		t.Error("expected sink2 to be closed")
	}
}

func TestAuditLogger_LogNodeProvisioned(t *testing.T) {
	sink := newMockEventSink()
	config := &AuditLoggerConfig{
		Enabled:    true,
		Logger:     zap.NewNop(),
		EventSinks: []EventSink{sink},
	}
	logger := NewAuditLogger(config)

	ctx := context.Background()
	logger.LogNodeProvisioned(ctx, "test-node", "test-nodegroup", "kube-system", 12345, 30*time.Second)

	events := sink.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.EventType != EventNodeProvisioned {
		t.Errorf("expected event type %s, got %s", EventNodeProvisioned, event.EventType)
	}
	if event.Outcome != "success" {
		t.Errorf("expected outcome 'success', got '%s'", event.Outcome)
	}
	if event.Duration != 30*time.Second {
		t.Errorf("expected duration 30s, got %v", event.Duration)
	}
	if event.Resource.Kind != "VPSieNode" {
		t.Errorf("expected resource kind 'VPSieNode', got '%s'", event.Resource.Kind)
	}
	if event.Details["vpsieId"] != 12345 {
		t.Errorf("expected vpsieId 12345, got %v", event.Details["vpsieId"])
	}
}

func TestAuditLogger_LogNodeProvisionFailed(t *testing.T) {
	sink := newMockEventSink()
	config := &AuditLoggerConfig{
		Enabled:    true,
		Logger:     zap.NewNop(),
		EventSinks: []EventSink{sink},
	}
	logger := NewAuditLogger(config)

	ctx := context.Background()
	logger.LogNodeProvisionFailed(ctx, "test-node", "test-nodegroup", "kube-system", "API timeout")

	events := sink.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.EventType != EventNodeProvisionFailed {
		t.Errorf("expected event type %s, got %s", EventNodeProvisionFailed, event.EventType)
	}
	if event.Outcome != "failure" {
		t.Errorf("expected outcome 'failure', got '%s'", event.Outcome)
	}
	if event.Details["reason"] != "API timeout" {
		t.Errorf("expected reason 'API timeout', got '%v'", event.Details["reason"])
	}
}

func TestAuditLogger_LogScaleUp(t *testing.T) {
	tests := []struct {
		name         string
		outcome      string
		expectedType EventType
	}{
		{"success", "success", EventScaleUpCompleted},
		{"failure", "failure", EventScaleUpFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := newMockEventSink()
			config := &AuditLoggerConfig{
				Enabled:    true,
				Logger:     zap.NewNop(),
				EventSinks: []EventSink{sink},
			}
			logger := NewAuditLogger(config)

			ctx := context.Background()
			logger.LogScaleUp(ctx, "test-nodegroup", "kube-system", 2, 5, tt.outcome)

			events := sink.getEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}

			event := events[0]
			if event.EventType != tt.expectedType {
				t.Errorf("expected event type %s, got %s", tt.expectedType, event.EventType)
			}
			if event.Details["fromCount"] != int32(2) {
				t.Errorf("expected fromCount 2, got %v", event.Details["fromCount"])
			}
			if event.Details["toCount"] != int32(5) {
				t.Errorf("expected toCount 5, got %v", event.Details["toCount"])
			}
			if event.Details["nodesAdded"] != int32(3) {
				t.Errorf("expected nodesAdded 3, got %v", event.Details["nodesAdded"])
			}
		})
	}
}

func TestAuditLogger_LogScaleDown(t *testing.T) {
	tests := []struct {
		name         string
		outcome      string
		expectedType EventType
	}{
		{"success", "success", EventScaleDownCompleted},
		{"failure", "failure", EventScaleDownFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := newMockEventSink()
			config := &AuditLoggerConfig{
				Enabled:    true,
				Logger:     zap.NewNop(),
				EventSinks: []EventSink{sink},
			}
			logger := NewAuditLogger(config)

			ctx := context.Background()
			logger.LogScaleDown(ctx, "test-nodegroup", "kube-system", 5, 2, tt.outcome)

			events := sink.getEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}

			event := events[0]
			if event.EventType != tt.expectedType {
				t.Errorf("expected event type %s, got %s", tt.expectedType, event.EventType)
			}
			if event.Details["nodesRemoved"] != int32(3) {
				t.Errorf("expected nodesRemoved 3, got %v", event.Details["nodesRemoved"])
			}
		})
	}
}

func TestAuditLogger_LogScaleDownBlocked(t *testing.T) {
	sink := newMockEventSink()
	config := &AuditLoggerConfig{
		Enabled:    true,
		Logger:     zap.NewNop(),
		EventSinks: []EventSink{sink},
	}
	logger := NewAuditLogger(config)

	ctx := context.Background()
	logger.LogScaleDownBlocked(ctx, "test-nodegroup", "kube-system", "pods cannot be rescheduled")

	events := sink.getEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	event := events[0]
	if event.EventType != EventScaleDownBlocked {
		t.Errorf("expected event type %s, got %s", EventScaleDownBlocked, event.EventType)
	}
	if event.Outcome != "blocked" {
		t.Errorf("expected outcome 'blocked', got '%s'", event.Outcome)
	}
	if event.Details["reason"] != "pods cannot be rescheduled" {
		t.Errorf("expected reason 'pods cannot be rescheduled', got '%v'", event.Details["reason"])
	}
}

func TestAuditLogger_LogAPICall(t *testing.T) {
	tests := []struct {
		name         string
		outcome      string
		expectedType EventType
	}{
		{"success", "success", EventAPICallSuccess},
		{"failure", "failure", EventAPICallFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := newMockEventSink()
			config := &AuditLoggerConfig{
				Enabled:    true,
				Logger:     zap.NewNop(),
				EventSinks: []EventSink{sink},
			}
			logger := NewAuditLogger(config)

			ctx := context.Background()
			logger.LogAPICall(ctx, "POST", "/api/v2/vps", 200, 150*time.Millisecond, tt.outcome)

			events := sink.getEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}

			event := events[0]
			if event.EventType != tt.expectedType {
				t.Errorf("expected event type %s, got %s", tt.expectedType, event.EventType)
			}
			if event.Details["method"] != "POST" {
				t.Errorf("expected method 'POST', got '%v'", event.Details["method"])
			}
			if event.Details["path"] != "/api/v2/vps" {
				t.Errorf("expected path '/api/v2/vps', got '%v'", event.Details["path"])
			}
			if event.Details["statusCode"] != 200 {
				t.Errorf("expected statusCode 200, got %v", event.Details["statusCode"])
			}
		})
	}
}

func TestAuditLogger_LogRebalance(t *testing.T) {
	tests := []struct {
		name         string
		operation    string
		outcome      string
		expectedType EventType
	}{
		{"analyze", "analyze", "success", EventRebalanceAnalyzed},
		{"plan", "plan", "success", EventRebalancePlanned},
		{"execute success", "execute", "success", EventRebalanceExecuted},
		{"execute failure", "execute", "failure", EventRebalanceFailed},
		{"rollback", "rollback", "success", EventRebalanceRolledBack},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink := newMockEventSink()
			config := &AuditLoggerConfig{
				Enabled:    true,
				Logger:     zap.NewNop(),
				EventSinks: []EventSink{sink},
			}
			logger := NewAuditLogger(config)

			ctx := context.Background()
			details := map[string]interface{}{"nodes": 3}
			logger.LogRebalance(ctx, "test-nodegroup", "kube-system", tt.operation, tt.outcome, details)

			events := sink.getEvents()
			if len(events) != 1 {
				t.Fatalf("expected 1 event, got %d", len(events))
			}

			event := events[0]
			if event.EventType != tt.expectedType {
				t.Errorf("expected event type %s, got %s", tt.expectedType, event.EventType)
			}
			if event.Details["nodes"] != 3 {
				t.Errorf("expected details['nodes'] = 3, got %v", event.Details["nodes"])
			}
		})
	}
}

func TestAuditLogger_ConcurrentWrites(t *testing.T) {
	sink := newMockEventSink()
	config := &AuditLoggerConfig{
		Enabled:    true,
		Logger:     zap.NewNop(),
		EventSinks: []EventSink{sink},
	}
	logger := NewAuditLogger(config)

	var wg sync.WaitGroup
	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx := context.Background()
			event := &AuditEvent{
				EventType: EventNodeProvisioned,
				Message:   "Test event",
				Details: map[string]interface{}{
					"index": i,
				},
			}
			logger.Log(ctx, event)
		}(i)
	}

	wg.Wait()

	events := sink.getEvents()
	if len(events) != numGoroutines {
		t.Errorf("expected %d events, got %d", numGoroutines, len(events))
	}
}

func TestGetGlobalAuditLogger(t *testing.T) {
	// Reset global logger
	globalAuditLoggerMu.Lock()
	globalAuditLogger = nil
	globalAuditLoggerMu.Unlock()

	// First call should create a default logger
	logger1 := GetGlobalAuditLogger()
	if logger1 == nil {
		t.Fatal("expected global logger to be created")
	}

	// Second call should return the same logger
	logger2 := GetGlobalAuditLogger()
	if logger1 != logger2 {
		t.Error("expected same logger instance")
	}
}

func TestSetGlobalAuditLogger(t *testing.T) {
	customLogger := NewAuditLogger(&AuditLoggerConfig{
		Enabled:      true,
		DefaultActor: "custom-actor",
	})

	SetGlobalAuditLogger(customLogger)

	retrieved := GetGlobalAuditLogger()
	if retrieved != customLogger {
		t.Error("expected retrieved logger to be the custom logger")
	}

	// Clean up
	SetGlobalAuditLogger(nil)
}

func TestGetCategory(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  EventCategory
	}{
		{EventNodeProvisioned, CategoryNode},
		{EventScaleUpCompleted, CategoryScaling},
		{EventRebalanceExecuted, CategoryRebalance},
		{EventNodeGroupCreated, CategoryConfig},
		{EventCredentialRotated, CategorySecurity},
		{EventAPICallSuccess, CategoryAPI},
		{EventControllerStarted, CategorySystem},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			result := GetCategory(tt.eventType)
			if result != tt.expected {
				t.Errorf("expected category %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetSeverity(t *testing.T) {
	tests := []struct {
		eventType EventType
		expected  EventSeverity
	}{
		{EventNodeProvisionFailed, SeverityCritical},
		{EventNodeDrainFailed, SeverityError},
		{EventScaleDownBlocked, SeverityWarning},
		{EventNodeProvisioned, SeverityInfo},
	}

	for _, tt := range tests {
		t.Run(string(tt.eventType), func(t *testing.T) {
			result := GetSeverity(tt.eventType)
			if result != tt.expected {
				t.Errorf("expected severity %s, got %s", tt.expected, result)
			}
		})
	}
}
