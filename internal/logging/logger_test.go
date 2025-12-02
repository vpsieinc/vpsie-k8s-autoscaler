package logging

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name        string
		development bool
		wantErr     bool
	}{
		{
			name:        "production logger",
			development: false,
			wantErr:     false,
		},
		{
			name:        "development logger",
			development: true,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := NewLogger(tt.development)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, logger)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, logger)

				// Verify logger works
				logger.Info("test info message")
				logger.Debug("test debug message")
				logger.Warn("test warn message", zap.String("key", "value"))
				logger.Error("test error message", zap.Int("count", 42))
			}
		})
	}
}

func TestNewLogger_ProductionMode(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)
	require.NotNil(t, logger)

	// Production logger should use JSON encoding and have sensible defaults
	logger.Info("production log",
		zap.String("environment", "production"),
		zap.Int("port", 8080),
	)
}

func TestNewLogger_DevelopmentMode(t *testing.T) {
	logger, err := NewLogger(true)
	require.NoError(t, err)
	require.NotNil(t, logger)

	// Development logger should use console encoding with color
	logger.Info("development log",
		zap.String("environment", "development"),
		zap.Bool("debug", true),
	)
}

func TestNewZapLogger(t *testing.T) {
	tests := []struct {
		name        string
		development bool
	}{
		{
			name:        "production mode",
			development: false,
		},
		{
			name:        "development mode",
			development: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a zap logger
			zapLogger, err := NewLogger(tt.development)
			require.NoError(t, err)
			require.NotNil(t, zapLogger)

			// Convert to logr.Logger
			logrLogger := NewZapLogger(zapLogger, tt.development)
			assert.NotNil(t, logrLogger)

			// Test that logr.Logger works
			logrLogger.Info("test message", "key", "value", "number", 42)
			logrLogger.Error(nil, "test error", "reason", "testing")

			// Test with context
			logrLogger.WithName("test-component").Info("named logger")
			logrLogger.WithValues("component", "test").Info("logger with values")
		})
	}
}

func TestWithRequestID(t *testing.T) {
	ctx := context.Background()

	// Add request ID to context
	ctxWithID := WithRequestID(ctx)
	assert.NotNil(t, ctxWithID)

	// Verify request ID was added
	requestID := GetRequestID(ctxWithID)
	assert.NotEmpty(t, requestID)

	// Verify it's a valid UUID format (36 characters with dashes)
	assert.Len(t, requestID, 36)
	assert.Contains(t, requestID, "-")
}

func TestGetRequestID(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		expected string
	}{
		{
			name:     "context without request ID returns empty string",
			ctx:      context.Background(),
			expected: "",
		},
		{
			name:     "context with request ID returns ID",
			ctx:      WithRequestID(context.Background()),
			expected: "", // Will be non-empty UUID, tested separately
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requestID := GetRequestID(tt.ctx)
			if tt.name == "context without request ID returns empty string" {
				assert.Empty(t, requestID)
			} else {
				assert.NotEmpty(t, requestID)
			}
		})
	}
}

func TestWithRequestIDField(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	t.Run("context with request ID adds field", func(t *testing.T) {
		ctx := WithRequestID(context.Background())
		loggerWithID := WithRequestIDField(ctx, logger)

		assert.NotNil(t, loggerWithID)
		// Logger should have request ID field
		loggerWithID.Info("test message")
	})

	t.Run("context without request ID returns original logger", func(t *testing.T) {
		ctx := context.Background()
		loggerWithoutID := WithRequestIDField(ctx, logger)

		assert.NotNil(t, loggerWithoutID)
		loggerWithoutID.Info("test message without request ID")
	})
}

func TestLogScaleUpDecision(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogScaleUpDecision(logger, "test-nodegroup", "default", 2, 5, 3, "insufficient capacity")
}

func TestLogScaleDownDecision(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogScaleDownDecision(logger, "test-nodegroup", "default", 5, 2, 3, "low utilization")
}

func TestLogAPICall(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogAPICall(logger, "GET", "/api/v2/vms", "request-123")
}

func TestLogAPIResponse(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogAPIResponse(logger, "GET", "/api/v2/vms", 200, "150ms", "request-123")
}

func TestLogAPIError(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogAPIError(logger, "POST", "/api/v2/vms", 500, assert.AnError, "request-123")
}

func TestLogNodeProvisioningStart(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogNodeProvisioningStart(logger, "node-1", "nodegroup-1", "default", "small-2cpu-4gb")
}

