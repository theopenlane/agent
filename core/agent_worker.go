package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/scheduler"
	"github.com/theopenlane/agent/internal/storage"
	"github.com/theopenlane/core/common/enums"
)

// AgentWorkerConfig contains configuration for an agent worker
type AgentWorkerConfig struct {
	// SpawnIndex is the zero-based index of this worker among all spawned workers
	SpawnIndex int
	// AgentConfiguration is the full agent configuration loaded from the config file
	AgentConfiguration *config.Config
	// AssignedChecks is the subset of checks assigned to this worker
	AssignedChecks []*config.Check
	// HasAssignedChecks indicates whether AssignedChecks should be used as-is
	HasAssignedChecks bool
	// MaxConcurrency is the maximum number of checks this worker may run simultaneously
	MaxConcurrency int
	// PollInterval is the interval at which the local scheduler is scanned for due checks
	PollInterval time.Duration
}

// agentWorkerState represents the current state of an agent worker.
type agentWorkerState string

const (
	agentWorkerStateIdle agentWorkerState = "idle"
	agentWorkerStateBusy agentWorkerState = "busy"
)

// agentStats tracks worker statistics.
type agentStats struct {
	sync.Mutex

	// Total checks executed
	totalChecks int64

	// Successful vs failed checks
	successfulChecks, failedChecks int64
}

// AgentWorker executes local scheduled checks
type AgentWorker struct {
	stats agentStats

	// API client for platform operations used by control validation/evidence upload
	apiClient *api.GraphQLClient

	// Agent configuration
	agentConfiguration *config.Config

	// Stop controls
	stopOnce sync.Once
	stop     chan struct{}

	// Worker index
	spawnIndex int

	// Current state management
	stateMtx       sync.Mutex
	state          agentWorkerState
	currentCheckID string
	activeChecks   map[string]*ComplianceCheckController
	maxConcurrency int

	// Interval for local scheduler scans
	pollInterval time.Duration

	// Local check scheduler
	scheduler *scheduler.Scheduler
	checks    []*config.Check

	// Storage for check results and evidence
	storage         storage.Storage
	evidenceService *storage.EvidenceService

	// Worker start time
	startTime time.Time
}

// NewAgentWorker creates a new local-scheduler worker
func NewAgentWorker(apiClient *api.GraphQLClient, workerConfig AgentWorkerConfig) *AgentWorker {
	// Create scheduler for local checks
	sched := scheduler.NewScheduler()

	assignedChecks := workerConfig.AssignedChecks
	if !workerConfig.HasAssignedChecks {
		assignedChecks = make([]*config.Check, len(workerConfig.AgentConfiguration.Checks))
		for i := range workerConfig.AgentConfiguration.Checks {
			assignedChecks[i] = &workerConfig.AgentConfiguration.Checks[i]
		}
	}

	// Add assigned checks to scheduler
	for _, check := range assignedChecks {
		if err := sched.AddCheck(check); err != nil {
			log.Error().Err(err).Str("check_name", check.Name).Msg("Failed to schedule check")
		}
	}

	// Initialize storage
	storageSystem, err := storage.NewStorageFromConfig(workerConfig.AgentConfiguration)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create storage system, results may not persist")

		storageSystem = nil
	}

	// Initialize evidence service
	var evidenceService *storage.EvidenceService

	if storageSystem != nil {
		storageConfig := storage.ConfigFromAgentConfig(workerConfig.AgentConfiguration)
		evidenceService = storage.NewEvidenceServiceFromConfig(storageConfig)
	}

	worker := &AgentWorker{
		apiClient:          apiClient,
		agentConfiguration: workerConfig.AgentConfiguration,
		checks:             assignedChecks,
		stop:               make(chan struct{}),
		spawnIndex:         workerConfig.SpawnIndex,
		state:              agentWorkerStateIdle,
		activeChecks:       make(map[string]*ComplianceCheckController),
		maxConcurrency:     workerConfig.MaxConcurrency,
		pollInterval:       workerConfig.PollInterval,
		scheduler:          sched,
		storage:            storageSystem,
		evidenceService:    evidenceService,
		startTime:          time.Now(),
	}

	log.Info().Str("mode", string(workerConfig.AgentConfiguration.Offline.Mode)).Msg("Storage manager initialized")

	return worker
}

// Start begins the local scheduler loop
func (w *AgentWorker) Start(ctx context.Context) error {
	log.Info().Int("worker", w.spawnIndex).Msg("Starting worker")

	if w.storage != nil {
		if err := w.storage.Health(); err != nil {
			log.Warn().Err(err).Msg("Storage health check failed")
		}
	}

	if len(w.checks) > 0 {
		go w.schedulerRoutine(ctx)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-w.stop:
		return nil
	}
}

