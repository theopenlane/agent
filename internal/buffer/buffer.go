package buffer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/theopenlane/agent/internal/config"
)

// BufferedResult represents a check result that's been buffered to disk
type BufferedResult struct {
	ID         string         `json:"id"`
	Result     *config.Result `json:"result"`
	BufferedAt time.Time      `json:"buffered_at"`
	Retries    int            `json:"retries"`
	LastError  string         `json:"last_error,omitempty"`
}

// ResultBuffer manages offline buffering of check results
type ResultBuffer struct {
	logger     zerolog.Logger
	bufferDir  string
	maxRetries int
	mu         sync.RWMutex
}

// NewResultBuffer creates a new result buffer
func NewResultBuffer(logger zerolog.Logger, bufferDir string) (*ResultBuffer, error) {
	// Ensure buffer directory exists
	if err := os.MkdirAll(bufferDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create buffer directory: %w", err)
	}

	return &ResultBuffer{
		logger:     logger,
		bufferDir:  bufferDir,
		maxRetries: 5,
	}, nil
}

// BufferResult saves a check result to disk for later transmission
func (b *ResultBuffer) BufferResult(result *config.Result) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Create a unique ID for this buffered result
	id := fmt.Sprintf("%s_%d_%s", result.CheckName, result.StartTime.Unix(), generateID())

	buffered := &BufferedResult{
		ID:         id,
		Result:     result,
		BufferedAt: time.Now(),
		Retries:    0,
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(buffered, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	// Write to file
	filename := filepath.Join(b.bufferDir, id+".json")
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write buffer file: %w", err)
	}

	b.logger.Info().
		Str("check", result.CheckName).
		Str("file", filename).
		Msg("Buffered check result to disk")

	return nil
}

// GetBufferedResults returns all buffered results
func (b *ResultBuffer) GetBufferedResults() ([]*BufferedResult, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	files, err := os.ReadDir(b.bufferDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read buffer directory: %w", err)
	}

	var results []*BufferedResult

	for _, file := range files {
		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		path := filepath.Join(b.bufferDir, file.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			b.logger.Error().Err(err).Str("file", path).Msg("Failed to read buffer file")
			continue
		}

		var buffered BufferedResult
		if err := json.Unmarshal(data, &buffered); err != nil {
			b.logger.Error().Err(err).Str("file", path).Msg("Failed to unmarshal buffer file")
			continue
		}

		results = append(results, &buffered)
	}

	return results, nil
}

// RemoveBufferedResult removes a successfully transmitted result from the buffer
func (b *ResultBuffer) RemoveBufferedResult(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	filename := filepath.Join(b.bufferDir, id+".json")
	if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove buffer file: %w", err)
	}

	b.logger.Debug().Str("id", id).Msg("Removed buffered result")
	return nil
}

// UpdateBufferedResult updates a buffered result (e.g., increment retry count)
func (b *ResultBuffer) UpdateBufferedResult(buffered *BufferedResult) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	data, err := json.MarshalIndent(buffered, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	filename := filepath.Join(b.bufferDir, buffered.ID+".json")
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to update buffer file: %w", err)
	}

	return nil
}

// CleanupOldResults removes buffered results older than the specified duration
func (b *ResultBuffer) CleanupOldResults(ctx context.Context, maxAge time.Duration) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	files, err := os.ReadDir(b.bufferDir)
	if err != nil {
		return fmt.Errorf("failed to read buffer directory: %w", err)
	}

	cutoff := time.Now().Add(-maxAge)
	removed := 0

	for _, file := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if filepath.Ext(file.Name()) != ".json" {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(b.bufferDir, file.Name())
			if err := os.Remove(path); err != nil {
				b.logger.Error().Err(err).Str("file", path).Msg("Failed to remove old buffer file")
			} else {
				removed++
			}
		}
	}

	if removed > 0 {
		b.logger.Info().Int("count", removed).Msg("Cleaned up old buffered results")
	}

	return nil
}

// GetBufferStats returns statistics about the buffer
func (b *ResultBuffer) GetBufferStats() (map[string]interface{}, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	files, err := os.ReadDir(b.bufferDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read buffer directory: %w", err)
	}

	var totalSize int64
	var count int

	for _, file := range files {
		if filepath.Ext(file.Name()) == ".json" {
			count++
			if info, err := file.Info(); err == nil {
				totalSize += info.Size()
			}
		}
	}

	return map[string]interface{}{
		"buffered_count": count,
		"total_size":     totalSize,
		"buffer_dir":     b.bufferDir,
	}, nil
}

// generateID generates a unique identifier
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}