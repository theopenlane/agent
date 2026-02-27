package storage

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/models"
)

const (
	// healthCheckTimeoutSeconds is the timeout used for API health check requests
	healthCheckTimeoutSeconds = 10
)

// APIStorage implements Storage interface for direct API storage
type APIStorage struct {
	config    *Config
	apiClient *api.GraphQLClient
	mu        sync.RWMutex
}

// NewAPIStorage creates a new API storage instance
func NewAPIStorage(cfg *Config) (*APIStorage, error) {
	if cfg.APIURL == "" {
		return nil, ErrAPIURLRequired
	}

	if cfg.APIToken == "" {
		return nil, ErrAPITokenRequired
	}

	apiClient, err := api.NewGraphQLClientWithRetry(cfg.APIURL, cfg.APIToken, cfg.OrgID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create API client: %w", err)
	}

	return &APIStorage{
		config:    cfg,
		apiClient: apiClient,
	}, nil
}

// StoreResult stores a compliance check result via API
func (as *APIStorage) StoreResult(result *config.Result) error {
	return as.StoreResultWithEvidence(result, nil)
}

// StoreResultWithEvidence stores a result along with collected evidence via API
func (as *APIStorage) StoreResultWithEvidence(result *config.Result, evidence []models.EvidenceFile) error {
	as.mu.Lock()
	defer as.mu.Unlock()

	ctx := context.Background()
	start := time.Now()

	// Upload evidence files if provided and evidence is enabled
	if len(evidence) > 0 && as.config.EvidenceEnabled {
		if err := as.uploadEvidence(ctx, result, evidence); err != nil {
			log.Error().Err(err).Int("evidence_files", len(evidence)).Str("check", result.CheckName).Msg("Failed to upload evidence files")
			// Continue with result upload even if evidence upload fails
		} else {
			log.Debug().Int("evidence_files", len(evidence)).Str("check", result.CheckName).Msg("Evidence files uploaded successfully")
		}
	}

	// Upload the main result (includes findings)
	if err := as.uploadResult(ctx, result); err != nil {
		return fmt.Errorf("failed to upload result: %w", err)
	}

	duration := time.Since(start)
	findingsCount := 0

	if result.Metadata != nil {
		if findings, ok := result.Metadata["findings"]; ok {
			switch findingsSlice := findings.(type) {
			case []any:
				findingsCount = len(findingsSlice)
			case []config.Finding:
				findingsCount = len(findingsSlice)
			}
		}
	}

	log.Info().Str("check", result.CheckName).Dur("duration", duration).Int("evidence_files", len(evidence)).Int("findings", findingsCount).Msg("Result and evidence uploaded to API")

	return nil
}

// uploadResult uploads a compliance check result to the API
func (as *APIStorage) uploadResult(ctx context.Context, result *config.Result) error {
	return as.uploadStandaloneResultAsEvidence(ctx, result)
}

func (as *APIStorage) uploadStandaloneResultAsEvidence(ctx context.Context, result *config.Result) error {
	payload, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal standalone result payload: %w", err)
	}

	checksum := fmt.Sprintf("%x", sha256.Sum256(payload))
	checkName := strings.ReplaceAll(strings.TrimSpace(result.CheckName), " ", "-")

	if checkName == "" {
		checkName = "check"
	}

	evidence := models.EvidenceFile{
		Path:        fmt.Sprintf("%s-result.json", checkName),
		Content:     payload,
		Size:        int64(len(payload)),
		Checksum:    checksum,
		ContentType: "application/json",
		CreatedAt:   time.Now().UTC(),
		Metadata: map[string]string{
			"source": "standalone-result",
			"check":  result.CheckName,
		},
	}

	runArtifactID := fmt.Sprintf("standalone-%s-%d", checkName, time.Now().Unix())

	if err := as.apiClient.CreateEvidence(ctx, nil, evidence, runArtifactID); err != nil {
		return fmt.Errorf("failed to upload standalone result evidence: %w", err)
	}

	return nil
}

