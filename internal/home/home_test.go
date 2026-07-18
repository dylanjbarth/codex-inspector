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
