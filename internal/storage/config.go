package storage

import (
	"time"

	"github.com/theopenlane/agent/config"
)

// OptionsFromConfig creates storage options from agent configuration
func OptionsFromConfig(cfg *config.Config) []Option {
	var opts []Option

	// Determine storage mode based on configuration
	switch cfg.Offline.Mode {
	case config.ModeNormal:
		opts = append(opts, WithAPIStorage(cfg.APIURL, cfg.RegistrationToken))
	case config.ModeStandalone:
		opts = append(opts, WithLocalStorage(cfg.DataDir, cfg.Offline.OutputDir, cfg.Offline.OutputFormat))
	case config.ModeBuffered:
		opts = append(opts, WithBufferedStorage(cfg.APIURL, cfg.RegistrationToken, cfg.Offline.BufferDir))

		// Add buffering specific configuration
		if cfg.Offline.MaxRetries > 0 {
			opts = append(opts, WithRetryPolicy(cfg.Offline.MaxRetries, 2*time.Minute)) // nolint:mnd
		}

		if cfg.Offline.ConnectivityCheckURL != "" {
			opts = append(opts, WithConnectivityMonitoring(cfg.Offline.ConnectivityCheckURL, cfg.Offline.ConnectivityInterval))
		}
	default:
		// Default to local storage if mode is unrecognized
		opts = append(opts, WithLocalStorage(cfg.DataDir, cfg.Offline.OutputDir, cfg.Offline.OutputFormat))
	}

	// Add evidence configuration if enabled
	if cfg.Evidence.Enabled {
		opts = append(opts, WithEvidence(
			cfg.Evidence.Enabled,
			cfg.Evidence.RetentionPeriod,
			cfg.Evidence.MaxFileSize,
			cfg.Evidence.CompressFiles,
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
		DataDir:      cfg.DataDir,
		OutputDir:    cfg.Offline.OutputDir,
		OutputFormat: cfg.Offline.OutputFormat,

		// API configuration
		APIURL:            cfg.APIURL,
		RegistrationToken: cfg.RegistrationToken,

		// Buffering configuration
		BufferDir:             cfg.Offline.BufferDir,
		MaxRetries:            cfg.Offline.MaxRetries,
		RetryBackoff:          2 * time.Minute, // Default backoff // nolint:mnd
		BufferRetentionPeriod: cfg.Offline.BufferRetentionPeriod,
		SyncInterval:          cfg.Offline.SyncInterval,

		// Evidence configuration
		EvidenceEnabled:         cfg.Evidence.Enabled,
		EvidenceRetentionPeriod: cfg.Evidence.RetentionPeriod,
		EvidenceMaxFileSize:     cfg.Evidence.MaxFileSize,
		EvidenceCompressFiles:   cfg.Evidence.CompressFiles,

		// Connectivity configuration
		ConnectivityCheckURL: cfg.Offline.ConnectivityCheckURL,
		ConnectivityInterval: cfg.Offline.ConnectivityInterval,
	}

	// Set storage mode
	switch cfg.Offline.Mode {
	case config.ModeNormal:
		storageConfig.Mode = ModeAPI
	case config.ModeStandalone:
		storageConfig.Mode = ModeLocal
	case config.ModeBuffered:
		storageConfig.Mode = ModeBuffered
	default:
		storageConfig.Mode = ModeLocal
	}

	return storageConfig
}
