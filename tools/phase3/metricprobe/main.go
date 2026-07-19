package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	"github.com/dylanjbarth/codex-inspector/internal/metrics"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

func main() {
	codex := flag.String("codex-home", "", "read-only Codex source home")
	inspector := flag.String("inspector-home", "", "disposable Inspector home")
	flag.Parse()
	if *codex == "" || *inspector == "" {
		fmt.Fprintln(os.Stderr, "both homes are required")
		os.Exit(2)
	}
	l := home.Layout{Root: *inspector, Reviews: filepath.Join(*inspector, "reviews"), Queue: filepath.Join(*inspector, "queue"), Run: filepath.Join(*inspector, "run"), Logs: filepath.Join(*inspector, "logs"), Cache: filepath.Join(*inspector, "cache")}
	if err := home.Ensure(l); err != nil {
		fatal(err)
	}
	progress, err := indexer.Run(context.Background(), indexer.Config{Layout: l, CodexHome: *codex})
	if err != nil {
		fatal(err)
	}
	store, err := storage.Open(filepath.Join(*inspector, "inspector.db"))
	if err != nil {
		fatal(err)
	}
	defer store.Close()
	result, err := metrics.New(8).Query(context.Background(), store, metrics.Query{MetricKeys: metrics.Keys, Timezone: "UTC", Grain: "month"})
	if err != nil {
		fatal(err)
	}
	summary := map[string]any{"inventory": map[string]int{"inventoried": progress.Inventoried, "processed": progress.Processed, "skipped": progress.Skipped, "failed": progress.Failed, "requiresRebuild": progress.RequiresRebuild}, "datasetEpochPresent": result.DatasetEpoch != "", "revision": result.AppliedRevision, "coverage": result.Coverage}
	for _, item := range result.Results {
		switch item.Key {
		case "recorded_tokens":
			summary["recordedTokens"] = item.Value
		case "top_root_sessions_by_tokens":
			summary["contributingRootCount"] = len(item.Value.([]metrics.Root))
		case "recorded_tokens_over_time":
			summary["timeBucketCount"] = len(item.Value.([]metrics.Bucket))
		case "latest_capacity_observation":
			summary["latestCapacityRecorded"] = item.Value != nil
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err = enc.Encode(summary); err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
