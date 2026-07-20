package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/compat"
	"github.com/dylanjbarth/codex-inspector/internal/evidence"
	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/hook"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	"github.com/dylanjbarth/codex-inspector/internal/inspector"
	"github.com/dylanjbarth/codex-inspector/internal/metrics"
	proc "github.com/dylanjbarth/codex-inspector/internal/process"
	"github.com/dylanjbarth/codex-inspector/internal/reviews"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
	"github.com/dylanjbarth/codex-inspector/internal/version"
)

//go:embed assets/* assets/assets/*
var assets embed.FS

type Config struct {
	Layout          home.Layout
	CodexHome       string
	CodexHomeSource string
	IdleTimeout     time.Duration
	Ready           chan<- proc.Metadata
	Compatibility   *compat.Snapshot
	AutoSync        bool
	CodexExecutable string
}
type state struct {
	mu              sync.Mutex
	ctx             context.Context
	indexWG         sync.WaitGroup
	meta            proc.Metadata
	cookie          string
	exchanged       bool
	lastActive      time.Time
	host, origin    string
	layout          home.Layout
	codexHome       string
	codexHomeSource string
	compat          compat.Snapshot
	indexing        bool
	indexProgress   indexer.Progress
	indexError      string
	metricEngine    *metrics.Engine
	reviewManager   *reviews.Manager
	events          *eventBuffer
}

type eventBuffer struct {
	mu     sync.Mutex
	next   int64
	events []map[string]any
	wake   chan struct{}
}

func newEventBuffer() *eventBuffer { return &eventBuffer{wake: make(chan struct{}, 1)} }
func (b *eventBuffer) publish(name string, data any) {
	b.mu.Lock()
	b.next++
	ev := map[string]any{"id": strconv.FormatInt(b.next, 10), "protocolVersion": 1, "event": name, "emittedAt": time.Now().UTC().Format(time.RFC3339Nano), "data": data}
	b.events = append(b.events, ev)
	if len(b.events) > 128 {
		b.events = b.events[len(b.events)-128:]
	}
	b.mu.Unlock()
	select {
	case b.wake <- struct{}{}:
	default:
	}
}
func (b *eventBuffer) after(id int64) []map[string]any {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []map[string]any{}
	for _, e := range b.events {
		n, _ := strconv.ParseInt(e["id"].(string), 10, 64)
		if n > id {
			out = append(out, e)
		}
	}
	return out
}

var opaqueID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func token(n int) (string, error) {
	b := make([]byte, n)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func Run(ctx context.Context, c Config) error {
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = 120 * time.Second
	}
	if err := home.Ensure(c.Layout); err != nil {
		return err
	}
	if c.CodexHome == "" {
		effective, err := home.ResolveCodexHome()
		if err != nil {
			return err
		}
		c.CodexHome, c.CodexHomeSource = effective.Path, effective.Resolution
	} else {
		absolute, err := filepath.Abs(c.CodexHome)
		if err != nil {
			return err
		}
		if canonical, err := filepath.EvalSymlinks(absolute); err == nil {
			absolute = canonical
		}
		c.CodexHome = filepath.Clean(absolute)
	}
	if c.CodexHomeSource == "" {
		c.CodexHomeSource = "environment"
	}
	if err := home.BindDatasetHome(c.Layout, home.CodexHome{Path: c.CodexHome, Resolution: c.CodexHomeSource}); err != nil {
		return err
	}
	lock, err := proc.Acquire(filepath.Join(c.Layout.Run, "process.lock"), true)
	if err != nil {
		return errors.New("another Inspector process owns the process lock")
	}
	defer lock.Close()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	addr := ln.Addr().(*net.TCPAddr)
	id, err := token(24)
	if err != nil {
		return err
	}
	// Base64url can begin with '-' or '_', while public opaque identifiers must
	// begin with an alphanumeric character.
	id = "i_" + id
	access, err := token(32)
	if err != nil {
		return err
	}
	fragment, err := token(32)
	if err != nil {
		return err
	}
	cookie, err := token(32)
	if err != nil {
		return err
	}
	m := proc.Metadata{InstanceID: id, PID: os.Getpid(), Port: addr.Port, ProtocolVersion: version.Protocol, AccessToken: access, FragmentToken: fragment, StartupStage: "codex_host", CodexHome: c.CodexHome, CodexHomeSource: c.CodexHomeSource, StartedAt: time.Now().UTC()}
	if err = proc.Write(c.Layout.Run, m); err != nil {
		return err
	}
	defer proc.RemoveIfInstance(c.Layout.Run, id)
	snapshot := compat.Snapshot{}
	if c.Compatibility != nil {
		snapshot = *c.Compatibility
	} else {
		snapshot = compat.InspectWithProgress(c.Layout, false, func(stage string) {
			m.StartupStage = stage
			_ = proc.Write(c.Layout.Run, m)
		})
	}
	m.StartupStage = "review_store"
	_ = proc.Write(c.Layout.Run, m)
	s := &state{ctx: ctx, meta: m, cookie: cookie, lastActive: time.Now(), host: fmt.Sprintf("127.0.0.1:%d", addr.Port), origin: fmt.Sprintf("http://127.0.0.1:%d", addr.Port), layout: c.Layout, codexHome: c.CodexHome, codexHomeSource: c.CodexHomeSource, compat: snapshot, metricEngine: metrics.New(64), events: newEventBuffer()}
	s.reviewManager, err = reviews.New(c.Layout, c.CodexExecutable, c.CodexHome, func(id, status string) {
		s.mu.Lock()
		s.lastActive = time.Now()
		s.mu.Unlock()
		s.events.publish("review.changed", map[string]any{"reviewId": id, "status": status})
	})
	if err != nil {
		return err
	}
	m.StartupStage = "http_server"
	s.meta.StartupStage = m.StartupStage
	_ = proc.Write(c.Layout.Run, m)
	defer s.indexWG.Wait()
	if c.AutoSync {
		s.startIndex()
	}
	mux := http.NewServeMux()
	s.routes(mux)
	h := s.headers(mux)
	// Streaming responses stay open until the browser disconnects; bounded JSON
	// handlers enforce their own body and response limits.
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	if c.Ready != nil {
		c.Ready <- m
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			srv.Shutdown(context.Background())
			return nil
		case err := <-errc:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-tick.C:
			if entries, readErr := os.ReadDir(c.Layout.Queue); readErr == nil {
				for _, entry := range entries {
					if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
						s.startIndex()
						break
					}
				}
			}
			s.mu.Lock()
			idle := !s.indexing && !s.reviewManager.Active() && time.Since(s.lastActive) >= c.IdleTimeout
			s.mu.Unlock()
			if idle {
				shutdown, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				srv.Shutdown(shutdown)
				cancel()
				return nil
			}
		}
	}
}

