package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/platform"
	"github.com/theopenlane/agent/internal/storage"
	"github.com/theopenlane/core/pkg/openlaneclient"
)

// ComplianceCheckController manages the execution of a single compliance check
// Similar to Buildkite's JobController but for compliance checks
type ComplianceCheckController struct {
	apiClient        *api.GraphQLClient
	agentID          string
	storage          storage.Storage
	evidenceService  *storage.EvidenceService
	platformSelector *platform.Selector

	// Check execution state
	stateMtx  sync.Mutex
	state     checkState
	startTime time.Time
	endTime   time.Time

	// Log streaming (similar to Buildkite's log chunks)
	logMutex    sync.Mutex
	logSequence uint64
	logOffset   uint64

	// Current executing check context
	currentCheck          *api.RemoteCheck
	currentScheduledJobID string
}

type checkState string

const (
	checkStateCreated  checkState = "created"
	checkStateStarted  checkState = "started"
	checkStateRunning  checkState = "running"
	checkStateFinished checkState = "finished"
	checkStateFailed   checkState = "failed"
)

// NewComplianceCheckController creates a new compliance check controller
func NewComplianceCheckController(apiClient *api.GraphQLClient, agentID string, storage storage.Storage, evidenceService *storage.EvidenceService) *ComplianceCheckController {
	return &ComplianceCheckController{
		apiClient:        apiClient,
		agentID:          agentID,
		storage:          storage,
		evidenceService:  evidenceService,
		platformSelector: platform.NewSelector(),
		state:            checkStateCreated,
	}
}

// ExecuteCheck executes a compliance check (similar to Buildkite's job execution)
func (c *ComplianceCheckController) ExecuteCheck(ctx context.Context, check *api.RemoteCheck) (*config.Result, error) {
	return c.executeCheckCommon(ctx, check, "")
}

// ExecuteScheduledJob executes a scheduled job and returns results
func (c *ComplianceCheckController) ExecuteScheduledJob(ctx context.Context, check *api.RemoteCheck, scheduledJob *openlaneclient.ScheduledJob) (*config.Result, error) {
	return c.executeCheckCommon(ctx, check, scheduledJob.ID)
}

// ExecuteLocalCheck executes a local check with full configuration support including evidence and actions
func (c *ComplianceCheckController) ExecuteLocalCheck(ctx context.Context, check *config.Check) (*config.Result, error) {
	c.setState(checkStateStarted)
	c.startTime = time.Now()

	// Apply platform-specific configuration if available
	platformVariant := c.platformSelector.SelectVariant(check)
	if platformVariant != nil {
		log.Info().Str("check", check.Name).Strs("platforms", platformVariant.Platforms).Msg("Applying platform-specific configuration")

		// Create a copy of the check to avoid modifying the original
		checkCopy := *check
		c.platformSelector.ApplyPlatformVariant(&checkCopy, platformVariant)
		check = &checkCopy
	} else if len(check.PlatformVariants) > 0 {
		// Check has platform variants but none match current platform
		platformInfo := c.platformSelector.GetPlatformInfo()
		log.Warn().Str("check", check.Name).Str("current_platform", platformInfo["platform"]).Msg("No platform variant matches current system, skipping check")

		result := &config.Result{
			CheckName:  check.Name,
			ExecutedAt: c.startTime,
			StartTime:  c.startTime,
			EndTime:    time.Now(),
			Controls:   check.Controls,
			Tags:       check.Tags,
			Error:      fmt.Sprintf("No platform variant available for %s", platformInfo["platform"]),
			Passed:     false,
		}

		return result, ErrCheckNotSupportedOnPlatform
	}

	log.Info().Str("check", check.Name).Msg("Executing local check with enhanced features")

	// Create the result object
	result := &config.Result{
		CheckName:  check.Name,
		ExecutedAt: c.startTime,
		StartTime:  c.startTime,
		Controls:   check.Controls,
		Tags:       check.Tags,
	}

	defer func() {
		c.endTime = time.Now()
		result.EndTime = c.endTime
		result.Duration = c.endTime.Sub(c.startTime).String()
		c.setState(checkStateFinished)
	}()

	// Handle different execution modes
	var stdout, stderr string

	var exitCode int

	var err error

	if platformVariant != nil && platformVariant.File != "" {
		// File-based check (gitMDM pattern)
		stdout, stderr, exitCode, err = c.executeFileCheck(check, platformVariant)
	} else {
		// Command-based check (existing pattern)
		stdout, stderr, exitCode, err = c.executeLocalCommand(ctx, check)
	}

	result.ExitCode = exitCode
	result.Stderr = stderr

	if err != nil {
		result.Error = err.Error()

		c.setState(checkStateFailed)

		return result, fmt.Errorf("%w: %w", ErrCheckExecutionTimeout, err)
	}

	// Apply platform-specific result evaluation if configured
	if platformVariant != nil {
		if err := c.evaluatePlatformResult(stdout, exitCode, platformVariant, result); err != nil {
			log.Error().Err(err).Str("check", check.Name).Msg("Platform result evaluation failed")
		}
	}

	// Parse the output as JSON (existing logic)
	if err := c.parseCheckOutput(stdout, result); err != nil {
		result.Error = fmt.Sprintf("Failed to parse check output: %v", err)
		log.Error().Err(err).Str("check", check.Name).Msg("Failed to parse output")

		// Store raw output for debugging
		result.Evidence = map[string]any{
			"raw_stdout":  stdout,
			"parse_error": err.Error(),
		}
	}

	// Determine pass/fail status
	result.Passed = c.determinePassStatus(result)

	// Collect evidence files
	evidenceFiles, err := c.handleEvidenceCollection(ctx, check, stdout, stderr)
	if err != nil {
		log.Error().Err(err).Str("check", check.Name).Msg("Failed to collect evidence")
	}

	// Store result with evidence using unified storage system
	if c.storage != nil {
		if err := c.storage.StoreResultWithEvidence(result, evidenceFiles); err != nil {
			log.Error().Err(err).Str("check", check.Name).Msg("Failed to store result")
		} else {
			log.Debug().Str("check", check.Name).Msg("Result stored successfully")
		}
	}

	// Execute pass/fail actions
	if err := c.executeActions(ctx, check, result); err != nil {
		log.Error().Err(err).Str("check", check.Name).Msg("Failed to execute actions")
	}

	log.Info().Str("check", check.Name).Int("exit_code", exitCode).Bool("passed", result.Passed).Msg("Local check completed")

	return result, nil
}

