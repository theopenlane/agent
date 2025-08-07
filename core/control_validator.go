package core

import (
	"fmt"
	"regexp"
	"strings"
)

// ControlValidator handles validation of control references and formats
type ControlValidator struct {
	supportedFrameworks map[string]FrameworkSpec
}

// FrameworkSpec defines specifications for a compliance framework
type FrameworkSpec struct {
	Name             string
	ReferencePattern *regexp.Regexp
	Description      string
	Examples         []string
}

// ValidationResult represents the result of control validation
type ValidationResult struct {
	IsValid       bool     `json:"is_valid"`
	Framework     string   `json:"framework,omitempty"`
	ControlID     string   `json:"control_id,omitempty"`
	NormalizedRef string   `json:"normalized_ref,omitempty"`
	Errors        []string `json:"errors,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

// NewControlValidator creates a new control validator with standard framework support
func NewControlValidator() *ControlValidator {
	return &ControlValidator{
		supportedFrameworks: map[string]FrameworkSpec{
			"SOC2": {
				Name:             "SOC 2",
				ReferencePattern: regexp.MustCompile(`^(SOC2?):?([A-Z]{2}\d+(\.\d+)*)$`),
				Description:      "Service Organization Control 2",
				Examples:         []string{"SOC2:CC6.1", "SOC2:CC6.3", "SOC2:DC1.1"},
			},
			"ISO27001": {
				Name:             "ISO/IEC 27001",
				ReferencePattern: regexp.MustCompile(`^(ISO27001):?([A-Z]\.\d+(\.\d+)*)$`),
				Description:      "Information Security Management System",
				Examples:         []string{"ISO27001:A.9.2.1", "ISO27001:A.12.1.2"},
			},
			"PCI-DSS": {
				Name:             "Payment Card Industry Data Security Standard",
				ReferencePattern: regexp.MustCompile(`^(PCI[-_]?DSS):?(\d+(\.\d+)*)$`),
				Description:      "Payment Card Industry Data Security Standard",
				Examples:         []string{"PCI-DSS:8.2", "PCI-DSS:12.1.1"},
			},
			"NIST": {
				Name:             "NIST Cybersecurity Framework",
				ReferencePattern: regexp.MustCompile(`^(NIST):?([A-Z]{2}[-.]?[A-Z]{2}[-.]?\d+(\.\d+)*)$`),
				Description:      "National Institute of Standards and Technology",
				Examples:         []string{"NIST:AC-01", "NIST:SC-7"},
			},
			"GDPR": {
				Name:             "General Data Protection Regulation",
				ReferencePattern: regexp.MustCompile(`^(GDPR):?(Art\.\s?\d+|Article\s?\d+|\d+(\.\d+)*)$`),
				Description:      "European Union General Data Protection Regulation",
				Examples:         []string{"GDPR:Art.32", "GDPR:Article.25"},
			},
			"HIPAA": {
				Name:             "Health Insurance Portability and Accountability Act",
				ReferencePattern: regexp.MustCompile(`^(HIPAA):?(\d+\.\d+\([a-z]\)(\(\d+\))?)$`),
				Description:      "Health Insurance Portability and Accountability Act",
				Examples:         []string{"HIPAA:164.312(a)(1)", "HIPAA:164.306(a)"},
			},
		},
	}
}

// ValidateControlReference validates a single control reference
func (v *ControlValidator) ValidateControlReference(controlRef string) ValidationResult {
	result := ValidationResult{
		IsValid:   false,
		Errors:    make([]string, 0),
		Warnings:  make([]string, 0),
	}

	// Basic format validation
	if controlRef == "" {
		result.Errors = append(result.Errors, "control reference cannot be empty")
		return result
	}

	// Normalize whitespace
	normalizedRef := strings.TrimSpace(controlRef)
	result.NormalizedRef = normalizedRef

	// Check if it contains a framework separator
	if !strings.Contains(normalizedRef, ":") {
		result.Errors = append(result.Errors, "control reference must contain framework separator (:)")
		return result
	}

	// Extract framework and control ID
	parts := strings.SplitN(normalizedRef, ":", 2)
	if len(parts) != 2 {
		result.Errors = append(result.Errors, "invalid control reference format")
		return result
	}

	frameworkName := strings.ToUpper(strings.TrimSpace(parts[0]))
	controlID := strings.TrimSpace(parts[1])

	result.Framework = frameworkName
	result.ControlID = controlID

	// Validate framework name is not empty
	if frameworkName == "" {
		result.Errors = append(result.Errors, "framework name cannot be empty")
		return result
	}

	// Validate control ID is not empty
	if controlID == "" {
		result.Errors = append(result.Errors, "control ID cannot be empty")
		return result
	}

	// Validate framework
	frameworkSpec, exists := v.supportedFrameworks[frameworkName]
	if !exists {
		result.Warnings = append(result.Warnings, fmt.Sprintf("framework '%s' is not in the list of well-known frameworks", frameworkName))
		// Still allow it as valid for custom frameworks
		result.IsValid = true
		return result
	}

	// Validate against framework pattern
	fullRef := fmt.Sprintf("%s:%s", frameworkName, controlID)
	if !frameworkSpec.ReferencePattern.MatchString(fullRef) {
		result.Errors = append(result.Errors, fmt.Sprintf("control ID '%s' does not match expected pattern for %s framework", controlID, frameworkSpec.Name))
		result.Errors = append(result.Errors, fmt.Sprintf("expected format examples: %s", strings.Join(frameworkSpec.Examples, ", ")))
		return result
	}

	result.IsValid = true
	return result
}

// ValidateControlList validates a list of control references
func (v *ControlValidator) ValidateControlList(controlRefs []string) map[string]ValidationResult {
	results := make(map[string]ValidationResult)

	for _, controlRef := range controlRefs {
		results[controlRef] = v.ValidateControlReference(controlRef)
	}

	return results
}

// GetSupportedFrameworks returns information about supported frameworks
func (v *ControlValidator) GetSupportedFrameworks() map[string]FrameworkSpec {
	return v.supportedFrameworks
}

// AddFramework adds a custom framework specification
func (v *ControlValidator) AddFramework(name string, spec FrameworkSpec) {
	v.supportedFrameworks[strings.ToUpper(name)] = spec
}

// NormalizeControlReference normalizes a control reference to standard format
func (v *ControlValidator) NormalizeControlReference(controlRef string) string {
	validation := v.ValidateControlReference(controlRef)
	if validation.IsValid {
		return validation.NormalizedRef
	}
	return controlRef
}

// GetValidationSummary returns a summary of validation results
func (v *ControlValidator) GetValidationSummary(results map[string]ValidationResult) ValidationSummary {
	summary := ValidationSummary{
		TotalControls:    len(results),
		ValidControls:    0,
		InvalidControls:  0,
		WarningControls:  0,
		FrameworkCounts:  make(map[string]int),
		CommonErrors:     make(map[string]int),
		CommonWarnings:   make(map[string]int),
	}

	for _, result := range results {
		if result.IsValid {
			summary.ValidControls++
			if result.Framework != "" {
				summary.FrameworkCounts[result.Framework]++
			}
		} else {
			summary.InvalidControls++
		}

		if len(result.Warnings) > 0 {
			summary.WarningControls++
		}

		// Count common errors and warnings
		for _, err := range result.Errors {
			summary.CommonErrors[err]++
		}
		for _, warn := range result.Warnings {
			summary.CommonWarnings[warn]++
		}
	}

	return summary
}

// ValidationSummary provides an overview of validation results
type ValidationSummary struct {
	TotalControls    int            `json:"total_controls"`
	ValidControls    int            `json:"valid_controls"`
	InvalidControls  int            `json:"invalid_controls"`
	WarningControls  int            `json:"warning_controls"`
	FrameworkCounts  map[string]int `json:"framework_counts"`
	CommonErrors     map[string]int `json:"common_errors"`
	CommonWarnings   map[string]int `json:"common_warnings"`
}

// IsHealthy returns true if the validation summary indicates healthy control references
func (s *ValidationSummary) IsHealthy() bool {
	if s.TotalControls == 0 {
		return true // No controls to validate
	}

	// Consider healthy if at least 90% of controls are valid
	validPercentage := float64(s.ValidControls) / float64(s.TotalControls)
	return validPercentage >= 0.9
}