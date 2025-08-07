package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the agent configuration loaded from agent.yaml
type Config struct {
	// API connection settings (following Buildkite pattern)
	RegistrationToken string `yaml:"registration_token" json:"registration_token"`
	APIURL            string `yaml:"api_url" json:"api_url"`

	// Agent identification
	AgentID   string `yaml:"agent_id,omitempty" json:"agent_id,omitempty"`
	AgentName string `yaml:"agent_name,omitempty" json:"agent_name,omitempty"`

	// Global settings
	LogLevel     string        `yaml:"log_level" json:"log_level"`
	DataDir      string        `yaml:"data_dir" json:"data_dir"`
	PollInterval time.Duration `yaml:"poll_interval" json:"poll_interval"`

	// Execution settings (following Buildkite pattern)
	Spawn          int           `yaml:"spawn" json:"spawn"`
	MaxConcurrency int           `yaml:"max_concurrency" json:"max_concurrency"`
	DefaultTimeout time.Duration `yaml:"default_timeout" json:"default_timeout"`

	// Compliance checks to run (will be mostly managed remotely)
	Checks []Check `yaml:"checks" json:"checks"`
}

// Check represents a single compliance check configuration
type Check struct {
	// Basic info
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`

	// Execution details
	Command string   `yaml:"command" json:"command"`
	Args    []string `yaml:"args,omitempty" json:"args,omitempty"`
	WorkDir string   `yaml:"work_dir,omitempty" json:"work_dir,omitempty"`

	// Environment variables
	Env []string `yaml:"env,omitempty" json:"env,omitempty"`

	// Scheduling
	Schedule string        `yaml:"schedule" json:"schedule"`
	Timeout  time.Duration `yaml:"timeout" json:"timeout"`

	// Compliance context
	Controls []string `yaml:"controls,omitempty" json:"controls,omitempty"`
	Tags     []string `yaml:"tags,omitempty" json:"tags,omitempty"`

	// Execution options
	Enabled         bool `yaml:"enabled" json:"enabled"`
	ContinueOnError bool `yaml:"continue_on_error" json:"continue_on_error"`

	// Last execution tracking (not persisted)
	LastRun    time.Time `yaml:"-" json:"last_run,omitempty"`
	LastResult *Result   `yaml:"-" json:"last_result,omitempty"`
}

// Result represents the output from a compliance check
type Result struct {
	// Execution metadata
	CheckName      string    `json:"check_name"`
	ScheduledJobID string    `json:"scheduled_job_id"`
	ExecutedAt     time.Time `json:"executed_at"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	Duration       string    `json:"duration"`
	ExitCode       int       `json:"exit_code"`

	// Compliance findings
	Findings []Finding `json:"findings"`

	// Supporting evidence
	Evidence map[string]any `json:"evidence,omitempty"`

	// Metrics and counts
	Metrics map[string]any `json:"metrics,omitempty"`

	// Error information
	Error  string `json:"error,omitempty"`
	Stderr string `json:"stderr,omitempty"`

	// Compliance context
	Controls []string `json:"controls,omitempty"`
	Tags     []string `json:"tags,omitempty"`

	// File reference for result storage
	ResultFileID string `json:"result_file_id,omitempty"`
}

// Finding represents a single compliance finding
type Finding struct {
	// Resource identification
	Resource     string `json:"resource"`
	ResourceType string `json:"resource_type,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`

	// Finding details
	Title       string          `json:"title"`
	Description string          `json:"description,omitempty"`
	Severity    FindingSeverity `json:"severity"`
	Status      FindingStatus   `json:"status"`

	// Detailed information
	Details map[string]any `json:"details,omitempty"`

	// Remediation guidance
	Remediation string   `json:"remediation,omitempty"`
	References  []string `json:"references,omitempty"`

	// Control mappings
	Controls []ControlMapping `json:"control_mappings,omitempty"`
}

// FindingSeverity represents the severity level of a finding
type FindingSeverity string

const (
	SeverityCritical FindingSeverity = "critical"
	SeverityHigh     FindingSeverity = "high"
	SeverityMedium   FindingSeverity = "medium"
	SeverityLow      FindingSeverity = "low"
	SeverityInfo     FindingSeverity = "info"
)

// FindingStatus represents the current status of a finding
type FindingStatus string

const (
	StatusOpen          FindingStatus = "open"
	StatusAcknowledged  FindingStatus = "acknowledged"
	StatusRemediated    FindingStatus = "remediated"
	StatusFalsePositive FindingStatus = "false_positive"
	StatusAccepted      FindingStatus = "accepted"
)

// ControlMapping maps a finding to compliance controls
type ControlMapping struct {
	Framework string `json:"framework"`
	ControlID string `json:"control_id"`
	Satisfied bool   `json:"satisfied"`
	Notes     string `json:"notes,omitempty"`
}

