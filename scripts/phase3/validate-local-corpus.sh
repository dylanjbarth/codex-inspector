#!/bin/sh
set -eu
codex_home=${CODEX_HOME:-"${HOME}/.codex"}
probe_home=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase3.XXXXXX")
trap 'rm -rf "$probe_home"' EXIT HUP INT TERM
GOCACHE=${GOCACHE:-/tmp/codex-inspector-phase3-gocache} go run ./tools/phase3/metricprobe --codex-home "$codex_home" --inspector-home "$probe_home"