// executeCheckCommon contains the common execution logic for both checks and scheduled jobs
func (c *ComplianceCheckController) executeCheckCommon(ctx context.Context, check *api.RemoteCheck, scheduledJobID string) (*config.Result, error) {
	c.setState(checkStateStarted)
	c.startTime = time.Now()

	log.Info().Str("check", check.Name).Msg("Executing check")

	// Create the result object
	result := &config.Result{
		CheckName:      check.Name,
		ScheduledJobID: scheduledJobID,
		ExecutedAt:     c.startTime,
		StartTime:      c.startTime,
		Controls:       check.Controls,
		Tags:           check.Tags,
	}

	defer func() {
		c.endTime = time.Now()
		result.EndTime = c.endTime
		result.Duration = c.endTime.Sub(c.startTime).String()
		c.setState(checkStateFinished)
	}()

	// Parse timeout
	timeout := 5 * time.Minute // default // nolint:mnd

	if check.Timeout != "" {
		if d, err := time.ParseDuration(check.Timeout); err == nil {
			timeout = d
		}
	}

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c.setState(checkStateRunning)

	// Execute the compliance check command
	stdout, stderr, exitCode, err := c.executeCommand(execCtx, check)

	result.ExitCode = exitCode
	result.Stderr = stderr

	if err != nil {
		result.Error = err.Error()

		c.setState(checkStateFailed)

		return result, fmt.Errorf("%w: %w", ErrCheckExecutionTimeout, err)
	}

	// Parse the output as JSON (following our compliance check output format)
	if err := c.parseCheckOutput(stdout, result); err != nil {
		result.Error = fmt.Sprintf("Failed to parse check output: %v", err)
		log.Error().Err(err).Str("check", check.Name).Msg("Failed to parse output")

		// Store raw output for debugging
		result.Evidence = map[string]any{
			"raw_stdout":  stdout,
			"parse_error": err.Error(),
		}
	}

	log.Info().Str("check", check.Name).Int("exit_code", exitCode).Msg("Check completed")

	return result, nil
}