func (s *state) headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; font-src 'none'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'; form-action 'none'")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func (s *state) routes(m *http.ServeMux) {
	sub, _ := fs.Sub(assets, "assets")
	m.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		b, err := fs.ReadFile(sub, name)
		if err != nil && !strings.HasPrefix(name, "assets/") {
			b, err = fs.ReadFile(sub, "index.html")
			name = "index.html"
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", mime.TypeByExtension(filepath.Ext(name)))
		w.Write(b)
	})
	m.HandleFunc("POST /v1/token/exchange", s.exchange)
	m.HandleFunc("GET /v1/startup-diagnostics", s.startupDiagnostics)
	m.HandleFunc("GET /v1/health", s.auth(s.health))
	m.HandleFunc("POST /v1/heartbeat", s.auth(s.sameOrigin(s.heartbeat)))
	m.HandleFunc("GET /v1/status", s.auth(s.status))
	m.HandleFunc("GET /v1/metrics/catalog", s.auth(s.metricCatalog))
	m.HandleFunc("POST /v1/metrics/query", s.auth(s.sameOrigin(s.metricQuery)))
	m.HandleFunc("GET /v1/events", s.auth(s.sameOrigin(s.streamEvents)))
	m.HandleFunc("POST /v1/sync", s.auth(s.sameOrigin(s.sync)))
	m.HandleFunc("GET /v1/sessions", s.auth(s.sessions))
	m.HandleFunc("GET /v1/sessions/{sessionId}/map", s.auth(s.sessionMap))
	m.HandleFunc("GET /v1/sessions/{sessionId}/turns/{turnId}/ledger", s.auth(s.turnLedger))
	m.HandleFunc("GET /v1/evidence/{evidenceId}", s.auth(s.evidence))
	m.HandleFunc("GET /v1/context/{evidenceId}", s.auth(s.recordedContext))
	m.HandleFunc("POST /v1/review-plans", s.auth(s.sameOrigin(s.reviewPlan)))
	m.HandleFunc("GET /v1/reviews", s.auth(s.reviews))
	m.HandleFunc("POST /v1/reviews", s.auth(s.sameOrigin(s.launchReview)))
	m.HandleFunc("GET /v1/reviews/{reviewId}", s.auth(s.reviewDetail))
}
func (s *state) validHost(r *http.Request) bool { return r.Host == s.host }
func (s *state) validOrigin(r *http.Request) bool {
	if r.Header.Get("Origin") == s.origin {
		return true
	}
	return r.Header.Get("Origin") == "" && r.Header.Get("X-Inspector-Origin") == s.origin && r.Header.Get("Sec-Fetch-Site") == "same-origin"
}
func (s *state) sameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validHost(r) || !s.validOrigin(r) {
			s.problem(w, 403, "origin_rejected", "Request origin rejected")
			return
		}
		next(w, r)
	}
}
func (s *state) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validHost(r) {
			s.problem(w, 403, "host_rejected", "Request host rejected")
			return
		}
		ok := false
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			v := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			ok = subtle.ConstantTimeCompare([]byte(v), []byte(s.meta.AccessToken)) == 1
		} else if c, e := r.Cookie("codex_inspector_session"); e == nil {
			ok = subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.cookie)) == 1
		}
		if !ok {
			s.problem(w, 401, "unauthorized", "Authentication required")
			return
		}
		next(w, r)
	}
}
func (s *state) exchange(w http.ResponseWriter, r *http.Request) {
	if !s.validHost(r) || !s.validOrigin(r) {
		s.problem(w, 403, "origin_rejected", "Request origin rejected")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var in struct {
		Token           string `json:"token"`
		InstanceID      string `json:"instanceId"`
		ProtocolVersion int    `json:"protocolVersion"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || len(in.Token) < 32 || len(in.Token) > 256 || !opaqueID.MatchString(in.InstanceID) {
		s.problem(w, 400, "invalid_request", "Invalid token request")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		s.problem(w, 400, "invalid_request", "Invalid token request")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exchanged {
		s.problem(w, 409, "token_used", "Startup token was already exchanged")
		return
	}
	if in.InstanceID != s.meta.InstanceID || in.ProtocolVersion != s.meta.ProtocolVersion {
		s.problem(w, 409, "bootstrap_mismatch", "Startup metadata does not match this process")
		return
	}
	if subtle.ConstantTimeCompare([]byte(in.Token), []byte(s.meta.FragmentToken)) != 1 {
		s.problem(w, 401, "invalid_token", "Startup token rejected")
		return
	}
	nextMeta := s.meta
	nextMeta.FragmentToken = ""
	nextMeta.FragmentExchanged = true
	if err := proc.Write(s.layout.Run, nextMeta); err != nil {
		s.problem(w, 500, "metadata_write_failed", "Secure session metadata could not be updated")
		return
	}
	s.exchanged = true
	s.meta = nextMeta
	s.lastActive = time.Now()
	http.SetCookie(w, &http.Cookie{Name: "codex_inspector_session", Value: s.cookie, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 3600})
	w.WriteHeader(204)
}
func (s *state) health(w http.ResponseWriter, r *http.Request) {
	s.write(w, map[string]any{"healthy": true, "instanceId": s.meta.InstanceID, "protocolVersion": version.Protocol, "cliVersion": version.CLI, "indexSchemaVersion": version.IndexSchema})
}
func (s *state) startupDiagnostics(w http.ResponseWriter, r *http.Request) {
	if !s.validHost(r) {
		s.problem(w, 403, "host_rejected", "Request host rejected")
		return
	}
	s.write(w, map[string]any{
		"codexHome": map[string]any{
			"path":       s.codexHome,
			"resolution": s.codexHomeSource,
		},
		"inspectorHome": map[string]any{
			"path": s.layout.Root,
		},
	})
}
func (s *state) heartbeat(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.lastActive = time.Now()
	s.mu.Unlock()
	w.WriteHeader(204)
}
func (s *state) sync(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var in struct {
		Mode string `json:"mode"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || (in.Mode != "background" && in.Mode != "wait") {
		s.problem(w, 400, "invalid_sync", "mode must be background or wait")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		s.problem(w, 400, "invalid_sync", "request must contain exactly one sync value")
		return
	}
	state := s.startIndex()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	json.NewEncoder(w).Encode(map[string]any{"syncId": "index-sync", "state": state})
}

func (s *state) startIndex() string {
	s.mu.Lock()
	if s.indexing {
		s.mu.Unlock()
		return "coalesced"
	}
	s.indexing = true
	s.indexError = ""
	s.lastActive = time.Now()
	s.indexWG.Add(1)
	s.mu.Unlock()
	s.events.publish("status.changed", map[string]any{"state": "scanning", "queuedChanges": 0})
	go func() {
		defer s.indexWG.Done()
		var lastEpoch string
		var lastRevision int64
		if store, e := storage.Open(filepath.Join(s.layout.Root, "inspector.db")); e == nil {
			lastEpoch, lastRevision, _ = store.Snapshot()
			_ = store.Close()
		}
		p, err := indexer.Run(s.ctx, indexer.Config{Layout: s.layout, CodexHome: s.codexHome, OnCommit: func(p indexer.Progress) {
			s.mu.Lock()
			s.indexProgress = p
			s.lastActive = time.Now()
			s.mu.Unlock()
			s.events.publish("sync.progress", map[string]any{"processed": p.Processed, "queued": max(0, p.Inventoried-p.Processed-p.Skipped-p.Failed-p.RequiresRebuild), "skipped": p.Skipped, "failed": p.Failed, "inventoryComplete": p.Processed+p.Skipped+p.Failed+p.RequiresRebuild >= p.Inventoried})
			if store, e := storage.Open(filepath.Join(s.layout.Root, "inspector.db")); e == nil {
				epoch, revision, _ := store.Snapshot()
				_ = store.Close()
				if epoch != lastEpoch || revision > lastRevision {
					s.events.publish("revision.available", map[string]any{"schemaVersion": 2, "datasetEpoch": epoch, "revision": revision, "fullRefreshRequired": lastEpoch != "" && epoch != lastEpoch})
					lastEpoch, lastRevision = epoch, revision
				}
			}
		}})
		s.mu.Lock()
		s.indexProgress = p
		s.indexing = false
		if err != nil {
			s.indexError = "index_failed"
		}
		s.lastActive = time.Now()
		s.mu.Unlock()
		stateName := "idle"
		if err != nil {
			stateName = "failed"
		}
		s.events.publish("status.changed", map[string]any{"state": stateName, "queuedChanges": 0})
		if store, e := storage.Open(filepath.Join(s.layout.Root, "inspector.db")); e == nil {
			epoch, revision, _ := store.Snapshot()
			_ = store.Close()
			if epoch != lastEpoch || revision > lastRevision {
				s.events.publish("revision.available", map[string]any{"schemaVersion": 2, "datasetEpoch": epoch, "revision": revision, "fullRefreshRequired": lastEpoch != "" && epoch != lastEpoch})
			}
		}
	}()
	return "running"
}

func (s *state) metricCatalog(w http.ResponseWriter, r *http.Request) {
	for key := range r.URL.Query() {
		if key != "requestedRevision" {
			s.problem(w, 400, "invalid_query_parameter", "Query parameter is invalid")
			return
		}
	}
	revision, err := storage.ParseRevision(r.URL.Query().Get("requestedRevision"))
	if err != nil {
		s.problem(w, 400, "invalid_revision", "Revision is invalid")
		return
	}
	catalog := s.metricEngine.Catalog()
	store, err := storage.Open(filepath.Join(s.layout.Root, "inspector.db"))
	if err != nil {
		s.problem(w, 500, "index_unavailable", "Index unavailable")
		return
	}
	defer store.Close()
	epoch, applied, options, err := metrics.Options(r.Context(), store, revision)
	if err != nil {
		if err.Error() == "revision_unavailable" {
			s.problem(w, 409, "revision_unavailable", "Revision is unavailable")
		} else if errors.Is(err, metrics.ErrResponseTooLarge) {
			s.problem(w, 413, "response_too_large", "Metric filter catalog exceeds the bounded response")
		} else {
			s.problem(w, 500, "query_failed", "Metric catalog query failed")
		}
		return
	}
	observed := len(options.Projects) + len(options.Models) + len(options.ReasoningEfforts) + len(options.ContributionKinds)
	catalog["schemaVersion"] = version.IndexSchema
	catalog["datasetEpoch"] = epoch
	catalog["appliedRevision"] = applied
	catalog["coverage"] = map[string]any{"fidelity": "exact", "observed": observed, "eligible": observed}
	catalog["filterOptions"] = options
	s.write(w, catalog)
}

func (s *state) metricQuery(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var q metrics.Query
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(&q) != nil {
		s.problem(w, 400, "invalid_metric_query", "Metric query is invalid")
		return
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		s.problem(w, 400, "invalid_metric_query", "Metric query must contain one object")
		return
	}
	store, err := storage.Open(filepath.Join(s.layout.Root, "inspector.db"))
	if err != nil {
		s.problem(w, 500, "index_unavailable", "Index unavailable")
		return
	}
	defer store.Close()
	result, err := s.metricEngine.Query(r.Context(), store, q)
	if err != nil {
		if err.Error() == "revision_unavailable" {
			s.problem(w, 409, "revision_unavailable", "Revision is unavailable")
		} else if errors.Is(err, metrics.ErrResponseTooLarge) {
			s.problem(w, 413, "metric_response_too_large", "Metric query exceeds frozen response bounds")
		} else {
			s.problem(w, 400, "invalid_metric_query", "Metric query is invalid")
		}
		return
	}
	b, err := json.Marshal(result)
	if err != nil {
		s.problem(w, 500, "metric_encode_failed", "Metric response failed")
		return
	}
	if len(b) > 2*1024*1024 {
		s.problem(w, 413, "metric_response_too_large", "Metric response exceeds 2 MiB")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(b, '\n'))
}

func (s *state) streamEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.problem(w, 500, "stream_unavailable", "Event stream unavailable")
		return
	}
	last := int64(0)
	if raw := r.Header.Get("Last-Event-ID"); raw != "" {
		var err error
		last, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || last < 0 {
			s.problem(w, 400, "invalid_event_id", "Last event ID is invalid")
			return
		}
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Connection", "keep-alive")
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	for {
		events := s.events.after(last)
		for _, event := range events {
			b, _ := json.Marshal(event)
			fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", event["id"], event["event"], b)
			last, _ = strconv.ParseInt(event["id"].(string), 10, 64)
		}
		if len(events) > 0 {
			flusher.Flush()
		}
		select {
		case <-r.Context().Done():
			return
		case <-s.events.wake:
		case <-heartbeat.C:
			s.events.publish("heartbeat", map[string]any{"instanceId": s.meta.InstanceID})
		}
	}
}

func (s *state) sessions(w http.ResponseWriter, r *http.Request) {
	allowed := map[string]bool{"revision": true, "pageSize": true, "query": true, "projectId": true, "rootId": true, "cursor": true}
	for key := range r.URL.Query() {
		if !allowed[key] {
			s.problem(w, 400, "invalid_query_parameter", "Query parameter is invalid")
			return
		}
	}
	revision, err := storage.ParseRevision(r.URL.Query().Get("revision"))
	if err != nil {
		s.problem(w, 400, "invalid_revision", "Revision is invalid")
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("pageSize"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 200 {
			s.problem(w, 400, "invalid_page_size", "Page size is invalid")
			return
		}
	}
	query := r.URL.Query().Get("query")
	if len(query) > 500 {
		s.problem(w, 400, "invalid_query", "Query is too long")
		return
	}
	projectID := r.URL.Query().Get("projectId")
	if projectID != "" && !opaqueID.MatchString(projectID) {
		s.problem(w, 400, "invalid_project_id", "Project ID is invalid")
		return
	}
	rootIDs := r.URL.Query()["rootId"]
	if len(rootIDs) > 200 {
		s.problem(w, 400, "invalid_root_ids", "Too many root session IDs")
		return
	}
	for _, rootID := range rootIDs {
		if !opaqueID.MatchString(rootID) {
			s.problem(w, 400, "invalid_root_id", "Root session ID is invalid")
			return
		}
	}
	offset := 0
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		if len(cursor) > 512 {
			s.problem(w, 400, "invalid_cursor", "Cursor is invalid")
			return
		}
		offset, err = strconv.Atoi(cursor)
		if err != nil || offset < 0 {
			s.problem(w, 400, "invalid_cursor", "Cursor is invalid")
			return
		}
	}
	store, err := storage.Open(filepath.Join(s.layout.Root, "inspector.db"))
	if err != nil {
		s.problem(w, 500, "index_unavailable", "Index unavailable")
		return
	}
	defer store.Close()
	repository := inspector.Repository{Store: store}
	page, err := repository.Sessions(r.Context(), revision, query, projectID, rootIDs, offset, limit)
	if err != nil {
		if err.Error() == "revision_unavailable" {
			s.problem(w, 409, "revision_unavailable", "Revision is unavailable")
		} else {
			s.problem(w, 500, "query_failed", "Session query failed")
		}
		return
	}
	response := map[string]any{"schemaVersion": page.SchemaVersion, "datasetEpoch": page.DatasetEpoch, "appliedRevision": page.AppliedRevision, "coverage": page.Coverage, "items": page.Items}
	if page.NextCursor != "" {
		response["nextCursor"] = page.NextCursor
	}
	s.write(w, response)
}

func (s *state) sessionMap(w http.ResponseWriter, r *http.Request) {
	for key := range r.URL.Query() {
		if key != "revision" {
			s.problem(w, 400, "invalid_query_parameter", "Query parameter is invalid")
			return
		}
	}
	sessionID := r.PathValue("sessionId")
	if !opaqueID.MatchString(sessionID) {
		s.problem(w, 400, "invalid_session_id", "Session ID is invalid")
		return
	}
	revision, err := storage.ParseRevision(r.URL.Query().Get("revision"))
	if err != nil {
		s.problem(w, 400, "invalid_revision", "Revision is invalid")
		return
	}
	store, err := storage.Open(filepath.Join(s.layout.Root, "inspector.db"))
	if err != nil {
		s.problem(w, 500, "index_unavailable", "Index unavailable")
		return
	}
	defer store.Close()
	result, err := (inspector.Repository{Store: store}).Map(r.Context(), revision, sessionID)
	if err != nil {
		if err.Error() == "revision_unavailable" {
			s.problem(w, 409, "revision_unavailable", "Revision is unavailable")
		} else if errors.Is(err, sql.ErrNoRows) {
			s.problem(w, 404, "session_not_found", "Session was not found at this revision")
		} else {
			s.problem(w, 500, "query_failed", "Session map query failed")
		}
		return
	}
	s.write(w, result)
}

func (s *state) turnLedger(w http.ResponseWriter, r *http.Request) {
	allowed := map[string]bool{"revision": true, "pageSize": true, "cursor": true}
	for key := range r.URL.Query() {
		if !allowed[key] {
			s.problem(w, 400, "invalid_query_parameter", "Query parameter is invalid")
			return
		}
	}
	sessionID, turnID := r.PathValue("sessionId"), r.PathValue("turnId")
	if !opaqueID.MatchString(sessionID) || !opaqueID.MatchString(turnID) {
		s.problem(w, 400, "invalid_inspector_id", "Session or turn ID is invalid")
		return
	}
	revision, err := storage.ParseRevision(r.URL.Query().Get("revision"))
	if err != nil {
		s.problem(w, 400, "invalid_revision", "Revision is invalid")
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("pageSize"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 200 {
			s.problem(w, 400, "invalid_page_size", "Page size is invalid")
			return
		}
	}
	offset := 0
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		if len(raw) > 512 {
			s.problem(w, 400, "invalid_cursor", "Cursor is invalid")
			return
		}
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			s.problem(w, 400, "invalid_cursor", "Cursor is invalid")
			return
		}
	}
	store, err := storage.Open(filepath.Join(s.layout.Root, "inspector.db"))
	if err != nil {
		s.problem(w, 500, "index_unavailable", "Index unavailable")
		return
	}
	defer store.Close()
	page, err := (inspector.Repository{Store: store}).Ledger(r.Context(), revision, sessionID, turnID, offset, limit)
	if err != nil {
		if err.Error() == "revision_unavailable" {
			s.problem(w, 409, "revision_unavailable", "Revision is unavailable")
		} else if errors.Is(err, sql.ErrNoRows) {
			s.problem(w, 404, "turn_not_found", "Turn was not found at this revision")
		} else {
			s.problem(w, 500, "query_failed", "Turn ledger query failed")
		}
		return
	}
	s.write(w, page)
}

