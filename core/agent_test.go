package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/theopenlane/agent/config"
)

func TestPartitionEnabledChecks(t *testing.T) {
	checks := []config.Check{
		{Name: "check-1", Enabled: true},
		{Name: "check-2", Enabled: false},
		{Name: "check-3", Enabled: true},
		{Name: "check-4", Enabled: true},
		{Name: "check-5", Enabled: false},
	}

	t.Run("zero workers returns empty partitions", func(t *testing.T) {
		result := partitionEnabledChecks(checks, 0)
		assert.Empty(t, result)
	})

	t.Run("single worker gets all enabled checks", func(t *testing.T) {
		result := partitionEnabledChecks(checks, 1)
		require.Len(t, result, 1)
		assert.Len(t, result[0], 3)
	})

	t.Run("two workers splits enabled checks round-robin", func(t *testing.T) {
		result := partitionEnabledChecks(checks, 2)
		require.Len(t, result, 2)

		total := len(result[0]) + len(result[1])
		assert.Equal(t, 3, total)
	})

	t.Run("more workers than checks leaves some workers with no checks", func(t *testing.T) {
		result := partitionEnabledChecks(checks, 10)
		require.Len(t, result, 10)

		nonEmpty := 0
		for _, partition := range result {
			if len(partition) > 0 {
				nonEmpty++
			}
		}

		assert.Equal(t, 3, nonEmpty)
	})

	t.Run("no enabled checks produces empty partitions", func(t *testing.T) {
		allDisabled := []config.Check{
			{Name: "a", Enabled: false},
			{Name: "b", Enabled: false},
		}

		result := partitionEnabledChecks(allDisabled, 2)
		require.Len(t, result, 2)
		assert.Empty(t, result[0])
		assert.Empty(t, result[1])
	})
}

func TestNewAgent_standaloneMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Offline.Mode = config.ModeStandalone
	cfg.Offline.OutputDir = t.TempDir()
	cfg.DataDir = t.TempDir()

	agent, err := NewAgent(WithConfig(cfg))
	require.NoError(t, err)
	require.NotNil(t, agent)

	// In standalone mode the API client should not be initialized
	assert.Nil(t, agent.apiClient)
}

func TestNewAgent_missingTokenInNormalMode(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Offline.Mode = config.ModeNormal
	cfg.APIURL = "http://localhost:17608"
	cfg.APIToken = ""

	_, err := NewAgent(WithConfig(cfg))
	// NewGraphQLClientFromConfig validates the token; expect an error
	require.Error(t, err)
}

func TestAgent_getStats(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Offline.Mode = config.ModeStandalone
	cfg.Offline.OutputDir = t.TempDir()
	cfg.DataDir = t.TempDir()
	cfg.AgentName = "test-agent"

	agent, err := NewAgent(WithConfig(cfg))
	require.NoError(t, err)

	stats := agent.GetStats()
	assert.Equal(t, "test-agent", stats["agent_name"])
	assert.Equal(t, 0, stats["worker_count"])
}
