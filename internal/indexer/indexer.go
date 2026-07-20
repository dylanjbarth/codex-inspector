package indexer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/facts"
	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/hook"
	proc "github.com/dylanjbarth/codex-inspector/internal/process"
	"github.com/dylanjbarth/codex-inspector/internal/sources"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

type Progress struct {
	Inventoried, Processed, Skipped, Failed, RequiresRebuild, QueueConsumed int
	Boundary                                                                string
	FailureReasons                                                          map[string]int
}

type Config struct {
	Layout      home.Layout
	CodexHome   string
	Concurrency int
	OnCommit    func(Progress)
}
type spawningProof struct{ parentSessionID, turnID string }

func Run(ctx context.Context, cfg Config) (Progress, error) {
	if cfg.CodexHome == "" {
		var err error
		cfg.CodexHome, err = sources.CodexHome()
		if err != nil {
			return Progress{}, err
		}
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = runtime.NumCPU()
		if cfg.Concurrency > 4 {
			cfg.Concurrency = 4
		}
		if cfg.Concurrency < 1 {
			cfg.Concurrency = 1
		}
	}
	if err := home.Ensure(cfg.Layout); err != nil {
		return Progress{}, err
	}
	lock, err := proc.Acquire(filepath.Join(cfg.Layout.Run, "writer.lock"), true)
	if err != nil {
		return Progress{}, errors.New("another Inspector writer is active")
	}
	defer lock.Close()
	candidates, err := sources.Discover(cfg.CodexHome, cfg.Layout.Root)
	if err != nil {
		return Progress{}, err
	}
	markers, markerPaths, terminalProofs, spawningProofs := readMarkers(cfg.Layout.Queue)
	prioritize(candidates, markers)
	p := Progress{Inventoried: len(candidates), FailureReasons: map[string]int{}}
	var firstFailure error
	rebuildRequested := false
	processedSessions := map[string]string{}
	labels := map[string]string{}
	for _, c := range candidates {
		if c.Kind == "session_index" {
			if got, e := sources.ReadSessionLabels(c.Path); e == nil {
				labels = got
			}
		}
	}
	indexPath := filepath.Join(cfg.Layout.Root, "inspector.db")
	store, err := storage.Open(indexPath)
	if errors.Is(err, storage.ErrSchemaV1RequiresRebuild) {
		batches, buildErr := inventoryBatches(ctx, candidates, labels, terminalProofs, spawningProofs, cfg.Layout.Reviews)
		if buildErr != nil {
			return p, buildErr
		}
		if buildErr = storage.BuildAndActivate(ctx, indexPath, batches); buildErr != nil {
			return p, buildErr
		}
		store, err = storage.Open(indexPath)
		if err == nil {
			p.Processed = p.Inventoried
			for _, markerPath := range markerPaths {
				if os.Remove(markerPath) == nil {
					p.QueueConsumed++
				}
			}
			store.Close()
			notify(cfg, p)
			return p, nil
		}
	}
	if err != nil {
		return Progress{}, err
	}
	defer store.Close()
	for _, c := range candidates {
		if c.Kind == "session_index" {
			batch, e := sources.ParseSessionIndex(c)
			if e != nil {
				p.Failed++
				notify(cfg, p)
				continue
			}
			checkpoint, e := store.Checkpoint(batch.Source.ID)
			if e == nil && batch.Source.Size == checkpoint.Size && batch.Source.PrefixSHA256 == checkpoint.Prefix && checkpoint.Path == c.Path {
				p.Skipped++
				notify(cfg, p)
				continue
			}
			if e == nil && (batch.Source.Size < checkpoint.Offset || !prefixMatches(c.Path, checkpoint.Offset, checkpoint.Prefix)) {
				_ = store.MarkRequiresRebuild(batch.Source.ID, "indexed_prefix_changed_or_shrank")
				p.RequiresRebuild++
				rebuildRequested = true
				notify(cfg, p)
				continue
			}
			chunks, splitErr := splitBatches(batch)
			if splitErr != nil {
				e = splitErr
			}
			for _, chunk := range chunks {
				if _, e = store.Apply(ctx, chunk, "session_index_labels"); e != nil {
					break
				}
			}
			if e != nil {
				p.Failed++
			} else {
				p.Processed++
			}
			notify(cfg, p)
		}
	}
	rollouts := make([]sources.Candidate, 0, len(candidates))
	for _, c := range candidates {
		if c.Kind != "session_index" {
			rollouts = append(rollouts, c)
		}
	}
	type result struct {
		index int
		batch facts.Batch
		err   error
	}
	jobs := make(chan int)
	results := make(chan result, cfg.Concurrency)
	var wg sync.WaitGroup
	for i := 0; i < cfg.Concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				b, e := sources.ParseContextWithProofs(ctx, rollouts[idx], labels, terminalProofs)
				results <- result{idx, b, e}
			}
		}()
	}
	go func() {
		defer close(results)
		for i := range rollouts {
			select {
			case jobs <- i:
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				return
			}
		}
		close(jobs)
		wg.Wait()
	}()
	next := 0
	pending := map[int]result{}
	for r := range results {
		pending[r.index] = r
		for {
			item, ok := pending[next]
			if !ok {
				break
			}
			delete(pending, next)
			candidate := rollouts[next]
			applySpawningProof(&item.batch, spawningProofs)
			classifyReview(&item.batch, cfg.Layout.Reviews)
			next++
			if item.err != nil {
				p.FailureReasons["parse_failed"]++
				if firstFailure == nil {
					firstFailure = item.err
				}
				_ = storeFailed(ctx, store, candidate, "parse_failed")
				p.Failed++
				notify(cfg, p)
				continue
			}
			if candidate.MTimeNS > 0 {
				p.Boundary = time.Unix(0, candidate.MTimeNS).UTC().Format(time.RFC3339Nano)
			}
			checkpoint, e := store.Checkpoint(item.batch.Source.ID)
			if e == nil {
				if item.batch.Source.Size < checkpoint.Offset || !prefixMatches(candidate.Path, checkpoint.Offset, checkpoint.Prefix) {
					_ = store.MarkRequiresRebuild(item.batch.Source.ID, "indexed_prefix_changed_or_shrank")
					p.RequiresRebuild++
					rebuildRequested = true
					notify(cfg, p)
					continue
				}
				if item.batch.Source.Size == checkpoint.Size && item.batch.Source.PrefixSHA256 == checkpoint.Prefix && checkpoint.Path == candidate.Path && checkpoint.State != "requires_rebuild" {
					p.Skipped++
					notify(cfg, p)
					continue
				}
			} else if !errors.Is(e, sql.ErrNoRows) {
				p.Failed++
				notify(cfg, p)
				continue
			}
			if item.batch.Session != nil {
				sessionKey := item.batch.Session.SourceSessionID
				if priorSource := processedSessions[sessionKey]; (priorSource != "" && priorSource != item.batch.Source.ID) || store.HasOtherSessionSegment(sessionKey, item.batch.Source.ID) {
					rebuildRequested = true
				}
				processedSessions[sessionKey] = item.batch.Source.ID
			}
			chunks, splitErr := splitBatches(item.batch)
			if splitErr != nil {
				e = splitErr
			}
			for _, chunk := range chunks {
				if _, e = store.Apply(ctx, chunk, "normalize_source"); e != nil {
					break
				}
			}
			if e != nil {
				p.FailureReasons[classifyFailure(e)]++
				if firstFailure == nil {
					firstFailure = e
				}
				_ = storeFailed(ctx, store, candidate, "normalization_failed")
				p.Failed++
				notify(cfg, p)
				continue
			}
			p.Processed++
			notify(cfg, p)
		}
	}
	if p.Failed == 0 && rebuildRequested {
		if err := rebuildInventory(ctx, store, candidates, labels, terminalProofs, spawningProofs, cfg.Layout.Reviews); err != nil {
			p.Failed++
			p.FailureReasons["rebuild_failed"]++
			if firstFailure == nil {
				firstFailure = err
			}
		} else {
			p.Processed = p.Inventoried
			p.Skipped = 0
			p.RequiresRebuild = 0
		}
	} else if err := store.ReconcileLineage(ctx); err != nil {
		p.Failed++
		if firstFailure == nil {
			firstFailure = err
		}
	}
	if p.Failed == 0 {
		for _, path := range markerPaths {
			if os.Remove(path) == nil {
				p.QueueConsumed++
			}
		}
	}
	if p.Failed > 0 {
		notify(cfg, p)
		return p, fmt.Errorf("%d sources failed: %v", p.Failed, firstFailure)
	}
	notify(cfg, p)
	return p, nil
}

