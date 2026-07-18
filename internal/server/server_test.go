package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/compat"
	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/hook"
	proc "github.com/dylanjbarth/codex-inspector/internal/process"
	"github.com/getkin/kin-openapi/openapi3"
)

func startTestServer(t *testing.T, idle time.Duration) (proc.Metadata, home.Layout, context.CancelFunc, <-chan error) {
	return startTestServerWith(t, idle, nil)
}
func startTestServerWith(t *testing.T, idle time.Duration, supplied *compat.Snapshot) (proc.Metadata, home.Layout, context.CancelFunc, <-chan error) {
	t.Helper()
	root := t.TempDir()
	l := home.Layout{Root: root, Reviews: filepath.Join(root, "reviews"), Queue: filepath.Join(root, "queue"), Run: filepath.Join(root, "run"), Logs: filepath.Join(root, "logs"), Cache: filepath.Join(root, "cache")}
	ready := make(chan proc.Metadata, 1)
	errs := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	pv := "0.1.0"
	pp := 1
	snapshot := compat.Snapshot{Checks: []compat.Check{{Name: "plugin", Status: "ok"}}, PluginVersion: &pv, PluginProtocol: &pp, CLICompatibility: "supported"}
	if supplied != nil {
		snapshot = *supplied
	}
	go func() { errs <- Run(ctx, Config{Layout: l, IdleTimeout: idle, Ready: ready, Compatibility: &snapshot}) }()
	select {
	case m := <-ready:
		return m, l, cancel, errs
	case e := <-errs:
		t.Fatalf("server failed: %v", e)
	case <-time.After(3 * time.Second):
		t.Fatal("server not ready")
	}
	return proc.Metadata{}, l, cancel, errs
}
func req(t *testing.T, m proc.Metadata, method, path string, body []byte, headers map[string]string) (*http.Response, []byte) {
	t.Helper()
	r, _ := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", m.Port, path), bytes.NewReader(body))
	for k, v := range headers {
		if strings.EqualFold(k, "Host") {
			r.Host = v
		} else {
			r.Header.Set(k, v)
		}
	}
	response, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	b := make([]byte, 2*1024*1024)
	n, _ := response.Body.Read(b)
	return response, b[:n]
}
func TestSecurityExchangeStatusAndIdleShutdown(t *testing.T) {
	m, _, cancel, errs := startTestServer(t, 350*time.Millisecond)
	defer cancel()
	origin := fmt.Sprintf("http://127.0.0.1:%d", m.Port)
	payload, _ := json.Marshal(map[string]any{"token": m.FragmentToken, "instanceId": m.InstanceID, "protocolVersion": m.ProtocolVersion})
	r, _ := req(t, m, "POST", "/v1/token/exchange", payload, map[string]string{"Origin": origin, "Content-Type": "application/json"})
	if r.StatusCode != 204 {
		t.Fatalf("exchange=%d", r.StatusCode)
	}
	if !strings.Contains(r.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("missing CSP")
	}
	var cookie *http.Cookie
	for _, c := range r.Cookies() {
		if c.Name == "codex_inspector_session" {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("bad cookie: %+v", cookie)
	}
	r, _ = req(t, m, "POST", "/v1/token/exchange", payload, map[string]string{"Origin": origin})
	if r.StatusCode != 409 {
		t.Fatalf("token reuse=%d", r.StatusCode)
	}
	r, statusBody := req(t, m, "GET", "/v1/status", nil, map[string]string{"Cookie": cookie.String()})
	if r.StatusCode != 200 {
		t.Fatalf("status=%d", r.StatusCode)
	}
	var statusJSON any
	if json.Unmarshal(statusBody, &statusJSON) != nil {
		t.Fatal("status is not JSON")
	}
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schemas", "internal-api.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = doc.Components.Schemas["Status"].Value.VisitJSON(statusJSON); err != nil {
		t.Fatalf("status violates frozen OpenAPI schema: %v", err)
	}
	r, _ = req(t, m, "POST", "/v1/heartbeat", nil, map[string]string{"Cookie": cookie.String(), "Origin": "http://evil.invalid"})
	if r.StatusCode != 403 {
		t.Fatalf("evil origin=%d", r.StatusCode)
	}
	r, _ = req(t, m, "GET", "/v1/health", nil, map[string]string{"Authorization": "Bearer " + m.AccessToken, "Host": "localhost"})
	if r.StatusCode != 403 {
		t.Fatalf("evil host=%d", r.StatusCode)
	}
	select {
	case err := <-errs:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("idle server did not exit")
	}
}

func TestExchangeRejectsIncompleteMismatchedAndExtraBootstrap(t *testing.T) {
	for _, tc := range []struct {
		name string
		body func(proc.Metadata) any
		want int
	}{
		{"missing metadata", func(m proc.Metadata) any { return map[string]any{"token": m.FragmentToken} }, 400},
		{"wrong instance", func(m proc.Metadata) any {
			return map[string]any{"token": m.FragmentToken, "instanceId": "wrong", "protocolVersion": 1}
		}, 409},
		{"wrong protocol", func(m proc.Metadata) any {
			return map[string]any{"token": m.FragmentToken, "instanceId": m.InstanceID, "protocolVersion": 2}
		}, 409},
		{"extra field", func(m proc.Metadata) any {
			return map[string]any{"token": m.FragmentToken, "instanceId": m.InstanceID, "protocolVersion": 1, "payload": "secret"}
		}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _, cancel, errs := startTestServer(t, time.Second)
			defer func() { cancel(); <-errs }()
			b, _ := json.Marshal(tc.body(m))
			origin := fmt.Sprintf("http://127.0.0.1:%d", m.Port)
			r, _ := req(t, m, "POST", "/v1/token/exchange", b, map[string]string{"Origin": origin, "Content-Type": "application/json"})
			if r.StatusCode != tc.want {
				t.Fatalf("status=%d want=%d", r.StatusCode, tc.want)
			}
		})
	}
}

