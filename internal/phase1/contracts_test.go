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
	var p struct {
		Name, Version, Skills, Hooks string
		Interface                    struct{ ComposerIcon, Logo string }
	}
	if json.Unmarshal(manifest, &p) != nil || p.Name != "codex-inspector" || p.Version != "0.1.0" || p.Skills != "./skills/" {
		t.Fatalf("invalid plugin manifest: %s", manifest)
	}
	if p.Interface.ComposerIcon != "./assets/icon.png" || p.Interface.Logo != "./assets/logo.png" {
		t.Fatalf("plugin brand assets are not wired: %s", manifest)
	}
	for _, asset := range []string{"icon.png", "logo.png"} {
		if _, err := os.Stat(root("plugin", "codex-inspector", "assets", asset)); err != nil {
			t.Fatalf("missing plugin brand asset %s: %v", asset, err)
		}
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
	run := func(data, path, hookLog string) {
		t.Helper()
		cmd := exec.Command("/bin/sh", shim)
		cmd.Env = []string{"PATH=" + path, "PLUGIN_DATA=" + data}
		if hookLog != "" {
			cmd.Env = append(cmd.Env, "FAKE_HOOK_LOG="+hookLog)
		}
		cmd.Stdin = strings.NewReader(`{"prompt":"PAYLOAD-CANARY"}`)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("shim must be nonblocking: %v %s", err, out)
		}
	}
	run(data, "/usr/bin:/bin", "")
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
	run(data, "/usr/bin:/bin", "")
	info, _ = os.Stat(missing)
	if !info.ModTime().Equal(first) {
		t.Fatal("diagnostic was not rate limited")
	}
	for _, tc := range []struct {
		name, version string
	}{
		{"protocol", "codex-inspector 0.1.0 (protocol 2, index schema 2)"},
		{"index_schema", "codex-inspector 0.1.0 (protocol 1, index schema 1)"},
	} {
		t.Run(tc.name+" mismatch", func(t *testing.T) {
			mismatchData := t.TempDir()
			bin := t.TempDir()
			hookLog := filepath.Join(t.TempDir(), "hook.log")
			fake := filepath.Join(bin, "codex-inspector")
			script := "#!/bin/sh\nif [ \"${1:-}\" = version ]; then echo '" + tc.version + "'; exit 0; fi\nprintf '%s\\n' \"$*\" >> \"${FAKE_HOOK_LOG}\"\n"
			if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			run(mismatchData, bin+":/usr/bin:/bin", hookLog)
			protocol := filepath.Join(mismatchData, "hook-diagnostic-protocol_mismatch.json")
			b, err := os.ReadFile(protocol)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(b), "PAYLOAD-CANARY") {
				t.Fatal("payload leaked")
			}
			info, _ := os.Stat(protocol)
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("mode=%o", info.Mode().Perm())
			}
			if _, err := os.Stat(hookLog); !os.IsNotExist(err) {
				t.Fatalf("incompatible CLI invoked _hook: %v", err)
			}
		})
	}

	compatibleData := t.TempDir()
	for _, name := range []string{"hook-diagnostic-missing_cli.json", "hook-diagnostic-protocol_mismatch.json"} {
		if err := os.WriteFile(filepath.Join(compatibleData, name), []byte("stale"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	hookLog := filepath.Join(t.TempDir(), "hook.log")
	fake := filepath.Join(bin, "codex-inspector")
	compatible := "#!/bin/sh\nif [ \"${1:-}\" = version ]; then echo 'codex-inspector 0.1.0 (protocol 1, index schema 2)'; exit 0; fi\nprintf '%s\\n' \"$*\" >> \"${FAKE_HOOK_LOG}\"\n"
	if err := os.WriteFile(fake, []byte(compatible), 0o700); err != nil {
		t.Fatal(err)
	}
	run(compatibleData, bin+":/usr/bin:/bin", hookLog)
	invocation, err := os.ReadFile(hookLog)
	if err != nil {
		t.Fatal(err)
	}
	if string(invocation) != "_hook --protocol 1\n" || strings.Contains(string(invocation), "PAYLOAD-CANARY") {
		t.Fatalf("unexpected compatible hook invocation: %q", invocation)
	}
	for _, name := range []string{"hook-diagnostic-missing_cli.json", "hook-diagnostic-protocol_mismatch.json"} {
		if _, err := os.Stat(filepath.Join(compatibleData, name)); !os.IsNotExist(err) {
			t.Fatalf("compatible CLI did not clear %s: %v", name, err)
		}
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
