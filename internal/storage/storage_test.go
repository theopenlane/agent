package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStorageConfiguration tests basic storage configuration
func TestStorageConfiguration(t *testing.T) {
	// Test API storage configuration validation
	_, err := NewAPIStorage(&Config{})
	assert.ErrorIs(t, err, ErrAPIURLRequired)

	_, err = NewAPIStorage(&Config{
		APIURL: "https://api.example.com",
	})
	assert.ErrorIs(t, err, ErrAPITokenRequired)
}
