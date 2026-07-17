# Codex Inspector MVP architecture

- **Status:** Aligned and ready for phased implementation
- **Date:** July 17, 2026
- **Scope:** Plugin foundation, metric collection, dashboard, session inspection, and Effectiveness Reviews
- **Authority:** Canonical implementation architecture for the MVP

## 1. Product boundary

Codex Inspector is a plugin-native, local-first observability product for Codex power users. The browser dashboard is a supporting surface owned and launched by the plugin.

The user discovers Inspector in the Codex plugin marketplace. The plugin supplies the hooks, skills, setup guidance, review instructions, and compatibility policy. A separately installed, open-source CLI supplies the local runtime.

The product has three connected surfaces:

1. **Dashboard** — cross-session metrics through two initial static views.
2. **Context Inspector** — source-backed session discovery, causal topology, event evidence, and locally reconstructed context.
3. **Reviews** — explicitly started, dedicated Codex review tasks with immutable structured reports.

The browser application is not an independent hosted product. There is no account, cloud service, required MCP server, or remote database.

## 2. Architectural principles

1. **The plugin is the product.** Skills and hooks create the Codex-native experience; the CLI and dashboard implement supporting capabilities.
2. **Facts precede metrics.** The index stores normalized facts. Metrics are versioned definitions over those facts. Widgets only render metric results.
3. **Source logs remain authoritative.** Inspector never edits or copies complete rollout logs.
4. **Derived state is rebuildable.** SQLite facts and metric caches can be deleted and reconstructed from readable source logs.
5. **Completed turns are the freshness boundary.** The UI does not promise event-level live indexing.
6. **Indexing is demand-driven.** No permanent daemon or scheduled background process is required.
7. **Partial data remains useful.** Reverse-chronological indexing exposes recent results early and labels incomplete coverage.
8. **Evidence is source-backed.** Metrics and reviews resolve to normalized facts and pointers into the original logs.
9. **Local evidence is transparent.** Inspector displays exact readable payloads from local logs without adding a secret-masking layer.
10. **Reviews are explicit Codex tasks.** Inspector provides scope and structure; Codex performs the qualitative analysis with its normal tools.
11. **No opaque score.** Resource use, friction, context, delegation, and review findings remain separate concepts.
12. **Every phase ends in a working vertical slice.**

## 3. Technology and deployment

### 3.1 Runtime

The local runtime is implemented in Go. One compiled binary owns:

- the public CLI;
- source discovery and indexing;
- SQLite migrations and access;
- normalized fact generation;
- metric queries and caching;
- the short-lived local HTTP server;
- review planning and artifact discovery;
- process status and diagnostics.

Go is selected for cross-platform compilation, predictable deployment, streaming I/O, and straightforward bounded concurrency. The first release publishes macOS and Linux binaries.

### 3.2 Dashboard

The dashboard uses React, TypeScript, Vite, and pnpm. Production assets are embedded in the Go binary and served from the local Inspector process.

A browser cannot safely open arbitrary local rollout files or the Inspector SQLite database. The Go process therefore exposes an internal loopback JSON API. This is an in-process boundary, not a separately deployed service.

### 3.3 Local process lifecycle

`codex-inspector open`:

1. reuses an existing healthy Inspector process when one exists;
2. otherwise starts a detached short-lived server and returns;
3. records its PID, port, protocol version, and access token;
4. remains alive while indexing is active or a dashboard heartbeat is present;
5. exits after an idle interval with no clients and no queued work;
6. recovers from stale process metadata on the next invocation.

The server binds to loopback only and serves the UI and API from the same origin. It uses a per-process access token and rejects unexpected hosts and origins.

The dashboard contains a visible process/indexing status indicator and debugging tray. It reports:

- process and CLI version;
- plugin compatibility;
- idle, scanning, catching-up, or failed index state;
- queued session changes;
- processed, skipped, and failed sources;
- current reverse-scan date;
- last completed-turn watermark;
- database size and index schema version;
- hook health and last observed hook marker;
- copyable doctor diagnostics.

### 3.4 Application data

Inspector-owned state lives under:

```text
CODEX_INSPECTOR_HOME
  default: ~/.codex-inspector
```

Initial layout:

```text
~/.codex-inspector/
  inspector.db
  reviews/
  queue/
  run/
  logs/
  cache/
```

`CODEX_HOME` is the inspected source. `CODEX_INSPECTOR_HOME` is derived Inspector state. The indexer must not discover its own data directory as source material.

