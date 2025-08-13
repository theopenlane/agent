package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	"github.com/mcuadros/go-defaults"
	jsonschemavalidator "github.com/santhosh-tekuri/jsonschema/v5"
	yamlv3 "gopkg.in/yaml.v3"
)

// Config represents the agent configuration loaded from agent.yaml
type Config struct {
	// API connection settings
	RegistrationToken string `json:"registrationToken" koanf:"registrationToken" jsonschema:"required" sensitive:"true" description:"Token used to register with the Openlane platform"`
	APIURL            string `json:"apiUrl" koanf:"apiUrl" default:"https://api.theopenlane.io" description:"Base URL for the Openlane API"`

	// Agent identification
	AgentID   string `json:"agentId,omitempty" koanf:"agentId" description:"Unique identifier for this agent instance"`
	AgentName string `json:"agentName,omitempty" koanf:"agentName" description:"Human-readable name for this agent"`

	// Global settings
	LogLevel     string        `json:"logLevel" koanf:"logLevel" default:"info" description:"Log level (debug, info, warn, error)"`
	DataDir      string        `json:"dataDir" koanf:"dataDir" default:"./data" description:"Directory for storing agent data"`
	PollInterval time.Duration `json:"pollInterval" koanf:"pollInterval" default:"1m" description:"Interval for polling the platform for work"`

	// Execution settings
	Spawn          int           `json:"spawn" koanf:"spawn" default:"1" description:"Number of worker processes to spawn"`
	MaxConcurrency int           `json:"maxConcurrency" koanf:"maxConcurrency" default:"3" description:"Maximum number of concurrent check executions"`
	DefaultTimeout time.Duration `json:"defaultTimeout" koanf:"defaultTimeout" default:"5m" description:"Default timeout for check execution"`

	// Evidence settings
	Evidence EvidenceConfig `json:"evidence" koanf:"evidence" description:"Evidence collection and retention configuration"`

	// Offline buffering settings
	Offline OfflineConfig `json:"offline" koanf:"offline" description:"Offline buffering and standalone mode configuration"`

	// Compliance checks to run locally
	Checks []Check `json:"checks" koanf:"checks" description:"Local compliance checks to execute"`
}

// EvidenceConfig configures evidence collection and retention
type EvidenceConfig struct {
	Enabled         bool          `json:"enabled" koanf:"enabled" default:"true" description:"Enable evidence collection"`
	RetentionPeriod time.Duration `json:"retentionPeriod" koanf:"retentionPeriod" default:"30d" description:"How long to retain evidence files"`
	MaxFileSize     int64         `json:"maxFileSize" koanf:"maxFileSize" default:"104857600" description:"Maximum evidence file size in bytes (100MB default)"`
	CompressFiles   bool          `json:"compressFiles" koanf:"compressFiles" default:"false" description:"Compress evidence files to save space"`
}

// OperationMode defines how the agent operates
type OperationMode string

const (
	// ModeNormal - Standard operation with API connectivity
	ModeNormal OperationMode = "normal"
	// ModeStandalone - Completely independent operation, no API connectivity
	ModeStandalone OperationMode = "standalone" 
	// ModeBuffered - Normal operation with local buffering when API unavailable
	ModeBuffered OperationMode = "buffered"
)

// OfflineConfig configures offline behavior and operation modes
type OfflineConfig struct {
	Mode                  OperationMode `json:"mode" koanf:"mode" default:"normal" description:"Operation mode: normal, standalone, or buffered"`
	
	// Standalone mode settings
	OutputDir             string        `json:"outputDir" koanf:"outputDir" default:"./results" description:"Directory for storing results in standalone mode"`
	OutputFormat          string        `json:"outputFormat" koanf:"outputFormat" default:"json" description:"Output format for standalone mode: json, yaml, csv"`
	
	// Buffered mode settings
	BufferDir             string        `json:"bufferDir" koanf:"bufferDir" default:"./buffer" description:"Directory for storing buffered results when offline"`
	ConnectivityCheckURL  string        `json:"connectivityCheckUrl" koanf:"connectivityCheckUrl" description:"URL endpoint for connectivity checks (defaults to API URL + /livez)"`
	ConnectivityInterval  time.Duration `json:"connectivityInterval" koanf:"connectivityInterval" default:"30s" description:"Interval for connectivity checks"`
	SyncInterval          time.Duration `json:"syncInterval" koanf:"syncInterval" default:"5m" description:"Interval for periodic sync attempts"`
	MaxRetries            int           `json:"maxRetries" koanf:"maxRetries" default:"5" description:"Maximum retry attempts for buffered results before giving up"`
	BufferRetentionPeriod time.Duration `json:"bufferRetentionPeriod" koanf:"bufferRetentionPeriod" default:"7d" description:"How long to retain buffered results before cleanup"`
}

