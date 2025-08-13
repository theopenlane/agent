package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/theopenlane/agent/internal/config"
	"gopkg.in/yaml.v3"
)

// LocalFileStorage implements ResultStorage for local file output
type LocalFileStorage struct {
	logger      zerolog.Logger
	outputDir   string
	format      string
	permissions os.FileMode
}

// NewLocalFileStorage creates a new local file storage instance
func NewLocalFileStorage(cfg config.Config) (*LocalFileStorage, error) {
	outputDir := cfg.Offline.OutputDir
	if outputDir == "" {
		outputDir = "./results"
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	format := cfg.Offline.OutputFormat
	if format == "" {
		format = "json"
	}

	logger := zerolog.New(os.Stdout).With().Str("component", "local-file-storage").Logger()

	return &LocalFileStorage{
		logger:      logger,
		outputDir:   outputDir,
		format:      format,
		permissions: 0644,
	}, nil
}

// StoreResult stores a single result to a local file
func (lfs *LocalFileStorage) StoreResult(ctx context.Context, result *config.Result) error {
	filename := lfs.generateFileName(result)
	filePath := filepath.Join(lfs.outputDir, filename)

	data, err := lfs.serializeResult(result)
	if err != nil {
		return fmt.Errorf("failed to serialize result: %w", err)
	}

	if err := os.WriteFile(filePath, data, lfs.permissions); err != nil {
		return fmt.Errorf("failed to write result file: %w", err)
	}

	lfs.logger.Info().
		Str("check", result.CheckName).
		Str("file", filePath).
		Msg("Result stored to local file")

	return nil
}

// StoreResults stores multiple results to local files
func (lfs *LocalFileStorage) StoreResults(ctx context.Context, results []*config.Result) error {
	for _, result := range results {
		if err := lfs.StoreResult(ctx, result); err != nil {
			return err
		}
	}
	return nil
}

// GetStorageInfo returns information about the local file storage
func (lfs *LocalFileStorage) GetStorageInfo() StorageInfo {
	// Check if output directory is accessible
	healthy := true
	var lastError error
	
	if _, err := os.Stat(lfs.outputDir); err != nil {
		healthy = false
		lastError = err
	}

	// Count existing result files
	fileCount := 0
	if files, err := os.ReadDir(lfs.outputDir); err == nil {
		for _, file := range files {
			if strings.HasSuffix(file.Name(), "."+lfs.format) {
				fileCount++
			}
		}
	}

	metadata := map[string]interface{}{
		"format":     lfs.format,
		"file_count": fileCount,
	}
	
	if lastError != nil {
		metadata["last_error"] = lastError.Error()
	}

	return StorageInfo{
		Type:        "local-file",
		Location:    lfs.outputDir,
		Healthy:     healthy,
		LastCheck:   time.Now(),
		Metadata:    metadata,
		Capabilities: []string{"store-results", "persistent"},
	}
}

// Close performs cleanup (no-op for file storage)
func (lfs *LocalFileStorage) Close() error {
	lfs.logger.Debug().Msg("Closing local file storage")
	return nil
}

// generateFileName creates a filename for the result
func (lfs *LocalFileStorage) generateFileName(result *config.Result) string {
	// Create a safe filename from the check name and timestamp
	safeName := strings.ReplaceAll(result.CheckName, " ", "_")
	safeName = strings.ReplaceAll(safeName, "/", "_")
	safeName = strings.ReplaceAll(safeName, "\\", "_")
	
	timestamp := result.ExecutedAt.Format("20060102_150405")
	return fmt.Sprintf("%s_%s.%s", safeName, timestamp, lfs.format)
}

// serializeResult converts the result to the specified format
func (lfs *LocalFileStorage) serializeResult(result *config.Result) ([]byte, error) {
	switch strings.ToLower(lfs.format) {
	case "json":
		return json.MarshalIndent(result, "", "  ")
	case "yaml", "yml":
		return yaml.Marshal(result)
	case "csv":
		return lfs.resultToCSV(result), nil
	default:
		return nil, fmt.Errorf("unsupported output format: %s", lfs.format)
	}
}

// resultToCSV converts a result to CSV format (simple implementation)
func (lfs *LocalFileStorage) resultToCSV(result *config.Result) []byte {
	lines := []string{
		"Field,Value",
		fmt.Sprintf("CheckName,%s", result.CheckName),
		fmt.Sprintf("ExecutedAt,%s", result.ExecutedAt.Format(time.RFC3339)),
		fmt.Sprintf("Duration,%s", result.Duration),
		fmt.Sprintf("ExitCode,%d", result.ExitCode),
		fmt.Sprintf("Passed,%t", result.Passed),
		fmt.Sprintf("FindingsCount,%d", len(result.Findings)),
	}
	
	if result.Error != "" {
		lines = append(lines, fmt.Sprintf("Error,%s", strings.ReplaceAll(result.Error, ",", ";")))
	}
	
	return []byte(strings.Join(lines, "\n"))
}

// LocalEvidenceStorage implements EvidenceStorage for local evidence files
type LocalEvidenceStorage struct {
	logger      zerolog.Logger
	outputDir   string
	permissions os.FileMode
}

// NewLocalEvidenceStorage creates a new local evidence storage instance
func NewLocalEvidenceStorage(cfg config.Config) (*LocalEvidenceStorage, error) {
	outputDir := filepath.Join(cfg.Offline.OutputDir, "evidence")
	
	// Ensure evidence directory exists
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create evidence directory: %w", err)
	}

	logger := zerolog.New(os.Stdout).With().Str("component", "local-evidence-storage").Logger()

	return &LocalEvidenceStorage{
		logger:      logger,
		outputDir:   outputDir,
		permissions: 0644,
	}, nil
}