func rebuildInventory(ctx context.Context, store *storage.Store, candidates []sources.Candidate, labels map[string]string, proofs map[string]map[string]sources.TerminalProof, spawning map[string]spawningProof, reviewsRoot string) error {
	batches, err := inventoryBatches(ctx, candidates, labels, proofs, spawning, reviewsRoot)
	if err != nil {
		return err
	}
	return store.Rebuild(ctx, batches)
}

func inventoryBatches(ctx context.Context, candidates []sources.Candidate, labels map[string]string, proofs map[string]map[string]sources.TerminalProof, spawning map[string]spawningProof, reviewsRoot string) ([]facts.Batch, error) {
	rawBatches := make([]facts.Batch, 0, len(candidates))
	for _, candidate := range candidates {
		var batch facts.Batch
		var err error
		if candidate.Kind == "session_index" {
			batch, err = sources.ParseSessionIndex(candidate)
		} else {
			batch, err = sources.ParseContextWithProofs(ctx, candidate, labels, proofs)
			applySpawningProof(&batch, spawning)
			classifyReview(&batch, reviewsRoot)
		}
		if err != nil {
			return nil, fmt.Errorf("rebuild parse %s: %w", candidate.Path, err)
		}
		rawBatches = append(rawBatches, batch)
	}
	if err := normalizeResumedSessions(rawBatches); err != nil {
		return nil, err
	}
	batches := make([]facts.Batch, 0, len(rawBatches))
	for _, batch := range rawBatches {
		chunks, err := splitBatches(batch)
		if err != nil {
			return nil, err
		}
		batches = append(batches, chunks...)
	}
	return batches, nil
}

