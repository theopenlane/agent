#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
AGENT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
HELPER_PATH="${SCRIPT_DIR}/demo_api_helper.go"

API_URL="${OPENLANE_API_URL:-http://localhost:17608}"
API_TOKEN="${OPENLANE_API_TOKEN:-${OPENLANE_AGENT_REGISTRATION_TOKEN:-}}"

STANDARD_NAME="${DEMO_STANDARD_NAME:-Openlane Agent Demo Standard}"
STANDARD_VERSION="${DEMO_STANDARD_VERSION:-2026.1}"
STANDARD_DESCRIPTION="${DEMO_STANDARD_DESCRIPTION:-Deterministic demo standard for openlane-agent evidence association}"

CONTROL_REF="${DEMO_CONTROL_REF:-OLAGENT-DEMO-CONTROL-001}"
CONTROL_DESCRIPTION="${DEMO_CONTROL_DESCRIPTION:-Deterministic demo control used for agent evidence linking validation}"

CHECK_NAME="${DEMO_CHECK_NAME:-agent-control-demo-check}"
MAX_ITEMS="${DEMO_MAX_ITEMS:-250}"

CONFIG_PATH="${DEMO_CONFIG_PATH:-${AGENT_DIR}/agent.demo.control.yaml}"
CHECK_LOG_PATH="${DEMO_CHECK_LOG_PATH:-${AGENT_DIR}/demo-control-evidence.check.log}"

export GOCACHE="${GOCACHE:-/tmp/go-build}"

if ! command -v jq >/dev/null 2>&1; then
	echo "jq is required to run this demo script."
	exit 1
fi

if ! command -v go >/dev/null 2>&1; then
	echo "go is required to run this demo script."
	exit 1
fi

if [[ ! -f "${HELPER_PATH}" ]]; then
	echo "missing helper script: ${HELPER_PATH}"
	exit 1
fi

if [[ -z "${API_TOKEN}" ]]; then
	echo "OPENLANE_API_TOKEN (or OPENLANE_AGENT_REGISTRATION_TOKEN) must be set."
	exit 1
fi

print_step() {
	echo ""
	echo "==> $*"
}

print_step "Ensuring deterministic standard/control via go-client"
ensure_json="$(
	cd "${AGENT_DIR}"
	go run "${HELPER_PATH}" ensure \
		-api-url "${API_URL}" \
		-token "${API_TOKEN}" \
		-standard-name "${STANDARD_NAME}" \
		-standard-version "${STANDARD_VERSION}" \
		-standard-description "${STANDARD_DESCRIPTION}" \
		-control-ref "${CONTROL_REF}" \
		-control-description "${CONTROL_DESCRIPTION}" \
		-max-items "${MAX_ITEMS}"
)"

standard_id="$(jq -r '.standard_id // empty' <<<"${ensure_json}")"
control_id="$(jq -r '.control_id // empty' <<<"${ensure_json}")"

if [[ -z "${standard_id}" || -z "${control_id}" ]]; then
	echo "Failed to determine standard/control IDs."
	echo "${ensure_json}"
	exit 1
fi

echo "Standard ID: ${standard_id}"
echo "Control ID: ${control_id}"

print_step "Writing demo agent config: ${CONFIG_PATH}"
cat >"${CONFIG_PATH}" <<EOF
registrationToken: "${API_TOKEN}"
apiUrl: "${API_URL}"
agentName: "agent-control-demo"
logLevel: "debug"
dataDir: "./data-demo"
pollInterval: "10s"
spawn: 1
maxConcurrency: 1
defaultTimeout: "1m"
enableRemotePoll: false

offline:
  mode: "buffered"
  outputDir: "./results-demo"
  bufferDir: "./buffer-demo"
  syncInterval: "10s"

evidence:
  enabled: true
  retentionPeriod: "24h"
  maxFileSize: 10485760
  compressFiles: false

checks:
  - name: "${CHECK_NAME}"
    description: "Demo check for control-linked evidence uploads"
    command: "/bin/sh"
    args:
      - "-c"
      - "echo '{\"findings\":[{\"resource\":\"demo-host\",\"title\":\"demo finding\",\"severity\":\"low\",\"status\":\"open\"}]}'"
    schedule: "*/5 * * * * *"
    timeout: "20s"
    enabled: true
    compliance_standards:
      - standard: "${standard_id}"
        controls:
          - "${CONTROL_REF}"
EOF

print_step "Executing check once through agent CLI"
run_started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
(
	cd "${AGENT_DIR}"
	go run -tags cli main.go check --config "${CONFIG_PATH}" "${CHECK_NAME}" >"${CHECK_LOG_PATH}" 2>&1
)
cat "${CHECK_LOG_PATH}"

print_step "Verifying evidence is linked to control ${CONTROL_REF}"
verify_tmp="$(mktemp)"
set +e
(
	cd "${AGENT_DIR}"
	go run "${HELPER_PATH}" verify \
		-api-url "${API_URL}" \
		-token "${API_TOKEN}" \
		-check-name "${CHECK_NAME}" \
		-control-ref "${CONTROL_REF}" \
		-since "${run_started_at}" \
		-max-items "${MAX_ITEMS}" >"${verify_tmp}"
)
verify_exit=$?
set -e

verify_json="$(cat "${verify_tmp}")"
rm -f "${verify_tmp}"

if [[ ${verify_exit} -ne 0 && ${verify_exit} -ne 2 ]]; then
	echo "Verification helper failed."
	exit ${verify_exit}
fi

found="$(jq -r '.found // false' <<<"${verify_json}")"
if [[ "${found}" != "true" ]]; then
	echo "No control-linked evidence found for check '${CHECK_NAME}' after ${run_started_at}."
	echo "Candidate evidence rows:"
	jq -r '.candidates[]? | "\(.created_at // "-") \(.evidence_id // "-") \(.evidence_name // "-") controls=\((.control_refs // []) | join(","))"' <<<"${verify_json}" | tail -n 20
	exit 1
fi

evidence_id="$(jq -r '.evidence_id // empty' <<<"${verify_json}")"
evidence_name="$(jq -r '.evidence_name // empty' <<<"${verify_json}")"
linked_controls="$(jq -r '(.control_refs // []) | join(",")' <<<"${verify_json}")"

echo ""
echo "Demo succeeded."
echo "Standard ID: ${standard_id}"
echo "Control ID: ${control_id}"
echo "Evidence ID: ${evidence_id}"
echo "Evidence Name: ${evidence_name}"
echo "Evidence Controls: ${linked_controls}"
echo "Config Path: ${CONFIG_PATH}"
echo "Check Log: ${CHECK_LOG_PATH}"
