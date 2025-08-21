# Openlane Compliance Agent - Technical Documentation

## Overview

The Openlane Agent is a lightweight, distributed compliance automation system designed to execute security and compliance checks across infrastructure and report results to the Openlane platform. It follows a worker-based architecture similar to Buildkite agents, with enhanced features for compliance use cases including evidence collection, buffered operation modes, and comprehensive retry mechanisms.

**Recent Architecture Improvements**: The agent now features a completely unified storage system that consolidates all result handling, evidence collection, and buffering functionality into a single, clean interface. This replaces the previous multi-package approach (buffer, evidence, storage, sync) with a more maintainable and understandable architecture using Go's functional options pattern.

## Architecture

### High-Level Design

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│  Openlane API   │◄───┤  Agent Workers   │◄───┤ Check Scripts   │
│   (GraphQL)     │    │                  │    │  & Evidence     │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         ▲                        │                       │
         │                        ▼                       ▼
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│  Control Sync   │    │ Unified Storage  │    │ Platform Logic  │
│    Service      │    │  - API Storage   │    │  - OS Detection │
└─────────────────┘    │  - Local Storage │    │  - Variants     │
                       │  - Auto Buffered │    └─────────────────┘
                       │  - Evidence Svc  │
                       └──────────────────┘
```

### Core Components

#### 1. **Agent Core (`core/agent.go`)**
- **Purpose**: Main agent orchestrator and lifecycle manager
- **Responsibilities**:
  - Agent registration with Openlane platform
  - Worker process management and spawning
  - Control synchronization coordination
  - Graceful shutdown handling
- **Key Features**:
  - Configurable worker spawn count
  - Hardware ID detection for agent identification
  - Integration with control synchronization service

#### 2. **Agent Worker (`core/agent_worker.go`)**
- **Purpose**: Individual worker processes that execute checks
- **Responsibilities**:
  - Polling Openlane API for remote scheduled jobs
  - Local check scheduling based on cron expressions
  - Concurrent check execution with configurable limits
  - Heartbeat reporting to platform
  - Result reporting through storage managers
- **Key Features**:
  - Dual execution mode: remote jobs + local checks
  - Configurable concurrency limits
  - Retry management with network-aware conditions
  - Statistics tracking and reporting

#### 3. **Compliance Check Controller (`core/compliance_check_controller.go`)**
- **Purpose**: Manages execution of individual compliance checks
- **Responsibilities**:
  - Command execution with environment setup
  - Check output parsing (JSON format)
  - Evidence collection and upload coordination
  - Pass/fail action execution
  - Platform-specific check variant handling
- **Key Features**:
  - Structured JSON output parsing with findings
  - Evidence file collection and upload
  - Platform-specific execution patterns (gitMDM-style)
  - Action execution for pass/fail scenarios

### Operation Modes

The agent supports three distinct operation modes:

#### 1. **Normal Mode** (`config.ModeNormal`)
- Standard operation with full API connectivity
- Real-time result reporting to Openlane platform
- Immediate evidence upload and control status updates
- Continuous polling for remote scheduled jobs

#### 2. **Standalone Mode** (`config.ModeStandalone`)
- Completely independent operation
- No API connectivity required
- Results stored locally in JSON/YAML/CSV formats
- Ideal for air-gapped environments or testing

#### 3. **Buffered Mode** (`config.ModeBuffered`)
- Normal operation with local buffering fallback
- Automatic detection of API connectivity issues
- Local result buffering when offline
- Automatic synchronization when connectivity resumes
- Configurable retry limits and cleanup policies

### Data Flow

#### Unified Check Execution Flow
1. **Scheduling**: Scheduler evaluates cron expressions for due checks
2. **Execution**: Check controller executes command with environment setup
3. **Parsing**: JSON output parsed into structured findings and evidence
4. **Evidence Collection**: Evidence service collects files from configured paths and command outputs
5. **Actions**: Pass/fail actions executed based on check results
6. **Unified Storage**: Single call to `StoreResultWithEvidence` handles both result and evidence
7. **Automatic Handling**: Storage implementation handles API transmission, local fallback, or standalone file storage

#### Intelligent Storage Flow
1. **Storage Selection**: Agent configuration determines storage mode (API, Local, or Buffered)
2. **Evidence Integration**: Evidence service automatically creates evidence files with metadata
3. **Storage Execution**: 
   - **API Mode**: Direct transmission to Openlane platform
   - **Local Mode**: File-based storage with configurable formats
   - **Buffered Mode**: API storage with automatic local fallback and background sync
4. **Connectivity Handling**: Buffered storage monitors connectivity and syncs when available
5. **Retry Management**: Built-in retry logic with exponential backoff for failed operations
6. **Statistics Tracking**: Comprehensive metrics across all storage operations

## Configuration System

### Configuration Structure (`internal/config/config.go`)

The agent uses a hierarchical configuration system with the following components:

#### Core Settings
```yaml
registrationToken: "${OPENLANE_REGISTRATION_TOKEN}"  # Required for API modes
apiUrl: "https://api.theopenlane.io"
agentName: "production-compliance-agent"
logLevel: "info"
dataDir: "./data"
pollInterval: "1m"
spawn: 1
maxConcurrency: 3
defaultTimeout: "5m"
```

#### Operation Mode Configuration
```yaml
offline:
  mode: "buffered"  # normal | standalone | buffered
  outputDir: "./results"       # For standalone mode
  bufferDir: "./buffer"        # For buffered mode
  connectivityInterval: "30s"  # Connectivity check frequency
  syncInterval: "5m"           # Buffer sync frequency
  maxRetries: 5               # Max retry attempts
