# Phase 3 verification

Phase 3 implements the revision-pinned Token & Capacity metric engine, frozen
loopback query/SSE contract, and the static dashboard vertical slice.

Run the deterministic suite from the repository root:

```sh
scripts/phase3/verify.sh
```

The suite proves the seven formula-v1 metric responses against the synthetic
root/descendant golden, mutually exclusive token composition, contribution
filtering, filter-independent capacity, Chicago DST calendar boundaries,
OpenAPI response validity, bounded cache behavior, server security, and UI
render/update behavior. UI tests exercise every shared filter, partial and
terminal states, stable `/context/<root-session-id>` links, and explicit
revision application without state loss.

Run the payload-free real-corpus proof with:

```sh
scripts/phase3/validate-local-corpus.sh
```

It reads `CODEX_HOME`, indexes into a disposable Inspector home, and emits only
aggregate inventory, coverage, token, bucket, root-count, and capacity-presence
values. It never prints source paths, session IDs, messages, tool payloads, or
access data and deletes the disposable derived index on exit.

For the manual browser proof, build embedded assets, run `codex-inspector open`
against the disposable or normal local Inspector home, and verify: all seven
widgets render; changing each filter updates the whole view; an indexing commit
offers **New data available** without navigation; applying it preserves filter
values; a contributing root link opens the honest Phase 4 placeholder at its
stable Context route; partial, stale-capacity, unsupported, empty,
incompatible, and error states are visibly distinct.

## 2026-07-18 candidate result

- `scripts/phase3/verify.sh`: exit 0. Frozen generators and schema were current;
  all Go tests, metric/server race tests, vet, build, TypeScript checking,
  ESLint, nine dashboard UI tests, Vite production build, and shellcheck
  passed.
- `scripts/phase3/validate-local-corpus.sh`: exit 0. The disposable scan
  inventoried and processed 260 sources with zero failures or rebuilds. Metric
  coverage was exact for 1,006 of 1,006 eligible completed turns, with 88,565,039
  recorded tokens, 17 contributing roots, and a latest recorded capacity
  observation. Only these aggregates were emitted.

Independent review findings were resolved as follows:

- revision commits now publish availability and progress events; the dashboard
  refreshes status, requires explicit apply, preserves filters, and reloads on
  a full-refresh epoch change;
- composition and capacity coverage now report incomplete source observations
  as partial or unavailable rather than silently complete;
- queries enforce calendar-bucket, capacity-series, capacity-point, and encoded
  response limits and return `413` when the result would exceed them;
- capacity freshness is recalculated against wall time instead of a cached
  revision, and the displayed reset countdown advances every second;
- project, model, reasoning, and contribution filters are populated from real
  indexed choices and exact-filter tests prove their effects; and
- browser bootstrap now finishes fragment exchange and authenticated status
  before requesting the catalog. A real loopback cookie-jar test proves the
  one-time exchange stores a SameSite cookie that authenticates the following
  catalog request. An explicit UI regression test holds exchange pending and
  proves no authenticated dashboard request races ahead of it;
- production compatibility is obtained only from normal inspection or injected
  test configuration; there is no environment-variable compatibility bypass;
- the generated metric catalog contract now carries bounded filter choices at
  its explicit dataset epoch and applied revision. Applying new data refetches
  both metrics and choices at that revision, including empty-to-populated
  transitions; and
- eligible turns with zero observed recorded usage produce a null,
  `unavailable` total and visible reason. Known partial sums remain numeric and
  visibly partial.

The worker's first normal-browser automation attempt was inconclusive because
the CLI reported the pre-cleanup fragment before the settled page could be
observed safely. Its browser, server, and artifacts were removed without a
workaround. Independent review subsequently performed the credential-safe
normal-browser proof and closed that evidence gap. The four later corrections
above affect injected test compatibility, catalog contracts/refetch, and
missing-usage presentation; they do not change the reviewed browser bootstrap
security flow, so the worker did not rerun it.
