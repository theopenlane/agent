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

	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/core"
	"github.com/theopenlane/agent/internal/constants"
	agentlogger "github.com/theopenlane/agent/internal/logger"
	"github.com/theopenlane/agent/internal/storage"
	cli "github.com/urfave/cli/v3"
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
	&cli.StringFlag{
		Name:  "pid-file",
		Value: "agent.pid",
		Usage: "Path to the PID file",
	},
}

// StatusFlags are the flags for the status command
var StatusFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "pid-file",
		Value: "agent.pid",
		Usage: "Path to the PID file",
	},
	&cli.StringFlag{
		Name:  "config",
		Value: "agent.yaml",
		Usage: "Path to the agent configuration file",
	},
}

// StartFlags are the flags for the start command
var StartFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "config",
		Value: "agent.yaml",
		Usage: "Path to the agent configuration file",
		// No EnvVars field in urfave/cli/v3
	},
	&cli.StringFlag{
		Name:  "log-level",
		Value: "info",
		Usage: "Set the log level (debug, info, warn, error)",
		// No EnvVars field in urfave/cli/v3
	},
	&cli.StringFlag{
		Name:  "data-dir",
		Value: "./data",
		Usage: "Directory for agent data and state",
		// No EnvVars field in urfave/cli/v3
	},
	&cli.StringFlag{
		Name:  "api-key",
		Usage: "Openlane API key (overrides config file)",
		// No EnvVars field in urfave/cli/v3
	},
	&cli.StringFlag{
		Name:  "api-url",
		Value: "https://api.theopenlane.io",
		Usage: "Openlane API URL (overrides config file)",
		// No EnvVars field in urfave/cli/v3
	},
	&cli.BoolFlag{
		Name:  "no-daemon",
		Usage: "Run in foreground instead of daemonizing",
	},
	&cli.StringFlag{
		Name:  "pid-file",
		Value: "agent.pid",
		Usage: "Path to the PID file (daemon mode only)",
	},
	&cli.IntFlag{
		Name:  "max-concurrency",
		Value: 3,
		Usage: "Maximum number of concurrent checks",
	},
	&cli.BoolFlag{
		Name:  "dry-run",
		Usage: "Validate configuration and exit without starting",
	},
}

// StartAction starts the agent
func StartAction(ctx context.Context, cmd *cli.Command) error {
	configPath := cmd.String("config")
	logLevel := cmd.String("log-level")
	dataDir := cmd.String("data-dir")
	apiKey := cmd.String("api-key")
	apiURL := cmd.String("api-url")
	noDaemon := cmd.Bool("no-daemon")
	pidFile := cmd.String("pid-file")
	maxConcurrency := cmd.Int("max-concurrency")
	dryRun := cmd.Bool("dry-run")

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToLoadConfig, err)
	}
	if logLevel != "" {
		cfg.LogLevel = logLevel
	}
	if apiKey != "" {
		cfg.RegistrationToken = apiKey
	}
	if apiURL != "" {
		cfg.APIURL = apiURL
	}
	if dataDir != "" {
		cfg.DataDir = dataDir
	}
	if maxConcurrency > 0 {
		cfg.MaxConcurrency = maxConcurrency
	}
	agentlogger.Initialize(cfg.LogLevel)
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCreateDataDir, err)
	}
	log.Info().Str("version", constants.FullVersion()).Msg("Starting Openlane Agent")
	log.Info().Str("config_path", configPath).Msg("Configuration loaded")
	log.Info().Str("data_dir", cfg.DataDir).Msg("Data directory set")
	log.Info().Str("api_url", cfg.APIURL).Msg("API endpoint configured")
	log.Info().Int("count", len(cfg.GetEnabledChecks())).Msg("Enabled checks loaded")
	if dryRun {
		log.Info().Msg("Dry run mode - configuration is valid, exiting")
		return nil
	}
	if !noDaemon {
		if pidFile == "" {
			pidFile = "agent.pid"
		}
		if err := daemonize(pidFile); err != nil {
			return fmt.Errorf("failed to daemonize: %w", err)
		}
	}
	agent, err := core.NewAgent(core.WithConfig(cfg))
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}
	ctxCancel, cancel := context.WithCancel(context.Background())
	defer cancel()
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Info().Str("signal", sig.String()).Msg("Received signal, shutting down gracefully")
		agent.Stop()
		cancel()
	}()
	log.Info().Msg("Registering Openlane compliance agent")
	if err := agent.Register(ctxCancel); err != nil {
		return fmt.Errorf("agent registration failed: %w", err)
	}
	log.Info().Int("workers", cfg.Spawn).Msg("Agent starting with workers")
	if err := agent.Start(ctxCancel); err != nil {
		return fmt.Errorf("agent failed to start: %w", err)
	}
	log.Info().Msg("Agent stopped")
	return nil
}

