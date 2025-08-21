package api

import "errors"

var (
	// ErrBaseURLRequired is returned when base URL is not provided
	ErrBaseURLRequired = errors.New("base URL is required")
	// ErrAPIKeyRequired is returned when API key is not provided
	ErrAPIKeyRequired = errors.New("API key is required")
	// ErrPingRequestFailed is returned when ping request fails
	ErrPingRequestFailed = errors.New("ping request failed")
	// ErrJobResultCreationFailed is returned when job result creation fails
	ErrJobResultCreationFailed = errors.New("failed to create job result")
	// ErrAgentRegistrationFailed is returned when agent registration fails
	ErrAgentRegistrationFailed = errors.New("failed to register agent")
	// ErrAgentStatusUpdateFailed is returned when agent status update fails
	ErrAgentStatusUpdateFailed = errors.New("failed to update agent status")
	// ErrScheduledJobsRetrievalFailed is returned when scheduled jobs retrieval fails
	ErrScheduledJobsRetrievalFailed = errors.New("failed to retrieve scheduled jobs")
	// ErrAgentNotRegistered is returned when agent is not registered
	ErrAgentNotRegistered = errors.New("agent not registered")
	// ErrControlsRetrievalFailed is returned when controls retrieval fails
	ErrControlsRetrievalFailed = errors.New("failed to get controls")
	// ErrControlCreationFailed is returned when control creation fails
	ErrControlCreationFailed = errors.New("failed to create control")
	// ErrJobRunnerUpdateFailed is returned when job runner update fails
	ErrJobRunnerUpdateFailed = errors.New("failed to update job runner")
	// ErrRegistrationTokenRequired is returned when registration token is not provided
	ErrRegistrationTokenRequired = errors.New("registration token is required")
	// ErrPollForWorkFailed is returned when polling for work fails
	ErrPollForWorkFailed = errors.New("failed to poll for work")
	// ErrControlUpdateFailed is returned when control update fails
	ErrControlUpdateFailed = errors.New("failed to update control")
)
