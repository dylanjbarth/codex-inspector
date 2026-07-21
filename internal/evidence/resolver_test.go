package evidence

import (
	"bytes"
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

func TestFingerprintMoveMissingAndBoundedResolution(t *testing.T) {
	root := t.TempDir()
	codex := filepath.Join(root, "codex")
	active := filepath.Join(codex, "sessions")
	archive := filepath.Join(codex, "archived_sessions")
	inspector := filepath.Join(root, "inspector")
	if e := os.MkdirAll(active, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(archive, 0700); e != nil {
		t.Fatal(e)
	}
	_, file, _, _ := runtime.Caller(0)
	fixture := filepath.Join(filepath.Dir(file), "..", "..", "fixtures", "synthetic", "root.jsonl")
	b, e := os.ReadFile(fixture)
	if e != nil {
		t.Fatal(e)
	}
	source := filepath.Join(active, "rollout-root.jsonl")
	if e = os.WriteFile(source, b, 0600); e != nil {
		t.Fatal(e)
	}
	layout := home.Layout{Root: inspector, Reviews: filepath.Join(inspector, "reviews"), Queue: filepath.Join(inspector, "queue"), Run: filepath.Join(inspector, "run"), Logs: filepath.Join(inspector, "logs"), Cache: filepath.Join(inspector, "cache")}
	if _, e = indexer.Run(context.Background(), indexer.Config{Layout: layout, CodexHome: codex}); e != nil {
		t.Fatal(e)
	}
	store, e := storage.Open(filepath.Join(inspector, "inspector.db"))
	if e != nil {
		t.Fatal(e)
	}
	var id string
	if e = store.DB().QueryRow("SELECT r.id FROM evidence_refs r JOIN events e ON e.id=r.event_id WHERE e.event_kind='message' ORDER BY e.record_ordinal LIMIT 1").Scan(&id); e != nil {
		t.Fatal(e)
	}
	chunk, e := Resolve(context.Background(), store, id, 0, 0, 32)
	if e != nil {
		t.Fatal(e)
	}
	if chunk.Availability != "available" || chunk.Bytes != 32 || chunk.Complete {
		t.Fatalf("bad first chunk: %#v", chunk)
	}
	pinnedRevision, pinnedPrefix, pinnedEvent := chunk.AppliedRevision, chunk.SourcePrefixSHA256, chunk.EventFingerprint
	if _, e = Resolve(context.Background(), store, id, 0, 0, MaxLimit+1); e == nil {
		t.Fatal("oversized evidence range accepted")
	}
	store.Close()
	moved := filepath.Join(archive, "rollout-root.jsonl")
	if e = os.Rename(source, moved); e != nil {
		t.Fatal(e)
	}
	store, e = storage.Open(filepath.Join(inspector, "inspector.db"))
	if e != nil {
		t.Fatal(e)
	}
	chunk, e = ResolveAt(context.Background(), store, codex, id, 0, 0, 64)
	if e != nil || chunk.Availability != "available" {
		t.Fatalf("archive rediscovery failed: %#v %v", chunk, e)
	}
	mutated := bytes.Replace(b, []byte("Create the fake widget."), []byte("Create ahe fake widget."), 1)
	if e = os.WriteFile(moved, mutated, 0600); e != nil {
		t.Fatal(e)
	}
	mismatchAt := time.Date(2026, 7, 18, 12, 0, 0, 123, time.UTC)
	chunk, e = resolveAt(context.Background(), store, codex, id, 0, 0, 64, func() time.Time { return mismatchAt })
	if e != nil || chunk.Availability != "fingerprint_mismatch" || chunk.AvailabilityObservedAt != mismatchAt.Format(time.RFC3339Nano) || chunk.AvailabilityRevision != nil {
		t.Fatalf("live mismatch observation is not current/unversioned: %#v %v", chunk, e)
	}
	if chunk.AppliedRevision != pinnedRevision || chunk.SourcePrefixSHA256 != pinnedPrefix || chunk.EventFingerprint != pinnedEvent {
		t.Fatal("live mismatch changed pinned locator facts")
	}
	store.Close()
	if e = os.Remove(moved); e != nil {
		t.Fatal(e)
	}
	store, e = storage.Open(filepath.Join(inspector, "inspector.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	missingAt := mismatchAt.Add(time.Minute)
	chunk, e = resolveAt(context.Background(), store, codex, id, 0, 0, 64, func() time.Time { return missingAt })
	if e != nil || chunk.Availability != "source_missing" || chunk.AvailabilityObservedAt != missingAt.Format(time.RFC3339Nano) || chunk.AvailabilityRevision != nil {
		t.Fatalf("missing source dishonest: %#v %v", chunk, e)
	}
	if chunk.AppliedRevision != pinnedRevision || chunk.SourcePrefixSHA256 != pinnedPrefix || chunk.EventFingerprint != pinnedEvent {
		t.Fatal("live missing result changed pinned locator facts")
	}
}

func TestEscapesUnsafeOrSplitBytes(t *testing.T) {
	text, encoding := escape([]byte{'a', 0x1b, 'b', 0xff})
	if encoding != "escaped-bytes" || text != "a\\x1bb\\xff" {
		t.Fatalf("unsafe evidence not escaped: %q %s", text, encoding)
	}
}
