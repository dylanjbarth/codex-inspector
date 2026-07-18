package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
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
	"strings"
	"sync"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/compat"
	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/hook"
	proc "github.com/dylanjbarth/codex-inspector/internal/process"
	"github.com/dylanjbarth/codex-inspector/internal/version"
)

//go:embed assets/* assets/assets/*
var assets embed.FS

type Config struct {
	Layout        home.Layout
	IdleTimeout   time.Duration
	Ready         chan<- proc.Metadata
	Compatibility *compat.Snapshot
}
type state struct {
	mu           sync.Mutex
	meta         proc.Metadata
	cookie       string
	exchanged    bool
	lastActive   time.Time
	host, origin string
	layout       home.Layout
	compat       compat.Snapshot
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
	s := &state{meta: m, cookie: cookie, lastActive: time.Now(), host: fmt.Sprintf("127.0.0.1:%d", addr.Port), origin: fmt.Sprintf("http://127.0.0.1:%d", addr.Port), layout: c.Layout, compat: snapshot}
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
			s.mu.Lock()
			idle := time.Since(s.lastActive) >= c.IdleTimeout
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
	s.write(w, map[string]any{"healthy": true, "instanceId": s.meta.InstanceID, "protocolVersion": version.Protocol, "cliVersion": version.CLI})
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(202)
	json.NewEncoder(w).Encode(map[string]any{"syncId": "phase1-empty", "state": "coalesced"})
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
	index := map[string]any{"state": indexState, "datasetEpoch": nil, "appliedRevision": 0, "schemaVersion": version.IndexSchema, "databaseBytes": 0, "sourceCount": 0, "supportedSourceCount": 0, "unsupportedSourceCount": 0, "pendingTailCount": 0, "queuedSessionChanges": len(names), "processedCount": 0, "queuedCount": len(names), "skippedCount": 0, "failedCount": 0, "requiresRebuildCount": 0, "reverseScanBoundary": nil, "completedWatermark": nil}
	if lastError != nil {
		index["lastErrorCode"] = lastError
	}
	s.write(w, map[string]any{"datasetEpoch": "phase1-empty", "appliedRevision": 1, "coverage": map[string]any{"fidelity": "unavailable", "observed": 0, "eligible": 0, "reason": "indexing begins in Phase 2"}, "process": map[string]any{"state": processState, "inspectorVersion": version.CLI, "cliVersion": version.CLI, "cliCompatibility": s.compat.CLICompatibility, "pluginVersion": s.compat.PluginVersion, "pluginProtocolVersion": s.compat.PluginProtocol, "pid": os.Getpid(), "startedAt": s.meta.StartedAt.Format(time.RFC3339Nano)}, "index": index, "hook": map[string]any{"state": hookState, "registeredEvents": []string{"SessionStart", "UserPromptSubmit", "PreCompact", "PostCompact", "SubagentStart", "SubagentStop", "Stop"}, "lastMarker": lastMarker, "diagnostics": diagnostics}})
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
