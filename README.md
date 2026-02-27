[![Go Report Card](https://goreportcard.com/badge/github.com/theopenlane/agent)](https://goreportcard.com/report/github.com/theopenlane/agent)
[![Build status](https://badge.buildkite.com/34ad31fe4231b2953cd3f2d116364d21a39b2a4dbf1eea539a.svg)](https://buildkite.com/theopenlane/agent?branch=main)
[![Go Reference](https://pkg.go.dev/badge/github.com/theopenlane/agent.svg)](https://pkg.go.dev/github.com/theopenlane/agent)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache2.0-brightgreen.svg)](https://opensource.org/licenses/Apache-2.0)

# Openlane Agent

The Openlane Agent runs compliance checks on your infrastructure, collects evidence, and syncs results to Openlane.

> This project is currently marked as work in progress.

## What You Can Do With It

- Run scheduled checks locally using cron expressions.
- Capture evidence from files/directories and command output.
- Buffer results on disk so they can be retried when connectivity is restored.
- Run in foreground or as a daemon process.
- Execute one check on demand for testing.

## Install

Build from source:

```bash
go build -o openlane-agent ./main.go
```

## Quick Start

1. Create a starter config:

```bash
./openlane-agent config init --output agent.yaml
```

2. Edit `agent.yaml` with your token, API URL, and checks.  
   Minimal example:

```yaml
token: "${OPENLANE_AGENT_TOKEN}"
apiUrl: "https://api.theopenlane.io"
agentName: "my-agent"
pollInterval: "1m"

offline:
  mode: "buffered"
  bufferDir: "./buffer"
  syncInterval: "5m"

evidence:
  enabled: true
  retentionPeriod: "168h"
  maxFileSize: 10485760

checks:
  - name: "disk-encryption-check"
    command: "fdesetup"
    args: ["status"]
    schedule: "*/5 * * * *"
    timeout: "30s"
    enabled: true
```

3. Validate configuration:

```bash
./openlane-agent config validate --config agent.yaml
```

4. Start in foreground:

```bash
./openlane-agent start --config agent.yaml --no-daemon
```

## Common Workflows

Run one check immediately:

```bash
./openlane-agent check disk-encryption-check --config agent.yaml
```

Start in daemon mode (default), then inspect/stop:

```bash
./openlane-agent start --config agent.yaml
./openlane-agent status --pid-file agent.pid
./openlane-agent stop --pid-file agent.pid
```

Show effective config without exposing secrets:

```bash
./openlane-agent config show --config agent.yaml --redact --format yaml
```

## Representative Check Examples

### 1. Scripted command check

```yaml
checks:
  - name: "aws-iam-compliance"
    command: "./scripts/check-aws-iam.sh"
    schedule: "0 */4 * * *"
    timeout: "10m"
    env:
      - "AWS_REGION=us-east-1"
    complianceStandards:
      - standard: "soc2v2022"
        controls: ["CC6.1"]
    evidencePaths:
      - "./evidence/aws-iam/"
    enabled: true
```

### 2. Platform-specific file check

```yaml
checks:
  - name: "ssh-root-login-disabled"
    command: "true"
    schedule: "0 * * * *"
    enabled: true
    platformVariants:
      - platforms: ["linux", "darwin"]
        file: "/etc/ssh/sshd_config"
        excludes: "(?m)^\\s*PermitRootLogin\\s+yes\\b"
        remediation:
          - "Set PermitRootLogin no"
          - "Restart sshd"
```

### 3. Run an action command on failure

```yaml
checks:
  - name: "encryption-check"
    command: "./scripts/check-disk-encryption.sh"
    schedule: "0 6 * * *"
    enabled: true
    onFail:
      commands:
        - name: "create-security-incident"
          command: "./scripts/create-incident.sh"
          args: ["--severity", "high"]
          timeout: "1m"
          continueOnError: true
```

## Check Output Format

Checks can emit plain text or JSON.  
If valid JSON is returned, the agent can extract findings and metadata.

Example JSON output:

```json
{
  "findings": [
    {
      "resource": "host-01",
      "title": "Root login enabled",
      "severity": "high",
      "status": "open"
    }
  ],
  "metrics": {
    "files_scanned": 12
  }
}
```

Non-zero exit code marks the check as failed.

## Evidence Behavior

When `evidence.enabled: true`, the agent can collect:

- files/directories listed in `checks[].evidencePaths`
- command stdout/stderr artifacts

Evidence and results are buffered locally first, then synced when API connectivity is available.

## Operation Modes

- `buffered`: Recommended. Buffers locally and retries API sync.
- `normal`: Requires API token and URL for connected operation.
- `standalone`: Intended for local-only runs. Leave `token` empty to keep API sync disabled.

## Environment Variables

You can inject settings via environment variables. Common ones:

- `OPENLANE_AGENT_TOKEN`
- `OPENLANE_AGENT_API_URL`
- `OPENLANE_AGENT_APIURL`

Legacy token aliases are also accepted for compatibility.
