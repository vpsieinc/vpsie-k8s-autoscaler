// Package tracing provides Sentry-based error tracking and performance monitoring
package tracing

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"go.uber.org/zap"
)

// Config holds Sentry configuration options
type Config struct {
	// DSN is the Sentry Data Source Name (from SENTRY_DSN env var)
	DSN string

	// Environment is the deployment environment (e.g., "production", "staging")
	Environment string

	// Release is the application version/release identifier
	Release string

	// TracesSampleRate is the sample rate for performance traces (0.0 to 1.0)
	// 1.0 means 100% of transactions are traced
	TracesSampleRate float64

	// ErrorSampleRate is the sample rate for error events (0.0 to 1.0)
	// 1.0 means 100% of errors are captured
	ErrorSampleRate float64

	// Debug enables Sentry debug mode
	Debug bool

	// ServerName is the name of this server/instance
	ServerName string
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		DSN:              "",
		Environment:      "development",
		Release:          "unknown",
		TracesSampleRate: 0.1, // 10% of transactions
		ErrorSampleRate:  1.0, // 100% of errors
		Debug:            false,
		ServerName:       "",
	}
}

// Tracer wraps Sentry functionality and provides tracing utilities
type Tracer struct {
	config  *Config
	logger  *zap.Logger
	enabled bool
}

// NewTracer initializes Sentry and returns a Tracer instance
func NewTracer(config *Config, logger *zap.Logger) (*Tracer, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if logger == nil {
		logger = zap.NewNop()
	}

	t := &Tracer{
		config:  config,
		logger:  logger,
		enabled: config.DSN != "",
	}

	if !t.enabled {
		logger.Info("Sentry tracing disabled (no DSN configured)")
		return t, nil
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:         config.DSN,
		Environment: config.Environment,
		Release:     config.Release,
		Debug:       config.Debug,
		ServerName:  config.ServerName,
		// Sample rate for performance traces (0.0 to 1.0)
		TracesSampleRate: config.TracesSampleRate,
		// Sample rate for error events (0.0 to 1.0)
		SampleRate: config.ErrorSampleRate,
		// Attach stack traces to all messages
		AttachStacktrace: true,
		// Set before send hook for additional processing
		BeforeSend: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			// Add custom tags
			event.Tags["service"] = "vpsie-autoscaler"
			return event
		},
		BeforeSendTransaction: func(event *sentry.Event, hint *sentry.EventHint) *sentry.Event {
			// Add custom tags to transactions
			event.Tags["service"] = "vpsie-autoscaler"
			return event
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Sentry: %w", err)
	}

	logger.Info("Sentry tracing initialized",
		zap.String("environment", config.Environment),
		zap.String("release", config.Release),
		zap.Float64("tracesSampleRate", config.TracesSampleRate),
		zap.Float64("errorSampleRate", config.ErrorSampleRate),
	)

	return t, nil
}

// IsEnabled returns true if Sentry tracing is enabled
func (t *Tracer) IsEnabled() bool {
	return t.enabled
}

// Flush waits for all events to be sent to Sentry
func (t *Tracer) Flush(timeout time.Duration) {
	if t.enabled {
		sentry.Flush(timeout)
	}
}

// Close flushes and closes the Sentry client
func (t *Tracer) Close() {
	t.Flush(5 * time.Second)
}

// CaptureError captures an error and sends it to Sentry
func (t *Tracer) CaptureError(err error, tags map[string]string) {
	if !t.enabled || err == nil {
		return
	}

	sentry.WithScope(func(scope *sentry.Scope) {
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		sentry.CaptureException(err)
	})
}

// CaptureErrorWithContext captures an error with context
func (t *Tracer) CaptureErrorWithContext(ctx context.Context, err error, tags map[string]string) {
	if !t.enabled || err == nil {
		return
	}

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}

	hub.WithScope(func(scope *sentry.Scope) {
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		hub.CaptureException(err)
	})
}

// CaptureMessage captures a message and sends it to Sentry
func (t *Tracer) CaptureMessage(message string, level sentry.Level, tags map[string]string) {
	if !t.enabled {
		return
	}

	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(level)
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		sentry.CaptureMessage(message)
	})
}

// StartTransaction starts a new Sentry transaction for performance monitoring
func (t *Tracer) StartTransaction(ctx context.Context, name, op string) (context.Context, *sentry.Span) {
	if !t.enabled {
		return ctx, nil
	}

	options := []sentry.SpanOption{
		sentry.WithOpName(op),
		sentry.WithTransactionSource(sentry.SourceCustom),
	}

	span := sentry.StartTransaction(ctx, name, options...)
	return span.Context(), span
}

// StartSpan starts a new child span within an existing transaction
func (t *Tracer) StartSpan(ctx context.Context, op string, description string) *sentry.Span {
	if !t.enabled {
		return nil
	}

	span := sentry.StartSpan(ctx, op)
	if span != nil {
		span.Description = description
	}
	return span
}

// FinishSpan finishes a span if it's not nil
func (t *Tracer) FinishSpan(span *sentry.Span) {
	if span != nil {
		span.Finish()
	}
}

