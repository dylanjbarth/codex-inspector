package reviews

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/evidence"
	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
	reviewschemas "github.com/dylanjbarth/codex-inspector/schemas/reviews"
	"github.com/santhosh-tekuri/jsonschema/v5"
	_ "modernc.org/sqlite"
)

var (
	ErrInvalidRequest      = errors.New("invalid_review_request")
	ErrRevisionUnavailable = errors.New("revision_unavailable")
	ErrScopeEmpty          = errors.New("review_scope_empty")
	ErrScopeTooLarge       = errors.New("review_scope_too_large")
	ErrPlanUnavailable     = errors.New("review_plan_unavailable")
	ErrConfirmation        = errors.New("review_confirmation_required")
	ErrManifestContract    = errors.New("review_manifest_contract")
)

type Manager struct {
	layout         home.Layout
	codex          string
	codexHome      string
	now            func() time.Time
	mu             sync.Mutex
	plans          map[string]Plan
	active         map[string]bool
	notify         func(string, string)
	reportSchema   *jsonschema.Schema
	manifestSchema *jsonschema.Schema
	runSchema      *jsonschema.Schema
}

type manifestContractError struct{ locations []string }

func (e *manifestContractError) Error() string { return ErrManifestContract.Error() }
func (e *manifestContractError) Unwrap() error { return ErrManifestContract }
func ManifestContractLocations(err error) []string {
	var target *manifestContractError
	if errors.As(err, &target) {
		return append([]string(nil), target.locations...)
	}
	return nil
}
func validationLocations(err error) []string {
	ve, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return []string{"unknown"}
	}
	out := []string{}
	var walk func(*jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(out) >= 10 {
			return
		}
		if len(e.Causes) == 0 {
			out = append(out, e.InstanceLocation+"|"+e.KeywordLocation)
			return
		}
		for _, cause := range e.Causes {
			walk(cause)
		}
	}
	walk(ve)
	return out
}

func New(layout home.Layout, codex, codexHome string, notify func(string, string)) (*Manager, error) {
	if codex == "" {
		codex = "codex"
	}
	manifest, err := compileSchema("manifest.schema.json")
	if err != nil {
		return nil, err
	}
	report, err := compileSchema("report.schema.json")
	if err != nil {
		return nil, err
	}
	run, err := compileSchema("run.schema.json")
	if err != nil {
		return nil, err
	}
	m := &Manager{layout: layout, codex: codex, codexHome: codexHome, now: time.Now, plans: map[string]Plan{}, active: map[string]bool{}, notify: notify, manifestSchema: manifest, reportSchema: report, runSchema: run}
	if err = m.ensureAcceptanceStore(); err != nil {
		return nil, err
	}
	return m, nil
}

func compileSchema(name string) (*jsonschema.Schema, error) {
	b, err := reviewschemas.Files.ReadFile(name)
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.Draft = jsonschema.Draft2020
	if err = c.AddResource(name, bytes.NewReader(b)); err != nil {
		return nil, err
	}
	return c.Compile(name)
}

func strictJSON(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("trailing JSON value")
	}
	return nil
}

func randomID(prefix string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + ":" + hex.EncodeToString(b), nil
}

func (m *Manager) Plan(ctx context.Context, store *storage.Store, req PlanRequest) (Plan, error) {
	if err := validatePlanRequest(req); err != nil {
		return Plan{}, err
	}
	epoch, latest, err := store.Snapshot()
	if err != nil {
		return Plan{}, err
	}
	revision := req.RequestedRevision
	if revision == 0 {
		revision = latest
	}
	var exists int
	if revision < 1 || revision > latest || store.DB().QueryRowContext(ctx, `SELECT count(*) FROM index_revisions WHERE epoch_id=? AND revision=?`, epoch, revision).Scan(&exists) != nil || exists != 1 {
		return Plan{}, ErrRevisionUnavailable
	}
	if req.Scope.Kind == "time_period" && req.Scope.ProjectID != "" {
		if err = store.DB().QueryRowContext(ctx, `SELECT count(*) FROM projects WHERE epoch_id=? AND id=? AND created_revision<=?`, epoch, req.Scope.ProjectID, revision).Scan(&exists); err != nil {
			return Plan{}, err
		}
		if exists != 1 {
			return Plan{}, ErrInvalidRequest
		}
	}
	reviewID, err := randomID("review")
	if err != nil {
		return Plan{}, err
	}
	manifest, counts, coverage, projects, estimate, err := buildManifest(ctx, store.DB(), epoch, revision, reviewID, req, m.now().UTC())
	if err != nil {
		return Plan{}, err
	}
	coverageSessions := []string(nil)
	if req.Scope.Kind == "single_session" {
		coverageSessions = manifest.IncludedSessionIDs
	}
	planCoverage, inventoryGaps, err := sourceInventoryCoverage(ctx, store, epoch, revision, coverageSessions)
	if err != nil {
		return Plan{}, err
	}
	manifest.CoverageGaps = append(manifest.CoverageGaps, inventoryGaps...)
	var doc any
	b, _ := json.Marshal(manifest)
	_ = json.Unmarshal(b, &doc)
	if err = m.manifestSchema.Validate(doc); err != nil {
		return Plan{}, &manifestContractError{locations: validationLocations(err)}
	}
	planID, err := randomID("plan")
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{SchemaVersion: 2, DatasetEpoch: epoch, AppliedRevision: revision, Coverage: planCoverage, PlanID: planID, ManifestPreview: manifest, LaunchPrompt: LaunchPrompt, EstimatedInputTokens: estimate, SourceByteCounts: counts, IndexedTimeCoverage: coverage, ProjectSummary: projects}
	m.mu.Lock()
	m.plans[planID] = plan
	m.mu.Unlock()
	return plan, nil
}