// Check represents a single compliance check configuration
type Check struct {
	// Basic info
	Name        string `json:"name" koanf:"name" jsonschema:"required" description:"Unique name for this check"`
	Description string `json:"description,omitempty" koanf:"description" description:"Human-readable description of what this check validates"`

	// Execution details
	Command string   `json:"command" koanf:"command" jsonschema:"required" description:"Command to execute for this check"`
	Args    []string `json:"args,omitempty" koanf:"args" description:"Arguments to pass to the command"`
	WorkDir string   `json:"workDir,omitempty" koanf:"workDir" description:"Working directory for command execution"`

	// Environment variables
	Env []string `json:"env,omitempty" koanf:"env" description:"Environment variables for command execution"`

	// Scheduling
	Schedule string        `json:"schedule" koanf:"schedule" jsonschema:"required" description:"Cron expression for when to run this check"`
	Timeout  time.Duration `json:"timeout" koanf:"timeout" default:"5m" description:"Timeout for this check execution"`

	// Compliance context
	Controls []string `json:"controls,omitempty" koanf:"controls" description:"Compliance controls this check validates"`
	Tags     []string `json:"tags,omitempty" koanf:"tags" description:"Tags for categorizing and filtering checks"`

	// Execution options
	Enabled         bool `json:"enabled" koanf:"enabled" default:"true" description:"Whether this check is enabled"`
	ContinueOnError bool `json:"continueOnError" koanf:"continueOnError" default:"false" description:"Continue executing other checks if this one fails"`

	// Evidence collection
	EvidencePaths []string `json:"evidencePaths,omitempty" koanf:"evidencePaths" description:"File paths or directories to collect as evidence"`

	// Pass/Fail behavior configuration
	OnPass *ActionConfig `json:"onPass,omitempty" koanf:"onPass" description:"Actions to execute when check passes"`
	OnFail *ActionConfig `json:"onFail,omitempty" koanf:"onFail" description:"Actions to execute when check fails"`

	// Runtime tracking (not persisted)
	LastRun    time.Time `json:"-" koanf:"-"`
	LastResult *Result   `json:"-" koanf:"-"`
}

// ActionConfig defines actions to take on pass/fail scenarios
type ActionConfig struct {
	// Upload evidence to controls
	UploadEvidence bool `json:"uploadEvidence" koanf:"uploadEvidence" default:"false" description:"Upload collected evidence to associated controls"`

	// Commands to execute
	Commands []ActionCommand `json:"commands,omitempty" koanf:"commands" description:"Commands to execute for this outcome"`

	// Control status updates
	UpdateControlStatus bool `json:"updateControlStatus" koanf:"updateControlStatus" default:"false" description:"Update control status based on check result"`
}

// ActionCommand represents a command to execute on pass/fail
type ActionCommand struct {
	Name            string        `json:"name" koanf:"name" jsonschema:"required" description:"Unique name for this action"`
	Command         string        `json:"command" koanf:"command" jsonschema:"required" description:"Command to execute"`
	Args            []string      `json:"args,omitempty" koanf:"args" description:"Arguments to pass to the command"`
	WorkDir         string        `json:"workDir,omitempty" koanf:"workDir" description:"Working directory for command execution"`
	Env             []string      `json:"env,omitempty" koanf:"env" description:"Environment variables for command execution"`
	Timeout         time.Duration `json:"timeout,omitempty" koanf:"timeout" default:"1m" description:"Timeout for command execution"`
	ContinueOnError bool          `json:"continueOnError" koanf:"continueOnError" default:"false" description:"Continue with other actions if this command fails"`
}

