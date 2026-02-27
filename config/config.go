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
	"github.com/robfig/cron/v3"
	jsonschemavalidator "github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/theopenlane/agent/schema"
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

	// Agent configuration defaults
	defaultMaxConcurrency              = 3
	defaultTimeoutMinutes              = 5
	defaultConnectivityIntervalSeconds = 30
	defaultSyncIntervalMinutes         = 5
	defaultMaxRetriesConfig            = 5
	defaultCheckTimeoutMinutes         = 10
)

// Config represents the agent configuration loaded from agent.yaml
type Config struct {
	// APIToken is the token used for authenticated API requests
	APIToken string `json:"token" yaml:"token" koanf:"token" sensitive:"true" description:"Token used for authenticated API requests"`
	// APIURL is the base URL for the Openlane API
	APIURL string `json:"apiUrl" yaml:"apiUrl" koanf:"apiUrl" default:"https://api.theopenlane.io" description:"Base URL for the Openlane API"`
	// OrgID scopes all API requests to a specific organization
	OrgID string `json:"orgId,omitempty" yaml:"orgId,omitempty" koanf:"orgId" description:"Organization ID to scope all API requests"`
	// AgentID is the unique identifier for this agent instance
	AgentID string `json:"agentId,omitempty" yaml:"agentId,omitempty" koanf:"agentId" description:"Unique identifier for this agent instance"`
	// AgentName is the human-readable name for this agent
	AgentName string `json:"agentName,omitempty" yaml:"agentName,omitempty" koanf:"agentName" description:"Human-readable name for this agent"`
	// LogLevel is the log verbosity level: debug, info, warn, or error
	LogLevel string `json:"logLevel" yaml:"logLevel" koanf:"logLevel" default:"info" description:"Log level (debug, info, warn, error)"`
	// DataDir is the directory used for storing agent data and state
	DataDir string `json:"dataDir" yaml:"dataDir" koanf:"dataDir" default:"./data" description:"Directory for storing agent data"`
	// PollInterval is the interval at which local check schedules are scanned
	PollInterval time.Duration `json:"pollInterval" yaml:"pollInterval" koanf:"pollInterval" default:"1m" description:"Interval for scanning local check schedules"`
	// Spawn is the number of worker processes to start
	Spawn int `json:"spawn" yaml:"spawn" koanf:"spawn" default:"1" description:"Number of worker processes to spawn"`
	// MaxConcurrency is the maximum number of checks that may run simultaneously
	MaxConcurrency int `json:"maxConcurrency" yaml:"maxConcurrency" koanf:"maxConcurrency" default:"3" description:"Maximum number of concurrent check executions"`
	// DefaultTimeout is the default timeout applied to each check execution
	DefaultTimeout time.Duration `json:"defaultTimeout" yaml:"defaultTimeout" koanf:"defaultTimeout" default:"5m" description:"Default timeout for check execution"`
	// Evidence configures evidence collection and retention
	Evidence EvidenceConfig `json:"evidence" yaml:"evidence" koanf:"evidence" description:"Evidence collection and retention configuration"`
	// Offline configures offline buffering and standalone operation
	Offline OfflineConfig `json:"offline" yaml:"offline" koanf:"offline" description:"Offline buffering and standalone mode configuration"`
	// Retry configures retry behavior for operations
	Retry RetryConfig `json:"retry" yaml:"retry" koanf:"retry" description:"Retry configuration for operations"`
	// Identity configures hardware identification
	Identity IdentityConfig `json:"identity" yaml:"identity" koanf:"identity" description:"Hardware identification configuration"`
	// Checks is the list of local compliance checks to execute
	Checks []Check `json:"checks" yaml:"checks" koanf:"checks" description:"Local compliance checks to execute"`
}

