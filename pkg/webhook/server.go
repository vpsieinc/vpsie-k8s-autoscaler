package webhook

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/tracing"
)

const (
	// MaxRequestBodySize for admission webhook requests
	// Typical CRD objects are 10-50KB; 128KB provides ample buffer
	MaxRequestBodySize = 128 * 1024 // 128KB
)

// Server represents the webhook server
type Server struct {
	server                *http.Server
	logger                *zap.Logger
	nodeGroupValidator    *NodeGroupValidator
	vpsieNodeValidator    *VPSieNodeValidator
	nodeDeletionValidator NodeDeletionValidatorInterface
	decoder               runtime.Decoder
}

// ServerConfig contains webhook server configuration
type ServerConfig struct {
	// Port is the port the webhook server listens on
	Port int

	// CertFile is the path to the TLS certificate file
	CertFile string

	// KeyFile is the path to the TLS private key file
	KeyFile string

	// Logger is the logger instance
	Logger *zap.Logger
}

// NewServer creates a new webhook server
func NewServer(config ServerConfig) (*Server, error) {
	if config.Logger == nil {
		config.Logger = zap.NewNop()
	}

	// Create runtime scheme for decoding
	scheme := runtime.NewScheme()
	if err := autoscalerv1alpha1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add autoscaler types to scheme: %w", err)
	}
	// Add core types for Node validation (Fix #8)
	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, fmt.Errorf("failed to add core types to scheme: %w", err)
	}

	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDeserializer()

	ws := &Server{
		logger:                config.Logger,
		nodeGroupValidator:    NewNodeGroupValidator(config.Logger),
		vpsieNodeValidator:    NewVPSieNodeValidator(config.Logger),
		nodeDeletionValidator: NewNodeDeletionValidator(config.Logger),
		decoder:               decoder,
	}

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/validate/nodegroups", ws.handleNodeGroupValidation)
	mux.HandleFunc("/validate/vpsienodes", ws.handleVPSieNodeValidation)
	mux.HandleFunc("/validate/node-deletion", ws.handleNodeDeletionValidation)
	mux.HandleFunc("/healthz", ws.handleHealthz)
	mux.HandleFunc("/readyz", ws.handleReadyz)

	ws.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		TLSConfig: &tls.Config{
			// Require TLS 1.3 for enhanced security
			MinVersion: tls.VersionTLS13,
			// CipherSuites only affect TLS 1.2 and below (if fallback needed)
			// TLS 1.3 uses its own secure cipher suites automatically
			CipherSuites: []uint16{
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
		},
	}

	return ws, nil
}

