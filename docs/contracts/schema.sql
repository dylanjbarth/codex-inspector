PRAGMA foreign_keys = ON;

CREATE TABLE schema_metadata (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  schema_version INTEGER NOT NULL CHECK (schema_version = 1),
  protocol_version INTEGER NOT NULL CHECK (protocol_version = 1),
  created_at TEXT NOT NULL
);

CREATE TABLE dataset_epochs (
  id TEXT PRIMARY KEY,
  schema_version INTEGER NOT NULL,
  adapter_version TEXT NOT NULL,
  state TEXT NOT NULL CHECK (state IN ('building', 'active', 'superseded', 'failed')),
  created_at TEXT NOT NULL,
  activated_at TEXT,
  CHECK ((state = 'active' AND activated_at IS NOT NULL) OR state != 'active')
);

CREATE UNIQUE INDEX one_active_dataset_epoch
  ON dataset_epochs(state) WHERE state = 'active';

CREATE TABLE index_revisions (
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  revision INTEGER NOT NULL CHECK (revision > 0),
  committed_at TEXT NOT NULL,
  reason TEXT NOT NULL,
  PRIMARY KEY (epoch_id, revision)
);

CREATE TABLE source_artifacts (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  source_kind TEXT NOT NULL CHECK (source_kind IN ('active_rollout', 'archived_rollout', 'session_index')),
  source_session_id TEXT NOT NULL,
  segment_fingerprint TEXT NOT NULL,
  canonical_path TEXT NOT NULL,
  inode TEXT,
  byte_size INTEGER NOT NULL CHECK (byte_size >= 0),
  mtime_ns INTEGER,
  detected_codex_version TEXT,
  adapter_version TEXT,
  state TEXT NOT NULL CHECK (state IN ('discovered', 'supported', 'indexing', 'current', 'unsupported', 'failed', 'requires_rebuild')),
  state_reason TEXT,
  evidence_available INTEGER NOT NULL DEFAULT 1 CHECK (evidence_available IN (0, 1)),
  first_seen_at TEXT NOT NULL,
  last_seen_at TEXT NOT NULL,
  UNIQUE (epoch_id, source_session_id, segment_fingerprint)
);

CREATE INDEX source_artifacts_path ON source_artifacts(canonical_path);

CREATE TABLE source_checkpoints (
  source_id TEXT PRIMARY KEY REFERENCES source_artifacts(id) ON DELETE CASCADE,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  complete_byte_offset INTEGER NOT NULL CHECK (complete_byte_offset >= 0),
  complete_record_ordinal INTEGER NOT NULL CHECK (complete_record_ordinal >= 0),
  observed_size INTEGER NOT NULL CHECK (observed_size >= complete_byte_offset),
  observed_mtime_ns INTEGER,
  prefix_sha256 TEXT NOT NULL,
  adapter_version TEXT NOT NULL,
  pending_tail_bytes INTEGER NOT NULL DEFAULT 0 CHECK (pending_tail_bytes >= 0),
  updated_revision INTEGER NOT NULL,
  FOREIGN KEY (epoch_id, updated_revision) REFERENCES index_revisions(epoch_id, revision)
);

CREATE TABLE projects (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  identity_kind TEXT NOT NULL CHECK (identity_kind IN ('git_remote', 'git_root', 'cwd')),
  canonical_identity TEXT NOT NULL,
  display_name TEXT,
  UNIQUE (epoch_id, identity_kind, canonical_identity)
);

CREATE TABLE project_aliases (
  project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  alias_kind TEXT NOT NULL CHECK (alias_kind IN ('cwd', 'git_root', 'worktree', 'branch', 'remote')),
  alias_value TEXT NOT NULL,
  PRIMARY KEY (project_id, alias_kind, alias_value)
);

CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  source_session_id TEXT NOT NULL,
  project_id TEXT REFERENCES projects(id),
  root_work_unit_id TEXT NOT NULL,
  purpose TEXT NOT NULL CHECK (purpose IN ('user', 'spawned', 'inspector_review', 'other', 'orphan')),
  source TEXT,
  originator TEXT,
  title TEXT,
  started_at TEXT,
  ended_at TEXT,
  lineage_coverage TEXT NOT NULL CHECK (lineage_coverage IN ('exact', 'partial', 'unavailable')),
  UNIQUE (epoch_id, source_session_id)
);

