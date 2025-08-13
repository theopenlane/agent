package storage

import (
	"context"
	"time"

	"github.com/theopenlane/agent/internal/config"
)

// ResultStorage defines the interface for storing compliance check results
type ResultStorage interface {
	// StoreResult stores a single compliance check result
	StoreResult(ctx context.Context, result *config.Result) error
	
	// StoreResults stores multiple compliance check results
	StoreResults(ctx context.Context, results []*config.Result) error
	
	// GetStorageInfo returns information about the storage backend
	GetStorageInfo() StorageInfo
	
	// Close performs any cleanup needed by the storage backend
	Close() error
}

// EvidenceStorage defines the interface for storing evidence files
type EvidenceStorage interface {
	// StoreEvidence stores evidence files for a check result
	StoreEvidence(ctx context.Context, evidencePaths []string, controlID string, metadata map[string]string) ([]*config.EvidenceFileResult, error)
	
	// GetStorageInfo returns information about the evidence storage backend
	GetStorageInfo() StorageInfo
	
	// Close performs any cleanup needed by the storage backend
	Close() error
}

// StorageInfo provides metadata about a storage backend
type StorageInfo struct {
	Type        string                 `json:"type"`         // e.g., "openlane-api", "local-file", "s3", "http"
	Location    string                 `json:"location"`     // Storage location/endpoint
	Healthy     bool                   `json:"healthy"`      // Whether the storage is accessible
	LastCheck   time.Time              `json:"last_check"`   // Last health check time
	Metadata    map[string]interface{} `json:"metadata"`     // Backend-specific metadata
	Capabilities []string              `json:"capabilities"` // What the storage supports
}

// ResultStorageFactory creates result storage instances based on configuration
type ResultStorageFactory interface {
	CreateResultStorage(config config.Config) (ResultStorage, error)
}

// EvidenceStorageFactory creates evidence storage instances based on configuration  
type EvidenceStorageFactory interface {
	CreateEvidenceStorage(config config.Config) (EvidenceStorage, error)
}

// StorageManager coordinates multiple storage backends
type StorageManager struct {
	ResultStorage   ResultStorage
	EvidenceStorage EvidenceStorage
}

// NewStorageManager creates a new storage manager with the appropriate backends
func NewStorageManager(cfg config.Config) (*StorageManager, error) {
	var resultStorage ResultStorage
	var evidenceStorage EvidenceStorage
	var err error

	// Choose result storage backend based on mode
	switch cfg.Offline.Mode {
	case config.ModeStandalone:
		resultStorage, err = NewLocalFileStorage(cfg)
	case config.ModeBuffered:
		resultStorage, err = NewBufferedAPIStorage(cfg)
	case config.ModeNormal:
		resultStorage, err = NewAPIResultStorage(cfg)
	default:
		resultStorage, err = NewAPIResultStorage(cfg) // Default to API
	}
	
	if err != nil {
		return nil, err
	}

	// Choose evidence storage backend
	if cfg.Offline.Mode == config.ModeStandalone {
		evidenceStorage, err = NewLocalEvidenceStorage(cfg)
	} else {
		evidenceStorage, err = NewAPIEvidenceStorage(cfg)
	}
	
	if err != nil {
		return nil, err
	}

	return &StorageManager{
		ResultStorage:   resultStorage,
		EvidenceStorage: evidenceStorage,
	}, nil
}

// StoreResult stores a result using the configured storage backend
func (sm *StorageManager) StoreResult(ctx context.Context, result *config.Result) error {
	return sm.ResultStorage.StoreResult(ctx, result)
}

// StoreResults stores multiple results using the configured storage backend
func (sm *StorageManager) StoreResults(ctx context.Context, results []*config.Result) error {
	return sm.ResultStorage.StoreResults(ctx, results)
}

// StoreEvidence stores evidence using the configured storage backend
func (sm *StorageManager) StoreEvidence(ctx context.Context, evidencePaths []string, controlID string, metadata map[string]string) ([]*config.EvidenceFileResult, error) {
	return sm.EvidenceStorage.StoreEvidence(ctx, evidencePaths, controlID, metadata)
}

// GetStorageStatus returns the status of both storage backends
func (sm *StorageManager) GetStorageStatus() map[string]StorageInfo {
	return map[string]StorageInfo{
		"results":  sm.ResultStorage.GetStorageInfo(),
		"evidence": sm.EvidenceStorage.GetStorageInfo(),
	}
}

// Close closes all storage backends
func (sm *StorageManager) Close() error {
	var resultErr, evidenceErr error
	
	if sm.ResultStorage != nil {
		resultErr = sm.ResultStorage.Close()
	}
	
	if sm.EvidenceStorage != nil {
		evidenceErr = sm.EvidenceStorage.Close()
	}
	
	// Return the first error encountered
	if resultErr != nil {
		return resultErr
	}
	return evidenceErr
}