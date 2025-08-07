package clicommand

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/core"
	"github.com/theopenlane/agent/internal/config"
	"github.com/theopenlane/agent/version"
	"github.com/urfave/cli"
)

const startDescription = `Usage:

    openlane-agent start [options...]

Description:

Starts the Openlane compliance agent with the specified configuration.
The agent will continuously run compliance checks according to their
schedules and report results to the Openlane platform.

The agent requires a configuration file (agent.yaml) that defines:
- API connection settings
- Compliance checks to run
- Scheduling information
- Environment variables and credentials

Example:

    # Start with default config
    openlane-agent start

    # Start with custom config file
    openlane-agent start --config /path/to/agent.yaml

    # Start with debug logging
    openlane-agent start --log-level debug

    # Start in foreground (don't daemonize)
    openlane-agent start --no-daemon
`

// StopFlags are the flags for the stop command
var StopFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "pid-file",
		Value: "agent.pid",
		Usage: "Path to the PID file",
	},
}

// StatusFlags are the flags for the status command
var StatusFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "pid-file",
		Value: "agent.pid",
		Usage: "Path to the PID file",
	},
	cli.StringFlag{
		Name:  "config",
		Value: "agent.yaml",
		Usage: "Path to the agent configuration file",
	},
}

// StartFlags are the flags for the start command
var StartFlags = []cli.Flag{
	cli.StringFlag{
		Name:   "config",
		Value:  "agent.yaml",
		Usage:  "Path to the agent configuration file",
		EnvVar: "OPENLANE_AGENT_CONFIG",
	},
	cli.StringFlag{
		Name:   "log-level",
		Value:  "info",
		Usage:  "Set the log level (debug, info, warn, error)",
		EnvVar: "OPENLANE_AGENT_LOG_LEVEL",
	},
	cli.StringFlag{
		Name:   "data-dir",
		Value:  "./data",
		Usage:  "Directory for agent data and state",
		EnvVar: "OPENLANE_AGENT_DATA_DIR",
	},
	cli.StringFlag{
		Name:   "api-key",
		Usage:  "Openlane API key (overrides config file)",
		EnvVar: "OPENLANE_API_KEY",
	},
	cli.StringFlag{
		Name:   "api-url",
		Value:  "https://api.openlane.io",
		Usage:  "Openlane API URL (overrides config file)",
		EnvVar: "OPENLANE_API_URL",
	},
	cli.BoolFlag{
		Name:  "no-daemon",
		Usage: "Run in foreground instead of daemonizing",
	},
	cli.StringFlag{
		Name:  "pid-file",
		Value: "agent.pid",
		Usage: "Path to the PID file (daemon mode only)",
	},
	cli.IntFlag{
		Name:  "max-concurrency",
		Value: 3,
		Usage: "Maximum number of concurrent checks",
	},
	cli.BoolFlag{
		Name:  "dry-run",
		Usage: "Validate configuration and exit without starting",
	},
}

// StartAction starts the agent
func StartAction(c *cli.Context) error {
	// Load configuration
	configPath := c.String("config")
	cfg, err := loadAndValidateConfig(configPath, c)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Setup logger
	logLevel, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		return fmt.Errorf("invalid log level: %w", err)
	}

	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).Level(logLevel).With().Timestamp().Logger()

	// Override config with CLI flags if provided
	if apiKey := c.String("api-key"); apiKey != "" {
		cfg.RegistrationToken = apiKey
	}
	if apiURL := c.String("api-url"); apiURL != "" {
		cfg.APIURL = apiURL
	}
	if dataDir := c.String("data-dir"); dataDir != "" {
		cfg.DataDir = dataDir
	}
	if maxConcurrency := c.Int("max-concurrency"); maxConcurrency > 0 {
		cfg.MaxConcurrency = maxConcurrency
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	logger.Info().Str("version", version.FullVersion()).Msg("Starting Openlane Agent")
	logger.Info().Str("config_path", configPath).Msg("Configuration loaded")
	logger.Info().Str("data_dir", cfg.DataDir).Msg("Data directory set")
	logger.Info().Str("api_url", cfg.APIURL).Msg("API endpoint configured")
	logger.Info().Int("count", len(cfg.GetEnabledChecks())).Msg("Enabled checks loaded")

	// Dry run mode
	if c.Bool("dry-run") {
		logger.Info().Msg("Dry run mode - configuration is valid, exiting")
		return nil
	}

	// Handle daemon mode
	if !c.Bool("no-daemon") {
		pidFile := c.String("pid-file")
		if pidFile == "" {
			pidFile = "agent.pid"
		}
		
		if err := daemonize(logger, pidFile); err != nil {
			return fmt.Errorf("failed to daemonize: %w", err)
		}
	}

	// Create the agent (following Buildkite's pattern)
	agent, err := core.NewAgent(logger, *cfg)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		logger.Info().Str("signal", sig.String()).Msg("Received signal, shutting down gracefully")
		agent.Stop()
		cancel()
	}()

	// Register the agent
	logger.Info().Msg("Registering Openlane compliance agent")
	if err := agent.Register(ctx); err != nil {
		return fmt.Errorf("agent registration failed: %w", err)
	}

	// Start the agent
	logger.Info().Int("workers", cfg.Spawn).Msg("Agent starting with workers")
	if err := agent.Start(ctx); err != nil {
		return fmt.Errorf("agent failed to start: %w", err)
	}

	logger.Info().Msg("Agent stopped")
	return nil
}

