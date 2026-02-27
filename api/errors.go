package api //nolint:revive

import (
	"errors"
	"slices"
	"strings"

	clientv2 "github.com/gqlgo/gqlgenc/clientv2"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

var (
	// ErrBaseURLRequired is returned when the base URL is not provided
	ErrBaseURLRequired = errors.New("base URL is required")
	// ErrAPITokenRequired is returned when the API token is not provided
	ErrAPITokenRequired = errors.New("API token is required")
	// ErrEvidenceCreationFailed is returned when evidence creation fails
	ErrEvidenceCreationFailed = errors.New("failed to create evidence")
	// ErrStandardNotFound is returned when a compliance standard is not found in the API
	ErrStandardNotFound = errors.New("compliance standard not found")
	// ErrControlNotFound is returned when a compliance control is not found in the API
	ErrControlNotFound = errors.New("compliance control not found")
	// ErrClientCreationFailed is returned when the underlying API client cannot be created
	ErrClientCreationFailed = errors.New("failed to create openlane client")
	// ErrHealthCheckFailed is returned when the API health check does not succeed
	ErrHealthCheckFailed = errors.New("api health check failed")
	// ErrControlIDResolutionFailed is returned when no control IDs could be resolved
	ErrControlIDResolutionFailed = errors.New("failed to resolve control IDs")
)

// HasAnyErrorCode checks whether a GraphQL error response contains any of the given extension codes
func HasAnyErrorCode(err error, codes ...string) bool {
	var gqlErr *clientv2.ErrorResponse
	if !errors.As(err, &gqlErr) {
		return false
	}

	if gqlErr.GqlErrors == nil {
		return false
	}

	return slices.ContainsFunc(*gqlErr.GqlErrors, func(e *gqlerror.Error) bool {
		code, ok := e.Extensions["code"].(string)
		return ok && slices.Contains(codes, code)
	})
}

// IsAlreadyExistsError returns true when the API reports a resource already exists
func IsAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}

	if HasAnyErrorCode(err, "ALREADY_EXISTS", "USER_EXISTS") {
		return true
	}

	return strings.Contains(err.Error(), "already exists")
}

// IsNotFoundError returns true when the API reports a resource was not found
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}

	return HasAnyErrorCode(err, "NOT_FOUND")
}

// IsConflictError returns true when the API reports a resource conflict
func IsConflictError(err error) bool {
	if err == nil {
		return false
	}

	return HasAnyErrorCode(err, "CONFLICT")
}

// IsForbiddenError returns true when the API reports a permission denied condition
func IsForbiddenError(err error) bool {
	if err == nil {
		return false
	}

	return HasAnyErrorCode(err, "FORBIDDEN", "PERMISSION_DENIED")
}

// IsModuleAccessError returns true when the API reports that a module or feature is not accessible
func IsModuleAccessError(err error) bool {
	if err == nil {
		return false
	}

	if HasAnyErrorCode(err, "MODULE_NO_ACCESS") {
		return true
	}

	msg := err.Error()

	return strings.Contains(msg, "MODULE_NO_ACCESS") || strings.Contains(msg, "feature not enabled for organization")
}