CREATE INDEX sessions_root_work_unit ON sessions(epoch_id, root_work_unit_id);

CREATE TABLE source_segments (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  session_id TEXT NOT NULL REFERENCES sessions(id),
  source_id TEXT NOT NULL REFERENCES source_artifacts(id),
  segment_fingerprint TEXT NOT NULL,
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
  UNIQUE (epoch_id, session_id, segment_fingerprint),
  UNIQUE (epoch_id, session_id, ordinal)
);

CREATE TABLE lineage_edges (
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  parent_session_id TEXT NOT NULL REFERENCES sessions(id),
  child_session_id TEXT NOT NULL REFERENCES sessions(id),
  edge_kind TEXT NOT NULL CHECK (edge_kind IN ('spawned', 'continued_as', 'forked_from', 'resumed')),
  spawning_turn_id TEXT,
  source_event_id TEXT,
  PRIMARY KEY (epoch_id, parent_session_id, child_session_id, edge_kind),
  CHECK (parent_session_id <> child_session_id)
);

CREATE TABLE turns (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  session_id TEXT NOT NULL REFERENCES sessions(id),
  source_turn_id TEXT NOT NULL,
  ordinal INTEGER NOT NULL CHECK (ordinal >= 0),
  state TEXT NOT NULL CHECK (state IN ('provisional', 'completed', 'aborted', 'interrupted', 'reconciled_truncated')),
  started_at TEXT,
  completed_at TEXT,
  model TEXT,
  reasoning_effort TEXT,
  commit_revision INTEGER,
  UNIQUE (epoch_id, session_id, source_turn_id),
  UNIQUE (epoch_id, session_id, ordinal),
  FOREIGN KEY (epoch_id, commit_revision) REFERENCES index_revisions(epoch_id, revision),
  CHECK ((state = 'completed' AND completed_at IS NOT NULL AND commit_revision IS NOT NULL) OR state <> 'completed')
);

CREATE TABLE events (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  segment_id TEXT NOT NULL REFERENCES source_segments(id),
  turn_id TEXT REFERENCES turns(id),
  record_ordinal INTEGER NOT NULL CHECK (record_ordinal >= 0),
  semantic_phase TEXT NOT NULL,
  event_kind TEXT NOT NULL,
  observed_at TEXT NOT NULL,
  byte_start INTEGER CHECK (byte_start IS NULL OR byte_start >= 0),
  byte_end INTEGER CHECK (byte_end IS NULL OR byte_end >= byte_start),
  content_sha256 TEXT NOT NULL,
  payload_length INTEGER NOT NULL CHECK (payload_length >= 0),
  adapter_version TEXT NOT NULL,
  UNIQUE (epoch_id, segment_id, record_ordinal, semantic_phase)
);

CREATE INDEX events_turn_order ON events(epoch_id, turn_id, record_ordinal, semantic_phase);

CREATE TABLE messages (
  event_id TEXT PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,
  role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'system', 'developer', 'tool')),
  phase TEXT NOT NULL,
  source_message_id TEXT,
  readable INTEGER NOT NULL CHECK (readable IN (0, 1)),
  content_length INTEGER NOT NULL CHECK (content_length >= 0),
  content_sha256 TEXT NOT NULL
);

CREATE TABLE tool_calls (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  session_id TEXT NOT NULL REFERENCES sessions(id),
  turn_id TEXT REFERENCES turns(id),
  source_call_id TEXT NOT NULL,
  semantic_phase TEXT NOT NULL CHECK (semantic_phase IN ('request', 'result')),
  event_id TEXT NOT NULL REFERENCES events(id),
  tool_name TEXT NOT NULL,
  tool_family TEXT NOT NULL,
  status TEXT,
  exit_code INTEGER,
  duration_ms INTEGER CHECK (duration_ms IS NULL OR duration_ms >= 0),
  UNIQUE (epoch_id, session_id, source_call_id, semantic_phase)
);

