package clicommand

import (
	"fmt"
	"os"

	"github.com/urfave/cli"
)

// AgentCommands are all the agent commands
var AgentCommands = []cli.Command{
	{
		Name:        "start",
		Usage:       "Starts the Openlane compliance agent",
		Description: startDescription,
		Flags:       StartFlags,
		Action:      StartAction,
	},
	{
		Name:        "stop",
		Usage:       "Stops a running Openlane agent",
		Description: "Stops a running Openlane agent by sending a graceful shutdown signal",
		Flags:       StopFlags,
		Action:      StopAction,
	},
	{
		Name:        "status",
		Usage:       "Shows the status of a running agent",
		Description: "Shows the current status of the agent, including running checks and statistics",
		Flags:       StatusFlags,
		Action:      StatusAction,
	},
	{
		Name:        "check",
		Usage:       "Runs a single compliance check",
		Description: "Executes a single compliance check and displays the results",
		Flags:       CheckFlags,
		Action:      CheckAction,
	},
	{
		Name:        "config",
		Usage:       "Manage agent configuration",
		Description: "Commands for managing agent configuration",
		Subcommands: []cli.Command{
			{
				Name:        "init",
				Usage:       "Initialize a new agent configuration",
				Description: "Creates a new agent.yaml configuration file with example checks",
				Flags:       ConfigInitFlags,
				Action:      ConfigInitAction,
			},
			{
				Name:        "validate",
				Usage:       "Validate agent configuration",
				Description: "Validates the agent configuration file for syntax and semantic errors",
				Flags:       ConfigValidateFlags,
				Action:      ConfigValidateAction,
			},
			{
				Name:        "show",
				Usage:       "Show current configuration",
				Description: "Displays the current agent configuration",
				Flags:       ConfigShowFlags,
				Action:      ConfigShowAction,
			},
		},
	},
	{
		Name:        "sync-controls",
		Usage:       "Synchronize controls from agent.yaml with Openlane system",
		Description: "Synchronizes the controls specified in agent.yaml checks with the Openlane platform. It validates control references, matches existing controls, creates new controls for unmatched references, and provides detailed reporting.",
		Flags:       SyncControlsFlags,
		Action:      SyncControlsAction,
	},
	{
		Name:        "version",
		Usage:       "Show version information",
		Description: "Displays version, build, and system information",
		Action:      VersionAction,
	},
}

// PrintMessageAndReturnExitCode prints an error message and returns the appropriate exit code
func PrintMessageAndReturnExitCode(err error) int {
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}