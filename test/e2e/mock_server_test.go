//go:build e2e
// +build e2e

package e2e

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

// MockVPSieServer simulates the VPSie API for E2E testing
type MockVPSieServer struct {
	server *httptest.Server
	mu     sync.RWMutex

	// VM storage
	vms           map[int]*vpsieclient.VPS
	vmTransitions map[int]time.Time
	nextVMID      int

	// Request tracking
	requestCounts map[string]int
	totalRequests atomic.Int64

	// Configuration
	AuthToken      string
	TokenExpiry    time.Time
	RefreshToken   string
	InjectErrors   bool
	ErrorRate      float64
	Latency        time.Duration
	QuotaLimit     int
	QuotaUsed      int
	AutoTransition bool

	// Custom handlers for specific test scenarios
	CustomHandlers map[string]http.HandlerFunc
}

// NewMockVPSieServer creates a new mock VPSie API server
func NewMockVPSieServer() *MockVPSieServer {
	mock := &MockVPSieServer{
		vms:            make(map[int]*vpsieclient.VPS),
		vmTransitions:  make(map[int]time.Time),
		nextVMID:       1000,
		requestCounts:  make(map[string]int),
		AuthToken:      "mock-token-" + randomString(10),
		RefreshToken:   "mock-refresh-" + randomString(10),
		TokenExpiry:    time.Now().Add(24 * time.Hour),
		QuotaLimit:     100,
		QuotaUsed:      0,
		AutoTransition: true,
		CustomHandlers: make(map[string]http.HandlerFunc),
	}

	mux := http.NewServeMux()

	// OAuth endpoints
	mux.HandleFunc("/oauth/token", mock.handleOAuthToken)

	// VM endpoints
	mux.HandleFunc("/v2/vms", mock.handleVMs)
	mux.HandleFunc("/v2/vms/", mock.handleVMDetail)

	// Resource endpoints
	mux.HandleFunc("/v2/offerings", mock.handleOfferings)
	mux.HandleFunc("/v2/datacenters", mock.handleDatacenters)

	// Wrap with middleware
	handler := mock.middleware(mux)
	mock.server = httptest.NewServer(handler)

	// Start state transition worker
	if mock.AutoTransition {
		go mock.stateTransitionWorker()
	}

	return mock
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// URL returns the mock server URL
func (m *MockVPSieServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *MockVPSieServer) Close() {
	if m.server != nil {
		m.server.Close()
	}
}

// GetRequestCount returns the number of requests to a specific endpoint
func (m *MockVPSieServer) GetRequestCount(endpoint string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.requestCounts[endpoint]
}

// GetTotalRequests returns total requests received
func (m *MockVPSieServer) GetTotalRequests() int64 {
	return m.totalRequests.Load()
}

// GetVMCount returns the current number of VMs
func (m *MockVPSieServer) GetVMCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.vms)
}

// SetErrorInjection enables/disables error injection
func (m *MockVPSieServer) SetErrorInjection(enabled bool, rate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InjectErrors = enabled
	m.ErrorRate = rate
}

// SetLatency sets artificial latency for responses
func (m *MockVPSieServer) SetLatency(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Latency = latency
}

