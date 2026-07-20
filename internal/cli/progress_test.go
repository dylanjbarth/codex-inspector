package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	proc "github.com/dylanjbarth/codex-inspector/internal/process"
)

func TestSyncProgressIsBoundedDeduplicatedAndPrivate(t *testing.T) {
	var out bytes.Buffer
	r := newSyncProgressReporter(&out)
	secret := "private-rollout-id"
	for i := 1; i <= 100; i++ {
		p := indexer.Progress{Inventoried: 100, Processed: i, Boundary: secret, FailureReasons: map[string]int{secret: 1}}
		r.Report(p)
		r.Report(p)
	}
	r.Complete(indexer.Progress{Inventoried: 100, Processed: 100, Boundary: secret})
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) > 22 {
		t.Fatalf("progress was not rate bounded: %d lines", len(lines))
	}
	if strings.Contains(out.String(), secret) {
		t.Fatal("progress leaked source metadata")
	}
	if !strings.Contains(out.String(), "indexing 1/100") || !strings.Contains(out.String(), "indexing 96/100") || !strings.Contains(out.String(), "complete 100/100") {
		t.Fatalf("missing bounded progress transitions: %s", out.String())
	}
	for _, line := range lines {
		if strings.ContainsAny(line, "\x1b\r") {
			t.Fatalf("progress is not plain line-oriented output: %q", line)
		}
	}
}

func TestSyncWaitPreservesJSONStdoutAndReportsStages(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_INSPECTOR_HOME", filepath.Join(root, "inspector"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	if err := os.MkdirAll(filepath.Join(root, "codex", "sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"sync", "--wait"}, IO{Out: &stdout, Err: &stderr}); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var got map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil || got["state"] != "complete" {
		t.Fatalf("stdout is not the stable final JSON: %s err=%v", stdout.String(), err)
	}
	if !strings.HasPrefix(stderr.String(), "Sync: discovering supported Codex sources...\n") || !strings.Contains(stderr.String(), "Sync: complete 0/0 sources") {
		t.Fatalf("missing sync stages: %s", stderr.String())
	}
}

func TestDoctorJSONSuppressesHumanProgressAndKeepsJSONStdout(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_INSPECTOR_HOME", filepath.Join(root, "inspector"))
	t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
	t.Setenv("PATH", "")
	var stdout, stderr bytes.Buffer
	_ = Main([]string{"doctor", "--json"}, IO{Out: &stdout, Err: &stderr})
	var got doctorReport
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatalf("doctor stdout is not JSON: %s err=%v", stdout.String(), err)
	}
	if strings.Contains(stderr.String(), "Doctor:") {
		t.Fatalf("JSON mode emitted human progress: %s", stderr.String())
	}
}

