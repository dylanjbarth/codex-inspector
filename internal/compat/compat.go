package compat

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/phase0"
	proc "github.com/dylanjbarth/codex-inspector/internal/process"
	"github.com/dylanjbarth/codex-inspector/internal/version"
)

type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type Diagnostic struct {
	Code           string `json:"code"`
	Severity       string `json:"severity"`
	Count          int    `json:"count"`
	LastObservedAt string `json:"lastObservedAt"`
}

type Snapshot struct {
	Checks           []Check
	PluginVersion    *string
	PluginProtocol   *int
	CLICompatibility string
	HookDiagnostics  []Diagnostic
}

var versionPattern = regexp.MustCompile(`codex-cli\s+([0-9A-Za-z.+-]+)`)
var hashPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var pluginIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+@[A-Za-z0-9._-]+$`)

func CodexHost() Check {
	out, err := exec.Command("codex", "--version").CombinedOutput()
	if err != nil {
		return Check{"codex_host", "error", "Codex CLI is unavailable"}
	}
	m := versionPattern.FindStringSubmatch(string(out))
	if len(m) != 2 {
		return Check{"codex_host", "error", "Codex CLI version could not be identified"}
	}
	if m[1] != version.CodexHost {
		return Check{"codex_host", "incompatible", fmt.Sprintf("requires codex-cli %s; found %s", version.CodexHost, m[1])}
	}
	return Check{"codex_host", "ok", fmt.Sprintf("codex-cli %s", m[1])}
}

func CodexHome() string {
	if p := os.Getenv("CODEX_HOME"); p != "" {
		return p
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, ".codex")
}

func SourceFormat() Check {
	root := CodexHome()
	matches, _ := filepath.Glob(filepath.Join(root, "sessions", "*", "*", "*", "*.jsonl"))
	archived, _ := filepath.Glob(filepath.Join(root, "archived_sessions", "*.jsonl"))
	matches = append(matches, archived...)
	if len(matches) == 0 {
		return Check{"source_format", "ok", "no rollout sources yet; current format will be checked before indexing"}
	}
	sort.Slice(matches, func(i, j int) bool {
		left, leftErr := os.Stat(matches[i])
		right, rightErr := os.Stat(matches[j])
		if leftErr != nil || rightErr != nil || left.ModTime().Equal(right.ModTime()) {
			return matches[i] > matches[j]
		}
		return left.ModTime().After(right.ModTime())
	})
	for _, match := range matches {
		decision, err := phase0.ProbeFixture(match)
		if err == nil && decision.Supported {
			return Check{"source_format", "ok", version.SourceAdapter}
		}
	}
	return Check{"source_format", "incompatible", "no discovered rollout matches the frozen current-format fingerprint"}
}

type pluginInventory struct {
	Installed []struct {
		Name, PluginID, Version string
		Enabled                 bool
	} `json:"installed"`
}

func inspectPlugin() (Check, *string, *int, string, string) {
	out, err := exec.Command("codex", "plugin", "list", "--json").Output()
	if err != nil {
		return Check{"plugin", "error", "Codex plugin inventory is unavailable"}, nil, nil, "unknown", ""
	}
	var raw pluginInventory
	if json.Unmarshal(out, &raw) != nil {
		return Check{"plugin", "error", "Codex plugin inventory is unreadable"}, nil, nil, "unknown", ""
	}
	for _, p := range raw.Installed {
		if p.Name != "codex-inspector" || !p.Enabled {
			continue
		}
		if !pluginIDPattern.MatchString(p.PluginID) {
			return Check{"plugin", "error", "installed plugin identity is invalid"}, nil, nil, "unknown", ""
		}
		v := p.Version
		if v == "" {
			v = version.Plugin
		}
		pv := version.Protocol
		if !strings.HasPrefix(v, "0.1.") {
			return Check{"plugin", "incompatible", "installed plugin is outside 0.1.x"}, &v, &pv, "unsupported", p.PluginID
		}
		return Check{"plugin", "ok", "codex-inspector " + v + " enabled; CLI range " + version.PluginCLIRange}, &v, &pv, "supported", p.PluginID
	}
	return Check{"plugin", "error", "codex-inspector plugin is not enabled"}, nil, nil, "unknown", ""
}

type hookMetadata struct {
	EventName, HandlerType, Command, CurrentHash, TrustStatus, PluginID, Source string
	TimeoutSec                                                                  int  `json:"timeoutSec"`
	Enabled                                                                     bool `json:"enabled"`
}

func inspectHookTrust(pluginID string) Check {
	if pluginID == "" {
		return Check{"hook_trust", "error", "installed Inspector hooks are unavailable until the plugin is enabled"}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "codex", "app-server", "--stdio")
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return Check{"hook_trust", "error", "Codex hook inventory could not be opened"}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Check{"hook_trust", "error", "Codex hook inventory could not be opened"}
	}
	if err = cmd.Start(); err != nil {
		return Check{"hook_trust", "error", "Codex hook inventory is unavailable"}
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()
	enc := json.NewEncoder(stdin)
	_ = enc.Encode(map[string]any{"id": 1, "method": "initialize", "params": map[string]any{"clientInfo": map[string]string{"name": "codex-inspector", "version": version.CLI}}})
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	initialized := false
	for scanner.Scan() {
		var msg struct {
			ID     int `json:"id"`
			Result struct {
				Data []struct {
					Hooks []hookMetadata `json:"hooks"`
				} `json:"data"`
			} `json:"result"`
		}
		if json.Unmarshal(scanner.Bytes(), &msg) != nil {
			continue
		}
		if msg.ID == 1 && !initialized {
			initialized = true
			_ = enc.Encode(map[string]any{"method": "initialized", "params": map[string]any{}})
			cwd, _ := os.Getwd()
			_ = enc.Encode(map[string]any{"id": 2, "method": "hooks/list", "params": map[string]any{"cwds": []string{cwd}}})
			continue
		}
		if msg.ID != 2 {
			continue
		}
		var hooks []hookMetadata
		for _, group := range msg.Result.Data {
			hooks = append(hooks, group.Hooks...)
		}
		return evaluateHooks(hooks, pluginID)
	}
	if ctx.Err() != nil {
		return Check{"hook_trust", "error", "Codex hook inventory timed out"}
	}
	return Check{"hook_trust", "error", "Codex hook inventory is unreadable"}
}

var expectedEvents = []string{"postCompact", "preCompact", "sessionStart", "stop", "subagentStart", "subagentStop", "userPromptSubmit"}

func evaluateHooks(all []hookMetadata, pluginID string) Check {
	var hooks []hookMetadata
	for _, h := range all {
		if h.PluginID == pluginID {
			hooks = append(hooks, h)
		}
	}
	if len(hooks) != 7 {
		return Check{"hook_trust", "error", fmt.Sprintf("%d of 7 installed Inspector hooks discovered", len(hooks))}
	}
	events := make([]string, 0, 7)
	counts := map[string]int{}
	hashes := map[string]bool{}
	for _, h := range hooks {
		events = append(events, h.EventName)
		counts[h.TrustStatus]++
		hashes[h.CurrentHash] = true
		if !hashPattern.MatchString(h.CurrentHash) || h.HandlerType != "command" || h.TimeoutSec != 2 || !strings.HasSuffix(h.Command, "/hooks/inspector-hook.sh") || h.Source != "plugin" || !h.Enabled {
			return Check{"hook_trust", "error", "installed Inspector hook definitions differ from the reviewed plugin"}
		}
	}
	sort.Strings(events)
	if strings.Join(events, ",") != strings.Join(expectedEvents, ",") {
		return Check{"hook_trust", "error", "installed Inspector hook event set is incomplete or changed"}
	}
	if len(hashes) != 7 {
		return Check{"hook_trust", "error", "installed Inspector hooks do not have seven distinct current hashes"}
	}
	if counts["modified"] > 0 {
		return Check{"hook_trust", "incompatible", fmt.Sprintf("%d of 7 Inspector hooks changed since trust review", counts["modified"])}
	}
	if counts["untrusted"] > 0 {
		return Check{"hook_trust", "error", fmt.Sprintf("%d of 7 Inspector hooks require trust review", counts["untrusted"])}
	}
	if counts["trusted"] != 7 {
		return Check{"hook_trust", "error", "Inspector hook trust state is stale or unsupported"}
	}
	return Check{"hook_trust", "ok", "all seven current canonical Inspector hook hashes are trusted"}
}

func pluginDiagnostics(pluginID string) []Diagnostic {
	roots := []string{os.Getenv("PLUGIN_DATA"), os.Getenv("CODEX_INSPECTOR_PLUGIN_DATA")}
	if roots[0] == "" && roots[1] == "" && pluginID != "" {
		roots = append(roots, filepath.Join(CodexHome(), "plugins", "data", strings.ReplaceAll(pluginID, "@", "-")))
	}
	seen := map[string]bool{}
	var out []Diagnostic
	for _, root := range roots {
		if root == "" {
			continue
		}
		_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if d.Name() != "hook-diagnostic-missing_cli.json" && d.Name() != "hook-diagnostic-protocol_mismatch.json" {
				return nil
			}
			info, e := d.Info()
			if e != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
				return nil
			}
			b, e := os.ReadFile(path)
			if e != nil || len(b) > 4096 {
				return nil
			}
			var v struct {
				SchemaVersion string `json:"schemaVersion"`
				Diagnostic
			}
			decoder := json.NewDecoder(bytes.NewReader(b))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&v) != nil {
				return nil
			}
			var extra any
			if decoder.Decode(&extra) != io.EOF || v.SchemaVersion != "inspector.plugin-diagnostic/v1" || seen[v.Code] || (v.Code != "missing_cli" && v.Code != "protocol_mismatch") || v.Count < 1 {
				return nil
			}
			if v.Severity != "warning" && v.Severity != "error" {
				return nil
			}
			if _, e = time.Parse(time.RFC3339Nano, v.LastObservedAt); e != nil {
				return nil
			}
			seen[v.Code] = true
			out = append(out, v.Diagnostic)
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

func Inspect(l home.Layout, includeServer bool) Snapshot {
	return InspectWithProgress(l, includeServer, nil)
}

func InspectWithProgress(l home.Layout, includeServer bool, progress func(string)) Snapshot {
	report := func(stage string) {
		if progress != nil {
			progress(stage)
		}
	}
	checks := []Check{{"inspector_cli", "ok", "codex-inspector " + version.CLI + "; protocol 1"}}
	report("codex_host")
	checks = append(checks, CodexHost())
	report("source_format")
	checks = append(checks, SourceFormat())
	report("data_home")
	if err := home.Ensure(l); err != nil {
		checks = append(checks, Check{"data_home", "error", "Inspector data home could not be initialized"})
	} else {
		checks = append(checks, Check{"data_home", "ok", "user-only Inspector directories are ready"})
	}
	report("plugin")
	pcheck, pv, pp, compatibility, pluginID := inspectPlugin()
	checks = append(checks, pcheck)
	report("hook_trust")
	checks = append(checks, inspectHookTrust(pluginID))
	report("hook_diagnostics")
	diagnostics := pluginDiagnostics(pluginID)
	for _, d := range diagnostics {
		checks = append(checks, Check{"hook_" + d.Code, "error", fmt.Sprintf("plugin hook reported %s at %s", d.Code, d.LastObservedAt)})
	}
	if includeServer {
		report("server")
		m, e := proc.Read(l.Run)
		if e == nil && proc.Healthy(m) {
			checks = append(checks, Check{"server", "ok", "local loopback server is healthy"})
		} else if os.IsNotExist(e) {
			checks = append(checks, Check{"server", "ok", "server is stopped and ready to start on an ephemeral loopback port"})
		} else {
			checks = append(checks, Check{"server", "error", "Inspector server is not running; run codex-inspector open"})
		}
	}
	report("complete")
	for _, c := range checks {
		if c.Name != "inspector_cli" && c.Status != "ok" {
			if c.Status == "incompatible" {
				compatibility = "unsupported"
			} else if compatibility == "supported" {
				compatibility = "unknown"
			}
		}
	}
	return Snapshot{checks, pv, pp, compatibility, diagnostics}
}
