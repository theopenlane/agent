// Openlane Agent is a lightweight compliance agent that executes
// customer-defined compliance checks and reports results back to Openlane.
package main

import (
	"context"
	"os"

	"github.com/theopenlane/agent/clicommand"
	"github.com/theopenlane/agent/internal/constants"
	cli "github.com/urfave/cli/v3"
)

const appHelpTemplate = `Usage:
  {{.Name}} <command> [options...]

Available commands are: {{range .VisibleCategories}}{{if .Name}}
{{.Name}}:{{range .VisibleCommands}}
  {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{"\n"}}{{else}}{{range .VisibleCommands}}
  {{join .Names ", "}}{{"\t"}}{{.Usage}}{{end}}{{"\n"}}{{end}}{{end}}
Use "{{.Name}} <command> --help" for more information about a command.

For more information, see: https://docs.openlane.io/agent
`

const subcommandHelpTemplate = `Usage:

  {{.Name}} {{if .VisibleFlags}}<command>{{end}} [options...]

Available commands are:

  {{range .Commands}}{{.Name}}{{with .ShortName}}, {{.}}{{end}}{{ "\t" }}{{.Usage}}
  {{end}}{{if .VisibleFlags}}

Options:

{{range .VisibleFlags}}  {{.}}
{{end}}{{ end -}}
`

const commandHelpTemplate = `{{.Description}}

Options:

{{range .VisibleFlags}}  {{.}}
{{ end -}}
`

func main() {
	root := &cli.Command{
		Name:        "openlane-agent",
		Version:     constants.AgentVersion,
		Commands:    clicommand.AgentCommands,
		Description: "Openlane compliance agent",
	}

	// Run the CLI
	if err := root.Run(context.Background(), os.Args); err != nil {
		os.Exit(clicommand.PrintMessageAndReturnExitCode(err))
	}
}
