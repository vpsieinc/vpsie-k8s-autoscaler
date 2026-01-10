package client

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/vpsie/vpsie-k8s-autoscaler/pkg/metrics"
)

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// CircuitBreakerState represents the state of the circuit breaker
type CircuitBreakerState string

const (
	// StateClosed means requests are allowed through
	StateClosed CircuitBreakerState = "closed"

	// StateOpen means requests are blocked
	StateOpen CircuitBreakerState = "open"

	// StateHalfOpen means limited requests are allowed for testing
	StateHalfOpen CircuitBreakerState = "half-open"
)

// CircuitBreakerConfig configures the circuit breaker behavior
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures before opening
	FailureThreshold int

	// SuccessThreshold is the number of consecutive successes to close from half-open
	SuccessThreshold int

	// Timeout is how long to wait in open state before trying half-open
	Timeout time.Duration

	// MaxHalfOpenRequests is the max concurrent requests allowed in half-open state
	MaxHalfOpenRequests int

	// FailureRateThreshold is the minimum failure rate (0.0-1.0) to trigger opening (optional)
	// When set > 0, both FailureThreshold AND FailureRateThreshold must be exceeded
	FailureRateThreshold float64

	// SlidingWindowSize is the size of the sliding window for failure rate calculation
	// When 0, only consecutive failures are counted
	SlidingWindowSize int

	// OnStateChange is an optional callback when state changes
	OnStateChange func(from, to CircuitBreakerState, reason string)
}

// DefaultCircuitBreakerConfig returns the default circuit breaker configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:     5,                // Open after 5 consecutive failures
		SuccessThreshold:     2,                // Close after 2 consecutive successes
		Timeout:              30 * time.Second, // Wait 30s before trying half-open
		MaxHalfOpenRequests:  1,                // Allow 1 request at a time when half-open
		FailureRateThreshold: 0,                // Disabled by default (use consecutive failures only)
		SlidingWindowSize:    0,                // No sliding window by default
	}
}

// CircuitBreaker implements the circuit breaker pattern for API calls
type CircuitBreaker struct {
	config           CircuitBreakerConfig
	state            CircuitBreakerState
	failureCount     int
	successCount     int
	lastStateChange  time.Time
	halfOpenRequests int
	logger           *zap.Logger
	mu               sync.RWMutex

	// Enhanced metrics tracking
	totalRequests         int64         // Total requests since creation
	totalFailures         int64         // Total failures since creation
	totalSuccesses        int64         // Total successes since creation
	consecutiveFailures   int           // Current consecutive failures
	openDurationTotal     time.Duration // Total time spent in open state
	halfOpenDurationTotal time.Duration // Total time spent in half-open state
	lastOpenTime          time.Time     // When circuit last opened
	lastHalfOpenTime      time.Time     // When circuit last entered half-open
	halfOpenAttempts      int64         // Number of half-open test requests
	halfOpenSuccesses     int64         // Number of successful half-open requests
	halfOpenFailures      int64         // Number of failed half-open requests

	// Sliding window for failure rate calculation
	slidingWindow []bool // true = success, false = failure
	windowIndex   int
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig, logger *zap.Logger) *CircuitBreaker {
	cb := &CircuitBreaker{
		config:          config,
		state:           StateClosed,
		lastStateChange: time.Now(),
		logger:          logger,
	}

	// Initialize sliding window if configured
	if config.SlidingWindowSize > 0 {
		cb.slidingWindow = make([]bool, config.SlidingWindowSize)
		// Pre-fill with successes to avoid false positives on startup
		for i := range cb.slidingWindow {
			cb.slidingWindow[i] = true
		}
	}

	// Initialize metrics
	metrics.VPSieAPICircuitBreakerState.WithLabelValues(string(StateClosed)).Set(1)
	metrics.VPSieAPICircuitBreakerState.WithLabelValues(string(StateOpen)).Set(0)
	metrics.VPSieAPICircuitBreakerState.WithLabelValues(string(StateHalfOpen)).Set(0)

	return cb
}

// Call executes a function with circuit breaker protection
func (cb *CircuitBreaker) Call(fn func() error) error {
	// Check if we can make the call
	if err := cb.beforeCall(); err != nil {
		return err
	}

	// Execute the function
	err := fn()

	// Record the result
	cb.afterCall(err)

	return err
}