// executeCommand executes the compliance check command
// WARNING: This function executes arbitrary commands from the control system.
// Only trusted control configurations should be processed.
func (c *ComplianceCheckController) executeCommand(ctx context.Context, check *api.RemoteCheck) (string, string, int, error) {
	// Validate command input for basic security
	if check.Command == "" {
		return "", "", -1, ErrInvalidCommand
	}

	// Prepare command (G204: This is intentional for compliance execution)
	cmd := exec.CommandContext(ctx, check.Command, check.Args...) // #nosec G204

	// Set working directory if specified
	if check.WorkDir != "" {
		cmd.Dir = check.WorkDir
	}

	// Set environment variables
	env := make([]string, 0, len(check.Env)+10)
	env = append(env, check.Env...)

	// Add standard Openlane environment variables
	env = append(env, []string{
		fmt.Sprintf("OPENLANE_CHECK_NAME=%s", check.Name),
		fmt.Sprintf("OPENLANE_AGENT_ID=%s", c.agentID),
		fmt.Sprintf("OPENLANE_CHECK_TIMEOUT=%s", check.Timeout),
	}...)

	// Add control information
	if len(check.Controls) > 0 {
		env = append(env, fmt.Sprintf("OPENLANE_CONTROLS=%s", strings.Join(check.Controls, ",")))
	}

	// Add tags
	if len(check.Tags) > 0 {
		env = append(env, fmt.Sprintf("OPENLANE_TAGS=%s", strings.Join(check.Tags, ",")))
	}

	cmd.Env = env

	log.Debug().Str("cmd", check.Command).Msg("Executing command")

	// Execute command and capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
			// Non-zero exit code is not necessarily an error for compliance checks
			err = nil
		} else {
			return "", stderr.String(), -1, fmt.Errorf("%w: %w", ErrFailedToExecuteCommand, err)
		}
	}

	return stdout.String(), stderr.String(), exitCode, err
}

// parseCheckOutput parses the JSON output from a compliance check script
func (c *ComplianceCheckController) parseCheckOutput(output string, result *config.Result) error {
	output = strings.TrimSpace(output)

	if output == "" {
		// Empty output is acceptable - no findings
		result.Findings = []config.Finding{}
		return nil
	}

	// Try to parse as JSON
	var checkOutput struct {
		Findings []config.Finding `json:"findings"`
		Evidence map[string]any   `json:"evidence,omitempty"`
		Metrics  map[string]any   `json:"metrics,omitempty"`
		Error    string           `json:"error,omitempty"`
	}

	if err := json.Unmarshal([]byte(output), &checkOutput); err != nil {
		// If JSON parsing fails, treat entire output as a single info finding
		result.Findings = []config.Finding{
			{
				Resource:    "script-output",
				Title:       "Script Output",
				Description: "Raw script output (non-JSON format)",
				Severity:    config.SeverityInfo,
				Status:      config.StatusOpen,
				Details: map[string]any{
					"raw_output": output,
				},
			},
		}

		return nil
	}

	// Use parsed output
	result.Findings = checkOutput.Findings
	result.Evidence = checkOutput.Evidence
	result.Metrics = checkOutput.Metrics

	if checkOutput.Error != "" {
		result.Error = checkOutput.Error
	}

	// Validate and sanitize findings
	for i := range result.Findings {
		finding := &result.Findings[i]

		// Set defaults
		if finding.Status == "" {
			finding.Status = config.StatusOpen
		}

		if finding.Severity == "" {
			finding.Severity = config.SeverityMedium
		}

		// Validate severity
		if !c.isValidSeverity(finding.Severity) {
			log.Warn().Str("severity", string(finding.Severity)).Msg("Invalid severity, using medium")
			finding.Severity = config.SeverityMedium
		}

		// Ensure required fields
		if finding.Resource == "" {
			finding.Resource = "unknown"
		}

		if finding.Title == "" {
			finding.Title = "Untitled Finding"
		}
	}

	return nil
}

// isValidSeverity checks if a severity level is valid
func (c *ComplianceCheckController) isValidSeverity(severity config.FindingSeverity) bool {
	switch severity {
	case config.SeverityCritical, config.SeverityHigh, config.SeverityMedium, config.SeverityLow, config.SeverityInfo:
		return true
	default:
		return false
	}
}

// setState sets the current state of the check
func (c *ComplianceCheckController) setState(state checkState) {
	c.stateMtx.Lock()
	defer c.stateMtx.Unlock()
	c.state = state
}

