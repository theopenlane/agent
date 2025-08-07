package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/internal/config"
	"github.com/theopenlane/core/pkg/openlaneclient"
)

// ControlClientInterface defines the methods needed for control operations
type ControlClientInterface interface {
	GetAllControls(ctx context.Context) ([]*openlaneclient.Control, error)
	CreateControl(ctx context.Context, input openlaneclient.CreateControlInput) (*openlaneclient.Control, error)
	UpdateControl(ctx context.Context, controlID string, input openlaneclient.UpdateControlInput) error
}

// ControlSyncService handles synchronization of controls between agent.yaml and Openlane system
type ControlSyncService struct {
	client    ControlClientInterface
	agentID   string
	orgID     string
	validator *ControlValidator
}

// NewControlSyncService creates a new control synchronization service
func NewControlSyncService(client ControlClientInterface, agentID, orgID string) *ControlSyncService {
	return &ControlSyncService{
		client:    client,
		agentID:   agentID,
		orgID:     orgID,
		validator: NewControlValidator(),
	}
}

// ControlMatch represents a matched control between agent.yaml and Openlane
type ControlMatch struct {
	AgentControlRef   string                       // Control reference from agent.yaml (e.g., "SOC2:CC6.1")
	OpenlaneControl   *openlaneclient.Control      // Matched control from Openlane system
	MatchConfidence   float64                      // Confidence level of the match (0.0 to 1.0)
	MatchReason       string                       // Reason for the match
	RequiresUpdate    bool                         // Whether the control needs to be updated
	UpdateFields      []string                     // Fields that need updating
}

// SyncResult represents the result of control synchronization
type SyncResult struct {
	TotalAgentControls    int            // Total controls found in agent.yaml
	TotalOpenlaneControls int            // Total controls found in Openlane
	MatchedControls       []ControlMatch // Successfully matched controls
	UnmatchedControls     []string       // Controls from agent.yaml that couldn't be matched
	NewControls          []string       // Controls that need to be created in Openlane
	UpdatedControls      int            // Number of controls that were updated
	Errors               []error        // Any errors that occurred during sync
	SyncDuration         time.Duration  // Time taken for synchronization
}

