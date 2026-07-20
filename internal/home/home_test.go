package home

import (
	"os"
	"path/filepath"
	"testing"
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

func TestDatasetBindingRejectsDifferentHomesAndLegacyCatalog(t *testing.T) {
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
	legacyRoot := t.TempDir()
	legacy := Layout{Root: legacyRoot, Reviews: filepath.Join(legacyRoot, "reviews"), Queue: filepath.Join(legacyRoot, "queue"), Run: filepath.Join(legacyRoot, "run"), Logs: filepath.Join(legacyRoot, "logs"), Cache: filepath.Join(legacyRoot, "cache")}
	if err := Ensure(legacy); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy.Root, "active-index"), []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := BindDatasetHome(legacy, first); err == nil {
		t.Fatal("accepted unbound legacy catalog")
	}
}
