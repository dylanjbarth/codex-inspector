package sources

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Candidate struct {
	Path    string
	Kind    string
	Size    int64
	MTimeNS int64
}

func CodexHome() (string, error) {
	root := os.Getenv("CODEX_HOME")
	if root == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		root = filepath.Join(h, ".codex")
	}
	return filepath.Abs(root)
}

func Discover(codexHome, inspectorHome string) ([]Candidate, error) {
	codexHome, err := filepath.Abs(codexHome)
	if err != nil {
		return nil, err
	}
	if sameCanonical(codexHome, inspectorHome) {
		return nil, errors.New("CODEX_HOME and CODEX_INSPECTOR_HOME overlap")
	}
	var out []Candidate
	for _, root := range []struct{ rel, kind string }{{"sessions", "active_rollout"}, {"archived_sessions", "archived_rollout"}} {
		base := filepath.Join(codexHome, root.rel)
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, os.ErrNotExist) {
					return nil
				}
				return walkErr
			}
			if entry.IsDir() {
				if sameCanonical(path, inspectorHome) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(entry.Name(), ".jsonl") || sameCanonical(path, inspectorHome) {
				return nil
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			canonical, err := filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			if within(canonical, inspectorHome) {
				return nil
			}
			out = append(out, Candidate{canonical, root.kind, info.Size(), info.ModTime().UnixNano()})
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	index := filepath.Join(codexHome, "session_index.jsonl")
	if info, err := os.Stat(index); err == nil && info.Mode().IsRegular() {
		canonical, err := filepath.EvalSymlinks(index)
		if err != nil {
			return nil, err
		}
		if !within(canonical, inspectorHome) {
			out = append(out, Candidate{canonical, "session_index", info.Size(), info.ModTime().UnixNano()})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MTimeNS == out[j].MTimeNS {
			return out[i].Path < out[j].Path
		}
		return out[i].MTimeNS > out[j].MTimeNS
	})
	return out, nil
}

func within(path, root string) bool {
	if root == "" {
		return false
	}
	p, pe := filepath.EvalSymlinks(path)
	if pe != nil {
		p = filepath.Clean(path)
	}
	r, re := filepath.EvalSymlinks(root)
	if re != nil {
		r = filepath.Clean(root)
	}
	rel, err := filepath.Rel(r, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
func sameCanonical(a, b string) bool { return b != "" && within(a, b) && within(b, a) }

func NewestBoundary(c []Candidate) *time.Time {
	if len(c) == 0 {
		return nil
	}
	t := time.Unix(0, c[len(c)-1].MTimeNS).UTC()
	return &t
}
