# Fact model contract

The executable schema is [`schema.sql`](schema.sql). Schema version `1` is the
only Phase 0 contract.

## Stable identities

| Fact | Stable key |
| --- | --- |
| source artifact | `dataset_epoch + source_session_id + segment_fingerprint`; active/archive kind and path are mutable discovery aliases |
| logical session | `dataset_epoch + source session_id`; the stored `id` is an epoch-scoped internal key so active and building epochs can coexist |
| source segment | `session_id + segment_fingerprint` |
| project | normalized Git remote, else canonical Git root, else normalized cwd |
| work unit | user-created root `session_id` |
| turn | `session_id + source turn_id` |
| event | `segment_id + record_ordinal + semantic_phase` |
| message | event identity plus message role/phase |
| tool call | `session_id + source call_id + semantic_phase` |
| usage contribution | `turn_id + formula_version` |
| capacity observation | source event identity plus limit/window identity |
| evidence | opaque Inspector ID resolving to event fingerprint and locator |

Every identity above is enforced by a database primary key or unique
constraint. Path, inode, mtime, and size are aliases/checkpoint hints and never
logical identities.

For `rollout-jsonl/codex-cli-0.144.1/v1`, the immutable segment identity
fingerprint is SHA-256 over the source session ID, a separator, and the exact
complete leading `session_meta` record bytes. The source ID and segment ID are
separate domain-prefixed hashes over the logical session ID plus that immutable
fingerprint. They therefore do not change when complete records are appended or
when the same artifact moves from active to archived discovery.

Mutable validation fingerprints are separate: a checkpoint fingerprint is
SHA-256 over exactly the already-validated complete byte prefix. Appending
preserves every older checkpoint fingerprint, while a change within an indexed
prefix produces a mismatch and moves the artifact to `requires_rebuild`.
Evidence binds to the prefix ending at its record locator, so later appends do
not change an existing evidence ID or locator.

Internal row IDs may include or hash the dataset epoch. Source identities do
not. In particular, `sessions.source_session_id` is the source-provided ID and
is unique within an epoch; this permits an active and a building epoch to hold
the same logical corpus during atomic rebuild.

## Ownership and lineage

`sessions.root_work_unit_id` is mutually exclusive ownership for accounting.
A user root owns itself. A source-proven spawned session inherits the parent
root and has a `spawned` edge. A fork is a new root with a `forked_from` edge.
An unresolved parent yields an `orphan` root and reduced lineage coverage.
Resumes retaining the same session ID remain one logical session across
segments. Inspector Reviews and descendants use purpose `inspector_review`.

Projects are dimensions, not lineage. Canonical normalized Git remote wins,
then canonical Git root, then normalized cwd. Aliases preserve observed source
strings without affecting identity.

## Turn commit boundary

Turns move through `provisional`, `completed`, `aborted`, `interrupted`, or
`reconciled_truncated`. Only `completed` turns participate in metric and event
read models. A `task_complete` record is the frozen supported completion
signal. A truncated final JSONL record stays in checkpoint state and does not
create committed facts.

## Source state machine

```text
discovered -> supported -> indexing -> current
                    |          |          |
                    v          v          v
                unsupported   failed   requires_rebuild
```

- `unsupported` is terminal for the current adapter version but remains in
  inventory.
- a truncated final line leaves the source `indexing` with the last complete
  byte checkpoint.
- shrinkage or a changed indexed-prefix fingerprint moves the source to
  `requires_rebuild`.
- missing files retain committed observations and set evidence availability to
  `source_missing`.

## Checkpoints and idempotence

The checkpoint records complete byte offset, record ordinal, size, mtime,
prefix fingerprint, adapter version, and dataset epoch. Writes use one SQLite
transaction containing source checkpoint advancement, normalized facts, and a
single new monotonic commit revision. Replaying the same complete records uses
the same identities and changes no totals.

## Dataset epochs and revisions

There is one `active` dataset epoch. An adapter/schema rebuild writes a
`building` epoch, validates it, then changes the old epoch to `superseded` and
the new epoch to `active` in one transaction. Failed builds never become
queryable.

Each committed normalization transaction allocates a strictly increasing
revision in that epoch. API requests resolve an omitted revision to the latest
applied value once, then remain pinned for the complete response. Cache keys
include epoch and revision. SSE announces a newer available revision but never
silently advances an existing page snapshot.

## Provenance and evidence

Source-backed facts retain artifact, segment, record ordinal, byte range when
known, content SHA-256, adapter version, and opaque evidence ID. SQLite stores
metadata, hashes, lengths, categories, and FTS tokens; raw messages, reasoning,
tool arguments/results, and instruction text are range-read from the source.

If fingerprint validation fails or the source cannot be rediscovered, evidence
resolution returns an explicit unavailable reason. Existing metrics remain
retained observations and are labeled not currently verifiable.

## Coverage rules

Coverage is stored per field/fact family as `exact`, `derived`, or
`unavailable`, with observed and eligible counts and a reason. Missing values
remain nullable. No query may coalesce an unavailable source field to zero.
