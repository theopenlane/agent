package core

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/connectivity"
	"github.com/theopenlane/agent/internal/constants"
	"github.com/theopenlane/agent/internal/retry"
	"github.com/theopenlane/agent/internal/scheduler"
	"github.com/theopenlane/agent/internal/storage"
	"github.com/theopenlane/go-client/graphclient"
)

// AgentWorkerConfig contains configuration for an agent worker
type AgentWorkerConfig struct {
	// Whether to enable debug logging
	Debug bool

	// The index of this agent worker (for multiple workers)
	SpawnIndex int

	// The agent configuration from CLI
	AgentConfiguration *config.Config

	// Assigned checks for this worker
	AssignedChecks []*config.Check

	// Whether AssignedChecks should be used as-is.
	HasAssignedChecks bool

	// Whether this worker sends heartbeats for the shared runner
	EnableHeartbeat bool

	// Whether this worker polls remote work
	EnableRemotePolling bool

	// Maximum number of concurrent checks
	MaxConcurrency int

	// Poll interval for checking for work
	PollInterval time.Duration

	// Heartbeat interval for health reporting
	HeartbeatInterval time.Duration
}

// agentWorkerState represents the current state of an agent worker
type agentWorkerState string

const (
	agentWorkerStateIdle agentWorkerState = "idle"
	agentWorkerStateBusy agentWorkerState = "busy"
)

// agentStats tracks agent statistics
type agentStats struct {
	sync.Mutex

	// Tracks the last successful heartbeat and ping
	lastPing, lastHeartbeat time.Time

	// The last error that occurred during heartbeat, or nil if it was successful
	lastHeartbeatError error

	// Total checks executed
	totalChecks int64

	// Successful vs failed checks
	successfulChecks, failedChecks int64
}

// AgentWorker is the core worker that polls for and executes compliance checks
type AgentWorker struct {
	stats agentStats

	// The GraphQL client for communicating with Openlane
	apiClient *api.GraphQLClient

	// The agent configuration
	agentConfiguration *config.Config

	// The registered agent information
	agentInfo *graphclient.JobRunner

	// Whether to enable debug logging
	debug bool

	// Stop controls
	stopOnce sync.Once
	stop     chan struct{}

	// The index of this agent worker
	spawnIndex int

	// Current state management
	stateMtx       sync.Mutex
	state          agentWorkerState
	currentCheckID string
	activeChecks   map[string]*ComplianceCheckController
	maxConcurrency int

	// Timing intervals
	pollInterval      time.Duration
	heartbeatInterval time.Duration

	// Local check scheduler
	scheduler *scheduler.Scheduler
	checks    []*config.Check

	// Retry manager for resilient operations
	retryManager *retry.Manager

	// Unified storage system for handling check results and evidence
	storage         storage.Storage
	evidenceService *storage.EvidenceService

	// JobTemplate ID mapping for check -> template ID
	jobTemplateIDs map[string]string
	templateMu     sync.RWMutex

	// Worker capability flags
	enableHeartbeat     bool
	enableRemotePolling bool

	// Remote work de-duplication state
	remoteMu       sync.Mutex
	remoteInFlight map[string]struct{}
	remoteLastRun  map[string]time.Time

	// The time when this agent worker started
	startTime time.Time
}