func normalizeResumedSessions(batches []facts.Batch) error {
	sort.SliceStable(batches, func(i, j int) bool {
		if batches[i].Session == nil || batches[j].Session == nil {
			return batches[i].Source.Kind == "session_index"
		}
		if batches[i].Session.SourceSessionID == batches[j].Session.SourceSessionID {
			if batches[i].Session.StartedAt == batches[j].Session.StartedAt {
				return batches[i].Source.SegmentFingerprint < batches[j].Source.SegmentFingerprint
			}
			return batches[i].Session.StartedAt < batches[j].Session.StartedAt
		}
		if batches[i].Session.StartedAt == batches[j].Session.StartedAt {
			return batches[i].Session.SourceSessionID < batches[j].Session.SourceSessionID
		}
		return batches[i].Session.StartedAt < batches[j].Session.StartedAt
	})
	type bounds struct{ start, end string }
	bySession := map[string]bounds{}
	for i := range batches {
		if batches[i].Session == nil {
			continue
		}
		id := batches[i].Session.SourceSessionID
		current := bySession[id]
		if current.start == "" || batches[i].Session.StartedAt < current.start {
			current.start = batches[i].Session.StartedAt
		}
		if batches[i].Session.EndedAt > current.end {
			current.end = batches[i].Session.EndedAt
		}
		bySession[id] = current
	}
	baseline := map[string]*facts.Usage{}
	for i := range batches {
		if batches[i].Session == nil {
			continue
		}
		id := batches[i].Session.SourceSessionID
		batches[i].Session.StartedAt = bySession[id].start
		batches[i].Session.EndedAt = bySession[id].end
		for turnIndex := range batches[i].Turns {
			turn := &batches[i].Turns[turnIndex]
			if turn.Usage == nil && turn.Cumulative != nil && baseline[id] != nil {
				delta, ok := subtractUsage(*turn.Cumulative, *baseline[id])
				if !ok {
					return fmt.Errorf("session %s has decreasing cumulative usage", id)
				}
				turn.Usage = &delta
				turn.NormalizationKind = "cumulative_delta"
				for coverageIndex := range batches[i].Coverage {
					coverage := &batches[i].Coverage[coverageIndex]
					if coverage.ScopeKind == "turn" && coverage.ScopeID == turn.ID {
						coverage.Fidelity, coverage.Observed, coverage.Reason = "exact", 1, ""
					}
				}
			}
			if turn.Cumulative != nil {
				copy := *turn.Cumulative
				baseline[id] = &copy
			}
		}
	}
	return nil
}

