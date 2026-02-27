package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/connectivity"
	"github.com/theopenlane/agent/internal/models"
)

// BufferedStorage implements Storage interface with API storage and local buffering fallback
type BufferedStorage struct {
	config          *Config
	apiStorage      *APIStorage
	connectivityMgr *connectivity.Manager
	mu              sync.RWMutex
	ctx             context.Context
	cancel          context.CancelFunc
	syncTicker      *time.Ticker
	stopCh          chan struct{}
}

// NewBufferedStorage creates a new buffered storage instance
func NewBufferedStorage(cfg *Config) (*BufferedStorage, error) {
	// Ensure buffer directory exists
	if err := os.MkdirAll(cfg.BufferDir, config.DefaultDirectoryPermissions); err != nil {
		return nil, fmt.Errorf("failed to create buffer directory %s: %w", cfg.BufferDir, err)
	}

	// Create API storage (optional - can be nil for standalone mode)
	var apiStorage *APIStorage

	if cfg.RegistrationToken != "" {
		var err error

		apiStorage, err = NewAPIStorage(cfg)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to create API storage, will operate in local-only mode")

			apiStorage = nil
		}
	}

	// Create connectivity manager
	connectivityURL := cfg.ConnectivityCheckURL
	if connectivityURL == "" && cfg.APIURL != "" {
		connectivityURL = cfg.APIURL
	}

	connectivityMgr := connectivity.NewManager(connectivityURL)

	ctx, cancel := context.WithCancel(context.Background())

	bs := &BufferedStorage{
		config:          cfg,
		apiStorage:      apiStorage,
		connectivityMgr: connectivityMgr,
		ctx:             ctx,
		cancel:          cancel,
		stopCh:          make(chan struct{}),
	}

	// Start background services
	if err := bs.start(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to start buffered storage: %w", err)
	}

	return bs, nil
}

// start initializes background services
func (bs *BufferedStorage) start() error {
	// Start connectivity monitoring only if API storage exists and interval is positive
	if bs.apiStorage != nil && bs.config.ConnectivityInterval > 0 {
		go bs.connectivityMgr.StartMonitoring(bs.ctx, bs.config.ConnectivityInterval)
	}

	// Start sync routine
	bs.syncTicker = time.NewTicker(bs.config.SyncInterval)
	go bs.syncLoop()

	log.Info().Str("buffer_dir", bs.config.BufferDir).Dur("sync_interval", bs.config.SyncInterval).Msg("Buffered storage started")

	return nil
}

// StoreResult stores a compliance check result with API fallback to buffering
func (bs *BufferedStorage) StoreResult(result *config.Result) error {
	return bs.StoreResultWithEvidence(result, nil)
}

// StoreResultWithEvidence stores a result with evidence, always buffering to disk first
// then optionally syncing to API if available
func (bs *BufferedStorage) StoreResultWithEvidence(result *config.Result, evidence []models.EvidenceFile) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// ALWAYS buffer to disk first (unified behavior for all modes)
	bufferPath, err := bs.bufferResult(result, evidence, "")
	if err != nil {
		return fmt.Errorf("failed to buffer result: %w", err)
	}

	// If API storage is available, try immediate sync (unless we know we're offline)
	if bs.apiStorage != nil && !bs.connectivityMgr.IsOffline() {
		if err := bs.apiStorage.StoreResultWithEvidence(result, evidence); err != nil {
			log.Warn().Err(err).Str("check", result.CheckName).Msg("Immediate API sync failed, will retry later")
			// Non-fatal - result is buffered and will be synced later
		} else if removeErr := os.Remove(bufferPath); removeErr != nil {
			log.Warn().Err(removeErr).Str("file", bufferPath).Msg("Immediate API sync succeeded but failed to remove buffer file")
		}
	}

	return nil
}

// bufferResult stores a result to local buffer
func (bs *BufferedStorage) bufferResult(result *config.Result, evidence []models.EvidenceFile, reason string) (string, error) {
	// Create buffered result
	bufferedResult := &Result{
		ID:         uuid.New().String(),
		Result:     result,
		BufferedAt: time.Now(),
		RetryCount: 0,
		LastError:  reason,
		Evidence:   evidence,
	}

	// Serialize to JSON
	data, err := json.MarshalIndent(bufferedResult, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal buffered result: %w", err)
	}

	// Write to buffer file
	filename := fmt.Sprintf("%s-%s.json", result.CheckName, bufferedResult.ID)
	filepath := filepath.Join(bs.config.BufferDir, filename)

	if err := os.WriteFile(filepath, data, 0o600); err != nil { // nolint:mnd
		return "", fmt.Errorf("failed to write buffer file %s: %w", filepath, err)
	}

	log.Info().Str("check", result.CheckName).Str("buffer_id", bufferedResult.ID).Str("reason", reason).Msg("Result buffered locally")

	return filepath, nil
}

