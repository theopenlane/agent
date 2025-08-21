package clicommand

import "errors"

var (
	// ErrConfigFileNotFound is returned when configuration file is not found
	ErrConfigFileNotFound = errors.New("configuration file not found")
	// ErrFailedToLoadConfig is returned when configuration loading fails
	ErrFailedToLoadConfig = errors.New("failed to load configuration")
	// ErrFailedToCreateDataDir is returned when data directory creation fails
	ErrFailedToCreateDataDir = errors.New("failed to create data directory")
	// ErrAgentRegistrationFailed is returned when agent registration fails
	ErrAgentRegistrationFailed = errors.New("agent registration failed")
	// ErrAgentFailedToStart is returned when agent fails to start
	ErrAgentFailedToStart = errors.New("agent failed to start")
	// ErrFailedToReadPIDFile is returned when PID file reading fails
	ErrFailedToReadPIDFile = errors.New("failed to read PID file")
	// ErrInvalidPID is returned when PID is invalid
	ErrInvalidPID = errors.New("invalid PID")
	// ErrFailedToFindProcess is returned when process finding fails
	ErrFailedToFindProcess = errors.New("failed to find process")
	// ErrFailedToSendSIGTERM is returned when SIGTERM sending fails
	ErrFailedToSendSIGTERM = errors.New("failed to send SIGTERM")
	// ErrFailedToKillProcess is returned when process killing fails
	ErrFailedToKillProcess = errors.New("failed to kill process")
	// ErrFailedToWritePIDFile is returned when PID file writing fails
	ErrFailedToWritePIDFile = errors.New("failed to write PID file")
	// ErrCheckNotFound is returned when check is not found
	ErrCheckNotFound = errors.New("check not found")
	// ErrFailedToCreateStorage is returned when storage creation fails
	ErrFailedToCreateStorage = errors.New("failed to create storage")
	// ErrCheckExecutionFailed is returned when check execution fails
	ErrCheckExecutionFailed = errors.New("check execution failed")
	// ErrFailedToCreateDirectory is returned when directory creation fails
	ErrFailedToCreateDirectory = errors.New("failed to create directory")
	// ErrFailedToSaveConfig is returned when configuration saving fails
	ErrFailedToSaveConfig = errors.New("failed to save configuration")
	// ErrFailedToMarshalConfig is returned when configuration marshaling fails
	ErrFailedToMarshalConfig = errors.New("failed to marshal configuration")
	// ErrFailedToEncodeConfig is returned when configuration encoding fails
	ErrFailedToEncodeConfig = errors.New("failed to encode configuration")
	// ErrUnsupportedFormat is returned when format is unsupported
	ErrUnsupportedFormat = errors.New("unsupported format")
	// ErrAgentAlreadyRunning is returned when agent is already running
	ErrAgentAlreadyRunning = errors.New("agent already running")
	// ErrConfigFileExists is returned when configuration file already exists
	ErrConfigFileExists = errors.New("configuration file already exists")
	// ErrSchemaGenerationFailed is returned when schema generation fails
	ErrSchemaGenerationFailed = errors.New("failed to generate schema")
	// ErrInvalidOutputFormat is returned when output format is invalid
	ErrInvalidOutputFormat = errors.New("invalid output format")
)