func (s *state) recordedContext(w http.ResponseWriter, r *http.Request) {
	for key := range r.URL.Query() {
		if key != "revision" {
			s.problem(w, 400, "invalid_query_parameter", "Query parameter is invalid")
			return
		}
	}
	id := r.PathValue("evidenceId")
	if !opaqueID.MatchString(id) {
		s.problem(w, 400, "invalid_evidence_id", "Evidence ID is invalid")
		return
	}
	revision, err := storage.ParseRevision(r.URL.Query().Get("revision"))
	if err != nil {
		s.problem(w, 400, "invalid_revision", "Revision is invalid")
		return
	}
	store, err := storage.Open(filepath.Join(s.layout.Root, "inspector.db"))
	if err != nil {
		s.problem(w, 500, "index_unavailable", "Index unavailable")
		return
	}
	defer store.Close()
	chunk, err := evidence.ResolveAt(r.Context(), store, s.codexHome, id, revision, 0, 1)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.problem(w, 404, "evidence_not_found", "Evidence not found")
		} else if err.Error() == "revision_unavailable" {
			s.problem(w, 409, "revision_unavailable", "Revision is unavailable")
		} else {
			s.problem(w, 400, "context_unavailable", "Recorded context request failed")
		}
		return
	}
	kind, err := (inspector.Repository{Store: store}).EvidenceKind(r.Context(), chunk.AppliedRevision, id)
	if err != nil {
		s.problem(w, 404, "evidence_not_found", "Evidence not found")
		return
	}
	fidelity, reason := "unavailable", "Complete model input was not recorded by the supported source; unavailable in demo."
	blockKind := "context_event"
	if kind == "model_input" {
		fidelity, reason, blockKind = "exact", "", "model_input"
	} else if kind == "compacted" || kind == "context_compacted" {
		fidelity, reason, blockKind = "exact", "Exact recorded compaction evidence; before/after context is not reconstructed.", "compaction"
	}
	if chunk.Availability != "available" {
		fidelity = "unavailable"
		reason = "The pinned source evidence is currently " + strings.ReplaceAll(chunk.Availability, "_", " ") + "."
	}
	block := map[string]any{"evidenceId": id, "kind": blockKind, "locator": chunk.Locator, "sourcePrefixSha256": chunk.SourcePrefixSHA256, "eventFingerprint": chunk.EventFingerprint, "availability": chunk.Availability, "availabilityObservedAt": chunk.AvailabilityObservedAt, "availabilityRevision": chunk.AvailabilityRevision}
	if reason != "" {
		block["reason"] = reason
	}
	coverage := map[string]any{"fidelity": fidelity, "observed": 0, "eligible": 1, "reason": reason}
	if fidelity == "exact" {
		coverage = map[string]any{"fidelity": "exact", "observed": 1, "eligible": 1}
	}
	s.write(w, map[string]any{"schemaVersion": version.IndexSchema, "datasetEpoch": chunk.DatasetEpoch, "appliedRevision": chunk.AppliedRevision, "coverage": coverage, "evidenceId": id, "fidelity": fidelity, "reason": reason, "blocks": []any{block}})
}