// SetSpanStatus sets the status of a span
func (t *Tracer) SetSpanStatus(span *sentry.Span, status sentry.SpanStatus) {
	if span != nil {
		span.Status = status
	}
}

// SetSpanData sets data on a span
func (t *Tracer) SetSpanData(span *sentry.Span, key string, value interface{}) {
	if span != nil {
		span.SetData(key, value)
	}
}

// SetSpanTag sets a tag on a span
func (t *Tracer) SetSpanTag(span *sentry.Span, key, value string) {
	if span != nil {
		span.SetTag(key, value)
	}
}

// RecordError records an error on a span
func (t *Tracer) RecordError(span *sentry.Span, err error) {
	if span != nil && err != nil {
		span.Status = sentry.SpanStatusInternalError
		span.SetData("error", err.Error())
	}
}

// HTTPTransport wraps an http.RoundTripper to add Sentry tracing
type HTTPTransport struct {
	tracer    *Tracer
	transport http.RoundTripper
}

// NewHTTPTransport creates a new tracing HTTP transport
func NewHTTPTransport(tracer *Tracer, transport http.RoundTripper) *HTTPTransport {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &HTTPTransport{
		tracer:    tracer,
		transport: transport,
	}
}

// RoundTrip implements http.RoundTripper with Sentry tracing
func (t *HTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.tracer == nil || !t.tracer.enabled {
		return t.transport.RoundTrip(req)
	}

	// Start a span for the HTTP request
	span := t.tracer.StartSpan(req.Context(), "http.client", fmt.Sprintf("%s %s", req.Method, req.URL.Path))
	if span != nil {
		span.SetTag("http.method", req.Method)
		span.SetTag("http.url", req.URL.String())
		span.SetTag("http.host", req.URL.Host)
		defer span.Finish()
	}

	// Perform the request
	resp, err := t.transport.RoundTrip(req)

	if span != nil {
		if err != nil {
			span.Status = sentry.SpanStatusInternalError
			span.SetData("error", err.Error())
		} else {
			span.SetTag("http.status_code", fmt.Sprintf("%d", resp.StatusCode))
			if resp.StatusCode >= 400 {
				span.Status = sentry.SpanStatusInternalError
			} else {
				span.Status = sentry.SpanStatusOK
			}
		}
	}

	return resp, err
}

// ReconcileTracer provides helpers for tracing reconciliation loops
type ReconcileTracer struct {
	tracer         *Tracer
	controllerName string
}

// NewReconcileTracer creates a new reconcile tracer for a controller
func NewReconcileTracer(tracer *Tracer, controllerName string) *ReconcileTracer {
	return &ReconcileTracer{
		tracer:         tracer,
		controllerName: controllerName,
	}
}

// StartReconcile starts a transaction for a reconciliation
func (rt *ReconcileTracer) StartReconcile(ctx context.Context, resourceName, namespace string) (context.Context, *sentry.Span) {
	if rt.tracer == nil || !rt.tracer.enabled {
		return ctx, nil
	}

	txName := fmt.Sprintf("%s.Reconcile", rt.controllerName)
	ctx, span := rt.tracer.StartTransaction(ctx, txName, "controller.reconcile")
	if span != nil {
		span.SetTag("controller", rt.controllerName)
		span.SetTag("resource.name", resourceName)
		span.SetTag("resource.namespace", namespace)
	}
	return ctx, span
}

// FinishReconcile finishes a reconciliation transaction
func (rt *ReconcileTracer) FinishReconcile(span *sentry.Span, err error) {
	if span == nil {
		return
	}

	if err != nil {
		span.Status = sentry.SpanStatusInternalError
		span.SetData("error", err.Error())
		// Also capture the error as an event
		if rt.tracer != nil && rt.tracer.enabled {
			rt.tracer.CaptureError(err, map[string]string{
				"controller": rt.controllerName,
			})
		}
	} else {
		span.Status = sentry.SpanStatusOK
	}
	span.Finish()
}

// Global tracer instance for convenience
var globalTracer *Tracer

// SetGlobalTracer sets the global tracer instance
func SetGlobalTracer(t *Tracer) {
	globalTracer = t
}

// GetGlobalTracer returns the global tracer instance
func GetGlobalTracer() *Tracer {
	return globalTracer
}

// CaptureError is a convenience function that uses the global tracer
func CaptureError(err error, tags map[string]string) {
	if globalTracer != nil {
		globalTracer.CaptureError(err, tags)
	}
}

// StartTransaction is a convenience function that uses the global tracer
func StartTransaction(ctx context.Context, name, op string) (context.Context, *sentry.Span) {
	if globalTracer != nil {
		return globalTracer.StartTransaction(ctx, name, op)
	}
	return ctx, nil
}

// StartSpan is a convenience function that uses the global tracer
func StartSpan(ctx context.Context, op, description string) *sentry.Span {
	if globalTracer != nil {
		return globalTracer.StartSpan(ctx, op, description)
	}
	return nil
}