// beforeCall checks if the call is allowed and updates state if needed
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	cb.totalRequests++

	switch cb.state {
	case StateClosed:
		// Allow the call
		return nil

	case StateOpen:
		// Check if timeout has elapsed to transition to half-open
		if now.Sub(cb.lastStateChange) >= cb.config.Timeout {
			// Track time spent in open state
			cb.openDurationTotal += now.Sub(cb.lastOpenTime)
			cb.transitionTo(StateHalfOpen, "timeout elapsed")
			cb.lastHalfOpenTime = now
			return nil
		}
		// Circuit is still open
		metrics.VPSieAPICircuitBreakerOpened.WithLabelValues().Inc()
		return ErrCircuitOpen

	case StateHalfOpen:
		// Check if we can allow another request
		if cb.halfOpenRequests >= cb.config.MaxHalfOpenRequests {
			metrics.VPSieAPICircuitBreakerHalfOpenRejected.Inc()
			return ErrCircuitOpen
		}
		cb.halfOpenRequests++
		cb.halfOpenAttempts++
		metrics.VPSieAPICircuitBreakerHalfOpenAttempts.Inc()
		return nil

	default:
		return fmt.Errorf("unknown circuit breaker state: %s", cb.state)
	}
}

// afterCall records the result and potentially transitions state
func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	// Update sliding window if configured
	if len(cb.slidingWindow) > 0 {
		cb.slidingWindow[cb.windowIndex] = (err == nil)
		cb.windowIndex = (cb.windowIndex + 1) % len(cb.slidingWindow)
	}

	switch cb.state {
	case StateClosed:
		if err != nil {
			cb.failureCount++
			cb.consecutiveFailures++
			cb.successCount = 0
			cb.totalFailures++

			shouldOpen := cb.failureCount >= cb.config.FailureThreshold

			// Check failure rate threshold if configured
			if shouldOpen && cb.config.FailureRateThreshold > 0 && len(cb.slidingWindow) > 0 {
				failureRate := cb.calculateFailureRate()
				shouldOpen = failureRate >= cb.config.FailureRateThreshold
				if shouldOpen {
					cb.transitionTo(StateOpen, fmt.Sprintf("failure threshold reached (%d failures, %.1f%% failure rate)", cb.failureCount, failureRate*100))
					cb.lastOpenTime = now
					return
				}
			}

			if shouldOpen {
				cb.transitionTo(StateOpen, fmt.Sprintf("failure threshold reached (%d failures)", cb.failureCount))
				cb.lastOpenTime = now
			}
		} else {
			cb.failureCount = 0
			cb.consecutiveFailures = 0
			cb.successCount++
			cb.totalSuccesses++
		}

	case StateHalfOpen:
		cb.halfOpenRequests--

		if err != nil {
			// Failure in half-open state -> back to open
			cb.failureCount++
			cb.consecutiveFailures++
			cb.successCount = 0
			cb.totalFailures++
			cb.halfOpenFailures++
			metrics.VPSieAPICircuitBreakerHalfOpenFailures.Inc()

			// Track time spent in half-open state
			cb.halfOpenDurationTotal += now.Sub(cb.lastHalfOpenTime)
			cb.transitionTo(StateOpen, "failure in half-open state")
			cb.lastOpenTime = now
		} else {
			// Success in half-open state
			cb.failureCount = 0
			cb.consecutiveFailures = 0
			cb.successCount++
			cb.totalSuccesses++
			cb.halfOpenSuccesses++
			metrics.VPSieAPICircuitBreakerHalfOpenSuccesses.Inc()

			if cb.successCount >= cb.config.SuccessThreshold {
				// Track time spent in half-open state
				cb.halfOpenDurationTotal += now.Sub(cb.lastHalfOpenTime)
				cb.transitionTo(StateClosed, fmt.Sprintf("success threshold reached (%d successes)", cb.successCount))
			}
		}

	case StateOpen:
		// Shouldn't get here, but handle it
		cb.logger.Warn("afterCall called in open state (should not happen)")
	}
}