// WriteLog writes log content for this check (similar to Buildkite's log streaming)
func (c *ComplianceCheckController) WriteLog(ctx context.Context, logLine string) error {
	c.logMutex.Lock()
	defer c.logMutex.Unlock()

	// Stream logs both locally and to platform
	checkName := "unknown"
	if c.currentCheck != nil {
		checkName = c.currentCheck.Name
	}

	log.Info().Str("check", checkName).Str("output", logLine).Uint64("sequence", c.logSequence).Msg("Check output")

	// Store log for potential platform streaming
	c.streamLogToPlatform(ctx, logLine)

	size := uint64(len(logLine))
	c.logSequence++
	c.logOffset += size

	return nil
}

// streamLogToPlatform streams log output to the Openlane platform if configured
func (c *ComplianceCheckController) streamLogToPlatform(ctx context.Context, logLine string) {
	// Check if we have a client and scheduled job ID for streaming
	if c.currentScheduledJobID == "" {
		// No job ID available for streaming
		return
	}

	checkName := "unknown"
	if c.currentCheck != nil {
		checkName = c.currentCheck.Name
	}

	// Create structured log entry for platform
	logEntry := map[string]any{
		"timestamp":        time.Now().UTC(),
		"check_name":       checkName,
		"scheduled_job_id": c.currentScheduledJobID,
		"sequence":         c.logSequence,
		"offset":           c.logOffset,
		"content":          logLine,
		"level":            "info",
	}

	// In a production implementation, this would stream to the platform
	// For now, we structure the log appropriately for future streaming
	log.Debug().Interface("log_entry", logEntry).Msg("Prepared log entry for platform streaming")
	// TODO: Implement actual platform streaming when GraphQL log streaming endpoint is available
	// This would involve:
	// 1. Batching logs to reduce API calls
	// 2. Buffering logs during network issues
	// 3. Retry logic for failed streams
	// 4. Compression for large log volumes
}

// GetCheckName returns a name for this check (for logging purposes)
func (c *ComplianceCheckController) GetCheckName() string {
	return fmt.Sprintf("check-%d", c.startTime.UnixNano())
}

// SetCurrentCheck sets the current check context for logging
func (c *ComplianceCheckController) SetCurrentCheck(check *api.RemoteCheck, scheduledJobID string) {
	c.currentCheck = check
	c.currentScheduledJobID = scheduledJobID
}

// GetStats returns statistics about this check execution
func (c *ComplianceCheckController) GetStats() map[string]any {
	c.stateMtx.Lock()
	defer c.stateMtx.Unlock()

	stats := map[string]any{
		"state":      string(c.state),
		"start_time": c.startTime,
		"agent_id":   c.agentID,
	}

	if !c.endTime.IsZero() {
		stats["end_time"] = c.endTime
		stats["duration"] = c.endTime.Sub(c.startTime).String()
	}

	return stats
}

// executeLocalCommand executes a command for a local check configuration
func (c *ComplianceCheckController) executeLocalCommand(ctx context.Context, check *config.Check) (string, string, int, error) {
	// Parse timeout
	timeout := check.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute // default // nolint:mnd
	}

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	c.setState(checkStateRunning)

	// Prepare command (G204: This is intentional for compliance execution)
	cmd := exec.CommandContext(execCtx, check.Command, check.Args...) // #nosec G204

	// Set working directory if specified
	if check.WorkDir != "" {
		cmd.Dir = check.WorkDir
	}

	// Set environment variables
	env := make([]string, 0, len(check.Env)+10)
	env = append(env, check.Env...)

	// Add standard Openlane environment variables
	env = append(env, []string{
		fmt.Sprintf("OPENLANE_CHECK_NAME=%s", check.Name),
		fmt.Sprintf("OPENLANE_AGENT_ID=%s", c.agentID),
		fmt.Sprintf("OPENLANE_CHECK_TIMEOUT=%s", timeout.String()),
	}...)

	// Add control information
	if len(check.Controls) > 0 {
		env = append(env, fmt.Sprintf("OPENLANE_CONTROLS=%s", strings.Join(check.Controls, ",")))
	}

	// Add tags
	if len(check.Tags) > 0 {
		env = append(env, fmt.Sprintf("OPENLANE_TAGS=%s", strings.Join(check.Tags, ",")))
	}

	cmd.Env = env

	log.Debug().Str("cmd", check.Command).Msg("Executing local command")

	// Execute command and capture output
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	exitCode := 0

	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
			// Non-zero exit code is not necessarily an error for compliance checks
			err = nil
		} else {
			return "", stderr.String(), -1, fmt.Errorf("%w: %w", ErrFailedToExecuteCommand, err)
		}
	}

	return stdout.String(), stderr.String(), exitCode, err
}

