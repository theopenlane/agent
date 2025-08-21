package core

import (
	"time"

	"github.com/theopenlane/agent/config"
)

// WithConfig sets the agent configuration
func WithConfig(cfg *config.Config) AgentOption {
	return func(a *Agent) {
		a.config = cfg
	}
}

// WithPollInterval sets the polling interval for the agent
func WithPollInterval(interval time.Duration) AgentOption {
	return func(a *Agent) {
		a.config.PollInterval = interval
	}
}

// WithDefaultTimeout sets the default timeout for check executions
func WithDefaultTimeout(timeout time.Duration) AgentOption {
	return func(a *Agent) {
		a.config.DefaultTimeout = timeout
	}
}

// WithOperationMode sets the operation mode (normal, standalone, buffered)
func WithOperationMode(mode config.OperationMode) AgentOption {
	return func(a *Agent) {
		a.config.Offline.Mode = mode
	}
}

// WithStandaloneMode configures the agent for standalone operation
func WithStandaloneMode(outputDir, outputFormat string) AgentOption {
	return func(a *Agent) {
		a.config.Offline.Mode = config.ModeStandalone
		a.config.Offline.OutputDir = outputDir
		a.config.Offline.OutputFormat = outputFormat
	}
}

// WithBufferedMode configures the agent for buffered operation
func WithBufferedMode(bufferDir string, maxRetries int, retentionPeriod time.Duration) AgentOption {
	return func(a *Agent) {
		a.config.Offline.Mode = config.ModeBuffered
		a.config.Offline.BufferDir = bufferDir
		a.config.Offline.MaxRetries = maxRetries
		a.config.Offline.BufferRetentionPeriod = retentionPeriod
	}
}

// WithEvidenceCollection configures evidence collection settings
func WithEvidenceCollection(enabled bool, retentionPeriod time.Duration, maxFileSize int64, compress bool) AgentOption {
	return func(a *Agent) {
		a.config.Evidence.Enabled = enabled
		a.config.Evidence.RetentionPeriod = retentionPeriod
		a.config.Evidence.MaxFileSize = maxFileSize
		a.config.Evidence.CompressFiles = compress
	}
}

// WithRetryStrategy configures retry behavior
func WithRetryStrategy(maxAttempts uint, initialDelay, maxDelay time.Duration, strategy string, multiplier float64) AgentOption {
	return func(a *Agent) {
		a.config.Retry.MaxAttempts = maxAttempts
		a.config.Retry.InitialDelay = initialDelay
		a.config.Retry.MaxDelay = maxDelay
		a.config.Retry.Strategy = strategy
		a.config.Retry.Multiplier = multiplier
	}
}

// WithIdentity configures hardware identity settings
func WithIdentity(enabled bool, cacheTimeout time.Duration, fallbackToHostname bool, overrideID string) AgentOption {
	return func(a *Agent) {
		a.config.Identity.Enabled = enabled
		a.config.Identity.CacheTimeout = cacheTimeout
		a.config.Identity.FallbackToHostname = fallbackToHostname
		a.config.Identity.OverrideHardwareID = overrideID
	}
}

// WithAPIConnection configures API connection settings
func WithAPIConnection(apiURL, registrationToken string) AgentOption {
	return func(a *Agent) {
		a.config.APIURL = apiURL
		a.config.RegistrationToken = registrationToken
	}
}

// WithDataDirectory sets the data directory for the agent
func WithDataDirectory(dataDir string) AgentOption {
	return func(a *Agent) {
		a.config.DataDir = dataDir
	}
}

// WithLogLevel sets the logging level
func WithLogLevel(level string) AgentOption {
	return func(a *Agent) {
		a.config.LogLevel = level
	}
}
