# Codex home dashboard metrics brainstorming

- **Status:** Working document for dashboard product discussion
- **Date inspected:** July 15, 2026
- **Primary question:** What can Codex Inspector derive locally from a user's Codex home, and how should those aggregates shape a power-user dashboard?

## Executive summary

The session rollout logs are the canonical behavioral event stream. They can support substantially more than a recent-sessions table: activity and token time series, project and model breakdowns, tool reliability and transition analysis, context-overhead trends, compaction behavior, parent/child agent graphs, and evidence-backed drill-down to a session, turn, tool call, or context source.

The other artifacts complement the rollouts rather than replacing them:

- `history.jsonl` is a compact history of user-entered text keyed by session ID. It is useful for prompt-volume, prompt-length, cadence, and active-period analysis, but it does not contain the complete interaction.
- `memories_1.sqlite` tracks generated memory outputs, memory-pipeline jobs, selection for consolidation, and partial usage data. It enables memory generation, reuse, freshness, and pipeline-health views.
- `sqlite/codex-dev.db` is mostly application/catalog state: a recent local thread catalog, automations, automation runs, inbox state, and feature enablement. It is useful for recent navigation and automation analytics but is not an authoritative all-time session warehouse.
- `session_index.jsonl`, although not part of the original list, is a useful supplemental title/index stream keyed by thread ID.

The dashboard should therefore sit on a versioned normalized index built from rollout events, with the smaller artifacts joined in where coverage is proven. It should never calculate totals by simply adding every number found in every file.

The strongest first dashboard concept is:

1. global filters for time, project, model, surface, and session kind;
2. a small row of aggregate totals;
3. one flexible time-series chart with a metric selector and comparison period;
4. breakdowns for projects, models, tools, and agent delegation;
5. direct drill-down to the contributing sessions and context evidence.

## Inspection method and privacy boundary

This pass inspected schemas, file metadata, field names, categorical distributions, lengths, timestamps, and cross-artifact identifier coverage. It did **not** quote or copy prompt text, assistant messages, memory contents, thread titles, repository URLs, tool arguments, tool outputs, or credentials.

The rollout corpus is about 10.1 GB, so this was not a full behavioral re-index. The inspection used:

- an exact inventory of rollout files and sizes;
- the first `session_meta` record from every rollout file;
- exact aggregate queries over the named SQLite databases and small JSONL files;
- stratified event-schema samples across January–July 2026;
- one current-format rollout to inspect newer timing, token, turn, and world-state fields.

All observed counts below are a point-in-time snapshot of a live directory. SQLite WAL files and active session logs continued changing during inspection.

## Observed artifact inventory

| Artifact | Observed footprint | Safe structural observations | Dashboard role |
| --- | ---: | --- | --- |
| `sessions/**/*.jsonl` | 3,117 files, about 9.3 GB | Active rollout event logs partitioned by date | Primary behavioral source |
| `archived_sessions/*.jsonl` | 68 files, about 128 MB | Archived rollout logs using the same general event shape | Historical behavioral source |
| Combined rollout corpus | 3,185 files, 3,060 unique first-record session IDs, about 10.1 GB | January 5–July 15, 2026 by filename; duplicate IDs exist across files | Deduplicated source for sessions, turns, tools, timing, tokens, context, and relationships |
| `history.jsonl` | 1,551 records, about 736 KB | Every inspected row has `session_id`, `ts`, and `text`; 228 distinct session IDs | User-input cadence and prompt-shape supplement |
| `memories_1.sqlite` | About 4 MB plus live WAL | 285 memory-stage outputs and 453 jobs at inspection time | Memory generation, reuse, freshness, and pipeline health |
| `sqlite/codex-dev.db` | About 124 KB, schema user version 23 | 60 catalog threads, 7 automations, 17 automation runs | Recent catalog and automation supplement |
| `session_index.jsonl` | 2,640 records, about 342 KB | `id`, `thread_name`, `updated_at`; 2,565 unique IDs | Labels and latest-known thread activity |

There is also an older `sqlite/memories_1.sqlite` snapshot. The newer root-level `memories_1.sqlite` and its WAL were active during inspection. An indexer must identify the authoritative current database and read it with SQLite/WAL semantics; copying only the main database file can silently miss committed data still represented by the WAL.

## Artifact deep dive

### 1. `history.jsonl`

#### Recorded shape

Every inspected row had exactly:

```text
session_id: string
ts: number (Unix seconds)
text: string
```

Observed coverage:

- 1,551 history entries across 228 session IDs.
- January 14–July 13, 2026 in UTC.
- 84 distinct active UTC dates.
- Median 3 entries per represented session; p90 17; maximum 116.
- Median entry length 102 characters; p90 609; p99 7,347; maximum 24,315.
- 100 entries were longer than 1,000 characters and 23 exceeded 5,000.

These statistics demonstrate useful distributions, not recommended dashboard defaults.

#### Direct metrics

