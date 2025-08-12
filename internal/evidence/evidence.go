package evidence

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

// EvidenceManager handles evidence file collection, storage, and upload
type EvidenceManager struct {
	dataDir string
	logger  zerolog.Logger
	uploader Uploader
}

// Uploader interface for uploading evidence files to the Openlane platform
type Uploader interface {
	UploadFile(ctx context.Context, filePath, controlID string, metadata map[string]string) (*UploadResult, error)
}

// UploadResult represents the result of an evidence upload
type UploadResult struct {
	FileID      string            `json:"file_id"`
	URL         string            `json:"url,omitempty"`
	Size        int64             `json:"size"`
	ContentType string            `json:"content_type"`
	Checksum    string            `json:"checksum"`
	UploadedAt  time.Time         `json:"uploaded_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// EvidenceFile represents an evidence file with metadata
type EvidenceFile struct {
	Path        string            `json:"path"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
	Size        int64             `json:"size"`
	Checksum    string            `json:"checksum"`
	CreatedAt   time.Time         `json:"created_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// NewEvidenceManager creates a new evidence manager
func NewEvidenceManager(dataDir string, logger zerolog.Logger, uploader Uploader) *EvidenceManager {
	return &EvidenceManager{
		dataDir:  dataDir,
		logger:   logger,
		uploader: uploader,
	}
}

// CollectEvidence collects evidence files from specified paths
func (e *EvidenceManager) CollectEvidence(evidencePaths []string, checkName string) ([]*EvidenceFile, error) {
	var evidenceFiles []*EvidenceFile
	
	for _, path := range evidencePaths {
		// Resolve relative paths relative to data directory
		if !filepath.IsAbs(path) {
			path = filepath.Join(e.dataDir, path)
		}
		
		// Check if path exists
		info, err := os.Stat(path)
		if err != nil {
			e.logger.Warn().Str("path", path).Err(err).Msg("Evidence file not found")
			continue
		}
		
		if info.IsDir() {
			// Collect all files in directory
			dirFiles, err := e.collectFromDirectory(path, checkName)
			if err != nil {
				e.logger.Error().Str("dir", path).Err(err).Msg("Failed to collect evidence from directory")
				continue
			}
			evidenceFiles = append(evidenceFiles, dirFiles...)
		} else {
			// Single file
			evidenceFile, err := e.createEvidenceFile(path, checkName)
			if err != nil {
				e.logger.Error().Str("file", path).Err(err).Msg("Failed to create evidence file")
				continue
			}
			evidenceFiles = append(evidenceFiles, evidenceFile)
		}
	}
	
	e.logger.Info().Int("count", len(evidenceFiles)).Str("check", checkName).Msg("Collected evidence files")
	return evidenceFiles, nil
}

// collectFromDirectory collects evidence files from a directory
func (e *EvidenceManager) collectFromDirectory(dirPath, checkName string) ([]*EvidenceFile, error) {
	var evidenceFiles []*EvidenceFile
	
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories
		if info.IsDir() {
			return nil
		}
		
		// Create evidence file
		evidenceFile, err := e.createEvidenceFile(path, checkName)
		if err != nil {
			e.logger.Warn().Str("file", path).Err(err).Msg("Failed to create evidence file")
			return nil // Continue walking
		}
		
		evidenceFiles = append(evidenceFiles, evidenceFile)
		return nil
	})
	
	return evidenceFiles, err
}

// createEvidenceFile creates an evidence file struct with metadata
func (e *EvidenceManager) createEvidenceFile(filePath, checkName string) (*EvidenceFile, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	
	// Calculate checksum
	checksum, err := e.calculateChecksum(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate checksum: %w", err)
	}
	
	// Determine content type based on file extension
	contentType := e.getContentType(filePath)
	
	evidenceFile := &EvidenceFile{
		Path:        filePath,
		Name:        filepath.Base(filePath),
		Size:        info.Size(),
		Checksum:    checksum,
		CreatedAt:   info.ModTime(),
		ContentType: contentType,
		Metadata: map[string]string{
			"check_name": checkName,
			"file_path": filePath,
		},
	}
	
	return evidenceFile, nil
}

// calculateChecksum calculates SHA256 checksum of a file
func (e *EvidenceManager) calculateChecksum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

// getContentType determines content type based on file extension
func (e *EvidenceManager) getContentType(filePath string) string {
	ext := filepath.Ext(filePath)
	
	contentTypes := map[string]string{
		".txt":  "text/plain",
		".log":  "text/plain",
		".json": "application/json",
		".xml":  "application/xml",
		".yaml": "application/yaml",
		".yml":  "application/yaml",
		".csv":  "text/csv",
		".png":  "image/png",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".pdf":  "application/pdf",
		".html": "text/html",
		".sh":   "text/x-shellscript",
		".py":   "text/x-python",
		".rb":   "text/x-ruby",
		".go":   "text/x-go",
	}
	
	if contentType, exists := contentTypes[ext]; exists {
		return contentType
	}
	
	return "application/octet-stream"
}

// UploadEvidence uploads evidence files to the Openlane platform
func (e *EvidenceManager) UploadEvidence(ctx context.Context, evidenceFiles []*EvidenceFile, controlID string) ([]*UploadResult, error) {
	var uploadResults []*UploadResult
	
	for _, evidenceFile := range evidenceFiles {
		e.logger.Info().Str("file", evidenceFile.Name).Str("control", controlID).Msg("Uploading evidence file")
		
		result, err := e.uploader.UploadFile(ctx, evidenceFile.Path, controlID, evidenceFile.Metadata)
		if err != nil {
			e.logger.Error().Str("file", evidenceFile.Path).Err(err).Msg("Failed to upload evidence file")
			continue
		}
		
		uploadResults = append(uploadResults, result)
		e.logger.Info().Str("file_id", result.FileID).Str("file", evidenceFile.Name).Msg("Evidence file uploaded successfully")
	}
	
	return uploadResults, nil
}

// CreateEvidenceFromOutput creates evidence files from command output
func (e *EvidenceManager) CreateEvidenceFromOutput(checkName, stdout, stderr string) ([]*EvidenceFile, error) {
	var evidenceFiles []*EvidenceFile
	
	// Create evidence directory for this check
	evidenceDir := filepath.Join(e.dataDir, "evidence", checkName, fmt.Sprintf("%d", time.Now().Unix()))
	if err := os.MkdirAll(evidenceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create evidence directory: %w", err)
	}
	
	// Save stdout if not empty
	if stdout != "" {
		stdoutPath := filepath.Join(evidenceDir, "stdout.log")
		if err := os.WriteFile(stdoutPath, []byte(stdout), 0644); err != nil {
			e.logger.Error().Err(err).Msg("Failed to write stdout evidence")
		} else {
			evidenceFile, err := e.createEvidenceFile(stdoutPath, checkName)
			if err == nil {
				evidenceFile.Description = "Command standard output"
				evidenceFiles = append(evidenceFiles, evidenceFile)
			}
		}
	}
	
	// Save stderr if not empty
	if stderr != "" {
		stderrPath := filepath.Join(evidenceDir, "stderr.log")
		if err := os.WriteFile(stderrPath, []byte(stderr), 0644); err != nil {
			e.logger.Error().Err(err).Msg("Failed to write stderr evidence")
		} else {
			evidenceFile, err := e.createEvidenceFile(stderrPath, checkName)
			if err == nil {
				evidenceFile.Description = "Command standard error"
				evidenceFiles = append(evidenceFiles, evidenceFile)
			}
		}
	}
	
	return evidenceFiles, nil
}

// CleanupOldEvidence removes evidence files older than the specified duration
func (e *EvidenceManager) CleanupOldEvidence(maxAge time.Duration) error {
	evidenceDir := filepath.Join(e.dataDir, "evidence")
	cutoffTime := time.Now().Add(-maxAge)
	
	return filepath.Walk(evidenceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking
		}
		
		if !info.IsDir() && info.ModTime().Before(cutoffTime) {
			if err := os.Remove(path); err != nil {
				e.logger.Warn().Str("file", path).Err(err).Msg("Failed to remove old evidence file")
			} else {
				e.logger.Debug().Str("file", path).Msg("Removed old evidence file")
			}
		}
		
		return nil
	})
}