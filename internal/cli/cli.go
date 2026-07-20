package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/compat"
	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/hook"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	proc "github.com/dylanjbarth/codex-inspector/internal/process"
	"github.com/dylanjbarth/codex-inspector/internal/server"
	"github.com/dylanjbarth/codex-inspector/internal/version"
)

type IO struct {
	In       io.Reader
	Out, Err io.Writer
}

func Main(args []string, streams IO) int {
	if len(args) == 0 {
		usage(streams.Err)
		return 2
	}
	var err error
	switch args[0] {
	case "version":
		err = versionCmd(args[1:], streams)
	case "doctor":
		err = doctorCmd(args[1:], streams)
	case "status":
		err = statusCmd(args[1:], streams)
	case "sync":
		err = syncCmd(args[1:], streams)
	case "open":
		err = openCmd(args[1:], streams)
	case "_hook":
		err = hookCmd(args[1:], streams)
	case "_serve":
		err = serveCmd(args[1:], streams)
	case "_worker":
		err = workerCmd(args[1:], streams)
	default:
		usage(streams.Err)
		return 2
	}
	if err != nil {
		fmt.Fprintln(streams.Err, "codex-inspector:", err)
		return 1
	}
	return 0
}
func usage(w io.Writer) {
	fmt.Fprintln(w, "usage: codex-inspector <version|doctor|status|sync|open> [options]")
}
func versionCmd(args []string, s IO) error {
	if len(args) > 0 {
		return errors.New("version takes no arguments")
	}
	fmt.Fprintf(s.Out, "codex-inspector %s (protocol %d, index schema %d)\n", version.CLI, version.Protocol, version.IndexSchema)
	return nil
}

type doctorReport struct {
	Healthy bool           `json:"healthy"`
	Checks  []compat.Check `json:"checks"`
}

func doctorCmd(args []string, s IO) error {
	f := flag.NewFlagSet("doctor", flag.ContinueOnError)
	f.SetOutput(s.Err)
	asJSON := f.Bool("json", false, "emit JSON")
	if err := f.Parse(args); err != nil {
		return err
	}
	if !*asJSON {
		fmt.Fprintln(s.Err, "Doctor: checking Codex, plugin hooks, data home, and local server...")
	}
	l, err := home.Resolve()
	if err != nil {
		if !*asJSON {
			fmt.Fprintln(s.Err, "Doctor: checks could not be completed.")
		}
		return errors.New("Inspector home could not be resolved; check CODEX_INSPECTOR_HOME and retry")
	}
	snapshot := compat.Inspect(l, true)
	checks := snapshot.Checks
	healthy := true
	for _, c := range checks {
		if c.Status != "ok" {
			healthy = false
		}
	}
	r := doctorReport{healthy, checks}
	if *asJSON {
		if err := json.NewEncoder(s.Out).Encode(r); err != nil {
			return err
		}
		if !healthy {
			return errors.New("one or more checks require attention")
		}
		return nil
	}
	for _, c := range checks {
		fmt.Fprintf(s.Out, "%-15s %-12s %s\n", c.Name, c.Status, c.Detail)
	}
	if !healthy {
		fmt.Fprintln(s.Err, "Doctor: checks complete; one or more require attention.")
		return errors.New("one or more checks require attention")
	}
	fmt.Fprintln(s.Err, "Doctor: all checks complete.")
	return nil
}

