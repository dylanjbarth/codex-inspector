#!/bin/sh
set -eu
export GOCACHE="${GOCACHE:-/tmp/codex-inspector-phase3-gocache}"
scripts/phase0/generate-fact-golden.sh --check
scripts/phase0/generate-internal-api.sh --check
scripts/phase2/generate-schema.sh --check
unformatted=$(find cmd internal tools -type f -name '*.go' -exec gofmt -l {} +)
test -z "$unformatted"
go vet ./...
go test ./...
go test -race ./internal/metrics ./internal/server
go build ./...
pnpm --filter @codex-inspector/web typecheck
pnpm --filter @codex-inspector/web lint
pnpm --filter @codex-inspector/web test
scripts/phase1/build-web.sh
shellcheck plugin/codex-inspector/hooks/inspector-hook.sh scripts/phase0/*.sh scripts/phase1/*.sh scripts/phase2/*.sh scripts/phase3/*.sh