// middleware adds logging and latency simulation
func (m *MockVPSieServer) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.totalRequests.Add(1)

		m.mu.Lock()
		m.requestCounts[r.URL.Path]++
		latency := m.Latency
		injectErrors := m.InjectErrors
		errorRate := m.ErrorRate
		m.mu.Unlock()

		// Simulate latency
		if latency > 0 {
			time.Sleep(latency)
		}

		// Inject random errors
		if injectErrors && rand.Float64() < errorRate {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Simulated server error",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleOAuthToken handles OAuth token requests
func (m *MockVPSieServer) handleOAuthToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	m.mu.Lock()
	token := m.AuthToken
	expiry := m.TokenExpiry
	m.mu.Unlock()

	response := map[string]interface{}{
		"access_token":  token,
		"token_type":    "Bearer",
		"expires_in":    3600,
		"expires_at":    expiry.Format(time.RFC3339),
		"refresh_token": m.RefreshToken,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleVMs handles VM list and create operations
func (m *MockVPSieServer) handleVMs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		m.listVMs(w, r)
	case http.MethodPost:
		m.createVM(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *MockVPSieServer) listVMs(w http.ResponseWriter, r *http.Request) {
	m.mu.RLock()
	vms := make([]*vpsieclient.VPS, 0, len(m.vms))
	for _, vm := range m.vms {
		vms = append(vms, vm)
	}
	m.mu.RUnlock()

	response := map[string]interface{}{
		"data": vms,
		"meta": map[string]int{
			"total": len(vms),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (m *MockVPSieServer) createVM(w http.ResponseWriter, r *http.Request) {
	var req vpsieclient.CreateVPSRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check quota
	if m.QuotaUsed >= m.QuotaLimit {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Quota exceeded",
		})
		return
	}

	vmID := m.nextVMID
	m.nextVMID++

	vm := &vpsieclient.VPS{
		ID:           vmID,
		Hostname:     req.Hostname,
		State:        "provisioning",
		CPU:          2,
		RAM:          4096,
		Disk:         50,
		DatacenterID: req.DatacenterID,
		CreatedAt:    time.Now().Format(time.RFC3339),
	}

	m.vms[vmID] = vm
	m.vmTransitions[vmID] = time.Now().Add(10 * time.Second)
	m.QuotaUsed++

	response := map[string]interface{}{
		"data": vm,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// handleVMDetail handles individual VM operations
func (m *MockVPSieServer) handleVMDetail(w http.ResponseWriter, r *http.Request) {
	// Extract VM ID from path: /v2/vms/{id}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	vmID, err := strconv.Atoi(parts[2])
	if err != nil {
		http.Error(w, "Invalid VM ID", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		m.getVM(w, vmID)
	case http.MethodDelete:
		m.deleteVM(w, vmID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (m *MockVPSieServer) getVM(w http.ResponseWriter, vmID int) {
	m.mu.RLock()
	vm, exists := m.vms[vmID]
	m.mu.RUnlock()

	if !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "VM not found",
		})
		return
	}

	response := map[string]interface{}{
		"data": vm,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (m *MockVPSieServer) deleteVM(w http.ResponseWriter, vmID int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.vms[vmID]; !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "VM not found",
		})
		return
	}

	delete(m.vms, vmID)
	delete(m.vmTransitions, vmID)
	m.QuotaUsed--

	w.WriteHeader(http.StatusNoContent)
}

// handleOfferings returns available VM offerings
func (m *MockVPSieServer) handleOfferings(w http.ResponseWriter, r *http.Request) {
	offerings := []map[string]interface{}{
		{
			"id":          "small-2cpu-4gb",
			"name":        "Small",
			"cpu":         2,
			"ram":         4096,
			"disk":        50,
			"price_hour":  0.02,
			"price_month": 14.40,
		},
		{
			"id":          "medium-4cpu-8gb",
			"name":        "Medium",
			"cpu":         4,
			"ram":         8192,
			"disk":        100,
			"price_hour":  0.04,
			"price_month": 28.80,
		},
		{
			"id":          "large-8cpu-16gb",
			"name":        "Large",
			"cpu":         8,
			"ram":         16384,
			"disk":        200,
			"price_hour":  0.08,
			"price_month": 57.60,
		},
	}

	response := map[string]interface{}{
		"data": offerings,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDatacenters returns available datacenters
func (m *MockVPSieServer) handleDatacenters(w http.ResponseWriter, r *http.Request) {
	datacenters := []map[string]interface{}{
		{
			"id":       "dc-test-1",
			"name":     "Test DC 1",
			"location": "US East",
			"country":  "US",
		},
		{
			"id":       "dc-test-2",
			"name":     "Test DC 2",
			"location": "EU West",
			"country":  "NL",
		},
	}

	response := map[string]interface{}{
		"data": datacenters,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// stateTransitionWorker automatically transitions VM states
func (m *MockVPSieServer) stateTransitionWorker() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for range ticker.C {
		m.mu.Lock()
		now := time.Now()
		for vmID, transitionTime := range m.vmTransitions {
			if now.After(transitionTime) {
				if vm, exists := m.vms[vmID]; exists {
					switch vm.State {
					case "provisioning":
						vm.State = "running"
						m.vmTransitions[vmID] = now.Add(5 * time.Second)
					case "running":
						vm.State = "ready"
						delete(m.vmTransitions, vmID)
					}
				}
			}
		}
		m.mu.Unlock()
	}
}

// AddVM adds a VM directly (for test setup)
func (m *MockVPSieServer) AddVM(hostname, state string) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	vmID := m.nextVMID
	m.nextVMID++

	m.vms[vmID] = &vpsieclient.VPS{
		ID:           vmID,
		Hostname:     hostname,
		State:        state,
		CPU:          2,
		RAM:          4096,
		Disk:         50,
		DatacenterID: "dc-test-1",
		CreatedAt:    time.Now().Format(time.RFC3339),
	}
	m.QuotaUsed++

	return vmID
}

// SetVMState sets the state of a specific VM
func (m *MockVPSieServer) SetVMState(vmID int, state string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	vm, exists := m.vms[vmID]
	if !exists {
		return fmt.Errorf("VM %d not found", vmID)
	}

	vm.State = state
	return nil
}

// Reset clears all VMs and resets counters
func (m *MockVPSieServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.vms = make(map[int]*vpsieclient.VPS)
	m.vmTransitions = make(map[int]time.Time)
	m.requestCounts = make(map[string]int)
	m.QuotaUsed = 0
	m.totalRequests.Store(0)
}
