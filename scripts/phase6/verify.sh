#!/bin/sh
set -eu

export GOCACHE="${GOCACHE:-${TMPDIR:-/tmp}/codex-inspector-phase6-gocache}"

scripts/phase6/browser-prerequisite.sh --check

scripts/phase0/generate-fact-golden.sh --check
scripts/phase0/generate-internal-api.sh --check
scripts/phase2/generate-schema.sh --check
unformatted=$(find cmd internal tools schemas/reviews -type f -name '*.go' -exec gofmt -l {} +)
test -z "${unformatted}"
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./internal/indexer ./internal/storage ./internal/metrics ./internal/evidence ./internal/inspector ./internal/reviews ./internal/process ./internal/server
go test -count=2 ./internal/server -run 'TestPhase6AutomatedDemoBoundarySmoke'
scripts/phase6/test-proof-script-cleanup.sh
pnpm --filter @codex-inspector/web typecheck
pnpm --filter @codex-inspector/web lint
pnpm --filter @codex-inspector/web test
scripts/phase1/build-web.sh
go build ./...
shellcheck plugin/codex-inspector/hooks/inspector-hook.sh scripts/phase0/*.sh scripts/phase1/*.sh scripts/phase2/*.sh scripts/phase3/*.sh scripts/phase4/*.sh scripts/phase5/*.sh scripts/phase6/*.sh
scripts/phase6/prove-browser-e2e.sh
scripts/phase6/prove-browser-e2e.sh
