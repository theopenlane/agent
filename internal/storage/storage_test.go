package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/theopenlane/agent/config"
)

func TestLocalStorage(t *testing.T) {
	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "storage_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create local storage
	storage, err := NewStorage(
		WithLocalStorage(tempDir, filepath.Join(tempDir, "results"), "json"),
		WithEvidence(true, 24*time.Hour, 1024*1024, false),
	)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Close()

	// Test health check
	if err := storage.Health(); err != nil {
		t.Errorf("Health check failed: %v", err)
	}

	// Create test result
	result := &config.Result{
		CheckName:  "test-check",
		ExecutedAt: time.Now(),
		StartTime:  time.Now(),
		EndTime:    time.Now(),
		Duration:   "1s",
		ExitCode:   0,
		Passed:     true,
	}

	// Store result
	if err := storage.StoreResult(result); err != nil {
		t.Errorf("Failed to store result: %v", err)
	}

	// Check stats
	stats := storage.GetStats()
	if stats.TotalResults != 1 {
		t.Errorf("Expected 1 total result, got %d", stats.TotalResults)
	}
	if stats.SuccessfulUploads != 1 {
		t.Errorf("Expected 1 successful upload, got %d", stats.SuccessfulUploads)
	}

	// Verify file was created
	resultFiles, err := filepath.Glob(filepath.Join(tempDir, "results", "*.json"))
	if err != nil {
		t.Errorf("Failed to list result files: %v", err)
	}
	if len(resultFiles) != 1 {
		t.Errorf("Expected 1 result file, got %d", len(resultFiles))
	}
}

func TestEvidenceService(t *testing.T) {
	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "evidence_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := []byte("test evidence content")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create evidence service
	cfg := &Config{
		EvidenceEnabled:         true,
		DataDir:                 tempDir,
		EvidenceMaxFileSize:     1024 * 1024,
		EvidenceRetentionPeriod: 24 * time.Hour,
		EvidenceCompressFiles:   false,
	}
	evidenceService := NewEvidenceService(cfg)

	// Test evidence collection
	ctx := context.Background()
	evidence, err := evidenceService.CollectEvidence(ctx, []string{testFile})
	if err != nil {
		t.Errorf("Failed to collect evidence: %v", err)
	}

	if len(evidence) != 1 {
		t.Errorf("Expected 1 evidence file, got %d", len(evidence))
	}

	// Test evidence from output
	stdout := []byte("command output")
	stderr := []byte("error output")
	outputEvidence, err := evidenceService.CreateEvidenceFromOutput("test-check", stdout, stderr)
	if err != nil {
		t.Errorf("Failed to create evidence from output: %v", err)
	}

	if len(outputEvidence) != 2 { // stdout + stderr
		t.Errorf("Expected 2 evidence files, got %d", len(outputEvidence))
	}
}

func TestStorageOptionsFromConfig(t *testing.T) {
	// Test API mode
	cfg := &config.Config{
		APIURL:            "https://api.example.com",
		RegistrationToken: "test-token",
		DataDir:           "./data",
		Offline: config.OfflineConfig{
			Mode:      config.ModeNormal,
			OutputDir: "./results",
		},
		Evidence: config.EvidenceConfig{
			Enabled: true,
		},
	}

	opts := OptionsFromConfig(cfg)
	if len(opts) < 2 { // Should have API storage + evidence options
		t.Errorf("Expected at least 2 options, got %d", len(opts))
	}

	// Test local mode
	cfg.Offline.Mode = config.ModeStandalone
	opts = OptionsFromConfig(cfg)
	if len(opts) < 2 { // Should have local storage + evidence options
		t.Errorf("Expected at least 2 options, got %d", len(opts))
	}

	// Test buffered mode
	cfg.Offline.Mode = config.ModeBuffered
	cfg.Offline.BufferDir = "./buffer"
	opts = OptionsFromConfig(cfg)
	if len(opts) < 2 { // Should have buffered storage + evidence options
		t.Errorf("Expected at least 2 options, got %d", len(opts))
	}
}
