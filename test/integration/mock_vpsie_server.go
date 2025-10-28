//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// RequestLogEntry represents a logged HTTP request
type RequestLogEntry struct {
	Timestamp  time.Time
	Method     string
	Path       string
	Headers    map[string]string
	Body       string
	StatusCode int
	Response   string
}

// VMStateTransition represents a VM state transition
type VMStateTransition struct {
	FromState string
	ToState   string
	Duration  time.Duration
}

// ErrorScenario represents a configured error scenario
type ErrorScenario struct {
	Endpoint   string
	Method     string
	StatusCode int
	Message    string
	ErrorCode  string
	Permanent  bool // If true, error persists; if false, error clears after first occurrence
}

// MockVPSieServer is a mock HTTP server that simulates the VPSie API
type MockVPSieServer struct {
	server          *httptest.Server
	mu              sync.RWMutex
	vms             map[int]*vpsieclient.VPS
	vmTransitions   map[int]time.Time // Track when VMs should transition states
	nextVMID        int
	requestCounts   map[string]int
	rateLimit       int
	rateLimitReset  time.Time
	currentRequests int
	totalRequests   atomic.Int64 // Thread-safe total request counter

	// Configuration
	AuthToken       string
	TokenExpiry     time.Time
	RefreshToken    string
	InjectErrors    bool
	ErrorRate       float64         // 0.0 to 1.0 for random errors
	ErrorScenarios  []ErrorScenario // Specific error scenarios
	Latency         time.Duration
	LatencyVariance time.Duration // Random variance in latency

	// Quota management
	QuotaLimit int // Maximum number of VMs allowed
	QuotaUsed  int // Current number of VMs

	// Custom response handlers for specific tests
	CustomHandlers map[string]http.HandlerFunc

	// State transition configuration
	StateTransitions []VMStateTransition
	AutoTransition   bool // Automatically transition VM states

	// Request/Response logging
	RequestLog  []RequestLogEntry
	LogRequests bool
}

// NewMockVPSieServer creates and starts a new mock VPSie API server
func NewMockVPSieServer() *MockVPSieServer {
	mock := &MockVPSieServer{
		vms:            make(map[int]*vpsieclient.VPS),
		vmTransitions:  make(map[int]time.Time),
		nextVMID:       1000,
		requestCounts:  make(map[string]int),
		rateLimit:      100,
		AuthToken:      "mock-access-token-" + generateRandomString(10),
		RefreshToken:   "mock-refresh-token-" + generateRandomString(10),
		TokenExpiry:    time.Now().Add(24 * time.Hour),
		QuotaLimit:     100,
		QuotaUsed:      0,
		CustomHandlers: make(map[string]http.HandlerFunc),
		RequestLog:     make([]RequestLogEntry, 0),
		StateTransitions: []VMStateTransition{
			{FromState: "provisioning", ToState: "running", Duration: 10 * time.Second},
			{FromState: "running", ToState: "ready", Duration: 5 * time.Second},
		},
		AutoTransition: true,
		ErrorScenarios: make([]ErrorScenario, 0),
	}

	// Create HTTP server with middleware
	mux := http.NewServeMux()

	// OAuth endpoints
	mux.HandleFunc("/oauth/token", mock.handleOAuthToken)
	mux.HandleFunc("/auth/from/api", mock.handleAuth) // Legacy auth endpoint

	// VM endpoints
	mux.HandleFunc("/v2/vms", mock.handleVMsList)
	mux.HandleFunc("/v2/vms/", mock.handleVMsDetail)

	// Resource endpoints
	mux.HandleFunc("/v2/offerings", mock.handleOfferings)
	mux.HandleFunc("/v2/datacenters", mock.handleDatacenters)

	// Wrap with middleware for logging and metrics
	handler := mock.middlewareChain(mux)
	mock.server = httptest.NewServer(handler)

	// Start VM state transition goroutine
	if mock.AutoTransition {
		go mock.vmStateTransitionWorker()
	}

	return mock
}

// generateRandomString generates a random alphanumeric string
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// URL returns the base URL of the mock server
func (m *MockVPSieServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *MockVPSieServer) Close() {
	m.server.Close()
}

