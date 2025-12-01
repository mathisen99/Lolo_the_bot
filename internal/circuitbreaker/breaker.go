package circuitbreaker

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed means the circuit is closed and requests are allowed
	StateClosed State = iota
	// StateOpen means the circuit is open and requests are blocked
	StateOpen
	// StateHalfOpen means the circuit is testing if it can close
	StateHalfOpen
)

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreaker implements the circuit breaker pattern for API calls
type CircuitBreaker struct {
	mu sync.RWMutex

	// Configuration
	threshold     int           // Number of consecutive failures before opening
	timeout       time.Duration // Time to wait before attempting to close
	healthCheckFn func(context.Context) error
	onStateChange func(from, to State)

	// State
	state               State
	consecutiveFailures int
	lastFailureTime     time.Time
	lastStateChange     time.Time
}

// Config holds configuration for the circuit breaker
type Config struct {
	Threshold     int                         // Number of consecutive failures before opening (default: 5)
	Timeout       time.Duration               // Time to wait before retry (default: 30s)
	HealthCheckFn func(context.Context) error // Function to check health
	OnStateChange func(from, to State)        // Callback when state changes
}

// New creates a new circuit breaker with the given configuration
func New(config Config) *CircuitBreaker {
	if config.Threshold <= 0 {
		config.Threshold = 5
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}

	return &CircuitBreaker{
		threshold:       config.Threshold,
		timeout:         config.Timeout,
		healthCheckFn:   config.HealthCheckFn,
		onStateChange:   config.OnStateChange,
		state:           StateClosed,
		lastStateChange: time.Now(),
	}
}

// Call executes the given function if the circuit is closed or half-open
// Returns an error if the circuit is open or if the function fails
func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error {
	// Check if we should allow the call
	if err := cb.beforeCall(); err != nil {
		return err
	}

	// Execute the function
	err := fn()

	// Record the result
	cb.afterCall(err)

	return err
}

// beforeCall checks if the call should be allowed
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		// Allow the call
		return nil

	case StateOpen:
		// Check if timeout has elapsed
		if time.Since(cb.lastFailureTime) >= cb.timeout {
			// Transition to half-open to test
			cb.setState(StateHalfOpen)
			return nil
		}
		// Circuit is still open
		return fmt.Errorf("circuit breaker is open")

	case StateHalfOpen:
		// Allow one test call
		return nil

	default:
		return fmt.Errorf("unknown circuit breaker state")
	}
}

// afterCall records the result of a call
func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		// Call failed
		cb.onFailure()
	} else {
		// Call succeeded
		cb.onSuccess()
	}
}

// onFailure handles a failed call
func (cb *CircuitBreaker) onFailure() {
	cb.lastFailureTime = time.Now()
	cb.consecutiveFailures++

	switch cb.state {
	case StateClosed:
		// Check if we've reached the threshold
		if cb.consecutiveFailures >= cb.threshold {
			cb.setState(StateOpen)
		}

	case StateHalfOpen:
		// Test failed, go back to open
		cb.setState(StateOpen)
	}
}

// onSuccess handles a successful call
func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case StateClosed:
		// Reset failure count
		cb.consecutiveFailures = 0

	case StateHalfOpen:
		// Test succeeded, close the circuit
		cb.consecutiveFailures = 0
		cb.setState(StateClosed)
	}
}

// setState changes the circuit breaker state
func (cb *CircuitBreaker) setState(newState State) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState
	cb.lastStateChange = time.Now()

	// Call the state change callback if provided
	if cb.onStateChange != nil {
		// Call in a goroutine to avoid blocking
		go cb.onStateChange(oldState, newState)
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetConsecutiveFailures returns the number of consecutive failures
func (cb *CircuitBreaker) GetConsecutiveFailures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.consecutiveFailures
}

// GetLastFailureTime returns the time of the last failure
func (cb *CircuitBreaker) GetLastFailureTime() time.Time {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.lastFailureTime
}

// GetLastStateChange returns the time of the last state change
func (cb *CircuitBreaker) GetLastStateChange() time.Time {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.lastStateChange
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state
	cb.state = StateClosed
	cb.consecutiveFailures = 0
	cb.lastStateChange = time.Now()

	if cb.onStateChange != nil && oldState != StateClosed {
		go cb.onStateChange(oldState, StateClosed)
	}
}

// TryHealthCheck attempts to perform a health check if one is configured
// This is useful for manually testing if the circuit can be closed
func (cb *CircuitBreaker) TryHealthCheck(ctx context.Context) error {
	if cb.healthCheckFn == nil {
		return fmt.Errorf("no health check function configured")
	}

	cb.mu.RLock()
	state := cb.state
	cb.mu.RUnlock()

	// Only perform health check if circuit is open
	if state != StateOpen {
		return nil
	}

	// Check if timeout has elapsed
	cb.mu.RLock()
	timeSinceFailure := time.Since(cb.lastFailureTime)
	cb.mu.RUnlock()

	if timeSinceFailure < cb.timeout {
		return fmt.Errorf("circuit is open, waiting for timeout")
	}

	// Transition to half-open
	cb.mu.Lock()
	cb.setState(StateHalfOpen)
	cb.mu.Unlock()

	// Perform health check
	err := cb.healthCheckFn(ctx)

	// Record result
	cb.afterCall(err)

	return err
}

// GetStats returns statistics about the circuit breaker
func (cb *CircuitBreaker) GetStats() Stats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return Stats{
		State:               cb.state,
		ConsecutiveFailures: cb.consecutiveFailures,
		LastFailureTime:     cb.lastFailureTime,
		LastStateChange:     cb.lastStateChange,
		Threshold:           cb.threshold,
		Timeout:             cb.timeout,
	}
}

// Stats holds statistics about the circuit breaker
type Stats struct {
	State               State
	ConsecutiveFailures int
	LastFailureTime     time.Time
	LastStateChange     time.Time
	Threshold           int
	Timeout             time.Duration
}
