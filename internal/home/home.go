package home

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type Layout struct {
	Root, Reviews, Queue, Run, Logs, Cache string
}

// CodexHome is the one canonical source root that an Inspector process may
// read. Resolution is deliberately retained for diagnostics rather than
// inferred later from the environment.
type CodexHome struct {
	Path       string `json:"path"`
	Resolution string `json:"resolution"`
}

func ResolveCodexHome() (CodexHome, error) {
	root := os.Getenv("CODEX_HOME")
	resolution := "environment"
	if root == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return CodexHome{}, err
		}
		root = filepath.Join(h, ".codex")
		resolution = "default"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return CodexHome{}, err
	}
	if canonical, err := filepath.EvalSymlinks(abs); err == nil {
		abs = canonical
	}
	return CodexHome{Path: filepath.Clean(abs), Resolution: resolution}, nil
}

func (c CodexHome) ID() string {
	sum := sha256.Sum256([]byte(c.Path))
	return hex.EncodeToString(sum[:])
}

func DatasetBindingPath(l Layout) string { return filepath.Join(l.Cache, "dataset-home.json") }

// BindDatasetHome prevents a catalog from being silently reused for another
// Codex home. A legacy populated catalog without a binding is intentionally
// rejected: its source identity cannot be proven without a rebuild.
func BindDatasetHome(l Layout, source CodexHome) error {
	path := DatasetBindingPath(l)
	b, err := os.ReadFile(path)
	if err == nil {
		var recorded CodexHome
		if json.Unmarshal(b, &recorded) != nil || recorded.Path == "" || (recorded.Resolution != "environment" && recorded.Resolution != "default") {
			return errors.New("invalid dataset home binding; use a separate CODEX_INSPECTOR_HOME and rebuild")
		}
		if recorded.Path != source.Path {
			return errors.New("active dataset belongs to a different Codex home; set a separate CODEX_INSPECTOR_HOME and rebuild")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(filepath.Join(l.Root, "active-index")); err == nil {
		return errors.New("legacy dataset has no Codex-home binding; set a separate CODEX_INSPECTOR_HOME and rebuild")
	} else if !os.IsNotExist(err) {
		return err
	}
	if _, err := os.Stat(filepath.Join(l.Root, "inspector.db")); err == nil {
		return errors.New("legacy dataset has no Codex-home binding; set a separate CODEX_INSPECTOR_HOME and rebuild")
	} else if !os.IsNotExist(err) {
		return err
	}
	b, err = json.Marshal(source)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err = os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func Resolve() (Layout, error) {
	root := os.Getenv("CODEX_INSPECTOR_HOME")
	if root == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return Layout{}, err
		}
		root = filepath.Join(h, ".codex-inspector")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return Layout{}, err
	}
	if root == string(filepath.Separator) {
		return Layout{}, errors.New("CODEX_INSPECTOR_HOME cannot be the filesystem root")
	}
	codexRoot := os.Getenv("CODEX_HOME")
	if codexRoot == "" {
		h, homeErr := os.UserHomeDir()
		if homeErr == nil {
			codexRoot = filepath.Join(h, ".codex")
		}
	}
	if codexRoot != "" {
		if absolute, absErr := filepath.Abs(codexRoot); absErr == nil && samePath(root, absolute) {
			return Layout{}, errors.New("CODEX_INSPECTOR_HOME must be separate from CODEX_HOME")
		}
	}
	l := Layout{Root: root, Reviews: filepath.Join(root, "reviews"), Queue: filepath.Join(root, "queue"), Run: filepath.Join(root, "run"), Logs: filepath.Join(root, "logs"), Cache: filepath.Join(root, "cache")}
	return l, nil
}

func samePath(a, b string) bool {
	ar, ae := filepath.EvalSymlinks(a)
	if ae != nil {
		ar = filepath.Clean(a)
	}
	br, be := filepath.EvalSymlinks(b)
	if be != nil {
		br = filepath.Clean(b)
	}
	return ar == br
}

func Ensure(l Layout) error {
	old := syscallUmask(0o077)
	defer syscallUmask(old)
	for _, p := range []string{l.Root, l.Reviews, l.Queue, l.Run, l.Logs, l.Cache} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(p, 0o700); err != nil {
			return err
		}
	}
	return nil
}
