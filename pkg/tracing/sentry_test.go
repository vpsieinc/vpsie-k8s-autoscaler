package tracing

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.DSN != "" {
		t.Errorf("expected empty DSN, got %s", cfg.DSN)
	}
	if cfg.Environment != "development" {
		t.Errorf("expected development environment, got %s", cfg.Environment)
	}
	if cfg.Release != "unknown" {
		t.Errorf("expected unknown release, got %s", cfg.Release)
	}
	if cfg.TracesSampleRate != 0.1 {
		t.Errorf("expected 0.1 traces sample rate, got %f", cfg.TracesSampleRate)
	}
	if cfg.ErrorSampleRate != 1.0 {
		t.Errorf("expected 1.0 error sample rate, got %f", cfg.ErrorSampleRate)
	}
	if cfg.Debug {
		t.Error("expected debug to be false")
	}
}

func TestNewTracer_NilConfig(t *testing.T) {
	tracer, err := NewTracer(nil, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracer == nil {
		t.Fatal("expected tracer to be created")
	}
	if tracer.IsEnabled() {
		t.Error("expected tracer to be disabled with nil config")
	}
}

func TestNewTracer_NilLogger(t *testing.T) {
	cfg := DefaultConfig()
	tracer, err := NewTracer(cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracer == nil {
		t.Fatal("expected tracer to be created")
	}
}

func TestNewTracer_EmptyDSN(t *testing.T) {
	cfg := &Config{
		DSN:         "",
		Environment: "test",
		Release:     "v1.0.0",
	}
	tracer, err := NewTracer(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracer.IsEnabled() {
		t.Error("expected tracer to be disabled with empty DSN")
	}
}

func TestTracer_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		dsn      string
		expected bool
	}{
		{
			name:     "empty DSN",
			dsn:      "",
			expected: false,
		},
		// Note: We can't test with a real DSN as it would make actual network calls
		// The disabled case is sufficient for unit testing
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{DSN: tt.dsn}
			tracer, err := NewTracer(cfg, zap.NewNop())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tracer.IsEnabled() != tt.expected {
				t.Errorf("IsEnabled() = %v, expected %v", tracer.IsEnabled(), tt.expected)
			}
		})
	}
}

func TestTracer_CaptureError_Disabled(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())

	// Should not panic when disabled
	tracer.CaptureError(errors.New("test error"), map[string]string{"key": "value"})
	tracer.CaptureError(nil, nil) // nil error should be handled gracefully
}

func TestTracer_CaptureErrorWithContext_Disabled(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	ctx := context.Background()

	// Should not panic when disabled
	tracer.CaptureErrorWithContext(ctx, errors.New("test error"), map[string]string{"key": "value"})
	tracer.CaptureErrorWithContext(ctx, nil, nil) // nil error should be handled gracefully
}

func TestTracer_CaptureMessage_Disabled(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())

	// Should not panic when disabled
	tracer.CaptureMessage("test message", sentry.LevelInfo, map[string]string{"key": "value"})
}

func TestTracer_StartTransaction_Disabled(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	ctx := context.Background()

	newCtx, span := tracer.StartTransaction(ctx, "test-tx", "test-op")
	if span != nil {
		t.Error("expected nil span when tracer is disabled")
	}
	if newCtx != ctx {
		t.Error("expected same context when tracer is disabled")
	}
}

func TestTracer_StartSpan_Disabled(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	ctx := context.Background()

	span := tracer.StartSpan(ctx, "test-op", "test description")
	if span != nil {
		t.Error("expected nil span when tracer is disabled")
	}
}

func TestTracer_FinishSpan_NilSpan(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())

	// Should not panic with nil span
	tracer.FinishSpan(nil)
}

func TestTracer_SetSpanStatus_NilSpan(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())

	// Should not panic with nil span
	tracer.SetSpanStatus(nil, sentry.SpanStatusOK)
}

func TestTracer_SetSpanData_NilSpan(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())

	// Should not panic with nil span
	tracer.SetSpanData(nil, "key", "value")
}

func TestTracer_SetSpanTag_NilSpan(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())

	// Should not panic with nil span
	tracer.SetSpanTag(nil, "key", "value")
}

