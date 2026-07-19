package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

func TestSourceInventoryAtIsRevisionPinned(t *testing.T) {
	root := t.TempDir()
	layout := home.Layout{Root: filepath.Join(root, "inspector"), Reviews: filepath.Join(root, "inspector", "reviews"), Queue: filepath.Join(root, "inspector", "queue"), Run: filepath.Join(root, "inspector", "run"), Logs: filepath.Join(root, "inspector", "logs"), Cache: filepath.Join(root, "inspector", "cache")}
	codexHome := filepath.Join(root, "codex")
	if err := os.MkdirAll(filepath.Join(codexHome, "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(codexHome, "sessions", "root.jsonl"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = indexer.Run(context.Background(), indexer.Config{Layout: layout, CodexHome: codexHome}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	epoch, revision, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	base, err := store.SourceInventoryAt(context.Background(), epoch, revision, nil)
	if err != nil || base.Total != 1 || base.Current != 1 {
		t.Fatalf("complete inventory=%+v err=%v", base, err)
	}
	var sessionID string
	if err = store.DB().QueryRow(`SELECT id FROM sessions WHERE epoch_id=?`, epoch).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}
	scoped, err := store.SourceInventoryAt(context.Background(), epoch, revision, []string{sessionID})
	if err != nil || scoped.Total != 1 || scoped.Current != 1 {
		t.Fatalf("scoped inventory=%+v err=%v", scoped, err)
	}
	outside, err := store.SourceInventoryAt(context.Background(), epoch, revision, []string{"session:not-in-scope"})
	if err != nil || outside.Total != 0 {
		t.Fatalf("out-of-scope inventory=%+v err=%v", outside, err)
	}
	var sourceID string
	if err = store.DB().QueryRow(`SELECT id FROM source_artifacts WHERE epoch_id=?`, epoch).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"discovered", "indexing", "unsupported", "failed", "requires_rebuild"} {
		previousRevision := revision
		revision++
		tx, txErr := store.DB().Begin()
		if txErr != nil {
			t.Fatal(txErr)
		}
		if _, txErr = tx.Exec(`INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES(?,?,?,'inventory')`, epoch, revision, "2026-07-19T12:00:00Z"); txErr == nil {
			_, txErr = tx.Exec(`INSERT INTO source_artifact_versions(epoch_id,source_id,revision,source_kind,canonical_path,inode,byte_size,mtime_ns,detected_codex_version,adapter_version,state,state_reason,source_evidence_availability,availability_observed_at)
			 SELECT epoch_id,source_id,?,source_kind,canonical_path,inode,byte_size,mtime_ns,detected_codex_version,adapter_version,?,'synthetic inventory state',source_evidence_availability,availability_observed_at
			 FROM source_artifact_versions WHERE epoch_id=? AND source_id=? ORDER BY revision DESC LIMIT 1`, revision, state, epoch, sourceID)
		}
		if txErr != nil {
			_ = tx.Rollback()
			t.Fatal(txErr)
		}
		if txErr = tx.Commit(); txErr != nil {
			t.Fatal(txErr)
		}
		old, oldErr := store.SourceInventoryAt(context.Background(), epoch, previousRevision, nil)
		if oldErr != nil || old.Total != 1 {
			t.Fatalf("old revision %d inventory=%+v err=%v", previousRevision, old, oldErr)
		}
		current, currentErr := store.SourceInventoryAt(context.Background(), epoch, revision, nil)
		if currentErr != nil || current.Total != 1 {
			t.Fatalf("state %s inventory=%+v err=%v", state, current, currentErr)
		}
		got := map[string]int{"discovered": current.Discovered, "indexing": current.Indexing, "unsupported": current.Unsupported, "failed": current.Failed, "requires_rebuild": current.RequiresRebuild}[state]
		if got != 1 {
			t.Fatalf("state %s not selected at revision %d: %+v", state, revision, current)
		}
	}
}