| Metric | Formula | Fidelity | Useful view |
| --- | --- | --- | --- |
| User entries | Count rows | Exact for retained history | Daily/weekly line |
| Sessions represented | Distinct `session_id` | Exact for this file, not all Codex sessions | KPI and coverage note |
| Entries per session | Rows grouped by `session_id` | Exact | Distribution/histogram |
| Prompt length | `length(text)` in characters or a local token estimate | Characters exact; tokens estimated | Histogram and trend |
| Active days | Distinct localized calendar dates | Exact after timezone choice | Calendar heatmap |
| Time-of-day pattern | Localized hour of `ts` | Exact after timezone choice | Hour × weekday heatmap |
| Prompt cadence | Time delta between adjacent entries in a session | Reconstructed | Distribution or session timeline |
| Follow-up density | Entries after the first per represented session | Reconstructed proxy | Trend and project comparison |
| Long-input frequency | Share above explicit length thresholds | Exact in characters | Trend with threshold control |

#### Important limits

- This is input history, not a complete transcript. It cannot supply assistant-message count, tool activity, task success, or actual context size.
- File retention and reset behavior are not self-describing. Month-to-month volume should not be interpreted as a change in user behavior until completeness is validated.
- History text is highly sensitive. Aggregate length and timing should be indexed by default; semantic clustering or prompt categorization should be explicit and local, with clear provenance.
- Only 217 of the 228 history session IDs appeared in the rollout first-record index at inspection time.
- Only 2 history IDs appeared in `session_index.jsonl`, even though each source separately overlaps the rollout corpus. This is a warning not to assume that every Codex surface uses or retains IDs in the same way.

### 2. `memories_1.sqlite`

#### Recorded schema

`stage1_outputs` contains:

- `thread_id` primary key;
- source and generation timestamps;
- raw generated memory and rollout summary text;
- rollout slug;
- `usage_count` and `last_usage` when available;
- phase-two selection flags and selection watermark.

`jobs` contains:

- job kind and key;
- status, worker ownership, start/finish/lease/retry timestamps;
- retry budget and error presence;
- input and success watermarks.

Observed snapshot:

- 285 generated memory rows.
- 256 selected for phase two.
- Usage count and last usage populated for 157 rows.
- Mean usage count among populated rows: 4.83; maximum: 53.
- Source updates span May 31–July 15, 2026 UTC.
- Memory generation spans May 31–July 15, 2026 UTC.
- 402 completed jobs, 50 error jobs, and 1 pending job during the final query.
- The generated-memory tables changed during this inspection, confirming that the database is live.

#### Direct and reconstructed metrics

| Metric | Formula | Fidelity | Useful view |
| --- | --- | --- | --- |
| Memories generated | Count `stage1_outputs` by `generated_at` | Exact | Daily/weekly bars |
| Generation coverage | Memory rows ÷ eligible completed threads | Reconstructed until eligibility is defined | Trend and coverage note |
| Phase-two selection rate | Selected rows ÷ generated rows | Exact for recorded pipeline state | Funnel |
| Memory reuse | Sum/distribution of `usage_count` | Exact where populated; incomplete overall | Histogram and top reusable memories without showing content |
| Time to first reuse | `last_usage` is insufficient by itself | Unavailable without citation events | Explicitly unavailable |
| Memory freshness | Now minus `generated_at` or `source_updated_at` | Exact | Age distribution |
| Regeneration latency | `generated_at - source_updated_at` | Reconstructed pipeline latency | Percentile trend |
| Stale memory candidates | Old memories with no recorded recent use | Reconstructed and coverage-dependent | Review list |
| Job success/error rate | Jobs by kind and status | Exact | Pipeline health trend |
| Retry exhaustion | Jobs with zero retries and error status | Exact | Alert/detail |
| Memory size | Character length of generated fields | Exact in characters; token size estimated | Distribution |

#### Graph opportunity

Newer rollout events can carry `memory_citation` objects with `entries` and `rolloutIds`. Combined with `stage1_outputs.thread_id`, this can support a directed memory-use graph:

```text
source session → generated memory → citing session/turn → affected response
```

That graph could answer:

- Which memories are repeatedly useful?
- Which source sessions produce broadly reused guidance?
- Which projects consume memories produced elsewhere?
- Are old or cross-project memories associated with later context leakage?

The last question must be phrased as an association until a deterministic analyzer can establish a stronger relationship.

### 3. `sqlite/codex-dev.db`

#### Recorded schema

The database contains:

- `local_thread_catalog`, hosts, metadata, and sync state;
- `automations` and `automation_runs`;
- `inbox_items`;
- local app-server feature enablement.

The local thread catalog records:

- host and thread IDs;
- display title;
- created and updated timestamps;
- cwd, source kind/detail, provider, and optional branch;
- observation sequence and missing-candidate state.

Observed snapshot:

- 60 catalog threads, all from a local VS Code/OpenAI source in this database.
- 50 distinct cwd values, but branch metadata present for only 1 row.
- Catalog dates span June 25–July 15, 2026 UTC.
- Sync state reported `initial_build_complete = 0`, so the catalog should not be treated as exhaustive.
- 7 automations: 1 active and 6 paused.
- 17 automation runs: 15 archived and 2 pending review.
- Automation configuration can include model, reasoning effort, target type, project, recurrence, cwd scope, and next/last run timestamps.

#### Useful metrics

| Area | Metrics | Fidelity and caveats |
| --- | --- | --- |
| Recent thread catalog | Recent thread count, cwd coverage, source kind, missing-candidate count, update recency | Exact for the catalog; currently incomplete |
| Automation inventory | Active/paused count, recurrence types, project-scoped vs projectless, configured model/effort | Exact |
| Automation execution | Runs over time, status mix, read/review rate, run update span | Exact fields, but status semantics need adapter tests |
| Automation attention | Pending-review count and age | Exact |
| Inbox | Read/unread and thread linkage | Exact; empty in this snapshot |

The catalog is valuable for labels, discovery, and recent-state reconciliation. It is too small and explicitly incomplete to be the main source for all-time usage totals.

### 4. Session rollout logs

#### File and session identity

Every one of the 3,185 inspected files began with a `session_meta` record. Those first records represented 3,060 unique session IDs, so a deduplication policy is required.

First-record metadata was consistently available for:

- session/thread ID;
- timestamp;
- cwd;
- originator and source;
- CLI version;
- model provider;
- recorded base instructions.

Additional coverage in the first records:

- Git metadata: 3,051 files.
- Thread-source metadata: 2,144 files.
- Dynamic tool definitions: 749 files.
- Context-window metadata: 37 files.
- 511 distinct cwd strings and 12 distinct recorded repository URLs.
- 68 distinct CLI versions, demonstrating substantial format-version variance.

Source classification is essential. The files were not 3,185 equivalent human-started sessions:

- 2,071 were structured spawned-subagent sources.
- 751 were VS Code sources.
- 231 were CLI sources.
- 118 were non-interactive exec sources.
- 12 were review subagents.
- 2 were memory-consolidation subagents.

The dashboard must let the user include, exclude, or break out spawned work. Otherwise totals such as session count become misleading.

#### Recorded event families

Across sampled formats, rollouts used top-level records such as:

- `session_meta`;
- `turn_context`;
- `world_state`;
- `event_msg`;
- `response_item`;
- `compacted`.

Sampled payload types included:

- user, assistant, and reasoning messages;
- task start, completion, and abort events;
- cumulative token counts;
- function calls and paired outputs;
- custom tool calls;
- command, patch, MCP, web-search, and tool-search completion events;
- context compaction;
- thread-name changes.

Current-format task completion records can include:

- `started_at` and `completed_at`;
- `duration_ms`;
- `time_to_first_token_ms`;
- turn ID;
- model context window;
- collaboration-mode kind.

Current-format token records expose:

- input tokens;
- cached input tokens;
- output tokens;
- reasoning output tokens;
- total tokens;
- model context window;
- last-turn usage and cumulative total usage.

Tool completion records can expose:

- call ID and turn ID;
- command/tool identity;
- status, success, exit code, or result;
- duration;
- cwd;
- structured invocation or parsed-command metadata;
- patch-change metadata.

The raw arguments and outputs are privacy-sensitive and must be redacted before display or analysis.

#### Session and turn metrics

| Metric | Formula | Fidelity | Notes |
| --- | --- | --- | --- |
| Sessions | Deduplicated logical session IDs | Exact after deduplication | Break out root, exec, review, memory, and spawned sessions |
| Turns | Distinct turn IDs or task-start events | Exact where recorded; reconstructed on older formats | Version-aware adapter required |
| User/assistant messages | Response/event rows by role/type | Exact where recorded | Avoid double-counting mirrored event forms |
| Session span | Last event timestamp minus first | Reconstructed | Includes inactive gaps |
| Active time | Sum turn durations, or gap-capped event intervals | Exact on newer turns; reconstructed otherwise | Show method |
| Time to first token | Recorded completion field | Exact where present | Coverage badge required |
| Aborted-turn rate | Aborted turns ÷ started turns | Exact where events exist | Break down abort reason category without exposing text |
| Compactions | Count paired compaction events per session/turn | Exact after deduplicating paired representations | Useful context-pressure signal |
| Long-session distribution | Sessions by turns, time, tools, or tokens | Mixed | Each axis needs its own fidelity |

#### Token and context metrics