// Result represents the output from a compliance check
type Result struct {
	// Execution metadata
	CheckName      string    `json:"checkName"`
	ScheduledJobID string    `json:"scheduledJobId,omitempty"`
	ExecutedAt     time.Time `json:"executedAt"`
	StartTime      time.Time `json:"startTime"`
	EndTime        time.Time `json:"endTime"`
	Duration       string    `json:"duration"`
	ExitCode       int       `json:"exitCode"`

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

	// Evidence file upload results
	EvidenceFiles []*EvidenceFileResult `json:"evidenceFiles,omitempty"`

	// Overall pass/fail status
	Passed bool `json:"passed"`
}

// Finding represents a single compliance finding
type Finding struct {
	// Resource identification
	Resource     string `json:"resource"`
	ResourceType string `json:"resourceType,omitempty"`
	ResourceID   string `json:"resourceId,omitempty"`

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
	Controls []ControlMapping `json:"controlMappings,omitempty"`
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
	ControlID string `json:"controlId"`
	Satisfied bool   `json:"satisfied"`
	Notes     string `json:"notes,omitempty"`
}

// EvidenceFileResult represents the result of uploading an evidence file
type EvidenceFileResult struct {
	FilePath    string            `json:"filePath"`
	FileID      string            `json:"fileId"`
	ControlID   string            `json:"controlId"`
	Size        int64             `json:"size"`
	ContentType string            `json:"contentType"`
	Checksum    string            `json:"checksum"`
	UploadedAt  time.Time         `json:"uploadedAt"`
	Error       string            `json:"error,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// DefaultConfig returns a default agent configuration
func DefaultConfig() *Config {
	config := &Config{}
	defaults.SetDefaults(config)
	return config
}

// LoadConfig loads configuration from a file with environment variable overrides
func LoadConfig(configPath string) (*Config, error) {
	// Create koanf instance
	k := koanf.New(".")

	// Start with defaults
	config := DefaultConfig()

	// Load from file if it exists
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			if err := k.Load(file.Provider(configPath), yaml.Parser()); err != nil {
				return nil, fmt.Errorf("failed to load config file %s: %w", configPath, err)
			}
		}
	}

	// Load environment variables with OPENLANE_AGENT_ prefix
	if err := k.Load(env.Provider("OPENLANE_AGENT_", ".", func(s string) string {
		// Convert OPENLANE_AGENT_API_URL to apiUrl (simple lowercase conversion)
		return strings.ToLower(s)
	}), nil); err != nil {
		return nil, fmt.Errorf("failed to load environment variables: %w", err)
	}

	// Unmarshal into config struct
	if err := k.Unmarshal("", config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// GenerateJSONSchema generates a JSON Schema for the agent configuration
func GenerateJSONSchema() (*jsonschema.Schema, error) {
	reflector := &jsonschema.Reflector{
		AllowAdditionalProperties:  false,
		RequiredFromJSONSchemaTags: true,
	}

	// Generate schema for Config struct
	schema := reflector.Reflect(&Config{})

	// Add custom metadata
	schema.Title = "Openlane Agent Configuration"
	schema.Description = "Configuration schema for the Openlane compliance automation agent"
	schema.Version = "https://json-schema.org/draft/2020-12/schema"

	return schema, nil
}

// ValidateConfig validates a configuration against the JSON Schema and business rules
func ValidateConfig(config *Config) error {
	// Generate schema
	schema, err := GenerateJSONSchema()
	if err != nil {
		return fmt.Errorf("failed to generate schema: %w", err)
	}

	// Marshal config to JSON for validation
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Convert schema to JSON for validation
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("failed to marshal schema: %w", err)
	}

	// Compile schema using jsonschema v5
	compiler := jsonschemavalidator.NewCompiler()
	if err := compiler.AddResource("agent-config.json", strings.NewReader(string(schemaBytes))); err != nil {
		return fmt.Errorf("failed to add schema resource: %w", err)
	}

	compiledSchema, err := compiler.Compile("agent-config.json")
	if err != nil {
		return fmt.Errorf("failed to compile schema: %w", err)
	}

	// Validate against schema
	var configData interface{}
	if err := json.Unmarshal(configBytes, &configData); err != nil {
		return fmt.Errorf("failed to unmarshal config for validation: %w", err)
	}

	if err := compiledSchema.Validate(configData); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}

	// Additional business logic validation
	if err := validateBusinessRules(config); err != nil {
		return fmt.Errorf("business rule validation failed: %w", err)
	}

	return nil
}

// validateOperationMode validates the operation mode setting
func validateOperationMode(config *Config) error {
	switch config.Offline.Mode {
	case ModeNormal, ModeStandalone, ModeBuffered:
		return nil
	case "":
		// Default to normal mode if not specified
		config.Offline.Mode = ModeNormal
		return nil
	default:
		return fmt.Errorf("invalid operation mode: %s (must be 'normal', 'standalone', or 'buffered')", config.Offline.Mode)
	}
}

// validateBusinessRules performs additional validation beyond JSON Schema
func validateBusinessRules(config *Config) error {
	// Validate operation mode
	if err := validateOperationMode(config); err != nil {
		return err
	}

	// Mode-specific validation
	switch config.Offline.Mode {
	case ModeStandalone:
		// Standalone mode doesn't require API URL or registration token
		// Clear them if they're empty to avoid validation issues
		if config.APIURL == "" {
			config.APIURL = "http://localhost" // Placeholder - not used
		}
		if config.RegistrationToken == "" {
			config.RegistrationToken = "standalone-mode" // Placeholder - not used
		}
		if config.Offline.OutputDir == "" {
			return fmt.Errorf("output_dir is required for standalone mode")
		}
		// Create output directory if it doesn't exist
		if err := os.MkdirAll(config.Offline.OutputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory %s: %w", config.Offline.OutputDir, err)
		}
	case ModeNormal, ModeBuffered:
		// Normal and buffered modes require API connectivity settings
		if config.APIURL == "" {
			return fmt.Errorf("api_url is required for %s mode", config.Offline.Mode)
		}
		if config.RegistrationToken == "" {
			return fmt.Errorf("registration_token is required for %s mode", config.Offline.Mode)
		}
		
		// Buffered mode specific validation
		if config.Offline.Mode == ModeBuffered {
			if config.Offline.BufferDir == "" {
				return fmt.Errorf("buffer_dir is required for buffered mode")
			}
			// Create buffer directory if it doesn't exist
			if err := os.MkdirAll(config.Offline.BufferDir, 0755); err != nil {
				return fmt.Errorf("failed to create buffer directory %s: %w", config.Offline.BufferDir, err)
			}
		}
	}

	// Validate data directory (required for all modes)
	if config.DataDir == "" {
		return fmt.Errorf("data_dir is required")
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory %s: %w", config.DataDir, err)
	}

	// Validate checks
	checkNames := make(map[string]bool)
	for i, check := range config.Checks {
		if err := validateCheck(&check, i); err != nil {
			return fmt.Errorf("check %d (%s): %w", i, check.Name, err)
		}

		// Check for duplicate names
		if checkNames[check.Name] {
			return fmt.Errorf("duplicate check name: %s", check.Name)
		}
		checkNames[check.Name] = true
	}

	return nil
}

// validateCheck validates a single check configuration
func validateCheck(check *Check, index int) error {
	if check.Name == "" {
		return fmt.Errorf("check name is required")
	}

	if check.Command == "" {
		return fmt.Errorf("check command is required")
	}

	if check.Schedule == "" {
		return fmt.Errorf("check schedule is required")
	}

	// Validate cron expression
	if !isValidCronExpression(check.Schedule) {
		return fmt.Errorf("invalid cron schedule: %s", check.Schedule)
	}

	// Validate evidence paths
	for _, path := range check.EvidencePaths {
		if !filepath.IsAbs(path) && !filepath.IsLocal(path) {
			return fmt.Errorf("invalid evidence path: %s", path)
		}
	}

	// Validate action commands
	if check.OnPass != nil {
		if err := validateActionConfig(check.OnPass, "onPass"); err != nil {
			return err
		}
	}

	if check.OnFail != nil {
		if err := validateActionConfig(check.OnFail, "onFail"); err != nil {
			return err
		}
	}

	return nil
}

// validateActionConfig validates an action configuration
func validateActionConfig(action *ActionConfig, context string) error {
	// Validate action commands
	commandNames := make(map[string]bool)
	for i, cmd := range action.Commands {
		if cmd.Name == "" {
			return fmt.Errorf("%s command %d: name is required", context, i)
		}

		if cmd.Command == "" {
			return fmt.Errorf("%s command %d (%s): command is required", context, i, cmd.Name)
		}

		// Check for duplicate command names
		if commandNames[cmd.Name] {
			return fmt.Errorf("%s: duplicate command name: %s", context, cmd.Name)
		}
		commandNames[cmd.Name] = true
	}

	return nil
}

// SaveConfig saves the configuration to a YAML file
func (c *Config) SaveConfig(path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to YAML using gopkg.in/yaml.v3
	data, err := yamlv3.Marshal(c)
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
	// Use robfig/cron library for proper validation if needed
	// For now, basic validation
	parts := strings.Fields(expr)
	// Standard cron: minute hour day month weekday (5 parts)
	// Extended cron: second minute hour day month weekday (6 parts)
	return len(parts) == 5 || len(parts) == 6
}

// Validate implements the Validator interface for the Config struct
func (c *Config) Validate() error {
	return ValidateConfig(c)
}

// ExampleConfig returns an example configuration for documentation
func ExampleConfig() *Config {
	return &Config{
		RegistrationToken: "${OPENLANE_REGISTRATION_TOKEN}",
		APIURL:            "https://api.theopenlane.io",
		AgentName:         "production-compliance-agent",
		LogLevel:          "info",
		DataDir:           "./data",
		PollInterval:      1 * time.Minute,
		Spawn:             1,
		MaxConcurrency:    3,
		DefaultTimeout:    5 * time.Minute,
		Evidence: EvidenceConfig{
			Enabled:         true,
			RetentionPeriod: 30 * 24 * time.Hour, // 30 days
			MaxFileSize:     100 * 1024 * 1024,   // 100MB
			CompressFiles:   false,
		},
		Offline: OfflineConfig{
			Mode:                  ModeBuffered,
			OutputDir:             "./results",
			OutputFormat:          "json",
			BufferDir:             "./buffer",
			ConnectivityInterval:  30 * time.Second,
			SyncInterval:          5 * time.Minute,
			MaxRetries:            5,
			BufferRetentionPeriod: 7 * 24 * time.Hour, // 7 days
		},
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
				EvidencePaths: []string{
					"./evidence/aws-iam/",
					"./logs/aws-audit.json",
				},
				OnPass: &ActionConfig{
					UploadEvidence:      true,
					UpdateControlStatus: true,
				},
				OnFail: &ActionConfig{
					UploadEvidence:      true,
					UpdateControlStatus: true,
					Commands: []ActionCommand{
						{
							Name:    "create-remediation-ticket",
							Command: "./scripts/create-ticket.sh",
							Args:    []string{"--category", "iam", "--priority", "high"},
							Timeout: 1 * time.Minute,
						},
					},
				},
			},
			{
				Name:        "disk-encryption-check",
				Description: "Verify that full disk encryption is enabled on the system",
				Command:     "./scripts/check-disk-encryption.sh",
				Schedule:    "0 6 * * *", // Daily at 6 AM
				Timeout:     5 * time.Minute,
				Env: []string{
					"ENCRYPTION_POLICY=required",
				},
				Controls: []string{
					"SOC2:CC6.7",
					"ISO27001:A.10.1.1",
					"NIST:SC-28",
				},
				Tags:    []string{"encryption", "storage", "host-security"},
				Enabled: true,
				EvidencePaths: []string{
					"./evidence/disk-encryption-check/",
				},
				OnPass: &ActionConfig{
					UploadEvidence:      true,
					UpdateControlStatus: true,
				},
				OnFail: &ActionConfig{
					UploadEvidence:      true,
					UpdateControlStatus: true,
					Commands: []ActionCommand{
						{
							Name:            "create-security-incident",
							Command:         "./scripts/create-incident.sh",
							Args:            []string{"--type", "encryption", "--severity", "high"},
							Timeout:         1 * time.Minute,
							ContinueOnError: true,
						},
					},
				},
			},
		},
	}
}