func TestLogNodeProvisioningComplete(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogNodeProvisioningComplete(logger, "node-1", "nodegroup-1", "default", "5m30s")
}

func TestLogNodeProvisioningFailed(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogNodeProvisioningFailed(logger, "node-1", "nodegroup-1", "default", assert.AnError, "API timeout")
}

func TestLogNodeTerminationStart(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogNodeTerminationStart(logger, "node-1", "nodegroup-1", "default")
}

func TestLogNodeTerminationComplete(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogNodeTerminationComplete(logger, "node-1", "nodegroup-1", "default", "2m15s")
}

func TestLogNodeTerminationFailed(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogNodeTerminationFailed(logger, "node-1", "nodegroup-1", "default", assert.AnError, "API error")
}

func TestLogPhaseTransition(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogPhaseTransition(logger, "node-1", "nodegroup-1", "default", "Pending", "Provisioning", "VM creation started")
}

func TestLogUnschedulablePods(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogUnschedulablePods(logger, 5, "InsufficientCPU", "default")
}

func TestLogReconciliationStart(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogReconciliationStart(logger, "NodeGroupController", "my-nodegroup", "default")
}

func TestLogReconciliationComplete(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogReconciliationComplete(logger, "NodeGroupController", "my-nodegroup", "default", "150ms", "success")
}

func TestLogReconciliationError(t *testing.T) {
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Should not panic
	LogReconciliationError(logger, "NodeGroupController", "my-nodegroup", "default", assert.AnError, "ValidationError")
}

func TestRequestIDUniqueness(t *testing.T) {
	// Create multiple contexts with request IDs
	ids := make(map[string]bool)
	count := 100

	for i := 0; i < count; i++ {
		ctx := WithRequestID(context.Background())
		id := GetRequestID(ctx)

		// Verify ID is not empty
		assert.NotEmpty(t, id)

		// Verify ID is unique
		assert.False(t, ids[id], "Request ID should be unique, got duplicate: %s", id)
		ids[id] = true
	}

	// Verify we got the expected number of unique IDs
	assert.Len(t, ids, count)
}

func TestLoggerIntegration(t *testing.T) {
	// Create a production logger
	logger, err := NewLogger(false)
	require.NoError(t, err)

	// Create context with request ID
	ctx := WithRequestID(context.Background())
	requestID := GetRequestID(ctx)
	assert.NotEmpty(t, requestID)

	// Create logger with request ID field
	loggerWithID := WithRequestIDField(ctx, logger)
	assert.NotNil(t, loggerWithID)

	// Use various logging functions
	LogScaleUpDecision(loggerWithID, "test-ng", "default", 3, 5, 2, "scale up needed")
	LogAPICall(loggerWithID, "GET", "/vms", requestID)
	LogNodeProvisioningStart(loggerWithID, "node-1", "test-ng", "default", "small")
	LogPhaseTransition(loggerWithID, "node-1", "test-ng", "default", "Pending", "Provisioning", "started")
	LogReconciliationStart(loggerWithID, "NodeGroupController", "test-ng", "default")
}

func TestNewZapLogger_Integration(t *testing.T) {
	// Create a zap logger
	zapLogger, err := NewLogger(false)
	require.NoError(t, err)

	// Convert to logr
	logrLogger := NewZapLogger(zapLogger, false)
	assert.NotNil(t, logrLogger)

	// Test various logr operations
	logrLogger.Info("controller started", "version", "v1.0.0")
	logrLogger.WithName("reconciler").Info("reconciling", "name", "test-ng")
	logrLogger.WithValues("component", "scaler").Info("scaling decision", "nodes", 5)

	// Test error logging
	logrLogger.Error(assert.AnError, "reconciliation failed", "retry", true)

	// Test V levels (verbosity)
	logrLogger.V(1).Info("debug message", "level", 1)
	logrLogger.V(2).Info("trace message", "level", 2)
}

func BenchmarkNewLogger(b *testing.B) {
	for i := 0; i < b.N; i++ {
		logger, _ := NewLogger(false)
		_ = logger
	}
}

func BenchmarkNewZapLogger(b *testing.B) {
	zapLogger, _ := NewLogger(false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logrLogger := NewZapLogger(zapLogger, false)
		_ = logrLogger
	}
}

func BenchmarkWithRequestID(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = WithRequestID(ctx)
	}
}

func BenchmarkLogScaleUpDecision(b *testing.B) {
	logger, _ := NewLogger(false)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LogScaleUpDecision(logger, "test-ng", "default", 3, 5, 2, "scale up")
	}
}
