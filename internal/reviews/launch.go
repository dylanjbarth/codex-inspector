package reviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/storage"
	reviewschemas "github.com/dylanjbarth/codex-inspector/schemas/reviews"
)

func (m *Manager) Launch(ctx context.Context, planID string, confirmed bool) (Summary, error) {
	if !confirmed {
		return Summary{}, ErrConfirmation
	}
	plan, ok := m.plan(planID)
	if !ok {
		return Summary{}, ErrPlanUnavailable
	}
	reviewID := plan.Review.ReviewID
	m.mu.Lock()
	if m.active[reviewID] {
		m.mu.Unlock()
		return Summary{}, ErrPlanUnavailable
	}
	m.active[reviewID] = true
	delete(m.plans, planID)
	m.mu.Unlock()
	failed := true
	defer func() {
		if failed {
			m.mu.Lock()
			delete(m.active, reviewID)
			m.mu.Unlock()
		}
	}()
	dir := filepath.Join(m.layout.Reviews, reviewID)
	if err := os.Mkdir(dir, 0700); err != nil {
		return Summary{}, err
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return Summary{}, err
	}
	schemaBytes, err := reviewschemasRead("report.schema.json")
	if err != nil {
		return Summary{}, err
	}
	if err = atomicWrite(filepath.Join(dir, "report.schema.json"), schemaBytes, 0600); err != nil {
		return Summary{}, err
	}
	args := []string{"exec", "--json", "--sandbox", "workspace-write", "--skip-git-repo-check", "-C", dir}
	if plan.Review.Model != "configured-default" {
		args = append(args, "--model", plan.Review.Model)
	}
	if plan.Review.Reasoning != "configured-default" {
		reasoningJSON, _ := json.Marshal(plan.Review.Reasoning)
		args = append(args, "-c", "model_reasoning_effort="+string(reasoningJSON))
	}
	args = append(args, plan.LaunchPrompt)
	now := m.now().UTC().Format(time.RFC3339Nano)
	review := plan.Review
	run := Run{SchemaVersion: SchemaVersion, ReviewID: reviewID, Status: "planned", CreatedAt: now, Command: append([]string{m.codex}, args...), Diagnostics: []string{}, Review: &review}
	if err = m.writeRun(dir, run); err != nil {
		return Summary{}, err
	}
	m.changed(reviewID, run.Status)
	cmd := exec.CommandContext(ctx, m.codex, args...)
	cmd.Dir = dir
	cmd.Stdin = nil
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Summary{}, err
	}
	if err = cmd.Start(); err != nil {
		run.Status = "failed"
		run.CompletedAt = m.now().UTC().Format(time.RFC3339Nano)
		run.FailureCode = "launch_failed"
		run.FailureMessage = "Codex process could not be started."
		_ = m.writeRun(dir, run)
		m.changed(reviewID, run.Status)
		return Summary{reviewID, run.Status, run.CreatedAt, ""}, nil
	}
	run.PID = cmd.Process.Pid
	run.StartedAt = m.now().UTC().Format(time.RFC3339Nano)
	if err = m.writeRun(dir, run); err != nil {
		_ = cmd.Process.Kill()
		return Summary{}, err
	}
	go m.monitor(ctx, dir, plan.Review, run, cmd, stdout)
	failed = false
	return Summary{reviewID, run.Status, run.CreatedAt, ""}, nil
}

func reviewschemasRead(name string) ([]byte, error) { return reviewschemas.Files.ReadFile(name) }

type processEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
}
type decodedEvent struct {
	event processEvent
	err   error
}

func (m *Manager) monitor(ctx context.Context, dir string, spec ReviewSpec, run Run, cmd *exec.Cmd, stdout io.ReadCloser) {
	defer func() { m.mu.Lock(); delete(m.active, run.ReviewID); m.mu.Unlock() }()
	events := make(chan decodedEvent, 1)
	go func() {
		d := json.NewDecoder(stdout)
		for {
			var e processEvent
			err := d.Decode(&e)
			events <- decodedEvent{e, err}
			if err != nil {
				return
			}
		}
	}()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	first := true
	accepted := false
	processExited := false
	var processErr error
	for !processExited {
		select {
		case got := <-events:
			if got.err != nil {
				if !errors.Is(got.err, io.EOF) && !accepted {
					run.Diagnostics = appendBounded(run.Diagnostics, "invalid_json_event")
				}
				events = nil
				continue
			}
			if first {
				first = false
				if got.event.Type != "thread.started" || got.event.ThreadID == "" {
					run.Status = "failed"
					run.CompletedAt = m.now().UTC().Format(time.RFC3339Nano)
					run.FailureCode = "missing_thread_started"
					run.FailureMessage = "Codex did not begin with the required persisted thread event."
					_ = cmd.Process.Kill()
					m.changed(run.ReviewID, run.Status)
					continue
				}
				run.ThreadID = got.event.ThreadID
				run.Status = "running"
				run.LaunchPromptVersion = LaunchPromptVersion
				run.Diagnostics = appendBounded(run.Diagnostics, "thread_started")
				_ = m.writeRun(dir, run)
				m.changed(run.ReviewID, run.Status)
			}
			if got.event.Type == "turn.failed" || got.event.Type == "error" {
				run.Diagnostics = appendBounded(run.Diagnostics, "codex_terminal_error")
			}
		case err := <-waited:
			processExited = true
			processErr = err
		case <-tick.C:
			if !accepted && run.ThreadID != "" {
				if report, b, hash, err := m.validReport(filepath.Join(dir, "review.json"), spec); err == nil {
					inserted, acceptErr := m.accept(run.ReviewID, b, hash)
					if acceptErr == nil {
						if !inserted {
							report, hash, _ = m.loadAccepted(run.ReviewID)
							_ = report
						}
						accepted = true
						run.Status = "complete"
						run.CompletedAt = m.now().UTC().Format(time.RFC3339Nano)
						run.AcceptedReportSHA256 = hash
						run.FailureCode = ""
						run.FailureMessage = ""
						_ = m.writeRun(dir, run)
						m.changed(run.ReviewID, run.Status)
					}
				}
			}
		case <-ctx.Done():
			_ = cmd.Process.Kill()
		}
	}
	exit := 0
	if processErr != nil {
		var ee *exec.ExitError
		if errors.As(processErr, &ee) {
			exit = ee.ExitCode()
		} else {
			exit = -1
		}
	}
	run.ExitCode = &exit
	if !accepted {
		if run.ThreadID == "" {
			run.Status = "failed"
			run.FailureCode = "missing_thread_started"
			run.FailureMessage = "Codex exited without a resumable persisted session ID."
		} else if _, reportBytes, hash, err := m.validReport(filepath.Join(dir, "review.json"), spec); err == nil {
			if _, e := m.accept(run.ReviewID, reportBytes, hash); e == nil {
				run.Status = "complete"
				run.AcceptedReportSHA256 = hash
				accepted = true
			}
		} else if errors.Is(err, os.ErrNotExist) {
			run.Status = "failed"
			run.FailureCode = "report_missing"
			run.FailureMessage = "Codex exited without writing review.json."
		} else {
			run.Status = "unrenderable"
			run.FailureCode = "report_invalid"
			run.FailureMessage = "review.json does not satisfy the report contract or selected scope."
		}
		run.CompletedAt = m.now().UTC().Format(time.RFC3339Nano)
	}
	_ = m.writeRun(dir, run)
	m.changed(run.ReviewID, run.Status)
}