// middlewareChain wraps the handler with middleware
func (m *MockVPSieServer) middlewareChain(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record total requests
		m.totalRequests.Add(1)

		// Log request if enabled
		if m.LogRequests {
			m.logRequest(r)
		}

		// Apply latency with variance
		m.applyLatencyWithVariance()

		// Check for custom handler
		if handler, exists := m.CustomHandlers[r.Method+" "+r.URL.Path]; exists {
			handler(w, r)
			return
		}

		// Check error scenarios
		if m.shouldInjectError(r) {
			m.handleErrorScenario(w, r)
			return
		}

		// Random error injection
		if m.InjectErrors && rand.Float64() < m.ErrorRate {
			m.injectRandomError(w)
			return
		}

		// Proceed with normal handling
		next.ServeHTTP(w, r)
	})
}

// vmStateTransitionWorker automatically transitions VM states
func (m *MockVPSieServer) vmStateTransitionWorker() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for vmID, transitionTime := range m.vmTransitions {
			if now.After(transitionTime) {
				if vm, exists := m.vms[vmID]; exists {
					// Find next state transition
					for _, transition := range m.StateTransitions {
						if vm.Status == transition.FromState {
							vm.Status = transition.ToState
							vm.UpdatedAt = now

							// Schedule next transition if applicable
							delete(m.vmTransitions, vmID)
							for _, nextTransition := range m.StateTransitions {
								if vm.Status == nextTransition.FromState {
									m.vmTransitions[vmID] = now.Add(nextTransition.Duration)
									break
								}
							}
							break
						}
					}
				}
			}
		}
		m.mu.Unlock()
	}
}

// logRequest logs HTTP request details
func (m *MockVPSieServer) logRequest(r *http.Request) {
	headers := make(map[string]string)
	for key, values := range r.Header {
		headers[key] = strings.Join(values, ",")
	}

	entry := RequestLogEntry{
		Timestamp: time.Now(),
		Method:    r.Method,
		Path:      r.URL.Path,
		Headers:   headers,
	}

	m.mu.Lock()
	m.RequestLog = append(m.RequestLog, entry)
	m.mu.Unlock()
}

// shouldInjectError checks if an error should be injected for this request
func (m *MockVPSieServer) shouldInjectError(r *http.Request) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i, scenario := range m.ErrorScenarios {
		if (scenario.Endpoint == "" || strings.Contains(r.URL.Path, scenario.Endpoint)) &&
			(scenario.Method == "" || scenario.Method == r.Method) {
			if !scenario.Permanent {
				// Remove non-permanent error after first occurrence
				m.mu.RUnlock()
				m.mu.Lock()
				m.ErrorScenarios = append(m.ErrorScenarios[:i], m.ErrorScenarios[i+1:]...)
				m.mu.Unlock()
				m.mu.RLock()
			}
			return true
		}
	}
	return false
}

// handleErrorScenario handles a configured error scenario
func (m *MockVPSieServer) handleErrorScenario(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, scenario := range m.ErrorScenarios {
		if (scenario.Endpoint == "" || strings.Contains(r.URL.Path, scenario.Endpoint)) &&
			(scenario.Method == "" || scenario.Method == r.Method) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(scenario.StatusCode)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   scenario.ErrorCode,
				"message": scenario.Message,
				"code":    scenario.StatusCode,
			})
			return
		}
	}
}

// injectRandomError injects a random error response
func (m *MockVPSieServer) injectRandomError(w http.ResponseWriter) {
	errors := []struct {
		Code    int
		Message string
		Error   string
	}{
		{500, "Internal server error", "InternalError"},
		{503, "Service temporarily unavailable", "ServiceUnavailable"},
		{504, "Gateway timeout", "GatewayTimeout"},
		{400, "Bad request", "BadRequest"},
	}

	err := errors[rand.Intn(len(errors))]
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   err.Error,
		"message": err.Message,
		"code":    err.Code,
	})
}

// GetRequestCount returns the number of requests for a specific endpoint
func (m *MockVPSieServer) GetRequestCount(endpoint string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.requestCounts[endpoint]
}

