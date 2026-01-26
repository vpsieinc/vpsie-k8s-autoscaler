package client

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func TestNewCircuitBreaker(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()

	cb := NewCircuitBreaker(config, logger)

	if cb == nil {
		t.Fatal("expected circuit breaker to be created")
	}

	if cb.state != StateClosed {
		t.Errorf("expected initial state to be closed, got %s", cb.state)
	}

	if cb.config.FailureThreshold != 5 {
		t.Errorf("expected failure threshold 5, got %d", cb.config.FailureThreshold)
	}
}

func TestCircuitBreaker_SuccessfulCalls(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	cb := NewCircuitBreaker(config, logger)

	// Make successful calls
	for i := 0; i < 10; i++ {
		err := cb.Call(func() error {
			return nil
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	}

	// Circuit should remain closed
	if cb.GetState() != StateClosed {
		t.Errorf("expected state to be closed, got %s", cb.GetState())
	}

	stats := cb.GetStats()
	if stats.TotalSuccesses != 10 {
		t.Errorf("expected 10 successes, got %d", stats.TotalSuccesses)
	}
}

func TestCircuitBreaker_StateTransition_ClosedToOpen(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 3
	cb := NewCircuitBreaker(config, logger)

	testErr := errors.New("test error")

	// Make failing calls up to threshold
	for i := 0; i < 3; i++ {
		_ = cb.Call(func() error {
			return testErr
		})
	}

	// Circuit should now be open
	if cb.GetState() != StateOpen {
		t.Errorf("expected state to be open, got %s", cb.GetState())
	}

	// Calls should be rejected
	err := cb.Call(func() error {
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen, got %v", err)
	}
}

func TestCircuitBreaker_StateTransition_OpenToHalfOpen(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.Timeout = 50 * time.Millisecond
	cb := NewCircuitBreaker(config, logger)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		_ = cb.Call(func() error {
			return testErr
		})
	}

	if cb.GetState() != StateOpen {
		t.Fatalf("expected state to be open, got %s", cb.GetState())
	}

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	// Next call should transition to half-open and execute
	err := cb.Call(func() error {
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// State should be half-open or closed depending on success threshold
	state := cb.GetState()
	if state != StateHalfOpen && state != StateClosed {
		t.Errorf("expected state to be half-open or closed, got %s", state)
	}
}

func TestCircuitBreaker_StateTransition_HalfOpenToClosed(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.SuccessThreshold = 2
	config.Timeout = 50 * time.Millisecond
	config.MaxHalfOpenRequests = 10
	cb := NewCircuitBreaker(config, logger)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		_ = cb.Call(func() error {
			return testErr
		})
	}

	// Wait for timeout to transition to half-open
	time.Sleep(100 * time.Millisecond)

	// Make successful calls to reach success threshold
	for i := 0; i < 2; i++ {
		err := cb.Call(func() error {
			return nil
		})
		if err != nil {
			t.Errorf("expected no error in half-open state, got %v", err)
		}
	}

	// Circuit should now be closed
	if cb.GetState() != StateClosed {
		t.Errorf("expected state to be closed, got %s", cb.GetState())
	}
}

func TestCircuitBreaker_StateTransition_HalfOpenToOpen(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.Timeout = 50 * time.Millisecond
	cb := NewCircuitBreaker(config, logger)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		_ = cb.Call(func() error {
			return testErr
		})
	}

	// Wait for timeout to transition to half-open
	time.Sleep(100 * time.Millisecond)

	// Fail in half-open state
	_ = cb.Call(func() error {
		return testErr
	})

	// Circuit should be back to open
	if cb.GetState() != StateOpen {
		t.Errorf("expected state to be open, got %s", cb.GetState())
	}
}

func TestCircuitBreaker_MaxHalfOpenRequests(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.Timeout = 50 * time.Millisecond
	config.MaxHalfOpenRequests = 1
	cb := NewCircuitBreaker(config, logger)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		_ = cb.Call(func() error {
			return testErr
		})
	}

	// Wait for timeout to allow transition to half-open
	time.Sleep(100 * time.Millisecond)

	// First call after timeout triggers the transition from Open to HalfOpen
	// This call goes through without consuming the half-open quota
	// We need to fail it to keep the circuit open and try again
	_ = cb.Call(func() error {
		return testErr // Fail to go back to open
	})

	// Wait for another timeout to allow transition to half-open again
	time.Sleep(100 * time.Millisecond)

	// Now trigger transition and immediately start blocking call
	callStarted := make(chan struct{})
	done := make(chan struct{})

	// This call triggers transition from Open to HalfOpen and goes through
	go func() {
		_ = cb.Call(func() error {
			close(callStarted)
			<-done
			return nil
		})
	}()

	<-callStarted

	// Now we're in half-open state, but the first (transition) call doesn't count
	// The next call will be the first real half-open request
	call2Started := make(chan struct{})
	done2 := make(chan struct{})

	go func() {
		_ = cb.Call(func() error {
			close(call2Started)
			<-done2
			return nil
		})
	}()

	<-call2Started

	// Now we have 1 half-open request in flight, the next should be rejected
	err := cb.Call(func() error {
		return nil
	})

	close(done)
	close(done2)

	if !errors.Is(err, ErrCircuitOpen) {
		t.Errorf("expected ErrCircuitOpen when max half-open requests reached, got %v", err)
	}
}

func TestCircuitBreaker_ConcurrentCalls(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 100 // High threshold to avoid opening
	cb := NewCircuitBreaker(config, logger)

	var wg sync.WaitGroup
	numGoroutines := 100
	callsPerGoroutine := 100

	var successCount int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				err := cb.Call(func() error {
					return nil
				})
				if err == nil {
					atomic.AddInt64(&successCount, 1)
				}
			}
		}()
	}

	wg.Wait()

	expectedTotal := int64(numGoroutines * callsPerGoroutine)
	if successCount != expectedTotal {
		t.Errorf("expected %d successful calls, got %d", expectedTotal, successCount)
	}

	stats := cb.GetStats()
	if stats.TotalRequests != expectedTotal {
		t.Errorf("expected %d total requests, got %d", expectedTotal, stats.TotalRequests)
	}
}

