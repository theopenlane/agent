package storage

import (
	"time"

	"github.com/theopenlane/agent/config"
)

const (
	// Default retry configuration
	defaultRetryBackoffMinutes = 2
)

// OptionsFromConfig creates storage options from agent configuration
func OptionsFromConfig(cfg *config.Config) []Option {
	var opts []Option

	// Use buffered storage for all modes (consolidation)
	opts = append(opts, WithAPIConfig(cfg.APIURL, cfg.RegistrationToken))
	opts = append(opts, WithBuffering(cfg.Offline.BufferDir))

	// Add retry configuration
	if cfg.Offline.MaxRetries > 0 {
		opts = append(opts, WithRetryPolicy(cfg.Offline.MaxRetries, 2*time.Minute)) // nolint:mnd
	}

	// Add connectivity monitoring
	if cfg.Offline.ConnectivityCheckURL != "" {
		opts = append(opts, WithConnectivityMonitoring(cfg.Offline.ConnectivityCheckURL, cfg.Offline.ConnectivityInterval))
	}

	// Add evidence configuration if enabled
	if cfg.Evidence.Enabled {
		opts = append(opts, WithEvidence(
			cfg.Evidence.Enabled,
			cfg.Evidence.RetentionPeriod,
			cfg.Evidence.MaxFileSize,
		))
	}

	return opts
}

// NewStorageFromConfig creates a storage instance from agent configuration
func NewStorageFromConfig(cfg *config.Config) (Storage, error) {
	opts := OptionsFromConfig(cfg)
	return NewStorage(opts...)
}

// ConfigFromAgentConfig converts agent config to storage config
func ConfigFromAgentConfig(cfg *config.Config) *Config {
	storageConfig := &Config{
		DataDir: cfg.DataDir,

		// API configuration
		APIURL:            cfg.APIURL,
		RegistrationToken: cfg.RegistrationToken,

		// Buffering configuration
		BufferDir:             cfg.Offline.BufferDir,
		MaxRetries:            cfg.Offline.MaxRetries,
		RetryBackoff:          defaultRetryBackoffMinutes * time.Minute,
		BufferRetentionPeriod: cfg.Offline.BufferRetentionPeriod,
		SyncInterval:          cfg.Offline.SyncInterval,

		// Evidence configuration
		EvidenceEnabled:         cfg.Evidence.Enabled,
		EvidenceRetentionPeriod: cfg.Evidence.RetentionPeriod,
		EvidenceMaxFileSize:     cfg.Evidence.MaxFileSize,

		// Connectivity configuration
		ConnectivityCheckURL: cfg.Offline.ConnectivityCheckURL,
		ConnectivityInterval: cfg.Offline.ConnectivityInterval,
	}

	// Mode consolidation complete - always use buffered storage

	return storageConfig
}
