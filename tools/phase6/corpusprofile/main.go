package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
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
		userHome, homeErr := os.UserHomeDir()
		if homeErr != nil {
			fail("source_home")
		}
		codexHome = filepath.Join(userHome, ".codex")
	}

	started := time.Now()
	first, err := indexer.Run(context.Background(), indexer.Config{Layout: layout, CodexHome: codexHome})
	if err != nil {
		fail("initial_sync")
	}
	initialDuration := time.Since(started)

	store, err := storage.Open(filepath.Join(layout.Root, "inspector.db"))
	if err != nil {
		fail("store")
	}
	epoch, revision, err := store.Snapshot()
	if err != nil || epoch == "" || revision < 1 {
		fail("snapshot")
	}
	var sourceTotal, supportedCurrent, unsupported, failed, requiresRebuild, completedTurns, pendingTails int
	if sourceTotal, supportedCurrent, unsupported, failed, requiresRebuild, err = countSourceStates(store.DB(), epoch); err != nil {
		_ = store.Close()
		fail("inventory_projection")
	}
	if err = store.DB().QueryRow(`SELECT count(*) FROM turns WHERE epoch_id=? AND state='completed'`, epoch).Scan(&completedTurns); err != nil {
		_ = store.Close()
		fail("turn_projection")
	}
	if err = store.DB().QueryRow(`SELECT count(*) FROM source_checkpoints WHERE epoch_id=? AND pending_tail_bytes>0`, epoch).Scan(&pendingTails); err != nil {
		_ = store.Close()
		fail("checkpoint_projection")
	}
	if err = store.Close(); err != nil {
		fail("store_close")
	}

	if first.Inventoried != sourceTotal || first.Failed != 0 || first.RequiresRebuild != 0 || failed != 0 || requiresRebuild != 0 {
		fail("inventory_completion")
	}

	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	fmt.Printf("inventory=%d supported_current=%d unsupported=%d failed=0 requires_rebuild=0 completed_turns=%d pending_tail_sources=%d processed=%d skipped=%d revision=%d initial_ms=%d heap_sys_bytes=%d concurrency_limit=%d writer_limit=1 transaction_records=500 transaction_bytes=8388608 payloads_emitted=0\n",
		sourceTotal, supportedCurrent, unsupported, completedTurns, pendingTails, first.Processed, first.Skipped, revision, initialDuration.Milliseconds(), memory.HeapSys, min(4, runtime.NumCPU()))
}

func countSourceStates(db *sql.DB, epoch string) (total, supported, unsupported, failed, requiresRebuild int, err error) {
	stateQuery := `WITH latest AS (
 SELECT v.* FROM source_artifact_versions v
 WHERE v.epoch_id=? AND v.revision=(
  SELECT max(x.revision) FROM source_artifact_versions x
  WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id
 ))
 SELECT count(*),
	  sum(CASE WHEN state IN ('supported','current') THEN 1 ELSE 0 END),
  sum(CASE WHEN state='unsupported' THEN 1 ELSE 0 END),
  sum(CASE WHEN state='failed' THEN 1 ELSE 0 END),
	  sum(CASE WHEN state='requires_rebuild' THEN 1 ELSE 0 END)
	 FROM latest`
	err = db.QueryRow(stateQuery, epoch).Scan(&total, &supported, &unsupported, &failed, &requiresRebuild)
	return
}

func fail(stage string) {
	fmt.Fprintf(os.Stderr, "Phase 6 corpus profile failed at payload-free stage=%s\n", stage)
	os.Exit(1)
}
