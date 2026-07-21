package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/facts"
	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/hook"
	"github.com/dylanjbarth/codex-inspector/internal/phase0"
	proc "github.com/dylanjbarth/codex-inspector/internal/process"
	"github.com/dylanjbarth/codex-inspector/internal/sources"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

func repoFixture(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "fixtures", "synthetic", name)
}
func setup(t *testing.T) (home.Layout, string) {
	t.Helper()
	root := t.TempDir()
	codex := filepath.Join(root, "codex")
	inspector := filepath.Join(root, "inspector")
	active := filepath.Join(codex, "sessions", "2026", "07", "01")
	archive := filepath.Join(codex, "archived_sessions")
	if err := os.MkdirAll(active, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(archive, 0700); err != nil {
		t.Fatal(err)
	}
	copyFile(t, repoFixture(t, "root.jsonl"), filepath.Join(active, "rollout-root-001.jsonl"))
	copyFile(t, repoFixture(t, "descendant.jsonl"), filepath.Join(active, "rollout-child-001.jsonl"))
	copyFile(t, repoFixture(t, "truncated.jsonl"), filepath.Join(active, "rollout-truncated-001.jsonl"))
	copyFile(t, repoFixture(t, "unsupported.jsonl"), filepath.Join(archive, "rollout-unsupported-001.jsonl"))
	copyFile(t, repoFixture(t, "session_index.jsonl"), filepath.Join(codex, "session_index.jsonl"))
	return home.Layout{Root: inspector, Reviews: filepath.Join(inspector, "reviews"), Queue: filepath.Join(inspector, "queue"), Run: filepath.Join(inspector, "run"), Logs: filepath.Join(inspector, "logs"), Cache: filepath.Join(inspector, "cache")}, codex
}
func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, e := os.ReadFile(from)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(to, b, 0600); e != nil {
		t.Fatal(e)
	}
}
func scalar(t *testing.T, db *sql.DB, q string) int64 {
	t.Helper()
	var n int64
	if e := db.QueryRow(q).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}

