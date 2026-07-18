package home

import (
	"errors"
	"os"
	"path/filepath"
)

type Layout struct {
	Root, Reviews, Queue, Run, Logs, Cache string
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