func statusCmd(args []string, s IO) error {
	f := flag.NewFlagSet("status", flag.ContinueOnError)
	f.SetOutput(s.Err)
	asJSON := f.Bool("json", false, "emit JSON")
	if err := f.Parse(args); err != nil {
		return err
	}
	l, e := home.Resolve()
	if e != nil {
		return errors.New("Inspector home could not be resolved; check CODEX_INSPECTOR_HOME and retry")
	}
	m, e := proc.Read(l.Run)
	if e != nil || !proc.Healthy(m) {
		if *asJSON {
			return json.NewEncoder(s.Out).Encode(map[string]any{"running": false, "state": "stopped"})
		}
		fmt.Fprintln(s.Out, "Inspector is stopped")
		return nil
	}
	body, e := request(m, http.MethodGet, "/v1/status", nil)
	if e != nil {
		return errors.New("Inspector server did not answer; run codex-inspector open")
	}
	if *asJSON {
		_, e = s.Out.Write(body)
		return e
	}
	var v struct {
		Process struct {
			State      string `json:"state"`
			CLIVersion string `json:"cliVersion"`
		} `json:"process"`
		Index struct {
			State  string `json:"state"`
			Queued int    `json:"queuedSessionChanges"`
		} `json:"index"`
		Hook struct {
			State string `json:"state"`
		} `json:"hook"`
	}
	if json.Unmarshal(body, &v) != nil {
		return errors.New("Inspector server returned an invalid status; run codex-inspector open")
	}
	fmt.Fprintf(s.Out, "process=%s cli=%s index=%s hook=%s queued_markers=%d\n", v.Process.State, v.Process.CLIVersion, v.Index.State, v.Hook.State, v.Index.Queued)
	return nil
}
func syncCmd(args []string, s IO) error {
	f := flag.NewFlagSet("sync", flag.ContinueOnError)
	f.SetOutput(s.Err)
	bg := f.Bool("background", false, "return after queueing")
	wait := f.Bool("wait", false, "wait for finite sync")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *bg && *wait {
		return errors.New("choose one of --background or --wait")
	}
	mode := "wait"
	if *bg {
		mode = "background"
	}
	if mode == "wait" {
		fmt.Fprintln(s.Err, "Sync: discovering supported Codex sources...")
	} else {
		fmt.Fprintln(s.Err, "Sync: asking the local server to queue background indexing...")
	}
	l, e := home.Resolve()
	if e != nil {
		return errors.New("Inspector home could not be resolved; check CODEX_INSPECTOR_HOME and retry")
	}
	if mode == "wait" {
		reporter := newSyncProgressReporter(s.Err)
		progress, runErr := indexer.Run(context.Background(), indexer.Config{Layout: l, OnCommit: reporter.Report})
		if runErr != nil {
			reporter.Failed(progress)
			return errors.New("sync did not complete; run codex-inspector doctor for compatibility and setup checks")
		}
		reporter.Complete(progress)
		return json.NewEncoder(s.Out).Encode(map[string]any{"state": "complete", "inventoried": progress.Inventoried, "processed": progress.Processed, "skipped": progress.Skipped, "failed": progress.Failed, "requiresRebuild": progress.RequiresRebuild, "queueConsumed": progress.QueueConsumed, "reverseScanBoundary": progress.Boundary})
	}
	m, e := proc.Read(l.Run)
	if e != nil || !proc.Healthy(m) {
		fmt.Fprintln(s.Err, "Sync: background indexing was not queued.")
		return errors.New("Inspector server is not running; run codex-inspector open")
	}
	b, _ := json.Marshal(map[string]string{"mode": mode})
	out, e := request(m, http.MethodPost, "/v1/sync", b)
	if e != nil {
		fmt.Fprintln(s.Err, "Sync: background indexing was not queued.")
		return errors.New("Inspector server did not queue background indexing; run codex-inspector status")
	}
	fmt.Fprintln(s.Err, "Sync: background indexing queued; run codex-inspector status to check it.")
	fmt.Fprintf(s.Out, "%s", out)
	return nil
}
func workerCmd(args []string, s IO) error {
	if len(args) > 0 {
		return errors.New("_worker takes no arguments")
	}
	l, e := home.Resolve()
	if e != nil {
		return e
	}
	p, e := indexer.Run(context.Background(), indexer.Config{Layout: l})
	if e != nil {
		return e
	}
	return json.NewEncoder(s.Out).Encode(p)
}
func openCmd(args []string, s IO) error {
	f := flag.NewFlagSet("open", flag.ContinueOnError)
	f.SetOutput(s.Err)
	noBrowser := f.Bool("no-browser", false, "start without launching a browser")
	route := f.String("route", "/", "dashboard route")
	current := f.Bool("current-session", false, "open current session or discovery fallback")
	review := f.Bool("review", false, "open a Review placeholder")
	session := f.String("session", "", "explicit opaque session ID")
	if err := f.Parse(args); err != nil {
		return err
	}
	if *current && *session == "" {
		*route = "/context?notice=current-session-unavailable"
	}
	if *session != "" {
		validOpaque, _ := regexp.MatchString(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`, *session)
		if !validOpaque {
			return errors.New("session must be an opaque Inspector ID")
		}
		*route = "/context/" + *session
	}
	if *review {
		*route = "/reviews/new"
	}
	fmt.Fprintln(s.Err, "Open: preparing the local dashboard...")
	l, e := home.Resolve()
	if e != nil {
		return errors.New("Inspector home could not be resolved; run codex-inspector doctor")
	}
	if e = home.Ensure(l); e != nil {
		return errors.New("Inspector data home could not be prepared; run codex-inspector doctor")
	}
	effectiveHome, e := home.ResolveCodexHome()
	if e != nil {
		return errors.New("Codex source home could not be resolved; run codex-inspector doctor")
	}
	lock, e := proc.Acquire(filepath.Join(l.Run, "open.lock"), false)
	if e != nil {
		return errors.New("Inspector could not coordinate local server startup; run codex-inspector doctor")
	}
	defer lock.Close()
	fmt.Fprintln(s.Err, "Open: checking for a healthy local Inspector server...")
	m, e := proc.Read(l.Run)
	reused := e == nil && proc.Healthy(m)
	if reused && m.CodexHome != effectiveHome.Path {
		return errors.New("a healthy Inspector server is bound to a different Codex home; wait for it to stop or use a separate CODEX_INSPECTOR_HOME")
	}
	if !reused {
		fmt.Fprintln(s.Err, "Open: starting the local server; initial indexing will continue in the background...")
		_ = os.Remove(proc.Path(l.Run))
		m, e = startServer(l, s.Err)
		if e != nil {
			fmt.Fprintln(s.Err, "Open: the local server did not become ready.")
			return e
		}
	} else {
		fmt.Fprintln(s.Err, "Open: reusing the healthy local server.")
	}
	url := fmt.Sprintf("http://127.0.0.1:%d%s", m.Port, safeRoute(*route))
	if !m.FragmentExchanged && m.FragmentToken != "" {
		url += "#token=" + m.FragmentToken + "&instanceId=" + m.InstanceID + "&protocolVersion=" + strconv.Itoa(m.ProtocolVersion)
	}
	if e = openDashboard(url, *noBrowser, s.Err, runtime.GOOS, func(target string) error {
		return exec.Command("/usr/bin/open", target).Run()
	}); e != nil {
		return e
	}
	kind := "started"
	if reused {
		kind = "reused"
	}
	fmt.Fprintf(s.Out, "Inspector %s at %s\n", kind, strings.Split(url, "#")[0])
	return nil
}

func openDashboard(url string, noBrowser bool, w io.Writer, goos string, launch func(string) error) error {
	if noBrowser {
		fmt.Fprintln(w, "Open: browser launch suppressed (--no-browser).")
		return nil
	}
	if goos != "darwin" {
		return errors.New("Phase 1 supports macOS only")
	}
	fmt.Fprintln(w, "Open: opening the dashboard in the default browser...")
	if err := launch(url); err != nil {
		fmt.Fprintln(w, "Open: the browser could not be opened.")
		return errors.New("default browser could not be opened; retry with codex-inspector open --no-browser")
	}
	fmt.Fprintln(w, "Open: dashboard opened in the browser.")
	return nil
}

func safeRoute(route string) string {
	if route == "" || route[0] != '/' || strings.Contains(route, "\\") || strings.Contains(route, "..") || strings.ContainsAny(route, "\r\n") {
		return "/"
	}
	return route
}

const serverStartupTimeout = 15 * time.Second

func startServer(l home.Layout, progress io.Writer) (proc.Metadata, error) {
	exe, e := os.Executable()
	if e != nil {
		return proc.Metadata{}, errors.New("Inspector server executable could not be located; run codex-inspector doctor")
	}
	serveArgs := []string{"_serve"}
	if testIdle := os.Getenv("CODEX_INSPECTOR_TEST_IDLE_TIMEOUT"); testIdle != "" {
		serveArgs = append(serveArgs, "--idle-timeout", testIdle)
	}
	cmd := exec.Command(exe, serveArgs...)
	cmd.Stdin = nil
	null, e := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if e != nil {
		return proc.Metadata{}, errors.New("Inspector server output could not be isolated; run codex-inspector doctor")
	}
	defer null.Close()
	cmd.Stdout = null
	cmd.Stderr = null
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if e = cmd.Start(); e != nil {
		return proc.Metadata{}, errors.New("Inspector server could not be started; run codex-inspector doctor")
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	startupCtx, cancel := context.WithTimeout(context.Background(), serverStartupTimeout)
	defer cancel()
	m, childExited, waitErr := awaitServer(startupCtx, l.Run, exited, serverStartupTimeout, func(ctx context.Context) bool {
		timer := time.NewTimer(25 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			return true
		case <-ctx.Done():
			return false
		}
	}, proc.Read, proc.HealthyContext, func(stage string) {
		if message := startupStageMessage(stage); message != "" {
			fmt.Fprintln(progress, message)
		}
	})
	if waitErr != nil {
		terminateServerStart(cmd.Process, exited, childExited, l.Run)
		return proc.Metadata{}, waitErr
	}
	return m, nil
}

func awaitServer(ctx context.Context, run string, exited <-chan error, timeout time.Duration, pause func(context.Context) bool, read func(string) (proc.Metadata, error), healthy func(context.Context, proc.Metadata) bool, onStage func(string)) (proc.Metadata, bool, error) {
	lastStage := ""
	for {
		select {
		case <-exited:
			return proc.Metadata{}, true, errors.New("Inspector server exited before becoming healthy; run codex-inspector doctor")
		case <-ctx.Done():
			return proc.Metadata{}, false, fmt.Errorf("Inspector server was still starting after %s; run codex-inspector doctor", timeout)
		default:
		}
		m, err := read(run)
		if err == nil && m.StartupStage != "" && m.StartupStage != lastStage {
			lastStage = m.StartupStage
			if onStage != nil {
				onStage(m.StartupStage)
			}
		}
		if err == nil && healthy(ctx, m) {
			return m, false, nil
		}
		select {
		case <-ctx.Done():
			return proc.Metadata{}, false, fmt.Errorf("Inspector server was still starting after %s; run codex-inspector doctor", timeout)
		default:
		}
		if !pause(ctx) {
			return proc.Metadata{}, false, fmt.Errorf("Inspector server was still starting after %s; run codex-inspector doctor", timeout)
		}
	}
}

func startupStageMessage(stage string) string {
	switch stage {
	case "codex_host":
		return "Open: checking Codex CLI compatibility..."
	case "source_format":
		return "Open: checking the recorded source format..."
	case "data_home":
		return "Open: checking the private Inspector data home..."
	case "plugin":
		return "Open: checking the Inspector plugin..."
	case "hook_trust":
		return "Open: checking the seven trusted plugin hooks..."
	case "hook_diagnostics":
		return "Open: checking hook diagnostics..."
	case "review_store":
		return "Open: preparing the local Review store..."
	case "http_server":
		return "Open: starting the authenticated dashboard endpoint..."
	default:
		return ""
	}
}

type processKiller interface {
	Kill() error
}

func terminateServerStart(process processKiller, exited <-chan error, childExited bool, run string) {
	if !childExited {
		_ = process.Kill()
		<-exited
	}
	_ = os.Remove(proc.Path(run))
}
func hookCmd(args []string, s IO) error {
	f := flag.NewFlagSet("_hook", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	protocol := f.String("protocol", "", "plugin protocol")
	if f.Parse(args) != nil || !hook.Compatible(*protocol) {
		return errors.New("incompatible plugin protocol")
	}
	return hook.Run(s.In)
}
func serveCmd(args []string, s IO) error {
	f := flag.NewFlagSet("_serve", flag.ContinueOnError)
	f.SetOutput(s.Err)
	idle := f.Duration("idle-timeout", 120*time.Second, "internal test override")
	if e := f.Parse(args); e != nil {
		return e
	}
	l, e := home.Resolve()
	if e != nil {
		return e
	}
	effective, e := home.ResolveCodexHome()
	if e != nil {
		return e
	}
	return server.Run(context.Background(), server.Config{Layout: l, CodexHome: effective.Path, CodexHomeSource: effective.Resolution, IdleTimeout: *idle, AutoSync: true})
}
func request(m proc.Metadata, method, path string, body []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, method, fmt.Sprintf("http://127.0.0.1:%d%s", m.Port, path), bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+m.AccessToken)
	if method != "GET" {
		req.Header.Set("Origin", fmt.Sprintf("http://127.0.0.1:%d", m.Port))
		req.Header.Set("Content-Type", "application/json")
	}
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	b, e := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if e != nil {
		return nil, e
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("server returned %s", resp.Status)
	}
	return b, nil
}