// loadAndValidateConfig loads and validates the configuration
func loadAndValidateConfig(configPath string, c *cli.Context) (*config.Config, error) {
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found: %s\nRun 'openlane-agent config init' to create one", configPath)
	}

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}

	// Override log level from CLI if provided
	if logLevel := c.String("log-level"); logLevel != "" {
		cfg.LogLevel = logLevel
	}

	// Final validation
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// StopAction stops a running agent
func StopAction(c *cli.Context) error {
	pidFile := c.String("pid-file")
	if pidFile == "" {
		pidFile = "agent.pid"
	}

	fmt.Println("Stopping agent...")

	// Try to read the PID file
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		fmt.Printf("PID file %s not found. Agent may not be running or was started in foreground mode.\n", pidFile)
		fmt.Println("Note: Use Ctrl+C to stop a foreground agent")
		return nil
	}

	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("failed to read PID file %s: %w", pidFile, err)
	}

	pidStr := strings.TrimSpace(string(pidBytes))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("invalid PID in file %s: %s", pidFile, pidStr)
	}

	// Find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process with PID %d: %w", pid, err)
	}

	// Send SIGTERM for graceful shutdown
	fmt.Printf("Sending SIGTERM to process %d...\n", pid)
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM to process %d: %w", pid, err)
	}

	// Wait a bit and check if process is still running
	time.Sleep(2 * time.Second)
	if err := process.Signal(syscall.Signal(0)); err != nil {
		// Process is not running anymore
		fmt.Println("Agent stopped successfully")
		os.Remove(pidFile)
		return nil
	}

	// Process is still running, try SIGKILL
	fmt.Printf("Process still running, sending SIGKILL to process %d...\n", pid)
	if err := process.Kill(); err != nil {
		return fmt.Errorf("failed to kill process %d: %w", pid, err)
	}

	fmt.Println("Agent forcefully stopped")
	os.Remove(pidFile)
	return nil
}

// StatusAction shows agent status
func StatusAction(c *cli.Context) error {
	pidFile := c.String("pid-file")
	if pidFile == "" {
		pidFile = "agent.pid"
	}

	fmt.Println("Agent Status:")

	// Check if PID file exists
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		fmt.Printf("Status: Not running (no PID file at %s)\n", pidFile)
		return nil
	}

	// Read PID file
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Printf("Status: Unknown (failed to read PID file: %v)\n", err)
		return nil
	}

	pidStr := strings.TrimSpace(string(pidBytes))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		fmt.Printf("Status: Unknown (invalid PID in file: %s)\n", pidStr)
		return nil
	}

	// Check if process is running
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("Status: Not running (failed to find process %d)\n", pid)
		os.Remove(pidFile) // Clean up stale PID file
		return nil
	}

	// Send signal 0 to check if process exists and is accessible
	if err := process.Signal(syscall.Signal(0)); err != nil {
		fmt.Printf("Status: Not running (process %d not accessible: %v)\n", pid, err)
		os.Remove(pidFile) // Clean up stale PID file
		return nil
	}

	fmt.Printf("Status: Running (PID: %d)\n", pid)
	fmt.Printf("PID file: %s\n", pidFile)

	// Try to load configuration and show basic info
	configPath := c.String("config")
	if configPath == "" {
		configPath = "agent.yaml"
	}

	if _, err := os.Stat(configPath); err == nil {
		cfg, err := config.LoadConfig(configPath)
		if err == nil {
			fmt.Printf("Agent name: %s\n", cfg.AgentName)
			fmt.Printf("API URL: %s\n", cfg.APIURL)
			fmt.Printf("Poll interval: %s\n", cfg.PollInterval)
			fmt.Printf("Max concurrency: %d\n", cfg.MaxConcurrency)
			fmt.Printf("Number of checks: %d\n", len(cfg.Checks))
			
			// Show enabled checks
			enabledChecks := 0
			for _, check := range cfg.Checks {
				if check.Enabled {
					enabledChecks++
				}
			}
			fmt.Printf("Enabled checks: %d\n", enabledChecks)
		}
	}

	return nil
}

