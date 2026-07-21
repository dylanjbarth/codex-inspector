#!/bin/sh
set -eu

export GOCACHE="${GOCACHE:-/tmp/codex-inspector-phase2-gocache}"

scripts/phase0/generate-fact-golden.sh --check
scripts/phase0/generate-internal-api.sh --check
scripts/phase2/generate-schema.sh --check
unformatted=$(find cmd internal tools -type f -name '*.go' -exec gofmt -l {} +)
test -z "$unformatted"
go vet ./...
go test ./...
go test -race ./internal/sources ./internal/storage ./internal/indexer ./internal/evidence
go build ./...
pnpm --filter @codex-inspector/web typecheck
pnpm --filter @codex-inspector/web lint
pnpm --filter @codex-inspector/web test
bundle_snapshot=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase2-assets.XXXXXX")
trap 'rm -rf "$bundle_snapshot"' EXIT HUP INT TERM
cp -R internal/server/assets/. "$bundle_snapshot/"
scripts/phase1/build-web.sh
diff -qr "$bundle_snapshot" internal/server/assets
rm -rf "$bundle_snapshot"
trap - EXIT HUP INT TERM
shellcheck plugin/codex-inspector/hooks/inspector-hook.sh scripts/phase0/*.sh scripts/phase1/*.sh scripts/phase2/*.sh
