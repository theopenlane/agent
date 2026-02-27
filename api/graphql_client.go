package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/rs/zerolog/log"
	"github.com/theopenlane/core/common/enums"
	coremodels "github.com/theopenlane/core/common/models"
	openlane "github.com/theopenlane/go-client"
	"github.com/theopenlane/go-client/graphclient"

	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/constants"
	agentmodels "github.com/theopenlane/agent/internal/models"
	"github.com/theopenlane/agent/internal/retry"
)

// GraphQLClient handles GraphQL communication with the Openlane platform
type GraphQLClient struct {
	client            *openlane.Client
	registrationToken string
	agentID           string
	userAgent         string
	retryManager      *retry.Manager
}

// NewGraphQLClient creates a new GraphQL client
func NewGraphQLClient(baseURL, registrationToken string) (*GraphQLClient, error) {
	return NewGraphQLClientWithRetry(baseURL, registrationToken, nil)
}

// NewGraphQLClientWithRetry creates a new GraphQL client with retry configuration
func NewGraphQLClientWithRetry(baseURL, registrationToken string, retryManager *retry.Manager) (*GraphQLClient, error) {
	if baseURL == "" {
		return nil, ErrBaseURLRequired
	}

	if registrationToken == "" {
		return nil, ErrRegistrationTokenRequired
	}

	client, err := openlane.New(
		openlane.WithBaseURL(baseURL),
		openlane.WithAPIToken(registrationToken),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create openlane client: %w", err)
	}

	return &GraphQLClient{
		client:            client,
		registrationToken: registrationToken,
		userAgent:         constants.UserAgent(),
		retryManager:      retryManager,
	}, nil
}

// RegisterAgent registers the agent as a JobRunner using GraphQL
func (c *GraphQLClient) RegisterAgent(ctx context.Context, registration JobRunnerRegistration) (*graphclient.JobRunner, error) {
	log.Debug().Str("name", registration.Name).Msg("Registering agent")

	input := graphclient.CreateJobRunnerInput{
		Name: registration.Name,
		Tags: registration.Tags,
	}
	if registration.IPAddress != "" {
		input.IPAddress = &registration.IPAddress
	}

	if registration.Version != "" {
		input.Version = &registration.Version
	}

	if registration.Platform != "" {
		input.Os = &registration.Platform
	}

	now := time.Now().UTC()
	input.LastSeen = &now

	var resp *graphclient.CreateJobRunner

	err := c.executeWithRetry(ctx, func(ctx context.Context) error {
		var execErr error

		resp, execErr = c.client.CreateJobRunner(ctx, input)

		return execErr
	})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrAgentRegistrationFailed, err)
	}

	c.agentID = resp.CreateJobRunner.JobRunner.ID

	log.Info().Str("id", resp.CreateJobRunner.JobRunner.ID).Msg("Agent registered")

	jr := &graphclient.JobRunner{
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
func (c *GraphQLClient) PollForWork(ctx context.Context) ([]*graphclient.ScheduledJob, error) {
	if c.agentID == "" {
		return nil, ErrAgentNotRegistered
	}

	where := &graphclient.ScheduledJobWhereInput{
		JobRunnerID: &c.agentID,
		Active:      &[]bool{true}[0],
	}

	resp, err := c.client.GetScheduledJobs(ctx, nil, nil, nil, nil, where, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPollForWorkFailed, err)
	}

	var scheduledJobs []*graphclient.ScheduledJob

	for _, edge := range resp.ScheduledJobs.Edges {
		sj := &graphclient.ScheduledJob{
			ID:            edge.Node.ID,
			Active:        edge.Node.Active,
			Configuration: edge.Node.Configuration,
			Cron:          edge.Node.Cron,
			JobID:         edge.Node.JobID,
			JobRunnerID:   edge.Node.JobRunnerID,
			OwnerID:       edge.Node.OwnerID,
		}

		scheduledJobs = append(scheduledJobs, sj)
	}

	log.Debug().Int("count", len(scheduledJobs)).Msg("Polled scheduled jobs")

	return scheduledJobs, nil
}