Disabling or uninstalling the plugin leaves Inspector data intact. Users may remove derived data or an individual review subtree explicitly.

## 4. Plugin and CLI contract

### 4.1 Plugin ownership

The plugin owns:

- the plugin manifest and marketplace presentation;
- lifecycle hook definitions;
- setup, open-dashboard, inspect-session, and review-session skills;
- the fixed Effectiveness Review rubric and instructions;
- the required CLI compatibility range;
- user-facing installation and upgrade guidance.

The CLI owns:

- source parsing and indexing;
- SQLite and schema migrations;
- metric calculation;
- local server and embedded dashboard;
- process coordination;
- review manifests, run metadata, and report discovery.

The CLI installer does not install another copy of the plugin hooks.

### 4.2 Installation

Marketplace installation alone is not assumed to install a binary or modify `PATH`.

When the CLI is unavailable, the plugin and dashboard setup state provide a prompt that asks Codex to:

1. detect the user's platform and architecture;
2. download a plugin-compatible binary from GitHub Releases;
3. verify the published checksum;
4. install it into a user-writable directory on `PATH`;
5. run `codex-inspector doctor`;
6. open Inspector and begin the initial sync.

GitHub Releases are the source of truth for initial macOS and Linux binaries. Source builds are a developer workflow, not the default user installation.

### 4.3 Version compatibility

Plugin and CLI releases are coordinated but independently versioned.

Compatibility uses:

- plugin semantic version;
- CLI semantic version;
- an explicit plugin/CLI protocol version;
- an index schema version;
- a plugin-declared supported CLI range.

Plugin-only skill or prompt improvements do not require a CLI upgrade when the protocol remains compatible. An incompatible CLI does not write the database through hooks; Inspector surfaces a rate-limited setup warning and upgrade prompt.

### 4.4 Public CLI surface

The MVP public CLI remains intentionally small:

```text
codex-inspector version
codex-inspector doctor [--json]
codex-inspector status [--json]
codex-inspector sync [--background|--wait]
codex-inspector open [route and scope flags]
```

Hooks and process orchestration use explicitly internal commands such as `_hook`, `_serve`, and `_worker`. They are not public compatibility contracts.

Terminal metric exploration, session tables, review execution, exports, and a public HTTP API are deferred.

### 4.5 Skills and routes

Initial plugin workflows include:

- opening the main Dashboard;
- opening Context Inspector for the current session;
- opening a prefilled single-session Review plan.

Skills are thin orchestration layers over `codex-inspector open`.

If the current Codex session ID cannot be resolved reliably, Inspector never guesses from cwd or timestamps. It opens session discovery with an “Unable to locate the current session ID” notice and any safe project/date hints that are available.

## 5. Hook and indexing model

### 5.1 Hooks signal work

Plugin-owned hooks never parse complete rollouts, calculate metrics, or write SQLite. They append compact durable markers that identify possible source changes.

Relevant lifecycle signals include:

- user prompt submitted;
- turn stopped;
- subagent started;
- subagent stopped;
- context compacted;
- session started or resumed when useful for reconciliation.

Markers contain only the identifiers, source locator when available, event kind, and observation time needed to schedule work.

If the CLI is absent, incompatible, disabled, or a marker is lost, correctness is unaffected. The next source scan recovers the missing work.

### 5.2 Demand-driven worker

There is no always-on indexer.

Indexing begins when:

- Inspector is opened;
- a user explicitly runs `sync`;
- a plugin skill invokes an Inspector workflow;
- a running Inspector process observes queued hook markers.

The worker exits when its finite queue and active scan are complete. An initial scan may continue as a finite detached job after a browser closes, but it is not a resident daemon.

### 5.3 Initial and incremental scans

The first sync scans in reverse chronological order so recent metrics become useful quickly.

Source inventory and checkpoints allow the indexer to:

- skip unchanged files using identity, size, timestamps, and fingerprints;
- resume append-only JSONL at the last complete record boundary;
- retry a truncated final record later;
- prioritize hook-marked sessions;
- coalesce duplicate markers;
- avoid concurrent writers through a process/database lock;
- yield and bound I/O concurrency to avoid CPU or disk thrashing.

Every normalization write is idempotent. Re-reading the same record with the same adapter version produces the same fact identity and does not change metric totals.

### 5.4 Turn consistency

A turn is provisional until a completion or terminal record makes it eligible for committed metrics.

