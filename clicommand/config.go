package clicommand

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/constants"
	cli "github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

const (
	// Environment variable parsing constants
	keyValueParts = 2

	// YAML formatting constants
	yamlIndentSize = 2
)

// ConfigInitFlags are the flags for the config init command
var ConfigInitFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "output",
		Value: "agent.yaml",
		Usage: "Output file path for the configuration",
	},
	&cli.BoolFlag{
		Name:  "force",
		Usage: "Overwrite existing configuration file",
	},
	&cli.StringFlag{
		Name:  "api-key",
		Usage: "Openlane API key",
		// No EnvVar in v3
	},
	&cli.StringFlag{
		Name:  "api-url",
		Value: "https://api.theopenlane.io",
		Usage: "Openlane API URL",
		// No EnvVar in v3
	},
	&cli.StringFlag{
		Name:  "agent-name",
		Usage: "Name for this agent",
	},
}

// ConfigInitAction initializes a new agent configuration (urfave/cli v3 signature)
func ConfigInitAction(_ context.Context, cmd *cli.Command) error {
	outputPath := cmd.String("output")
	force := cmd.Bool("force")

	// Check if file exists and not forcing
	if _, err := os.Stat(outputPath); err == nil && !force {
		return fmt.Errorf("%w: %s (use --force to overwrite)", ErrConfigFileExists, outputPath)
	}

	// Create example configuration
	cfg := config.ExampleConfig()

	// Override with CLI flags if provided
	if apiKey := cmd.String("api-key"); apiKey != "" {
		cfg.RegistrationToken = apiKey
	}

	if apiURL := cmd.String("api-url"); apiURL != "" {
		cfg.APIURL = apiURL
	}

	if agentName := cmd.String("agent-name"); agentName != "" {
		cfg.AgentName = agentName
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, config.DefaultDirectoryPermissions); err != nil {
		return fmt.Errorf("%w %s: %w", ErrFailedToCreateDirectory, dir, err)
	}

	// Save configuration
	if err := cfg.SaveConfig(outputPath); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToSaveConfig, err)
	}

	fmt.Printf("Configuration initialized: %s\n", outputPath)
	fmt.Println("")
	fmt.Println("Next steps:")
	fmt.Println("1. Edit the configuration file to add your API key and customize checks")
	fmt.Println("2. Create your compliance check scripts in the ./scripts directory")
	fmt.Printf("3. Start the agent with: openlane-agent start --config %s\n", outputPath)

	return nil
}

// ConfigValidateFlags are the flags for the config validate command
var ConfigValidateFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "config",
		Value: "agent.yaml",
		Usage: "Path to the configuration file to validate",
	},
	&cli.BoolFlag{
		Name:  "verbose",
		Usage: "Show detailed validation information",
	},
}

// ConfigValidateAction validates an agent configuration
func ConfigValidateAction(_ context.Context, cmd *cli.Command) error {
	configPath := cmd.String("config")
	verbose := cmd.Bool("verbose")

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("%w: %s", ErrConfigFileNotFound, configPath)
	}

	// Load and validate configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	fmt.Printf("✓ Configuration is valid: %s\n", configPath)

	if verbose {
		fmt.Println("")
		fmt.Printf("Agent Name: %s\n", cfg.AgentName)
		fmt.Printf("API URL: %s\n", cfg.APIURL)
		fmt.Printf("Log Level: %s\n", cfg.LogLevel)
		fmt.Printf("Data Directory: %s\n", cfg.DataDir)
		fmt.Printf("Poll Interval: %s\n", cfg.PollInterval)
		fmt.Printf("Max Concurrency: %d\n", cfg.MaxConcurrency)
		fmt.Printf("Default Timeout: %s\n", cfg.DefaultTimeout)
		fmt.Println("")

		enabledChecks := cfg.GetEnabledChecks()
		fmt.Printf("Enabled Checks: %d\n", len(enabledChecks))

		for _, check := range enabledChecks {
			fmt.Printf("  - %s: %s (schedule: %s)\n", check.Name, check.Description, check.Schedule)
		}

		if len(cfg.Checks) > len(enabledChecks) {
			fmt.Printf("Disabled Checks: %d\n", len(cfg.Checks)-len(enabledChecks))
		}
	}

	return nil
}

// ConfigShowFlags are the flags for the config show command
var ConfigShowFlags = []cli.Flag{
	&cli.StringFlag{
		Name:  "config",
		Value: "agent.yaml",
		Usage: "Path to the configuration file to show",
	},
	&cli.StringFlag{
		Name:  "format",
		Value: "yaml",
		Usage: "Output format (yaml or json)",
	},
	&cli.BoolFlag{
		Name:  "redact",
		Usage: "Redact sensitive information (API keys, secrets)",
	},
}

// ConfigShowAction displays the current configuration
func ConfigShowAction(_ context.Context, cmd *cli.Command) error {
	configPath := cmd.String("config")
	format := cmd.String("format")
	redact := cmd.Bool("redact")

	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToLoadConfig, err)
	}

	// Redact sensitive information if requested
	if redact {
		cfg.RegistrationToken = "[REDACTED]"
		for i := range cfg.Checks {
			for j, env := range cfg.Checks[i].Env {
				if containsSensitiveKey(env) {
					parts := splitEnvVar(env)
					if len(parts) == keyValueParts {
						cfg.Checks[i].Env[j] = parts[0] + "=[REDACTED]"
					}
				}
			}
		}
	}

	// Output in requested format
	switch format {
	case "yaml", "yml":
		data, err := yaml.Marshal(cfg)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrFailedToMarshalConfig, err)
		}

		fmt.Print(string(data))

	case "json":
		data, err := json.MarshalIndent(cfg, "", strings.Repeat(" ", yamlIndentSize))
		if err != nil {
			return fmt.Errorf("%w: %w", ErrFailedToEncodeConfig, err)
		}

		fmt.Println(string(data))

	default:
		return fmt.Errorf("%w: %s (supported: yaml, json)", ErrUnsupportedFormat, format)
	}

	return nil
}

// VersionAction shows version information
func VersionAction(_ context.Context, _ *cli.Command) error {
	fmt.Printf("openlane-agent version %s\n", constants.FullVersion())
	fmt.Printf("User-Agent: %s\n", constants.UserAgent())

	return nil
}

// Helper functions

func containsSensitiveKey(envVar string) bool {
	sensitiveKeys := []string{
		"API_KEY", "SECRET", "TOKEN", "PASSWORD", "PRIVATE_KEY",
		"AWS_SECRET_ACCESS_KEY", "GITHUB_TOKEN", "SLACK_TOKEN",
	}

	upperEnv := strings.ToUpper(envVar)
	for _, key := range sensitiveKeys {
		if strings.Contains(upperEnv, key) {
			return true
		}
	}

	return false
}

func splitEnvVar(envVar string) []string {
	parts := strings.SplitN(envVar, "=", keyValueParts)
	if len(parts) == keyValueParts {
		return parts
	}

	return []string{envVar}
}
