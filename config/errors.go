package config

import "errors"

var (
	// ErrInvalidCronSchedule is returned when cron schedule is invalid
	ErrInvalidCronSchedule = errors.New("invalid cron schedule")
	// ErrCommandRequired is returned when command is required but not provided
	ErrCommandRequired = errors.New("command is required")
	// ErrNameRequired is returned when name is required but not provided
	ErrNameRequired = errors.New("name is required")
	// ErrScheduleRequired is returned when schedule is required but not provided
	ErrScheduleRequired = errors.New("schedule is required")
	// ErrInvalidTimeout is returned when timeout is invalid
	ErrInvalidTimeout = errors.New("invalid timeout")
	// ErrInvalidConcurrency is returned when concurrency setting is invalid
	ErrInvalidConcurrency = errors.New("invalid concurrency setting")
	// ErrInvalidRetentionPeriod is returned when retention period is invalid
	ErrInvalidRetentionPeriod = errors.New("invalid retention period")
	// ErrInvalidMaxFileSize is returned when max file size is invalid
	ErrInvalidMaxFileSize = errors.New("invalid max file size")
	// ErrUnknownOperationMode is returned when operation mode is unknown
	ErrUnknownOperationMode = errors.New("unknown operation mode")
	// ErrInvalidPollInterval is returned when poll interval is invalid
	ErrInvalidPollInterval = errors.New("invalid poll interval")
	// ErrFailedToSaveConfig is returned when configuration saving fails
	ErrFailedToSaveConfig = errors.New("failed to save configuration")
	// ErrFailedToLoadConfig is returned when configuration loading fails
	ErrFailedToLoadConfig = errors.New("failed to load configuration")
	// ErrFailedToGenerateJSONSchema is returned when JSON schema generation fails
	ErrFailedToGenerateJSONSchema = errors.New("failed to generate JSON schema")
	// ErrFailedToMarshalConfig is returned when configuration marshaling fails
	ErrFailedToMarshalConfig = errors.New("failed to marshal configuration")
	// ErrFailedToMarshalSchema is returned when schema marshaling fails
	ErrFailedToMarshalSchema = errors.New("failed to marshal schema")
	// ErrFailedToAddSchemaResource is returned when adding schema resource fails
	ErrFailedToAddSchemaResource = errors.New("failed to add schema resource")
	// ErrFailedToCompileSchema is returned when schema compilation fails
	ErrFailedToCompileSchema = errors.New("failed to compile schema")
	// ErrFailedToUnmarshalConfig is returned when configuration unmarshaling fails
	ErrFailedToUnmarshalConfig = errors.New("failed to unmarshal configuration")
	// ErrSchemaValidationFailed is returned when schema validation fails
	ErrSchemaValidationFailed = errors.New("schema validation failed")
	// ErrBusinessRuleValidationFailed is returned when business rule validation fails
	ErrBusinessRuleValidationFailed = errors.New("business rule validation failed")
	// ErrInvalidOperationMode is returned when operation mode is invalid
	ErrInvalidOperationMode = errors.New("invalid operation mode")
	// ErrOutputDirRequired is returned when output directory is required
	ErrOutputDirRequired = errors.New("output directory is required")
	// ErrFailedToCreateOutputDir is returned when output directory creation fails
	ErrFailedToCreateOutputDir = errors.New("failed to create output directory")
	// ErrAPIURLRequired is returned when API URL is required
	ErrAPIURLRequired = errors.New("API URL is required")
	// ErrRegistrationTokenRequired is returned when registration token is required
	ErrRegistrationTokenRequired = errors.New("registration token is required")
	// ErrBufferDirRequired is returned when buffer directory is required
	ErrBufferDirRequired = errors.New("buffer directory is required")
	// ErrFailedToCreateBufferDir is returned when buffer directory creation fails
	ErrFailedToCreateBufferDir = errors.New("failed to create buffer directory")
	// ErrDataDirRequired is returned when data directory is required
	ErrDataDirRequired = errors.New("data directory is required")
	// ErrFailedToCreateDataDir is returned when data directory creation fails
	ErrFailedToCreateDataDir = errors.New("failed to create data directory")
	// ErrDuplicateCheckName is returned when check name is duplicated
	ErrDuplicateCheckName = errors.New("duplicate check name")
	// ErrCheckNameRequired is returned when check name is required
	ErrCheckNameRequired = errors.New("check name is required")
	// ErrCheckCommandRequired is returned when check command is required
	ErrCheckCommandRequired = errors.New("check command is required")
	// ErrCheckScheduleRequired is returned when check schedule is required
	ErrCheckScheduleRequired = errors.New("check schedule is required")
	// ErrInvalidEvidencePath is returned when evidence path is invalid
	ErrInvalidEvidencePath = errors.New("invalid evidence path")
	// ErrActionCommandNameRequired is returned when action command name is required
	ErrActionCommandNameRequired = errors.New("action command name is required")
	// ErrActionCommandRequired is returned when action command is required
	ErrActionCommandRequired = errors.New("action command is required")
	// ErrDuplicateActionCommandName is returned when action command name is duplicated
	ErrDuplicateActionCommandName = errors.New("duplicate action command name")
	// ErrFailedToCreateConfigDir is returned when config directory creation fails
	ErrFailedToCreateConfigDir = errors.New("failed to create config directory")
	// ErrFailedToWriteConfigFile is returned when config file writing fails
	ErrFailedToWriteConfigFile = errors.New("failed to write config file")
	// ErrCheckNotFound is returned when check is not found
	ErrCheckNotFound = errors.New("check not found")
	// ErrFailedToLoadConfigFile is returned when config file loading fails
	ErrFailedToLoadConfigFile = errors.New("failed to load config file")
	// ErrFailedToLoadEnvironmentVariables is returned when environment variable loading fails
	ErrFailedToLoadEnvironmentVariables = errors.New("failed to load environment variables")
	// ErrFailedToUnmarshalConfigFile is returned when config unmarshaling fails
	ErrFailedToUnmarshalConfigFile = errors.New("failed to unmarshal config")
	// ErrConfigurationValidationFailed is returned when configuration validation fails
	ErrConfigurationValidationFailed = errors.New("configuration validation failed")
	// ErrCheckValidationFailed is returned when check validation fails
	ErrCheckValidationFailed = errors.New("check validation failed")
)
