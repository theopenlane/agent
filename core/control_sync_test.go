package core

import (
	"context"
	"testing"
	"time"

	"github.com/theopenlane/agent/internal/config"
	"github.com/theopenlane/core/pkg/openlaneclient"
)

// ControlClient interface for dependency injection in tests
type ControlClient interface {
	GetAllControls(ctx context.Context) ([]*openlaneclient.Control, error)
	CreateControl(ctx context.Context, input openlaneclient.CreateControlInput) (*openlaneclient.Control, error)
	UpdateControl(ctx context.Context, controlID string, input openlaneclient.UpdateControlInput) error
}

// MockGraphQLClient implements a mock version of the GraphQL client for testing
type MockGraphQLClient struct {
	controls     []*openlaneclient.Control
	createCalls  []openlaneclient.CreateControlInput
	updateCalls  map[string]openlaneclient.UpdateControlInput
}

// GetAllControls returns mock controls
func (m *MockGraphQLClient) GetAllControls(ctx context.Context) ([]*openlaneclient.Control, error) {
	return m.controls, nil
}

// CreateControl mocks control creation
func (m *MockGraphQLClient) CreateControl(ctx context.Context, input openlaneclient.CreateControlInput) (*openlaneclient.Control, error) {
	m.createCalls = append(m.createCalls, input)
	
	// Generate a mock control
	now := time.Now()
	control := &openlaneclient.Control{
		ID:        "mock-id-" + string(rune(len(m.createCalls))),
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	
	m.controls = append(m.controls, control)
	return control, nil
}

// UpdateControl mocks control updates
func (m *MockGraphQLClient) UpdateControl(ctx context.Context, controlID string, input openlaneclient.UpdateControlInput) error {
	if m.updateCalls == nil {
		m.updateCalls = make(map[string]openlaneclient.UpdateControlInput)
	}
	m.updateCalls[controlID] = input
	
	// Find and update the control
	for _, control := range m.controls {
		if control.ID == controlID {
			now := time.Now()
			control.UpdatedAt = &now
			return nil
		}
	}
	
	return nil
}

func TestControlSyncService_ValidateControls(t *testing.T) {
	validator := NewControlValidator()
	
	testCases := []struct {
		name        string
		controlRef  string
		expectValid bool
		expectWarn  bool
	}{
		{
			name:        "Valid SOC2 Control",
			controlRef:  "SOC2:CC6.1",
			expectValid: true,
			expectWarn:  false,
		},
		{
			name:        "Valid ISO27001 Control",
			controlRef:  "ISO27001:A.9.2.1",
			expectValid: true,
			expectWarn:  false,
		},
		{
			name:        "Valid PCI-DSS Control",
			controlRef:  "PCI-DSS:8.2",
			expectValid: true,
			expectWarn:  false,
		},
		{
			name:        "Invalid Format - No Colon",
			controlRef:  "SOC2CC61",
			expectValid: false,
			expectWarn:  false,
		},
		{
			name:        "Invalid Format - Missing Framework",
			controlRef:  ":CC6.1",
			expectValid: false,
			expectWarn:  false,
		},
		{
			name:        "Invalid Format - Missing Control ID",
			controlRef:  "SOC2:",
			expectValid: false,
			expectWarn:  false,
		},
		{
			name:        "Custom Framework - Should Warn",
			controlRef:  "CUSTOM:SEC-001",
			expectValid: true,
			expectWarn:  true,
		},
		{
			name:        "Empty Control Reference",
			controlRef:  "",
			expectValid: false,
			expectWarn:  false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := validator.ValidateControlReference(tc.controlRef)
			
			if result.IsValid != tc.expectValid {
				t.Errorf("Expected IsValid=%v, got %v for control %s", tc.expectValid, result.IsValid, tc.controlRef)
			}
			
			hasWarnings := len(result.Warnings) > 0
			if hasWarnings != tc.expectWarn {
				t.Errorf("Expected warnings=%v, got %v for control %s. Warnings: %v", tc.expectWarn, hasWarnings, tc.controlRef, result.Warnings)
			}
			
			if !result.IsValid && len(result.Errors) == 0 {
				t.Errorf("Invalid control should have errors: %s", tc.controlRef)
			}
		})
	}
}

