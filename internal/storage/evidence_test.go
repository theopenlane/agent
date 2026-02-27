package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEvidenceService(t *testing.T) {
	config := &EvidenceConfig{
		Enabled:         true,
		DataDir:         "/tmp/evidence",
		MaxFileSize:     1024 * 1024,
		RetentionPeriod: 24 * time.Hour,
	}

	service := NewEvidenceService(config)
	assert.NotNil(t, service)
	assert.Equal(t, config, service.config)
}

func TestNewEvidenceServiceFromConfig(t *testing.T) {
	config := &Config{
		EvidenceEnabled:         true,
		DataDir:                 "/tmp/evidence",
		EvidenceMaxFileSize:     1024 * 1024,
		EvidenceRetentionPeriod: 24 * time.Hour,
	}

	service := NewEvidenceServiceFromConfig(config)
	assert.NotNil(t, service)
	assert.True(t, service.config.Enabled)
	assert.Equal(t, "/tmp/evidence", service.config.DataDir)
	assert.Equal(t, int64(1024*1024), service.config.MaxFileSize)
	assert.Equal(t, 24*time.Hour, service.config.RetentionPeriod)
}

func TestEvidenceService_CollectEvidence_Disabled(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: false,
	}
	service := NewEvidenceService(config)

	ctx := context.Background()
	evidence, err := service.CollectEvidence(ctx, []string{"/some/path"})

	assert.NoError(t, err)
	assert.Nil(t, evidence)
}

func TestEvidenceService_CollectEvidence_EmptyPaths(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: true,
	}
	service := NewEvidenceService(config)

	ctx := context.Background()
	evidence, err := service.CollectEvidence(ctx, []string{})

	assert.NoError(t, err)
	assert.Nil(t, evidence)
}

func TestEvidenceService_CollectEvidence_NonExistentPath(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: true,
		DataDir: "/tmp",
	}
	service := NewEvidenceService(config)

	ctx := context.Background()
	evidence, err := service.CollectEvidence(ctx, []string{"/non/existent/path"})

	assert.NoError(t, err)
	assert.Empty(t, evidence)
}

func TestEvidenceService_CollectEvidence_ValidFile(t *testing.T) {
	// Create temporary test file
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "test.txt")
	testContent := []byte("test evidence content")
	require.NoError(t, os.WriteFile(testFile, testContent, 0644))

	config := &EvidenceConfig{
		Enabled:     true,
		DataDir:     tempDir,
		MaxFileSize: 1024 * 1024,
	}
	service := NewEvidenceService(config)

	ctx := context.Background()
	evidence, err := service.CollectEvidence(ctx, []string{testFile})

	require.NoError(t, err)
	require.Len(t, evidence, 1)

	evidenceFile := evidence[0]
	assert.Equal(t, testFile, evidenceFile.Path)
	assert.Equal(t, testContent, evidenceFile.Content)
	assert.Equal(t, int64(len(testContent)), evidenceFile.Size)
	assert.NotEmpty(t, evidenceFile.Checksum)
	assert.Equal(t, "text/plain; charset=utf-8", evidenceFile.ContentType)
	assert.NotZero(t, evidenceFile.CreatedAt)
	assert.Contains(t, evidenceFile.Metadata, "originalMode")
	assert.Contains(t, evidenceFile.Metadata, "modTime")
}

func TestEvidenceService_CollectEvidence_FileTooLarge(t *testing.T) {
	// Create temporary test directory (not direct file) to trigger directory walk
	tempDir := t.TempDir()
	testFile := filepath.Join(tempDir, "large.txt")
	testContent := make([]byte, 2048) // 2KB file
	require.NoError(t, os.WriteFile(testFile, testContent, 0644))

	config := &EvidenceConfig{
		Enabled:     true,
		DataDir:     tempDir,
		MaxFileSize: 1024, // 1KB limit
	}
	service := NewEvidenceService(config)

	ctx := context.Background()
	// Pass directory path to trigger directory walk with size check
	evidence, err := service.CollectEvidence(ctx, []string{tempDir})

	assert.NoError(t, err)
	assert.Empty(t, evidence) // File should be skipped due to size limit
}

func TestEvidenceService_CollectEvidence_Directory(t *testing.T) {
	// Create temporary test directory with files
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "subdir")
	require.NoError(t, os.MkdirAll(subDir, 0755))

	file1 := filepath.Join(tempDir, "file1.txt")
	file2 := filepath.Join(subDir, "file2.log")
	content1 := []byte("content 1")
	content2 := []byte("content 2")
	require.NoError(t, os.WriteFile(file1, content1, 0644))
	require.NoError(t, os.WriteFile(file2, content2, 0644))

	config := &EvidenceConfig{
		Enabled:     true,
		DataDir:     tempDir,
		MaxFileSize: 1024 * 1024,
	}
	service := NewEvidenceService(config)

	ctx := context.Background()
	evidence, err := service.CollectEvidence(ctx, []string{tempDir})

	require.NoError(t, err)
	require.Len(t, evidence, 2)

	// Sort evidence by path for consistent testing
	if evidence[0].Path > evidence[1].Path {
		evidence[0], evidence[1] = evidence[1], evidence[0]
	}

	assert.Equal(t, file1, evidence[0].Path)
	assert.Equal(t, content1, evidence[0].Content)
	assert.Equal(t, file2, evidence[1].Path)
	assert.Equal(t, content2, evidence[1].Content)
}

func TestEvidenceService_CreateEvidenceFromOutput_Disabled(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: false,
	}
	service := NewEvidenceService(config)

	ctx := context.Background()
	evidence, err := service.CreateEvidenceFromOutput(ctx, "test-check", []byte("stdout"), []byte("stderr"))

	assert.NoError(t, err)
	assert.Nil(t, evidence)
}