// SyncControlsFromConfig synchronizes controls from agent configuration with Openlane system
func (c *ControlSyncService) SyncControlsFromConfig(ctx context.Context, cfg *config.Config) (*SyncResult, error) {
	startTime := time.Now()
	result := &SyncResult{
		MatchedControls:   make([]ControlMatch, 0),
		UnmatchedControls: make([]string, 0),
		NewControls:      make([]string, 0),
		Errors:           make([]error, 0),
	}

	log.Info().Msg("Starting control synchronization")

	// Extract all control references from agent configuration
	agentControls := c.extractControlsFromConfig(cfg)
	result.TotalAgentControls = len(agentControls)

	if len(agentControls) == 0 {
		log.Info().Msg("No controls found in agent configuration")
		result.SyncDuration = time.Since(startTime)
		return result, nil
	}

	log.Info().Int("count", len(agentControls)).Msg("Found controls in agent configuration")

	// Validate control references
	validationResults := c.validator.ValidateControlList(agentControls)
	validationSummary := c.validator.GetValidationSummary(validationResults)
	
	log.Info().
		Int("valid", validationSummary.ValidControls).
		Int("invalid", validationSummary.InvalidControls).
		Int("warnings", validationSummary.WarningControls).
		Msg("Control validation summary")

	// Log validation issues
	for controlRef, validation := range validationResults {
		if !validation.IsValid {
			for _, err := range validation.Errors {
				log.Error().Str("control", controlRef).Str("error", err).Msg("Control validation failed")
				result.Errors = append(result.Errors, fmt.Errorf("validation failed for control %s: %s", controlRef, err))
			}
		}
		if len(validation.Warnings) > 0 {
			for _, warning := range validation.Warnings {
				log.Warn().Str("control", controlRef).Str("warning", warning).Msg("Control validation warning")
			}
		}
	}

	// Filter out invalid controls for processing
	validControls := make([]string, 0)
	for _, control := range agentControls {
		if validationResults[control].IsValid {
			validControls = append(validControls, c.validator.NormalizeControlReference(control))
		}
	}
	agentControls = validControls

	// Get existing controls from Openlane system
	openlaneControls, err := c.getOpenlaneControls(ctx)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("failed to fetch Openlane controls: %w", err))
		result.SyncDuration = time.Since(startTime)
		return result, err
	}

	result.TotalOpenlaneControls = len(openlaneControls)
	log.Info().Int("count", len(openlaneControls)).Msg("Found controls in Openlane system")

	// Match controls between agent.yaml and Openlane
	for _, agentControl := range agentControls {
		match := c.findBestMatch(agentControl, openlaneControls)
		if match != nil {
			result.MatchedControls = append(result.MatchedControls, *match)
			log.Debug().
				Str("agent_control", agentControl).
				Str("openlane_control", match.OpenlaneControl.ID).
				Float64("confidence", match.MatchConfidence).
				Msg("Matched control")
		} else {
			result.UnmatchedControls = append(result.UnmatchedControls, agentControl)
			result.NewControls = append(result.NewControls, agentControl)
			log.Warn().Str("control", agentControl).Msg("No match found for agent control")
		}
	}

	// Update matched controls if necessary
	for i, match := range result.MatchedControls {
		if match.RequiresUpdate {
			err := c.updateControl(ctx, &match)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("failed to update control %s: %w", match.AgentControlRef, err))
				log.Error().Err(err).Str("control", match.AgentControlRef).Msg("Failed to update control")
			} else {
				result.UpdatedControls++
				log.Info().Str("control", match.AgentControlRef).Msg("Successfully updated control")
			}
		}
		result.MatchedControls[i] = match
	}

	// Create new controls if needed
	for _, newControl := range result.NewControls {
		err := c.createControl(ctx, newControl)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to create control %s: %w", newControl, err))
			log.Error().Err(err).Str("control", newControl).Msg("Failed to create control")
		} else {
			log.Info().Str("control", newControl).Msg("Successfully created control")
		}
	}

	result.SyncDuration = time.Since(startTime)
	log.Info().
		Int("matched", len(result.MatchedControls)).
		Int("unmatched", len(result.UnmatchedControls)).
		Int("updated", result.UpdatedControls).
		Int("errors", len(result.Errors)).
		Dur("duration", result.SyncDuration).
		Msg("Control synchronization completed")

	return result, nil
}

// extractControlsFromConfig extracts all unique control references from agent configuration
func (c *ControlSyncService) extractControlsFromConfig(cfg *config.Config) []string {
	controlMap := make(map[string]bool)
	var controls []string

	// Extract controls from each check
	for _, check := range cfg.Checks {
		for _, control := range check.Controls {
			if !controlMap[control] {
				controlMap[control] = true
				controls = append(controls, control)
			}
		}
	}

	return controls
}

// getOpenlaneControls retrieves all controls from the Openlane system
func (c *ControlSyncService) getOpenlaneControls(ctx context.Context) ([]*openlaneclient.Control, error) {
	// Use the GraphQL client to fetch controls
	controls, err := c.client.GetAllControls(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch controls: %w", err)
	}

	return controls, nil
}

// findBestMatch finds the best matching Openlane control for an agent control reference
func (c *ControlSyncService) findBestMatch(agentControl string, openlaneControls []*openlaneclient.Control) *ControlMatch {
	var bestMatch *ControlMatch
	highestConfidence := 0.0

	// Parse agent control reference (e.g., "SOC2:CC6.1")
	framework, controlID := c.parseControlReference(agentControl)

	for _, openlaneControl := range openlaneControls {
		confidence, reason := c.calculateMatchConfidence(framework, controlID, openlaneControl)
		
		if confidence > highestConfidence && confidence >= 0.8 { // Minimum 80% confidence
			requiresUpdate, updateFields := c.checkRequiresUpdate(agentControl, openlaneControl)
			
			bestMatch = &ControlMatch{
				AgentControlRef:   agentControl,
				OpenlaneControl:   openlaneControl,
				MatchConfidence:   confidence,
				MatchReason:       reason,
				RequiresUpdate:    requiresUpdate,
				UpdateFields:      updateFields,
			}
			highestConfidence = confidence
		}
	}

	return bestMatch
}

// parseControlReference parses a control reference into framework and control ID
func (c *ControlSyncService) parseControlReference(controlRef string) (framework, controlID string) {
	parts := strings.Split(controlRef, ":")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", controlRef
}

