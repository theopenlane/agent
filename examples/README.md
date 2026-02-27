# Scripted Examples

This directory now contains a cohesive example set for common agent workflows.

## What Each Script Does

- `./scripts/run-examples.sh`: entrypoint that runs one or more examples.
- `./scripts/example-operation-modes.sh`: generates and validates `standalone`, `buffered`, and `normal` configs, then runs a customer-style compliance check (`check-disk-encryption.sh`).
- `./scripts/example-offline-buffering.sh`: runs the same compliance check in `buffered` mode against an unreachable API URL and verifies local buffer files are created.
- `./scripts/check-disk-encryption.sh`: representative compliance check script that emits findings and evidence artifacts.

## Prerequisites

- Run commands from the `agent` directory.
- Either:
  - `./openlane-agent` exists and is executable, or
  - `go` is installed (scripts will fall back to `go run -tags cli ./main.go`).

Optional environment variables:

- `OPENLANE_AGENT_BIN`: explicit path to agent binary (overrides auto-detection).
- `OPENLANE_EXAMPLE_WORKSPACE_ROOT`: base directory for generated example workspaces.
- `OPENLANE_EXAMPLE_CHECK_SCRIPT`: override the check script path used by examples.
- `OPENLANE_EXAMPLE_CHECK_NAME`: override the check name used by examples.

## Quick Start

Run both local-only examples:

```bash
./scripts/run-examples.sh all
```

Run only operation mode example:

```bash
./scripts/run-examples.sh modes
```

Run only offline buffering example:

```bash
./scripts/run-examples.sh offline
```

## Custom Workspace Path

Both local-only examples accept `--workspace`:

```bash
./scripts/run-examples.sh modes --workspace /tmp/agent-modes-demo
./scripts/run-examples.sh offline --workspace /tmp/agent-offline-demo
```

Without `--workspace`, each script creates a temp workspace and prints its location.

## Notes

- The examples execute a real compliance script. Depending on host posture, the check may exit non-zero.
- A non-zero check result indicates non-compliance, not example failure; examples still validate config wiring and buffering behavior.
