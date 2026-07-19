PRAGMA foreign_keys = ON;

CREATE TABLE schema_metadata (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  schema_version INTEGER NOT NULL CHECK (schema_version = 2),
  protocol_version INTEGER NOT NULL CHECK (protocol_version = 1),
  created_at TEXT NOT NULL
);

CREATE TABLE dataset_epochs (
  id TEXT PRIMARY KEY,
  schema_version INTEGER NOT NULL CHECK (schema_version = 2),
  adapter_version TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('building', 'active', 'superseded', 'failed')),
  created_at TEXT NOT NULL,
  activated_at TEXT,
  CHECK ((state IN ('active','superseded') AND activated_at IS NOT NULL) OR (state IN ('building','failed') AND activated_at IS NULL))
);

CREATE UNIQUE INDEX one_active_dataset_epoch
  ON dataset_epochs(state) WHERE state = 'active';

CREATE TABLE epoch_validations (
  epoch_id TEXT PRIMARY KEY REFERENCES dataset_epochs(id),
  validated_at TEXT NOT NULL,
  validation_sha256 TEXT NOT NULL CHECK (length(validation_sha256)=64 AND validation_sha256 NOT GLOB '*[^0-9a-f]*')
);

CREATE TABLE index_revisions (
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  revision INTEGER NOT NULL CHECK (revision > 0),
  committed_at TEXT NOT NULL,
  reason TEXT NOT NULL CHECK (reason IN ('inventory', 'normalize', 'append', 'source_move', 'label', 'lineage', 'availability', 'reconcile', 'rebuild')),
  PRIMARY KEY (epoch_id, revision)
);

CREATE TRIGGER index_revisions_monotonic_insert
BEFORE INSERT ON index_revisions
BEGIN
  SELECT CASE WHEN NEW.revision <> COALESCE((
    SELECT max(revision) + 1 FROM index_revisions WHERE epoch_id = NEW.epoch_id
  ), 1) THEN RAISE(ABORT, 'revision must be the next monotonic value in its epoch') END;
END;