func TestSyncRejectsUnknownTrailingAndMalformedRequests(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"unknown field", `{"mode":"wait","payload":"secret"}`},
		{"trailing object", `{"mode":"wait"} {"mode":"background"}`},
		{"malformed", `{"mode":"wait"`},
		{"missing mode", `{}`},
		{"unsupported mode", `{"mode":"future"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, _, cancel, errs := startTestServer(t, time.Second)
			defer func() { cancel(); <-errs }()
			origin := fmt.Sprintf("http://127.0.0.1:%d", m.Port)
			r, _ := req(t, m, "POST", "/v1/sync", []byte(tc.body), map[string]string{"Authorization": "Bearer " + m.AccessToken, "Origin": origin, "Content-Type": "application/json"})
			if r.StatusCode != 400 {
				t.Fatalf("status=%d", r.StatusCode)
			}
		})
	}
	m, _, cancel, errs := startTestServer(t, time.Second)
	defer func() { cancel(); <-errs }()
	origin := fmt.Sprintf("http://127.0.0.1:%d", m.Port)
	r, _ := req(t, m, "POST", "/v1/sync", []byte(`{"mode":"background"}`), map[string]string{"Authorization": "Bearer " + m.AccessToken, "Origin": origin, "Content-Type": "application/json"})
	if r.StatusCode != 202 {
		t.Fatalf("valid status=%d", r.StatusCode)
	}
}
func TestUnauthenticatedAPIRejectedAndNoSecretsInResponses(t *testing.T) {
	m, _, cancel, errs := startTestServer(t, time.Second)
	defer func() { cancel(); <-errs }()
	r, b := req(t, m, "GET", "/v1/status", nil, nil)
	if r.StatusCode != 401 {
		t.Fatalf("status=%d", r.StatusCode)
	}
	if bytes.Contains(b, []byte(m.AccessToken)) || bytes.Contains(b, []byte(m.FragmentToken)) {
		t.Fatal("secret leaked")
	}
	r, _ = req(t, m, "GET", "/", nil, nil)
	if r.StatusCode != 200 {
		t.Fatalf("shell=%d", r.StatusCode)
	}
}

func TestStatusExcludesInvalidPersistedMarkerAndReportsDiagnostic(t *testing.T) {
	m, layout, cancel, errs := startTestServer(t, time.Second)
	defer func() { cancel(); <-errs }()
	valid := hook.Marker{SchemaVersion: "inspector.hook-marker/v1", ProtocolVersion: 1, EventKind: "turn_stop", SessionID: "valid-session", ObservedAt: "2026-07-18T00:00:00Z"}
	b, _ := json.Marshal(valid)
	if err := os.WriteFile(filepath.Join(layout.Queue, "001-valid.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.SessionID = "bad/id"
	invalid.ObservedAt = "not-a-time"
	b, _ = json.Marshal(invalid)
	for i := 0; i < 55; i++ {
		if err := os.WriteFile(filepath.Join(layout.Queue, fmt.Sprintf("%03d-invalid.json", i+2)), b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r, body := req(t, m, "GET", "/v1/status", nil, map[string]string{"Authorization": "Bearer " + m.AccessToken})
	if r.StatusCode != 200 {
		t.Fatalf("status=%d", r.StatusCode)
	}
	var got struct {
		Hook struct {
			State      string `json:"state"`
			LastMarker struct {
				SessionID string `json:"sessionId"`
			} `json:"lastMarker"`
			Diagnostics []compat.Diagnostic `json:"diagnostics"`
		} `json:"hook"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Hook.LastMarker.SessionID != "valid-session" {
		t.Fatalf("invalid marker escaped: %s", body)
	}
	if got.Hook.State != "degraded" || len(got.Hook.Diagnostics) != 1 || got.Hook.Diagnostics[0].Code != "invalid_payload" || got.Hook.Diagnostics[0].Count != 55 {
		t.Fatalf("missing diagnostic: %s", body)
	}
	var statusJSON any
	if json.Unmarshal(body, &statusJSON) != nil {
		t.Fatal("status json")
	}
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schemas", "internal-api.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = doc.Components.Schemas["Status"].Value.VisitJSON(statusJSON); err != nil {
		t.Fatalf("51+ invalid status violates schema: %v", err)
	}
}