func (s *state) reviewPlan(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64*1024)
	var request reviews.PlanRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil {
		s.problem(w, 400, "invalid_review_plan", "Review plan request is invalid")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		s.problem(w, 400, "invalid_review_plan", "Review plan request must contain one object")
		return
	}
	store, err := storage.Open(filepath.Join(s.layout.Root, "inspector.db"))
	if err != nil {
		s.problem(w, 500, "index_unavailable", "Index unavailable")
		return
	}
	defer store.Close()
	plan, err := s.reviewManager.Plan(r.Context(), store, request)
	if err != nil {
		switch {
		case errors.Is(err, reviews.ErrInvalidRequest):
			s.problem(w, 400, "invalid_review_plan", "Review plan request is invalid")
		case errors.Is(err, reviews.ErrRevisionUnavailable):
			s.problem(w, 409, "revision_unavailable", "Revision is unavailable")
		case errors.Is(err, reviews.ErrScopeEmpty):
			s.problem(w, 409, "review_scope_empty", "No eligible completed turns exist in this scope")
		case errors.Is(err, reviews.ErrScopeTooLarge):
			s.problem(w, 413, "review_scope_too_large", "Review scope exceeds the frozen manifest bounds")
		default:
			s.problem(w, 500, "review_plan_failed", "Review plan could not be created")
		}
		return
	}
	if !s.writeBounded(w, plan, 2*1024*1024) {
		s.problem(w, 413, "review_plan_too_large", "Review plan exceeds the bounded response")
	}
}