// EvidenceConfig configures evidence collection and retention
type EvidenceConfig struct {
	// Enabled controls whether evidence collection is active
	Enabled bool `json:"enabled" yaml:"enabled" koanf:"enabled" default:"true" description:"Enable evidence collection"`
	// RetentionPeriod is how long to retain collected evidence files
	RetentionPeriod time.Duration `json:"retentionPeriod" yaml:"retentionPeriod" koanf:"retentionPeriod" default:"720h" description:"How long to retain evidence files"`
	// MaxFileSize is the maximum size in bytes of a single evidence file (default 100MB)
	MaxFileSize int64 `json:"maxFileSize" yaml:"maxFileSize" koanf:"maxFileSize" default:"104857600" description:"Maximum evidence file size in bytes (100MB default)"`
	// CompressFiles controls whether evidence files are compressed before storage
	CompressFiles bool `json:"compressFiles" yaml:"compressFiles" koanf:"compressFiles" default:"false" description:"Compress evidence files to save space"`
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
	// Mode is the operation mode: normal, standalone, or buffered
	Mode OperationMode `json:"mode" yaml:"mode" koanf:"mode" default:"normal" description:"Operation mode: normal, standalone, or buffered"`
	// OutputDir is the directory for storing results in standalone mode
	OutputDir string `json:"outputDir" yaml:"outputDir" koanf:"outputDir" default:"./results" description:"Directory for storing results in standalone mode"`
	// OutputFormat is the output format for standalone mode: json, yaml, or csv
	OutputFormat string `json:"outputFormat" yaml:"outputFormat" koanf:"outputFormat" default:"json" description:"Output format for standalone mode: json, yaml, csv"`
	// BufferDir is the directory for storing buffered results when offline
	BufferDir string `json:"bufferDir" yaml:"bufferDir" koanf:"bufferDir" default:"./buffer" description:"Directory for storing buffered results when offline"`
	// ConnectivityCheckURL is the URL endpoint for connectivity checks; defaults to the API URL
	ConnectivityCheckURL string `json:"connectivityCheckUrl" yaml:"connectivityCheckUrl" koanf:"connectivityCheckUrl" description:"URL endpoint for connectivity checks (defaults to API URL + /livez)"`
	// ConnectivityInterval is the interval between connectivity checks
	ConnectivityInterval time.Duration `json:"connectivityInterval" yaml:"connectivityInterval" koanf:"connectivityInterval" default:"30s" description:"Interval for connectivity checks"`
	// SyncInterval is the interval between periodic buffered-result sync attempts
	SyncInterval time.Duration `json:"syncInterval" yaml:"syncInterval" koanf:"syncInterval" default:"5m" description:"Interval for periodic sync attempts"`
	// MaxRetries is the maximum retry attempts for a buffered result before it is abandoned
	MaxRetries int `json:"maxRetries" yaml:"maxRetries" koanf:"maxRetries" default:"5" description:"Maximum retry attempts for buffered results before giving up"`
	// BufferRetentionPeriod is how long to retain buffered results before cleanup
	BufferRetentionPeriod time.Duration `json:"bufferRetentionPeriod" yaml:"bufferRetentionPeriod" koanf:"bufferRetentionPeriod" default:"168h" description:"How long to retain buffered results before cleanup"`
}

// RetryConfig configures retry behavior for operations
type RetryConfig struct {
	// MaxAttempts is the maximum number of retry attempts
	MaxAttempts uint `json:"maxAttempts" yaml:"maxAttempts" koanf:"maxAttempts" default:"5" description:"Maximum number of retry attempts"`
	// InitialDelay is the initial delay between retries
	InitialDelay time.Duration `json:"initialDelay" yaml:"initialDelay" koanf:"initialDelay" default:"1s" description:"Initial delay between retries"`
	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration `json:"maxDelay" yaml:"maxDelay" koanf:"maxDelay" default:"30s" description:"Maximum delay between retries"`
	// Strategy is the retry strategy: exponential, linear, or random
	Strategy string `json:"strategy" yaml:"strategy" koanf:"strategy" default:"exponential" description:"Retry strategy: exponential, linear, random"`
	// Multiplier is the backoff multiplier used by the exponential strategy
	Multiplier float64 `json:"multiplier" yaml:"multiplier" koanf:"multiplier" default:"2.0" description:"Backoff multiplier for exponential strategy"`
}

// IdentityConfig configures hardware identification
type IdentityConfig struct {
	// Enabled controls whether hardware ID detection is active
	Enabled bool `json:"enabled" yaml:"enabled" koanf:"enabled" default:"true" description:"Enable hardware ID detection"`
	// CacheTimeout is how long to cache the detected hardware ID before re-detection
	CacheTimeout time.Duration `json:"cacheTimeout" yaml:"cacheTimeout" koanf:"cacheTimeout" default:"24h" description:"How long to cache hardware ID before re-detection"`
	// FallbackToHostname enables hostname-based fallback when hardware detection fails
	FallbackToHostname bool `json:"fallbackToHostname" yaml:"fallbackToHostname" koanf:"fallbackToHostname" default:"true" description:"Use hostname-based fallback if hardware detection fails"`
	// OverrideHardwareID replaces the detected hardware ID with a static value
	OverrideHardwareID string `json:"overrideHardwareId,omitempty" yaml:"overrideHardwareId,omitempty" koanf:"overrideHardwareId" description:"Override hardware ID with this value instead of detection"`
}

