package controller

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"k8s.io/client-go/kubernetes"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// HealthStatus contains detailed health information
type HealthStatus struct {
	// VPSieAPIHealthy indicates if the VPSie API is reachable
	VPSieAPIHealthy bool `json:"vpsieApiHealthy"`

	// KubernetesAPIHealthy indicates if the Kubernetes API is reachable
	KubernetesAPIHealthy bool `json:"kubernetesApiHealthy"`

	// LeaderElected indicates if this instance is the elected leader
	LeaderElected bool `json:"leaderElected"`

	// LastReconcileTime is the time of the last successful reconciliation
	LastReconcileTime time.Time `json:"lastReconcileTime,omitempty"`

	// ReconcileStale indicates if reconciliation is stale (> 5 min since last success)
	ReconcileStale bool `json:"reconcileStale"`

	// CircuitBreakerOpen indicates if the VPSie API circuit breaker is open
	CircuitBreakerOpen bool `json:"circuitBreakerOpen"`

	// LastError contains the last error message if any
	LastError string `json:"lastError,omitempty"`
}

// HealthChecker provides health checking functionality for the controller
type HealthChecker struct {
	vpsieClient       *client.Client
	k8sClient         kubernetes.Interface
	mu                sync.RWMutex
	healthy           bool
	ready             bool
	lastCheck         time.Time
	lastError         error
	checkInterval     time.Duration
	shutdownInitiated bool

	// Enhanced health tracking
	leaderElected         bool
	lastReconcileTime     time.Time
	reconcileStaleTimeout time.Duration
	k8sAPIHealthy         bool
	circuitBreakerOpen    bool
}

// NewHealthChecker creates a new HealthChecker
func NewHealthChecker(vpsieClient *client.Client) *HealthChecker {
	return &HealthChecker{
		vpsieClient:           vpsieClient,
		healthy:               false,
		ready:                 false,
		checkInterval:         30 * time.Second,
		reconcileStaleTimeout: 5 * time.Minute,
	}
}

// SetKubernetesClient sets the Kubernetes client for API health checks
func (h *HealthChecker) SetKubernetesClient(client kubernetes.Interface) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.k8sClient = client
}

// Start begins periodic health checks in the background
func (h *HealthChecker) Start(ctx context.Context) error {
	// Perform initial health check
	if err := h.performCheck(ctx); err != nil {
		return fmt.Errorf("initial health check failed: %w", err)
	}

	// Mark as healthy and ready after successful initial check
	h.mu.Lock()
	h.healthy = true
	h.ready = true
	h.mu.Unlock()

	// Start periodic health checks
	go h.runPeriodicChecks(ctx)

	return nil
}

// runPeriodicChecks runs health checks periodically
func (h *HealthChecker) runPeriodicChecks(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			// Log panic but don't crash - mark as unhealthy
			h.mu.Lock()
			h.healthy = false
			h.lastError = fmt.Errorf("panic in health checker: %v", r)
			h.mu.Unlock()
		}
	}()

	ticker := time.NewTicker(h.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			h.shutdownInitiated = true
			h.mu.Unlock()
			return
		case <-ticker.C:
			if err := h.performCheck(ctx); err != nil {
				h.mu.Lock()
				h.lastError = err
				h.healthy = false
				h.mu.Unlock()
			} else {
				h.mu.Lock()
				h.lastError = nil
				h.healthy = true
				h.mu.Unlock()
			}
		}
	}
}

// performCheck performs health checks against all dependencies
func (h *HealthChecker) performCheck(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var errors []string

	// Check VPSie API connectivity
	vpsieHealthy := true
	_, err := h.vpsieClient.ListVMs(checkCtx)
	if err != nil {
		vpsieHealthy = false
		errors = append(errors, fmt.Sprintf("VPSie API: %v", err))
	}

	// Check Kubernetes API connectivity
	k8sHealthy := true
	h.mu.RLock()
	k8sClient := h.k8sClient
	h.mu.RUnlock()

	if k8sClient != nil {
		_, err := k8sClient.Discovery().ServerVersion()
		if err != nil {
			k8sHealthy = false
			errors = append(errors, fmt.Sprintf("Kubernetes API: %v", err))
		}
	}

	// Check circuit breaker state
	circuitBreakerOpen := false
	if h.vpsieClient != nil {
		cbStats := h.vpsieClient.GetCircuitBreakerStats()
		circuitBreakerOpen = cbStats.State == client.StateOpen
	}

	// Check reconciliation staleness
	h.mu.RLock()
	lastReconcile := h.lastReconcileTime
	staleTimeout := h.reconcileStaleTimeout
	leaderElected := h.leaderElected
	h.mu.RUnlock()

	if leaderElected && !lastReconcile.IsZero() {
		if time.Since(lastReconcile) > staleTimeout {
			errors = append(errors, fmt.Sprintf("reconciliation stale: last success was %v ago", time.Since(lastReconcile).Round(time.Second)))
		}
	}

	// Update state
	h.mu.Lock()
	h.lastCheck = time.Now()
	h.k8sAPIHealthy = k8sHealthy
	h.circuitBreakerOpen = circuitBreakerOpen

	// Determine overall health
	// Healthy if VPSie API is reachable and K8s API is reachable
	// Circuit breaker open is a warning but not a failure
	h.healthy = vpsieHealthy && k8sHealthy

	if len(errors) > 0 {
		h.lastError = fmt.Errorf("%s", errors[0])
	} else {
		h.lastError = nil
	}
	h.mu.Unlock()

	if !h.healthy {
		return fmt.Errorf("health check failed: %v", errors)
	}

	return nil
}

