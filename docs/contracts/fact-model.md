# Fact model contract

The executable contract is [`schema.sql`](schema.sql). Schema version `2` is
the only supported runtime schema. A version-1 database is left intact, rebuilt
into a separate v2 database/epoch, validated, and atomically swapped; it is
never upgraded with `ALTER TABLE`.

## Hybrid revision model

Query-visible data uses one of two revision strategies:

- Immutable core rows carry an explicit creation, observation, or commit
  revision. This includes source artifacts, projects, project aliases,
  sessions, source segments, lineage edges, terminal turns, and events.
- Values that can evolve use append-only version tables with primary key
  `(epoch, entity, revision)`: `source_artifact_versions`,
  `session_versions`, `session_label_versions`, and
  `coverage_observation_versions`. Reads select `max(revision) <=
  applied_revision` through the matching descending revision index.

Immutable child rows are not fully versioned. Messages, tools, capacity,
compactions, and evidence inherit visibility by joining their parent event;
usage joins its terminal turn. Direct child-table queries without that parent
join are invalid. This avoids redundant revision columns without losing pinned
snapshot semantics.

Project display name is deterministic and immutable in v2: it is derived when
the canonical project identity is created. A future user-editable display name
would require a project version table rather than an in-place update.

## Stable identities and ordering

| Fact | Stable key |
| --- | --- |
| source artifact | `dataset_epoch + source_session_id + segment_fingerprint` |
| logical session | `dataset_epoch + source session_id` |
| source segment | `session_id + segment_fingerprint` |
| project | normalized Git remote, else canonical Git root, else normalized cwd |
| work unit | user-created root `session_id` |
| turn | `session_id + source turn_id` |
| event | `segment_id + record_ordinal + semantic_phase` |
| message | parent event identity plus message role/phase |
| tool phase | `session_id + source call_id + semantic_phase` |
| usage | terminal `turn_id + formula_version` |
| capacity | parent event plus limit/window identity |
| evidence | opaque ID resolving to pinned event/source fingerprints and locator |

For `rollout-jsonl/codex-structural/v5`, the immutable segment identity
fingerprint is SHA-256 over the source session ID, a separator, and exact
complete leading `session_meta` record bytes. Source and segment IDs are
domain-prefixed hashes over that immutable material. Active/archive kind, path,
inode, size, and mtime are versioned discovery values, not identity inputs.

`source_order_key` is exactly
`<20-digit UTC Unix-nanosecond start>:<64-character segment fingerprint>`.
The start is the leading `session_meta.timestamp`, falling back to the earliest
valid record timestamp in that segment; a segment without either is unsupported.
The fingerprint is the final deterministic tie-break for equal timestamps.
A move preserves both inputs. A same-ID resume has a new segment fingerprint
and therefore a deterministic position even when timestamps tie. The stored
turn key is exactly `<segment-source-order-key>:<20-digit first-turn-context
record ordinal>`. The derived event key is exactly
`<segment-source-order-key>:<20-digit record ordinal>:<semantic-phase>`, with
the final phase tie-break compared using SQLite `BINARY` byte order. Colons are
not allowed inside a semantic phase. Reverse scanning only chooses work
priority and cannot renumber facts or causal order.

Project `display_name` is deterministic from the already-normalized canonical
identity. For `git_remote`, remove trailing slashes and one trailing `.git`,
then use the last `/` component. For `git_root` and `cwd`, clean separators,
remove trailing slashes except filesystem root, and use the last path component.
An empty component or filesystem root uses the complete normalized identity.
Case is preserved. Display names are never taken from discovery order or a
mutable alias.

## Source versions, checkpoints, and transactions

`source_artifact_versions` records source kind, canonical path, inode, size,
mtime, adapter/version decision, state/reason, and source-level evidence state.
An active-to-archive move adds a version and preserves source identity. Current
aliases aid rediscovery but never replace source identity or pinned evidence
fingerprints.

Checkpoints are operational and unversioned. They retain the current complete
byte/record boundary, observed size/mtime, mutable prefix hash, adapter,
pending-tail size, update time, and the revision whose transaction wrote them.
The checkpoint prefix hash covers the current validated complete prefix. It is
for append validation and changed-prefix detection only.

The trusted CLI writer never deletes a checkpoint and fixes its source, epoch,
and adapter identity. Every persisted checkpoint change sets
`updated_revision` to a newly allocated, later revision in the same transaction;
an unchanged scan writes nothing and allocates no revision. Byte, record, and
observed-size watermarks do not move backward. The schema enforces these normal
operations as defense in depth, but arbitrary direct SQL by the same OS user is
outside the demo threat model.

Any persisted fact, alias, label, source version/state, or checkpoint change
makes a scan changed and allocates exactly one new revision. An unchanged scan
allocates no revision. A normalization transaction atomically writes:

1. the `index_revisions` row;
2. all new immutable facts and mutable versions; and
3. the supporting checkpoint.

No visible fact may commit ahead of its proving checkpoint. A crash rolls the
whole transaction back.

The database enforces that a new revision is exactly `max(revision)+1` within
its epoch. Every parent creation/commit revision must be no newer than the
child/version/observation revision. Query-visible core, version, provenance,
coverage, availability, and search-document rows reject UPDATE and DELETE;
retired data is removed only by retiring the whole database file. The only
mutable rows are operational checkpoints (identity fixed; byte/record/revision
watermarks may only advance) and dataset epoch state (`building` to
`active|failed`, `active` to `superseded`).
An active epoch also rejects inserting a row at an older revision or attaching
a new child to an older event/turn. A building epoch may replay historical
revisions only in monotonic order, and every temporal parent must already be
visible at the replayed revision.

## Sessions, labels, and lineage