// ComplianceStandard represents a compliance standard and its associated controls
type ComplianceStandard struct {
	// Standard is the compliance standard identifier (e.g. soc2v2022, nist80053v5)
	Standard string `json:"standard" yaml:"standard" koanf:"standard" description:"Compliance standard identifier (e.g., soc2v2022, nist80053v5)"`
	// Controls is the list of control reference codes within this standard
	Controls []string `json:"controls" yaml:"controls" koanf:"controls" description:"Control reference codes within this standard"`
}

// Check represents a single compliance check configuration
type Check struct {
	// Name is the unique name for this check
	Name string `json:"name" yaml:"name" koanf:"name" description:"Unique name for this check"`
	// Description is a human-readable description of what this check validates
	Description string `json:"description,omitempty" yaml:"description,omitempty" koanf:"description" description:"Human-readable description of what this check validates"`
	// Command is the command to execute for this check
	Command string `json:"command" yaml:"command" koanf:"command" description:"Command to execute for this check"`
	// Args is the list of arguments to pass to the command
	Args []string `json:"args,omitempty" yaml:"args,omitempty" koanf:"args" description:"Arguments to pass to the command"`
	// WorkDir is the working directory for command execution
	WorkDir string `json:"workDir,omitempty" yaml:"workDir,omitempty" koanf:"workDir" description:"Working directory for command execution"`
	// Env lists environment variables to set for command execution
	Env []string `json:"env,omitempty" yaml:"env,omitempty" koanf:"env" description:"Environment variables for command execution"`
	// Schedule is the cron expression controlling when to run this check
	Schedule string `json:"schedule" yaml:"schedule" koanf:"schedule" description:"Cron expression for when to run this check"`
	// Timeout is the maximum execution time for this check
	Timeout time.Duration `json:"timeout" yaml:"timeout" koanf:"timeout" default:"5m" description:"Timeout for this check execution"`
	// ComplianceStandards lists the compliance standards and controls this check validates
	ComplianceStandards []ComplianceStandard `json:"complianceStandards,omitempty" yaml:"complianceStandards,omitempty" koanf:"complianceStandards" description:"Compliance standards and controls this check validates"`
	// Tags categorize and filter this check
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty" koanf:"tags" description:"Tags for categorizing and filtering checks"`
	// Enabled controls whether this check is active
	Enabled bool `json:"enabled" yaml:"enabled" koanf:"enabled" default:"true" description:"Whether this check is enabled"`
	// ContinueOnError controls whether other checks continue if this one fails
	ContinueOnError bool `json:"continueOnError" yaml:"continueOnError" koanf:"continueOnError" default:"false" description:"Continue executing other checks if this one fails"`
	// EvidencePaths lists file paths or directories to collect as evidence
	EvidencePaths []string `json:"evidencePaths,omitempty" yaml:"evidencePaths,omitempty" koanf:"evidencePaths" description:"File paths or directories to collect as evidence"`
	// PlatformVariants contains platform-specific check configurations
	PlatformVariants []PlatformVariant `json:"platformVariants,omitempty" yaml:"platformVariants,omitempty" koanf:"platformVariants" description:"Platform-specific check configurations"`
	// OnPass defines actions to execute when the check passes
	OnPass *ActionConfig `json:"onPass,omitempty" yaml:"onPass,omitempty" koanf:"onPass" description:"Actions to execute when check passes"`
	// OnFail defines actions to execute when the check fails
	OnFail *ActionConfig `json:"onFail,omitempty" yaml:"onFail,omitempty" koanf:"onFail" description:"Actions to execute when check fails"`
	// LastRun is the time the check was last executed; not persisted across restarts
	LastRun time.Time `json:"-" yaml:"-" koanf:"-"`
	// LastResult is the result of the most recent execution; not persisted across restarts
	LastResult *Result `json:"-" yaml:"-" koanf:"-"`
}