- Dashboard metrics include completed turns.
- Context Inspector shows data through the last indexed completed turn.
- The active turn is labeled pending rather than presented as complete.
- Aborted, interrupted, or truncated turns are classified explicitly during reconciliation.

Completed-turn consistency is the user-facing promise. Event-level live updates are out of scope.

### 5.5 Source coverage

The initial release indexes:

- active rollout JSONL logs;
- archived rollout JSONL logs;
- `session_index.jsonl` for titles and discovery metadata;
- capacity and rate-limit observations recorded in rollouts.

The first release does not index `history.jsonl`, memory databases, automation/application databases, or other Codex state.

## 6. Normalized fact store

### 6.1 Durable layers

```text
Codex rollout sources
  → compact normalized facts and source locators
  → versioned metric definitions
  → bounded in-memory result cache
  → static dashboard widgets
```

SQLite stores normalized facts, not complete raw session payloads and not widget-owned totals.

The fact schema must cover every metric required by the two initial Dashboard views and every source field required by the initial Session Inspector.

Adding a future metric that needs facts omitted by the installed index schema may require a resumable re-index. This is acceptable and occurs as part of a release/index-schema upgrade.

### 6.2 Fact families

The initial fact model includes:

- discovered source artifacts and parser/checkpoint state;
- projects and observed cwd/worktree aliases;
- logical sessions and source segments;
- session lineage edges;
- completed, aborted, and provisional turns;
- ordered normalized event envelopes;
- user and assistant message metadata and source locators;
- a deterministic content-search index that retains searchable terms and source locators without copying complete payloads;
- model invocations and configuration;
- recorded token snapshots and normalized usage;
- tool invocations and results;
- tool family, timing, success, and exit state;
- compaction and context-lifecycle changes;
- recorded capacity/rate-limit snapshots;
- evidence references;
- parser and field-coverage observations.

Raw messages, tool arguments, tool results, instruction text, and recorded reasoning summaries are read from source logs when requested. SQLite retains the facts, hashes, lengths, classifications, ordering, and locators needed to find them.

### 6.3 Source locators and missing evidence

Every source-backed fact retains enough provenance to attempt resolution:

- source artifact identity and path;
- source-provided session, turn, event, and call IDs;
- record ordinal and byte range where practical;
- content fingerprint;
- adapter/parser version.

If a source log is moved, archived, or deleted:

- previously calculated metrics remain valid derived facts;
- Inspector attempts deterministic rediscovery by identity/fingerprint;
- exact payload evidence may become unavailable;
- the UI explains the missing source without deleting the metric or report reference.

### 6.4 Session identity and lineage

Navigation lineage and metric ownership are related but separate.

- A user-initiated root session owns one work-unit tree.
- Spawned agents belong to the parent root's work unit and attach to the spawning turn when known.
- Descendant metrics are rolled up while remaining separately attributable.
- A resume with the same session identity continues the logical session even across multiple source files or long gaps.
- An explicit continuation with a new identity may remain another segment in the same work unit when the source proves that relationship.
- A user-created fork starts a new root work unit and retains a `forked_from` lineage edge.
- Inherited fork context is provenance, not newly consumed historical tokens.
- A session with an unresolved parent becomes an orphan root with reduced lineage coverage; Inspector never guesses.
- Worktree identity is a project/environment dimension, not a lineage relationship.

### 6.5 Accounting invariants

1. A source file is not necessarily a logical session.
2. A spawned agent is not another user-initiated root.
3. Additive root metrics preserve combined, root-direct, and descendant contributions.
4. Cumulative token snapshots are never summed as independent usage.
5. Mirrored tool/event forms are deduplicated by call identity and phase.
6. Tool payload size is not added to recorded model token usage.
7. Wall-clock session span is not active time.
8. Missing source fields are never rendered as zero.
9. Capacity utilization remains a recorded point-in-time observation, not a judgment.

## 7. Metric engine

### 7.1 Metric definitions

Metrics are versioned Go definitions over normalized facts. Each definition declares:

- stable key and display name;
- unit;
- formula and formula version;
- supported aggregations;
- supported time grains;
- filters and grouping dimensions;
- root/descendant rollup behavior;
- coverage requirements;
- fidelity;
- contributing-evidence query.

The shared calculation grammar remains:

```text
measure + aggregation + time grain + group by + filters + normalization
```

Widgets reference metric keys and valid dimensions. They never embed SQL or metric formulas.

### 7.2 Time series

Facts retain their natural timestamps. The metric engine supports hour, day, week, and month bucketing where the metric definition allows it.