// loadAndValidateConfig loads and validates the configuration
// Remove this function or refactor to not use cli.Context

// StopAction stops a running agent (urfave/cli v3 signature)
func StopAction(ctx context.Context, cmd *cli.Command) error {
	pidFile := cmd.String("pid-file")
	if pidFile == "" {
		pidFile = "agent.pid"
	}
	fmt.Println("Stopping agent...")
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		fmt.Printf("PID file %s not found. Agent may not be running or was started in foreground mode.\n", pidFile)
		fmt.Println("Note: Use Ctrl+C to stop a foreground agent")
		return nil
	}
	pidBytes, err := os.ReadFile(pidFile)
	if err != nil {
		return fmt.Errorf("%w %s: %w", ErrFailedToReadPIDFile, pidFile, err)
	}
	pidStr := strings.TrimSpace(string(pidBytes))
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return fmt.Errorf("%w: %s in file %s", ErrInvalidPID, pidStr, pidFile)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("%w with PID %d: %w", ErrFailedToFindProcess, pid, err)
	}
	fmt.Printf("Sending SIGTERM to process %d...\n", pid)
	if err := process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("%w to process %d: %w", ErrFailedToSendSIGTERM, pid, err)
	}
	time.Sleep(2 * time.Second)
	if err := process.Signal(syscall.Signal(0)); err != nil {
		fmt.Println("Agent stopped successfully")
		os.Remove(pidFile)
		return nil
	}
	fmt.Printf("Process still running, sending SIGKILL to process %d...\n", pid)
	if err := process.Kill(); err != nil {
		return fmt.Errorf("%w %d: %w", ErrFailedToKillProcess, pid, err)
	}
	fmt.Println("Agent forcefully stopped")
	os.Remove(pidFile)
	return nil
}

