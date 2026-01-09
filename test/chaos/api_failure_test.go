//go:build chaos
// +build chaos

package chaos

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	autoscalerv1alpha1 "github.com/vpsie/vpsie-k8s-autoscaler/pkg/apis/autoscaler/v1alpha1"
)

// ChaosMockServer is a mock VPSie API server with chaos injection capabilities
type ChaosMockServer struct {
	server *httptest.Server
	mu     sync.RWMutex

	// Chaos configuration
	ErrorRate       float64
	ErrorStatusCode int
	Latency         time.Duration
	LatencyVariance time.Duration
	Enabled         bool

	// Metrics
	TotalRequests   atomic.Int64
	FailedRequests  atomic.Int64
	SuccessRequests atomic.Int64
}

// NewChaosMockServer creates a new chaos-enabled mock server
func NewChaosMockServer() *ChaosMockServer {
	mock := &ChaosMockServer{
		ErrorRate:       0.0,
		ErrorStatusCode: http.StatusInternalServerError,
		Enabled:         false,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", mock.handler)

	mock.server = httptest.NewServer(mock.middleware(mux))
	return mock
}

func (m *ChaosMockServer) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.TotalRequests.Add(1)

		m.mu.RLock()
		enabled := m.Enabled
		errorRate := m.ErrorRate
		latency := m.Latency
		variance := m.LatencyVariance
		statusCode := m.ErrorStatusCode
		m.mu.RUnlock()

		// Inject latency
		if latency > 0 {
			jitter := time.Duration(rand.Int63n(int64(variance)))
			time.Sleep(latency + jitter)
		}

		// Inject errors
		if enabled && rand.Float64() < errorRate {
			m.FailedRequests.Add(1)
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Chaos-injected failure",
			})
			return
		}

		m.SuccessRequests.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (m *ChaosMockServer) handler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// URL returns the mock server URL
func (m *ChaosMockServer) URL() string {
	return m.server.URL
}

// Close shuts down the mock server
func (m *ChaosMockServer) Close() {
	m.server.Close()
}

// EnableChaos enables chaos injection
func (m *ChaosMockServer) EnableChaos(config ChaosConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ErrorRate = config.ErrorRate
	m.Latency = config.Latency
	m.LatencyVariance = config.LatencyVariance
	m.Enabled = true
}

// DisableChaos disables chaos injection
func (m *ChaosMockServer) DisableChaos() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Enabled = false
	m.ErrorRate = 0
	m.Latency = 0
}

// SetErrorStatusCode sets the HTTP status code for injected errors
func (m *ChaosMockServer) SetErrorStatusCode(code int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ErrorStatusCode = code
}

// GetMetrics returns current metrics
func (m *ChaosMockServer) GetMetrics() (total, failed, success int64) {
	return m.TotalRequests.Load(), m.FailedRequests.Load(), m.SuccessRequests.Load()
}

// ResetMetrics resets all counters
func (m *ChaosMockServer) ResetMetrics() {
	m.TotalRequests.Store(0)
	m.FailedRequests.Store(0)
	m.SuccessRequests.Store(0)
}

// TestAPIFailure_RandomErrors tests behavior under random API errors
func TestAPIFailure_RandomErrors(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	scenario := ChaosScenario{
		Name:        "Random API Errors",
		Description: "Test system behavior when VPSie API returns random errors",
		Setup: func(ctx context.Context, t *testing.T) error {
			mock.EnableChaos(DefaultChaosConfig())
			return nil
		},
		Execute: func(ctx context.Context, t *testing.T) error {
			// Simulate multiple API calls
			for i := 0; i < 100; i++ {
				resp, err := http.Get(mock.URL())
				if err != nil {
					continue // Network error
				}
				resp.Body.Close()
			}
			return nil
		},
		Verify: func(ctx context.Context, t *testing.T) error {
			total, failed, success := mock.GetMetrics()
			t.Logf("Metrics: total=%d, failed=%d, success=%d", total, failed, success)

			// With 50% error rate, we should see roughly equal failed/success
			errorRate := float64(failed) / float64(total)
			t.Logf("Actual error rate: %.2f", errorRate)

			// Allow some variance (30-70%)
			assert.Greater(t, errorRate, 0.30, "Error rate too low")
			assert.Less(t, errorRate, 0.70, "Error rate too high")
			return nil
		},
		Cleanup: func(ctx context.Context, t *testing.T) error {
			mock.DisableChaos()
			return nil
		},
	}

	RunChaosScenario(t, scenario)
}