// Start starts the webhook server
func (s *Server) Start(ctx context.Context, certFile, keyFile string) error {
	s.logger.Info("starting webhook server",
		zap.String("addr", s.server.Addr),
		zap.String("cert", certFile),
		zap.String("key", keyFile))

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or error
	select {
	case <-ctx.Done():
		s.logger.Info("shutting down webhook server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return s.server.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}

// Shutdown gracefully shuts down the webhook server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// handleNodeGroupValidation handles NodeGroup validation requests
func (s *Server) handleNodeGroupValidation(w http.ResponseWriter, r *http.Request) {
	// Start Sentry transaction for tracing
	ctx, span := tracing.StartTransaction(r.Context(), "webhook.validateNodeGroup", "webhook.validate")
	if span != nil {
		span.SetTag("webhook.type", "nodegroup")
		defer span.Finish()
	}
	_ = ctx // ctx available for future use

	s.logger.Debug("received NodeGroup validation request")

	// Layer 1: Validate Content-Type
	if r.Header.Get("Content-Type") != "application/json" {
		s.logger.Warn("invalid content type", zap.String("contentType", r.Header.Get("Content-Type")))
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	// Layer 2: Enforce size limit
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBodySize))
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	defer r.Body.Close()

	// Layer 3: Validate JSON structure
	admissionReview := &admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, admissionReview); err != nil {
		s.logger.Error("failed to unmarshal admission review", zap.Error(err))
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Layer 4: Validate request not nil
	if admissionReview.Request == nil {
		s.logger.Warn("admission request is nil")
		http.Error(w, "admission request is nil", http.StatusBadRequest)
		return
	}

	// Validate the request
	response := s.validateNodeGroup(admissionReview.Request)

	// Build admission review response
	admissionReview.Response = response
	admissionReview.Response.UID = admissionReview.Request.UID

	// Encode response
	respBytes, err := json.Marshal(admissionReview)
	if err != nil {
		s.logger.Error("failed to marshal admission review response", zap.Error(err))
		http.Error(w, "failed to marshal response", http.StatusInternalServerError)
		return
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBytes); err != nil {
		s.logger.Error("failed to write response", zap.Error(err))
	}
}

// handleVPSieNodeValidation handles VPSieNode validation requests
func (s *Server) handleVPSieNodeValidation(w http.ResponseWriter, r *http.Request) {
	// Start Sentry transaction for tracing
	ctx, span := tracing.StartTransaction(r.Context(), "webhook.validateVPSieNode", "webhook.validate")
	if span != nil {
		span.SetTag("webhook.type", "vpsienode")
		defer span.Finish()
	}
	_ = ctx // ctx available for future use

	s.logger.Debug("received VPSieNode validation request")

	// Layer 1: Validate Content-Type
	if r.Header.Get("Content-Type") != "application/json" {
		s.logger.Warn("invalid content type", zap.String("contentType", r.Header.Get("Content-Type")))
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	// Layer 2: Enforce size limit
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBodySize))
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	defer r.Body.Close()

	// Layer 3: Validate JSON structure
	admissionReview := &admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, admissionReview); err != nil {
		s.logger.Error("failed to unmarshal admission review", zap.Error(err))
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Layer 4: Validate request not nil
	if admissionReview.Request == nil {
		s.logger.Warn("admission request is nil")
		http.Error(w, "admission request is nil", http.StatusBadRequest)
		return
	}

	// Validate the request
	response := s.validateVPSieNode(admissionReview.Request)

	// Build admission review response
	admissionReview.Response = response
	admissionReview.Response.UID = admissionReview.Request.UID

	// Encode response
	respBytes, err := json.Marshal(admissionReview)
	if err != nil {
		s.logger.Error("failed to marshal admission review response", zap.Error(err))
		http.Error(w, "failed to marshal response", http.StatusInternalServerError)
		return
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBytes); err != nil {
		s.logger.Error("failed to write response", zap.Error(err))
	}
}

// validateNodeGroup validates a NodeGroup resource
func (s *Server) validateNodeGroup(req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	// Validate Object exists and has content
	if len(req.Object.Raw) == 0 {
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: "request object is empty",
				Code:    http.StatusBadRequest,
			},
		}
	}

	// Decode the NodeGroup
	nodeGroup := &autoscalerv1alpha1.NodeGroup{}
	if _, _, err := s.decoder.Decode(req.Object.Raw, nil, nodeGroup); err != nil {
		s.logger.Error("failed to decode NodeGroup", zap.Error(err))
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("failed to decode NodeGroup: %v", err),
				Code:    http.StatusBadRequest,
			},
		}
	}

	// Validate the NodeGroup
	if err := s.nodeGroupValidator.Validate(nodeGroup, req.Operation); err != nil {
		s.logger.Info("NodeGroup validation failed",
			zap.String("name", nodeGroup.Name),
			zap.String("namespace", nodeGroup.Namespace),
			zap.Error(err))
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: err.Error(),
				Code:    http.StatusUnprocessableEntity,
			},
		}
	}

	s.logger.Debug("NodeGroup validation succeeded",
		zap.String("name", nodeGroup.Name),
		zap.String("namespace", nodeGroup.Namespace))

	return &admissionv1.AdmissionResponse{
		Allowed: true,
	}
}

// validateVPSieNode validates a VPSieNode resource
func (s *Server) validateVPSieNode(req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	// Validate Object exists and has content
	if len(req.Object.Raw) == 0 {
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: "request object is empty",
				Code:    http.StatusBadRequest,
			},
		}
	}

	// Decode the VPSieNode
	vpsieNode := &autoscalerv1alpha1.VPSieNode{}
	if _, _, err := s.decoder.Decode(req.Object.Raw, nil, vpsieNode); err != nil {
		s.logger.Error("failed to decode VPSieNode", zap.Error(err))
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("failed to decode VPSieNode: %v", err),
				Code:    http.StatusBadRequest,
			},
		}
	}

	// Validate the VPSieNode
	if err := s.vpsieNodeValidator.Validate(vpsieNode, req.Operation); err != nil {
		s.logger.Info("VPSieNode validation failed",
			zap.String("name", vpsieNode.Name),
			zap.String("namespace", vpsieNode.Namespace),
			zap.Error(err))
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: err.Error(),
				Code:    http.StatusUnprocessableEntity,
			},
		}
	}

	s.logger.Debug("VPSieNode validation succeeded",
		zap.String("name", vpsieNode.Name),
		zap.String("namespace", vpsieNode.Namespace))

	return &admissionv1.AdmissionResponse{
		Allowed: true,
	}
}