func subtractUsage(current, prior facts.Usage) (facts.Usage, bool) {
	sub := func(a, b *int64) (*int64, bool) {
		if a == nil || b == nil || *a < *b {
			return nil, false
		}
		value := *a - *b
		return &value, true
	}
	input, ok := sub(current.Input, prior.Input)
	if !ok {
		return facts.Usage{}, false
	}
	cached, ok := sub(current.CachedInput, prior.CachedInput)
	if !ok {
		return facts.Usage{}, false
	}
	output, ok := sub(current.Output, prior.Output)
	if !ok {
		return facts.Usage{}, false
	}
	reasoning, ok := sub(current.ReasoningOutput, prior.ReasoningOutput)
	if !ok {
		return facts.Usage{}, false
	}
	total, ok := sub(current.Total, prior.Total)
	if !ok {
		return facts.Usage{}, false
	}
	return facts.Usage{Input: input, CachedInput: cached, Output: output, ReasoningOutput: reasoning, Total: total}, true
}

func splitBatches(b facts.Batch) ([]facts.Batch, error) {
	if len(b.SessionLabels) > 500 {
		keys := make([]string, 0, len(b.SessionLabels))
		for k := range b.SessionLabels {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out []facts.Batch
		for start := 0; start < len(keys); start += 500 {
			end := min(start+500, len(keys))
			chunk := b
			chunk.SessionLabels = map[string]string{}
			for _, k := range keys[start:end] {
				chunk.SessionLabels[k] = b.SessionLabels[k]
			}
			if end < len(keys) {
				chunk.Source.CompleteOffset = 0
				chunk.Source.CompleteOrdinal = 0
				chunk.Source.PrefixSHA256 = emptySHA256
				chunk.Source.PendingTail = 1
			}
			out = append(out, chunk)
		}
		return out, nil
	}
	if len(b.Events) == 0 {
		return []facts.Batch{b}, nil
	}
	byEvidence := map[string]facts.Evidence{}
	for _, e := range b.Evidence {
		byEvidence[e.EventID] = e
	}
	// Preflight terminal-turn units before exposing any revision. A turn and all
	// of its supporting facts are never split across commits. The 500-event
	// target may be exceeded by one bounded turn, but no unit may exceed the
	// accepted 8 MiB parsed-input transaction limit.
	var units [][2]int
	start := 0
	for i, e := range b.Events {
		if e.Kind == "task_complete" || e.Kind == "turn_aborted" || e.Kind == "turn_interrupted" {
			units = append(units, [2]int{start, i + 1})
			start = i + 1
		}
	}
	if start < len(b.Events) {
		if len(units) == 0 {
			units = append(units, [2]int{start, len(b.Events)})
		} else {
			units[len(units)-1][1] = len(b.Events)
		}
	}
	var ranges [][2]int
	start, count := 0, 0
	var bytes int64
	for _, unit := range units {
		var unitBytes int64
		for _, event := range b.Events[unit[0]:unit[1]] {
			unitBytes += event.PayloadLength
		}
		if unitBytes > 8<<20 {
			return nil, errors.New("terminal turn exceeds 8 MiB transaction bound")
		}
		unitCount := unit[1] - unit[0]
		if count > 0 && (count+unitCount > 500 || bytes+unitBytes > 8<<20) {
			ranges = append(ranges, [2]int{start, unit[0]})
			start, count, bytes = unit[0], 0, 0
		}
		count += unitCount
		bytes += unitBytes
	}
	ranges = append(ranges, [2]int{start, len(b.Events)})
	if len(ranges) == 1 {
		return []facts.Batch{b}, nil
	}
	var out []facts.Batch
	for ri, r := range ranges {
		ids := map[string]bool{}
		chunk := facts.Batch{Source: b.Source}
		if ri == 0 {
			chunk.Project = b.Project
			chunk.Lineage = b.Lineage
		}
		chunk.Session = b.Session
		chunk.Segment = b.Segment
		chunk.Events = append([]facts.Event(nil), b.Events[r[0]:r[1]]...)
		turnIDs := map[string]bool{}
		for _, e := range chunk.Events {
			ids[e.ID] = true
			if e.TurnID != "" {
				turnIDs[e.TurnID] = true
			}
		}
		for _, turn := range b.Turns {
			if turnIDs[turn.ID] {
				chunk.Turns = append(chunk.Turns, turn)
			}
		}
		for _, coverage := range b.Coverage {
			if (coverage.ScopeKind == "turn" && turnIDs[coverage.ScopeID]) || (ri == 0 && coverage.ScopeKind != "turn") {
				chunk.Coverage = append(chunk.Coverage, coverage)
			}
		}
		for _, v := range b.Messages {
			if ids[v.EventID] {
				chunk.Messages = append(chunk.Messages, v)
			}
		}
		for _, v := range b.Tools {
			if ids[v.EventID] {
				chunk.Tools = append(chunk.Tools, v)
			}
		}
		for _, v := range b.Capacity {
			if ids[v.EventID] {
				chunk.Capacity = append(chunk.Capacity, v)
			}
		}
		for _, v := range b.Compactions {
			if ids[v.EventID] {
				chunk.Compactions = append(chunk.Compactions, v)
			}
		}
		for _, v := range b.Evidence {
			if ids[v.EventID] {
				chunk.Evidence = append(chunk.Evidence, v)
			}
		}
		if ri < len(ranges)-1 {
			last := chunk.Events[len(chunk.Events)-1]
			chunk.Source.CompleteOrdinal = last.RecordOrdinal + 1
			chunk.Source.CompleteOffset = last.ByteEnd
			if chunk.Source.CompleteOffset < b.Source.CompleteOffset {
				chunk.Source.CompleteOffset++
			}
			if ev, ok := byEvidence[last.ID]; ok {
				chunk.Source.PrefixSHA256 = ev.SourceFingerprint
			}
			chunk.Source.PendingTail = 1
		}
		out = append(out, chunk)
	}
	return out, nil
}

const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

func classifyFailure(err error) string {
	s := strings.ToLower(err.Error())
	switch {
	case strings.Contains(s, "foreign key"):
		return "foreign_key_constraint"
	case strings.Contains(s, "unique"):
		return "identity_constraint"
	case strings.Contains(s, "check constraint"):
		return "fact_check_constraint"
	case strings.Contains(s, "locked") || strings.Contains(s, "busy"):
		return "storage_busy"
	default:
		return "normalization_failed"
	}
}
func storeFailed(ctx context.Context, store *storage.Store, c sources.Candidate, reason string) error {
	sum := sha256.Sum256([]byte(c.Kind + ":" + c.Path))
	id := hex.EncodeToString(sum[:])
	b := facts.Batch{Source: facts.Source{ID: "source:" + id, SessionID: "unresolved:" + id[:24], SegmentFingerprint: id, Kind: c.Kind, Path: c.Path, Size: c.Size, MTimeNS: c.MTimeNS, CompleteOffset: 0, CompleteOrdinal: 0, PrefixSHA256: emptySHA256, AdapterVersion: sources.AdapterVersion, State: "failed", StateReason: reason}}
	_, err := store.Apply(ctx, b, "source_failed")
	return err
}
func classifyReview(b *facts.Batch, reviewsRoot string) {
	if b.Session == nil || b.Project == nil {
		return
	}
	cwd := b.Project.Aliases["cwd"]
	if cwd == "" {
		return
	}
	root, er := filepath.Abs(reviewsRoot)
	path, ep := filepath.Abs(cwd)
	if er != nil || ep != nil {
		return
	}
	if canonical, e := filepath.EvalSymlinks(root); e == nil {
		root = canonical
	}
	if canonical, e := filepath.EvalSymlinks(path); e == nil {
		path = canonical
	}
	rel, e := filepath.Rel(root, path)
	if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		b.Session.Purpose = "inspector_review"
		if b.Session.RootWorkUnitID == "" {
			b.Session.RootWorkUnitID = b.Session.SourceSessionID
		}
	}
}

