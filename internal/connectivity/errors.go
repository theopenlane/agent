package connectivity

import "errors"

// ErrAPIUnsuccessfulStatusCode is returned when API returns unsuccessful status code
var ErrAPIUnsuccessfulStatusCode = errors.New("API returned unsuccessful status code")
