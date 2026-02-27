package core

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/config"
)

// AgentOption is a functional option for Agent
type AgentOption func(*Agent)

// Agent represents the main compliance agent (similar to Buildkite's Agent)
type Agent struct {
	config *config.Config

	// API client for communicating with Openlane
	apiClient *api.GraphQLClient

	// Workers
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
		client, err := api.NewGraphQLClientFromConfig(a.config, nil)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrFailedToCreateAPIClient, err)
		}

		apiClient = client
	}

	a.apiClient = apiClient

	return a, nil
}

// Start starts the agent with the specified number of workers
func (a *Agent) Start(ctx context.Context) error {
	// Determine number of workers to spawn
	spawnCount := a.config.Spawn
	if spawnCount <= 0 {
		spawnCount = 1
	}

	log.Info().Int("workers", spawnCount).Msg("Starting workers")

	assignedChecks := partitionEnabledChecks(a.config.Checks, spawnCount)
	// Create and start workers
	var wg sync.WaitGroup

	for i := 0; i < spawnCount; i++ {
		workerConfig := AgentWorkerConfig{
			SpawnIndex:         i,
			AgentConfiguration: a.config,
			AssignedChecks:     assignedChecks[i],
			HasAssignedChecks:  true,
			MaxConcurrency:     a.config.MaxConcurrency,
			PollInterval:       a.config.PollInterval,
		}

		worker := NewAgentWorker(a.apiClient, workerConfig)
		a.workers = append(a.workers, worker)

		wg.Add(1)

		go func(w *AgentWorker, index int) {
			defer wg.Done()

			log.Info().Int("worker", index).Msg("Starting worker")

			if err := w.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
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

func partitionEnabledChecks(checks []config.Check, workers int) [][]*config.Check {
	partitions := make([][]*config.Check, workers)
	if workers <= 0 {
		return partitions
	}

	enabledChecks := make([]*config.Check, 0, len(checks))
	for i := range checks {
		if checks[i].Enabled {
			enabledChecks = append(enabledChecks, &checks[i])
		}
	}

	for idx, check := range enabledChecks {
		workerIndex := idx % workers
		partitions[workerIndex] = append(partitions[workerIndex], check)
	}

	return partitions
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