```

#### Evidence Settings
```yaml
evidence:
  enabled: true
  retentionPeriod: "30d"
  maxFileSize: 104857600  # 100MB
  compressFiles: false
```

#### Retry Configuration
```yaml
retry:
  maxAttempts: 5
  initialDelay: "1s"
  maxDelay: "30s"
  strategy: "exponential"
  multiplier: 2.0
```

### Check Configuration

#### Basic Check Structure
```yaml
checks:
  - name: "aws-iam-compliance"
    description: "Check AWS IAM configuration"
    command: "./scripts/check-aws-iam.sh"
    schedule: "0 */4 * * *"  # Every 4 hours
    timeout: "10m"
    enabled: true
    controls:
      - "SOC2:CC6.1"
      - "ISO27001:A.9.2.1"
    tags:
      - "aws"
      - "iam"
    evidencePaths:
      - "./evidence/aws-iam/"
```

#### Platform-Specific Variants
The agent supports platform-specific check implementations:

```yaml
checks:
  - name: "disk-encryption-check"
    description: "Verify disk encryption"
    schedule: "0 6 * * *"
    
    platformVariants:
      # macOS implementation
      - platforms: ["darwin"]
        command: "fdesetup"
        args: ["status"]
        includes: "FileVault is On"
        
      # Linux implementation  
      - platforms: ["linux"]
        file: "/proc/mounts"
        includes: "luks"
        
      # Windows implementation
      - platforms: ["windows"]
        command: "manage-bde"
        args: ["-status", "C:"]
        excludes: "Protection Off"
```

#### Pass/Fail Actions
```yaml
checks:
  - name: "example-check"
    onPass:
      uploadEvidence: true
      updateControlStatus: true
    
    onFail:
      uploadEvidence: true
      updateControlStatus: true
      commands:
        - name: "create-incident"
          command: "./scripts/create-incident.sh"
          args: ["--severity", "high"]
          timeout: "1m"
