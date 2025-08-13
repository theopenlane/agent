package core

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/internal/config"
	"github.com/theopenlane/agent/version"
	"github.com/theopenlane/core/pkg/openlaneclient"
)

// Agent represents the main compliance agent (similar to Buildkite's Agent)
type Agent struct {
	logger zerolog.Logger
	config config.Config

	// API client for communicating with Openlane
	apiClient *api.GraphQLClient

	// Agent registration information
	agentInfo *openlaneclient.JobRunner

	// Workers (can spawn multiple like Buildkite)
	workers []*AgentWorker

	// Control synchronization service
	controlSyncService *ControlSyncService

	// Control channels
	stopOnce sync.Once
	stop     chan struct{}
}

// NewAgent creates a new compliance agent
func NewAgent(logger zerolog.Logger, config config.Config) (*Agent, error) {
	var apiClient *api.GraphQLClient
	
	// Only create API client if not in standalone mode
	if config.Offline.Mode != "standalone" {
		client, err := api.NewGraphQLClient(config.APIURL, config.RegistrationToken)
		if err != nil {
			return nil, fmt.Errorf("failed to create API client: %w", err)
		}
		apiClient = client
	} else {
		logger.Info().Msg("Running in standalone mode - API client disabled")
	}

	return &Agent{
		logger:    logger,
		config:    config,
		apiClient: apiClient,
		stop:      make(chan struct{}),
	}, nil
}

// Register registers the agent with the Openlane platform
func (a *Agent) Register(ctx context.Context) error {
	// Skip registration in standalone mode
	if a.config.Offline.Mode == "standalone" {
		a.logger.Info().Msg("Standalone mode - skipping agent registration")
		return nil
	}

	if a.apiClient == nil {
		return fmt.Errorf("API client not available for registration")
	}

	a.logger.Info().Str("name", a.config.AgentName).Msg("Registering agent")

	// Get system information
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Create registration request (following Buildkite's pattern)
	regReq := api.JobRunnerRegistration{
		Name:      a.config.AgentName,
		IPAddress: "192.168.1.100", // Use non-loopback IP for testing
		Version:   version.Version,
		Platform:  runtime.GOOS,
		Hostname:  hostname,
		Tags:      []string{"compliance", "automated"},
		Metadata: map[string]string{
			"poll_interval":   a.config.PollInterval.String(),
			"max_concurrency": fmt.Sprintf("%d", a.config.MaxConcurrency),
			"go_version":      runtime.Version(),
			"arch":            runtime.GOARCH,
		},
		Capabilities: []string{"compliance-checks", "script-execution"},
	}

	// Register with the platform
	agentInfo, err := a.apiClient.RegisterAgent(ctx, regReq)
	if err != nil {
		return fmt.Errorf("agent registration failed: %w", err)
	}

	a.agentInfo = agentInfo
	a.logger.Info().Str("id", agentInfo.ID).Msg("Agent registered")

	// Initialize control synchronization service
	a.controlSyncService = NewControlSyncService(a.apiClient, agentInfo.ID, "default-org")

	// Synchronize controls from configuration
	syncResult, err := a.controlSyncService.SyncControlsFromConfig(ctx, &a.config)
	if err != nil {
		a.logger.Warn().Err(err).Msg("Control synchronization failed")
	} else {
		a.logger.Info().
			Int("matched", len(syncResult.MatchedControls)).
			Int("updated", syncResult.UpdatedControls).
			Int("new", len(syncResult.NewControls)).
			Msg("Control synchronization completed")
	}

	return nil
}

// Start starts the agent with the specified number of workers
func (a *Agent) Start(ctx context.Context) error {
	// In standalone mode, we don't need agent registration
	if a.config.Offline.Mode != "standalone" && a.agentInfo == nil {
		return fmt.Errorf("agent not registered - call Register() first")
	}

	// Determine number of workers to spawn
	spawnCount := a.config.Spawn
	if spawnCount <= 0 {
		spawnCount = 1
	}

	a.logger.Info().Int("workers", spawnCount).Msg("Starting workers")

	// Create and start workers (following Buildkite's spawn pattern)
	var wg sync.WaitGroup
	for i := 0; i < spawnCount; i++ {
		workerConfig := AgentWorkerConfig{
			Debug:              a.config.LogLevel == "debug",
			SpawnIndex:         i,
			AgentConfiguration: a.config,
			MaxConcurrency:     a.config.MaxConcurrency,
			PollInterval:       a.config.PollInterval,
			HeartbeatInterval:  60 * time.Second, // Default heartbeat
		}

		worker := NewAgentWorker(a.logger, a.agentInfo, a.apiClient, workerConfig)
		a.workers = append(a.workers, worker)

		wg.Add(1)
		go func(w *AgentWorker, index int) {
			defer wg.Done()

			a.logger.Info().Int("worker", index).Msg("Starting worker")
			if err := w.Start(ctx); err != nil {
				a.logger.Error().Err(err).Int("worker", index).Msg("Worker failed")
			}
		}(worker, i)
	}

	// Wait for all workers or stop signal
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
		a.logger.Info().Msg("Context cancelled")
		a.Stop()
		return ctx.Err()
	case <-a.stop:
		a.logger.Info().Msg("Stop signal received")
		return nil
	case <-done:
		a.logger.Info().Msg("All workers finished")
		return nil
	}
}

// Stop gracefully stops the agent and all workers
func (a *Agent) Stop() {
	a.stopOnce.Do(func() {
		a.logger.Info().Msg("Stopping agent")

		// Stop all workers
		for i, worker := range a.workers {
			a.logger.Debug().Int("worker", i).Msg("Stopping worker")
			worker.Stop()
		}

		close(a.stop)
	})
}

// SyncControls manually triggers control synchronization
func (a *Agent) SyncControls(ctx context.Context) (*SyncResult, error) {
	if a.controlSyncService == nil {
		return nil, fmt.Errorf("control sync service not initialized")
	}

	a.logger.Info().Msg("Manual control synchronization requested")
	return a.controlSyncService.SyncControlsFromConfig(ctx, &a.config)
}

// GetControlSyncStatus returns the current control synchronization status
func (a *Agent) GetControlSyncStatus(ctx context.Context) (*api.ControlComplianceStatus, error) {
	if a.controlSyncService == nil {
		return nil, fmt.Errorf("control sync service not initialized")
	}

	return a.controlSyncService.GetSyncStatus(ctx)
}

// GetStats returns statistics for the agent and all workers
func (a *Agent) GetStats() map[string]any {
	stats := map[string]any{
		"agent_id":     a.agentInfo.ID,
		"agent_name":   a.config.AgentName,
		"worker_count": len(a.workers),
		"workers":      make([]map[string]any, 0, len(a.workers)),
	}

	// Collect stats from all workers
	for i, worker := range a.workers {
		workerStats := worker.GetStats()
		workerStats["index"] = i
		stats["workers"] = append(stats["workers"].([]map[string]any), workerStats)
	}

	return stats
}