| Metric | Formula | Fidelity | Notes |
| --- | --- | --- | --- |
| Total recorded tokens | Final cumulative total per logical session, or sum non-cumulative deltas | Exact | Never sum every cumulative snapshot |
| Input/output/reasoning mix | Final token totals by category | Exact where present | Stacked time series |
| Cache-hit ratio | Cached input ÷ input tokens | Exact where present | Define denominator carefully |
| Tokens per turn/session | Recorded totals ÷ normalized counts | Exact where both inputs are exact | Distribution plus trend |
| Context utilization | Last-turn input ÷ model context window | Exact where context window and last usage exist | Coverage is currently version-sensitive |
| Compaction threshold behavior | Utilization before compaction | Reconstructed from nearby events | Useful diagnostic |
| Base-instruction size | Recorded characters or locally tokenized text | Characters exact; tokens estimated | Potential context-overhead trend |
| Dynamic-tool-definition size | Serialized definition characters or locally tokenized schema | Characters exact; tokens estimated | Only present in 749 first records |
| Context composition | Messages, base instructions, dynamic tools, skills, runtime components | Mixed exact/reconstructed/estimated/unavailable | Powers the context explorer |

Observed first-record size distributions make context overhead worth surfacing:

- Recorded base instructions had a median length of 21,335 characters and p90 of 21,335 across this corpus. The near-constant value suggests a large common instruction block in many sessions.
- Among the 749 files with dynamic tool definitions, the median was 3 definitions, p90 was 12, and maximum was 14.
- Serialized dynamic-tool definitions had a median length of about 5,148 characters and p90 of about 18,553.

These are character measurements. Token contribution requires a named local tokenizer or a labeled approximation.

#### Tool metrics

| Metric | Formula | Fidelity | Useful view |
| --- | --- | --- | --- |
| Tool calls | Normalized call records | Exact | Trend and breakdown |
| Tool success/failure | Paired completion status, success flag, or exit code | Exact where normalized | Stacked trend |
| Tool latency | Completion duration or paired timestamps | Exact/reconstructed | Percentiles by tool |
| Retries | Equivalent normalized calls repeated within a turn/window | Deterministic reconstruction | Trend and drill-down |
| Tool switching | Transitions between normalized tool families | Exact sequence; interpretation deterministic | Sankey/transition graph |
| CLI ↔ MCP alternation | Alternating interfaces for the same service | Deterministic analyzer | Finding and trend |
| Patch success | Successful patch completions ÷ attempts | Exact where event exists | Trend |
| Shell error rate | Nonzero exit completions ÷ command completions | Exact | Trend and project comparison |
| Web/tool-search usage | Calls and outcomes by search surface | Exact | Breakdown |
| Tool-definition overhead | Available-definition size versus tools actually invoked | Mixed exact/estimated | Efficiency scatter plot |

Tool names can be normalized into families such as filesystem, shell, Git, GitHub, browser, search, MCP service, and collaboration. Keep the original tool identity for drill-down, but use families for readable aggregate views.

#### Agent and collaboration metrics

Structured sources and thread metadata make agent topology a first-class opportunity:

| Metric | Formula | Fidelity |
| --- | --- | --- |
| Root vs spawned sessions | Source classification | Exact |
| Spawn count | Child thread edges per parent | Exact where parent ID recorded |
| Delegation fan-out | Distinct children per root/turn | Exact |
| Maximum delegation depth | Longest parent-child path | Exact where the tree is complete |
| Delegated token share | Child tokens ÷ root-plus-child tokens | Exact where token coverage exists |
| Delegated time share | Child active time ÷ total active time | Mixed exact/reconstructed |
| Parallelism | Overlap of child active intervals | Reconstructed |
| Role mix | Child role categories | Exact where recorded |
| Review-agent usage | Review children per root/project/time period | Exact |
| Orphaned children | Child references without a locally indexed parent | Exact integrity metric |

This should not be framed as “more agents is better.” It is a way for power users to understand where work and tokens went.

## Cross-artifact join map

| Canonical concept | Rollout logs | `history.jsonl` | `memories_1.sqlite` | `codex-dev.db` | `session_index.jsonl` |
| --- | --- | --- | --- | --- | --- |
| Session/thread ID | `session_meta.session_id` or `id` | `session_id` | `stage1_outputs.thread_id` | `local_thread_catalog.thread_id`, run `thread_id` | `id` |
| Turn ID | Turn context and task/tool events | — | — | — | — |
| Tool call ID | Call and completion payloads | — | — | — | — |
| Time | Record timestamp and task timing | `ts` | source/generated/usage/job times | catalog and automation times | `updated_at` |
| Project | cwd and Git metadata | Join through session ID | Join through thread ID | cwd, project ID, source cwd | Join through ID |
| Parent/child edge | structured source/thread source/fork metadata | — | citation rollout IDs | indirect via catalog | — |
| Display label | Thread-name update events | prompt text should not be a default label | rollout slug can supplement | display title | `thread_name` |

Observed identifier intersections:

- History ↔ rollouts: 217 IDs.
- Session index ↔ rollouts: 2,477 IDs.
- History ↔ session index: 2 IDs.
- Codex-dev catalog ↔ rollouts: 56 of 60 catalog IDs.
- Codex-dev catalog ↔ session index: 56 of 60.
- Memory outputs ↔ rollouts: 285 IDs at the final query.
- Memory outputs ↔ session index: 196 IDs.