CREATE TABLE source_artifacts (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  source_session_id TEXT NOT NULL,
  segment_fingerprint TEXT NOT NULL CHECK (length(segment_fingerprint) = 64 AND segment_fingerprint NOT GLOB '*[^0-9a-f]*'),
  created_revision INTEGER NOT NULL,
  UNIQUE (epoch_id, id),
  UNIQUE (epoch_id, source_session_id, segment_fingerprint),
  FOREIGN KEY (epoch_id, created_revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE TABLE source_artifact_versions (
  epoch_id TEXT NOT NULL,
  source_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  source_kind TEXT NOT NULL CHECK (source_kind IN ('active_rollout', 'archived_rollout', 'session_index')),
  canonical_path TEXT NOT NULL,
  inode TEXT,
  byte_size INTEGER NOT NULL CHECK (byte_size >= 0),
  mtime_ns INTEGER,
  detected_codex_version TEXT,
  adapter_version TEXT,
  state TEXT NOT NULL CHECK (state IN ('discovered', 'supported', 'indexing', 'current', 'unsupported', 'failed', 'requires_rebuild', 'missing')),
  state_reason TEXT,
  source_evidence_availability TEXT NOT NULL CHECK (source_evidence_availability IN ('available', 'source_missing', 'fingerprint_mismatch', 'unreadable')),
  availability_observed_at TEXT NOT NULL,
  PRIMARY KEY (epoch_id, source_id, revision),
  FOREIGN KEY (epoch_id, source_id) REFERENCES source_artifacts(epoch_id, id) ON DELETE CASCADE,
  FOREIGN KEY (epoch_id, revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE INDEX source_artifact_versions_latest
  ON source_artifact_versions(epoch_id, source_id, revision DESC);
CREATE INDEX source_artifact_versions_path
  ON source_artifact_versions(epoch_id, canonical_path, revision DESC);

-- Operational current state. It is not queried as a historical fact. Every
-- persisted change still belongs to the same transaction as updated_revision.
CREATE TABLE source_checkpoints (
  source_id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL,
  complete_byte_offset INTEGER NOT NULL CHECK (complete_byte_offset >= 0),
  complete_record_ordinal INTEGER NOT NULL CHECK (complete_record_ordinal >= 0),
  observed_size INTEGER NOT NULL CHECK (observed_size >= complete_byte_offset),
  observed_mtime_ns INTEGER,
  prefix_sha256 TEXT NOT NULL CHECK (length(prefix_sha256) = 64 AND prefix_sha256 NOT GLOB '*[^0-9a-f]*'),
  adapter_version TEXT NOT NULL,
  pending_tail_bytes INTEGER NOT NULL DEFAULT 0 CHECK (pending_tail_bytes >= 0),
  updated_at TEXT NOT NULL,
  updated_revision INTEGER NOT NULL,
  FOREIGN KEY (epoch_id, source_id) REFERENCES source_artifacts(epoch_id, id) ON DELETE CASCADE,
  FOREIGN KEY (epoch_id, updated_revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  identity_kind TEXT NOT NULL CHECK (identity_kind IN ('git_remote', 'git_root', 'cwd')),
  canonical_identity TEXT NOT NULL,
  display_name TEXT NOT NULL,
  created_revision INTEGER NOT NULL,
  UNIQUE (epoch_id, id),
  UNIQUE (epoch_id, identity_kind, canonical_identity),
  FOREIGN KEY (epoch_id, created_revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE TABLE project_aliases (
  epoch_id TEXT NOT NULL,
  project_id TEXT NOT NULL,
  alias_kind TEXT NOT NULL CHECK (alias_kind IN ('cwd', 'git_root', 'worktree', 'branch', 'remote')),
  alias_value TEXT NOT NULL,
  observed_revision INTEGER NOT NULL,
  PRIMARY KEY (epoch_id, project_id, alias_kind, alias_value),
  FOREIGN KEY (epoch_id, project_id) REFERENCES projects(epoch_id, id) ON DELETE CASCADE,
  FOREIGN KEY (epoch_id, observed_revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE INDEX project_aliases_revision
  ON project_aliases(epoch_id, observed_revision, project_id);

CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  source_session_id TEXT NOT NULL,
  created_revision INTEGER NOT NULL,
  UNIQUE (epoch_id, id),
  UNIQUE (epoch_id, source_session_id),
  FOREIGN KEY (epoch_id, created_revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE TABLE session_versions (
  epoch_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  project_id TEXT,
  root_work_unit_id TEXT NOT NULL,
  purpose TEXT NOT NULL CHECK (purpose IN ('user', 'spawned', 'inspector_review', 'other', 'orphan')),
  source TEXT,
  originator TEXT,
  earliest_proven_start TEXT,
  latest_terminal_end TEXT,
  lineage_coverage TEXT NOT NULL CHECK (lineage_coverage IN ('exact', 'partial', 'unavailable')),
  inventory_reconciled INTEGER NOT NULL CHECK (inventory_reconciled IN (0, 1)),
  PRIMARY KEY (epoch_id, session_id, revision),
  FOREIGN KEY (epoch_id, session_id) REFERENCES sessions(epoch_id, id) ON DELETE CASCADE,
  FOREIGN KEY (epoch_id, project_id) REFERENCES projects(epoch_id, id),
  FOREIGN KEY (epoch_id, root_work_unit_id) REFERENCES sessions(epoch_id, id),
  FOREIGN KEY (epoch_id, revision) REFERENCES index_revisions(epoch_id, revision),
  CHECK (purpose <> 'orphan' OR inventory_reconciled = 1)
);

CREATE INDEX session_versions_latest
  ON session_versions(epoch_id, session_id, revision DESC);
CREATE INDEX session_versions_root_latest
  ON session_versions(epoch_id, root_work_unit_id, revision DESC, session_id);

CREATE TABLE session_label_versions (
  epoch_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  revision INTEGER NOT NULL,
  title TEXT NOT NULL,
  source_updated_at TEXT NOT NULL,
  source_record_ordinal INTEGER NOT NULL CHECK (source_record_ordinal >= 0),
  PRIMARY KEY (epoch_id, session_id, revision),
  FOREIGN KEY (epoch_id, session_id) REFERENCES sessions(epoch_id, id) ON DELETE CASCADE,
  FOREIGN KEY (epoch_id, revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE INDEX session_label_versions_latest
  ON session_label_versions(epoch_id, session_id, revision DESC);

CREATE TABLE source_segments (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  session_id TEXT NOT NULL,
  source_id TEXT NOT NULL,
  segment_fingerprint TEXT NOT NULL CHECK (length(segment_fingerprint) = 64 AND segment_fingerprint NOT GLOB '*[^0-9a-f]*'),
  source_order_key TEXT NOT NULL CHECK (
    length(source_order_key) = 85 AND substr(source_order_key, 21, 1) = ':' AND
    substr(source_order_key, 1, 20) NOT GLOB '*[^0-9]*' AND
    substr(source_order_key, 22, 64) NOT GLOB '*[^0-9a-f]*'
  ),
  source_record_start INTEGER NOT NULL CHECK (source_record_start >= 0),
  created_revision INTEGER NOT NULL,
  UNIQUE (epoch_id, id),
  UNIQUE (epoch_id, session_id, segment_fingerprint),
  UNIQUE (epoch_id, session_id, source_order_key, source_record_start),
  FOREIGN KEY (epoch_id, session_id) REFERENCES sessions(epoch_id, id),
  FOREIGN KEY (epoch_id, source_id) REFERENCES source_artifacts(epoch_id, id),
  FOREIGN KEY (epoch_id, created_revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE INDEX source_segments_order
  ON source_segments(epoch_id, session_id, source_order_key, source_record_start);

CREATE TABLE turns (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  session_id TEXT NOT NULL,
  source_turn_id TEXT NOT NULL,
  source_order_key TEXT NOT NULL CHECK (
    length(source_order_key) = 106 AND substr(source_order_key, 86, 1) = ':' AND
    substr(source_order_key, 1, 85) NOT GLOB '*[^0-9a-f:]*' AND
    substr(source_order_key, 87, 20) NOT GLOB '*[^0-9]*'
  ),
  state TEXT NOT NULL CHECK (state IN ('completed', 'aborted', 'interrupted', 'reconciled_truncated')),
  started_at TEXT,
  terminal_at TEXT NOT NULL,
  completed_at TEXT,
  model TEXT,
  reasoning_effort TEXT,
  commit_revision INTEGER NOT NULL,
  UNIQUE (epoch_id, id),
  UNIQUE (epoch_id, session_id, source_turn_id),
  UNIQUE (epoch_id, session_id, source_order_key),
  FOREIGN KEY (epoch_id, session_id) REFERENCES sessions(epoch_id, id),
  FOREIGN KEY (epoch_id, commit_revision) REFERENCES index_revisions(epoch_id, revision),
  CHECK ((state = 'completed' AND completed_at IS NOT NULL) OR (state <> 'completed' AND completed_at IS NULL))
);

CREATE INDEX turns_revision
  ON turns(epoch_id, commit_revision, session_id, source_order_key);

CREATE TABLE terminal_watermarks (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  source_id TEXT NOT NULL,
  segment_id TEXT NOT NULL,
  terminal_event_id TEXT NOT NULL,
  terminal_revision INTEGER NOT NULL,
  max_record_ordinal INTEGER NOT NULL CHECK (max_record_ordinal >= 0),
  observed_at TEXT NOT NULL,
  UNIQUE (epoch_id, id),
  FOREIGN KEY (epoch_id, session_id) REFERENCES sessions(epoch_id, id),
  FOREIGN KEY (epoch_id, source_id) REFERENCES source_artifacts(epoch_id, id),
  FOREIGN KEY (epoch_id, segment_id) REFERENCES source_segments(epoch_id, id),
  FOREIGN KEY (epoch_id, terminal_event_id) REFERENCES events(epoch_id, id),
  FOREIGN KEY (epoch_id, terminal_revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE TRIGGER terminal_watermark_parent_insert
BEFORE INSERT ON terminal_watermarks
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM events e
    JOIN turns t ON t.epoch_id=e.epoch_id AND t.id=e.turn_id
    JOIN source_segments s ON s.epoch_id=e.epoch_id AND s.id=e.segment_id
    WHERE s.epoch_id = NEW.epoch_id AND s.id = NEW.segment_id
      AND s.session_id = NEW.session_id AND s.source_id = NEW.source_id
      AND e.id = NEW.terminal_event_id AND e.event_kind = 'task_complete'
      AND e.record_ordinal <= NEW.max_record_ordinal
      AND e.commit_revision = NEW.terminal_revision
      AND t.commit_revision = NEW.terminal_revision
  ) THEN RAISE(ABORT, 'watermark must be proven by a terminal event in the same source segment') END;
END;

CREATE TABLE events (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  segment_id TEXT NOT NULL,
  turn_id TEXT,
  record_ordinal INTEGER NOT NULL CHECK (record_ordinal >= 0),
  semantic_phase TEXT NOT NULL CHECK (semantic_phase <> '' AND instr(semantic_phase, ':') = 0),
  event_kind TEXT NOT NULL CHECK (event_kind <> ''),
  observed_at TEXT NOT NULL,
  byte_start INTEGER NOT NULL CHECK (byte_start >= 0),
  byte_end INTEGER NOT NULL CHECK (byte_end > byte_start),
  content_sha256 TEXT NOT NULL CHECK (length(content_sha256) = 64 AND content_sha256 NOT GLOB '*[^0-9a-f]*'),
  payload_length INTEGER NOT NULL CHECK (payload_length >= 0),
  adapter_version TEXT NOT NULL,
  terminal_watermark_id TEXT,
  commit_revision INTEGER NOT NULL,
  UNIQUE (epoch_id, id),
  UNIQUE (epoch_id, segment_id, record_ordinal, semantic_phase),
  FOREIGN KEY (epoch_id, segment_id) REFERENCES source_segments(epoch_id, id),
  FOREIGN KEY (epoch_id, turn_id) REFERENCES turns(epoch_id, id),
  FOREIGN KEY (epoch_id, terminal_watermark_id) REFERENCES terminal_watermarks(epoch_id, id),
  FOREIGN KEY (epoch_id, commit_revision) REFERENCES index_revisions(epoch_id, revision),
  CHECK (
    (turn_id IS NOT NULL AND terminal_watermark_id IS NULL) OR
    (turn_id IS NULL AND terminal_watermark_id IS NOT NULL AND event_kind IN ('session_meta', 'source_boundary', 'capacity_observation', 'compaction_boundary', 'lineage_observation'))
  )
);

CREATE INDEX events_revision_order
  ON events(epoch_id, commit_revision, segment_id, record_ordinal, semantic_phase);
CREATE INDEX events_turn_revision_order
  ON events(epoch_id, turn_id, commit_revision, record_ordinal, semantic_phase);

CREATE TRIGGER events_turn_revision_insert
BEFORE INSERT ON events WHEN NEW.turn_id IS NOT NULL
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM turns t
    WHERE t.epoch_id = NEW.epoch_id AND t.id = NEW.turn_id AND t.commit_revision = NEW.commit_revision
  ) THEN RAISE(ABORT, 'turn-bound event revision must equal terminal turn revision') END;
END;

CREATE TRIGGER events_null_turn_watermark_insert
BEFORE INSERT ON events WHEN NEW.turn_id IS NULL
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM terminal_watermarks w
    WHERE w.epoch_id = NEW.epoch_id AND w.id = NEW.terminal_watermark_id
      AND w.segment_id = NEW.segment_id
      AND NEW.record_ordinal <= w.max_record_ordinal
      AND w.terminal_revision <= NEW.commit_revision
  ) THEN RAISE(ABORT, 'null-turn event must be within a relational terminal watermark') END;
END;

CREATE TABLE lineage_edges (
  epoch_id TEXT NOT NULL,
  parent_session_id TEXT NOT NULL,
  child_session_id TEXT NOT NULL,
  edge_kind TEXT NOT NULL CHECK (edge_kind IN ('spawned', 'continued_as', 'forked_from', 'resumed')),
  spawning_turn_id TEXT,
  source_event_id TEXT NOT NULL,
  commit_revision INTEGER NOT NULL,
  PRIMARY KEY (epoch_id, parent_session_id, child_session_id, edge_kind),
  FOREIGN KEY (epoch_id, parent_session_id) REFERENCES sessions(epoch_id, id),
  FOREIGN KEY (epoch_id, child_session_id) REFERENCES sessions(epoch_id, id),
  FOREIGN KEY (epoch_id, spawning_turn_id) REFERENCES turns(epoch_id, id),
  FOREIGN KEY (epoch_id, source_event_id) REFERENCES events(epoch_id, id),
  FOREIGN KEY (epoch_id, commit_revision) REFERENCES index_revisions(epoch_id, revision),
  CHECK (parent_session_id <> child_session_id)
);

CREATE INDEX lineage_edges_revision
  ON lineage_edges(epoch_id, commit_revision, parent_session_id, child_session_id);

CREATE TRIGGER lineage_session_version_insert
BEFORE INSERT ON lineage_edges
BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM sessions p WHERE p.epoch_id=NEW.epoch_id AND p.id=NEW.parent_session_id AND p.created_revision<=NEW.commit_revision) OR NOT EXISTS (SELECT 1 FROM sessions c WHERE c.epoch_id=NEW.epoch_id AND c.id=NEW.child_session_id AND c.created_revision<=NEW.commit_revision) THEN RAISE(ABORT,'lineage session is from the future') END;
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM session_versions sv
    WHERE sv.epoch_id = NEW.epoch_id AND sv.session_id = NEW.child_session_id AND sv.revision = NEW.commit_revision
  ) THEN RAISE(ABORT, 'lineage proof and corrected child session version must share revision') END;
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM events e JOIN source_segments s
      ON s.epoch_id=e.epoch_id AND s.id=e.segment_id
    WHERE e.epoch_id = NEW.epoch_id AND e.id = NEW.source_event_id
      AND e.event_kind = 'lineage_observation' AND e.commit_revision = NEW.commit_revision
      AND s.session_id IN (NEW.parent_session_id, NEW.child_session_id)
  ) THEN RAISE(ABORT, 'lineage edge must inherit its proof event revision') END;
  SELECT CASE WHEN NEW.spawning_turn_id IS NOT NULL AND NOT EXISTS (
    SELECT 1 FROM turns t
    WHERE t.epoch_id = NEW.epoch_id AND t.id = NEW.spawning_turn_id
      AND t.session_id = NEW.parent_session_id
  ) THEN RAISE(ABORT, 'lineage spawning turn must belong to parent session') END;
END;

-- Immutable child facts carry no duplicate revision column. Direct queries
-- must join their revision-bearing parent event (or terminal turn for usage).
CREATE TABLE messages (
  event_id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'developer', 'tool')),
  phase TEXT NOT NULL,
  source_message_id TEXT,
  readable INTEGER NOT NULL CHECK (readable IN (0, 1)),
  content_length INTEGER NOT NULL CHECK (content_length >= 0),
  content_sha256 TEXT NOT NULL CHECK (length(content_sha256) = 64 AND content_sha256 NOT GLOB '*[^0-9a-f]*'),
  FOREIGN KEY (epoch_id, event_id) REFERENCES events(epoch_id, id) ON DELETE CASCADE
);

CREATE TABLE tool_calls (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  session_id TEXT NOT NULL,
  turn_id TEXT NOT NULL,
  source_call_id TEXT NOT NULL,
  semantic_phase TEXT NOT NULL CHECK (semantic_phase IN ('request', 'result')),
  event_id TEXT NOT NULL,
  tool_name TEXT NOT NULL,
  tool_family TEXT NOT NULL,
  status TEXT,
  exit_code INTEGER,
  duration_ms INTEGER CHECK (duration_ms IS NULL OR duration_ms >= 0),
  UNIQUE (epoch_id, session_id, source_call_id, semantic_phase),
  FOREIGN KEY (epoch_id, session_id) REFERENCES sessions(epoch_id, id),
  FOREIGN KEY (epoch_id, turn_id) REFERENCES turns(epoch_id, id),
  FOREIGN KEY (epoch_id, event_id) REFERENCES events(epoch_id, id)
);

CREATE TRIGGER tool_event_parent_insert
BEFORE INSERT ON tool_calls
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM events e JOIN turns t
      ON t.epoch_id = e.epoch_id AND t.id = e.turn_id
    WHERE e.id = NEW.event_id AND e.epoch_id = NEW.epoch_id
      AND e.turn_id = NEW.turn_id AND t.session_id = NEW.session_id
  ) THEN RAISE(ABORT, 'tool fact must inherit its parent event and turn') END;
END;

CREATE TABLE turn_usage (
  epoch_id TEXT NOT NULL,
  turn_id TEXT NOT NULL,
  formula_version INTEGER NOT NULL CHECK (formula_version = 1),
  normalization_kind TEXT NOT NULL CHECK (normalization_kind IN ('last_turn', 'cumulative_delta')),
  input_tokens INTEGER CHECK (input_tokens IS NULL OR input_tokens >= 0),
  cached_input_tokens INTEGER CHECK (cached_input_tokens IS NULL OR cached_input_tokens >= 0),
  output_tokens INTEGER CHECK (output_tokens IS NULL OR output_tokens >= 0),
  reasoning_output_tokens INTEGER CHECK (reasoning_output_tokens IS NULL OR reasoning_output_tokens >= 0),
  total_tokens INTEGER CHECK (total_tokens IS NULL OR total_tokens >= 0),
  residual_tokens INTEGER CHECK (residual_tokens IS NULL OR residual_tokens >= 0),
  fidelity TEXT NOT NULL CHECK (fidelity IN ('exact', 'derived', 'unavailable')),
  PRIMARY KEY (epoch_id, turn_id, formula_version),
  FOREIGN KEY (epoch_id, turn_id) REFERENCES turns(epoch_id, id) ON DELETE CASCADE,
  CHECK (cached_input_tokens IS NULL OR input_tokens IS NULL OR cached_input_tokens <= input_tokens),
  CHECK (reasoning_output_tokens IS NULL OR output_tokens IS NULL OR reasoning_output_tokens <= output_tokens)
);

CREATE TABLE capacity_observations (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  event_id TEXT NOT NULL,
  limit_id TEXT NOT NULL,
  window_minutes INTEGER CHECK (window_minutes IS NULL OR window_minutes > 0),
  used_percent REAL CHECK (used_percent IS NULL OR (used_percent >= 0 AND used_percent <= 100)),
  remaining_percent REAL CHECK (remaining_percent IS NULL OR (remaining_percent >= 0 AND remaining_percent <= 100)),
  resets_at TEXT,
  observed_at TEXT NOT NULL,
  UNIQUE (epoch_id, event_id, limit_id, window_minutes),
  FOREIGN KEY (epoch_id, event_id) REFERENCES events(epoch_id, id)
);

CREATE TABLE compactions (
  event_id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  turn_id TEXT,
  trigger_kind TEXT,
  recorded_summary_length INTEGER CHECK (recorded_summary_length IS NULL OR recorded_summary_length >= 0),
  recorded_summary_sha256 TEXT CHECK (recorded_summary_sha256 IS NULL OR (length(recorded_summary_sha256) = 64 AND recorded_summary_sha256 NOT GLOB '*[^0-9a-f]*')),
  FOREIGN KEY (epoch_id, event_id) REFERENCES events(epoch_id, id) ON DELETE CASCADE,
  FOREIGN KEY (epoch_id, session_id) REFERENCES sessions(epoch_id, id),
  FOREIGN KEY (epoch_id, turn_id) REFERENCES turns(epoch_id, id)
);

CREATE TRIGGER compaction_parent_insert
BEFORE INSERT ON compactions
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM events e
    LEFT JOIN turns t ON t.epoch_id=e.epoch_id AND t.id=e.turn_id
    LEFT JOIN terminal_watermarks w
      ON w.epoch_id=e.epoch_id AND w.id=e.terminal_watermark_id
    WHERE e.epoch_id=NEW.epoch_id AND e.id=NEW.event_id
      AND ((NEW.turn_id IS NULL AND e.turn_id IS NULL AND w.session_id=NEW.session_id) OR
           (NEW.turn_id=e.turn_id AND t.session_id=NEW.session_id))
  ) THEN RAISE(ABORT, 'compaction must inherit event turn and session') END;
END;

CREATE TABLE evidence_refs (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  event_id TEXT NOT NULL,
  source_id TEXT NOT NULL,
  source_prefix_sha256 TEXT NOT NULL CHECK (length(source_prefix_sha256) = 64 AND source_prefix_sha256 NOT GLOB '*[^0-9a-f]*'),
  event_fingerprint TEXT NOT NULL CHECK (length(event_fingerprint) = 64 AND event_fingerprint NOT GLOB '*[^0-9a-f]*'),
  UNIQUE (epoch_id, id),
  UNIQUE (epoch_id, event_id, event_fingerprint),
  FOREIGN KEY (epoch_id, event_id) REFERENCES events(epoch_id, id),
  FOREIGN KEY (epoch_id, source_id) REFERENCES source_artifacts(epoch_id, id)
);

CREATE TRIGGER evidence_source_parent_insert
BEFORE INSERT ON evidence_refs
BEGIN
  SELECT CASE WHEN NOT EXISTS (
    SELECT 1 FROM events e JOIN source_segments s
      ON s.epoch_id=e.epoch_id AND s.id=e.segment_id
    WHERE e.epoch_id=NEW.epoch_id AND e.id=NEW.event_id
      AND s.source_id=NEW.source_id
  ) THEN RAISE(ABORT, 'evidence source must own its parent event segment') END;
END;

CREATE TABLE evidence_availability_versions (
  epoch_id TEXT NOT NULL,
  evidence_id TEXT NOT NULL,
  observed_at TEXT NOT NULL,
  availability TEXT NOT NULL CHECK (availability IN ('available', 'source_missing', 'fingerprint_mismatch', 'unreadable')),
  availability_revision INTEGER,
  detail_code TEXT,
  PRIMARY KEY (epoch_id, evidence_id, observed_at),
  FOREIGN KEY (epoch_id, evidence_id) REFERENCES evidence_refs(epoch_id, id) ON DELETE CASCADE,
  FOREIGN KEY (epoch_id, availability_revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE INDEX evidence_availability_latest
  ON evidence_availability_versions(epoch_id, evidence_id, observed_at DESC);
CREATE INDEX evidence_availability_revision
  ON evidence_availability_versions(epoch_id, availability_revision, evidence_id);

CREATE TABLE coverage_observation_versions (
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  scope_kind TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  field_key TEXT NOT NULL,
  revision INTEGER NOT NULL,
  fidelity TEXT NOT NULL CHECK (fidelity IN ('exact', 'derived', 'unavailable')),
  observed_count INTEGER NOT NULL CHECK (observed_count >= 0),
  eligible_count INTEGER NOT NULL CHECK (eligible_count >= observed_count),
  reason TEXT,
  PRIMARY KEY (epoch_id, scope_kind, scope_id, field_key, revision),
  FOREIGN KEY (epoch_id, revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE INDEX coverage_observation_versions_latest
  ON coverage_observation_versions(epoch_id, scope_kind, scope_id, field_key, revision DESC);

CREATE TABLE event_search_documents (
  rowid INTEGER PRIMARY KEY,
  epoch_id TEXT NOT NULL,
  event_id TEXT NOT NULL,
  match_category TEXT NOT NULL,
  UNIQUE (epoch_id, event_id, match_category),
  FOREIGN KEY (epoch_id, event_id) REFERENCES events(epoch_id, id) ON DELETE CASCADE
);

CREATE VIRTUAL TABLE event_search USING fts5(
  content,
  content = '',
  contentless_delete = 1,
  tokenize = 'unicode61'
);

CREATE TABLE session_label_search_documents (
  rowid INTEGER PRIMARY KEY,
  epoch_id TEXT NOT NULL,
  session_id TEXT NOT NULL,
  label_revision INTEGER NOT NULL,
  match_category TEXT NOT NULL CHECK (match_category = 'session_title'),
  UNIQUE (epoch_id, session_id, label_revision),
  FOREIGN KEY (epoch_id, session_id, label_revision)
    REFERENCES session_label_versions(epoch_id, session_id, revision) ON DELETE CASCADE
);

CREATE INDEX session_label_search_revision
  ON session_label_search_documents(epoch_id, session_id, label_revision DESC);

CREATE VIRTUAL TABLE session_label_search USING fts5(
  content,
  content = '',
  contentless_delete = 1,
  tokenize = 'unicode61'
);

-- Temporal visibility checks. Foreign keys prove existence; these triggers
-- additionally prove that every parent was visible by the child's revision.
CREATE TRIGGER source_version_visible_parent BEFORE INSERT ON source_artifact_versions BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM source_artifacts p WHERE p.epoch_id=NEW.epoch_id AND p.id=NEW.source_id AND p.created_revision<=NEW.revision) THEN RAISE(ABORT,'source version parent is from the future') END;
END;
CREATE TRIGGER checkpoint_visible_parent BEFORE INSERT ON source_checkpoints BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM source_artifacts p WHERE p.epoch_id=NEW.epoch_id AND p.id=NEW.source_id AND p.created_revision<=NEW.updated_revision) THEN RAISE(ABORT,'checkpoint parent is from the future') END;
END;
CREATE TRIGGER checkpoint_visible_parent_update BEFORE UPDATE ON source_checkpoints BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM source_artifacts p WHERE p.epoch_id=NEW.epoch_id AND p.id=NEW.source_id AND p.created_revision<=NEW.updated_revision) THEN RAISE(ABORT,'checkpoint parent is from the future') END;
  SELECT CASE WHEN NEW.epoch_id<>OLD.epoch_id OR NEW.source_id<>OLD.source_id OR NEW.adapter_version<>OLD.adapter_version OR NEW.updated_revision<=OLD.updated_revision OR NEW.complete_byte_offset<OLD.complete_byte_offset OR NEW.complete_record_ordinal<OLD.complete_record_ordinal OR NEW.observed_size<OLD.observed_size THEN RAISE(ABORT,'checkpoint update must advance revision and append watermarks') END;
END;
CREATE TRIGGER project_alias_visible_parent BEFORE INSERT ON project_aliases BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM projects p WHERE p.epoch_id=NEW.epoch_id AND p.id=NEW.project_id AND p.created_revision<=NEW.observed_revision) THEN RAISE(ABORT,'project alias parent is from the future') END;
END;
CREATE TRIGGER session_version_visible_parents BEFORE INSERT ON session_versions BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM sessions s WHERE s.epoch_id=NEW.epoch_id AND s.id=NEW.session_id AND s.created_revision<=NEW.revision) THEN RAISE(ABORT,'session version parent is from the future') END;
  SELECT CASE WHEN NEW.project_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM projects p WHERE p.epoch_id=NEW.epoch_id AND p.id=NEW.project_id AND p.created_revision<=NEW.revision) THEN RAISE(ABORT,'session project is from the future') END;
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM sessions r WHERE r.epoch_id=NEW.epoch_id AND r.id=NEW.root_work_unit_id AND r.created_revision<=NEW.revision) THEN RAISE(ABORT,'session root is from the future') END;
END;
CREATE TRIGGER session_label_visible_parent BEFORE INSERT ON session_label_versions BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM sessions s WHERE s.epoch_id=NEW.epoch_id AND s.id=NEW.session_id AND s.created_revision<=NEW.revision) THEN RAISE(ABORT,'session label parent is from the future') END;
END;
CREATE TRIGGER source_segment_visible_parents BEFORE INSERT ON source_segments BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM sessions s WHERE s.epoch_id=NEW.epoch_id AND s.id=NEW.session_id AND s.created_revision<=NEW.created_revision) OR NOT EXISTS (SELECT 1 FROM source_artifacts a WHERE a.epoch_id=NEW.epoch_id AND a.id=NEW.source_id AND a.created_revision<=NEW.created_revision) THEN RAISE(ABORT,'segment parent is from the future') END;
END;
CREATE TRIGGER turn_visible_parent BEFORE INSERT ON turns BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM sessions s WHERE s.epoch_id=NEW.epoch_id AND s.id=NEW.session_id AND s.created_revision<=NEW.commit_revision) THEN RAISE(ABORT,'turn parent is from the future') END;
END;
CREATE TRIGGER event_visible_parent BEFORE INSERT ON events BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM source_segments s WHERE s.epoch_id=NEW.epoch_id AND s.id=NEW.segment_id AND s.created_revision<=NEW.commit_revision) THEN RAISE(ABORT,'event segment is from the future') END;
END;
CREATE TRIGGER availability_visible_parent BEFORE INSERT ON evidence_availability_versions WHEN NEW.availability_revision IS NOT NULL BEGIN
  SELECT CASE WHEN NOT EXISTS (SELECT 1 FROM evidence_refs r JOIN events e ON e.epoch_id=r.epoch_id AND e.id=r.event_id WHERE r.epoch_id=NEW.epoch_id AND r.id=NEW.evidence_id AND e.commit_revision<=NEW.availability_revision) THEN RAISE(ABORT,'availability evidence is from the future') END;
