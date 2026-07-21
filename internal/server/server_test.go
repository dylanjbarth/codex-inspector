package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/compat"
	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/hook"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	proc "github.com/dylanjbarth/codex-inspector/internal/process"
	"github.com/dylanjbarth/codex-inspector/internal/reviews"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
	"github.com/getkin/kin-openapi/openapi3"
)

func startTestServer(t *testing.T, idle time.Duration) (proc.Metadata, home.Layout, context.CancelFunc, <-chan error) {
	return startTestServerWith(t, idle, nil)
}

func TestAutomaticIndexCircuitBreaker(t *testing.T) {
	s := &state{}
	for i := 0; i < maxAutomaticIndexRetries; i++ {
		s.recordIndexResult(indexer.Progress{Rebuilt: true}, nil)
	}
	if !s.autoSuppressed || s.indexError != "index_retry_suppressed" {
		t.Fatalf("repeated rebuilds did not suppress automatic indexing: %#v", s)
	}
	s = &state{}
	for i := 0; i < maxAutomaticIndexRetries; i++ {
		s.recordIndexResult(indexer.Progress{}, errors.New("failed"))
	}
	if !s.autoSuppressed || s.indexError != "index_retry_suppressed" {
		t.Fatalf("repeated failures did not suppress automatic indexing: %#v", s)
	}
}