// ReportResults sends compliance check results to the platform as JobResult entities
func (c *GraphQLClient) ReportResults(ctx context.Context, results []*config.Result) error {
	if len(results) == 0 {
		return nil
	}

	var reportErrors []error

	for _, result := range results {
		if err := c.createJobResult(ctx, result); err != nil {
			log.Error().Err(err).Str("check", result.CheckName).Msg("Failed to create job result")
			reportErrors = append(reportErrors, err)
			continue
		}

		exitCode := 0
		if result.ExitCode != nil {
			exitCode = *result.ExitCode
		}

		log.Info().Str("check", result.CheckName).Int("exit_code", exitCode).Msg("Result reported")
	}

	if len(reportErrors) > 0 {
		return fmt.Errorf("failed to report %d of %d results: %w", len(reportErrors), len(results), errors.Join(reportErrors...))
	}

	log.Info().Int("count", len(results)).Msg("All results reported")

	return nil
}

// createJobResult creates a JobResult entity for a compliance check result
func (c *GraphQLClient) createJobResult(ctx context.Context, result *config.Result) error {
	status := result.Status

	scheduledJobID := ""

	var ownerID *string

	if result.Metadata != nil {
		if jobID, ok := result.Metadata["scheduled_job_id"].(string); ok {
			scheduledJobID = jobID
		}

		if ownerIDStr, ok := result.Metadata["owner_id"].(string); ok {
			ownerID = &ownerIDStr
		}
	}

	if scheduledJobID == "" {
		log.Debug().Str("check", result.CheckName).Msg("No scheduled job ID, skipping JobResult creation (manual check)")
		return nil
	}

	var exitCode int64

	if result.ExitCode != nil {
		exitCode = int64(*result.ExitCode)
	}

	input := graphclient.CreateJobResultInput{
		ScheduledJobID: scheduledJobID,
		Status:         status,
		ExitCode:       exitCode,
		FileID:         "",
		StartedAt:      &result.StartedAt,
		FinishedAt:     &result.FinishedAt,
		Log:            &result.Log,
		OwnerID:        ownerID,
	}

	var uploads []*graphql.Upload
	if resultFile := PrepareResultFileUpload(result); resultFile != nil {
		uploads = append(uploads, resultFile)
	}

	resp, err := c.client.CreateJobResult(ctx, input, uploads)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrJobResultCreationFailed, err)
	}

	if resp != nil {
		if result.Metadata == nil {
			result.Metadata = make(map[string]any)
		}

		result.Metadata["job_result_id"] = resp.CreateJobResult.JobResult.ID
		log.Debug().Str("job_result_id", resp.CreateJobResult.JobResult.ID).Bool("has_file", len(uploads) > 0).Msg("Created job result")
	}

	return nil
}

// SetTimeout stores a timeout setting for the GraphQL client
func (c *GraphQLClient) SetTimeout(timeout time.Duration) {
	log.Debug().Dur("timeout", timeout).Msg("GraphQL timeout setting requested - stored for future HTTP client configuration")
}

// GetAgentID returns the registered agent ID
func (c *GraphQLClient) GetAgentID() string {
	return c.agentID
}

// SetAgentID sets the agent ID
func (c *GraphQLClient) SetAgentID(agentID string) {
	c.agentID = agentID
}

// GetClient returns the underlying openlane Client for direct API calls
func (c *GraphQLClient) GetClient() *openlane.Client {
	return c.client
}

// GetAllControls retrieves all controls from the Openlane system
func (c *GraphQLClient) GetAllControls(ctx context.Context) ([]*graphclient.Control, error) {
	resp, err := c.client.GetAllControls(ctx, nil, nil, nil, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrControlsRetrievalFailed, err)
	}

	var controls []*graphclient.Control

	for _, edge := range resp.Controls.Edges {
		control := &graphclient.Control{
			ID:                 edge.Node.ID,
			CreatedAt:          edge.Node.CreatedAt,
			UpdatedAt:          edge.Node.UpdatedAt,
			RefCode:            edge.Node.RefCode,
			ReferenceFramework: edge.Node.ReferenceFramework,
			StandardID:         edge.Node.StandardID,
		}

		controls = append(controls, control)
	}

	log.Debug().Int("count", len(controls)).Msg("Retrieved controls with standard relationships")

	return controls, nil
}

