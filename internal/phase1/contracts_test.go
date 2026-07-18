package phase1

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func root(parts ...string) string { return filepath.Join(append([]string{"..", ".."}, parts...)...) }
func TestPluginContractAndSevenNonBlockingHooks(t *testing.T) {
	manifest, err := os.ReadFile(root("plugin", "codex-inspector", ".codex-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var p struct{ Name, Version, Skills, Hooks string }
	if json.Unmarshal(manifest, &p) != nil || p.Name != "codex-inspector" || p.Version != "0.1.0" || p.Skills != "./skills/" {
		t.Fatalf("invalid plugin manifest: %s", manifest)
	}
	hooks, err := os.ReadFile(root("plugin", "codex-inspector", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var h struct {
		Hooks map[string]json.RawMessage `json:"hooks"`
	}
	if json.Unmarshal(hooks, &h) != nil || len(h.Hooks) != 7 {
		t.Fatalf("hook count=%d", len(h.Hooks))
	}
	for _, name := range []string{"SessionStart", "UserPromptSubmit", "PreCompact", "PostCompact", "SubagentStart", "SubagentStop", "Stop"} {
		if _, ok := h.Hooks[name]; !ok {
			t.Fatalf("missing %s", name)
		}
	}
	shim, err := os.ReadFile(root("plugin", "codex-inspector", "hooks", "inspector-hook.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(shim), "codex-inspector version") || !strings.Contains(string(shim), "_hook --protocol 1") || !strings.Contains(string(shim), "exit 0") {
		t.Fatal("shim does not freeze protocol/non-blocking behavior")
	}
}

func TestPluginShimDiagnosticsAreAtomicPrivateRateLimitedAndPayloadFree(t *testing.T) {
	shim, _ := filepath.Abs(root("plugin", "codex-inspector", "hooks", "inspector-hook.sh"))
	data := t.TempDir()
	run := func(path string) {
		t.Helper()
		cmd := exec.Command("/bin/sh", shim)
		cmd.Env = []string{"PATH=" + path, "PLUGIN_DATA=" + data}
		cmd.Stdin = strings.NewReader(`{"prompt":"PAYLOAD-CANARY"}`)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("shim must be nonblocking: %v %s", err, out)
		}
	}
	run("/usr/bin:/bin")
	missing := filepath.Join(data, "hook-diagnostic-missing_cli.json")
	b, err := os.ReadFile(missing)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "PAYLOAD-CANARY") {
		t.Fatal("payload leaked")
	}
	info, _ := os.Stat(missing)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	first := info.ModTime()
	run("/usr/bin:/bin")
	info, _ = os.Stat(missing)
	if !info.ModTime().Equal(first) {
		t.Fatal("diagnostic was not rate limited")
	}
	bin := t.TempDir()
	fake := filepath.Join(bin, "codex-inspector")
	if err = os.WriteFile(fake, []byte("#!/bin/sh\necho 'codex-inspector 0.1.0 (protocol 2, index schema 1)'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	run(bin + ":/usr/bin:/bin")
	protocol := filepath.Join(data, "hook-diagnostic-protocol_mismatch.json")
	b, err = os.ReadFile(protocol)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "PAYLOAD-CANARY") {
		t.Fatal("payload leaked")
	}
	info, _ = os.Stat(protocol)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	entries, _ := os.ReadDir(data)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Fatalf("atomic temp remained: %s", e.Name())
		}
	}
}
func TestSkillsAndMarketplaceMetadata(t *testing.T) {
	for _, skill := range []string{"setup", "open-dashboard", "inspect-session", "review-session"} {
		b, err := os.ReadFile(root("plugin", "codex-inspector", "skills", skill, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "name: "+skill) {
			t.Fatalf("bad skill %s", skill)
		}
	}
	for _, path := range [][]string{{".agents", "plugins", "marketplace.json"}, {"marketplace", "release.json"}} {
		b, err := os.ReadFile(root(path...))
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(b) || !strings.Contains(string(b), "codex-inspector") {
			t.Fatalf("bad marketplace %s", path)
		}
	}
}