func validatePlanRequest(r PlanRequest) error {
	if len(r.Model) == 0 || len(r.Model) > 200 || len(r.ReasoningEffort) == 0 || len(r.ReasoningEffort) > 200 || len(r.Focus) > 2000 {
		return ErrInvalidRequest
	}
	switch r.Scope.Kind {
	case "single_session":
		if r.Scope.RootSessionID == "" || r.Scope.Start != "" || r.Scope.End != "" || r.Scope.Timezone != "" || r.Scope.ProjectID != "" {
			return ErrInvalidRequest
		}
	case "time_period":
		start, e1 := time.Parse(time.RFC3339, r.Scope.Start)
		end, e2 := time.Parse(time.RFC3339, r.Scope.End)
		if e1 != nil || e2 != nil || !start.Before(end) || end.Sub(start) > 366*24*time.Hour || r.Scope.Timezone == "" {
			return ErrInvalidRequest
		}
		if _, err := time.LoadLocation(r.Scope.Timezone); err != nil {
			return ErrInvalidRequest
		}
	default:
		return ErrInvalidRequest
	}
	return nil
}

func (m *Manager) plan(id string) (Plan, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.plans[id]
	return p, ok
}

func (m *Manager) Active() bool { m.mu.Lock(); defer m.mu.Unlock(); return len(m.active) > 0 }

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".review-write-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() { _ = f.Close(); _ = os.Remove(tmp) }
	if err = f.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err = f.Write(data); err != nil {
		cleanup()
		return err
	}
	if err = f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err = f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	d, err := os.Open(dir)
	if err == nil {
		err = d.Sync()
		_ = d.Close()
	}
	return err
}

func (m *Manager) writeRun(dir string, run Run) error {
	compact, err := json.Marshal(run)
	if err != nil {
		return err
	}
	var document any
	_ = json.Unmarshal(compact, &document)
	if err = m.runSchema.Validate(document); err != nil {
		return fmt.Errorf("run violates frozen schema: %w", err)
	}
	b, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, "run.json"), append(b, '\n'), 0600)
}

func (m *Manager) List(offset, limit int) ([]Summary, string, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	entries, err := os.ReadDir(m.layout.Reviews)
	if err != nil {
		return nil, "", err
	}
	type item struct {
		summary Summary
		stamp   time.Time
	}
	items := []item{}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		run, err := readRun(filepath.Join(m.layout.Reviews, e.Name(), "run.json"))
		if err != nil {
			continue
		}
		created, _ := time.Parse(time.RFC3339Nano, run.CreatedAt)
		items = append(items, item{Summary{run.ReviewID, run.Status, run.CreatedAt, run.ThreadID}, created})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].stamp.Equal(items[j].stamp) {
			return items[i].summary.ReviewID > items[j].summary.ReviewID
		}
		return items[i].stamp.After(items[j].stamp)
	})
	if offset > len(items) {
		offset = len(items)
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	out := make([]Summary, 0, end-offset)
	for _, x := range items[offset:end] {
		out = append(out, x.summary)
	}
	next := ""
	if end < len(items) {
		next = fmt.Sprint(end)
	}
	return out, next, nil
}

func readRun(path string) (Run, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Run{}, err
	}
	var run Run
	if err = strictJSON(b, &run); err != nil {
		return Run{}, err
	}
	return run, nil
}
func readManifest(path string) (Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, err
	}
	var v Manifest
	if err = strictJSON(b, &v); err != nil {
		return Manifest{}, err
	}
	return v, nil
}

