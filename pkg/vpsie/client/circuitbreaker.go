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
}

// DefaultCircuitBreakerConfig returns the default circuit breaker configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    5,                // Open after 5 consecutive failures
		SuccessThreshold:    2,                // Close after 2 consecutive successes
		Timeout:             30 * time.Second, // Wait 30s before trying half-open
		MaxHalfOpenRequests: 1,                // Allow 1 request at a time when half-open
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
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig, logger *zap.Logger) *CircuitBreaker {
	cb := &CircuitBreaker{
		config:          config,
		state:           StateClosed,
		lastStateChange: time.Now(),
		logger:          logger,
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

	switch cb.state {
	case StateClosed:
		// Allow the call
		return nil

	case StateOpen:
		// Check if timeout has elapsed to transition to half-open
		if now.Sub(cb.lastStateChange) >= cb.config.Timeout {
			cb.transitionTo(StateHalfOpen, "timeout elapsed")
			return nil
		}
		// Circuit is still open
		metrics.VPSieAPICircuitBreakerOpened.WithLabelValues().Inc()
		return ErrCircuitOpen

	case StateHalfOpen:
		// Check if we can allow another request
		if cb.halfOpenRequests >= cb.config.MaxHalfOpenRequests {
			return ErrCircuitOpen
		}
		cb.halfOpenRequests++
		return nil

	default:
		return fmt.Errorf("unknown circuit breaker state: %s", cb.state)
	}
}

// afterCall records the result and potentially transitions state
func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		if err != nil {
			cb.failureCount++
			cb.successCount = 0

			if cb.failureCount >= cb.config.FailureThreshold {
				cb.transitionTo(StateOpen, fmt.Sprintf("failure threshold reached (%d failures)", cb.failureCount))
			}
		} else {
			cb.failureCount = 0
			cb.successCount++
		}

	case StateHalfOpen:
		cb.halfOpenRequests--

		if err != nil {
			// Failure in half-open state -> back to open
			cb.failureCount++
			cb.successCount = 0
			cb.transitionTo(StateOpen, "failure in half-open state")
		} else {
			// Success in half-open state
			cb.failureCount = 0
			cb.successCount++

			if cb.successCount >= cb.config.SuccessThreshold {
				cb.transitionTo(StateClosed, fmt.Sprintf("success threshold reached (%d successes)", cb.successCount))
			}
		}

	case StateOpen:
		// Shouldn't get here, but handle it
		cb.logger.Warn("afterCall called in open state (should not happen)")
	}
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

	return CircuitBreakerStats{
		State:            cb.state,
		FailureCount:     cb.failureCount,
		SuccessCount:     cb.successCount,
		LastStateChange:  cb.lastStateChange,
		HalfOpenRequests: cb.halfOpenRequests,
	}
}

// CircuitBreakerStats represents circuit breaker statistics
type CircuitBreakerStats struct {
	State            CircuitBreakerState
	FailureCount     int
	SuccessCount     int
	LastStateChange  time.Time
	HalfOpenRequests int
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
