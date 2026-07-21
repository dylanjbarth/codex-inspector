# Demo-critical feedback implementation plan

- **Status:** Approved product direction; ready for implementation delegation
- **Prepared:** July 19, 2026
- **Deadline:** Tuesday, July 21, 2026, afternoon
- **Target:** Existing PR #6 on `feat/mvp-architecture-phases`
- **Baseline:** `b919245` (`fix: unblock server startup on large histories`)

## Objective

Make the Codex Inspector demo trustworthy, understandable, and visually polished across the Dashboard and Context Inspector. Correctness and source clarity take priority over visual polish. Reviews must use the new shared shell and continue to work, but deeper Reviews redesign is deferred until its workflow has been exercised.

The implementation must address all approved feedback without reintroducing deferred dashboard customization or speculative product scope.

## Approved decisions

1. Introduce a light shadcn application shell with a sidebar linking Dashboard, Context Inspector, and Reviews.
2. Fully polish Dashboard and Context Inspector. Move Reviews into the shared shell and smoke-test it, but do not substantially redesign it in this pass.
3. Use exactly one effective Codex home at a time. Honor an explicit `CODEX_HOME`; otherwise use `~/.codex`. Never silently aggregate multiple homes.
4. Display the effective Codex home and whether it came from the environment or default resolution.
5. Derive capacity from the newest supported historical rollout observation rather than invoking `/status`. The displayed semantics and reset time must agree with the source record.
6. Keep the explicit “New data available” snapshot-advance interaction. Do not auto-update the Dashboard.
7. Indexing must reach a stable idle/current state. Active-file tails must not make the UI appear to index forever. Show meaningful progress while a scan is actually running.
8. Perform a bounded compatibility spike before expanding source support. Amend the architecture and support contract for cohorts proven structurally compatible.
9. Use shadcn's Recharts integration for ordinary charts. Reserve D3 for the Context Inspector causal map if it is materially useful.
10. Present formatted source evidence first and retain exact JSON under an explicit “View original record” disclosure.
11. Remove the vague global sensitive-data warning. Use concise contextual copy where exact local evidence is displayed.
12. Update the existing PR with the completed work.

## Initial evidence and likely root causes

### Effective home and server reuse

The inspected environment currently resolves:

```text
CODEX_HOME=/Users/dylanbarth/Code/personal/.codex
default home also present=/Users/dylanbarth/.codex
CODEX_INSPECTOR_HOME=/Users/dylanbarth/.codex-inspector
```

The current `server.json` metadata does not record the Codex home served by the process. `codex-inspector open` reuses any healthy server found under the Inspector run directory without comparing its source home to the currently resolved `CODEX_HOME`.

The active dataset also does not visibly identify its source home. A reused process or dataset can therefore make the UI appear to read another home or show capacity from an unexpected source. Implementation must prove the exact failure mode and bind both process reuse and indexed data to one canonical source home.

### Indexing state

The server currently maps any database `Pending` source count to `catching_up`, even when the index job is no longer running. An active rollout with an incomplete tail can therefore leave the UI in a permanent indexing-like state.

The implementation must distinguish:

- an index job actively scanning;
- queued hook-triggered changes;
- a stable active source with an incomplete tail;
- a completed scan with unsupported or failed sources;
- a newer committed revision waiting for user acknowledgement.

### Capacity

A bounded structural read of the latest rollout in the currently effective home found a recent observation shaped as:

```text
used_percent: approximately 28
window_minutes: 10080
reset: late July 24 in the local timezone
```

The current active database similarly contains recent weekly observations around 27% used with a late-July-24 local reset. A dashboard showing a roughly 12-hour reset is therefore not using the latest observation from this effective home and applied dataset.

The implementation must verify whether this is caused by server reuse, home mixing, stale revision handling, latest-observation selection, or more than one issue. It must also test `used_percent`, optional `remaining_percent`, multiple limit windows, and reset conversion explicitly.

### Unsupported coverage

The active source inventory contained:

| State | Sources |
| --- | ---: |
| Current | 69 |
| Unsupported | 3,432 |
| Failed | 6 |

Most unsupported sources are rejected by the exact CLI-version gate rather than an unexplained parse failure. Leading cohorts included:

| Codex version | Sources | Current reason |
| --- | ---: | --- |
| `0.129.0-alpha.15` | 354 | `unsupported_codex_version` |
| `0.140.0-alpha.2` | 284 | `unsupported_codex_version` |
| `0.136.0-alpha.2` | 266 | `unsupported_codex_version` |
| `0.128.0-alpha.1` | 265 | `unsupported_codex_version` |
| `0.133.0-alpha.1` | 244 | `unsupported_codex_version` |
| `0.137.0-alpha.4` | 228 | `unsupported_codex_version` |
| `0.142.5` | 134 | `unsupported_codex_version` |
| `0.142.2` | 107 | `unsupported_codex_version` |
| `0.144.1` | 30 | `incompatible_turn_context` |
| `0.144.2` | 39 | `unsupported_codex_version` |

The spike must explain all unsupported and failed sources by bounded, payload-safe reason. It must not optimistically accept entire version ranges.

## Work package 1 — Amend architecture and contracts

Update the MVP architecture and affected frozen contracts before dependent implementation.

Required amendments:

- one canonical effective Codex home at a time;
- explicit environment-versus-default resolution;
- process and dataset home binding;
- behavior when the requested home differs from the running process or active dataset;
- finite indexing state and active-tail semantics;
- progress and revision-acknowledgement semantics;
- compatibility cohorts proven by structural fixtures rather than one exact CLI version;
- light shadcn application shell and sidebar;
- Recharts dashboard visualization standard;
- contextual exact-evidence disclosure.

Generated contracts must be regenerated through the documented generator. Do not hand-edit generated outputs.

### Exit gate

- Architecture and contracts describe every changed invariant.
- No implementation depends on an undocumented reinterpretation of the old contract.
- Generated Go and TypeScript contracts are current.

## Work package 2 — Source-home, indexing, revision, and capacity correctness

### Source-home binding

1. Resolve and canonicalize the effective home once.
2. Record a source-home identity in process metadata and the dataset epoch/catalog.
3. Compare the requested effective home before reusing a healthy server.
4. Never query an active dataset associated with another home.
5. Choose a safe and explicit home-change behavior: activate or build a home-bound dataset, or stop with actionable remediation. Silent mixing is forbidden.
6. Return the canonical home and resolution source in the status API.
7. Display it in the sidebar and diagnostic details.

### Finite indexing and progress

1. Make runtime indexing state depend on an active index job, not merely a pending source tail.
2. Represent incomplete active tails separately and non-alarmingly.
3. Publish progress with inventoried, completed, skipped, unsupported, failed, rebuild-required, and remaining counts.
4. Show determinate progress when total inventory is known and an honest indeterminate state before it is known.
5. Emit `revision.available` only for a genuinely newer committed revision.
6. After the user applies the newest revision, clear the control and suppress events at or below the applied revision.
7. Preserve filters, scroll, and route state during snapshot advancement.

### Capacity correctness

1. Add sanitized fixtures matching current real `rate_limits` shapes.
2. Test weekly and shorter windows, absent secondary windows, optional remaining percentage, and numeric reset timestamps.
3. Select the newest observation deterministically by recorded observation time and stable tie-breaker.
4. Do not invert used and remaining semantics.
5. Show all meaningful recorded limit windows when more than one is present.
6. Label the observation timestamp, window duration, used percentage, remaining percentage when derivable or recorded, and local reset time.
7. Preserve the rule that local token totals do not infer plan usage.

### Exit gate

- Changing `CODEX_HOME` cannot silently reuse another home's server or dataset.
- An idle index with an active incomplete tail reports stable/current, not perpetual indexing.
- Progress reaches completion for a finite corpus.
- Applying the latest revision clears “New data available.”
- A real-home read-only check agrees with the newest supported historical capacity record without invoking `/status`.

## Work package 3 — Unsupported-source compatibility spike

Produce a payload-safe report grouped by:

- CLI version;
- source kind;
- state and rejection reason;
- metadata and discriminator shape;
- turn lifecycle shape;
- usage and rate-limit shape;
- evidence-offset safety;
- lineage compatibility;
- source count and approximate recoverable coverage.

For each candidate cohort:

1. Sample structural shapes without retaining message or tool payload content.
2. Create synthetic or normalized fixtures that preserve the relevant structure.
3. Run adapter, golden-metric, evidence-offset, and adversarial tests.
4. Add support only when identity, completed-turn state, token accounting, evidence resolution, and lineage remain provable.
5. Amend the support matrix with the exact accepted fingerprint or cohort.
6. Retain explicit rejection reasons for everything else.

### Exit gate

- One report explains 100% of unsupported and failed inventory by reason.
- Dominant safely compatible cohorts are implemented when feasible within the deadline.
- No source is accepted solely because its version falls inside a broad range.
- Remaining limitations appear clearly in diagnostics.

## Work package 4 — Shared shadcn shell

Configure shadcn for the existing React 19 and Vite project using the official existing-project approach. Add Tailwind, path aliases, CSS variables, and only the components needed for this pass.

Required shared UI:

- responsive sidebar;
- Dashboard, Context Inspector, and Reviews navigation;
- active-route treatment;
- effective Codex-home summary;
- index status and progress;
- cards;
- alerts;
- badges;
- buttons and links;
- selects and inputs;
- progress indicators;
- tables;
- sheets or dialogs;
- collapsibles;
- tooltips;
- skeleton and empty states;
- accessible chart wrappers;
- consistent typography, spacing, focus, and color tokens.

Use a light application shell. A darker code/evidence treatment is allowed inside Context Inspector. Avoid gratuitously rewriting the causal-map engine; D3 belongs only there and only if it preserves or improves the required interactions.

### Exit gate

- All three routes use the shared shell and sidebar.
- Dashboard and Context Inspector use shadcn primitives consistently.
- Reviews remains functional in the new shell.
- The embedded production asset build succeeds.
- Desktop and narrow layouts remain usable with keyboard-visible focus.

## Work package 5 — Dashboard redesign

### Header and filters

- Clear page title and short purpose statement.
- Compact global filters using shadcn controls.
- Visible applied snapshot and effective Codex home.
- Index progress only while a job is active.
- “New data available” remains an explicit user action.

### Coverage

- Replace the large banner with a compact warning.
- Include observed/eligible counts and a concise explanation.
- Open a coverage-details sheet containing unsupported, failed, incomplete-tail, and rebuild-required groups with counts, versions, reasons, and next steps.
- Keep complete coverage visually quiet.

### Capacity

- Use clear cards for the latest recorded limit windows.
- Show used and remaining semantics without ambiguity.
- Show observation freshness and local reset time.
- Render capacity drawdown with Recharts, including reset boundaries and honest gaps.

### Token usage

- Render usage over time as a stacked chart split into user-root-direct, descendant, Inspector Review, and other/orphan contributions.
- Include an accessible legend, tooltip, and textual summary.
- Render token composition with mutually exclusive categories and no cached/reasoning double counting.

### Root sessions

- Present a table or structured list with friendly title, project, start date, total tokens, direct tokens, descendant tokens, and actions.
- Keep the entire primary session target easy to open.
- Retain Review effectiveness as a secondary action.
- Use the approved deterministic title fallback when `session_index.jsonl` has no title.

### Diagnostics

- Replace the current JSON-dump-first disclosure with understandable status.
- Include source home, process version, index state, progress, applied/latest revision, source-state counts, grouped failures, hook health, completed-turn watermark, database size, and actionable remediation.
- Retain copyable payload-safe doctor diagnostics as a secondary action.
- Remove the global “Sensitive-data notice” copy.

### Exit gate

- A user can identify the active home, current index state, coverage limits, capacity state, contribution split, and top named sessions from the initial Dashboard.
- Every chart has readable labels, tooltip/legend support, and a nonvisual summary.
- Synthetic golden values remain exact after presentation changes.

## Work package 6 — Context Inspector polish

### Discovery

- Render a friendly session title using `session_index.jsonl` when available and the approved fallback otherwise.
- Show project, date, token totals, completed turns, and match reason.
- Limit keyword discovery to human-authored user messages in root sessions, with exact session-ID lookup as the only metadata-search exception.
- Replace the complete result region with a pending state while a query is in flight; never highlight stale results with newly typed text.
- Safely highlight visible query substrings by splitting text nodes; do not use injected HTML.
- Make the entire result row one accessible link. Remove the separate “Open” button.

### Detail and evidence

