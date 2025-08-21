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

// File permission constants
const (
	// DefaultDirectoryPermissions for creating directories
	DefaultDirectoryPermissions = 0o755 // nolint:mnd
	// RestrictedFilePermissions for sensitive files
	RestrictedFilePermissions = 0o600 // nolint:mnd
)

// Default configuration values
const (
	// DefaultMaxRetries for operations
	DefaultMaxRetries = 10 // nolint:mnd
	// DefaultTimeoutSeconds for operations
	DefaultTimeoutSeconds = 30 // nolint:mnd
	// DefaultFileSizeLimit in bytes (1024 bytes = 1KB)
	DefaultFileSizeLimit = 1024 // nolint:mnd
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

	// Retry configuration
	Retry RetryConfig `json:"retry" koanf:"retry" description:"Retry configuration for operations"`

	// Identity configuration
	Identity IdentityConfig `json:"identity" koanf:"identity" description:"Hardware identification configuration"`

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
	Mode OperationMode `json:"mode" koanf:"mode" default:"normal" description:"Operation mode: normal, standalone, or buffered"`

	// Standalone mode settings
	OutputDir    string `json:"outputDir" koanf:"outputDir" default:"./results" description:"Directory for storing results in standalone mode"`
	OutputFormat string `json:"outputFormat" koanf:"outputFormat" default:"json" description:"Output format for standalone mode: json, yaml, csv"`

	// Buffered mode settings
	BufferDir             string        `json:"bufferDir" koanf:"bufferDir" default:"./buffer" description:"Directory for storing buffered results when offline"`
	ConnectivityCheckURL  string        `json:"connectivityCheckUrl" koanf:"connectivityCheckUrl" description:"URL endpoint for connectivity checks (defaults to API URL + /livez)"`
	ConnectivityInterval  time.Duration `json:"connectivityInterval" koanf:"connectivityInterval" default:"30s" description:"Interval for connectivity checks"`
	SyncInterval          time.Duration `json:"syncInterval" koanf:"syncInterval" default:"5m" description:"Interval for periodic sync attempts"`
	MaxRetries            int           `json:"maxRetries" koanf:"maxRetries" default:"5" description:"Maximum retry attempts for buffered results before giving up"`
	BufferRetentionPeriod time.Duration `json:"bufferRetentionPeriod" koanf:"bufferRetentionPeriod" default:"7d" description:"How long to retain buffered results before cleanup"`
}

// RetryConfig configures retry behavior for operations
type RetryConfig struct {
	MaxAttempts  uint          `json:"maxAttempts" koanf:"maxAttempts" default:"5" description:"Maximum number of retry attempts"`
	InitialDelay time.Duration `json:"initialDelay" koanf:"initialDelay" default:"1s" description:"Initial delay between retries"`
	MaxDelay     time.Duration `json:"maxDelay" koanf:"maxDelay" default:"30s" description:"Maximum delay between retries"`
	Strategy     string        `json:"strategy" koanf:"strategy" default:"exponential" description:"Retry strategy: exponential, linear, random"`
	Multiplier   float64       `json:"multiplier" koanf:"multiplier" default:"2.0" description:"Backoff multiplier for exponential strategy"`
}

// IdentityConfig configures hardware identification
type IdentityConfig struct {
	Enabled            bool          `json:"enabled" koanf:"enabled" default:"true" description:"Enable hardware ID detection"`
	CacheTimeout       time.Duration `json:"cacheTimeout" koanf:"cacheTimeout" default:"24h" description:"How long to cache hardware ID before re-detection"`
	FallbackToHostname bool          `json:"fallbackToHostname" koanf:"fallbackToHostname" default:"true" description:"Use hostname-based fallback if hardware detection fails"`
	OverrideHardwareID string        `json:"overrideHardwareId,omitempty" koanf:"overrideHardwareId" description:"Override hardware ID with this value instead of detection"`
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

	// Platform-specific variants (following gitMDM pattern)
	PlatformVariants []PlatformVariant `json:"platformVariants,omitempty" koanf:"platformVariants" description:"Platform-specific check configurations"`

	// Pass/Fail behavior configuration
	OnPass *ActionConfig `json:"onPass,omitempty" koanf:"onPass" description:"Actions to execute when check passes"`
	OnFail *ActionConfig `json:"onFail,omitempty" koanf:"onFail" description:"Actions to execute when check fails"`

	// Runtime tracking (not persisted)
	LastRun    time.Time `json:"-" koanf:"-"`
	LastResult *Result   `json:"-" koanf:"-"`
}