// calculateFailureRate calculates the failure rate from the sliding window
func (cb *CircuitBreaker) calculateFailureRate() float64 {
	if len(cb.slidingWindow) == 0 {
		return 0
	}

	failures := 0
	for _, success := range cb.slidingWindow {
		if !success {
			failures++
		}
	}
	return float64(failures) / float64(len(cb.slidingWindow))
}

// transitionTo changes the circuit breaker state
func (cb *CircuitBreaker) transitionTo(newState CircuitBreakerState, reason string) {
	oldState := cb.state

	// Reset counters on state change
	if newState != oldState {
		cb.state = newState
		cb.lastStateChange = time.Now()
		cb.failureCount = 0
		cb.successCount = 0
		cb.halfOpenRequests = 0

		// Update metrics
		metrics.VPSieAPICircuitBreakerState.WithLabelValues(string(oldState)).Set(0)
		metrics.VPSieAPICircuitBreakerState.WithLabelValues(string(newState)).Set(1)
		metrics.VPSieAPICircuitBreakerStateChanges.WithLabelValues(string(oldState), string(newState)).Inc()

		cb.logger.Info("circuit breaker state changed",
			zap.String("from", string(oldState)),
			zap.String("to", string(newState)),
			zap.String("reason", reason))

		// Call state change callback if configured
		if cb.config.OnStateChange != nil {
			// Call async to avoid blocking
			go cb.config.OnStateChange(oldState, newState, reason)
		}
	}
}

// GetState returns the current circuit breaker state (for testing/monitoring)
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns current circuit breaker statistics
func (cb *CircuitBreaker) GetStats() CircuitBreakerStats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	var failureRate float64
	if len(cb.slidingWindow) > 0 {
		failureRate = cb.calculateFailureRateUnlocked()
	}

	return CircuitBreakerStats{
		State:                 cb.state,
		FailureCount:          cb.failureCount,
		SuccessCount:          cb.successCount,
		LastStateChange:       cb.lastStateChange,
		HalfOpenRequests:      cb.halfOpenRequests,
		TotalRequests:         cb.totalRequests,
		TotalFailures:         cb.totalFailures,
		TotalSuccesses:        cb.totalSuccesses,
		ConsecutiveFailures:   cb.consecutiveFailures,
		OpenDurationTotal:     cb.openDurationTotal,
		HalfOpenDurationTotal: cb.halfOpenDurationTotal,
		HalfOpenAttempts:      cb.halfOpenAttempts,
		HalfOpenSuccesses:     cb.halfOpenSuccesses,
		HalfOpenFailures:      cb.halfOpenFailures,
		FailureRate:           failureRate,
	}
}

// calculateFailureRateUnlocked calculates failure rate without acquiring lock (caller must hold lock)
func (cb *CircuitBreaker) calculateFailureRateUnlocked() float64 {
	if len(cb.slidingWindow) == 0 {
		return 0
	}

	failures := 0
	for _, success := range cb.slidingWindow {
		if !success {
			failures++
		}
	}
	return float64(failures) / float64(len(cb.slidingWindow))
}

// CircuitBreakerStats represents circuit breaker statistics
type CircuitBreakerStats struct {
	State                 CircuitBreakerState
	FailureCount          int
	SuccessCount          int
	LastStateChange       time.Time
	HalfOpenRequests      int
	TotalRequests         int64
	TotalFailures         int64
	TotalSuccesses        int64
	ConsecutiveFailures   int
	OpenDurationTotal     time.Duration
	HalfOpenDurationTotal time.Duration
	HalfOpenAttempts      int64
	HalfOpenSuccesses     int64
	HalfOpenFailures      int64
	FailureRate           float64
}

// Reset resets the circuit breaker to closed state (for testing)
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state
	cb.state = StateClosed
	cb.failureCount = 0
	cb.successCount = 0
	cb.halfOpenRequests = 0
	cb.lastStateChange = time.Now()

	// Update metrics
	if oldState != StateClosed {
		metrics.VPSieAPICircuitBreakerState.WithLabelValues(string(oldState)).Set(0)
		metrics.VPSieAPICircuitBreakerState.WithLabelValues(string(StateClosed)).Set(1)
	}
}
