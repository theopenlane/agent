package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/internal/config"
	"github.com/theopenlane/agent/internal/scheduler"
	"github.com/theopenlane/core/pkg/models"
	"github.com/theopenlane/core/pkg/openlaneclient"
)

// AgentWorkerConfig contains configuration for an agent worker
type AgentWorkerConfig struct {
	// Whether to enable debug logging
	Debug bool

	// The index of this agent worker (for multiple workers)
	SpawnIndex int

	// The agent configuration from CLI
	AgentConfiguration config.Config

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

	// The logger instance to use
	logger zerolog.Logger

	// The agent configuration
	agentConfiguration config.Config

	// The registered agent information
	agentInfo *openlaneclient.JobRunner

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

	// The time when this agent worker started
	startTime time.Time
}

// NewAgentWorker creates a new agent worker
func NewAgentWorker(l zerolog.Logger, agentInfo *openlaneclient.JobRunner, apiClient *api.GraphQLClient, config AgentWorkerConfig) *AgentWorker {
	// Create scheduler for local checks
	sched := scheduler.NewScheduler(l)

	// Add local checks to scheduler
	for i := range config.AgentConfiguration.Checks {
		check := &config.AgentConfiguration.Checks[i]
		if err := sched.AddCheck(check); err != nil {
			l.Error().Err(err).Str("check_name", check.Name).Msg("Failed to schedule check")
		}
	}

	return &AgentWorker{
		logger:             l,
		agentInfo:          agentInfo,
		apiClient:          apiClient,
		agentConfiguration: config.AgentConfiguration,
		debug:              config.Debug,
		stop:               make(chan struct{}),
		spawnIndex:         config.SpawnIndex,
		state:              agentWorkerStateIdle,
		activeChecks:       make(map[string]*ComplianceCheckController),
		maxConcurrency:     config.MaxConcurrency,
		pollInterval:       config.PollInterval,
		heartbeatInterval:  config.HeartbeatInterval,
		scheduler:          sched,
		startTime:          time.Now(),
	}
}

// Start begins the agent worker's main loop
func (w *AgentWorker) Start(ctx context.Context) error {
	w.logger.Info().Int("worker", w.spawnIndex).Msg("Starting worker")

	// Start the heartbeat routine
	go w.heartbeatRoutine(ctx)

	// Start the local check scheduler routine
	go w.schedulerRoutine(ctx)

	// Start the main polling loop for remote work
	return w.pollLoop(ctx)
}

// Stop gracefully stops the agent worker
func (w *AgentWorker) Stop() {
	w.stopOnce.Do(func() {
		w.logger.Info().Int("worker", w.spawnIndex).Msg("Stopping worker")
		close(w.stop)
	})
}

// pollLoop is the main polling loop for getting compliance work
func (w *AgentWorker) pollLoop(ctx context.Context) error {
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
				w.logger.Error().Err(err).Msg("Error polling for work")
				// Continue polling even if there's an error
			}
		}
	}
}

// pollForWork polls the Openlane platform for REMOTE scheduled jobs
func (w *AgentWorker) pollForWork(ctx context.Context) error {
	// Update stats
	w.stats.Lock()
	w.stats.lastPing = time.Now()
	w.stats.Unlock()

	// Check if we have capacity for more work
	if w.getCurrentConcurrency() >= w.maxConcurrency {
		w.logger.Debug().Msg("At max concurrency, skipping remote poll")
		return nil
	}

	// Poll for REMOTE scheduled jobs from platform
	scheduledJobs, err := w.apiClient.PollForWork(ctx)
	if err != nil {
		return fmt.Errorf("failed to poll for remote work: %w", err)
	}

	if len(scheduledJobs) == 0 {
		w.logger.Debug().Msg("No remote scheduled jobs available")
		return nil
	}

	w.logger.Info().Int("count", len(scheduledJobs)).Msg("Received remote jobs")

	// Execute scheduled jobs concurrently up to our limit
	for _, scheduledJob := range scheduledJobs {
		if w.getCurrentConcurrency() >= w.maxConcurrency {
			w.logger.Debug().Msg("Max concurrency reached")
			break
		}

		// Start executing the remote scheduled job
		go w.executeScheduledJob(ctx, scheduledJob)
	}

	return nil
}

// schedulerRoutine handles LOCAL check scheduling based on cron expressions
func (w *AgentWorker) schedulerRoutine(ctx context.Context) {
	// Check for due local checks every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-ticker.C:
			if err := w.executeScheduledChecks(ctx); err != nil {
				w.logger.Error().Err(err).Msg("Error executing scheduled checks")
			}
		}
	}
}

