package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/invopop/jsonschema"
	"github.com/invopop/yaml"
	"github.com/mcuadros/go-defaults"

	"github.com/theopenlane/utils/envparse"

	"github.com/theopenlane/agent/config"
)

// Configuration paths and constants
const (
	tagName        = "koanf"
	skipper        = "-"
	defaultTag     = "default"
	jsonSchemaPath = "./jsonschema/agent.config.json"
	yamlConfigPath = "./config/config.example.yaml"
	envConfigPath  = "./config/.env.example"
	configMapPath  = "./config/configmap.yaml"
	sensitiveTag   = "sensitive"
	varPrefix      = "OPENLANE_AGENT"
	ownerReadWrite = 0o600
	dirPermission  = 0o755
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

func main() {
	c := schemaConfig{
		jsonSchemaPath: jsonSchemaPath,
		yamlConfigPath: yamlConfigPath,
		envConfigPath:  envConfigPath,
	}

	if err := generateSchema(c, &config.Config{}); err != nil {
		panic(err)
	}
}

// generateSchema generates all configuration files from the provided structure
func generateSchema(c schemaConfig, structure interface{}) error {
	if err := generateJSONSchema(c.jsonSchemaPath, structure); err != nil {
		return err
	}

	if err := generateYAMLConfig(c.yamlConfigPath); err != nil {
		return err
	}

	envFields, _, err := processEnvironmentVariables()
	if err != nil {
		return err
	}

	if err := generateEnvironmentFile(c.envConfigPath, envFields); err != nil {
		return err
	}

	return nil
}

// generateJSONSchema creates the JSON schema file from the config structure
func generateJSONSchema(jsonSchemaPath string, structure interface{}) error {
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
	yamlConfig := &config.Config{}
	defaults.SetDefaults(yamlConfig)

	// Set example values that are more meaningful than defaults
	yamlConfig.RegistrationToken = "${OPENLANE_AGENT_REGISTRATIONTOKEN}"
	yamlConfig.APIURL = "https://api.theopenlane.io"
	yamlConfig.AgentName = "production-compliance-agent"
	yamlConfig.LogLevel = "info"
	yamlConfig.DataDir = "./data"
	yamlConfig.PollInterval = time.Minute
	yamlConfig.MaxConcurrency = 3
	yamlConfig.DefaultTimeout = 5 * time.Minute

	// Configure evidence collection
	yamlConfig.Evidence.Enabled = true
	yamlConfig.Evidence.RetentionPeriod = 30 * 24 * time.Hour // 30 days
	yamlConfig.Evidence.MaxFileSize = 100 * 1024 * 1024       // 100MB
	yamlConfig.Evidence.CompressFiles = false

	// Configure operation mode
	yamlConfig.Offline.Mode = config.ModeNormal
	yamlConfig.Offline.OutputDir = "./results"
	yamlConfig.Offline.OutputFormat = "json"

	// Add example checks
	yamlConfig.Checks = []config.Check{
		{
			Name:        "disk-encryption-check",
			Description: "Verify that full disk encryption is enabled on the system",
			Command:     "./scripts/check-disk-encryption.sh",
			Schedule:    "0 6 * * *", // Daily at 6 AM
			Timeout:     5 * time.Minute,
			Env: []string{
				"ENCRYPTION_POLICY=required",
			},
			Controls: []string{
				"SOC2:CC6.7",
				"ISO27001:A.10.1.1",
				"NIST:SC-28",
			},
			Tags:    []string{"encryption", "storage", "host-security"},
			Enabled: true,
			EvidencePaths: []string{
				"./evidence/disk-encryption-check/",
			},
			OnPass: &config.ActionConfig{
				UploadEvidence:      true,
				UpdateControlStatus: true,
			},
			OnFail: &config.ActionConfig{
				UploadEvidence:      true,
				UpdateControlStatus: true,
				Commands: []config.ActionCommand{
					{
						Name:    "create-security-incident",
						Command: "./scripts/create-incident.sh",
						Args:    []string{"--type", "encryption", "--severity", "high"},
						Timeout: time.Minute,
					},
				},
			},
		},
	}

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
func processEnvironmentVariables() (string, []SensitiveField, error) {
	cp := envparse.Config{
		FieldTagName: tagName,
		Skipper:      skipper,
	}

	out, err := cp.GatherEnvInfo(varPrefix, &config.Config{})
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