// StoreEvidence copies evidence files to the local evidence directory
func (les *LocalEvidenceStorage) StoreEvidence(ctx context.Context, evidencePaths []string, controlID string, metadata map[string]string) ([]*config.EvidenceFileResult, error) {
	var results []*config.EvidenceFileResult
	
	// Create control-specific directory
	controlDir := filepath.Join(les.outputDir, controlID)
	if err := os.MkdirAll(controlDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create control evidence directory: %w", err)
	}

	for _, evidencePath := range evidencePaths {
		result, err := les.storeEvidenceFile(evidencePath, controlDir, controlID, metadata)
		if err != nil {
			les.logger.Error().Err(err).Str("path", evidencePath).Msg("Failed to store evidence file")
			result = &config.EvidenceFileResult{
				FilePath:  evidencePath,
				ControlID: controlID,
				Error:     err.Error(),
			}
		}
		results = append(results, result)
	}

	return results, nil
}

// storeEvidenceFile copies a single evidence file
func (les *LocalEvidenceStorage) storeEvidenceFile(srcPath, destDir, controlID string, metadata map[string]string) (*config.EvidenceFileResult, error) {
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat evidence file: %w", err)
	}

	srcData, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read evidence file: %w", err)
	}

	// Generate destination filename with timestamp
	baseName := filepath.Base(srcPath)
	timestamp := time.Now().Format("20060102_150405")
	destName := fmt.Sprintf("%s_%s", timestamp, baseName)
	destPath := filepath.Join(destDir, destName)

	if err := os.WriteFile(destPath, srcData, les.permissions); err != nil {
		return nil, fmt.Errorf("failed to write evidence file: %w", err)
	}

	les.logger.Info().
		Str("src", srcPath).
		Str("dest", destPath).
		Str("control", controlID).
		Msg("Evidence file stored locally")

	return &config.EvidenceFileResult{
		FilePath:    destPath,
		FileID:      fmt.Sprintf("local_%s_%s", controlID, destName),
		ControlID:   controlID,
		Size:        srcInfo.Size(),
		ContentType: "application/octet-stream", // Simple default
		UploadedAt:  time.Now(),
		Metadata:    metadata,
	}, nil
}

// GetStorageInfo returns information about the local evidence storage
func (les *LocalEvidenceStorage) GetStorageInfo() StorageInfo {
	healthy := true
	var lastError error
	
	if _, err := os.Stat(les.outputDir); err != nil {
		healthy = false
		lastError = err
	}

	// Count evidence files
	fileCount := 0
	if err := filepath.Walk(les.outputDir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			fileCount++
		}
		return nil
	}); err != nil {
		lastError = err
		healthy = false
	}

	metadata := map[string]interface{}{
		"file_count": fileCount,
	}
	
	if lastError != nil {
		metadata["last_error"] = lastError.Error()
	}

	return StorageInfo{
		Type:        "local-evidence",
		Location:    les.outputDir,
		Healthy:     healthy,
		LastCheck:   time.Now(),
		Metadata:    metadata,
		Capabilities: []string{"store-evidence", "persistent", "local-copy"},
	}
}

// Close performs cleanup (no-op for file storage)
func (les *LocalEvidenceStorage) Close() error {
	les.logger.Debug().Msg("Closing local evidence storage")
	return nil
}