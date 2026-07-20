package compat

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func fakeCodex(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "codex")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

func TestHostAndPluginCompatibilityFailuresAreAuthentic(t *testing.T) {
	fakeCodex(t, `if [ "$1" = "--version" ]; then echo 'codex-cli 0.141.9'; else echo '{"installed":[]}'; fi`)
	if got := CodexHost(); got.Status != "incompatible" {
		t.Fatalf("host: %+v", got)
	}
	check, pv, pp, state, id := inspectPlugin()
	if check.Status != "error" || pv != nil || pp != nil || state != "unknown" || id != "" {
		t.Fatalf("empty plugin inventory: %+v %v %v %s %s", check, pv, pp, state, id)
	}
}

func TestCodexHostRejectsVersionsBelowTheRecentFloor(t *testing.T) {
	for _, candidate := range []string{"0.142.4", "0.142.5-alpha.1", "not-semver"} {
		t.Run(candidate, func(t *testing.T) {
			fakeCodex(t, `echo 'codex-cli `+candidate+`'`)
			if got := CodexHost(); got.Status != "incompatible" {
				t.Fatalf("host: %+v", got)
			}
		})
	}
}

func TestCodexHostAcceptsRecentStableAndPrereleaseVersions(t *testing.T) {
	for _, candidate := range []string{"0.142.5", "0.143.0", "0.144.1", "0.145.0-alpha.18", "0.145.0-alpha.19", "1.0.0"} {
		t.Run(candidate, func(t *testing.T) {
			fakeCodex(t, `echo 'codex-cli `+candidate+`'`)
			if got := CodexHost(); got.Status != "ok" || got.Detail != "codex-cli "+candidate {
				t.Fatalf("host: %+v", got)
			}
		})
	}
}

func TestPluginVersionAndIdentityMismatches(t *testing.T) {
	fakeCodex(t, `echo '{"installed":[{"name":"codex-inspector","pluginId":"codex-inspector@market","version":"0.2.0","enabled":true}]}'`)
	check, _, _, state, _ := inspectPlugin()
	if check.Status != "incompatible" || state != "unsupported" {
		t.Fatalf("version: %+v %s", check, state)
	}
	fakeCodex(t, `echo '{"installed":[{"name":"codex-inspector","pluginId":"../escape@market","version":"0.1.0","enabled":true}]}'`)
	check, _, _, state, _ = inspectPlugin()
	if check.Status != "error" || state != "unknown" {
		t.Fatalf("identity: %+v %s", check, state)
	}
}

func TestSourceFormatUsesFrozenDiscriminator(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sessions", "2026", "07", "18")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", root)
	copyFixture := func(name string) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "synthetic", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, "rollout.jsonl"), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	copyFixture("unsupported.jsonl")
	if got := SourceFormat(); got.Status != "incompatible" {
		t.Fatalf("unsupported source: %+v", got)
	}
	copyFixture("root.jsonl")
	if got := SourceFormat(); got.Status != "ok" || got.Detail != "rollout-jsonl/codex-recent-structural/v4" {
		t.Fatalf("supported source: %+v", got)
	}
}

func TestSourceFormatDoesNotDisableHealthyRuntime(t *testing.T) {
	checks := []Check{
		{Name: "codex_host", Status: "ok"},
		{Name: "source_format", Status: "incompatible"},
		{Name: "plugin", Status: "ok"},
	}
	if got := runtimeCompatibility(checks, "supported"); got != "supported" {
		t.Fatalf("source coverage disabled runtime: %s", got)
	}
	checks = append(checks, Check{Name: "hook_trust", Status: "incompatible"})
	if got := runtimeCompatibility(checks, "supported"); got != "unsupported" {
		t.Fatalf("runtime incompatibility was ignored: %s", got)
	}
}

func canonicalHooks(status string) []hookMetadata {
	events := append([]string(nil), expectedEvents...)
	out := make([]hookMetadata, 0, 7)
	for i, event := range events {
		out = append(out, hookMetadata{EventName: event, HandlerType: "command", Command: "/tmp/plugin/hooks/inspector-hook.sh", CurrentHash: "sha256:" + fmt.Sprintf("%063x%x", 0, i), TrustStatus: status, PluginID: "codex-inspector@test", Source: "plugin", Enabled: true, TimeoutSec: 2 + i - i})
	}
	return out
}