func notify(c Config, p Progress) {
	if c.OnCommit != nil {
		c.OnCommit(p)
	}
	runtime.Gosched()
}
func prefixMatches(path string, n int64, want string) bool {
	if n < 0 {
		return false
	}
	f, e := os.Open(path)
	if e != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	read, e := io.CopyN(h, f, n)
	if e != nil && !(errors.Is(e, io.EOF) && read == n) {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == want
}
func readMarkers(dir string) (map[string]bool, []string, map[string]map[string]sources.TerminalProof, map[string]spawningProof) {
	entries, _ := os.ReadDir(dir)
	ids := map[string]bool{}
	proofs := map[string]map[string]sources.TerminalProof{}
	spawning := map[string]spawningProof{}
	var paths []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, er := os.ReadFile(path)
		if er != nil || len(b) > 4096 {
			continue
		}
		m, er := hook.DecodeMarker(b)
		if er != nil {
			continue
		}
		ids[m.SessionID] = true
		if m.EventKind == "turn_stop" && m.TurnID != "" {
			if proofs[m.SessionID] == nil {
				proofs[m.SessionID] = map[string]sources.TerminalProof{}
			}
			proofs[m.SessionID][m.TurnID] = sources.TerminalProof{ObservedAt: m.ObservedAt}
		}
		if (m.EventKind == "subagent_start" || m.EventKind == "subagent_stop") && m.SubagentID != "" && m.TurnID != "" {
			spawning[m.SubagentID] = spawningProof{parentSessionID: m.SessionID, turnID: m.TurnID}
		}
		if m.TranscriptPath != nil {
			ids[filepath.Clean(*m.TranscriptPath)] = true
		}
		paths = append(paths, path)
	}
	return ids, paths, proofs, spawning
}