// NewAgentWorker creates a new agent worker
func NewAgentWorker(agentInfo *graphclient.JobRunner, apiClient *api.GraphQLClient, workerConfig AgentWorkerConfig) *AgentWorker {
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

	// Initialize unified storage system based on operation mode
	storageSystem, err := storage.NewStorageFromConfig(workerConfig.AgentConfiguration)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create storage system, agent will not function properly")
		// We should still return the worker, but it won't be able to store results
		storageSystem = nil
	}

	// Initialize evidence service
	var evidenceService *storage.EvidenceService

	if storageSystem != nil {
		storageConfig := storage.ConfigFromAgentConfig(workerConfig.AgentConfiguration)
		evidenceService = storage.NewEvidenceServiceFromConfig(storageConfig)
	}

	// Initialize retry manager with configured settings
	retryConfig := retry.Config{
		MaxAttempts:  workerConfig.AgentConfiguration.Retry.MaxAttempts,
		InitialDelay: workerConfig.AgentConfiguration.Retry.InitialDelay,
		MaxDelay:     workerConfig.AgentConfiguration.Retry.MaxDelay,
		Strategy:     workerConfig.AgentConfiguration.Retry.Strategy,
		Multiplier:   workerConfig.AgentConfiguration.Retry.Multiplier,
	}
	retryManager := retry.NewManager(retryConfig)

	worker := &AgentWorker{
		agentInfo:           agentInfo,
		apiClient:           apiClient,
		agentConfiguration:  workerConfig.AgentConfiguration,
		checks:              assignedChecks,
		debug:               workerConfig.Debug,
		stop:                make(chan struct{}),
		spawnIndex:          workerConfig.SpawnIndex,
		state:               agentWorkerStateIdle,
		activeChecks:        make(map[string]*ComplianceCheckController),
		maxConcurrency:      workerConfig.MaxConcurrency,
		pollInterval:        workerConfig.PollInterval,
		heartbeatInterval:   workerConfig.HeartbeatInterval,
		scheduler:           sched,
		storage:             storageSystem,
		evidenceService:     evidenceService,
		retryManager:        retryManager,
		jobTemplateIDs:      make(map[string]string),
		enableHeartbeat:     workerConfig.EnableHeartbeat,
		enableRemotePolling: workerConfig.EnableRemotePolling,
		remoteInFlight:      make(map[string]struct{}),
		remoteLastRun:       make(map[string]time.Time),
		startTime:           time.Now(),
	}

	log.Info().Str("mode", string(workerConfig.AgentConfiguration.Offline.Mode)).Msg("Storage manager initialized")

	return worker
}

// Start begins the agent worker's main loop
func (w *AgentWorker) Start(ctx context.Context) error {
	log.Info().Int("worker", w.spawnIndex).Msg("Starting worker")

	// Storage system is already initialized and ready to use
	if w.storage != nil {
		log.Info().Msg("Storage system ready")

		if err := w.storage.Health(); err != nil {
			log.Warn().Err(err).Msg("Storage health check failed")
		}
	}

	// Sync JobTemplates only when remote polling/control-plane mode is enabled.
	if w.enableRemotePolling && w.apiClient != nil && len(w.checks) > 0 {
		if err := w.syncJobTemplates(ctx); err != nil {
			log.Warn().Err(err).Msg("Failed to sync job templates, will retry later")
		}
	}

	// Start the heartbeat routine
	if w.enableHeartbeat {
		go w.heartbeatRoutine(ctx)
	}

	// Start the local check scheduler routine when this worker has assigned checks.
	if len(w.checks) > 0 {
		go w.schedulerRoutine(ctx)
	}

	// Start the main polling loop for remote work
	return w.pollLoop(ctx)
}

// Stop gracefully stops the agent worker
func (w *AgentWorker) Stop() {
	w.stopOnce.Do(func() {
		log.Info().Int("worker", w.spawnIndex).Msg("Stopping worker")

		// Close storage system
		if w.storage != nil {
			w.storage.Close()
		}

		close(w.stop)
	})
}

// pollLoop is the main polling loop for getting compliance work
func (w *AgentWorker) pollLoop(ctx context.Context) error {
	if !w.enableRemotePolling {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.stop:
			return nil
		}
	}

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-w.stop:
			return nil
		case <-ticker.C:
			if err := w.pollForWork(ctx); err != nil {
				log.Error().Err(err).Msg("Error polling for work")
				// Continue polling even if there's an error
			}
		}
	}
}