// determinePassStatus determines if a check passed based on exit code and findings
func (c *ComplianceCheckController) determinePassStatus(result *config.Result) bool {
	// If there was an execution error, it's a fail
	if result.Error != "" {
		return false
	}

	// Check exit code (0 = success by default)
	if result.ExitCode != 0 {
		return false
	}

	// Check findings for critical or high severity issues
	for _, finding := range result.Findings {
		if finding.Severity == config.SeverityCritical || finding.Severity == config.SeverityHigh {
			return false
		}
	}

	return true
}

// handleEvidenceCollection collects evidence files using the new unified storage system
func (c *ComplianceCheckController) handleEvidenceCollection(ctx context.Context, check *config.Check, stdout, stderr string) ([]storage.EvidenceFile, error) {
	if c.evidenceService == nil {
		log.Debug().Msg("Evidence service not available, skipping evidence collection")
		return nil, nil
	}

	var allEvidenceFiles []storage.EvidenceFile

	// Collect evidence from configured paths
	if len(check.EvidencePaths) > 0 {
		evidenceFiles, err := c.evidenceService.CollectEvidence(ctx, check.EvidencePaths)
		if err != nil {
			log.Error().Err(err).Msg("Failed to collect evidence from configured paths")
		} else {
			allEvidenceFiles = append(allEvidenceFiles, evidenceFiles...)
		}
	}

	// Create evidence from command output
	outputEvidenceFiles, err := c.evidenceService.CreateEvidenceFromOutput(check.Name, []byte(stdout), []byte(stderr))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create evidence from output")
	} else {
		allEvidenceFiles = append(allEvidenceFiles, outputEvidenceFiles...)
	}

	log.Debug().Int("evidence_files", len(allEvidenceFiles)).Str("check", check.Name).Msg("Evidence collection completed")

	return allEvidenceFiles, nil
}

// executeActions executes the appropriate actions based on pass/fail status
func (c *ComplianceCheckController) executeActions(ctx context.Context, check *config.Check, result *config.Result) error {
	var actionConfig *config.ActionConfig

	switch {
	case result.Passed && check.OnPass != nil:
		actionConfig = check.OnPass
		log.Info().Str("check", check.Name).Msg("Executing pass actions")
	case !result.Passed && check.OnFail != nil:
		actionConfig = check.OnFail
		log.Info().Str("check", check.Name).Msg("Executing fail actions")
	default:
		log.Debug().Str("check", check.Name).Bool("passed", result.Passed).Msg("No actions configured for this outcome")
		return nil
	}

	// Execute commands
	for _, command := range actionConfig.Commands {
		if err := c.executeActionCommand(ctx, command, check.Name); err != nil {
			log.Error().Err(err).Str("command", command.Name).Msg("Action command failed")

			if !command.ContinueOnError {
				return fmt.Errorf("%w %s: %w", ErrFailedToExecuteAction, command.Name, err)
			}
		}
	}

	// Update control status if configured
	if actionConfig.UpdateControlStatus {
		for _, controlID := range check.Controls {
			if err := c.updateControlStatus(ctx, controlID, result.Passed); err != nil {
				log.Error().Err(err).Str("control_id", controlID).Msg("Failed to update control status")
			}
		}
	}

	return nil
}

// executeActionCommand executes a single action command
func (c *ComplianceCheckController) executeActionCommand(ctx context.Context, actionCmd config.ActionCommand, checkName string) error {
	// Use configured timeout or default
	timeout := actionCmd.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute // default // nolint:mnd
	}

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Prepare command (G204: This is intentional for compliance execution)
	cmd := exec.CommandContext(execCtx, actionCmd.Command, actionCmd.Args...) // #nosec G204

	// Set working directory if specified
	if actionCmd.WorkDir != "" {
		cmd.Dir = actionCmd.WorkDir
	}

	// Set environment variables
	env := make([]string, 0, len(actionCmd.Env)+5)
	env = append(env, actionCmd.Env...)

	// Add context environment variables
	env = append(env, []string{
		fmt.Sprintf("OPENLANE_CHECK_NAME=%s", checkName),
		fmt.Sprintf("OPENLANE_ACTION_NAME=%s", actionCmd.Name),
		fmt.Sprintf("OPENLANE_AGENT_ID=%s", c.agentID),
	}...)

	cmd.Env = env

	log.Info().Str("command", actionCmd.Command).Str("action", actionCmd.Name).Msg("Executing action command")

	// Execute command
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().Err(err).Str("output", string(output)).Msg("Action command failed")
		return fmt.Errorf("%w: %w", ErrFailedToExecuteCommand, err)
	}

	log.Info().Str("action", actionCmd.Name).Msg("Action command completed successfully")

	return nil
}