// GetTotalRequests returns the total number of requests
func (m *MockVPSieServer) GetTotalRequests() int64 {
	return m.totalRequests.Load()
}

// ResetRequestCounts resets all request counters
func (m *MockVPSieServer) ResetRequestCounts() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestCounts = make(map[string]int)
	m.totalRequests.Store(0)
}

// SetErrorScenario adds an error scenario
func (m *MockVPSieServer) SetErrorScenario(scenario ErrorScenario) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ErrorScenarios = append(m.ErrorScenarios, scenario)
}

// ClearErrorScenarios removes all error scenarios
func (m *MockVPSieServer) ClearErrorScenarios() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ErrorScenarios = make([]ErrorScenario, 0)
}

// SetQuotaLimit sets the maximum number of VMs allowed
func (m *MockVPSieServer) SetQuotaLimit(limit int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.QuotaLimit = limit
}

// SetRateLimit sets the rate limit (requests per minute)
func (m *MockVPSieServer) SetRateLimit(limit int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rateLimit = limit
}

// ExpireToken sets the token to expired state
func (m *MockVPSieServer) ExpireToken() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.TokenExpiry = time.Now().Add(-1 * time.Hour)
}

// RefreshAuthToken generates a new auth token
func (m *MockVPSieServer) RefreshAuthToken() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.AuthToken = "mock-access-token-" + generateRandomString(10)
	m.TokenExpiry = time.Now().Add(24 * time.Hour)
	return m.AuthToken
}

// GetRequestLog returns the request log
func (m *MockVPSieServer) GetRequestLog() []RequestLogEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	logCopy := make([]RequestLogEntry, len(m.RequestLog))
	copy(logCopy, m.RequestLog)
	return logCopy
}

// ClearRequestLog clears the request log
func (m *MockVPSieServer) ClearRequestLog() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RequestLog = make([]RequestLogEntry, 0)
}

// SetVMStatus updates the status of a VM
func (m *MockVPSieServer) SetVMStatus(vmID int, status string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.vms[vmID]
	if !exists {
		return fmt.Errorf("VM %d not found", vmID)
	}

	vm.Status = status
	return nil
}

// GetVM returns a VM by ID
func (m *MockVPSieServer) GetVM(vmID int) (*vpsieclient.VPS, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vm, exists := m.vms[vmID]
	if !exists {
		return nil, fmt.Errorf("VM %d not found", vmID)
	}

	// Return a copy
	vmCopy := *vm
	return &vmCopy, nil
}

// checkRateLimit checks if the rate limit has been exceeded
func (m *MockVPSieServer) checkRateLimit() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Reset counter if time window has passed
	if now.After(m.rateLimitReset) {
		m.currentRequests = 0
		m.rateLimitReset = now.Add(1 * time.Minute)
	}

	m.currentRequests++
	return m.currentRequests > m.rateLimit
}

// applyLatency simulates network latency (legacy method)
func (m *MockVPSieServer) applyLatency() {
	if m.Latency > 0 {
		time.Sleep(m.Latency)
	}
}

// applyLatencyWithVariance simulates network latency with variance
func (m *MockVPSieServer) applyLatencyWithVariance() {
	if m.Latency > 0 {
		latency := m.Latency
		if m.LatencyVariance > 0 {
			// Add random variance
			variance := time.Duration(rand.Int63n(int64(m.LatencyVariance)))
			if rand.Intn(2) == 0 {
				latency += variance
			} else if latency > variance {
				latency -= variance
			}
		}
		time.Sleep(latency)
	}
}

// incrementRequestCount increments the request counter for an endpoint
func (m *MockVPSieServer) incrementRequestCount(endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestCounts[endpoint]++
}

