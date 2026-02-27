package storage

import "errors"

var (
	// ErrAPIURLRequired is returned when the API URL is missing from the storage configuration
	ErrAPIURLRequired = errors.New("API URL is required for API storage")
	// ErrAPITokenRequired is returned when the API token is missing from the storage configuration
	ErrAPITokenRequired = errors.New("API token is required for API storage")
)