These intersections support joins, but also demonstrate that none of the smaller artifacts is a complete registry. Inspector should report source coverage and unresolved joins rather than silently dropping unmatched records.

## Proposed normalized analytics model

The dashboard and context explorer should share one append-friendly local model.

### Dimensions

```text
dim_session
  logical_session_id, source_kind, originator, root_or_child,
  parent_session_id, cwd_id, project_id, repo_id, git_branch,
  cli_version, model_provider, started_at, ended_at, parser_version

dim_project
  project_id, normalized_repo_identity, display_name, local_paths,
  first_seen_at, last_seen_at

dim_tool
  tool_id, raw_name, normalized_family, interface_kind, service

dim_model_configuration
  model, reasoning_effort, personality, collaboration_mode,
  approval_policy, sandbox_profile, context_window
```

### Facts

```text
fact_turn
  session_id, turn_id, started_at, completed_at, duration_ms,
  time_to_first_token_ms, outcome, abort_reason_category

fact_message
  session_id, turn_id, timestamp, role, content_length,
  content_locator, redaction_state

fact_tool_call
  session_id, turn_id, call_id, tool_id, started_at, ended_at,
  duration_ms, success, error_category, input_locator, output_locator

fact_token_snapshot
  session_id, turn_id, timestamp, is_cumulative,
  input, cached_input, output, reasoning_output, total, context_window

fact_context_component
  session_id, turn_id, component_kind, source_locator,
  recorded_chars, estimated_tokens, fidelity, comp_hash

fact_memory
  source_thread_id, generated_at, selected_phase2,
  usage_count, last_usage, content_locator

fact_memory_citation
  memory_source_thread_id, citing_session_id, citing_turn_id, timestamp

fact_automation_run
  automation_id, thread_id, project_id, created_at, updated_at,
  status, read_at
```

### Edges

```text
edge_thread_relationship(parent_session_id, child_session_id, edge_kind, depth)
edge_tool_transition(session_id, turn_id, from_tool_id, to_tool_id, elapsed_ms)
edge_context_source(project_id, source_project_id, session_id, turn_id, fidelity)
```

Every normalized record should retain a stable local source locator, source file/database version, parser version, and fidelity. Raw sensitive content should remain on demand rather than being copied into every analytics table.

## Metric catalog for a power-user dashboard

### A. Activity and usage

- Sessions over time, separated into root interactive, exec, automation, review, memory, and spawned-agent work.
- Active days and calendar heatmap.
- Turns, user entries, assistant responses, and follow-up density.
- Active time versus wall-clock session span.
- Median/p90 session duration and turns per session.
- Time-to-first-token percentiles.
- Aborted-turn frequency and reason category.
- Usage by hour of day and weekday in the user's chosen timezone.
- Consecutive active-day streaks as descriptive behavior, not gamification.
- Project switching within a day or work block.

### B. Tokens and context

- Input, cached input, output, and reasoning tokens over time.
- Tokens per session, turn, project, model, and source surface.
- Cache ratio and cache savings expressed in tokens, not assumed dollars.
- Context-window utilization distribution.
- Sessions approaching the recorded context window.
- Compactions per session and token level before compaction.
- Base-instruction and dynamic-tool-definition overhead over time.
- Estimated context composition by messages, instructions, tools, skills, and unavailable components.
- Model/reasoning mix and how token/output distributions differ by configuration.

Cost should remain optional unless Inspector ships a versioned, date-aware pricing catalog. Pricing is not recorded in these artifacts and changes independently.

### C. Tools and execution

- Calls, failures, and latency by tool and normalized tool family.
- Top tools by project and time period.
- Tool success and nonzero-exit trends.
- Retry-loop rate and repeated equivalent calls.
- CLI/MCP/browser transitions and overlapping-interface switching.
- Patch attempt/success rate.
- Search usage and result-producing versus repeated searches.
- Available tools versus used tools.
- Tool-definition context overhead versus actual invocation frequency.
- Long-running tools and time spent waiting on tools.

### D. Projects and environments

- Sessions, active time, tokens, turns, and tools by normalized repository.
- Branch and commit coverage where recorded.
- Worktree/cwd proliferation collapsed under a repository identity.
- Interactive versus automation versus spawned-agent mix by project.
- CLI/Codex version mix and version-change overlays on time series.
- Source surface: Desktop/VS Code, CLI/TUI, exec, review, memory, automation.
- Approval, sandbox, collaboration, personality, model, and effort mix where recorded.

### E. Agent delegation

- Root sessions versus spawned child sessions.
- Parent-child session tree and fan-out distribution.
- Token and active-time share delegated to child agents.
- Maximum depth, concurrency, and child-role mix.
- Review-agent usage and review-to-change loops.
- Tool specialization by agent role.
- Orphaned or incomplete thread relationships as a parser-health metric.

### F. Memory

