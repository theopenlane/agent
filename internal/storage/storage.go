package storage

import (
	"context"
	"time"

	"github.com/theopenlane/agent/config"
)

// EvidenceFile represents a file collected as evidence during a compliance check
type EvidenceFile struct {
	Path        string            `json:"path"`
	Content     []byte            `json:"content,omitempty"`
	Size        int64             `json:"size"`
	Checksum    string            `json:"checksum"`
	ContentType string            `json:"contentType"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
}

// Result wraps a result with storage metadata for buffering
type Result struct {
	ID         string         `json:"id"`
	Result     *config.Result `json:"result"`
	BufferedAt time.Time      `json:"bufferedAt"`
	RetryCount int            `json:"retryCount"`
	LastError  string         `json:"lastError,omitempty"`
	Evidence   []EvidenceFile `json:"evidence,omitempty"`
}

// Stats provides information about storage operations
type Stats struct {
	TotalResults       int64     `json:"totalResults"`
	SuccessfulUploads  int64     `json:"successfulUploads"`
	FailedUploads      int64     `json:"failedUploads"`
	BufferedResults    int64     `json:"bufferedResults"`
	EvidenceFiles      int64     `json:"evidenceFiles"`
	LastSyncAttempt    time.Time `json:"lastSyncAttempt,omitempty"`
	LastSuccessfulSync time.Time `json:"lastSuccessfulSync,omitempty"`
}

// Storage is the unified interface for storing compliance results and evidence
type Storage interface {
	// StoreResult stores a compliance check result
	StoreResult(result *config.Result) error

	// StoreResultWithEvidence stores a result along with collected evidence
	StoreResultWithEvidence(result *config.Result, evidence []EvidenceFile) error

	// GetStats returns storage statistics
	GetStats() Stats

	// Health returns the current health status
	Health() error

	// Close gracefully shuts down the storage system
	Close() error
}

// EvidenceCollector handles evidence file collection
type EvidenceCollector interface {
	// CollectEvidence collects evidence files from the specified paths
	CollectEvidence(ctx context.Context, paths []string) ([]EvidenceFile, error)

	// CreateEvidenceFromOutput creates evidence files from command output
	CreateEvidenceFromOutput(ctx context.Context, checkName string, stdout, stderr []byte) ([]EvidenceFile, error)

	// CleanupOldEvidence removes old evidence files based on retention policy
	CleanupOldEvidence(ctx context.Context, retentionPeriod time.Duration) error
}

// Option configures storage behavior
type Option func(*Config)

// Config contains all storage configuration
type Config struct {
	// Mode determines the storage strategy
	Mode Mode

	// API configuration
	APIURL            string
	RegistrationToken string

	// Local storage configuration
	DataDir      string
	OutputDir    string
	OutputFormat string

	// Buffering configuration
	BufferDir             string
	MaxRetries            int
	RetryBackoff          time.Duration
	BufferRetentionPeriod time.Duration
	SyncInterval          time.Duration

	// Evidence configuration
	EvidenceEnabled         bool
	EvidenceRetentionPeriod time.Duration
	EvidenceMaxFileSize     int64
	EvidenceCompressFiles   bool

	// Connectivity configuration
	ConnectivityCheckURL string
	ConnectivityInterval time.Duration
}

// Mode defines how storage operates
type Mode string

const (
	// ModeAPI stores directly to the Openlane API
	ModeAPI Mode = "api"

	// ModeLocal stores to local files only
	ModeLocal Mode = "local"

	// ModeBuffered stores to API with local buffering fallback
	ModeBuffered Mode = "buffered"
)

// Storage option constructors

// WithAPIStorage configures direct API storage
func WithAPIStorage(apiURL, token string) Option {
	return func(c *Config) {
		c.Mode = ModeAPI
		c.APIURL = apiURL
		c.RegistrationToken = token
	}
}

// WithLocalStorage configures local file storage
func WithLocalStorage(dataDir, outputDir, format string) Option {
	return func(c *Config) {
		c.Mode = ModeLocal
		c.DataDir = dataDir
		c.OutputDir = outputDir
		c.OutputFormat = format
	}
}

// WithBufferedStorage configures API storage with local buffering fallback
func WithBufferedStorage(apiURL, token, bufferDir string) Option {
	return func(c *Config) {
		c.Mode = ModeBuffered
		c.APIURL = apiURL
		c.RegistrationToken = token
		c.BufferDir = bufferDir
		c.MaxRetries = 5 // nolint:mnd
		c.RetryBackoff = time.Minute
		c.BufferRetentionPeriod = 7 * 24 * time.Hour // nolint:mnd
		c.SyncInterval = 5 * time.Minute             // nolint:mnd
	}
}

// WithEvidence enables evidence collection
func WithEvidence(enabled bool, retentionPeriod time.Duration, maxFileSize int64, compress bool) Option {
	return func(c *Config) {
		c.EvidenceEnabled = enabled
		c.EvidenceRetentionPeriod = retentionPeriod
		c.EvidenceMaxFileSize = maxFileSize
		c.EvidenceCompressFiles = compress
	}
}

// WithConnectivityMonitoring configures connectivity checking for buffered mode
func WithConnectivityMonitoring(checkURL string, interval time.Duration) Option {
	return func(c *Config) {
		c.ConnectivityCheckURL = checkURL
		c.ConnectivityInterval = interval
	}
}

// WithRetryPolicy configures retry behavior for buffered mode
func WithRetryPolicy(maxRetries int, backoff time.Duration) Option {
	return func(c *Config) {
		c.MaxRetries = maxRetries
		c.RetryBackoff = backoff
	}
}

// NewStorage creates a new storage instance based on the provided options
func NewStorage(opts ...Option) (Storage, error) {
	config := &Config{
		Mode:         ModeLocal, // Default to local storage
		DataDir:      "./data",
		OutputDir:    "./results",
		OutputFormat: "json",
	}

	for _, opt := range opts {
		opt(config)
	}

	switch config.Mode {
	case ModeAPI:
		return NewAPIStorage(config)
	case ModeLocal:
		return NewLocalStorage(config)
	case ModeBuffered:
		return NewBufferedStorage(config)
	default:
		return NewLocalStorage(config)
	}
}
