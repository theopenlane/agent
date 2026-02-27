package api //nolint:revive

import (
	"bytes"

	"github.com/99designs/gqlgen/graphql"
	"github.com/theopenlane/agent/internal/models"
)

// PrepareEvidenceFileUpload creates a GraphQL upload from an evidence file
func PrepareEvidenceFileUpload(evidence models.EvidenceFile) *graphql.Upload {
	return &graphql.Upload{
		File:        bytes.NewReader(evidence.Content),
		Filename:    evidence.Path,
		Size:        evidence.Size,
		ContentType: evidence.ContentType,
	}
}