// PlatformVariant represents platform-specific check configuration
type PlatformVariant struct {
	Platforms   []string       `json:"platforms" koanf:"platforms" description:"Target platforms (os, os/arch, or comma-separated list)"`
	Command     string         `json:"command,omitempty" koanf:"command" description:"Platform-specific command (overrides default)"`
	Args        []string       `json:"args,omitempty" koanf:"args" description:"Platform-specific arguments (overrides default)"`
	File        string         `json:"file,omitempty" koanf:"file" description:"File to check instead of command"`
	Includes    string         `json:"includes,omitempty" koanf:"includes" description:"Regex pattern - pass if matches"`
	Excludes    string         `json:"excludes,omitempty" koanf:"excludes" description:"Regex pattern - fail if matches"`
	ExitCode    *int           `json:"exitCode,omitempty" koanf:"exitCode" description:"Expected exit code for success"`
	Env         []string       `json:"env,omitempty" koanf:"env" description:"Platform-specific environment variables"`
	WorkDir     string         `json:"workDir,omitempty" koanf:"workDir" description:"Platform-specific working directory"`
	Timeout     *time.Duration `json:"timeout,omitempty" koanf:"timeout" description:"Platform-specific timeout"`
	Remediation []string       `json:"remediation,omitempty" koanf:"remediation" description:"Platform-specific remediation steps"`
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
	// SeverityCritical represents critical severity level
	SeverityCritical FindingSeverity = "critical"
	// SeverityHigh represents high severity level
	SeverityHigh FindingSeverity = "high"
	// SeverityMedium represents medium severity level
	SeverityMedium FindingSeverity = "medium"
	// SeverityLow represents low severity level
	SeverityLow FindingSeverity = "low"
	// SeverityInfo represents informational severity level
	SeverityInfo FindingSeverity = "info"
)

// FindingStatus represents the current status of a finding
type FindingStatus string

const (
	// StatusOpen represents an open finding status
	StatusOpen FindingStatus = "open"
	// StatusAcknowledged represents an acknowledged finding status
	StatusAcknowledged FindingStatus = "acknowledged"
	// StatusRemediated represents a remediated finding status
	StatusRemediated FindingStatus = "remediated"
	// StatusFalsePositive represents a false positive finding status
	StatusFalsePositive FindingStatus = "false_positive"
	// StatusAccepted represents an accepted finding status
	StatusAccepted FindingStatus = "accepted"
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
				return nil, fmt.Errorf("%w %s: %w", ErrFailedToLoadConfigFile, configPath, err)
			}
		}
	}

	// Load environment variables with OPENLANE_AGENT_ prefix
	if err := k.Load(env.Provider("OPENLANE_AGENT_", ".", strings.ToLower), nil); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToLoadEnvironmentVariables, err)
	}

	// Unmarshal into config struct
	if err := k.Unmarshal("", config); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrFailedToUnmarshalConfigFile, err)
	}

	// Validate configuration
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConfigurationValidationFailed, err)
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
		return fmt.Errorf("%w: %w", ErrFailedToGenerateJSONSchema, err)
	}

	// Marshal config to JSON for validation
	configBytes, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToMarshalConfig, err)
	}

	// Convert schema to JSON for validation
	schemaBytes, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToMarshalSchema, err)
	}

	// Compile schema using jsonschema v5
	compiler := jsonschemavalidator.NewCompiler()
	if err := compiler.AddResource("agent-config.json", strings.NewReader(string(schemaBytes))); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToAddSchemaResource, err)
	}

	compiledSchema, err := compiler.Compile("agent-config.json")
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCompileSchema, err)
	}

	// Validate against schema
	var configData any
	if err := json.Unmarshal(configBytes, &configData); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToUnmarshalConfig, err)
	}

	if err := compiledSchema.Validate(configData); err != nil {
		return fmt.Errorf("%w: %w", ErrSchemaValidationFailed, err)
	}

	// Additional business logic validation
	if err := validateBusinessRules(config); err != nil {
		return fmt.Errorf("%w: %w", ErrBusinessRuleValidationFailed, err)
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
		return fmt.Errorf("%w: %s (must be 'normal', 'standalone', or 'buffered')", ErrInvalidOperationMode, config.Offline.Mode)
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
			return ErrOutputDirRequired
		}
		// Create output directory if it doesn't exist
		if err := os.MkdirAll(config.Offline.OutputDir, 0o755 /* nolint:mnd */); err != nil {
			return fmt.Errorf("%w %s: %w", ErrFailedToCreateOutputDir, config.Offline.OutputDir, err)
		}
	case ModeNormal, ModeBuffered:
		// Normal and buffered modes require API connectivity settings
		if config.APIURL == "" {
			return fmt.Errorf("%w for %s mode", ErrAPIURLRequired, config.Offline.Mode)
		}

		if config.RegistrationToken == "" {
			return fmt.Errorf("%w for %s mode", ErrRegistrationTokenRequired, config.Offline.Mode)
		}

		// Buffered mode specific validation
		if config.Offline.Mode == ModeBuffered {
			if config.Offline.BufferDir == "" {
				return ErrBufferDirRequired
			}
			// Create buffer directory if it doesn't exist
			if err := os.MkdirAll(config.Offline.BufferDir, 0o755 /* nolint:mnd */); err != nil {
				return fmt.Errorf("%w %s: %w", ErrFailedToCreateBufferDir, config.Offline.BufferDir, err)
			}
		}
	}

	// Validate data directory (required for all modes)
	if config.DataDir == "" {
		return ErrDataDirRequired
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(config.DataDir, 0o755 /* nolint:mnd */); err != nil {
		return fmt.Errorf("%w %s: %w", ErrFailedToCreateDataDir, config.DataDir, err)
	}

	// Validate checks
	checkNames := make(map[string]bool)

	for i, check := range config.Checks {
		if err := validateCheck(&check); err != nil {
			return fmt.Errorf("%w %d (%s): %w", ErrCheckValidationFailed, i, check.Name, err)
		}

		// Check for duplicate names
		if checkNames[check.Name] {
			return fmt.Errorf("%w: %s", ErrDuplicateCheckName, check.Name)
		}

		checkNames[check.Name] = true
	}

	return nil
}

