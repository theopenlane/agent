#!/usr/bin/env bash

# Shared helpers for example scripts in this directory.
set -euo pipefail

if [[ -n "${OPENLANE_EXAMPLE_COMMON_LOADED:-}" ]]; then
	return 0
fi
OPENLANE_EXAMPLE_COMMON_LOADED=1

readonly OPENLANE_EXAMPLE_SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
readonly OPENLANE_EXAMPLE_AGENT_DIR="$(cd "${OPENLANE_EXAMPLE_SCRIPT_DIR}/.." && pwd)"

OPENLANE_AGENT_CMD=()

example_die() {
	echo "error: $*" >&2
	exit 1
}

example_print_step() {
	echo ""
	echo "==> $*"
}

example_require_cmd() {
	local command_name="$1"

	if ! command -v "${command_name}" >/dev/null 2>&1; then
		example_die "required command not found: ${command_name}"
	fi
}

example_resolve_agent_cmd() {
	if [[ ${#OPENLANE_AGENT_CMD[@]} -gt 0 ]]; then
		return 0
	fi

	if [[ -n "${OPENLANE_AGENT_BIN:-}" ]]; then
		if [[ ! -x "${OPENLANE_AGENT_BIN}" ]]; then
			example_die "OPENLANE_AGENT_BIN is not executable: ${OPENLANE_AGENT_BIN}"
		fi

		OPENLANE_AGENT_CMD=("${OPENLANE_AGENT_BIN}")

		return 0
	fi

	if [[ -x "${OPENLANE_EXAMPLE_AGENT_DIR}/openlane-agent" ]]; then
		OPENLANE_AGENT_CMD=("${OPENLANE_EXAMPLE_AGENT_DIR}/openlane-agent")

		return 0
	fi

	example_require_cmd "go"
	OPENLANE_AGENT_CMD=("go" "run" "-tags" "cli" "./main.go")
}

example_agent_cmd_string() {
	example_resolve_agent_cmd

	local command_string=""
	local segment

	for segment in "${OPENLANE_AGENT_CMD[@]}"; do
		if [[ -n "${command_string}" ]]; then
			command_string+=" "
		fi

		command_string+="${segment}"
	done

	printf "%s" "${command_string}"
}

example_run_agent() {
	example_resolve_agent_cmd

	(
		cd "${OPENLANE_EXAMPLE_AGENT_DIR}"
		"${OPENLANE_AGENT_CMD[@]}" "$@"
	)
}

example_create_workspace() {
	local label="$1"
	local root_dir="${OPENLANE_EXAMPLE_WORKSPACE_ROOT:-${TMPDIR:-/tmp}}"

	mkdir -p "${root_dir}"

	mktemp -d "${root_dir%/}/openlane-agent-${label}.XXXXXX"
}
