# Supported Codex contract matrix

## Supported runtime target

| Capability | Supported value | Compatibility decision |
| --- | --- | --- |
| Codex CLI/host | `codex-cli >=0.142.5` | recent stable and prerelease hosts are accepted |
| rollout adapter | `rollout-jsonl/codex-structural/v5` | historical sources proceed to the complete structural validator regardless of release age |
| proven rollout fixtures | `0.100.0-alpha.10` through `0.145.0-alpha.18` observed locally; canonical fixtures for `0.142.5`, `0.144.0-alpha.4`, `0.144.1`, `0.145.0-alpha.18` | versions are diagnostic cohorts, not an allowlist |
| session index | legacy append-only `id`/`thread_name`/`updated_at` records; file absent on proof host | optional label input; absence is supported |
| demo OS | macOS 26.5.1 | exact proof machine |
| architecture | `arm64` | only published demo artifact |
| Go | 1.26.0 darwin/arm64 | build proof toolchain |
| plugin protocol | `1` | plugin and CLI must agree |
| plugin version | `0.1.x` | Phase 1 initial range |
| CLI range | `>=0.1.0 <0.2.0` | declared by the plugin |
| index schema | `2` | exact runtime schema; schema 1 is rebuild input only, never opened as current or ALTERed |
| schema-1 transition | separate-file rebuild | validate a separately named v2 database, then atomically replace the same-directory active-catalog pointer |
| API snapshot schema | `2` | every indexed snapshot and revision event identifies schema 2 |
| revision scope | dataset epoch | monotonic only within one epoch; epoch replacement requires full refresh |
| evidence availability | live overlay | pinned facts/fingerprints remain at the applied revision; observed availability is separately timestamped and may have no revision |

“Supported” means the metadata and required event shapes match; it does not
mean every file produced by a process reporting the same CLI version is parsed
optimistically.

## Rollout fingerprint

A supported source is newline-delimited JSON with:

1. exactly one leading `session_meta` record whose payload has `cli_version`,
   `session_id` (or legacy-equivalent `id` with the same value), `cwd`,
   `originator`, and `source`; user roots use a non-empty string source while
   descendants use the observed `subagent.other` or
   `subagent.thread_spawn` tagged-object shape;
2. `payload.cli_version` is a syntactically valid release; historical ingestion
   has no version floor, and every source must independently pass the structural
   checks below;
3. a `turn_context` record per visible turn with `payload.turn_id`, `model`, and
   `cwd`; historical null or absent `effort` is retained as unavailable, and a
   repeated context for the same turn is treated as a later snapshot;
4. `event_msg/task_started` and, for committed turns,
   `event_msg/task_complete`, both carrying the same `turn_id`; when present,
   `completed_at` is numeric (string or object values are rejected), while the
   enclosing record's required RFC 3339 `timestamp` is the normalized
   completion timestamp. The proof corpus also contains the observed
   payload-minimal completion variant with no `completed_at`;
5. token observations shaped as `event_msg/token_count`, with
   `info.last_token_usage` and/or `info.total_token_usage` using
   `input_tokens`, `cached_input_tokens`, `output_tokens`,
   `reasoning_output_tokens`, and `total_tokens`;
6. ordered source evidence in `response_item` records, using a stable source
   record ordinal and `internal_chat_message_metadata_passthrough.turn_id`
   where supplied.

Recorded compaction uses the observed paired shapes
`event_msg/context_compacted` and `compacted`, with the latter carrying the
recorded compacted message/replacement history and optional window identities.

Optional supported fields include `session_meta.parent_thread_id`, Git
metadata, `rate_limits`, `world_state`, reasoning effort, response IDs, and
tool call IDs. Rate-limit-only token records retain capacity without claiming
token usage. Unknown non-empty event and response discriminators are retained
as generic evidence; only known semantic records affect lifecycle, usage, and
tool facts. A repeated `session_meta` is accepted only when its session identity
and version match the leading record. A missing optional field lowers field
coverage; it is not synthesized.

The adapter rejects the source before normalization when the CLI version is
malformed, the leading metadata record is absent, required identity fields are
missing, a known lifecycle or tool identity is inconsistent, or a complete
record or known semantic field has an incompatible type. The inventory keeps
the detected version, byte count, and a payload-free reason.

