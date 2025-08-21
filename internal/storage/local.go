package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/invopop/yaml"
	"github.com/rs/zerolog/log"

	"github.com/theopenlane/agent/config"
)

// LocalStorage implements Storage interface for local file storage
type LocalStorage struct {
	config *Config
	stats  Stats
	mu     sync.RWMutex
}

// NewLocalStorage creates a new local storage instance
func NewLocalStorage(cfg *Config) (*LocalStorage, error) {
	// Ensure output directory exists
	if err := os.MkdirAll(cfg.OutputDir, 0o755); err != nil { // nolint:mnd
		return nil, fmt.Errorf("failed to create output directory %s: %w", cfg.OutputDir, err)
	}

	// Ensure data directory exists for evidence
	if cfg.EvidenceEnabled && cfg.DataDir != "" {
		if err := os.MkdirAll(filepath.Join(cfg.DataDir, "evidence"), 0o755); err != nil { // nolint:mnd
			return nil, fmt.Errorf("failed to create evidence directory: %w", err)
		}
	}

	return &LocalStorage{
		config: cfg,
		stats:  Stats{},
	}, nil
}

// StoreResult stores a compliance check result to local files
func (ls *LocalStorage) StoreResult(result *config.Result) error {
	return ls.StoreResultWithEvidence(result, nil)
}

// StoreResultWithEvidence stores a result along with collected evidence
func (ls *LocalStorage) StoreResultWithEvidence(result *config.Result, evidence []EvidenceFile) error {
	ls.mu.Lock()
	defer ls.mu.Unlock()

	// Generate filename based on check name and timestamp
	timestamp := result.ExecutedAt.Format("20060102-150405")
	baseFilename := fmt.Sprintf("%s-%s", result.CheckName, timestamp)

	// Store the result
	if err := ls.storeResultFile(result, baseFilename); err != nil {
		ls.stats.FailedUploads++
		return fmt.Errorf("failed to store result: %w", err)
	}

	// Store evidence files if any
	if len(evidence) > 0 && ls.config.EvidenceEnabled {
		if err := ls.storeEvidenceFiles(evidence, result.CheckName, timestamp); err != nil {
			log.Error().Err(err).Msg("Failed to store evidence files")
			// Don't fail the entire operation for evidence storage errors
		} else {
			ls.stats.EvidenceFiles += int64(len(evidence))
		}
	}

	ls.stats.TotalResults++
	ls.stats.SuccessfulUploads++

	return nil
}

// storeResultFile writes the result to a file in the specified format
func (ls *LocalStorage) storeResultFile(result *config.Result, baseFilename string) error {
	var data []byte

	var err error

	var filename string

	switch strings.ToLower(ls.config.OutputFormat) {
	case "yaml", "yml":
		filename = baseFilename + ".yaml"
		data, err = yaml.Marshal(result)
	case "json":
		filename = baseFilename + ".json"
		data, err = json.MarshalIndent(result, "", "  ")
	case "csv":
		filename = baseFilename + ".csv"
		data, err = ls.resultToCSV(result)
	default:
		filename = baseFilename + ".json"
		data, err = json.MarshalIndent(result, "", "  ")
	}

	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	filepath := filepath.Join(ls.config.OutputDir, filename)
	if err := os.WriteFile(filepath, data, 0o600); err != nil { // nolint:mnd
		return fmt.Errorf("failed to write result file %s: %w", filepath, err)
	}

	log.Info().Str("check", result.CheckName).Str("file", filepath).Msg("result stored locally")

	return nil
}

// storeEvidenceFiles copies evidence files to the evidence directory
func (ls *LocalStorage) storeEvidenceFiles(evidence []EvidenceFile, checkName, timestamp string) error {
	evidenceDir := filepath.Join(ls.config.DataDir, "evidence", checkName, timestamp)
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil { // nolint:mnd
		return fmt.Errorf("failed to create evidence directory: %w", err)
	}

	for i, file := range evidence {
		// Generate safe filename
		filename := fmt.Sprintf("evidence-%d-%s", i+1, filepath.Base(file.Path))
		if filename == fmt.Sprintf("evidence-%d-", i+1) {
			filename = fmt.Sprintf("evidence-%d.txt", i+1)
		}

		destPath := filepath.Join(evidenceDir, filename)

		// Write evidence content
		if err := os.WriteFile(destPath, file.Content, 0o600); err != nil { // nolint:mnd
			log.Error().Err(err).Str("source", file.Path).Str("dest", destPath).Msg("Failed to store evidence file")

			continue
		}

		// Write metadata file
		metadataPath := destPath + ".metadata.json"
		metadata := struct {
			OriginalPath string            `json:"originalPath"`
			Checksum     string            `json:"checksum"`
			ContentType  string            `json:"contentType"`
			Size         int64             `json:"size"`
			CreatedAt    time.Time         `json:"createdAt"`
			Metadata     map[string]string `json:"metadata,omitempty"`
		}{
			OriginalPath: file.Path,
			Checksum:     file.Checksum,
			ContentType:  file.ContentType,
			Size:         file.Size,
			CreatedAt:    file.CreatedAt,
			Metadata:     file.Metadata,
		}

		metadataJSON, err := json.MarshalIndent(metadata, "", "  ")
		if err == nil {
			if writeErr := os.WriteFile(metadataPath, metadataJSON, 0o644); writeErr != nil { // nolint:mnd
				log.Warn().Err(writeErr).Str("metadata_path", metadataPath).Msg("Failed to write metadata file")
			}
		}

		log.Debug().Str("source", file.Path).Str("dest", destPath).Msg("Evidence file stored")
	}

	return nil
}

// resultToCSV converts a result to CSV format (simplified)
func (ls *LocalStorage) resultToCSV(result *config.Result) ([]byte, error) {
	// Simple CSV format with key result information
	csvHeader := "CheckName,ExecutedAt,Duration,ExitCode,Passed,Error\n"
	csvRow := fmt.Sprintf("%s,%s,%s,%d,%t,%s\n",
		escapeCSVField(result.CheckName),
		result.ExecutedAt.Format(time.RFC3339),
		result.Duration,
		result.ExitCode,
		result.Passed,
		escapeCSVField(result.Error),
	)

	return []byte(csvHeader + csvRow), nil
}

// escapeCSVField escapes a field for CSV format
func escapeCSVField(field string) string {
	if strings.Contains(field, ",") || strings.Contains(field, "\"") || strings.Contains(field, "\n") {
		field = strings.ReplaceAll(field, "\"", "\"\"")
		return "\"" + field + "\""
	}

	return field
}

// GetStats returns storage statistics
func (ls *LocalStorage) GetStats() Stats {
	ls.mu.RLock()
	defer ls.mu.RUnlock()

	return ls.stats
}

// Health returns the current health status
func (ls *LocalStorage) Health() error {
	// Check if output directory is writable
	testFile := filepath.Join(ls.config.OutputDir, ".health_check")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil { // nolint:mnd
		return fmt.Errorf("output directory not writable: %w", err)
	}

	os.Remove(testFile)

	// Check evidence directory if enabled
	if ls.config.EvidenceEnabled && ls.config.DataDir != "" {
		evidenceDir := filepath.Join(ls.config.DataDir, "evidence")

		testFile := filepath.Join(evidenceDir, ".health_check")
		if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil { // nolint:mnd
			return fmt.Errorf("evidence directory not writable: %w", err)
		}

		os.Remove(testFile)
	}

	return nil
}

// Close gracefully shuts down the storage system
func (ls *LocalStorage) Close() error {
	log.Info().Msg("Local storage closed")
	return nil
}
