#!/bin/sh
set -eu

case "$(uname -s)/$(uname -m)" in Darwin/arm64) ;; *) echo 'Phase 1 clean install supports only macOS arm64.' >&2; exit 2;; esac

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)
proof_root=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase1.XXXXXX")
trap 'rm -rf "${proof_root}"' EXIT HUP INT TERM
codex_home="${proof_root}/codex"
inspector_home="${proof_root}/inspector"
install_bin="${proof_root}/bin"
artifact_dir="${proof_root}/release"
plugin_data="${codex_home}/plugins/data/codex-inspector-codex-inspector-development"
mkdir -p "${codex_home}" "${install_bin}" "${artifact_dir}"

"${repo_root}/scripts/phase1/package-macos.sh" "${artifact_dir}"
(cd "${artifact_dir}" && shasum -a 256 -c codex-inspector-darwin-arm64.sha256)
printf '%s\n' 'stage=package_verified'
install -m 0755 "${artifact_dir}/codex-inspector-darwin-arm64" "${install_bin}/codex-inspector"
PATH="${install_bin}:${PATH}" codex-inspector version | grep -q '^codex-inspector 0.1.0 (protocol 1, index schema 2)$'

CODEX_HOME="${codex_home}" codex plugin marketplace add "${repo_root}" >/dev/null
CODEX_HOME="${codex_home}" codex plugin add codex-inspector@codex-inspector-development --json >/dev/null
CODEX_HOME="${codex_home}" codex plugin list --json | jq -e '.installed[] | select(.name == "codex-inspector" and .enabled == true)' >/dev/null
printf '%s\n' 'stage=plugin_discovered'