// pollForWork polls the Openlane platform for REMOTE scheduled jobs
func (w *AgentWorker) pollForWork(ctx context.Context) error {
	if !w.enableRemotePolling {
		return nil
	}

	// Skip remote polling if disabled in config
	if !w.agentConfiguration.EnableRemotePoll {
		log.Debug().Msg("Remote polling disabled in config, skipping")
		return nil
	}

	// Skip remote polling if no API client (standalone mode)
	if w.apiClient == nil {
		log.Debug().Msg("No API client available, skipping remote poll")
		return nil
	}

	// Update stats
	w.stats.Lock()
	w.stats.lastPing = time.Now()
	w.stats.Unlock()

	// Check if we have capacity for more work
	if w.getCurrentConcurrency() >= w.maxConcurrency {
		log.Debug().Msg("At max concurrency, skipping remote poll")
		return nil
	}

	// Poll for REMOTE scheduled jobs from platform with retry
	var scheduledJobs []*graphclient.ScheduledJob

	err := w.retryManager.ExecuteWithContext(ctx, func(ctx context.Context) error {
		var pollErr error

		scheduledJobs, pollErr = w.apiClient.PollForWork(ctx)

		return pollErr
	})
	if err != nil {
		return fmt.Errorf("failed to poll for remote work after retries: %w", err)
	}

	if len(scheduledJobs) == 0 {
		log.Debug().Msg("No remote scheduled jobs available")
		return nil
	}

	log.Info().Int("count", len(scheduledJobs)).Msg("Received remote jobs")

	// Execute scheduled jobs concurrently up to our limit
	for _, scheduledJob := range scheduledJobs {
		if w.getCurrentConcurrency() >= w.maxConcurrency {
			log.Debug().Msg("Max concurrency reached")
			break
		}

		if !w.shouldRunRemoteJob(scheduledJob) {
			continue
		}

		w.markRemoteJobRunning(scheduledJob.ID)

		// Start executing the remote scheduled job
		go w.executeScheduledJob(ctx, scheduledJob)
	}

	return nil
}