// UpdateControl updates a control in the Openlane system
func (c *GraphQLClient) UpdateControl(ctx context.Context, controlID string, input graphclient.UpdateControlInput) error {
	_, err := c.client.UpdateControl(ctx, controlID, input)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrControlUpdateFailed, err)
	}

	log.Debug().Str("control_id", controlID).Msg("Updated control")

	return nil
}

// UpdateJobRunner updates a job runner (used for heartbeats)
func (c *GraphQLClient) UpdateJobRunner(ctx context.Context, jobRunnerID string, input graphclient.UpdateJobRunnerInput) (*graphclient.JobRunner, error) {
	resp, err := c.client.UpdateJobRunner(ctx, jobRunnerID, input)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrJobRunnerUpdateFailed, err)
	}

	jr := &graphclient.JobRunner{
		ID:        resp.UpdateJobRunner.JobRunner.ID,
		Name:      resp.UpdateJobRunner.JobRunner.Name,
		Status:    resp.UpdateJobRunner.JobRunner.Status,
		IPAddress: resp.UpdateJobRunner.JobRunner.IPAddress,
		CreatedAt: resp.UpdateJobRunner.JobRunner.CreatedAt,
		UpdatedAt: resp.UpdateJobRunner.JobRunner.UpdatedAt,
	}

	return jr, nil
}

// ValidateControls validates that controls exist within their specified standards
func (c *GraphQLClient) ValidateControls(standards []config.ComplianceStandard) (map[string][]string, error) {
	validControls := make(map[string][]string)

	for _, standard := range standards {
		if err := c.validateStandard(standard.Standard); err != nil {
			log.Error().Err(err).Str("standard", standard.Standard).Msg("Standard validation failed")
			continue
		}

		for _, control := range standard.Controls {
			if err := c.validateControl(standard.Standard, control); err != nil {
				log.Error().Err(err).Str("standard", standard.Standard).Str("control", control).Msg("Control validation failed")
				continue
			}

			if validControls[standard.Standard] == nil {
				validControls[standard.Standard] = make([]string, 0)
			}

			validControls[standard.Standard] = append(validControls[standard.Standard], control)
		}
	}

	return validControls, nil
}

// ResolveControlIDs resolves control reference codes to control IDs for evidence associations.
func (c *GraphQLClient) ResolveControlIDs(standards []config.ComplianceStandard) ([]string, error) {
	controlIDs := make(map[string]struct{})

	var resolveErrors []error

	for _, standard := range standards {
		if err := c.validateStandard(standard.Standard); err != nil {
			resolveErrors = append(resolveErrors, err)
			continue
		}

		for _, controlRef := range standard.Controls {
			controlID, err := c.lookupControlID(standard.Standard, controlRef)
			if err != nil {
				resolveErrors = append(resolveErrors, err)
				continue
			}

			controlIDs[controlID] = struct{}{}
		}
	}

	resolved := make([]string, 0, len(controlIDs))
	for controlID := range controlIDs {
		resolved = append(resolved, controlID)
	}

	slices.Sort(resolved)

	if len(resolveErrors) > 0 && len(resolved) == 0 {
		return resolved, fmt.Errorf("failed to resolve control IDs: %w", errors.Join(resolveErrors...))
	}

	return resolved, nil
}

// validateStandard validates that a compliance standard exists
func (c *GraphQLClient) validateStandard(standardID string) error {
	ctx := context.Background()

	standard, err := c.client.GetStandardByID(ctx, standardID)
	if err != nil {
		log.Error().Err(err).Str("standard", standardID).Msg("Failed to validate standard")
		return fmt.Errorf("failed to validate standard %s: %w", standardID, err)
	}

	if standard == nil {
		log.Error().Str("standard", standardID).Msg("Standard not found")
		return fmt.Errorf("%w: %s", ErrStandardNotFound, standardID)
	}

	log.Debug().Str("standard", standardID).Str("name", standard.Standard.Name).Msg("Standard validated successfully")

	return nil
}

// validateControl validates that a control exists within a standard
func (c *GraphQLClient) validateControl(standardID, controlRefCode string) error {
	if _, err := c.lookupControlID(standardID, controlRefCode); err != nil {
		log.Error().Err(err).Str("standard", standardID).Str("control", controlRefCode).Msg("Failed to validate control")
		return fmt.Errorf("failed to validate control %s in standard %s: %w", controlRefCode, standardID, err)
	}

	log.Debug().Str("standard", standardID).Str("control", controlRefCode).Msg("Control validated successfully")

	return nil
}