func TestStatusSurfacesPluginDataDiagnostics(t *testing.T) {
	pv := "0.1.0"
	pp := 1
	snapshot := compat.Snapshot{Checks: []compat.Check{{Name: "hook_missing_cli", Status: "error"}}, PluginVersion: &pv, PluginProtocol: &pp, CLICompatibility: "unknown", HookDiagnostics: []compat.Diagnostic{{Code: "missing_cli", Severity: "warning", Count: 1, LastObservedAt: "2026-07-18T00:00:00Z"}}}
	m, _, cancel, errs := startTestServerWith(t, time.Second, &snapshot)
	defer func() { cancel(); <-errs }()
	r, body := req(t, m, "GET", "/v1/status", nil, map[string]string{"Authorization": "Bearer " + m.AccessToken})
	if r.StatusCode != 200 {
		t.Fatalf("status=%d", r.StatusCode)
	}
	var got struct {
		Process struct{ State, CLICompatibility string } `json:"process"`
		Hook    struct {
			State       string              `json:"state"`
			Diagnostics []compat.Diagnostic `json:"diagnostics"`
		} `json:"hook"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Process.State != "degraded" || got.Process.CLICompatibility != "unknown" || got.Hook.State != "degraded" || len(got.Hook.Diagnostics) != 1 || got.Hook.Diagnostics[0].Code != "missing_cli" {
		t.Fatalf("diagnostic not surfaced: %s", body)
	}
}
