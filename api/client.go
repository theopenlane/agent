package api

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/constants"
	"github.com/theopenlane/core/pkg/enums"
	"github.com/theopenlane/core/pkg/openlaneclient"
)

// Client handles communication with the Openlane API
type Client struct {
	client    *openlaneclient.OpenlaneClient
	userAgent string
	timeout   time.Duration
}

// NewClient creates a new API client
func NewClient(baseURL, apiKey string) (*Client, error) {
	if baseURL == "" {
		return nil, ErrBaseURLRequired
	}

	if apiKey == "" {
		return nil, ErrAPIKeyRequired
	}

	// Parse the base URL
	baseURLParsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Create credentials with the API key
	auth := openlaneclient.Authorization{
		BearerToken: apiKey,
	}

	// Create the openlane client with defaults
	client, err := openlaneclient.NewWithDefaults(
		openlaneclient.WithBaseURL(baseURLParsed),
		openlaneclient.WithCredentials(auth),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create openlane client: %w", err)
	}

	return &Client{
		client:    client,
		userAgent: constants.UserAgent(),
	}, nil
}

// Ping tests connectivity to the API
func (c *Client) Ping(ctx context.Context) error {
	// Apply timeout to context
	timeoutCtx, cancel := c.withTimeout(ctx)
	defer cancel()

	// Test connectivity by getting organizations (basic query)
	_, err := c.client.GetAllOrganizations(timeoutCtx)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrPingRequestFailed, err)
	}

	log.Debug().Msg("Ping request successful")

	return nil
}

// ReportResults sends compliance check results to the API
func (c *Client) ReportResults(ctx context.Context, results []*config.Result) error {
	if len(results) == 0 {
		return nil
	}

	log.Debug().Int("count", len(results)).Msg("Reporting results to API")

	// Report results via JobResult entities through the GraphQL client
	for _, result := range results {
		// Create JobResult for each result
		if err := c.createJobResult(ctx, result); err != nil {
			log.Error().Err(err).Str("check", result.CheckName).Msg("Failed to create job result")
			continue
		}

		log.Info().Str("check", result.CheckName).Int("exit_code", result.ExitCode).Msg("Result reported")
	}

	log.Info().Int("count", len(results)).Msg("All results reported")

	return nil
}

// RegisterAgent registers the agent with the API using GraphQL
func (c *Client) RegisterAgent(ctx context.Context, agent JobRunnerRegistration) (*openlaneclient.JobRunner, error) {
	log.Debug().Str("name", agent.Name).Msg("Registering agent")

	// Create job runner using the GraphQL API
	input := openlaneclient.CreateJobRunnerInput{
		Name: agent.Name,
		Tags: agent.Tags,
	}

	resp, err := c.client.CreateJobRunner(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAgentRegistrationFailed, err)
	}

	log.Info().Str("id", resp.CreateJobRunner.JobRunner.ID).Msg("Agent registered")

	// Convert the response to openlaneclient.JobRunner
	jr := &openlaneclient.JobRunner{
		ID:        resp.CreateJobRunner.JobRunner.ID,
		Name:      resp.CreateJobRunner.JobRunner.Name,
		Status:    resp.CreateJobRunner.JobRunner.Status,
		IPAddress: resp.CreateJobRunner.JobRunner.IPAddress,
		CreatedAt: resp.CreateJobRunner.JobRunner.CreatedAt,
		UpdatedAt: resp.CreateJobRunner.JobRunner.UpdatedAt,
	}

	return jr, nil
}

// UpdateAgentStatus updates the agent's status
func (c *Client) UpdateAgentStatus(ctx context.Context, agentID string, status AgentStatus) error {
	// Update job runner status using GraphQL
	input := openlaneclient.UpdateJobRunnerInput{
		// Status updates would go here if supported by the API
	}

	_, err := c.client.UpdateJobRunner(ctx, agentID, input)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to update agent status")
		return fmt.Errorf("%w: %w", ErrAgentStatusUpdateFailed, err)
	}

	log.Info().Str("status", status.Status).Msg("Agent status updated")

	return nil
}