// handleNodeDeletionValidation handles node deletion validation requests
// This addresses Fix #8: RBAC Protection - prevents deletion of non-managed nodes
func (s *Server) handleNodeDeletionValidation(w http.ResponseWriter, r *http.Request) {
	// Start Sentry transaction for tracing
	ctx, span := tracing.StartTransaction(r.Context(), "webhook.validateNodeDeletion", "webhook.validate")
	if span != nil {
		span.SetTag("webhook.type", "node-deletion")
		defer span.Finish()
	}
	_ = ctx // ctx available for future use

	s.logger.Debug("received node deletion validation request")

	// Layer 1: Validate Content-Type
	if r.Header.Get("Content-Type") != "application/json" {
		s.logger.Warn("invalid content type", zap.String("contentType", r.Header.Get("Content-Type")))
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	// Layer 2: Enforce size limit
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxRequestBodySize))
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	defer r.Body.Close()

	// Layer 3: Validate JSON structure
	admissionReview := &admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, admissionReview); err != nil {
		s.logger.Error("failed to unmarshal admission review", zap.Error(err))
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Layer 4: Validate request not nil
	if admissionReview.Request == nil {
		s.logger.Warn("admission request is nil")
		http.Error(w, "admission request is nil", http.StatusBadRequest)
		return
	}

	// Validate the request
	response := s.validateNodeDeletion(admissionReview.Request)

	// Build admission review response
	admissionReview.Response = response
	admissionReview.Response.UID = admissionReview.Request.UID

	// Encode response
	respBytes, err := json.Marshal(admissionReview)
	if err != nil {
		s.logger.Error("failed to marshal admission review response", zap.Error(err))
		http.Error(w, "failed to marshal response", http.StatusInternalServerError)
		return
	}

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBytes); err != nil {
		s.logger.Error("failed to write response", zap.Error(err))
	}
}

// validateNodeDeletion validates a node deletion request
// Only nodes with label "autoscaler.vpsie.com/managed=true" can be deleted
func (s *Server) validateNodeDeletion(req *admissionv1.AdmissionRequest) *admissionv1.AdmissionResponse {
	// Only validate DELETE operations
	if req.Operation != admissionv1.Delete {
		s.logger.Warn("unexpected operation for node deletion webhook",
			zap.String("operation", string(req.Operation)))
		return &admissionv1.AdmissionResponse{
			Allowed: true, // Allow non-DELETE operations
		}
	}

	// For DELETE operations, we need to decode from OldObject, not Object
	// The Object field is empty for DELETE operations in Kubernetes
	if len(req.OldObject.Raw) == 0 {
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: "request oldObject is empty",
				Code:    http.StatusBadRequest,
			},
		}
	}

	// Decode the Node
	node := &corev1.Node{}
	if _, _, err := s.decoder.Decode(req.OldObject.Raw, nil, node); err != nil {
		s.logger.Error("failed to decode Node", zap.Error(err))
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: fmt.Sprintf("failed to decode Node: %v", err),
				Code:    http.StatusBadRequest,
			},
		}
	}

	// Validate the node deletion
	if err := s.nodeDeletionValidator.ValidateDelete(node); err != nil {
		s.logger.Info("node deletion validation failed",
			zap.String("node", node.Name),
			zap.Error(err))
		return &admissionv1.AdmissionResponse{
			Allowed: false,
			Result: &metav1.Status{
				Status:  metav1.StatusFailure,
				Message: err.Error(),
				Code:    http.StatusForbidden,
			},
		}
	}

	s.logger.Debug("node deletion validation succeeded",
		zap.String("node", node.Name))

	return &admissionv1.AdmissionResponse{
		Allowed: true,
	}
}

// handleHealthz handles liveness probe requests
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// handleReadyz handles readiness probe requests
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ready"))
}
