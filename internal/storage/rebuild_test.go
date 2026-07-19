package storage_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dylanjbarth/codex-inspector/internal/facts"
	"github.com/dylanjbarth/codex-inspector/internal/sources"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

func parsed(t *testing.T, name, kind string) facts.Batch {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "fixtures", "synthetic", name)
	info, e := os.Stat(path)
	if e != nil {
		t.Fatal(e)
	}
	b, e := sources.Parse(sources.Candidate{Path: path, Kind: kind, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestCanceledNormalizationTransactionLeavesNoPartialRevision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inspector.db")
	store, err := storage.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, before, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = store.Apply(ctx, parsed(t, "root.jsonl", "active_rollout"), "crash_point"); err == nil {
		t.Fatal("canceled transaction committed")
	}
	_, after, _ := store.Snapshot()
	if after != before {
		t.Fatalf("partial revision committed: %d -> %d", before, after)
	}
	var n int
	if err = store.DB().QueryRow("SELECT count(*) FROM sessions").Scan(&n); err != nil || n != 0 {
		t.Fatalf("partial facts survived: %d %v", n, err)
	}
}

func TestDatasetEpochAtomicReplacementAndFailedBuildIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inspector.db")
	store, e := storage.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	if _, e = store.Apply(context.Background(), parsed(t, "root.jsonl", "active_rollout"), "initial"); e != nil {
		t.Fatal(e)
	}
	oldEpoch, oldRev, e := store.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	oldReader, e := storage.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer oldReader.Close()
	oldSnapshot, e := oldReader.DB().Begin()
	if e != nil {
		t.Fatal(e)
	}
	var oldSessionCount int
	if e = oldSnapshot.QueryRow("SELECT count(*) FROM sessions").Scan(&oldSessionCount); e != nil {
		t.Fatal(e)
	}
	bad := facts.Batch{Source: facts.Source{State: "supported", Kind: "active_rollout"}}
	if e = store.Rebuild(context.Background(), []facts.Batch{bad}); e == nil {
		t.Fatal("invalid replacement activated")
	}
	epochAfterFailure, revAfterFailure, _ := store.Snapshot()
	if epochAfterFailure != oldEpoch || revAfterFailure != oldRev {
		t.Fatal("failed build changed active epoch")
	}
	if e = store.Rebuild(context.Background(), []facts.Batch{parsed(t, "root.jsonl", "active_rollout"), parsed(t, "descendant.jsonl", "archived_rollout")}); e != nil {
		t.Fatal(e)
	}
	newStore, e := storage.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	defer newStore.Close()
	newEpoch, newRev, e := newStore.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	if newEpoch == oldEpoch || newRev < 1 {
		t.Fatalf("replacement did not establish a new epoch-local revision: %s/%d -> %s/%d", oldEpoch, oldRev, newEpoch, newRev)
	}
	var state string
	if e = store.DB().QueryRow("SELECT state FROM dataset_epochs WHERE id=?", oldEpoch).Scan(&state); e != nil || state != "superseded" {
		t.Fatalf("old epoch not superseded: %q %v", state, e)
	}
	// Revisions are epoch-local in schema v2. A numerically equal revision in
	// the replacement epoch is distinguished by the response dataset epoch.
	if e = oldSnapshot.QueryRow("SELECT count(*) FROM sessions").Scan(&oldSessionCount); e != nil || oldSessionCount != 1 {
		t.Fatalf("existing read transaction could not finish on old immutable file: count=%d err=%v", oldSessionCount, e)
	}
	if e = oldSnapshot.Commit(); e != nil {
		t.Fatal(e)
	}
}
