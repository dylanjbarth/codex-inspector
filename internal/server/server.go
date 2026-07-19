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
	proc "github.com/dylanjbarth/codex-inspector/internal/process"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
	"github.com/dylanjbarth/codex-inspector/internal/version"
)

//go:embed assets/* assets/assets/*
var assets embed.FS

type Config struct {
	Layout        home.Layout
	CodexHome     string
	IdleTimeout   time.Duration
	Ready         chan<- proc.Metadata
	Compatibility *compat.Snapshot
	AutoSync      bool
}
type state struct {
	mu            sync.Mutex
	ctx           context.Context
	indexWG       sync.WaitGroup
	meta          proc.Metadata
	cookie        string
	exchanged     bool
	lastActive    time.Time
	host, origin  string
	layout        home.Layout
	codexHome     string
	compat        compat.Snapshot
	indexing      bool
	indexProgress indexer.Progress
	indexError    string
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
	m := proc.Metadata{InstanceID: id, PID: os.Getpid(), Port: addr.Port, ProtocolVersion: version.Protocol, AccessToken: access, FragmentToken: fragment, StartedAt: time.Now().UTC()}
	if err = proc.Write(c.Layout.Run, m); err != nil {
		return err
	}
	defer proc.RemoveIfInstance(c.Layout.Run, id)
	snapshot := compat.Snapshot{}
	if c.Compatibility != nil {
		snapshot = *c.Compatibility
	} else {
		snapshot = compat.Inspect(c.Layout, false)
	}
	s := &state{ctx: ctx, meta: m, cookie: cookie, lastActive: time.Now(), host: fmt.Sprintf("127.0.0.1:%d", addr.Port), origin: fmt.Sprintf("http://127.0.0.1:%d", addr.Port), layout: c.Layout, codexHome: c.CodexHome, compat: snapshot}
	defer s.indexWG.Wait()
	if c.AutoSync {
		s.startIndex()
	}
	mux := http.NewServeMux()
	s.routes(mux)
	h := s.headers(mux)
	srv := &http.Server{Handler: h, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
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
			idle := !s.indexing && time.Since(s.lastActive) >= c.IdleTimeout
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
	m.HandleFunc("GET /v1/health", s.auth(s.health))
	m.HandleFunc("POST /v1/heartbeat", s.auth(s.sameOrigin(s.heartbeat)))
	m.HandleFunc("GET /v1/status", s.auth(s.status))
	m.HandleFunc("POST /v1/sync", s.auth(s.sameOrigin(s.sync)))
	m.HandleFunc("GET /v1/sessions", s.auth(s.sessions))
	m.HandleFunc("GET /v1/evidence/{evidenceId}", s.auth(s.evidence))
}
func (s *state) validHost(r *http.Request) bool { return r.Host == s.host }
func (s *state) sameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.validHost(r) || r.Header.Get("Origin") != s.origin {
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
	if !s.validHost(r) || r.Header.Get("Origin") != s.origin {
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
	go func() {
		defer s.indexWG.Done()
		p, err := indexer.Run(s.ctx, indexer.Config{Layout: s.layout, CodexHome: s.codexHome, OnCommit: func(p indexer.Progress) { s.mu.Lock(); s.indexProgress = p; s.lastActive = time.Now(); s.mu.Unlock() }})
		s.mu.Lock()
		s.indexProgress = p
		s.indexing = false
		if err != nil {
			s.indexError = "index_failed"
		}
		s.lastActive = time.Now()
		s.mu.Unlock()
	}()
	return "running"
}

func (s *state) sessions(w http.ResponseWriter, r *http.Request) {
	allowed := map[string]bool{"revision": true, "pageSize": true, "query": true, "projectId": true, "cursor": true}
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
	epoch, applied, rows, err := store.SessionsPage(revision, query, projectID, offset, limit+1)
	if err != nil {
		if err.Error() == "revision_unavailable" {
			s.problem(w, 409, "revision_unavailable", "Revision is unavailable")
		} else {
			s.problem(w, 500, "query_failed", "Session query failed")
		}
		return
	}
	hasNext := len(rows) > limit
	if hasNext {
		rows = rows[:limit]
	}
	items := make([]map[string]any, 0, len(rows))
	for _, x := range rows {
		items = append(items, map[string]any{"sessionId": x.ID, "rootWorkUnitId": x.RootWorkUnitID, "purpose": x.Purpose, "title": x.Title, "matchCategories": []string{"metadata"}, "directTokens": nil, "descendantTokens": nil})
	}
	response := map[string]any{"schemaVersion": version.IndexSchema, "datasetEpoch": epoch, "appliedRevision": applied, "coverage": map[string]any{"fidelity": "exact", "observed": len(items), "eligible": len(items)}, "items": items}
	if hasNext {
		response["nextCursor"] = strconv.Itoa(offset + limit)
	}
	s.write(w, response)
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
	chunk, err := evidence.Resolve(r.Context(), store, id, revision, offset, limit)
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
	if store, openErr := storage.Open(filepath.Join(s.layout.Root, "inspector.db")); openErr == nil {
		dbStatus, _ = store.Status()
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
	} else if dbStatus.Pending > 0 {
		indexState = "catching_up"
	} else if dbStatus.Sources > 0 {
		indexState = "current"
	}
	if indexErr != "" {
		indexState = "failed"
		lastError = indexErr
	}
	index := map[string]any{"state": indexState, "datasetEpoch": epoch, "appliedRevision": revision, "schemaVersion": version.IndexSchema, "databaseBytes": dbStatus.DatabaseBytes, "sourceCount": dbStatus.Sources, "supportedSourceCount": dbStatus.Supported, "unsupportedSourceCount": dbStatus.Unsupported, "pendingTailCount": dbStatus.Pending, "queuedSessionChanges": len(names), "processedCount": max(dbStatus.Processed, progress.Processed), "queuedCount": max(dbStatus.Pending, max(0, progress.Inventoried-progress.Processed-progress.Skipped-progress.Failed-progress.RequiresRebuild)), "skippedCount": max(dbStatus.Unsupported, progress.Skipped), "failedCount": dbStatus.Failed + progress.Failed, "requiresRebuildCount": dbStatus.RequiresRebuild, "reverseScanBoundary": nil, "completedWatermark": dbStatus.Watermark}
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
	s.write(w, map[string]any{"schemaVersion": version.IndexSchema, "datasetEpoch": epoch, "appliedRevision": revision, "coverage": coverage, "process": map[string]any{"state": processState, "inspectorVersion": version.CLI, "cliVersion": version.CLI, "cliCompatibility": s.compat.CLICompatibility, "pluginVersion": s.compat.PluginVersion, "pluginProtocolVersion": s.compat.PluginProtocol, "pid": os.Getpid(), "startedAt": s.meta.StartedAt.Format(time.RFC3339Nano)}, "index": index, "hook": map[string]any{"state": hookState, "registeredEvents": []string{"SessionStart", "UserPromptSubmit", "PreCompact", "PostCompact", "SubagentStart", "SubagentStop", "Stop"}, "lastMarker": lastMarker, "diagnostics": diagnostics}})
}
func (s *state) write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func (s *state) problem(w http.ResponseWriter, status int, code, title string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"code": code, "title": title, "status": status, "retryable": false})
}

func ReadBodyLimit(r io.Reader) ([]byte, error) { return io.ReadAll(io.LimitReader(r, 2*1024*1024+1)) }