func (s *state) launchReview(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var request struct {
		PlanID    string `json:"planId"`
		Confirmed bool   `json:"confirmed"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || !request.Confirmed || !opaqueID.MatchString(request.PlanID) {
		s.problem(w, 400, "review_confirmation_required", "Explicit Review confirmation is required")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		s.problem(w, 400, "invalid_review_launch", "Review launch request must contain one object")
		return
	}
	summary, err := s.reviewManager.Launch(s.ctx, request.PlanID, request.Confirmed)
	if err != nil {
		if errors.Is(err, reviews.ErrConfirmation) {
			s.problem(w, 403, "review_confirmation_required", "Explicit Review confirmation is required")
		} else if errors.Is(err, reviews.ErrPlanUnavailable) {
			s.problem(w, 409, "review_plan_unavailable", "Review plan is unavailable or already launched")
		} else {
			s.problem(w, 500, "review_launch_failed", "Review launch could not be prepared")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(summary)
}

func (s *state) reviews(w http.ResponseWriter, r *http.Request) {
	for key := range r.URL.Query() {
		if key != "cursor" && key != "pageSize" {
			s.problem(w, 400, "invalid_query_parameter", "Query parameter is invalid")
			return
		}
	}
	limit, offset := 50, 0
	var err error
	if raw := r.URL.Query().Get("pageSize"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 200 {
			s.problem(w, 400, "invalid_page_size", "Page size is invalid")
			return
		}
	}
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 || len(raw) > 512 {
			s.problem(w, 400, "invalid_cursor", "Cursor is invalid")
			return
		}
	}
	items, next, err := s.reviewManager.List(offset, limit)
	if err != nil {
		s.problem(w, 500, "review_history_failed", "Review history is unavailable")
		return
	}
	response := map[string]any{"items": items}
	if next != "" {
		response["nextCursor"] = next
	}
	s.write(w, response)
}

func (s *state) reviewDetail(w http.ResponseWriter, r *http.Request) {
	if len(r.URL.Query()) != 0 {
		s.problem(w, 400, "invalid_query_parameter", "Query parameter is invalid")
		return
	}
	id := r.PathValue("reviewId")
	if !opaqueID.MatchString(id) {
		s.problem(w, 400, "invalid_review_id", "Review ID is invalid")
		return
	}
	detail, err := s.reviewManager.Detail(r.Context(), id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.problem(w, 404, "review_not_found", "Review was not found")
		} else {
			s.problem(w, 500, "review_read_failed", "Review could not be read")
		}
		return
	}
	if !s.writeBounded(w, detail, 2*1024*1024) {
		s.problem(w, 413, "review_response_too_large", "Review response exceeds the bounded response")
	}
}

func (s *state) evidence(w http.ResponseWriter, r *http.Request) {
	allowed := map[string]bool{"revision": true, "offset": true, "limit": true}
	for key := range r.URL.Query() {
		if !allowed[key] {
			s.problem(w, 400, "invalid_query_parameter", "Query parameter is invalid")
			return
		}
	}
	id := r.PathValue("evidenceId")
	if !opaqueID.MatchString(id) {
		s.problem(w, 400, "invalid_evidence_id", "Evidence ID is invalid")
		return
	}
	revision, err := storage.ParseRevision(r.URL.Query().Get("revision"))
	if err != nil {
		s.problem(w, 400, "invalid_revision", "Revision is invalid")
		return
	}
	offset := int64(0)
	limit := evidence.DefaultLimit
	if raw := r.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			s.problem(w, 400, "invalid_range", "Evidence range is invalid")
			return
		}
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			s.problem(w, 400, "invalid_range", "Evidence range is invalid")
			return
		}
	}
	store, err := storage.Open(filepath.Join(s.layout.Root, "inspector.db"))
	if err != nil {
		s.problem(w, 500, "index_unavailable", "Index unavailable")
		return
	}
	defer store.Close()
	chunk, err := evidence.ResolveAt(r.Context(), store, s.codexHome, id, revision, offset, limit)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.problem(w, 404, "evidence_not_found", "Evidence not found")
		} else if err.Error() == "revision_unavailable" {
			s.problem(w, 409, "revision_unavailable", "Revision is unavailable")
		} else {
			s.problem(w, 400, "evidence_unavailable", "Evidence request failed")
		}
		return
	}
	s.write(w, chunk)
}
func (s *state) status(w http.ResponseWriter, r *http.Request) {
	entries, _ := os.ReadDir(s.layout.Queue)
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	hookState := "idle"
	var lastMarker any
	diagnostics := append([]compat.Diagnostic{}, s.compat.HookDiagnostics...)
	invalidCount := 0
	var invalidObserved time.Time
	recordInvalid := func(path string) {
		invalidCount++
		if info, statErr := os.Stat(path); statErr == nil && info.ModTime().After(invalidObserved) {
			invalidObserved = info.ModTime()
		}
	}
	for i := len(names) - 1; i >= 0; i-- {
		path := filepath.Join(s.layout.Queue, names[i])
		b, err := os.ReadFile(path)
		if err != nil || len(b) > 4096 {
			recordInvalid(path)
			continue
		}
		marker, markerErr := hook.DecodeMarker(b)
		if markerErr != nil {
			recordInvalid(path)
			continue
		}
		if lastMarker == nil {
			observed, _ := time.Parse(time.RFC3339Nano, marker.ObservedAt)
			age := int64(0)
			if time.Since(observed) > 0 {
				age = int64(time.Since(observed).Seconds())
			}
			value := map[string]any{"eventKind": marker.EventKind, "sessionId": marker.SessionID, "observedAt": marker.ObservedAt, "ageSeconds": age}
			if marker.TurnID != "" {
				value["turnId"] = marker.TurnID
			}
			lastMarker = value
		}
	}
	if invalidCount > 0 {
		if invalidObserved.IsZero() {
			invalidObserved = s.meta.StartedAt
		}
		diagnostics = append(diagnostics, compat.Diagnostic{Code: "invalid_payload", Severity: "warning", Count: invalidCount, LastObservedAt: invalidObserved.UTC().Format(time.RFC3339Nano)})
	}
	sort.Slice(diagnostics, func(i, j int) bool { return diagnostics[i].Code < diagnostics[j].Code })
	if len(diagnostics) > 50 {
		diagnostics = diagnostics[:50]
	}
	if lastMarker != nil {
		hookState = "healthy"
	}
	processState := "ready"
	indexState := "empty"
	var lastError any
	for _, c := range s.compat.Checks {
		if c.Status != "ok" {
			processState = "degraded"
		}
		if c.Name == "source_format" && c.Status != "ok" {
			indexState = "failed"
			lastError = "source_format_incompatible"
		}
	}
	if len(diagnostics) > 0 {
		hookState = "degraded"
		processState = "degraded"
	}
	s.mu.Lock()
	indexing, progress, indexErr := s.indexing, s.indexProgress, s.indexError
	s.mu.Unlock()
	epoch := "phase2-empty"
	revision := int64(1)
	dbStatus := storage.Status{}
	diagnosticGroups := []map[string]any{}
	if store, openErr := storage.Open(filepath.Join(s.layout.Root, "inspector.db")); openErr == nil {
		dbStatus, _ = store.Status()
		if groups, groupErr := store.SourceDiagnosticGroups(r.Context()); groupErr == nil {
			for _, group := range groups {
				diagnosticGroups = append(diagnosticGroups, map[string]any{"state": group.State, "reason": group.Reason, "detectedVersion": group.DetectedVersion, "count": group.Count, "remediation": sourceRemediation(group.State, group.Reason)})
			}
		} else {
			diagnosticGroups = append(diagnosticGroups, map[string]any{"state": "failed", "reason": "diagnostic_query_failed", "detectedVersion": "unknown", "count": 1, "remediation": sourceRemediation("failed", "diagnostic_query_failed")})
			processState = "degraded"
			lastError = "source_diagnostics_unavailable"
		}
		_ = store.Close()
		if dbStatus.Epoch != "" {
			epoch, revision = dbStatus.Epoch, dbStatus.Revision
		}
	}
	if indexing {
		indexState = "building"
		if dbStatus.Sources > 0 {
			indexState = "catching_up"
		}
	} else if dbStatus.RequiresRebuild > 0 {
		indexState = "requires_rebuild"
	} else if dbStatus.Sources > 0 {
		indexState = "current"
	}
	if indexErr != "" {
		indexState = "failed"
		lastError = indexErr
	}
	remaining := max(0, progress.Inventoried-progress.Processed-progress.Skipped-progress.Failed-progress.RequiresRebuild)
	index := map[string]any{"state": indexState, "datasetEpoch": epoch, "appliedRevision": revision, "schemaVersion": version.IndexSchema, "databaseBytes": dbStatus.DatabaseBytes, "sourceCount": dbStatus.Sources, "supportedSourceCount": dbStatus.Supported, "unsupportedSourceCount": dbStatus.Unsupported, "pendingTailCount": dbStatus.Pending, "queuedSessionChanges": len(names), "inventoriedCount": max(dbStatus.Sources, progress.Inventoried), "processedCount": max(dbStatus.Processed, progress.Processed), "remainingCount": remaining, "queuedCount": max(dbStatus.Pending, remaining), "skippedCount": max(dbStatus.Unsupported, progress.Skipped), "failedCount": dbStatus.Failed + progress.Failed, "requiresRebuildCount": dbStatus.RequiresRebuild, "diagnosticGroups": diagnosticGroups, "reverseScanBoundary": nil, "completedWatermark": dbStatus.Watermark}
	if progress.Boundary != "" {
		index["reverseScanBoundary"] = progress.Boundary
	}
	if lastError != nil {
		index["lastErrorCode"] = lastError
	}
	coverage := map[string]any{"fidelity": "unavailable", "observed": 0, "eligible": dbStatus.Sources, "reason": "no supported sources indexed"}
	if dbStatus.Supported > 0 {
		coverage = map[string]any{"fidelity": "exact", "observed": dbStatus.Processed, "eligible": dbStatus.Sources}
	}
	s.write(w, map[string]any{"schemaVersion": version.IndexSchema, "datasetEpoch": epoch, "appliedRevision": revision, "coverage": coverage, "sourceHome": map[string]any{"path": s.codexHome, "resolution": s.codexHomeSource}, "process": map[string]any{"state": processState, "inspectorVersion": version.CLI, "cliVersion": version.CLI, "cliCompatibility": s.compat.CLICompatibility, "pluginVersion": s.compat.PluginVersion, "pluginProtocolVersion": s.compat.PluginProtocol, "pid": os.Getpid(), "startedAt": s.meta.StartedAt.Format(time.RFC3339Nano)}, "index": index, "hook": map[string]any{"state": hookState, "registeredEvents": []string{"SessionStart", "UserPromptSubmit", "PreCompact", "PostCompact", "SubagentStart", "SubagentStop", "Stop"}, "lastMarker": lastMarker, "diagnostics": diagnostics}})
}
func sourceRemediation(state, reason string) string {
	switch reason {
	case "diagnostic_query_failed":
		return "Run codex-inspector doctor, then restart Inspector. Source issue groups could not be read safely."
	case "additional_diagnostic_groups":
		return "Additional source issue groups were combined to keep diagnostics bounded. Update Inspector and sync again before reporting the aggregate."
	case "unclassified_source_state":
		return "Inspector withheld an unrecognized diagnostic value. Update Inspector and sync again; report only this reason code if it remains."
	case "indexed_prefix_changed_or_shrank":
		return "The recorded source changed before its indexed boundary. Run codex-inspector sync to rebuild derived data from the current local source."
	case "incompatible_record_envelope", "missing_leading_session_meta", "invalid_session_meta":
		return "Keep the source intact, update Inspector, and sync again; this record shape is not currently supported."
	case "parse_failed", "normalization_failed":
		return "Run codex-inspector sync again. If this group remains, run codex-inspector doctor and report the reason and detected version."
	case "rebuild_failed":
		return "Run codex-inspector doctor, resolve the reported data-home issue, then retry codex-inspector sync."
	}
	if state == "requires_rebuild" {
		return "Run codex-inspector sync to rebuild derived data from the unchanged local sources."
	}
	return "Update Inspector and retry codex-inspector sync; if the group remains, include this reason and version in the bug report."
}
func (s *state) write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func (s *state) writeBounded(w http.ResponseWriter, v any, limit int) bool {
	b, err := json.Marshal(v)
	if err != nil || len(b)+1 > limit {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(append(b, '\n'))
	return true
}
func (s *state) problem(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"code": code, "title": title, "status": status, "retryable": false})
}

func ReadBodyLimit(r io.Reader) ([]byte, error) { return io.ReadAll(io.LimitReader(r, 2*1024*1024+1)) }
