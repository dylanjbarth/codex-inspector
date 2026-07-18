# Codex Inspector demo architecture

- **Status:** Demo scope aligned; ready for Phase 0 implementation
- **Date:** July 17, 2026
- **Scope:** macOS plugin installation, current-format local indexing, Token & Capacity, session inspection, and basic Effectiveness Reviews
- **Authority:** Sole implementation authority for the demo milestone

## 1. Product boundary

Codex Inspector is a plugin-native, local-first observability product for Codex power users. The browser dashboard is a supporting surface owned and launched by the plugin.

The user discovers Inspector in the Codex plugin marketplace. The plugin supplies the hooks, skills, setup guidance, review instructions, and compatibility policy. A separately installed, open-source CLI supplies the local runtime.

The demo has three connected surfaces:

1. **Dashboard** — cross-session metrics through one static Token & Capacity view.
2. **Context Inspector** — source-backed session discovery, causal topology, completed-turn event evidence, and exact readable payloads.
3. **Reviews** — explicitly started, persisted `codex exec` tasks with structured local reports.

The browser application is not an independent hosted product. There is no account, cloud service, required MCP server, or remote database.

### 1.1 Demo completion boundary

The demo is complete when a user can:

1. install the plugin and a published macOS Inspector binary from a clean Codex environment;
2. run `codex-inspector doctor` and confirm the plugin, hook trust, Inspector CLI, Codex host, data home, and supported source format;
3. index every discoverable supported-format active and archived rollout in reverse chronological order with bounded concurrency;
4. view Token & Capacity metrics with root and descendant attribution;
5. move from a contributing root session into the wireframe-faithful causal map, select a completed turn, and inspect its exact recorded events and payloads;
6. start a single-session or time-period Review as a persisted `codex exec` task;
7. render the task's schema-valid `review.json`, follow its citations into Context Inspector, and open or resume the originating Codex task.

The demo does not include Linux, legacy source adapters, active-turn streaming, deterministic context reconstruction, component-level context token attribution, secret masking, or the complete Efficiency & Friction view.

### 1.2 Superseded decisions

This document is the sole implementation authority. Earlier brainstorming, Build Week, and wireframe documents remain design history and research inputs; they do not add requirements when they conflict with this architecture.

| Earlier direction | Demo decision in this architecture |
| --- | --- |
| Custom views, widget editing, resizing, and a widget wizard | One plugin-defined static Token & Capacity view |
| Token & Capacity plus a required Efficiency & Friction view | Token & Capacity is required; Efficiency & Friction is the first stretch phase |
| Live active-session events and automatic map growth | Context Inspector is a read-only snapshot through the last indexed completed turn |
| Deterministic context-leakage and tool-thrashing analyzers | Deferred; neutral facts may be available to Reviews |
| Complete locally reconstructed context and component attribution | Deferred; show exact recorded context only when the source proves it and otherwise show unavailable |
| Inspector-added secret redaction | Exact raw local evidence with a prominent sensitive-data warning; diagnostics exclude payloads |
| macOS and Linux release binaries | macOS demo binary only |
| Broad or historical Codex-format compatibility | Only explicitly tested current Codex source formats; unsupported sources are visibly skipped |
| A bundled sanitized demo corpus | The demo uses the user's recent and historical local supported-format sessions; tests use tiny synthetic records only |
| Read-only, output-schema-constrained review execution | A persisted ordinary `codex exec` task follows the plugin Review skill and writes `review.json` directly |
| A technically restricted review evidence interface | Prompt- and manifest-scoped ordinary Codex tools; technical enforcement is deferred |
| Five large implementation phases | Phase 0 plus six worker-sized demo phases |

## 2. Architectural principles

1. **The plugin is the product.** Skills and hooks create the Codex-native experience; the CLI and dashboard implement supporting capabilities.
2. **Facts precede metrics.** The index stores normalized facts. Metrics are versioned definitions over those facts. Widgets only render metric results.
3. **Source logs remain authoritative.** Inspector never edits or copies complete rollout logs.
4. **Derived state is rebuildable while sources remain readable.** SQLite facts and metric caches can be reconstructed from supported source logs. Facts retained after a source disappears are labeled as retained observations with unavailable evidence, not fully rebuildable truth.
5. **Completed turns are the freshness boundary.** The UI does not promise event-level live indexing.
6. **Indexing is demand-driven.** No permanent daemon or scheduled background process is required.
7. **Partial data remains useful.** Reverse-chronological indexing exposes recent results early, continues toward the complete supported history without a session cap, and labels incomplete coverage.
8. **Evidence is source-backed.** Metrics and reviews resolve to normalized facts and pointers into the original logs.
9. **Local evidence is transparent.** Inspector displays exact readable payloads from local logs without adding a secret-masking layer. The UI warns that evidence may contain sensitive data, and diagnostics never include raw payloads.
10. **Reviews are explicit Codex tasks.** Inspector provides scope, a plugin skill, and a report contract; a persisted `codex exec` session performs the qualitative analysis with ordinary Codex tools.
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

Go is selected for predictable deployment, streaming I/O, and straightforward bounded concurrency. The demo publishes one macOS binary artifact and checksum for the architecture used by the demo and clean-install test machine. Other macOS architectures and Linux cross-compilation are deferred.

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

Process metadata is written atomically with user-only permissions after the server has bound its port. It contains a random instance ID in addition to PID, port, protocol, and token metadata. Reuse requires an authenticated health response with the expected instance ID and protocol; PID existence alone is never considered healthy, which prevents PID-reuse mistakes. A user-scoped process lock serializes competing `open`, `_serve`, and writer starts.

The demo loopback security contract is:

- bind only to `127.0.0.1` on an ephemeral port;
- generate a cryptographically random per-process access token;
- place the token only in the fragment of the initial browser URL;
- exchange the fragment token once for a SameSite session cookie and remove the fragment from browser history;
- validate `Host` and `Origin` on state-changing and streaming requests;
- serve no remote scripts, fonts, images, or other assets;
- set a restrictive Content Security Policy;
- expose evidence through opaque indexed identifiers, never client-supplied filesystem paths;
- create run metadata and the database with user-only filesystem permissions.