func TestCircuitBreaker_ConcurrentFailures(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 5
	cb := NewCircuitBreaker(config, logger)

	var wg sync.WaitGroup
	numGoroutines := 10

	testErr := errors.New("test error")

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cb.Call(func() error {
				return testErr
			})
		}()
	}

	wg.Wait()

	// Circuit should be open after enough failures
	state := cb.GetState()
	if state != StateOpen {
		t.Errorf("expected state to be open after concurrent failures, got %s", state)
	}
}

func TestCircuitBreaker_SlidingWindowFailureRate(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 3
	config.FailureRateThreshold = 0.5 // 50% failure rate
	config.SlidingWindowSize = 10
	cb := NewCircuitBreaker(config, logger)

	testErr := errors.New("test error")

	// Make 5 successful calls to fill the window partially
	for i := 0; i < 5; i++ {
		_ = cb.Call(func() error {
			return nil
		})
	}

	// Make 3 failures - this should trigger open state
	// because failure rate will exceed threshold AND failure count >= threshold
	for i := 0; i < 5; i++ {
		_ = cb.Call(func() error {
			return testErr
		})
	}

	// Circuit should be open now
	if cb.GetState() != StateOpen {
		t.Errorf("expected state to be open with high failure rate, got %s", cb.GetState())
	}
}

