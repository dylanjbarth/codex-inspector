package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	"github.com/dylanjbarth/codex-inspector/internal/reviews"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

func main() {
	layout, err := home.Resolve()
	if err != nil {
		fail("home")
	}
	if err = home.Ensure(layout); err != nil {
		fail("ensure")
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		user, homeErr := os.UserHomeDir()
		if homeErr != nil {
			fail("user_home")
		}
		codexHome = filepath.Join(user, ".codex")
	}
	progress, err := indexer.Run(context.Background(), indexer.Config{Layout: layout, CodexHome: codexHome})
	if err != nil {
		fail("index")
	}
	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		fail("store")
	}
	defer store.Close()
	manager, err := reviews.New(layout, "codex", codexHome, nil)
	if err != nil {
		fail("manager")
	}
	var targetRoot string
	if err = store.DB().QueryRow(`WITH sv AS (SELECT v.* FROM session_versions v WHERE v.revision=(SELECT max(x.revision) FROM session_versions x WHERE x.epoch_id=v.epoch_id AND x.session_id=v.session_id)) SELECT sv.session_id FROM sv JOIN turns t ON t.session_id=sv.session_id AND t.state='completed' JOIN events e ON e.turn_id=t.id WHERE sv.purpose='user' AND sv.root_work_unit_id=sv.session_id GROUP BY sv.session_id HAVING count(e.id) BETWEEN 1 AND 1000 ORDER BY max(t.completed_at) DESC LIMIT 1`).Scan(&targetRoot); err != nil {
		fail("eligible_root")
	}
	plan, err := manager.Plan(context.Background(), store, reviews.PlanRequest{Scope: reviews.Scope{Kind: "single_session", RootSessionID: targetRoot}, Model: "gpt-5.6-sol", ReasoningEffort: "high"})
	if err != nil {
		switch {
		case errors.Is(err, reviews.ErrScopeTooLarge):
			fail("plan_scope_too_large")
		case errors.Is(err, reviews.ErrScopeEmpty):
			fail("plan_scope_empty")
		case errors.Is(err, reviews.ErrRevisionUnavailable):
			fail("plan_revision")
		default:
			fail("plan_query")
		}
	}
	var reviewSessions int
	if err = store.DB().QueryRow(`SELECT count(DISTINCT s.id) FROM sessions s JOIN session_versions v ON v.session_id=s.id WHERE v.purpose='inspector_review'`).Scan(&reviewSessions); err != nil {
		fail("review_count")
	}
	encoded, err := json.Marshal(plan)
	if err != nil || plan.Review.ReviewID == "" || plan.Review.Scope.RootSessionID != targetRoot || !strings.Contains(plan.LaunchPrompt, targetRoot) || strings.Contains(string(encoded), "manifestPreview") {
		fail("prompt_contract")
	}
	fmt.Printf("inventory=%d processed=%d skipped=%d failed=%d requires_rebuild=%d review_roots_indexed=%d prompt_scope=single_session prompt_bytes=%d prompt_first=true\n", progress.Inventoried, progress.Processed, progress.Skipped, progress.Failed, progress.RequiresRebuild, reviewSessions, len(plan.LaunchPrompt))
}
func fail(stage string) {
	fmt.Fprintf(os.Stderr, "phase5 corpus proof failed stage=%s without emitting source payloads\n", stage)
	os.Exit(1)
}
