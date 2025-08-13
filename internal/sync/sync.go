package sync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/internal/buffer"
	"github.com/theopenlane/agent/internal/config"
	"github.com/theopenlane/agent/internal/connectivity"
)

// Service manages synchronization of buffered results when connectivity is restored
type Service struct {
	logger     zerolog.Logger
	buffer     *buffer.ResultBuffer
	connMgr    *connectivity.Manager
	apiClient  *api.GraphQLClient
	agentID    string
	maxRetries int
	
	mu         sync.RWMutex
	running    bool
	stopChan   chan struct{}
	syncStats  SyncStats
}

// SyncStats tracks synchronization statistics
type SyncStats struct {
	mu                    sync.RWMutex
	totalSyncAttempts     int64
	successfulSyncs       int64
	failedSyncs           int64
	lastSyncTime          time.Time
	lastSyncError         error
	bufferedItemsSynced   int64
	lastConnectivityEvent time.Time
}

// NewService creates a new sync service
func NewService(
	logger zerolog.Logger,
	buffer *buffer.ResultBuffer,
	connMgr *connectivity.Manager,
	apiClient *api.GraphQLClient,
	agentID string,
	maxRetries int,
) *Service {
	return &Service{
		logger:     logger,
		buffer:     buffer,
		connMgr:    connMgr,
		apiClient:  apiClient,
		agentID:    agentID,
		maxRetries: maxRetries,
		stopChan:   make(chan struct{}),
	}
}

// Start begins monitoring connectivity and syncing buffered results
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("sync service already running")
	}
	s.running = true
	s.mu.Unlock()

	s.logger.Info().Msg("Starting sync service")

	// Subscribe to connectivity status changes
	statusChan := s.connMgr.Subscribe()

	// Start the sync routine
	go s.syncRoutine(ctx, statusChan)

	return nil
}

// Stop gracefully stops the sync service
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.logger.Info().Msg("Stopping sync service")
	s.running = false
	close(s.stopChan)
}

// syncRoutine handles the main sync logic
func (s *Service) syncRoutine(ctx context.Context, statusChan <-chan connectivity.Status) {
	// Perform initial sync if we're already online
	if s.connMgr.IsOnline() {
		s.logger.Info().Msg("Already online, performing initial sync")
		s.syncBufferedResults(ctx)
	}

	// Also run periodic sync attempts in case we missed connectivity events
	syncTicker := time.NewTicker(5 * time.Minute)
	defer syncTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Debug().Msg("Sync routine stopping due to context cancellation")
			return
		case <-s.stopChan:
			s.logger.Debug().Msg("Sync routine stopping due to stop signal")
			return
		case status := <-statusChan:
			s.updateConnectivityEvent()
			if status == connectivity.StatusOnline {
				s.logger.Info().Msg("Connectivity restored, syncing buffered results")
				s.syncBufferedResults(ctx)
			} else if status == connectivity.StatusOffline {
				s.logger.Info().Msg("Connectivity lost, buffering mode active")
			}
		case <-syncTicker.C:
			// Periodic sync attempt if we're online
			if s.connMgr.IsOnline() {
				s.logger.Debug().Msg("Periodic sync check")
				s.syncBufferedResults(ctx)
			}
		}
	}
}

// syncBufferedResults attempts to sync all buffered results
func (s *Service) syncBufferedResults(ctx context.Context) {
	s.updateSyncAttempt()

	// Get all buffered results
	bufferedResults, err := s.buffer.GetBufferedResults()
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to get buffered results")
		s.updateSyncFailure(err)
		return
	}

	if len(bufferedResults) == 0 {
		s.logger.Debug().Msg("No buffered results to sync")
		s.updateSyncSuccess(0)
		return
	}

	s.logger.Info().Int("count", len(bufferedResults)).Msg("Found buffered results to sync")

	// Sync results in batches to avoid overwhelming the API
	const batchSize = 10
	var totalSynced int

	for i := 0; i < len(bufferedResults); i += batchSize {
		select {
		case <-ctx.Done():
			s.logger.Debug().Msg("Context cancelled during sync")
			return
		default:
		}

		end := i + batchSize
		if end > len(bufferedResults) {
			end = len(bufferedResults)
		}

		batch := bufferedResults[i:end]
		synced, err := s.syncBatch(ctx, batch)
		totalSynced += synced

		if err != nil {
			s.logger.Error().Err(err).Int("batch_start", i).Int("batch_size", len(batch)).Msg("Batch sync failed")
			// Continue with next batch on error
		}

		// Brief pause between batches to be gentle on the API
		time.Sleep(100 * time.Millisecond)
	}

	s.logger.Info().Int("synced", totalSynced).Int("total", len(bufferedResults)).Msg("Sync completed")
	s.updateSyncSuccess(int64(totalSynced))
}