// lookupControlID resolves a control reference within a standard to its control ID.
func (c *GraphQLClient) lookupControlID(standardID, controlRefCode string) (string, error) {
	ctx := context.Background()

	response, err := c.client.GetControls(ctx, nil, nil, nil, nil, &graphclient.ControlWhereInput{
		StandardID: &standardID,
		RefCode:    &controlRefCode,
	}, nil)
	if err != nil {
		return "", fmt.Errorf("query control %s in standard %s: %w", controlRefCode, standardID, err)
	}

	if response == nil || len(response.Controls.Edges) == 0 || response.Controls.Edges[0].Node == nil {
		return "", fmt.Errorf("%w: %s in standard %s", ErrControlNotFound, controlRefCode, standardID)
	}

	return response.Controls.Edges[0].Node.ID, nil
}

// CreateEvidence creates evidence with optional control associations using GraphQL mutation
func (c *GraphQLClient) CreateEvidence(controlIDs []string, evidence agentmodels.EvidenceFile, jobResultID string) error {
	ctx := context.Background()

	isAutomated := true
	creationTime := evidence.CreatedAt

	evidenceName := strings.TrimPrefix(evidence.Path, "./")
	if evidenceName == "" {
		evidenceName = "evidence-" + jobResultID
	}

	description := fmt.Sprintf("Evidence collected by agent from %s", evidence.Path)
	collectionProcedure := "Automated collection by Openlane compliance agent"
	source := "openlane-agent"

	input := graphclient.CreateEvidenceInput{
		Name:                evidenceName,
		Description:         &description,
		CollectionProcedure: &collectionProcedure,
		CreationDate:        &creationTime,
		Source:              &source,
		IsAutomated:         &isAutomated,
		ControlIDs:          controlIDs,
		Tags:                []string{"automated", "agent-collected", fmt.Sprintf("job-result:%s", jobResultID)},
	}

	var uploads []*graphql.Upload

	if len(evidence.Content) > 0 {
		upload := PrepareEvidenceFileUpload(evidence)
		uploads = append(uploads, upload)
	}

	var resp *graphclient.CreateEvidence

	err := c.executeWithRetry(ctx, func(ctx context.Context) error {
		var execErr error

		resp, execErr = c.client.CreateEvidence(ctx, input, uploads)

		return execErr
	})
	if err != nil {
		controlIDsStr := "none"
		if len(controlIDs) > 0 {
			controlIDsStr = strings.Join(controlIDs, ", ")
		}

		log.Error().Err(err).Str("controls", controlIDsStr).Str("evidence_path", evidence.Path).Msg("Failed to create evidence")

		return fmt.Errorf("%w: %w", ErrEvidenceCreationFailed, err)
	}

	controlIDsStr := "none"
	if len(controlIDs) > 0 {
		controlIDsStr = strings.Join(controlIDs, ", ")
	}

	log.Info().Str("controls", controlIDsStr).Str("evidence_id", resp.CreateEvidence.Evidence.ID).Str("evidence_path", evidence.Path).Msg("Evidence created")

	return nil
}

// CreateEvidenceForControl creates evidence directly associated with a control
func (c *GraphQLClient) CreateEvidenceForControl(controlID string, evidence agentmodels.EvidenceFile, jobResultID string) error {
	return c.CreateEvidence([]string{controlID}, evidence, jobResultID)
}

// UpdateAgentStatus updates the agent's status and metadata in the JobRunner
func (c *GraphQLClient) UpdateAgentStatus(ctx context.Context, agentID string, status AgentStatus) error {
	input := graphclient.UpdateJobRunnerInput{
		LastSeen: &status.LastPing,
	}

	if status.SystemInfo.OS != "" {
		input.Os = &status.SystemInfo.OS
	}

	if status.Version != "" {
		input.Version = &status.Version
	}

	if status.IPAddress != "" {
		input.IPAddress = &status.IPAddress
	}

	_, err := c.UpdateJobRunner(ctx, agentID, input)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to update job runner")
		return fmt.Errorf("%w: %w", ErrAgentStatusUpdateFailed, err)
	}

	log.Info().Str("agent_status", status.Status).Str("version", status.Version).Str("ip_address", status.IPAddress).Msg("Job runner updated with agent status")

	return nil
}

