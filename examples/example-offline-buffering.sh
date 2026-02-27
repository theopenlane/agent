#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/lib/common.sh
source "${SCRIPT_DIR}/lib/common.sh"

usage() {
	cat <<'EOF'
Usage: ./scripts/example-offline-buffering.sh [--workspace PATH]

Runs one check in buffered mode against an intentionally unreachable API URL,
then verifies that a buffered result file is written locally.

Options:
  --workspace PATH   Reuse/write example files in PATH
  -h, --help         Show this help text
EOF
}

WORKSPACE_PATH=""

while [[ $# -gt 0 ]]; do
	case "$1" in
	--workspace)
		[[ $# -ge 2 ]] || example_die "--workspace requires a value"
		WORKSPACE_PATH="$2"
		shift 2
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		example_die "unknown argument: $1"
		;;
	esac
done

if [[ -z "${WORKSPACE_PATH}" ]]; then
	WORKSPACE_PATH="$(example_create_workspace "offline-buffering")"
fi

mkdir -p "${WORKSPACE_PATH}/evidence"

CHECK_SCRIPT_PATH="${OPENLANE_EXAMPLE_CHECK_SCRIPT:-${OPENLANE_EXAMPLE_AGENT_DIR}/scripts/check-disk-encryption.sh}"
CHECK_NAME="${OPENLANE_EXAMPLE_CHECK_NAME:-offline-disk-encryption-check}"
CONFIG_PATH="${WORKSPACE_PATH}/offline-buffered.yaml"
CHECK_LOG_PATH="${WORKSPACE_PATH}/offline-buffered.check.log"
BUFFER_DIR_PATH="${WORKSPACE_PATH}/buffer"

if [[ ! -x "${CHECK_SCRIPT_PATH}" ]]; then
	example_die "example check script is not executable: ${CHECK_SCRIPT_PATH}"
fi

cat >"${CONFIG_PATH}" <<EOF
token: "demo-token"
apiUrl: "http://127.0.0.1:1"
agentName: "example-offline-buffer-agent"
logLevel: "debug"
dataDir: "${WORKSPACE_PATH}/data"
pollInterval: "30s"
spawn: 1
maxConcurrency: 1
defaultTimeout: "30s"

offline:
  mode: "buffered"
  outputDir: "${WORKSPACE_PATH}/results"
  outputFormat: "json"
  bufferDir: "${BUFFER_DIR_PATH}"
  connectivityInterval: "5s"
  syncInterval: "1m"
  maxRetries: 3
  bufferRetentionPeriod: "24h"

evidence:
  enabled: true
  retentionPeriod: "24h"
  maxFileSize: 10485760
  compressFiles: false

checks:
  - name: "${CHECK_NAME}"
    description: "Runs customer-style disk encryption check with local buffering fallback"
    command: "${CHECK_SCRIPT_PATH}"
    schedule: "*/5 * * * *"
    timeout: "20s"
    enabled: true
    env:
      - "OPENLANE_WORK_DIR=${WORKSPACE_PATH}"
      - "OPENLANE_CHECK_NAME=${CHECK_NAME}"
    evidencePaths:
      - "${WORKSPACE_PATH}/evidence/${CHECK_NAME}"
EOF

example_print_step "Using agent command: $(example_agent_cmd_string)"

example_print_step "Validating buffered-mode config"
example_run_agent config validate --config "${CONFIG_PATH}"

example_print_step "Running check once (expected API upload failure with local buffering fallback)"
set +e
example_run_agent check "${CHECK_NAME}" --config "${CONFIG_PATH}" >"${CHECK_LOG_PATH}" 2>&1
check_exit=$?
set -e
cat "${CHECK_LOG_PATH}"

if [[ ${check_exit} -ne 0 ]]; then
	echo ""
	echo "Check exited non-zero (${check_exit}). This is expected on non-compliant hosts."
	echo "Buffered mode behavior is still valid as long as local buffer files exist."
fi

buffer_file_count="$(find "${BUFFER_DIR_PATH}" -type f -name '*.json' 2>/dev/null | wc -l | tr -d ' ')"
evidence_file_count="$(find "${WORKSPACE_PATH}/evidence" -type f 2>/dev/null | wc -l | tr -d ' ')"

if [[ "${buffer_file_count}" -eq 0 ]]; then
	echo ""
	echo "No buffered result files were detected in ${BUFFER_DIR_PATH}."
	echo "Inspect the check log for details: ${CHECK_LOG_PATH}"
	exit 1
fi

echo ""
echo "Offline buffering example complete."
echo "Workspace: ${WORKSPACE_PATH}"
echo "Buffered result files: ${buffer_file_count}"
echo "Evidence files: ${evidence_file_count}"
echo ""
echo "Buffered files:"
find "${BUFFER_DIR_PATH}" -maxdepth 1 -type f -name '*.json' -print