func TestTracer_RecordError_NilSpan(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())

	// Should not panic with nil span
	tracer.RecordError(nil, errors.New("test error"))
}

func TestTracer_RecordError_NilError(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())

	// Should not panic with nil error (even though span would be nil in this case)
	tracer.RecordError(nil, nil)
}

func TestTracer_Flush_Disabled(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())

	// Should not hang or panic when disabled
	tracer.Flush(100 * time.Millisecond)
}

func TestTracer_Close_Disabled(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())

	// Should not hang or panic when disabled
	tracer.Close()
}

// =============================================================================
// HTTPTransport Tests
// =============================================================================

func TestNewHTTPTransport_NilTransport(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	transport := NewHTTPTransport(tracer, nil)

	if transport == nil {
		t.Fatal("expected transport to be created")
	}
	if transport.transport != http.DefaultTransport {
		t.Error("expected default transport when nil is passed")
	}
}

func TestNewHTTPTransport_CustomTransport(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	customTransport := &http.Transport{}
	transport := NewHTTPTransport(tracer, customTransport)

	if transport.transport != customTransport {
		t.Error("expected custom transport to be used")
	}
}

func TestHTTPTransport_RoundTrip_TracerDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	transport := NewHTTPTransport(tracer, http.DefaultTransport)

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHTTPTransport_RoundTrip_NilTracer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewHTTPTransport(nil, http.DefaultTransport)

	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestHTTPTransport_RoundTrip_Error(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	transport := NewHTTPTransport(tracer, http.DefaultTransport)

	// Use invalid URL to trigger error
	req, _ := http.NewRequest(http.MethodGet, "http://invalid.invalid.invalid:12345", nil)
	req = req.WithContext(context.Background())

	_, err := transport.RoundTrip(req)
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

// =============================================================================
// ReconcileTracer Tests
// =============================================================================

func TestNewReconcileTracer(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	rt := NewReconcileTracer(tracer, "test-controller")

	if rt == nil {
		t.Fatal("expected reconcile tracer to be created")
	}
	if rt.controllerName != "test-controller" {
		t.Errorf("expected controller name 'test-controller', got %s", rt.controllerName)
	}
}

func TestNewReconcileTracer_NilTracer(t *testing.T) {
	rt := NewReconcileTracer(nil, "test-controller")

	if rt == nil {
		t.Fatal("expected reconcile tracer to be created even with nil tracer")
	}
}

func TestReconcileTracer_StartReconcile_Disabled(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	rt := NewReconcileTracer(tracer, "test-controller")
	ctx := context.Background()

	newCtx, span := rt.StartReconcile(ctx, "test-resource", "test-namespace")
	if span != nil {
		t.Error("expected nil span when tracer is disabled")
	}
	if newCtx != ctx {
		t.Error("expected same context when tracer is disabled")
	}
}

func TestReconcileTracer_StartReconcile_NilTracer(t *testing.T) {
	rt := NewReconcileTracer(nil, "test-controller")
	ctx := context.Background()

	newCtx, span := rt.StartReconcile(ctx, "test-resource", "test-namespace")
	if span != nil {
		t.Error("expected nil span when tracer is nil")
	}
	if newCtx != ctx {
		t.Error("expected same context when tracer is nil")
	}
}

func TestReconcileTracer_FinishReconcile_NilSpan(t *testing.T) {
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	rt := NewReconcileTracer(tracer, "test-controller")

	// Should not panic with nil span
	rt.FinishReconcile(nil, nil)
	rt.FinishReconcile(nil, errors.New("test error"))
}

// =============================================================================
// Global Tracer Tests
// =============================================================================

func TestGlobalTracer(t *testing.T) {
	// Save original global tracer
	original := globalTracer
	defer func() { globalTracer = original }()

	// Test with nil global tracer
	SetGlobalTracer(nil)
	if GetGlobalTracer() != nil {
		t.Error("expected nil global tracer")
	}

	// Test setting global tracer
	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	SetGlobalTracer(tracer)
	if GetGlobalTracer() != tracer {
		t.Error("expected global tracer to be set")
	}
}

func TestGlobalCaptureError_NilTracer(t *testing.T) {
	// Save original global tracer
	original := globalTracer
	defer func() { globalTracer = original }()

	SetGlobalTracer(nil)

	// Should not panic with nil global tracer
	CaptureError(errors.New("test error"), nil)
}

func TestGlobalCaptureError_WithTracer(t *testing.T) {
	// Save original global tracer
	original := globalTracer
	defer func() { globalTracer = original }()

	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	SetGlobalTracer(tracer)

	// Should not panic when called
	CaptureError(errors.New("test error"), map[string]string{"key": "value"})
}

func TestGlobalStartTransaction_NilTracer(t *testing.T) {
	// Save original global tracer
	original := globalTracer
	defer func() { globalTracer = original }()

	SetGlobalTracer(nil)
	ctx := context.Background()

	newCtx, span := StartTransaction(ctx, "test-tx", "test-op")
	if span != nil {
		t.Error("expected nil span with nil global tracer")
	}
	if newCtx != ctx {
		t.Error("expected same context with nil global tracer")
	}
}

func TestGlobalStartTransaction_WithTracer(t *testing.T) {
	// Save original global tracer
	original := globalTracer
	defer func() { globalTracer = original }()

	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	SetGlobalTracer(tracer)
	ctx := context.Background()

	newCtx, span := StartTransaction(ctx, "test-tx", "test-op")
	if span != nil {
		t.Error("expected nil span when tracer is disabled")
	}
	if newCtx != ctx {
		t.Error("expected same context when tracer is disabled")
	}
}

func TestGlobalStartSpan_NilTracer(t *testing.T) {
	// Save original global tracer
	original := globalTracer
	defer func() { globalTracer = original }()

	SetGlobalTracer(nil)
	ctx := context.Background()

	span := StartSpan(ctx, "test-op", "test description")
	if span != nil {
		t.Error("expected nil span with nil global tracer")
	}
}

func TestGlobalStartSpan_WithTracer(t *testing.T) {
	// Save original global tracer
	original := globalTracer
	defer func() { globalTracer = original }()

	tracer, _ := NewTracer(&Config{DSN: ""}, zap.NewNop())
	SetGlobalTracer(tracer)
	ctx := context.Background()

	span := StartSpan(ctx, "test-op", "test description")
	if span != nil {
		t.Error("expected nil span when tracer is disabled")
	}
}

// =============================================================================
// Config Validation Tests
// =============================================================================

func TestConfig_SampleRates(t *testing.T) {
	tests := []struct {
		name             string
		tracesSampleRate float64
		errorSampleRate  float64
	}{
		{
			name:             "default rates",
			tracesSampleRate: 0.1,
			errorSampleRate:  1.0,
		},
		{
			name:             "zero rates",
			tracesSampleRate: 0.0,
			errorSampleRate:  0.0,
		},
		{
			name:             "max rates",
			tracesSampleRate: 1.0,
			errorSampleRate:  1.0,
		},
		{
			name:             "custom rates",
			tracesSampleRate: 0.5,
			errorSampleRate:  0.75,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				DSN:              "", // Disabled
				TracesSampleRate: tt.tracesSampleRate,
				ErrorSampleRate:  tt.errorSampleRate,
			}
			tracer, err := NewTracer(cfg, zap.NewNop())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tracer == nil {
				t.Fatal("expected tracer to be created")
			}
		})
	}
}

func TestConfig_AllFields(t *testing.T) {
	cfg := &Config{
		DSN:              "", // Empty to keep disabled for testing
		Environment:      "production",
		Release:          "v1.2.3",
		TracesSampleRate: 0.5,
		ErrorSampleRate:  0.8,
		Debug:            true,
		ServerName:       "test-server",
	}

	tracer, err := NewTracer(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tracer == nil {
		t.Fatal("expected tracer to be created")
	}
	if tracer.config.Environment != "production" {
		t.Errorf("expected production environment, got %s", tracer.config.Environment)
	}
	if tracer.config.Release != "v1.2.3" {
		t.Errorf("expected v1.2.3 release, got %s", tracer.config.Release)
	}
}
