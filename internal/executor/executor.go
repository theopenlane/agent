package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/theopenlane/agent/internal/config"
)

// Executor handles the execution of compliance checks
type Executor struct {
	dataDir        string
	maxConcurrency int
	logger         zerolog.Logger

	// Semaphore for concurrency control
	semaphore chan struct{}

	// Statistics
	stats Stats
	mu    sync.RWMutex
}

// Stats tracks executor statistics
type Stats struct {
	TotalExecutions int64         `json:"total_executions"`
	SuccessfulRuns  int64         `json:"successful_runs"`
	FailedRuns      int64         `json:"failed_runs"`
	AverageRunTime  time.Duration `json:"average_run_time"`
	ActiveChecks    int           `json:"active_checks"`
	LastExecution   time.Time     `json:"last_execution"`
}

// NewExecutor creates a new executor
func NewExecutor(dataDir string, maxConcurrency int, log zerolog.Logger) *Executor {
	return &Executor{
		dataDir:        dataDir,
		maxConcurrency: maxConcurrency,
		logger:         log,
		semaphore:      make(chan struct{}, maxConcurrency),
	}
}

// ExecuteCheck executes a single compliance check
func (e *Executor) ExecuteCheck(ctx context.Context, check *config.Check) *config.Result {
	// Acquire semaphore for concurrency control
	select {
	case e.semaphore <- struct{}{}:
		defer func() { <-e.semaphore }()
	case <-ctx.Done():
		return &config.Result{
			CheckName:  check.Name,
			ExecutedAt: time.Now(),
			Error:      "execution cancelled due to context cancellation",
			ExitCode:   -1,
			Controls:   check.Controls,
			Tags:       check.Tags,
		}
	}

	startTime := time.Now()

	result := &config.Result{
		CheckName:  check.Name,
		ExecutedAt: startTime,
		Controls:   check.Controls,
		Tags:       check.Tags,
	}

	// Update stats
	e.mu.Lock()
	e.stats.TotalExecutions++
	e.stats.ActiveChecks++
	e.stats.LastExecution = startTime
	e.mu.Unlock()

	defer func() {
		duration := time.Since(startTime)
		result.Duration = duration.String()

		// Update stats
		e.mu.Lock()
		e.stats.ActiveChecks--
		if result.Error == "" && result.ExitCode == 0 {
			e.stats.SuccessfulRuns++
		} else {
			e.stats.FailedRuns++
		}

		// Update average run time
		if e.stats.TotalExecutions > 0 {
			totalTime := e.stats.AverageRunTime * time.Duration(e.stats.TotalExecutions-1)
			e.stats.AverageRunTime = (totalTime + duration) / time.Duration(e.stats.TotalExecutions)
		} else {
			e.stats.AverageRunTime = duration
		}
		e.mu.Unlock()

		e.logger.Info().Str("check_name", check.Name).Str("duration", duration.String()).Int("exit_code", result.ExitCode).Msg("Check completed")
	}()

	e.logger.Info().Str("check_name", check.Name).Msg("Executing check")

	// Create execution context with timeout
	execCtx, cancel := context.WithTimeout(ctx, check.Timeout)
	defer cancel()

	// Validate check script exists (if it's a file)
	if err := e.validateCheckScript(check); err != nil {
		result.Error = fmt.Sprintf("Check validation failed: %v", err)
		result.ExitCode = -1
		return result
	}

	// Execute the command
	stdout, stderr, exitCode, err := e.executeCommand(execCtx, check)

	result.ExitCode = exitCode
	result.Stderr = stderr

	if err != nil {
		result.Error = err.Error()
		e.logger.Error().Str("check_name", check.Name).Err(err).Msg("Check failed")
		return result
	}

	// Parse the output as JSON
	if err := e.parseCheckOutput(stdout, result); err != nil {
		result.Error = fmt.Sprintf("Failed to parse check output: %v", err)
		e.logger.Error().Str("check_name", check.Name).Err(err).Msg("Failed to parse output for check")

		// Store raw output for debugging
		result.Evidence = map[string]any{
			"raw_stdout":  stdout,
			"parse_error": err.Error(),
		}
		return result
	}

	return result
}

