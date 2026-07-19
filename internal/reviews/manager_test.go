package reviews

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

func indexedFixture(t *testing.T) (home.Layout, *storage.Store) {
	t.Helper()
	root := t.TempDir()
	layout := home.Layout{Root: filepath.Join(root, "inspector"), Reviews: filepath.Join(root, "inspector", "reviews"), Queue: filepath.Join(root, "inspector", "queue"), Run: filepath.Join(root, "inspector", "run"), Logs: filepath.Join(root, "inspector", "logs"), Cache: filepath.Join(root, "inspector", "cache")}
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
	reviewBytes, err := os.ReadFile(filepath.Join("..", "..", "fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	reviewText := strings.ReplaceAll(string(reviewBytes), "root-001", "review-1")
	reviewText = strings.ReplaceAll(reviewText, "/fake/acme", layout.Reviews)
	if err = os.WriteFile(filepath.Join(sessions, "review.jsonl"), []byte(reviewText), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := indexer.Run(context.Background(), indexer.Config{Layout: layout, CodexHome: codexHome}); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return layout, store
}

func baseRequest(root string) PlanRequest {
	return PlanRequest{Scope: Scope{Kind: "single_session", RootSessionID: root}, Model: "gpt-demo", ReasoningEffort: "high", Focus: "Make the demo clearer."}
}

func appendSourceState(t *testing.T, store *storage.Store, state string) int64 {
	t.Helper()
	epoch, revision, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var sourceID string
	if err = store.DB().QueryRow(`SELECT v.source_id FROM source_artifact_versions v JOIN source_artifacts a ON a.epoch_id=v.epoch_id AND a.id=v.source_id WHERE v.epoch_id=? AND a.source_session_id='root-001' ORDER BY v.revision DESC LIMIT 1`, epoch).Scan(&sourceID); err != nil {
		t.Fatal(err)
	}
	revision++
	tx, err := store.DB().Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES(?,?,?,'inventory')`, epoch, revision, time.Now().UTC().Format(time.RFC3339Nano)); err == nil {
		_, err = tx.Exec(`INSERT INTO source_artifact_versions(epoch_id,source_id,revision,source_kind,canonical_path,inode,byte_size,mtime_ns,detected_codex_version,adapter_version,state,state_reason,source_evidence_availability,availability_observed_at)
		 SELECT epoch_id,source_id,?,source_kind,canonical_path,inode,byte_size,mtime_ns,detected_codex_version,adapter_version,?,'synthetic inventory state',source_evidence_availability,availability_observed_at
		 FROM source_artifact_versions WHERE epoch_id=? AND source_id=? ORDER BY revision DESC LIMIT 1`, revision, state, epoch, sourceID)
	}
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return revision
}

func TestManifestScopeMarshalsNoDescendantsAsEmptyArray(t *testing.T) {
	b, err := json.Marshal(ManifestScope{Kind: "single_session", RootSessionID: "root-only", AppliedRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"descendantSessionIds":[]`) {
		t.Fatalf("single-session manifest must encode no descendants as an empty array: %s", b)
	}
}

func TestPlanFreezesCompleteSingleAndTimeScopes(t *testing.T) {
	layout, store := indexedFixture(t)
	m, err := New(layout, "/missing", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	pageRoot := ""
	if err = store.DB().QueryRow(`SELECT id FROM sessions s WHERE source_session_id='root-001'`).Scan(&pageRoot); err != nil {
		t.Fatal(err)
	}
	plan, err := m.Plan(context.Background(), store, baseRequest(pageRoot))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.ManifestPreview.IncludedSessionIDs) != 2 || len(plan.ManifestPreview.IncludedTurnIDs) != 3 || len(plan.ManifestPreview.Evidence) == 0 || len(plan.ManifestPreview.Sources) != 2 {
		t.Fatalf("incomplete plan: sessions=%d turns=%d evidence=%d sources=%d", len(plan.ManifestPreview.IncludedSessionIDs), len(plan.ManifestPreview.IncludedTurnIDs), len(plan.ManifestPreview.Evidence), len(plan.ManifestPreview.Sources))
	}
	if plan.ManifestPreview.Scope.RootSessionID != pageRoot || plan.ManifestPreview.ReportDestination != "./review.json" || !strings.Contains(plan.LaunchPrompt, "$codex-inspector:review-session") {
		t.Fatalf("contract missing: %+v", plan)
	}
	if plan.EstimatedInputTokens == nil || *plan.EstimatedInputTokens < 1 || plan.SourceByteCounts.IncludedBytes < 1 || plan.ProjectSummary.ProjectCount != 1 {
		t.Fatalf("preview missing: %+v", plan)
	}
	timeReq := PlanRequest{Scope: Scope{Kind: "time_period", Start: "2026-07-01T00:00:00Z", End: "2026-07-02T00:00:00Z", Timezone: "America/Chicago"}, Model: "gpt-demo", ReasoningEffort: "medium"}
	var projectID string
	if err = store.DB().QueryRow(`SELECT id FROM projects LIMIT 1`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	timeReq.Scope.ProjectID = projectID
	timePlan, err := m.Plan(context.Background(), store, timeReq)
	if err != nil {
		t.Fatal(err)
	}
	if len(timePlan.ManifestPreview.IncludedTurnIDs) != 3 || timePlan.ManifestPreview.Scope.Timezone != "America/Chicago" || timePlan.ManifestPreview.Scope.ProjectID != projectID || timePlan.ProjectSummary.ProjectCount != 1 || timePlan.ProjectSummary.Projects[0].ProjectID != projectID || timePlan.ProjectSummary.Projects[0].TurnCount != 3 {
		t.Fatalf("time scope=%+v", timePlan.ManifestPreview.Scope)
	}
	for _, sessionID := range timePlan.ManifestPreview.IncludedSessionIDs {
		var includedRootProject string
		err = store.DB().QueryRow(`WITH sv AS (SELECT v.* FROM session_versions v WHERE v.epoch_id=? AND v.revision=(SELECT max(x.revision) FROM session_versions x WHERE x.epoch_id=v.epoch_id AND x.session_id=v.session_id AND x.revision<=?)) SELECT coalesce(root.project_id,'') FROM sv JOIN sv root ON root.session_id=sv.root_work_unit_id WHERE sv.session_id=?`, timePlan.DatasetEpoch, timePlan.AppliedRevision, sessionID).Scan(&includedRootProject)
		if err != nil || includedRootProject != projectID {
			t.Fatalf("included session %s root project=%s err=%v", sessionID, includedRootProject, err)
		}
	}
	timeReq.Scope.ProjectID = "project-guessed"
	if _, err = m.Plan(context.Background(), store, timeReq); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown canonical project accepted: %v", err)
	}
	var reviewRoot string
	if err = store.DB().QueryRow(`SELECT s.id FROM sessions s JOIN session_versions v ON v.session_id=s.id WHERE s.source_session_id='review-1' AND v.purpose='inspector_review' ORDER BY v.revision DESC LIMIT 1`).Scan(&reviewRoot); err != nil {
		t.Fatal(err)
	}
	if _, err = m.Plan(context.Background(), store, baseRequest(reviewRoot)); !errors.Is(err, ErrScopeEmpty) && !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Review-created root was eligible: %v", err)
	}
}

func TestPlanCoverageUsesPinnedSourceInventoryStates(t *testing.T) {
	layout, store := indexedFixture(t)
	m, err := New(layout, "/missing", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var root string
	if err = store.DB().QueryRow(`SELECT id FROM sessions WHERE source_session_id='root-001'`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	_, baseline, err := store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	request := baseRequest(root)
	request.RequestedRevision = baseline
	complete, err := m.Plan(context.Background(), store, request)
	if err != nil {
		t.Fatal(err)
	}
	if complete.Coverage.Fidelity != "exact" || complete.Coverage.Observed != complete.Coverage.Eligible {
		t.Fatalf("complete coverage=%+v gaps=%v", complete.Coverage, complete.ManifestPreview.CoverageGaps)
	}
	for _, state := range []string{"discovered", "indexing", "unsupported", "failed", "requires_rebuild"} {
		revision := appendSourceState(t, store, state)
		request.RequestedRevision = revision
		plan, planErr := m.Plan(context.Background(), store, request)
		if planErr != nil {
			t.Fatal(planErr)
		}
		if plan.Coverage.Fidelity != "derived" || plan.Coverage.Reason == "" || len(plan.ManifestPreview.CoverageGaps) == 0 {
			t.Fatalf("state %s coverage=%+v gaps=%v", state, plan.Coverage, plan.ManifestPreview.CoverageGaps)
		}
		joined := strings.Join(plan.ManifestPreview.CoverageGaps, " ")
		expected := map[string]string{"discovered": "had not started", "indexing": "still indexing", "unsupported": "unsupported", "failed": "failed indexing", "requires_rebuild": "required a rebuild"}[state]
		if !strings.Contains(joined, expected) {
			t.Fatalf("state %s missing diagnostic: %s", state, joined)
		}
	}
	request.RequestedRevision = baseline
	pinned, err := m.Plan(context.Background(), store, request)
	if err != nil {
		t.Fatal(err)
	}
	if pinned.Coverage.Fidelity != "exact" || pinned.AppliedRevision != baseline {
		t.Fatalf("newer inventory leaked into pinned plan: %+v", pinned.Coverage)
	}
}

func writeFakeCodex(t *testing.T, dir, report string, exit int, overwrite string) string {
	t.Helper()
	path := filepath.Join(dir, "fake-codex.sh")
	script := "#!/bin/sh\nset -eu\nprintf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"thread-demo\"}'\nprintf '%s' '{' > review.json\nsleep 0.2\nprintf '%s\\n' '" + strings.ReplaceAll(report, "'", "'\\''") + "' > review.json\nsleep 0.3\n"
	if overwrite != "" {
		script += "printf '%s\\n' '" + strings.ReplaceAll(overwrite, "'", "'\\''") + "' > review.json\n"
	}
	script += fmt.Sprintf("exit %d\n", exit)
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func reportJSON(p Plan, citation string) string {
	v := map[string]any{"schemaVersion": SchemaVersion, "reviewId": p.ManifestPreview.ReviewID, "scope": map[string]any{"kind": p.ManifestPreview.Scope.Kind, "summary": "Fake review scope.", "datasetEpoch": p.DatasetEpoch, "indexRevision": p.AppliedRevision}, "model": "gpt-demo", "reasoning": "high", "completedAt": "2026-07-18T12:00:00Z", "summary": "A concise fake report.", "findings": []any{map[string]any{"findingId": "finding-1", "kind": "opportunity", "lens": "task_framing_and_steering", "title": "State the test first", "observation": "The task began before its acceptance test was stated.", "impact": "This can increase rework.", "support": "directly_observed", "evidenceSummary": "The cited fake event locates the observation.", "citations": []string{citation}, "recommendation": "State the acceptance test first.", "actionPrompt": "Draft acceptance criteria; do not execute them."}}}
	b, _ := json.Marshal(v)
	return string(b)
}

func waitStatus(t *testing.T, m *Manager, id, want string) Detail {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		d, err := m.Detail(context.Background(), id)
		if err == nil && d.Run.Status == want {
			return d
		}
		time.Sleep(25 * time.Millisecond)
	}
	d, err := m.Detail(context.Background(), id)
	t.Fatalf("status not %s: detail=%+v err=%v", want, d, err)
	return Detail{}
}

func TestLaunchRequiresConfirmationCapturesThreadAndAcceptsFirstValidReport(t *testing.T) {
	layout, store := indexedFixture(t)
	probe, err := New(layout, "/missing", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	var root string
	_ = store.DB().QueryRow(`SELECT id FROM sessions s WHERE source_session_id='root-001'`).Scan(&root)
	plan, err := probe.Plan(context.Background(), store, baseRequest(root))
	if err != nil {
		t.Fatal(err)
	}
	valid := reportJSON(plan, plan.ManifestPreview.Evidence[0].EvidenceID)
	overwrite := strings.Replace(valid, "A concise fake report.", "A later overwrite that must not win.", 1)
	fake := writeFakeCodex(t, t.TempDir(), valid, 0, overwrite)
	m, err := New(layout, fake, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.plans[plan.PlanID] = plan
	m.mu.Unlock()
	if _, err = m.Launch(context.Background(), plan.PlanID, false); err != ErrConfirmation {
		t.Fatalf("unconfirmed launch=%v", err)
	}
	summary, err := m.Launch(context.Background(), plan.PlanID, true)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Status != "planned" {
		t.Fatalf("initial=%+v", summary)
	}
	detail := waitStatus(t, m, summary.ReviewID, "complete")
	if detail.Run.ThreadID != "thread-demo" || detail.Run.AcceptedReportSHA256 == "" || detail.AcceptedReport == nil {
		t.Fatalf("complete detail=%+v", detail)
	}
	if detail.AcceptedReport.Summary != "A concise fake report." {
		t.Fatalf("later overwrite replaced accepted snapshot: %q", detail.AcceptedReport.Summary)
	}
	if len(detail.AcceptedReport.FindingsRendered) != 1 || detail.AcceptedReport.FindingsRendered[0].Citations[0].EvidenceID == "" {
		t.Fatal("citation did not resolve")
	}
	if ResumeCommand(detail.Run.ThreadID) != "codex resume thread-demo" || DeepLink(detail.Run.ThreadID) != "codex://threads/thread-demo" {
		t.Fatal("handoff mismatch")
	}
	citedSource := detail.Manifest.Evidence[0].SourceID
	for _, source := range detail.Manifest.Sources {
		if source.SourceID == citedSource {
			if err = os.Remove(source.Locator); err != nil {
				t.Fatal(err)
			}
		}
	}
	afterMissing, err := m.Detail(context.Background(), summary.ReviewID)
	if err != nil {
		t.Fatal(err)
	}
	if got := afterMissing.AcceptedReport.FindingsRendered[0].Citations[0].Availability; got != "source_missing" {
		t.Fatalf("later missing source not diagnosed: %s", got)
	}
}

func TestInvalidMissingCitationAndProcessFailureRemainDiagnosable(t *testing.T) {
	layout, store := indexedFixture(t)
	var root string
	_ = store.DB().QueryRow(`SELECT id FROM sessions s WHERE source_session_id='root-001'`).Scan(&root)
	for _, tc := range []struct {
		name       string
		makeReport func(Plan) string
		exit       int
		want       string
	}{
		{"invalid artifact", func(Plan) string { return `{"bad":true}` }, 0, "unrenderable"},
		{"missing citation", func(p Plan) string { return reportJSON(p, "evidence-not-in-manifest") }, 0, "unrenderable"},
		{"process failure", func(p Plan) string { return "" }, 7, "failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seed, _ := New(layout, "/missing", "", nil)
			plan, err := seed.Plan(context.Background(), store, baseRequest(root))
			if err != nil {
				t.Fatal(err)
			}
			fakeDir := t.TempDir()
			var fake string
			if tc.makeReport(plan) == "" {
				fake = filepath.Join(fakeDir, "fail.sh")
				_ = os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"thread-fail\"}'\nexit 7\n"), 0700)
			} else {
				fake = writeFakeCodex(t, fakeDir, tc.makeReport(plan), tc.exit, "")
			}
			m, _ := New(layout, fake, "", nil)
			m.mu.Lock()
			m.plans[plan.PlanID] = plan
			m.mu.Unlock()
			summary, err := m.Launch(context.Background(), plan.PlanID, true)
			if err != nil {
				t.Fatal(err)
			}
			d := waitStatus(t, m, summary.ReviewID, tc.want)
			if d.Run.FailureCode == "" || d.ReportState == "accepted" {
				t.Fatalf("failure not diagnosed: %+v", d)
			}
		})
	}
}
