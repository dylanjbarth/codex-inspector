# Codex Inspector demo runbook

This runbook covers the demo-critical boundary in
[`mvp-architecture.md`](architecture/mvp-architecture.md). It is not a support
promise for other hosts, source formats, operating systems, or release lines.

## Tested matrix

| Component | Tested value |
| --- | --- |
| macOS | 26.5.1 (build 25F80) |
| architecture | arm64 |
| Codex CLI/host | codex-cli >=0.142.5 (proof host: 0.145.0-alpha.18) |
| rollout adapter | rollout-jsonl/codex-recent-structural/v4 |
| Inspector CLI | 0.1.x |
| Inspector plugin | 0.1.x |
| plugin protocol | 1 |
| index schema | 2 |
| Go build toolchain | 1.26.0 darwin/arm64 |
| pnpm | 10.28.0 |
| Node.js | 25.6.1; Phase 6 scripts reject versions below 20 |
| Playwright CLI | 0.1.17, exact workspace lock |
| Playwright runtime | 1.62.0-alpha-1783623505000 |
| Browser | Chrome for Testing 151.0.7922.10, Chromium revision 1232 |

The exact current contract and structural fingerprint are in
[`support-matrix.md`](contracts/support-matrix.md). Inspector rejects unknown
or incompatible rollout shapes instead of parsing them optimistically.

## Privacy and sensitive data

Inspector is local-first. Opening it does not upload the derived index or
source logs. The source rollout files remain authoritative and Inspector does
not edit or copy complete rollouts. The derived SQLite index and token-only
full-text data are sensitive local data and use user-only permissions.

Evidence views intentionally display exact readable local payloads. Those
payloads can contain prompts, source code, tool arguments and results,
credentials, personal data, or other secrets. Inspector does not add a secret
masking layer. Treat screen sharing, screenshots, browser automation artifacts,
and copied evidence as sensitive.

Doctor output, status, logs, hook markers, SSE events, copied diagnostics, and
the verification scripts contain aggregate metadata only. They must not be
used to collect rollout payloads, evidence text, access tokens, cookie values,
session IDs, report bodies, or absolute source paths.

Starting an Effectiveness Review is a separate model boundary. The persisted
Codex task may send evidence that it reads to the user's configured Codex
model/service. The plan preview shows the frozen scope, and the task starts only
after explicit confirmation. Recorded evidence and repository content are
untrusted data, not instructions.

## Published clean installation

The demo installation source of truth is GitHub Releases. A local source build
is useful for development but is not clean-install evidence.

For ordinary user installation, start with the repository
[`README`](../README.md). It documents the checksum-verifying CLI installer,
manual alternative, pinned plugin setup, and first-run commands. The isolated
procedure below is the stricter release-proof rehearsal and copies credentials
only because that separate, explicitly authorized proof needs an authenticated
Codex task; the ordinary installer never accesses credentials or Codex data.