func TestEvaluateHooksRequiresExactCurrentCanonicalTrust(t *testing.T) {
	if got := evaluateHooks(canonicalHooks("trusted"), "codex-inspector@test"); got.Status != "ok" {
		t.Fatalf("trusted: %+v", got)
	}
	untrusted := canonicalHooks("trusted")
	untrusted[0].TrustStatus = "untrusted"
	if got := evaluateHooks(untrusted, "codex-inspector@test"); got.Status != "error" || got.Detail != "1 of 7 Inspector hooks require trust review" {
		t.Fatalf("untrusted: %+v", got)
	}
	modified := canonicalHooks("trusted")
	modified[0].TrustStatus = "modified"
	if got := evaluateHooks(modified, "codex-inspector@test"); got.Status != "incompatible" {
		t.Fatalf("modified: %+v", got)
	}
	changed := canonicalHooks("trusted")
	changed[0].CurrentHash = "sha256:bogus"
	if got := evaluateHooks(changed, "codex-inspector@test"); got.Status != "error" {
		t.Fatalf("bogus hash: %+v", got)
	}
	missing := canonicalHooks("trusted")[:6]
	if got := evaluateHooks(missing, "codex-inspector@test"); got.Status != "error" {
		t.Fatalf("missing: %+v", got)
	}
}

func TestPluginDiagnosticsAreValidatedAndPayloadFree(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PLUGIN_DATA", root)
	t.Setenv("CODEX_INSPECTOR_PLUGIN_DATA", "")
	v := map[string]any{"schemaVersion": "inspector.plugin-diagnostic/v1", "code": "missing_cli", "severity": "warning", "count": 1, "lastObservedAt": "2026-07-18T00:00:00Z"}
	b, _ := json.Marshal(v)
	if err := os.WriteFile(filepath.Join(root, "hook-diagnostic-missing_cli.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := pluginDiagnostics("codex-inspector@test"); len(got) != 1 || got[0].Code != "missing_cli" {
		t.Fatalf("diagnostics: %+v", got)
	}
	v["code"] = "payload"
	v["secret"] = "PAYLOAD-CANARY"
	b, _ = json.Marshal(v)
	_ = os.WriteFile(filepath.Join(root, "hook-diagnostic-protocol_mismatch.json"), b, 0o600)
	if got := pluginDiagnostics("codex-inspector@test"); len(got) != 1 {
		t.Fatalf("invalid diagnostic accepted: %+v", got)
	}
	v = map[string]any{"schemaVersion": "inspector.plugin-diagnostic/v1", "code": "protocol_mismatch", "severity": "error", "count": 1, "lastObservedAt": "2026-07-18T00:00:00Z", "unknown": "PAYLOAD-CANARY"}
	b, _ = json.Marshal(v)
	_ = os.WriteFile(filepath.Join(root, "hook-diagnostic-protocol_mismatch.json"), b, 0o600)
	if got := pluginDiagnostics("codex-inspector@test"); len(got) != 1 {
		t.Fatalf("unknown field accepted: %+v", got)
	}
	delete(v, "unknown")
	b, _ = json.Marshal(v)
	b = append(b, []byte(` {}`)...)
	_ = os.WriteFile(filepath.Join(root, "hook-diagnostic-protocol_mismatch.json"), b, 0o600)
	if got := pluginDiagnostics("codex-inspector@test"); len(got) != 1 {
		t.Fatalf("trailing object accepted: %+v", got)
	}
	b, _ = json.Marshal(v)
	protocolPath := filepath.Join(root, "hook-diagnostic-protocol_mismatch.json")
	_ = os.WriteFile(protocolPath, b, 0o600)
	_ = os.Chmod(protocolPath, 0o644)
	if got := pluginDiagnostics("codex-inspector@test"); len(got) != 1 {
		t.Fatalf("permissive mode accepted: %+v", got)
	}
}
