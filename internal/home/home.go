package home

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
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
// Codex home. Legacy catalogs predate the binding sidecar, so their identity is
// established from the canonical source paths already recorded in the active
// database before a sidecar is written. The database itself is never modified.
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
	legacyDB, populated, err := legacyDatasetPath(l)
	if err != nil {
		return err
	}
	if populated {
		if err = verifyLegacyDatasetHome(legacyDB, source.Path); err != nil {
			return err
		}
	}
	return writeDatasetBinding(path, source)
}

func writeDatasetBinding(path string, source CodexHome) error {
	b, err := json.Marshal(source)
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

func legacyDatasetPath(l Layout) (string, bool, error) {
	catalog := filepath.Join(l.Root, "active-index")
	b, err := os.ReadFile(catalog)
	if err == nil {
		name := strings.TrimSpace(string(b))
		if name == "" || filepath.Base(name) != name || name == "." {
			return "", true, errors.New("legacy dataset identity cannot be established: active-index is invalid; preserve the existing Inspector home and rebuild into a separate CODEX_INSPECTOR_HOME")
		}
		return filepath.Join(l.Root, name), true, nil
	}
	if !os.IsNotExist(err) {
		return "", false, err
	}
	database := filepath.Join(l.Root, "inspector.db")
	if _, err = os.Stat(database); err == nil {
		return database, true, nil
	}
	if !os.IsNotExist(err) {
		return "", false, err
	}
	return "", false, nil
}

func verifyLegacyDatasetHome(database, requestedHome string) error {
	dsn := (&url.URL{Scheme: "file", Path: database, RawQuery: "mode=ro"}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return legacyIdentityError(requestedHome, err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT DISTINCT canonical_path FROM source_artifact_versions`)
	if err != nil {
		return legacyIdentityError(requestedHome, err)
	}
	defer rows.Close()

	seen := false
	for rows.Next() {
		var sourcePath string
		if err = rows.Scan(&sourcePath); err != nil {
			return legacyIdentityError(requestedHome, err)
		}
		seen = true
		if !pathWithin(requestedHome, sourcePath) {
			return fmt.Errorf("legacy dataset is not proven to belong to requested Codex home %q (recorded source %q); preserve it and use a separate CODEX_INSPECTOR_HOME or rebuild", requestedHome, sourcePath)
		}
	}
	if err = rows.Err(); err != nil {
		return legacyIdentityError(requestedHome, err)
	}
	if !seen {
		return legacyIdentityError(requestedHome, errors.New("catalog contains no recorded source paths"))
	}
	return nil
}

func legacyIdentityError(requestedHome string, cause error) error {
	return fmt.Errorf("legacy dataset identity cannot be established for requested Codex home %q: %v; preserve the existing Inspector home and rebuild into a separate CODEX_INSPECTOR_HOME", requestedHome, cause)
}

func pathWithin(root, candidate string) bool {
	root = filepath.Clean(root)
	candidate = filepath.Clean(candidate)
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