The demo does not defend against a malicious process already running as the same OS user. That threat and Unix-domain-socket or OS-auth alternatives are deferred.

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

`CODEX_HOME` is the inspected source. `CODEX_INSPECTOR_HOME` is derived Inspector state. Source discovery canonicalizes paths and rejects `CODEX_INSPECTOR_HOME`, including aliases reached through symlinks, so the indexer cannot ingest its own data.

The plugin-provided hook receives Codex's `PLUGIN_ROOT` and `PLUGIN_DATA` environment variables. `PLUGIN_DATA` is diagnostic/plugin state, not the analytics database. The installed Inspector CLI remains the only writer to `CODEX_INSPECTOR_HOME`.

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

When the CLI is unavailable, the plugin setup skill provides a prompt that asks Codex to:

1. confirm that the user is on a supported macOS architecture;
2. download a plugin-compatible binary from GitHub Releases;
3. verify the published checksum;
4. install it into a user-writable directory on `PATH`;
5. run `codex-inspector doctor`;
6. open Inspector and begin the initial sync.

GitHub Releases are the source of truth for the demo macOS binary and checksum. Source builds are a developer workflow, not the clean-install demo path. Linux packages, macOS notarization, automatic updates, shell-profile repair, and complete uninstall management are deferred.

### 4.3 Version compatibility

Plugin and CLI releases are coordinated but independently versioned.

Compatibility uses:

- Codex host version and source-format capability identifier;
- plugin semantic version;
- CLI semantic version;
- an explicit plugin/CLI protocol version;
- an index schema version;
- a plugin-declared supported CLI range.

Plugin-only skill or prompt improvements do not require a CLI upgrade when the protocol remains compatible. An incompatible Inspector CLI refuses hook markers and database writes, while the hook exits successfully and Inspector surfaces a rate-limited setup warning. An unsupported Codex host or source format is reported by `doctor` and skipped during indexing rather than parsed optimistically.

Phase 0 freezes the exact tested Codex host version, source-format fingerprints, plugin/CLI range, and supported demo macOS architecture. “Current Codex” means that explicit matrix, not an unbounded promise about future releases.

### 4.4 Public CLI surface

The demo public CLI remains intentionally small:

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

Plugin-owned hooks never parse complete rollouts, calculate metrics, or write SQLite. A thin plugin-bundled hook command locates the compatible Inspector CLI and passes the hook JSON to `codex-inspector _hook`. The CLI writes one uniquely named marker file through exclusive create and atomic rename, then exits quickly; hooks never contend on one shared append file. If the CLI is absent or incompatible, the hook exits successfully without a marker so it never blocks Codex.

Relevant lifecycle signals include:

- user prompt submitted;
- turn stopped;
- subagent started;
- subagent stopped;
- context compacted;
- session started or resumed when useful for reconciliation.

Markers contain only the identifiers, source locator when available, event kind, and observation time needed to schedule work. The supported hook contract uses current documented `session_id`, `turn_id`, `transcript_path`, and subagent identifiers where the event supplies them; Phase 0 records the exact payload fixtures. The transcript path is a locator, not a stable transcript schema.

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

The first sync inventories all discoverable active and archived rollout sources, then parses supported sources in reverse chronological order so recent metrics become useful quickly. It does not impose a session, file, date, or byte cap. Bounded worker concurrency, cooperative yielding, and transaction sizing control resource use while the scan proceeds toward the entire supported history.

Source inventory and checkpoints allow the indexer to:

- skip unchanged files using identity, size, timestamps, and fingerprints;
- resume append-only JSONL at the last complete record boundary;
- retry a truncated final record later;
- prioritize hook-marked sessions;
- coalesce duplicate markers;
- avoid concurrent writers through a process/database lock;
- yield and bound I/O concurrency to avoid CPU or disk thrashing.

Every committed normalization write is idempotent. Re-reading the same record with the same adapter version produces the same fact identity and does not change metric totals. File identity, inode, timestamps, and paths are discovery hints; stable session/segment metadata and content fingerprints prevent an active-to-archive move from creating duplicates.

The demo adapter assumes supported rollouts append complete JSONL records or move unchanged between active and archive locations. A truncated final record is retried. A source that shrinks or whose indexed prefix changes is marked `requires_rebuild`; it is not patched speculatively while queries are active. Rebuilding that source or changing adapter versions creates a new dataset epoch and atomically replaces the old derived dataset after validation.

### 5.4 Turn consistency

A turn is provisional until a completion or terminal record makes it eligible for committed facts and metrics.

- Dashboard metrics include completed turns.
- Context Inspector shows data through the last indexed completed turn.
- Active-turn records remain in the source checkpoint buffer and are not exposed as committed event facts.
- Aborted, interrupted, or truncated turns are classified explicitly during reconciliation.

Completed-turn consistency is the user-facing promise. Event-level live updates are out of scope.

### 5.5 Source coverage

The demo indexes supported current-format instances of:

- active rollout JSONL logs;
- archived rollout JSONL logs;
- `session_index.jsonl` for titles and discovery metadata;
- capacity and rate-limit observations recorded in rollouts.

The demo does not index `history.jsonl`, memory databases, automation/application databases, or other Codex state. It also does not attempt older rollout adapters. Unsupported sources remain in inventory with their detected Codex version, reason, and byte count so coverage is honest.

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

The fact schema must cover every metric required by Token & Capacity, the causal map and completed-turn event ledger, Review scope selection, and evidence citations. Efficiency & Friction facts are added only when they fall out cheaply from the same normalized events.

Adding a future metric that needs facts omitted by the installed index schema may require building and atomically swapping a new dataset epoch. This is acceptable as part of a later release/index-schema upgrade.

### 6.2 Fact families

The initial fact model includes:

- discovered source artifacts and parser/checkpoint state;
- projects and observed cwd/worktree aliases;
- logical sessions, source/purpose classification, and source segments;
- session lineage edges;
- completed and other terminal turns, including aborted, interrupted, and reconciled truncated outcomes;
- ordered normalized event envelopes;
- user and assistant message metadata and source locators;
- a local full-text search index over readable titles, messages, and tool results that returns event IDs and match categories without copying complete payloads into display tables;
- model invocations and configuration;
- recorded token snapshots and normalized usage;
- tool invocations and results;
- tool family, timing, success, and exit state;
- compaction and context-lifecycle changes;
- recorded capacity/rate-limit snapshots;
- evidence references;
- parser and field-coverage observations.

Raw messages, tool arguments, tool results, instruction text, and recorded reasoning summaries are read from source logs when requested. SQLite retains the facts, hashes, lengths, classifications, ordering, and locators needed to find them.

The full-text index is sensitive derived data even when configured as a contentless/token-only FTS index. It receives the same user-only filesystem permissions as the database, is deleted with the derived index, never appears in diagnostics, and is not described as anonymized.

### 6.3 Minimum identity contract

Phase 0 must freeze a concrete schema and synthetic contract fixtures before the indexing worker begins. At minimum, it defines these stable identities and ownership rules:

| Concept | Stable identity and ownership |
| --- | --- |
| Source artifact | Canonical source kind plus immutable session/segment metadata and prefix fingerprint; path and inode are aliases |
| Logical session | Source-provided session/thread ID |
| Source segment | Logical session ID plus source-provided segment identity or deterministic segment fingerprint |
| Root work unit | User-initiated root session ID |
| Turn | Logical session ID plus source turn ID; deterministic ordinal fallback is allowed only for a documented supported shape |
| Event | Source segment plus source record identity/ordinal plus normalized semantic phase |
| Tool call | Logical session ID plus source call ID and semantic phase |
| Metric contribution | Completed turn identity plus metric formula version |
| Evidence reference | Opaque Inspector evidence ID resolving to an event fingerprint and source locator |

The schema must include source inventory/checkpoints, projects and aliases, sessions and segments, lineage edges, completed turns, ordered event envelopes, messages, tools, normalized turn usage, capacity observations, coverage observations, evidence locators, and index revisions. Database uniqueness constraints enforce the identities above; application-only deduplication is insufficient.

### 6.4 Source locators and missing evidence

Every source-backed fact retains enough provenance to attempt resolution:

- source artifact identity and path;
- source-provided session, turn, event, and call IDs;
- record ordinal and byte range where practical;
- content fingerprint;
- adapter/parser version.

If a source log is moved, archived, or becomes unreadable:

- facts already committed from completed turns remain visible as retained observations;
- Inspector attempts deterministic rediscovery by identity/fingerprint;
- exact payload evidence may become unavailable;
- the UI explains the missing source without silently deleting the metric or report reference;
- retained observations are no longer described as rebuildable or currently verifiable until the source is rediscovered.

Source deletion, “forget this session,” and retention controls are deferred from the demo. The plan must not imply that deleting a Codex source automatically deletes Inspector-derived data.

### 6.5 Session identity, project identity, and lineage

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
- Inspector Review roots and their descendants retain an `inspector_review` purpose classification. They are excluded from future Review scopes but not silently removed from recorded-capacity accounting.

Project identity uses the canonical normalized Git remote when present. Local clones and worktrees with that remote roll up to the same project. When no remote exists, Inspector falls back to the canonical Git root and then normalized cwd. Original cwd, Git root, worktree, branch, and remote strings remain locally available as secondary evidence and dimensions.

### 6.6 Accounting invariants

1. A source file is not necessarily a logical session.
2. A spawned agent is not another user-initiated root.
3. Additive work-unit metrics preserve combined, user-root-direct, descendant, Inspector Review, and other/orphan contributions where those populations exist.
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

### 7.2 Required demo metric catalog

Phase 0 freezes these keys and formula versions with golden examples. The demo does not leave their semantics to the Dashboard worker.

| Metric | Required formula |
| --- | --- |
| `recorded_tokens` | Sum normalized usage for eligible completed turns. Prefer an exact source-provided last-turn usage; otherwise derive a non-negative delta from consecutive cumulative snapshots in the same logical session. Never sum cumulative snapshots. |
| `recorded_tokens_by_kind` | `recorded_tokens` grouped by user-root-direct, descendants of user roots, Inspector Review work, and other/orphan supported roots using purpose and root work-unit ownership on each completed turn. The UI may visually subordinate zero or small residual categories but may not silently relabel them as user roots. |
| `recorded_tokens_over_time` | Assign each completed turn's normalized usage to its completion timestamp, bucketed in the selected display timezone and grouped by the same mutually exclusive contribution kinds. |
| `token_composition` | Show mutually exclusive components: uncached input, cached input, visible output, and reasoning output. Cached input is a subset of recorded input and reasoning is a subset of recorded output where the source uses inclusive totals; subtract only when the supported source contract proves those semantics. Show any unreconciled residual explicitly. |
| `top_root_sessions_by_tokens` | Rank root work units by inclusive `recorded_tokens`, while returning root-direct and descendant contributions separately. |
| `latest_capacity_observation` | Latest supported recorded observation by observation time, preserving limit identity, used/remaining percentage, reset timestamp, and staleness. It is not recomputed from token facts. |
| `capacity_drawdown` | Ordered supported capacity observations partitioned by recorded limit/window identity and reset boundary. Do not infer missing observations or causally equate plan utilization with local token totals. |

Time filtering applies to completed-turn contribution time, not the root session start time. A root session may therefore contribute only some turns to a selected period while its drill-down still opens the complete indexed root tree. Model and reasoning filters apply at the completed-turn configuration level. Project filtering applies through canonical root project identity. Session-kind filtering never causes user-root-direct, descendant, Inspector Review, or other/orphan labels to change meaning.

Capacity observations are account/plan-wide recorded facts. Project, model, reasoning, and session-kind filters do not alter the latest-capacity or drawdown widgets; those widgets label their filter independence explicitly. The selected time range constrains capacity history but not the separate latest-observation card. Local token totals and recorded capacity utilization may be juxtaposed, but the UI never claims that one reconciles to or caused the other.

### 7.3 Time series

Facts retain their natural timestamps. The metric engine supports hour, day, week, and month bucketing where the metric definition allows it.

