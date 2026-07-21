#!/bin/sh
set -eu

if [ "${CODEX_INSPECTOR_RUN_LIVE_PROOFS:-}" != "1" ]; then
  printf 'Set CODEX_INSPECTOR_RUN_LIVE_PROOFS=1 to launch capacity-consuming disposable Codex tasks.\n' >&2
  exit 2
fi

source_codex_home="${CODEX_HOME:-${HOME}/.codex}"
if [ ! -f "${source_codex_home}/auth.json" ]; then
  printf 'A logged-in Codex home is required for the isolated live proof.\n' >&2
  exit 2
fi

proof_root="$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase5-proof.XXXXXX")"
trap 'rm -rf "${proof_root}"' EXIT HUP INT TERM
codex_home="${proof_root}/codex"
inspector_home="${proof_root}/inspector"
mkdir -p "${codex_home}/sessions" "${inspector_home}"
chmod 700 "${proof_root}" "${codex_home}" "${codex_home}/sessions" "${inspector_home}"
cp "${source_codex_home}/auth.json" "${codex_home}/auth.json"
chmod 600 "${codex_home}/auth.json"
cp fixtures/synthetic/root.jsonl fixtures/synthetic/descendant.jsonl "${codex_home}/sessions/"

CODEX_HOME="${codex_home}" codex plugin marketplace add "$(pwd)" >/dev/null
CODEX_HOME="${codex_home}" codex plugin add codex-inspector@codex-inspector-development --json >/dev/null

CODEX_HOME="${codex_home}" CODEX_INSPECTOR_HOME="${inspector_home}" \
  GOCACHE="${TMPDIR:-/tmp}/codex-inspector-phase5-gocache" \
  go run ./tools/phase5/reviewproof --codex-home "${codex_home}" --inspector-home "${inspector_home}"