Time-series responses include:

- ordered buckets;
- timezone;
- grouped series;
- root/direct/descendant attribution where applicable;
- indexed coverage boundary;
- formula version;
- fidelity and source coverage.

### 7.3 Metric result caching

The MVP calculates metrics from the compact fact store and maintains a bounded in-memory result cache keyed by:

- metric definition and formula version;
- filters and groupings;
- time range and grain;
- index revision.

The cache is disposable and does not survive process restart. Persistent materialized rollups are added only after real query profiling proves they are necessary.

### 7.4 Coverage and fidelity

Each result carries:

- eligible sessions or turns;
- observations with usable data;
- coverage ratio;
- indexed time boundary;
- exact, derived, estimated, or unavailable fidelity;
- reasons for excluded records.

Complete coverage remains visually quiet. Partial coverage produces a compact warning and a link to diagnostic details. Missing values use an explicit fallback, never zero.

## 8. Initial Dashboard

### 8.1 Static views

The initial dashboard ships two plugin-defined static views. Users may switch views and apply global filters. User-created views, drag-and-drop, resizing, and widget configuration are deferred.

View and widget definitions are declarative from the start so future editing creates user-owned configurations without changing metric contracts.

Global controls include:

- relative time range;
- project;
- model;
- reasoning level;
- root/descendant session kind.

### 8.2 Token & Capacity

User question:

> Where did my recorded Codex usage go, and how does it relate to my recorded plan limits?

The agreed view elements are:

1. current recorded limit utilization and reset countdown;
2. total recorded tokens with root and descendant contributions;
3. token usage over time stacked by root versus descendant work;
4. token composition across input, cached input, output, and reasoning tokens;
5. most token-intensive user-initiated root sessions;
6. capacity drawdown across recorded limit windows;
7. direct drill-down to contributing sessions and Context Inspector.

### 8.3 Efficiency & Friction

User question:

> Where is execution friction accumulating, and which sessions, tools, and working patterns explain it?

The view covers the agreed metric areas:

- active time;
- time to first token;
- failures;
- tool latency;
- retries;
- overlapping-interface switching;
- compactions;
- aborted turns;
- sessions with notable contributing measures.

Measures remain separate rather than becoming an efficiency or friction score. Every session or pattern can resolve to source-backed evidence.

### 8.4 Incremental UI updates

The application never hard-refreshes the page in response to indexing.

Each committed indexing transaction receives a monotonically increasing index revision. Newly inserted facts record the revision at which they became visible. A browser session pins one applied revision for its data queries.

The local server emits index-status and new-revision notifications through server-sent events. Incoming commits do not automatically advance the browser's applied revision or replace displayed data.

- The status tray updates as work progresses.
- Existing widgets, tables, filters, scroll, and selections remain unchanged.
- A global **New data available** control appears when a newer index revision exists.
- Filter changes continue to query the currently applied revision.
- Activating New data available advances the applied revision and refetches visible data without page navigation or application-state reset.

During the initial reverse scan, the Dashboard is usable as soon as facts exist. Widgets clearly disclose the current indexed time boundary.

## 9. Context Inspector

### 9.1 Discovery and entry

Context Inspector opens to discovery unless a route identifies a root session.

Discovery searches indexed titles, projects, working directories, IDs, readable messages, and readable tool results. Results resolve to canonical root sessions. A match found in a descendant remains nested beneath its root and explains why it matched.

Dashboard rows and exact review citations open stable Inspector routes. A missing current-session ID falls back to discovery rather than a guessed session.

### 9.2 Causal session map

The primary session view is a user-initiated root plus its descendant tree.

- Root turns preserve recorded order.
- Spawned sessions attach to their spawning turn when known.
- Nodes show direct recorded token usage and inclusive descendant usage.
- Fork lineage is visible without merging fork metrics into the source root.
- Unknown parent/spawn relationships remain explicit gaps.
- The map uses a stable absolute token scale.

Selecting a turn enters focused inspection while preserving the full topology and return path.

### 9.3 Exact event ledger

A selected full agent turn shows its chronological recorded events, including:

- user and assistant messages;
- recorded reasoning summaries;
- model invocations;
- tool calls and results;
- patches and searches;
- subagent lifecycle;
- token usage;
- compactions;
- task lifecycle.

Inspector displays readable source payloads exactly as found. It does not mutate the source or add a masking layer. Content already truncated, redacted, encrypted, or unavailable upstream is labeled accordingly.

