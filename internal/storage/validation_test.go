package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dylanjbarth/codex-inspector/internal/facts"
	"github.com/dylanjbarth/codex-inspector/internal/sources"
)

func validationBatch(t *testing.T, name, kind string) facts.Batch {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "fixtures", "synthetic", name)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := sources.Parse(sources.Candidate{Path: path, Kind: kind, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return batch
}

func TestCandidateCorruptionCannotReplaceActiveCatalog(t *testing.T) {
	mutations := []struct {
		name   string
		mutate func(*testing.T, *sql.DB)
	}{
		{"same_count_fts_token", func(t *testing.T, db *sql.DB) {
			var rowid int64
			mustValidationExec(t, db.QueryRow(`SELECT min(rowid) FROM event_search_documents`).Scan(&rowid))
			mustValidationExec(t, execValidation(db, `CREATE VIRTUAL TABLE temp.mutation_vocab USING fts5vocab(main,'event_search','instance')`))
			var tokenCount int
			mustValidationExec(t, db.QueryRow(`SELECT count(*) FROM temp.mutation_vocab WHERE doc=?`, rowid).Scan(&tokenCount))
			mustValidationExec(t, execValidation(db, `DROP TABLE temp.mutation_vocab`))
			if tokenCount == 0 {
				t.Fatal("fixture lacks event search tokens")
			}
			mustValidationExec(t, execValidation(db, `DELETE FROM event_search WHERE rowid=?`, rowid))
			mustValidationExec(t, execValidation(db, `INSERT INTO event_search(rowid,content) VALUES(?,?)`, rowid, strings.Repeat("different ", tokenCount)))
		}},
		{"fts_provenance", func(t *testing.T, db *sql.DB) {
			mustValidationExec(t, execValidation(db, `DROP TRIGGER immutable_event_search_docs_u`))
			mustValidationExec(t, execValidation(db, `UPDATE event_search_documents SET match_category=CASE match_category WHEN 'message' THEN 'tool_result' ELSE 'message' END WHERE rowid=(SELECT min(rowid) FROM event_search_documents)`))
		}},
		{"latest_selector_revision", func(t *testing.T, db *sql.DB) {
			mustValidationExec(t, execValidation(db, `DROP TRIGGER immutable_session_versions_u`))
			mustValidationExec(t, execValidation(db, `UPDATE session_versions SET earliest_proven_start='2099-01-01T00:00:00Z' WHERE revision=(SELECT max(revision) FROM session_versions)`))
		}},
		{"identity_projection", func(t *testing.T, db *sql.DB) {
			mustValidationExec(t, execValidation(db, `DROP TRIGGER immutable_events_u`))
			mustValidationExec(t, execValidation(db, `UPDATE events SET content_sha256=printf('%064d',0) WHERE id=(SELECT min(id) FROM events)`))
		}},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "inspector.db")
			active, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer active.Close()
			catalogBefore, err := os.ReadFile(filepath.Join(filepath.Dir(path), "active-index"))
			if err != nil {
				t.Fatal(err)
			}
			oldEpoch, _, err := active.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			validationTestHook = func(db *sql.DB) { test.mutate(t, db) }
			defer func() { validationTestHook = nil }()
			err = active.Rebuild(context.Background(), []facts.Batch{
				validationBatch(t, "root.jsonl", "active_rollout"),
				validationBatch(t, "descendant.jsonl", "archived_rollout"),
			})
			if err == nil {
				t.Fatal("corrupted candidate activated")
			}
			catalogAfter, readErr := os.ReadFile(filepath.Join(filepath.Dir(path), "active-index"))
			if readErr != nil || string(catalogAfter) != string(catalogBefore) {
				t.Fatalf("active catalog changed after failed validation: before=%q after=%q err=%v", catalogBefore, catalogAfter, readErr)
			}
			reader, openErr := Open(path)
			if openErr != nil {
				t.Fatal(openErr)
			}
			defer reader.Close()
			gotEpoch, _, snapshotErr := reader.Snapshot()
			if snapshotErr != nil || gotEpoch != oldEpoch {
				t.Fatalf("failed candidate replaced readable active epoch: got=%q want=%q err=%v", gotEpoch, oldEpoch, snapshotErr)
			}
		})
	}
}

func execValidation(db *sql.DB, query string, args ...any) error {
	_, err := db.Exec(query, args...)
	return err
}

func mustValidationExec(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
