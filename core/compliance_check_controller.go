package core

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/theopenlane/agent/internal/models"
	"github.com/theopenlane/agent/internal/platform"
	"github.com/theopenlane/agent/internal/storage"
	"github.com/theopenlane/core/common/enums"
)

const (
	// Command execution defaults
	defaultExecutionTimeoutMinutes = 5
	defaultEnvBufferSize           = 10
	actionEnvBufferSize            = 5
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
	currentCheckName string
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

// ExecuteCheck executes a compliance check using the unified config.Check format
func (c *ComplianceCheckController) ExecuteCheck(ctx context.Context, check *config.Check) (*config.Result, error) {
	return c.ExecuteLocalCheck(ctx, check)
}

// ExecuteLocalCheck executes a local check with full configuration support including evidence and actions
func (c *ComplianceCheckController) ExecuteLocalCheck(ctx context.Context, check *config.Check) (*config.Result, error) {
	c.setState(checkStateStarted)
	c.startTime = time.Now()

	// Set the current execution context for logging
	c.SetCurrentCheck(check.Name)

	// Validate compliance controls before execution
	validatedControls, shouldExecute, reason := c.validateControlsBeforeExecution(ctx, check)
	if !shouldExecute {
		log.Warn().Str("check", check.Name).Str("reason", reason).Msg("Skipping check execution due to control validation")

		result := &config.Result{
			CheckName:  check.Name,
			StartedAt:  c.startTime,
			FinishedAt: time.Now(),
			Status:     enums.JobExecutionStatusFailed,
			Error:      fmt.Sprintf("Check skipped: %s", reason),
			ExitCode:   &[]int{1}[0],
			Metadata:   c.baseMetadata(),
		}
		c.attachComplianceMetadata(ctx, result, check, validatedControls)
		// Add duration to metadata
		result.Metadata["duration"] = result.FinishedAt.Sub(result.StartedAt).String()

		return result, nil
	}

	log.Info().Str("check", check.Name).Interface("validated_controls", validatedControls).Msg("Control validation passed, proceeding with check execution")

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
			StartedAt:  c.startTime,
			FinishedAt: time.Now(),
			Status:     enums.JobExecutionStatusFailed,
			Error:      fmt.Sprintf("No platform variant available for %s", platformInfo["platform"]),
			ExitCode:   &[]int{1}[0],
			Metadata:   c.baseMetadata(),
		}
		c.attachComplianceMetadata(ctx, result, check, validatedControls)

		return result, ErrCheckNotSupportedOnPlatform
	}

	log.Info().Str("check", check.Name).Msg("Executing local check with enhanced features")

	// Create the result object
	result := &config.Result{
		CheckName: check.Name,
		StartedAt: c.startTime,
		Status:    "PENDING",
		Metadata:  c.baseMetadata(),
	}
	c.attachComplianceMetadata(ctx, result, check, validatedControls)

	defer func() {
		c.endTime = time.Now()
		result.FinishedAt = c.endTime
		result.Metadata["duration"] = c.endTime.Sub(c.startTime).String()
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

	result.ExitCode = &exitCode
	result.Log = stdout
	// Store stderr in metadata if present
	if stderr != "" {
		result.Metadata["stderr"] = stderr
	}

	if err != nil {
		result.Error = err.Error()

		c.setState(checkStateFailed)

		return result, err
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

		// Store raw output for debugging in metadata
		result.Metadata["raw_stdout"] = stdout
		result.Metadata["parse_error"] = err.Error()
	}

	// Determine pass/fail status
	result.Status = c.determineStatus(result)

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

	log.Info().Str("check", check.Name).Int("exit_code", exitCode).Str("status", result.Status.String()).Msg("Local check completed")

	return result, nil
}

