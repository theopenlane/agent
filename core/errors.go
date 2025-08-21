package core

import "errors"

var (
	// ErrFailedToCreateAPIClient is returned when API client creation fails
	ErrFailedToCreateAPIClient = errors.New("failed to create API client")
	// ErrControlSyncServiceNotInitialized is returned when control sync service is not initialized
	ErrControlSyncServiceNotInitialized = errors.New("control sync service not initialized")
	// ErrControlNotFound is returned when control is not found
	ErrControlNotFound = errors.New("control not found")
	// ErrFailedToCreateControl is returned when control creation fails
	ErrFailedToCreateControl = errors.New("failed to create control")
	// ErrFailedToUpdateControl is returned when control update fails
	ErrFailedToUpdateControl = errors.New("failed to update control")
	// ErrInvalidControlReference is returned when control reference is invalid
	ErrInvalidControlReference = errors.New("invalid control reference")
	// ErrCheckExecutionTimeout is returned when check execution times out
	ErrCheckExecutionTimeout = errors.New("check execution timeout")
	// ErrInvalidExitCode is returned when exit code is invalid
	ErrInvalidExitCode = errors.New("invalid exit code")
	// ErrInvalidCommand is returned when command is invalid
	ErrInvalidCommand = errors.New("invalid command")
	// ErrCheckNotExecutable is returned when check is not executable
	ErrCheckNotExecutable = errors.New("check not executable")
	// ErrFailedToParseOutput is returned when output parsing fails
	ErrFailedToParseOutput = errors.New("failed to parse output")
	// ErrFailedToProcessEvidence is returned when evidence processing fails
	ErrFailedToProcessEvidence = errors.New("failed to process evidence")
	// ErrFailedToUploadEvidence is returned when evidence upload fails
	ErrFailedToUploadEvidence = errors.New("failed to upload evidence")
	// ErrFailedToUpdateControlStatus is returned when control status update fails
	ErrFailedToUpdateControlStatus = errors.New("failed to update control status")
	// ErrFailedToExecuteAction is returned when action execution fails
	ErrFailedToExecuteAction = errors.New("failed to execute action")
	// ErrFailedToExecuteCommand is returned when command execution fails
	ErrFailedToExecuteCommand = errors.New("failed to execute command")
	// ErrCommandNotFound is returned when command is not found
	ErrCommandNotFound = errors.New("command not found")
	// ErrInvalidRegex is returned when regex pattern is invalid
	ErrInvalidRegex = errors.New("invalid regex pattern")
	// ErrCheckNotSupportedOnPlatform is returned when check is not supported on current platform
	ErrCheckNotSupportedOnPlatform = errors.New("check not supported on current platform")
	// ErrFailedToReadFile is returned when file reading fails
	ErrFailedToReadFile = errors.New("failed to read file")
	// ErrAPIClientNotAvailable is returned when API client is not available
	ErrAPIClientNotAvailable = errors.New("API client not available")
	// ErrAgentNotRegistered is returned when agent is not registered
	ErrAgentNotRegistered = errors.New("agent not registered")
	// ErrAgentRegistrationFailed is returned when agent registration fails
	ErrAgentRegistrationFailed = errors.New("agent registration failed")
	// ErrFailedToPollForWork is returned when polling for work fails
	ErrFailedToPollForWork = errors.New("failed to poll for work")
	// ErrNoStorageSystemAvailable is returned when no storage system is available
	ErrNoStorageSystemAvailable = errors.New("no storage system available")
	// ErrFailedToSendHeartbeat is returned when heartbeat sending fails
	ErrFailedToSendHeartbeat = errors.New("failed to send heartbeat")
	// ErrFailedToFetchControls is returned when controls fetching fails
	ErrFailedToFetchControls = errors.New("failed to fetch controls")
	// ErrFailedToFetchOpenlaneControls is returned when Openlane controls fetching fails
	ErrFailedToFetchOpenlaneControls = errors.New("failed to fetch Openlane controls")
	// ErrFailedToGetControlsForStatus is returned when getting controls for status fails
	ErrFailedToGetControlsForStatus = errors.New("failed to get controls for status")
)
