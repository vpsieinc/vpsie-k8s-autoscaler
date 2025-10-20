package logging

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ContextKey is the type for context keys
type ContextKey string

const (
	// RequestIDKey is the context key for request ID
	RequestIDKey ContextKey = "requestID"
)

// NewLogger creates a new structured logger
func NewLogger(development bool) (*zap.Logger, error) {
	var config zap.Config
	if development {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		config = zap.NewProductionConfig()
	}

	// Always use ISO8601 time encoding
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	logger, err := config.Build(
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	if err != nil {
		return nil, err
	}

	return logger, nil
}

// NewZapLogger creates a logr.Logger from a zap.Logger for use with controller-runtime
func NewZapLogger(zapLogger *zap.Logger, development bool) logr.Logger {
	return zapr.NewLogger(zapLogger)
}

// WithRequestID adds a request ID to the context
func WithRequestID(ctx context.Context) context.Context {
	requestID := uuid.New().String()
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// GetRequestID retrieves the request ID from the context
func GetRequestID(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDKey).(string); ok {
		return requestID
	}
	return ""
}

// WithRequestIDField adds request ID field to logger if present in context
func WithRequestIDField(ctx context.Context, logger *zap.Logger) *zap.Logger {
	if requestID := GetRequestID(ctx); requestID != "" {
		return logger.With(zap.String("requestID", requestID))
	}
	return logger
}

// LogScaleUpDecision logs a scale-up decision with full context
func LogScaleUpDecision(logger *zap.Logger, nodeGroup, namespace string, currentNodes, desiredNodes, nodesAdded int32, reason string) {
	logger.Info("Scale-up decision made",
		zap.String("action", "scale-up"),
		zap.String("nodeGroup", nodeGroup),
		zap.String("namespace", namespace),
		zap.Int32("currentNodes", currentNodes),
		zap.Int32("desiredNodes", desiredNodes),
		zap.Int32("nodesAdded", nodesAdded),
		zap.String("reason", reason),
	)
}

// LogScaleDownDecision logs a scale-down decision with full context
func LogScaleDownDecision(logger *zap.Logger, nodeGroup, namespace string, currentNodes, desiredNodes, nodesRemoved int32, reason string) {
	logger.Info("Scale-down decision made",
		zap.String("action", "scale-down"),
		zap.String("nodeGroup", nodeGroup),
		zap.String("namespace", namespace),
		zap.Int32("currentNodes", currentNodes),
		zap.Int32("desiredNodes", desiredNodes),
		zap.Int32("nodesRemoved", nodesRemoved),
		zap.String("reason", reason),
	)
}

// LogAPICall logs a VPSie API call
func LogAPICall(logger *zap.Logger, method, endpoint string, requestID string) {
	logger.Debug("VPSie API call",
		zap.String("method", method),
		zap.String("endpoint", endpoint),
		zap.String("requestID", requestID),
	)
}

// LogAPIResponse logs a VPSie API response
func LogAPIResponse(logger *zap.Logger, method, endpoint string, statusCode int, duration string, requestID string) {
	logger.Debug("VPSie API response",
		zap.String("method", method),
		zap.String("endpoint", endpoint),
		zap.Int("statusCode", statusCode),
		zap.String("duration", duration),
		zap.String("requestID", requestID),
	)
}

// LogAPIError logs a VPSie API error
func LogAPIError(logger *zap.Logger, method, endpoint string, statusCode int, err error, requestID string) {
	logger.Error("VPSie API error",
		zap.String("method", method),
		zap.String("endpoint", endpoint),
		zap.Int("statusCode", statusCode),
		zap.Error(err),
		zap.String("requestID", requestID),
	)
}

// LogNodeProvisioningStart logs the start of node provisioning
func LogNodeProvisioningStart(logger *zap.Logger, nodeName, nodeGroup, namespace, instanceType string) {
	logger.Info("Starting node provisioning",
		zap.String("node", nodeName),
		zap.String("nodeGroup", nodeGroup),
		zap.String("namespace", namespace),
		zap.String("instanceType", instanceType),
	)
}

// LogNodeProvisioningComplete logs the completion of node provisioning
func LogNodeProvisioningComplete(logger *zap.Logger, nodeName, nodeGroup, namespace string, duration string) {
	logger.Info("Node provisioning completed",
		zap.String("node", nodeName),
		zap.String("nodeGroup", nodeGroup),
		zap.String("namespace", namespace),
		zap.String("duration", duration),
	)
}

// LogNodeProvisioningFailed logs a node provisioning failure
func LogNodeProvisioningFailed(logger *zap.Logger, nodeName, nodeGroup, namespace string, err error, reason string) {
	logger.Error("Node provisioning failed",
		zap.String("node", nodeName),
		zap.String("nodeGroup", nodeGroup),
		zap.String("namespace", namespace),
		zap.Error(err),
		zap.String("reason", reason),
	)
}

// LogNodeTerminationStart logs the start of node termination
func LogNodeTerminationStart(logger *zap.Logger, nodeName, nodeGroup, namespace string) {
	logger.Info("Starting node termination",
		zap.String("node", nodeName),
		zap.String("nodeGroup", nodeGroup),
		zap.String("namespace", namespace),
	)
}

// LogNodeTerminationComplete logs the completion of node termination
func LogNodeTerminationComplete(logger *zap.Logger, nodeName, nodeGroup, namespace string, duration string) {
	logger.Info("Node termination completed",
		zap.String("node", nodeName),
		zap.String("nodeGroup", nodeGroup),
		zap.String("namespace", namespace),
		zap.String("duration", duration),
	)
}

// LogNodeTerminationFailed logs a node termination failure
func LogNodeTerminationFailed(logger *zap.Logger, nodeName, nodeGroup, namespace string, err error, reason string) {
	logger.Error("Node termination failed",
		zap.String("node", nodeName),
		zap.String("nodeGroup", nodeGroup),
		zap.String("namespace", namespace),
		zap.Error(err),
		zap.String("reason", reason),
	)
}

// LogPhaseTransition logs a VPSieNode phase transition
func LogPhaseTransition(logger *zap.Logger, nodeName, nodeGroup, namespace string, fromPhase, toPhase, reason string) {
	logger.Info("Node phase transition",
		zap.String("node", nodeName),
		zap.String("nodeGroup", nodeGroup),
		zap.String("namespace", namespace),
		zap.String("fromPhase", fromPhase),
		zap.String("toPhase", toPhase),
		zap.String("reason", reason),
	)
}

// LogUnschedulablePods logs unschedulable pods detected
func LogUnschedulablePods(logger *zap.Logger, count int, constraint, namespace string) {
	logger.Info("Unschedulable pods detected",
		zap.Int("count", count),
		zap.String("constraint", constraint),
		zap.String("namespace", namespace),
	)
}

// LogReconciliationStart logs the start of a reconciliation
func LogReconciliationStart(logger *zap.Logger, controller, objectName, namespace string) {
	logger.Debug("Starting reconciliation",
		zap.String("controller", controller),
		zap.String("object", objectName),
		zap.String("namespace", namespace),
	)
}

// LogReconciliationComplete logs the completion of a reconciliation
func LogReconciliationComplete(logger *zap.Logger, controller, objectName, namespace string, duration string, result string) {
	logger.Debug("Reconciliation completed",
		zap.String("controller", controller),
		zap.String("object", objectName),
		zap.String("namespace", namespace),
		zap.String("duration", duration),
		zap.String("result", result),
	)
}

// LogReconciliationError logs a reconciliation error
func LogReconciliationError(logger *zap.Logger, controller, objectName, namespace string, err error, errorType string) {
	logger.Error("Reconciliation error",
		zap.String("controller", controller),
		zap.String("object", objectName),
		zap.String("namespace", namespace),
		zap.Error(err),
		zap.String("errorType", errorType),
	)
}
