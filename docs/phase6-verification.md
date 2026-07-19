# Phase 6 verification candidate

Phase 6 hardens and rehearses the complete demo-critical boundary. This file
records aggregate, payload-safe evidence only. It must never contain real
rollout text, absolute source paths, session IDs, Review contents, credentials,
browser screenshots/traces, access tokens, or cookie values.

## Environment

- macOS 26.5.1 build 25F80, arm64
- Codex CLI 0.144.1
- Go 1.26.0 darwin/arm64
- pnpm 10.28.0
- Node.js 25.6.1 (minimum enforced: 20)
- `@playwright/cli` 0.1.17
- Playwright runtime 1.62.0-alpha-1783623505000
- Chrome for Testing 151.0.7922.10, Playwright Chromium revision 1232
- Inspector/plugin 0.1.x, protocol 1, index schema 2

The exact usage and manual clean-install sequence are documented in
[`demo-runbook.md`](demo-runbook.md).

## Automated demo-boundary E2E

`TestPhase6AutomatedDemoBoundarySmoke` starts the production loopback server
against synthetic supported rollouts and uses the normal API boundaries for:

```text
open/server → automatic sync → metrics → session discovery → causal map →
completed-turn ledger → exact evidence → confirmed persisted Review subprocess
→ accepted schema-valid report → citation evidence return → task handoff
```

The Review subprocess is deterministic synthetic test infrastructure. It does
not replace the separately required actual authenticated Codex proof.

The focused command passed on 2026-07-19:

```sh
go test -count=1 ./internal/server -run 'TestPhase6AutomatedDemoBoundarySmoke' -v
```

Result: exit 0; one smoke passed in 0.41 seconds.

The pinned browser prerequisite and public-path browser proof are:

```sh
pnpm exec playwright-cli install-browser chromium
scripts/phase6/browser-prerequisite.sh --check
scripts/phase6/prove-browser-e2e.sh
```

It invokes the built `codex-inspector open` launcher in a clean isolated home,
passes the one-time fragment through the browser handoff, and drives the real
embedded UI in headed Playwright. It proves automatic sync, dashboard metrics,
the causal map and ledger, exact evidence, zero Review directories before
confirmation, the production `codex exec --json` process/JSONL/report path with
a deterministic fake Codex executable, report rendering, citation return, a
safely intercepted desktop-task link, clipboard resume feedback, exact process
shutdown, and zero retained artifacts. It reports Playwright `### Error`
records as failures even when the CLI itself exits zero, and does not print the
fragment, source payload, opaque IDs, or report body. The prerequisite rejects
Node below 20, any CLI/runtime version other than the lockfile values above,
and a missing or different Chromium executable/version. `verify.sh` checks the
prerequisite and runs this public-path browser proof twice from two fresh
isolated homes.

## Deterministic, crash, security, and failure-state verification

Run:

```sh
scripts/phase6/verify.sh
scripts/phase6/rehearse-crash-security-failures.sh
```

The suites cover generated-contract freshness; formatting, vet, build, tests,
races, TypeScript, lint, production embedding, and shell scripts; two
consecutive automated smoke runs; process metadata replacement; indexing
checkpoint and transaction rollback; dataset-epoch validation and failed-build
isolation; Review process/report failure; loopback Host/Origin/auth/CSP;
payload-free diagnostics; opaque evidence, escaped unsafe bytes, bounded range
reads, missing-source behavior; source/home path rejection; hook payload
minimization; Review prompt-injection guidance; and UI rehearsals for empty,
partial, unsupported, stale-capacity, missing-evidence, malformed-report, and
failed-review states.

The initial candidate verification run passed on 2026-07-19. `verify.sh` exited 0:
all generators were current; `gofmt`, `go vet`, the full Go suite, the listed
race suites, TypeScript type checking, lint, the 18-test web suite, production
web embedding, the Go build, shell checks, and two consecutive automated
demo-boundary smoke runs passed. The focused crash/security/failure-state
script also exited 0, including all 18 UI rehearsals.

The reviewer-requested 2026-07-19 rerun also exited 0 after the proof-script
regression was added and a race-only test server lifetime was separated from
idle-shutdown coverage. It included two fresh public-path headed-browser E2E
runs; each reported production launcher/bootstrap, zero pre-confirm Reviews,
report rendering, citation return, task-link interception, resume copy, and
zero retained artifacts as passed.