func applySpawningProof(batch *facts.Batch, proofs map[string]spawningProof) {
	if batch.Session == nil {
		return
	}
	proof, ok := proofs[batch.Session.SourceSessionID]
	if !ok {
		return
	}
	wantParent := "session:" + sha256Hex([]byte(proof.parentSessionID))
	for i := range batch.Lineage {
		if batch.Lineage[i].Kind == "spawned" && batch.Lineage[i].ParentSessionID == wantParent {
			batch.Lineage[i].SpawningTurnID = "turn:" + sha256Hex([]byte(proof.parentSessionID+":"+proof.turnID))
			batch.Coverage = append(batch.Coverage, facts.Coverage{ScopeKind: "session", ScopeID: batch.Session.ID, FieldKey: "spawning_turn_reference", Fidelity: "exact", Observed: 1, Eligible: 1, Reason: proof.turnID})
		}
	}
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
func prioritize(c []sources.Candidate, ids map[string]bool) {
	sort.SliceStable(c, func(i, j int) bool {
		im, jm := ids[c[i].Path], ids[c[j].Path]
		for id := range ids {
			im = im || strings.Contains(filepath.Base(c[i].Path), id)
			jm = jm || strings.Contains(filepath.Base(c[j].Path), id)
		}
		if im != jm {
			return im
		}
		if c[i].MTimeNS == c[j].MTimeNS {
			return c[i].Path < c[j].Path
		}
		return c[i].MTimeNS > c[j].MTimeNS
	})
}
