# Internal API and revision contract

[`schemas/internal-api.openapi.json`](../../schemas/internal-api.openapi.json)
is the canonical OpenAPI 3.1 source. It freezes protocol `1` and index schema
version `2`. TypeScript output
is generated at `web/src/generated/internal-api.ts` by pinned
`openapi-typescript@7.10.1`; that file must never be edited by hand.

## Boundary rules

- bind only to `127.0.0.1` on an ephemeral port;
- all `/v1/*` routes except token exchange require the SameSite session cookie;
- token exchange accepts the per-process fragment token once, rotates it into
  an HttpOnly `SameSite=Strict` cookie, and invalidates the token;
- state-changing and streaming requests validate `Host` and `Origin`;
- paths from browser input are never accepted;
- evidence is addressed only by opaque `evidenceId` and byte/chunk bounds;
- every read response contains `datasetEpoch`, `appliedRevision`, and coverage
  when it represents indexed facts;
- every indexed snapshot carries `schemaVersion: 2`; a v1 database is a
  rebuild input, never an API snapshot;
- status identifies the one canonical effective Codex home and whether it was
  resolved from `CODEX_HOME` or the default; processes and dataset catalogs are
  bound to that identity and cannot be reused across homes;
- list endpoints use cursor pagination with maximum page size 200;
- errors use the shared `Problem` schema and never include raw source payloads.

## Revision behavior

An omitted requested revision resolves to the latest committed revision at
request start. A supplied unavailable or superseded revision returns
`409 revision_unavailable`. SSE `revision.available` means the client may
offer **New data available**; it does not mutate the page's applied revision.
The client suppresses events at or below its applied revision and clears the
action after applying the newest revision without changing route, filters, or
scroll position.
Revisions compare only within one epoch. `fullRefreshRequired` is true for an
epoch replacement, and the event carries schema version 2.

The metric catalog is also an indexed snapshot. Its optional
`requestedRevision` query parameter uses the same resolution rules, and the
response returns `datasetEpoch`, `appliedRevision`, coverage, the seven metric
definitions, and filter options selected at that revision. Project, model, and
reasoning choices are bounded; a complete catalog that exceeds the response
contract returns `413` instead of silently truncating usable choices.

Required SSE event names are `status.changed`, `revision.available`,
`sync.progress`, `review.changed`, and `heartbeat`. Each event has an ID and
protocol version and is resumable with `Last-Event-ID` within the process's
bounded event buffer.

## Render contracts

Status is not a generic section map. It returns concrete process, Codex CLI,
plugin/protocol compatibility, index/schema/database/count/watermark, and
seven-hook marker/diagnostic fields. Metric query results are a discriminated
union covering all seven frozen formulas, including fidelity, coverage,
exclusion reasons, contribution and root identities, time buckets, token
composition/residual, and capacity window/reset/staleness state.

Metric filter options are part of the generated `MetricCatalog` response, not
an extension field on one metric definition. The generated response closes
unknown properties and bounds project, model, reasoning, and contribution-kind
arrays.

Index status also exposes queued session changes and inventoried, processed,
remaining, skipped, unsupported, failed, and requires-rebuild counts. An
incomplete active tail is coverage state, not an active index job. Every metric item carries both
indexed time coverage and the requested/effective time boundary. Capacity
points always expose `remainingPercent`, using `null` when the source did not
record it rather than deriving a value from `usedPercent`.

Session maps include ordered root turns plus explicit child-to-spawn-turn
topology. Root-turn responses expose only completed or terminal turns, never
active/provisional state. Review plan previews use the complete frozen manifest
contract and include source byte counts, indexed time coverage, and a concrete
project/session/turn summary.
Review detail returns the concrete manifest and run state together with the
accepted report, findings, and per-citation evidence availability needed to
render it; artifact hashes alone are not a render contract.

## Pinned facts and live evidence

Status and checkpoints are current operational state. Metric, session, label,
coverage, event, and locator data are pinned to `datasetEpoch` plus
`appliedRevision`. Evidence responses expose the pinned locator,
`sourcePrefixSha256` for exact bytes `[0,byteEnd)`, and `eventFingerprint` for
the exact record bytes. They separately expose current `availability`,
`availabilityObservedAt`, and nullable `availabilityRevision`. A read-time
revalidation may have no index revision; it must not rewrite pinned coverage or
masquerade as the state at the applied revision.

## Generation

Run `scripts/phase0/generate-internal-api.sh` to regenerate and
`scripts/phase0/generate-internal-api.sh --check` to fail on drift. Go handlers
may use generated or hand-authored domain types, but contract tests validate
every response against this document.
