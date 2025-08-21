package retry

import (
	"context"
	"time"

	"github.com/codeGROOVE-dev/retry"
	"github.com/rs/zerolog/log"
)

// Manager provides centralized retry functionality with configurable strategies
type Manager struct {
	config Config
}

// Config defines retry behavior settings
type Config struct {
	MaxAttempts  uint          `json:"maxAttempts" koanf:"maxAttempts" default:"5" description:"Maximum number of retry attempts"`
	InitialDelay time.Duration `json:"initialDelay" koanf:"initialDelay" default:"1s" description:"Initial delay between retries"`
	MaxDelay     time.Duration `json:"maxDelay" koanf:"maxDelay" default:"30s" description:"Maximum delay between retries"`
	Strategy     string        `json:"strategy" koanf:"strategy" default:"exponential" description:"Retry strategy: exponential, linear, random"`
	Multiplier   float64       `json:"multiplier" koanf:"multiplier" default:"2.0" description:"Backoff multiplier for exponential strategy"`
}

// Operation represents a retryable operation
type Operation func() error

// OperationWithContext represents a retryable operation that accepts context
type OperationWithContext func(context.Context) error

// NewManager creates a new retry manager with the given configuration
func NewManager(config Config) *Manager {
	// Set defaults if not provided
	if config.MaxAttempts == 0 {
		config.MaxAttempts = 5
	}

	if config.InitialDelay == 0 {
		config.InitialDelay = time.Second
	}

	if config.MaxDelay == 0 {
		config.MaxDelay = 30 * time.Second
	}

	if config.Strategy == "" {
		config.Strategy = "exponential"
	}

	if config.Multiplier == 0 {
		config.Multiplier = 2.0
	}

	return &Manager{
		config: config,
	}
}

// Execute executes an operation with retry logic
func (m *Manager) Execute(ctx context.Context, operation Operation) error {
	return m.executeWithContext(ctx, func(context.Context) error {
		return operation()
	})
}

// ExecuteWithContext executes a context-aware operation with retry logic
func (m *Manager) ExecuteWithContext(ctx context.Context, operation OperationWithContext) error {
	return m.executeWithContext(ctx, operation)
}

// executeWithContext is the internal implementation that handles both operation types
func (m *Manager) executeWithContext(ctx context.Context, operation OperationWithContext) error {
	maxRetries := m.config.MaxAttempts
	initialBackoff := m.config.InitialDelay
	maxBackoff := m.config.MaxDelay

	return retry.Do(func() error {
		return operation(ctx)
	},
		retry.Attempts(maxRetries),
		retry.DelayType(retry.FullJitterBackoffDelay),
		retry.Delay(initialBackoff),
		retry.MaxDelay(maxBackoff),
		retry.OnRetry(func(n uint, err error) {
			log.Warn().Uint("attempt", n+1).Uint("max_attempts", maxRetries).Err(err).Msg("Operation failed, retrying")
		}),
	)
}

// ExecuteWithCustomRetry executes an operation with specific retry parameters
// This matches gitMDM's pattern of direct retry configuration
func (m *Manager) ExecuteWithCustomRetry(operation Operation, maxRetries uint, initialBackoff, maxBackoff time.Duration) error {
	return retry.Do(func() error {
		return operation()
	},
		retry.Attempts(maxRetries),
		retry.DelayType(retry.FullJitterBackoffDelay),
		retry.Delay(initialBackoff),
		retry.MaxDelay(maxBackoff),
		retry.OnRetry(func(n uint, err error) {
			log.Warn().Uint("attempt", n+1).Uint("max_attempts", maxRetries).Err(err).Msg("Operation failed, retrying")
		}),
	)
}

// ExecuteWithCustomConfig executes an operation with custom retry configuration
func (m *Manager) ExecuteWithCustomConfig(ctx context.Context, operation Operation, customConfig Config) error {
	tempManager := NewManager(customConfig)
	return tempManager.Execute(ctx, operation)
}

// IsRetryableError checks if an error should trigger a retry
// This can be extended to handle specific error types
func (m *Manager) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Add specific error type checking here
	// For now, retry all errors
	return true
}

// GetConfig returns the current retry configuration
func (m *Manager) GetConfig() Config {
	return m.config
}

// UpdateConfig updates the retry configuration
func (m *Manager) UpdateConfig(config Config) {
	m.config = config
	log.Info().Uint("max_attempts", config.MaxAttempts).Dur("initial_delay", config.InitialDelay).Dur("max_delay", config.MaxDelay).Str("strategy", config.Strategy).Msg("Retry configuration updated")
}

// GetStats returns retry statistics (for future implementation)
func (m *Manager) GetStats() map[string]any {
	return map[string]any{
		"max_attempts":  m.config.MaxAttempts,
		"initial_delay": m.config.InitialDelay.String(),
		"max_delay":     m.config.MaxDelay.String(),
		"strategy":      m.config.Strategy,
		"multiplier":    m.config.Multiplier,
	}
}