CREATE TABLE turn_usage (
  turn_id TEXT NOT NULL REFERENCES turns(id) ON DELETE CASCADE,
  formula_version INTEGER NOT NULL CHECK (formula_version = 1),
  normalization_kind TEXT NOT NULL CHECK (normalization_kind IN ('last_turn', 'cumulative_delta')),
  input_tokens INTEGER CHECK (input_tokens IS NULL OR input_tokens >= 0),
  cached_input_tokens INTEGER CHECK (cached_input_tokens IS NULL OR cached_input_tokens >= 0),
  output_tokens INTEGER CHECK (output_tokens IS NULL OR output_tokens >= 0),
  reasoning_output_tokens INTEGER CHECK (reasoning_output_tokens IS NULL OR reasoning_output_tokens >= 0),
  total_tokens INTEGER CHECK (total_tokens IS NULL OR total_tokens >= 0),
  residual_tokens INTEGER CHECK (residual_tokens IS NULL OR residual_tokens >= 0),
  fidelity TEXT NOT NULL CHECK (fidelity IN ('exact', 'derived', 'unavailable')),
  PRIMARY KEY (turn_id, formula_version),
  CHECK (cached_input_tokens IS NULL OR input_tokens IS NULL OR cached_input_tokens <= input_tokens),
  CHECK (reasoning_output_tokens IS NULL OR output_tokens IS NULL OR reasoning_output_tokens <= output_tokens)
);

CREATE TABLE capacity_observations (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  event_id TEXT NOT NULL REFERENCES events(id),
  limit_id TEXT NOT NULL,
  window_minutes INTEGER CHECK (window_minutes IS NULL OR window_minutes > 0),
  used_percent REAL CHECK (used_percent IS NULL OR (used_percent >= 0 AND used_percent <= 100)),
  remaining_percent REAL CHECK (remaining_percent IS NULL OR (remaining_percent >= 0 AND remaining_percent <= 100)),
  resets_at TEXT,
  observed_at TEXT NOT NULL,
  UNIQUE (epoch_id, event_id, limit_id, window_minutes)
);

CREATE TABLE compactions (
  event_id TEXT PRIMARY KEY REFERENCES events(id) ON DELETE CASCADE,
  turn_id TEXT REFERENCES turns(id),
  trigger_kind TEXT,
  recorded_summary_length INTEGER CHECK (recorded_summary_length IS NULL OR recorded_summary_length >= 0),
  recorded_summary_sha256 TEXT
);

CREATE TABLE evidence_refs (
  id TEXT PRIMARY KEY,
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  event_id TEXT NOT NULL REFERENCES events(id),
  source_id TEXT NOT NULL REFERENCES source_artifacts(id),
  event_fingerprint TEXT NOT NULL,
  availability TEXT NOT NULL CHECK (availability IN ('available', 'source_missing', 'fingerprint_mismatch', 'unreadable')),
  UNIQUE (epoch_id, event_id, event_fingerprint)
);

CREATE TABLE coverage_observations (
  epoch_id TEXT NOT NULL REFERENCES dataset_epochs(id),
  scope_kind TEXT NOT NULL,
  scope_id TEXT NOT NULL,
  field_key TEXT NOT NULL,
  fidelity TEXT NOT NULL CHECK (fidelity IN ('exact', 'derived', 'unavailable')),
  observed_count INTEGER NOT NULL CHECK (observed_count >= 0),
  eligible_count INTEGER NOT NULL CHECK (eligible_count >= observed_count),
  reason TEXT,
  PRIMARY KEY (epoch_id, scope_kind, scope_id, field_key)
);

CREATE TABLE event_search_documents (
  rowid INTEGER PRIMARY KEY,
  event_id TEXT NOT NULL REFERENCES events(id) ON DELETE CASCADE,
  match_category TEXT NOT NULL,
  UNIQUE (event_id, match_category)
);

CREATE VIRTUAL TABLE event_search USING fts5(
  content,
  content = '',
  contentless_delete = 1,
  tokenize = 'unicode61'
);

INSERT INTO schema_metadata(singleton, schema_version, protocol_version, created_at)
VALUES (1, 1, 1, '2026-07-18T00:00:00Z');
