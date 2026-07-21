// Command fakecodex is deterministic Phase 6 browser-E2E infrastructure. It
// implements only the Codex surfaces Inspector invokes and never contacts a
// model service.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dylanjbarth/codex-inspector/internal/reviews"
)

func main() {
	if os.Getenv("CODEX_INSPECTOR_PHASE6_E2E") != "1" {
		fail()
	}
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println("codex-cli 0.145.0-alpha.18")
		return
	}
	if len(args) == 3 && args[0] == "plugin" && args[1] == "list" && args[2] == "--json" {
		fmt.Println(`{"installed":[{"name":"codex-inspector","pluginId":"codex-inspector@phase6-e2e","version":"0.1.0","enabled":true}]}`)
		return
	}
	if len(args) == 2 && args[0] == "app-server" && args[1] == "--stdio" {
		serveHooks()
		return
	}
	if len(args) > 0 && args[0] == "exec" {
		runReview()
		return
	}
	fail()
}

func serveHooks() {
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request struct {
			ID int `json:"id"`
		}
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			continue
		}
		switch request.ID {
		case 1:
			_ = encoder.Encode(map[string]any{"id": 1, "result": map[string]any{}})
		case 2:
			events := []string{"postCompact", "preCompact", "sessionStart", "stop", "subagentStart", "subagentStop", "userPromptSubmit"}
			hooks := make([]map[string]any, 0, len(events))
			for i, event := range events {
				hooks = append(hooks, map[string]any{"eventName": event, "handlerType": "command", "command": "/phase6-e2e/hooks/inspector-hook.sh", "currentHash": fmt.Sprintf("sha256:%064x", i+1), "trustStatus": "trusted", "pluginId": "codex-inspector@phase6-e2e", "source": "plugin", "enabled": true, "timeoutSec": 2})
			}
			_ = encoder.Encode(map[string]any{"id": 2, "result": map[string]any{"data": []any{map[string]any{"hooks": hooks}}}})
			return
		}
	}
}

func runReview() {
	b, err := os.ReadFile("manifest.json")
	if err != nil {
		fail()
	}
	var manifest reviews.Manifest
	if json.Unmarshal(b, &manifest) != nil || len(manifest.Evidence) == 0 {
		fail()
	}
	report := reviews.Report{
		SchemaVersion: reviews.SchemaVersion,
		ReviewID:      manifest.ReviewID,
		Scope: reviews.ReportScope{
			Kind:          manifest.Scope.Kind,
			Summary:       "Synthetic Phase 6 browser scope.",
			DatasetEpoch:  manifest.DatasetEpoch,
			IndexRevision: manifest.IndexRevision,
		},
		Model:       "gpt-synthetic",
		Reasoning:   "high",
		CompletedAt: "2026-07-19T12:00:00Z",
		Summary:     "Synthetic Phase 6 browser report.",
		Findings: []reviews.Finding{{
			FindingID:       "finding-phase6-browser",
			Kind:            "strength",
			Lens:            "reusable_leverage",
			Title:           "The production evidence path remains reusable",
			Observation:     "The deterministic persisted task used a manifest-authorized evidence reference.",
			Impact:          "The rendered citation returns to the same source-backed Context Inspector event.",
			Support:         "directly_observed",
			EvidenceSummary: "The citation is frozen in the persisted manifest.",
			Citations:       []string{manifest.Evidence[0].EvidenceID},
			Recommendation:  "Keep the evidence-linked workflow.",
		}},
	}
	reportBytes, err := json.Marshal(report)
	if err != nil || os.WriteFile(filepath.Join("review.json"), reportBytes, 0o600) != nil {
		fail()
	}
	fmt.Println(`{"type":"thread.started","thread_id":"thread-phase6-browser-e2e"}`)
	fmt.Println(`{"type":"turn.completed"}`)
}

func fail() {
	os.Exit(2)
}
