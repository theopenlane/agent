package main

import (
	"fmt"
	"os"

	"github.com/theopenlane/agent/internal/config"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <config-file>\n", os.Args[0])
		os.Exit(1)
	}

	configFile := os.Args[1]

	// Load configuration
	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Configuration is valid!\n")
	fmt.Printf("  Agent Name: %s\n", cfg.AgentName)
	fmt.Printf("  API URL: %s\n", cfg.APIURL)
	fmt.Printf("  Data Directory: %s\n", cfg.DataDir)
	fmt.Printf("  Evidence Collection: %v\n", cfg.Evidence.Enabled)
	fmt.Printf("  Checks Defined: %d\n", len(cfg.Checks))
	
	enabledChecks := cfg.GetEnabledChecks()
	fmt.Printf("  Enabled Checks: %d\n", len(enabledChecks))
	
	for i, check := range enabledChecks {
		fmt.Printf("    %d. %s\n", i+1, check.Name)
		if len(check.Controls) > 0 {
			fmt.Printf("       Controls: %v\n", check.Controls)
		}
		if len(check.EvidencePaths) > 0 {
			fmt.Printf("       Evidence Paths: %v\n", check.EvidencePaths)
		}
		if check.OnPass != nil {
			fmt.Printf("       Pass Actions: Upload Evidence=%v, Update Controls=%v, Commands=%d\n", 
				check.OnPass.UploadEvidence, check.OnPass.UpdateControlStatus, len(check.OnPass.Commands))
		}
		if check.OnFail != nil {
			fmt.Printf("       Fail Actions: Upload Evidence=%v, Update Controls=%v, Commands=%d\n", 
				check.OnFail.UploadEvidence, check.OnFail.UpdateControlStatus, len(check.OnFail.Commands))
		}
		fmt.Println()
	}

	fmt.Println("✓ All validations passed successfully!")
}