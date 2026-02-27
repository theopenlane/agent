package api

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGraphQLClient_validation(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		apiToken  string
		expectErr error
	}{
		{
			name:      "empty base URL returns error",
			baseURL:   "",
			apiToken:  "token",
			expectErr: ErrBaseURLRequired,
		},
		{
			name:      "whitespace base URL returns error",
			baseURL:   "   ",
			apiToken:  "token",
			expectErr: ErrBaseURLRequired,
		},
		{
			name:      "empty token returns error",
			baseURL:   "https://api.example.com",
			apiToken:  "",
			expectErr: ErrAPITokenRequired,
		},
		{
			name:      "whitespace token returns error",
			baseURL:   "https://api.example.com",
			apiToken:  "   ",
			expectErr: ErrAPITokenRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewGraphQLClient(tt.baseURL, tt.apiToken)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.expectErr)
			assert.Nil(t, client)
		})
	}
}

func TestNewGraphQLClientWithOrgID_validation(t *testing.T) {
	_, err := NewGraphQLClientWithOrgID("", "token", "org-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBaseURLRequired)

	_, err = NewGraphQLClientWithOrgID("https://api.example.com", "", "org-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAPITokenRequired)
}

func TestNewGraphQLClientWithRetry_orgIDInterceptor(t *testing.T) {
	// A valid base URL and token with no org ID should succeed
	client, err := NewGraphQLClientWithRetry("https://localhost:8080", "test-token", "", nil)
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.NotNil(t, client.cache)

	// Providing an org ID should also succeed
	clientWithOrg, err := NewGraphQLClientWithRetry("https://localhost:8080", "test-token", "org-abc", nil)
	require.NoError(t, err)
	require.NotNil(t, clientWithOrg)
}

func TestControlCache(t *testing.T) {
	cache := &controlCache{entries: make(map[string]string)}

	// Miss returns empty string and false
	id, ok := cache.get("std-1", "CC6.1")
	assert.False(t, ok)
	assert.Empty(t, id)

	// Set and retrieve
	cache.set("std-1", "CC6.1", "control-uuid-1")

	id, ok = cache.get("std-1", "CC6.1")
	assert.True(t, ok)
	assert.Equal(t, "control-uuid-1", id)

	// Lookup is case-insensitive (keys are lower-cased)
	id, ok = cache.get("std-1", "cc6.1")
	assert.True(t, ok)
	assert.Equal(t, "control-uuid-1", id)

	// Different standard is a separate entry
	_, ok = cache.get("std-2", "CC6.1")
	assert.False(t, ok)
}

func TestControlsLabel(t *testing.T) {
	assert.Equal(t, "none", controlsLabel(nil))
	assert.Equal(t, "none", controlsLabel([]string{}))
	assert.Equal(t, "a", controlsLabel([]string{"a"}))
	assert.Equal(t, "a, b", controlsLabel([]string{"a", "b"}))
}
