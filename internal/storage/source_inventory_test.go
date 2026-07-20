package storage_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dylanjbarth/codex-inspector/internal/facts"
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
			 SELECT epoch_id,source_id,?,source_kind,canonical_path,inode,byte_size,mtime_ns,detected_codex_version,adapter_version,?,'normalization_failed',source_evidence_availability,availability_observed_at
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
	groups, err := store.SourceDiagnosticGroups(context.Background())
	if err != nil || len(groups) != 1 || groups[0].State != "requires_rebuild" || groups[0].Reason != "normalization_failed" || groups[0].DetectedVersion != "0.144.1" || groups[0].Count != 1 {
		t.Fatalf("payload-free diagnostic cohort mismatch: groups=%+v err=%v", groups, err)
	}

	revision++
	if _, err = store.DB().Exec(`INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES(?,?,?,'inventory')`, epoch, revision, "2026-07-19T12:00:30Z"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB().Exec(`INSERT INTO source_artifact_versions(epoch_id,source_id,revision,source_kind,canonical_path,inode,byte_size,mtime_ns,detected_codex_version,adapter_version,state,state_reason,source_evidence_availability,availability_observed_at)
		SELECT epoch_id,source_id,?,source_kind,canonical_path,inode,byte_size,mtime_ns,'0.145.0-alpha.19',adapter_version,'unsupported','unsupported_codex_version',source_evidence_availability,availability_observed_at
		FROM source_artifact_versions WHERE epoch_id=? AND source_id=? ORDER BY revision DESC LIMIT 1`, revision, epoch, sourceID); err != nil {
		t.Fatal(err)
	}
	groups, err = store.SourceDiagnosticGroups(context.Background())
	if err != nil || len(groups) != 1 || groups[0].State != "unsupported" || groups[0].Reason != "unsupported_codex_version" || groups[0].DetectedVersion != "0.145.0-alpha.19" || groups[0].Count != 1 {
		t.Fatalf("unsupported version diagnostic was not preserved: groups=%+v err=%v", groups, err)
	}

	revision++
	if _, err = store.DB().Exec(`INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES(?,?,?,'inventory')`, epoch, revision, "2026-07-19T12:01:00Z"); err != nil {
		t.Fatal(err)
	}
	secret := "/Users/demo/private prompt text"
	if _, err = store.DB().Exec(`INSERT INTO source_artifact_versions(epoch_id,source_id,revision,source_kind,canonical_path,inode,byte_size,mtime_ns,detected_codex_version,adapter_version,state,state_reason,source_evidence_availability,availability_observed_at)
		SELECT epoch_id,source_id,?,source_kind,canonical_path,inode,byte_size,mtime_ns,?,adapter_version,'failed',?,source_evidence_availability,availability_observed_at
		FROM source_artifact_versions WHERE epoch_id=? AND source_id=? ORDER BY revision DESC LIMIT 1`, revision, secret, secret, epoch, sourceID); err != nil {
		t.Fatal(err)
	}
	groups, err = store.SourceDiagnosticGroups(context.Background())
	if err != nil || len(groups) != 1 || groups[0].Reason != "unclassified_source_state" || groups[0].DetectedVersion != "unknown" || strings.Contains(fmt.Sprint(groups), secret) {
		t.Fatalf("untrusted diagnostic text escaped normalization: groups=%+v err=%v", groups, err)
	}

	for i := 0; i < 60; i++ {
		_, err = store.Apply(context.Background(), facts.Batch{Source: facts.Source{
			ID:                 fmt.Sprintf("diagnostic-%03d", i),
			SessionID:          fmt.Sprintf("diagnostic-session-%03d", i),
			SegmentFingerprint: fmt.Sprintf("%064x", i+1),
			Kind:               "active_rollout",
			Path:               fmt.Sprintf("/synthetic/diagnostic-%03d.jsonl", i),
			DetectedVersion:    fmt.Sprintf("1.0.%d", i),
			AdapterVersion:     storage.AdapterVersion,
			PrefixSHA256:       strings.Repeat("a", 64),
			State:              "unsupported",
			StateReason:        "incompatible_record_envelope",
		}}, "diagnostic-bound")
		if err != nil {
			t.Fatal(err)
		}
	}
	groups, err = store.SourceDiagnosticGroups(context.Background())
	if err != nil || len(groups) != storage.MaxSourceDiagnosticGroups {
		t.Fatalf("bounded diagnostic groups=%d err=%v", len(groups), err)
	}
	total := 0
	var overflow *storage.SourceDiagnosticGroup
	for i := range groups {
		total += groups[i].Count
		if groups[i].State == "multiple" {
			overflow = &groups[i]
		}
	}
	if total != 61 || overflow == nil || overflow.Reason != "additional_diagnostic_groups" || overflow.DetectedVersion != "multiple" || overflow.Count != 12 {
		t.Fatalf("dishonest diagnostic overflow: total=%d overflow=%+v groups=%+v", total, overflow, groups)
	}
	if err = store.MarkRequiresRebuild("diagnostic-000", "indexed_prefix_changed_or_shrank"); err != nil {
		t.Fatal(err)
	}
	groups, err = store.SourceDiagnosticGroups(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	foundRebuild := false
	for _, group := range groups {
		if group.State == "requires_rebuild" && group.Reason == "indexed_prefix_changed_or_shrank" && group.DetectedVersion == "1.0.0" && group.Count == 1 {
			foundRebuild = true
		}
	}
	if !foundRebuild {
		t.Fatalf("actual requires_rebuild transition lost its fixed reason: %+v", groups)
	}
}
