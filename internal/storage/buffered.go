package storage

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/internal/buffer"
	"github.com/theopenlane/agent/internal/config"
	"github.com/theopenlane/agent/internal/connectivity"
	syncservice "github.com/theopenlane/agent/internal/sync"
)

// BufferedAPIStorage implements ResultStorage with API + local buffering fallback
type BufferedAPIStorage struct {
	logger      zerolog.Logger
	apiStorage  *APIResultStorage
	buffer      *buffer.ResultBuffer
	connMgr     *connectivity.Manager
	syncService *syncservice.Service
	config      config.Config
}

// NewBufferedAPIStorage creates a new buffered API storage instance
func NewBufferedAPIStorage(cfg config.Config) (*BufferedAPIStorage, error) {
	logger := zerolog.New(os.Stdout).With().Str("component", "buffered-api-storage").Logger()

	// Create API storage
	apiStorage, err := NewAPIResultStorage(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create API storage: %w", err)
	}

	// Create buffer
	buffer, err := buffer.NewResultBuffer(logger, cfg.Offline.BufferDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create result buffer: %w", err)
	}

	// Create connectivity manager
	connectivityURL := cfg.Offline.ConnectivityCheckURL
	if connectivityURL == "" {
		connectivityURL = cfg.APIURL
	}
	connMgr := connectivity.NewManager(logger, connectivityURL)

	// Create sync service
	maxRetries := cfg.Offline.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 5
	}

	apiClient, err := api.NewGraphQLClient(cfg.APIURL, cfg.RegistrationToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client for sync service: %w", err)
	}

	syncService := syncservice.NewService(
		logger,
		buffer,
		connMgr,
		apiClient,
		cfg.AgentID,
		maxRetries,
	)

	return &BufferedAPIStorage{
		logger:      logger,
		apiStorage:  apiStorage,
		buffer:      buffer,
		connMgr:     connMgr,
		syncService: syncService,
		config:      cfg,
	}, nil
}

// Start initializes the buffered storage (starts connectivity monitoring and sync)
func (bas *BufferedAPIStorage) Start(ctx context.Context) error {
	// Start connectivity monitoring
	interval := bas.config.Offline.ConnectivityInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	
	bas.logger.Info().Dur("interval", interval).Msg("Starting connectivity monitoring")
	go bas.connMgr.StartMonitoring(ctx, interval)

	// Start sync service
	bas.logger.Info().Msg("Starting sync service")
	return bas.syncService.Start(ctx)
}

// Stop stops the buffered storage components
func (bas *BufferedAPIStorage) Stop() {
	bas.logger.Info().Msg("Stopping buffered API storage")
	if bas.syncService != nil {
		bas.syncService.Stop()
	}
}

// StoreResult stores a single result, with API fallback to buffering
func (bas *BufferedAPIStorage) StoreResult(ctx context.Context, result *config.Result) error {
	// Try API first
	err := bas.apiStorage.StoreResult(ctx, result)
	if err == nil {
		return nil // Success
	}

	// API failed, buffer the result
	bas.logger.Warn().Err(err).Str("check", result.CheckName).Msg("API storage failed, buffering result")
	
	if bufferErr := bas.buffer.BufferResult(result); bufferErr != nil {
		return fmt.Errorf("both API storage and buffering failed: api_err=%w, buffer_err=%v", err, bufferErr)
	}

	bas.logger.Info().Str("check", result.CheckName).Msg("Result buffered for later sync")
	return nil // Return success since we buffered it
}

// StoreResults stores multiple results, with API fallback to buffering
func (bas *BufferedAPIStorage) StoreResults(ctx context.Context, results []*config.Result) error {
	// Try API first
	err := bas.apiStorage.StoreResults(ctx, results)
	if err == nil {
		return nil // Success
	}

	// API failed, buffer each result
	bas.logger.Warn().Err(err).Int("count", len(results)).Msg("API storage failed, buffering results")
	
	var bufferErrors []error
	bufferedCount := 0
	
	for _, result := range results {
		if bufferErr := bas.buffer.BufferResult(result); bufferErr != nil {
			bufferErrors = append(bufferErrors, bufferErr)
		} else {
			bufferedCount++
		}
	}

	if len(bufferErrors) > 0 {
		return fmt.Errorf("API storage failed and %d results failed to buffer: api_err=%w", len(bufferErrors), err)
	}

	bas.logger.Info().Int("count", bufferedCount).Msg("Results buffered for later sync")
	return nil // Return success since we buffered them
}

// GetStorageInfo returns combined information about API and buffer storage
func (bas *BufferedAPIStorage) GetStorageInfo() StorageInfo {
	apiInfo := bas.apiStorage.GetStorageInfo()
	
	bufferStats, err := bas.buffer.GetBufferStats()
	if err != nil {
		bufferStats = map[string]interface{}{"error": err.Error()}
	}

	connStats := bas.connMgr.GetStats()
	
	metadata := map[string]interface{}{
		"api_storage":    apiInfo.Metadata,
		"buffer_stats":   bufferStats,
		"connectivity":   connStats,
		"mode":           "buffered",
	}

	if bas.syncService != nil {
		metadata["sync_stats"] = bas.syncService.GetStats()
	}

	// Overall health is healthy if either API works OR buffering works
	healthy := apiInfo.Healthy || bufferStats["buffered_count"] != nil

	capabilities := []string{"store-results", "fallback-buffering", "auto-sync", "connectivity-aware"}

	return StorageInfo{
		Type:         "buffered-api",
		Location:     fmt.Sprintf("%s (buffer: %s)", apiInfo.Location, bas.config.Offline.BufferDir),
		Healthy:      healthy,
		LastCheck:    time.Now(),
		Metadata:     metadata,
		Capabilities: capabilities,
	}
}

// Close closes all components of the buffered storage
func (bas *BufferedAPIStorage) Close() error {
	bas.logger.Info().Msg("Closing buffered API storage")
	
	// Stop sync service
	bas.Stop()
	
	// Close API storage
	return bas.apiStorage.Close()
}