// DefaultConfig returns a default agent configuration
func DefaultConfig() *Config {
	return &Config{
		APIURL:         "https://api.openlane.io",
		LogLevel:       "info",
		DataDir:        "./data",
		PollInterval:   1 * time.Minute,
		MaxConcurrency: 3,
		DefaultTimeout: 5 * time.Minute,
		Checks:         []Check{},
	}
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	// Start with defaults
	config := DefaultConfig()

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", path, err)
	}

	// Expand environment variables
	expanded := os.ExpandEnv(string(data))

	// Parse YAML
	if err := yaml.Unmarshal([]byte(expanded), config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", path, err)
	}

	// Validate and set defaults
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// API configuration
	if c.APIURL == "" {
		return fmt.Errorf("api_url is required")
	}

	if c.RegistrationToken == "" {
		return fmt.Errorf("registration_token is required")
	}

	// Set defaults
	if c.LogLevel == "" {
		c.LogLevel = "info"
	}

	if c.DataDir == "" {
		c.DataDir = "./data"
	}

	if c.PollInterval == 0 {
		c.PollInterval = 1 * time.Minute
	}

	if c.MaxConcurrency == 0 {
		c.MaxConcurrency = 3
	}

	if c.DefaultTimeout == 0 {
		c.DefaultTimeout = 5 * time.Minute
	}

	// Validate checks
	for i := range c.Checks {
		if err := c.Checks[i].Validate(); err != nil {
			return fmt.Errorf("check %d (%s): %w", i, c.Checks[i].Name, err)
		}

		// Set defaults for checks
		c.Checks[i].SetDefaults(c.DefaultTimeout)
	}

	return nil
}

// Validate validates a check configuration
func (ch *Check) Validate() error {
	if ch.Name == "" {
		return fmt.Errorf("check name is required")
	}

	if ch.Command == "" {
		return fmt.Errorf("check command is required")
	}

	if ch.Schedule == "" {
		return fmt.Errorf("check schedule is required")
	}

	// Validate schedule format (basic validation)
	if !isValidCronExpression(ch.Schedule) {
		return fmt.Errorf("invalid cron schedule: %s", ch.Schedule)
	}

	return nil
}

// SetDefaults sets default values for a check
func (ch *Check) SetDefaults(defaultTimeout time.Duration) {
	if ch.Timeout == 0 {
		ch.Timeout = defaultTimeout
	}

	if ch.WorkDir == "" {
		ch.WorkDir = "."
	}

	// Default to enabled if not explicitly set
	if !ch.hasExplicitEnabledSetting() {
		ch.Enabled = true
	}
}

// hasExplicitEnabledSetting checks if enabled was explicitly set
func (ch *Check) hasExplicitEnabledSetting() bool {
	// This is a simplified approach - in a real implementation you might
	// want to use a pointer or custom YAML unmarshaling
	return !ch.Enabled
}

// SaveConfig saves the configuration to a YAML file
func (c *Config) SaveConfig(path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// GetCheck returns a check by name
func (c *Config) GetCheck(name string) (*Check, error) {
	for i := range c.Checks {
		if c.Checks[i].Name == name {
			return &c.Checks[i], nil
		}
	}
	return nil, fmt.Errorf("check %s not found", name)
}

// GetEnabledChecks returns only enabled checks
func (c *Config) GetEnabledChecks() []Check {
	var enabled []Check
	for _, check := range c.Checks {
		if check.Enabled {
			enabled = append(enabled, check)
		}
	}
	return enabled
}

// isValidCronExpression performs basic validation of cron expressions
func isValidCronExpression(expr string) bool {
	// Basic validation - proper cron parsing would use a library like robfig/cron
	parts := strings.Fields(expr)

	// Standard cron: minute hour day month weekday (5 parts)
	// Extended cron: second minute hour day month weekday (6 parts)
	return len(parts) == 5 || len(parts) == 6
}

// ExampleConfig returns an example configuration for documentation
func ExampleConfig() *Config {
	return &Config{
		RegistrationToken: "${OPENLANE_REGISTRATION_TOKEN}",
		APIURL:            "https://api.openlane.io",
		AgentName:         "production-compliance-agent",
		LogLevel:          "info",
		DataDir:           "./data",
		PollInterval:      1 * time.Minute,
		Spawn:             1,
		MaxConcurrency:    3,
		DefaultTimeout:    5 * time.Minute,
		Checks: []Check{
			{
				Name:        "aws-iam-compliance",
				Description: "Check AWS IAM configuration for compliance issues",
				Command:     "./scripts/check-aws-iam.sh",
				Schedule:    "0 */4 * * *", // Every 4 hours
				Timeout:     10 * time.Minute,
				Env: []string{
					"AWS_REGION=us-east-1",
				},
				Controls: []string{
					"SOC2:CC6.1",
					"ISO27001:A.9.2.1",
				},
				Tags:    []string{"aws", "iam", "critical"},
				Enabled: true,
			},
			{
				Name:        "github-audit",
				Description: "Audit GitHub organization settings",
				Command:     "python3",
				Args:        []string{"./scripts/github-audit.py", "--org", "mycompany"},
				Schedule:    "0 0 * * *", // Daily at midnight
				Timeout:     5 * time.Minute,
				Env: []string{
					"GITHUB_TOKEN=${GITHUB_TOKEN}",
				},
				Controls: []string{
					"SOC2:CC6.3",
				},
				Tags:    []string{"github", "access-control"},
				Enabled: true,
			},
			{
				Name:        "database-compliance",
				Description: "Check database security settings",
				Command:     "./scripts/check-database.rb",
				Schedule:    "*/30 * * * *", // Every 30 minutes
				Timeout:     2 * time.Minute,
				Controls: []string{
					"PCI-DSS:8.2",
				},
				Tags:    []string{"database", "security"},
				Enabled: false, // Disabled by default
			},
		},
	}
}
