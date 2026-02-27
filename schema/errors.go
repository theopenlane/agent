package schema

import "errors"

var (
	// ErrCheckNameRequired is returned when checkName is required but missing
	ErrCheckNameRequired = errors.New("checkName is required")
	// ErrStandardRequired is returned when standard is required but missing
	ErrStandardRequired = errors.New("standard is required")
	// ErrControlRefRequired is returned when controlRef is required but missing
	ErrControlRefRequired = errors.New("controlRef is required")
	// ErrStatusRequired is returned when status is required but missing
	ErrStatusRequired = errors.New("status is required")
	// ErrInvalidFindings is returned when findings format is invalid
	ErrInvalidFindings = errors.New("invalid findings format")
	// ErrStartedAtRequired is returned when startedAt is required but missing
	ErrStartedAtRequired = errors.New("startedAt is required")
	// ErrFinishedAtRequired is returned when finishedAt is required but missing
	ErrFinishedAtRequired = errors.New("finishedAt is required")
	// ErrInvalidStatus is returned when status value is invalid
	ErrInvalidStatus = errors.New("status must be one of: SUCCESS, FAILED, CANCELED, PENDING")
)
