package storage

import (
	"time"

	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/models"
)

// Result wraps a result with storage metadata for buffering
type Result struct {
	ID         string                `json:"id"`
	Result     *config.Result        `json:"result"`
	BufferedAt time.Time             `json:"bufferedAt"`
	RetryCount int                   `json:"retryCount"`
	LastError  string                `json:"lastError,omitempty"`
	Evidence   []models.EvidenceFile `json:"evidence,omitempty"`
}

// Storage is the unified interface for storing compliance results and evidence
type Storage interface {
	// StoreResult stores a compliance check result
	StoreResult(result *config.Result) error

	// StoreResultWithEvidence stores a result along with collected evidence
	StoreResultWithEvidence(result *config.Result, evidence []models.EvidenceFile) error

	// Health returns the current health status
	Health() error

	// Close gracefully shuts down the storage system
	Close() error
}

// Option configures storage behavior
type Option func(*Config)

// Config contains all storage configuration
type Config struct {
	// API configuration
	APIURL            string
	RegistrationToken string

	// Storage directories
	DataDir   string
	BufferDir string

	// Buffering configuration
	MaxRetries            int
	RetryBackoff          time.Duration
	BufferRetentionPeriod time.Duration
	SyncInterval          time.Duration

	// Evidence configuration
	EvidenceEnabled         bool
	EvidenceRetentionPeriod time.Duration
	EvidenceMaxFileSize     int64

	// Connectivity configuration
	ConnectivityCheckURL string
	ConnectivityInterval time.Duration

	// JobResult configuration
	DefaultScheduledJobID string
	OwnerID               string
	AgentID               string
}

// Storage option constructors

// WithAPIConfig configures API connection
func WithAPIConfig(apiURL, token string) Option {
	return func(c *Config) {
		c.APIURL = apiURL
		c.RegistrationToken = token
	}
}

// WithBuffering configures local buffering behavior
func WithBuffering(bufferDir string) Option {
	return func(c *Config) {
		c.BufferDir = bufferDir
		c.MaxRetries = 5 // nolint:mnd
		c.RetryBackoff = time.Minute
		c.BufferRetentionPeriod = 7 * 24 * time.Hour // nolint:mnd
		c.SyncInterval = 5 * time.Minute             // nolint:mnd
	}
}

// WithEvidence enables evidence collection
func WithEvidence(enabled bool, retentionPeriod time.Duration, maxFileSize int64) Option {
	return func(c *Config) {
		c.EvidenceEnabled = enabled
		c.EvidenceRetentionPeriod = retentionPeriod
		c.EvidenceMaxFileSize = maxFileSize
	}
}

// WithConnectivityMonitoring configures connectivity checking for buffered mode
func WithConnectivityMonitoring(checkURL string, interval time.Duration) Option {
	return func(c *Config) {
		c.ConnectivityCheckURL = checkURL
		c.ConnectivityInterval = interval
	}
}

// WithRetryPolicy configures retry behavior
func WithRetryPolicy(maxRetries int, backoff time.Duration) Option {
	return func(c *Config) {
		c.MaxRetries = maxRetries
		c.RetryBackoff = backoff
	}
}

// WithJobResultConfig configures JobResult creation
func WithJobResultConfig(scheduledJobID, ownerID, agentID string) Option {
	return func(c *Config) {
		c.DefaultScheduledJobID = scheduledJobID
		c.OwnerID = ownerID
		c.AgentID = agentID
	}
}

// NewStorage creates a new storage instance
func NewStorage(opts ...Option) (Storage, error) {
	config := &Config{
		DataDir:   "./data",
		BufferDir: "./buffer",
		// Default buffering configuration
		MaxRetries:            5, // nolint:mnd
		RetryBackoff:          time.Minute,
		BufferRetentionPeriod: 7 * 24 * time.Hour, // nolint:mnd
		SyncInterval:          5 * time.Minute,    // nolint:mnd
	}

	for _, opt := range opts {
		opt(config)
	}

	return NewBufferedStorage(config)
}
