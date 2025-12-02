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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// Server represents the webhook server
type Server struct {
	server             *http.Server
	logger             *zap.Logger
	nodeGroupValidator *NodeGroupValidator
	vpsieNodeValidator *VPSieNodeValidator
	decoder            runtime.Decoder
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

	codecFactory := serializer.NewCodecFactory(scheme)
	decoder := codecFactory.UniversalDeserializer()

	ws := &Server{
		logger:             config.Logger,
		nodeGroupValidator: NewNodeGroupValidator(config.Logger),
		vpsieNodeValidator: NewVPSieNodeValidator(config.Logger),
		decoder:            decoder,
	}

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/validate/nodegroups", ws.handleNodeGroupValidation)
	mux.HandleFunc("/validate/vpsienodes", ws.handleVPSieNodeValidation)
	mux.HandleFunc("/healthz", ws.handleHealthz)
	mux.HandleFunc("/readyz", ws.handleReadyz)

	ws.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Port),
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
		TLSConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
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
	s.logger.Debug("received NodeGroup validation request")

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Decode admission review
	admissionReview := &admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, admissionReview); err != nil {
		s.logger.Error("failed to unmarshal admission review", zap.Error(err))
		http.Error(w, "failed to unmarshal admission review", http.StatusBadRequest)
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
	s.logger.Debug("received VPSieNode validation request")

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.logger.Error("failed to read request body", zap.Error(err))
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Decode admission review
	admissionReview := &admissionv1.AdmissionReview{}
	if err := json.Unmarshal(body, admissionReview); err != nil {
		s.logger.Error("failed to unmarshal admission review", zap.Error(err))
		http.Error(w, "failed to unmarshal admission review", http.StatusBadRequest)
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

// handleHealthz handles liveness probe requests
func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

// handleReadyz handles readiness probe requests
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ready"))
}
