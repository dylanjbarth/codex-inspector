package home

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestResolveAndEnsureUserOnlySeparateHome(t *testing.T) {
	root := t.TempDir()
	inspector := filepath.Join(root, "inspector")
	codex := filepath.Join(root, "codex")
	t.Setenv("CODEX_INSPECTOR_HOME", inspector)
	t.Setenv("CODEX_HOME", codex)
	l, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if l.Root == codex {
		t.Fatal("Inspector home aliases Codex source home")
	}
	if err = Ensure(l); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{l.Root, l.Reviews, l.Queue, l.Run, l.Logs, l.Cache} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Fatalf("%s mode=%o", filepath.Base(p), info.Mode().Perm())
		}
	}
}

func TestResolveRejectsCodexHomeAlias(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", root)
	t.Setenv("CODEX_INSPECTOR_HOME", root)
	if _, err := Resolve(); err == nil {
		t.Fatal("same source and derived home accepted")
	}
}

func TestResolveCodexHomeRecordsSourceAndCanonicalPath(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_HOME", root+"/../"+filepath.Base(root))
	effective, err := ResolveCodexHome()
	canonicalRoot, _ := filepath.EvalSymlinks(root)
	if err != nil || effective.Path != canonicalRoot || effective.Resolution != "environment" {
		t.Fatalf("effective=%+v err=%v", effective, err)
	}
	t.Setenv("CODEX_HOME", "")
	effective, err = ResolveCodexHome()
	if err != nil || effective.Resolution != "default" {
		t.Fatalf("effective=%+v err=%v", effective, err)
	}
}

func TestDatasetBindingCleanInstallAndRejectsDifferentHome(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root, Reviews: filepath.Join(root, "reviews"), Queue: filepath.Join(root, "queue"), Run: filepath.Join(root, "run"), Logs: filepath.Join(root, "logs"), Cache: filepath.Join(root, "cache")}
	if err := Ensure(l); err != nil {
		t.Fatal(err)
	}
	first := CodexHome{Path: filepath.Join(root, "one"), Resolution: "environment"}
	if err := BindDatasetHome(l, first); err != nil {
		t.Fatal(err)
	}
	if err := BindDatasetHome(l, CodexHome{Path: filepath.Join(root, "two"), Resolution: "environment"}); err == nil {
		t.Fatal("accepted another source home")
	}
	b, err := os.ReadFile(DatasetBindingPath(l))
	if err != nil {
		t.Fatal(err)
	}
	var recorded CodexHome
	if err = json.Unmarshal(b, &recorded); err != nil || recorded != first {
		t.Fatalf("binding=%+v err=%v", recorded, err)
	}
}

func TestDatasetBindingUpgradesLegacyCatalogWithoutModifyingDatabase(t *testing.T) {
	legacy, codexHome, database := legacyCatalog(t, true, "sessions/2026/07/rollout.jsonl")
	before, err := os.ReadFile(database)
	if err != nil {
		t.Fatal(err)
	}
	source := CodexHome{Path: codexHome, Resolution: "default"}
	if err = BindDatasetHome(legacy, source); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(database)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("legacy database content changed during binding migration")
	}
	b, err := os.ReadFile(DatasetBindingPath(legacy))
	if err != nil {
		t.Fatal(err)
	}
	var recorded CodexHome
	if err = json.Unmarshal(b, &recorded); err != nil || recorded != source {
		t.Fatalf("binding=%+v err=%v", recorded, err)
	}
	if err = BindDatasetHome(legacy, source); err != nil {
		t.Fatalf("migrated binding is not reusable: %v", err)
	}
}

func TestDatasetBindingUpgradesLegacyInspectorDatabaseWithoutCatalogPointer(t *testing.T) {
	legacy, codexHome, database := legacyCatalog(t, true, "archived_sessions/rollout.jsonl")
	inspectorDatabase := filepath.Join(legacy.Root, "inspector.db")
	if err := os.Rename(database, inspectorDatabase); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(legacy.Root, "active-index")); err != nil {
		t.Fatal(err)
	}
	if err := BindDatasetHome(legacy, CodexHome{Path: codexHome, Resolution: "environment"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(DatasetBindingPath(legacy)); err != nil {
		t.Fatalf("legacy inspector.db was not bound: %v", err)
	}
}

func TestDatasetBindingRejectsLegacyHomeMismatchWithoutWritingBinding(t *testing.T) {
	legacy, _, _ := legacyCatalog(t, true, "sessions/rollout.jsonl")
	otherHome := filepath.Join(t.TempDir(), "other-codex")
	err := BindDatasetHome(legacy, CodexHome{Path: otherHome, Resolution: "environment"})
	if err == nil || !strings.Contains(err.Error(), "not proven to belong") {
		t.Fatalf("err=%v", err)
	}
	if _, statErr := os.Stat(DatasetBindingPath(legacy)); !os.IsNotExist(statErr) {
		t.Fatalf("binding written after mismatch: %v", statErr)
	}
}

func TestDatasetBindingRejectsInvalidBinding(t *testing.T) {
	root := t.TempDir()
	l := testLayout(root)
	if err := Ensure(l); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(DatasetBindingPath(l), []byte(`{"path":`), 0o600); err != nil {
		t.Fatal(err)
	}
	err := BindDatasetHome(l, CodexHome{Path: filepath.Join(root, "codex"), Resolution: "environment"})
	if err == nil || !strings.Contains(err.Error(), "invalid dataset home binding") {
		t.Fatalf("err=%v", err)
	}
}

func TestDatasetBindingRejectsLegacyCatalogWhenIdentityCannotBeEstablished(t *testing.T) {
	legacy, codexHome, _ := legacyCatalog(t, false, "")
	err := BindDatasetHome(legacy, CodexHome{Path: codexHome, Resolution: "environment"})
	if err == nil || !strings.Contains(err.Error(), "identity cannot be established") || !strings.Contains(err.Error(), "separate CODEX_INSPECTOR_HOME") {
		t.Fatalf("err=%v", err)
	}
	if _, statErr := os.Stat(DatasetBindingPath(legacy)); !os.IsNotExist(statErr) {
		t.Fatalf("binding written without identity proof: %v", statErr)
	}
}

func legacyCatalog(t *testing.T, withSource bool, relativeSource string) (Layout, string, string) {
	t.Helper()
	root := t.TempDir()
	l := testLayout(filepath.Join(root, "inspector"))
	if err := Ensure(l); err != nil {
		t.Fatal(err)
	}
	codexHome := filepath.Join(root, "codex")
	database := filepath.Join(l.Root, "legacy.db")
	db, err := sql.Open("sqlite", database)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE source_artifact_versions (canonical_path TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if withSource {
		if _, err = db.Exec(`INSERT INTO source_artifact_versions(canonical_path) VALUES(?)`, filepath.Join(codexHome, relativeSource)); err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(l.Root, "active-index"), []byte(filepath.Base(database)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return l, codexHome, database
}

func testLayout(root string) Layout {
	return Layout{Root: root, Reviews: filepath.Join(root, "reviews"), Queue: filepath.Join(root, "queue"), Run: filepath.Join(root, "run"), Logs: filepath.Join(root, "logs"), Cache: filepath.Join(root, "cache")}
}