### 9.4 Locally reconstructed context

Inspector maintains a versioned deterministic context reducer over recorded events.

It tracks:

```text
add, remove, replace, compact, truncate, unknown gap
```

Context blocks retain:

- model order;
- role and category;
- source evidence locator;
- recorded size;
- token contribution;
- lifetime boundaries;
- fidelity.

The phrase **Context seen by Codex** is reserved for an exact recorded model-input boundary. All other snapshots are labeled **Locally reconstructed context state**.

Inspector cannot reconstruct absent server-side information. Missing instructions, encrypted reasoning, unexplained token differences, and unsupported source behavior remain explicit unavailable gaps.

### 9.5 Token attribution

The first Session Inspector includes component-level token contribution:

- recorded overall usage is exact where present;
- individual messages, instructions, tool definitions, and tool outputs may use local token estimates;
- estimates name the tokenizer/model rule and fidelity;
- unsupported estimates are unavailable;
- tool payload introduced and cumulative context burden are separate values;
- cumulative burden is estimated unless the source proves it.

Tokenizer selection and source-version rules are an implementation spike, not a deferred product feature.

### 9.6 Compaction

Compaction is a first-class event and context checkpoint. Where sources permit, Inspector shows:

- before and after context;
- absolute and percentage reduction;
- category changes;
- preserved content;
- recorded compacted text;
- removed or replaced content;
- reconstruction gaps.

## 10. Effectiveness Reviews

### 10.1 Review entry and task isolation

A review begins from:

- **New review** in Reviews;
- **Review effectiveness** for a root session on the Dashboard;
- **Review effectiveness** for the selected root in Context Inspector.

Every entry opens the same prefilled Review plan.

Starting a review always creates a new Inspector-classified Codex task. A review never executes inside the session being reviewed. Review-created sessions are excluded from ordinary review scopes by default.

### 10.2 Scope

The MVP supports:

- one user-initiated root session and its descendants; or
- a bounded time period of eligible roots, optionally filtered to one project.

The aggregate default is seven days. The plan displays included roots, turns, projects, time coverage, model, reasoning level, estimated input, output location, and the prompt before Codex starts.

### 10.3 Fixed rubric

Every review uses one rubric:

1. task framing and steering;
2. execution efficiency;
3. delegation and workflow;
4. reusable leverage.

The user may add a plain-language focus. There are no review profiles, developer grades, usefulness ratings, scheduled reviews, or universal efficiency scores.

### 10.4 Scope manifest and Codex tools

Inspector writes a scope manifest containing:

- review and schema identity;
- target session or time/project boundaries;
- included session and turn IDs;
- source log locators;
- aggregate metric facts;
- known coverage gaps;
- evidence-reference rules;
- fixed rubric and optional focus;
- report destination.

The manifest is a clear instruction and provenance boundary, not a filesystem security sandbox. The dedicated Codex task may use its normal filesystem and shell tools. Inspector asks Codex to inspect only the referenced records and validates returned citations afterward.

The review task is not required to use the Inspector CLI as a restricted read interface.

### 10.5 Filesystem lifecycle

Each review run uses:

```text
~/.codex-inspector/reviews/<review-id>/
  manifest.json
  run.json
  review.json
```

- `manifest.json` freezes scope and requested configuration.
- `run.json` stores originating Codex task identity and lifecycle metadata.
- `review.json` is the authoritative completed report.

No `review.json` means no completed report, though an in-progress or failed run may still exist.

The first schema-valid `review.json` is immutable. Continuing the Codex conversation does not rewrite it. A revision is a new review linked to the original.

Users may delete a review by deleting its subtree. Inspector tolerates missing and partially present review directories during discovery.

### 10.6 Report contract

A completed report contains:

- review identity, scope, model, reasoning, and completion time;
- a concise scope summary;
- no more than five prioritized findings;
- strengths and improvement opportunities without a forced quota;
- one primary rubric lens per finding;
- observation and impact;
- directly observed, strongly supported, or worth investigating evidence support;
- source-backed citations;
- recommendation;
- optional Codex action prompt.

Inspector validates JSON structure, schema version, review identity, required fields, and citation resolvability. It does not act as a second semantic judge.

Structurally valid findings remain visible when a source citation later becomes unavailable. Invalid or partial artifacts preserve the run and raw file for diagnosis.

Inspector never applies a recommended change automatically.

## 11. Internal API and revision model

The loopback API is an internal same-version interface between the embedded dashboard and Go runtime.