// syncBatch syncs a batch of buffered results
func (s *Service) syncBatch(ctx context.Context, batch []*buffer.BufferedResult) (int, error) {
	var synced int
	var lastError error

	for _, bufferedResult := range batch {
		err := s.syncSingleResult(ctx, bufferedResult)
		if err != nil {
			s.logger.Error().Err(err).Str("result_id", bufferedResult.ID).Msg("Failed to sync result")
			
			// Update retry count and error
			bufferedResult.Retries++
			bufferedResult.LastError = err.Error()
			
			// Remove if max retries exceeded
			if bufferedResult.Retries >= s.maxRetries {
				s.logger.Warn().Str("result_id", bufferedResult.ID).Int("retries", bufferedResult.Retries).Msg("Max retries exceeded, removing buffered result")
				if removeErr := s.buffer.RemoveBufferedResult(bufferedResult.ID); removeErr != nil {
					s.logger.Error().Err(removeErr).Str("result_id", bufferedResult.ID).Msg("Failed to remove failed result")
				}
			} else {
				// Update the buffered result with new retry count
				if updateErr := s.buffer.UpdateBufferedResult(bufferedResult); updateErr != nil {
					s.logger.Error().Err(updateErr).Str("result_id", bufferedResult.ID).Msg("Failed to update buffered result")
				}
			}
			
			lastError = err
			continue
		}

		// Successfully synced, remove from buffer
		if err := s.buffer.RemoveBufferedResult(bufferedResult.ID); err != nil {
			s.logger.Error().Err(err).Str("result_id", bufferedResult.ID).Msg("Failed to remove synced result")
		} else {
			synced++
			s.logger.Debug().Str("result_id", bufferedResult.ID).Msg("Successfully synced and removed buffered result")
		}
	}

	return synced, lastError
}

// syncSingleResult syncs a single buffered result to the API
func (s *Service) syncSingleResult(ctx context.Context, bufferedResult *buffer.BufferedResult) error {
	result := bufferedResult.Result
	
	s.logger.Debug().
		Str("result_id", bufferedResult.ID).
		Str("check", result.CheckName).
		Time("buffered_at", bufferedResult.BufferedAt).
		Int("retries", bufferedResult.Retries).
		Msg("Syncing buffered result")

	// Attempt to report the result using the API client
	results := []*config.Result{result}
	
	// Use a timeout for individual sync operations
	syncCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return s.apiClient.ReportResults(syncCtx, s.agentID, results)
}

// GetStats returns current sync statistics
func (s *Service) GetStats() map[string]interface{} {
	s.syncStats.mu.RLock()
	defer s.syncStats.mu.RUnlock()

	stats := map[string]interface{}{
		"running":                    s.isRunning(),
		"total_sync_attempts":        s.syncStats.totalSyncAttempts,
		"successful_syncs":           s.syncStats.successfulSyncs,
		"failed_syncs":               s.syncStats.failedSyncs,
		"last_sync_time":             s.syncStats.lastSyncTime,
		"buffered_items_synced":      s.syncStats.bufferedItemsSynced,
		"last_connectivity_event":    s.syncStats.lastConnectivityEvent,
	}

	if s.syncStats.lastSyncError != nil {
		stats["last_sync_error"] = s.syncStats.lastSyncError.Error()
	}

	return stats
}

// Helper methods for stats tracking
func (s *Service) updateSyncAttempt() {
	s.syncStats.mu.Lock()
	defer s.syncStats.mu.Unlock()
	s.syncStats.totalSyncAttempts++
}

func (s *Service) updateSyncSuccess(itemsSynced int64) {
	s.syncStats.mu.Lock()
	defer s.syncStats.mu.Unlock()
	s.syncStats.successfulSyncs++
	s.syncStats.bufferedItemsSynced += itemsSynced
	s.syncStats.lastSyncTime = time.Now()
	s.syncStats.lastSyncError = nil
}

func (s *Service) updateSyncFailure(err error) {
	s.syncStats.mu.Lock()
	defer s.syncStats.mu.Unlock()
	s.syncStats.failedSyncs++
	s.syncStats.lastSyncTime = time.Now()
	s.syncStats.lastSyncError = err
}

func (s *Service) updateConnectivityEvent() {
	s.syncStats.mu.Lock()
	defer s.syncStats.mu.Unlock()
	s.syncStats.lastConnectivityEvent = time.Now()
}

func (s *Service) isRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}