package clicommand

import (
	"testing"

	"github.com/theopenlane/agent/config"
)

func TestRedactSensitiveEnvVars(t *testing.T) {
	env := []string{
		"SAFE_VALUE=ok",
		"OPENLANE_TOKEN=secret",
		"AWS_SECRET_ACCESS_KEY=very-secret",
		"TOKEN_WITHOUT_EQUALS",
	}

	redactSensitiveEnvVars(env)

	if got := env[0]; got != "SAFE_VALUE=ok" {
		t.Fatalf("expected safe env to remain unchanged, got %q", got)
	}

	if got := env[1]; got != "OPENLANE_TOKEN=[REDACTED]" {
		t.Fatalf("expected token env redacted, got %q", got)
	}

	if got := env[2]; got != "AWS_SECRET_ACCESS_KEY=[REDACTED]" {
		t.Fatalf("expected aws secret env redacted, got %q", got)
	}

	if got := env[3]; got != "TOKEN_WITHOUT_EQUALS" {
		t.Fatalf("expected invalid env format to remain unchanged, got %q", got)
	}
}

func TestRedactSensitiveConfig(t *testing.T) {
	cfg := &config.Config{
		APIToken: "raw-token",
		Checks: []config.Check{
			{
				Env: []string{
					"CHECK_TOKEN=secret",
				},
				PlatformVariants: []config.PlatformVariant{
					{
						Env: []string{
							"PLATFORM_API_KEY=secret",
						},
					},
				},
				OnPass: &config.ActionConfig{
					Commands: []config.ActionCommand{
						{
							Env: []string{
								"PASS_SECRET=secret",
							},
						},
					},
				},
				OnFail: &config.ActionConfig{
					Commands: []config.ActionCommand{
						{
							Env: []string{
								"FAIL_PASSWORD=secret",
							},
						},
					},
				},
			},
		},
	}

	redactSensitiveConfig(cfg)

	if got := cfg.APIToken; got != "[REDACTED]" {
		t.Fatalf("expected API token redacted, got %q", got)
	}

	if got := cfg.Checks[0].Env[0]; got != "CHECK_TOKEN=[REDACTED]" {
		t.Fatalf("expected check env redacted, got %q", got)
	}

	if got := cfg.Checks[0].PlatformVariants[0].Env[0]; got != "PLATFORM_API_KEY=[REDACTED]" {
		t.Fatalf("expected platform variant env redacted, got %q", got)
	}

	if got := cfg.Checks[0].OnPass.Commands[0].Env[0]; got != "PASS_SECRET=[REDACTED]" {
		t.Fatalf("expected onPass command env redacted, got %q", got)
	}

	if got := cfg.Checks[0].OnFail.Commands[0].Env[0]; got != "FAIL_PASSWORD=[REDACTED]" {
		t.Fatalf("expected onFail command env redacted, got %q", got)
	}
}
