package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/theopenlane/agent/internal/config"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <output-file>\n", os.Args[0])
		os.Exit(1)
	}

	outputFile := os.Args[1]

	// Generate JSON Schema
	schema, err := config.GenerateJSONSchema()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate JSON schema: %v\n", err)
		os.Exit(1)
	}

	// Marshal schema to JSON
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to marshal schema: %v\n", err)
		os.Exit(1)
	}

	// Write to file
	if err := os.WriteFile(outputFile, schemaJSON, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to write schema file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("JSON Schema written to %s\n", outputFile)
}