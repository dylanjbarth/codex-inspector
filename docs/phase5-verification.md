# Phase 5 verification candidate

Phase 5 implements the Effectiveness Reviews vertical slice through the normal
local server and browser contracts. The candidate remains uncommitted pending
independent review.

Implemented behavior:

- single-root and bounded time-period planning pinned to one dataset epoch and
  revision, excluding Inspector Review sessions; time periods optionally use a
  revision-pinned canonical project choice from the generated metric catalog;
- complete frozen manifest previews, four fixed lenses, optional focus, model
  and reasoning selection, source/turn/project/byte estimates, coverage gaps,
  exact launch prompt, output destination, and explicit model-boundary consent;
- revision-pinned rollout-source inventory coverage that distinguishes complete,
  discovered, indexing, unsupported, failed, missing, and requires-rebuild
  states without claiming unsupported or unavailable data was sampled;
- no launch until a separate `confirmed: true` request;
- isolated persisted `codex exec --json` launch, first-event session capture,
  payload-free bounded lifecycle diagnostics, report monitoring, and
  `review.changed` notifications;
- strict canonical manifest/run/report schema validation, contextual review and
  citation identity checks, 1 MiB/five-finding bounds, partial-file retry,
  first-valid acceptance into a separate Inspector-owned Reviews SQLite store,
  and accepted-content hashing in `run.json`;
- Reviews history plus planned, in-progress, complete, failed, and unrenderable
  displays; live citation availability, Context Inspector return links,
  desktop task link, copyable resume command, and copy-only action prompts;
- product Review skill invocation with normal-tool scope, untrusted-evidence,
  inspected-project protection, and no-recommendation-execution instructions.

## Deterministic verification

Run:

```sh
scripts/phase5/verify.sh
```

2026-07-19 candidate result: exit 0.

- frozen fact, OpenAPI/TypeScript, and schema generators were current;
- `gofmt`, `go vet ./...`, `go test -count=1 ./...`, and `go build ./...`
  passed;
- race tests passed for Reviews, evidence, Context Inspector, and the loopback
  server;
- TypeScript checking and ESLint passed;
- all 18 UI tests passed;
- the embedded Vite production build passed (33 modules, 236.02 kB JS and
  11.99 kB CSS before gzip);
- shellcheck passed for plugin hooks and Phase 0 through Phase 5 scripts.

Focused Review tests cover valid, structurally invalid, partial, overwritten,
process-failure, absent-report, and manifest-missing-citation artifacts. They
also prove explicit confirmation, isolated review cwd, persisted thread event
capture, first-valid acceptance, later-overwrite rejection, accepted hash,
live missing-source citation state, copy-only handoff contracts, and Review-root
scope exclusion. Storage and planner tests prove latest-at-revision inventory
selection, exact complete state, and payload-free partial diagnostics for
discovered, indexing, unsupported, failed, and requires-rebuild sources without
leaking newer state into an older plan. The server tests validate concrete
single-session and canonical-project time-period plans against the frozen
OpenAPI schema, exact and partial pinned coverage, and unknown-field rejection
without reflecting a payload. UI tests cover history without launch,
plan/consent/confirmation, canonical project request and manifest preview,
visible partial coverage, complete report/citation/task handoff, copy-only
prompts, and unrenderable diagnostics.

## Local-corpus status

The payload-free command is:

```sh
scripts/phase5/validate-local-corpus.sh
```

2026-07-19 candidate result: exit 0.

```text
inventory=270 processed=270 skipped=0 failed=0 requires_rebuild=0 review_roots_indexed=0 review_roots_in_scope=0 scope_sessions=1 scope_turns=2 scope_sources=1 scope_evidence=50 included_bytes=200700 coverage_fidelity=exact coverage_observed=1 coverage_eligible=1 coverage_gaps=1 manifest_schema=valid
```

