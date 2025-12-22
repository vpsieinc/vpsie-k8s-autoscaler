package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	admissionv1 "k8s.io/api/admission/v1"
	"k8s.io/apimachinery/pkg/runtime"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// createTestServer creates a test webhook server instance
func createTestServer(t *testing.T) *Server {
	logger := zap.NewNop()

	scheme := runtime.NewScheme()
	if err := autoscalerv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add autoscaler types to scheme: %v", err)
	}

	return &Server{
		logger:             logger,
		nodeGroupValidator: NewNodeGroupValidator(logger),
		vpsieNodeValidator: NewVPSieNodeValidator(logger),
		decoder:            nil, // Not needed for these tests
	}
}

// TestHandleNodeGroupValidation_ContentTypeValidation tests Content-Type validation
func TestHandleNodeGroupValidation_ContentTypeValidation(t *testing.T) {
	server := createTestServer(t)

	tests := []struct {
		name           string
		contentType    string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "valid content type",
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest, // Will fail at JSON parsing, but passes content-type check
			expectedBody:   "invalid JSON",
		},
		{
			name:           "missing content type",
			contentType:    "",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedBody:   "Content-Type must be application/json",
		},
		{
			name:           "wrong content type - text/plain",
			contentType:    "text/plain",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedBody:   "Content-Type must be application/json",
		},
		{
			name:           "wrong content type - application/xml",
			contentType:    "application/xml",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedBody:   "Content-Type must be application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/validate/nodegroups", strings.NewReader("invalid"))
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()

			server.handleNodeGroupValidation(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}

// TestHandleNodeGroupValidation_SizeLimit tests request body size limit
func TestHandleNodeGroupValidation_SizeLimit(t *testing.T) {
	server := createTestServer(t)

	tests := []struct {
		name           string
		bodySize       int
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "small request - 1KB",
			bodySize:       1024,
			expectedStatus: http.StatusBadRequest, // Will fail at JSON parsing
			expectedBody:   "invalid JSON",
		},
		{
			name:           "medium request - 64KB",
			bodySize:       64 * 1024,
			expectedStatus: http.StatusBadRequest, // Will fail at JSON parsing
			expectedBody:   "invalid JSON",
		},
		{
			name:           "at limit - 128KB",
			bodySize:       128 * 1024,
			expectedStatus: http.StatusBadRequest, // Will fail at JSON parsing
			expectedBody:   "invalid JSON",
		},
		{
			name:           "over limit - 129KB",
			bodySize:       129 * 1024,
			expectedStatus: http.StatusBadRequest, // Will fail at JSON parsing (LimitReader truncates silently)
			expectedBody:   "invalid JSON",
		},
		{
			name:           "way over limit - 256KB",
			bodySize:       256 * 1024,
			expectedStatus: http.StatusBadRequest, // Will fail at JSON parsing (LimitReader truncates silently)
			expectedBody:   "invalid JSON",
		},
		{
			name:           "extremely large - 1MB",
			bodySize:       1024 * 1024,
			expectedStatus: http.StatusBadRequest, // Will fail at JSON parsing (LimitReader truncates silently)
			expectedBody:   "invalid JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a body of the specified size
			body := make([]byte, tt.bodySize)
			for i := range body {
				body[i] = 'a'
			}

			req := httptest.NewRequest(http.MethodPost, "/validate/nodegroups", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleNodeGroupValidation(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}

// TestHandleNodeGroupValidation_JSONValidation tests JSON structure validation
func TestHandleNodeGroupValidation_JSONValidation(t *testing.T) {
	server := createTestServer(t)

	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "invalid JSON - not JSON at all",
			body:           "not json",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "invalid JSON",
		},
		{
			name:           "invalid JSON - incomplete",
			body:           `{"request":`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "invalid JSON",
		},
		{
			name:           "invalid JSON - empty object",
			body:           `{}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "admission request is nil",
		},
		{
			name:           "invalid JSON - null request",
			body:           `{"request": null}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "admission request is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/validate/nodegroups", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleNodeGroupValidation(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}

// TestHandleNodeGroupValidation_NilRequestValidation tests nil request validation
func TestHandleNodeGroupValidation_NilRequestValidation(t *testing.T) {
	server := createTestServer(t)

	tests := []struct {
		name           string
		admissionReview *admissionv1.AdmissionReview
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "nil request",
			admissionReview: &admissionv1.AdmissionReview{
				Request: nil,
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "admission request is nil",
		},
		{
			name: "empty request",
			admissionReview: &admissionv1.AdmissionReview{
				Request: &admissionv1.AdmissionRequest{},
			},
			expectedStatus: http.StatusOK, // Returns AdmissionResponse with Allowed=false
			expectedBody:   "request object is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := json.Marshal(tt.admissionReview)
			if err != nil {
				t.Fatalf("failed to marshal admission review: %v", err)
			}

			req := httptest.NewRequest(http.MethodPost, "/validate/nodegroups", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleNodeGroupValidation(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedBody != "" && !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}

// TestHandleVPSieNodeValidation_ContentTypeValidation tests Content-Type validation for VPSieNode
func TestHandleVPSieNodeValidation_ContentTypeValidation(t *testing.T) {
	server := createTestServer(t)

	tests := []struct {
		name           string
		contentType    string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "valid content type",
			contentType:    "application/json",
			expectedStatus: http.StatusBadRequest, // Will fail at JSON parsing
			expectedBody:   "invalid JSON",
		},
		{
			name:           "missing content type",
			contentType:    "",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedBody:   "Content-Type must be application/json",
		},
		{
			name:           "wrong content type",
			contentType:    "text/plain",
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedBody:   "Content-Type must be application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/validate/vpsienodes", strings.NewReader("invalid"))
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()

			server.handleVPSieNodeValidation(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}

// TestHandleVPSieNodeValidation_SizeLimit tests request body size limit for VPSieNode
func TestHandleVPSieNodeValidation_SizeLimit(t *testing.T) {
	server := createTestServer(t)

	tests := []struct {
		name           string
		bodySize       int
		expectedStatus int
	}{
		{
			name:           "small request",
			bodySize:       1024,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "at limit - 128KB",
			bodySize:       128 * 1024,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "over limit - 256KB",
			bodySize:       256 * 1024,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := make([]byte, tt.bodySize)
			for i := range body {
				body[i] = 'a'
			}

			req := httptest.NewRequest(http.MethodPost, "/validate/vpsienodes", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleVPSieNodeValidation(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestHandleVPSieNodeValidation_JSONValidation tests JSON validation for VPSieNode
func TestHandleVPSieNodeValidation_JSONValidation(t *testing.T) {
	server := createTestServer(t)

	tests := []struct {
		name           string
		body           string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "invalid JSON",
			body:           "not json",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "invalid JSON",
		},
		{
			name:           "empty object - nil request",
			body:           `{}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "admission request is nil",
		},
		{
			name:           "null request",
			body:           `{"request": null}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "admission request is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/validate/vpsienodes", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			server.handleVPSieNodeValidation(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("expected body to contain %q, got %q", tt.expectedBody, w.Body.String())
			}
		})
	}
}

// TestMaxRequestBodySize verifies the constant value
func TestMaxRequestBodySize(t *testing.T) {
	expected := 128 * 1024 // 128KB
	if MaxRequestBodySize != expected {
		t.Errorf("expected MaxRequestBodySize to be %d, got %d", expected, MaxRequestBodySize)
	}
}

// TestHealthzEndpoint tests the health check endpoint
func TestHealthzEndpoint(t *testing.T) {
	server := createTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	server.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got %q", w.Body.String())
	}
}

// TestReadyzEndpoint tests the readiness check endpoint
func TestReadyzEndpoint(t *testing.T) {
	server := createTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	server.handleReadyz(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "ready" {
		t.Errorf("expected body 'ready', got %q", w.Body.String())
	}
}

// TestValidationLayers_AllLayersInOrder tests that all validation layers are applied in order
func TestValidationLayers_AllLayersInOrder(t *testing.T) {
	server := createTestServer(t)

	tests := []struct {
		name           string
		setupRequest   func() *http.Request
		expectedStatus int
		expectedBody   string
		description    string
	}{
		{
			name: "Layer 1 fails - wrong content type",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/validate/nodegroups", strings.NewReader("{}"))
				req.Header.Set("Content-Type", "text/plain")
				return req
			},
			expectedStatus: http.StatusUnsupportedMediaType,
			expectedBody:   "Content-Type must be application/json",
			description:    "Should fail at Layer 1 (Content-Type check)",
		},
		{
			name: "Layer 2 passes, Layer 3 fails - invalid JSON",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest(http.MethodPost, "/validate/nodegroups", strings.NewReader("not json"))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "invalid JSON",
			description:    "Should fail at Layer 3 (JSON validation)",
		},
		{
			name: "Layer 3 passes, Layer 4 fails - nil request",
			setupRequest: func() *http.Request {
				body, _ := json.Marshal(&admissionv1.AdmissionReview{Request: nil})
				req := httptest.NewRequest(http.MethodPost, "/validate/nodegroups", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				return req
			},
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "admission request is nil",
			description:    "Should fail at Layer 4 (nil request check)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			w := httptest.NewRecorder()

			server.handleNodeGroupValidation(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("%s: expected status %d, got %d", tt.description, tt.expectedStatus, w.Code)
			}

			if !strings.Contains(w.Body.String(), tt.expectedBody) {
				t.Errorf("%s: expected body to contain %q, got %q", tt.description, tt.expectedBody, w.Body.String())
			}
		})
	}
}
