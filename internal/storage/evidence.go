package storage

import (
	"context"
	"crypto/sha256"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/internal/models"
)

// EvidenceConfig contains evidence-specific configuration
type EvidenceConfig struct {
	Enabled         bool
	DataDir         string
	MaxFileSize     int64
	RetentionPeriod time.Duration
}

// EvidenceService implements evidence collection and management
type EvidenceService struct {
	config *EvidenceConfig
}

// NewEvidenceService creates a new evidence service
func NewEvidenceService(config *EvidenceConfig) *EvidenceService {
	return &EvidenceService{
		config: config,
	}
}

// NewEvidenceServiceFromConfig creates evidence service from storage config
func NewEvidenceServiceFromConfig(config *Config) *EvidenceService {
	return NewEvidenceService(&EvidenceConfig{
		Enabled:         config.EvidenceEnabled,
		DataDir:         config.DataDir,
		MaxFileSize:     config.EvidenceMaxFileSize,
		RetentionPeriod: config.EvidenceRetentionPeriod,
	})
}

// CollectEvidence collects evidence files from the specified paths
func (es *EvidenceService) CollectEvidence(ctx context.Context, paths []string) ([]models.EvidenceFile, error) {
	if !es.config.Enabled || len(paths) == 0 {
		return nil, nil
	}

	var evidenceFiles []models.EvidenceFile

	for _, path := range paths {
		files, err := es.collectFromPath(ctx, path)
		if err != nil {
			log.Error().Err(err).Str("path", path).Msg("Failed to collect evidence from path")

			continue
		}

		evidenceFiles = append(evidenceFiles, files...)
	}

	log.Debug().Int("files", len(evidenceFiles)).Strs("paths", paths).Msg("Evidence collection completed")

	return evidenceFiles, nil
}

// collectFromPath collects evidence from a single path (file or directory)
func (es *EvidenceService) collectFromPath(_ context.Context, path string) ([]models.EvidenceFile, error) {
	// Handle relative paths
	if !filepath.IsAbs(path) {
		if es.config.DataDir != "" {
			path = filepath.Join(es.config.DataDir, path)
		} else {
			// Convert to absolute path
			absPath, err := filepath.Abs(path)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve path %s: %w", path, err)
			}

			path = absPath
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Debug().Str("path", path).Msg("Evidence path does not exist")
			return nil, nil
		}

		return nil, fmt.Errorf("failed to stat path %s: %w", path, err)
	}

	if info.IsDir() {
		return es.collectFromDirectory(path)
	}

	file, err := es.collectFromFile(path, info)
	if err != nil {
		return nil, err
	}

	return []models.EvidenceFile{*file}, nil
}

// collectFromDirectory recursively collects evidence from a directory
func (es *EvidenceService) collectFromDirectory(dirPath string) ([]models.EvidenceFile, error) {
	var evidenceFiles []models.EvidenceFile

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("Error walking directory")
			return nil // Continue walking
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check file size limits
		if es.config.MaxFileSize > 0 && info.Size() > es.config.MaxFileSize {
			log.Warn().Str("path", path).Int64("size", info.Size()).Int64("max_size", es.config.MaxFileSize).Msg("Evidence file exceeds size limit, skipping")

			return nil
		}

		// Collect the file
		file, err := es.collectFromFile(path, info)
		if err != nil {
			log.Error().Err(err).Str("path", path).Msg("Failed to collect evidence file")

			return nil // Continue walking
		}

		evidenceFiles = append(evidenceFiles, *file)

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", dirPath, err)
	}

	return evidenceFiles, nil
}

// collectFromFile collects evidence from a single file
func (es *EvidenceService) collectFromFile(filePath string, info os.FileInfo) (*models.EvidenceFile, error) {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	// Calculate checksum
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	// Determine content type
	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	evidenceFile := &models.EvidenceFile{
		Path:        filePath,
		Content:     content,
		Size:        info.Size(),
		Checksum:    checksum,
		ContentType: contentType,
		CreatedAt:   time.Now(),
		Metadata: map[string]string{
			"originalMode": info.Mode().String(),
			"modTime":      info.ModTime().Format(time.RFC3339),
		},
	}

	log.Debug().Str("path", filePath).Int64("size", info.Size()).Str("checksum", checksum[:8]).Msg("Evidence file collected")

	return evidenceFile, nil
}

// CreateEvidenceFromOutput creates evidence files from command output
func (es *EvidenceService) CreateEvidenceFromOutput(_ context.Context, checkName string, stdout, stderr []byte) ([]models.EvidenceFile, error) {
	if !es.config.Enabled {
		return nil, nil
	}

	var evidenceFiles []models.EvidenceFile

	timestamp := time.Now()

	// Create stdout evidence if not empty
	if len(stdout) > 0 {
		stdoutFile := es.createOutputEvidenceFile(
			fmt.Sprintf("%s-stdout", checkName),
			stdout,
			timestamp,
			checkName,
		)
		evidenceFiles = append(evidenceFiles, *stdoutFile)
	}

	// Create stderr evidence if not empty
	if len(stderr) > 0 {
		stderrFile := es.createOutputEvidenceFile(
			fmt.Sprintf("%s-stderr", checkName),
			stderr,
			timestamp,
			checkName,
		)
		evidenceFiles = append(evidenceFiles, *stderrFile)
	}

	if len(evidenceFiles) > 0 {
		log.Debug().Str("check", checkName).Int("files", len(evidenceFiles)).Msg("Evidence created from command output")
	}

	return evidenceFiles, nil
}

// createOutputEvidenceFile creates an evidence file from command output
func (es *EvidenceService) createOutputEvidenceFile(name string, content []byte, timestamp time.Time, checkName string) *models.EvidenceFile {
	// Calculate checksum
	checksum := fmt.Sprintf("%x", sha256.Sum256(content))

	return &models.EvidenceFile{
		Path:        name, // Virtual path for output
		Content:     content,
		Size:        int64(len(content)),
		Checksum:    checksum,
		ContentType: "text/plain",
		CreatedAt:   timestamp,
		Metadata: map[string]string{
			"type":       "command_output",
			"source":     strings.TrimSuffix(name, filepath.Ext(name)),
			"check_name": checkName,
		},
	}
}

// CleanupOldEvidence removes old evidence files based on retention policy
func (es *EvidenceService) CleanupOldEvidence(retentionPeriod time.Duration) error {
	if !es.config.Enabled || retentionPeriod <= 0 || es.config.DataDir == "" {
		return nil
	}

	evidenceDir := filepath.Join(es.config.DataDir, "evidence")
	if _, err := os.Stat(evidenceDir); os.IsNotExist(err) {
		return nil // No evidence directory, nothing to clean
	}

	cutoff := time.Now().Add(-retentionPeriod)
	deletedCount := 0

	err := filepath.Walk(evidenceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("Error during evidence cleanup walk")
			return nil // Continue walking
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if file is older than retention period
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				log.Error().Err(err).Str("path", path).Msg("Failed to remove old evidence file")
			} else {
				log.Debug().Str("path", path).Time("mod_time", info.ModTime()).Msg("Removed old evidence file")

				deletedCount++
			}
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to cleanup evidence directory: %w", err)
	}

	if deletedCount > 0 {
		log.Info().Int("deleted", deletedCount).Dur("retention_period", retentionPeriod).Msg("Evidence cleanup completed")
	}

	return nil
}