```

## Unified Storage Architecture

### Storage System (`internal/storage/`)

The agent features a completely unified storage system that consolidates all result handling, evidence collection, and buffering functionality into a single, clean interface using Go's functional options pattern.

#### Core Storage Interface
```go
type Storage interface {
    StoreResult(ctx context.Context, result *config.Result) error
    StoreResultWithEvidence(ctx context.Context, result *config.Result, evidence []EvidenceFile) error
    GetStats() StorageStats
    Health() error
    Close() error
}
```

#### Storage Implementations

##### API Storage (`storage/api.go`)
- Direct integration with Openlane GraphQL API using existing `ReportResults` method
- Evidence upload capability via GraphQL client middleware
- Full compliance result upload including findings and evidence
- Health monitoring through API connectivity
- Real-time result transmission with evidence file upload support

##### Local Storage (`storage/local.go`)
- File-based storage supporting JSON, YAML, and CSV formats
- Evidence file management with metadata tracking
- Configurable output directories and file naming patterns
- Atomic file operations for reliability

##### Buffered Storage (`storage/buffered.go`)
- **Intelligent hybrid approach**: Attempts API storage first, automatically falls back to local buffering
- **Background synchronization**: Monitors connectivity and syncs buffered results when API becomes available
- **Built-in retry logic**: Configurable retry attempts with exponential backoff
- **Seamless operation**: Applications use the same interface regardless of connectivity state

#### Evidence Service (`storage/evidence.go`)

The unified evidence service handles all evidence collection and management:

##### Features
- **File Collection**: Gathers evidence from configured file paths with size limits and filtering
- **Command Output Evidence**: Automatically creates evidence from check stdout/stderr
- **Metadata Tracking**: SHA256 checksums, content types, creation timestamps
- **Retention Management**: Automatic cleanup based on configurable retention periods

##### Usage Example
```go
// Create storage with functional options
storage, err := storage.NewStorage(
    storage.WithBufferedStorage(apiURL, token, bufferDir),
    storage.WithEvidence(true, 30*24*time.Hour, 100*1024*1024, false),
)

// Evidence service automatically created and configured
evidenceFiles, _ := evidenceService.CollectEvidence(ctx, evidencePaths)

// Single call handles both result and evidence storage
err = storage.StoreResultWithEvidence(ctx, result, evidenceFiles)
```

#### Functional Options Pattern

The storage system uses functional options for clean, extensible configuration:

```go
// API storage with evidence
storage.NewStorage(
    storage.WithAPIStorage(apiURL, token),
    storage.WithEvidence(true, retention, maxSize, compress),
)

// Local storage for standalone mode
storage.NewStorage(
    storage.WithLocalStorage(dataDir, resultDir, format),
    storage.WithEvidence(true, retention, maxSize, false),
)

// Buffered storage with automatic fallback
storage.NewStorage(
    storage.WithBufferedStorage(apiURL, token, bufferDir),
    storage.WithEvidence(true, retention, maxSize, compress),
)
```

#### Configuration Integration

Storage configuration is automatically derived from agent configuration:

```go
// Automatically selects appropriate storage based on operation mode
storage, err := storage.NewStorageFromConfig(&agentConfig)

