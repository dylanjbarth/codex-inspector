#!/bin/sh
set -eu
out_dir="${1:-dist}"
mkdir -p "${out_dir}"
phase1_go_cache="${GOCACHE:-${TMPDIR:-/tmp}/codex-inspector-phase1-go-cache}"
GOCACHE="${phase1_go_cache}" GOTOOLCHAIN=local GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "${out_dir}/codex-inspector-darwin-arm64" ./cmd/codex-inspector
(cd "${out_dir}" && shasum -a 256 codex-inspector-darwin-arm64 > codex-inspector-darwin-arm64.sha256)
