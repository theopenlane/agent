// Openlane Agent is a lightweight compliance agent that executes
// customer-defined compliance checks and reports results back to Openlane.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/theopenlane/agent/clicommand"
	"github.com/theopenlane/agent/internal/constants"
	cli "github.com/urfave/cli/v3"
)

func main() {
	root := &cli.Command{
		Name:        "openlane-agent",
		Version:     constants.AgentVersion,
		Commands:    clicommand.AgentCommands,
		Description: "Openlane compliance agent",
	}

	if err := root.Run(context.Background(), os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
