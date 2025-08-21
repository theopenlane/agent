package core

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/constants"
	"github.com/theopenlane/agent/internal/identity"
	"github.com/theopenlane/core/pkg/openlaneclient"
)

// AgentOption is a functional option for Agent
type AgentOption func(*Agent)

// Agent represents the main compliance agent (similar to Buildkite's Agent)
type Agent struct {
	config *config.Config

	// API client for communicating with Openlane
	apiClient *api.GraphQLClient

	// Hardware ID detector
	hardwareDetector *identity.Detector

	// Agent registration information
	agentInfo *openlaneclient.JobRunner

	// Workers (can spawn multiple like Buildkite)
	workers []*AgentWorker

	// Control channels
	stopOnce sync.Once
	stop     chan struct{}
}

// NewAgent creates a new compliance agent with functional options
func NewAgent(opts ...AgentOption) (*Agent, error) {

	a := &Agent{
		config: config.DefaultConfig(),
		stop:   make(chan struct{}),
	}
	for _, opt := range opts {
		opt(a)
	}

	var apiClient *api.GraphQLClient
	// Only create API client if not in standalone mode
	if a.config.Offline.Mode != "standalone" {
		client, err := api.NewGraphQLClient(a.config.APIURL, a.config.RegistrationToken)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrFailedToCreateAPIClient, err)
		}
		apiClient = client
	}
	a.apiClient = apiClient
	// Initialize hardware detector
	a.hardwareDetector = identity.NewDetector()
	return a, nil
}

// Register registers the agent with the Openlane platform
func (a *Agent) Register(ctx context.Context) error {
	// Skip registration in standalone mode
	if a.config.Offline.Mode == "standalone" {
		log.Info().Msg("Standalone mode - skipping agent registration")
		return nil
	}

	if a.apiClient == nil {
		return ErrAPIClientNotAvailable
	}

	log.Info().Str("name", a.config.AgentName).Msg("Registering agent")

	// Get system information
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Get hardware ID for registration
	hardwareID := ""

	if a.config.Identity.Enabled {
		if a.config.Identity.OverrideHardwareID != "" {
			hardwareID = a.config.Identity.OverrideHardwareID
			log.Info().Str("hardware_id", hardwareID).Msg("Using override hardware ID")
		} else {
			hardwareID = a.hardwareDetector.GetHardwareID()
			log.Info().Str("hardware_id", hardwareID).Msg("Detected hardware ID for registration")
		}
	} else {
		log.Info().Msg("Hardware ID detection disabled")
	}

	// Create registration request (following Buildkite's pattern)
	regReq := api.JobRunnerRegistration{
		Name:       a.config.AgentName,
		IPAddress:  "192.168.1.100", // Use non-loopback IP for testing
		HardwareID: hardwareID,
		Version:    constants.AgentVersion,
		Platform:   runtime.GOOS,
		Hostname:   hostname,
		Tags:       []string{"compliance", "automated"},
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
		return fmt.Errorf("%w: %w", ErrAgentRegistrationFailed, err)
	}

	a.agentInfo = agentInfo
	log.Info().Str("id", agentInfo.ID).Msg("Agent registered")

	return nil
}

// Start starts the agent with the specified number of workers
func (a *Agent) Start(ctx context.Context) error {
	// In standalone mode, we don't need agent registration
	if a.config.Offline.Mode != "standalone" && a.agentInfo == nil {
		return ErrAgentNotRegistered
	}

	// Determine number of workers to spawn
	spawnCount := a.config.Spawn
	if spawnCount <= 0 {
		spawnCount = 1
	}

	log.Info().Int("workers", spawnCount).Msg("Starting workers")

	// Create and start workers (following Buildkite's spawn pattern)
	var wg sync.WaitGroup

	for i := 0; i < spawnCount; i++ {
		workerConfig := AgentWorkerConfig{
			Debug:              a.config.LogLevel == "debug",
			SpawnIndex:         i,
			AgentConfiguration: a.config, // Make sure AgentWorkerConfig expects *config.Config
			MaxConcurrency:     a.config.MaxConcurrency,
			PollInterval:       a.config.PollInterval,
			HeartbeatInterval:  60 * time.Second, // Default heartbeat
		}

		worker := NewAgentWorker(a.agentInfo, a.apiClient, workerConfig)
		a.workers = append(a.workers, worker)

		wg.Add(1)

		go func(w *AgentWorker, index int) {
			defer wg.Done()

			log.Info().Int("worker", index).Msg("Starting worker")

			if err := w.Start(ctx); err != nil {
				log.Error().Err(err).Int("worker", index).Msg("Worker failed")
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
		log.Info().Msg("Context cancelled")
		a.Stop()

		return ctx.Err()
	case <-a.stop:
		log.Info().Msg("Stop signal received")
		return nil
	case <-done:
		log.Info().Msg("All workers finished")
		return nil
	}
}

// Stop gracefully stops the agent and all workers
func (a *Agent) Stop() {
	a.stopOnce.Do(func() {
		log.Info().Msg("Stopping agent")

		// Stop all workers
		for i, worker := range a.workers {
			log.Debug().Int("worker", i).Msg("Stopping worker")
			worker.Stop()
		}

		close(a.stop)
	})
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