- Preserve the causal map, minimap, zoom, fit, branch-collapse, focus, topology rail, ledger, and deep links.
- Format supported event types with purpose-built views:
  - user and assistant messages;
  - recorded reasoning summaries;
  - tool calls and results;
  - token and rate-limit observations;
  - spawn and return events;
  - lifecycle events;
  - compactions.
- Show role, type, timestamp, fidelity, source availability, and relevant identifiers near the formatted content.
- Put the exact inert record under “View original record.”
- Preserve bounded chunk loading, escaped unsafe bytes, missing-source behavior, and adversarial-payload safety.
- Replace the vague global warning with local copy such as: “Displays the original local Codex record.”

### Exit gate

- Named search results are understandable without opening them.
- Visible substring matches are highlighted safely.
- Clicking anywhere on a result opens the expected stable route.
- Source messages and tool evidence are readable by default.
- Exact source access and fidelity labels remain available.

## Work package 7 — Verification and PR update

Run and report exact commands, exit codes, test counts, and environmental limitations.

Minimum automated verification:

```sh
go test ./...
pnpm --dir web lint
pnpm --dir web typecheck
pnpm --dir web test
pnpm --dir web build
```

Also run the relevant documented contract generation, synthetic integration, security, and browser E2E commands.

Required targeted tests:

- default and environment-provided Codex-home resolution;
- server reuse with matching and mismatching homes;
- dataset home-binding and no cross-home aggregation;
- finite indexing with incomplete active tails;
- progress completion and failure reporting;
- revision notification acknowledgement;
- capacity used/remaining/reset semantics and multiple windows;
- structural compatibility cohorts;
- stacked contribution charts and accessible summaries;
- friendly session fallback labels;
- safe match highlighting;
- formatted evidence with raw fallback;
- missing and adversarial evidence;
- Reviews shell regression.

Required manual verification:

1. Run against the synthetic corpus and capture Dashboard and Context Inspector screenshots at desktop and narrow widths.
2. Run a read-only validation against the selected real Codex home.
3. Confirm displayed capacity against the newest supported historical record without invoking `/status` as the data source.
4. Confirm indexing becomes idle/current and progress stops.
5. Apply one newer revision and confirm the control clears without losing filters or route state.
6. Navigate Dashboard → named session → turn → formatted event → exact original record.
7. Smoke-test Reviews navigation, plan creation, and existing E2E behavior in the shared shell.
8. Check keyboard navigation, visible focus, chart summaries, contrast, zoom, and responsive reflow.

Update PR #6 only after the candidate is buildable and the required evidence passes. Do not push a release or tag as part of this plan unless separately requested.

## Delegation brief

Use one fast implementation worker to preserve context and move through the work packages sequentially. Do not allow work on a later package while a correctness dependency remains unresolved.

Recommended reviewable commits:

1. `fix: bind indexing and capacity to the active Codex home`
2. `feat: add the shared shadcn application shell`
3. `feat: redesign the Token and Capacity dashboard`
4. `feat: polish Context Inspector discovery and evidence`

The worker must:

- inspect existing implementation before editing;
- preserve unrelated worktree changes;
- update architecture/contracts before dependent behavior;
- use documented generators;
- keep raw user payloads, paths, and secrets out of fixtures and logs;
- report commands and exit codes;
- leave each review candidate buildable;
- avoid substantial Reviews redesign;
- avoid dashboard customization, user-created views, cloud sync, or other deferred scope.

An independent reviewer should inspect each candidate milestone, with particular attention to home isolation, capacity correctness, indexing lifecycle, source compatibility, evidence safety, accessible charts, and regression risk.

## Schedule and stop conditions

Expected focused effort: approximately 16–22 hours.

Suggested order:

- **First block:** architecture, home binding, indexing, capacity diagnosis and fixes;
- **Second block:** compatibility spike and diagnostics API;
- **Third block:** shadcn shell and Dashboard;
- **Fourth block:** Context Inspector and full verification;
- **Tuesday buffer:** real-home validation, review fixes, PR evidence.

Stop and report a precise blocker rather than consuming the deadline when:

- the requested home cannot be isolated without destructive migration;
- historical capacity semantics cannot be proven from retained records;
- a compatibility cohort cannot preserve identities, completion state, or evidence offsets;
- generated contracts cannot be updated consistently;
- browser E2E or real-home validation is unreliable in the available environment;
- a required change would weaken exact-evidence or security invariants.
