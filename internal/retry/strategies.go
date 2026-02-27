package retry

import (
	"context"
	"net"
	"time"
)

// Predefined retry strategies for common use cases
var (
	// QuickRetry for fast operations that should retry quickly
	QuickRetry = Config{
		MaxAttempts:  3,                      // nolint:mnd
		InitialDelay: 100 * time.Millisecond, // nolint:mnd
		MaxDelay:     1 * time.Second,
		Strategy:     "exponential",
		Multiplier:   2.0, // nolint:mnd
	}

	// StandardRetry for normal operations
	StandardRetry = Config{
		MaxAttempts:  5, // nolint:mnd
		InitialDelay: 1 * time.Second,
		MaxDelay:     30 * time.Second, // nolint:mnd
		Strategy:     "exponential",
		Multiplier:   2.0, // nolint:mnd
	}

	// SlowRetry for expensive operations that need longer delays
	SlowRetry = Config{
		MaxAttempts:  3,                // nolint:mnd
		InitialDelay: 5 * time.Second,  // nolint:mnd
		MaxDelay:     60 * time.Second, // nolint:mnd
		Strategy:     "exponential",
		Multiplier:   2.0, // nolint:mnd
	}

	// NetworkRetry optimized for network operations
	NetworkRetry = Config{
		MaxAttempts:  10,                     // nolint:mnd
		InitialDelay: 500 * time.Millisecond, // nolint:mnd
		MaxDelay:     30 * time.Second,       // nolint:mnd
		Strategy:     "exponential",
		Multiplier:   1.5, // nolint:mnd
	}

	// FileIORetry for file system operations
	FileIORetry = Config{
		MaxAttempts:  3,                      // nolint:mnd
		InitialDelay: 100 * time.Millisecond, // nolint:mnd
		MaxDelay:     5 * time.Second,        // nolint:mnd
		Strategy:     "linear",
		Multiplier:   1.0, // nolint:mnd
	}
)

// Condition represents a function that determines if an error should trigger a retry
type Condition func(error) bool

// Common retry conditions
var (
	// RetryOnNetworkError retries on network-related errors
	RetryOnNetworkError Condition = func(err error) bool {
		if err == nil {
			return false
		}

		// Check for network errors (timeout errors are considered retryable)
		if netErr, ok := err.(net.Error); ok {
			return netErr.Timeout()
		}

		// Check for DNS errors (retry on timeout and server failures)
		if dnsErr, ok := err.(*net.DNSError); ok {
			return dnsErr.Timeout() || dnsErr.IsTimeout
		}

		return false
	}

	// RetryOnTimeout retries on timeout errors
	RetryOnTimeout Condition = func(err error) bool {
		if err == nil {
			return false
		}

		if netErr, ok := err.(net.Error); ok {
			return netErr.Timeout()
		}

		return false
	}

	// RetryOnAny retries on any error
	RetryOnAny Condition = func(err error) bool {
		return err != nil
	}

	// NoRetry never retries
	NoRetry Condition = func(error) bool {
		return false
	}
)

// ConditionalManager wraps a Manager with retry conditions
type ConditionalManager struct {
	*Manager
	condition Condition
}

// NewConditionalManager creates a retry manager with a specific retry condition
func NewConditionalManager(manager *Manager, condition Condition) *ConditionalManager {
	return &ConditionalManager{
		Manager:   manager,
		condition: condition,
	}
}

// Execute executes an operation with the configured retry condition
func (cm *ConditionalManager) Execute(ctx context.Context, operation Operation) error {
	return cm.executeWithContext(ctx, func(context.Context) error {
		err := operation()
		if err != nil && !cm.condition(err) {
			// Don't retry this error - return immediately
			return err
		}

		return err
	})
}

// ExecuteWithContext executes a context-aware operation with the configured retry condition
func (cm *ConditionalManager) ExecuteWithContext(ctx context.Context, operation OperationWithContext) error {
	return cm.executeWithContext(ctx, func(context.Context) error {
		err := operation(ctx)
		if err != nil && !cm.condition(err) {
			// Don't retry this error - return immediately
			return err
		}

		return err
	})
}