// executeScheduledChecks executes any local checks that are due
func (w *AgentWorker) executeScheduledChecks(ctx context.Context) error {
	// Check if we have capacity for more work
	if w.getCurrentConcurrency() >= w.maxConcurrency {
		w.logger.Debug().Msg("At max concurrency, skipping local checks")
		return nil
	}

	// Get due checks from scheduler
	dueChecks := w.scheduler.GetDueChecks()
	if len(dueChecks) == 0 {
		return nil
	}

	w.logger.Info().Int("count", len(dueChecks)).Msg("Found local checks due")

	// Execute due checks
	for _, check := range dueChecks {
		if w.getCurrentConcurrency() >= w.maxConcurrency {
			w.logger.Debug().Msg("Max concurrency reached, deferring checks")
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
func (w *AgentWorker) executeScheduledJob(ctx context.Context, scheduledJob *openlaneclient.ScheduledJob) {
	checkID := fmt.Sprintf("remote-%s-%d", scheduledJob.ID, time.Now().UnixNano())

	w.logger.Info().Str("job_id", scheduledJob.ID).Msg("Starting remote job")

	// Create a check controller
	controller := NewComplianceCheckController(w.logger, w.apiClient, w.agentInfo.ID, nil)

	// Add to active checks
	w.stateMtx.Lock()
	w.activeChecks[checkID] = controller
	if len(w.activeChecks) == 1 {
		w.state = agentWorkerStateBusy
		w.currentCheckID = checkID
	}
	w.stateMtx.Unlock()

	// Convert ScheduledJob to RemoteCheck format for execution
	remoteCheck := &api.RemoteCheck{
		Name:           "unknown-job",
		ScheduledJobID: scheduledJob.ID,
		Settings:       convertJobConfigurationToSettings(scheduledJob.Configuration),
	}

	// Set JobTemplate fields if available
	if scheduledJob.JobTemplate != nil {
		remoteCheck.Name = scheduledJob.JobTemplate.Title
		remoteCheck.Description = getStringValue(scheduledJob.JobTemplate.Description)
		// Note: DownloadURL and Platform fields would be set here if available in RemoteCheck
	}

	// Set Controls if available
	if scheduledJob.Controls != nil {
		remoteCheck.Controls = extractControlConnectionReferences(scheduledJob.Controls)
	}

	// Set the current execution context for logging
	controller.SetCurrentCheck(remoteCheck, scheduledJob.ID)

	// Execute the check
	result, err := controller.ExecuteScheduledJob(ctx, remoteCheck, scheduledJob)

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
		// Log at appropriate level based on continue_on_error setting
		if remoteCheck.ContinueOnError {
			w.logger.Warn().Err(err).Str("job_id", scheduledJob.ID).Msg("Remote job failed (continuing)")
		} else {
			w.logger.Error().Err(err).Str("job_id", scheduledJob.ID).Msg("Remote job failed")
		}
	} else {
		w.stats.successfulChecks++
		w.logger.Info().Str("job_id", scheduledJob.ID).Msg("Remote job completed")
	}
	w.stats.Unlock()

	// Report results if we have them
	if result != nil {
		if err := w.reportResult(ctx, result); err != nil {
			w.logger.Error().Err(err).Str("job_id", scheduledJob.ID).Msg("Failed to report results")
		}
	}
}

// executeLocalCheck executes a single LOCAL compliance check from the config
func (w *AgentWorker) executeLocalCheck(ctx context.Context, check *config.Check) {
	checkID := fmt.Sprintf("local-%s-%d", check.Name, time.Now().UnixNano())

	w.logger.Info().Str("check", check.Name).Msg("Starting local check")

	// Create a check controller
	controller := NewComplianceCheckController(w.logger, w.apiClient, w.agentInfo.ID, nil)

	// Add to active checks
	w.stateMtx.Lock()
	w.activeChecks[checkID] = controller
	if len(w.activeChecks) == 1 {
		w.state = agentWorkerStateBusy
		w.currentCheckID = checkID
	}
	w.stateMtx.Unlock()

	// Convert local check to RemoteCheck format for execution
	remoteCheck := &api.RemoteCheck{
		Name:        check.Name,
		Description: check.Description,
		Command:     check.Command,
		Args:        check.Args,
		WorkDir:     check.WorkDir,
		Timeout:         check.Timeout.String(),
		Controls:        check.Controls,
		Tags:            check.Tags,
		Env:             check.Env,
		ContinueOnError: check.ContinueOnError,
	}

	// Set the current execution context for logging
	controller.SetCurrentCheck(remoteCheck, "")

	// Execute the check
	result, err := controller.ExecuteCheck(ctx, remoteCheck)

	// Mark as completed in scheduler
	if schedErr := w.scheduler.MarkCompleted(check.Name); schedErr != nil {
		w.logger.Error().Err(schedErr).Str("check", check.Name).Msg("Failed to mark check completed")
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
	if err != nil {
		w.stats.failedChecks++
		// Log at appropriate level based on continue_on_error setting
		if remoteCheck.ContinueOnError {
			w.logger.Warn().Err(err).Str("check", check.Name).Msg("Local check failed (continuing)")
		} else {
			w.logger.Error().Err(err).Str("check", check.Name).Msg("Local check failed")
		}
	} else {
		w.stats.successfulChecks++
		w.logger.Info().Str("check", check.Name).Msg("Local check completed")
	}
	w.stats.Unlock()

	// Report results if we have them
	if result != nil {
		if err := w.reportResult(ctx, result); err != nil {
			w.logger.Error().Err(err).Str("check", check.Name).Msg("Failed to report results")
		}
	}
}

// reportResult reports the compliance check result back to Openlane
func (w *AgentWorker) reportResult(ctx context.Context, result *config.Result) error {
	results := []*config.Result{result}
	return w.apiClient.ReportResults(ctx, w.agentInfo.ID, results)
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
			if err := w.sendHeartbeat(ctx); err != nil {
				w.stats.Lock()
				w.stats.lastHeartbeatError = err
				w.stats.Unlock()
				w.logger.Error().Err(err).Msg("Heartbeat failed")
			} else {
				w.stats.Lock()
				w.stats.lastHeartbeat = time.Now()
				w.stats.lastHeartbeatError = nil
				w.stats.Unlock()
				w.logger.Debug().Msg("Heartbeat sent successfully")
			}
		}
	}
}

// sendHeartbeat sends a heartbeat to the platform
func (w *AgentWorker) sendHeartbeat(ctx context.Context) error {
	w.logger.Debug().Msg("Sending heartbeat")

	// Create a heartbeat by updating the JobRunner with current status
	// This serves as a "ping" to show the agent is alive and provides current state
	input := openlaneclient.UpdateJobRunnerInput{
		// Note: We're not changing any actual fields, just triggering an update
		// to show the agent is alive. The UpdatedAt timestamp will be refreshed.
	}

	_, err := w.apiClient.UpdateJobRunner(ctx, w.agentInfo.ID, input)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

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

// getCurrentCheckID returns the ID of the currently running check (if any)
func (w *AgentWorker) getCurrentCheckID() string {
	w.stateMtx.Lock()
	defer w.stateMtx.Unlock()
	return w.currentCheckID
}

// GetStats returns current worker statistics
func (w *AgentWorker) GetStats() map[string]any {
	w.stats.Lock()
	defer w.stats.Unlock()

	return map[string]any{
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
	}
}

// Helper functions for data conversion

// getStringValue safely gets a string value from a pointer
func getStringValue(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// convertConfigToSettings converts a map[string]any to map[string]string
func convertConfigToSettings(config map[string]any) map[string]string {
	settings := make(map[string]string)
	for key, value := range config {
		settings[key] = fmt.Sprintf("%v", value)
	}
	return settings
}

// extractControlReferences extracts control reference codes from Control structs
func extractControlReferences(controls []*openlaneclient.Control) []string {
	var refs []string
	for _, control := range controls {
		if control.ReferenceID != nil {
			refs = append(refs, *control.ReferenceID)
		}
	}
	return refs
}

// convertJobConfigurationToSettings converts models.JobConfiguration to map[string]string
func convertJobConfigurationToSettings(config models.JobConfiguration) map[string]string {
	settings := make(map[string]string)

	// JobConfiguration is json.RawMessage, so we need to unmarshal it
	if len(config) > 0 {
		var configMap map[string]any
		if err := json.Unmarshal(config, &configMap); err == nil {
			return convertConfigToSettings(configMap)
		}
	}

	return settings
}

// extractControlConnectionReferences extracts control IDs from ControlConnection
func extractControlConnectionReferences(controlConn *openlaneclient.ControlConnection) []string {
	var refs []string

	if controlConn != nil && controlConn.Edges != nil {
		for _, edge := range controlConn.Edges {
			if edge.Node != nil {
				refs = append(refs, edge.Node.ID)
			}
		}
	}

	return refs
}