The disposable scan selected one bounded recent eligible root, built a complete
single-session plan, validated it against the frozen manifest schema, confirmed
that no Inspector Review root entered scope, emitted aggregate counts only, and
removed its disposable index. The selected single-root tree had one relevant
source and that source was complete at the pinned revision, so its inventory
coverage was exact; unrelated partial sources did not degrade this explicit-root
scope. Time-period plans conservatively use the whole pinned rollout inventory
because an unparsed source cannot be safely assigned to a project or time
boundary. This probe exposed and now guards a real planner
defect: a root with no descendants serialized `descendantSessionIds` as `null`
instead of the schema-required empty array. A focused regression test and the
real-corpus pass cover that case.

## External and browser proof status

The bounded synthetic external command is prepared as:

```sh
CODEX_INSPECTOR_RUN_LIVE_PROOFS=1 scripts/phase5/prove-review-launch.sh
```

It installs the local product plugin into an isolated authenticated Codex home,
indexes only fake fixtures, launches through the production Review manager,
requires a captured `thread.started`, accepts a valid report, resumes the same
persisted thread, invokes its desktop deep link, emits aggregate lifecycle
booleans only, and removes the isolated home. 2026-07-19 result: exit 0.

```text
review_status=complete thread_captured=true persisted_resume=true deep_link_invoked=true findings=4 citations_resolved=true report_hash_recorded=true
```

The isolated setup command exited 0. `codex --no-alt-screen` exited 0 after
**Trust all and continue**, with all seven hooks active. Then
`codex-inspector doctor --json` exited 0 and reported `hook_trust` as “all seven
current canonical Inspector hook hashes are trusted.” No real indexed evidence
was sent to the model and no credential, rollout payload, source path, prompt
body, report body, or session identifier entered the proof output.

A headed Playwright rehearsal then used the production binary and loopback
server with the same isolated synthetic environment.

- `codex-inspector open --no-browser --review`: exit 0; initial index 2/2 fake
  sources, two processed, zero failed; fragment removed at `/reviews/new`.
- Preview: two sessions, three completed turns, one project, 6,302 source bytes,
  1,575 estimated input tokens, and all four fixed lenses.
- Before confirmation: zero review directories. One confirmation then visibly
  moved from Running to Complete with process exit code 0, four findings, an
  accepted hash, and every citation Available.
- Citation handoff opened exact inert Context evidence and returned to the
  report. Reviews history showed Complete; the accepted report, Open Codex task,
  session ID, and resume-copy feedback remained available.
- Post-review sync: exit 0 at revision 5 with three sources, three supported,
  three processed, zero skipped, and zero failed. A new unconfirmed 30-day plan
  remained at two sessions and three turns, proving Review-root exclusion.

One malformed nonessential Playwright wait expression exited 1 and was replaced
by bounded CLI waits that exited 0. Initial unapproved loopback-bind attempts
failed with operation-not-permitted; the approved production server invocation
exited 0. Neither affected product behavior or acceptance evidence.

Playwright close and exact server-process termination both exited 0. `lsof +D`
found no remaining handles; disposable-root removal exited 0; final scans found
no Phase 5 temporary directories, repository Playwright state, reports, results,
screenshots, traces, fragments, payloads, or copied authentication.

After independent review requested canonical-project and pinned-inventory
coverage fixes, a second bounded headed production rehearsal covered only those
changed paths. All production and Playwright commands exited 0:

- the isolated doctor again proved all seven hooks trusted and the browser
  removed the secure fragment;
- the canonical project chooser listed `controls` and `widgets`; selecting
  `widgets` submitted its catalog-provided project ID and previewed one project,
  two sessions, and three turns with the descendant retained, compared with two
  projects, three sessions, and five turns for all projects;
- the complete synthetic inventory rendered `3 of 3 completely indexed ·
  Exact`;
- a single-session request contained exactly `kind` and `rootSessionId`, with no
  `projectId`;
- a pinned partial inventory rendered `3 of 8 · Derived`, explained that
  unavailable sources may contain eligible work and were not sampled, and
  displayed discovered, indexing/committed-only, unsupported, failed, and
  requires-rebuild diagnostics; the `widgets` scope remained two sessions and
  three turns;
- confirmation was left untouched, the review-directory count remained zero,
  and no model task or desktop deep link was started.

