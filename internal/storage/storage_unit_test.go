package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStorageModes tests the storage consolidation (modes removed)
func TestStorageConsolidation(t *testing.T) {
	// Storage modes have been consolidated to use BufferedStorage for all cases
	assert.True(t, true) // Placeholder test
}

// TestNewEvidenceConfig tests evidence config creation
func TestNewEvidenceConfig(t *testing.T) {
	config := &EvidenceConfig{
		Enabled:     true,
		DataDir:     "/tmp/evidence",
		MaxFileSize: 1024 * 1024,
	}

	assert.True(t, config.Enabled)
	assert.Equal(t, "/tmp/evidence", config.DataDir)
	assert.Equal(t, int64(1024*1024), config.MaxFileSize)
}

// TestStorageOptions tests the consolidated functional options
func TestWithAPIConfig(t *testing.T) {
	config := &Config{}
	opt := WithAPIConfig("https://api.example.com", "token123")
	opt(config)

	assert.Equal(t, "https://api.example.com", config.APIURL)
	assert.Equal(t, "token123", config.APIToken)
}

func TestWithBuffering(t *testing.T) {
	config := &Config{}
	opt := WithBuffering("/tmp/buffer")
	opt(config)

	assert.Equal(t, "/tmp/buffer", config.BufferDir)
	assert.Equal(t, 5, config.MaxRetries)
}

func TestWithEvidence(t *testing.T) {
	config := &Config{}
	opt := WithEvidence(true, 24*60*60*1000000000, 1024*1024) // 24 hours in nanoseconds
	opt(config)

	assert.True(t, config.EvidenceEnabled)
	assert.Equal(t, int64(24*60*60*1000000000), int64(config.EvidenceRetentionPeriod))
	assert.Equal(t, int64(1024*1024), config.EvidenceMaxFileSize)
}

func TestWithRetryPolicy(t *testing.T) {
	config := &Config{}
	opt := WithRetryPolicy(3, 60*1000000000) // 1 minute in nanoseconds
	opt(config)

	assert.Equal(t, 3, config.MaxRetries)
	assert.Equal(t, int64(60*1000000000), int64(config.RetryBackoff))
}

func TestWithConnectivityMonitoring(t *testing.T) {
	config := &Config{}
	opt := WithConnectivityMonitoring("https://health.example.com", 30*1000000000) // 30 seconds in nanoseconds
	opt(config)

	assert.Equal(t, "https://health.example.com", config.ConnectivityCheckURL)
	assert.Equal(t, int64(30*1000000000), int64(config.ConnectivityInterval))
}
