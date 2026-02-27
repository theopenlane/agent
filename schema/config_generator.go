package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/invopop/jsonschema"
	"github.com/invopop/yaml"

	"github.com/theopenlane/utils/envparse"
)

// Configuration paths and constants
const (
	tagName        = "koanf"
	skipper        = "-"
	defaultTag     = "default"
	jsonSchemaPath = "./schema/agent.config.json"
	yamlConfigPath = "./config/config.example.yaml"
	envConfigPath  = "./config/.env.example"
	sensitiveTag   = "sensitive"
	varPrefix      = "OPENLANE_AGENT"
	ownerReadWrite = 0o600

	// File size constants
	maxFileSizeBytes = 104857600 // 100MB
)

// includedPackages for Go comment extraction
var includedPackages = []string{
	"./config",
	"./internal/agentopts",
	"./internal/evidence",
	"./internal/storage",
	"./internal/connectivity",
	"./internal/retry",
	"./internal/identity",
	"./internal/scheduler",
}

// schemaConfig represents the configuration for the schema generator
type schemaConfig struct {
	jsonSchemaPath string
	yamlConfigPath string
	envConfigPath  string
}

// SensitiveField represents a sensitive configuration field
type SensitiveField struct {
	Key        string
	Path       string
	SecretName string
}

// GenerateConfigSchemas generates all configuration schema files
func GenerateConfigSchemas(configStruct any) error {
	c := schemaConfig{
		jsonSchemaPath: jsonSchemaPath,
		yamlConfigPath: yamlConfigPath,
		envConfigPath:  envConfigPath,
	}

	return generateSchema(c, configStruct)
}

// generateSchema generates all configuration files from the provided structure
func generateSchema(c schemaConfig, structure any) error {
	if err := generateJSONSchema(c.jsonSchemaPath, structure); err != nil {
		return err
	}

	if err := generateYAMLConfig(c.yamlConfigPath); err != nil {
		return err
	}

	envFields, _, err := processEnvironmentVariables(structure)
	if err != nil {
		return err
	}

	if err := generateEnvironmentFile(c.envConfigPath, envFields); err != nil {
		return err
	}

	return nil
}

// generateJSONSchema creates the JSON schema file from the config structure
func generateJSONSchema(jsonSchemaPath string, structure any) error {
	r := jsonschema.Reflector{Namer: namePkg}
	r.ExpandedStruct = true
	r.RequiredFromJSONSchemaTags = true
	r.FieldNameTag = tagName

	// Add go comments to the schema
	for _, pkg := range includedPackages {
		if err := r.AddGoComments("github.com/theopenlane/agent/", pkg); err != nil {
			fmt.Printf("Warning: failed to add go comments for package %s: %v\n", pkg, err)
		}
	}

	s := r.Reflect(structure)

	// Add custom metadata
	s.Title = "Openlane Agent Configuration"
	s.Description = "Configuration schema for the Openlane compliance automation agent"

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON schema: %w", err)
	}

	if err := os.WriteFile(jsonSchemaPath, data, ownerReadWrite); err != nil {
		return fmt.Errorf("failed to write JSON schema file: %w", err)
	}

	fmt.Printf("JSON Schema written to %s\n", jsonSchemaPath)

	return nil
}