The Dashboard sends an IANA timezone, defaulting to the browser's current local timezone, with every time-series query. Go performs calendar bucketing in that timezone using timezone-aware boundaries; day/week/month buckets are not fixed-duration UTC arithmetic and must handle daylight-saving transitions.

Time-series responses include:

- ordered buckets;
- timezone;
- grouped series;
- root/direct/descendant attribution where applicable;
- indexed coverage boundary;
- formula version;
- fidelity and source coverage.

### 7.4 Metric result caching

The demo calculates metrics from the compact fact store and maintains a bounded in-memory result cache keyed by:

- metric definition and formula version;
- filters and groupings;
- time range and grain;
- index revision.

The cache is disposable and does not survive process restart. Persistent materialized rollups are added only after real query profiling proves they are necessary.

### 7.5 Coverage and fidelity

Each result carries:

- eligible sessions or turns;
- observations with usable data;
- coverage ratio;
- indexed time boundary;
- exact, derived, estimated, or unavailable fidelity;
- reasons for excluded records.

Complete coverage remains visually quiet. Partial coverage produces a compact warning and a link to diagnostic details. Missing values use an explicit fallback, never zero.

## 8. Initial Dashboard

### 8.1 Static view

The required dashboard ships one plugin-defined static Token & Capacity view. User-created views, drag-and-drop, resizing, widget configuration, and the complete Efficiency & Friction view are deferred.

View and widget definitions are declarative from the start so future editing creates user-owned configurations without changing metric contracts.

Global controls include:

- relative time range;
- project;
- model;
- reasoning level;
- contribution kind: user root, descendant, Inspector Review, or other/orphan.

### 8.2 Token & Capacity

User question:

> Where did my recorded Codex usage go, and how does it relate to my recorded plan limits?

The agreed view elements are:

1. current recorded limit utilization and reset countdown;
2. total recorded tokens with user-root-direct, descendant, Inspector Review, and other/orphan contributions when present;
3. token usage over time stacked by the same mutually exclusive contribution kinds;
4. token composition across input, cached input, output, and reasoning tokens;
5. most token-intensive user-initiated root sessions;
6. capacity drawdown across recorded limit windows;
7. direct drill-down to contributing sessions and Context Inspector.

### 8.3 Efficiency & Friction stretch phase

User question:

> Where is execution friction accumulating, and which sessions, tools, and working patterns explain it?

After the required demo is complete, the first stretch phase may add the agreed metric areas:

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

Each committed indexing transaction receives a monotonically increasing index revision within one dataset epoch. Only immutable facts from completed turns become query-visible, and each records the revision at which it became visible. A browser session pins one applied revision for all data queries, which filter out facts from newer revisions.

The demo does not perform in-place corrections to committed metric/event facts. A supported append adds new completed-turn facts. An adapter change, changed source prefix, or other correction builds a new dataset epoch and swaps it into place only after validation; open browsers must then accept a full dataset refresh. This constraint is what makes revision-pinned queries implementable without retaining general historical row versions.

The local server emits index-status and new-revision notifications through server-sent events. Incoming commits do not automatically advance the browser's applied revision or replace displayed data.

- The status tray updates as work progresses.
- Existing widgets, tables, filters, scroll, and selections remain unchanged.
- A global **New data available** control appears when a newer index revision exists.
- Filter changes continue to query the currently applied revision.
- Activating New data available advances the applied revision and refetches visible data without page navigation or application-state reset.

During the initial reverse scan, the Dashboard is usable as soon as facts exist. Widgets disclose processed supported sources, skipped/failed sources, whether the supported inventory is complete, and the oldest fully processed contribution time. A single date must not imply gap-free coverage when failed or unsupported sources exist.

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
- The wireframe interactions remain: minimap, pan/zoom, fit controls, manual branch collapse, and automatic turn focus with a persistent topology rail.

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

Inspector displays readable source payloads exactly as found. It does not mutate the source or add a masking layer. The evidence surface carries a persistent warning that payloads may contain secrets or other sensitive local data. Content already truncated, redacted, encrypted, or unavailable upstream is labeled accordingly.

Raw evidence is untrusted display data. Text and structured values are escaped; recorded HTML, Markdown, ANSI sequences, terminal control characters, and URLs are never executed as application content. Large records use opaque-ID range reads and bounded browser chunks while remaining fully inspectable. Raw payloads never enter doctor output, server logs, or copied diagnostics.

### 9.4 Demo context boundary

The demo does not maintain a local context reducer. It preserves the wireframe's continuous evidence surface, but every context region follows these rules:

- If a supported source record contains an exact model-input boundary, Inspector may label and render it **Context seen by Codex**.
- If the source records a context-related event without the complete model input, Inspector renders the exact event and an **Unavailable in demo** context state.
- Inspector does not synthesize intermediate before/after context, infer block lifetimes, or estimate missing server-side material.
- Missing instructions, encrypted reasoning, unexplained token differences, and unsupported behavior remain explicit gaps.
- Recorded overall token usage remains available, but individual message, instruction, tool-definition, and tool-output token contributions are not estimated.

This keeps the causal map, focus topology, chronological ledger, surrounding recorded model-cycle evidence, token accounting, and review deep links from the approved wireframe without implementing context reconstruction.

### 9.5 Compaction

Compaction remains a first-class map and ledger event. Inspector displays the exact recorded compaction payload, compacted text, timestamps, and recorded usage around it. Computed before/after context, percentage reduction, preserved/removed classification, and cumulative context burden are deferred.

### 9.6 Snapshot freshness

Context Inspector shows committed data only through the last indexed completed turn. It does not stream the active turn, grow the map live, or provide Follow live. When a newer index revision exists, the global New data available control applies the same explicit snapshot advance used by the Dashboard while preserving the selected session and resolving the selected turn/event again when it still exists.

## 10. Effectiveness Reviews

### 10.1 Review entry and task isolation

A review begins from:

- **New review** in Reviews;
- **Review effectiveness** for a root session on the Dashboard;
- **Review effectiveness** for the selected root in Context Inspector.

