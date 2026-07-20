package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dylanjbarth/codex-inspector/internal/evidence"
	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	"github.com/dylanjbarth/codex-inspector/internal/inspector"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

func main() {
	codex := flag.String("codex-home", "", "read-only Codex source home")
	inspectorHome := flag.String("inspector-home", "", "disposable Inspector home")
	flag.Parse()
	if *codex == "" || *inspectorHome == "" {
		fatal(fmt.Errorf("both homes are required"))
	}
	layout := home.Layout{Root: *inspectorHome, Reviews: filepath.Join(*inspectorHome, "reviews"), Queue: filepath.Join(*inspectorHome, "queue"), Run: filepath.Join(*inspectorHome, "run"), Logs: filepath.Join(*inspectorHome, "logs"), Cache: filepath.Join(*inspectorHome, "cache")}
	if err := home.Ensure(layout); err != nil {
		fatal(err)
	}
	progress, err := indexer.Run(context.Background(), indexer.Config{Layout: layout, CodexHome: *codex})
	if err != nil {
		fatal(err)
	}
	store, err := storage.Open(filepath.Join(*inspectorHome, "inspector.db"))
	if err != nil {
		fatal(err)
	}
	defer store.Close()
	repository := inspector.Repository{Store: store}
	page, err := repository.Sessions(context.Background(), 0, "", "", nil, 0, 200)
	if err != nil {
		fatal(err)
	}
	summary := map[string]any{
		"inventory":           map[string]int{"inventoried": progress.Inventoried, "processed": progress.Processed, "skipped": progress.Skipped, "failed": progress.Failed, "requiresRebuild": progress.RequiresRebuild},
		"datasetEpochPresent": page.DatasetEpoch != "", "revision": page.AppliedRevision, "discoverableRootCount": len(page.Items),
	}
	mapCount, nodeCount, edgeCount, rootTurnCount, ledgerEventCount, metricMismatchCount := 0, 0, 0, 0, 0, 0
	evidenceChecked, evidenceAvailable, exactCompactionSeen := false, false, false
	eventKinds := map[string]int{}
	for _, item := range page.Items {
		mapped, mapErr := repository.Map(context.Background(), page.AppliedRevision, item.SessionID)
		if mapErr != nil {
			fatal(mapErr)
		}
		mapCount++
		nodeCount += len(mapped.Nodes)
		edgeCount += len(mapped.Edges)
		rootTurnCount += len(mapped.RootTurns)
		for _, node := range mapped.Nodes {
			if node.Kind == "root" && item.DirectTokens != nil && item.DescendantTokens != nil && node.InclusiveTokens != nil && *node.InclusiveTokens != *item.DirectTokens+*item.DescendantTokens {
				metricMismatchCount++
			}
		}
		if len(mapped.RootTurns) == 0 {
			continue
		}
		ledger, ledgerErr := repository.Ledger(context.Background(), page.AppliedRevision, item.SessionID, mapped.RootTurns[0].TurnID, 0, 200)
		if ledgerErr != nil {
			fatal(ledgerErr)
		}
		ledgerEventCount += len(ledger.Items)
		for _, event := range ledger.Items {
			eventKinds[event.Kind]++
			if event.Kind == "compacted" || event.Kind == "context_compacted" {
				exactCompactionSeen = true
			}
			if !evidenceChecked {
				chunk, resolveErr := evidence.ResolveAt(context.Background(), store, *codex, event.EvidenceID, page.AppliedRevision, 0, 1)
				if resolveErr != nil {
					fatal(resolveErr)
				}
				evidenceChecked = true
				evidenceAvailable = chunk.Availability == "available" && chunk.Bytes <= 1
			}
		}
	}
	var activeOrProvisional int
	if err = store.DB().QueryRow(`SELECT count(*) FROM turns WHERE state NOT IN ('completed','aborted','interrupted','reconciled_truncated')`).Scan(&activeOrProvisional); err != nil {
		fatal(err)
	}
	summary["inspector"] = map[string]any{"mapCount": mapCount, "nodeCount": nodeCount, "edgeCount": edgeCount, "rootTurnCount": rootTurnCount, "ledgerEventCount": ledgerEventCount, "eventKindCounts": eventKinds, "metricTotalMismatchCount": metricMismatchCount, "activeOrProvisionalVisible": activeOrProvisional, "boundedExactEvidenceChecked": evidenceChecked, "boundedExactEvidenceAvailable": evidenceAvailable, "recordedCompactionSeen": exactCompactionSeen}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(summary); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