// PlatformVariant represents platform-specific check configuration
type PlatformVariant struct {
	// Platforms lists target platforms using os, os/arch, or comma-separated values
	Platforms []string `json:"platforms" yaml:"platforms" koanf:"platforms" description:"Target platforms (os, os/arch, or comma-separated list)"`
	// Command overrides the default check command for this platform
	Command string `json:"command,omitempty" yaml:"command,omitempty" koanf:"command" description:"Platform-specific command (overrides default)"`
	// Args overrides the default command arguments for this platform
	Args []string `json:"args,omitempty" yaml:"args,omitempty" koanf:"args" description:"Platform-specific arguments (overrides default)"`
	// File is the file to read and evaluate instead of running a command
	File string `json:"file,omitempty" yaml:"file,omitempty" koanf:"file" description:"File to check instead of command"`
	// Includes is a regex pattern; the check passes when the output matches
	Includes string `json:"includes,omitempty" yaml:"includes,omitempty" koanf:"includes" description:"Regex pattern - pass if matches"`
	// Excludes is a regex pattern; the check fails when the output matches
	Excludes string `json:"excludes,omitempty" yaml:"excludes,omitempty" koanf:"excludes" description:"Regex pattern - fail if matches"`
	// ExitCode is the expected exit code for a successful check
	ExitCode *int `json:"exitCode,omitempty" yaml:"exitCode,omitempty" koanf:"exitCode" description:"Expected exit code for success"`
	// Env lists platform-specific environment variables
	Env []string `json:"env,omitempty" yaml:"env,omitempty" koanf:"env" description:"Platform-specific environment variables"`
	// WorkDir overrides the working directory for this platform
	WorkDir string `json:"workDir,omitempty" yaml:"workDir,omitempty" koanf:"workDir" description:"Platform-specific working directory"`
	// Timeout overrides the check timeout for this platform
	Timeout *time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty" koanf:"timeout" description:"Platform-specific timeout"`
	// Remediation lists platform-specific steps to resolve a failing check
	Remediation []string `json:"remediation,omitempty" yaml:"remediation,omitempty" koanf:"remediation" description:"Platform-specific remediation steps"`
}

// ActionConfig defines actions to take on pass/fail scenarios
type ActionConfig struct {
	// UploadEvidence controls whether collected evidence is uploaded to associated controls
	UploadEvidence bool `json:"uploadEvidence" yaml:"uploadEvidence" koanf:"uploadEvidence" default:"false" description:"Upload collected evidence to associated controls"`
	// Commands lists the commands to execute when this outcome is triggered
	Commands []ActionCommand `json:"commands,omitempty" yaml:"commands,omitempty" koanf:"commands" description:"Commands to execute for this outcome"`
}

// ActionCommand represents a command to execute on pass/fail
type ActionCommand struct {
	// Name is the unique name for this action command
	Name string `json:"name" yaml:"name" koanf:"name" description:"Unique name for this action"`
	// Command is the command string to execute
	Command string `json:"command" yaml:"command" koanf:"command" description:"Command to execute"`
	// Args is the list of arguments to pass to the command
	Args []string `json:"args,omitempty" yaml:"args,omitempty" koanf:"args" description:"Arguments to pass to the command"`
	// WorkDir is the working directory for command execution
	WorkDir string `json:"workDir,omitempty" yaml:"workDir,omitempty" koanf:"workDir" description:"Working directory for command execution"`
	// Env lists environment variables for command execution
	Env []string `json:"env,omitempty" yaml:"env,omitempty" koanf:"env" description:"Environment variables for command execution"`
	// Timeout is the maximum execution time for this action command
	Timeout time.Duration `json:"timeout,omitempty" yaml:"timeout,omitempty" koanf:"timeout" default:"1m" description:"Timeout for command execution"`
	// ContinueOnError controls whether subsequent actions run if this command fails
	ContinueOnError bool `json:"continueOnError" yaml:"continueOnError" koanf:"continueOnError" default:"false" description:"Continue with other actions if this command fails"`
}

// Result is a type alias for the standardized compliance check result
type Result = schema.ComplianceCheckResult

