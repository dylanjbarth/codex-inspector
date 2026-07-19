#!/bin/sh
set -eu

proof_home="$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase5-corpus.XXXXXX")"
trap 'rm -rf "${proof_home}"' EXIT HUP INT TERM
chmod 700 "${proof_home}"
CODEX_INSPECTOR_HOME="${proof_home}" \
  GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/codex-inspector-phase5-gocache}" \
  go run ./tools/phase5/corpusprobe
