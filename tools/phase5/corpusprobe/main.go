package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

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
	plan, err := manager.Plan(context.Background(), store, reviews.PlanRequest{Scope: reviews.Scope{Kind: "single_session", RootSessionID: targetRoot}, Model: "configured-default", ReasoningEffort: "configured-default"})
	if err != nil {
		switch {
		case errors.Is(err, reviews.ErrScopeTooLarge):
			fail("plan_scope_too_large")
		case errors.Is(err, reviews.ErrScopeEmpty):
			fail("plan_scope_empty")
		case errors.Is(err, reviews.ErrRevisionUnavailable):
			fail("plan_revision")
		case errors.Is(err, reviews.ErrManifestContract):
			fmt.Fprintf(os.Stderr, "manifest_contract_locations=%v\n", reviews.ManifestContractLocations(err))
			fail("plan_manifest_contract")
		default:
			fail("plan_query")
		}
	}
	var reviewSessions int
	if err = store.DB().QueryRow(`SELECT count(DISTINCT s.id) FROM sessions s JOIN session_versions v ON v.session_id=s.id WHERE v.purpose='inspector_review'`).Scan(&reviewSessions); err != nil {
		fail("review_count")
	}
	for _, id := range plan.ManifestPreview.IncludedSessionIDs {
		var purpose string
		if err = store.DB().QueryRow(`SELECT purpose FROM session_versions WHERE session_id=? ORDER BY revision DESC LIMIT 1`, id).Scan(&purpose); err != nil || purpose == "inspector_review" {
			fail("review_exclusion")
		}
	}
	fmt.Printf("inventory=%d processed=%d skipped=%d failed=%d requires_rebuild=%d review_roots_indexed=%d review_roots_in_scope=0 scope_sessions=%d scope_turns=%d scope_sources=%d scope_evidence=%d included_bytes=%d coverage_fidelity=%s coverage_observed=%d coverage_eligible=%d coverage_gaps=%d manifest_schema=valid\n", progress.Inventoried, progress.Processed, progress.Skipped, progress.Failed, progress.RequiresRebuild, reviewSessions, len(plan.ManifestPreview.IncludedSessionIDs), len(plan.ManifestPreview.IncludedTurnIDs), len(plan.ManifestPreview.Sources), len(plan.ManifestPreview.Evidence), plan.SourceByteCounts.IncludedBytes, plan.Coverage.Fidelity, plan.Coverage.Observed, plan.Coverage.Eligible, len(plan.ManifestPreview.CoverageGaps))
}
func fail(stage string) {
	fmt.Fprintf(os.Stderr, "phase5 corpus proof failed stage=%s without emitting source payloads\n", stage)
	os.Exit(1)
}