// uploadEvidence uploads evidence files with optional control associations using the GraphQL client
func (as *APIStorage) uploadEvidence(ctx context.Context, result *config.Result, evidence []models.EvidenceFile) error {
	// Prefer pre-resolved control IDs attached by the check controller
	allControls := controlIDsFromMetadata(result.Metadata)

	// Fallback for results that only carry standard+control_ref values.
	if len(allControls) == 0 {
		allControls = as.resolveControlIDsFromReferences(ctx, result)
	}

	runArtifactID := fmt.Sprintf("result-%s-%d", result.CheckName, result.StartedAt.Unix())

	for _, evidenceFile := range evidence {
		if err := as.uploadEvidenceFile(ctx, result, evidenceFile, allControls, runArtifactID); err != nil {
			log.Error().Err(err).Str("evidence_path", evidenceFile.Path).Str("check", result.CheckName).Msg("Failed to upload evidence file")
			return fmt.Errorf("failed to upload evidence file %s: %w", evidenceFile.Path, err)
		}
	}

	return nil
}

// uploadEvidenceFile uploads a single evidence file with optional control associations
func (as *APIStorage) uploadEvidenceFile(ctx context.Context, result *config.Result, evidence models.EvidenceFile, controlIDs []string, runArtifactID string) error {
	controlIDsStr := "none"
	if len(controlIDs) > 0 {
		controlIDsStr = fmt.Sprintf("%d controls", len(controlIDs))
	}

	checksumPrefix := evidence.Checksum
	if len(checksumPrefix) > 8 { //nolint:mnd
		checksumPrefix = checksumPrefix[:8]
	}

	log.Debug().Str("path", evidence.Path).Str("controls", controlIDsStr).Int64("size", evidence.Size).Str("checksum", checksumPrefix).Msg("Uploading evidence file")

	if err := as.apiClient.CreateEvidence(ctx, controlIDs, evidence, runArtifactID); err != nil {
		return fmt.Errorf("evidence upload failed: %w", err)
	}

	log.Info().Str("path", evidence.Path).Str("controls", controlIDsStr).Str("check", result.CheckName).Msg("Evidence file uploaded")

	return nil
}

// Health returns the current health status
func (as *APIStorage) Health() error {
	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeoutSeconds*time.Second)
	defer cancel()

	if err := as.apiClient.Health(ctx); err != nil {
		return fmt.Errorf("API connectivity test failed: %w", err)
	}

	return nil
}

// Close gracefully shuts down the storage system
func (as *APIStorage) Close() error {
	log.Info().Msg("API storage closed")
	return nil
}

func controlIDsFromMetadata(metadata map[string]any) []string {
	if metadata == nil {
		return nil
	}

	rawControlIDs, ok := metadata["control_ids"]
	if !ok {
		return nil
	}

	var parsed []string

	switch v := rawControlIDs.(type) {
	case []string:
		parsed = append(parsed, v...)
	case []any:
		for _, item := range v {
			if id, ok := item.(string); ok {
				parsed = append(parsed, id)
			}
		}
	case string:
		parsed = append(parsed, v)
	}

	seen := make(map[string]struct{}, len(parsed))
	deduped := make([]string, 0, len(parsed))

	for _, id := range parsed {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}

		if _, exists := seen[trimmed]; exists {
			continue
		}

		seen[trimmed] = struct{}{}
		deduped = append(deduped, trimmed)
	}

	return deduped
}

// resolveControlIDsFromReferences resolves control IDs from standard/control_ref values.
func (as *APIStorage) resolveControlIDsFromReferences(ctx context.Context, result *config.Result) []string {
	if result == nil || as.apiClient == nil {
		return nil
	}

	standard := ""
	controlRef := ""

	if result.Metadata != nil {
		if v, ok := result.Metadata["standard"].(string); ok {
			standard = strings.TrimSpace(v)
		}

		if v, ok := result.Metadata["control_ref"].(string); ok {
			controlRef = strings.TrimSpace(v)
		}
	}

	if standard == "" {
		standard = strings.TrimSpace(result.Standard)
	}

	if controlRef == "" {
		controlRef = strings.TrimSpace(result.ControlRef)
	}

	if standard == "" || controlRef == "" {
		return nil
	}

	controlIDs, err := as.apiClient.ResolveControlIDs(ctx, []config.ComplianceStandard{
		{
			Standard: standard,
			Controls: []string{controlRef},
		},
	})
	if err != nil {
		log.Warn().Err(err).Str("standard", standard).Str("control_ref", controlRef).Str("check", result.CheckName).Msg("Failed to resolve control reference")
		return nil
	}

	return controlIDs
}
