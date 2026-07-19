package reviews

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

type selectedTurn struct {
	id, sessionID, rootID, purpose, completedAt, model, reasoning, projectID, projectName string
	input, cached, output, reasoningOutput, total                                         *int64
}

const selectedSessionsCTE = `WITH sv AS (
 SELECT v.* FROM session_versions v WHERE v.epoch_id=? AND v.revision=(SELECT max(x.revision) FROM session_versions x WHERE x.epoch_id=v.epoch_id AND x.session_id=v.session_id AND x.revision<=?)
) `

func buildManifest(ctx context.Context, db *sql.DB, epoch string, revision int64, reviewID string, req PlanRequest, now time.Time) (Manifest, SourceByteCounts, IndexedCoverage, ProjectSummary, *int64, error) {
	turns, err := selectTurns(ctx, db, epoch, revision, req.Scope)
	if err != nil {
		return Manifest{}, SourceByteCounts{}, IndexedCoverage{}, ProjectSummary{}, nil, err
	}
	if len(turns) == 0 {
		return Manifest{}, SourceByteCounts{}, IndexedCoverage{}, ProjectSummary{}, nil, ErrScopeEmpty
	}
	sessionSet := map[string]bool{}
	turnSet := map[string]bool{}
	for _, t := range turns {
		sessionSet[t.sessionID] = true
		turnSet[t.id] = true
	}
	sessions := sortedKeys(sessionSet)
	turnIDs := sortedKeys(turnSet)
	evidence, sources, counts, err := selectEvidence(ctx, db, epoch, revision, sessionSet, turnSet)
	if err != nil {
		return Manifest{}, SourceByteCounts{}, IndexedCoverage{}, ProjectSummary{}, nil, err
	}
	if len(evidence) > 10000 {
		return Manifest{}, SourceByteCounts{}, IndexedCoverage{}, ProjectSummary{}, nil, ErrScopeTooLarge
	}
	if len(evidence) == 0 || len(sources) == 0 {
		return Manifest{}, SourceByteCounts{}, IndexedCoverage{}, ProjectSummary{}, nil, ErrScopeEmpty
	}
	scope := ManifestScope{Kind: req.Scope.Kind, AppliedRevision: revision}
	if req.Scope.Kind == "single_session" {
		scope.RootSessionID = req.Scope.RootSessionID
		for _, id := range sessions {
			if id != req.Scope.RootSessionID {
				scope.DescendantSessionIDs = append(scope.DescendantSessionIDs, id)
			}
		}
	} else {
		scope.Start = req.Scope.Start
		scope.End = req.Scope.End
		scope.Timezone = req.Scope.Timezone
		scope.ProjectID = req.Scope.ProjectID
	}
	agg, gaps := aggregateMetrics(turns)
	latestCapacity, drawdown, capacityGap, err := capacityMetrics(ctx, db, epoch, revision, req.Scope)
	if err != nil {
		return Manifest{}, SourceByteCounts{}, IndexedCoverage{}, ProjectSummary{}, nil, err
	}
	agg["latest_capacity_observation"] = latestCapacity
	agg["capacity_drawdown"] = drawdown
	if capacityGap != "" {
		gaps = append(gaps, capacityGap)
	}
	manifest := Manifest{SchemaVersion: SchemaVersion, ReviewID: reviewID, CreatedAt: now.Format(time.RFC3339Nano), DatasetEpoch: epoch, IndexRevision: revision, Scope: scope, IncludedSessionIDs: sessions, IncludedTurnIDs: turnIDs, Sources: sources, AggregateMetrics: agg, CoverageGaps: gaps, EvidenceRules: map[string]any{"citationShape": "manifest_evidence_id", "treatAsUntrusted": true}, Rubric: append([]string(nil), Rubric...), Focus: req.Focus, Model: req.Model, Reasoning: req.ReasoningEffort, Evidence: evidence, ReportDestination: "./review.json", ReportSchema: "./report.schema.json", Limits: map[string]int{"maxFindings": 5, "maxReportBytes": 1048576}}
	coverage := timeCoverage(turns)
	projects := projectSummary(turns)
	var estimate *int64
	if counts.IncludedBytes > 0 {
		x := counts.IncludedBytes / 4
		if x < 1 {
			x = 1
		}
		estimate = &x
	}
	return manifest, counts, coverage, projects, estimate, nil
}

