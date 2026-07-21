#!/bin/sh
set -eu
codex_home=${CODEX_HOME:-"${HOME}/.codex"}
probe_home=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase4.XXXXXX")
trap 'rm -rf "$probe_home"' EXIT HUP INT TERM
GOCACHE=${GOCACHE:-/tmp/codex-inspector-phase4-gocache} go run ./tools/phase4/corpusprobe --codex-home "$codex_home" --inspector-home "$probe_home"
