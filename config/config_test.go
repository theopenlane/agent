package config

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		config    *Config
		expectErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				APIToken:       "test-token",
				APIURL:         "https://api.theopenlane.io",
				AgentName:      "test-agent",
				LogLevel:       "info",
				DataDir:        "/tmp/test-data",
				PollInterval:   1 * time.Minute,
				Spawn:          1,
				MaxConcurrency: 3,
				DefaultTimeout: 5 * time.Minute,
				Evidence: EvidenceConfig{
					Enabled:         true,
					RetentionPeriod: 30 * 24 * time.Hour,
					MaxFileSize:     100 * 1024 * 1024,
					CompressFiles:   false,
				},
				Checks: []Check{
					{
						Name:        "test-check",
						Description: "A test check",
						Command:     "echo",
						Args:        []string{"hello"},
						Schedule:    "* * * * *",
						Timeout:     1 * time.Minute,
						ComplianceStandards: []ComplianceStandard{
							{
								Standard: "test",
								Controls: []string{"001"},
							},
						},
						Tags:          []string{"test"},
						Enabled:       true,
						EvidencePaths: []string{"/tmp/evidence"},
						OnPass: &ActionConfig{
							UploadEvidence: true,
						},
						OnFail: &ActionConfig{
							UploadEvidence: true,
							Commands: []ActionCommand{
								{
									Name:    "alert",
									Command: "echo",
									Args:    []string{"failed"},
									Timeout: 30 * time.Second,
								},
							},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "missing API token",
			config: &Config{
				APIURL:   "https://api.theopenlane.io",
				DataDir:  "/tmp/test-data",
				LogLevel: "info",
			},
			expectErr: true,
		},
		{
			name: "missing API URL",
			config: &Config{
				APIToken: "test-token",
				DataDir:  "/tmp/test-data",
				LogLevel: "info",
			},
			expectErr: true,
		},
		{
			name: "invalid check - missing name",
			config: &Config{
				APIToken: "test-token",
				APIURL:   "https://api.theopenlane.io",
				DataDir:  "/tmp/test-data",
				LogLevel: "info",
				Checks: []Check{
					{
						Command:  "echo",
						Schedule: "* * * * *",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "invalid check - missing command",
			config: &Config{
				APIToken: "test-token",
				APIURL:   "https://api.theopenlane.io",
				DataDir:  "/tmp/test-data",
				LogLevel: "info",
				Checks: []Check{
					{
						Name:     "test",
						Schedule: "* * * * *",
					},
				},
			},
			expectErr: true,
		},
		{
			name: "invalid cron schedule",
			config: &Config{
				APIToken: "test-token",
				APIURL:   "https://api.theopenlane.io",
				DataDir:  "/tmp/test-data",
				LogLevel: "info",
				Checks: []Check{
					{
						Name:     "test",
						Command:  "echo",
						Schedule: "invalid",
					},
				},
			},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clean up data directory
			defer func() {
				if tt.config.DataDir != "" {
					os.RemoveAll(tt.config.DataDir)
				}
			}()

			// Use runtime validation for tests that expect check validation errors
			var err error
			if tt.name == "invalid check - missing name" || tt.name == "invalid check - missing command" || tt.name == "invalid cron schedule" {
				err = ValidateConfigForRuntime(tt.config)
			} else {
				err = ValidateConfig(tt.config)
			}

			if tt.expectErr && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestJSONSchemaGeneration(t *testing.T) {
	schema, err := GenerateJSONSchema()
	if err != nil {
		t.Fatalf("Failed to generate JSON schema: %v", err)
	}

	if schema.Title != "Openlane Agent Configuration" {
		t.Errorf("Expected title 'Openlane Agent Configuration', got '%s'", schema.Title)
	}

	if schema.Description == "" {
		t.Error("Schema description should not be empty")
	}

	// Ensure the schema can be marshaled to JSON
	_, err = json.Marshal(schema)
	if err != nil {
		t.Errorf("Failed to marshal schema to JSON: %v", err)
	}
}

func TestConfigSerialization(t *testing.T) {
	config := ExampleConfig()

	// Test JSON serialization
	jsonData, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("Failed to marshal config to JSON: %v", err)
	}

	var unmarshaled Config
	if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal config from JSON: %v", err)
	}

	// Basic validation that critical fields are preserved
	if unmarshaled.APIURL != config.APIURL {
		t.Errorf("API URL mismatch: expected %s, got %s", config.APIURL, unmarshaled.APIURL)
	}

	if len(unmarshaled.Checks) != len(config.Checks) {
		t.Errorf("Checks count mismatch: expected %d, got %d", len(config.Checks), len(unmarshaled.Checks))
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.APIURL == "" {
		t.Error("Default config should have API URL")
	}

	if config.LogLevel != "info" {
		t.Errorf("Expected default log level 'info', got '%s'", config.LogLevel)
	}

	if config.MaxConcurrency <= 0 {
		t.Error("Default max concurrency should be positive")
	}
}

func TestGetEnabledChecks(t *testing.T) {
	config := &Config{
		Checks: []Check{
			{Name: "enabled1", Enabled: true, Command: "test", Schedule: "* * * * *"},
			{Name: "disabled", Enabled: false, Command: "test", Schedule: "* * * * *"},
			{Name: "enabled2", Enabled: true, Command: "test", Schedule: "* * * * *"},
		},
	}

	enabled := config.GetEnabledChecks()
	if len(enabled) != 2 {
		t.Errorf("Expected 2 enabled checks, got %d", len(enabled))
	}

	for _, check := range enabled {
		if !check.Enabled {
			t.Errorf("Check %s should be enabled", check.Name)
		}
	}
}

func TestGetCheck(t *testing.T) {
	config := &Config{
		Checks: []Check{
			{Name: "test-check", Command: "test", Schedule: "* * * * *"},
		},
	}

	check, err := config.GetCheck("test-check")
	if err != nil {
		t.Fatalf("Failed to get check: %v", err)
	}

	if check.Name != "test-check" {
		t.Errorf("Expected check name 'test-check', got '%s'", check.Name)
	}

	_, err = config.GetCheck("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent check")
	}
}

func TestTokenCompatibility(t *testing.T) {
	cfg := &Config{APIToken: "  new-token  "}
	if got := cfg.Token(); got != "new-token" {
		t.Fatalf("expected token trimming, got %q", got)
	}

	tmpFile, err := os.CreateTemp("", "agent-config-*.yaml")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	configYAML := `
registrationToken: legacy-token
apiUrl: https://api.theopenlane.io
dataDir: /tmp/test-data-legacy
logLevel: info
offline:
  mode: standalone
  outputDir: /tmp/test-results-legacy
checks: []
`
	if _, err := tmpFile.WriteString(configYAML); err != nil {
		t.Fatalf("failed to write temp config: %v", err)
	}
	_ = tmpFile.Close()

	loaded, err := LoadConfig(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	defer os.RemoveAll("/tmp/test-data-legacy")
	defer os.RemoveAll("/tmp/test-results-legacy")

	if loaded.Token() != "legacy-token" {
		t.Fatalf("expected legacy registrationToken fallback, got %q", loaded.Token())
	}
}

func TestActionConfigValidation(t *testing.T) {
	tests := []struct {
		name      string
		action    *ActionConfig
		context   string
		expectErr bool
	}{
		{
			name: "valid action config",
			action: &ActionConfig{
				UploadEvidence: true,
				Commands: []ActionCommand{
					{
						Name:    "test-action",
						Command: "echo",
						Args:    []string{"test"},
						Timeout: 1 * time.Minute,
					},
				},
			},
			context:   "test",
			expectErr: false,
		},
		{
			name: "missing command name",
			action: &ActionConfig{
				Commands: []ActionCommand{
					{
						Command: "echo",
					},
				},
			},
			context:   "test",
			expectErr: true,
		},
		{
			name: "missing command",
			action: &ActionConfig{
				Commands: []ActionCommand{
					{
						Name: "test",
					},
				},
			},
			context:   "test",
			expectErr: true,
		},
		{
			name: "duplicate command names",
			action: &ActionConfig{
				Commands: []ActionCommand{
					{
						Name:    "duplicate",
						Command: "echo",
					},
					{
						Name:    "duplicate",
						Command: "echo",
					},
				},
			},
			context:   "test",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateActionConfig(tt.action, tt.context)
			if tt.expectErr && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}