## Real supported-corpus resource profile

Run:

```sh
scripts/phase6/profile-local-corpus.sh
```

The profiler uses a fresh private Inspector home, reads the real Codex corpus
without modification, completes a full reverse scan, and emits aggregate
inventory/state/turn/checkpoint, duration, heap, RSS, and frozen
concurrency/transaction counters. It deletes the derived database and resource
file at exit. Deterministic synthetic tests prove repeated-scan idempotence;
the developer's active rollout can append while this proof runs, so it is not
treated as an immutable repeat input.

An earlier candidate profiler emitted the ambiguous key `supported_complete`.
That output is not acceptance evidence and is intentionally not reproduced as
if it had used the corrected name. Reviewer-requested rerun on 2026-07-19 after
renaming the field to the frozen `supported_current` semantics: exit 0.

```text
inventory=274 supported_current=49 unsupported=225 failed=0 requires_rebuild=0
completed_turns=1259 pending_tail_sources=0 processed=274 skipped=0
revision=290 initial_ms=79889 heap_sys_bytes=265912320
max_rss_bytes=285818880 concurrency_limit=4 writer_limit=1
transaction_records=500 transaction_bytes=8388608 payloads_emitted=0
```

The aggregate differs from the earlier dated result because the active local
corpus appended between read-only runs; both runs completed their entire
inventoried snapshots without a hidden source cap.

## Actual persisted Review and headed browser

The bounded synthetic authenticated command is:

```sh
CODEX_INSPECTOR_RUN_LIVE_PROOFS=1 scripts/phase5/prove-review-launch.sh
```

It copies only local `auth.json` into a private disposable home, installs the
plugin, indexes synthetic fixtures only, runs authenticated `codex exec --json`,
resumes the captured task, invokes the desktop deep link, emits aggregate
lifecycle booleans, and removes all temporary state. The user explicitly
authorized the authentication copy, authenticated task/resume, deep link, and
manual trust of all seven isolated hooks.

One fresh Phase 6 local-candidate rehearsal completed the entire headed flow:
all seven hooks were manually inspected and trusted; doctor was healthy; three
synthetic sources synchronized with two supported and one visibly unsupported;
the fragment disappeared; dashboard attribution, stale capacity, causal map,
three completed turns, 11-event ledger, exact inert evidence, and unavailable
context were visible; no Review directory existed before confirmation; one
authenticated persisted Review reached complete with four findings and
available citations; a citation returned to Context Inspector; the desktop
deep link opened; authenticated `codex exec resume --json` completed on the
same thread; and the copy-resume control reported success. The exact browser,
server, temporary auth home, task stream, and Playwright state were removed.

A second fresh local-candidate rehearsal independently reached the Review entry
page after re-proving fragment removal, dashboard attribution, stale/partial
labels, the two-node causal map, three completed turns, the 11-event ledger,
and exact inert evidence. It was stopped before preview or confirmation, so no
second authenticated Review was launched and it is not counted as a complete
run. Neither local-candidate rehearsal substitutes for the two required
published clean-install runs.

Two superseded `v0.1.0` official-release attempts reached the authenticated
Review, citation, desktop handoff, and same-thread resume. The actual resume
then invoked the trusted installed hook and exposed a stale schema-1
compatibility probe: post-hook doctor reported `hook_protocol_mismatch` and the
UI returned to its setup gate. They are retained only as defect/attempt
history, not counted as successful clean Section 1.1 runs. Corrected official
release `v0.1.1` pins the tagged schema-2 hook while retaining the frozen 0.1.0
binary and plugin metadata.

Two `v0.1.1` attempts were also deliberately excluded before the final pair.
The first preflight downloaded and checksummed the official release and reached
manual 7/7 hook trust, then revealed that the proof script had not seeded the
documented synthetic fixtures; it launched no browser or Review and its exact
isolated home was removed. The next attempt reached the complete authenticated
flow and proved that the resumed trusted hook produced no protocol diagnostic,
but its fixtures had been placed directly under `sessions/`. Doctor only
recognizes the frozen Codex dated layout, so post-resume `source_format` was
incompatible. That attempt was not repaired or counted; its browser, exact
server, auth home, Review, streams, and private logs were removed. The proof
now seeds exactly three fixtures under `sessions/2026/07/19`, and its regression
requires that dated placement before launch.