// schedulerRoutine handles LOCAL check scheduling based on cron expressions
func (w *AgentWorker) schedulerRoutine(ctx context.Context) {
	// Check for due local checks every 30 seconds
	ticker := time.NewTicker(30 * time.Second) // nolint:mnd
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

// executeScheduledChecks executes any local checks that are due
func (w *AgentWorker) executeScheduledChecks(ctx context.Context) error {
	// Check if we have capacity for more work
	if w.getCurrentConcurrency() >= w.maxConcurrency {
		log.Debug().Msg("At max concurrency, skipping local checks")
		return nil
	}

	// Get due checks from scheduler
	dueChecks := w.scheduler.GetDueChecks()
	if len(dueChecks) == 0 {
		return nil
	}

	log.Info().Int("count", len(dueChecks)).Msg("Found local checks due")

	// Execute due checks
	for _, check := range dueChecks {
		if w.getCurrentConcurrency() >= w.maxConcurrency {
			log.Debug().Msg("Max concurrency reached, deferring checks")
			break
		}

		// Mark as running to prevent duplicate execution
		w.scheduler.MarkRunning(check.Name)

		// Start executing the local check
		go w.executeLocalCheck(ctx, check)
	}

	return nil
}

// executeScheduledJob executes a single REMOTE scheduled job from the platform
func (w *AgentWorker) executeScheduledJob(ctx context.Context, scheduledJob *graphclient.ScheduledJob) {
	startedAt := time.Now()
	defer w.markRemoteJobFinished(scheduledJob.ID, startedAt)

	checkID := fmt.Sprintf("remote-%s-%d", scheduledJob.ID, time.Now().UnixNano())

	log.Info().Str("job_id", scheduledJob.ID).Msg("Starting remote job")

	// Create a check controller
	agentID := ""
	if w.agentInfo != nil {
		agentID = w.agentInfo.ID
	}

	controller := NewComplianceCheckController(w.apiClient, agentID, w.storage, w.evidenceService)

	// Add to active checks
	w.stateMtx.Lock()

	w.activeChecks[checkID] = controller
	if len(w.activeChecks) == 1 {
		w.state = agentWorkerStateBusy
		w.currentCheckID = checkID
	}

	w.stateMtx.Unlock()

	// Execute the scheduled job using the unified execution method
	_, err := controller.ExecuteScheduledJob(ctx, scheduledJob)

	// Remove from active checks
	w.stateMtx.Lock()
	delete(w.activeChecks, checkID)

	if len(w.activeChecks) == 0 {
		w.state = agentWorkerStateIdle
		w.currentCheckID = ""
	}

	w.stateMtx.Unlock()

	// Update stats
	w.stats.Lock()

	w.stats.totalChecks++
	if err != nil {
		w.stats.failedChecks++

		log.Error().Err(err).Str("job_id", scheduledJob.ID).Msg("Scheduled job failed")
	} else {
		w.stats.successfulChecks++

		log.Info().Str("job_id", scheduledJob.ID).Msg("Scheduled job completed")
	}

	w.stats.Unlock()
}

// executeLocalCheck executes a single LOCAL compliance check from the config
func (w *AgentWorker) executeLocalCheck(ctx context.Context, check *config.Check) {
	checkID := fmt.Sprintf("local-%s-%d", check.Name, time.Now().UnixNano())

	log.Info().Str("check", check.Name).Msg("Starting local check")

	// Get agent ID (empty string in standalone mode)
	agentID := ""
	if w.agentInfo != nil {
		agentID = w.agentInfo.ID
	}

	// Create a check controller
	controller := NewComplianceCheckController(w.apiClient, agentID, w.storage, w.evidenceService)
	controller.SetCurrentCheck(check.Name, "")

	// Add to active checks
	w.stateMtx.Lock()

	w.activeChecks[checkID] = controller
	if len(w.activeChecks) == 1 {
		w.state = agentWorkerStateBusy
		w.currentCheckID = checkID
	}

	w.stateMtx.Unlock()

	// Execute the local check using the unified execution method
	_, execErr := controller.ExecuteCheck(ctx, check)

	// Mark as completed in scheduler
	if schedErr := w.scheduler.MarkCompleted(check.Name); schedErr != nil {
		log.Error().Err(schedErr).Str("check", check.Name).Msg("Failed to mark check completed")
	}

	// Remove from active checks
	w.stateMtx.Lock()
	delete(w.activeChecks, checkID)

	if len(w.activeChecks) == 0 {
		w.state = agentWorkerStateIdle
		w.currentCheckID = ""
	}

	w.stateMtx.Unlock()

	// Update stats
	w.stats.Lock()

	w.stats.totalChecks++
	if execErr != nil {
		w.stats.failedChecks++
		// Log at appropriate level based on continue_on_error setting
		if check.ContinueOnError {
			log.Warn().Err(execErr).Str("check", check.Name).Msg("Local check failed (continuing)")
		} else {
			log.Error().Err(execErr).Str("check", check.Name).Msg("Local check failed")
		}
	} else {
		w.stats.successfulChecks++

		log.Info().Str("check", check.Name).Msg("Local check completed")
	}

	w.stats.Unlock()
}

// reportResult reports the compliance check result using the configured storage
func (w *AgentWorker) reportResult(result *config.Result) error {
	if w.storage == nil {
		log.Error().Msg("No storage system available, cannot store result")
		return ErrNoStorageSystemAvailable
	}

	return w.storage.StoreResult(result)
}

// heartbeatRoutine sends periodic heartbeats to the platform
func (w *AgentWorker) heartbeatRoutine(ctx context.Context) {
	ticker := time.NewTicker(w.heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			// Use retry manager for heartbeat with network retry conditions
			conditionalRetry := retry.NewConditionalManager(w.retryManager, retry.RetryOnNetworkError)
			if err := conditionalRetry.ExecuteWithContext(ctx, func(ctx context.Context) error {
				return w.sendHeartbeat(ctx)
			}); err != nil {
				w.stats.Lock()
				w.stats.lastHeartbeatError = err
				w.stats.Unlock()
				log.Error().Err(err).Msg("Heartbeat failed after retries")
			} else {
				w.stats.Lock()
				w.stats.lastHeartbeat = time.Now()
				w.stats.lastHeartbeatError = nil
				w.stats.Unlock()
				log.Debug().Msg("Heartbeat sent successfully")
			}
		}
	}
}