func TestImmediateCommandsKeepStableSeparatedOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"version"}, IO{Out: &stdout, Err: &stderr}); code != 0 {
		t.Fatalf("version exit=%d stderr=%s", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "codex-inspector ") || stderr.Len() != 0 {
		t.Fatalf("version output changed: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	root := t.TempDir()
	t.Setenv("CODEX_INSPECTOR_HOME", root)
	codexHome := filepath.Join(t.TempDir(), "codex")
	t.Setenv("CODEX_HOME", codexHome)
	stdout.Reset()
	stderr.Reset()
	if code := Main([]string{"status"}, IO{Out: &stdout, Err: &stderr}); code != 0 {
		t.Fatalf("status exit=%d stderr=%s", code, stderr.String())
	}
	if stdout.String() != fmt.Sprintf("Inspector is stopped\ncodex_home=%s\ninspector_home=%s\n", codexHome, root) || stderr.Len() != 0 {
		t.Fatalf("status output changed: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	stdout.Reset()
	if code := Main([]string{"stop"}, IO{Out: &stdout, Err: &stderr}); code != 0 {
		t.Fatalf("stop exit=%d stderr=%s", code, stderr.String())
	}
	if stdout.String() != fmt.Sprintf("Inspector is already stopped\ncodex_home=%s\ninspector_home=%s\n", codexHome, root) || stderr.Len() != 0 {
		t.Fatalf("stopped stop output changed: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestSyncTerminalErrorIsActionableAndDoesNotLeakSourcePath(t *testing.T) {
	root := t.TempDir()
	inspector := filepath.Join(root, "inspector")
	codex := filepath.Join(root, "codex")
	secretPath := filepath.Join(codex, "sessions", "private-customer-name.jsonl")
	if err := os.MkdirAll(filepath.Dir(secretPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretPath, []byte("{not-json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_INSPECTOR_HOME", inspector)
	t.Setenv("CODEX_HOME", codex)
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"sync", "--wait"}, IO{Out: &stdout, Err: &stderr}); code != 1 {
		t.Fatalf("sync exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "Sync: failed") || !strings.Contains(stderr.String(), "codex-inspector doctor") || strings.Contains(stderr.String(), secretPath) || strings.Contains(stderr.String(), "private-customer-name") || strings.Contains(stderr.String(), "not-json") {
		t.Fatalf("unsafe or unactionable sync failure: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestBackgroundSyncStoppedErrorIsActionable(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CODEX_INSPECTOR_HOME", root)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "codex"))
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"sync", "--background"}, IO{Out: &stdout, Err: &stderr}); code != 1 {
		t.Fatalf("sync exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "was not queued") || !strings.Contains(stderr.String(), "codex-inspector open") {
		t.Fatalf("unhelpful background error: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestOpenReuseNoBrowserAndBackgroundSyncStagesPreserveStdout(t *testing.T) {
	const token = "private-access-token"
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			fmt.Fprint(w, `{"healthy":true,"instanceId":"test-instance","protocolVersion":1}`)
		case "/v1/sync":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"state":"queued"}`)
		default:
			http.NotFound(w, r)
		}
	})
	ts := httptest.NewServer(h)
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	port, _ := strconv.Atoi(u.Port())
	root := t.TempDir()
	t.Setenv("CODEX_INSPECTOR_HOME", root)
	t.Setenv("CODEX_HOME", filepath.Join(t.TempDir(), "codex"))
	l, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err = home.Ensure(l); err != nil {
		t.Fatal(err)
	}
	effective, err := home.ResolveCodexHome()
	if err != nil {
		t.Fatal(err)
	}
	m := proc.Metadata{InstanceID: "test-instance", PID: os.Getpid(), Port: port, ProtocolVersion: 1, CodexHome: effective.Path, CodexHomeSource: effective.Resolution, StartedAt: time.Now()}
	if err = proc.Write(l.Run, m); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := Main([]string{"open", "--no-browser"}, IO{Out: &stdout, Err: &stderr}); code != 0 {
		t.Fatalf("open exit=%d stderr=%s", code, stderr.String())
	}
	if got := stdout.String(); got != fmt.Sprintf("Inspector reused at %s/\nport=%d\ncodex_home=%s\ninspector_home=%s\n", ts.URL, port, effective.Path, root) {
		t.Fatalf("open stdout changed: %q", got)
	}
	if !strings.Contains(stderr.String(), "reusing the healthy local server") || !strings.Contains(stderr.String(), "browser launch suppressed") || strings.Contains(stderr.String(), token) || strings.Contains(stderr.String(), "private-fragment") {
		t.Fatalf("bad open stages: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Main([]string{"sync", "--background"}, IO{Out: &stdout, Err: &stderr}); code != 0 {
		t.Fatalf("background sync exit=%d stderr=%s", code, stderr.String())
	}
	if stdout.String() != `{"state":"queued"}` {
		t.Fatalf("background stdout changed: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "background indexing queued") || strings.Contains(stderr.String(), token) {
		t.Fatalf("bad background stages: %s", stderr.String())
	}
}

func TestStatusAndStopReportRuntimeEndpointsAndHomes(t *testing.T) {
	root := t.TempDir()
	inspectorHome := filepath.Join(root, "inspector")
	codexHome := filepath.Join(root, "codex")
	t.Setenv("CODEX_INSPECTOR_HOME", inspectorHome)
	t.Setenv("CODEX_HOME", codexHome)
	l, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if err = home.Ensure(l); err != nil {
		t.Fatal(err)
	}
	var port int
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			fmt.Fprint(w, `{"healthy":true,"instanceId":"stop-instance","protocolVersion":1}`)
		case "/v1/status":
			fmt.Fprint(w, `{"process":{"state":"ready","cliVersion":"0.1.0"},"index":{"state":"current","queuedSessionChanges":0},"hook":{"state":"healthy"}}`)
		case "/v1/shutdown":
			if r.Header.Get("Authorization") != "" || r.Header.Get("Origin") != fmt.Sprintf("http://127.0.0.1:%d", port) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_ = os.Remove(proc.Path(l.Run))
		default:
			http.NotFound(w, r)
		}
	})
	ts := httptest.NewServer(h)
	defer ts.Close()
	u, _ := url.Parse(ts.URL)
	port, _ = strconv.Atoi(u.Port())
	m := proc.Metadata{InstanceID: "stop-instance", PID: os.Getpid(), Port: port, ProtocolVersion: 1, CodexHome: codexHome, CodexHomeSource: "environment", StartedAt: time.Now()}
	if err = proc.Write(l.Run, m); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"status"}, IO{Out: &stdout, Err: &stderr}); code != 0 {
		t.Fatalf("status exit=%d stderr=%s", code, stderr.String())
	}
	for _, want := range []string{"Inspector is running", fmt.Sprintf("port=%d", port), "codex_home=" + codexHome, "inspector_home=" + inspectorHome, "process=ready cli=0.1.0 index=current hook=healthy"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("status missing %q: %s", want, stdout.String())
		}
	}
	stdout.Reset()
	if code := Main([]string{"status", "--json"}, IO{Out: &stdout, Err: &stderr}); code != 0 {
		t.Fatalf("JSON status exit=%d stderr=%s", code, stderr.String())
	}
	var statusJSON map[string]any
	if json.Unmarshal(stdout.Bytes(), &statusJSON) != nil || statusJSON["running"] != true || statusJSON["port"] != float64(port) || statusJSON["codexHome"] != codexHome || statusJSON["inspectorHome"] != inspectorHome {
		t.Fatalf("JSON status omitted runtime details: %s", stdout.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Main([]string{"stop"}, IO{Out: &stdout, Err: &stderr}); code != 0 {
		t.Fatalf("stop exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), fmt.Sprintf("port %d", port)) || !strings.Contains(stdout.String(), "Inspector stopped") || !strings.Contains(stdout.String(), "codex_home="+codexHome) || !strings.Contains(stdout.String(), "inspector_home="+inspectorHome) {
		t.Fatalf("stop omitted runtime details: stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
}

func TestAwaitServerAllowsSlowReadinessAndSanitizesFailures(t *testing.T) {
	want := proc.Metadata{InstanceID: "ready", Port: 1234, ProtocolVersion: 1}
	reads := 0
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, childExited, err := awaitServer(ctx, "ignored", make(chan error), time.Second, func(context.Context) bool { return true }, func(string) (proc.Metadata, error) {
		reads++
		if reads < 7 {
			return proc.Metadata{}, os.ErrNotExist
		}
		return want, nil
	}, func(_ context.Context, m proc.Metadata) bool { return m.InstanceID == "ready" }, nil)
	if err != nil || childExited || got.InstanceID != want.InstanceID || reads != 7 {
		t.Fatalf("slow valid startup failed: got=%+v childExited=%t reads=%d err=%v", got, childExited, reads, err)
	}

	exited := make(chan error, 1)
	exited <- fmt.Errorf("raw child failure at /private/path with token-secret")
	_, childExited, err = awaitServer(ctx, "ignored", exited, time.Second, func(context.Context) bool { return true }, func(string) (proc.Metadata, error) {
		return proc.Metadata{}, os.ErrNotExist
	}, func(context.Context, proc.Metadata) bool { return false }, nil)
	if err == nil || !childExited || !strings.Contains(err.Error(), "exited before becoming healthy") || !strings.Contains(err.Error(), "codex-inspector doctor") || strings.Contains(err.Error(), "private/path") || strings.Contains(err.Error(), "token-secret") {
		t.Fatalf("unsanitized or unactionable exit error: %v", err)
	}

	timeoutCtx, timeoutCancel := context.WithCancel(context.Background())
	timeoutCancel()
	_, childExited, err = awaitServer(timeoutCtx, "ignored", make(chan error), 2*time.Second, func(context.Context) bool { return true }, func(string) (proc.Metadata, error) {
		return proc.Metadata{}, os.ErrNotExist
	}, func(context.Context, proc.Metadata) bool { return false }, nil)
	if err == nil || childExited || !strings.Contains(err.Error(), "still starting after 2s") || !strings.Contains(err.Error(), "codex-inspector doctor") {
		t.Fatalf("unactionable timeout: %v", err)
	}
}

func TestAwaitServerReportsOnlyKnownSanitizedStartupStages(t *testing.T) {
	reads := 0
	var stages []string
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, childExited, err := awaitServer(ctx, "ignored", make(chan error), time.Second, func(context.Context) bool { return true }, func(string) (proc.Metadata, error) {
		reads++
		switch reads {
		case 1:
			return proc.Metadata{StartupStage: "source_format"}, nil
		case 2:
			return proc.Metadata{StartupStage: "source_format"}, nil
		case 3:
			return proc.Metadata{StartupStage: "raw-secret-/private/path"}, nil
		default:
			return proc.Metadata{InstanceID: "ready", StartupStage: "http_server"}, nil
		}
	}, func(_ context.Context, m proc.Metadata) bool { return m.InstanceID == "ready" }, func(stage string) {
		if message := startupStageMessage(stage); message != "" {
			stages = append(stages, message)
		}
	})
	if err != nil || childExited {
		t.Fatalf("await server failed: childExited=%t err=%v", childExited, err)
	}
	got := strings.Join(stages, "\n")
	if strings.Count(got, "recorded source format") != 1 || !strings.Contains(got, "local dashboard endpoint") || strings.Contains(got, "raw-secret") || strings.Contains(got, "/private/path") {
		t.Fatalf("unexpected startup stages: %q", got)
	}
}

func TestAwaitServerDeadlineInterruptsBlockingHealthProbe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, childExited, err := awaitServer(ctx, "ignored", make(chan error), 25*time.Millisecond, func(context.Context) bool {
		t.Fatal("pause must not run after the deadline")
		return false
	}, func(string) (proc.Metadata, error) {
		return proc.Metadata{InstanceID: "candidate"}, nil
	}, func(ctx context.Context, _ proc.Metadata) bool {
		<-ctx.Done()
		return false
	}, nil)
	if elapsed := time.Since(started); elapsed > 150*time.Millisecond {
		t.Fatalf("deadline was not bounded: %s", elapsed)
	}
	if err == nil || childExited || !strings.Contains(err.Error(), "still starting after 25ms") {
		t.Fatalf("unexpected blocking-probe result: childExited=%t err=%v", childExited, err)
	}
}

type recordingKiller struct {
	killed chan struct{}
}

func (p *recordingKiller) Kill() error {
	close(p.killed)
	return nil
}

func TestTerminateServerStartKillsReapsAndRemovesMetadata(t *testing.T) {
	run := t.TempDir()
	if err := os.WriteFile(proc.Path(run), []byte("stale-private-metadata"), 0o600); err != nil {
		t.Fatal(err)
	}
	killer := &recordingKiller{killed: make(chan struct{})}
	exited := make(chan error, 1)
	reaped := make(chan struct{})
	go func() {
		<-killer.killed
		close(reaped)
		exited <- fmt.Errorf("private child error")
	}()
	terminateServerStart(killer, exited, false, run)
	select {
	case <-reaped:
	default:
		t.Fatal("terminate returned before child wait completed")
	}
	if _, err := os.Stat(proc.Path(run)); !os.IsNotExist(err) {
		t.Fatalf("failed startup left process metadata: %v", err)
	}
}

func TestTerminateServerStartDoesNotKillOrWaitTwiceAfterObservedExit(t *testing.T) {
	run := t.TempDir()
	if err := os.WriteFile(proc.Path(run), []byte("stale-private-metadata"), 0o600); err != nil {
		t.Fatal(err)
	}
	killer := &recordingKiller{killed: make(chan struct{})}
	terminateServerStart(killer, make(chan error), true, run)
	select {
	case <-killer.killed:
		t.Fatal("already-reaped child was killed twice")
	default:
	}
	if _, err := os.Stat(proc.Path(run)); !os.IsNotExist(err) {
		t.Fatalf("observed child exit left process metadata: %v", err)
	}
}

func TestOpenDashboardReportsBrowserAndSuppressedVariants(t *testing.T) {
	var out bytes.Buffer
	launched := ""
	if err := openDashboard("http://127.0.0.1:1234/#private-token", false, &out, "darwin", func(target string) error {
		launched = target
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if launched == "" || !strings.Contains(out.String(), "opening the dashboard") || !strings.Contains(out.String(), "dashboard opened") || strings.Contains(out.String(), "private-token") {
		t.Fatalf("bad browser progress: launched=%q output=%q", launched, out.String())
	}
	out.Reset()
	launched = ""
	if err := openDashboard("http://127.0.0.1:1234/", true, &out, "darwin", func(target string) error {
		launched = target
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if launched != "" || !strings.Contains(out.String(), "browser launch suppressed") {
		t.Fatalf("bad no-browser progress: launched=%q output=%q", launched, out.String())
	}

	out.Reset()
	err := openDashboard("http://127.0.0.1:1234/", false, &out, "darwin", func(string) error {
		return fmt.Errorf("private browser failure")
	})
	if err == nil || !strings.Contains(out.String(), "browser could not be opened") || strings.Contains(err.Error(), "private browser failure") {
		t.Fatalf("missing browser failure transition: err=%v output=%q", err, out.String())
	}
}

func TestDashboardURLUsesDirectLoopbackRoute(t *testing.T) {
	m := proc.Metadata{Port: 52557, InstanceID: "instance-1", ProtocolVersion: 1}
	got := dashboardURL(m, "/context")
	want := "http://127.0.0.1:52557/context"
	if got != want {
		t.Fatalf("dashboard URL=%q want=%q", got, want)
	}
}

func TestProcessLockReleasedTracksServerCleanup(t *testing.T) {
	run := t.TempDir()
	lock, err := proc.Acquire(filepath.Join(run, "process.lock"), true)
	if err != nil {
		t.Fatal(err)
	}
	if processLockReleased(run) {
		t.Fatal("held process lock reported as released")
	}
	if err = lock.Close(); err != nil {
		t.Fatal(err)
	}
	if !processLockReleased(run) {
		t.Fatal("released process lock remained unavailable")
	}
}
