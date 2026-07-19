#!/bin/sh
set -eu

shell_quote() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }

case "$(uname -s)/$(uname -m)" in
  Darwin/arm64) ;;
  *) echo 'The published demo supports only macOS arm64.' >&2; exit 2 ;;
esac

release_tag=${CODEX_INSPECTOR_RELEASE_TAG:-v0.1.1}
release_repo=${CODEX_INSPECTOR_RELEASE_REPOSITORY:-dylanjbarth/codex-inspector}
source_codex_home=${CODEX_HOME:-"${HOME}/.codex"}
repo_root=$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)
test -f "${source_codex_home}/auth.json"
case "${release_tag}" in v0.1.1) ;; *) echo 'Published proof is pinned to corrected release v0.1.1.' >&2; exit 2 ;; esac
case "${release_repo}" in dylanjbarth/codex-inspector) ;; *) echo 'Release repository differs from the frozen marketplace contract.' >&2; exit 2 ;; esac

proof_root=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase6-install.XXXXXX")
cleanup() { rm -rf -- "${proof_root}"; }
trap cleanup EXIT HUP INT TERM
chmod 700 "${proof_root}"
codex_home="${proof_root}/codex"
inspector_home="${proof_root}/inspector"
download_dir="${proof_root}/download"
install_dir="${proof_root}/bin"
fixture_dir="${codex_home}/sessions/2026/07/19"
mkdir -p "${fixture_dir}" "${inspector_home}" "${download_dir}" "${install_dir}"
chmod 700 "${codex_home}" "${codex_home}/sessions" "${codex_home}/sessions/2026" "${codex_home}/sessions/2026/07" "${fixture_dir}" "${inspector_home}" "${download_dir}" "${install_dir}"
cp "${source_codex_home}/auth.json" "${codex_home}/auth.json"
chmod 600 "${codex_home}/auth.json"
cp "${repo_root}/fixtures/synthetic/root.jsonl" "${repo_root}/fixtures/synthetic/descendant.jsonl" "${repo_root}/fixtures/synthetic/unsupported.jsonl" "${fixture_dir}/"

base_url="https://github.com/${release_repo}/releases/download/${release_tag}"
artifact=codex-inspector-darwin-arm64
checksum=codex-inspector-darwin-arm64.sha256
if [ "${CODEX_INSPECTOR_PHASE6_DRY_RUN_RELEASE:-}" = "1" ]; then
  # The generated dry-run executable evaluates its own positional parameter.
  # shellcheck disable=SC2016
  printf '%s\n' '#!/bin/sh' 'case "${1:-}" in' \
    "  version) echo 'codex-inspector 0.1.0 (protocol 1, index schema 2)' ;;" \
    "  doctor) echo '{\"checks\":[{\"name\":\"synthetic\",\"status\":\"ok\"}]}' ;;" \
    'esac' >"${install_dir}/codex-inspector"
  chmod 755 "${install_dir}/codex-inspector"
  installed_hook="${codex_home}/plugins/cache/codex-inspector/v0.1.1/hooks/inspector-hook.sh"
  mkdir -p "$(dirname "${installed_hook}")"
  printf '%s\n' "if codex-inspector version | grep -q '^codex-inspector 0\\.1\\.[0-9][0-9]* (protocol 1, index schema 2)$'; then :; fi" >"${installed_hook}"
  chmod 755 "${installed_hook}"
  release_state=dry_run
else
  curl --fail --location --proto '=https' --tlsv1.2 --output "${download_dir}/${artifact}" "${base_url}/${artifact}"
  curl --fail --location --proto '=https' --tlsv1.2 --output "${download_dir}/${checksum}" "${base_url}/${checksum}"
  (cd "${download_dir}" && shasum -a 256 -c "${checksum}")
  install -m 0755 "${download_dir}/${artifact}" "${install_dir}/codex-inspector"

  PATH="${install_dir}:${PATH}" CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" codex-inspector version | grep -q '^codex-inspector 0\.1\.[0-9][0-9]* (protocol 1, index schema 2)$'
  CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" codex plugin marketplace add "dylanjbarth/codex-inspector@v0.1.1" >/dev/null
  CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" codex plugin add codex-inspector@codex-inspector-development --json >/dev/null
  CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" codex plugin list --json | jq -e '.installed[] | select(.name == "codex-inspector" and .enabled == true)' >/dev/null
  installed_hook=$(find "${codex_home}/plugins/cache" -type f -path '*/hooks/inspector-hook.sh' -print -quit)
  test -n "${installed_hook}"
  grep -q 'index schema 2' "${installed_hook}"
  if grep -q 'index schema 1' "${installed_hook}"; then
    echo 'Installed release hook contains the superseded schema 1 compatibility gate.' >&2
    exit 1
  fi
  release_state=verified
