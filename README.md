[![Go Report Card](https://goreportcard.com/badge/github.com/theopenlane/agent)](https://goreportcard.com/report/github.com/theopenlane/agent)
[![Build status](https://badge.buildkite.com/34ad31fe4231b2953cd3f2d116364d21a39b2a4dbf1eea539a.svg)](https://buildkite.com/theopenlane/agent?branch=main)
[![Go Reference](https://pkg.go.dev/badge/github.com/theopenlane/agent.svg)](https://pkg.go.dev/github.com/theopenlane/agent)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache2.0-brightgreen.svg)](https://opensource.org/licenses/Apache-2.0)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=theopenlane_REPONAME&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=theopenlane_REPONAME)


# Openlane Agent

Lightweight compliance agent that runs customer-defined checks, collects evidence, and reports results back to the Openlane platform. It supports connected, buffered, and standalone modes, executes both locally scheduled and remotely assigned jobs, and stores everything locally when connectivity is unavailable.

## Key Capabilities
- Run local checks on cron schedules and optionally poll the platform for remote scheduled jobs.
- Register as a JobRunner with Openlane, send heartbeats, and report JobResults (including findings and evidence uploads).
- Buffer results/evidence to disk with automatic retry-based syncing when the API is reachable.
- Validate controls against platform metadata (fail-open when validation fails) and map results to compliance standards.
- Platform-aware check variants, pass/fail hooks, and evidence collection from files and command output.
- CLI for starting/stopping the agent, running single checks, and initializing/validating configuration.


## How the Agent Runs
1. **CLI start** (`clicommand/agent_start.go`): load config, set logging, optionally daemonize, and build the core `Agent`.
2. **Registration** (`core/agent.go`): unless in standalone mode, register as a JobRunner via the GraphQL client with hardware ID, host metadata, and tags.
3. **Workers** (`core/agent_worker.go`):
   - Spawn `cfg.spawn` workers, each with its own scheduler and retry manager.
   - Heartbeat every 60s updates JobRunner status, version, IP, and last seen.
   - Local scheduler scans due checks every 30s (simple 5-field cron parser).
   - Optional remote polling uses JobRunner ID to fetch scheduled jobs (guarded by `enableRemotePoll`).
   - Concurrency capped by `maxConcurrency`; active checks tracked per worker.
4. **Check execution** (`core/compliance_check_controller.go`):
   - Validate controls through the API (fail-open with reason logging) and apply platform variants (`internal/platform`).
   - Execute commands with injected env (`OPENLANE_CHECK_NAME`, `OPENLANE_AGENT_ID`, `OPENLANE_CHECK_TIMEOUT`, `OPENLANE_CONTROLS`, `OPENLANE_TAGS`).
   - Parse JSON output into findings/metrics/evidence metadata when present; status defaults to non-zero exit code = failure.
   - Collect evidence from configured paths plus stdout/stderr snapshots; run pass/fail action commands; optionally update control status (placeholder).
   - Store results through the storage layer.

## Configuration
- Default config file: `agent.yaml` (override with `--config`). Env overrides use the `OPENLANE_AGENT_` prefix and mirror keys (e.g., `OPENLANE_AGENT_APIURL`, `OPENLANE_AGENT_LOGLEVEL`).
- Full schema lives in `schema/agent.config.json`; example config in `config/config.example.yaml` and `examples/agent.example.yaml`.
- Key fields (see `config/config.go`):
  - **API & identity:** `registrationToken`, `apiUrl`, `agentName`, `identity.*` (hardware ID detection/fallbacks).
  - **Runtime:** `logLevel`, `dataDir`, `pollInterval`, `spawn`, `maxConcurrency`, `defaultTimeout`, `enableRemotePoll`.
  - **Offline modes (`offline.mode`):**
    - `normal` – online operation with API.
    - `buffered` – online-first with disk buffering + retries (`offline.bufferDir`, `syncInterval`, `connectivityInterval`, `maxRetries`).
    - `standalone` – skip registration/remote polling; still buffers locally.
  - **Evidence:** toggle, retention period, max file size, compression flag.
  - **Retry:** max attempts, initial/max delay, strategy, multiplier.
  - **Checks:** `name`, `command`, `args`, `env`, `workDir`, `schedule` (5/6-field cron), `timeout`, `tags`, `continueOnError`.
    - Compliance context: `complianceStandards[].standard` + `controls`.
    - Evidence sources: `evidencePaths`.
    - Platform variants: per-OS/arch overrides, includes/excludes regex, expected exit codes, remediation hints.
    - Actions: `onPass`/`onFail` with `uploadEvidence`, `updateControlStatus`, and custom commands.
- Validate configs via `./openlane-agent config validate --config agent.yaml`; initialize via `./openlane-agent config init --output agent.yaml [--force]`.

### Result Payloads
Checks emit a `schema.ComplianceCheckResult`:
- Core fields: `checkName`, `standard`, `controlRef`, `status`, `exitCode`, `startedAt`, `finishedAt`, `log`, `error`, `metadata`.
- Parsed output can add `findings`, `metrics`, and evidence hints into metadata. Raw stdout/stderr is preserved for troubleshooting.

## Storage & Offline Behavior
- `internal/storage` always buffers results/evidence to `offline.bufferDir` (default `./buffer`) with metadata and retry counts.
- If API credentials are set, `APIStorage` immediately attempts to create JobResults and upload evidence; failures leave the buffered copy for retry.
- Background sync (`syncInterval`) runs while connectivity checks pass; old buffer files pruned via `bufferRetentionPeriod`.
- Evidence collection (`EvidenceService`) enforces max file size and can generate stdout/stderr artifacts; retention cleanup available.

## Platform API Integration
- GraphQL client (`api/graphql_client.go`) wraps `openlaneclient` with retry hooks:
  - Register JobRunner (`RegisterAgent`) and expose JobRunner ID for polling/heartbeats.
  - Sync JobTemplates per check (`SyncJobTemplates`), with a known upstream issue where IDs may be empty; logs warnings and skips scheduled job creation when missing.
  - Create ScheduledJobs for manual/local runs when templates exist.
  - Poll scheduled jobs for this runner (`PollForWork`) when enabled.
  - Report JobResults (`ReportResults`) with optional result file uploads; evidence uploads (`CreateEvidence`) can be tied to controls and JobResult IDs.
  - Control metadata: fetch/update controls and validate standards/controls before execution.
  - Heartbeats: `UpdateAgentStatus` updates version, IP, last seen, and system info.
- User agent string is `openlane-agent/<version>` from `internal/constants`.

## CLI & Common Commands
Build the binary and exercise the CLI:

```bash
go build -o openlane-agent ./main.go
./openlane-agent config init --output agent.yaml
./openlane-agent config validate --config agent.yaml
./openlane-agent start --config agent.yaml --no-daemon --log-level debug
./openlane-agent check <check-name> --config agent.yaml
./openlane-agent stop --pid-file agent.pid
./openlane-agent status --pid-file agent.pid
./openlane-agent version
```

Flags like `--api-url`, `--api-key`, `--max-concurrency`, `--data-dir`, and `--no-daemon` override config values at startup. Daemon mode writes a PID file (default `agent.pid`); foreground mode is recommended during development.

## Development & Testing
- Go modules: Go 1.25+ (`go.mod`).
- Format/lint/test: `task go:fmt`, `task go:lint`, `task go:test` or `go test ./...`.
- Build convenience: `task build`.
- Config helpers: `task config:init`, `task config:validate`, `task config:show`.
- Schema generation: `go generate ./schema` (runs `schema/generate_schemas.go` to refresh `schema/agent.config.json` and example files).
- Sample scripts in `scripts/` exercise storage/connectivity and include a disk-encryption example check.

## Notes & Operational Defaults
- Heartbeat interval: 60s. Local scheduler scan: 30s. Placeholder cron when creating JobTemplates: `0 */30 * * * *` (server validation).
- Control validation is fail-open unless the only referenced control is invalid, in which case the check is skipped with a failure result.
- Buffered results live under `buffer/` by default; clean up after local runs as needed.