// Finding represents a single compliance finding
type Finding struct {
	// Resource identifies the resource this finding applies to
	Resource string `json:"resource" yaml:"resource"`
	// ResourceType is the type classification of the resource
	ResourceType string `json:"resourceType,omitempty" yaml:"resourceType,omitempty"`
	// ResourceID is the unique identifier of the affected resource
	ResourceID string `json:"resourceId,omitempty" yaml:"resourceId,omitempty"`
	// Title is a short description of the finding
	Title string `json:"title" yaml:"title"`
	// Description provides additional detail about the finding
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Severity is the severity level of the finding
	Severity FindingSeverity `json:"severity" yaml:"severity"`
	// Status is the current lifecycle status of the finding
	Status FindingStatus `json:"status" yaml:"status"`
	// Details contains additional structured information about the finding
	Details map[string]any `json:"details,omitempty" yaml:"details,omitempty"`
	// Remediation provides guidance for resolving the finding
	Remediation string `json:"remediation,omitempty" yaml:"remediation,omitempty"`
	// References lists URLs or document references related to the finding
	References []string `json:"references,omitempty" yaml:"references,omitempty"`
	// Controls maps the finding to compliance controls
	Controls []ControlMapping `json:"controlMappings,omitempty" yaml:"controlMappings,omitempty"`
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
	// Framework is the compliance framework identifier
	Framework string `json:"framework" yaml:"framework"`
	// ControlID is the control identifier within the framework
	ControlID string `json:"controlId" yaml:"controlId"`
	// Satisfied indicates whether the mapped control requirement is met
	Satisfied bool `json:"satisfied" yaml:"satisfied"`
	// Notes contains additional information about this control mapping
	Notes string `json:"notes,omitempty" yaml:"notes,omitempty"`
}

// EvidenceFileResult represents the result of uploading an evidence file
type EvidenceFileResult struct {
	// FilePath is the local path of the evidence file that was uploaded
	FilePath string `json:"filePath" yaml:"filePath"`
	// FileID is the unique identifier assigned to the file by the platform
	FileID string `json:"fileId" yaml:"fileId"`
	// ControlID is the control this evidence file is associated with
	ControlID string `json:"controlId" yaml:"controlId"`
	// Size is the size of the evidence file in bytes
	Size int64 `json:"size" yaml:"size"`
	// ContentType is the MIME type of the evidence file
	ContentType string `json:"contentType" yaml:"contentType"`
	// Checksum is the SHA-256 checksum of the evidence file
	Checksum string `json:"checksum" yaml:"checksum"`
	// UploadedAt is the time the file was successfully uploaded
	UploadedAt time.Time `json:"uploadedAt" yaml:"uploadedAt"`
	// Error holds an error message when the upload failed
	Error string `json:"error,omitempty" yaml:"error,omitempty"`
	// Metadata contains additional key-value pairs about the upload
	Metadata map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
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

	// Explicit env aliases for camelCase top-level keys that don't map cleanly
	// via koanf's generic env provider transform.
	if v := firstNonEmptyEnv("OPENLANE_AGENT_TOKEN", "OPENLANE_AGENT_API_TOKEN"); v != "" {
		config.APIToken = v
	}

	if v := firstNonEmptyEnv("OPENLANE_AGENT_REGISTRATIONTOKEN", "OPENLANE_AGENT_REGISTRATION_TOKEN"); v != "" && strings.TrimSpace(config.APIToken) == "" {
		config.APIToken = v
	}

	if strings.TrimSpace(config.APIToken) == "" {
		if v := strings.TrimSpace(k.String("registrationToken")); v != "" {
			config.APIToken = v
		}
	}

	if v := firstNonEmptyEnv("OPENLANE_AGENT_APIURL", "OPENLANE_AGENT_API_URL"); v != "" {
		config.APIURL = v
	}

	config.normalizeToken()

	// Validate configuration structure
	if err := ValidateConfig(config); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConfigurationValidationFailed, err)
	}

	return config, nil
}

func firstNonEmptyEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}

	return ""
}

func (c *Config) normalizeToken() {
	if c == nil {
		return
	}

	c.APIToken = strings.TrimSpace(c.APIToken)
}

