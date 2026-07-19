package inspector

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

func syntheticRepository(t *testing.T) (Repository, func()) {
	t.Helper()
	root := t.TempDir()
	codex := filepath.Join(root, "codex")
	sessions := filepath.Join(codex, "sessions")
	inspectorHome := filepath.Join(root, "inspector")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	fixtures := filepath.Join(filepath.Dir(file), "..", "..", "fixtures", "synthetic")
	for _, name := range []string{"root.jsonl", "descendant.jsonl"} {
		data, err := os.ReadFile(filepath.Join(fixtures, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(sessions, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	layout := home.Layout{Root: inspectorHome, Reviews: filepath.Join(inspectorHome, "reviews"), Queue: filepath.Join(inspectorHome, "queue"), Run: filepath.Join(inspectorHome, "run"), Logs: filepath.Join(inspectorHome, "logs"), Cache: filepath.Join(inspectorHome, "cache")}
	if _, err := indexer.Run(context.Background(), indexer.Config{Layout: layout, CodexHome: codex}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(inspectorHome, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	return Repository{Store: store}, func() { _ = store.Close() }
}

func TestDiscoveryExplainsDescendantMatchAndTotals(t *testing.T) {
	repository, closeStore := syntheticRepository(t)
	defer closeStore()
	page, err := repository.Sessions(context.Background(), 0, "delegated check", "", 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].SessionID != page.Items[0].RootWorkUnitID {
		t.Fatalf("descendant match did not resolve to its root: %#v", page.Items)
	}
	if len(page.Items[0].MatchCategories) != 1 || page.Items[0].MatchCategories[0] != "descendant: message" {
		t.Fatalf("match explanation is not descendant-specific: %#v", page.Items[0].MatchCategories)
	}
	if page.Items[0].DirectTokens == nil || *page.Items[0].DirectTokens != 2000 || page.Items[0].DescendantTokens == nil || *page.Items[0].DescendantTokens != 500 {
		t.Fatalf("discovery totals differ from metric golden: %#v", page.Items[0])
	}
}

func TestMapLedgerAndCompactionEvidenceAreRevisionPinned(t *testing.T) {
	repository, closeStore := syntheticRepository(t)
	defer closeStore()
	page, err := repository.Sessions(context.Background(), 0, "", "", 0, 50)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("session discovery failed: %#v %v", page, err)
	}
	rootID := page.Items[0].SessionID
	result, err := repository.Map(context.Background(), page.AppliedRevision, rootID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 2 || len(result.RootTurns) != 2 || len(result.Edges) != 1 {
		t.Fatalf("causal map is incomplete: %#v", result)
	}
	var root, child *MapNode
	for i := range result.Nodes {
		if result.Nodes[i].Kind == "root" {
			root = &result.Nodes[i]
		} else if result.Nodes[i].Kind == "descendant" {
			child = &result.Nodes[i]
		}
	}
	if root == nil || root.DirectTokens == nil || *root.DirectTokens != 2000 || root.InclusiveTokens == nil || *root.InclusiveTokens != 2500 || child == nil || child.DirectTokens == nil || *child.DirectTokens != 500 {
		t.Fatalf("map totals differ from metric engine: root=%#v child=%#v", root, child)
	}
	ledger, err := repository.Ledger(context.Background(), page.AppliedRevision, rootID, result.RootTurns[0].TurnID, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(ledger.Items) != 11 {
		t.Fatalf("unexpected complete turn ledger: %#v", ledger.Items)
	}
	for i := 1; i < len(ledger.Items); i++ {
		previous, previousErr := time.Parse(time.RFC3339Nano, ledger.Items[i-1].ObservedAt)
		current, currentErr := time.Parse(time.RFC3339Nano, ledger.Items[i].ObservedAt)
		if previousErr != nil || currentErr != nil || current.Before(previous) {
			t.Fatalf("ledger is not chronological: %#v", ledger.Items)
		}
	}
	var compactionEvidence string
	for _, item := range ledger.Items {
		if item.Kind == "compacted" {
			compactionEvidence = item.EvidenceID
		}
	}
	if compactionEvidence == "" {
		t.Fatal("exact recorded compaction evidence is absent")
	}
	kind, err := repository.EvidenceKind(context.Background(), page.AppliedRevision, compactionEvidence)
	if err != nil || kind != "compacted" {
		t.Fatalf("compaction evidence lookup failed: %q %v", kind, err)
	}
}