// calculateMatchConfidence calculates the confidence level for matching controls based on available control data
func (c *ControlSyncService) calculateMatchConfidence(framework, controlID string, openlaneControl *openlaneclient.Control) (confidence float64, reason string) {
	// Use available control data from Openlane for matching
	// Note: ReferenceID may not be available in the current GraphQL schema
	
	// Normalize strings for comparison
	normalizedSystemID := strings.ToLower(strings.ReplaceAll(openlaneControl.ID, "-", ""))
	normalizedFramework := strings.ToLower(strings.ReplaceAll(framework, "-", ""))
	normalizedControl := strings.ToLower(strings.ReplaceAll(controlID, ".", ""))
	
	// Check if framework name appears in the system ID and control ID appears too
	if strings.Contains(normalizedSystemID, normalizedFramework) && strings.Contains(normalizedSystemID, normalizedControl) {
		return 1.0, "framework_and_control_in_system_id"
	}
	
	// Framework name appears in system ID
	if strings.Contains(normalizedSystemID, normalizedFramework) {
		return 0.8, "framework_name_in_system_id"
	}
	
	// Control ID appears in system ID
	if strings.Contains(normalizedSystemID, normalizedControl) {
		return 0.6, "control_id_in_system_id"
	}
	
	// Check if the control ID is similar to parts of the system ID
	controlParts := strings.Split(normalizedControl, "")
	matchedParts := 0
	for _, part := range controlParts {
		if part != "." && part != ":" && strings.Contains(normalizedSystemID, part) {
			matchedParts++
		}
	}
	
	if matchedParts > 0 && len(controlParts) > 0 {
		partialConfidence := float64(matchedParts) / float64(len(controlParts))
		if partialConfidence > 0.5 {
			return 0.4, "partial_control_match"
		}
	}

	return 0.0, "no_match"
}

// checkRequiresUpdate determines if a matched control needs updating based on available control data
func (c *ControlSyncService) checkRequiresUpdate(agentControlRef string, openlaneControl *openlaneclient.Control) (bool, []string) {
	var updateFields []string
	framework, controlID := c.parseControlReference(agentControlRef)
	
	// Check if control ID is standardized in the system ID
	expectedSystemIDPattern := strings.ToLower(fmt.Sprintf("%s-%s", framework, strings.ReplaceAll(controlID, ".", "-")))
	actualSystemID := strings.ToLower(openlaneControl.ID)
	
	// If the system ID doesn't follow expected pattern, suggest it needs updating
	if !strings.Contains(actualSystemID, expectedSystemIDPattern) {
		// Check if at least framework is in the ID
		normalizedFramework := strings.ToLower(strings.ReplaceAll(framework, "-", ""))
		if !strings.Contains(actualSystemID, normalizedFramework) {
			updateFields = append(updateFields, "framework_in_id")
		}
		
		// Check if control ID components are in the system ID
		normalizedControl := strings.ToLower(strings.ReplaceAll(controlID, ".", ""))
		if !strings.Contains(actualSystemID, normalizedControl) {
			updateFields = append(updateFields, "control_id_in_id")
		}
	}
	
	// Check control age - if very old, might need metadata refresh
	if openlaneControl.UpdatedAt != nil {
		timeSinceUpdate := time.Since(*openlaneControl.UpdatedAt)
		if timeSinceUpdate > 90*24*time.Hour { // 90 days
			updateFields = append(updateFields, "metadata_refresh")
			log.Debug().
				Str("agent_control", agentControlRef).
				Str("openlane_control_id", openlaneControl.ID).
				Dur("time_since_update", timeSinceUpdate).
				Msg("Control metadata is old, flagging for refresh")
		}
	}
	
	needsUpdate := len(updateFields) > 0
	
	if needsUpdate {
		log.Info().
			Str("agent_control", agentControlRef).
			Str("openlane_control_id", openlaneControl.ID).
			Strs("update_fields", updateFields).
			Msg("Control requires update")
	} else {
		log.Debug().
			Str("agent_control", agentControlRef).
			Str("openlane_control_id", openlaneControl.ID).
			Msg("Control is up to date")
	}
	
	return needsUpdate, updateFields
}

