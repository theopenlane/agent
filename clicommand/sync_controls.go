package clicommand

import (
	"context"

	"github.com/rs/zerolog/log"
	"github.com/urfave/cli"
	"github.com/theopenlane/agent/api"
	"github.com/theopenlane/agent/core"
	"github.com/theopenlane/agent/internal/config"
)

// SyncControlsFlags are the flags for the sync-controls command
var SyncControlsFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "config, c",
		Usage: "Path to agent configuration file",
		Value: "agent.yaml",
	},
}

// SyncControlsAction runs the control synchronization
func SyncControlsAction(c *cli.Context) error {
	configPath := c.String("config")
	ctx := context.Background()

	// Load configuration
	log.Info().Str("config", configPath).Msg("Loading agent configuration")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		log.Fatal().Err(err).Str("config", configPath).Msg("Failed to load configuration")
		return err
	}

	// Create API client
	log.Info().Str("api_url", cfg.APIURL).Msg("Creating Openlane API client")
	client, err := api.NewGraphQLClient(cfg.APIURL, cfg.RegistrationToken)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create API client")
		return err
	}

	// Create control sync service
	syncService := core.NewControlSyncService(client, "sync-cli-agent", "default-org")

	// Perform synchronization
	log.Info().Msg("Starting control synchronization")
	result, err := syncService.SyncControlsFromConfig(ctx, cfg)
	if err != nil {
		log.Error().Err(err).Msg("Control synchronization failed")
		return err
	}

	// Display results
	log.Info().
		Int("total_agent_controls", result.TotalAgentControls).
		Int("total_openlane_controls", result.TotalOpenlaneControls).
		Int("matched_controls", len(result.MatchedControls)).
		Int("new_controls", len(result.NewControls)).
		Int("updated_controls", result.UpdatedControls).
		Int("errors", len(result.Errors)).
		Dur("duration", result.SyncDuration).
		Msg("Control synchronization completed")

	// Show detailed results
	if len(result.MatchedControls) > 0 {
		log.Info().Msg("Matched controls:")
		for _, match := range result.MatchedControls {
			log.Info().
				Str("agent_control", match.AgentControlRef).
				Str("openlane_control", match.OpenlaneControl.ID).
				Float64("confidence", match.MatchConfidence).
				Str("reason", match.MatchReason).
				Bool("requires_update", match.RequiresUpdate).
				Msg("  - Match found")
		}
	}

	if len(result.NewControls) > 0 {
		log.Info().Msg("New controls created:")
		for _, newControl := range result.NewControls {
			log.Info().Str("control", newControl).Msg("  - Created")
		}
	}

	if len(result.Errors) > 0 {
		log.Warn().Msg("Synchronization errors:")
		for _, err := range result.Errors {
			log.Error().Err(err).Msg("  - Error")
		}
		return cli.NewExitError("Control synchronization completed with errors", 1)
	}

	log.Info().Msg("Control synchronization successful")
	return nil
}