// Configuration helpers convert between config formats
opts := storage.StorageOptionsFromConfig(&agentConfig)
storage, err := storage.NewStorage(opts...)
```

#### Benefits of Unified Storage Architecture

The unified storage system provides several key advantages over the previous multi-package approach:

##### **Simplified Architecture**
- **Single Interface**: One storage interface replaces multiple overlapping packages
- **Reduced Complexity**: Eliminates confusion between buffer, evidence, storage, and sync packages
- **Clear Separation**: Storage logic cleanly separated from business logic

##### **Functional Options Pattern**
- **Clean Configuration**: `WithAPIStorage()`, `WithLocalStorage()`, `WithBufferedStorage()` options
- **Composable**: Mix and match storage options and evidence settings
- **Extensible**: Easy to add new storage backends without interface changes

##### **Intelligent Operation**
- **Dynamic Fallback**: Buffered storage automatically switches between API and local storage
- **Connectivity Awareness**: Built-in connectivity monitoring and automatic synchronization
- **Unified Evidence**: Single evidence service handles all evidence collection and management

##### **Maintainability**
- **Consistent Error Handling**: Unified error patterns across all storage operations
- **Comprehensive Metrics**: Single source of truth for storage statistics and health
- **Testable Design**: Clean interfaces make unit testing straightforward

## Platform Integration

### Platform Detection (`internal/platform/selector.go`)

The agent includes sophisticated platform detection and variant selection:

#### Supported Platforms
- **Operating Systems**: darwin, linux, windows
- **Distributions**: ubuntu, debian, rhel, centos, fedora
- **Architectures**: amd64, arm64, 386

#### Variant Selection Logic
1. **Exact Match**: OS/arch combination (e.g., "linux/amd64")
2. **OS Match**: Operating system only (e.g., "linux")
3. **Distribution Match**: Linux distribution (e.g., "ubuntu")
4. **Fallback**: Default check configuration

### Check Script Standards

#### Output Format
Compliance check scripts must output structured JSON:

```json
{
  "findings": [
    {
      "resource": "arn:aws:iam::123456789:user/john",
      "title": "User without MFA",
      "description": "IAM user does not have MFA enabled",
      "severity": "high",
      "status": "open",
      "details": {
        "user_name": "john",
        "mfa_devices": []
      }
    }
  ],
  "evidence": {
    "account_id": "123456789",
    "check_time": "2024-01-01T12:00:00Z"
  },
  "metrics": {
    "total_users": 5,
    "users_without_mfa": 1
  }
}
```

#### Environment Variables
The agent provides standard environment variables to check scripts:

```bash
OPENLANE_CHECK_NAME=aws-iam-compliance
OPENLANE_AGENT_ID=agent-12345
OPENLANE_CHECK_TIMEOUT=10m
OPENLANE_CONTROLS=SOC2:CC6.1,ISO27001:A.9.2.1
OPENLANE_TAGS=aws,iam
```

## Retry and Resilience

### Retry Manager (`internal/retry/manager.go`)

#### Retry Strategies
- **Exponential Backoff**: Default strategy with jitter
- **Linear Backoff**: Fixed increment delays
- **Random Backoff**: Random delay within bounds

#### Conditional Retries
The agent includes intelligent retry conditions:
- Network errors: Automatic retry
- Authentication errors: No retry (requires manual intervention)
- Timeout errors: Retry with longer timeout
- Resource errors: Retry after delay

#### Retry Configuration
```go
type Config struct {
    MaxAttempts  uint          `default:"5"`
    InitialDelay time.Duration `default:"1s"`
    MaxDelay     time.Duration `default:"30s"`
    Strategy     string        `default:"exponential"`
    Multiplier   float64       `default:"2.0"`
}
```

### Connectivity Management (`internal/connectivity/connectivity.go`)

#### Monitoring Features
- Periodic connectivity checks
- Health status tracking
- Connection state transitions
- Statistics and metrics

#### Connectivity States
- **Online**: Full API connectivity
- **Degraded**: Partial connectivity with issues
- **Offline**: No API connectivity

## Scheduling System

### Scheduler (`internal/scheduler/scheduler.go`)

#### Cron Expression Support
- Standard 5-field cron expressions
- Support for intervals (*/5)
- Wildcard and specific values
- Timezone-aware scheduling

#### Scheduling Features
- **Due Check Detection**: Identifies checks ready for execution
- **Concurrency Management**: Respects maximum execution limits
- **State Tracking**: Prevents duplicate executions
- **Statistics**: Execution counts and timing

#### Example Cron Expressions
```yaml
schedule: "0 */4 * * *"     # Every 4 hours
schedule: "*/30 * * * *"    # Every 30 minutes
schedule: "0 6 * * *"       # Daily at 6 AM
schedule: "0 0 * * 0"       # Weekly on Sunday
```

## Security Considerations

### Credential Management
- **API Tokens**: Stored securely with environment variable support
- **Evidence Encryption**: Optional encryption for sensitive evidence
- **Access Controls**: Platform-based access control integration

### Audit and Compliance
- **Execution Logging**: Comprehensive audit trails
- **Evidence Integrity**: Checksums and metadata tracking
- **Compliance Reporting**: Integration with compliance frameworks

### Network Security
- **TLS**: All API communications use TLS
- **Certificate Validation**: Strict certificate validation
- **Timeout Management**: Configurable timeouts to prevent hangs

## Deployment Patterns

### Standalone Deployment
```yaml
# agent.yaml
offline:
  mode: "standalone"
  outputDir: "./results"
  outputFormat: "json"