func TestEvidenceService_CreateEvidenceFromOutput_EmptyOutput(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: true,
	}
	service := NewEvidenceService(config)

	ctx := context.Background()
	evidence, err := service.CreateEvidenceFromOutput(ctx, "test-check", []byte{}, []byte{})

	assert.NoError(t, err)
	assert.Empty(t, evidence)
}

func TestEvidenceService_CreateEvidenceFromOutput_StdoutOnly(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: true,
	}
	service := NewEvidenceService(config)

	ctx := context.Background()
	stdout := []byte("command output")
	evidence, err := service.CreateEvidenceFromOutput(ctx, "test-check", stdout, []byte{})

	require.NoError(t, err)
	require.Len(t, evidence, 1)

	evidenceFile := evidence[0]
	assert.Equal(t, "test-check-stdout", evidenceFile.Path)
	assert.Equal(t, stdout, evidenceFile.Content)
	assert.Equal(t, int64(len(stdout)), evidenceFile.Size)
	assert.Equal(t, "text/plain", evidenceFile.ContentType)
	assert.Equal(t, "command_output", evidenceFile.Metadata["type"])
	assert.Equal(t, "test-check-stdout", evidenceFile.Metadata["source"])
	assert.Equal(t, "test-check", evidenceFile.Metadata["check_name"])
}

func TestEvidenceService_CreateEvidenceFromOutput_StderrOnly(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: true,
	}
	service := NewEvidenceService(config)

	ctx := context.Background()
	stderr := []byte("error output")
	evidence, err := service.CreateEvidenceFromOutput(ctx, "test-check", []byte{}, stderr)

	require.NoError(t, err)
	require.Len(t, evidence, 1)

	evidenceFile := evidence[0]
	assert.Equal(t, "test-check-stderr", evidenceFile.Path)
	assert.Equal(t, stderr, evidenceFile.Content)
	assert.Equal(t, int64(len(stderr)), evidenceFile.Size)
	assert.Equal(t, "text/plain", evidenceFile.ContentType)
	assert.Equal(t, "command_output", evidenceFile.Metadata["type"])
	assert.Equal(t, "test-check-stderr", evidenceFile.Metadata["source"])
	assert.Equal(t, "test-check", evidenceFile.Metadata["check_name"])
}

func TestEvidenceService_CreateEvidenceFromOutput_BothOutputs(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: true,
	}
	service := NewEvidenceService(config)

	ctx := context.Background()
	stdout := []byte("command output")
	stderr := []byte("error output")
	evidence, err := service.CreateEvidenceFromOutput(ctx, "test-check", stdout, stderr)

	require.NoError(t, err)
	require.Len(t, evidence, 2)

	// Sort evidence by path for consistent testing
	if evidence[0].Path > evidence[1].Path {
		evidence[0], evidence[1] = evidence[1], evidence[0]
	}

	// Check stderr evidence (comes first alphabetically)
	stderrEvidence := evidence[0]
	assert.Equal(t, "test-check-stderr", stderrEvidence.Path)
	assert.Equal(t, stderr, stderrEvidence.Content)

	// Check stdout evidence
	stdoutEvidence := evidence[1]
	assert.Equal(t, "test-check-stdout", stdoutEvidence.Path)
	assert.Equal(t, stdout, stdoutEvidence.Content)
}

func TestEvidenceService_CleanupOldEvidence_Disabled(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: false,
	}
	service := NewEvidenceService(config)

	err := service.CleanupOldEvidence(24 * time.Hour)
	assert.NoError(t, err)
}

func TestEvidenceService_CleanupOldEvidence_ZeroRetention(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: true,
		DataDir: "/tmp",
	}
	service := NewEvidenceService(config)

	err := service.CleanupOldEvidence(0)
	assert.NoError(t, err)
}

func TestEvidenceService_CleanupOldEvidence_NoDataDir(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: true,
		DataDir: "",
	}
	service := NewEvidenceService(config)

	err := service.CleanupOldEvidence(24 * time.Hour)
	assert.NoError(t, err)
}

func TestEvidenceService_CleanupOldEvidence_NonExistentDir(t *testing.T) {
	config := &EvidenceConfig{
		Enabled: true,
		DataDir: "/non/existent/dir",
	}
	service := NewEvidenceService(config)

	err := service.CleanupOldEvidence(24 * time.Hour)
	assert.NoError(t, err) // Should not fail if directory doesn't exist
}

func TestEvidenceService_CleanupOldEvidence_ValidCleanup(t *testing.T) {
	// Create temporary evidence directory
	tempDir := t.TempDir()
	evidenceDir := filepath.Join(tempDir, "evidence")
	require.NoError(t, os.MkdirAll(evidenceDir, 0755))

	// Create old and new files
	oldFile := filepath.Join(evidenceDir, "old.txt")
	newFile := filepath.Join(evidenceDir, "new.txt")
	require.NoError(t, os.WriteFile(oldFile, []byte("old"), 0644))
	require.NoError(t, os.WriteFile(newFile, []byte("new"), 0644))

	// Set old file modification time to 2 days ago
	oldTime := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(oldFile, oldTime, oldTime))

	config := &EvidenceConfig{
		Enabled: true,
		DataDir: tempDir,
	}
	service := NewEvidenceService(config)

	// Cleanup files older than 24 hours
	err := service.CleanupOldEvidence(24 * time.Hour)
	assert.NoError(t, err)

	// Check that old file was removed and new file remains
	_, err = os.Stat(oldFile)
	assert.True(t, os.IsNotExist(err))

	_, err = os.Stat(newFile)
	assert.NoError(t, err)
}