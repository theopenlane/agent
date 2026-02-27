package storage

import "errors"

var (
	// ErrAPIURLRequired is returned when API URL is required for API storage
	ErrAPIURLRequired = errors.New("API URL is required for API storage")
	// ErrRegistrationTokenRequired is returned when registration token is required for API storage
	ErrRegistrationTokenRequired = errors.New("registration token is required for API storage")
	// ErrJobResultSyncFailed is returned when job result synchronization fails
	ErrJobResultSyncFailed = errors.New("failed to sync job result to core system")
	// ErrFileUploadFailed is returned when file upload fails
	ErrFileUploadFailed = errors.New("failed to upload file to core system")
	// ErrMissingRequiredField is returned when a required field is missing
	ErrMissingRequiredField = errors.New("missing required field")
	// ErrComplianceResultNil is returned when compliance result is nil
	ErrComplianceResultNil = errors.New("compliance result is nil")
)