// parseCheckOutput parses the JSON output from a compliance check script
func (c *ComplianceCheckController) parseCheckOutput(output string, result *config.Result) error {
	output = strings.TrimSpace(output)

	if output == "" {
		// Empty output is acceptable
		result.Log = ""
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
		// If JSON parsing fails, store entire output as log content
		result.Log = output
		return nil
	}

	// Store parsed output in log and metadata
	if len(checkOutput.Findings) > 0 {
		result.Metadata["findings"] = checkOutput.Findings
	}

	if checkOutput.Evidence != nil {
		result.Metadata["evidence"] = checkOutput.Evidence
	}

	if checkOutput.Metrics != nil {
		result.Metadata["metrics"] = checkOutput.Metrics
	}

	if checkOutput.Error != "" {
		result.Error = checkOutput.Error
	}

	// Store main output in log field
	result.Log = output

	// Validate findings stored in metadata
	if findings, ok := result.Metadata["findings"].([]config.Finding); ok {
		// Validate and sanitize findings
		for i := range findings {
			finding := &findings[i]

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

		result.Metadata["findings"] = findings
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

// WriteLog writes check log content to the local logger
func (c *ComplianceCheckController) WriteLog(logLine string) error {
	c.logMutex.Lock()
	defer c.logMutex.Unlock()

	checkName := c.currentCheckName
	if checkName == "" {
		checkName = "unknown"
	}

	log.Info().Str("check", checkName).Str("output", logLine).Uint64("sequence", c.logSequence).Msg("Check output")

	size := uint64(len(logLine))
	c.logSequence++
	c.logOffset += size

	return nil
}

// GetCheckName returns a name for this check (for logging purposes)
func (c *ComplianceCheckController) GetCheckName() string {
	return fmt.Sprintf("check-%d", c.startTime.UnixNano())
}

// SetCurrentCheck sets the current check context for logging
func (c *ComplianceCheckController) SetCurrentCheck(checkName string) {
	c.currentCheckName = checkName
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
		timeout = defaultExecutionTimeoutMinutes * time.Minute // default
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
	env := make([]string, 0, len(os.Environ())+len(check.Env)+defaultEnvBufferSize)
	env = append(env, os.Environ()...)
	env = append(env, check.Env...)

	// Add standard Openlane environment variables
	env = append(env, []string{
		fmt.Sprintf("OPENLANE_CHECK_NAME=%s", check.Name),
		fmt.Sprintf("OPENLANE_AGENT_ID=%s", c.agentID),
		fmt.Sprintf("OPENLANE_CHECK_TIMEOUT=%s", timeout.String()),
	}...)

	// Add control information
	allControls := check.GetAllControls()
	if len(allControls) > 0 {
		env = append(env, fmt.Sprintf("OPENLANE_CONTROLS=%s", strings.Join(allControls, ",")))
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
		} else if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return stdout.String(), stderr.String(), -1, ErrCheckExecutionTimeout
		} else {
			return "", stderr.String(), -1, fmt.Errorf("%w: %w", ErrFailedToExecuteCommand, err)
		}
	}

	return stdout.String(), stderr.String(), exitCode, err
}

// determineStatus determines the status based on exit code, error, and log content
func (c *ComplianceCheckController) determineStatus(result *config.Result) enums.JobExecutionStatus {
	if result.Status == enums.JobExecutionStatusFailed {
		return enums.JobExecutionStatusFailed
	}

	if result.Error != "" {
		return enums.JobExecutionStatusFailed
	}

	if result.ExitCode != nil && *result.ExitCode != 0 {
		return enums.JobExecutionStatusFailed
	}

	return enums.JobExecutionStatusSuccess
}

// handleEvidenceCollection collects evidence files using the new unified storage system
func (c *ComplianceCheckController) handleEvidenceCollection(ctx context.Context, check *config.Check, stdout, stderr string) ([]models.EvidenceFile, error) {
	if c.evidenceService == nil {
		log.Debug().Msg("Evidence service not available, skipping evidence collection")
		return nil, nil
	}

	var allEvidenceFiles []models.EvidenceFile

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
	outputEvidenceFiles, err := c.evidenceService.CreateEvidenceFromOutput(ctx, check.Name, []byte(stdout), []byte(stderr))
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
	case result.Status == enums.JobExecutionStatusSuccess && check.OnPass != nil:
		actionConfig = check.OnPass
		log.Info().Str("check", check.Name).Msg("Executing pass actions")
	case result.Status == enums.JobExecutionStatusFailed && check.OnFail != nil:
		actionConfig = check.OnFail
		log.Info().Str("check", check.Name).Msg("Executing fail actions")
	default:
		log.Debug().Str("check", check.Name).Str("status", result.Status.String()).Msg("No actions configured for this outcome")
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

	return nil
}

// executeActionCommand executes a single action command
func (c *ComplianceCheckController) executeActionCommand(ctx context.Context, actionCmd config.ActionCommand, checkName string) error {
	// Use configured timeout or default
	timeout := actionCmd.Timeout
	if timeout == 0 {
		timeout = defaultExecutionTimeoutMinutes * time.Minute // default
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
	env := make([]string, 0, len(os.Environ())+len(actionCmd.Env)+actionEnvBufferSize)
	env = append(env, os.Environ()...)
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

func (c *ComplianceCheckController) baseMetadata() map[string]any {
	return make(map[string]any)
}

func (c *ComplianceCheckController) attachComplianceMetadata(ctx context.Context, result *config.Result, check *config.Check, validatedControls map[string][]string) {
	if result == nil || check == nil || len(check.ComplianceStandards) == 0 {
		return
	}

	if result.Metadata == nil {
		result.Metadata = make(map[string]any)
	}

	result.Metadata["compliance_standards"] = check.ComplianceStandards
	if len(validatedControls) > 0 {
		result.Metadata["validated_controls"] = validatedControls
	}

	primaryStandard, primaryControlRef := selectPrimaryComplianceReference(check, validatedControls)
	if primaryStandard != "" {
		result.Standard = primaryStandard
		result.Metadata["standard"] = primaryStandard
	}

	if primaryControlRef != "" {
		result.ControlRef = primaryControlRef
		result.Metadata["control_ref"] = primaryControlRef
	}

	controlIDs, err := c.resolveControlIDs(ctx, check, validatedControls)
	if err != nil {
		log.Warn().Err(err).Str("check", check.Name).Msg("Failed to resolve control IDs for evidence association")
	}

	if len(controlIDs) > 0 {
		result.Metadata["control_ids"] = controlIDs
	}
}

func selectPrimaryComplianceReference(check *config.Check, validatedControls map[string][]string) (string, string) {
	if check == nil {
		return "", ""
	}

	for _, standard := range check.ComplianceStandards {
		if controls, ok := validatedControls[standard.Standard]; ok && len(controls) > 0 {
			return standard.Standard, controls[0]
		}
	}

	for _, standard := range check.ComplianceStandards {
		if len(standard.Controls) > 0 {
			return standard.Standard, standard.Controls[0]
		}
	}

	return "", ""
}

func (c *ComplianceCheckController) resolveControlIDs(ctx context.Context, check *config.Check, validatedControls map[string][]string) ([]string, error) {
	if c.apiClient == nil || check == nil || len(check.ComplianceStandards) == 0 {
		return nil, nil
	}

	standardsToResolve := make([]config.ComplianceStandard, 0, len(check.ComplianceStandards))

	for _, configured := range check.ComplianceStandards {
		controls := configured.Controls
		if validated, ok := validatedControls[configured.Standard]; ok && len(validated) > 0 {
			controls = validated
		}

		if len(controls) == 0 {
			continue
		}

		standardsToResolve = append(standardsToResolve, config.ComplianceStandard{
			Standard: configured.Standard,
			Controls: controls,
		})
	}

	if len(standardsToResolve) == 0 {
		return nil, nil
	}

	return c.apiClient.ResolveControlIDs(ctx, standardsToResolve)
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
			result.Status = enums.JobExecutionStatusFailed
			// Add failure reason to log content
			if result.Log == "" {
				result.Log = fmt.Sprintf("Expected exit code %d, got %d", expectedExitCode, exitCode)
			} else {
				result.Log += fmt.Sprintf("\nExpected exit code %d, got %d", expectedExitCode, exitCode)
			}
		}
	}

	// Handle includes pattern (pass if matches)
	if variant.Includes != "" {
		matched, err := regexp.MatchString(variant.Includes, stdout)
		if err != nil {
			return fmt.Errorf("%w %s: %w", ErrInvalidRegex, variant.Includes, err)
		}

		if !matched {
			result.Status = enums.JobExecutionStatusFailed
			// Add failure reason to log content
			if result.Log == "" {
				result.Log = fmt.Sprintf("Output did not match required pattern: %s", variant.Includes)
			} else {
				result.Log += fmt.Sprintf("\nOutput did not match required pattern: %s", variant.Includes)
			}
		}
	}

	// Handle excludes pattern (fail if matches)
	if variant.Excludes != "" {
		matched, err := regexp.MatchString(variant.Excludes, stdout)
		if err != nil {
			return fmt.Errorf("%w %s: %w", ErrInvalidRegex, variant.Excludes, err)
		}

		if matched {
			result.Status = enums.JobExecutionStatusFailed
			// Add failure reason to log content
			if result.Log == "" {
				result.Log = fmt.Sprintf("Output matched prohibited pattern: %s", variant.Excludes)
			} else {
				result.Log += fmt.Sprintf("\nOutput matched prohibited pattern: %s", variant.Excludes)
			}
		}
	}

	// Add remediation steps if check failed and remediation is available
	if result.Status == enums.JobExecutionStatusFailed && len(variant.Remediation) > 0 {
		if result.Metadata == nil {
			result.Metadata = make(map[string]any)
		}

		result.Metadata["remediation_steps"] = variant.Remediation
	}

	return nil
}

// validateControlsBeforeExecution validates controls before executing a check using fail-open logic
func (c *ComplianceCheckController) validateControlsBeforeExecution(ctx context.Context, check *config.Check) (map[string][]string, bool, string) {
	// If no API client available, skip validation and allow execution
	if c.apiClient == nil {
		log.Debug().Str("check", check.Name).Msg("No API client available, skipping control validation")
		return make(map[string][]string), true, "no API client for validation"
	}

	// If no compliance standards configured, allow execution
	if len(check.ComplianceStandards) == 0 {
		log.Debug().Str("check", check.Name).Msg("No compliance standards configured, proceeding without validation")
		return make(map[string][]string), true, "no compliance standards configured"
	}

	// Validate controls against Openlane system
	validatedControls, err := c.apiClient.ValidateControls(ctx, check.ComplianceStandards)
	if err != nil {
		log.Error().Err(err).Str("check", check.Name).Msg("Control validation failed, proceeding anyway (fail-open)")
		return make(map[string][]string), true, "control validation failed but proceeding (fail-open)"
	}

	// Use the check's built-in logic to determine if it should execute
	shouldExecute, validControls, reason := check.ShouldExecuteCheck(validatedControls)

	if !shouldExecute {
		log.Warn().Str("check", check.Name).Str("reason", reason).Msg("Check execution blocked by control validation logic")
	} else {
		log.Info().Str("check", check.Name).Interface("valid_controls", validControls).Str("reason", reason).Msg("Control validation successful")
	}

	return validControls, shouldExecute, reason
}