checks:
  - name: "local-security-check"
    command: "./scripts/security-scan.sh"
    schedule: "0 6 * * *"
```

**Use Cases**:
- Air-gapped environments
- Development and testing
- Compliance audits without platform integration

### Buffered Deployment
```yaml
# agent.yaml
offline:
  mode: "buffered"
  bufferDir: "./buffer"
  connectivityInterval: "30s"
  syncInterval: "5m"
  maxRetries: 5

registrationToken: "${OPENLANE_TOKEN}"
apiUrl: "https://api.theopenlane.io"
```

**Use Cases**:
- Unreliable network connectivity
- Edge deployments
- High-availability requirements

### Cloud-Native Deployment
```yaml
# agent.yaml
offline:
  mode: "normal"

registrationToken: "${OPENLANE_TOKEN}"
apiUrl: "https://api.theopenlane.io"
pollInterval: "30s"
maxConcurrency: 10
```

**Use Cases**:
- Kubernetes deployments
- Container orchestration
- Microservices architectures

## Monitoring and Observability

### Metrics and Statistics
The agent provides comprehensive metrics through multiple interfaces:

#### Agent-Level Metrics
```json
{
  "agent_id": "agent-12345",
  "worker_count": 3,
  "uptime": "24h15m30s",
  "total_checks": 150,
  "successful_checks": 142,
  "failed_checks": 8
}
```

#### Worker Metrics
```json
{
  "state": "busy",
  "active_checks": 2,
  "max_concurrency": 5,
  "operation_mode": "buffered",
  "last_heartbeat": "2024-01-01T12:00:00Z"
}
```

#### Unified Storage Metrics
```json
{
  "storage_type": "buffered",
  "healthy": true,
  "total_results": 150,
  "successful_uploads": 142,
  "failed_uploads": 8,
  "buffered_results": 5,
  "evidence_files": 75,
  "last_sync_attempt": "2024-01-01T12:00:00Z",
  "last_successful_sync": "2024-01-01T11:58:30Z"
}
```

### Logging
The agent uses structured logging with configurable levels:

```bash
# Debug level - detailed execution information
{"level":"debug","check":"aws-iam","msg":"Executing command: ./scripts/check-aws-iam.sh"}

# Info level - operational events
{"level":"info","check":"aws-iam","exit_code":0,"msg":"Check completed"}

# Warn level - non-fatal issues
{"level":"warn","msg":"API storage failed, buffering result"}

# Error level - execution failures
{"level":"error","check":"disk-encryption","msg":"Check execution failed"}
```

## Command Line Interface

### Available Commands

#### Start Agent
```bash
# Basic startup
openlane-agent start

# Custom configuration
openlane-agent start --config /path/to/agent.yaml

# Debug mode
openlane-agent start --log-level debug --no-daemon

# Dry run (validate configuration)
openlane-agent start --dry-run
```

#### Agent Management
```bash
# Check agent status
openlane-agent status

# Stop running agent
openlane-agent stop

# Run single check
openlane-agent check aws-iam-compliance
```

#### Configuration Management
```bash
# Initialize new configuration
openlane-agent config init

# Validate configuration
openlane-agent config validate

# Show current configuration
openlane-agent config show
```

#### Control Synchronization
```bash
# Sync controls with platform
openlane-agent sync-controls

