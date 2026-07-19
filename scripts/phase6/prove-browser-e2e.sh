#!/bin/sh
set -eu

case "$(uname -s)/$(uname -m)" in Darwin/arm64) ;; *) echo 'Phase 6 browser E2E requires macOS arm64.' >&2; exit 2 ;; esac
command -v jq >/dev/null

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)
"${repo_root}/scripts/phase6/browser-prerequisite.sh" --check >/dev/null
playwright_cli=${PLAYWRIGHT_CLI:-"${repo_root}/node_modules/.bin/playwright-cli"}
test -x "${playwright_cli}"
proof_root=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase6-browser.XXXXXX")
codex_home="${proof_root}/codex"
inspector_home="${proof_root}/inspector"
bin_dir="${proof_root}/bin"
browser_session="phase6-browser-$$"
handoff_pid=''
current_stage=setup
mkdir -p "${codex_home}/sessions" "${inspector_home}" "${bin_dir}"
chmod 700 "${proof_root}" "${codex_home}" "${codex_home}/sessions" "${inspector_home}" "${bin_dir}"

pw() {
  "${playwright_cli}" --session "${browser_session}" "$@"
}

run_pw() {
  stage=$1
  shift
  stage_log="${proof_root}/playwright-${stage}.log"
  if ! (cd "${proof_root}" && pw "$@" >"${stage_log}" 2>&1); then
    printf 'Phase 6 browser E2E failed at payload-free stage=%s\n' "${stage}" >&2
    sed -n '/Error:/p' "${stage_log}" | head -n 3 >&2
    return 1
  fi
  if grep -q '^### Error' "${stage_log}"; then
    printf 'Phase 6 browser E2E failed at payload-free stage=%s\n' "${stage}" >&2
    sed -n '/Error:/p' "${stage_log}" | head -n 3 >&2
    return 1
  fi
}

cleanup() {
  cleanup_status=$?
  set +e
  (cd "${proof_root}" && pw close >/dev/null 2>&1)
  if [ -n "${handoff_pid}" ]; then kill "${handoff_pid}" >/dev/null 2>&1; fi
  metadata="${inspector_home}/run/server.json"
  if [ -f "${metadata}" ]; then
    server_pid=$(jq -r '.pid // empty' "${metadata}" 2>/dev/null)
    case "${server_pid}" in
      ''|*[!0-9]*) ;;
      *)
        kill -TERM "${server_pid}" >/dev/null 2>&1
        attempts=0
        while kill -0 "${server_pid}" >/dev/null 2>&1 && [ "${attempts}" -lt 50 ]; do
          sleep 0.05
          attempts=$((attempts + 1))
        done
        ;;
    esac
  fi
  rm -rf -- "${proof_root}"
  if [ "${cleanup_status}" -ne 0 ]; then
    printf 'Phase 6 browser E2E failed at payload-free stage=%s\n' "${current_stage}" >&2
  fi
  return "${cleanup_status}"
}
trap cleanup EXIT HUP INT TERM

cp "${repo_root}/fixtures/synthetic/root.jsonl" "${repo_root}/fixtures/synthetic/descendant.jsonl" "${repo_root}/fixtures/synthetic/unsupported.jsonl" "${codex_home}/sessions/"
current_stage=build
"${repo_root}/scripts/phase1/build-web.sh" >/dev/null
GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/codex-inspector-phase6-gocache}" go build -o "${bin_dir}/codex-inspector" "${repo_root}/cmd/codex-inspector"
GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/codex-inspector-phase6-gocache}" go build -o "${bin_dir}/codex" "${repo_root}/tools/phase6/fakecodex"
GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/codex-inspector-phase6-gocache}" go build -o "${bin_dir}/browserhandoff" "${repo_root}/tools/phase3/browserhandoff"

current_stage=production_open
PATH="${bin_dir}:${PATH}" CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" \
  CODEX_INSPECTOR_PHASE6_E2E=1 CODEX_INSPECTOR_TEST_IDLE_TIMEOUT=120s \
  "${bin_dir}/codex-inspector" open -no-browser >/dev/null

metadata="${inspector_home}/run/server.json"
current_stage=metadata
attempts=0
while [ ! -s "${metadata}" ] && [ "${attempts}" -lt 100 ]; do
  sleep 0.05
  attempts=$((attempts + 1))
done
test -s "${metadata}"

handoff_file="${proof_root}/handoff-url"
current_stage=handoff
CODEX_INSPECTOR_HOME="${inspector_home}" "${bin_dir}/browserhandoff" --run-dir "${inspector_home}/run" >"${handoff_file}" 2>/dev/null &
handoff_pid=$!
attempts=0
while [ ! -s "${handoff_file}" ] && [ "${attempts}" -lt 100 ]; do
  sleep 0.05
  attempts=$((attempts + 1))
done
test -s "${handoff_file}"
handoff_url=$(sed -n '1p' "${handoff_file}")
case "${handoff_url}" in http://127.0.0.1:*/) ;; *) exit 1 ;; esac

current_stage=browser_open
run_pw open open --headed "${handoff_url}"
current_stage=browser_before_confirm
run_pw before run-code "$(sed -n '1,$p' "${repo_root}/scripts/phase6/playwright-before-confirm.js")"

current_stage=preconfirm_artifacts
review_count=$(find "${inspector_home}/reviews" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')
test "${review_count}" = "0"

current_stage=browser_after_confirm
run_pw after run-code "$(sed -n '1,$p' "${repo_root}/scripts/phase6/playwright-after-confirm.js")"
current_stage=accepted_review_count
review_count=$(find "${inspector_home}/reviews" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')
if [ "${review_count}" != "1" ]; then
  printf 'Phase 6 browser E2E accepted review directory count=%s\n' "${review_count}" >&2
  exit 1
fi
current_stage=accepted_report_artifact
find "${inspector_home}/reviews" -mindepth 2 -maxdepth 2 -name review.json -type f | grep -q .

current_stage=cleanup
cleanup
trap - EXIT HUP INT TERM
test ! -e "${proof_root}"
printf 'phase6_browser_e2e=passed production_launcher=true fragment_cookie=true preconfirm_reviews=0 report_rendered=true citation_return=true handoff_intercepted=true resume_copied=true payloads_emitted=0 artifacts_retained=0\n'
