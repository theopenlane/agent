package clicommand

import "errors"

var (
	// ErrConfigFileNotFound is returned when the configuration file path does not exist
	ErrConfigFileNotFound = errors.New("configuration file not found")
	// ErrFailedToLoadConfig is returned when the configuration file cannot be loaded or parsed
	ErrFailedToLoadConfig = errors.New("failed to load configuration")
	// ErrFailedToCreateDataDir is returned when the agent data directory cannot be created
	ErrFailedToCreateDataDir = errors.New("failed to create data directory")
	// ErrFailedToReadPIDFile is returned when the PID file cannot be read
	ErrFailedToReadPIDFile = errors.New("failed to read PID file")
	// ErrInvalidPID is returned when the PID file contains a non-integer value
	ErrInvalidPID = errors.New("invalid PID")
	// ErrFailedToFindProcess is returned when the OS cannot locate a process by PID
	ErrFailedToFindProcess = errors.New("failed to find process")
	// ErrFailedToSendSIGTERM is returned when SIGTERM cannot be delivered to the agent process
	ErrFailedToSendSIGTERM = errors.New("failed to send SIGTERM")
	// ErrFailedToKillProcess is returned when the agent process cannot be forcefully terminated
	ErrFailedToKillProcess = errors.New("failed to kill process")
	// ErrFailedToWritePIDFile is returned when the PID file cannot be written
	ErrFailedToWritePIDFile = errors.New("failed to write PID file")
	// ErrCheckNotFound is returned when the requested check name does not exist in the configuration
	ErrCheckNotFound = errors.New("check not found")
	// ErrFailedToCreateDirectory is returned when a required directory cannot be created
	ErrFailedToCreateDirectory = errors.New("failed to create directory")
	// ErrFailedToSaveConfig is returned when the configuration cannot be written to disk
	ErrFailedToSaveConfig = errors.New("failed to save configuration")
	// ErrFailedToMarshalConfig is returned when the configuration cannot be serialized
	ErrFailedToMarshalConfig = errors.New("failed to marshal configuration")
	// ErrFailedToEncodeConfig is returned when the configuration cannot be JSON-encoded
	ErrFailedToEncodeConfig = errors.New("failed to encode configuration")
	// ErrUnsupportedFormat is returned when an output format other than yaml or json is requested
	ErrUnsupportedFormat = errors.New("unsupported format")
	// ErrAgentAlreadyRunning is returned when a PID file indicates the agent is already active
	ErrAgentAlreadyRunning = errors.New("agent already running")
	// ErrConfigFileExists is returned when an init would overwrite an existing configuration file without --force
	ErrConfigFileExists = errors.New("configuration file already exists")
)
