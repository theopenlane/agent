#!/bin/bash

# Example compliance check script for disk encryption
# This script checks if disk encryption is enabled on the system
# and creates evidence files to be uploaded to controls

set -euo pipefail

# Configuration
CHECK_NAME="${OPENLANE_CHECK_NAME:-disk-encryption-check}"
EVIDENCE_DIR="${OPENLANE_WORK_DIR:-/tmp}/evidence/${CHECK_NAME}/$(date +%s)"
FINDINGS=()

# Create evidence directory
mkdir -p "$EVIDENCE_DIR"

# Initialize result variables
PASSED=true
EXIT_CODE=0

# Function to add a finding
add_finding() {
    local resource="$1"
    local title="$2"
    local description="$3"
    local severity="$4"
    local status="${5:-open}"
    
    FINDINGS+=("$(cat <<EOF
{
  "resource": "$resource",
  "title": "$title", 
  "description": "$description",
  "severity": "$severity",
  "status": "$status",
  "details": {}
}
EOF
)")
}

# Function to log evidence
log_evidence() {
    local filename="$1"
    local content="$2"
    local evidence_file="$EVIDENCE_DIR/$filename"
    
    echo "$content" > "$evidence_file"
    echo "Created evidence file: $evidence_file" >&2
}

echo "Starting disk encryption compliance check..." >&2

# Check if running on macOS
if [[ "$OSTYPE" == "darwin"* ]]; then
    echo "Detected macOS system" >&2
    
    # Check FileVault status
    filevault_status=$(fdesetup status 2>/dev/null || echo "FileVault is not enabled")
    log_evidence "filevault_status.txt" "$filevault_status"
    
    # Get system information
    system_info=$(system_profiler SPHardwareDataType SPSoftwareDataType 2>/dev/null || echo "Unable to get system info")
    log_evidence "system_info.txt" "$system_info"
    
    if echo "$filevault_status" | grep -q "FileVault is On"; then
        echo "✓ FileVault encryption is enabled" >&2
        add_finding "disk" "FileVault Enabled" "Full disk encryption is active via FileVault" "info"
    else
        echo "✗ FileVault encryption is NOT enabled" >&2
        add_finding "disk" "FileVault Disabled" "Full disk encryption is not active - security risk" "high"
        PASSED=false
        EXIT_CODE=1
    fi
    
    # Check for external drives
    diskutil_info=$(diskutil list 2>/dev/null || echo "Unable to list disks")
    log_evidence "diskutil_list.txt" "$diskutil_info"
    
    # Count external volumes that might be unencrypted
    external_count=$(echo "$diskutil_info" | grep -c "external" || true)
    if [[ $external_count -gt 0 ]]; then
        add_finding "external-storage" "External Storage Detected" "Found $external_count external storage devices - verify encryption status" "medium"
    fi

