#!/bin/sh
set -eu

repo_root=$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)
test_root=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase6-script-test.XXXXXX")
cleanup() { rm -rf -- "${test_root}"; }
trap cleanup EXIT HUP INT TERM
chmod 700 "${test_root}"
source_home="${test_root}/source-codex"
mkdir -p "${source_home}"
printf '{"synthetic":"phase6-cleanup-test"}\n' >"${source_home}/auth.json"
chmod 600 "${source_home}/auth.json"

set +e
TMPDIR="${test_root}" CODEX_HOME="${source_home}" CODEX_INSPECTOR_RUN_LIVE_PROOFS=1 \
  CODEX_INSPECTOR_PHASE6_FAIL_AFTER_AUTH_COPY=1 \
  "${repo_root}/scripts/phase6/prepare-local-candidate-proof.sh" >/dev/null 2>&1
failure_status=$?
set -e
test "${failure_status}" = "91"
test -z "$(find "${test_root}" -maxdepth 1 -type d -name 'codex-inspector-phase6-local.*' -print -quit)"

TMPDIR="${test_root}" CODEX_HOME="${source_home}" CODEX_INSPECTOR_PHASE6_DRY_RUN_RELEASE=1 \
  CODEX_INSPECTOR_KEEP_INSTALL_PROOF=1 \
  "${repo_root}/scripts/phase6/prove-published-clean-install.sh" >"${test_root}/published-output"
proof_root=$(find "${test_root}" -maxdepth 1 -type d -name 'codex-inspector-phase6-install.*' -print -quit)
test -n "${proof_root}"
test -f "${proof_root}/environment.sh"
test -x "${proof_root}/verify-after-hook.sh"
test "$(find "${proof_root}/codex/sessions" -type f -name '*.jsonl' | wc -l | tr -d ' ')" = "3"
test "$(find "${proof_root}/codex/sessions" -mindepth 1 -maxdepth 1 -type f | wc -l | tr -d ' ')" = "0"
test "$(find "${proof_root}/codex/sessions/2026/07/19" -type f -name '*.jsonl' | wc -l | tr -d ' ')" = "3"
test "$(stat -f '%Lp' "${proof_root}/codex/auth.json")" = "600"
if grep -Eq '(^|[=:/])(~|\.codex)(/|$)' "${proof_root}/environment.sh"; then exit 1; fi
# Variables are intentionally evaluated by the isolated child shell.
# shellcheck disable=SC2016
env -i PATH=/usr/bin:/bin EXPECT_CODEX_HOME="${proof_root}/codex" EXPECT_INSPECTOR_HOME="${proof_root}/inspector" \
  /bin/sh -c '. "$1"; test "$CODEX_HOME" = "$EXPECT_CODEX_HOME"; test "$CODEX_INSPECTOR_HOME" = "$EXPECT_INSPECTOR_HOME"; case "$PATH" in "$2":*) ;; *) exit 1 ;; esac' \
  phase6-environment "${proof_root}/environment.sh" "${proof_root}/bin"
grep -q 'published_release=dry_run' "${test_root}/published-output"
grep -q 'release_tag=v0.1.1' "${test_root}/published-output"
grep -q 'marketplace_source=dylanjbarth/codex-inspector@v0.1.1' "${test_root}/published-output"
grep -q 'continuation_command=' "${test_root}/published-output"
grep -q 'post_hook_verify_command=' "${test_root}/published-output"
grep -q 'cleanup_command=rm -rf --' "${test_root}/published-output"
"${proof_root}/verify-after-hook.sh" >"${test_root}/post-hook-output"
grep -q 'post_hook_doctor=healthy hook_protocol_mismatch=absent installed_hook_schema=2 payloads_emitted=0' "${test_root}/post-hook-output"
grep -q 'marketplace add "dylanjbarth/codex-inspector@v0.1.1"' "${repo_root}/scripts/phase6/prove-published-clean-install.sh"
grep -q 'v0.1.0' "${repo_root}/marketplace/release.json"
grep -q 'v0.1.1' "${repo_root}/plugin/codex-inspector/skills/setup/SKILL.md"
if grep -q 'releases/download/v0.1.0' "${repo_root}/plugin/codex-inspector/skills/setup/SKILL.md"; then exit 1; fi
rm -rf -- "${proof_root}"

set +e
TMPDIR="${test_root}" CODEX_HOME="${source_home}" CODEX_INSPECTOR_RELEASE_TAG=v0.1.0 \
  CODEX_INSPECTOR_PHASE6_DRY_RUN_RELEASE=1 \
  "${repo_root}/scripts/phase6/prove-published-clean-install.sh" >/dev/null 2>&1
wrong_tag_status=$?
set -e
test "${wrong_tag_status}" = "2"

TMPDIR="${test_root}" CODEX_HOME="${source_home}" CODEX_INSPECTOR_PHASE6_DRY_RUN_RELEASE=1 \
  "${repo_root}/scripts/phase6/prove-published-clean-install.sh" >/dev/null
test -z "$(find "${test_root}" -maxdepth 1 -type d -name 'codex-inspector-phase6-install.*' -print -quit)"

printf 'phase6_proof_script_regression=passed release_tag=v0.1.1 marketplace_pin=true post_hook_doctor=true frozen_product_metadata=0.1.0 fail_path_auth_cleanup=true continuation_homes=isolated default_cleanup=true payloads_emitted=0\n'