// syncLoop runs the background synchronization process
func (bs *BufferedStorage) syncLoop() {
	defer bs.syncTicker.Stop()

	for {
		select {
		case <-bs.ctx.Done():
			return
		case <-bs.stopCh:
			return
		case <-bs.syncTicker.C:
			if bs.connectivityMgr.IsOnline() {
				bs.syncBufferedResults()
			}
		}
	}
}

// syncBufferedResults attempts to sync all buffered results to the API
func (bs *BufferedStorage) syncBufferedResults() {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	// Get all buffered files
	files, err := filepath.Glob(filepath.Join(bs.config.BufferDir, "*.json"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to list buffer files")
		return
	}

	if len(files) == 0 {
		return // Nothing to sync
	}

	log.Info().Int("files", len(files)).Msg("Starting buffered results sync")

	successCount := 0

	for _, file := range files {
		if bs.syncBufferedFile(file) {
			successCount++
		}
	}

	if successCount > 0 {
		log.Info().Int("synced", successCount).Int("total", len(files)).Msg("Buffered results sync completed")
	}

	// Cleanup old buffer files
	bs.cleanupOldBufferFiles()
}

// syncBufferedFile attempts to sync a single buffered file
func (bs *BufferedStorage) syncBufferedFile(filepath string) bool {
	// Skip if no API storage available (standalone mode)
	if bs.apiStorage == nil {
		return false
	}

	// Read buffered result
	data, err := os.ReadFile(filepath)
	if err != nil {
		log.Error().Err(err).Str("file", filepath).Msg("Failed to read buffer file")
		return false
	}

	var bufferedResult Result
	if err := json.Unmarshal(data, &bufferedResult); err != nil {
		log.Error().Err(err).Str("file", filepath).Msg("Failed to unmarshal buffer file")
		return false
	}

	// Check retry limits
	if bufferedResult.RetryCount >= bs.config.MaxRetries {
		log.Warn().Str("id", bufferedResult.ID).Int("retries", bufferedResult.RetryCount).Msg("Buffer result exceeded max retries, skipping")

		return false
	}

	// Attempt API upload
	err = bs.apiStorage.StoreResultWithEvidence(bufferedResult.Result, bufferedResult.Evidence)
	if err != nil {
		// Update retry count and error
		bufferedResult.RetryCount++
		bufferedResult.LastError = err.Error()

		// Write back to file
		updatedData, marshalErr := json.MarshalIndent(bufferedResult, "", "  ")
		if marshalErr == nil {
			if writeErr := os.WriteFile(filepath, updatedData, 0o600); writeErr != nil { // nolint:mnd
				log.Warn().Err(writeErr).Str("filepath", filepath).Msg("Failed to write buffered result to file")
			}
		}

		log.Warn().Err(err).Str("id", bufferedResult.ID).Int("retry_count", bufferedResult.RetryCount).Msg("Failed to sync buffered result")

		return false
	}

	// Success - remove buffer file
	if err := os.Remove(filepath); err != nil {
		log.Error().Err(err).Str("file", filepath).Msg("Failed to remove synced buffer file")
	}

	log.Debug().Str("id", bufferedResult.ID).Str("check", bufferedResult.Result.CheckName).Msg("Buffered result synced successfully")

	return true
}

// cleanupOldBufferFiles removes buffer files older than retention period
func (bs *BufferedStorage) cleanupOldBufferFiles() {
	if bs.config.BufferRetentionPeriod <= 0 {
		return
	}

	cutoff := time.Now().Add(-bs.config.BufferRetentionPeriod)

	files, err := filepath.Glob(filepath.Join(bs.config.BufferDir, "*.json"))
	if err != nil {
		return
	}

	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(file); err != nil {
				log.Error().Err(err).Str("file", file).Msg("Failed to remove old buffer file")
			}
		}
	}
}

// Health returns the current health status
func (bs *BufferedStorage) Health() error {
	// Check buffer directory
	if err := os.MkdirAll(bs.config.BufferDir, config.DefaultDirectoryPermissions); err != nil {
		return fmt.Errorf("buffer directory not accessible: %w", err)
	}

	// Test buffer directory writability
	testFile := filepath.Join(bs.config.BufferDir, ".health_check")
	if err := os.WriteFile(testFile, []byte("test"), config.RestrictedFilePermissions); err != nil {
		return fmt.Errorf("buffer directory not writable: %w", err)
	}

	os.Remove(testFile)

	// Check API health (non-fatal if failing, and may not exist in standalone mode)
	if bs.apiStorage != nil {
		if err := bs.apiStorage.Health(); err != nil {
			log.Warn().Err(err).Msg("API health check failed (buffering will be used)")
		}
	}

	return nil
}

// Close gracefully shuts down the storage system
func (bs *BufferedStorage) Close() error {
	// Stop background processes
	close(bs.stopCh)
	bs.cancel()

	// Connectivity manager will stop when context is canceled (no explicit Stop method)

	// Close API storage
	if bs.apiStorage != nil {
		bs.apiStorage.Close()
	}

	log.Info().Msg("Buffered storage closed")

	return nil
}
