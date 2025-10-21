//go:build integration
// +build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"time"

	vpsieclient "github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// MockVPSieServer is a mock HTTP server that simulates the VPSie API
type MockVPSieServer struct {
	server         *httptest.Server
	mu             sync.RWMutex
	vms            map[int]*vpsieclient.VPS
	nextVMID       int
	requestCounts  map[string]int
	rateLimit      int
	rateLimitReset time.Time
	currentRequests int

	// Configuration
	AuthToken      string
	TokenExpiry    time.Time
	InjectErrors   bool
	ErrorRate      float64 // 0.0 to 1.0
	Latency        time.Duration
}

// NewMockVPSieServer creates and starts a new mock VPSie API server
func NewMockVPSieServer() *MockVPSieServer {
	mock := &MockVPSieServer{
		vms:           make(map[int]*vpsieclient.VPS),
		nextVMID:      1000,
		requestCounts: make(map[string]int),
		rateLimit:     100,
		AuthToken:     "mock-access-token-12345",
		TokenExpiry:   time.Now().Add(24 * time.Hour),
	}

	// Create HTTP server with mux
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/from/api", mock.handleAuth)
	mux.HandleFunc("/v2/vms", mock.handleVMsList)
	mux.HandleFunc("/v2/vms/", mock.handleVMsDetail)
	mux.HandleFunc("/v2/offerings", mock.handleOfferings)
	mux.HandleFunc("/v2/datacenters", mock.handleDatacenters)

	mock.server = httptest.NewServer(mux)
	return mock
}

// URL returns the base URL of the mock server
func (m *MockVPSieServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *MockVPSieServer) Close() {
	m.server.Close()
}

// GetRequestCount returns the number of requests for a specific endpoint
func (m *MockVPSieServer) GetRequestCount(endpoint string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.requestCounts[endpoint]
}

// ResetRequestCounts resets all request counters
func (m *MockVPSieServer) ResetRequestCounts() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestCounts = make(map[string]int)
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

// applyLatency simulates network latency
func (m *MockVPSieServer) applyLatency() {
	if m.Latency > 0 {
		time.Sleep(m.Latency)
	}
}

// incrementRequestCount increments the request counter for an endpoint
func (m *MockVPSieServer) incrementRequestCount(endpoint string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestCounts[endpoint]++
}

// handleAuth handles OAuth authentication requests
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
	if !strings.HasPrefix(authHeader, "Bearer ") || authHeader != "Bearer "+m.AuthToken {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Unauthorized",
			"message": "Invalid or missing authorization token",
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

	// Inject errors if configured
	if m.InjectErrors {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Internal Server Error",
			"message": "Failed to create VM",
			"code":    500,
		})
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Create new VM
	vm := &vpsieclient.VPS{
		ID:           m.nextVMID,
		Name:         req.Name,
		Hostname:     req.Hostname,
		Status:       "provisioning",
		CPU:          4,
		RAM:          8192,
		Disk:         80,
		Bandwidth:    1000,
		IPAddress:    fmt.Sprintf("192.168.%d.%d", (m.nextVMID/256)%256, m.nextVMID%256),
		IPv6Address:  fmt.Sprintf("2001:db8::%x", m.nextVMID),
		OSName:       "Ubuntu",
		OSVersion:    "22.04",
		Tags:         req.Tags,
		Notes:        req.Notes,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	m.vms[m.nextVMID] = vm
	m.nextVMID++

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"data": vm,
	})
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

	_, exists := m.vms[vmID]
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":   "Not Found",
			"message": "VPS not found",
			"code":    404,
		})
		return
	}

	delete(m.vms, vmID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "VPS deleted successfully",
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