- Memories generated and selected over time.
- Pipeline completion/error/retry rate.
- Generation delay after source-session updates.
- Recorded reuse count and last-use recency.
- Memory citations by project and session.
- Reuse concentration: broadly helpful memories versus one-off memories.
- Cross-project memory flow.
- Old unused memory candidates, labeled as a review suggestion rather than automatic deletion.

### G. Automations

- Active and paused automation count.
- Scheduled versus observed runs.
- Run status and pending-review age.
- Read/review rate.
- Project-scoped versus projectless automations.
- Model and effort mix.
- Automation-generated tokens and time after joining run thread IDs to rollouts.

### H. Data and parser health

- Indexed versus discovered rollout files.
- Unique logical sessions versus duplicate files.
- Unknown record types by CLI version.
- Parse failures and unsupported versions.
- Source coverage for tokens, timing, turn context, Git, dynamic tools, and parent IDs.
- Unresolved joins among history, memory, catalog, index, and rollout sources.
- Last scan/index time and incremental backlog.

This category belongs in a quiet trust/details surface, but power users will care about it because every aggregate depends on it.

## Time-series ideas

### Primary metric explorer

One chart can support multiple metrics rather than placing many mini charts on the dashboard:

- metric selector: active time, sessions, turns, tool calls, tokens, compactions, memory citations;
- interval selector: hour, day, week, month;
- stack/group selector: project, model, source kind, root/child, tool family;
- compare toggle: previous period or rolling baseline;
- brushing to filter the rest of the dashboard and session table;
- coverage/fidelity overlay when a field appears only in newer versions.

### Useful secondary temporal views

- Weekday × hour heatmap for active time or user entries.
- Stacked token composition over time.
- Tool failures and p95 latency over time.
- Context overhead and compactions over time.
- Root versus delegated work over time.
- Memory creation and citation over time.
- Event markers for CLI-version changes, parser changes, and configuration changes when deterministically observable.

Avoid implying causation when two trends move together. A chart can invite drill-down to the sessions behind a change.

## Graph ideas

### 1. Session delegation graph

```text
root session → spawned worker → nested worker/reviewer
```

Node size can represent tokens or active time; edge labels can represent spawn turn and relationship kind. Default to one selected root session or a filtered period—never render every thread as a hairball.

### 2. Project ↔ tool graph

A bipartite graph can show which projects rely on which tool families. Edge weight can be calls, active tool time, failures, or tokens near tool activity. This could reveal project-specific tool ecosystems and redundant interfaces.

### 3. Tool transition graph

Directed edges show what tool family followed another within a turn. Useful filters:

- successful versus failed transitions;
- within one project or across all projects;
- root versus child agents;
- only repeated or alternating sequences.

This is a natural aggregate view for tool thrashing.

### 4. Context source graph

```text
instruction/memory/tool-definition source → session project
```

Expected same-project edges can stay quiet. Cross-project edges can surface as reviewable anomalies with fidelity and evidence counts.

### 5. Memory citation graph

```text
source session → memory → citing sessions/projects
```

This shows useful reuse and potentially stale cross-project influence without exposing the memory text by default.

For all graphs, the product should offer a table alternative, keyboard-accessible selection, top-N limits, and direct session drill-down.

## Candidate dashboard information architecture

### Global filter bar

- Time range and interval.
- Project/repository.
- Model and reasoning effort.
- Source surface.
- Session kind: root interactive, exec, automation, review, memory, spawned agent.
- Include/exclude archived sessions.

### Aggregate summary row

Keep this to five or six descriptive totals:

- active time;
- root sessions;
- turns;
- recorded tokens;
- tool calls;
- active projects.

Each total should disclose whether spawned sessions are included and show coverage/fidelity on hover or drill-down. “Session count” without root/child semantics is not meaningful in this corpus.

### Main analytical surface

- Flexible time-series chart occupying the strongest visual position.
- Compare-period delta as supporting text, not a health score.
- Click/brush a period to constrain all lower sections.

### Breakdowns

- Project/model/source distribution.
- Token mix.
- Tool calls, failures, and latency.
- Root versus delegated work.

Start as ranked tables or compact bars. Graph mode can be an explicit analytical view rather than a decorative dashboard element.

### Session evidence table

The bottom of the dashboard should always resolve aggregates back to sessions:

- project and task label;
- root/child relationship;
- source surface;
- start time and active duration;
- turns, tools, and tokens;
- compactions or deterministic findings;
- coverage flags;
- direct link to context inspector.

## Recommended metric tiers

### Tier 1: deterministic and broadly useful

- Deduplicated root sessions and spawned sessions.
- Active time, turns, and tool calls.
- Recorded token totals and category mix.
- Project, model, source surface, and CLI-version breakdowns.
- Tool success/failure and latency.
- Compactions and aborted turns.
- Base-instruction and dynamic-tool-definition size.
- Parent-child thread topology.
- Data/parser coverage.

### Tier 2: power-user derived metrics

- Active-day and time-of-day patterns.
- Context utilization.
- Retry and tool-switching patterns.
- Tool-definition overhead versus usage.
- Delegated token/time share and concurrency.
- Memory generation, reuse, and citation.
- Automation execution and review behavior.