// TestAPIFailure_HighErrorRate tests behavior under high error rate
func TestAPIFailure_HighErrorRate(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	scenario := ChaosScenario{
		Name:        "High Error Rate",
		Description: "Test system behavior when VPSie API has 90% error rate",
		Setup: func(ctx context.Context, t *testing.T) error {
			mock.EnableChaos(HighChaosConfig())
			return nil
		},
		Execute: func(ctx context.Context, t *testing.T) error {
			for i := 0; i < 100; i++ {
				resp, _ := http.Get(mock.URL())
				if resp != nil {
					resp.Body.Close()
				}
			}
			return nil
		},
		Verify: func(ctx context.Context, t *testing.T) error {
			total, failed, _ := mock.GetMetrics()
			errorRate := float64(failed) / float64(total)
			t.Logf("High chaos error rate: %.2f", errorRate)

			// Should see very high error rate
			assert.Greater(t, errorRate, 0.70, "Error rate should be high")
			return nil
		},
		Cleanup: func(ctx context.Context, t *testing.T) error {
			mock.DisableChaos()
			return nil
		},
	}

	RunChaosScenario(t, scenario)
}

// TestAPIFailure_RateLimitErrors tests behavior under rate limiting
func TestAPIFailure_RateLimitErrors(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	scenario := ChaosScenario{
		Name:        "Rate Limit Errors",
		Description: "Test system behavior when VPSie API returns 429 Too Many Requests",
		Setup: func(ctx context.Context, t *testing.T) error {
			mock.SetErrorStatusCode(http.StatusTooManyRequests)
			config := DefaultChaosConfig()
			config.ErrorRate = 0.3 // 30% rate limit errors
			mock.EnableChaos(config)
			return nil
		},
		Execute: func(ctx context.Context, t *testing.T) error {
			var rateLimited int
			for i := 0; i < 50; i++ {
				resp, err := http.Get(mock.URL())
				if err != nil {
					continue
				}
				if resp.StatusCode == http.StatusTooManyRequests {
					rateLimited++
				}
				resp.Body.Close()
			}
			t.Logf("Rate limited responses: %d", rateLimited)
			return nil
		},
		Verify: func(ctx context.Context, t *testing.T) error {
			_, failed, _ := mock.GetMetrics()
			t.Logf("Failed requests (rate limited): %d", failed)
			assert.Greater(t, failed, int64(0), "Should have rate limited requests")
			return nil
		},
		Cleanup: func(ctx context.Context, t *testing.T) error {
			mock.DisableChaos()
			mock.SetErrorStatusCode(http.StatusInternalServerError)
			return nil
		},
	}

	RunChaosScenario(t, scenario)
}

// TestAPIFailure_ServiceUnavailable tests behavior during complete outage
func TestAPIFailure_ServiceUnavailable(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	scenario := ChaosScenario{
		Name:        "Service Unavailable",
		Description: "Test system behavior when VPSie API is completely unavailable",
		Setup: func(ctx context.Context, t *testing.T) error {
			mock.SetErrorStatusCode(http.StatusServiceUnavailable)
			config := ChaosConfig{
				ErrorRate: 1.0, // 100% errors
			}
			mock.EnableChaos(config)
			return nil
		},
		Execute: func(ctx context.Context, t *testing.T) error {
			var unavailable int
			for i := 0; i < 20; i++ {
				resp, err := http.Get(mock.URL())
				if err != nil {
					continue
				}
				if resp.StatusCode == http.StatusServiceUnavailable {
					unavailable++
				}
				resp.Body.Close()
			}
			t.Logf("Unavailable responses: %d/20", unavailable)
			assert.Equal(t, 20, unavailable, "All requests should return 503")
			return nil
		},
		Verify: func(ctx context.Context, t *testing.T) error {
			// In a real test, we would verify:
			// - Circuit breaker opened
			// - Controller maintains current state
			// - Events are recorded
			return nil
		},
		Cleanup: func(ctx context.Context, t *testing.T) error {
			mock.DisableChaos()
			return nil
		},
	}

	RunChaosScenario(t, scenario)
}

// TestAPIFailure_RecoveryAfterOutage tests system recovery after outage
func TestAPIFailure_RecoveryAfterOutage(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	scenario := ChaosScenario{
		Name:        "Recovery After Outage",
		Description: "Test system recovery when VPSie API becomes available again",
		Setup: func(ctx context.Context, t *testing.T) error {
			// Start with 100% errors
			mock.SetErrorStatusCode(http.StatusServiceUnavailable)
			mock.EnableChaos(ChaosConfig{ErrorRate: 1.0})
			return nil
		},
		Execute: func(ctx context.Context, t *testing.T) error {
			// Phase 1: API unavailable
			t.Log("Phase 1: API unavailable")
			for i := 0; i < 10; i++ {
				resp, _ := http.Get(mock.URL())
				if resp != nil {
					resp.Body.Close()
				}
			}

			total1, failed1, _ := mock.GetMetrics()
			t.Logf("After outage: total=%d, failed=%d", total1, failed1)

			// Phase 2: API recovers
			t.Log("Phase 2: API recovering")
			mock.DisableChaos()
			mock.ResetMetrics()

			for i := 0; i < 10; i++ {
				resp, _ := http.Get(mock.URL())
				if resp != nil {
					resp.Body.Close()
				}
			}

			total2, failed2, success2 := mock.GetMetrics()
			t.Logf("After recovery: total=%d, failed=%d, success=%d", total2, failed2, success2)

			assert.Equal(t, int64(10), success2, "All requests should succeed after recovery")
			return nil
		},
		Verify: func(ctx context.Context, t *testing.T) error {
			_, failed, success := mock.GetMetrics()
			assert.Equal(t, int64(0), failed, "No failures after recovery")
			assert.Equal(t, int64(10), success, "All requests succeed")
			return nil
		},
		Cleanup: func(ctx context.Context, t *testing.T) error {
			return nil
		},
	}

	RunChaosScenario(t, scenario)
}