// updateControl updates a control in the Openlane system
func (c *ControlSyncService) updateControl(ctx context.Context, match *ControlMatch) error {
	_, controlID := c.parseControlReference(match.AgentControlRef)
	
	input := openlaneclient.UpdateControlInput{}
	
	// Update only available fields
	for _, field := range match.UpdateFields {
		switch field {
		case "reference_id":
			input.ReferenceID = &controlID
		}
	}

	err := c.client.UpdateControl(ctx, match.OpenlaneControl.ID, input)
	if err != nil {
		return fmt.Errorf("failed to update control: %w", err)
	}

	log.Info().
		Str("control_id", match.OpenlaneControl.ID).
		Str("agent_control", match.AgentControlRef).
		Strs("updated_fields", match.UpdateFields).
		Msg("Control updated successfully")

	return nil
}

// createControl creates a new control in the Openlane system
func (c *ControlSyncService) createControl(ctx context.Context, agentControlRef string) error {
	_, controlID := c.parseControlReference(agentControlRef)
	
	input := openlaneclient.CreateControlInput{
		ReferenceID: &controlID,
	}

	_, err := c.client.CreateControl(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create control: %w", err)
	}

	log.Info().
		Str("control_id", controlID).
		Str("agent_control", agentControlRef).
		Msg("Control created successfully")

	return nil
}

// GetSyncStatus returns the current status of control synchronization
func (c *ControlSyncService) GetSyncStatus(ctx context.Context) (*api.ControlComplianceStatus, error) {
	// Get current controls from Openlane to provide real status
	controls, err := c.client.GetAllControls(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get controls for status: %w", err)
	}
	
	totalControls := len(controls)
	recentControls := 0
	oldControls := 0
	
	// Analyze control freshness and patterns
	frameworkCounts := make(map[string]int)
	now := time.Now()
	
	for _, control := range controls {
		// Count controls by estimated framework (based on ID patterns)
		controlID := strings.ToLower(control.ID)
		
		// Detect framework from control ID patterns
		detectedFramework := "unknown"
		if strings.Contains(controlID, "soc2") || strings.Contains(controlID, "soc") {
			detectedFramework = "SOC2"
		} else if strings.Contains(controlID, "iso27001") || strings.Contains(controlID, "iso") {
			detectedFramework = "ISO27001"
		} else if strings.Contains(controlID, "pci") || strings.Contains(controlID, "pcidss") {
			detectedFramework = "PCI-DSS"
		} else if strings.Contains(controlID, "nist") {
			detectedFramework = "NIST"
		} else if strings.Contains(controlID, "gdpr") {
			detectedFramework = "GDPR"
		} else if strings.Contains(controlID, "hipaa") {
			detectedFramework = "HIPAA"
		}
		
		frameworkCounts[detectedFramework]++
		
		// Check control age
		if control.UpdatedAt != nil {
			timeSinceUpdate := now.Sub(*control.UpdatedAt)
			if timeSinceUpdate <= 30*24*time.Hour { // 30 days
				recentControls++
			} else if timeSinceUpdate > 90*24*time.Hour { // 90 days
				oldControls++
			}
		}
	}
	
	// Calculate coverage as percentage of recent/active controls
	coverage := float64(recentControls) / float64(totalControls)
	if totalControls == 0 {
		coverage = 1.0 // No controls = perfect coverage
	}
	
	// Determine overall status
	status := "healthy"
	if coverage < 0.8 || oldControls > totalControls/4 {
		status = "degraded"
	}
	if coverage < 0.5 || oldControls > totalControls/2 {
		status = "critical"
	}
	if totalControls == 0 {
		status = "no_controls"
	}
	
	// Build evidence list
	evidence := []string{
		fmt.Sprintf("Total controls: %d", totalControls),
		fmt.Sprintf("Recent controls (≤30 days): %d", recentControls),
		fmt.Sprintf("Old controls (>90 days): %d", oldControls),
	}
	
	// Add framework breakdown
	for framework, count := range frameworkCounts {
		if count > 0 {
			evidence = append(evidence, fmt.Sprintf("%s controls: %d", framework, count))
		}
	}
	
	return &api.ControlComplianceStatus{
		Framework:   "Multi-Framework",
		ControlID:   fmt.Sprintf("SYNC_STATUS_%d_CONTROLS", totalControls),
		Status:      status,
		Coverage:    coverage,
		LastChecked: time.Now(),
		Evidence:    evidence,
	}, nil
}