### Tier 3: opt-in interpretation

- Prompt or task-topic clustering.
- Workflow archetypes.
- Semantic duplicate prompts.
- Suggested skills or automations.
- Correlation between configuration/context patterns and outcomes.
- Model-powered coaching.

Tier 3 should require explicit scope, local evidence selection, and user approval as described in the Build Week plan.

## Correctness traps to design around

1. **Duplicate rollout files.** File count is not logical session count.
2. **Root versus child sessions.** Spawned agents dominate the file inventory and can overwhelm totals.
3. **Cumulative token snapshots.** Summing snapshots greatly overcounts usage; use deltas or the final valid cumulative total.
4. **Mirrored events.** A tool call may have response-item and event representations. Normalize by call ID and semantic phase.
5. **Paired compaction events.** Deduplicate top-level and event-message representations.
6. **Schema evolution.** Sixty-eight CLI versions and multiple metadata shapes were present.
7. **Mixed timestamp units.** History and memory use Unix seconds; automation tables use milliseconds; rollout events use timestamp strings and duration fields.
8. **Live WAL state.** SQLite databases can change during indexing and must be read consistently.
9. **Incomplete catalogs.** `codex-dev.db` explicitly reported an incomplete initial catalog build.
10. **Inconsistent retention.** History, session index, catalog, memory, active rollouts, and archived rollouts cover different subsets and dates.
11. **Cwd is not project identity.** Worktrees, subdirectories, and generated working directories can create hundreds of cwd strings for a small set of repositories.
12. **Wall time is not active time.** Long idle gaps require a documented reconstruction method.
13. **Content is sensitive.** Titles, prompts, memories, commands, tool payloads, outputs, paths, and repository URLs need on-demand access and redaction.
14. **Unavailable server context.** Local records cannot prove the complete server-side prompt. Show unavailable rather than inventing precision.

## Decisions from the Home Dashboard grilling session

- **Decision date:** July 15, 2026
- **Status:** Shared product direction for the next low-fidelity dashboard wireframe

### Dashboard purpose

The Home Dashboard should help a power user answer:

> When and how am I spending my available Codex capacity, and which sessions and working patterns explain that usage?

Resource utilization and outcome value are related but distinct:

- Inspector can deterministically measure tokens, time, turns, tools, retries, compactions, context overhead, and delegation.
- Inspector cannot reliably infer universal session value from resource use alone because valuable outcomes differ across implementation, research, planning, and exploration.
- Outcome value should combine explicit user feedback with evidence-backed deterministic and qualitative review signals.
- The product should not collapse these concepts into a single health, efficiency, or usefulness score.

### Root sessions and spawned-agent attribution

- A user-initiated root session is the primary unit of work.
- All descendant-agent sessions roll up into that root, including nested descendants.
- Additive metrics must preserve three values: combined total, root-session contribution, and descendant-agent contribution.
- Users should be able to view root and spawned work independently as filters or breakdown dimensions.
- Token totals must show how much usage came from the root versus its descendant tree.
- Session counts must not present rollout-file count as human-initiated work.

### Customizable analytics canvas

Home is a customizable widget dashboard built on the canonical normalized metric layer.

- Users can add, remove, configure, resize, and arrange widgets.
- Templates provide opinionated starting points and remain editable.
- Widgets query the indexed metric model, not raw rollout files directly.
- The product includes a front-and-center searchable Metrics Catalog.
- The catalog explains each metric's definition, formula, source artifacts, supported dimensions, coverage, and fidelity.
- Customization is bounded by tested metrics and visualization primitives rather than arbitrary SQL.

### Widget wizard

Adding a widget uses a guided sequence:

1. Choose the widget type.
2. Choose the primary metric.
3. Choose the session population and time scope.
4. Configure grouping, interval, normalization, or columns when supported.
5. Add filters and comparison behavior.
6. Review a live preview, name the widget, and save it.

Each widget type exposes only valid controls. A hero metric should remain simple; a pivot table can support multiple dimensions.

The shared deterministic calculation grammar is:

```text
measure + aggregation + time grain + group by + filters + normalization
```

Example:

```text
recorded token sum
grouped by root versus spawned session kind
per day
over the global relative time range
filtered to selected projects and models
```

### Initial widget vocabulary

- **Hero metric:** one aggregate with optional comparison and a compact root/spawned attribution.
- **Time series:** line, stacked, and 100%-stacked modes with adaptive time intervals.
- **Ranked breakdown:** categorical distributions such as project, model, reasoning level, tool family, or session kind.
- **Pivot table:** exact values across one or more supported dimensions.
- **Session table:** the contributing root sessions, their rolled-up descendant activity, and direct context-inspector drill-down.

Graphs remain a valid analytical direction, but the first dashboard wireframe does not yet decide whether graph exploration is a widget type or a separate Explore surface.

### Metric catalog organization