END;

-- Coverage scopes are closed and must name a visible entity at the observation
-- revision. Epoch scope uses the epoch id itself.
CREATE TRIGGER coverage_visible_scope BEFORE INSERT ON coverage_observation_versions BEGIN
  SELECT CASE
    WHEN NEW.scope_kind='epoch' AND NEW.scope_id=NEW.epoch_id THEN NULL
    WHEN NEW.scope_kind='source' AND EXISTS (SELECT 1 FROM source_artifacts x WHERE x.epoch_id=NEW.epoch_id AND x.id=NEW.scope_id AND x.created_revision<=NEW.revision) THEN NULL
    WHEN NEW.scope_kind='project' AND EXISTS (SELECT 1 FROM projects x WHERE x.epoch_id=NEW.epoch_id AND x.id=NEW.scope_id AND x.created_revision<=NEW.revision) THEN NULL
    WHEN NEW.scope_kind='session' AND EXISTS (SELECT 1 FROM sessions x WHERE x.epoch_id=NEW.epoch_id AND x.id=NEW.scope_id AND x.created_revision<=NEW.revision) THEN NULL
    WHEN NEW.scope_kind='turn' AND EXISTS (SELECT 1 FROM turns x WHERE x.epoch_id=NEW.epoch_id AND x.id=NEW.scope_id AND x.commit_revision<=NEW.revision) THEN NULL
    WHEN NEW.scope_kind='event' AND EXISTS (SELECT 1 FROM events x WHERE x.epoch_id=NEW.epoch_id AND x.id=NEW.scope_id AND x.commit_revision<=NEW.revision) THEN NULL
    ELSE RAISE(ABORT,'coverage scope is invalid or from the future') END;