// handleOAuthToken handles OAuth token requests (POST /oauth/token)
func (m *MockVPSieServer) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	m.applyLatency()
	m.incrementRequestCount("/oauth/token")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check rate limit
	if m.checkRateLimit() {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":             "too_many_requests",
			"error_description": "Rate limit exceeded",
			"retry_after":       60,
		})
		return
	}

	// Parse form data or JSON
	var grantType, clientID, clientSecret, refreshToken string
	contentType := r.Header.Get("Content-Type")

	if strings.Contains(contentType, "application/x-www-form-urlencoded") {
		r.ParseForm()
		grantType = r.FormValue("grant_type")
		clientID = r.FormValue("client_id")
		clientSecret = r.FormValue("client_secret")
		refreshToken = r.FormValue("refresh_token")
	} else {
		var req struct {
			GrantType    string `json:"grant_type"`
			ClientID     string `json:"client_id"`
			ClientSecret string `json:"client_secret"`
			RefreshToken string `json:"refresh_token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":             "invalid_request",
				"error_description": "Invalid request format",
			})
			return
		}
		grantType = req.GrantType
		clientID = req.ClientID
		clientSecret = req.ClientSecret
		refreshToken = req.RefreshToken
	}

	// Handle different grant types
	switch grantType {
	case "client_credentials":
		if clientID == "" || clientSecret == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":             "invalid_client",
				"error_description": "Client authentication failed",
			})
			return
		}

		// Generate new tokens
		m.mu.Lock()
		m.AuthToken = "mock-access-token-" + generateRandomString(10)
		m.RefreshToken = "mock-refresh-token-" + generateRandomString(10)
		m.TokenExpiry = time.Now().Add(1 * time.Hour)
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  m.AuthToken,
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": m.RefreshToken,
			"scope":         "vps:manage",
		})

	case "refresh_token":
		m.mu.RLock()
		expectedRefreshToken := m.RefreshToken
		m.mu.RUnlock()

		if refreshToken == "" || refreshToken != expectedRefreshToken {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":             "invalid_grant",
				"error_description": "Invalid refresh token",
			})
			return
		}

		// Generate new access token
		m.mu.Lock()
		m.AuthToken = "mock-access-token-" + generateRandomString(10)
		m.TokenExpiry = time.Now().Add(1 * time.Hour)
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  m.AuthToken,
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": m.RefreshToken,
			"scope":         "vps:manage",
		})

	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":             "unsupported_grant_type",
			"error_description": "Grant type not supported",
		})
	}
}

// handleAuth handles legacy authentication requests (POST /auth/from/api)
func (m *MockVPSieServer) handleAuth(w http.ResponseWriter, r *http.Request) {
	m.applyLatency()
	m.incrementRequestCount("/auth/from/api")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check rate limit
	if m.checkRateLimit() {
		w.WriteHeader(http.StatusTooManyRequests)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   true,
			"message": "Rate limit exceeded",
			"code":    429,
		})
		return
	}

	// Parse request body
	var authReq struct {
		ClientID     string `json:"clientId"`
		ClientSecret string `json:"clientSecret"`
	}

	if err := json.NewDecoder(r.Body).Decode(&authReq); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate credentials (accept any non-empty credentials for testing)
	if authReq.ClientID == "" || authReq.ClientSecret == "" {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   true,
			"message": "Invalid credentials",
			"code":    401,
		})
		return
	}

	// Return success with token
	response := map[string]interface{}{
		"accessToken": map[string]string{
			"token":   m.AuthToken,
			"expires": m.TokenExpiry.Format(time.RFC3339),
		},
		"refreshToken": map[string]string{
			"token":   "mock-refresh-token",
			"expires": time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleVMsList handles listing and creating VMs
func (m *MockVPSieServer) handleVMsList(w http.ResponseWriter, r *http.Request) {
	m.applyLatency()
	m.incrementRequestCount("/v2/vms")

	// Check authorization
	authHeader := r.Header.Get("Authorization")
	expectedToken := "Bearer " + m.AuthToken

	// Check token validity
	m.mu.RLock()
	tokenExpired := time.Now().After(m.TokenExpiry)
	m.mu.RUnlock()

	if authHeader == "" || authHeader != expectedToken {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Unauthorized",
			"message": "Invalid or missing authorization token",
			"code":    401,
		})
		return
	}

	if tokenExpired {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "TokenExpired",
			"message": "Access token has expired",
			"code":    401,
		})
		return
	}

	switch r.Method {
	case http.MethodGet:
		m.handleListVMs(w, r)
	case http.MethodPost:
		m.handleCreateVM(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListVMs handles GET /v2/vms
func (m *MockVPSieServer) handleListVMs(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vms := make([]vpsieclient.VPS, 0, len(m.vms))
	for _, vm := range m.vms {
		vms = append(vms, *vm)
	}

	response := map[string]interface{}{
		"data": vms,
		"pagination": map[string]int{
			"total":        len(vms),
			"count":        len(vms),
			"per_page":     50,
			"current_page": 1,
			"total_pages":  1,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleCreateVM handles POST /v2/vms
func (m *MockVPSieServer) handleCreateVM(w http.ResponseWriter, r *http.Request) {
	var req vpsieclient.CreateVPSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check quota limit
	if m.QuotaLimit > 0 && m.QuotaUsed >= m.QuotaLimit {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "QuotaExceeded",
			"message": fmt.Sprintf("VM quota exceeded. Limit: %d, Used: %d", m.QuotaLimit, m.QuotaUsed),
			"code":    403,
			"details": map[string]int{
				"limit": m.QuotaLimit,
				"used":  m.QuotaUsed,
			},
		})
		return
	}

	// Parse offering details if provided
	cpu := 4
	ram := 8192
	disk := 80
	if req.OfferingID != "" {
		// Parse offering to get resources (simplified for mock)
		if strings.Contains(req.OfferingID, "small") {
			cpu, ram, disk = 2, 4096, 40
		} else if strings.Contains(req.OfferingID, "large") {
			cpu, ram, disk = 8, 16384, 160
		}
	}

	// Parse datacenter
	datacenterID := 1
	if req.DatacenterID != "" {
		if strings.Contains(req.DatacenterID, "eu") {
			datacenterID = 2
		} else if strings.Contains(req.DatacenterID, "asia") {
			datacenterID = 3
		}
	}

	// Create new VM
	now := time.Now()
	vm := &vpsieclient.VPS{
		ID:           m.nextVMID,
		Name:         req.Name,
		Hostname:     req.Hostname,
		Status:       "provisioning",
		CPU:          cpu,
		RAM:          ram,
		Disk:         disk,
		Bandwidth:    1000,
		IPAddress:    fmt.Sprintf("192.168.%d.%d", (m.nextVMID/256)%256, m.nextVMID%256),
		IPv6Address:  fmt.Sprintf("2001:db8::%x", m.nextVMID),
		OfferingID:   parseOfferingIDFromString(req.OfferingID),
		DatacenterID: datacenterID,
		OSName:       "Ubuntu",
		OSVersion:    "22.04",
		Tags:         req.Tags,
		Notes:        req.Notes,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	// Parse OS details if provided
	if req.OSImageID != "" {
		if strings.Contains(req.OSImageID, "centos") {
			vm.OSName = "CentOS"
			vm.OSVersion = "8"
		} else if strings.Contains(req.OSImageID, "debian") {
			vm.OSName = "Debian"
			vm.OSVersion = "11"
		}
	}

	m.vms[m.nextVMID] = vm
	m.QuotaUsed++

	// Schedule state transitions if auto-transition is enabled
	if m.AutoTransition && len(m.StateTransitions) > 0 {
		for _, transition := range m.StateTransitions {
			if transition.FromState == vm.Status {
				m.vmTransitions[m.nextVMID] = now.Add(transition.Duration)
				break
			}
		}
	}

	m.nextVMID++

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": vm,
	})
}

// parseOfferingIDFromString converts string offering ID to int
func parseOfferingIDFromString(id string) int {
	// Simple mapping for mock purposes
	switch {
	case strings.Contains(id, "small"):
		return 1
	case strings.Contains(id, "medium"):
		return 2
	case strings.Contains(id, "large"):
		return 3
	default:
		return 1
	}
}

// handleVMsDetail handles GET and DELETE for specific VMs
func (m *MockVPSieServer) handleVMsDetail(w http.ResponseWriter, r *http.Request) {
	m.applyLatency()

	// Check authorization
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") || authHeader != "Bearer "+m.AuthToken {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Unauthorized",
			"message": "Invalid or missing authorization token",
			"code":    401,
		})
		return
	}

	// Extract VM ID from path: /v2/vms/{id}
	path := strings.TrimPrefix(r.URL.Path, "/v2/vms/")
	vmID, err := strconv.Atoi(path)
	if err != nil {
		http.Error(w, "Invalid VM ID", http.StatusBadRequest)
		return
	}

	m.incrementRequestCount(fmt.Sprintf("/v2/vms/%d", vmID))

	switch r.Method {
	case http.MethodGet:
		m.handleGetVM(w, r, vmID)
	case http.MethodDelete:
		m.handleDeleteVM(w, r, vmID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetVM handles GET /v2/vms/{id}
func (m *MockVPSieServer) handleGetVM(w http.ResponseWriter, r *http.Request, vmID int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	vm, exists := m.vms[vmID]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Not Found",
			"message": "VPS not found",
			"code":    404,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": vm,
	})
}

// handleDeleteVM handles DELETE /v2/vms/{id}
func (m *MockVPSieServer) handleDeleteVM(w http.ResponseWriter, r *http.Request, vmID int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.vms[vmID]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "NotFound",
			"message": fmt.Sprintf("VPS with ID %d not found", vmID),
			"code":    404,
		})
		return
	}

	// Check if VM is in a deletable state
	if vm.Status == "deleting" {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "ResourceInTransition",
			"message": "VPS is already being deleted",
			"code":    409,
		})
		return
	}

	// Remove from state transitions if exists
	delete(m.vmTransitions, vmID)

	// Delete the VM
	delete(m.vms, vmID)

	// Decrease quota usage
	if m.QuotaUsed > 0 {
		m.QuotaUsed--
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "VPS deleted successfully",
		"vmId":    vmID,
	})
}

// handleOfferings handles GET /v2/offerings
func (m *MockVPSieServer) handleOfferings(w http.ResponseWriter, r *http.Request) {
	m.applyLatency()
	m.incrementRequestCount("/v2/offerings")

	offerings := []vpsieclient.Offering{
		{
			ID:           "small-2cpu-4gb",
			Name:         "Small - 2 CPU, 4GB RAM",
			CPU:          2,
			RAM:          4096,
			Disk:         80,
			Bandwidth:    1000,
			Price:        10.0,
			HourlyPrice:  0.015,
			Available:    true,
			DatacenterID: "dc-us-east-1",
			Category:     "standard",
		},
		{
			ID:           "medium-4cpu-8gb",
			Name:         "Medium - 4 CPU, 8GB RAM",
			CPU:          4,
			RAM:          8192,
			Disk:         160,
			Bandwidth:    2000,
			Price:        20.0,
			HourlyPrice:  0.030,
			Available:    true,
			DatacenterID: "dc-us-east-1",
			Category:     "standard",
		},
	}

	response := map[string]interface{}{
		"data": offerings,
		"pagination": map[string]int{
			"total":        len(offerings),
			"count":        len(offerings),
			"per_page":     50,
			"current_page": 1,
			"total_pages":  1,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// handleDatacenters handles GET /v2/datacenters
func (m *MockVPSieServer) handleDatacenters(w http.ResponseWriter, r *http.Request) {
	m.applyLatency()
	m.incrementRequestCount("/v2/datacenters")

	datacenters := []vpsieclient.Datacenter{
		{
			ID:         "dc-us-east-1",
			Name:       "US East (New York)",
			Code:       "us-east-1",
			Country:    "United States",
			City:       "New York",
			Continent:  "North America",
			Available:  true,
			FeaturedDC: true,
		},
		{
			ID:         "dc-eu-west-1",
			Name:       "EU West (Frankfurt)",
			Code:       "eu-west-1",
			Country:    "Germany",
			City:       "Frankfurt",
			Continent:  "Europe",
			Available:  true,
			FeaturedDC: true,
		},
	}

	response := map[string]interface{}{
		"data": datacenters,
		"pagination": map[string]int{
			"total":        len(datacenters),
			"count":        len(datacenters),
			"per_page":     50,
			"current_page": 1,
			"total_pages":  1,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