// updateControlStatus updates the status of a control based on the check result
func (c *ComplianceCheckController) updateControlStatus(ctx context.Context, controlID string, passed bool) error {
	if c.apiClient == nil {
		log.Debug().Msg("API client not available, skipping control status update")
		return nil
	}

	log.Info().Str("control_id", controlID).Bool("passed", passed).Msg("Updating control status")

	// This would update the control status in the Openlane platform
	// The exact implementation would depend on the available GraphQL mutations
	status := "compliant"
	if !passed {
		status = "non_compliant"
	}

	// Placeholder for actual control update
	log.Info().Str("control_id", controlID).Str("status", status).Msg("Control status update (placeholder)")

	return nil
}

// executeFileCheck performs a file-based check following gitMDM patterns
func (c *ComplianceCheckController) executeFileCheck(check *config.Check, variant *config.PlatformVariant) (string, string, int, error) {
	log.Debug().Str("file", variant.File).Str("check", check.Name).Msg("Executing file-based check")

	// Read the file content
	content, err := os.ReadFile(variant.File)
	if err != nil {
		return "", fmt.Sprintf("Failed to read file %s: %v", variant.File, err), 1, nil
	}

	contentStr := string(content)

	return contentStr, "", 0, nil
}

// evaluatePlatformResult evaluates check results using platform-specific patterns
func (c *ComplianceCheckController) evaluatePlatformResult(stdout string, exitCode int, variant *config.PlatformVariant, result *config.Result) error {
	// Handle exit code evaluation
	if variant.ExitCode != nil {
		expectedExitCode := *variant.ExitCode
		if exitCode != expectedExitCode {
			result.Passed = false
			result.Findings = append(result.Findings, config.Finding{
				Resource:    "exit_code",
				Title:       "Unexpected Exit Code",
				Description: fmt.Sprintf("Expected exit code %d, got %d", expectedExitCode, exitCode),
				Severity:    config.SeverityHigh,
				Status:      config.StatusOpen,
				Details: map[string]any{
					"expected_exit_code": expectedExitCode,
					"actual_exit_code":   exitCode,
				},
			})
		}
	}

	// Handle includes pattern (pass if matches)
	if variant.Includes != "" {
		matched, err := regexp.MatchString(variant.Includes, stdout)
		if err != nil {
			return fmt.Errorf("%w %s: %w", ErrInvalidRegex, variant.Includes, err)
		}

		if !matched {
			result.Passed = false
			result.Findings = append(result.Findings, config.Finding{
				Resource:    "stdout_pattern",
				Title:       "Required Pattern Not Found",
				Description: fmt.Sprintf("Output did not match required pattern: %s", variant.Includes),
				Severity:    config.SeverityHigh,
				Status:      config.StatusOpen,
				Details: map[string]any{
					"required_pattern": variant.Includes,
					"output_preview":   truncateString(stdout, 200), // nolint:mnd
				},
			})
		}
	}

	// Handle excludes pattern (fail if matches)
	if variant.Excludes != "" {
		matched, err := regexp.MatchString(variant.Excludes, stdout)
		if err != nil {
			return fmt.Errorf("%w %s: %w", ErrInvalidRegex, variant.Excludes, err)
		}

		if matched {
			result.Passed = false
			result.Findings = append(result.Findings, config.Finding{
				Resource:    "stdout_pattern",
				Title:       "Prohibited Pattern Found",
				Description: fmt.Sprintf("Output matched prohibited pattern: %s", variant.Excludes),
				Severity:    config.SeverityHigh,
				Status:      config.StatusOpen,
				Details: map[string]any{
					"prohibited_pattern": variant.Excludes,
					"output_preview":     truncateString(stdout, 200), // nolint:mnd
				},
			})
		}
	}

	// Add remediation steps if check failed and remediation is available
	if !result.Passed && len(variant.Remediation) > 0 {
		if result.Evidence == nil {
			result.Evidence = make(map[string]any)
		}

		result.Evidence["remediation_steps"] = variant.Remediation
	}

	return nil
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return s[:maxLen] + "..." // nolint:mnd
}