func TestCircuitBreaker_OnStateChangeCallback(t *testing.T) {
	logger := zaptest.NewLogger(t)

	var mu sync.Mutex
	stateChanges := []struct {
		from   CircuitBreakerState
		to     CircuitBreakerState
		reason string
	}{}

	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.OnStateChange = func(from, to CircuitBreakerState, reason string) {
		mu.Lock()
		defer mu.Unlock()
		stateChanges = append(stateChanges, struct {
			from   CircuitBreakerState
			to     CircuitBreakerState
			reason string
		}{from, to, reason})
	}

	cb := NewCircuitBreaker(config, logger)

	testErr := errors.New("test error")

	// Trigger state change to open
	for i := 0; i < 2; i++ {
		_ = cb.Call(func() error {
			return testErr
		})
	}

	// Give async callback time to execute
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(stateChanges) != 1 {
		t.Errorf("expected 1 state change, got %d", len(stateChanges))
	}

	if len(stateChanges) > 0 {
		if stateChanges[0].from != StateClosed {
			t.Errorf("expected from state to be closed, got %s", stateChanges[0].from)
		}
		if stateChanges[0].to != StateOpen {
			t.Errorf("expected to state to be open, got %s", stateChanges[0].to)
		}
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	cb := NewCircuitBreaker(config, logger)

	testErr := errors.New("test error")

	// Trigger open state
	for i := 0; i < 2; i++ {
		_ = cb.Call(func() error {
			return testErr
		})
	}

	if cb.GetState() != StateOpen {
		t.Fatalf("expected state to be open, got %s", cb.GetState())
	}

	// Reset the circuit breaker
	cb.Reset()

	if cb.GetState() != StateClosed {
		t.Errorf("expected state to be closed after reset, got %s", cb.GetState())
	}

	// Should be able to make calls again
	err := cb.Call(func() error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}

func TestCircuitBreaker_GetStats(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 10 // High to avoid opening
	cb := NewCircuitBreaker(config, logger)

	testErr := errors.New("test error")

	// Make some successful and failed calls
	for i := 0; i < 5; i++ {
		_ = cb.Call(func() error {
			return nil
		})
	}
	for i := 0; i < 3; i++ {
		_ = cb.Call(func() error {
			return testErr
		})
	}

	stats := cb.GetStats()

	if stats.TotalRequests != 8 {
		t.Errorf("expected 8 total requests, got %d", stats.TotalRequests)
	}
	if stats.TotalSuccesses != 5 {
		t.Errorf("expected 5 successes, got %d", stats.TotalSuccesses)
	}
	if stats.TotalFailures != 3 {
		t.Errorf("expected 3 failures, got %d", stats.TotalFailures)
	}
	if stats.ConsecutiveFailures != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", stats.ConsecutiveFailures)
	}
	if stats.State != StateClosed {
		t.Errorf("expected state to be closed, got %s", stats.State)
	}
}

func TestCircuitBreaker_FailureRateCalculation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.SlidingWindowSize = 10
	config.FailureThreshold = 100 // High to prevent opening
	cb := NewCircuitBreaker(config, logger)

	testErr := errors.New("test error")

	// Window is pre-filled with successes, so rate should be 0
	stats := cb.GetStats()
	if stats.FailureRate != 0 {
		t.Errorf("expected initial failure rate 0, got %f", stats.FailureRate)
	}

	// Add 5 failures
	for i := 0; i < 5; i++ {
		_ = cb.Call(func() error {
			return testErr
		})
	}

	// Add 5 successes
	for i := 0; i < 5; i++ {
		_ = cb.Call(func() error {
			return nil
		})
	}

	stats = cb.GetStats()
	// Failure rate should be 5/10 = 0.5
	if stats.FailureRate != 0.5 {
		t.Errorf("expected failure rate 0.5, got %f", stats.FailureRate)
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	if config.FailureThreshold != 5 {
		t.Errorf("expected failure threshold 5, got %d", config.FailureThreshold)
	}
	if config.SuccessThreshold != 2 {
		t.Errorf("expected success threshold 2, got %d", config.SuccessThreshold)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", config.Timeout)
	}
	if config.MaxHalfOpenRequests != 1 {
		t.Errorf("expected max half-open requests 1, got %d", config.MaxHalfOpenRequests)
	}
	if config.FailureRateThreshold != 0 {
		t.Errorf("expected failure rate threshold 0, got %f", config.FailureRateThreshold)
	}
	if config.SlidingWindowSize != 0 {
		t.Errorf("expected sliding window size 0, got %d", config.SlidingWindowSize)
	}
}

func TestCircuitBreaker_PanicRecoveryInCallback(t *testing.T) {
	logger := zaptest.NewLogger(t)
	config := DefaultCircuitBreakerConfig()
	config.FailureThreshold = 2
	config.OnStateChange = func(from, to CircuitBreakerState, reason string) {
		panic("test panic in callback")
	}

	cb := NewCircuitBreaker(config, logger)

	testErr := errors.New("test error")

	// This should not panic the main goroutine
	for i := 0; i < 2; i++ {
		_ = cb.Call(func() error {
			return testErr
		})
	}

	// Give async callback time to execute and recover
	time.Sleep(50 * time.Millisecond)

	// Circuit should still be open
	if cb.GetState() != StateOpen {
		t.Errorf("expected state to be open, got %s", cb.GetState())
	}
}
