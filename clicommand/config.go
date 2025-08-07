package clicommand

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/theopenlane/agent/internal/config"
	"github.com/theopenlane/agent/version"
	"github.com/urfave/cli"
	"gopkg.in/yaml.v3"
)

// ConfigInitFlags are the flags for the config init command
var ConfigInitFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "output",
		Value: "agent.yaml",
		Usage: "Output file path for the configuration",
	},
	cli.BoolFlag{
		Name:  "force",
		Usage: "Overwrite existing configuration file",
	},
	cli.StringFlag{
		Name:  "api-key",
		Usage: "Openlane API key",
		EnvVar: "OPENLANE_API_KEY",
	},
	cli.StringFlag{
		Name:  "api-url",
		Value: "https://api.openlane.io",
		Usage: "Openlane API URL",
		EnvVar: "OPENLANE_API_URL",
	},
	cli.StringFlag{
		Name:  "agent-name",
		Usage: "Name for this agent",
	},
}

// ConfigInitAction initializes a new agent configuration
func ConfigInitAction(c *cli.Context) error {
	outputPath := c.String("output")
	force := c.Bool("force")
	
	// Check if file exists and not forcing
	if _, err := os.Stat(outputPath); err == nil && !force {
		return fmt.Errorf("configuration file %s already exists (use --force to overwrite)", outputPath)
	}
	
	// Create example configuration
	cfg := config.ExampleConfig()
	
	// Override with CLI flags if provided
	if apiKey := c.String("api-key"); apiKey != "" {
		cfg.RegistrationToken = apiKey
	}
	if apiURL := c.String("api-url"); apiURL != "" {
		cfg.APIURL = apiURL
	}
	if agentName := c.String("agent-name"); agentName != "" {
		cfg.AgentName = agentName
	}
	
	// Create directory if it doesn't exist
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	
	// Save configuration
	if err := cfg.SaveConfig(outputPath); err != nil {
		return fmt.Errorf("failed to save configuration: %w", err)
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
	cli.StringFlag{
		Name:  "config",
		Value: "agent.yaml",
		Usage: "Path to the configuration file to validate",
	},
	cli.BoolFlag{
		Name:  "verbose",
		Usage: "Show detailed validation information",
	},
}

// ConfigValidateAction validates an agent configuration
func ConfigValidateAction(c *cli.Context) error {
	configPath := c.String("config")
	verbose := c.Bool("verbose")
	
	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("configuration file not found: %s", configPath)
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
	cli.StringFlag{
		Name:  "config",
		Value: "agent.yaml",
		Usage: "Path to the configuration file to show",
	},
	cli.StringFlag{
		Name:  "format",
		Value: "yaml",
		Usage: "Output format (yaml or json)",
	},
	cli.BoolFlag{
		Name:  "redact",
		Usage: "Redact sensitive information (API keys, secrets)",
	},
}

// ConfigShowAction displays the current configuration
func ConfigShowAction(c *cli.Context) error {
	configPath := c.String("config")
	format := c.String("format")
	redact := c.Bool("redact")
	
	// Load configuration
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	
	// Redact sensitive information if requested
	if redact {
		cfg.RegistrationToken = "[REDACTED]"
		for i := range cfg.Checks {
			for j, env := range cfg.Checks[i].Env {
				if containsSensitiveKey(env) {
					parts := splitEnvVar(env)
					if len(parts) == 2 {
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
			return fmt.Errorf("failed to marshal configuration: %w", err)
		}
		fmt.Print(string(data))
		
	case "json":
		encoder := yaml.NewEncoder(os.Stdout)
		encoder.SetIndent(2)
		if err := encoder.Encode(cfg); err != nil {
			return fmt.Errorf("failed to encode configuration: %w", err)
		}
		
	default:
		return fmt.Errorf("unsupported format: %s (supported: yaml, json)", format)
	}
	
	return nil
}

// VersionAction shows version information
func VersionAction(c *cli.Context) error {
	fmt.Printf("openlane-agent version %s\n", version.FullVersion())
	fmt.Printf("User-Agent: %s\n", version.UserAgent())
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
	parts := strings.SplitN(envVar, "=", 2)
	if len(parts) == 2 {
		return parts
	}
	return []string{envVar}
}