// sendHeartbeat sends a heartbeat to the platform by updating JobRunner with comprehensive status
func (w *AgentWorker) sendHeartbeat(ctx context.Context) error {
	// Skip heartbeat if no API client (standalone mode)
	if w.apiClient == nil {
		log.Debug().Msg("No API client available, skipping heartbeat")
		return nil
	}

	// Skip heartbeat if no agent info (standalone mode)
	if w.agentInfo == nil {
		log.Debug().Msg("No agent info available, skipping heartbeat")
		return nil
	}

	log.Debug().Msg("Sending heartbeat")

	// Collect current system information
	now := time.Now()

	// Access stats directly for better performance and type safety
	w.stats.Lock()
	totalChecks := w.stats.totalChecks
	successfulChecks := w.stats.successfulChecks
	failedChecks := w.stats.failedChecks
	w.stats.Unlock()

	// Build comprehensive agent status
	status := api.AgentStatus{
		Status:          "active",
		LastPing:        now,
		Version:         constants.FullVersion(),
		IPAddress:       connectivity.GetPreferredIPAddress(ctx),
		ActiveChecks:    w.getCurrentConcurrency(),
		TotalExecutions: totalChecks,
		SuccessfulRuns:  successfulChecks,
		FailedRuns:      failedChecks,
		SystemInfo: api.SystemInfo{
			OS: runtime.GOOS,
		},
	}

	// Use the comprehensive UpdateAgentStatus method
	if err := w.apiClient.UpdateAgentStatus(ctx, w.agentInfo.ID, status); err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

	log.Debug().Time("last_seen", now).Str("version", status.Version).Str("ip_address", status.IPAddress).Msg("Heartbeat sent with comprehensive status")

	return nil
}

// getCurrentConcurrency returns the current number of active checks
func (w *AgentWorker) getCurrentConcurrency() int {
	w.stateMtx.Lock()
	defer w.stateMtx.Unlock()

	return len(w.activeChecks)
}

// getState returns the current worker state
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
		"state":                string(w.getState()),
		"spawn_index":          w.spawnIndex,
		"uptime":               time.Since(w.startTime).String(),
		"active_checks":        w.getCurrentConcurrency(),
		"max_concurrency":      w.maxConcurrency,
		"total_checks":         w.stats.totalChecks,
		"successful_checks":    w.stats.successfulChecks,
		"failed_checks":        w.stats.failedChecks,
		"last_ping":            w.stats.lastPing,
		"last_heartbeat":       w.stats.lastHeartbeat,
		"last_heartbeat_error": w.stats.lastHeartbeatError,
		"operation_mode":       string(w.agentConfiguration.Offline.Mode),
	}

	// Storage is available
	if w.storage != nil {
		stats["storage"] = "available"
	}

	return stats
}

// syncJobTemplates synchronizes JobTemplates for all configured checks
func (w *AgentWorker) syncJobTemplates(ctx context.Context) error {
	if len(w.checks) == 0 {
		return nil
	}

	log.Info().Int("count", len(w.checks)).Int("worker", w.spawnIndex).Msg("Syncing job templates for assigned checks")

	templateIDs, err := w.apiClient.SyncJobTemplates(ctx, w.checks)
	if err != nil {
		return fmt.Errorf("failed to sync job templates: %w", err)
	}

	// Store template IDs
	w.templateMu.Lock()
	w.jobTemplateIDs = templateIDs
	w.templateMu.Unlock()

	log.Info().Int("synced", len(templateIDs)).Msg("Job templates synchronized")

	return nil
}