## Published release and clean-install status

The exact proof command is:

```sh
CODEX_INSPECTOR_KEEP_INSTALL_PROOF=1 scripts/phase6/prove-published-clean-install.sh
```

It refuses local builds and downloads the frozen macOS arm64 binary and
checksum from the exact `v0.1.1` GitHub Release before adding
`dylanjbarth/codex-inspector@v0.1.1` as the marketplace source in a clean
isolated `CODEX_HOME`. Before any Review, it copies only local
`auth.json` with mode 0600 into that isolated home. Explicit keep mode creates
a mode-0700 `environment.sh` that sets `PATH`, `CODEX_HOME`, and
`CODEX_INSPECTOR_HOME`, and prints exact continuation and cleanup commands.
Default and failure paths remove the isolated credential. The generated
post-hook verifier checks the installed schema-2 hook, rejects a persisted
`hook_protocol_mismatch`, and requires every doctor check to be `ok` after an
actual trusted-hook invocation. The dry-run shell regression proves those
invariants and both v0.1.1 pins without claiming a published release.

The official release is
[`v0.1.1`](https://github.com/dylanjbarth/codex-inspector/releases/tag/v0.1.1).
Its annotated tag peels to approved compatibility commit
`35bc97a3f3533da9972074ad329e21b40d9de36f`; this is the manual-fallback source
provenance used for the published binary while the Phase 6 verification
candidate remains uncommitted. The default branch had no release publication
workflow, so the fallback publication was manually vetted: tag target, local
Mach-O/version, release asset names, GitHub asset digests, and the downloaded
checksum were each verified before clean-run acceptance. The exact release
assets are:

```text
codex-inspector-darwin-arm64
sha256:53e14038f069b4642c4b060a08e8d65a5764d881c37f8646d31fdd8148db1217
codex-inspector-darwin-arm64.sha256
sha256:5de5a71c12712637a6e4b47f5c7ad928e1820a6107c4ca158c380a44f45bf795
```

The asset digests and embedded `codex-inspector 0.1.0` version are intentionally
unchanged: `v0.1.1` is the corrected tagged plugin-source patch. The setup skill
and `marketplace/release.json` retain textual 0.1.0 as frozen product/plugin
metadata; neither is used as the release or marketplace source in this proof.

On 2026-07-19, the exact `v0.1.1` published-install script completed twice
consecutively in keep/continuation mode. Both fresh environments passed the
complete Section 1.1 manual flow, the actual resumed task triggered the trusted
hooks, the post-hook verifier remained healthy without a protocol mismatch,
the headed UI remained outside the setup gate, and each environment was then
removed. No local candidate binary substituted for either run.

The first corrected Review reached its accepted terminal report in about 70
seconds and the second in about 130 seconds, both within the five-minute
terminal timeout. Each run inventoried and processed exactly three synthetic
sources with zero failed or rebuild-required sources. All recorded evidence is
aggregate-only; opaque task/session IDs, report content, source payloads,
tokens, cookies, and absolute temporary paths were not retained.

| Official clean run | Release/install | Isolated setup | Headed product flow | Actual Review/handoff | Cleanup |
| --- | --- | --- | --- | --- | --- |
| 1 | v0.1.1 checksum, tag-pinned marketplace, Mach-O arm64, embedded 0.1.0/protocol 1/schema 2 passed | explicit homes/PATH, auth 0600, 7/7 hooks trusted, initial and post-resume doctor healthy, no protocol mismatch, 3-source sync with zero failures | fragment removed; gaps, metrics, map, turn, 11 events, exact/unavailable evidence; zero pre-confirm Reviews; no post-hook setup gate | schema-valid complete report with findings, citation return, task link/copy, desktop open, same-thread resume complete and trusted hook invoked | browser closed; exact PID, auth/review home, streams, logs absent |
| 2 | v0.1.1 checksum, tag-pinned marketplace, Mach-O arm64, embedded 0.1.0/protocol 1/schema 2 passed | explicit homes/PATH, auth 0600, 7/7 hooks trusted, initial and post-resume doctor healthy, no protocol mismatch, 3-source sync with zero failures | fragment removed; gaps, metrics, map, turn, 11 events, exact/unavailable evidence; zero pre-confirm Reviews; no post-hook setup gate | schema-valid complete report with findings, citation return, task link/copy, desktop open, same-thread resume complete and trusted hook invoked | browser closed; exact PID, auth/review home, streams, logs absent |

## Deliverable and exit-gate matrix

| Requirement | Candidate evidence |
| --- | --- |
| D1 published clean install | Official v0.1.1 assets and tag-pinned marketplace install verified twice from fresh isolated homes; exact provenance/digests and frozen embedded 0.1.0 metadata recorded. |
| D2 real supported corpus | Reviewer rerun processed 274/274 sources read-only; 49 supported/current, 225 visibly unsupported, zero failed/rebuild/pending-tail sources. |
| D3 reverse-scan resources | Reviewer rerun: 79.889 s and 285,818,880-byte max RSS; concurrency 4, one writer, 500-record/8-MiB transaction bounds; no hidden cap. |
| D4 crash/restart | Focused metadata, index transaction/checkpoint, epoch, and Review lifecycle rehearsals passed. |
| D5 security | Loopback, evidence escaping/ranges, prompt scope, source/home path, and diagnostics suites passed. |
| D6 failure states | Deterministic API/manager/UI rehearsals passed for every named state; 18/18 UI tests. |
| D7 docs/privacy/limitations | Tested matrix, privacy disclosure, sensitive-data warning, failure recovery, limitations, and demo runbook documented. |
| D8 automated E2E | Public production-launcher headed-Playwright proof covers bootstrap through report/citation/handoff; two consecutive fresh runs passed in the final full verification. |
| D9 optional SQL/query-plan | Deliberately deferred until required demo path is accepted. |
| G1 Section 1.1 twice clean | Met: two consecutive corrected official v0.1.1 runs completed the full actual headed/authenticated flow, post-resume hook/doctor/UI check, and exact cleanup; all earlier attempts are explicitly non-counting. |
| G2 entire inventory/budgets | Reviewer rerun processed 274/274 with frozen concurrency/writer/transaction limits and 285,818,880-byte max RSS. |
| G3 gaps honest/totals intact | Current reviewer rerun kept 225 unsupported sources explicit; focused API/metric/storage/UI rehearsals passed. |
| G4 no raw payload leakage | Scripts/docs and recorded evidence are aggregate-only; final artifact scan found no retained auth, report, browser, server, screenshot, trace, or task-stream artifact. |
| G5 every failure state | Focused Go and UI rehearsal command passed. |
| G6 clean-checkout tests/build | Candidate full verification passed; independent reviewer rerun remains required. |
| G7 hardening explicit | Direct-SQL/query-plan work remains non-blocking and does not substitute for any demo-critical gate. |

## Final Phase 6 completion mapping

| Final criterion | Evidence |
| --- | --- |
| Published clean install twice | Two consecutive official v0.1.1 runs above; no local-binary substitution; actual trusted-hook resume followed by healthy post-hook doctor in both. |
| Final build and tests | Final `verify.sh` passed generators, formatting, vet, full Go, races, internal smoke twice, proof-script regressions, TypeScript, lint, 18/18 web tests, production web build, Go build, shell checks, and two deterministic production-launcher browser E2Es. |
| Security and failure recovery | Final crash/security/failure-state rehearsal passed process, storage, indexer, Review, server, evidence, home, source, hook, and 18/18 UI checks. |
| Real-corpus and resource budget | Corrected dated profile processed 274/274 read-only with 49 supported/current, 225 explicit unsupported, zero failure/rebuild/tail gaps, and frozen resource/concurrency/transaction counters. |
| Actual external lifecycle | Both corrected official runs launched exactly one authenticated persisted Review, accepted a schema-valid report, returned through a citation, opened the desktop handoff, resumed the same thread, invoked the trusted hook, and retained a healthy post-resume doctor/UI state. |
| Privacy and clean terminal state | Aggregate-only record; final scans found no retained auth, report, run, manifest, server, browser, video, archive, task-stream, private-log, proof-root, or proof-process artifact. |
| Deferred scope | Only the non-blocking items listed in the runbook and architecture Section 16 remain; none substitutes for a required demo-critical gate. |
