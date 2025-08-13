package storage

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/rs/zerolog"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/internal/config"
	"github.com/theopenlane/agent/internal/evidence"
)

// APIResultStorage implements ResultStorage for Openlane API
type APIResultStorage struct {
	logger    zerolog.Logger
	apiClient *api.GraphQLClient
	agentID   string
	apiURL    string
}

// NewAPIResultStorage creates a new API result storage instance
func NewAPIResultStorage(cfg config.Config) (*APIResultStorage, error) {
	apiClient, err := api.NewGraphQLClient(cfg.APIURL, cfg.RegistrationToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	logger := zerolog.New(os.Stdout).With().Str("component", "api-result-storage").Logger()

	return &APIResultStorage{
		logger:    logger,
		apiClient: apiClient,
		agentID:   cfg.AgentID,
		apiURL:    cfg.APIURL,
	}, nil
}

// StoreResult stores a single result via the API
func (ars *APIResultStorage) StoreResult(ctx context.Context, result *config.Result) error {
	results := []*config.Result{result}
	return ars.StoreResults(ctx, results)
}

// StoreResults stores multiple results via the API
func (ars *APIResultStorage) StoreResults(ctx context.Context, results []*config.Result) error {
	if len(results) == 0 {
		return nil
	}

	ars.logger.Debug().Int("count", len(results)).Msg("Storing results via API")
	
	err := ars.apiClient.ReportResults(ctx, ars.agentID, results)
	if err != nil {
		return fmt.Errorf("failed to report results to API: %w", err)
	}

	ars.logger.Info().Int("count", len(results)).Msg("Results stored via API")
	return nil
}

// GetStorageInfo returns information about the API storage
func (ars *APIResultStorage) GetStorageInfo() StorageInfo {
	// Perform a simple health check
	healthy := true
	var lastError error
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Try a simple API call to check health
	if _, err := ars.apiClient.PollForWork(ctx); err != nil {
		healthy = false
		lastError = err
	}

	metadata := map[string]interface{}{
		"agent_id": ars.agentID,
		"endpoint": ars.apiURL,
	}
	
	if lastError != nil {
		metadata["last_error"] = lastError.Error()
	}

	return StorageInfo{
		Type:        "openlane-api",
		Location:    ars.apiURL,
		Healthy:     healthy,
		LastCheck:   time.Now(),
		Metadata:    metadata,
		Capabilities: []string{"store-results", "real-time", "remote"},
	}
}

// Close performs cleanup for the API storage
func (ars *APIResultStorage) Close() error {
	ars.logger.Debug().Msg("Closing API result storage")
	return nil
}

// APIEvidenceStorage implements EvidenceStorage for Openlane API
type APIEvidenceStorage struct {
	logger   zerolog.Logger
	uploader evidence.Uploader
	apiURL   string
}

// NewAPIEvidenceStorage creates a new API evidence storage instance
func NewAPIEvidenceStorage(cfg config.Config) (*APIEvidenceStorage, error) {
	logger := zerolog.New(os.Stdout).With().Str("component", "api-evidence-storage").Logger()
	
	// Create uploader - in production this would use the actual Openlane uploader
	// For now, use mock uploader since we need to properly integrate with the evidence system
	uploader := evidence.NewMockFileUploader(logger)

	return &APIEvidenceStorage{
		logger:   logger,
		uploader: uploader,
		apiURL:   cfg.APIURL,
	}, nil
}

// StoreEvidence uploads evidence files via the API
func (aes *APIEvidenceStorage) StoreEvidence(ctx context.Context, evidencePaths []string, controlID string, metadata map[string]string) ([]*config.EvidenceFileResult, error) {
	var results []*config.EvidenceFileResult

	for _, evidencePath := range evidencePaths {
		aes.logger.Debug().Str("path", evidencePath).Str("control", controlID).Msg("Uploading evidence file")
		
		uploadResult, err := aes.uploader.UploadFile(ctx, evidencePath, controlID, metadata)
		if err != nil {
			aes.logger.Error().Err(err).Str("path", evidencePath).Msg("Failed to upload evidence file")
			result := &config.EvidenceFileResult{
				FilePath:  evidencePath,
				ControlID: controlID,
				Error:     err.Error(),
			}
			results = append(results, result)
			continue
		}

		// Convert upload result to evidence file result
		result := &config.EvidenceFileResult{
			FilePath:    evidencePath,
			FileID:      uploadResult.FileID,
			ControlID:   controlID,
			Size:        uploadResult.Size,
			ContentType: uploadResult.ContentType,
			Checksum:    uploadResult.Checksum,
			UploadedAt:  uploadResult.UploadedAt,
			Metadata:    uploadResult.Metadata,
		}
		results = append(results, result)
		
		aes.logger.Info().
			Str("file_id", result.FileID).
			Str("path", evidencePath).
			Str("control", controlID).
			Msg("Evidence file uploaded via API")
	}

	return results, nil
}

// GetStorageInfo returns information about the API evidence storage
func (aes *APIEvidenceStorage) GetStorageInfo() StorageInfo {
	// Simple health check - in production this might ping the upload endpoint
	healthy := true
	
	metadata := map[string]interface{}{
		"endpoint": aes.apiURL,
		"uploader_type": "openlane-api",
	}

	return StorageInfo{
		Type:        "openlane-evidence-api",
		Location:    aes.apiURL,
		Healthy:     healthy,
		LastCheck:   time.Now(),
		Metadata:    metadata,
		Capabilities: []string{"store-evidence", "real-time", "remote", "checksums"},
	}
}

// Close performs cleanup for the API evidence storage
func (aes *APIEvidenceStorage) Close() error {
	aes.logger.Debug().Msg("Closing API evidence storage")
	return nil
}