# Missing CLI is non-blocking and cannot create derived state.
for fixture in "${repo_root}"/fixtures/hooks/*.json; do
  PATH="/usr/bin:/bin" PLUGIN_ROOT="${repo_root}/plugin/codex-inspector" PLUGIN_DATA="${plugin_data}" "${repo_root}/plugin/codex-inspector/hooks/inspector-hook.sh" < "${fixture}"
done
test ! -e "${inspector_home}/queue"
test "$(stat -f '%Lp' "${plugin_data}/hook-diagnostic-missing_cli.json")" = 600
if grep -q 'PAYLOAD-CANARY' "${plugin_data}/hook-diagnostic-missing_cli.json"; then
  exit 1
fi
if PATH="${install_bin}:${PATH}" CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" codex-inspector doctor --json > "${proof_root}/missing-doctor.json" 2>/dev/null; then :; fi
jq -e '.checks[] | select(.name == "hook_missing_cli" and .status == "error")' "${proof_root}/missing-doctor.json" >/dev/null
printf '%s\n' 'stage=missing_cli_nonblocking'

# An incompatible executable is also non-blocking and leaves only a current,
# payload-free plugin diagnostic until a compatible hook run clears it.
fake_bin="${proof_root}/fake-bin"
mkdir -p "${fake_bin}"
printf '%s\n' '#!/bin/sh' "echo 'codex-inspector 0.1.0 (protocol 1, index schema 1)'" > "${fake_bin}/codex-inspector"
chmod 700 "${fake_bin}/codex-inspector"
PATH="${fake_bin}:/usr/bin:/bin" PLUGIN_DATA="${plugin_data}" "${repo_root}/plugin/codex-inspector/hooks/inspector-hook.sh" < "${repo_root}/fixtures/hooks/stop.json"
test "$(stat -f '%Lp' "${plugin_data}/hook-diagnostic-protocol_mismatch.json")" = 600
if PATH="${install_bin}:${PATH}" CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" codex-inspector doctor --json > "${proof_root}/protocol-doctor.json" 2>/dev/null; then :; fi
jq -e '.checks[] | select(.name == "hook_protocol_mismatch" and .status == "error")' "${proof_root}/protocol-doctor.json" >/dev/null
printf '%s\n' 'stage=protocol_mismatch_nonblocking'

# The installed CLI writes marker-only queue state and never creates SQLite.
PATH="${install_bin}:${PATH}" CODEX_INSPECTOR_HOME="${inspector_home}" PLUGIN_ROOT="${repo_root}/plugin/codex-inspector" PLUGIN_DATA="${plugin_data}" "${repo_root}/plugin/codex-inspector/hooks/inspector-hook.sh" < "${repo_root}/fixtures/hooks/stop.json"
test ! -e "${plugin_data}/hook-diagnostic-missing_cli.json"
test ! -e "${plugin_data}/hook-diagnostic-protocol_mismatch.json"
test "$(find "${inspector_home}/queue" -type f -name '*.json' | wc -l | tr -d ' ')" = 1
test ! -e "${inspector_home}/inspector.db"
printf '%s\n' 'stage=hook_marker_only'

# Prove stale PID/metadata is not treated as a reusable process.
mkdir -p "${inspector_home}/run"
printf '%s' '{"instanceId":"stale-instance","pid":999999,"port":9,"protocolVersion":1,"codexHome":"/tmp/stale-codex","codexHomeSource":"environment","startedAt":"2026-07-18T00:00:00Z"}' > "${inspector_home}/run/server.json"
chmod 600 "${inspector_home}/run/server.json"
PATH="${install_bin}:${PATH}" CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" CODEX_INSPECTOR_TEST_IDLE_TIMEOUT=2s codex-inspector open --no-browser >/dev/null
first_instance=$(jq -r .instanceId "${inspector_home}/run/server.json")
test "${first_instance}" != stale-instance
printf '%s\n' 'stage=stale_metadata_recovered'
PATH="${install_bin}:${PATH}" CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" codex-inspector open --no-browser >/dev/null
test "$(jq -r .instanceId "${inspector_home}/run/server.json")" = "${first_instance}"
printf '%s\n' 'stage=open_reused'
port=$(jq -r .port "${inspector_home}/run/server.json")
origin="http://127.0.0.1:${port}"
curl --fail --silent --show-error "${origin}/" | grep -q 'Codex Inspector'
curl --fail --silent --show-error "${origin}/v1/status" > "${proof_root}/dashboard-status.json"
if ! jq -e '.process.state == "degraded" and .process.cliCompatibility == "unknown" and .index.state == "empty" and .hook.state == "idle" and (.hook.diagnostics | length) == 0' "${proof_root}/dashboard-status.json" >/dev/null; then
  jq '{process: .process, index: {state: .index.state}, hook: {state: .hook.state, diagnostics: .hook.diagnostics}}' "${proof_root}/dashboard-status.json" >&2
  exit 1
fi
PATH="${install_bin}:${PATH}" CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" codex-inspector sync --background | grep -q '"state"'
printf '%s\n' 'stage=dashboard_accessible'
curl --fail --silent --show-error -H "Origin: ${origin}" -X POST "${origin}/v1/heartbeat" >/dev/null
sleep 3
PATH="${install_bin}:${PATH}" CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" codex-inspector status --json | jq -e '.running == false' >/dev/null
printf '%s\n' 'stage=idle_exit'

# Hook trust remains an intentional interactive Codex decision. The automated
# proof verifies all other doctor checks and emits no source payloads.
if PATH="${install_bin}:${PATH}" CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" codex-inspector doctor --json > "${proof_root}/doctor.json" 2> "${proof_root}/doctor.stderr"; then :; fi
if ! jq -e '[.checks[] | select(.name != "hook_trust") | .status] | all(. == "ok")' "${proof_root}/doctor.json" >/dev/null; then
  jq -r '.checks[] | select(.name != "hook_trust" and .status != "ok") | "doctor_failure=\(.name):\(.status):\(.detail)"' "${proof_root}/doctor.json" >&2
  exit 1
fi
printf '%s\n' 'stage=doctor_noninteractive'

printf '%s\n' 'artifact_checksum=verified plugin=discoverable missing_cli=nonblocking hook_marker=payload_free open=start_and_reuse dashboard=loaded idle_exit=observed doctor=all_noninteractive_checks_pass hook_trust=manual_required'