func TestReviewPlanHandlerFrozenContractAndConfirmationGate(t *testing.T) {
	root := t.TempDir()
	layout := home.Layout{Root: filepath.Join(root, "inspector")}
	layout.Reviews = filepath.Join(layout.Root, "reviews")
	layout.Queue = filepath.Join(layout.Root, "queue")
	layout.Run = filepath.Join(layout.Root, "run")
	layout.Logs = filepath.Join(layout.Root, "logs")
	layout.Cache = filepath.Join(layout.Root, "cache")
	if err := home.Ensure(layout); err != nil {
		t.Fatal(err)
	}
	codexHome := filepath.Join(root, "codex")
	sessions := filepath.Join(codexHome, "sessions")
	if err := os.MkdirAll(sessions, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"root.jsonl", "descendant.jsonl"} {
		b, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "synthetic", name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(sessions, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := indexer.Run(context.Background(), indexer.Config{Layout: layout, CodexHome: codexHome}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	var rootID, projectID string
	if err = store.DB().QueryRow(`SELECT id FROM sessions WHERE source_session_id='root-001'`).Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	if err = store.DB().QueryRow(`SELECT id FROM projects LIMIT 1`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	epoch, baselineRevision, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	manager, err := reviews.New(layout, "/definitely/missing/codex", codexHome, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := &state{layout: layout, reviewManager: manager}
	body, _ := json.Marshal(map[string]any{"scope": map[string]any{"kind": "single_session", "rootSessionId": rootID}, "model": "configured-default", "reasoningEffort": "high", "focus": "demo focus"})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/review-plans", bytes.NewReader(body))
	s.reviewPlan(recorder, request)
	if recorder.Code != 200 {
		t.Fatalf("plan=%d %s", recorder.Code, recorder.Body.String())
	}
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schemas", "internal-api.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	var plan any
	if err = json.Unmarshal(recorder.Body.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if err = doc.Components.Schemas["ReviewPlan"].Value.VisitJSON(plan); err != nil {
		t.Fatalf("plan violates frozen OpenAPI: %v\n%s", err, recorder.Body.String())
	}
	planMap := plan.(map[string]any)
	if planMap["coverage"].(map[string]any)["fidelity"] != "exact" {
		t.Fatalf("complete source inventory was not exact: %s", recorder.Body.String())
	}
	planID := planMap["planId"].(string)
	timeBody, _ := json.Marshal(map[string]any{"scope": map[string]any{"kind": "time_period", "start": "2026-07-01T00:00:00Z", "end": "2026-07-02T00:00:00Z", "timezone": "UTC", "projectId": projectID}, "model": "configured-default", "reasoningEffort": "high", "requestedRevision": baselineRevision})
	timeRecorder := httptest.NewRecorder()
	s.reviewPlan(timeRecorder, httptest.NewRequest(http.MethodPost, "/v1/review-plans", bytes.NewReader(timeBody)))
	if timeRecorder.Code != 200 {
		t.Fatalf("project plan=%d %s", timeRecorder.Code, timeRecorder.Body.String())
	}
	var timePlan map[string]any
	if err = json.Unmarshal(timeRecorder.Body.Bytes(), &timePlan); err != nil {
		t.Fatal(err)
	}
	manifest := timePlan["manifestPreview"].(map[string]any)
	scope := manifest["scope"].(map[string]any)
	projects := timePlan["projectSummary"].(map[string]any)["projects"].([]any)
	if scope["projectId"] != projectID || len(projects) != 1 || projects[0].(map[string]any)["projectId"] != projectID || len(manifest["includedSessionIds"].([]any)) != 2 || len(manifest["includedTurnIds"].([]any)) != 3 {
		t.Fatalf("canonical project did not reach frozen plan: %s", timeRecorder.Body.String())
	}
	if err = doc.Components.Schemas["ReviewPlan"].Value.VisitJSON(timePlan); err != nil {
		t.Fatalf("project plan violates frozen OpenAPI: %v\n%s", err, timeRecorder.Body.String())
	}

	store, err = storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	var sourceID string
	if err = store.DB().QueryRow(`SELECT source_id FROM source_artifact_versions WHERE epoch_id=? AND source_kind<>'session_index' ORDER BY revision DESC LIMIT 1`, epoch).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	partialRevision := baselineRevision + 1
	tx, err := store.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES(?,?,?,'inventory')`, epoch, partialRevision, time.Now().UTC().Format(time.RFC3339Nano)); err == nil {
		_, err = tx.Exec(`INSERT INTO source_artifact_versions(epoch_id,source_id,revision,source_kind,canonical_path,inode,byte_size,mtime_ns,detected_codex_version,adapter_version,state,state_reason,source_evidence_availability,availability_observed_at)
		 SELECT epoch_id,source_id,?,source_kind,canonical_path,inode,byte_size,mtime_ns,detected_codex_version,adapter_version,'indexing','synthetic indexing state',source_evidence_availability,availability_observed_at
		 FROM source_artifact_versions WHERE epoch_id=? AND source_id=? ORDER BY revision DESC LIMIT 1`, partialRevision, epoch, sourceID)
	}
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	store.Close()
	partialBody, _ := json.Marshal(map[string]any{"scope": map[string]any{"kind": "single_session", "rootSessionId": rootID}, "model": "configured-default", "reasoningEffort": "high", "requestedRevision": partialRevision})
	partialRecorder := httptest.NewRecorder()
	s.reviewPlan(partialRecorder, httptest.NewRequest(http.MethodPost, "/v1/review-plans", bytes.NewReader(partialBody)))
	if partialRecorder.Code != 200 {
		t.Fatalf("partial plan=%d %s", partialRecorder.Code, partialRecorder.Body.String())
	}
	var partialPlan map[string]any
	_ = json.Unmarshal(partialRecorder.Body.Bytes(), &partialPlan)
	if partialPlan["coverage"].(map[string]any)["fidelity"] != "derived" || !strings.Contains(partialRecorder.Body.String(), "still indexing") {
		t.Fatalf("partial inventory was not visible: %s", partialRecorder.Body.String())
	}
	launch := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/reviews", strings.NewReader(`{"planId":"`+planID+`","confirmed":false}`))
	s.launchReview(launch, request)
	if launch.Code != 400 {
		t.Fatalf("unconfirmed launch=%d", launch.Code)
	}
	if entries, _ := os.ReadDir(layout.Reviews); len(entries) != 1 {
		t.Fatalf("unconfirmed launch created review artifacts: %v", entries)
	} // hidden acceptance DB only
	bad := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/v1/review-plans", strings.NewReader(`{"scope":{"kind":"single_session","rootSessionId":"`+rootID+`"},"model":"m","reasoningEffort":"high","payload":"secret"}`))
	s.reviewPlan(bad, request)
	if bad.Code != 400 || strings.Contains(bad.Body.String(), "secret") {
		t.Fatalf("unknown/payload leaked: %d %s", bad.Code, bad.Body.String())
	}
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
func TestLoopbackStatusLoadsWithoutAuthenticationAndIdleShutdown(t *testing.T) {
	m, _, cancel, errs := startTestServer(t, 1500*time.Millisecond)
	defer cancel()
	r, statusBody := req(t, m, "GET", "/v1/status", nil, nil)
	if !strings.Contains(r.Header.Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("missing CSP")
	}
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
	r, _ = req(t, m, "POST", "/v1/heartbeat", nil, map[string]string{"Origin": "http://evil.invalid"})
	if r.StatusCode != 403 {
		t.Fatalf("evil origin=%d", r.StatusCode)
	}
	r, _ = req(t, m, "GET", "/v1/health", nil, map[string]string{"Host": "localhost"})
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

func TestActivePassProgressHasDistinctDiscoveringIndexingAndFinalizingPhases(t *testing.T) {
	if got := activePassProgress(false, indexer.Progress{Inventoried: 10}); got != nil {
		t.Fatalf("inactive pass was reported: %+v", got)
	}
	if got := activePassProgress(true, indexer.Progress{}); got == nil || got.Phase != "discovering" || got.InventoriedCount != 0 {
		t.Fatalf("discovering pass was not explicit: %+v", got)
	}
	if got := activePassProgress(true, indexer.Progress{Inventoried: 10, Processed: 2, Skipped: 3}); got == nil || got.Phase != "indexing" || got.RemainingCount != 5 || got.ProcessedCount != 2 || got.SkippedCount != 3 {
		t.Fatalf("active pass counters are not pass-local: %+v", got)
	}
	if got := activePassProgress(true, indexer.Progress{Inventoried: 10, Processed: 2, Skipped: 7, Failed: 1}); got == nil || got.Phase != "finalizing" || got.RemainingCount != 0 {
		t.Fatalf("finalizing pass was not explicit: %+v", got)
	}
	if got := activePassProgress(true, indexer.Progress{Stage: "rebuilding", Inventoried: 10, Scanned: 4, Processed: 10}); got == nil || got.Phase != "rebuilding" || got.ScannedCount != 4 || got.RemainingCount != 6 {
		t.Fatalf("adapter rebuild progress was not explicit: %+v", got)
	}
}

func TestSameOriginShutdownStopsServer(t *testing.T) {
	m, layout, cancel, errs := startTestServer(t, time.Minute)
	defer cancel()
	origin := fmt.Sprintf("http://127.0.0.1:%d", m.Port)
	r, _ := req(t, m, "POST", "/v1/shutdown", nil, map[string]string{"Origin": origin})
	if r.StatusCode != http.StatusAccepted {
		t.Fatalf("shutdown=%d", r.StatusCode)
	}
	select {
	case err := <-errs:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not stop server")
	}
	if _, err := proc.Read(layout.Run); !os.IsNotExist(err) {
		t.Fatalf("server metadata remained after shutdown: %v", err)
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
			r, _ := req(t, m, "POST", "/v1/sync", []byte(tc.body), map[string]string{"Authorization": "Bearer " + m.InstanceID, "Origin": origin, "Content-Type": "application/json"})
			if r.StatusCode != 400 {
				t.Fatalf("status=%d", r.StatusCode)
			}
		})
	}
	m, _, cancel, errs := startTestServer(t, time.Second)
	defer func() { cancel(); <-errs }()
	origin := fmt.Sprintf("http://127.0.0.1:%d", m.Port)
	r, _ := req(t, m, "POST", "/v1/sync", []byte(`{"mode":"background"}`), map[string]string{"Authorization": "Bearer " + m.InstanceID, "Origin": origin, "Content-Type": "application/json"})
	if r.StatusCode != 202 {
		t.Fatalf("valid status=%d", r.StatusCode)
	}
}

func TestSchemaV2HealthAndSessionSnapshotContract(t *testing.T) {
	// Loading and validating the OpenAPI document can exceed the production
	// activity cadence under the race detector; keep the server alive for the
	// contract assertions instead of testing idle shutdown here.
	m, _, cancel, errs := startTestServer(t, 10*time.Second)
	defer func() { cancel(); <-errs }()
	headers := map[string]string{"Authorization": "Bearer " + m.InstanceID}
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

func phase6ShellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func phase6FakeCodex(t *testing.T, root string) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "phase6-fake-codex")
	script := "#!/bin/sh\nGO_WANT_PHASE6_CODEX_HELPER=1 exec " + phase6ShellQuote(executable) + " -test.run '^TestPhase6FakeCodexProcess$'\n"
	if err = os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPhase6FakeCodexProcess(t *testing.T) {
	if os.Getenv("GO_WANT_PHASE6_CODEX_HELPER") != "1" {
		return
	}
	manifestBytes, err := os.ReadFile("manifest.json")
	if err != nil {
		os.Exit(10)
	}
	var manifest reviews.Manifest
	if err = json.Unmarshal(manifestBytes, &manifest); err != nil || len(manifest.Evidence) == 0 {
		os.Exit(11)
	}
	report := reviews.Report{
		SchemaVersion: reviews.SchemaVersion,
		ReviewID:      manifest.ReviewID,
		Scope: reviews.ReportScope{
			Kind:          manifest.Scope.Kind,
			Summary:       "Synthetic Phase 6 end-to-end scope.",
			DatasetEpoch:  manifest.DatasetEpoch,
			IndexRevision: manifest.IndexRevision,
		},
		Model:       "gpt-synthetic",
		Reasoning:   "high",
		CompletedAt: "2026-07-19T12:00:00Z",
		Summary:     "Synthetic end-to-end Review completed.",
		Findings: []reviews.Finding{{
			FindingID:       "finding-phase6",
			Kind:            "strength",
			Lens:            "reusable_leverage",
			Title:           "The evidence path remains reusable",
			Observation:     "The synthetic task used the frozen manifest evidence reference.",
			Impact:          "The Review can return to the same Context Inspector evidence.",
			Support:         "directly_observed",
			EvidenceSummary: "The citation is one of the manifest-authorized evidence IDs.",
			Citations:       []string{manifest.Evidence[0].EvidenceID},
			Recommendation:  "Keep the evidence-linked workflow.",
		}},
	}
	reportBytes, err := json.Marshal(report)
	if err != nil || os.WriteFile("review.json", reportBytes, 0o600) != nil {
		os.Exit(12)
	}
	fmt.Println(`{"type":"thread.started","thread_id":"thread-phase6-e2e"}`)
	fmt.Println(`{"type":"turn.completed"}`)
	os.Exit(0)
}

func TestPhase6AutomatedDemoBoundarySmoke(t *testing.T) {
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
	fakeCodex := phase6FakeCodex(t, root)
	go func() {
		errs <- Run(ctx, Config{Layout: layout, CodexHome: codexHome, IdleTimeout: 20 * time.Second, Ready: ready, Compatibility: &snapshot, AutoSync: true, CodexExecutable: fakeCodex})
	}()
	var meta proc.Metadata
	select {
	case meta = <-ready:
	case err := <-errs:
		t.Fatalf("server failed: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("server not ready")
	}
	headers := map[string]string{"Authorization": "Bearer " + meta.InstanceID}
	var sessionBody []byte
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, body := req(t, meta, "GET", "/v1/sessions?pageSize=50&query=fake%20widget", nil, headers)
		if response.StatusCode == 200 && bytes.Contains(body, []byte(`"root: user message"`)) {
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
	if len(sessions.Items[0].MatchCategories) != 1 || sessions.Items[0].MatchCategories[0] != "root: user message" || sessions.Items[0].DirectTokens == nil || *sessions.Items[0].DirectTokens != 2000 || sessions.Items[0].DescendantTokens == nil || *sessions.Items[0].DescendantTokens != 500 {
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
	origin := fmt.Sprintf("http://127.0.0.1:%d", meta.Port)
	metricRequest := []byte(`{"metricKeys":["recorded_tokens","recorded_tokens_by_kind","top_root_sessions_by_tokens"],"timezone":"UTC","grain":"day"}`)
	response, metricBody := req(t, meta, "POST", "/v1/metrics/query", metricRequest, map[string]string{"Authorization": "Bearer " + meta.InstanceID, "Origin": origin, "Content-Type": "application/json"})
	if response.StatusCode != 200 || !bytes.Contains(metricBody, []byte(`"value":2500`)) || !bytes.Contains(metricBody, []byte(rootID)) {
		t.Fatalf("metric path failed: status=%d body=%s", response.StatusCode, metricBody)
	}
	validate("MetricResult", metricBody)
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
	planRequest, _ := json.Marshal(map[string]any{
		"scope":             map[string]any{"kind": "single_session", "rootSessionId": rootID},
		"model":             "configured-default",
		"reasoningEffort":   "high",
		"requestedRevision": sessions.AppliedRevision,
	})
	response, planBody := req(t, meta, "POST", "/v1/review-plans", planRequest, map[string]string{"Authorization": "Bearer " + meta.InstanceID, "Origin": origin, "Content-Type": "application/json"})
	if response.StatusCode != 200 {
		t.Fatalf("review plan status=%d body=%s", response.StatusCode, planBody)
	}
	validate("ReviewPlan", planBody)
	var reviewPlan struct {
		PlanID          string `json:"planId"`
		ManifestPreview struct {
			ReviewID string `json:"reviewId"`
		} `json:"manifestPreview"`
	}
	if err = json.Unmarshal(planBody, &reviewPlan); err != nil || reviewPlan.PlanID == "" || reviewPlan.ManifestPreview.ReviewID == "" {
		t.Fatalf("invalid review plan: %s", planBody)
	}
	launchRequest, _ := json.Marshal(map[string]any{"planId": reviewPlan.PlanID, "confirmed": true})
	response, launchBody := req(t, meta, "POST", "/v1/reviews", launchRequest, map[string]string{"Authorization": "Bearer " + meta.InstanceID, "Origin": origin, "Content-Type": "application/json"})
	if response.StatusCode != 202 {
		t.Fatalf("review launch status=%d body=%s", response.StatusCode, launchBody)
	}
	var detailBody []byte
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, detailBody = req(t, meta, "GET", "/v1/reviews/"+reviewPlan.ManifestPreview.ReviewID, nil, headers)
		if response.StatusCode == 200 && bytes.Contains(detailBody, []byte(`"status":"complete"`)) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !bytes.Contains(detailBody, []byte(`"status":"complete"`)) || !bytes.Contains(detailBody, []byte(`"threadId":"thread-phase6-e2e"`)) || !bytes.Contains(detailBody, []byte(`"availability":"available"`)) {
		t.Fatalf("accepted Review and task handoff failed: %s", detailBody)
	}
	validate("ReviewDetail", detailBody)
	var detail struct {
		AcceptedReport struct {
			Findings []struct {
				Citations []struct {
					EvidenceID string `json:"evidenceId"`
				} `json:"citations"`
			} `json:"findings"`
		} `json:"acceptedReport"`
	}
	if err = json.Unmarshal(detailBody, &detail); err != nil || len(detail.AcceptedReport.Findings) != 1 || len(detail.AcceptedReport.Findings[0].Citations) != 1 {
		t.Fatalf("citation return contract missing: %s", detailBody)
	}
	citationID := detail.AcceptedReport.Findings[0].Citations[0].EvidenceID
	response, citationBody := req(t, meta, "GET", fmt.Sprintf("/v1/evidence/%s?revision=%d", citationID, sessions.AppliedRevision), nil, headers)
	if response.StatusCode != 200 || !bytes.Contains(citationBody, []byte(`"availability":"available"`)) {
		t.Fatalf("review citation did not return to evidence: status=%d body=%s", response.StatusCode, citationBody)
	}
	validate("EvidenceChunk", citationBody)
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
		request.Header.Set("Authorization", "Bearer "+serverMeta.InstanceID)
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
				response, resultBody := req(t, serverMeta, "POST", "/v1/metrics/query", body, map[string]string{"Authorization": "Bearer " + serverMeta.InstanceID, "Origin": origin, "Content-Type": "application/json"})
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
				catalogResponse, catalogBody := req(t, serverMeta, "GET", catalogPath, nil, map[string]string{"Authorization": "Bearer " + serverMeta.InstanceID})
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
				oldResponse, oldBody := req(t, serverMeta, "GET", "/v1/metrics/catalog?requestedRevision=1", nil, map[string]string{"Authorization": "Bearer " + serverMeta.InstanceID})
				if oldResponse.StatusCode != 200 {
					t.Fatalf("old catalog=%d", oldResponse.StatusCode)
				}
				var oldCatalog map[string]any
				_ = json.Unmarshal(oldBody, &oldCatalog)
				oldOptions := oldCatalog["filterOptions"].(map[string]any)
				if oldCatalog["appliedRevision"].(float64) != 1 || len(oldOptions["projects"].([]any)) != 0 || len(oldOptions["models"].([]any)) != 0 {
					t.Fatalf("revision-one catalog=%v", oldCatalog)
				}
				invalidCatalog, _ := req(t, serverMeta, "GET", "/v1/metrics/catalog?unknown=1", nil, map[string]string{"Authorization": "Bearer " + serverMeta.InstanceID})
				if invalidCatalog.StatusCode != 400 {
					t.Fatalf("invalid catalog query=%d", invalidCatalog.StatusCode)
				}
				unavailableCatalog, _ := req(t, serverMeta, "GET", fmt.Sprintf("/v1/metrics/catalog?requestedRevision=%d", appliedRevision+1000), nil, map[string]string{"Authorization": "Bearer " + serverMeta.InstanceID})
				if unavailableCatalog.StatusCode != 409 {
					t.Fatalf("unavailable catalog revision=%d", unavailableCatalog.StatusCode)
				}
				oversized := []byte(`{"metricKeys":["recorded_tokens_over_time"],"timezone":"UTC","grain":"hour","start":"2026-01-01T00:00:00Z","end":"2026-04-01T00:00:00Z"}`)
				bounded, _ := req(t, serverMeta, "POST", "/v1/metrics/query", oversized, map[string]string{"Authorization": "Bearer " + serverMeta.InstanceID, "Origin": origin, "Content-Type": "application/json"})
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
func TestLoopbackAPIAndNestedPageLoadWithoutCredentials(t *testing.T) {
	m, _, cancel, errs := startTestServer(t, time.Second)
	defer func() { cancel(); <-errs }()
	r, _ := req(t, m, "GET", "/v1/status", nil, nil)
	if r.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", r.StatusCode)
	}
	r, _ = req(t, m, "GET", "/context/example", nil, nil)
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
	r, body := req(t, m, "GET", "/v1/status", nil, map[string]string{"Authorization": "Bearer " + m.InstanceID})
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
	r, body := req(t, m, "GET", "/v1/status", nil, map[string]string{"Authorization": "Bearer " + m.InstanceID})
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

func TestStatusReportsPayloadSafeDiagnosticQueryFailure(t *testing.T) {
	m, layout, cancel, errs := startTestServer(t, time.Second)
	defer func() { cancel(); <-errs }()
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB().Exec(`DROP TABLE source_artifact_versions`); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}

	response, body := req(t, m, "GET", "/v1/status", nil, map[string]string{"Authorization": "Bearer " + m.InstanceID})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.StatusCode, body)
	}
	var got struct {
		Process struct{ State string } `json:"process"`
		Index   struct {
			LastErrorCode    string `json:"lastErrorCode"`
			DiagnosticGroups []struct {
				State, Reason, DetectedVersion, Remediation string
				Count                                       int
			} `json:"diagnosticGroups"`
		} `json:"index"`
	}
	if err = json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.Process.State != "degraded" || got.Index.LastErrorCode != "source_diagnostics_unavailable" || len(got.Index.DiagnosticGroups) != 1 {
		t.Fatalf("query failure falsely reported healthy: %s", body)
	}
	group := got.Index.DiagnosticGroups[0]
	if group.State != "failed" || group.Reason != "diagnostic_query_failed" || group.DetectedVersion != "unknown" || group.Count != 1 || group.Remediation == "" {
		t.Fatalf("unsafe query diagnostic: %+v", group)
	}
	if bytes.Contains(body, []byte("source_artifact_versions")) || bytes.Contains(body, []byte("SQL")) {
		t.Fatalf("database details escaped status: %s", body)
	}
	var statusJSON any
	if err = json.Unmarshal(body, &statusJSON); err != nil {
		t.Fatal(err)
	}
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schemas", "internal-api.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = doc.Components.Schemas["Status"].Value.VisitJSON(statusJSON); err != nil {
		t.Fatalf("query failure status violates bounded schema: %v", err)
	}
}

func TestIndexedPrefixChangeHasActionableRemediation(t *testing.T) {
	remediation := sourceRemediation("requires_rebuild", "indexed_prefix_changed_or_shrank")
	if !strings.Contains(remediation, "codex-inspector sync") || !strings.Contains(remediation, "rebuild") {
		t.Fatalf("prefix-change remediation is not actionable: %q", remediation)
	}
}

func TestUnsupportedCodexVersionHasActionableRemediation(t *testing.T) {
	remediation := sourceRemediation("unsupported", "unsupported_codex_version")
	if !strings.Contains(remediation, "malformed Codex version metadata") || !strings.Contains(remediation, "bug report") {
		t.Fatalf("unsupported-version remediation is not actionable: %q", remediation)
	}
}