func (m *Manager) Detail(ctx context.Context, reviewID string) (Detail, error) {
	if filepath.Base(reviewID) != reviewID || strings.HasPrefix(reviewID, ".") {
		return Detail{}, os.ErrNotExist
	}
	dir := filepath.Join(m.layout.Reviews, reviewID)
	manifest, err := readManifest(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return Detail{}, err
	}
	manifestBytes, _ := json.Marshal(manifest)
	var manifestDocument any
	_ = json.Unmarshal(manifestBytes, &manifestDocument)
	if err = m.manifestSchema.Validate(manifestDocument); err != nil {
		return Detail{}, err
	}
	run, err := readRun(filepath.Join(dir, "run.json"))
	if err != nil {
		return Detail{}, err
	}
	if run.Diagnostics == nil {
		run.Diagnostics = []string{}
	}
	detail := Detail{Summary: Summary{run.ReviewID, run.Status, run.CreatedAt, run.ThreadID}, Manifest: manifest, Run: run, ReportState: "absent", AcceptedReport: nil}
	accepted, hash, err := m.loadAccepted(reviewID)
	if err == nil {
		live := map[string]ManifestEvidence{}
		if store, openErr := storage.Open(filepath.Join(m.layout.Root, "inspector.db")); openErr == nil {
			defer store.Close()
			for _, finding := range accepted.Findings {
				for _, id := range finding.Citations {
					if _, seen := live[id]; seen {
						continue
					}
					for _, entry := range manifest.Evidence {
						if entry.EvidenceID != id {
							continue
						}
						chunk, resolveErr := evidence.ResolveAt(ctx, store, m.codexHome, id, manifest.IndexRevision, 0, 1)
						if resolveErr == nil {
							entry.Availability = chunk.Availability
							entry.AvailabilityObservedAt = chunk.AvailabilityObservedAt
							entry.AvailabilityRevision = chunk.AvailabilityRevision
						}
						live[id] = entry
						break
					}
				}
			}
		}
		rendered := renderReport(accepted, manifest, live)
		detail.AcceptedReport = &AcceptedReport{Report: accepted, FindingsRendered: rendered, ContentSHA256: hash}
		detail.ReportState = "accepted"
		return detail, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Detail{}, err
	}
	if _, statErr := os.Stat(filepath.Join(dir, "review.json")); statErr == nil {
		if run.Status == "running" || run.Status == "planned" {
			detail.ReportState = "pending"
		} else if run.Status == "unrenderable" {
			detail.ReportState = "unrenderable"
		} else {
			detail.ReportState = "invalid"
		}
	} else if run.Status == "running" || run.Status == "planned" {
		detail.ReportState = "pending"
	}
	return detail, nil
}

func renderReport(r Report, m Manifest, live map[string]ManifestEvidence) []RenderFinding {
	lookup := map[string]ManifestEvidence{}
	for _, e := range m.Evidence {
		lookup[e.EvidenceID] = e
	}
	for id, e := range live {
		lookup[id] = e
	}
	out := make([]RenderFinding, 0, len(r.Findings))
	for _, f := range r.Findings {
		rf := RenderFinding{f.FindingID, f.Kind, f.Lens, f.Title, f.Observation, f.Impact, f.Support, f.EvidenceSummary, f.Recommendation, f.ActionPrompt, []CitationState{}}
		for _, id := range f.Citations {
			e := lookup[id]
			rf.Citations = append(rf.Citations, CitationState{e.EvidenceID, e.SourcePrefixSHA256, e.EventFingerprint, e.Availability, e.AvailabilityObservedAt, e.AvailabilityRevision})
		}
		out = append(out, rf)
	}
	return out
}

func (m *Manager) ensureAcceptanceStore() error {
	db, err := m.openAcceptance()
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS accepted_reports(review_id TEXT PRIMARY KEY, accepted_at TEXT NOT NULL, sha256 TEXT NOT NULL, report_json BLOB NOT NULL)`)
	return err
}
func (m *Manager) openAcceptance() (*sql.DB, error) {
	path := filepath.Join(m.layout.Reviews, ".accepted.sqlite")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	return db, err
}
func (m *Manager) accept(reviewID string, b []byte, hash string) (bool, error) {
	db, err := m.openAcceptance()
	if err != nil {
		return false, err
	}
	defer db.Close()
	res, err := db.Exec(`INSERT OR IGNORE INTO accepted_reports(review_id,accepted_at,sha256,report_json) VALUES(?,?,?,?)`, reviewID, m.now().UTC().Format(time.RFC3339Nano), hash, b)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}
func (m *Manager) loadAccepted(reviewID string) (Report, string, error) {
	db, err := m.openAcceptance()
	if err != nil {
		return Report{}, "", err
	}
	defer db.Close()
	var b []byte
	var hash string
	if err = db.QueryRow(`SELECT report_json,sha256 FROM accepted_reports WHERE review_id=?`, reviewID).Scan(&b, &hash); err != nil {
		return Report{}, "", err
	}
	var report Report
	if err = strictJSON(b, &report); err != nil {
		return Report{}, "", err
	}
	return report, hash, nil
}

func hashBytes(b []byte) string { x := sha256.Sum256(b); return hex.EncodeToString(x[:]) }