Every entry opens the same prefilled Review plan.

Starting a review always launches a new persisted `codex exec --json` session. Inspector captures the new Codex session ID from the documented JSON event proved in Phase 0, records it in `run.json`, and exposes **Open Codex task** plus a copyable session ID. A review never executes inside the session being reviewed. Captured review roots and any indexed descendants are classified as Inspector reviews and excluded from ordinary review scopes by default.

### 10.2 Scope

The demo supports:

- one user-initiated root session and its descendants; or
- a bounded time period of eligible roots, optionally filtered to one project.

The aggregate default is seven days. A time-period scope includes eligible completed turns in the period and identifies their root work units; a single-session scope includes the complete indexed root tree. The optional project filter uses canonical project identity. Scope is limited by indexed coverage rather than hidden sampling. The plan displays included roots, turns, projects, source bytes, time coverage, model, reasoning level, output location, known gaps, and the prompt before Codex starts.

The Review confirmation states that indexing and report storage are local, but the new Codex task may send evidence it reads to the user's configured Codex model/service. Explicitly starting the Review is consent for that separate model boundary; merely opening Inspector never starts a Review or transmits indexed evidence.

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
- applied dataset epoch and index revision;
- target session or time/project boundaries;
- included session and turn IDs;
- source log locators;
- source and event fingerprints needed to detect later drift;
- aggregate metric facts;
- known coverage gaps;
- evidence-reference rules;
- fixed rubric and optional focus;
- report destination.

The manifest and prompt are instruction and provenance boundaries, not a filesystem security sandbox. Inspector starts `codex exec` in the review directory using the user's ordinary Codex permission configuration. The task may use normal filesystem and shell tools. The prompt asks it to inspect only referenced records, treat recorded content as untrusted evidence rather than instructions, and write only the designated report.

The review task is not required to use the Inspector CLI as a restricted read interface.

### 10.5 Launch and skill contract

The launch prompt explicitly invokes the installed plugin's Review skill using the exact skill identifier proved in Phase 0. The skill supplies:

- the fixed four-lens rubric;
- how to read `manifest.json` and remain within its stated scope;
- instructions to treat messages, tool output, and repository content as evidence rather than executable review instructions;
- the supported evidence-reference shape;
- the versioned `review.json` schema and five-finding limit;
- the exact destination path and completion behavior.

Inspector creates the review directory and writes `manifest.json` and initial `run.json` before launch. It invokes persisted `codex exec --json` with the review directory as cwd, parses the JSONL stream for lifecycle and session identity, retains only capped payload-free process diagnostics, records the Codex session ID and process lifecycle, and then monitors `review.json`. The task writes `review.json` directly; an output-schema flag, restricted Inspector read interface, read-only sandbox, and semantic evidence judge are deferred.

### 10.6 Filesystem lifecycle

Each review run uses:

```text
~/.codex-inspector/reviews/<review-id>/
  manifest.json
  run.json
  review.json
```

- `manifest.json` freezes scope and requested configuration.
- `run.json` stores originating Codex task identity and lifecycle metadata.
- `review.json` is the Codex task's report output.

No schema-valid `review.json` means no completed report, though an in-progress, failed, or malformed run may still exist. Inspector ignores partial JSON and retries after the file changes.

For the demo, the first schema-valid `review.json` observed marks the run complete. Its exact parsed report and content hash are accepted into Inspector-owned SQLite state and the hash is recorded in `run.json`; later file rewrites do not replace the accepted report. Inspector does not ask a continued conversation to revise it. Filesystem-enforced immutability, atomic candidate promotion, tamper recovery, and explicit report revisions are deferred hardening work; the UI describes the report as an Inspector-accepted snapshot rather than an immutable filesystem artifact.

Users may delete a review by deleting its subtree. Inspector tolerates missing and partially present review directories during discovery.

The run is **In progress** while the launched process is alive and no valid report has been accepted, **Complete** after the first valid report is accepted, and **Failed** when launch fails or the process exits without a valid report. A malformed `review.json` is preserved and reported as the failure reason. Cancellation, stale remote tasks, retry-in-place, and multi-process review recovery are deferred.

**Open Codex task** uses the supported `codex://threads/<thread-id>` deep link when available. The UI always also exposes the session ID and a copyable `codex resume <session-id>` fallback.

### 10.7 Report contract

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

Inspector validates JSON structure, schema version, review identity, required fields, allowed enum values, finding count, and citation resolvability against the frozen manifest revision. It does not act as a second semantic judge. A resolvable citation proves location, not that the cited evidence semantically supports the finding; the demo documents that limitation.

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
- opaque evidence resolution and exact recorded-context resolution;
- review planning, launch coordination, history, and reports;
- server-sent status and new-revision notifications.

The browser does not join normalized tables, calculate metrics, reconstruct context, read arbitrary paths, or write review artifacts.

Phase 0 defines an internal OpenAPI document or equivalently generated Go-first schema covering every required endpoint, request, response, error, and SSE event. TypeScript contracts are generated from that source; “where practical” is not sufficient for a phase boundary. List/search endpoints are paginated, metric responses limit buckets and series, and evidence responses use explicit byte/chunk limits. The API may change with coordinated CLI/dashboard releases and is not a public integration surface in the demo.

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
  inspector/                  causal map, ledger, exact recorded evidence read models
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
  synthetic/                 tiny hand-authored records for supported contracts