// SetLeaderElected updates the leader election status
func (h *HealthChecker) SetLeaderElected(elected bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.leaderElected = elected
}

// IsLeaderElected returns whether this instance is the elected leader
func (h *HealthChecker) IsLeaderElected() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.leaderElected
}

// RecordReconcileSuccess records a successful reconciliation
func (h *HealthChecker) RecordReconcileSuccess() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.lastReconcileTime = time.Now()
}

// GetLastReconcileTime returns the time of the last successful reconciliation
func (h *HealthChecker) GetLastReconcileTime() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastReconcileTime
}

// IsReconcileStale returns true if reconciliation is stale
func (h *HealthChecker) IsReconcileStale() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.leaderElected || h.lastReconcileTime.IsZero() {
		return false
	}
	return time.Since(h.lastReconcileTime) > h.reconcileStaleTimeout
}

// IsCircuitBreakerOpen returns true if the circuit breaker is open
func (h *HealthChecker) IsCircuitBreakerOpen() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.circuitBreakerOpen
}

// GetHealthStatus returns detailed health status information
func (h *HealthChecker) GetHealthStatus() HealthStatus {
	h.mu.RLock()
	defer h.mu.RUnlock()

	status := HealthStatus{
		VPSieAPIHealthy:      h.healthy,
		KubernetesAPIHealthy: h.k8sAPIHealthy,
		LeaderElected:        h.leaderElected,
		LastReconcileTime:    h.lastReconcileTime,
		CircuitBreakerOpen:   h.circuitBreakerOpen,
	}

	// Check reconcile staleness
	if h.leaderElected && !h.lastReconcileTime.IsZero() {
		status.ReconcileStale = time.Since(h.lastReconcileTime) > h.reconcileStaleTimeout
	}

	if h.lastError != nil {
		status.LastError = h.lastError.Error()
	}

	return status
}

// HealthzHandler implements the /healthz endpoint
// Returns 200 if the controller is healthy (basic liveness check)
func (h *HealthChecker) HealthzHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	healthy := h.healthy
	lastError := h.lastError
	shutdownInitiated := h.shutdownInitiated
	h.mu.RUnlock()

	// During shutdown, still report as healthy (but not ready)
	// This prevents the pod from being killed before graceful shutdown completes
	if shutdownInitiated || healthy {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
		return
	}

	w.WriteHeader(http.StatusServiceUnavailable)
	if lastError != nil {
		fmt.Fprintf(w, "unhealthy: %v", lastError)
	} else {
		fmt.Fprint(w, "unhealthy")
	}
}

// ReadyzHandler implements the /readyz endpoint
// Returns 200 if the controller is ready to handle requests (readiness check)
func (h *HealthChecker) ReadyzHandler(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	ready := h.ready
	lastError := h.lastError
	lastCheck := h.lastCheck
	shutdownInitiated := h.shutdownInitiated
	h.mu.RUnlock()

	// During shutdown, report as not ready
	if shutdownInitiated {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "shutting down")
		return
	}

	if ready {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "ready (last check: %s)", lastCheck.Format(time.RFC3339))
		return
	}

	w.WriteHeader(http.StatusServiceUnavailable)
	if lastError != nil {
		fmt.Fprintf(w, "not ready: %v", lastError)
	} else {
		fmt.Fprint(w, "not ready")
	}
}

// SetReady manually sets the ready status
func (h *HealthChecker) SetReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = ready
}

// SetHealthy manually sets the healthy status
func (h *HealthChecker) SetHealthy(healthy bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.healthy = healthy
}

// IsHealthy returns the current health status
func (h *HealthChecker) IsHealthy() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.healthy
}

// IsReady returns the current readiness status
func (h *HealthChecker) IsReady() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ready
}

// LastError returns the last error encountered during health checks
func (h *HealthChecker) LastError() error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastError
}

// LastCheckTime returns the time of the last health check
func (h *HealthChecker) LastCheckTime() time.Time {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.lastCheck
}
