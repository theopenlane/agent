package storage

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/config"
)

// APIStorage implements Storage interface for direct API storage
type APIStorage struct {
	config    *Config
	apiClient *api.GraphQLClient
	stats     Stats
	mu        sync.RWMutex
}

// NewAPIStorage creates a new API storage instance
func NewAPIStorage(cfg *Config) (*APIStorage, error) {
	if cfg.APIURL == "" {
		return nil, ErrAPIURLRequired
	}

	if cfg.RegistrationToken == "" {
		return nil, ErrRegistrationTokenRequired
	}

	apiClient, err := api.NewGraphQLClient(cfg.APIURL, cfg.RegistrationToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return &APIStorage{
		config:    cfg,
		apiClient: apiClient,
		stats:     Stats{},
	}, nil
}

// StoreResult stores a compliance check result via API
func (as *APIStorage) StoreResult(result *config.Result) error {
	return as.StoreResultWithEvidence(result, nil)
}

// StoreResultWithEvidence stores a result along with collected evidence via API
func (as *APIStorage) StoreResultWithEvidence(result *config.Result, evidence []EvidenceFile) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	ctx := context.Background()
	start := time.Now()

	// Upload evidence files if provided and evidence is enabled
	if len(evidence) > 0 && as.config.EvidenceEnabled {
		if err := as.uploadEvidence(result, evidence); err != nil {
			log.Error().Err(err).Int("evidence_files", len(evidence)).Str("check", result.CheckName).Msg("Failed to upload evidence files")
			// Continue with result upload even if evidence upload fails
		} else {
			log.Debug().Int("evidence_files", len(evidence)).Str("check", result.CheckName).Msg("Evidence files uploaded successfully")
		}

		as.stats.EvidenceFiles += int64(len(evidence))
	}

	// Upload the main result (includes findings)
	if err := as.uploadResult(ctx, result); err != nil {
		as.stats.FailedUploads++
		return fmt.Errorf("failed to upload result: %w", err)
	}

	as.stats.TotalResults++
	as.stats.SuccessfulUploads++

	duration := time.Since(start)
	log.Info().Str("check", result.CheckName).Dur("duration", duration).Int("evidence_files", len(evidence)).Int("findings", len(result.Findings)).Msg("Result and evidence uploaded to API")

	return nil
}

// uploadResult uploads a compliance check result to the API
func (as *APIStorage) uploadResult(ctx context.Context, result *config.Result) error {
	// Use the existing ReportResults method which leverages the GraphQL client middleware
	// to handle both result data and any associated findings
	return as.apiClient.ReportResults(ctx, []*config.Result{result})
}

// uploadEvidence uploads evidence files using the GraphQL client's evidence upload capability
func (as *APIStorage) uploadEvidence(result *config.Result, evidence []EvidenceFile) error {
	// The GraphQL client has evidence upload capability built into its middleware
	// We can leverage this by calling the client with the evidence data
	for _, evidenceFile := range evidence {
		// Use the GraphQL client to upload each evidence file
		// The middleware will handle the actual upload process
		if err := as.uploadSingleEvidenceFile(result, evidenceFile); err != nil {
			log.Error().Err(err).Str("evidence_path", evidenceFile.Path).Str("check", result.CheckName).Msg("Failed to upload evidence file")
			return fmt.Errorf("failed to upload evidence file %s: %w", evidenceFile.Path, err)
		}
	}

	return nil
}

// uploadSingleEvidenceFile uploads a single evidence file via the GraphQL client
func (as *APIStorage) uploadSingleEvidenceFile(result *config.Result, evidence EvidenceFile) error {
	log.Debug().Str("path", evidence.Path).Int64("size", evidence.Size).Str("checksum", evidence.Checksum[:8]).Str("content_type", evidence.ContentType).Msg("Uploading evidence file via GraphQL client middleware")

	// Convert storage.EvidenceFile to api.EvidenceFile
	apiEvidence := api.EvidenceFile{
		Path:        evidence.Path,
		Content:     evidence.Content,
		Size:        evidence.Size,
		Checksum:    evidence.Checksum,
		ContentType: evidence.ContentType,
		CreatedAt:   evidence.CreatedAt,
		Metadata:    evidence.Metadata,
	}

	// Use the GraphQL client's evidence upload capability
	// The middleware handles the actual upload process
	if err := as.apiClient.UploadEvidence(result.CheckName, apiEvidence); err != nil {
		return fmt.Errorf("GraphQL client evidence upload failed: %w", err)
	}

	log.Info().Str("path", evidence.Path).Str("check", result.CheckName).Msg("Evidence file uploaded successfully")

	return nil
}

// GetStats returns storage statistics
func (as *APIStorage) GetStats() Stats {
	as.mu.RLock()
	defer as.mu.RUnlock()

	return as.stats
}

// Health returns the current health status
func (as *APIStorage) Health() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second) // nolint:mnd
	defer cancel()

	// Test API connectivity by trying to poll for work (simple connectivity test)
	_, err := as.apiClient.PollForWork(ctx)
	if err != nil {
		return fmt.Errorf("API connectivity test failed: %w", err)
	}

	return nil
}

// Close gracefully shuts down the storage system
func (as *APIStorage) Close() error {
	// API client doesn't need explicit closing in current implementation
	log.Info().Msg("API storage closed")
	return nil
}
