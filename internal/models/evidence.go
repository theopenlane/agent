// Package models provides shared type definitions used across the agent
package models

import "time"

// EvidenceFile represents a file collected as evidence during a compliance check
type EvidenceFile struct {
	Path        string            `json:"path"`
	Content     []byte            `json:"content,omitempty"`
	Size        int64             `json:"size"`
	Checksum    string            `json:"checksum"`
	ContentType string            `json:"contentType"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
}
