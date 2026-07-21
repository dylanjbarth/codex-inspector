#!/bin/sh
set -eu

export GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/codex-inspector-phase5-gocache}"
scripts/phase0/generate-fact-golden.sh --check
scripts/phase0/generate-internal-api.sh --check
scripts/phase2/generate-schema.sh --check
unformatted="$(find cmd internal tools schemas/reviews -type f -name '*.go' -exec gofmt -l {} +)"
test -z "${unformatted}"
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./internal/reviews ./internal/evidence ./internal/inspector ./internal/server
pnpm --filter @codex-inspector/web typecheck
pnpm --filter @codex-inspector/web lint
pnpm --filter @codex-inspector/web test
scripts/phase1/build-web.sh
go build ./...
shellcheck plugin/codex-inspector/hooks/inspector-hook.sh scripts/phase0/*.sh scripts/phase1/*.sh scripts/phase2/*.sh scripts/phase3/*.sh scripts/phase4/*.sh scripts/phase5/*.sh
