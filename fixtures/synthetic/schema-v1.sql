PRAGMA foreign_keys = ON;

CREATE TABLE schema_metadata (
  singleton INTEGER PRIMARY KEY CHECK (singleton = 1),
  schema_version INTEGER NOT NULL CHECK (schema_version = 1),
  protocol_version INTEGER NOT NULL CHECK (protocol_version = 1)
);

CREATE TABLE legacy_facts (
  id TEXT PRIMARY KEY,
  source_prefix_sha256 TEXT NOT NULL,
  event_fingerprint TEXT NOT NULL
);

INSERT INTO schema_metadata(singleton, schema_version, protocol_version)
VALUES (1, 1, 1);

INSERT INTO legacy_facts(id, source_prefix_sha256, event_fingerprint)
VALUES (
  'legacy-fact-1',
  'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',
  'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb'
);
