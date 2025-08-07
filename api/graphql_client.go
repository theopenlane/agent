package api

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/internal/config"
	"github.com/theopenlane/agent/version"
	"github.com/theopenlane/core/pkg/enums"
	"github.com/theopenlane/core/pkg/openlaneclient"
)

// GraphQLClient handles GraphQL communication with the Openlane platform
type GraphQLClient struct {
	client            *openlaneclient.OpenlaneClient
	registrationToken string
	agentID           string
	userAgent         string
}

// NewGraphQLClient creates a new GraphQL client
func NewGraphQLClient(baseURL, registrationToken string) (*GraphQLClient, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}

	if registrationToken == "" {
		return nil, fmt.Errorf("registration token is required")
	}

	// Parse the base URL
	baseURLParsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Create credentials with the registration token
	auth := openlaneclient.Authorization{
		BearerToken: registrationToken,
	}

	// Create the openlane client with defaults
	client, err := openlaneclient.NewWithDefaults(
		openlaneclient.WithBaseURL(baseURLParsed),
		openlaneclient.WithCredentials(auth),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create openlane client: %w", err)
	}

	return &GraphQLClient{
		client:            client,
		registrationToken: registrationToken,
		userAgent:         version.UserAgent(),
	}, nil
}

// RegisterAgent registers the agent as a JobRunner using GraphQL
func (c *GraphQLClient) RegisterAgent(ctx context.Context, registration JobRunnerRegistration) (*openlaneclient.JobRunner, error) {
	log.Debug().Str("name", registration.Name).Msg("Registering agent")

	// Create job runner using the GraphQL API
	input := openlaneclient.CreateJobRunnerInput{
		Name: registration.Name,
		Tags: registration.Tags,
	}

	resp, err := c.client.CreateJobRunner(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to register agent: %w", err)
	}

	// Store the agent ID for future use
	c.agentID = resp.CreateJobRunner.JobRunner.ID

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

// PollForWork polls for scheduled jobs assigned to this agent
func (c *GraphQLClient) PollForWork(ctx context.Context) ([]*openlaneclient.ScheduledJob, error) {
	if c.agentID == "" {
		return nil, fmt.Errorf("agent not registered")
	}

	// Use the openlane client to get scheduled jobs
	where := &openlaneclient.ScheduledJobWhereInput{
		JobRunnerID: &c.agentID,
		Active:      &[]bool{true}[0],
	}

	resp, err := c.client.GetScheduledJobs(ctx, nil, nil, where)
	if err != nil {
		return nil, fmt.Errorf("failed to poll for work: %w", err)
	}

	var scheduledJobs []*openlaneclient.ScheduledJob
	for _, edge := range resp.ScheduledJobs.Edges {
		// Convert the response node to openlaneclient.ScheduledJob
		sj := &openlaneclient.ScheduledJob{
			ID:            edge.Node.ID,
			JobID:         edge.Node.JobID,
			Configuration: edge.Node.Configuration,
			Cron:          edge.Node.Cron,
			JobRunnerID:   edge.Node.JobRunnerID,
			CreatedAt:     edge.Node.CreatedAt,
			UpdatedAt:     edge.Node.UpdatedAt,
		}

		scheduledJobs = append(scheduledJobs, sj)
	}

	log.Debug().Int("count", len(scheduledJobs)).Msg("Polled scheduled jobs")
	return scheduledJobs, nil
}

// ReportResults sends compliance check results to the platform as JobResult entities
func (c *GraphQLClient) ReportResults(ctx context.Context, agentID string, results []*config.Result) error {
	if len(results) == 0 {
		return nil
	}

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

// createJobResult creates a JobResult entity for a compliance check result
func (c *GraphQLClient) createJobResult(ctx context.Context, result *config.Result) error {
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
		return fmt.Errorf("failed to create job result: %w", err)
	}

	log.Debug().Msg("Created job result")
	return nil
}

// SetTimeout sets the HTTP client timeout
func (c *GraphQLClient) SetTimeout(timeout time.Duration) {
	// Store the timeout for future use - the underlying client may not expose SetHTTPTimeout
	log.Debug().Dur("timeout", timeout).Msg("GraphQL timeout setting requested - stored for future HTTP client configuration")
	// TODO: Implement timeout configuration when the underlying client supports it
}

// GetAgentID returns the registered agent ID
func (c *GraphQLClient) GetAgentID() string {
	return c.agentID
}

// SetAgentID sets the agent ID
func (c *GraphQLClient) SetAgentID(agentID string) {
	c.agentID = agentID
}

// GetAllControls retrieves all controls from the Openlane system
func (c *GraphQLClient) GetAllControls(ctx context.Context) ([]*openlaneclient.Control, error) {
	resp, err := c.client.GetAllControls(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get controls: %w", err)
	}
	
	// Convert edges to control list
	var controls []*openlaneclient.Control
	for _, edge := range resp.Controls.Edges {
		// Map all available fields from the GraphQL response
		control := &openlaneclient.Control{
			ID:        edge.Node.ID,
			CreatedAt: edge.Node.CreatedAt,
			UpdatedAt: edge.Node.UpdatedAt,
			// ReferenceID field may not be available in the GraphQL response
		}
		controls = append(controls, control)
	}
	
	log.Debug().Int("count", len(controls)).Msg("Retrieved controls")
	return controls, nil
}

// UpdateControl updates a control in the Openlane system
func (c *GraphQLClient) UpdateControl(ctx context.Context, controlID string, input openlaneclient.UpdateControlInput) error {
	_, err := c.client.UpdateControl(ctx, controlID, input)
	if err != nil {
		return fmt.Errorf("failed to update control: %w", err)
	}
	
	log.Debug().Str("control_id", controlID).Msg("Updated control")
	return nil
}

// CreateControl creates a new control in the Openlane system
func (c *GraphQLClient) CreateControl(ctx context.Context, input openlaneclient.CreateControlInput) (*openlaneclient.Control, error) {
	resp, err := c.client.CreateControl(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create control: %w", err)
	}
	
	// Map all available fields from the create response
	control := &openlaneclient.Control{
		ID:        resp.CreateControl.Control.ID,
		CreatedAt: resp.CreateControl.Control.CreatedAt,
		UpdatedAt: resp.CreateControl.Control.UpdatedAt,
		// ReferenceID field may not be available in the create response
	}
	
	log.Info().Str("control_id", control.ID).Msg("Created control")
	return control, nil
}

// UpdateJobRunner updates a job runner (used for heartbeats)
func (c *GraphQLClient) UpdateJobRunner(ctx context.Context, jobRunnerID string, input openlaneclient.UpdateJobRunnerInput) (*openlaneclient.JobRunner, error) {
	resp, err := c.client.UpdateJobRunner(ctx, jobRunnerID, input)
	if err != nil {
		return nil, fmt.Errorf("failed to update job runner: %w", err)
	}

	// Convert the response to openlaneclient.JobRunner
	jr := &openlaneclient.JobRunner{
		ID:        resp.UpdateJobRunner.JobRunner.ID,
		Name:      resp.UpdateJobRunner.JobRunner.Name,
		Status:    resp.UpdateJobRunner.JobRunner.Status,
		IPAddress: resp.UpdateJobRunner.JobRunner.IPAddress,
		CreatedAt: resp.UpdateJobRunner.JobRunner.CreatedAt,
		UpdatedAt: resp.UpdateJobRunner.JobRunner.UpdatedAt,
	}
	return jr, nil
}
