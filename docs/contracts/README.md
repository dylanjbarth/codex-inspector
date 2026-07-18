# Phase 0 frozen contracts

These contracts implement the first milestone in
[`mvp-architecture.md`](../architecture/mvp-architecture.md): Phase 0,
Feasibility and frozen contracts.

The frozen demo contract is identified by:

- Inspector protocol: `1`
- index schema: `1`
- rollout adapter: `rollout-jsonl/codex-cli-0.144.1/v1`
- Review artifact contract: `inspector.review/v1`
- supported release target: macOS arm64

Contract sources:

- [support matrix](support-matrix.md)
- [fact model](fact-model.md) and [SQLite schema](schema.sql)
- [metrics](metrics.md)
- [internal API](internal-api.md) and
  [`internal-api.openapi.json`](../../schemas/internal-api.openapi.json)
- [Review artifacts and launch](reviews.md)
- [`hook-marker.schema.json`](../../schemas/hook-marker.schema.json)
- [performance budgets](performance-budgets.md)
- [verification record](phase-0-verification.md)

Tiny fake source records live in [`fixtures/synthetic`](../../fixtures/synthetic).
They are contract inputs, not a demo corpus. `go test ./...` validates the
source discriminator, normalized golden facts, metric goldens, JSON schemas,
and SQLite identity constraints without reading a developer's Codex data.
The fact golden records the complete metadata-only normalized corpus, including
source/segment identities, ownership, turns, ordered events, messages, tool
phases, compaction, capacity, coverage, evidence locators, fingerprints, and
adapter provenance. Regenerate it with
`scripts/phase0/generate-fact-golden.sh`; use `--check` in verification to
prove it is reproducible from the synthetic rollouts.