func sourceInventoryCoverage(ctx context.Context, store *storage.Store, epoch string, revision int64, sessionIDs []string) (Coverage, []string, error) {
	inventory, err := store.SourceInventoryAt(ctx, epoch, revision, sessionIDs)
	if err != nil {
		return Coverage{}, nil, fmt.Errorf("select source inventory coverage: %w", err)
	}
	coverage := Coverage{Fidelity: "exact", Observed: inventory.Current, Eligible: inventory.Total}
	gaps := []string{}
	for _, item := range []struct {
		count   int
		message string
	}{
		{inventory.Discovered, "%d discovered source(s) had not started indexing at the applied revision; their records may be absent from the plan."},
		{inventory.Supported, "%d supported source(s) had not completed indexing at the applied revision; their records may be absent from the plan."},
		{inventory.Indexing, "%d source(s) were still indexing at the applied revision; only committed indexed evidence was included."},
		{inventory.Unsupported, "%d discovered source(s) were unsupported; unsupported records were not sampled into the plan."},
		{inventory.Failed, "%d source(s) had failed indexing; unavailable records were not included."},
		{inventory.RequiresRebuild, "%d source(s) required a rebuild; the plan remained pinned to previously committed facts and identifies this gap."},
		{inventory.Missing, "%d previously discovered source(s) were missing at the applied revision; preserved citations may be unavailable."},
	} {
		if item.count > 0 {
			gaps = append(gaps, fmt.Sprintf(item.message, item.count))
		}
	}
	if len(gaps) > 0 {
		coverage.Fidelity = "derived"
		coverage.Reason = fmt.Sprintf("%d of %d discovered rollout sources were completely indexed at revision %d; unavailable sources may contain additional eligible work.", coverage.Observed, coverage.Eligible, revision)
	}
	return coverage, gaps, nil
}

func capacityMetrics(ctx context.Context, db *sql.DB, epoch string, revision int64, scope Scope) (json.RawMessage, json.RawMessage, string, error) {
	type point struct {
		ObservedAt, LimitID string
		Window              int64
		Used                float64
		Remaining           *float64
		Resets              int64
	}
	read := func(where string, args ...any) ([]point, error) {
		q := `SELECT c.observed_at,c.limit_id,c.window_minutes,c.used_percent,c.remaining_percent,c.resets_at FROM capacity_observations c JOIN events e ON e.epoch_id=c.epoch_id AND e.id=c.event_id WHERE c.epoch_id=? AND e.commit_revision<=? AND c.window_minutes IS NOT NULL AND c.used_percent IS NOT NULL AND c.resets_at IS NOT NULL` + where + ` ORDER BY c.observed_at,c.id LIMIT 2001`
		all := []any{epoch, revision}
		all = append(all, args...)
		rows, err := db.QueryContext(ctx, q, all...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		out := []point{}
		for rows.Next() {
			var p point
			var remaining sql.NullFloat64
			var resets string
			if err = rows.Scan(&p.ObservedAt, &p.LimitID, &p.Window, &p.Used, &remaining, &resets); err != nil {
				return nil, err
			}
			parsed, e := time.Parse(time.RFC3339Nano, resets)
			if e != nil {
				continue
			}
			p.Resets = parsed.Unix()
			if remaining.Valid {
				x := remaining.Float64
				p.Remaining = &x
			}
			out = append(out, p)
		}
		return out, rows.Err()
	}
	latestRows, err := read("")
	if err != nil {
		return nil, nil, "", err
	}
	drawRows := latestRows
	if scope.Kind == "time_period" {
		drawRows, err = read(` AND c.observed_at>=? AND c.observed_at<?`, scope.Start, scope.End)
		if err != nil {
			return nil, nil, "", err
		}
	}
	if len(latestRows) > 2000 || len(drawRows) > 2000 {
		return nil, nil, "", ErrScopeTooLarge
	}
	value := func(p point) map[string]any {
		m := map[string]any{"observedAt": p.ObservedAt, "limitId": p.LimitID, "windowMinutes": p.Window, "usedPercent": p.Used, "resetsAt": p.Resets}
		if p.Remaining != nil {
			m["remainingPercent"] = *p.Remaining
		}
		return m
	}
	metric := func(f string, v any) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"formulaVersion": 1, "fidelity": f, "value": v})
		return b
	}
	latest := metric("unavailable", nil)
	gap := ""
	if len(latestRows) > 0 {
		latest = metric("exact", value(latestRows[len(latestRows)-1]))
	} else {
		gap = "No supported recorded capacity observation is available for this Review manifest."
	}
	draw := make([]map[string]any, 0, len(drawRows))
	for _, p := range drawRows {
		draw = append(draw, value(p))
	}
	drawFidelity := "exact"
	if len(draw) == 0 {
		drawFidelity = "unavailable"
	}
	return latest, metric(drawFidelity, draw), gap, nil
}