// CheckAction runs a single check
func CheckAction(c *cli.Context) error {
	configPath := c.String("config")
	if configPath == "" {
		configPath = "agent.yaml"
	}

	checkName := c.Args().First()
	if checkName == "" {
		return fmt.Errorf("check name is required")
	}

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Find the check
	check, err := cfg.GetCheck(checkName)
	if err != nil {
		return fmt.Errorf("check not found: %w", err)
	}

	// Setup logger
	logLevel, _ := zerolog.ParseLevel(cfg.LogLevel)
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).Level(logLevel).With().Timestamp().Logger()

	// Create executor and run the check
	fmt.Printf("Running check: %s\n", check.Name)
	fmt.Printf("Command: %s %v\n", check.Command, check.Args)
	fmt.Printf("Schedule: %s\n", check.Schedule)
	fmt.Println("---")

	// Execute the check directly using ComplianceCheckController
	controller := core.NewComplianceCheckController(logger, nil, "cli-execution")
	
	// Convert config.Check to api.RemoteCheck for execution
	remoteCheck := &api.RemoteCheck{
		Name:        check.Name,
		Description: check.Description,
		Command:     check.Command,
		Args:        check.Args,
		WorkDir:     check.WorkDir,
		Env:         check.Env,
		Timeout:         check.Timeout.String(),
		Controls:        check.Controls,
		Tags:            check.Tags,
		Enabled:         true,
		ContinueOnError: check.ContinueOnError,
	}
	
	// Set the current execution context for logging
	controller.SetCurrentCheck(remoteCheck, "")
	
	// Execute the check
	ctx := context.Background()
	result, err := controller.ExecuteCheck(ctx, remoteCheck)
	if err != nil {
		fmt.Printf("Check execution failed: %v\n", err)
		return err
	}
	
	// Display results
	fmt.Printf("\n=== Check Results ===\n")
	fmt.Printf("Check: %s\n", result.CheckName)
	fmt.Printf("Exit Code: %d\n", result.ExitCode)
	fmt.Printf("Duration: %s\n", result.Duration)
	fmt.Printf("Findings: %d\n", len(result.Findings))
	
	if result.Error != "" {
		fmt.Printf("Error: %s\n", result.Error)
	}
	
	// Display findings
	if len(result.Findings) > 0 {
		fmt.Printf("\n=== Findings ===\n")
		for i, finding := range result.Findings {
			fmt.Printf("%d. %s\n", i+1, finding.Title)
			fmt.Printf("   Resource: %s\n", finding.Resource)
			fmt.Printf("   Severity: %s\n", finding.Severity)
			fmt.Printf("   Status: %s\n", finding.Status)
			if finding.Description != "" {
				fmt.Printf("   Description: %s\n", finding.Description)
			}
			fmt.Println()
		}
	}
	
	return nil
}

// CheckFlags are the flags for the check command
var CheckFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "config",
		Value: "agent.yaml",
		Usage: "Path to the agent configuration file",
	},
	cli.BoolFlag{
		Name:  "verbose",
		Usage: "Show verbose output",
	},
}

// daemonize forks the process and writes PID file for background operation
func daemonize(logger zerolog.Logger, pidFile string) error {
	// Check if PID file already exists
	if _, err := os.Stat(pidFile); err == nil {
		// Read existing PID and check if process is running
		pidBytes, readErr := os.ReadFile(pidFile)
		if readErr == nil {
			pidStr := strings.TrimSpace(string(pidBytes))
			if pid, parseErr := strconv.Atoi(pidStr); parseErr == nil {
				if process, findErr := os.FindProcess(pid); findErr == nil {
					if process.Signal(syscall.Signal(0)) == nil {
						return fmt.Errorf("agent already running with PID %d (PID file: %s)", pid, pidFile)
					}
				}
			}
		}
		// Remove stale PID file
		os.Remove(pidFile)
	}

	// Fork process
	logger.Info().Str("pid_file", pidFile).Msg("Starting in daemon mode")
	
	// Create new session and detach from terminal
	// Note: In a full implementation, this would use proper Unix daemon techniques
	// For now, we'll just write the PID and continue
	
	// Write PID file
	pid := os.Getpid()
	pidContent := fmt.Sprintf("%d\n", pid)
	
	if err := os.WriteFile(pidFile, []byte(pidContent), 0644); err != nil {
		return fmt.Errorf("failed to write PID file %s: %w", pidFile, err)
	}
	
	logger.Info().Int("pid", pid).Str("pid_file", pidFile).Msg("Daemon started")
	
	// Setup cleanup on exit
	go func() {
		defer os.Remove(pidFile)
		
		// Wait for program termination signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		<-sigChan
		
		logger.Info().Str("pid_file", pidFile).Msg("Cleaning up PID file")
	}()
	
	return nil
}