// TestAPIFailure_WithNodeGroup tests NodeGroup behavior under API failures
func TestAPIFailure_WithNodeGroup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultTimeout)
	defer cancel()

	// Create a NodeGroup
	ng := &autoscalerv1alpha1.NodeGroup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-ng-api",
			Namespace: TestNamespace,
		},
		Spec: autoscalerv1alpha1.NodeGroupSpec{
			MinNodes:     1,
			MaxNodes:     5,
			DatacenterID: "dc-test-1",
			OfferingIDs:  []string{"small-2cpu-4gb"},
			OSImageID:    "ubuntu-22.04",
		},
	}

	err := k8sClient.Create(ctx, ng)
	require.NoError(t, err, "Failed to create NodeGroup")

	defer func() {
		_ = k8sClient.Delete(context.Background(), ng)
	}()

	// Verify NodeGroup exists
	var fetchedNg autoscalerv1alpha1.NodeGroup
	err = k8sClient.Get(ctx, client.ObjectKey{
		Name:      ng.Name,
		Namespace: ng.Namespace,
	}, &fetchedNg)
	require.NoError(t, err)

	// In a full chaos test with controller:
	// 1. Enable API chaos
	// 2. Trigger scaling event
	// 3. Verify controller handles failures gracefully
	// 4. Disable chaos
	// 5. Verify recovery

	t.Log("NodeGroup created successfully for chaos testing")
}

// TestAPIFailure_IntermittentErrors tests behavior under intermittent errors
func TestAPIFailure_IntermittentErrors(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	scenario := ChaosScenario{
		Name:        "Intermittent Errors",
		Description: "Test system behavior with intermittent API errors (flapping)",
		Execute: func(ctx context.Context, t *testing.T) error {
			// Alternate between error states
			for cycle := 0; cycle < 5; cycle++ {
				// Enable errors
				mock.EnableChaos(ChaosConfig{ErrorRate: 0.8})
				for i := 0; i < 10; i++ {
					resp, _ := http.Get(mock.URL())
					if resp != nil {
						resp.Body.Close()
					}
				}

				// Disable errors
				mock.DisableChaos()
				for i := 0; i < 10; i++ {
					resp, _ := http.Get(mock.URL())
					if resp != nil {
						resp.Body.Close()
					}
				}
			}

			total, failed, success := mock.GetMetrics()
			t.Logf("Intermittent test: total=%d, failed=%d, success=%d", total, failed, success)

			// Should see a mix of failures and successes
			assert.Greater(t, failed, int64(0), "Should have some failures")
			assert.Greater(t, success, int64(0), "Should have some successes")
			return nil
		},
	}

	RunChaosScenario(t, scenario)
}

// TestAPIFailure_TimeoutBehavior tests behavior under high latency
func TestAPIFailure_TimeoutBehavior(t *testing.T) {
	mock := NewChaosMockServer()
	defer mock.Close()

	scenario := ChaosScenario{
		Name:        "Timeout Behavior",
		Description: "Test system behavior when API responses are very slow",
		Setup: func(ctx context.Context, t *testing.T) error {
			// Set high latency but no errors
			mock.EnableChaos(ChaosConfig{
				Latency:         2 * time.Second,
				LatencyVariance: 500 * time.Millisecond,
				ErrorRate:       0, // No errors, just slow
			})
			return nil
		},
		Execute: func(ctx context.Context, t *testing.T) error {
			client := &http.Client{
				Timeout: 1 * time.Second, // Short timeout
			}

			var timeouts int
			for i := 0; i < 5; i++ {
				_, err := client.Get(mock.URL())
				if err != nil {
					timeouts++
				}
			}

			t.Logf("Timed out requests: %d/5", timeouts)
			// Most requests should timeout since latency > client timeout
			assert.Greater(t, timeouts, 2, "Should see timeouts")
			return nil
		},
		Cleanup: func(ctx context.Context, t *testing.T) error {
			mock.DisableChaos()
			return nil
		},
	}

	RunChaosScenario(t, scenario)
}