docs/
```

The Go module and pnpm workspace build together. The production web bundle is embedded into the Go binary.

## 13. Testing strategy

### 13.1 Synthetic contracts and local corpus validation

The demo does not spend time producing a sanitized user-facing corpus or copy real rollout content into the repository. Automated tests use tiny hand-authored synthetic JSONL records that cover only the frozen current-format shapes needed by the contract:

- root and descendant metadata;
- one completed and one truncated turn;
- cumulative and last-turn token snapshots;
- mirrored tool-call phases;
- capacity observations;
- compaction and exact raw evidence locators;
- active-to-archive movement and an unsupported-version source.

The synthetic records use obviously fake content and explicit expected normalized facts. They are test inputs, not a sample mode or demo dataset.

The developer's real local supported-format corpus is the integration and demo corpus. Tests and scripts may read it only through explicit local commands, must never modify it, and must never copy its payloads into repository files, snapshots, diagnostics, or CI artifacts. Golden unit tests remain deterministic and do not depend on the developer corpus.

### 13.2 Test layers

- **Contract tests:** current source-format discriminator and hook payload to expected supported/unsupported decisions.
- **Adapter golden tests:** synthetic supported source to expected normalized facts.
- **Metric golden tests:** exact expected totals and time series for Token & Capacity.
- **Accounting law tests:** cumulative token deduplication, root/descendant attribution, filter consistency, and missing-data behavior.
- **Indexer tests:** reverse ordering, idempotence, checkpoints, truncated lines, coalesced markers, monotonic commit revisions, and crash recovery.
- **Dataset-epoch tests:** changed-prefix detection, rebuild, validation, atomic swap, and cache invalidation.
- **Evidence tests:** raw locator resolution, archive/move rediscovery, and unavailable-source behavior.
- **Inspector tests:** map/ledger ordering, focus state, exact payload resolution, compaction evidence, and unavailable-context states.
- **Review tests:** scope manifests, `codex exec` event parsing, run discovery, strict report schema, first-valid acceptance, missing citations, and partial directories.
- **Server security tests:** loopback binding, fragment-token exchange, cookie/origin/host checks, CSP, opaque evidence IDs, process reuse, revision notifications, and idle shutdown.
- **Adversarial display tests:** HTML, Markdown, ANSI/control characters, malformed UTF-8, huge payload chunking, and path-traversal attempts.
- **UI tests:** partial coverage, status tray, New data available behavior, preserved state, deep links, and review return paths.
- **Plugin smoke tests:** marketplace discovery, hook trust, missing/incompatible CLI guidance, bootstrap, and skill routes.

Each phase adds or extends its synthetic contract and end-to-end test before the vertical slice is complete. The final phase also runs a manual read-only validation against the real local corpus and a clean-install test in an isolated `CODEX_HOME`.

## 14. Phased implementation

### Phase 0 — Feasibility and frozen contracts

**Outcome:** Every external Codex dependency and every cross-phase data contract needed by the demo is proved before product implementation begins.

Deliver:

1. Record the exact supported Codex host version, rollout/session-index shapes, hook events and payloads, and supported demo macOS architecture.
2. Prove the clean local marketplace install, plugin hook trust flow, `PLUGIN_ROOT`/`PLUGIN_DATA` behavior, and missing-CLI hook behavior.
3. Prove the supported way, if any, for an invoked skill to identify the current Codex session; freeze the discovery fallback when no reliable handoff exists.
4. Prove a persisted `codex exec --json` launch, identify the session-start event carrying the resumable session ID, and prove `codex://threads/<id>` plus `codex resume <id>` handoff.
5. Prove that the installed Review skill is available inside the launched task and freeze its invocation identifier.
6. Select the self-contained Go SQLite driver/build mode and prove the demo macOS binary.
7. Freeze synthetic current-format contract records without copying real rollout content.
8. Freeze the fact identities, SQLite schema, dataset-epoch/revision rules, and source state machine in `docs/contracts/fact-model.md`.
9. Freeze the Token & Capacity metric keys, formulas, filter semantics, coverage rules, and golden examples in `docs/contracts/metrics.md`.
10. Freeze the internal API/SSE schema and generated TypeScript contract path in `docs/contracts/internal-api.md`.
11. Freeze versioned manifest, run, and report JSON schemas plus the Review launch prompt contract.
12. Record explicit performance budgets for hook latency, indexing concurrency/transaction size, API page and payload sizes, browser evidence chunks, and process idle timing.

Exit gate:

- every proof runs on the demo macOS machine from a documented command;
- no required capability remains described only as a later spike;
- unsupported source versions are detected without optimistic parsing;
- one synthetic root/descendant corpus produces agreed facts and Token & Capacity results;
- a disposable Review task writes a schema-valid report, yields a session ID, and can be opened or resumed;
- downstream workers can import frozen schemas rather than inventing parallel contracts.

### Phase 1 — Plugin foundation

**Outcome:** A user can discover the plugin, install the published macOS CLI with Codex's help, verify it, and open a secure short-lived empty Inspector dashboard.

Deliver:

1. Go module and React/Vite/pnpm workspace.
2. Plugin manifest, local development marketplace entry, and release marketplace metadata needed for the clean-install demo.
3. Initial setup/open/inspect/review skills.
4. Plugin-owned hook shim and definitions using the frozen hook-marker contract and non-blocking missing-CLI behavior.
5. GitHub Release packaging for the one supported demo macOS artifact with its checksum.
6. `version`, `doctor`, `status`, `sync`, and `open` command skeletons.
7. `CODEX_INSPECTOR_HOME` resolution and initial directory ownership.
8. Short-lived process manager, `127.0.0.1` server, embedded dashboard shell, fragment-token/cookie exchange, Host/Origin/CSP checks, heartbeat, and idle shutdown.
9. Codex-host/plugin/CLI/source-format compatibility checks.
10. Empty/setup/error states and the status/debugging tray.

Exit gate:

- the plugin is discoverable from a development marketplace;
- Codex can follow the setup prompt and install a verified binary;
- `doctor` confirms compatible Codex host, supported source format, plugin, Inspector CLI, data home, hook trust, and server;
- a hook can signal work without parsing or writing SQLite;
- `open` starts or reuses the process and loads the dashboard;
- the process exits when idle;
- the phase works end to end from a clean isolated macOS Codex environment.

### Phase 2 — Current-format full-history indexing

**Outcome:** Inspector inventories all local sources and incrementally normalizes every supported-format completed turn into immutable source-backed facts.

Deliver:

1. Rollout and archive discovery.
2. `session_index.jsonl` label adapter.
3. The one frozen current-format rollout adapter plus explicit unsupported-version detection.
4. The frozen SQLite schema, migrations, dataset epoch, and source state machine.
5. Source checkpoints, fingerprints, idempotent normalization, and single-writer coordination.
6. Hook queue consumption and completed-turn reconciliation.
7. Reverse-chronological partial indexing with bounded concurrency.
8. Monotonic commit revisions and revision-addressable immutable fact queries.
9. Session lineage and work-unit ownership, including forks, resumes, continuations, and orphans.
10. Message, token, model, tool, compaction, capacity, coverage, evidence-locator, and local full-text-search facts.
11. Opaque evidence resolution with fingerprint verification, move/archive rediscovery, escaped range reads, and unavailable-source behavior.
12. Synthetic golden normalization, accounting-law, checkpoint, changed-prefix, crash-recovery, and unsupported-version tests.

Exit gate:

- the same corpus can be scanned repeatedly without changing totals;
- appending one completed turn changes only the affected facts and normalized usage contributions;
- golden tests prove cumulative-token normalization, mirrored-event deduplication, project identity, and root/descendant ownership;
- partial recent facts and session discovery are queryable while the full supported history scan continues;
- the scan has no history cap and eventually reaches every inventoried supported source;
- status reports processed, queued, skipped, failed, and requires-rebuild sources without implying gap-free date coverage;
- queries can remain pinned to an applied revision while newer commits arrive;
- unknown or missing fields produce coverage gaps rather than zeros;
- active and truncated turns never appear as completed metrics or completed-turn event evidence.

### Phase 3 — Token & Capacity dashboard

**Outcome:** Users can answer where recorded Codex usage went and how it relates to the latest recorded capacity observations.

Deliver:

1. Versioned metric registry, time bucketing, coverage, contributing-session queries, and bounded in-memory cache using the frozen formulas.
2. Internal metric/query API generated from the frozen contract.
3. Static Token & Capacity view and its agreed widgets.
4. Shared time/project/model/reasoning/contribution-kind filters.
5. User-root-direct, descendant, Inspector Review, other/orphan, and combined attribution without double-counting.
6. Contributing root-session links using stable Context Inspector routes with an honest pre-Phase-4 placeholder state.
7. Partial coverage, unsupported-source, and stale-capacity states with diagnostic details.
8. SSE status/revision events.
9. Global New data available workflow with state-preserving refetch.
10. Populated, indexing, empty, stale, incompatible, and error states.

Exit gate:

- every displayed value matches metric golden tests;
- the Token & Capacity question can be answered using synthetic golden data and the real local corpus;
- filters apply consistently across widgets;
- partial coverage cannot be mistaken for complete totals;
- background indexing never hard-refreshes or disrupts the page;
- clicking New data available updates visible data without losing state;
- every session-level result has a stable Context Inspector route, even before the next phase implements the destination UI;
- token composition never double-counts inclusive cached-input or reasoning-output subsets;
- capacity is visibly a recorded point-in-time observation rather than a value inferred from local token totals.

### Phase 4 — Context Inspector

**Outcome:** Users can move from a metric to the approved causal-map experience and exact source evidence through the last completed turn.

Deliver:

1. Session discovery and explainable root/descendant matches.
2. Root-session causal map with fork lineage and direct/inclusive token usage.
3. Focused full-turn inspection and chronological event ledger.
4. Exact source payload resolution through opaque IDs, safe range reads, and unavailable-source handling.
5. Surrounding model-cycle evidence.
6. The wireframe's minimap, pan/zoom, fit, branch-collapse, automatic turn-focus, and topology-rail interactions.
7. Exact recorded model-input display only where the supported source proves it; otherwise explicit unavailable-context states.
8. First-class recorded compaction evidence without reconstructed before/after context.
9. Stable deep links for session, turn, and event.
10. New-data snapshot advancement without active-turn streaming or live map growth.
11. Fallback discovery page when current session ID is unavailable.
12. Safe rendering and adversarial payload tests.

Exit gate:

- Dashboard → session map → turn → event → exact evidence or explicit unavailable context works end to end;
- direct and descendant totals agree with the metric engine;
- exact, derived, and unavailable states are distinguishable; the demo does not introduce component estimates;
- exact payloads are read from source rather than copied into SQLite;
- missing source files preserve metrics and produce honest evidence failures;
- compaction shows its exact recorded event and surrounding usage without claiming reconstructed preserved/removed content;
- the UI never claims complete server-side context without exact evidence;
- no active or provisional turn appears, and applying a newer completed-turn snapshot preserves the route and selection when possible.

### Phase 5 — Effectiveness Reviews

**Outcome:** A user can explicitly start a persisted Codex review task, receive its evidence-linked structured report, and reopen the originating task.

Deliver:

1. Reviews landing and history.
2. Shared single-session/time-period Review plan.
3. Fixed four-lens rubric and optional focus.
4. Scope, estimate, model, reasoning, prompt, and output preview.
5. The frozen versioned manifest, run, and report schemas.
6. Persisted `codex exec --json` launch, process monitoring, session-ID capture, review-session classification, and open/resume handoff.
7. Explicit invocation of the plugin Review skill, normal-tool review instructions, untrusted-evidence guidance, and scope manifest.
8. In-progress, complete, failed, and unrenderable states.
9. Partial-file-tolerant `review.json` discovery, first-valid acceptance, hash recording, and validation.
10. Up to five evidence-backed findings.
11. Context Inspector citation links and return-to-review state.
12. Copyable action prompts without automatic execution.
13. Review-created-session exclusion.
14. Synthetic valid, invalid, partial, overwritten, process-failure, and missing-citation report tests.

Exit gate:

- a review never runs inside the target session;
- Codex does not start before explicit user confirmation;
- the new task receives the fixed rubric, plugin Review skill, and frozen scope manifest;
- a completed report exists only when a schema-valid `review.json` has been accepted;
- the first valid report marks the run complete and its hash is recorded without claiming filesystem-enforced immutability;
- citations resolve into Context Inspector when their source exists;
- invalid or missing sources remain visible and diagnosable;
- Open Codex task opens the captured session when supported and a copyable resume fallback always exists;
- Inspector never executes a recommendation.

### Phase 6 — Demo hardening and clean-install verification

**Outcome:** The complete demo boundary works repeatedly from a clean macOS Codex environment and degrades honestly on unsupported or partial local data.

