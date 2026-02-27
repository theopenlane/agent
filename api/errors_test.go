package api

import (
	"errors"
	"testing"

	clientv2 "github.com/gqlgo/gqlgenc/clientv2"
	"github.com/stretchr/testify/assert"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// newGQLError constructs a clientv2.ErrorResponse with the given extension codes for testing
func newGQLError(codes ...string) *clientv2.ErrorResponse {
	errs := make(gqlerror.List, len(codes))
	for i, code := range codes {
		errs[i] = &gqlerror.Error{
			Extensions: map[string]any{"code": code},
		}
	}

	return &clientv2.ErrorResponse{GqlErrors: &errs}
}

func TestHasAnyErrorCode(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		codes  []string
		expect bool
	}{
		{
			name:   "nil error returns false",
			err:    nil,
			codes:  []string{"NOT_FOUND"},
			expect: false,
		},
		{
			name:   "non-gql error returns false",
			err:    errors.New("plain error"),
			codes:  []string{"NOT_FOUND"},
			expect: false,
		},
		{
			name:   "matching code returns true",
			err:    newGQLError("NOT_FOUND"),
			codes:  []string{"NOT_FOUND"},
			expect: true,
		},
		{
			name:   "non-matching code returns false",
			err:    newGQLError("FORBIDDEN"),
			codes:  []string{"NOT_FOUND"},
			expect: false,
		},
		{
			name:   "one of multiple codes matches",
			err:    newGQLError("ALREADY_EXISTS"),
			codes:  []string{"NOT_FOUND", "ALREADY_EXISTS"},
			expect: true,
		},
		{
			name:   "empty codes returns false",
			err:    newGQLError("NOT_FOUND"),
			codes:  []string{},
			expect: false,
		},
		{
			name:   "gql error with nil GqlErrors returns false",
			err:    &clientv2.ErrorResponse{GqlErrors: nil},
			codes:  []string{"NOT_FOUND"},
			expect: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, HasAnyErrorCode(tt.err, tt.codes...))
		})
	}
}

func TestIsAlreadyExistsError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{"nil returns false", nil, false},
		{"plain error returns false", errors.New("some other error"), false},
		{"ALREADY_EXISTS code", newGQLError("ALREADY_EXISTS"), true},
		{"USER_EXISTS code", newGQLError("USER_EXISTS"), true},
		{"string contains already exists", errors.New("resource already exists in database"), true},
		{"unrelated gql error", newGQLError("FORBIDDEN"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, IsAlreadyExistsError(tt.err))
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{"nil returns false", nil, false},
		{"plain error returns false", errors.New("missing"), false},
		{"NOT_FOUND code", newGQLError("NOT_FOUND"), true},
		{"FORBIDDEN code returns false", newGQLError("FORBIDDEN"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, IsNotFoundError(tt.err))
		})
	}
}

func TestIsConflictError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{"nil returns false", nil, false},
		{"CONFLICT code", newGQLError("CONFLICT"), true},
		{"NOT_FOUND code returns false", newGQLError("NOT_FOUND"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, IsConflictError(tt.err))
		})
	}
}

func TestIsForbiddenError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{"nil returns false", nil, false},
		{"FORBIDDEN code", newGQLError("FORBIDDEN"), true},
		{"PERMISSION_DENIED code", newGQLError("PERMISSION_DENIED"), true},
		{"NOT_FOUND code returns false", newGQLError("NOT_FOUND"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, IsForbiddenError(tt.err))
		})
	}
}

func TestIsModuleAccessError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expect bool
	}{
		{"nil returns false", nil, false},
		{"MODULE_NO_ACCESS code", newGQLError("MODULE_NO_ACCESS"), true},
		{"string contains MODULE_NO_ACCESS", errors.New("error: MODULE_NO_ACCESS"), true},
		{"string contains feature not enabled", errors.New("feature not enabled for organization"), true},
		{"unrelated error returns false", errors.New("something else"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, IsModuleAccessError(tt.err))
		})
	}
}
