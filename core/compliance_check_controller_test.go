package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theopenlane/core/common/enums"

	"github.com/theopenlane/agent/config"
)

func TestIsValidSeverity(t *testing.T) {
	c := &ComplianceCheckController{}

	validSeverities := []config.FindingSeverity{
		config.SeverityCritical,
		config.SeverityHigh,
		config.SeverityMedium,
		config.SeverityLow,
		config.SeverityInfo,
	}

	for _, sev := range validSeverities {
		assert.True(t, c.isValidSeverity(sev), "expected %s to be valid", sev)
	}

	assert.False(t, c.isValidSeverity("unknown"))
	assert.False(t, c.isValidSeverity(""))
}

func TestDetermineStatus(t *testing.T) {
	c := &ComplianceCheckController{}

	exitZero := 0
	exitNonZero := 1

	tests := []struct {
		name   string
		result *config.Result
		want   enums.JobExecutionStatus
	}{
		{
			name:   "already failed status is preserved",
			result: &config.Result{Status: enums.JobExecutionStatusFailed},
			want:   enums.JobExecutionStatusFailed,
		},
		{
			name:   "non-empty error string means failed",
			result: &config.Result{Error: "something went wrong"},
			want:   enums.JobExecutionStatusFailed,
		},
		{
			name:   "non-zero exit code means failed",
			result: &config.Result{ExitCode: &exitNonZero},
			want:   enums.JobExecutionStatusFailed,
		},
		{
			name:   "zero exit code with no error means success",
			result: &config.Result{ExitCode: &exitZero},
			want:   enums.JobExecutionStatusSuccess,
		},
		{
			name:   "nil exit code with no error means success",
			result: &config.Result{},
			want:   enums.JobExecutionStatusSuccess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, c.determineStatus(tt.result))
		})
	}
}

func TestParseCheckOutput_emptyOutput(t *testing.T) {
	c := &ComplianceCheckController{}
	result := &config.Result{}

	err := c.parseCheckOutput("", result)
	require.NoError(t, err)
	assert.Empty(t, result.Log)
}

func TestParseCheckOutput_nonJSONOutput(t *testing.T) {
	c := &ComplianceCheckController{}
	result := &config.Result{}

	raw := "plain text output from script"
	err := c.parseCheckOutput(raw, result)
	require.NoError(t, err)
	assert.Equal(t, raw, result.Log)
}

func TestParseCheckOutput_validJSON(t *testing.T) {
	c := &ComplianceCheckController{}
	result := &config.Result{Metadata: make(map[string]any)}

	jsonOutput := `{
		"findings": [
			{
				"resource": "iam-role",
				"title": "Overly permissive role",
				"severity": "high",
				"status": "open"
			}
		]
	}`

	err := c.parseCheckOutput(jsonOutput, result)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Log)
	assert.Contains(t, result.Metadata, "findings")
}

func TestParseCheckOutput_defaultsApplied(t *testing.T) {
	c := &ComplianceCheckController{}
	result := &config.Result{Metadata: make(map[string]any)}

	// Findings with missing severity and status should get defaults applied
	jsonOutput := `{"findings": [{"resource": "disk", "title": "Check"}]}`
	err := c.parseCheckOutput(jsonOutput, result)
	require.NoError(t, err)

	findings, ok := result.Metadata["findings"].([]config.Finding)
	require.True(t, ok)
	require.Len(t, findings, 1)
	assert.Equal(t, config.StatusOpen, findings[0].Status)
	assert.Equal(t, config.SeverityMedium, findings[0].Severity)
}

func TestSelectPrimaryComplianceReference(t *testing.T) {
	t.Run("nil check returns empty strings", func(t *testing.T) {
		std, ctrl := selectPrimaryComplianceReference(nil, nil)
		assert.Empty(t, std)
		assert.Empty(t, ctrl)
	})

	t.Run("picks first validated control", func(t *testing.T) {
		check := &config.Check{
			ComplianceStandards: []config.ComplianceStandard{
				{Standard: "soc2v2022", Controls: []string{"CC6.1"}},
			},
		}

		validated := map[string][]string{
			"soc2v2022": {"CC6.1"},
		}

		std, ctrl := selectPrimaryComplianceReference(check, validated)
		assert.Equal(t, "soc2v2022", std)
		assert.Equal(t, "CC6.1", ctrl)
	})

	t.Run("falls back to configured controls when none validated", func(t *testing.T) {
		check := &config.Check{
			ComplianceStandards: []config.ComplianceStandard{
				{Standard: "soc2v2022", Controls: []string{"CC6.2"}},
			},
		}

		std, ctrl := selectPrimaryComplianceReference(check, map[string][]string{})
		assert.Equal(t, "soc2v2022", std)
		assert.Equal(t, "CC6.2", ctrl)
	})
}
