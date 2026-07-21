# Phase 2 verification

Phase 2 implements current-format discovery, normalization, immutable fact
storage, revision-pinned discovery, and opaque source evidence. The frozen
architecture and Phase 0 contracts remain authoritative.

Run the deterministic verification from the repository root:

```sh
scripts/phase2/verify.sh
```

The focused tests prove:

- production/frozen-Phase-0 discriminator parity for supported, unsupported,
  descendant, and truncated synthetic sources, plus normalization totals;
- cumulative usage, mirrored tool phases, missing-field coverage, project
  precedence, and root/descendant ownership;
- repeated scans, append-only changes, checkpoints, truncated tails, queue
  coalescing, post-start filesystem watcher activation, reverse partial
  availability, uncapped completion, single-writer exclusion, and automatic
  changed-prefix replacement rebuild;
- terminal-turn/source-boundary transaction batching, including a 500+ event
  bounded turn, canceled-transaction invisibility, pinned-revision visibility,
  and checkpoint alignment;
- monotonic revisions, pinned queries, canceled-transaction recovery, failed
  replacement isolation, and atomic dataset replacement;
- fork, continuation, resume, orphan, and Inspector Review ownership behavior;
- opaque bounded evidence, full-record fingerprint verification, archive
  rediscovery, unavailable sources, and unsafe-byte escaping.
- schema-v2 health, status, and session-page OpenAPI compatibility plus strict
  rejection of unknown or malformed query parameters.
- schema-v1 CLI rebuild success/failure through an immutable database filename,
  validated building epoch, and fsynced `active-index` pointer swap;
- exact production adapter equality with the Phase 0 expected-facts golden,
  including 2 sessions, 23 source events, 23 evidence rows, and 3 usage
  coverage observations;
- reverse-ingested resumed-session cumulative baselines and session bounds,
  hook-proven aborted/interrupted/reconciled-truncated outcomes, and exact
  spawning-turn lineage;
- subprocess WAL recovery before commit and immediately after durable commit,
  including checkpoint atomicity, remaining-work resume, and idempotent restart;
- deterministic live evidence revalidation timestamps with nullable live
  availability revisions while pinned locator facts remain unchanged.
- activation rejection for same-cardinality FTS token corruption, FTS
  provenance corruption, latest-version projection corruption, and immutable
  identity corruption, with the prior catalog target remaining readable;
- exact frozen segment, turn, and event order keys, including equal timestamp
  fingerprint ties, while resumed-session bounds aggregate independently from
  each segment's original `session_meta` timestamp.

For the developer corpus, first build a binary and run the payload-free probe:

```sh
GOCACHE=/tmp/codex-inspector-phase2-gocache go build -o /tmp/codex-inspector-phase2 ./cmd/codex-inspector
scripts/phase2/validate-local-corpus.sh /tmp/codex-inspector-phase2
```

The probe uses a disposable `CODEX_INSPECTOR_HOME`, never mutates `CODEX_HOME`,
and emits only aggregate counts, byte totals, source states, checkpoints,
resource measurements, and timestamps. It never copies source payloads into
the repository, snapshots, logs, or diagnostics.

The validated developer-corpus run on 2026-07-18 inventoried 256 sources
(356,329,647 bytes) with zero failures or rebuilds in 46.55 seconds and a peak
resident set of 336,674,816 bytes, then repeated with 254 sources skipped and
two metadata sources refreshed. The aggregate projection reported 40 current
and 216 explicitly unsupported sources, 941 completed turns, and no pending
tails.
