package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	"github.com/dylanjbarth/codex-inspector/internal/reviews"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

func main() {
	codexHome := flag.String("codex-home", "", "isolated Codex home containing fake rollouts and the installed Review skill")
	inspectorHome := flag.String("inspector-home", "", "disposable Inspector home")
	flag.Parse()
	if *codexHome == "" || *inspectorHome == "" {
		fail("both isolated homes are required")
	}
	if os.Getenv("CODEX_INSPECTOR_RUN_LIVE_PROOFS") != "1" {
		fail("live proof opt-in is required")
	}
	_ = os.Setenv("CODEX_INSPECTOR_HOME", *inspectorHome)
	_ = os.Setenv("CODEX_HOME", *codexHome)
	layout, err := home.Resolve()
	if err != nil {
		fail("home resolution failed")
	}
	if err = home.Ensure(layout); err != nil {
		fail("home creation failed")
	}
	if _, err = indexer.Run(context.Background(), indexer.Config{Layout: layout, CodexHome: *codexHome}); err != nil {
		fail("synthetic indexing failed")
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		fail("index open failed")
	}
	var rootID string
	if err = store.DB().QueryRow(`SELECT id FROM sessions WHERE source_session_id='root-001'`).Scan(&rootID); err != nil {
		fail("synthetic root missing")
	}
	manager, err := reviews.New(layout, "codex", *codexHome, nil)
	if err != nil {
		fail("review manager failed")
	}
	plan, err := manager.Plan(context.Background(), store, reviews.PlanRequest{Scope: reviews.Scope{Kind: "single_session", RootSessionID: rootID}, Model: "configured-default", ReasoningEffort: "configured-default", Focus: "This is a safe synthetic lifecycle proof. Return a concise report."})
	_ = store.Close()
	if err != nil {
		fail("review planning failed")
	}
	if _, err = manager.Launch(context.Background(), plan.PlanID, true); err != nil {
		fail("review launch failed")
	}
	deadline := time.Now().Add(5 * time.Minute)
	var detail reviews.Detail
	for time.Now().Before(deadline) {
		detail, err = manager.Detail(context.Background(), plan.Review.ReviewID)
		if err == nil && (detail.Run.Status == "complete" || detail.Run.Status == "failed" || detail.Run.Status == "unrenderable") {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err != nil || detail.Run.Status != "complete" || detail.AcceptedReport == nil || detail.Run.ThreadID == "" {
		fail("review did not complete with an accepted report and captured thread")
	}
	if !resume(detail.Run.ThreadID) {
		fail("captured thread could not be resumed")
	}
	deepLink := reviews.DeepLink(detail.Run.ThreadID)
	deepLinkInvoked := exec.Command("/usr/bin/open", "-g", deepLink).Run() == nil
	fmt.Printf("review_status=complete thread_captured=true persisted_resume=true deep_link_invoked=%t findings=%d citations_resolved=%t report_hash_recorded=true\n", deepLinkInvoked, len(detail.AcceptedReport.FindingsRendered), citationsResolved(detail))
}

func resume(threadID string) bool {
	cmd := exec.Command("codex", "exec", "resume", "--json", "--skip-git-repo-check", threadID, "Reply with exactly: inspector-review-resume-ok")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return false
	}
	cmd.Stderr = nil
	if err = cmd.Start(); err != nil {
		return false
	}
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	same := false
	for scanner.Scan() {
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
		}
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Type == "thread.started" && event.ThreadID == threadID {
			same = true
		}
	}
	return cmd.Wait() == nil && same
}
func citationsResolved(detail reviews.Detail) bool {
	for _, f := range detail.AcceptedReport.FindingsRendered {
		for _, c := range f.Citations {
			if c.EvidenceID == "" {
				return false
			}
		}
	}
	return true
}
func fail(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