func appendBounded(v []string, s string) []string {
	if len(s) > 500 {
		s = s[:500]
	}
	if len(v) >= 100 {
		return v
	}
	return append(v, s)
}
func (m *Manager) changed(id, status string) {
	if m.notify != nil {
		m.notify(id, status)
	}
}

func (m *Manager) validReport(path string, spec ReviewSpec) (Report, []byte, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Report{}, nil, "", err
	}
	if info.Size() > 1048576 {
		return Report{}, nil, "", errors.New("report exceeds 1 MiB")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Report{}, nil, "", err
	}
	var doc any
	if err = json.Unmarshal(b, &doc); err != nil {
		return Report{}, nil, "", err
	}
	if err = m.reportSchema.Validate(doc); err != nil {
		return Report{}, nil, "", err
	}
	var report Report
	if err = strictJSON(b, &report); err != nil {
		return Report{}, nil, "", err
	}
	if report.ReviewID != spec.ReviewID || report.SchemaVersion != SchemaVersion || report.Scope.Kind != spec.Scope.Kind || report.Scope.DatasetEpoch != spec.DatasetEpoch || report.Scope.IndexRevision != spec.IndexRevision {
		return Report{}, nil, "", errors.New("report identity does not match review parameters")
	}
	store, err := storage.Open(filepath.Join(m.layout.Root, "inspector.db"))
	if err != nil {
		return Report{}, nil, "", err
	}
	defer store.Close()
	for _, f := range report.Findings {
		for _, id := range f.Citations {
			if ok, checkErr := citationBelongsToScope(context.Background(), store.DB(), spec, id); checkErr != nil {
				return Report{}, nil, "", checkErr
			} else if !ok {
				return Report{}, nil, "", fmt.Errorf("citation %s is outside the selected scope", id)
			}
		}
	}
	return report, b, hashBytes(b), nil
}

func citationBelongsToScope(ctx context.Context, db *sql.DB, spec ReviewSpec, evidenceID string) (bool, error) {
	query := selectedSessionsCTE + `SELECT count(*) FROM evidence_refs r
	 JOIN events e ON e.epoch_id=r.epoch_id AND e.id=r.event_id
	 JOIN turns t ON t.epoch_id=e.epoch_id AND t.id=e.turn_id
	 JOIN sv ON sv.epoch_id=t.epoch_id AND sv.session_id=t.session_id
	 JOIN sv root ON root.epoch_id=sv.epoch_id AND root.session_id=sv.root_work_unit_id
	 WHERE r.epoch_id=? AND r.id=? AND e.commit_revision<=? AND t.commit_revision<=? AND t.state='completed' AND sv.purpose<>'inspector_review'`
	args := []any{spec.DatasetEpoch, spec.IndexRevision, spec.DatasetEpoch, evidenceID, spec.IndexRevision, spec.IndexRevision}
	if spec.Scope.Kind == "single_session" {
		query += ` AND sv.root_work_unit_id=?`
		args = append(args, spec.Scope.RootSessionID)
	} else {
		query += ` AND t.completed_at>=? AND t.completed_at<?`
		args = append(args, spec.Scope.Start, spec.Scope.End)
		if spec.Scope.ProjectID != "" {
			query += ` AND root.project_id=?`
			args = append(args, spec.Scope.ProjectID)
		}
	}
	var count int
	if err := db.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
		return false, err
	}
	return count == 1, nil
}

func ResumeCommand(threadID string) string { return "codex resume " + threadID }
func DeepLink(threadID string) string      { return "codex://threads/" + threadID }
func safeThreadID(id string) bool          { return id != "" && !strings.ContainsAny(id, " \t\r\n") }