func selectTurns(ctx context.Context, db *sql.DB, epoch string, revision int64, scope Scope) ([]selectedTurn, error) {
	query := selectedSessionsCTE + `SELECT t.id,t.session_id,sv.root_work_unit_id,sv.purpose,t.completed_at,coalesce(t.model,''),coalesce(t.reasoning_effort,''),coalesce(rsv.project_id,''),coalesce(p.display_name,''),u.input_tokens,u.cached_input_tokens,u.output_tokens,u.reasoning_output_tokens,u.total_tokens
 FROM sv JOIN turns t ON t.epoch_id=sv.epoch_id AND t.session_id=sv.session_id AND t.state='completed' AND t.commit_revision<=?
	JOIN sv rsv ON rsv.epoch_id=sv.epoch_id AND rsv.session_id=sv.root_work_unit_id
 LEFT JOIN turn_usage u ON u.epoch_id=t.epoch_id AND u.turn_id=t.id AND u.formula_version=1
	LEFT JOIN projects p ON p.epoch_id=rsv.epoch_id AND p.id=rsv.project_id
 WHERE sv.epoch_id=? AND sv.purpose<>'inspector_review'`
	args := []any{epoch, revision, revision, epoch}
	if scope.Kind == "single_session" {
		query += ` AND sv.root_work_unit_id=?`
		args = append(args, scope.RootSessionID)
	} else {
		query += ` AND t.completed_at>=? AND t.completed_at<?`
		args = append(args, scope.Start, scope.End)
		if scope.ProjectID != "" {
			query += ` AND rsv.project_id=?`
			args = append(args, scope.ProjectID)
		}
	}
	query += ` ORDER BY t.completed_at,t.source_order_key,t.id`
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select turns: %w", err)
	}
	defer rows.Close()
	out := []selectedTurn{}
	for rows.Next() {
		var t selectedTurn
		var in, ca, ou, rea, tot sql.NullInt64
		if err = rows.Scan(&t.id, &t.sessionID, &t.rootID, &t.purpose, &t.completedAt, &t.model, &t.reasoning, &t.projectID, &t.projectName, &in, &ca, &ou, &rea, &tot); err != nil {
			return nil, err
		}
		t.input = nullable(in)
		t.cached = nullable(ca)
		t.output = nullable(ou)
		t.reasoningOutput = nullable(rea)
		t.total = nullable(tot)
		out = append(out, t)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if scope.Kind == "single_session" {
		var purpose string
		err = db.QueryRowContext(ctx, selectedSessionsCTE+`SELECT purpose FROM sv WHERE epoch_id=? AND session_id=? AND root_work_unit_id=session_id`, epoch, revision, epoch, scope.RootSessionID).Scan(&purpose)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrScopeEmpty
		}
		if err != nil {
			return nil, fmt.Errorf("select root purpose: %w", err)
		}
		if purpose != "user" {
			return nil, ErrInvalidRequest
		}
	}
	return out, nil
}
func nullable(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	x := v.Int64
	return &x
}