func TestControlSyncService_ExtractControlsFromConfig(t *testing.T) {
	mockClient := &MockGraphQLClient{
		controls: []*openlaneclient.Control{},
	}
	
	service := NewControlSyncService(mockClient, "test-agent", "test-org")
	
	// Create test configuration
	cfg := &config.Config{
		Checks: []config.Check{
			{
				Name:        "check1",
				Controls:    []string{"SOC2:CC6.1", "ISO27001:A.9.2.1"},
			},
			{
				Name:        "check2", 
				Controls:    []string{"PCI-DSS:8.2", "SOC2:CC6.1"}, // Duplicate SOC2:CC6.1
			},
			{
				Name:        "check3",
				Controls:    []string{}, // No controls
			},
		},
	}
	
	controls := service.extractControlsFromConfig(cfg)
	
	// Should have 3 unique controls (SOC2:CC6.1 should be deduplicated)
	expectedCount := 3
	if len(controls) != expectedCount {
		t.Errorf("Expected %d unique controls, got %d: %v", expectedCount, len(controls), controls)
	}
	
	// Check that all expected controls are present
	expectedControls := map[string]bool{
		"SOC2:CC6.1":       false,
		"ISO27001:A.9.2.1": false,
		"PCI-DSS:8.2":      false,
	}
	
	for _, control := range controls {
		if _, exists := expectedControls[control]; exists {
			expectedControls[control] = true
		} else {
			t.Errorf("Unexpected control found: %s", control)
		}
	}
	
	for control, found := range expectedControls {
		if !found {
			t.Errorf("Expected control not found: %s", control)
		}
	}
}

func TestControlSyncService_MatchControls(t *testing.T) {
	// Create mock Openlane controls with IDs that contain the control references
	now := time.Now()
	soc2Control := &openlaneclient.Control{
		ID:        "soc2-cc61-control-id",
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	
	iso27001Control := &openlaneclient.Control{
		ID:        "iso27001-a921-control-id",
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	
	mockClient := &MockGraphQLClient{
		controls: []*openlaneclient.Control{soc2Control, iso27001Control},
	}
	
	service := NewControlSyncService(mockClient, "test-agent", "test-org")
	
	testCases := []struct {
		name               string
		agentControl       string
		expectedMatch      bool
		minConfidence      float64
	}{
		{
			name:          "SOC2 Framework Match",
			agentControl:  "SOC2:CC6.1",
			expectedMatch: true,
			minConfidence: 0.5,
		},
		{
			name:          "ISO27001 Framework Match",
			agentControl:  "ISO27001:A.9.2.1",
			expectedMatch: true,
			minConfidence: 0.5,
		},
		{
			name:          "No Match - Different Framework",
			agentControl:  "NIST:AC-01",
			expectedMatch: false,
			minConfidence: 0.0,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			match := service.findBestMatch(tc.agentControl, mockClient.controls)
			
			hasMatch := match != nil
			if hasMatch != tc.expectedMatch {
				t.Errorf("Expected match=%v, got %v for control %s", tc.expectedMatch, hasMatch, tc.agentControl)
			}
			
			if hasMatch && match.MatchConfidence < tc.minConfidence {
				t.Errorf("Expected confidence >= %.1f, got %.1f for control %s", tc.minConfidence, match.MatchConfidence, tc.agentControl)
			}
		})
	}
}

func TestControlSyncService_SyncControlsFromConfig(t *testing.T) {
	// Create existing Openlane control with ID that matches SOC2 pattern
	now := time.Now()
	existingControl := &openlaneclient.Control{
		ID:        "soc2-cc61-existing-control",
		CreatedAt: &now,
		UpdatedAt: &now,
	}
	
	mockClient := &MockGraphQLClient{
		controls: []*openlaneclient.Control{existingControl},
	}
	
	service := NewControlSyncService(mockClient, "test-agent", "test-org")
	
	// Create test configuration with mix of existing and new controls
	cfg := &config.Config{
		Checks: []config.Check{
			{
				Name:     "test-check",
				Controls: []string{"SOC2:CC6.1", "ISO27001:A.9.2.1", "PCI-DSS:8.2"},
			},
		},
	}
	
	ctx := context.Background()
	result, err := service.SyncControlsFromConfig(ctx, cfg)
	
	if err != nil {
		t.Fatalf("SyncControlsFromConfig failed: %v", err)
	}
	
	// Verify results
	if result.TotalAgentControls != 3 {
		t.Errorf("Expected 3 agent controls, got %d", result.TotalAgentControls)
	}
	
	if result.TotalOpenlaneControls != 1 {
		t.Errorf("Expected 1 Openlane control initially, got %d", result.TotalOpenlaneControls)
	}
	
	// Should have some matched controls and some new controls
	totalProcessed := len(result.MatchedControls) + len(result.NewControls)
	if totalProcessed < 2 {
		t.Errorf("Expected at least 2 controls processed, got %d matched + %d new = %d total", 
			len(result.MatchedControls), len(result.NewControls), totalProcessed)
	}
	
	// Verify mock client received create calls for new controls
	expectedCreateCalls := len(result.NewControls)
	if len(mockClient.createCalls) != expectedCreateCalls {
		t.Errorf("Expected %d create calls, got %d", expectedCreateCalls, len(mockClient.createCalls))
	}
}

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}