func TestFullScanIdempotenceGoldenAndStates(t *testing.T) {
	layout, codex := setup(t)
	cfg := Config{Layout: layout, CodexHome: codex, Concurrency: 2}
	first, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.Inventoried != 5 || first.Processed != 5 {
		t.Fatalf("first scan: %#v", first)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.DB()
	checks := map[string]int64{"select count(*) from source_artifacts": 5, "select count(*) from sessions": 2, "select count(*) from turns where state='completed'": 3, "select count(*) from events where event_kind<>'lineage_observation'": 23, "select count(*) from evidence_refs": 23, "select count(*) from coverage_observation_versions where field_key='usage'": 3, "select sum(total_tokens) from turn_usage": 2500, "select count(*) from tool_calls": 2, "select count(*) from lineage_edges where edge_kind='spawned'": 1, "select count(*) from source_artifact_versions v where v.state='unsupported' and v.revision=(select max(x.revision) from source_artifact_versions x where x.epoch_id=v.epoch_id and x.source_id=v.source_id)": 1, "select count(*) from source_artifact_versions v where v.state='indexing' and v.revision=(select max(x.revision) from source_artifact_versions x where x.epoch_id=v.epoch_id and x.source_id=v.source_id)": 1}
	for q, want := range checks {
		if got := scalar(t, db, q); got != want {
			t.Fatalf("%s: got %d want %d", q, got, want)
		}
	}
	_, _, matches, err := store.Sessions(0, "fake widget", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].SourceSessionID != "root-001" {
		t.Fatalf("root user-message FTS did not resolve to root: %#v", matches)
	}
	_, _, excluded, err := store.Sessions(0, "delegated check", 50)
	if err != nil || len(excluded) != 0 {
		t.Fatalf("assistant descendant content should not be discoverable: matches=%#v err=%v", excluded, err)
	}
	_, rev1, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	second, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if second.Processed != 0 || second.Skipped != 5 {
		t.Fatalf("repeat scan changed work: %#v", second)
	}
	store, err = storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	_, rev2, _ := store.Snapshot()
	if rev2 != rev1 || scalar(t, store.DB(), "select sum(total_tokens) from turn_usage") != 2500 {
		t.Fatalf("idempotence failed: revisions %d -> %d", rev1, rev2)
	}
}

func TestCheckpointReconciliationParsesOnlyChangedSources(t *testing.T) {
	layout, codex := setup(t)
	cfg := Config{Layout: layout, CodexHome: codex, Concurrency: 2}
	if _, err := Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	var parsed atomic.Int64
	cfg.OnParse = func(sources.Candidate) { parsed.Add(1) }
	unchanged, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Load() != 0 || unchanged.Processed != 0 || unchanged.Skipped != unchanged.Inventoried {
		t.Fatalf("unchanged reconciliation parsed sources: parsed=%d progress=%#v", parsed.Load(), unchanged)
	}

	root := filepath.Join(codex, "sessions", "2026", "07", "01", "rollout-root-001.jsonl")
	f, err := os.OpenFile(root, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString(appendTurn); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}

	parsed.Store(0)
	changed, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Load() != 1 || changed.Processed != 1 || changed.Skipped != changed.Inventoried-1 {
		t.Fatalf("append did not isolate changed source: parsed=%d progress=%#v", parsed.Load(), changed)
	}
}

func TestCheckpointAdapterUpgradeReprocessesUnchangedSource(t *testing.T) {
	layout, codex := setup(t)
	var reported []Progress
	cfg := Config{Layout: layout, CodexHome: codex, Concurrency: 2, OnCommit: func(progress Progress) { reported = append(reported, progress) }}
	rootPath := filepath.Join(codex, "sessions", "2026", "07", "01", "rollout-root-001.jsonl")
	rootPath, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := sources.Parse(sources.Candidate{Path: rootPath, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	batch.Source.AdapterVersion = "obsolete-adapter"
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Apply(context.Background(), batch, "old_adapter_seed"); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	// Adapter upgrades replace the whole catalog, but one malformed source must
	// remain a source-level diagnostic rather than aborting and retrying the
	// deterministic rebuild forever.
	badPath := filepath.Join(codex, "sessions", "2026", "07", "01", "rollout-bad.jsonl")
	if err = os.WriteFile(badPath, []byte("{not-json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	progress, err := Run(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !progress.Rebuilt || progress.Processed+progress.Failed != progress.Inventoried || progress.Failed != 1 || progress.Skipped != 0 {
		t.Fatalf("adapter upgrade did not rebuild unchanged source: progress=%#v", progress)
	}
	sawStart, sawComplete, sawFinalizing := false, false, false
	for _, update := range reported {
		if update.Stage == "rebuilding" && update.Scanned == 0 {
			sawStart = true
		}
		if update.Stage == "rebuilding" && update.Scanned == update.Inventoried {
			sawComplete = true
		}
		if update.Stage == "finalizing" && update.Scanned == update.Inventoried {
			sawFinalizing = true
		}
	}
	if !sawStart || !sawComplete || !sawFinalizing {
		t.Fatalf("adapter rebuild did not publish scan progress: %#v", reported)
	}
}

func TestRebuildQuarantinesSourceThatExceedsTransactionBound(t *testing.T) {
	batch := facts.Batch{
		Source: facts.Source{Path: "/tmp/oversized.jsonl", Kind: "active_rollout", Size: 9 << 20, MTimeNS: 1},
		Events: []facts.Event{{Kind: "task_complete", PayloadLength: (8 << 20) + 1}},
	}
	chunks, quarantined, err := rebuildChunks(batch, true)
	if err != nil {
		t.Fatal(err)
	}
	if !quarantined || len(chunks) != 1 || chunks[0].Source.State != "failed" || chunks[0].Source.StateReason != "terminal_turn_exceeds_transaction_bound" {
		t.Fatalf("oversized rebuild source was not quarantined: quarantined=%t chunks=%#v", quarantined, chunks)
	}
}

func TestAppendChangesOnlyNewTurnAndPinnedRevision(t *testing.T) {
	layout, codex := setup(t)
	cfg := Config{Layout: layout, CodexHome: codex}
	if _, e := Run(context.Background(), cfg); e != nil {
		t.Fatal(e)
	}
	store, e := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if e != nil {
		t.Fatal(e)
	}
	epoch, oldRev, _, e := store.Sessions(0, "root-001", 50)
	if e != nil {
		t.Fatal(e)
	}
	oldEvents := scalar(t, store.DB(), "select count(*) from events")
	store.Close()
	root := filepath.Join(codex, "sessions", "2026", "07", "01", "rollout-root-001.jsonl")
	f, e := os.OpenFile(root, os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.WriteString(appendTurn)
	_ = f.Close()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Run(context.Background(), cfg); e != nil {
		t.Fatal(e)
	}
	store, e = storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	newEpoch, newRev, latest, e := store.Sessions(0, "root-001", 50)
	if e != nil {
		t.Fatal(e)
	}
	_, pinnedRev, pinned, e := store.Sessions(oldRev, "root-001", 50)
	if e != nil {
		t.Fatal(e)
	}
	if epoch != newEpoch || pinnedRev != oldRev || newRev <= oldRev || len(latest) != 1 || latest[0].CompletedTurns != 4 || len(pinned) != 1 || pinned[0].CompletedTurns != 3 {
		t.Fatalf("revision pin mismatch latest=%#v pinned=%#v revisions=%d/%d", latest, pinned, oldRev, newRev)
	}
	if scalar(t, store.DB(), "select sum(total_tokens) from turn_usage") != 2600 || scalar(t, store.DB(), "select count(*) from events") != oldEvents+4 {
		t.Fatal("append changed unexpected facts")
	}
}

func TestAppendToPreviouslyChunkedSourceAdvancesCheckpoint(t *testing.T) {
	root := t.TempDir()
	codex := filepath.Join(root, "codex")
	sessions := filepath.Join(codex, "sessions")
	layout := home.Layout{Root: filepath.Join(root, "inspector")}
	layout.Reviews = filepath.Join(layout.Root, "reviews")
	layout.Queue = filepath.Join(layout.Root, "queue")
	layout.Run = filepath.Join(layout.Root, "run")
	layout.Logs = filepath.Join(layout.Root, "logs")
	layout.Cache = filepath.Join(layout.Root, "cache")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessions, "rollout-chunked.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(`{"timestamp":"2026-07-01T00:00:00Z","type":"session_meta","payload":{"session_id":"chunked-append","cwd":"/fake/chunked","originator":"codex-tui","cli_version":"0.144.1","source":"cli"}}` + "\n"); err != nil {
		t.Fatal(err)
	}
	writeCompletedTurns(t, file, 0, 200)
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Layout: layout, CodexHome: codex, Concurrency: 1}
	if _, err = Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	file, err = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writeCompletedTurns(t, file, 200, 200)
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	progress, err := Run(context.Background(), cfg)
	if err != nil || progress.Processed != 1 || progress.Failed != 0 {
		t.Fatalf("chunked append failed: progress=%#v err=%v", progress, err)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got := scalar(t, store.DB(), "select count(*) from turns where state='completed'"); got != 400 {
		t.Fatalf("completed turns=%d, want 400", got)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	var offset int64
	if err = store.DB().QueryRow("select max(complete_byte_offset) from source_checkpoints").Scan(&offset); err != nil || offset != info.Size() {
		t.Fatalf("checkpoint offset=%d size=%d err=%v", offset, info.Size(), err)
	}
}

func writeCompletedTurns(t *testing.T, file *os.File, start, count int) {
	t.Helper()
	for i := start; i < start+count; i++ {
		turnID := fmt.Sprintf("turn-%04d", i)
		for _, line := range []string{
			fmt.Sprintf(`{"timestamp":"2026-07-01T00:00:01Z","type":"turn_context","payload":{"turn_id":%q,"cwd":"/fake/chunked","model":"gpt-fake","effort":"low"}}`, turnID),
			fmt.Sprintf(`{"timestamp":"2026-07-01T00:00:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":%q}}`, turnID),
			fmt.Sprintf(`{"timestamp":"2026-07-01T00:00:03Z","type":"event_msg","payload":{"type":"task_complete","turn_id":%q}}`, turnID),
		} {
			if _, err := file.WriteString(line + "\n"); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestOversizedTerminalTurnIsQuarantinedWithoutFailingPass(t *testing.T) {
	root := t.TempDir()
	codex := filepath.Join(root, "codex")
	sessions := filepath.Join(codex, "sessions")
	layout := home.Layout{Root: filepath.Join(root, "inspector")}
	layout.Reviews = filepath.Join(layout.Root, "reviews")
	layout.Queue = filepath.Join(layout.Root, "queue")
	layout.Run = filepath.Join(layout.Root, "run")
	layout.Logs = filepath.Join(layout.Root, "logs")
	layout.Cache = filepath.Join(layout.Root, "cache")
	if err := os.MkdirAll(sessions, 0o700); err != nil {
		t.Fatal(err)
	}
	var data strings.Builder
	data.WriteString(`{"timestamp":"2026-07-01T00:00:00Z","type":"session_meta","payload":{"session_id":"oversized-turn","cwd":"/fake/large","originator":"codex-tui","cli_version":"0.144.1","source":"cli"}}` + "\n")
	data.WriteString(`{"timestamp":"2026-07-01T00:00:01Z","type":"turn_context","payload":{"turn_id":"turn-large","cwd":"/fake/large","model":"gpt-fake","effort":"low"}}` + "\n")
	payload := strings.Repeat("x", 1<<20)
	for i := 0; i < 9; i++ {
		fmt.Fprintf(&data, `{"timestamp":"2026-07-01T00:00:02Z","type":"response_item","payload":{"type":"message","role":"assistant","phase":"analysis","content":[{"type":"output_text","text":"%s"}]}}`+"\n", payload)
	}
	data.WriteString(`{"timestamp":"2026-07-01T00:00:03Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"turn-large"}}` + "\n")
	if err := os.WriteFile(filepath.Join(sessions, "rollout-oversized.jsonl"), []byte(data.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	progress, err := Run(context.Background(), Config{Layout: layout, CodexHome: codex, Concurrency: 1})
	if err != nil || progress.Failed != 1 || progress.FailureReasons["terminal_turn_exceeds_transaction_bound"] != 1 {
		t.Fatalf("oversized source was not locally quarantined: progress=%#v err=%v", progress, err)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var reason string
	if err = store.DB().QueryRow(`SELECT state_reason FROM source_artifact_versions WHERE state='failed' ORDER BY revision DESC LIMIT 1`).Scan(&reason); err != nil || reason != "terminal_turn_exceeds_transaction_bound" {
		t.Fatalf("quarantine reason=%q err=%v", reason, err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	repeated, err := Run(context.Background(), Config{Layout: layout, CodexHome: codex, Concurrency: 1})
	if err != nil || repeated.Processed != 0 || repeated.Skipped != 1 {
		t.Fatalf("unchanged quarantine was not skipped: progress=%#v err=%v", repeated, err)
	}
}

func TestRecoveredPathFailureRebuildsAwayStaleDiagnostic(t *testing.T) {
	layout, codex := setup(t)
	cfg := Config{Layout: layout, CodexHome: codex, Concurrency: 1}
	if _, err := Run(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(codex, "sessions", "2026", "07", "01", "rollout-root-001.jsonl")
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	candidate := sources.Candidate{Path: root, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}
	if err = storeFailed(context.Background(), store, candidate, "normalization_failed"); err != nil {
		store.Close()
		t.Fatal(err)
	}
	recoverable, recoverableErr := store.HasRecoverableFailedSources()
	if recoverableErr != nil || !recoverable {
		store.Close()
		t.Fatalf("seeded duplicate path failure was not detected: recoverable=%t err=%v", recoverable, recoverableErr)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(root, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.WriteString(appendTurn); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	progress, err := Run(context.Background(), cfg)
	if err != nil || !progress.Rebuilt || progress.Failed != 0 {
		t.Fatalf("recovered failure did not replace catalog: progress=%#v err=%v", progress, err)
	}
	store, err = storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got := scalar(t, store.DB(), `SELECT count(*) FROM source_artifact_versions v WHERE v.state='failed' AND v.revision=(SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id)`); got != 0 {
		t.Fatalf("stale failed diagnostics=%d, want 0", got)
	}
}

func TestChangedPrefixAutomaticallyRebuildsAndWriterLock(t *testing.T) {
	layout, codex := setup(t)
	cfg := Config{Layout: layout, CodexHome: codex}
	if _, e := Run(context.Background(), cfg); e != nil {
		t.Fatal(e)
	}
	store, e := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if e != nil {
		t.Fatal(e)
	}
	beforeEpoch, _, e := store.Snapshot()
	store.Close()
	if e != nil {
		t.Fatal(e)
	}
	root := filepath.Join(codex, "sessions", "2026", "07", "01", "rollout-root-001.jsonl")
	b, e := os.ReadFile(root)
	if e != nil {
		t.Fatal(e)
	}
	b[strings.Index(string(b), "Create the fake widget")] = 'c'
	if e = os.WriteFile(root, b, 0600); e != nil {
		t.Fatal(e)
	}
	p, e := Run(context.Background(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	if p.RequiresRebuild != 0 || p.Failed != 0 {
		t.Fatalf("changed prefix did not complete replacement rebuild: %#v", p)
	}
	store, e = storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if e != nil {
		t.Fatal(e)
	}
	afterEpoch, _, e := store.Snapshot()
	if e != nil {
		store.Close()
		t.Fatal(e)
	}
	if afterEpoch == beforeEpoch || scalar(t, store.DB(), "select count(*) from source_artifact_versions v where v.state='requires_rebuild' and v.revision=(select max(x.revision) from source_artifact_versions x where x.epoch_id=v.epoch_id and x.source_id=v.source_id)") != 0 {
		store.Close()
		t.Fatalf("replacement epoch not activated: before=%s after=%s", beforeEpoch, afterEpoch)
	}
	store.Close()
	lock, e := proc.Acquire(filepath.Join(layout.Run, "writer.lock"), true)
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	if _, e = Run(context.Background(), cfg); e == nil || !strings.Contains(e.Error(), "writer") {
		t.Fatalf("single writer not enforced: %v", e)
	}
}

func TestPartialRecentFactsQueryableBeforeFullScanCompletes(t *testing.T) {
	layout, codex := setup(t)
	recent := filepath.Join(codex, "sessions", "2026", "07", "01", "rollout-root-001.jsonl")
	now := time.Now()
	_ = os.Chtimes(recent, now, now)
	reached := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		_, e := Run(context.Background(), Config{Layout: layout, CodexHome: codex, Concurrency: 1, OnCommit: func(p Progress) {
			if p.Processed == 2 {
				close(reached)
				<-release
			}
		}})
		done <- e
	}()
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("no partial commit")
	}
	store, e := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if e != nil {
		t.Fatal(e)
	}
	_, _, sessions, e := store.Sessions(0, "root-001", 50)
	store.Close()
	if e != nil || len(sessions) != 1 || sessions[0].CompletedTurns != 2 {
		t.Fatalf("recent facts unavailable during scan: %#v %v", sessions, e)
	}
	close(release)
	if e = <-done; e != nil {
		t.Fatal(e)
	}
}

func TestOversizedEventCountTurnCommitsAtomicallyWithinByteBudget(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large-turn.jsonl")
	var data strings.Builder
	data.WriteString(`{"timestamp":"2026-07-01T00:00:00Z","type":"session_meta","payload":{"session_id":"large-turn","cwd":"/fake/large","originator":"codex-tui","cli_version":"0.144.1","source":"cli"}}` + "\n")
	data.WriteString(`{"timestamp":"2026-07-01T00:00:01Z","type":"turn_context","payload":{"turn_id":"turn-large","cwd":"/fake/large","model":"gpt-fake","effort":"low"}}` + "\n")
	data.WriteString(`{"timestamp":"2026-07-01T00:00:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-large"}}` + "\n")
	for i := 0; i < 501; i++ {
		fmt.Fprintf(&data, `{"timestamp":"2026-07-01T00:00:03Z","type":"response_item","payload":{"type":"message","role":"assistant","phase":"analysis","content":[{"type":"output_text","text":"synthetic-%03d"}]}}`+"\n", i)
	}
	data.WriteString(`{"timestamp":"2026-07-01T00:00:04Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":1,"reasoning_output_tokens":0,"total_tokens":2}}}}` + "\n")
	data.WriteString(`{"timestamp":"2026-07-01T00:00:05Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"turn-large"}}` + "\n")
	if err := os.WriteFile(path, []byte(data.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	batch, err := sources.Parse(sources.Candidate{Path: path, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Events) <= 500 {
		t.Fatalf("fixture does not cross event target: %d", len(batch.Events))
	}
	chunks, err := splitBatches(batch)
	if err != nil || len(chunks) != 1 {
		t.Fatalf("one bounded terminal turn was split/rejected: chunks=%d err=%v", len(chunks), err)
	}
	tooLarge := batch
	tooLarge.Events = append([]facts.Event(nil), batch.Events...)
	tooLarge.Events[0].PayloadLength = (8 << 20) + 1
	if rejected, splitErr := splitBatches(tooLarge); splitErr == nil || rejected != nil {
		t.Fatalf("over-budget terminal unit was not rejected during preflight: chunks=%d err=%v", len(rejected), splitErr)
	}
	dbPath := filepath.Join(dir, "inspector.db")
	store, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	epoch, before, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = store.Apply(canceled, chunks[0], "normalize"); err == nil {
		t.Fatal("canceled transaction unexpectedly committed")
	}
	_, afterCancel, _ := store.Snapshot()
	if afterCancel != before || scalar(t, store.DB(), "select count(*) from events") != 0 || scalar(t, store.DB(), "select count(*) from source_checkpoints") != 0 {
		t.Fatal("canceled transaction exposed partial turn facts or checkpoint")
	}
	revision, err := store.Apply(context.Background(), chunks[0], "normalize")
	if err != nil {
		t.Fatal(err)
	}
	if revision != before+1 || scalar(t, store.DB(), "select count(*) from events") != int64(len(batch.Events)) {
		t.Fatal("successful terminal unit was not committed in one revision")
	}
	checkpoint, err := store.Checkpoint(batch.Source.ID)
	if err != nil || checkpoint.Offset != batch.Source.CompleteOffset || checkpoint.Prefix != batch.Source.PrefixSHA256 {
		t.Fatalf("checkpoint not aligned with committed unit: %#v err=%v", checkpoint, err)
	}
	oldEpoch, oldRevision, oldRows, err := store.Sessions(before, "large-turn", 10)
	if err != nil || oldEpoch != epoch || oldRevision != before || len(oldRows) != 0 {
		t.Fatalf("pre-commit revision exposed turn: epoch=%s revision=%d rows=%d err=%v", oldEpoch, oldRevision, len(oldRows), err)
	}
	_, _, latestRows, err := store.Sessions(revision, "large-turn", 10)
	if err != nil || len(latestRows) != 1 || latestRows[0].CompletedTurns != 1 {
		t.Fatalf("committed turn absent at pinned revision: %#v err=%v", latestRows, err)
	}
}

const appendTurn = `{"timestamp":"2026-07-01T12:00:01Z","type":"turn_context","payload":{"turn_id":"turn-root-3","cwd":"/fake/acme","model":"gpt-fake","effort":"high"}}
{"timestamp":"2026-07-01T12:00:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-root-3"}}
{"timestamp":"2026-07-01T12:00:03Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"input_tokens":80,"cached_input_tokens":20,"output_tokens":20,"reasoning_output_tokens":5,"total_tokens":100},"total_token_usage":{"input_tokens":1680,"cached_input_tokens":520,"output_tokens":420,"reasoning_output_tokens":85,"total_tokens":2100}}}}
{"timestamp":"2026-07-01T12:00:04Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"turn-root-3"}}
`

func TestNoHiddenHistoryCap(t *testing.T) {
	layout, codex := setup(t)
	active := filepath.Join(codex, "sessions", "bulk")
	if e := os.MkdirAll(active, 0700); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 205; i++ {
		id := fmt.Sprintf("bulk-%03d", i)
		data := fmt.Sprintf("{\"timestamp\":\"2026-06-01T00:00:00Z\",\"type\":\"session_meta\",\"payload\":{\"session_id\":%q,\"cwd\":\"/fake/bulk\",\"originator\":\"codex-tui\",\"cli_version\":\"0.144.1\",\"source\":\"cli\"}}\n", id)
		if e := os.WriteFile(filepath.Join(active, id+".jsonl"), []byte(data), 0600); e != nil {
			t.Fatal(e)
		}
	}
	p, e := Run(context.Background(), Config{Layout: layout, CodexHome: codex})
	if e != nil {
		t.Fatal(e)
	}
	if p.Inventoried != 210 || p.Processed != 210 {
		t.Fatalf("history capped: %#v", p)
	}
}

func TestHookQueueCoalescesAndConsumesAfterSuccessfulReconciliation(t *testing.T) {
	layout, codex := setup(t)
	if err := home.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		marker := hook.Marker{SchemaVersion: "inspector.hook-marker/v1", ProtocolVersion: 1, EventKind: "turn_stop", SessionID: "root-001", TurnID: "turn-root-2", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		b, _ := json.Marshal(marker)
		if err := os.WriteFile(filepath.Join(layout.Queue, fmt.Sprintf("%d.json", i)), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := Run(context.Background(), Config{Layout: layout, CodexHome: codex})
	if err != nil {
		t.Fatal(err)
	}
	if p.QueueConsumed != 2 {
		t.Fatalf("markers not consumed: %#v", p)
	}
	entries, err := os.ReadDir(layout.Queue)
	if err != nil || len(entries) != 0 {
		t.Fatalf("queue not empty: %d %v", len(entries), err)
	}
}

func TestMarkerForExistingMultiSegmentSessionDoesNotRotateEpoch(t *testing.T) {
	layout, codex := setup(t)
	resumeDir := filepath.Join(codex, "sessions", "resume")
	if err := os.MkdirAll(resumeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	resume := `{"timestamp":"2026-07-04T00:00:00Z","type":"session_meta","payload":{"session_id":"root-001","timestamp":"2026-07-04T00:00:00Z","cwd":"/fake/acme","originator":"codex-tui","cli_version":"0.144.1","source":"cli"}}` + "\n" +
		`{"timestamp":"2026-07-04T00:00:01Z","type":"turn_context","payload":{"turn_id":"resume-turn","model":"gpt-5","effort":"high","cwd":"/fake/acme"}}` + "\n" +
		`{"timestamp":"2026-07-04T00:00:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":"resume-turn"}}` + "\n" +
		`{"timestamp":"2026-07-04T00:00:03Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"resume-turn"}}` + "\n"
	if err := os.WriteFile(filepath.Join(resumeDir, "resume.jsonl"), []byte(resume), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), Config{Layout: layout, CodexHome: codex}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	beforeEpoch, _, err := store.Snapshot()
	_ = store.Close()
	if err != nil {
		t.Fatal(err)
	}
	beforeFiles, err := filepath.Glob(filepath.Join(layout.Root, "index-v2-*.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	marker := hook.Marker{SchemaVersion: "inspector.hook-marker/v1", ProtocolVersion: 1, EventKind: "turn_stop", SessionID: "root-001", TurnID: "turn-root-2", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	encoded, _ := json.Marshal(marker)
	if err = os.WriteFile(filepath.Join(layout.Queue, "multi-segment.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	progress, err := Run(context.Background(), Config{Layout: layout, CodexHome: codex})
	if err != nil {
		t.Fatal(err)
	}
	store, err = storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	afterEpoch, _, err := store.Snapshot()
	_ = store.Close()
	if err != nil {
		t.Fatal(err)
	}
	afterFiles, err := filepath.Glob(filepath.Join(layout.Root, "index-v2-*.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if progress.Rebuilt || progress.QueueConsumed != 1 || afterEpoch != beforeEpoch || len(afterFiles) != len(beforeFiles) {
		t.Fatalf("marker rotated multi-segment epoch: progress=%#v epoch=%s/%s files=%d/%d", progress, beforeEpoch, afterEpoch, len(beforeFiles), len(afterFiles))
	}
}

func TestHookProvenTerminalStatesAndExactSpawningTurn(t *testing.T) {
	root := t.TempDir()
	codex := filepath.Join(root, "codex")
	active := filepath.Join(codex, "sessions")
	inspector := filepath.Join(root, "inspector")
	layout := home.Layout{Root: inspector, Reviews: filepath.Join(inspector, "reviews"), Queue: filepath.Join(inspector, "queue"), Run: filepath.Join(inspector, "run"), Logs: filepath.Join(inspector, "logs"), Cache: filepath.Join(inspector, "cache")}
	if err := os.MkdirAll(active, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := home.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	terminalSource := func(session, turn, terminal string, truncated bool) string {
		data := fmt.Sprintf(`{"timestamp":"2026-07-01T00:00:00Z","type":"session_meta","payload":{"session_id":%q,"cwd":"/fake","originator":"codex-tui","cli_version":"0.144.1","source":"cli"}}`+"\n"+
			`{"timestamp":"2026-07-01T00:00:01Z","type":"turn_context","payload":{"turn_id":%q,"cwd":"/fake","model":"gpt-fake","effort":"low"}}`+"\n"+
			`{"timestamp":"2026-07-01T00:00:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":%q}}`+"\n", session, turn, turn)
		if terminal != "" {
			data += fmt.Sprintf(`{"timestamp":"2026-07-01T00:00:03Z","type":"event_msg","payload":{"type":%q,"turn_id":%q}}`+"\n", terminal, turn)
		}
		if truncated {
			data += `{"timestamp":"2026-07-01T00:00:04Z","type":"response_item"`
		}
		return data
	}
	for name, data := range map[string]string{"aborted.jsonl": terminalSource("aborted-session", "aborted-turn", "turn_aborted", false), "interrupted.jsonl": terminalSource("interrupted-session", "interrupted-turn", "", false), "truncated.jsonl": terminalSource("truncated-session", "truncated-turn", "", true)} {
		if err := os.WriteFile(filepath.Join(active, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for i, marker := range []hook.Marker{{SchemaVersion: "inspector.hook-marker/v1", ProtocolVersion: 1, EventKind: "turn_stop", SessionID: "interrupted-session", TurnID: "interrupted-turn", ObservedAt: "2026-07-01T00:00:05Z"}, {SchemaVersion: "inspector.hook-marker/v1", ProtocolVersion: 1, EventKind: "turn_stop", SessionID: "truncated-session", TurnID: "truncated-turn", ObservedAt: "2026-07-01T00:00:05Z"}} {
		encoded, _ := json.Marshal(marker)
		if err := os.WriteFile(filepath.Join(layout.Queue, fmt.Sprintf("terminal-%d.json", i)), encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Run(context.Background(), Config{Layout: layout, CodexHome: codex}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	for _, state := range []string{"aborted", "interrupted", "reconciled_truncated"} {
		if got := scalar(t, store.DB(), `SELECT count(*) FROM turns WHERE state='`+state+`'`); got != 1 {
			t.Fatalf("%s turns=%d", state, got)
		}
	}
	if got := scalar(t, store.DB(), `SELECT count(*) FROM turn_usage`); got != 0 {
		t.Fatalf("non-completed terminal metrics=%d", got)
	}
	if got := scalar(t, store.DB(), `SELECT count(*) FROM events WHERE turn_id IS NULL`); got != 0 {
		t.Fatalf("unwatermarked null-turn events=%d", got)
	}

	// A subagent marker enriches lineage only when parent, child, and turn IDs
	// all match source identities; it never guesses by timing.
	layout2, codex2 := setup(t)
	if err = home.Ensure(layout2); err != nil {
		t.Fatal(err)
	}
	marker := hook.Marker{SchemaVersion: "inspector.hook-marker/v1", ProtocolVersion: 1, EventKind: "subagent_start", SessionID: "root-001", TurnID: "turn-root-1", SubagentID: "child-001", ObservedAt: "2026-07-01T10:00:08Z"}
	encoded, _ := json.Marshal(marker)
	if err = os.WriteFile(filepath.Join(layout2.Queue, "spawn.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = Run(context.Background(), Config{Layout: layout2, CodexHome: codex2}); err != nil {
		t.Fatal(err)
	}
	spawnStore, err := storage.Open(filepath.Join(layout2.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer spawnStore.Close()
	if got := scalar(t, spawnStore.DB(), `SELECT count(*) FROM lineage_edges l JOIN turns t ON t.id=l.spawning_turn_id WHERE l.edge_kind='spawned' AND t.source_turn_id='turn-root-1'`); got != 1 {
		t.Fatalf("exact spawning turn lineage rows=%d", got)
	}
}

func TestLineageForkContinuationResumeAndOrphanOwnership(t *testing.T) {
	layout, codex := setup(t)
	dir := filepath.Join(codex, "sessions", "lineage")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	records := map[string]string{
		"fork.jsonl":         metaOnly("fork-001", `"parent_thread_id":"root-001",`, "\"cli\""),
		"continuation.jsonl": metaOnly("continuation-001", `"continued_from_thread_id":"root-001",`, "\"cli\""),
		"orphan.jsonl":       metaOnly("orphan-child", `"parent_thread_id":"missing-parent",`, `{"subagent":{"other":"fake"}}`),
		"resume.jsonl": `{"timestamp":"2026-07-04T00:00:00Z","type":"session_meta","payload":{"session_id":"root-001","timestamp":"2026-07-04T00:00:00Z","cwd":"/fake/acme","originator":"codex-tui","cli_version":"0.144.1","source":"cli","git":{"repository_url":"https://example.invalid/acme/widgets.git"}}}` + "\n" +
			`{"timestamp":"2026-07-04T00:00:01Z","type":"turn_context","payload":{"turn_id":"resume-turn","model":"gpt-5","effort":"high","cwd":"/fake/acme"}}` + "\n" +
			`{"timestamp":"2026-07-04T00:00:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":"resume-turn"}}` + "\n" +
			`{"timestamp":"2026-07-04T00:00:03Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"resume-turn"}}` + "\n",
	}
	for name, data := range records {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Run(context.Background(), Config{Layout: layout, CodexHome: codex}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	db := store.DB()
	if got := scalar(t, db, "select count(*) from sessions where source_session_id='root-001'"); got != 1 {
		t.Fatalf("resume split logical session: %d", got)
	}
	if got := scalar(t, db, "select count(*) from source_segments sg join sessions s on s.id=sg.session_id where s.source_session_id='root-001'"); got != 2 {
		t.Fatalf("resume segments=%d", got)
	}
	if got := scalar(t, db, "select count(*) from lineage_edges where edge_kind='forked_from'"); got != 1 {
		t.Fatalf("fork edges=%d", got)
	}
	if got := scalar(t, db, "select count(*) from lineage_edges where edge_kind='continued_as'"); got != 1 {
		t.Fatalf("continuation edges=%d", got)
	}
	var purpose, rootID, coverage string
	if err = db.QueryRow("select v.purpose,r.source_session_id,v.lineage_coverage from sessions s join session_versions v on v.session_id=s.id join sessions r on r.id=v.root_work_unit_id where s.source_session_id='orphan-child' order by v.revision desc limit 1").Scan(&purpose, &rootID, &coverage); err != nil {
		t.Fatal(err)
	}
	if purpose != "orphan" || rootID != "orphan-child" || coverage != "partial" {
		t.Fatalf("orphan ownership guessed: %s %s %s", purpose, rootID, coverage)
	}
}

func TestReverseIngestedResumeUsesSharedCumulativeBaselineAndSessionBounds(t *testing.T) {
	root := t.TempDir()
	codex := filepath.Join(root, "codex")
	active := filepath.Join(codex, "sessions")
	if err := os.MkdirAll(active, 0o700); err != nil {
		t.Fatal(err)
	}
	layout := home.Layout{Root: filepath.Join(root, "inspector"), Reviews: filepath.Join(root, "inspector", "reviews"), Queue: filepath.Join(root, "inspector", "queue"), Run: filepath.Join(root, "inspector", "run"), Logs: filepath.Join(root, "inspector", "logs"), Cache: filepath.Join(root, "inspector", "cache")}
	segment := func(start, turn string, last, cumulative int64) string {
		lastJSON := ""
		if last > 0 {
			lastJSON = fmt.Sprintf(`"last_token_usage":{"input_tokens":%d,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0,"total_tokens":%d},`, last, last)
		}
		return fmt.Sprintf(`{"timestamp":%q,"type":"session_meta","payload":{"session_id":"resume-shared","timestamp":%q,"cwd":"/fake/resume","originator":"codex-tui","cli_version":"0.144.1","source":"cli"}}`+"\n"+
			`{"timestamp":%q,"type":"turn_context","payload":{"turn_id":%q,"cwd":"/fake/resume","model":"gpt-fake","effort":"low"}}`+"\n"+
			`{"timestamp":%q,"type":"event_msg","payload":{"type":"task_started","turn_id":%q}}`+"\n"+
			`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{%s"total_token_usage":{"input_tokens":%d,"cached_input_tokens":0,"output_tokens":0,"reasoning_output_tokens":0,"total_tokens":%d}}}}`+"\n"+
			`{"timestamp":%q,"type":"event_msg","payload":{"type":"task_complete","turn_id":%q}}`+"\n", start, start, start, turn, start, turn, start, lastJSON, cumulative, cumulative, start, turn)
	}
	older, newer := filepath.Join(active, "older.jsonl"), filepath.Join(active, "newer.jsonl")
	if err := os.WriteFile(older, []byte(segment("2026-07-01T10:00:00Z", "turn-old", 100, 100)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte(segment("2026-07-01T11:00:00Z", "turn-new", 0, 250)), 0o600); err != nil {
		t.Fatal(err)
	}
	expectedSegments := map[string]string{}
	expectedTurns := map[string]string{}
	for _, path := range []string{older, newer} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		batch, parseErr := sources.Parse(sources.Candidate{Path: path, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		key, keyErr := phase0.SourceOrderKey(batch.Segment.StartedAt, batch.Segment.Fingerprint)
		if keyErr != nil {
			t.Fatal(keyErr)
		}
		expectedSegments[batch.Segment.Fingerprint] = key
		for _, turn := range batch.Turns {
			expectedTurns[turn.SourceTurnID], keyErr = phase0.TurnOrderKey(key, turn.Ordinal)
			if keyErr != nil {
				t.Fatal(keyErr)
			}
		}
	}
	oldTime := time.Now().Add(-time.Hour)
	if err := os.Chtimes(older, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, time.Now(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), Config{Layout: layout, CodexHome: codex, Concurrency: 2}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got := scalar(t, store.DB(), `SELECT coalesce(sum(total_tokens),0) FROM turn_usage`); got != 250 {
		t.Fatalf("cross-segment total=%d want=250", got)
	}
	if got := scalar(t, store.DB(), `SELECT count(*) FROM turn_usage WHERE normalization_kind='cumulative_delta' AND total_tokens=150`); got != 1 {
		t.Fatalf("derived resumed contribution rows=%d", got)
	}
	var earliest, latest string
	if err = store.DB().QueryRow(`SELECT earliest_proven_start,latest_terminal_end FROM session_versions v JOIN sessions s ON s.id=v.session_id WHERE s.source_session_id='resume-shared' ORDER BY v.revision DESC LIMIT 1`).Scan(&earliest, &latest); err != nil {
		t.Fatal(err)
	}
	if earliest != "2026-07-01T10:00:00Z" || latest != "2026-07-01T11:00:00Z" {
		t.Fatalf("session bounds=%s..%s", earliest, latest)
	}
	if got := scalar(t, store.DB(), `SELECT count(*) FROM source_segments sg JOIN sessions s ON s.id=sg.session_id WHERE s.source_session_id='resume-shared'`); got != 2 {
		t.Fatalf("resume segments=%d", got)
	}
	segmentRows, err := store.DB().Query(`SELECT segment_fingerprint,source_order_key FROM source_segments ORDER BY source_order_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer segmentRows.Close()
	for segmentRows.Next() {
		var fingerprint, key string
		if err = segmentRows.Scan(&fingerprint, &key); err != nil {
			t.Fatal(err)
		}
		if key != expectedSegments[fingerprint] {
			t.Fatalf("segment order key=%q want=%q", key, expectedSegments[fingerprint])
		}
	}
	turnRows, err := store.DB().Query(`SELECT source_turn_id,source_order_key FROM turns ORDER BY source_order_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer turnRows.Close()
	for turnRows.Next() {
		var turnID, key string
		if err = turnRows.Scan(&turnID, &key); err != nil {
			t.Fatal(err)
		}
		if key != expectedTurns[turnID] {
			t.Fatalf("turn order key=%q want=%q", key, expectedTurns[turnID])
		}
	}
}
func metaOnly(id, extra, source string) string {
	turnID := id + "-turn"
	return fmt.Sprintf(`{"timestamp":"2026-07-03T00:00:00Z","type":"session_meta","payload":{"session_id":%q,%s"timestamp":"2026-07-03T00:00:00Z","cwd":"/fake/acme","originator":"codex-tui","cli_version":"0.144.1","source":%s}}`+"\n"+
		`{"timestamp":"2026-07-03T00:00:01Z","type":"turn_context","payload":{"turn_id":%q,"model":"gpt-5","effort":"high","cwd":"/fake/acme"}}`+"\n"+
		`{"timestamp":"2026-07-03T00:00:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":%q}}`+"\n"+
		`{"timestamp":"2026-07-03T00:00:03Z","type":"event_msg","payload":{"type":"task_complete","turn_id":%q,"completed_at":1770000000}}`+"\n", id, extra, source, turnID, turnID, turnID)
}

func TestReviewRootClassificationUsesRecordedInspectorReviewDirectory(t *testing.T) {
	layout, codex := setup(t)
	if err := home.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	reviewDir := filepath.Join(layout.Reviews, "review-001")
	if err := os.MkdirAll(reviewDir, 0700); err != nil {
		t.Fatal(err)
	}
	data := fmt.Sprintf(`{"timestamp":"2026-07-03T00:00:00Z","type":"session_meta","payload":{"session_id":"review-root","timestamp":"2026-07-03T00:00:00Z","cwd":%q,"originator":"codex-exec","cli_version":"0.144.1","source":"exec"}}`+"\n"+
		`{"timestamp":"2026-07-03T00:00:01Z","type":"turn_context","payload":{"turn_id":"review-turn","model":"gpt-5","effort":"high","cwd":%q}}`+"\n"+
		`{"timestamp":"2026-07-03T00:00:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":"review-turn"}}`+"\n"+
		`{"timestamp":"2026-07-03T00:00:03Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"review-turn"}}`+"\n", reviewDir, reviewDir)
	path := filepath.Join(codex, "sessions", "review.jsonl")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(context.Background(), Config{Layout: layout, CodexHome: codex}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var purpose string
	if err = store.DB().QueryRow("select v.purpose from sessions s join session_versions v on v.session_id=s.id where s.source_session_id='review-root' order by v.revision desc limit 1").Scan(&purpose); err != nil {
		t.Fatal(err)
	}
	if purpose != "inspector_review" {
		t.Fatalf("review root classified %q", purpose)
	}
}
