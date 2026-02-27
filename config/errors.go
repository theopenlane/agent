package config

import "errors"

var (
	// ErrInvalidCronSchedule is returned when a cron expression is invalid
	ErrInvalidCronSchedule = errors.New("invalid cron schedule")
	// ErrInvalidOperationMode is returned when an unsupported operation mode is specified
	ErrInvalidOperationMode = errors.New("invalid operation mode")
	// ErrFailedToGenerateJSONSchema is returned when JSON schema generation fails
	ErrFailedToGenerateJSONSchema = errors.New("failed to generate JSON schema")
	// ErrFailedToMarshalConfig is returned when the configuration cannot be marshaled
	ErrFailedToMarshalConfig = errors.New("failed to marshal configuration")
	// ErrFailedToMarshalSchema is returned when the schema cannot be marshaled
	ErrFailedToMarshalSchema = errors.New("failed to marshal schema")
	// ErrFailedToAddSchemaResource is returned when a schema resource cannot be added to the compiler
	ErrFailedToAddSchemaResource = errors.New("failed to add schema resource")
	// ErrFailedToCompileSchema is returned when the schema cannot be compiled
	ErrFailedToCompileSchema = errors.New("failed to compile schema")
	// ErrFailedToUnmarshalConfig is returned when the configuration cannot be unmarshaled from JSON
	ErrFailedToUnmarshalConfig = errors.New("failed to unmarshal configuration")
	// ErrSchemaValidationFailed is returned when configuration fails schema validation
	ErrSchemaValidationFailed = errors.New("schema validation failed")
	// ErrOutputDirRequired is returned when the output directory is not specified in standalone mode
	ErrOutputDirRequired = errors.New("output directory is required")
	// ErrFailedToCreateOutputDir is returned when the output directory cannot be created
	ErrFailedToCreateOutputDir = errors.New("failed to create output directory")
	// ErrAPIURLRequired is returned when the API URL is not provided for non-standalone modes
	ErrAPIURLRequired = errors.New("API URL is required")
	// ErrAPITokenRequired is returned when the API token is not provided for non-standalone modes
	ErrAPITokenRequired = errors.New("API token is required")
	// ErrBufferDirRequired is returned when the buffer directory is not specified in buffered mode
	ErrBufferDirRequired = errors.New("buffer directory is required")
	// ErrFailedToCreateBufferDir is returned when the buffer directory cannot be created
	ErrFailedToCreateBufferDir = errors.New("failed to create buffer directory")
	// ErrDataDirRequired is returned when the data directory is not specified
	ErrDataDirRequired = errors.New("data directory is required")
	// ErrFailedToCreateDataDir is returned when the data directory cannot be created
	ErrFailedToCreateDataDir = errors.New("failed to create data directory")
	// ErrDuplicateCheckName is returned when two checks share the same name
	ErrDuplicateCheckName = errors.New("duplicate check name")
	// ErrCheckNameRequired is returned when a check has no name
	ErrCheckNameRequired = errors.New("check name is required")
	// ErrCheckCommandRequired is returned when a check has no command
	ErrCheckCommandRequired = errors.New("check command is required")
	// ErrCheckScheduleRequired is returned when a check has no schedule
	ErrCheckScheduleRequired = errors.New("check schedule is required")
	// ErrInvalidEvidencePath is returned when an evidence path is not a valid relative or absolute path
	ErrInvalidEvidencePath = errors.New("invalid evidence path")
	// ErrActionCommandNameRequired is returned when an action command has no name
	ErrActionCommandNameRequired = errors.New("action command name is required")
	// ErrActionCommandRequired is returned when an action command has no command string
	ErrActionCommandRequired = errors.New("action command is required")
	// ErrDuplicateActionCommandName is returned when two action commands share the same name
	ErrDuplicateActionCommandName = errors.New("duplicate action command name")
	// ErrFailedToCreateConfigDir is returned when the configuration directory cannot be created
	ErrFailedToCreateConfigDir = errors.New("failed to create config directory")
	// ErrFailedToWriteConfigFile is returned when the configuration file cannot be written
	ErrFailedToWriteConfigFile = errors.New("failed to write config file")
	// ErrCheckNotFound is returned when a check with the given name does not exist
	ErrCheckNotFound = errors.New("check not found")
	// ErrFailedToLoadConfigFile is returned when the configuration file cannot be loaded
	ErrFailedToLoadConfigFile = errors.New("failed to load config file")
	// ErrFailedToLoadEnvironmentVariables is returned when environment variables cannot be loaded
	ErrFailedToLoadEnvironmentVariables = errors.New("failed to load environment variables")
	// ErrFailedToUnmarshalConfigFile is returned when the configuration file cannot be unmarshaled
	ErrFailedToUnmarshalConfigFile = errors.New("failed to unmarshal config")
	// ErrConfigurationValidationFailed is returned when the configuration fails validation
	ErrConfigurationValidationFailed = errors.New("configuration validation failed")
	// ErrCheckValidationFailed is returned when a check entry fails validation
	ErrCheckValidationFailed = errors.New("check validation failed")
)
