# Phase 4 verification

Phase 4 implements revision-pinned session discovery, the root-session causal
map, completed-turn ledgers, and exact source evidence resolution for the
Context Inspector.

Run the deterministic suite from the repository root:

```sh
scripts/phase4/verify.sh
```

The suite checks frozen generated contracts and schemas, formatting, vet,
build, Go and race tests, TypeScript, ESLint, UI tests, the embedded production
bundle, and shell scripts. API and UI tests cover explainable descendant
discovery, metric-aligned direct/inclusive totals, lineage and stable routes,
complete chronological ledgers, bounded exact-source reads, honest missing
source behavior, explicit unavailable model input, compaction without
reconstruction, inert adversarial payload rendering, and route-preserving
revision advancement.

Run the payload-free real-corpus proof with:

```sh
scripts/phase4/validate-local-corpus.sh
```

It indexes `CODEX_HOME` into a disposable Inspector home and emits aggregate
counts only. It does not print paths, session or evidence IDs, messages, tool
payloads, source text, or access data. It checks that root-map totals agree
with the metric engine, active/provisional turns are absent, ledger ordering is
stable, and one bounded source-evidence reference remains available.

For the manual browser proof, use `codex-inspector open --no-browser` against a
synthetic or otherwise safe source home, exchange the one-time fragment through
the normal Phase 3 browser handoff, and verify discovery → causal map → completed
turn → event. The browser URL must be fragment-free after exchange. Confirm the
map has visible lineage, minimap, direct/inclusive totals and the persistent
turn rail; the turn has the complete chronological ledger; exact evidence is
rendered as inert source text; unavailable model input is explicit; and an
exact compaction event disclaims reconstructed before/after content.

## 2026-07-18 candidate result

- `scripts/phase4/verify.sh`: exit 0. Generated contracts and schema were
  current; gofmt, vet, all Go tests, evidence/inspector/server race tests, Go
  build, TypeScript checking, ESLint, 13 UI tests, Vite/embedded production
  build, and shellcheck passed.
- `scripts/phase4/validate-local-corpus.sh`: exit 0. The disposable scan
  inventoried and processed 262 sources with zero failures or rebuilds at
  revision 276. It found 19 discoverable root maps, 19 map nodes, 1,015 root
  turns, and 1,185 sampled ledger events; checked bounded exact source evidence;
  found no active/provisional turns; and found zero metric/map total
  mismatches. The sampled supported corpus exposed no descendant edge or
  compaction in its first turns, so those cases remain proven by deterministic
  synthetic API and UI tests rather than claimed from the local corpus.
- A headed Chromium proof used the normal one-shot Phase 3 fragment handoff,
  an isolated Inspector home, synthetic rollout fixtures, and a payload-free
  fake Codex compatibility inventory. The exchange settled on a clean
  fragment-free `/context` URL. Discovery showed one root with 2,000 direct and
  500 descendant tokens; its map showed root and descendant nodes, a visible
  `spawned` edge, minimap, 2,500 inclusive tokens, and two completed turns. The
  first turn displayed all 11 chronological source-backed events. Selecting
  its message displayed the exact source record as inert text and labelled the
  complete model input unavailable. Selecting its compacted event displayed
  exact recorded compaction evidence and explicitly disclaimed reconstructed
  before/after context, preserved/removed classification, and component token
  estimates. No credential appeared in wrapper output. The browser, server,
  temporary compatibility executable, and generated snapshots were removed
  immediately after the proof.