END;

CREATE TRIGGER dataset_epoch_state_update BEFORE UPDATE ON dataset_epochs BEGIN
  SELECT CASE WHEN NOT (
    (OLD.state='building' AND NEW.state='active' AND OLD.activated_at IS NULL AND NEW.activated_at IS NOT NULL) OR
    (OLD.state='building' AND NEW.state='failed' AND OLD.activated_at IS NULL AND NEW.activated_at IS NULL) OR
    (OLD.state='active' AND NEW.state='superseded' AND NEW.activated_at=OLD.activated_at)
  ) OR NEW.id<>OLD.id OR NEW.schema_version<>OLD.schema_version OR NEW.adapter_version<>OLD.adapter_version OR NEW.created_at<>OLD.created_at
  THEN RAISE(ABORT,'invalid dataset epoch transition') END;
  SELECT CASE WHEN OLD.state='building' AND NEW.state='active' AND NOT EXISTS (SELECT 1 FROM epoch_validations v WHERE v.epoch_id=OLD.id) THEN RAISE(ABORT,'epoch activation requires validated candidate marker') END;
END;

-- Active epochs cannot be backfilled after a later revision exists. Building
-- epochs may replay validated historical revisions in order during rebuild.
CREATE TRIGGER current_source_artifact BEFORE INSERT ON source_artifacts WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.created_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_source_version BEFORE INSERT ON source_artifact_versions WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_checkpoint_insert BEFORE INSERT ON source_checkpoints WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.updated_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_checkpoint_update BEFORE UPDATE ON source_checkpoints WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.updated_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_project BEFORE INSERT ON projects WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.created_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_project_alias BEFORE INSERT ON project_aliases WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.observed_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_session BEFORE INSERT ON sessions WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.created_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_session_version BEFORE INSERT ON session_versions WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_session_label BEFORE INSERT ON session_label_versions WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_segment BEFORE INSERT ON source_segments WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.created_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_turn BEFORE INSERT ON turns WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.commit_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_event BEFORE INSERT ON events WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.commit_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_watermark BEFORE INSERT ON terminal_watermarks WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.terminal_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_lineage BEFORE INSERT ON lineage_edges WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.commit_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_availability BEFORE INSERT ON evidence_availability_versions WHEN NEW.availability_revision IS NOT NULL AND (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.availability_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_coverage BEFORE INSERT ON coverage_observation_versions WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical backfill forbidden'); END;
CREATE TRIGGER current_message BEFORE INSERT ON messages WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND (SELECT commit_revision FROM events WHERE epoch_id=NEW.epoch_id AND id=NEW.event_id)<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical child backfill forbidden'); END;
CREATE TRIGGER current_tool BEFORE INSERT ON tool_calls WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND (SELECT commit_revision FROM events WHERE epoch_id=NEW.epoch_id AND id=NEW.event_id)<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical child backfill forbidden'); END;
CREATE TRIGGER current_usage BEFORE INSERT ON turn_usage WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND (SELECT commit_revision FROM turns WHERE epoch_id=NEW.epoch_id AND id=NEW.turn_id)<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical child backfill forbidden'); END;
CREATE TRIGGER current_capacity BEFORE INSERT ON capacity_observations WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND (SELECT commit_revision FROM events WHERE epoch_id=NEW.epoch_id AND id=NEW.event_id)<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical child backfill forbidden'); END;
CREATE TRIGGER current_compaction BEFORE INSERT ON compactions WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND (SELECT commit_revision FROM events WHERE epoch_id=NEW.epoch_id AND id=NEW.event_id)<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical child backfill forbidden'); END;
CREATE TRIGGER current_evidence BEFORE INSERT ON evidence_refs WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND (SELECT commit_revision FROM events WHERE epoch_id=NEW.epoch_id AND id=NEW.event_id)<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical child backfill forbidden'); END;
CREATE TRIGGER current_event_search_doc BEFORE INSERT ON event_search_documents WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND (SELECT commit_revision FROM events WHERE epoch_id=NEW.epoch_id AND id=NEW.event_id)<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical child backfill forbidden'); END;
CREATE TRIGGER current_label_search_doc BEFORE INSERT ON session_label_search_documents WHEN (SELECT state FROM dataset_epochs WHERE id=NEW.epoch_id)='active' AND NEW.label_revision<>(SELECT max(revision) FROM index_revisions WHERE epoch_id=NEW.epoch_id) BEGIN SELECT RAISE(ABORT,'historical child backfill forbidden'); END;

-- Query-visible facts and versions are append-only. Rebuilds replace the whole
-- database through the catalog; only operational checkpoints and epoch state
-- have narrowly scoped updates.
CREATE TRIGGER immutable_index_revisions_u BEFORE UPDATE ON index_revisions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_index_revisions_d BEFORE DELETE ON index_revisions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_source_artifacts_u BEFORE UPDATE ON source_artifacts BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_source_artifacts_d BEFORE DELETE ON source_artifacts BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_source_versions_u BEFORE UPDATE ON source_artifact_versions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_source_versions_d BEFORE DELETE ON source_artifact_versions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_projects_u BEFORE UPDATE ON projects BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_projects_d BEFORE DELETE ON projects BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_project_aliases_u BEFORE UPDATE ON project_aliases BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_project_aliases_d BEFORE DELETE ON project_aliases BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_sessions_u BEFORE UPDATE ON sessions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_sessions_d BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_session_versions_u BEFORE UPDATE ON session_versions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_session_versions_d BEFORE DELETE ON session_versions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_session_labels_u BEFORE UPDATE ON session_label_versions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_session_labels_d BEFORE DELETE ON session_label_versions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_segments_u BEFORE UPDATE ON source_segments BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_segments_d BEFORE DELETE ON source_segments BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_turns_u BEFORE UPDATE ON turns BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_turns_d BEFORE DELETE ON turns BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_watermarks_u BEFORE UPDATE ON terminal_watermarks BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_watermarks_d BEFORE DELETE ON terminal_watermarks BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_events_u BEFORE UPDATE ON events BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_events_d BEFORE DELETE ON events BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_lineage_u BEFORE UPDATE ON lineage_edges BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_lineage_d BEFORE DELETE ON lineage_edges BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_messages_u BEFORE UPDATE ON messages BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_messages_d BEFORE DELETE ON messages BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_tools_u BEFORE UPDATE ON tool_calls BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_tools_d BEFORE DELETE ON tool_calls BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_usage_u BEFORE UPDATE ON turn_usage BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_usage_d BEFORE DELETE ON turn_usage BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_capacity_u BEFORE UPDATE ON capacity_observations BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_capacity_d BEFORE DELETE ON capacity_observations BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_compactions_u BEFORE UPDATE ON compactions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_compactions_d BEFORE DELETE ON compactions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_evidence_u BEFORE UPDATE ON evidence_refs BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_evidence_d BEFORE DELETE ON evidence_refs BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_availability_u BEFORE UPDATE ON evidence_availability_versions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_availability_d BEFORE DELETE ON evidence_availability_versions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_coverage_u BEFORE UPDATE ON coverage_observation_versions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_coverage_d BEFORE DELETE ON coverage_observation_versions BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_event_search_docs_u BEFORE UPDATE ON event_search_documents BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_event_search_docs_d BEFORE DELETE ON event_search_documents BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_label_search_docs_u BEFORE UPDATE ON session_label_search_documents BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_label_search_docs_d BEFORE DELETE ON session_label_search_documents BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_schema_metadata_u BEFORE UPDATE ON schema_metadata BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_schema_metadata_d BEFORE DELETE ON schema_metadata BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_checkpoints_d BEFORE DELETE ON source_checkpoints BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_dataset_epochs_d BEFORE DELETE ON dataset_epochs BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_epoch_validations_u BEFORE UPDATE ON epoch_validations BEGIN SELECT RAISE(ABORT,'immutable'); END;
CREATE TRIGGER immutable_epoch_validations_d BEFORE DELETE ON epoch_validations BEGIN SELECT RAISE(ABORT,'immutable'); END;

INSERT INTO schema_metadata(singleton, schema_version, protocol_version, created_at)
VALUES (1, 2, 1, '2026-07-18T00:00:00Z');