Metrics are grouped by the user question they answer:

1. Token usage.
2. Sessions and activity.
3. Agents and delegation.
4. Tools and execution.
5. Context and configuration.
6. Outcomes and reviews.
7. Memory and automations.
8. Data and parser health.

Project, model, reasoning level, root versus spawned, source surface, tool family, outcome label, and CLI version are primarily dimensions used to filter, group, or normalize measures.

### Dashboard templates

The first template set is:

- **Token & Capacity:** where usage went and how it relates to recorded plan limits.
- **Workflow Profile:** how the user works across root sessions, delegation, models, reasoning levels, projects, surfaces, and tools.
- **Efficiency & Friction:** time, TTFT, failures, latency, retries, switching, compactions, aborts, and anomalous sessions.
- **Outcomes & Reviews:** user-reported value and evidence-backed review signals; most useful after feedback history accumulates.

Memory, automation, context, or parser-health templates can be added without changing the widget contract.

### Default Token & Capacity template

The agreed first dashboard template contains:

1. Current recorded limit utilization and reset countdown.
2. Total recorded tokens with root and descendant-agent contributions.
3. Token usage over time stacked by root versus spawned work.
4. Token composition across input, cached input, output, and reasoning tokens.
5. Most token-intensive user-initiated root sessions, with descendant usage rolled up and separately attributed.
6. Capacity drawdown across recorded limit windows, preserving reset markers.
7. Global filters for project, model, reasoning level, and session kind.
8. Direct drill-down from aggregate values to contributing sessions and the context inspector.

Capacity is descriptive. Inspector should show observed utilization, remaining percentage, drawdown, reset timing, and data staleness without labeling behavior healthy, unhealthy, wasteful, or optimal.

Current-format rollout events can record:

- plan and limit identity;
- used percentage;
- window duration;
- reset timestamp;
- optional credits, secondary limits, individual limits, and reached-limit state.

The inspected current-format session recorded a 10,080-minute primary window, which is seven days. Credit and secondary-limit fields existed but were null, so those values must be capability- and coverage-gated rather than promised.

### Time controls

- The dashboard has a global relative-time selector inherited by its widgets.
- Quick ranges include 24 hours, 7 days, 14 days, and 30 days.
- Longer and custom ranges are supported.
- Chart interval adapts to the selected range unless the widget configuration requires a supported explicit interval.
- Capacity widgets retain reset boundaries inside the selected global period.
- Per-widget time-range override behavior remains undecided.

### Daily, weekly, and ad hoc outcome review

Outcome feedback should use a periodic retrospective rather than a prompt after every session.

- Reviews can run daily, weekly, or ad hoc based on user preference.
- The user enables recurring Codex review once rather than confirming every scheduled run.
- The rankable item is a user-initiated root session with activity in the review period.
- All descendant work rolls into that root.
- A root session started earlier remains eligible when it has meaningful activity during the current review period.
- Multi-day sessions are marked as long-running and show both today's contribution and the overall session purpose.
- Inspector should propose a small candidate set—roughly five sessions—rather than asking the user to rank every session.
- The user confirms or corrects lightweight labels such as “useful outcome” and “could have been better.”
- Unselected sessions remain available but require no response.

Inspector may use all evidence derivable from the session logs when estimating candidate usefulness, including:

- PR or main-branch promotion and other artifact signals;
- user-frustration or resolution signals;
- user-intervention count relative to agent runtime and turns;
- retries, aborts, failures, and anomalies;
- qualitative assessment of whether the final output addressed the stated goal.

Every proposed usefulness label should cite evidence and confidence. It remains a provisional estimate until the user confirms or corrects it.

### Review trust boundary

Inspector remains local-first for discovery, indexing, storage, scheduling, and saved results. Scheduled aggregate reviews invoke Codex after the user enables them. The product should keep review inputs scoped to relevant evidence for cost, relevance, and inspectability without adding repeated consent prompts that make the workflow tedious.

## Remaining product questions

1. Should project identity prefer repository URL, Git root, or normalized cwd when those sources disagree?
2. Do deterministic Findings/Insights share the analytics canvas or live in a dedicated area?
3. How much memory and automation analytics belongs in the first product?
4. Is graph exploration a widget primitive or a dedicated Explore surface?
5. Which coverage gaps appear directly on widgets versus in a data-health drawer?
6. Should individual widgets be allowed to override the global relative-time range?
7. What final outcome-label vocabulary balances usefulness with low feedback effort?
8. How should template discovery work for first-run users versus returning users?

## Next wireframe exercise

Revise the low-fidelity Home Dashboard around the agreed **Token & Capacity** template while keeping the customization model visible but shallow:

- show the global relative-time selector;
- render the agreed default widgets with realistic indexed values;
- preserve combined/root/spawned attribution;
- make the most token-intensive root sessions the primary drill-down;
- expose one lightweight “Add widget” entry into the wizard;
- defer full dashboard layout editing and advanced widget configuration until the analytical hierarchy is approved.
