package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"runtime"
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
	m, _, cancel, errs := startTestServer(t, 1500*time.Millisecond)
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
	case <-time.After(5 * time.Second):
		t.Fatal("idle server did not exit")
	}
}

func TestBrowserCookieJarExchangeAuthenticatesCatalog(t *testing.T) {
	m, _, cancel, errs := startTestServer(t, time.Second)
	defer func() { cancel(); <-errs }()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	origin := fmt.Sprintf("http://127.0.0.1:%d", m.Port)
	payload, _ := json.Marshal(map[string]any{"token": m.FragmentToken, "instanceId": m.InstanceID, "protocolVersion": m.ProtocolVersion})
	request, _ := http.NewRequest("POST", origin+"/v1/token/exchange", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Inspector-Origin", origin)
	request.Header.Set("Sec-Fetch-Site", "same-origin")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("exchange=%d", response.StatusCode)
	}
	if got := len(jar.Cookies(request.URL)); got != 1 {
		t.Fatalf("session cookie count=%d want=1", got)
	}
	response, err = client.Get(origin + "/v1/metrics/catalog")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("catalog=%d", response.StatusCode)
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

func TestSchemaV2HealthAndSessionSnapshotContract(t *testing.T) {
	m, _, cancel, errs := startTestServer(t, time.Second)
	defer func() { cancel(); <-errs }()
	headers := map[string]string{"Authorization": "Bearer " + m.AccessToken}
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schemas", "internal-api.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	for path, schema := range map[string]string{"/v1/health": "Health", "/v1/sessions": "SessionPage"} {
		r, body := req(t, m, "GET", path, nil, headers)
		if r.StatusCode != 200 {
			t.Fatalf("%s status=%d body=%s", path, r.StatusCode, body)
		}
		var value any
		if err = json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		if err = doc.Components.Schemas[schema].Value.VisitJSON(value); err != nil {
			t.Fatalf("%s violates %s: %v", path, schema, err)
		}
	}
	for _, path := range []string{"/v1/sessions?unknown=1", "/v1/sessions?cursor=-1", "/v1/sessions?projectId=%2Ftmp", "/v1/sessions?query=" + strings.Repeat("x", 501)} {
		r, _ := req(t, m, "GET", path, nil, headers)
		if r.StatusCode != 400 {
			t.Fatalf("invalid query accepted: %s => %d", path, r.StatusCode)
		}
	}
}

func TestSSEBufferIsBoundedResumableAndContractValid(t *testing.T) {
	b := newEventBuffer()
	for i := 1; i <= 140; i++ {
		b.publish("revision.available", map[string]any{"schemaVersion": 2, "datasetEpoch": "epoch-demo", "revision": i, "fullRefreshRequired": false})
	}
	if got := len(b.after(0)); got != 128 {
		t.Fatalf("buffer=%d", got)
	}
	resumed := b.after(139)
	if len(resumed) != 1 || resumed[0]["id"] != "140" {
		t.Fatalf("resume=%v", resumed)
	}
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schemas", "internal-api.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(resumed[0])
	var eventJSON any
	_ = json.Unmarshal(encoded, &eventJSON)
	if err = doc.Components.Schemas["StatusEvent"].Value.VisitJSON(eventJSON); err != nil {
		t.Fatalf("event violates contract: %v", err)
	}
}

func TestContextInspectorAPIEndToEnd(t *testing.T) {
	root := t.TempDir()
	layout := home.Layout{Root: filepath.Join(root, "inspector")}
	layout.Reviews = filepath.Join(layout.Root, "reviews")
	layout.Queue = filepath.Join(layout.Root, "queue")
	layout.Run = filepath.Join(layout.Root, "run")
	layout.Logs = filepath.Join(layout.Root, "logs")
	layout.Cache = filepath.Join(layout.Root, "cache")
	codexHome := filepath.Join(root, "codex")
	sessionsDir := filepath.Join(codexHome, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		t.Fatal(err)
	}
	_, sourceFile, _, _ := runtime.Caller(0)
	fixtureDir := filepath.Join(filepath.Dir(sourceFile), "..", "..", "fixtures", "synthetic")
	for _, name := range []string{"root.jsonl", "descendant.jsonl", "truncated.jsonl"} {
		data, err := os.ReadFile(filepath.Join(fixtureDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(sessionsDir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	ready := make(chan proc.Metadata, 1)
	errs := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pv, pp := "0.1.0", 1
	snapshot := compat.Snapshot{Checks: []compat.Check{{Name: "plugin", Status: "ok"}}, PluginVersion: &pv, PluginProtocol: &pp, CLICompatibility: "supported"}
	go func() {
		errs <- Run(ctx, Config{Layout: layout, CodexHome: codexHome, IdleTimeout: 20 * time.Second, Ready: ready, Compatibility: &snapshot, AutoSync: true})
	}()
	var meta proc.Metadata
	select {
	case meta = <-ready:
	case err := <-errs:
		t.Fatalf("server failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("server not ready")
	}
	headers := map[string]string{"Authorization": "Bearer " + meta.AccessToken}
	var sessionBody []byte
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, body := req(t, meta, "GET", "/v1/sessions?pageSize=50&query=delegated%20check", nil, headers)
		if response.StatusCode == 200 && bytes.Contains(body, []byte(`"descendant: message"`)) {
			sessionBody = body
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(sessionBody) == 0 {
		t.Fatal("session discovery never became available")
	}
	var sessions struct {
		AppliedRevision int64 `json:"appliedRevision"`
		Items           []struct {
			SessionID        string   `json:"sessionId"`
			MatchCategories  []string `json:"matchCategories"`
			DirectTokens     *int64   `json:"directTokens"`
			DescendantTokens *int64   `json:"descendantTokens"`
		} `json:"items"`
	}
	if err := json.Unmarshal(sessionBody, &sessions); err != nil || len(sessions.Items) != 1 {
		t.Fatalf("bad discovery response: %s %v", sessionBody, err)
	}
	if len(sessions.Items[0].MatchCategories) != 1 || sessions.Items[0].MatchCategories[0] != "descendant: message" || sessions.Items[0].DirectTokens == nil || *sessions.Items[0].DirectTokens != 2000 || sessions.Items[0].DescendantTokens == nil || *sessions.Items[0].DescendantTokens != 500 {
		t.Fatalf("discovery response is not explainable or metric-aligned: %#v", sessions.Items[0])
	}
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schemas", "internal-api.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	validate := func(schema string, body []byte) {
		t.Helper()
		var value any
		if err := json.Unmarshal(body, &value); err != nil {
			t.Fatal(err)
		}
		if err := doc.Components.Schemas[schema].Value.VisitJSON(value); err != nil {
			t.Fatalf("%s response violates frozen contract: %v\n%s", schema, err, body)
		}
	}
	validate("SessionPage", sessionBody)
	rootID := sessions.Items[0].SessionID
	mapPath := fmt.Sprintf("/v1/sessions/%s/map?revision=%d", rootID, sessions.AppliedRevision)
	response, mapBody := req(t, meta, "GET", mapPath, nil, headers)
	if response.StatusCode != 200 {
		t.Fatalf("map status=%d: %s", response.StatusCode, mapBody)
	}
	validate("SessionMap", mapBody)
	var sessionMap struct {
		Nodes []struct {
			Kind            string `json:"kind"`
			DirectTokens    *int64 `json:"directTokens"`
			InclusiveTokens *int64 `json:"inclusiveTokens"`
		} `json:"nodes"`
		RootTurns []struct {
			TurnID string `json:"turnId"`
		} `json:"rootTurns"`
	}
	if err = json.Unmarshal(mapBody, &sessionMap); err != nil || len(sessionMap.Nodes) != 2 || len(sessionMap.RootTurns) != 2 {
		t.Fatalf("bad map: %s %v", mapBody, err)
	}
	turnID := sessionMap.RootTurns[0].TurnID
	ledgerPath := fmt.Sprintf("/v1/sessions/%s/turns/%s/ledger?revision=%d&pageSize=200", rootID, turnID, sessions.AppliedRevision)
	response, ledgerBody := req(t, meta, "GET", ledgerPath, nil, headers)
	if response.StatusCode != 200 {
		t.Fatalf("ledger status=%d: %s", response.StatusCode, ledgerBody)
	}
	validate("LedgerPage", ledgerBody)
	var ledger struct {
		Items []struct{ EventID, EvidenceID, Kind string } `json:"items"`
	}
	if err = json.Unmarshal(ledgerBody, &ledger); err != nil || len(ledger.Items) != 11 {
		t.Fatalf("bad ledger: %s %v", ledgerBody, err)
	}
	messageEvidence, compactionEvidence := "", ""
	for _, item := range ledger.Items {
		if item.Kind == "message" {
			messageEvidence = item.EvidenceID
		}
		if item.Kind == "compacted" {
			compactionEvidence = item.EvidenceID
		}
	}
	response, evidenceBody := req(t, meta, "GET", fmt.Sprintf("/v1/evidence/%s?revision=%d", messageEvidence, sessions.AppliedRevision), nil, headers)
	if response.StatusCode != 200 || !bytes.Contains(evidenceBody, []byte("Create the fake widget.")) {
		t.Fatalf("exact source evidence failed: status=%d body=%s", response.StatusCode, evidenceBody)
	}
	validate("EvidenceChunk", evidenceBody)
	response, contextBody := req(t, meta, "GET", fmt.Sprintf("/v1/context/%s?revision=%d", messageEvidence, sessions.AppliedRevision), nil, headers)
	if response.StatusCode != 200 || !bytes.Contains(contextBody, []byte(`"fidelity":"unavailable"`)) || !bytes.Contains(contextBody, []byte("Complete model input was not recorded")) {
		t.Fatalf("unavailable context was not explicit: %s", contextBody)
	}
	validate("RecordedContext", contextBody)
	response, compactBody := req(t, meta, "GET", fmt.Sprintf("/v1/context/%s?revision=%d", compactionEvidence, sessions.AppliedRevision), nil, headers)
	if response.StatusCode != 200 || !bytes.Contains(compactBody, []byte(`"fidelity":"exact"`)) || !bytes.Contains(compactBody, []byte("before/after context is not reconstructed")) {
		t.Fatalf("recorded compaction contract failed: %s", compactBody)
	}
	validate("RecordedContext", compactBody)
	if err = os.Remove(filepath.Join(sessionsDir, "root.jsonl")); err != nil {
		t.Fatal(err)
	}
	response, missingBody := req(t, meta, "GET", fmt.Sprintf("/v1/evidence/%s?revision=%d", messageEvidence, sessions.AppliedRevision), nil, headers)
	if response.StatusCode != 200 || !bytes.Contains(missingBody, []byte(`"availability":"source_missing"`)) {
		t.Fatalf("missing source was not honest: status=%d body=%s", response.StatusCode, missingBody)
	}
	validate("EvidenceChunk", missingBody)
}

func TestWatcherIndexesAndConsumesMarkerCreatedAfterStartup(t *testing.T) {
	root := t.TempDir()
	layout := home.Layout{Root: filepath.Join(root, "inspector")}
	layout.Reviews = filepath.Join(layout.Root, "reviews")
	layout.Queue = filepath.Join(layout.Root, "queue")
	layout.Run = filepath.Join(layout.Root, "run")
	layout.Logs = filepath.Join(layout.Root, "logs")
	layout.Cache = filepath.Join(layout.Root, "cache")
	codexHome := filepath.Join(root, "codex")
	rollouts := filepath.Join(codexHome, "sessions")
	if err := os.MkdirAll(rollouts, 0o700); err != nil {
		t.Fatal(err)
	}
	_, sourceFile, _, _ := runtime.Caller(0)
	fixture := filepath.Join(filepath.Dir(sourceFile), "..", "..", "fixtures", "synthetic", "root.jsonl")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(rollouts, "root.jsonl"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	ready := make(chan proc.Metadata, 1)
	errs := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	snapshot := compat.Snapshot{Checks: []compat.Check{{Name: "plugin", Status: "ok"}}, CLICompatibility: "supported"}
	go func() {
		errs <- Run(ctx, Config{Layout: layout, CodexHome: codexHome, IdleTimeout: 10 * time.Second, Ready: ready, Compatibility: &snapshot})
	}()
	var serverMeta proc.Metadata
	select {
	case serverMeta = <-ready:
	case err = <-errs:
		t.Fatalf("server failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("server not ready")
	}
	streamCtx, stopStream := context.WithCancel(context.Background())
	defer stopStream()
	streamEvents := make(chan string, 32)
	go func() {
		request, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/v1/events", serverMeta.Port), nil)
		request.Header.Set("Authorization", "Bearer "+serverMeta.AccessToken)
		request.Header.Set("Origin", fmt.Sprintf("http://127.0.0.1:%d", serverMeta.Port))
		response, requestErr := http.DefaultClient.Do(request)
		if requestErr != nil {
			return
		}
		defer response.Body.Close()
		scanner := bufio.NewScanner(response.Body)
		for scanner.Scan() {
			if strings.HasPrefix(scanner.Text(), "event: ") {
				streamEvents <- strings.TrimPrefix(scanner.Text(), "event: ")
			}
		}
	}()
	marker := hook.Marker{SchemaVersion: "inspector.hook-marker/v1", ProtocolVersion: 1, EventKind: "turn_stop", SessionID: "root-001", TurnID: "turn-root-2", ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	encoded, _ := json.Marshal(marker)
	markerPath := filepath.Join(layout.Queue, "after-start.json")
	if err = os.WriteFile(markerPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if _, statErr := os.Stat(markerPath); os.IsNotExist(statErr) {
			if _, statErr = os.Stat(filepath.Join(layout.Root, "active-index")); statErr == nil {
				origin := fmt.Sprintf("http://127.0.0.1:%d", serverMeta.Port)
				body := []byte(`{"metricKeys":["recorded_tokens","latest_capacity_observation"],"timezone":"UTC","grain":"day"}`)
				response, resultBody := req(t, serverMeta, "POST", "/v1/metrics/query", body, map[string]string{"Authorization": "Bearer " + serverMeta.AccessToken, "Origin": origin, "Content-Type": "application/json"})
				if response.StatusCode != 200 {
					t.Fatalf("metric query=%d %s", response.StatusCode, resultBody)
				}
				var resultJSON any
				if err = json.Unmarshal(resultBody, &resultJSON); err != nil {
					t.Fatal(err)
				}
				doc, loadErr := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schemas", "internal-api.openapi.json"))
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				if err = doc.Components.Schemas["MetricResult"].Value.VisitJSON(resultJSON); err != nil {
					t.Fatalf("metric response violates contract: %v", err)
				}
				appliedRevision := int64(resultJSON.(map[string]any)["appliedRevision"].(float64))
				catalogPath := fmt.Sprintf("/v1/metrics/catalog?requestedRevision=%d", appliedRevision)
				catalogResponse, catalogBody := req(t, serverMeta, "GET", catalogPath, nil, map[string]string{"Authorization": "Bearer " + serverMeta.AccessToken})
				if catalogResponse.StatusCode != 200 {
					t.Fatalf("catalog=%d", catalogResponse.StatusCode)
				}
				var catalogJSON any
				_ = json.Unmarshal(catalogBody, &catalogJSON)
				if err = doc.Components.Schemas["MetricCatalog"].Value.VisitJSON(catalogJSON); err != nil {
					t.Fatalf("catalog contract: %v", err)
				}
				catalogObject := catalogJSON.(map[string]any)
				if int64(catalogObject["appliedRevision"].(float64)) != appliedRevision || catalogObject["filterOptions"] == nil {
					t.Fatalf("catalog snapshot=%v", catalogObject)
				}
				firstMetric := catalogObject["metrics"].([]any)[0].(map[string]any)
				if _, permissiveNestedOptions := firstMetric["filterOptions"]; permissiveNestedOptions {
					t.Fatal("filter options escaped the generated top-level contract")
				}
				oldResponse, oldBody := req(t, serverMeta, "GET", "/v1/metrics/catalog?requestedRevision=1", nil, map[string]string{"Authorization": "Bearer " + serverMeta.AccessToken})
				if oldResponse.StatusCode != 200 {
					t.Fatalf("old catalog=%d", oldResponse.StatusCode)
				}
				var oldCatalog map[string]any
				_ = json.Unmarshal(oldBody, &oldCatalog)
				oldOptions := oldCatalog["filterOptions"].(map[string]any)
				if oldCatalog["appliedRevision"].(float64) != 1 || len(oldOptions["projects"].([]any)) != 0 || len(oldOptions["models"].([]any)) != 0 {
					t.Fatalf("revision-one catalog=%v", oldCatalog)
				}
				invalidCatalog, _ := req(t, serverMeta, "GET", "/v1/metrics/catalog?unknown=1", nil, map[string]string{"Authorization": "Bearer " + serverMeta.AccessToken})
				if invalidCatalog.StatusCode != 400 {
					t.Fatalf("invalid catalog query=%d", invalidCatalog.StatusCode)
				}
				unavailableCatalog, _ := req(t, serverMeta, "GET", fmt.Sprintf("/v1/metrics/catalog?requestedRevision=%d", appliedRevision+1000), nil, map[string]string{"Authorization": "Bearer " + serverMeta.AccessToken})
				if unavailableCatalog.StatusCode != 409 {
					t.Fatalf("unavailable catalog revision=%d", unavailableCatalog.StatusCode)
				}
				oversized := []byte(`{"metricKeys":["recorded_tokens_over_time"],"timezone":"UTC","grain":"hour","start":"2026-01-01T00:00:00Z","end":"2026-04-01T00:00:00Z"}`)
				bounded, _ := req(t, serverMeta, "POST", "/v1/metrics/query", oversized, map[string]string{"Authorization": "Bearer " + serverMeta.AccessToken, "Origin": origin, "Content-Type": "application/json"})
				if bounded.StatusCode != 413 {
					t.Fatalf("oversized metric=%d", bounded.StatusCode)
				}
				seenRevision := false
				eventDeadline := time.After(2 * time.Second)
				for !seenRevision {
					select {
					case name := <-streamEvents:
						seenRevision = name == "revision.available"
					case <-eventDeadline:
						t.Fatal("actual SSE stream did not emit per-commit revision.available")
					}
				}
				stopStream()
				cancel()
				if err = <-errs; err != nil {
					t.Fatal(err)
				}
				return
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	stopStream()
	cancel()
	<-errs
	t.Fatal("watcher did not consume the post-start marker after a successful index")
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
