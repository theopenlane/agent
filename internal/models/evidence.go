// Package models provides shared type definitions used across the agent
package models

import "time"

// EvidenceFile represents a file collected as evidence during a compliance check
type EvidenceFile struct {
	// Path is the file system location of the evidence file
	Path string `json:"path"`
	// Content is the raw file data collected as evidence
	Content []byte `json:"content,omitempty"`
	// Size is the length of the file in bytes
	Size int64 `json:"size"`
	// Checksum is the hash value for file integrity verification
	Checksum string `json:"checksum"`
	// ContentType is the MIME type of the evidence file
	ContentType string `json:"contentType"`
	// Metadata contains additional attributes about the file
	Metadata map[string]string `json:"metadata,omitempty"`
	// CreatedAt is the timestamp when the file was created
	CreatedAt time.Time `json:"createdAt"`
}