The immutable session row stores only identity and creation revision.
`session_versions` supplies project, root work unit, purpose, source,
originator, earliest proven start, latest terminal end, lineage coverage, and
inventory-reconciliation state at an applied revision. Earliest/latest values
are accumulated into new versions; old versions remain unchanged.

Session-index titles live in `session_label_versions`. FTS title documents carry
their label revision so index validation remains revision-complete. Context
Inspector displays the latest title not newer than the applied revision, but
does not use title documents for keyword discovery. Future and superseded labels
cannot appear.

The CLI's fixed `FTSWriter` is the only supported application mutation path for
the contentless event and session-label FTS5 tables. It inserts the immutable
provenance document and matching FTS row together in the caller's transaction
and exposes no update or delete operation. Candidate validation compares exact
document and token-instance cardinalities plus canonical token/provenance
digests. SQLite cannot attach ordinary triggers to FTS5 virtual tables, so
hostile direct-SQL UPDATE/DELETE resistance and exhaustive shadow-table guards
are deferred hardening, not demo correctness requirements.

A descendant remains unresolved until full inventory reconciliation either
proves lineage or proves the parent absent. Only then may a session version use
`purpose='orphan'`, enforced by `inventory_reconciled=1`. A proven lineage edge
and the corrected child session version that assigns root/purpose are inserted
at the same revision. Forks retain `forked_from`; same-ID resumes keep one
session across ordered segments and preserve cumulative usage baselines.

## Terminal turns and event visibility

Provisional turns are never stored. The `turns` table contains only
`completed`, `aborted`, `interrupted`, and `reconciled_truncated` rows, each
with a non-null commit revision and terminal time. Completed turns additionally
require `completed_at`; only completed turns contribute metrics.

Turn-bound events have exactly the terminal turn's commit revision, enforced by
a trigger. A null-turn event has its own revision and is permitted only for the
explicit session/source-level kinds frozen in schema v2. It references a
`terminal_watermarks` row proven by a concrete turn-bound `task_complete` event
in the same epoch, session, source, segment, and terminal revision. Its record
ordinal cannot exceed the frozen maximum and the proving terminal revision
cannot be newer than the event revision. A terminal from another segment of the
same resumed session is insufficient. Callers cannot invent a timestamp
watermark. Null-turn storage is never a fallback for unresolved, provisional,
cross-source, cross-segment, or post-watermark data.

All immutable child visibility is inherited from the parent:

```sql
SELECT m.*
FROM messages m
JOIN events e ON e.epoch_id = m.epoch_id AND e.id = m.event_id
WHERE e.epoch_id = ? AND e.commit_revision <= ?;
```

The corresponding revision indexes are part of the executable contract.

## Coverage and pinned queries

Coverage versions record fidelity, observed/eligible counts, and reason. Metric
and session responses select coverage at the applied revision and never
coalesce unavailable fields to zero. The required latest-at-revision selector
is:

```sql
SELECT v.*
FROM session_versions v
WHERE v.epoch_id = ? AND v.session_id = ? AND v.revision = (
  SELECT max(v2.revision)
  FROM session_versions v2
  WHERE v2.epoch_id = v.epoch_id
    AND v2.session_id = v.session_id
    AND v2.revision <= ?
);
```

Equivalent selectors apply to source, label, and coverage versions and must use
their descending revision indexes.

## Evidence fingerprints and live availability

Every evidence reference freezes two different 64-character lowercase SHA-256
values:

- `source_prefix_sha256`: exact source bytes `[0,event.byte_end)`;
- `event_fingerprint`: exact record bytes for the referenced event.

They are computed and stored independently and cannot be populated by
substituting one algorithm for the other or by copying the operational
checkpoint hash. Their values may naturally coincide when one record is the
entire covered prefix. The event locator, epoch, applied revision, source ID,
and both fingerprints are pinned facts.

Availability is separate. `evidence_availability_versions` is an append-only
current overlay with `availability`, `observed_at`, and optional
`availability_revision`. Persisted availability changes normally receive a
revision; a read-time revalidation may return a newer observation without
allocating one. The API exposes that distinction through
`availabilityObservedAt` and nullable `availabilityRevision`. Current
availability may change while an older fact revision remains pinned; historical
coverage does not change with it.

## Epoch rebuild and swap

Revisions are strictly monotonic only within an epoch. A changed indexed prefix,
adapter change, immutable-fact correction, or schema-v1 database creates a
separate schema-v2 database and `building` epoch. Validation covers schema
metadata (one active/building schema-v2 epoch with the exact adapter),
`foreign_key_check`, `integrity_check`, gap-free monotonic revisions, all
latest-at-revision selectors, contentless and revision-filtered FTS, both
evidence hashes recomputed from the frozen fixture bytes, and the complete
identity/cardinality/revision golden. Every check is fail-closed. The files are
never attached for a cross-database transaction and SQLite does not rename
them. The validated database keeps a unique immutable filename. Activation
writes and fsyncs a same-directory catalog temporary file, atomically renames
that small file over `active-index`, then fsyncs the directory. A failed build
or validation leaves the catalog naming v1. New requests resolve the catalog on
open; already-open read handles may finish against the old file. Both files
remain recoverable until no old handle exists. Successful replacement emits a
full refresh rather than pretending revisions are comparable across epochs.

The normal rebuild path records one immutable `epoch_validations` row containing
the successful validation digest, then transitions that epoch exactly once from
`building` to `active` with its immutable activation timestamp before switching
the catalog. Failure transitions `building` to `failed`; replacement transitions
`active` to `superseded`. The trusted writer exposes no reverse or same-state
transition. DDL rejects those transitions as defense in depth. Exhaustive
tamper-resistance against manual direct SQL remains outside the demo threat
model.