// executeCommand executes the command and returns stdout, stderr, exit code, and error
func (e *Executor) executeCommand(ctx context.Context, check *config.Check) (string, string, int, error) {
	// Prepare command
	cmd := exec.CommandContext(ctx, check.Command, check.Args...)

	// Set working directory
	workDir := check.WorkDir
	if workDir != "" {
		if !filepath.IsAbs(workDir) {
			workDir = filepath.Join(e.dataDir, workDir)
		}
		cmd.Dir = workDir
	} else {
		cmd.Dir = e.dataDir
	}

	// Set environment variables
	env := os.Environ()
	env = append(env, check.Env...)

	// Add standard Openlane environment variables
	env = append(env, []string{
		fmt.Sprintf("OPENLANE_CHECK_NAME=%s", check.Name),
		fmt.Sprintf("OPENLANE_DATA_DIR=%s", e.dataDir),
		fmt.Sprintf("OPENLANE_WORK_DIR=%s", cmd.Dir),
	}...)

	cmd.Env = env

	e.logger.Debug().Str("command", check.Command).Interface("args", check.Args).Str("dir", cmd.Dir).Msg("Executing command")

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
			return "", stderr.String(), -1, fmt.Errorf("failed to execute command: %w", err)
		}
	}

	return stdout.String(), stderr.String(), exitCode, err
}

// parseCheckOutput parses the JSON output from a check script
func (e *Executor) parseCheckOutput(output string, result *config.Result) error {
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
		if !isValidSeverity(finding.Severity) {
			e.logger.Warn().Str("severity", string(finding.Severity)).Str("title", finding.Title).Msg("Invalid severity for finding, using 'medium'")
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

// validateCheckScript validates that a check script exists and is executable
func (e *Executor) validateCheckScript(check *config.Check) error {
	// If command is a built-in or in PATH, skip file checks
	if e.isBuiltinCommand(check.Command) {
		return nil
	}

	// Check if it's an absolute path
	var scriptPath string
	if filepath.IsAbs(check.Command) {
		scriptPath = check.Command
	} else {
		// Relative to working directory or data directory
		workDir := check.WorkDir
		if workDir == "" {
			workDir = e.dataDir
		} else if !filepath.IsAbs(workDir) {
			workDir = filepath.Join(e.dataDir, workDir)
		}
		scriptPath = filepath.Join(workDir, check.Command)
	}

	// Check if file exists
	info, err := os.Stat(scriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("script not found: %s", scriptPath)
		}
		return fmt.Errorf("cannot access script %s: %w", scriptPath, err)
	}

	// Check if it's executable (Unix-like systems)
	if info.Mode()&0111 == 0 {
		e.logger.Warn().Str("script_path", scriptPath).Str("mode", info.Mode().String()).Msg("Script may not be executable")
	}

	return nil
}

// isBuiltinCommand checks if a command is a built-in or available in PATH
func (e *Executor) isBuiltinCommand(cmd string) bool {
	builtins := []string{
		"python", "python3", "ruby", "node", "nodejs", "bash", "sh", "perl", "php",
		"aws", "gcloud", "kubectl", "curl", "jq", "grep", "sed", "awk",
		"docker", "podman", "git", "ssh", "scp", "rsync",
	}

	for _, builtin := range builtins {
		if cmd == builtin {
			return true
		}
	}

	// Check if command is in PATH
	_, err := exec.LookPath(cmd)
	return err == nil
}

// isValidSeverity checks if a severity level is valid
func isValidSeverity(severity config.FindingSeverity) bool {
	switch severity {
	case config.SeverityCritical, config.SeverityHigh, config.SeverityMedium, config.SeverityLow, config.SeverityInfo:
		return true
	default:
		return false
	}
}

// GetStats returns executor statistics
func (e *Executor) GetStats() Stats {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.stats
}

// ExecuteChecks executes multiple checks concurrently
func (e *Executor) ExecuteChecks(ctx context.Context, checks []*config.Check) []*config.Result {
	if len(checks) == 0 {
		return []*config.Result{}
	}

	results := make([]*config.Result, len(checks))
	var wg sync.WaitGroup

	// Execute checks concurrently
	for i, check := range checks {
		wg.Add(1)
		go func(index int, ch *config.Check) {
			defer wg.Done()
			results[index] = e.ExecuteCheck(ctx, ch)
		}(i, check)
	}

	// Wait for all to complete
	wg.Wait()

	return results
}

// Helper functions for creating properly formatted output

// CreateCheckOutput creates a properly formatted JSON output for checks
func CreateCheckOutput(findings []config.Finding, evidence map[string]any, metrics map[string]any) ([]byte, error) {
	output := struct {
		Findings []config.Finding `json:"findings"`
		Evidence map[string]any   `json:"evidence,omitempty"`
		Metrics  map[string]any   `json:"metrics,omitempty"`
	}{
		Findings: findings,
		Evidence: evidence,
		Metrics:  metrics,
	}

	return json.MarshalIndent(output, "", "  ")
}

// CreateFinding creates a properly formatted finding
func CreateFinding(resource, title, description string, severity config.FindingSeverity) config.Finding {
	return config.Finding{
		Resource:    resource,
		Title:       title,
		Description: description,
		Severity:    severity,
		Status:      config.StatusOpen,
	}
}