# Check if running on Linux
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    echo "Detected Linux system" >&2
    
    # Check LUKS encryption status
    cryptsetup_status=""
    if command -v cryptsetup >/dev/null 2>&1; then
        cryptsetup_status=$(cryptsetup status /dev/mapper/* 2>/dev/null || echo "No LUKS devices found")
        log_evidence "cryptsetup_status.txt" "$cryptsetup_status"
    else
        cryptsetup_status="cryptsetup not available"
        log_evidence "cryptsetup_status.txt" "$cryptsetup_status"
    fi
    
    # Check /proc/mounts for encrypted filesystems
    mounts_info=$(cat /proc/mounts 2>/dev/null || echo "Unable to read /proc/mounts")
    log_evidence "proc_mounts.txt" "$mounts_info"
    
    # Check for dm-crypt devices
    dmsetup_info=$(dmsetup table 2>/dev/null || echo "dmsetup not available or no devices")
    log_evidence "dmsetup_table.txt" "$dmsetup_info"
    
    # Simple heuristic: look for encrypted filesystems
    if echo "$mounts_info" | grep -q "/dev/mapper/"; then
        if echo "$dmsetup_info" | grep -q "crypt"; then
            echo "✓ Encrypted filesystems detected" >&2
            add_finding "disk" "Disk Encryption Active" "LUKS/dm-crypt encryption detected on mounted filesystems" "info"
        else
            echo "⚠ Mapped devices found but encryption status unclear" >&2
            add_finding "disk" "Encryption Status Unknown" "Device mapper devices found but encryption type unclear" "medium"
        fi
    else
        echo "✗ No encrypted filesystems detected" >&2
        add_finding "disk" "No Disk Encryption" "No encrypted filesystems detected - security risk" "high"
        PASSED=false
        EXIT_CODE=1
    fi
    
    # Get disk information
    if command -v lsblk >/dev/null 2>&1; then
        lsblk_info=$(lsblk -f 2>/dev/null || echo "Unable to get block device info")
        log_evidence "lsblk_info.txt" "$lsblk_info"
    fi

else
    echo "✗ Unsupported operating system: $OSTYPE" >&2
    add_finding "system" "Unsupported OS" "Cannot check disk encryption on OS type: $OSTYPE" "medium"
    EXIT_CODE=1
fi

# Collect additional system evidence
echo "Collecting additional system evidence..." >&2

# Get running processes (for audit trail)
if command -v ps >/dev/null 2>&1; then
    ps_info=$(ps aux 2>/dev/null || ps -ef 2>/dev/null || echo "Unable to get process info")
    log_evidence "running_processes.txt" "$ps_info"
fi

# Get network interfaces (for context)
if command -v ip >/dev/null 2>&1; then
    ip_info=$(ip addr show 2>/dev/null || echo "ip command failed")
    log_evidence "network_interfaces.txt" "$ip_info"
elif command -v ifconfig >/dev/null 2>&1; then
    ifconfig_info=$(ifconfig 2>/dev/null || echo "ifconfig command failed") 
    log_evidence "network_interfaces.txt" "$ifconfig_info"
fi

# Create execution log
execution_log=$(cat <<EOF
Disk Encryption Compliance Check Execution Log
=============================================
Check Name: $CHECK_NAME
Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
OS Type: $OSTYPE
Working Directory: $(pwd)
Evidence Directory: $EVIDENCE_DIR
Agent ID: ${OPENLANE_AGENT_ID:-unknown}
Controls: ${OPENLANE_CONTROLS:-none}
Tags: ${OPENLANE_TAGS:-none}

Check Result: $(if $PASSED; then echo "PASSED"; else echo "FAILED"; fi)
Exit Code: $EXIT_CODE
Findings Count: ${#FINDINGS[@]}

Evidence Files Created:
$(ls -la "$EVIDENCE_DIR" 2>/dev/null || echo "No evidence files")
EOF
)

log_evidence "execution_log.txt" "$execution_log"

echo "Evidence collection completed. Files saved to: $EVIDENCE_DIR" >&2

# Format findings array for JSON output
findings_json=""
if [[ ${#FINDINGS[@]} -gt 0 ]]; then
    findings_json=$(printf "%s," "${FINDINGS[@]}")
    findings_json="[${findings_json%,}]"  # Remove trailing comma and wrap in array
else
    findings_json="[]"
fi

# Create structured JSON output
output_json=$(cat <<EOF
{
  "findings": $findings_json,
  "evidence": {
    "evidence_directory": "$EVIDENCE_DIR",
    "files_created": $(ls -1 "$EVIDENCE_DIR" 2>/dev/null | wc -l || echo 0),
    "os_type": "$OSTYPE",
    "check_timestamp": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  },
  "metrics": {
    "execution_time_seconds": $SECONDS,
    "evidence_files_count": $(ls -1 "$EVIDENCE_DIR" 2>/dev/null | wc -l || echo 0),
    "findings_count": ${#FINDINGS[@]},
    "passed": $PASSED
  }
}
EOF
)

echo "$output_json"

echo "Disk encryption compliance check completed with exit code: $EXIT_CODE" >&2
exit $EXIT_CODE