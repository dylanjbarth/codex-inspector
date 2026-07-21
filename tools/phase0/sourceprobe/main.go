package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dylanjbarth/codex-inspector/internal/phase0"
)

func main() {
	fixture := flag.String("fixture", "", "synthetic rollout fixture to inspect")
	codexHome := flag.String("codex-home", "", "Codex home to inspect without printing paths, IDs, or payloads")
	flag.Parse()
	if (*fixture == "") == (*codexHome == "") {
		fmt.Fprintln(os.Stderr, "exactly one of --fixture or --codex-home is required")
		os.Exit(2)
	}
	if *codexHome != "" {
		probeHome(*codexHome)
		return
	}
	decision, err := phase0.ParseFixture(*fixture)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	completed := 0
	for _, turn := range decision.Turns {
		if turn.Completed {
			completed++
		}
	}
	fmt.Printf("adapter=%s supported=%t reason=%s turns=%d completed=%d pending_tail=%t\n", phase0.AdapterVersion, decision.Supported, decision.Reason, len(decision.Turns), completed, decision.PendingTail)
}

func probeHome(home string) {
	var files, supported, unsupported, failed, completed int
	reasons := map[string]int{}
	for _, directory := range []string{"sessions", "archived_sessions"} {
		root := filepath.Join(home, directory)
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if os.IsNotExist(walkErr) {
					return nil
				}
				return walkErr
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				return nil
			}
			files++
			decision, err := phase0.ParseFixture(path)
			if err != nil {
				failed++
				return nil
			}
			if decision.Supported {
				supported++
				for _, turn := range decision.Turns {
					if turn.Completed {
						completed++
					}
				}
			} else {
				unsupported++
				reasons[decision.Reason]++
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "structural inventory failed without emitting source details")
			os.Exit(1)
		}
	}
	fmt.Printf("adapter=%s files=%d supported=%d unsupported=%d failed=%d completed_turns=%d payloads_emitted=0\n", phase0.AdapterVersion, files, supported, unsupported, failed, completed)
	keys := make([]string, 0, len(reasons))
	for reason := range reasons {
		keys = append(keys, reason)
	}
	sort.Strings(keys)
	for _, reason := range keys {
		fmt.Printf("decision=%s count=%d\n", reason, reasons[reason])
	}
}
