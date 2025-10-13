package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/vpsie/client"
)

func TestNewHealthChecker(t *testing.T) {
	// Create a mock client (nil is acceptable for this test)
	var vpsieClient *client.Client

	hc := NewHealthChecker(vpsieClient)

	assert.NotNil(t, hc)
	assert.False(t, hc.healthy)
	assert.False(t, hc.ready)
	assert.Equal(t, 30*time.Second, hc.checkInterval)
}

func TestHealthChecker_SetHealthy(t *testing.T) {
	hc := NewHealthChecker(nil)

	assert.False(t, hc.IsHealthy())

	hc.SetHealthy(true)
	assert.True(t, hc.IsHealthy())

	hc.SetHealthy(false)
	assert.False(t, hc.IsHealthy())
}

func TestHealthChecker_SetReady(t *testing.T) {
	hc := NewHealthChecker(nil)

	assert.False(t, hc.IsReady())

	hc.SetReady(true)
	assert.True(t, hc.IsReady())

	hc.SetReady(false)
	assert.False(t, hc.IsReady())
}

func TestHealthChecker_HealthzHandler(t *testing.T) {
	tests := []struct {
		name                 string
		healthy              bool
		lastError            error
		shutdownInitiated    bool
		expectedStatus       int
		expectedBodyContains string
	}{
		{
			name:                 "healthy",
			healthy:              true,
			lastError:            nil,
			shutdownInitiated:    false,
			expectedStatus:       http.StatusOK,
			expectedBodyContains: "ok",
		},
		{
			name:                 "unhealthy without error",
			healthy:              false,
			lastError:            nil,
			shutdownInitiated:    false,
			expectedStatus:       http.StatusServiceUnavailable,
			expectedBodyContains: "unhealthy",
		},
		{
			name:                 "unhealthy with error",
			healthy:              false,
			lastError:            errors.New("API connection failed"),
			shutdownInitiated:    false,
			expectedStatus:       http.StatusServiceUnavailable,
			expectedBodyContains: "API connection failed",
		},
		{
			name:                 "shutdown initiated",
			healthy:              false,
			lastError:            nil,
			shutdownInitiated:    true,
			expectedStatus:       http.StatusOK,
			expectedBodyContains: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := NewHealthChecker(nil)
			hc.SetHealthy(tt.healthy)
			hc.mu.Lock()
			hc.lastError = tt.lastError
			hc.shutdownInitiated = tt.shutdownInitiated
			hc.mu.Unlock()

			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			w := httptest.NewRecorder()

			hc.HealthzHandler(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedBodyContains)
		})
	}
}

func TestHealthChecker_ReadyzHandler(t *testing.T) {
	tests := []struct {
		name                 string
		ready                bool
		lastError            error
		shutdownInitiated    bool
		expectedStatus       int
		expectedBodyContains string
	}{
		{
			name:                 "ready",
			ready:                true,
			lastError:            nil,
			shutdownInitiated:    false,
			expectedStatus:       http.StatusOK,
			expectedBodyContains: "ready",
		},
		{
			name:                 "not ready without error",
			ready:                false,
			lastError:            nil,
			shutdownInitiated:    false,
			expectedStatus:       http.StatusServiceUnavailable,
			expectedBodyContains: "not ready",
		},
		{
			name:                 "not ready with error",
			ready:                false,
			lastError:            errors.New("initialization failed"),
			shutdownInitiated:    false,
			expectedStatus:       http.StatusServiceUnavailable,
			expectedBodyContains: "initialization failed",
		},
		{
			name:                 "shutdown initiated",
			ready:                true,
			lastError:            nil,
			shutdownInitiated:    true,
			expectedStatus:       http.StatusServiceUnavailable,
			expectedBodyContains: "shutting down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hc := NewHealthChecker(nil)
			hc.SetReady(tt.ready)
			hc.mu.Lock()
			hc.lastError = tt.lastError
			hc.shutdownInitiated = tt.shutdownInitiated
			hc.lastCheck = time.Now()
			hc.mu.Unlock()

			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			w := httptest.NewRecorder()

			hc.ReadyzHandler(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.Contains(t, w.Body.String(), tt.expectedBodyContains)
		})
	}
}

func TestHealthChecker_LastError(t *testing.T) {
	hc := NewHealthChecker(nil)

	assert.Nil(t, hc.LastError())

	testErr := errors.New("test error")
	hc.mu.Lock()
	hc.lastError = testErr
	hc.mu.Unlock()

	assert.Equal(t, testErr, hc.LastError())
}

func TestHealthChecker_LastCheckTime(t *testing.T) {
	hc := NewHealthChecker(nil)

	// Initially zero
	assert.True(t, hc.LastCheckTime().IsZero())

	now := time.Now()
	hc.mu.Lock()
	hc.lastCheck = now
	hc.mu.Unlock()

	assert.Equal(t, now, hc.LastCheckTime())
}

func TestHealthChecker_ConcurrentAccess(t *testing.T) {
	// Test concurrent reads and writes to ensure thread safety
	hc := NewHealthChecker(nil)

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			hc.SetHealthy(i%2 == 0)
			hc.SetReady(i%2 == 1)
			time.Sleep(time.Microsecond)
		}
		done <- true
	}()

	// Reader goroutines
	for j := 0; j < 3; j++ {
		go func() {
			for i := 0; i < 100; i++ {
				_ = hc.IsHealthy()
				_ = hc.IsReady()
				_ = hc.LastError()
				_ = hc.LastCheckTime()
				time.Sleep(time.Microsecond)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 4; i++ {
		<-done
	}
}

func TestHealthChecker_PerformCheck_Timeout(t *testing.T) {
	// Test that performCheck respects context timeout
	// Skip this test as it requires a real VPSie client
	// In practice, the VPSie client will handle context cancellation
	t.Skip("Skipping test that requires real VPSie client")
}

func TestHealthChecker_StatusTransitions(t *testing.T) {
	// Test various state transitions
	hc := NewHealthChecker(nil)

	// Initial state
	assert.False(t, hc.IsHealthy())
	assert.False(t, hc.IsReady())

	// Become healthy and ready
	hc.SetHealthy(true)
	hc.SetReady(true)
	assert.True(t, hc.IsHealthy())
	assert.True(t, hc.IsReady())

	// Become unhealthy but still ready
	hc.SetHealthy(false)
	assert.False(t, hc.IsHealthy())
	assert.True(t, hc.IsReady())

	// Become not ready
	hc.SetReady(false)
	assert.False(t, hc.IsHealthy())
	assert.False(t, hc.IsReady())

	// Recover
	hc.SetHealthy(true)
	hc.SetReady(true)
	assert.True(t, hc.IsHealthy())
	assert.True(t, hc.IsReady())
}