// executeWithRetry is a helper method that wraps operations with retry logic when available
func (c *GraphQLClient) executeWithRetry(ctx context.Context, operation func(context.Context) error) error {
	if c.retryManager != nil {
		return c.retryManager.ExecuteWithContext(ctx, operation)
	}

	return operation(ctx)
}

// SyncJobTemplates ensures JobTemplates exist for all checks and returns their IDs
func (c *GraphQLClient) SyncJobTemplates(ctx context.Context, checks []*config.Check) (map[string]string, error) {
	if c.agentID == "" {
		return nil, ErrAgentNotRegistered
	}

	templateIDs := make(map[string]string)

	for _, check := range checks {
		templateID, err := c.syncJobTemplate(ctx, check)
		if err != nil {
			log.Error().Err(err).Str("check", check.Name).Msg("Failed to sync job template")
			continue
		}

		templateIDs[check.Name] = templateID
		log.Debug().Str("check", check.Name).Str("template_id", templateID).Msg("Job template synced")
	}

	log.Info().Int("count", len(templateIDs)).Msg("Job templates synced")

	return templateIDs, nil
}

// syncJobTemplate creates or retrieves a JobTemplate for a check
func (c *GraphQLClient) syncJobTemplate(ctx context.Context, check *config.Check) (string, error) {
	templateID, err := c.createJobTemplate(ctx, check)
	if err != nil {
		log.Warn().Err(err).Str("check", check.Name).Msg("Failed to sync job template - scheduled job creation will be skipped")
		return "", nil
	}

	if templateID == "" {
		log.Warn().Str("check", check.Name).Msg("Job template created but ID is empty - scheduled job creation will be skipped")
	}

	return templateID, nil
}

// createJobTemplate creates a new JobTemplate from a check configuration
func (c *GraphQLClient) createJobTemplate(ctx context.Context, check *config.Check) (string, error) {
	checkJSON, err := json.Marshal(check)
	if err != nil {
		return "", fmt.Errorf("failed to marshal check configuration: %w", err)
	}

	// Every 30 minutes - placeholder for server validation
	cron := "0 */30 * * * *"

	downloadURL := ""

	input := graphclient.CreateJobTemplateInput{
		Title:         check.Name,
		Description:   &check.Description,
		Platform:      enums.JobPlatformTypeGo,
		DownloadURL:   downloadURL,
		Configuration: coremodels.JobConfiguration(checkJSON),
		Cron:          &cron,
		Tags:          check.Tags,
	}

	resp, err := c.client.CreateJobTemplate(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to create job template: %w", err)
	}

	templateID := resp.CreateJobTemplate.JobTemplate.ID

	log.Debug().Str("template_id", templateID).Msg("CreateJobTemplate response received")

	queryResp, queryErr := c.client.GetJobTemplates(ctx, nil, nil, nil, nil, &graphclient.JobTemplateWhereInput{
		Title: &check.Name,
	}, nil)
	if queryErr != nil {
		log.Warn().Err(queryErr).Msg("Failed to query for job template after creation")
	} else if queryResp != nil && len(queryResp.JobTemplates.Edges) > 0 {
		queriedTemplate := queryResp.JobTemplates.Edges[0].Node
		log.Debug().
			Str("queried_id", queriedTemplate.ID).
			Str("queried_title", queriedTemplate.Title).
			Str("queried_display_id", queriedTemplate.DisplayID).
			Str("queried_platform", string(queriedTemplate.Platform)).
			Int("queried_config_len", len(queriedTemplate.Configuration)).
			Msg("GetJobTemplates query result after creation")

		if templateID == "" && queriedTemplate.ID != "" {
			log.Info().Str("check", check.Name).Str("template_id", queriedTemplate.ID).Msg("Using template ID from query since create response was empty")
			return queriedTemplate.ID, nil
		}
	}

	if templateID == "" {
		log.Warn().Str("check", check.Name).Msg("JobTemplate ID is empty after deserialization")
		return "", nil
	}

	log.Info().Str("check", check.Name).Str("template_id", templateID).Msg("Created job template")

	return templateID, nil
}
