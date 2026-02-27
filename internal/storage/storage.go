package storage

import (
	"time"

	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/models"
)

// Result wraps a result with storage metadata for buffering
type Result struct {
	// ID is the unique identifier for this buffered result entry
	ID string `json:"id"`
	// Result is the compliance check result being buffered
	Result *config.Result `json:"result"`
	// BufferedAt is the time the result was written to the local buffer
	BufferedAt time.Time `json:"bufferedAt"`
	// RetryCount is the number of upload attempts made for this result
	RetryCount int `json:"retryCount"`
	// LastError holds the error message from the most recent failed upload attempt
	LastError string `json:"lastError,omitempty"`
	// Evidence is the list of evidence files associated with this result
	Evidence []models.EvidenceFile `json:"evidence,omitempty"`
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
	// APIURL is the URL of the Openlane API endpoint
	APIURL string
	// APIToken is the authentication token for API requests
	APIToken string
	// OrgID is the organization ID used to scope API requests
	OrgID string

	// DataDir is the base directory for storing local data
	DataDir string
	// BufferDir is the directory used for buffering results when offline
	BufferDir string

	// MaxRetries is the maximum number of upload retry attempts
	MaxRetries int
	// RetryBackoff is the duration to wait between retry attempts
	RetryBackoff time.Duration
	// BufferRetentionPeriod is the duration to retain buffered results before pruning
	BufferRetentionPeriod time.Duration
	// SyncInterval is the interval at which buffered results are synced to the API
	SyncInterval time.Duration

	// EvidenceEnabled indicates whether evidence collection is active
	EvidenceEnabled bool
	// EvidenceRetentionPeriod is the duration to retain evidence files before pruning
	EvidenceRetentionPeriod time.Duration
	// EvidenceMaxFileSize is the maximum allowed size in bytes for a single evidence file
	EvidenceMaxFileSize int64

	// ConnectivityCheckURL is the URL used to verify API reachability
	ConnectivityCheckURL string
	// ConnectivityInterval is the interval at which connectivity is checked
	ConnectivityInterval time.Duration
}

// Storage option constructors

// WithAPIConfig configures the API connection URL and token
func WithAPIConfig(apiURL, token string) Option {
	return func(c *Config) {
		c.APIURL = apiURL
		c.APIToken = token
	}
}

// WithOrgID sets the organization ID used to scope API requests
func WithOrgID(orgID string) Option {
	return func(c *Config) {
		c.OrgID = orgID
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