// generateYAMLConfig creates the YAML configuration file with defaults
func generateYAMLConfig(yamlConfigPath string) error {
	// Create a generic config structure - this will need to be updated
	// to work without the config import
	yamlConfig := make(map[string]any)

	// Set example values manually since we can't import config
	yamlConfig["token"] = "${OPENLANE_AGENT_TOKEN}"
	yamlConfig["apiUrl"] = "https://api.theopenlane.io"
	yamlConfig["agentName"] = "production-compliance-agent"
	yamlConfig["logLevel"] = "info"
	yamlConfig["dataDir"] = "./data"
	yamlConfig["pollInterval"] = "1m"
	yamlConfig["maxConcurrency"] = 3
	yamlConfig["defaultTimeout"] = "5m"

	// Configure evidence collection
	evidence := map[string]any{
		"enabled":         true,
		"retentionPeriod": "720h",           // 30 days
		"maxFileSize":     maxFileSizeBytes, // 100MB
		"compressFiles":   false,
	}
	yamlConfig["evidence"] = evidence

	// Configure operation mode
	offline := map[string]any{
		"mode":         "normal",
		"outputDir":    "./results",
		"outputFormat": "json",
	}
	yamlConfig["offline"] = offline

	// Add example checks
	checks := []map[string]any{
		{
			"name":        "disk-encryption-check",
			"description": "Verify that full disk encryption is enabled on the system",
			"command":     "./scripts/check-disk-encryption.sh",
			"schedule":    "0 6 * * *", // Daily at 6 AM
			"timeout":     "5m",
			"env":         []string{"ENCRYPTION_POLICY=required"},
			"complianceStandards": []map[string]any{
				{
					"standard": "soc2v2022",
					"controls": []string{"CC6.7"},
				},
				{
					"standard": "iso27001v2022",
					"controls": []string{"A.10.1.1"},
				},
				{
					"standard": "nist80053v5",
					"controls": []string{"SC-28"},
				},
			},
			"tags":          []string{"encryption", "storage", "host-security"},
			"enabled":       true,
			"evidencePaths": []string{"./evidence/disk-encryption-check/"},
			"onPass": map[string]any{
				"uploadEvidence": true,
			},
			"onFail": map[string]any{
				"uploadEvidence": true,
				"commands": []map[string]any{
					{
						"name":    "create-security-incident",
						"command": "./scripts/create-incident.sh",
						"args":    []string{"--type", "encryption", "--severity", "high"},
						"timeout": "1m",
					},
				},
			},
		},
	}
	yamlConfig["checks"] = checks

	yamlSchema, err := yaml.Marshal(yamlConfig)
	if err != nil {
		return fmt.Errorf("failed to marshal YAML config: %w", err)
	}

	if err := os.WriteFile(yamlConfigPath, yamlSchema, ownerReadWrite); err != nil {
		return fmt.Errorf("failed to write YAML config file: %w", err)
	}

	fmt.Printf("YAML config example written to %s\n", yamlConfigPath)

	return nil
}

// processEnvironmentVariables extracts and processes all environment variables from the config
func processEnvironmentVariables(configStruct any) (string, []SensitiveField, error) {
	cp := envparse.Config{
		FieldTagName: tagName,
		Skipper:      skipper,
	}

	out, err := cp.GatherEnvInfo(varPrefix, configStruct)
	if err != nil {
		return "", nil, fmt.Errorf("failed to gather environment info: %w", err)
	}

	envVars := "# Openlane Agent Environment Variables\n"
	envVars += "# This file contains all configurable environment variables for the agent\n"
	envVars += "# Copy this file to .env and customize as needed\n\n"

	var sensitiveFields []SensitiveField

	for _, field := range out {
		defaultVal := field.Tags.Get(defaultTag)
		isSecret := field.Tags.Get(sensitiveTag) == "true"

		if !isSecret {
			// Add comment describing the field if available
			if desc := field.Tags.Get("description"); desc != "" {
				envVars += fmt.Sprintf("# %s\n", desc)
			}

			envVars += fmt.Sprintf("%s=\"%s\"\n\n", field.Key, defaultVal)
		} else {
			// Track sensitive fields separately
			secretName := generateSecretName(field.FullPath)
			sensitiveFields = append(sensitiveFields, SensitiveField{
				Key:        field.Key,
				Path:       field.FullPath,
				SecretName: secretName,
			})

			// Add placeholder for sensitive field
			desc := field.Tags.Get("description")
			if desc == "" {
				desc = "Sensitive configuration value"
			}

			envVars += fmt.Sprintf("# %s (SENSITIVE - set in production)\n", desc)
			envVars += fmt.Sprintf("# %s=\"\"\n\n", field.Key)
		}
	}

	return envVars, sensitiveFields, nil
}

// generateEnvironmentFile writes the environment variables to a file
func generateEnvironmentFile(envConfigPath, envVars string) error {
	if err := os.WriteFile(envConfigPath, []byte(envVars), ownerReadWrite); err != nil {
		return fmt.Errorf("failed to write environment file: %w", err)
	}

	fmt.Printf("Environment variables example written to %s\n", envConfigPath)

	return nil
}

// generateSecretName creates a Kubernetes-friendly secret name from a config path
func generateSecretName(path string) string {
	name := strings.ToLower(path)
	name = strings.ReplaceAll(name, ".", "-")
	// Convert camelCase to kebab-case
	var result strings.Builder

	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('-')
		}

		result.WriteRune(r)
	}

	return strings.ToLower(result.String())
}

func namePkg(r reflect.Type) string {
	return r.String()
}