Two harness-only setup attempts failed before the passing rehearsal: an obsolete
database-inspection query exited 1, and an invalid synthetic CHECK-constrained
injection exited 19 and rolled back. Both were replaced with contract-valid
bounded setup; neither exposed a product defect. Cleanup completed with no
browser, server, temporary-home, report, screenshot, trace, fragment, payload,
or authentication artifact remaining.

The live report happened to omit the optional `actionPrompt`, so the live
browser could not click that optional copy button. The deterministic UI test
proves the action-prompt copy affordance and that Inspector does not execute it.
The live task did not produce an unrenderable artifact; deterministic manager,
server, and UI tests cover malformed, partial, missing, failed, and unrenderable
states. These are optional live branches, not unresolved Phase 5 requirements.

## Deliverable and exit-gate evidence matrix

| Architecture requirement | Acceptance evidence |
| --- | --- |
| D1 Reviews landing/history | Headed production rehearsal reopened the completed review from Reviews history; UI tests cover empty history. |
| D2 shared single-session/time-period plan | Manager/server/UI tests validate both frozen scopes and the optional revision-pinned canonical project selector; headed production rehearsals covered single-session, all-project time-period, and canonical-project time-period plans; real-corpus validation exercised a bounded root. |
| D3 fixed rubric and optional focus | Contract tests assert the four frozen lenses; the plan and manifest preserve optional focus. |
| D4 scope/estimate/model/reasoning/prompt/output preview | Headed rehearsals inspected production preview and consent before confirmation, canonical project identity/count changes, exact 3/3 coverage, and derived 3/8 diagnostics; server/UI tests validate the same contracts. |
| D5 frozen manifest/run/report schemas | Generators are current; schema tests and the synthetic external proof validate concrete artifacts. |
| D6 persisted launch/monitoring/session classification/handoffs | External proof captured `thread.started`, accepted completion, resumed the same thread, and opened its desktop deep link; browser proved task/resume controls; subsequent planning excluded the review root. |
| D7 plugin skill, ordinary tools, untrusted evidence, frozen scope | Launch prompt/skill contract tests plus the authenticated task's schema-valid evidence-linked report prove the installed skill and manifest reached the new task. |
| D8 lifecycle states | Browser observed running then complete; deterministic tests cover planned, failed, and unrenderable states. |
| D9 partial tolerance/first-valid/hash/validation | Manager tests cover partial and overwritten files, accept only the first valid report, and assert the persisted hash; live proof recorded a hash. |
| D10 up to five evidence-backed findings | Frozen schema enforces the limit; live report contained four findings with resolved citations. |
| D11 Context citation and return | Headed browser opened a cited event in Context Inspector and returned to the accepted report. |
| D12 copy-only action prompts | UI test proves copy feedback and no execution; the live report omitted this optional field. |
| D13 review-created-session exclusion | Synthetic tests and the post-review browser plan exclude Inspector Review roots; local-corpus probe reports zero review roots in scope. |
| D14 report artifact matrix | Deterministic tests cover valid, invalid, partial, overwritten, process-failure, absent-report, and missing-citation cases. |
| G1 review isolated from target | External task ran in its own review directory with a distinct captured thread. |
| G2 no start before confirmation | Browser proved no review directory existed after planning and before the separate confirmation action. |
| G3 rubric/skill/manifest received | Authenticated task produced a schema-valid report from the frozen manifest through the installed Review skill; contract tests assert the exact launch prompt. |
| G4 complete only after valid report | Invalid/partial/process-failure tests never complete; external proof completed only after schema-valid acceptance. |
| G5 first valid plus hash | Overwrite test preserves the first accepted report; deterministic and live evidence record its hash without an immutability claim. |
| G6 citations resolve | Four live findings resolved citations; the browser opened one into Context Inspector and returned. |
| G7 invalid/missing remains diagnosable | Deterministic manager/server/UI tests preserve malformed artifacts and render missing-source availability. |
| G8 open task and resume fallback | External proof invoked the captured thread deep link and resumed the same thread; browser exposed both handoffs. |
| G9 no recommendation execution | UI and manager contracts expose recommendations/action prompts as inert text with copy-only controls; no execution endpoint exists. |