func selectEvidence(ctx context.Context, db *sql.DB, epoch string, revision int64, sessions, turns map[string]bool) ([]ManifestEvidence, []ManifestSource, SourceByteCounts, error) {
	turnIDs := sortedKeys(turns)
	if len(turnIDs) == 0 {
		return nil, nil, SourceByteCounts{}, nil
	}
	place := strings.TrimRight(strings.Repeat("?,", len(turnIDs)), ",")
	args := []any{epoch, revision}
	for _, id := range turnIDs {
		args = append(args, id)
	}
	q := `SELECT r.id,r.source_id,r.source_prefix_sha256,r.event_fingerprint,e.adapter_version,e.record_ordinal,e.byte_start,e.byte_end,t.session_id,t.id,e.event_kind,e.observed_at,
 coalesce((SELECT a.availability FROM evidence_availability_versions a WHERE a.epoch_id=r.epoch_id AND a.evidence_id=r.id ORDER BY a.observed_at DESC LIMIT 1),'unreadable'),
 coalesce((SELECT a.observed_at FROM evidence_availability_versions a WHERE a.epoch_id=r.epoch_id AND a.evidence_id=r.id ORDER BY a.observed_at DESC LIMIT 1),e.observed_at),
 (SELECT a.availability_revision FROM evidence_availability_versions a WHERE a.epoch_id=r.epoch_id AND a.evidence_id=r.id ORDER BY a.observed_at DESC LIMIT 1)
 FROM evidence_refs r JOIN events e ON e.epoch_id=r.epoch_id AND e.id=r.event_id JOIN turns t ON t.epoch_id=e.epoch_id AND t.id=e.turn_id
 WHERE r.epoch_id=? AND e.commit_revision<=? AND t.id IN (` + place + `) ORDER BY t.completed_at,e.record_ordinal,e.semantic_phase`
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, SourceByteCounts{}, fmt.Errorf("select evidence: %w", err)
	}
	defer rows.Close()
	ev := []ManifestEvidence{}
	sourceSet := map[string]bool{}
	for rows.Next() {
		var x ManifestEvidence
		var ar sql.NullInt64
		if err = rows.Scan(&x.EvidenceID, &x.SourceID, &x.SourcePrefixSHA256, &x.EventFingerprint, &x.AdapterVersion, &x.RecordOrdinal, &x.ByteStart, &x.ByteEnd, &x.SessionID, &x.TurnID, &x.EventKind, &x.ObservedAt, &x.Availability, &x.AvailabilityObservedAt, &ar); err != nil {
			return nil, nil, SourceByteCounts{}, err
		}
		if ar.Valid {
			x.AvailabilityRevision = &ar.Int64
		}
		ev = append(ev, x)
		sourceSet[x.SourceID] = true
	}
	if err = rows.Err(); err != nil {
		return nil, nil, SourceByteCounts{}, err
	}
	sourceIDs := sortedKeys(sourceSet)
	sources := []ManifestSource{}
	counts := SourceByteCounts{SourceCount: len(sourceIDs)}
	for _, id := range sourceIDs {
		var s ManifestSource
		var size, indexed int64
		err = db.QueryRowContext(ctx, `SELECT v.source_kind,v.canonical_path,a.segment_fingerprint,v.byte_size,coalesce(c.complete_byte_offset,0) FROM source_artifacts a JOIN source_artifact_versions v ON v.epoch_id=a.epoch_id AND v.source_id=a.id LEFT JOIN source_checkpoints c ON c.epoch_id=a.epoch_id AND c.source_id=a.id WHERE a.epoch_id=? AND a.id=? AND v.revision=(SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id AND x.revision<=?)`, epoch, id, revision).Scan(&s.SourceKind, &s.Locator, &s.Fingerprint, &size, &indexed)
		if err != nil {
			return nil, nil, SourceByteCounts{}, fmt.Errorf("select source: %w", err)
		}
		s.SourceID = id
		sources = append(sources, s)
		counts.DiscoveredBytes += size
		counts.IndexedBytes += indexed
		counts.IncludedBytes += indexed
	}
	return ev, sources, counts, nil
}

