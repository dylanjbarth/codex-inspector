package cli

import (
	"fmt"
	"io"

	"github.com/dylanjbarth/codex-inspector/internal/indexer"
)

type syncProgressReporter struct {
	w           io.Writer
	lastHandled int
	lastLine    string
	started     bool
}

func newSyncProgressReporter(w io.Writer) *syncProgressReporter {
	return &syncProgressReporter{w: w}
}

func (r *syncProgressReporter) Report(p indexer.Progress) {
	handled := syncHandled(p)
	step := p.Inventoried / 20
	if step < 1 {
		step = 1
	}
	if r.started && handled < p.Inventoried && handled-r.lastHandled < step {
		return
	}
	r.started = true
	r.lastHandled = handled
	r.write(progressLine("indexing", p))
}

func (r *syncProgressReporter) Complete(p indexer.Progress) {
	r.write(progressLine("complete", p))
}

func (r *syncProgressReporter) Failed(p indexer.Progress) {
	r.write(progressLine("failed", p))
}

func (r *syncProgressReporter) write(line string) {
	if line == r.lastLine {
		return
	}
	r.lastLine = line
	fmt.Fprintln(r.w, line)
}

func syncHandled(p indexer.Progress) int {
	handled := p.Processed + p.Skipped + p.Failed + p.RequiresRebuild
	if handled > p.Inventoried {
		return p.Inventoried
	}
	return handled
}

func progressLine(state string, p indexer.Progress) string {
	return fmt.Sprintf("Sync: %s %d/%d sources (processed=%d skipped=%d failed=%d requires_rebuild=%d).", state, syncHandled(p), p.Inventoried, p.Processed, p.Skipped, p.Failed, p.RequiresRebuild)
}