fi

environment_file="${proof_root}/environment.sh"
{
  # The generated environment evaluates PATH when it is sourced.
  # shellcheck disable=SC2016
  printf "export PATH=%s:\"\${PATH}\"\n" "$(shell_quote "${install_dir}")"
  printf 'export CODEX_HOME=%s\n' "$(shell_quote "${codex_home}")"
  printf 'export CODEX_INSPECTOR_HOME=%s\n' "$(shell_quote "${inspector_home}")"
} >"${environment_file}"
chmod 700 "${environment_file}"

post_hook_verify="${proof_root}/verify-after-hook.sh"
{
  printf '%s\n' '#!/bin/sh' 'set -eu'
  # The generated verifier evaluates these variables only when it is run.
  # shellcheck disable=SC2016
  printf 'export PATH=%s:"${PATH}"\n' "$(shell_quote "${install_dir}")"
  printf 'export CODEX_HOME=%s\n' "$(shell_quote "${codex_home}")"
  printf 'export CODEX_INSPECTOR_HOME=%s\n' "$(shell_quote "${inspector_home}")"
  printf 'installed_hook=%s\n' "$(shell_quote "${installed_hook}")"
  # shellcheck disable=SC2016
  printf '%s\n' \
    'grep -q '\''index schema 2'\'' "${installed_hook}"' \
    'if grep -q '\''index schema 1'\'' "${installed_hook}"; then exit 1; fi' \
    'if find "${CODEX_HOME}/plugins/data" -type f -name '\''hook-diagnostic-protocol_mismatch.json'\'' -print -quit 2>/dev/null | grep -q .; then exit 1; fi' \
    'doctor_file=$(mktemp "${TMPDIR:-/tmp}/codex-inspector-phase6-doctor.XXXXXX")' \
    'trap '\''rm -f -- "${doctor_file}"'\'' EXIT HUP INT TERM' \
    'if ! codex-inspector doctor --json >"${doctor_file}" 2>/dev/null; then exit 1; fi' \
    'jq -e '\''[.checks[].status] | length > 0 and all(. == "ok")'\'' "${doctor_file}" >/dev/null' \
    'printf '\''post_hook_doctor=healthy hook_protocol_mismatch=absent installed_hook_schema=2 payloads_emitted=0\n'\'''
} >"${post_hook_verify}"
chmod 700 "${post_hook_verify}"

# Hook trust is an intentional interactive Codex decision. Keep the isolated
# home only when explicitly requested so a human can perform that step and the
# remaining Section 1.1 rehearsal in this exact environment.
if [ "${CODEX_INSPECTOR_KEEP_INSTALL_PROOF:-}" = "1" ]; then
  trap - EXIT HUP INT TERM
  printf 'published_release=%s release_tag=v0.1.1 marketplace_source=dylanjbarth/codex-inspector@v0.1.1 plugin=%s installed_hook_schema=2 synthetic_sources=3 hook_trust=manual_required auth_copy=isolated proof_root=%s continuation_command=. %s post_hook_verify_command=%s cleanup_command=rm -rf -- %s\n' "${release_state}" "${release_state}" "${proof_root}" "$(shell_quote "${environment_file}")" "$(shell_quote "${post_hook_verify}")" "$(shell_quote "${proof_root}")"
else
  printf 'published_release=%s release_tag=v0.1.1 marketplace_source=dylanjbarth/codex-inspector@v0.1.1 plugin=%s installed_hook_schema=2 synthetic_sources=3 hook_trust=not_run auth_copy=removed proof_root=removed artifacts_retained=0\n' "${release_state}" "${release_state}"
fi
