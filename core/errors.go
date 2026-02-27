package core

import "errors"

var (
	// ErrFailedToCreateAPIClient is returned when the API client cannot be initialized
	ErrFailedToCreateAPIClient = errors.New("failed to create API client")
	// ErrCheckExecutionTimeout is returned when a check exceeds its configured timeout
	ErrCheckExecutionTimeout = errors.New("check execution timeout")
	// ErrCheckNotSupportedOnPlatform is returned when no platform variant matches the current OS
	ErrCheckNotSupportedOnPlatform = errors.New("check not supported on platform")
	// ErrFailedToExecuteAction is returned when an on-pass or on-fail action command fails
	ErrFailedToExecuteAction = errors.New("failed to execute action")
	// ErrFailedToExecuteCommand is returned when the check command itself cannot be run
	ErrFailedToExecuteCommand = errors.New("failed to execute command")
	// ErrInvalidRegex is returned when a platform variant includes or excludes pattern is not a valid regex
	ErrInvalidRegex = errors.New("invalid regex pattern")
)
