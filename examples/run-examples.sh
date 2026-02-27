#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/lib/common.sh
source "${SCRIPT_DIR}/lib/common.sh"

usage() {
	cat <<'EOF'
Usage: ./scripts/run-examples.sh [all|modes|offline] [args...]

Targets:
  all               Run the local-only examples (modes + offline)
  modes             Run operation mode example
  offline           Run offline buffering example

Any extra args are forwarded to the selected target script (except "all").
EOF
}

target="${1:-all}"
if [[ $# -gt 0 ]]; then
	shift
fi

case "${target}" in
all)
	[[ $# -eq 0 ]] || example_die "'all' does not accept extra args"
	"${SCRIPT_DIR}/example-operation-modes.sh"
	"${SCRIPT_DIR}/example-offline-buffering.sh"
	;;
modes)
	"${SCRIPT_DIR}/example-operation-modes.sh" "$@"
	;;
offline)
	"${SCRIPT_DIR}/example-offline-buffering.sh" "$@"
	;;
-h | --help | help)
	usage
	;;
*)
	usage >&2
	example_die "unknown target: ${target}"
	;;
esac