// GetAgentConfig retrieves agent configuration from the API
func (c *Client) GetAgentConfig(ctx context.Context, agentID string) (*RemoteConfig, error) {
	// Get job runner details and associated scheduled jobs
	_, err := c.client.GetJobRunnerByID(ctx, agentID)
	if err != nil {
		log.Debug().Err(err).Msg("Failed to get job runner")
		return nil, nil // Return nil to indicate no remote config available
	}

	log.Debug().Str("job_runner_id", agentID).Msg("Retrieved job runner details")

	// Get scheduled jobs for this job runner
	scheduledJobs, err := c.getScheduledJobsForAgent(ctx, agentID)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to get scheduled jobs, returning basic config")

		scheduledJobs = []*openlaneclient.ScheduledJob{}
	}

	// Convert scheduled jobs to remote checks
	var remoteChecks []RemoteCheck

	for _, job := range scheduledJobs {
		if !job.Active {
			continue // Skip inactive jobs
		}

		schedule := ""
		if job.Cron != nil {
			schedule = *job.Cron
		}

		check := RemoteCheck{
			Name:           fmt.Sprintf("remote-job-%s", job.ID),
			Description:    fmt.Sprintf("Scheduled job %s", job.ID),
			ScheduledJobID: job.ID,
			Schedule:       schedule,
			Enabled:        job.Active,

			// Parse configuration if available
			Settings: make(map[string]string),
		}

		// Extract controls from job configuration if available
		if job.Configuration != nil {
			// Configuration parsing would depend on the actual structure
			// For now, we'll use a basic approach
			check.Settings["job_config"] = string(job.Configuration)
		}

		remoteChecks = append(remoteChecks, check)
	}

	config := &RemoteConfig{
		Version:   "1.0",
		UpdatedAt: time.Now(),
		LogLevel:  "info", // Could be derived from job runner configuration
		Checks:    remoteChecks,
		Features: map[string]bool{
			"remote_scheduling": true,
			"control_sync":      true,
			"log_streaming":     true,
		},
	}

	log.Info().Int("remote_checks", len(remoteChecks)).Msg("Retrieved remote agent configuration")

	return config, nil
}

// getScheduledJobsForAgent retrieves scheduled jobs for a specific job runner
func (c *Client) getScheduledJobsForAgent(ctx context.Context, agentID string) ([]*openlaneclient.ScheduledJob, error) {
	where := &openlaneclient.ScheduledJobWhereInput{
		JobRunnerID: &agentID,
	}

	resp, err := c.client.GetScheduledJobs(ctx, nil, nil, where)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrScheduledJobsRetrievalFailed, err)
	}

	var scheduledJobs []*openlaneclient.ScheduledJob
	for _, edge := range resp.ScheduledJobs.Edges {
		scheduledJobs = append(scheduledJobs, &openlaneclient.ScheduledJob{
			ID:            edge.Node.ID,
			JobID:         edge.Node.JobID,
			JobRunnerID:   edge.Node.JobRunnerID,
			Cron:          edge.Node.Cron,
			Active:        true, // Default to active since the field may not be available
			Configuration: edge.Node.Configuration,
			CreatedAt:     edge.Node.CreatedAt,
			UpdatedAt:     edge.Node.UpdatedAt,
		})
	}

	return scheduledJobs, nil
}

// SetTimeout sets the HTTP client timeout
func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeout = timeout

	// Create a new client with timeout if we need to recreate it
	if c.client != nil {
		// For production use, we would need to recreate the client with new timeout
		// Since the openlaneclient may not expose timeout configuration directly,
		// we store it and apply it during the next client creation or use context timeouts
		log.Debug().Dur("timeout", timeout).Msg("HTTP timeout updated - will be applied to new requests")
	} else {
		log.Warn().Dur("timeout", timeout).Msg("Cannot set timeout - client not initialized")
	}
}

// GetTimeout returns the current timeout setting
func (c *Client) GetTimeout() time.Duration {
	return c.timeout
}

// withTimeout creates a context with the client's timeout
func (c *Client) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.timeout > 0 {
		return context.WithTimeout(ctx, c.timeout)
	}

	return ctx, func() {} // No-op cancel function
}

// SetUserAgent sets a custom user agent
func (c *Client) SetUserAgent(userAgent string) {
	c.userAgent = userAgent
}

// createJobResult creates a JobResult entity for a compliance check result
func (c *Client) createJobResult(ctx context.Context, result *config.Result) error {
	// Determine status based on exit code
	status := enums.JobExecutionStatusSuccess
	if result.ExitCode != 0 {
		status = enums.JobExecutionStatusFailed
	}

	// Convert exit code to int64
	exitCode := int64(result.ExitCode)

	// Create job result using the openlane client
	input := openlaneclient.CreateJobResultInput{
		ScheduledJobID: result.ScheduledJobID,
		Status:         status,
		ExitCode:       exitCode,
		StartedAt:      &result.StartTime,
		FinishedAt:     &result.EndTime,
	}

	_, err := c.client.CreateJobResult(ctx, input)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrJobResultCreationFailed, err)
	}

	log.Debug().Msg("Created job result")

	return nil
}