The verified release is
[`v0.1.1`](https://github.com/dylanjbarth/codex-inspector/releases/tag/v0.1.1),
whose tag peels to accepted commit
`35bc97a3f3533da9972074ad329e21b40d9de36f`. The macOS arm64 binary digest is
`sha256:53e14038f069b4642c4b060a08e8d65a5764d881c37f8646d31fdd8148db1217`;
the checksum-file asset digest is
`sha256:5de5a71c12712637a6e4b47f5c7ad928e1820a6107c4ca158c380a44f45bf795`.
This accepted tag is the manual-fallback provenance; a working-tree build is
never substituted for published-install evidence. The default branch did not
contain a release publication workflow, so publication was manually vetted by
checking the tag target, Mach-O/version, official asset names and digests, and
the downloaded checksum before either clean run began.

`v0.1.1` is a source-only compatibility patch over the frozen 0.1.0 product
contract: the binary assets and their embedded CLI version remain 0.1.0, while
the tagged plugin hook correctly accepts index schema 2. The 0.1.0 text in the
setup skill and `marketplace/release.json` is frozen product/plugin metadata,
not authority for this clean-install source. The Phase 6 proof pins both asset
downloads and `codex plugin marketplace add` to the corrected `v0.1.1` tag.
The superseded `v0.1.0` clean runs exposed a real hook compatibility defect:
after an actual resumed Codex task invoked a trusted hook, doctor reported
`hook_protocol_mismatch` and the UI returned to its setup gate. Those attempts
are defect evidence, not accepted Section 1.1 runs.

The clean proof places its three synthetic fixtures in the dated Codex layout
`sessions/2026/07/19`. Directly placing fixtures under `sessions/` is not valid
doctor evidence even if the indexer can inventory them; the proof regression
rejects that layout before any manual or model step.

From a clean checkout, verify that the expected release exists, then run:

```sh
CODEX_INSPECTOR_KEEP_INSTALL_PROOF=1 scripts/phase6/prove-published-clean-install.sh
```

The script creates a private disposable root, downloads
`codex-inspector-darwin-arm64` and
`codex-inspector-darwin-arm64.sha256` from the pinned `v0.1.1` GitHub Release,
verifies the checksum, installs the binary in the disposable `PATH`, adds the
`dylanjbarth/codex-inspector@v0.1.1` marketplace and plugin to an isolated `CODEX_HOME`, and copies only
the local `auth.json` into that home with mode 0600. Keep mode creates a
mode-0700 `environment.sh`; run the printed `. '<path>/environment.sh'`
continuation command before any manual step. That file explicitly sets
`PATH`, `CODEX_HOME`, and `CODEX_INSPECTOR_HOME`, so the rehearsal cannot fall
back to normal user homes.

In the printed disposable environment:

1. Start Codex without a hook-trust bypass.
2. Select **Review hooks**, verify that all seven definitions invoke the
   plugin-owned `inspector-hook.sh`, then choose **Trust all and continue**.
3. Run `codex-inspector doctor`; every check must be `ok`.
4. Run `codex-inspector open`. A new server automatically starts indexing in
   the background. For this acceptance proof, deliberately run
   `codex-inspector sync --wait` afterward so the finite pass and its final
   counts are captured before continuing; this blocking command is not needed
   in normal interactive use.
5. Confirm the dashboard reports the entire supported inventory with explicit
   unsupported, partial, or failed counts.
6. Complete the user flow below.
7. The same-thread resume in the user flow is the required actual trusted-hook
   trigger. After it completes, run the printed `post_hook_verify_command`.
   It must report a healthy all-ok doctor, installed schema-2 hook, and no
   `hook_protocol_mismatch`; reload the UI and confirm it did not return to the
   setup gate.
8. Stop the server, close the browser, and run the printed exact cleanup
   command for the disposable root.

Never retain the isolated `auth.json`, browser profile, source fixtures, Review
artifacts, server metadata, or access tokens as verification artifacts.

## Demo flow

Run this complete sequence twice, starting from a newly created isolated
environment each time:

1. Open **Token & Capacity** and confirm recorded totals are labeled with
   user-root-direct and descendant attribution. Capacity is a recorded
   point-in-time observation, not an inference from local token totals.
2. Open a contributing root session. Confirm the causal map, direct/inclusive
   token totals, completed-turn focus, and chronological event ledger.
3. Select an event and inspect exact inert source evidence. Confirm the
   sensitive-data warning. Where complete recorded model input is absent, the
   UI must say **Unavailable in demo**.
4. Open **Review effectiveness**, inspect the scope, coverage, estimates,
   model, reasoning, prompt, and output destination. Confirm that no task starts
   before the separate confirmation.
5. Confirm once. Observe the persisted Review move through running to complete,
   with a captured Codex thread and schema-valid accepted `review.json`.
6. Open a report citation in Context Inspector and return to the report.
7. Use **Open Codex task**, then prove the copyable `codex resume <id>` fallback
   resumes the same thread and invokes the trusted installed hooks. Inspector
   must not execute a recommendation. Run the post-hook verifier and confirm
   the UI remains out of the setup gate before cleanup.

## Verification commands

Run from the repository root after `pnpm install --frozen-lockfile`. Install
the exact lockfile-selected browser, then validate the complete prerequisite:

```sh
pnpm exec playwright-cli install-browser chromium
scripts/phase6/browser-prerequisite.sh --check
```

The check requires Node 20 or newer and the exact tested CLI, Playwright
runtime, Chrome-for-Testing version, and Chromium revision in the matrix above.
The browser E2E uses that pinned workspace runtime; it does not silently select
an arbitrary system channel, cached daemon, or runtime package lookup.

Then run:

```sh
scripts/phase6/verify.sh
scripts/phase6/rehearse-crash-security-failures.sh
scripts/phase6/profile-local-corpus.sh
scripts/phase6/prove-browser-e2e.sh
scripts/phase6/test-proof-script-cleanup.sh
CODEX_INSPECTOR_RUN_LIVE_PROOFS=1 scripts/phase5/prove-review-launch.sh
```

The corpus profile reads the real `CODEX_HOME` once and writes only to a
disposable `CODEX_INSPECTOR_HOME`. It emits aggregate counts and resource
counters, never payloads, paths, IDs, or report contents. Repeated-scan
idempotence is proved with the immutable synthetic corpus because the active
developer rollout may append during a real-corpus profile.

## Required failure-state rehearsal

| State | Expected recovery or terminal explanation |
| --- | --- |
| empty | **Ready for your first sync** with an explicit sync action |
| partial/indexing | usable recent facts plus processed/eligible counts and a partial-coverage warning |
| unsupported source | skipped with detected-version/reason metadata; supported totals remain intact |
| stale capacity | visibly stale recorded observation; never recomputed from local tokens |
| missing source | retained metric facts plus `source_missing` evidence availability |
| malformed report | unrenderable Review with preserved local diagnostic state; never complete |
| failed Review process | failed state with payload-free lifecycle reason and no accepted report |
| stale server metadata | next `open` authenticates a new process and replaces stale metadata |
| interrupted normalization | transaction and checkpoint remain aligned; retry is idempotent |
| failed epoch build | active catalog remains on the prior validated database |

## Known limitations and deferred hardening

- Only the tested macOS arm64, the documented exact Codex source cohorts, and
  Inspector 0.1.x line are supported for this demo.
- There is no notarization, automatic update, shell-profile repair, complete
  uninstall workflow, Linux/Windows support, cloud service, or public API.
- Active turns do not stream. Context Inspector stops at the last indexed
  completed turn and does not reconstruct unrecorded server-side context.
- Exact evidence is not secret-masked. Missing, truncated, redacted, encrypted,
  or unsupported upstream content stays unavailable.
- Review scope is prompt- and manifest-bounded, not a technical read sandbox.
  Citation resolution proves location, not semantic support.
- Review cancellation, retry-in-place, multi-process recovery, report revisions,
  and automatic recommendation execution are out of scope.
- Same-user hostile direct SQLite mutation, exhaustive corruption matrices, and
  query-plan perfection are non-blocking hardening, not demo acceptance.

See Section 16 of the architecture for the complete deferred list.