The optional `session_index.jsonl` label adapter accepts append-only objects
with string `id`, string `thread_name`, and RFC 3339 `updated_at`. The latest
valid entry by file order wins for a repeated ID. It supplies display labels
only and can never create a logical session or make an unsupported rollout
supported. The frozen fake shape is in `fixtures/synthetic/session_index.jsonl`.

The three locally observed historical cohorts also have leaf-canonicalized,
structure-preserving fixtures under `fixtures/local-structural/cohorts/`.
Their manifest records the exact structural fingerprint and deterministic
expected lifecycle, evidence, lineage, and token outcomes. One spawned source
and one root source from the local inventory share an explicit canonical
replacement ID so persistence can be reproduced without retaining private
identifiers. The manifest labels that constructed relationship and does not
claim the raw sources were related. Tests pass the fixtures through the
production adapter and full local index path. They remain regression evidence
for the exact shapes recorded in the manifest. Newer versions are accepted as
candidates but must independently pass the same validator.

## Source locations

Exactly one effective `CODEX_HOME` is resolved per invocation. An explicit
environment value has resolution source `environment`; otherwise the default
`~/.codex` has source `default`. The canonical home identity is written to
server metadata and used to select a separate derived dataset catalog. A
healthy process or dataset for another home is never reused or aggregated.
Discovery includes:

- `sessions/**/*.jsonl` for active rollouts;
- `archived_sessions/**/*.jsonl` when that directory exists;
- `session_index.jsonl` when present.

The Phase 0 proof host had active rollout files but no
`archived_sessions/` directory and no `session_index.jsonl`. Both absences are
normal empty inputs. `history.jsonl`, SQLite databases, memories, and logs are
explicitly excluded.

Canonical discovery rejects `CODEX_INSPECTOR_HOME` and any symlink alias of it.

## Hook contract

The plugin registers command hooks for `SessionStart`, `UserPromptSubmit`,
`PreCompact`, `PostCompact`, `SubagentStart`, `SubagentStop`, and `Stop`.
Recent Codex hosts send one JSON object on stdin. Inspector consumes only:

- `session_id`
- `turn_id` when present
- `transcript_path` as a locator only
- `hook_event_name`
- subagent discriminator/type fields when present

The shim uses `PLUGIN_ROOT` to locate the bundled command and `PLUGIN_DATA`
only for plugin diagnostics. It invokes the CLI `_hook` command, emits no
payload to stdout, and exits zero when the CLI is absent or incompatible.
Hook trust is not bypassed: installed or changed plugin hooks must be reviewed
by the user, and trust is tied to the current definition hash.

On the proof host, a clean interactive launch first offered **Review hooks**,
**Trust all and continue**, or **Continue without trusting**. Review showed
all seven installed definitions; accepting them persisted seven separate
SHA-256 trust hashes before Codex continued. No bypass flag was used, and the
isolated config contained no proof payload canaries.

The transcript path is not a supported transcript schema. It can schedule a
source scan but cannot bypass the rollout fingerprint.

## Current-session handoff

No documented skill variable directly exposes the current thread ID. A skill
may call `codex-inspector open --current-session`; the CLI resolves the parent
session only from an in-scope hook marker or an explicit session identifier.
It never guesses using cwd or timestamps. Without a reliable marker it opens
session discovery and reports that the current session could not be located.

## Persisted task handoff

`codex exec --json` persists by default. The first JSONL event is
`{"type":"thread.started","thread_id":"..."}`. Inspector records that
`thread_id`, supports `codex exec resume <id>`/`codex resume <id>`, and exposes
the desktop deep link `codex://threads/<id>`. `--ephemeral` is prohibited for
Reviews.

The proof host resumed the captured ID through both `codex exec resume` and an
actual interactive `codex resume` PTY. The interactive continuation returned a
unique requested canary, establishing that it was the persisted Review thread
rather than a new session. The desktop deep link was invoked through macOS;
visual rendering in the desktop client is not part of the claimed evidence.

The installed skill invocation identifier is `$codex-inspector:review-session`.
The launch prompt must include that exact identifier.

## Official contract evidence

The matrix was checked on 2026-07-18 against the installed CLI help, structural
field-only probes of the local rollout format, and the current official Codex
manual sections for plugins, hooks, skills, non-interactive mode, and deep
links. No local message, tool payload, absolute rollout path, session ID, or
credential is stored in these contracts.