// getJobTemplateID retrieves the JobTemplate ID for a check
func (w *AgentWorker) getJobTemplateID(checkName string) string {
	w.templateMu.RLock()
	defer w.templateMu.RUnlock()

	return w.jobTemplateIDs[checkName]
}

// createScheduledJob creates a ScheduledJob for a check execution
func (w *AgentWorker) createScheduledJob(ctx context.Context, check *config.Check, isManual bool) (string, error) {
	if w.apiClient == nil {
		return "", nil // Standalone mode
	}

	templateID := w.getJobTemplateID(check.Name)
	if templateID == "" {
		return "", fmt.Errorf("%w: %s", ErrJobTemplateIDNotFound, check.Name)
	}

	// Set active based on whether this is a manual or scheduled execution
	active := !isManual

	input := graphclient.CreateScheduledJobInput{
		JobTemplateID: templateID,
		Active:        &active,
	}

	// Add agent as the runner if we have agent info
	if w.agentInfo != nil {
		input.JobRunnerID = &w.agentInfo.ID
	}

	resp, err := w.apiClient.GetClient().CreateScheduledJob(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to create scheduled job: %w", err)
	}

	log.Debug().Str("check", check.Name).Str("scheduled_job_id", resp.CreateScheduledJob.ScheduledJob.ID).Bool("manual", isManual).Msg("Created scheduled job")

	return resp.CreateScheduledJob.ScheduledJob.ID, nil
}

func (w *AgentWorker) shouldRunRemoteJob(scheduledJob *graphclient.ScheduledJob) bool {
	now := time.Now()

	w.remoteMu.Lock()
	defer w.remoteMu.Unlock()

	if _, exists := w.remoteInFlight[scheduledJob.ID]; exists {
		return false
	}

	lastRun, hasLastRun := w.remoteLastRun[scheduledJob.ID]
	cronExpr := ""
	if scheduledJob.Cron != nil {
		cronExpr = strings.TrimSpace(*scheduledJob.Cron)
	}

	if cronExpr == "" {
		if !hasLastRun {
			return true
		}

		return now.Sub(lastRun) >= w.pollInterval
	}

	schedule, err := parseRemoteCronSchedule(cronExpr)
	if err != nil {
		log.Warn().Err(err).Str("job_id", scheduledJob.ID).Str("cron", cronExpr).Msg("Invalid remote cron expression, falling back to poll interval")
		if !hasLastRun {
			return true
		}

		return now.Sub(lastRun) >= w.pollInterval
	}

	if !hasLastRun {
		windowStart := now.Add(-w.pollInterval)
		return !schedule.Next(windowStart).After(now)
	}

	return !schedule.Next(lastRun).After(now)
}

func (w *AgentWorker) markRemoteJobRunning(jobID string) {
	w.remoteMu.Lock()
	defer w.remoteMu.Unlock()

	w.remoteInFlight[jobID] = struct{}{}
}

func (w *AgentWorker) markRemoteJobFinished(jobID string, runTime time.Time) {
	w.remoteMu.Lock()
	defer w.remoteMu.Unlock()

	delete(w.remoteInFlight, jobID)
	w.remoteLastRun[jobID] = runTime
}

func parseRemoteCronSchedule(cronExpr string) (cron.Schedule, error) {
	fields := strings.Fields(cronExpr)
	if len(fields) != 5 && len(fields) != 6 { // nolint:mnd
		return nil, fmt.Errorf("%w: %s", scheduler.ErrInvalidCronExpression, cronExpr)
	}

	var parser cron.Parser
	if len(fields) == 6 { //nolint:mnd
		parser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	} else {
		parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	}

	schedule, err := parser.Parse(cronExpr)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", scheduler.ErrInvalidCronExpression, cronExpr)
	}

	return schedule, nil
}

// Helper functions for data conversion
