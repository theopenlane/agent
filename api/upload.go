package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/config"
	"github.com/theopenlane/agent/internal/models"
)

// FileUploadOptions contains options for file upload preparation
type FileUploadOptions struct {
	ContentType string
	Filename    string
	Prefix      string
}

// PrepareJSONFileUpload creates a GraphQL upload from any JSON-marshalable data
func PrepareJSONFileUpload(data any, filename string) (*graphql.Upload, error) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data to JSON: %w", err)
	}

	return &graphql.Upload{
		File:        bytes.NewReader(jsonBytes),
		Filename:    filename,
		Size:        int64(len(jsonBytes)),
		ContentType: "application/json",
	}, nil
}

// PrepareTextFileUpload creates a GraphQL upload from text content
func PrepareTextFileUpload(content, filename string) *graphql.Upload {
	contentBytes := []byte(content)

	return &graphql.Upload{
		File:        bytes.NewReader(contentBytes),
		Filename:    filename,
		Size:        int64(len(contentBytes)),
		ContentType: "text/plain",
	}
}

// PrepareEvidenceFileUpload creates a GraphQL upload from evidence file
func PrepareEvidenceFileUpload(evidence models.EvidenceFile) *graphql.Upload {
	return &graphql.Upload{
		File:        bytes.NewReader(evidence.Content),
		Filename:    evidence.Path,
		Size:        evidence.Size,
		ContentType: evidence.ContentType,
	}
}

// PrepareResultFileUpload creates a GraphQL upload from a compliance result
func PrepareResultFileUpload(result *config.Result) *graphql.Upload {
	if result == nil {
		return nil
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.json", strings.ReplaceAll(result.CheckName, " ", "-"), timestamp)

	upload, err := PrepareJSONFileUpload(result, filename)
	if err != nil {
		log.Error().Err(err).Str("check", result.CheckName).Msg("Failed to marshal result for file upload")
		return nil
	}

	return upload
}

// PrepareLogFileUpload creates a GraphQL upload from log content
func PrepareLogFileUpload(checkName, logContent string) (*graphql.Upload, error) {
	if logContent == "" {
		return nil, ErrLogContentEmpty
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%s.log", strings.ReplaceAll(checkName, " ", "-"), timestamp)

	return PrepareTextFileUpload(logContent, filename), nil
}

// GenerateTimestampedFilename creates a filename with timestamp
func GenerateTimestampedFilename(baseName, extension string) string {
	timestamp := time.Now().Format("20060102-150405")
	cleanBaseName := strings.ReplaceAll(baseName, " ", "-")

	return fmt.Sprintf("%s-%s.%s", cleanBaseName, timestamp, extension)
}
