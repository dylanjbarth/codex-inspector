#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: $0 /path/to/codex-inspector" >&2
  exit 2
fi

binary=$1
test -x "$binary"
validation_home=$(mktemp -d "${TMPDIR:-/tmp}/codex-inspector-phase2-corpus.XXXXXX")
cleanup() { rm -rf -- "$validation_home"; }
trap cleanup EXIT INT TERM

# The CLI result and SQL projection contain only counts, revisions, states,
# byte totals, and timestamps. No source path or payload is emitted.
CODEX_INSPECTOR_HOME="$validation_home" /usr/bin/time -l "$binary" sync --wait
active_name=$(tr -d '\r\n' < "$validation_home/active-index")
case "$active_name" in
  index-v2-*.sqlite) ;;
  *) echo "invalid active-index catalog" >&2; exit 1 ;;
esac
sqlite3 "$validation_home/$active_name" <<'SQL'
SELECT 'epochs', count(*) FROM dataset_epochs WHERE state='active';
SELECT 'revisions', count(*) FROM index_revisions;
SELECT 'sources', count(*), sum(v.byte_size)
FROM source_artifacts a
JOIN source_artifact_versions v ON v.epoch_id=a.epoch_id AND v.source_id=a.id
WHERE a.epoch_id=(SELECT id FROM dataset_epochs WHERE state='active')
  AND v.revision=(SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id);
SELECT 'source_states', v.state, count(*)
FROM source_artifact_versions v
WHERE v.epoch_id=(SELECT id FROM dataset_epochs WHERE state='active')
  AND v.revision=(SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id)
GROUP BY v.state ORDER BY v.state;
SELECT 'completed_turns', count(*) FROM turns WHERE state='completed';
SELECT 'pending_tail_sources', count(*) FROM source_checkpoints WHERE pending_tail_bytes > 0;
SELECT 'max_checkpoint', max(complete_byte_offset), max(complete_record_ordinal) FROM source_checkpoints;
SQL
CODEX_INSPECTOR_HOME="$validation_home" "$binary" sync --wait