Initial responsibilities include:

- process/index status and diagnostics;
- index sync control;
- metric view definitions and queries;
- session discovery;
- causal map and turn ledger read models;
- evidence and reconstructed-context resolution;
- review planning, launch coordination, history, and reports;
- server-sent status and new-revision notifications.

The browser does not join normalized tables, calculate metrics, reconstruct context, read arbitrary paths, or write review artifacts.

API responses use typed generated TypeScript contracts where practical. The API may change with coordinated CLI/dashboard releases and is not a public integration surface in the MVP.

## 12. Repository shape

```text
cmd/
  codex-inspector/            Go entrypoint
internal/
  app/                        runtime orchestration and dependency wiring
  cli/                        public and internal commands
  process/                    server lifecycle, locks, heartbeat, revisions
  sources/                    discovery and adapter registry
  indexer/                    reverse scan, queue, checkpoints, normalization
  facts/                      domain facts and identity rules
  storage/                    SQLite schema, migrations, and repositories
  metrics/                    definitions, queries, coverage, cache
  evidence/                   source locators and raw resolution
  context/                    deterministic context reducer and token estimates
  reviews/                    plans, manifests, runs, schemas, artifacts
  server/                     loopback HTTP and SSE
web/
  src/                        React application
  package.json
  vite.config.ts
plugin/
  codex-inspector/
    .codex-plugin/plugin.json
    hooks/hooks.json
    skills/
fixtures/
  rollouts/
  reviews/
docs/
```

The Go module and pnpm workspace build together. The production web bundle is embedded into the Go binary.

## 13. Testing strategy

### 13.1 Real structure-preserving fixtures

Metric and indexing tests use fixtures derived from real Codex rollout logs and sanitized before commit.

Sanitization must preserve the behavior under test:

- event ordering and record shapes;
- session, turn, call, and lineage relationships;
- cumulative token snapshots;
- timestamps and duration relationships;
- tool success/failure semantics;
- compaction behavior;
- source-version distinctions.

Recorded numeric fields remain unchanged when they are the subject of an assertion. Content used for tokenizer estimates receives explicit fixture expectations because redaction can change tokenization.

Unredacted personal corpora may be used locally for manual validation but are never committed. Repository fixtures receive secret scanning and a privacy review.

### 13.2 Test layers

- **Adapter golden tests:** real-shaped source fixture to expected normalized facts.
- **Metric golden tests:** exact expected totals and time series for both initial views.
- **Accounting law tests:** cumulative token deduplication, root/descendant attribution, filter consistency, and missing-data behavior.
- **Indexer tests:** reverse ordering, idempotence, checkpoints, truncated lines, coalesced markers, monotonic commit revisions, and crash recovery.
- **Migration tests:** schema upgrade, resumable re-index, and cache invalidation.
- **Evidence tests:** raw locator resolution, archive/move rediscovery, and unavailable-source behavior.
- **Context tests:** ordered accumulation, downward steps, compaction, gaps, and token-estimate fidelity.
- **Review tests:** manifests, run discovery, strict report schema, immutability, missing citations, and partial directories.
- **Server tests:** loopback binding, process reuse, access token, revision notifications, and idle shutdown.
- **UI tests:** partial coverage, status tray, New data available behavior, preserved state, deep links, and review return paths.
- **Plugin smoke tests:** marketplace discovery, hook trust, missing/incompatible CLI guidance, bootstrap, and skill routes.

Each phase adds its fixture and end-to-end test before the vertical slice is complete.

## 14. Phased implementation

### Phase 1 — Plugin foundation

**Outcome:** A user can discover the plugin, install the CLI with Codex's help, verify it, and open a short-lived empty Inspector dashboard.

Deliver:

1. Go module and React/Vite/pnpm workspace.
2. Plugin manifest and local development marketplace entry.
3. Initial setup/open/inspect/review skills.
4. Plugin-owned hook definitions and the hook-marker contract.
5. GitHub Release packaging for macOS and Linux with checksums.
6. `version`, `doctor`, `status`, `sync`, and `open` command skeletons.
7. `CODEX_INSPECTOR_HOME` resolution and initial directory ownership.
8. Short-lived process manager, loopback server, embedded dashboard shell, access token, heartbeat, and idle shutdown.
9. CLI/plugin protocol compatibility checks.
10. Empty/setup/error states and the status/debugging tray.

Exit gate:

- the plugin is discoverable from a development marketplace;
- Codex can follow the setup prompt and install a verified binary;
- `doctor` confirms compatible plugin, CLI, data home, hooks, and server;
- a hook can signal work without parsing or writing SQLite;
- `open` starts or reuses the process and loads the dashboard;
- the process exits when idle;
- the phase works end to end on macOS and Linux.

### Phase 2 — Fact indexing and metric engine

**Outcome:** Inspector performs a real reverse-chronological sync and derives tested metrics from source-backed normalized facts.

Deliver:

1. Rollout and archive discovery.
2. `session_index.jsonl` label adapter.
3. Versioned rollout adapter registry.
4. SQLite schema and migrations for initial fact families.
5. Source checkpoints, fingerprints, idempotent normalization, and single-writer coordination.
6. Hook queue consumption and completed-turn reconciliation.
7. Reverse-chronological partial indexing with bounded concurrency.
8. Monotonic commit revisions and revision-addressable fact queries.
9. Session lineage and work-unit ownership, including forks, resumes, continuations, and orphans.
10. Token, timing, tool, compaction, capacity, coverage, and evidence facts.
11. Versioned metric registry, time-series bucketing, coverage, and in-memory cache.
12. Internal metric read models for the two agreed views.
13. Real sanitized rollout fixtures and golden derivation tests.

Exit gate:

- the same corpus can be scanned repeatedly without changing totals;
- appending one completed turn changes only the affected facts and metrics;
- golden tests prove cumulative-token handling and root/descendant rollups;
- partial recent metrics are queryable while the historical scan continues;
- status reports progress, failures, and the indexed time boundary;
- queries can remain pinned to an applied revision while newer commits arrive;
- every metric needed by both initial views is backed by normalized facts;
- unknown or missing fields produce coverage gaps rather than zeros.

### Phase 3 — Static Dashboard views

**Outcome:** Users can answer the Token & Capacity and Efficiency & Friction questions with real indexed data.

Deliver:

1. Internal metric/query API and typed React contracts.
2. Static Token & Capacity view and its agreed widgets.
3. Static Efficiency & Friction view and its agreed metric areas.
4. Shared time/project/model/reasoning/session-kind filters.
5. Root/descendant attribution.
6. Session evidence/drill-down links.
7. Partial coverage states and diagnostic details.
8. SSE status/revision events.
9. Global New data available workflow with state-preserving refetch.
10. Populated, indexing, empty, stale, incompatible, and error states.

Exit gate:

- every displayed value matches metric golden tests;
- both view questions can be answered using real indexed fixtures;
- filters apply consistently across widgets;
- partial coverage cannot be mistaken for complete totals;
- background indexing never hard-refreshes or disrupts the page;
- clicking New data available updates visible data without losing state;
- every session-level result has a stable Context Inspector route.

### Phase 4 — Context Inspector

**Outcome:** Users can move from a metric to exact source evidence and an honest locally reconstructed context state.

Deliver:

1. Session discovery and explainable root/descendant matches.
2. Root-session causal map with fork lineage and direct/inclusive token usage.
3. Focused full-turn inspection and chronological event ledger.
4. Exact source payload resolution with unavailable-source handling.
5. Surrounding model-cycle evidence.
6. Versioned context reducer.
7. Ordered context blocks, fidelity, and before/after boundaries.
8. Component token estimates and tokenizer/source rules.
9. Tool payload introduced versus cumulative context burden.
10. First-class compaction evidence.
11. Deep links for session, turn, event, and boundary.
12. Fallback discovery page when current session ID is unavailable.

Exit gate:

- Dashboard → session map → turn → event → context works end to end;
- direct and descendant totals agree with the metric engine;
- exact, derived, estimated, and unavailable states are distinguishable;
- exact payloads are read from source rather than copied into SQLite;
- missing source files preserve metrics and produce honest evidence failures;
- compaction can reduce context and show preserved, replaced, removed, and unavailable content;
- the UI never claims complete server-side context without exact evidence.

### Phase 5 — Effectiveness Reviews

**Outcome:** A user can explicitly start a dedicated Codex review and receive an immutable, evidence-linked report.

Deliver:

1. Reviews landing and history.
2. Shared single-session/time-period Review plan.
3. Fixed four-lens rubric and optional focus.
4. Scope, estimate, model, reasoning, prompt, and output preview.
5. Versioned manifest, run, and report schemas.
6. Dedicated Inspector-classified Codex task launch.
7. Normal-tool review instructions and scope manifest.
8. In-progress, complete, failed, and unrenderable states.
9. Atomic `review.json` discovery and validation.
10. Up to five evidence-backed findings.
11. Context Inspector citation links and return-to-review state.
12. Copyable action prompts without automatic execution.
13. Review-created-session exclusion.
14. Real-shaped valid, invalid, partial, and missing-citation fixtures.

