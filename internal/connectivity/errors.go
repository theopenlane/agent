package connectivity

import "errors"

var (
	// ErrAPIUnsuccessfulStatusCode is returned when API returns unsuccessful status code.
	ErrAPIUnsuccessfulStatusCode = errors.New("API returned unsuccessful status code")
	// ErrAPIURLRequired is returned when the manager is configured without an API URL.
	ErrAPIURLRequired = errors.New("api url is required")
	// ErrInvalidAPIURL is returned when the configured API URL is invalid.
	ErrInvalidAPIURL = errors.New("invalid api url")
)
