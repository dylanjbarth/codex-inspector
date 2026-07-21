package inspector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestDiscoverySearchesRootUserMessagesAndExactSessionIDs(t *testing.T) {
	repository, closeStore := syntheticRepository(t)
	defer closeStore()
	page, err := repository.Sessions(context.Background(), 0, "fake widget", "", nil, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].SessionID != page.Items[0].RootWorkUnitID {
		t.Fatalf("root user-message match did not resolve to its root: %#v", page.Items)
	}
	if len(page.Items[0].MatchCategories) != 1 || page.Items[0].MatchCategories[0] != "root: user message" {
		t.Fatalf("match explanation is not user-message-specific: %#v", page.Items[0].MatchCategories)
	}
	if len(page.Items[0].MatchSnippets) == 0 || page.Items[0].MatchSnippets[0].Category != "root: user message" || !strings.Contains(strings.ToLower(page.Items[0].MatchSnippets[0].Text), "fake widget") || len([]rune(page.Items[0].MatchSnippets[0].Text)) > 240 {
		t.Fatalf("bounded user-message snippet missing: %#v", page.Items[0].MatchSnippets)
	}
	assistantPage, err := repository.Sessions(context.Background(), page.AppliedRevision, "delegated check", "", nil, 0, 50)
	if err != nil || len(assistantPage.Items) != 0 {
		t.Fatalf("assistant and descendant content should not match: page=%#v err=%v", assistantPage, err)
	}
	toolPage, err := repository.Sessions(context.Background(), page.AppliedRevision, "fake success", "", nil, 0, 50)
	if err != nil || len(toolPage.Items) != 0 {
		t.Fatalf("tool results should not match: page=%#v err=%v", toolPage, err)
	}
	exactPage, err := repository.Sessions(context.Background(), page.AppliedRevision, "root-001", "", nil, 0, 50)
	if err != nil || len(exactPage.Items) != 1 || len(exactPage.Items[0].MatchCategories) != 1 || exactPage.Items[0].MatchCategories[0] != "root: session ID" {
		t.Fatalf("exact source session ID lookup failed: page=%#v err=%v", exactPage, err)
	}
	reorderedPage, err := repository.Sessions(context.Background(), page.AppliedRevision, "widget fake", "", nil, 0, 50)
	if err != nil || len(reorderedPage.Items) != 1 {
		t.Fatalf("multi-term search should match all words regardless of phrase order: page=%#v err=%v", reorderedPage, err)
	}
	requestedRoots := make([]string, 50, 51)
	for i := range requestedRoots {
		requestedRoots[i] = fmt.Sprintf("session:outside-page-one-%02d", i)
	}
	requestedRoots = append(requestedRoots, page.Items[0].SessionID)
	targeted, err := repository.Sessions(context.Background(), page.AppliedRevision, "", "", requestedRoots, 0, 50)
	if err != nil || len(targeted.Items) != 1 || targeted.Items[0].SessionID != page.Items[0].SessionID || targeted.Items[0].Title == "" || targeted.Items[0].Project == "" || targeted.Items[0].StartedAt == "" || targeted.Items[0].CompletedTurns == 0 {
		t.Fatalf("targeted metadata beyond a 50-root discovery page is incomplete: page=%#v err=%v", targeted, err)
	}
	if page.Items[0].DirectTokens == nil || *page.Items[0].DirectTokens != 2000 || page.Items[0].DescendantTokens == nil || *page.Items[0].DescendantTokens != 500 {
		t.Fatalf("discovery totals differ from metric golden: %#v", page.Items[0])
	}
	if page.Items[0].DescendantSessions != 1 {
		t.Fatalf("discovery tree size differs from the indexed lineage: %#v", page.Items[0])
	}
}

func TestMapLedgerAndCompactionEvidenceAreRevisionPinned(t *testing.T) {
	repository, closeStore := syntheticRepository(t)
	defer closeStore()
	page, err := repository.Sessions(context.Background(), 0, "", "", nil, 0, 50)
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("session discovery failed: %#v %v", page, err)
	}
	rootID := page.Items[0].SessionID
	result, err := repository.Map(context.Background(), page.AppliedRevision, rootID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 2 || len(result.RootTurns) != 2 || len(result.Turns) != 3 || len(result.Edges) != 1 {
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
	var rootTurn, descendantTurn *MapTurn
	for i := range result.Turns {
		turn := &result.Turns[i]
		if turn.SessionKind == "root" && rootTurn == nil {
			rootTurn = turn
		}
		if turn.SessionKind == "descendant" {
			descendantTurn = turn
		}
	}
	if rootTurn == nil || rootTurn.DirectTokens == nil || *rootTurn.DirectTokens != 1200 || rootTurn.InclusiveTokens == nil || *rootTurn.InclusiveTokens != 1200 || descendantTurn == nil || descendantTurn.DirectTokens == nil || *descendantTurn.DirectTokens != 500 || descendantTurn.InclusiveTokens == nil || *descendantTurn.InclusiveTokens != 500 {
		t.Fatalf("turn-level causal topology is incomplete: root=%#v descendant=%#v all=%#v", rootTurn, descendantTurn, result.Turns)
	}
	if rootTurn.ToolCount != 1 || rootTurn.ErrorCount != 0 || rootTurn.CompactionCount != 1 {
		t.Fatalf("turn activity aggregates changed while avoiding the fact-table cross product: %#v", rootTurn)
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
