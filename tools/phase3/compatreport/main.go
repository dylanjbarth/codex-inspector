// compatreport indexes a selected source home into a disposable Inspector
// home, then emits only aggregate structural compatibility diagnostics.
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
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

type cohort struct {
	Version string `json:"version"`
	Kind    string `json:"kind"`
	State   string `json:"state"`
	Reason  string `json:"reason"`
	Count   int    `json:"count"`
}

func main() {
	codexRoot := flag.String("codex-home", "", "read-only Codex source home")
	inspectorRoot := flag.String("inspector-home", "", "disposable Inspector home")
	flag.Parse()
	if *codexRoot == "" || *inspectorRoot == "" {
		fatal(fmt.Errorf("both --codex-home and --inspector-home are required"))
	}
	l := home.Layout{Root: *inspectorRoot, Reviews: filepath.Join(*inspectorRoot, "reviews"), Queue: filepath.Join(*inspectorRoot, "queue"), Run: filepath.Join(*inspectorRoot, "run"), Logs: filepath.Join(*inspectorRoot, "logs"), Cache: filepath.Join(*inspectorRoot, "cache")}
	if err := home.Ensure(l); err != nil {
		fatal(err)
	}
	progress, indexErr := indexer.Run(context.Background(), indexer.Config{Layout: l, CodexHome: *codexRoot})
	store, err := storage.Open(filepath.Join(l.Root, "inspector.db"))
	if err != nil {
		fatal(err)
	}
	defer store.Close()
	epoch, _, err := store.Snapshot()
	if err != nil {
		fatal(err)
	}
	rows, err := store.DB().Query(`WITH latest AS (
SELECT v.* FROM source_artifact_versions v WHERE v.epoch_id=? AND v.revision=(
 SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id)
) SELECT coalesce(detected_codex_version,'unknown'),source_kind,state,coalesce(state_reason,''),count(*)
FROM latest GROUP BY 1,2,3,4 ORDER BY 5 DESC,1,2,3,4`, epoch)
	if err != nil {
		fatal(err)
	}
	defer rows.Close()
	cohorts := []cohort{}
	for rows.Next() {
		var item cohort
		if err := rows.Scan(&item.Version, &item.Kind, &item.State, &item.Reason, &item.Count); err != nil {
			fatal(err)
		}
		cohorts = append(cohorts, item)
	}
	if err := rows.Err(); err != nil {
		fatal(err)
	}
	result := map[string]any{"inventory": progress, "indexError": indexErr != nil, "cohorts": cohorts, "payloadsEmitted": false}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "compatibility report failed:", err)
	os.Exit(1)
}
