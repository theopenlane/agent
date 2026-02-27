// Package schema provides standardized data structures for compliance check results
package schema

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/theopenlane/core/common/enums"
)

const (
	// Example data constants for schema examples
	exampleSSHConfigLine = 32
)

// ComplianceCheckResult represents a normalized compliance check result payload
type ComplianceCheckResult struct {
	// CheckName is the name of the compliance check that produced this result
	CheckName string `json:"checkName" jsonschema:"required,description=Name of the compliance check"`
	// StartedAt is the time at which check execution began
	StartedAt time.Time `json:"startedAt" jsonschema:"required,description=When the check execution started"`
	// FinishedAt is the time at which check execution completed
	FinishedAt time.Time `json:"finishedAt" jsonschema:"required,description=When the check execution finished"`
	// Standard is the compliance standard identifier (e.g. soc2, nist80053v5)
	Standard string `json:"standard" jsonschema:"required,description=Compliance standard identifier,example=soc2"`
	// ControlRef is the control reference code within the standard (e.g. CC6.1)
	ControlRef string `json:"controlRef" jsonschema:"required,description=Control reference code,example=CC6.1"`
	// Status is the execution status of the check
	Status enums.JobExecutionStatus `json:"status" jsonschema:"required,description=Execution status"`
	// ExitCode is the process exit code; nil when not applicable
	ExitCode *int `json:"exitCode,omitempty" jsonschema:"description=Process exit code (null if not applicable),minimum=0"`
	// Log contains captured check output and log lines
	Log string `json:"log,omitempty" jsonschema:"description=Check output and logs"`
	// Error holds the error message when the check failed
	Error string `json:"error,omitempty" jsonschema:"description=Error message if check failed"`
	// Metadata holds additional key-value data attached to the result
	Metadata map[string]any `json:"metadata,omitempty" jsonschema:"description=Additional metadata"`
}

// GenerateComplianceResultSchema generates a JSON Schema for the simplified ComplianceCheckResult
func GenerateComplianceResultSchema() (*jsonschema.Schema, error) {
	reflector := &jsonschema.Reflector{
		AllowAdditionalProperties:  false,
		RequiredFromJSONSchemaTags: true,
		ExpandedStruct:             true,
	}

	// Generate schema for ComplianceCheckResult struct
	schema := reflector.Reflect(&ComplianceCheckResult{})

	// Add custom metadata
	schema.ID = "https://schemas.openlane.io/compliance-result/v1.0.0"
	schema.Title = "Openlane Compliance Check Result Schema"
	schema.Description = "Simplified schema for compliance check results"
	schema.Version = "https://json-schema.org/draft/2020-12/schema"

	// Add examples
	schema.Examples = []any{
		map[string]any{
			"checkName":  "AWS IAM Password Policy Check",
			"startedAt":  "2024-03-15T14:30:20Z",
			"finishedAt": "2024-03-15T14:30:22Z",
			"standard":   "soc2",
			"controlRef": "CC6.1",
			"status":     "SUCCESS",
			"exitCode":   0,
			"log":        "Password policy compliant: minimum length 14, complexity enabled",
		},
		map[string]any{
			"checkName":  "SSH Configuration Check",
			"startedAt":  "2024-03-15T14:32:10Z",
			"finishedAt": "2024-03-15T14:32:15Z",
			"standard":   "nist-csf",
			"controlRef": "PR.AC-4",
			"status":     "FAILED",
			"exitCode":   1,
			"log":        "SSH root login enabled in /etc/ssh/sshd_config",
			"error":      "Root login should be disabled for security",
			"metadata": map[string]any{
				"severity": "high",
				"file":     "/etc/ssh/sshd_config",
				"line":     exampleSSHConfigLine,
			},
		},
	}

	return schema, nil
}

// MarshalComplianceResult marshals a ComplianceCheckResult to JSON with proper formatting
func MarshalComplianceResult(result *ComplianceCheckResult) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

// UnmarshalComplianceResult unmarshals JSON to a ComplianceCheckResult
func UnmarshalComplianceResult(data []byte) (*ComplianceCheckResult, error) {
	var result ComplianceCheckResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ValidateComplianceResult validates a ComplianceCheckResult struct fields
func ValidateComplianceResult(result *ComplianceCheckResult) error {
	if result.CheckName == "" {
		return ErrCheckNameRequired
	}

	if result.Standard == "" {
		return ErrStandardRequired
	}

	if result.ControlRef == "" {
		return ErrControlRefRequired
	}

	if result.StartedAt.IsZero() {
		return ErrStartedAtRequired
	}

	if result.FinishedAt.IsZero() {
		return ErrFinishedAtRequired
	}

	// Validate status enum values
	validStatuses := []enums.JobExecutionStatus{
		enums.JobExecutionStatusSuccess,
		enums.JobExecutionStatusFailed,
		enums.JobExecutionStatusCanceled,
		enums.JobExecutionStatusPending,
	}
	statusValid := slices.Contains(validStatuses, result.Status)

	if !statusValid {
		return ErrInvalidStatus
	}

	return nil
}