Exit gate:

- a review never runs inside the target session;
- Codex does not start before explicit user confirmation;
- the new task receives the fixed rubric and frozen scope manifest;
- a completed report exists only when `review.json` exists;
- the first valid report is immutable;
- citations resolve into Context Inspector when their source exists;
- invalid or missing sources remain visible and diagnosable;
- continuing the Codex conversation cannot rewrite the report;
- Inspector never executes a recommendation.

## 15. Required implementation spikes

These spikes are bounded implementation work, not unresolved product direction:

1. **Plugin bootstrap:** confirm the clean marketplace-to-Codex-assisted CLI installation flow.
2. **Hook invocation:** verify trusted plugin hook paths, payload fields, and missing/incompatible CLI behavior on macOS and Linux.
3. **Session identity handoff:** identify supported ways for a skill to pass the current Codex session ID; retain discovery fallback.
4. **Review task launch:** prove how to create a new Inspector-classified Codex task, capture its identity, and open it when supported.
5. **Tokenizer rules:** select local tokenizer implementations and source-version rules for component estimates.
6. **SQLite packaging:** select and validate the driver/build mode for self-contained macOS and Linux binaries.
7. **Idle lifecycle:** tune heartbeat and idle-exit timing using real browser/skill behavior.

## 16. Explicit deferred scope

The architecture preserves extension points for these items, but the initial implementation does not include them.

### Distribution and runtime

- Windows binaries, hook commands, installation, and browser behavior.
- Always-on daemon or scheduled indexing.
- Cloud sync, accounts, hosted storage, or team dashboards.
- Required MCP server.
- Public local HTTP API.
- Public terminal metric queries, session inspection, review execution, or exports.

### Source adapters

- `history.jsonl`.
- memory SQLite databases and memory citation analytics.
- Codex application/catalog databases.
- automations, automation runs, and inbox state.
- semantic embeddings or topic clustering.

### Metrics and dashboard

- User-defined metrics.
- Runtime installation of new metric definitions.
- Persistent materialized metric rollups.
- User-created dashboard views.
- Drag-and-drop, resizing, and layout persistence.
- Widget configuration and the widget wizard.
- Arbitrary SQL.
- Workflow Profile and Outcomes & Reviews templates.
- Memory, automation, context, and parser-health templates.
- Graph exploration beyond the root-session causal map.

New plugin/CLI releases may add metrics. If they require new facts, the index schema upgrade may trigger a resumable re-index.

### Analysis and updates

- Standalone deterministic context-leakage analyzer.
- Standalone deterministic tool-thrashing analyzer.
- Deterministic user-facing findings outside Reviews.
- Event-level live indexing.
- Automatic application of new index revisions to visible widgets or Inspector state.

Neutral metric facts such as retries, failures, switching, compactions, context sources, and token concentration remain available to the review prompt without being pre-labeled as findings.

### Session evidence

- Complete reconstruction of unrecorded server-side context.
- Inspector-added secret masking or redaction.
- Durable copies of raw rollout payloads.
- Cross-session comparison inside Context Inspector.

### Reviews

- Review inside the target Codex thread.
- Technically sandboxed or CLI-restricted review evidence access.
- Multiple review profiles.
- User usefulness ratings.
- Developer levels, grades, or longitudinal scores.
- Scheduled or recurring reviews.
- Automatic report revision from continued conversation.
- Automatic application of recommendations.

## 17. References

Accepted product research remains unchanged:

- [Dashboard metrics brainstorming](../brainstorming/codex-home-dashboard-metrics.md)
- [Context Inspector brainstorming](../brainstorming/context-inspector-session-map.md)
- [Effectiveness Reviews brainstorming](../brainstorming/codex-effectiveness-reviews.md)
- [Core wireframe requirements](../ui/inspector-core/requirements.md)
- [Core wireframe decisions](../ui/inspector-core/decisions.md)
- [Core wireframe state matrix](../ui/inspector-core/state-matrix.md)
- [Interactive wireframe](../../prototypes/inspector-core/index.html)

Current Codex extension references:

- [Build plugins](https://developers.openai.com/codex/plugins/build)
- [Hooks](https://learn.chatgpt.com/docs/hooks)
