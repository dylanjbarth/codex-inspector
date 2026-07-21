package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: hookbench <plugin-shim> <codex-inspector-binary> <hook-fixture>")
		os.Exit(2)
	}
	shim, err := filepath.Abs(os.Args[1])
	if err != nil {
		fail(err)
	}
	binary, err := filepath.Abs(os.Args[2])
	if err != nil {
		fail(err)
	}
	payload, err := os.ReadFile(os.Args[3])
	if err != nil {
		fail(err)
	}
	root, err := os.MkdirTemp("", "codex-inspector-hookbench.")
	if err != nil {
		fail(err)
	}
	defer os.RemoveAll(root)
	binDir := filepath.Join(root, "bin")
	pluginData := filepath.Join(root, "plugin-data")
	inspectorHome := filepath.Join(root, "inspector")
	for _, d := range []string{binDir, pluginData, inspectorHome} {
		if err = os.MkdirAll(d, 0o700); err != nil {
			fail(err)
		}
	}
	installed := filepath.Join(binDir, "codex-inspector")
	if err = os.Symlink(binary, installed); err != nil {
		fail(err)
	}
	pluginRoot := filepath.Dir(filepath.Dir(shim))
	baseEnv := []string{"CODEX_INSPECTOR_HOME=" + inspectorHome, "PLUGIN_ROOT=" + pluginRoot, "PLUGIN_DATA=" + pluginData}
	// Exercise the real shim diagnostic branch before timing. A regression that
	// skips or leaks this path fails the benchmark rather than hiding in unit tests.
	missing := exec.Command(shim)
	missing.Env = append(os.Environ(), append(baseEnv, "PATH=/usr/bin:/bin")...)
	missing.Stdin = bytes.NewReader(payload)
	if err = missing.Run(); err != nil {
		fail(fmt.Errorf("missing-CLI shim: %w", err))
	}
	diagnostic := filepath.Join(pluginData, "hook-diagnostic-missing_cli.json")
	b, err := os.ReadFile(diagnostic)
	if err != nil {
		fail(fmt.Errorf("missing diagnostic: %w", err))
	}
	if bytes.Contains(b, payload) || bytes.Contains(b, []byte("fake")) || len(b) > 4096 {
		fail(fmt.Errorf("diagnostic leaked payload or exceeded bound"))
	}
	samples := make([]time.Duration, 100)
	for i := range samples {
		cmd := exec.Command(shim)
		cmd.Env = append(os.Environ(), append(baseEnv, "PATH="+binDir+":/usr/bin:/bin")...)
		cmd.Stdin = bytes.NewReader(payload)
		start := time.Now()
		err = cmd.Run()
		samples[i] = time.Since(start)
		if err != nil {
			fail(err)
		}
		if samples[i] >= 2*time.Second {
			fail(fmt.Errorf("real shim hard timeout exceeded"))
		}
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	p95 := samples[94]
	max := samples[99]
	fmt.Printf("hook_path=registered_plugin_shim hook_samples=100 p95_ms=%.3f max_ms=%.3f\n", float64(p95.Microseconds())/1000, float64(max.Microseconds())/1000)
	if p95 > 50*time.Millisecond {
		fail(fmt.Errorf("real shim p95 exceeds 50ms"))
	}
	entries, err := os.ReadDir(filepath.Join(inspectorHome, "queue"))
	if err != nil || len(entries) != 100 {
		fail(fmt.Errorf("marker count is not 100"))
	}
	if _, err = os.Stat(diagnostic); !os.IsNotExist(err) {
		fail(fmt.Errorf("resolved missing-CLI diagnostic was not cleared"))
	}
	pluginEntries, _ := os.ReadDir(pluginData)
	for _, entry := range pluginEntries {
		if strings.HasPrefix(entry.Name(), ".hook-diagnostic-") {
			fail(fmt.Errorf("atomic diagnostic temp remained"))
		}
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "hookbench:", err); os.Exit(1) }