// validateCheck validates a single check configuration
func validateCheck(check *Check) error {
	if check.Name == "" {
		return ErrCheckNameRequired
	}

	if check.Command == "" {
		return ErrCheckCommandRequired
	}

	if check.Schedule == "" {
		return ErrCheckScheduleRequired
	}

	// Validate cron expression
	if !isValidCronExpression(check.Schedule) {
		return fmt.Errorf("%w: %s", ErrInvalidCronSchedule, check.Schedule)
	}

	// Validate evidence paths
	for _, path := range check.EvidencePaths {
		if !filepath.IsAbs(path) && !filepath.IsLocal(path) {
			return fmt.Errorf("%w: %s", ErrInvalidEvidencePath, path)
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
			return fmt.Errorf("%w for %s command %d", ErrActionCommandNameRequired, context, i)
		}

		if cmd.Command == "" {
			return fmt.Errorf("%w for %s command %d (%s)", ErrActionCommandRequired, context, i, cmd.Name)
		}

		// Check for duplicate command names
		if commandNames[cmd.Name] {
			return fmt.Errorf("%w for %s: %s", ErrDuplicateActionCommandName, context, cmd.Name)
		}

		commandNames[cmd.Name] = true
	}

	return nil
}

// SaveConfig saves the configuration to a YAML file
func (c *Config) SaveConfig(path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755 /* nolint:mnd */); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCreateConfigDir, err)
	}

	// Marshal to YAML using gopkg.in/yaml.v3
	data, err := yamlv3.Marshal(c)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToMarshalConfig, err)
	}

	// Write file
	if err := os.WriteFile(path, data, 0o600 /* nolint:mnd */); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToWriteConfigFile, err)
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

	return nil, fmt.Errorf("%w: %s", ErrCheckNotFound, name)
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
	return len(parts) == 5 /* nolint:mnd */ || len(parts) == 6 /* nolint:mnd */
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
			RetentionPeriod: 30 * 24 * time.Hour, // 30 days // nolint:mnd
			MaxFileSize:     100 * 1024 * 1024,   // 100MB // nolint:mnd
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
			BufferRetentionPeriod: 7 * 24 * time.Hour, // 7 days // nolint:mnd
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