// Token returns the normalized API token value
func (c *Config) Token() string {
	if c == nil {
		return ""
	}

	c.normalizeToken()

	return strings.TrimSpace(c.APIToken)
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

// ValidateConfig validates configuration for runtime execution
func ValidateConfig(config *Config) error {
	return ValidateConfigForRuntime(config)
}

// ValidateConfigStructure validates configuration structure only (JSON schema)
func ValidateConfigStructure(config *Config) error {
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

	return nil
}

// ValidateConfigForRuntime validates config for agent runtime operation
func ValidateConfigForRuntime(config *Config) error {
	config.normalizeToken()

	// First validate structure
	if err := ValidateConfigStructure(config); err != nil {
		return err
	}

	// Then validate runtime requirements
	if err := validateBusinessRules(config); err != nil {
		return fmt.Errorf("runtime validation failed: %w", err)
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
		if config.Offline.OutputDir == "" {
			return ErrOutputDirRequired
		}
		// Create output directory if it doesn't exist
		if err := os.MkdirAll(config.Offline.OutputDir, DefaultDirectoryPermissions); err != nil {
			return fmt.Errorf("%w %s: %w", ErrFailedToCreateOutputDir, config.Offline.OutputDir, err)
		}
	case ModeNormal, ModeBuffered:
		// Normal and buffered modes require API connectivity settings
		if config.APIURL == "" {
			return fmt.Errorf("%w for %s mode", ErrAPIURLRequired, config.Offline.Mode)
		}

		if config.Token() == "" {
			return fmt.Errorf("%w for %s mode", ErrAPITokenRequired, config.Offline.Mode)
		}

		// Buffered mode specific validation
		if config.Offline.Mode == ModeBuffered {
			if config.Offline.BufferDir == "" {
				return ErrBufferDirRequired
			}
			// Create buffer directory if it doesn't exist
			if err := os.MkdirAll(config.Offline.BufferDir, DefaultDirectoryPermissions); err != nil {
				return fmt.Errorf("%w %s: %w", ErrFailedToCreateBufferDir, config.Offline.BufferDir, err)
			}
		}
	}

	// Validate data directory (required for all modes)
	if config.DataDir == "" {
		return ErrDataDirRequired
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(config.DataDir, DefaultDirectoryPermissions); err != nil {
		return fmt.Errorf("%w %s: %w", ErrFailedToCreateDataDir, config.DataDir, err)
	}

	// Validate checks
	checkNames := make(map[string]bool)

	for i := range config.Checks {
		check := &config.Checks[i]
		if err := validateCheck(check); err != nil {
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
	if err := os.MkdirAll(dir, DefaultDirectoryPermissions); err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToCreateConfigDir, err)
	}

	// Marshal to YAML using gopkg.in/yaml.v3
	data, err := yamlv3.Marshal(c)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrFailedToMarshalConfig, err)
	}

	// Write file
	if err := os.WriteFile(path, data, RestrictedFilePermissions); err != nil {
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

// GetAllControls returns all controls from all standards as a flat list (for backward compatibility)
func (c *Check) GetAllControls() []string {
	var controls []string
	for _, standard := range c.ComplianceStandards {
		controls = append(controls, standard.Controls...)
	}

	return controls
}

// GetControlsByStandard returns a map of standard -> controls
func (c *Check) GetControlsByStandard() map[string][]string {
	controlsByStandard := make(map[string][]string)
	for _, standard := range c.ComplianceStandards {
		controlsByStandard[standard.Standard] = standard.Controls
	}

	return controlsByStandard
}

// HasSingleControl returns true if this check has exactly one control across all standards
func (c *Check) HasSingleControl() bool {
	totalControls := 0
	for _, standard := range c.ComplianceStandards {
		totalControls += len(standard.Controls)
	}

	return totalControls == 1
}

// GetStandards returns all standard identifiers
func (c *Check) GetStandards() []string {
	var standards []string
	for _, standard := range c.ComplianceStandards {
		standards = append(standards, standard.Standard)
	}

	return standards
}

// ShouldExecuteCheck determines if a check should run based on validated controls
// Returns (shouldExecute, validControls, reason)
func (c *Check) ShouldExecuteCheck(validatedControls map[string][]string) (bool, map[string][]string, string) {
	if len(c.ComplianceStandards) == 0 {
		// No compliance standards configured, run the check
		return true, make(map[string][]string), "no compliance standards configured"
	}

	// Count total controls and valid controls
	totalControls := 0
	validControls := make(map[string][]string)

	for _, standard := range c.ComplianceStandards {
		totalControls += len(standard.Controls)

		// Check if any controls from this standard are valid
		if validStandardControls, exists := validatedControls[standard.Standard]; exists && len(validStandardControls) > 0 {
			validControls[standard.Standard] = validStandardControls
		}
	}

	// Count valid controls
	validControlCount := 0
	for _, controls := range validControls {
		validControlCount += len(controls)
	}

	// Apply fail-open logic:
	// 1. If only 1 control total and it's invalid, skip check
	// 2. If any controls are valid, run check (even if some are invalid)

	if totalControls == 1 && validControlCount == 0 {
		return false, validControls, "single control reference is invalid"
	}

	if validControlCount > 0 {
		return true, validControls, fmt.Sprintf("found %d valid controls out of %d total", validControlCount, totalControls)
	}

	// All controls are invalid but there are multiple controls - run anyway
	if totalControls > 1 {
		return true, validControls, "multiple controls configured, running despite validation failures"
	}

	// Shouldn't reach here, but default to running
	return true, validControls, "default: running check"
}

// isValidCronExpression performs basic validation of cron expressions
func isValidCronExpression(expr string) bool {
	parts := strings.Fields(expr)
	if len(parts) != 5 && len(parts) != 6 { // nolint:mnd
		return false
	}

	var parser cron.Parser
	if len(parts) == 6 { // nolint:mnd
		parser = cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	} else {
		parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	}

	_, err := parser.Parse(expr)

	return err == nil
}

// Validate implements the Validator interface for the Config struct
func (c *Config) Validate() error {
	return ValidateConfig(c)
}

// ExampleConfig returns an example configuration for documentation
func ExampleConfig() *Config {
	return &Config{
		APIToken:       "${OPENLANE_AGENT_TOKEN}",
		APIURL:         "https://api.theopenlane.io",
		AgentName:      "production-compliance-agent",
		LogLevel:       "info",
		DataDir:        "./data",
		PollInterval:   1 * time.Minute,
		Spawn:          1,
		MaxConcurrency: defaultMaxConcurrency,
		DefaultTimeout: defaultTimeoutMinutes * time.Minute,
		Evidence: EvidenceConfig{
			Enabled:         true,
			RetentionPeriod: 30 * 24 * time.Hour,                               // 30 days // nolint:mnd
			MaxFileSize:     100 * DefaultFileSizeLimit * DefaultFileSizeLimit, // 100MB
			CompressFiles:   false,
		},
		Offline: OfflineConfig{
			Mode:                  ModeBuffered,
			OutputDir:             "./results",
			OutputFormat:          "json",
			BufferDir:             "./buffer",
			ConnectivityInterval:  defaultConnectivityIntervalSeconds * time.Second,
			SyncInterval:          defaultSyncIntervalMinutes * time.Minute,
			MaxRetries:            defaultMaxRetriesConfig,
			BufferRetentionPeriod: 7 * 24 * time.Hour, // 7 days // nolint:mnd
		},
		Checks: []Check{
			{
				Name:        "aws-iam-compliance",
				Description: "Check AWS IAM configuration for compliance issues",
				Command:     "./scripts/check-aws-iam.sh",
				Schedule:    "0 */4 * * *", // Every 4 hours
				Timeout:     defaultCheckTimeoutMinutes * time.Minute,
				Env: []string{
					"AWS_REGION=us-east-1",
				},
				ComplianceStandards: []ComplianceStandard{
					{
						Standard: "soc2v2022",
						Controls: []string{"CC6.1"},
					},
					{
						Standard: "iso27001v2022",
						Controls: []string{"A.9.2.1"},
					},
				},
				Tags:    []string{"aws", "iam", "critical"},
				Enabled: true,
				EvidencePaths: []string{
					"./evidence/aws-iam/",
					"./logs/aws-audit.json",
				},
				OnPass: &ActionConfig{
					UploadEvidence: true,
				},
				OnFail: &ActionConfig{
					UploadEvidence: true,
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
				Timeout:     defaultTimeoutMinutes * time.Minute,
				Env: []string{
					"ENCRYPTION_POLICY=required",
				},
				ComplianceStandards: []ComplianceStandard{
					{
						Standard: "soc2v2022",
						Controls: []string{"CC6.7"},
					},
					{
						Standard: "iso27001v2022",
						Controls: []string{"A.10.1.1"},
					},
					{
						Standard: "nist80053v5",
						Controls: []string{"SC-28"},
					},
				},
				Tags:    []string{"encryption", "storage", "host-security"},
				Enabled: true,
				EvidencePaths: []string{
					"./evidence/disk-encryption-check/",
				},
				OnPass: &ActionConfig{
					UploadEvidence: true,
				},
				OnFail: &ActionConfig{
					UploadEvidence: true,
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
