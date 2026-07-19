package cli

import (
	"bytes"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dylanjbarth/codex-inspector/internal/storage"
	_ "modernc.org/sqlite"
)

func TestSyncRebuildsSchemaV1ThroughValidatedCatalogAndPreservesOnFailure(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(file), "..", "..")
	v1Schema, err := os.ReadFile(filepath.Join(repo, "fixtures", "synthetic", "schema-v1.sql"))
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name     string
		corrupt  bool
		wantCode int
	}{{"success", false, 0}, {"failed candidate", true, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			inspector, codex := filepath.Join(root, "inspector"), filepath.Join(root, "codex")
			if err := os.MkdirAll(filepath.Join(codex, "sessions"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(inspector, 0o700); err != nil {
				t.Fatal(err)
			}
			v1Path := filepath.Join(inspector, "inspector.db")
			db, openErr := sql.Open("sqlite", v1Path)
			if openErr != nil {
				t.Fatal(openErr)
			}
			if _, openErr = db.Exec(string(v1Schema)); openErr != nil {
				t.Fatal(openErr)
			}
			if openErr = db.Close(); openErr != nil {
				t.Fatal(openErr)
			}
			if err := os.WriteFile(filepath.Join(inspector, "active-index"), []byte("inspector.db\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			rollout := filepath.Join(codex, "sessions", "root.jsonl")
			if tc.corrupt {
				err = os.WriteFile(rollout, []byte("{not-json}\n"), 0o600)
			} else {
				data, readErr := os.ReadFile(filepath.Join(repo, "fixtures", "synthetic", "root.jsonl"))
				if readErr != nil {
					t.Fatal(readErr)
				}
				err = os.WriteFile(rollout, data, 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("CODEX_INSPECTOR_HOME", inspector)
			t.Setenv("CODEX_HOME", codex)
			var stdout, stderr bytes.Buffer
			if code := Main([]string{"sync", "--wait"}, IO{In: bytes.NewReader(nil), Out: &stdout, Err: &stderr}); code != tc.wantCode {
				t.Fatalf("exit=%d want=%d stdout=%s stderr=%s", code, tc.wantCode, stdout.String(), stderr.String())
			}
			catalogBytes, readErr := os.ReadFile(filepath.Join(inspector, "active-index"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			activeName := strings.TrimSpace(string(catalogBytes))
			if tc.corrupt {
				if activeName != "inspector.db" {
					t.Fatalf("failed build switched catalog to %q", activeName)
				}
			} else {
				if activeName == "inspector.db" || !strings.HasPrefix(activeName, "index-v2-") {
					t.Fatalf("v1 was not replaced by immutable v2: %q", activeName)
				}
				var version int
				newDB, openErr := sql.Open("sqlite", filepath.Join(inspector, activeName))
				if openErr != nil {
					t.Fatal(openErr)
				}
				defer newDB.Close()
				if openErr = newDB.QueryRow(`SELECT schema_version FROM schema_metadata WHERE singleton=1`).Scan(&version); openErr != nil || version != 2 {
					t.Fatalf("replacement schema=%d err=%v", version, openErr)
				}
			}
			var legacy int
			oldDB, openErr := sql.Open("sqlite", v1Path)
			if openErr != nil {
				t.Fatal(openErr)
			}
			defer oldDB.Close()
			if openErr = oldDB.QueryRow(`SELECT count(*) FROM legacy_facts`).Scan(&legacy); openErr != nil || legacy != 1 {
				t.Fatalf("v1 input changed: count=%d err=%v", legacy, openErr)
			}
		})
	}
}

func TestPhase2CrashHelper(t *testing.T) {
	if os.Getenv("CODEX_INSPECTOR_CRASH_HELPER") != "1" {
		return
	}
	code := Main([]string{"sync", "--wait"}, IO{In: bytes.NewReader(nil), Out: os.Stdout, Err: os.Stderr})
	os.Exit(code)
}

func TestSyncCrashRestartRecoversWALAndCheckpointAtomically(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(file), "..", "..")
	for _, tc := range []struct {
		name, failpoint string
		exit, rootTurns int
	}{{"mid transaction", "apply_before_commit", 86, 0}, {"after durable commit", "apply_after_commit", 87, 2}} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			inspector, codex := filepath.Join(root, "inspector"), filepath.Join(root, "codex")
			active := filepath.Join(codex, "sessions")
			if err := os.MkdirAll(active, 0o700); err != nil {
				t.Fatal(err)
			}
			for _, fixture := range []string{"root.jsonl", "descendant.jsonl"} {
				data, err := os.ReadFile(filepath.Join(repo, "fixtures", "synthetic", fixture))
				if err != nil {
					t.Fatal(err)
				}
				if err = os.WriteFile(filepath.Join(active, fixture), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			run := func(failpoint string) int {
				command := exec.Command(os.Args[0], "-test.run=^TestPhase2CrashHelper$")
				command.Env = append(os.Environ(), "CODEX_INSPECTOR_CRASH_HELPER=1", "CODEX_INSPECTOR_HOME="+inspector, "CODEX_HOME="+codex)
				if failpoint != "" {
					command.Env = append(command.Env, "CODEX_INSPECTOR_FAILPOINT="+failpoint, "CODEX_INSPECTOR_FAILPOINT_SESSION=root-001")
				}
				err := command.Run()
				if err == nil {
					return 0
				}
				if exitErr, ok := err.(*exec.ExitError); ok {
					return exitErr.ExitCode()
				}
				t.Fatalf("run helper: %v", err)
				return -1
			}
			if exit := run(tc.failpoint); exit != tc.exit {
				t.Fatalf("crash exit=%d want=%d", exit, tc.exit)
			}
			store, err := storage.Open(filepath.Join(inspector, "inspector.db"))
			if err != nil {
				t.Fatal(err)
			}
			var rootTurns, rootCheckpoints int
			if err = store.DB().QueryRow(`SELECT count(*) FROM turns t JOIN sessions s ON s.id=t.session_id WHERE s.source_session_id='root-001'`).Scan(&rootTurns); err != nil {
				t.Fatal(err)
			}
			if err = store.DB().QueryRow(`SELECT count(*) FROM source_checkpoints c JOIN source_artifacts a ON a.id=c.source_id WHERE a.source_session_id='root-001'`).Scan(&rootCheckpoints); err != nil {
				t.Fatal(err)
			}
			store.Close()
			if rootTurns != tc.rootTurns || (rootTurns == 0 && rootCheckpoints != 0) || (rootTurns > 0 && rootCheckpoints != 1) {
				t.Fatalf("post-crash root turns/checkpoints=%d/%d", rootTurns, rootCheckpoints)
			}
			if exit := run(""); exit != 0 {
				t.Fatalf("restart exit=%d", exit)
			}
			store, err = storage.Open(filepath.Join(inspector, "inspector.db"))
			if err != nil {
				t.Fatal(err)
			}
			var revision int64
			_, revision, err = store.Snapshot()
			if err != nil {
				t.Fatal(err)
			}
			for query, want := range map[string]int{"SELECT count(*) FROM sessions": 2, "SELECT count(*) FROM turns": 3, "SELECT count(*) FROM events WHERE event_kind<>'lineage_observation'": 23, "SELECT count(*) FROM evidence_refs": 23} {
				var got int
				if err = store.DB().QueryRow(query).Scan(&got); err != nil || got != want {
					t.Fatalf("%s got=%d want=%d err=%v", query, got, want, err)
				}
			}
			store.Close()
			if exit := run(""); exit != 0 {
				t.Fatalf("idempotent restart exit=%d", exit)
			}
			store, err = storage.Open(filepath.Join(inspector, "inspector.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			_, repeatRevision, err := store.Snapshot()
			if err != nil || repeatRevision != revision {
				t.Fatalf("repeat revision=%d want=%d err=%v", repeatRevision, revision, err)
			}
		})
	}
}