// Stop gracefully stops the worker
func (w *AgentWorker) Stop() {
	w.stopOnce.Do(func() {
		log.Info().Int("worker", w.spawnIndex).Msg("Stopping worker")

		if w.storage != nil {
			_ = w.storage.Close()
		}

		close(w.stop)
	})
}

// schedulerRoutine handles local check scheduling based on cron expressions.
func (w *AgentWorker) schedulerRoutine(ctx context.Context) {
	interval := w.pollInterval
	if interval <= 0 {
		interval = 30 * time.Second // nolint:mnd
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			if err := w.executeScheduledChecks(ctx); err != nil {
				log.Error().Err(err).Msg("Error executing scheduled checks")
			}
		}
	}
}

// executeScheduledChecks executes any local checks that are due.
func (w *AgentWorker) executeScheduledChecks(ctx context.Context) error {
	if w.getCurrentConcurrency() >= w.maxConcurrency {
		log.Debug().Msg("At max concurrency, skipping local checks")
		return nil
	}

	dueChecks := w.scheduler.GetDueChecks()
	if len(dueChecks) == 0 {
		return nil
	}

	log.Info().Int("count", len(dueChecks)).Msg("Found local checks due")

	for _, check := range dueChecks {
		if w.getCurrentConcurrency() >= w.maxConcurrency {
			log.Debug().Msg("Max concurrency reached, deferring checks")
			break
		}

		w.scheduler.MarkRunning(check.Name)

		go w.executeLocalCheck(ctx, check)
	}

	return nil
}

// executeLocalCheck executes a single local compliance check from the config.
func (w *AgentWorker) executeLocalCheck(ctx context.Context, check *config.Check) {
	checkID := fmt.Sprintf("local-%s-%d", check.Name, time.Now().UnixNano())
	log.Info().Str("check", check.Name).Msg("Starting local check")

	controller := NewComplianceCheckController(w.apiClient, w.agentConfiguration.AgentName, w.storage, w.evidenceService)
	controller.SetCurrentCheck(check.Name)

	w.stateMtx.Lock()

	w.activeChecks[checkID] = controller
	if len(w.activeChecks) == 1 {
		w.state = agentWorkerStateBusy
		w.currentCheckID = checkID
	}

	w.stateMtx.Unlock()

	result, execErr := controller.ExecuteCheck(ctx, check)

	if schedErr := w.scheduler.MarkCompleted(check.Name); schedErr != nil {
		log.Error().Err(schedErr).Str("check", check.Name).Msg("Failed to mark check completed")
	}

	w.stateMtx.Lock()
	delete(w.activeChecks, checkID)

	if len(w.activeChecks) == 0 {
		w.state = agentWorkerStateIdle
		w.currentCheckID = ""
	}

	w.stateMtx.Unlock()

	w.stats.Lock()

	w.stats.totalChecks++
	if checkExecutionFailed(result, execErr) {
		w.stats.failedChecks++

		if check.ContinueOnError {
			log.Warn().Err(execErr).Str("check", check.Name).Str("status", resultStatus(result)).Msg("Local check failed (continuing)")
		} else {
			log.Error().Err(execErr).Str("check", check.Name).Str("status", resultStatus(result)).Msg("Local check failed")
		}
	} else {
		w.stats.successfulChecks++

		log.Info().Str("check", check.Name).Msg("Local check completed")
	}

	w.stats.Unlock()
}

func checkExecutionFailed(result *config.Result, execErr error) bool {
	if execErr != nil {
		return true
	}

	if result == nil {
		return true
	}

	return result.Status != enums.JobExecutionStatusSuccess
}

func resultStatus(result *config.Result) string {
	if result == nil {
		return ""
	}

	return result.Status.String()
}

// getCurrentConcurrency returns the current number of active checks.
func (w *AgentWorker) getCurrentConcurrency() int {
	w.stateMtx.Lock()
	defer w.stateMtx.Unlock()

	return len(w.activeChecks)
}

// getState returns the current worker state.
func (w *AgentWorker) getState() agentWorkerState {
	w.stateMtx.Lock()
	defer w.stateMtx.Unlock()

	return w.state
}

// GetStats returns current worker statistics
func (w *AgentWorker) GetStats() map[string]any {
	w.stats.Lock()
	defer w.stats.Unlock()

	stats := map[string]any{
		"state":             string(w.getState()),
		"spawn_index":       w.spawnIndex,
		"uptime":            time.Since(w.startTime).String(),
		"active_checks":     w.getCurrentConcurrency(),
		"max_concurrency":   w.maxConcurrency,
		"total_checks":      w.stats.totalChecks,
		"successful_checks": w.stats.successfulChecks,
		"failed_checks":     w.stats.failedChecks,
		"poll_interval":     w.pollInterval.String(),
		"operation_mode":    string(w.agentConfiguration.Offline.Mode),
	}

	if w.storage != nil {
		stats["storage"] = "available"
	}

	return stats
}
