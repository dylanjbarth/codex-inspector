# Phase 0 verification record

Run on the demo machine:

```text
GOCACHE="${TMPDIR}/codex-inspector-go-cache" go test ./...
GOCACHE="${TMPDIR}/codex-inspector-go-cache" go vet ./...
GOCACHE="${TMPDIR}/codex-inspector-go-cache" go run ./tools/phase0/sqliteprobe
GOCACHE="${TMPDIR}/codex-inspector-go-cache" go run ./tools/phase0/sourceprobe --fixture fixtures/synthetic/root.jsonl
GOCACHE="${TMPDIR}/codex-inspector-go-cache" go run ./tools/phase0/sourceprobe --fixture fixtures/synthetic/unsupported.jsonl
GOCACHE="${TMPDIR}/codex-inspector-go-cache" go run ./tools/phase0/sourceprobe --codex-home "${CODEX_HOME:-${HOME}/.codex}"
GOCACHE="${TMPDIR}/codex-inspector-go-cache" CGO_ENABLED=0 go build -o "${TMPDIR}/codex-inspector-sqliteprobe" ./tools/phase0/sqliteprobe
file "${TMPDIR}/codex-inspector-sqliteprobe"
scripts/phase0/check-local-contracts.sh
shellcheck scripts/phase0/*.sh fixtures/plugin-marketplace/plugin/hooks/probe.sh
scripts/phase0/generate-internal-api.sh --check
scripts/phase0/generate-fact-golden.sh --check
```

The fact generator writes only `expected-facts.json`.
`expected-revisions.json` is an independently authored expected projection;
schema-v2 tests derive actual revision selection, fingerprint separation, live
evidence overlay, and real separate-file rebuild/catalog-swap outcomes.

Phase acceptance uses the demo-critical boundary in the architecture. The CLI
is the sole trusted writer to the local derived database. Contract tests prove
the normal writer path: paired provenance/FTS insertion, exact FTS token
cardinality and digest validation, changed-checkpoint revision allocation, and
validated one-way epoch activation. Hostile/manual direct-SQL mutation by the
same OS user, exhaustive FTS shadow-table guards, exhaustive corrupt-candidate
matrices, and query-plan perfection are recorded as post-demo hardening rather
than Phase 0 or Phase 2 blockers.

`check-local-contracts.sh` prints only versions, counts, and structural
decisions. It never prints rollout paths, IDs, messages, tool payloads, or
credentials. Live plugin installation/trust and disposable Review launch use
separate opt-in commands because they mutate isolated Codex state and may
consume account capacity:

```text
scripts/phase0/prove-plugin-install.sh
scripts/phase0/prove-review-launch.sh
```

The normal Review proof establishes persisted non-interactive resume and invokes
the desktop deep link. To prove the exact interactive `codex resume` contract,
preserve one disposable proof home, resume its captured thread in a PTY, then
remove the proof home immediately because it contains a temporary copy of the
operator's Codex authentication:

```text
CODEX_INSPECTOR_RUN_LIVE_PROOFS=1 CODEX_INSPECTOR_PRESERVE_FOR_PTY=1 scripts/phase0/prove-review-launch.sh
state_file="${TMPDIR:-/tmp}/codex-inspector-manual-resume-state"
. "${state_file}"
CODEX_HOME="${CODEX_HOME}" codex --no-alt-screen resume "${THREAD_ID}" "Return a unique four-word canary joined by hyphens, with no other text."
/usr/bin/open -Ra Codex
/usr/bin/open -g "codex://threads/${THREAD_ID}"
case "${CODEX_HOME}" in "${TMPDIR:-/tmp}"/codex-inspector-review-home.*) ;; *) exit 1 ;; esac
rm -rf "${CODEX_HOME}"
rm -f "${state_file}"
```

The operator must verify that the resumed TUI displays the requested unique
canary and the same captured thread ID. The canary and ID are transient proof
data and must not be recorded in repository artifacts.

## Evidence table

| Phase 0 requirement | Evidence |
| --- | --- |
| host/OS/format frozen | `support-matrix.md`, local contract check |
| marketplace/plugin/hook behavior | fake probe plugin plus opt-in isolated install script |
| current session behavior | documented no-guess fallback in support matrix |
| persisted exec/session handoff | Review launch parser test and opt-in live proof |
| installed Review skill identifier | launch contract and probe plugin skill |
| self-contained SQLite | pure-Go driver probe and schema test |
| synthetic current-format contract | `fixtures/synthetic` and adapter tests |
| identities/schema/epochs/revisions | `fact-model.md`, `schema.sql`, schema tests |
| metric semantics/goldens | `metrics.md`, expected metrics, golden tests |
| API/SSE | OpenAPI contract and schema checks |
| Review schemas/prompt | `reviews.md`, three JSON Schemas, schema tests |
| budgets | `performance-budgets.md` |

## 2026-07-18 result

- `go test ./...`: pass
- `go vet ./...`: pass
- pure-Go SQLite round trip: pass (`modernc.org/sqlite`, no cgo required)
- supported source fixture: pass (2 completed root turns)
- local structural check: pass on Codex CLI 0.144.1, Go 1.26.0, macOS
  26.5.1 arm64; 244 active rollouts inventoried at the final worker run,
  archives and session index
  absent
- payload-free real-corpus discriminator: pass; 34 supported sources, 210
  unsupported sources, zero parse failures, and 781 completed turns. Unsupported
  reasons were 198 CLI-version mismatches, eight incompatible turn contexts,
  two incompatible record envelopes, one inconsistent turn identity, and one
  missing required session identity. The command emitted no paths, IDs,
  messages, tool data, or other payloads
- isolated marketplace installation and enabled plugin: pass
- hook environment probe: pass for `PLUGIN_ROOT` and `PLUGIN_DATA`
- missing-CLI shim: pass; exited zero and created no Inspector queue marker
- interactive hook trust: pass without bypass; Codex displayed seven new hooks,
  allowed definition review, and persisted seven distinct SHA-256 trust hashes
  before continuing. A payload-canary scan of the isolated config was clean
- disposable installed-skill Review: pass; persisted exec emitted
  `thread.started`, wrote a schema-valid report, resumed the same ID through
  `codex exec resume`, then resumed that captured ID through the actual
  interactive `codex resume` TUI and returned the unique requested canary. The
  macOS `codex://threads/<id>` handoff was invoked successfully; visual desktop
  rendering was not claimed
- shellcheck: pass
- generated TypeScript contract: current with pinned
  `openapi-typescript@7.10.1`

The normal hook-trust interaction is an explicit manual clean-install check
because Codex intentionally requires an interactive user decision tied to the
installed hook-definition hash and there is no safe non-interactive “trust
this hook” command. The proof must not use
`--dangerously-bypass-hook-trust`. Run the isolated marketplace command, start
Codex with that isolated `CODEX_HOME`, inspect the displayed hook definition,
accept it. Inspecting the isolated config may verify that each accepted hook
has its own `trusted_hash`; do not record the temporary home, config, rollout,
or authentication contents. A changed hook definition must be treated as
untrusted until accepted again.