func aggregateMetrics(turns []selectedTurn) (map[string]json.RawMessage, []string) {
	type kindTotals struct {
		UserRootDirect  int64 `json:"user_root_direct"`
		Descendant      int64 `json:"descendant"`
		InspectorReview int64 `json:"inspector_review"`
		OtherOrphan     int64 `json:"other_orphan"`
	}
	type comp struct {
		Uncached  int64 `json:"uncached_input"`
		Cached    int64 `json:"cached_input"`
		Visible   int64 `json:"visible_output"`
		Reasoning int64 `json:"reasoning_output"`
		Residual  int64 `json:"residual"`
	}
	totals := kindTotals{}
	composition := comp{}
	observed := 0
	total := int64(0)
	gaps := []string{}
	rootMap := map[string]*struct{ Inclusive, Direct, Desc int64 }{}
	over := []map[string]any{}
	for _, t := range turns {
		if t.total == nil {
			continue
		}
		observed++
		total += *t.total
		kind := "other_orphan"
		if t.purpose == "user" && t.sessionID == t.rootID {
			kind = "user_root_direct"
		} else if t.purpose == "spawned" {
			kind = "descendant"
		}
		switch kind {
		case "user_root_direct":
			totals.UserRootDirect += *t.total
		case "descendant":
			totals.Descendant += *t.total
		case "inspector_review":
			totals.InspectorReview += *t.total
		default:
			totals.OtherOrphan += *t.total
		}
		r := rootMap[t.rootID]
		if r == nil {
			r = &struct{ Inclusive, Direct, Desc int64 }{}
			rootMap[t.rootID] = r
		}
		r.Inclusive += *t.total
		if kind == "descendant" {
			r.Desc += *t.total
		} else {
			r.Direct += *t.total
		}
		kt := kindTotals{}
		switch kind {
		case "user_root_direct":
			kt.UserRootDirect = *t.total
		case "descendant":
			kt.Descendant = *t.total
		default:
			kt.OtherOrphan = *t.total
		}
		over = append(over, map[string]any{"start": t.completedAt, "byKind": kt})
		if t.input != nil && t.cached != nil && t.output != nil && t.reasoningOutput != nil && *t.input >= *t.cached && *t.output >= *t.reasoningOutput {
			composition.Uncached += *t.input - *t.cached
			composition.Cached += *t.cached
			composition.Visible += *t.output - *t.reasoningOutput
			composition.Reasoning += *t.reasoningOutput
			sum := *t.input + *t.output
			if *t.total > sum {
				composition.Residual += *t.total - sum
			}
		}
	}
	fidelity := "exact"
	if observed < len(turns) {
		fidelity = "derived"
		gaps = append(gaps, fmt.Sprintf("Recorded token usage is available for %d of %d included completed turns.", observed, len(turns)))
	}
	metric := func(f string, v any) json.RawMessage {
		b, _ := json.Marshal(map[string]any{"formulaVersion": 1, "fidelity": f, "value": v})
		return b
	}
	var tokenValue any = total
	if observed == 0 {
		fidelity = "unavailable"
		tokenValue = nil
	}
	roots := []map[string]any{}
	for id, r := range rootMap {
		roots = append(roots, map[string]any{"rootSessionId": id, "inclusiveTokens": r.Inclusive, "directTokens": r.Direct, "descendantTokens": r.Desc})
	}
	sort.Slice(roots, func(i, j int) bool { return roots[i]["inclusiveTokens"].(int64) > roots[j]["inclusiveTokens"].(int64) })
	return map[string]json.RawMessage{"recorded_tokens": metric(fidelity, tokenValue), "recorded_tokens_by_kind": metric(fidelity, totals), "recorded_tokens_over_time": metric(fidelity, over), "token_composition": metric(fidelity, composition), "top_root_sessions_by_tokens": metric(fidelity, roots), "latest_capacity_observation": metric("unavailable", nil), "capacity_drawdown": metric("unavailable", []any{})}, gaps
}

func timeCoverage(turns []selectedTurn) IndexedCoverage {
	if len(turns) == 0 {
		return IndexedCoverage{}
	}
	a, b := turns[0].completedAt, turns[len(turns)-1].completedAt
	return IndexedCoverage{&a, &b, &b}
}
func projectSummary(turns []selectedTurn) ProjectSummary {
	type p struct {
		name            string
		sessions, turns map[string]bool
	}
	m := map[string]*p{}
	for _, t := range turns {
		if t.projectID == "" {
			continue
		}
		x := m[t.projectID]
		if x == nil {
			x = &p{t.projectName, map[string]bool{}, map[string]bool{}}
			m[t.projectID] = x
		}
		x.sessions[t.sessionID] = true
		x.turns[t.id] = true
	}
	out := ProjectSummary{Projects: []ProjectSummaryItem{}}
	for id, x := range m {
		out.Projects = append(out.Projects, ProjectSummaryItem{id, x.name, len(x.sessions), len(x.turns)})
	}
	sort.Slice(out.Projects, func(i, j int) bool { return out.Projects[i].DisplayName < out.Projects[j].DisplayName })
	out.ProjectCount = len(out.Projects)
	return out
}
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var _ = storage.AdapterVersion