// StatusAction shows agent status (urfave/cli v3 signature)
func StatusAction(ctx context.Context, cmd *cli.Command) error {
	pidFile := cmd.String("pid-file")
	if pidFile == "" {
		pidFile = "agent.pid"
	}
	fmt.Println("Agent Status:")
	if _, err := os.Stat(pidFile); os.IsNotExist(err) {
		fmt.Printf("Status: Not running (no PID file at %s)\n", pidFile)
		return nil
	}
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
	process, err := os.FindProcess(pid)
	if err != nil {
		fmt.Printf("Status: Not running (failed to find process %d)\n", pid)
		os.Remove(pidFile)
		return nil
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		fmt.Printf("Status: Not running (process %d not accessible: %v)\n", pid, err)
		os.Remove(pidFile)
		return nil
	}
	fmt.Printf("Status: Running (PID: %d)\n", pid)
	fmt.Printf("PID file: %s\n", pidFile)
	configPath := cmd.String("config")
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

// CheckAction runs a single check (urfave/cli v3 signature)
func CheckAction(ctx context.Context, cmd *cli.Command) error {
	configPath := cmd.String("config")
	if configPath == "" {
		configPath = "agent.yaml"
	}
	args := cmd.Args().Slice()
	if len(args) == 0 {
		return ErrCheckNotFound
	}
	checkName := args[0]
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToLoadConfig, err)
	}
	check, err := cfg.GetCheck(checkName)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrCheckNotFound, err)
	}
	fmt.Printf("Running check: %s\n", check.Name)
	fmt.Printf("Command: %s %v\n", check.Command, check.Args)
	fmt.Printf("Schedule: %s\n", check.Schedule)
	fmt.Println("---")
	storageSystem, err := storage.NewStorageFromConfig(cfg)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create storage, results may not be saved")
		storageSystem = nil
	}
	var evidenceService *storage.EvidenceService
	if storageSystem != nil {
		storageConfig := storage.ConfigFromAgentConfig(cfg)
		evidenceService = storage.NewEvidenceService(storageConfig)
	}
	controller := core.NewComplianceCheckController(nil, "cli-execution", storageSystem, evidenceService)
	remoteCheck := &api.RemoteCheck{
		Name:            check.Name,
		Description:     check.Description,
		Command:         check.Command,
		Args:            check.Args,
		WorkDir:         check.WorkDir,
		Env:             check.Env,
		Timeout:         check.Timeout.String(),
		Controls:        check.Controls,
		Tags:            check.Tags,
		Enabled:         true,
		ContinueOnError: check.ContinueOnError,
	}
	controller.SetCurrentCheck(remoteCheck, "")
	ctxExec := context.Background()
	result, err := controller.ExecuteCheck(ctxExec, remoteCheck)
	if err != nil {
		fmt.Printf("Check execution failed: %v\n", err)
		return err
	}
	fmt.Printf("\n=== Check Results ===\n")
	fmt.Printf("Check: %s\n", result.CheckName)
	fmt.Printf("Exit Code: %d\n", result.ExitCode)
	fmt.Printf("Duration: %s\n", result.Duration)
	fmt.Printf("Findings: %d\n", len(result.Findings))
	if result.Error != "" {
		fmt.Printf("Error: %s\n", result.Error)
	}
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
	&cli.StringFlag{
		Name:  "config",
		Value: "agent.yaml",
		Usage: "Path to the agent configuration file",
	},
	&cli.BoolFlag{
		Name:  "verbose",
		Usage: "Show verbose output",
	},
}

// daemonize forks the process and writes PID file for background operation
func daemonize(pidFile string) error {
	// Check if PID file already exists
	if _, err := os.Stat(pidFile); err == nil {
		// Read existing PID and check if process is running
		pidBytes, readErr := os.ReadFile(pidFile)
		if readErr == nil {
			pidStr := strings.TrimSpace(string(pidBytes))
			if pid, parseErr := strconv.Atoi(pidStr); parseErr == nil {
				if process, findErr := os.FindProcess(pid); findErr == nil {
					if process.Signal(syscall.Signal(0)) == nil {
						return fmt.Errorf("%w with PID %d (PID file: %s)", ErrAgentAlreadyRunning, pid, pidFile)
					}
				}
			}
		}
		// Remove stale PID file
		os.Remove(pidFile)
	}

	// Fork process
	log.Info().Str("pid_file", pidFile).Msg("Starting in daemon mode")

	// Create new session and detach from terminal
	// Note: In a full implementation, this would use proper Unix daemon techniques
	// For now, we'll just write the PID and continue

	// Write PID file
	pid := os.Getpid()
	pidContent := fmt.Sprintf("%d\n", pid)

	if err := os.WriteFile(pidFile, []byte(pidContent), 0o644); err != nil { // nolint:mnd
		return fmt.Errorf("%w %s: %w", ErrFailedToWritePIDFile, pidFile, err)
	}

	log.Info().Int("pid", pid).Str("pid_file", pidFile).Msg("Daemon started")

	// Setup cleanup on exit
	go func() {
		defer os.Remove(pidFile)

		// Wait for program termination signals
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		<-sigChan

		log.Info().Str("pid_file", pidFile).Msg("Cleaning up PID file")
	}()

	return nil
}
