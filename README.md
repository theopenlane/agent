[![Go Report Card](https://goreportcard.com/badge/github.com/theopenlane/agent)](https://goreportcard.com/report/github.com/theopenlane/agent)
[![Build status](https://badge.buildkite.com/34ad31fe4231b2953cd3f2d116364d21a39b2a4dbf1eea539a.svg)](https://buildkite.com/theopenlane/agent?branch=main)
[![Go Reference](https://pkg.go.dev/badge/github.com/theopenlane/agent.svg)](https://pkg.go.dev/github.com/theopenlane/agent)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache2.0-brightgreen.svg)](https://opensource.org/licenses/Apache-2.0)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=theopenlane_REPONAME&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=theopenlane_REPONAME)


# agent

## Evidence Collection

### Configuration

Evidence collection is configured per check using the `evidence_paths` field:

```yaml
checks:
  - name: "disk-encryption-check"
    evidence_paths:
      - "./evidence/disk-encryption/"    # Directory containing evidence files
      - "./logs/encryption-audit.log"    # Specific file
      - "/var/log/security.log"          # Absolute path
```

### Automatic Evidence Creation

The agent automatically creates evidence from:
1. **Command Output**: `stdout` and `stderr` are saved as evidence files
2. **Configured Paths**: Files and directories specified in `evidence_paths`
3. **Script-Generated Files**: Scripts can create evidence files in `$OPENLANE_WORK_DIR/evidence/`

### Evidence File Types

Supported evidence file types:
- Text files (`.txt`, `.log`)
- JSON files (`.json`)
- Configuration files (`.yaml`, `.yml`, `.xml`)
- Images (`.png`, `.jpg`, `.jpeg`)
- Documents (`.pdf`)
- Scripts (`.sh`, `.py`, `.rb`)

## Pass/Fail Status Determination

The agent determines pass/fail status using the following logic:

1. **Execution Error**: If the command fails to execute → FAIL
2. **Exit Code**: Non-zero exit code → FAIL
3. **Critical/High Findings**: Any finding with `critical` or `high` severity → FAIL
4. **Otherwise**: PASS

### Example Script Output

Scripts should output JSON with findings:

```json
{
  "findings": [
    {
      "resource": "disk",
      "title": "FileVault Disabled",
      "description": "Full disk encryption is not active",
      "severity": "high",
      "status": "open"
    }
  ],
  "evidence": {
    "evidence_directory": "/tmp/evidence/disk-check/1234567890",
    "files_created": 3
  },
  "metrics": {
    "execution_time_seconds": 15,
    "passed": false
  }
}
```

## Pass/Fail Actions

### Configuration

Configure actions to execute on pass or fail outcomes:

```yaml
checks:
  - name: "security-check"
    on_pass:
      upload_evidence: true
      update_control_status: true
      notifications:
        - type: "slack"
          enabled: true
          config:
            channel: "#security"
            message: "✓ Security check passed"

    on_fail:
      upload_evidence: true
      update_control_status: true
      commands:
        - name: "create-ticket"
          command: "./scripts/create-ticket.sh"
          args: ["--priority", "high"]
          timeout: "1m"
      notifications:
        - type: "email"
          enabled: true
          config:
            to: "security@company.com"
            subject: "Security Check Failed"
```

### Available Actions

#### Evidence Upload
- `upload_evidence: true` - Uploads all collected evidence files to associated controls

#### Control Status Update
- `update_control_status: true` - Updates control compliance status in Openlane platform

#### Command Execution
Execute remediation or notification scripts:

```yaml
commands:
  - name: "remediation-action"
    command: "./scripts/fix-issue.sh"
    args: ["--auto-fix"]
    work_dir: "./remediation"
    env: ["FIX_MODE=auto"]
    timeout: "5m"
    continue_on_error: true
```

#### Notifications
Send notifications via multiple channels:

```yaml
notifications:
  - type: "slack"
    enabled: true
    config:
      channel: "#alerts"
      message: "Check failed: {{.check_name}}"

  - type: "email"
    enabled: true
    config:
      to: "team@company.com"
      subject: "Alert: {{.check_name}} failed"

  - type: "webhook"
    enabled: true
    config:
      url: "https://alerts.company.com/webhook"
      method: "POST"
```

## Example: Disk Encryption Check

Here's a complete example of a disk encryption compliance check:

```yaml
checks:
  - name: "disk-encryption-check"
    description: "Verify that full disk encryption is enabled"
    command: "./scripts/check-disk-encryption.sh"
    schedule: "0 6 * * *"  # Daily at 6 AM
    timeout: 5m

    controls:
      - "SOC2:CC6.7"
      - "ISO27001:A.10.1.1"
      - "NIST:SC-28"

    tags: ["encryption", "storage", "host-security"]
    enabled: true

    evidence_paths:
      - "./evidence/disk-encryption-check/"

    on_pass:
      upload_evidence: true
      update_control_status: true
      notifications:
        - type: "slack"
          enabled: true
          config:
            channel: "#security"
            message: "✓ Disk encryption verified"

    on_fail:
      upload_evidence: true
      update_control_status: true
      commands:
        - name: "create-incident"
          command: "./scripts/create-incident.sh"
          args: ["--type", "encryption", "--severity", "high"]
          timeout: "1m"
      notifications:
        - type: "email"
          enabled: true
          config:
            to: "security-team@company.com"
            subject: "CRITICAL: Disk encryption not enabled"
```

## Environment Variables

Scripts have access to these environment variables:

- `OPENLANE_CHECK_NAME` - Name of the executing check
- `OPENLANE_AGENT_ID` - Agent identifier
- `OPENLANE_CONTROLS` - Comma-separated list of associated controls
- `OPENLANE_TAGS` - Comma-separated list of tags
- `OPENLANE_WORK_DIR` - Working directory for evidence files
- `OPENLANE_DATA_DIR` - Agent data directory

## Evidence File Upload

Evidence files are automatically uploaded to controls when:
1. `upload_evidence: true` is configured in actions
2. The check has associated `controls`
3. Evidence files are successfully collected

Each evidence file is uploaded with metadata:
- Original file path
- Content type
- File size
- SHA256 checksum
- Upload timestamp
- Check name and control associations

## Security Considerations

- Evidence files may contain sensitive information
- Files are uploaded with appropriate access controls
- Checksums verify file integrity
- Evidence cleanup removes old files after configurable retention period
- Action commands run with limited privileges

## Troubleshooting

### Evidence Collection Issues
- Check file permissions on evidence paths
- Verify evidence directory creation permissions
- Review agent logs for collection errors

### Upload Failures
- Verify network connectivity to Openlane platform
- Check authentication tokens
- Review file size limits

### Action Execution Problems
- Verify script permissions and paths
- Check timeout settings
- Review environment variable requirements

## Migration from Simple Schema

To migrate existing checks to use the new features:

1. Add `evidence_paths` for files to collect
2. Configure `on_pass` and `on_fail` actions as needed
3. Update scripts to create evidence in `$OPENLANE_WORK_DIR/evidence/`
4. Test pass/fail logic with various scenarios
5. Verify notification and command integrations