# Show version information
openlane-agent version
```

### Configuration Flags
```bash
--config, -c          Configuration file path (default: agent.yaml)
--log-level          Log level: debug, info, warn, error
--data-dir           Data directory path
--api-key            Openlane API key (overrides config)
--api-url            Openlane API URL (overrides config)
--no-daemon          Run in foreground
--dry-run            Validate configuration and exit
--max-concurrency    Maximum concurrent checks
```

## Development and Testing

### Build System
The project uses Taskfile for build automation:

```bash
# Build agent binary
task build

# Run tests
task test

# Generate configuration schema
task config:generate

# Integration tests
task test:integration
```

### Testing Framework
```bash
# Unit tests
task test

# Storage system tests
go test ./internal/storage/... -v

# Test unified storage functionality
go run scripts/test-storage-functions.go

# Integration tests with real server
task test:integration

# Demo environment
task demo:prepare
task demo:run
```

### Development Workflow
```bash
# One-time setup
task dev:setup

# Complete test cycle
task dev:test

# Quick start for new users
task quickstart
```

## Troubleshooting

### Common Issues

#### Connection Problems
```bash
# Check API connectivity
curl -f https://api.theopenlane.io/livez

# Verify token
openlane-agent config validate

# Check agent logs
tail -f agent.log
```

#### Check Execution Issues
```bash
# Run check manually
openlane-agent check disk-encryption

# Debug mode
openlane-agent start --log-level debug --no-daemon

# Check evidence collection
ls -la ./data/evidence/
```

#### Storage and Buffering Issues
```bash
# Check storage status (via agent stats)
openlane-agent status

# Check buffered results directory
ls -la ./buffer/

# Check evidence storage
ls -la ./data/evidence/

# Check local result files (standalone/buffered mode)
ls -la ./results/

# Force sync by restarting agent (buffered mode)
openlane-agent stop && openlane-agent start

# Clear buffer (emergency only)
rm -rf ./buffer/*
```

### Log Analysis
```bash
# Filter by check name
grep "aws-iam" agent.log

# Check error patterns
grep "ERROR" agent.log

# Monitor real-time
tail -f agent.log | jq '.'
```

## Performance Considerations

### Scalability Factors
- **Worker Count**: Configure based on available CPU cores
- **Concurrency Limits**: Balance between throughput and resource usage
- **Poll Intervals**: Adjust based on check frequency requirements
- **Buffer Size**: Monitor buffer growth in disconnected scenarios

### Resource Management
- **Memory Usage**: Scales with concurrent check count and evidence size
- **Disk Usage**: Evidence and buffer storage requirements
- **Network Usage**: API communication and evidence upload bandwidth
- **CPU Usage**: Check execution and JSON parsing overhead

### Optimization Strategies
- **Check Batching**: Group related checks for efficiency
- **Evidence Compression**: Enable for large evidence files
- **Buffer Cleanup**: Regular cleanup of old buffered results
- **Connection Pooling**: Reuse API connections where possible

## Integration Patterns

### CI/CD Integration
```yaml
# GitLab CI example
compliance-check:
  stage: security
  script:
    - openlane-agent config init
    - openlane-agent start --dry-run  # Validate only
    - openlane-agent check security-baseline
```

### Container Deployment
```dockerfile
FROM golang:1.21-alpine AS builder
COPY . /app
WORKDIR /app
RUN go build -o openlane-agent .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=builder /app/openlane-agent /usr/local/bin/
COPY agent.yaml /etc/openlane/
CMD ["openlane-agent", "start", "-c", "/etc/openlane/agent.yaml"]
```

### Kubernetes Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: openlane-agent
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: agent
        image: openlane/agent:latest
        env:
        - name: OPENLANE_REGISTRATION_TOKEN
          valueFrom:
            secretKeyRef:
              name: openlane-secret
              key: token
        volumeMounts:
        - name: config
          mountPath: /etc/openlane
```

This technical documentation provides a comprehensive overview of the Openlane Agent architecture, configuration, and deployment patterns. The agent is designed for flexibility, reliability, and scalability across diverse compliance automation scenarios.