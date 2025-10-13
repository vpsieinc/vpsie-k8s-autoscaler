package controller

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

// HealthChecker provides health checking functionality for the controller
type HealthChecker struct {
	vpsieClient       *client.Client
	mu                sync.RWMutex
	healthy           bool
	ready             bool
	lastCheck         time.Time
	lastError         error
	checkInterval     time.Duration
	shutdownInitiated bool
}

// NewHealthChecker creates a new HealthChecker
func NewHealthChecker(vpsieClient *client.Client) *HealthChecker {
	return &HealthChecker{
		vpsieClient:   vpsieClient,
		healthy:       false,
		ready:         false,
		checkInterval: 30 * time.Second,
	}
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

// performCheck performs a health check against the VPSie API
func (h *HealthChecker) performCheck(ctx context.Context) error {
	checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Try to list VMs to verify API connectivity
	_, err := h.vpsieClient.ListVMs(checkCtx)
	if err != nil {
		h.mu.Lock()
		h.lastCheck = time.Now()
		h.lastError = err
		h.mu.Unlock()
		return fmt.Errorf("VPSie API check failed: %w", err)
	}

	h.mu.Lock()
	h.lastCheck = time.Now()
	h.lastError = nil
	h.mu.Unlock()

	return nil
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
