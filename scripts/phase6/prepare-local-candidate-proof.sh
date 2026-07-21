#!/bin/sh
set -eu

shell_quote() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }

if [ "${CODEX_INSPECTOR_RUN_LIVE_PROOFS:-}" != "1" ]; then
  echo 'Set CODEX_INSPECTOR_RUN_LIVE_PROOFS=1 for an authorized disposable proof.' >&2
  exit 2
fi
case "$(uname -s)/$(uname -m)" in Darwin/arm64) ;; *) exit 2 ;; esac

source_codex_home=${CODEX_HOME:-"${HOME}/.codex"}
test -f "${source_codex_home}/auth.json"
repo_root=$(CDPATH='' cd -- "$(dirname "$0")/../.." && pwd)
proof_root=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase6-local.XXXXXX")
cleanup() { rm -rf -- "${proof_root}"; }
trap cleanup EXIT HUP INT TERM
codex_home="${proof_root}/codex"
inspector_home="${proof_root}/inspector"
install_bin="${proof_root}/bin"
release_dir="${proof_root}/candidate-release"
mkdir -p "${codex_home}/sessions" "${inspector_home}" "${install_bin}" "${release_dir}"
chmod 700 "${proof_root}" "${codex_home}" "${codex_home}/sessions" "${inspector_home}" "${install_bin}" "${release_dir}"
cp "${source_codex_home}/auth.json" "${codex_home}/auth.json"
chmod 600 "${codex_home}/auth.json"
if [ "${CODEX_INSPECTOR_PHASE6_FAIL_AFTER_AUTH_COPY:-}" = "1" ]; then
  echo 'Intentional Phase 6 cleanup-test failure after isolated auth copy.' >&2
  exit 91
fi
cp "${repo_root}/fixtures/synthetic/root.jsonl" "${repo_root}/fixtures/synthetic/descendant.jsonl" "${repo_root}/fixtures/synthetic/unsupported.jsonl" "${codex_home}/sessions/"

"${repo_root}/scripts/phase1/build-web.sh" >/dev/null
"${repo_root}/scripts/phase1/package-macos.sh" "${release_dir}"
(cd "${release_dir}" && shasum -a 256 -c codex-inspector-darwin-arm64.sha256 >/dev/null)
install -m 0755 "${release_dir}/codex-inspector-darwin-arm64" "${install_bin}/codex-inspector"
CODEX_HOME="${codex_home}" codex plugin marketplace add "${repo_root}" >/dev/null
CODEX_HOME="${codex_home}" codex plugin add codex-inspector@codex-inspector-development --json >/dev/null

if [ "${CODEX_INSPECTOR_KEEP_LOCAL_PROOF:-}" = "1" ]; then
  trap - EXIT HUP INT TERM
  printf 'proof_root=%s cleanup_command=rm -rf -- %s\n' "${proof_root}" "$(shell_quote "${proof_root}")"
  printf 'candidate_checksum=verified synthetic_sources=3 auth_copy=isolated plugin=installed hook_trust=manual_required published_release=false retained_by_explicit_opt_in=true\n'
else
  cleanup
  trap - EXIT HUP INT TERM
  printf 'candidate_checksum=verified synthetic_sources=3 auth_copy=removed plugin=validated hook_trust=not_run published_release=false artifacts_retained=0\n'
fi