Deliver:

1. Clean marketplace-to-binary installation rehearsal in an isolated `CODEX_HOME` with the published checksum path.
2. Read-only manual validation against the real local supported-format corpus without copying payloads into the repository or diagnostics.
3. Full-history reverse-scan resource profiling and tuning against the Phase 0 budgets.
4. Crash/restart exercises for server metadata, indexing checkpoints, SQLite transactions, dataset-epoch rebuild, and Review processes.
5. Loopback, evidence-rendering, prompt-injection-guidance, and filesystem-path security tests.
6. Empty, partial, unsupported, stale-capacity, missing-source, malformed-report, and failed-review demo rehearsals.
7. Exact tested-version documentation, privacy disclosure, sensitive-data warning, known limitations, and demo runbook.
8. One end-to-end automated smoke path covering open, sync, metrics, session map, event evidence, Review launch, report rendering, citation return, and task handoff.

Exit gate:

- the demo completion boundary in Section 1.1 succeeds from a clean isolated environment twice consecutively;
- the index reaches the entire supported local inventory without a hidden cap or resource-budget violation;
- unsupported sources, coverage gaps, and missing evidence remain visible and do not corrupt supported totals;
- no raw payload appears in logs, doctor output, copied diagnostics, repository artifacts, or test snapshots;
- every required failure state has a rehearsed recovery or honest terminal explanation;
- all required tests and build commands pass from the documented clean checkout.

## 15. Worker-agent phase contract

Only one phase is handed to a worker at a time. A phase prompt must include:

- this architecture and every frozen contract produced by earlier phases;
- the phase outcome, deliverables, exit gate, owned directories, and explicit non-goals;
- exact build, test, lint, and smoke commands available at the start of the phase;
- the synthetic inputs and expected outputs relevant to that phase;
- performance and security budgets that the phase may not weaken;
- known pre-existing worktree changes that must be preserved.

A worker must:

1. inspect the existing implementation and contracts before editing;
2. implement only the active phase and necessary compatibility fixes within its boundaries;
3. update generated contracts through their documented generator rather than hand-editing both sides;
4. add the phase's unit, contract, security, and end-to-end tests;
5. run the exact exit-gate verification and report evidence for each criterion;
6. record any discovered source-format mismatch or required architecture change instead of guessing;
7. stop for an architecture decision when a frozen invariant cannot be met.

The coordinator accepts a phase only when every exit criterion is evidenced. A later phase must not be used to excuse an incomplete earlier vertical slice. Architecture changes discovered during implementation are made here first, with the affected contract and downstream phases updated in the same change.

## 16. Explicit deferred scope

The architecture preserves extension points for these items, but the initial implementation does not include them.

### Distribution and runtime

- Linux binaries, hook behavior, installation, and browser verification.
- Windows binaries, hook commands, installation, and browser behavior.
- macOS architectures other than the one explicitly tested demo architecture.
- macOS notarization, automatic updates, complete uninstall management, and shell-profile repair.
- Always-on daemon or scheduled indexing.
- Cloud sync, accounts, hosted storage, or team dashboards.
- Required MCP server.
- Public local HTTP API.
- Public terminal metric queries, session inspection, review execution, or exports.
- Hardening against malicious processes already running as the same OS user.

### Source adapters

- Rollout and session-index formats other than the explicitly tested current Codex contract.
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
- The complete Efficiency & Friction view; it is the first stretch phase after the demo.
- Workflow Profile and Outcomes & Reviews templates.
- Memory, automation, context, and parser-health templates.
- Graph exploration beyond the root-session causal map.

New plugin/CLI releases may add metrics. If they require new facts, the index schema upgrade builds and atomically swaps a new dataset epoch.

### Analysis and updates

- Standalone deterministic context-leakage analyzer.
- Standalone deterministic tool-thrashing analyzer.
- Deterministic user-facing findings outside Reviews.
- Event-level live indexing.
- Automatic application of new index revisions to visible widgets or Inspector state.

Neutral metric facts such as retries, failures, switching, compactions, context sources, and token concentration remain available to the review prompt without being pre-labeled as findings.

### Session evidence

- Deterministic context reduction and intermediate before/after context states.
- Component-level token estimates and cumulative context burden.
- Computed compaction preservation/removal analysis.
- Complete reconstruction of unrecorded server-side context.
- Inspector-added secret masking or redaction.
- Durable copies of raw rollout payloads.
- Cross-session comparison inside Context Inspector.

### Reviews

- Review inside the target Codex thread.
- Technically sandboxed or CLI-restricted review evidence access.
- `codex exec --output-schema` enforcement and a dedicated read-only review sandbox.
- Semantic verification that a citation supports a finding.
- Filesystem-enforced report immutability, atomic candidate promotion, and report revisions.
- Cancellation, retry-in-place, stale-task detection, and multi-process recovery.
- Multiple review profiles.
- User usefulness ratings.
- Developer levels, grades, or longitudinal scores.
- Scheduled or recurring reviews.
- Automatic report revision from continued conversation.
- Automatic application of recommendations.

## 17. References

These documents are design history and research inputs. Section 1.2 governs every conflict:

- [Dashboard metrics brainstorming](../brainstorming/codex-home-dashboard-metrics.md)
- [Context Inspector brainstorming](../brainstorming/context-inspector-session-map.md)
- [Effectiveness Reviews brainstorming](../brainstorming/codex-effectiveness-reviews.md)
- [Core wireframe requirements](../ui/inspector-core/requirements.md)
- [Core wireframe decisions](../ui/inspector-core/decisions.md)
- [Core wireframe state matrix](../ui/inspector-core/state-matrix.md)
- [Interactive wireframe](../../prototypes/inspector-core/index.html)

Current Codex extension references:

- [Build plugins](https://learn.chatgpt.com/docs/build-plugins)
- [Hooks](https://learn.chatgpt.com/docs/hooks)
- [Codex developer commands and `codex exec`](https://learn.chatgpt.com/docs/developer-commands)
- [ChatGPT desktop deep links](https://learn.chatgpt.com/docs/reference/commands#chats)
