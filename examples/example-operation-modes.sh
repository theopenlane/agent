#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/lib/common.sh
source "${SCRIPT_DIR}/lib/common.sh"

usage() {
	cat <<'EOF'
Usage: ./scripts/example-operation-modes.sh [--workspace PATH]

Creates one standalone, buffered, and normal config, validates each config,
then runs a single check in standalone mode.

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
	WORKSPACE_PATH="$(example_create_workspace "operation-modes")"
fi

CHECK_SCRIPT_PATH="${OPENLANE_EXAMPLE_CHECK_SCRIPT:-${OPENLANE_EXAMPLE_AGENT_DIR}/scripts/check-disk-encryption.sh}"
CHECK_NAME="${OPENLANE_EXAMPLE_CHECK_NAME:-disk-encryption-check}"

if [[ ! -x "${CHECK_SCRIPT_PATH}" ]]; then
	example_die "example check script is not executable: ${CHECK_SCRIPT_PATH}"
fi

write_mode_config() {
	local mode="$1"
	local output_path="$2"

	{
		if [[ "${mode}" != "standalone" ]]; then
			cat <<EOF
token: "demo-token"
apiUrl: "http://127.0.0.1:17608"
EOF
		fi

		cat <<EOF
agentName: "example-${mode}-agent"
logLevel: "info"
dataDir: "${WORKSPACE_PATH}/data-${mode}"
pollInterval: "30s"
spawn: 1
maxConcurrency: 1
defaultTimeout: "30s"

offline:
  mode: "${mode}"
  outputDir: "${WORKSPACE_PATH}/results-${mode}"
  outputFormat: "json"
  bufferDir: "${WORKSPACE_PATH}/buffer-${mode}"
  connectivityInterval: "10s"
  syncInterval: "30s"
  maxRetries: 3
  bufferRetentionPeriod: "24h"

evidence:
  enabled: true
  retentionPeriod: "24h"
  maxFileSize: 10485760
  compressFiles: false

checks:
  - name: "${CHECK_NAME}"
    description: "Customer-style disk encryption compliance check example"
    command: "${CHECK_SCRIPT_PATH}"
    schedule: "*/5 * * * *"
    timeout: "20s"
    enabled: true
    env:
      - "OPENLANE_WORK_DIR=${WORKSPACE_PATH}"
    evidencePaths:
      - "${WORKSPACE_PATH}/evidence/${CHECK_NAME}"
EOF
	} >"${output_path}"
}

STANDALONE_CONFIG="${WORKSPACE_PATH}/standalone.yaml"
BUFFERED_CONFIG="${WORKSPACE_PATH}/buffered.yaml"
NORMAL_CONFIG="${WORKSPACE_PATH}/normal.yaml"

write_mode_config "standalone" "${STANDALONE_CONFIG}"
write_mode_config "buffered" "${BUFFERED_CONFIG}"
write_mode_config "normal" "${NORMAL_CONFIG}"

example_print_step "Using agent command: $(example_agent_cmd_string)"

example_print_step "Validating standalone mode configuration"
example_run_agent config validate --config "${STANDALONE_CONFIG}"

example_print_step "Validating buffered mode configuration"
example_run_agent config validate --config "${BUFFERED_CONFIG}"

example_print_step "Validating normal mode configuration"
example_run_agent config validate --config "${NORMAL_CONFIG}"

example_print_step "Running ${CHECK_NAME} once in standalone mode"
set +e
example_run_agent check "${CHECK_NAME}" --config "${STANDALONE_CONFIG}"
check_exit=$?
set -e

if [[ ${check_exit} -ne 0 ]]; then
	echo ""
	echo "Check exited non-zero (${check_exit}). This is expected on non-compliant hosts."
	echo "The example still demonstrates real check execution and mode wiring."
fi

echo ""
echo "Operation mode example complete."
echo "Workspace: ${WORKSPACE_PATH}"
echo ""
echo "Try these next:"
echo "  $(example_agent_cmd_string) check ${CHECK_NAME} --config ${BUFFERED_CONFIG}"
echo "  $(example_agent_cmd_string) check ${CHECK_NAME} --config ${NORMAL_CONFIG}"
