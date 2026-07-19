package metrics

import (
	"container/list"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

const FormulaVersion = 1
const MaxBuckets = 2000
const MaxCapacitySeries = 100
const MaxCapacityPoints = 2000
const MaxFilterProjects = 200
const MaxFilterModels = 100
const MaxFilterReasoningEfforts = 20

var ErrResponseTooLarge = errors.New("metric_response_too_large")

var Keys = []string{"recorded_tokens", "recorded_tokens_by_kind", "recorded_tokens_over_time", "token_composition", "top_root_sessions_by_tokens", "latest_capacity_observation", "capacity_drawdown"}

type Query struct {
	MetricKeys        []string `json:"metricKeys"`
	Timezone          string   `json:"timezone"`
	Grain             string   `json:"grain,omitempty"`
	Start             string   `json:"start,omitempty"`
	End               string   `json:"end,omitempty"`
	ProjectIDs        []string `json:"projectIds,omitempty"`
	Models            []string `json:"models,omitempty"`
	ReasoningEfforts  []string `json:"reasoningEfforts,omitempty"`
	ContributionKinds []string `json:"contributionKinds,omitempty"`
	RequestedRevision int64    `json:"requestedRevision,omitempty"`
}

type Result struct {
	SchemaVersion   int          `json:"schemaVersion"`
	DatasetEpoch    string       `json:"datasetEpoch"`
	AppliedRevision int64        `json:"appliedRevision"`
	Coverage        Coverage     `json:"coverage"`
	Results         []MetricItem `json:"results"`
}
type Coverage struct {
	Fidelity string `json:"fidelity"`
	Observed int    `json:"observed"`
	Eligible int    `json:"eligible"`
	Reason   string `json:"reason,omitempty"`
}
type IndexedCoverage struct{ IndexedStart, IndexedEnd, CompletedWatermark *string }
type TimeBoundary struct {
	RequestedStart, RequestedEnd, EffectiveStart, EffectiveEnd *string
	Timezone                                                   string
}
type Metadata struct {
	FormulaVersion   int
	Fidelity         string
	Coverage         Coverage
	IndexedCoverage  IndexedCoverage
	TimeBoundary     TimeBoundary
	ExclusionReasons map[string]int
}
type Totals struct{ UserRootDirect, Descendant, InspectorReview, OtherOrphan int64 }
type Composition struct{ UncachedInput, CachedInput, VisibleOutput, ReasoningOutput, Residual int64 }
type Bucket struct {
	BucketStart, BucketEnd, Timezone, Grain string
	ByKind                                  Totals
}
type Root struct {
	RootSessionID                                   string
	InclusiveTokens, DirectTokens, DescendantTokens int64
}
type CapacityPoint struct {
	ObservedAt, LimitID string
	WindowMinutes       int64
	UsedPercent         float64
	RemainingPercent    *float64
	ResetsAt            string
	Stale               bool
}
type CapacitySeries struct {
	LimitID       string
	WindowMinutes int64
	ResetBoundary string
	Points        []CapacityPoint
}
type MetricItem struct {
	Key      string
	Metadata Metadata
	Value    any
}

func (c Coverage) MarshalJSON() ([]byte, error) { type x Coverage; return json.Marshal(x(c)) }
func (i IndexedCoverage) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"indexedStart": i.IndexedStart, "indexedEnd": i.IndexedEnd, "completedWatermark": i.CompletedWatermark})
}
func (b TimeBoundary) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"requestedStart": b.RequestedStart, "requestedEnd": b.RequestedEnd, "effectiveStart": b.EffectiveStart, "effectiveEnd": b.EffectiveEnd, "timezone": b.Timezone})
}
func (m Metadata) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"formulaVersion": m.FormulaVersion, "fidelity": m.Fidelity, "coverage": m.Coverage, "indexedCoverage": m.IndexedCoverage, "timeBoundary": m.TimeBoundary, "exclusionReasons": m.ExclusionReasons})
}
func (t Totals) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]int64{"userRootDirect": t.UserRootDirect, "descendant": t.Descendant, "inspectorReview": t.InspectorReview, "otherOrphan": t.OtherOrphan})
}
func (c Composition) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]int64{"uncachedInput": c.UncachedInput, "cachedInput": c.CachedInput, "visibleOutput": c.VisibleOutput, "reasoningOutput": c.ReasoningOutput, "residual": c.Residual})
}
func (b Bucket) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"bucketStart": b.BucketStart, "bucketEnd": b.BucketEnd, "timezone": b.Timezone, "grain": b.Grain, "byKind": b.ByKind})
}
func (r Root) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"rootSessionId": r.RootSessionID, "inclusiveTokens": r.InclusiveTokens, "directTokens": r.DirectTokens, "descendantTokens": r.DescendantTokens})
}
func (p CapacityPoint) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"observedAt": p.ObservedAt, "limitId": p.LimitID, "windowMinutes": p.WindowMinutes, "usedPercent": p.UsedPercent, "remainingPercent": p.RemainingPercent, "resetsAt": p.ResetsAt, "stale": p.Stale})
}
func (s CapacitySeries) MarshalJSON() ([]byte, error) {
	return json.Marshal(map[string]any{"limitId": s.LimitID, "windowMinutes": s.WindowMinutes, "resetBoundary": s.ResetBoundary, "points": s.Points})
}
func (m MetricItem) MarshalJSON() ([]byte, error) {
	base := map[string]any{"key": m.Key, "value": m.Value}
	b, _ := json.Marshal(m.Metadata)
	_ = json.Unmarshal(b, &base)
	return json.Marshal(base)
}

type cached struct {
	key   string
	value Result
}
type Engine struct {
	mu    sync.Mutex
	max   int
	items map[string]*list.Element
	order *list.List
	now   func() time.Time
}

func New(max int) *Engine {
	if max < 1 {
		max = 64
	}
	return &Engine{max: max, items: map[string]*list.Element{}, order: list.New(), now: time.Now}
}

func (e *Engine) SetClockForTest(now func() time.Time) { e.now = now }

func (e *Engine) Catalog() map[string]any {
	dims := []string{"time", "project", "model", "reasoning", "contribution_kind"}
	out := make([]map[string]any, 0, len(Keys))
	for _, k := range Keys {
		unit := "tokens"
		d := dims
		if strings.Contains(k, "capacity") {
			unit = "percent"
			d = []string{"time"}
		}
		out = append(out, map[string]any{"key": k, "formulaVersion": 1, "unit": unit, "dimensions": d})
	}
	return map[string]any{"metrics": out}
}

func (e *Engine) Query(ctx context.Context, store *storage.Store, q Query) (Result, error) {
	if err := validate(&q); err != nil {
		return Result{}, err
	}
	epoch, latest, err := store.Snapshot()
	if err != nil {
		return Result{}, err
	}
	rev := q.RequestedRevision
	if rev == 0 {
		rev = latest
	}
	if rev < 1 || rev > latest {
		return Result{}, errors.New("revision_unavailable")
	}
	if err := validateRequestedBucketBound(q); err != nil {
		return Result{}, err
	}
	cacheable := !containsCapacity(q.MetricKeys)
	keyBytes, _ := json.Marshal(struct {
		E string
		R int64
		Q Query
	}{epoch, rev, q})
	key := string(keyBytes)
	e.mu.Lock()
	if cacheable {
		if el := e.items[key]; el != nil {
			e.order.MoveToFront(el)
			v := el.Value.(cached).value
			e.mu.Unlock()
			return v, nil
		}
	}
	e.mu.Unlock()
	r, err := calculate(ctx, store.DB(), epoch, rev, q, e.now())
	if err != nil {
		return Result{}, err
	}
	if !cacheable {
		return r, nil
	}
	e.mu.Lock()
	el := e.order.PushFront(cached{key, r})
	e.items[key] = el
	if e.order.Len() > e.max {
		old := e.order.Back()
		delete(e.items, old.Value.(cached).key)
		e.order.Remove(old)
	}
	e.mu.Unlock()
	return r, nil
}

func containsCapacity(keys []string) bool {
	for _, k := range keys {
		if k == "latest_capacity_observation" || k == "capacity_drawdown" {
			return true
		}
	}
	return false
}

func validateRequestedBucketBound(q Query) error {
	if q.Start == "" || q.End == "" {
		return nil
	}
	start, _ := time.Parse(time.RFC3339, q.Start)
	end, _ := time.Parse(time.RFC3339, q.End)
	loc, _ := time.LoadLocation(q.Timezone)
	cursor, _ := CalendarBucket(start.In(loc), q.Grain)
	for n := 0; cursor.Before(end); n++ {
		if n >= MaxBuckets {
			return ErrResponseTooLarge
		}
		_, cursor = CalendarBucket(cursor.Add(time.Nanosecond), q.Grain)
	}
	return nil
}

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
type FilterOptions struct {
	Projects          []Option `json:"projects"`
	Models            []string `json:"models"`
	ReasoningEfforts  []string `json:"reasoningEfforts"`
	ContributionKinds []Option `json:"contributionKinds"`
}

func Options(ctx context.Context, store *storage.Store, requestedRevision int64) (string, int64, FilterOptions, error) {
	epoch, latestRevision, err := store.Snapshot()
	if err != nil {
		return "", 0, FilterOptions{}, err
	}
	rev := requestedRevision
	if rev == 0 {
		rev = latestRevision
	}
	if rev < 1 || rev > latestRevision {
		return "", 0, FilterOptions{}, errors.New("revision_unavailable")
	}
	out := FilterOptions{Projects: []Option{}, Models: []string{}, ReasoningEfforts: []string{}, ContributionKinds: []Option{{"user_root_direct", "User root"}, {"descendant", "Descendants"}, {"inspector_review", "Inspector Review"}, {"other_orphan", "Other / orphan"}}}
	rows, err := store.DB().QueryContext(ctx, `SELECT id,display_name FROM projects WHERE epoch_id=? AND created_revision<=? ORDER BY display_name,id LIMIT ?`, epoch, rev, MaxFilterProjects+1)
	if err != nil {
		return "", 0, out, err
	}
	for rows.Next() {
		var x Option
		if err = rows.Scan(&x.Value, &x.Label); err != nil {
			rows.Close()
			return "", 0, out, err
		}
		if len(x.Label) > 500 {
			rows.Close()
			return "", 0, out, ErrResponseTooLarge
		}
		out.Projects = append(out.Projects, x)
	}
	if err = rows.Close(); err != nil {
		return "", 0, out, err
	}
	if len(out.Projects) > MaxFilterProjects {
		return "", 0, out, ErrResponseTooLarge
	}
	for _, target := range []struct {
		column string
		max    int
		values *[]string
	}{{"model", MaxFilterModels, &out.Models}, {"reasoning_effort", MaxFilterReasoningEfforts, &out.ReasoningEfforts}} {
		rows, err = store.DB().QueryContext(ctx, fmt.Sprintf(`SELECT DISTINCT %s FROM turns WHERE epoch_id=? AND state='completed' AND commit_revision<=? AND %s IS NOT NULL AND %s<>'' ORDER BY %s LIMIT ?`, target.column, target.column, target.column, target.column), epoch, rev, target.max+1)
		if err != nil {
			return "", 0, out, err
		}
		for rows.Next() {
			var v string
			if err = rows.Scan(&v); err != nil {
				rows.Close()
				return "", 0, out, err
			}
			maxLength := 500
			if target.column == "reasoning_effort" {
				maxLength = 100
			}
			if len(v) > maxLength {
				rows.Close()
				return "", 0, out, ErrResponseTooLarge
			}
			*target.values = append(*target.values, v)
		}
		if err = rows.Close(); err != nil {
			return "", 0, out, err
		}
		if len(*target.values) > target.max {
			return "", 0, out, ErrResponseTooLarge
		}
	}
	return epoch, rev, out, nil
}

func validate(q *Query) error {
	if len(q.MetricKeys) < 1 || len(q.MetricKeys) > 7 {
		return errors.New("metricKeys must contain 1 to 7 metrics")
	}
	known := map[string]bool{}
	for _, k := range Keys {
		known[k] = true
	}
	seen := map[string]bool{}
	for _, k := range q.MetricKeys {
		if !known[k] || seen[k] {
			return fmt.Errorf("invalid metric key %q", k)
		}
		seen[k] = true
	}
	if q.Timezone == "" {
		return errors.New("timezone is required")
	}
	if _, e := time.LoadLocation(q.Timezone); e != nil {
		return errors.New("invalid timezone")
	}
	if q.Grain == "" {
		q.Grain = "day"
	}
	if q.Grain != "hour" && q.Grain != "day" && q.Grain != "week" && q.Grain != "month" {
		return errors.New("invalid grain")
	}
	if len(q.ProjectIDs) > 50 || len(q.Models) > 20 || len(q.ReasoningEfforts) > 10 || len(q.ContributionKinds) > 4 {
		return errors.New("filter limit exceeded")
	}
	validKinds := map[string]bool{"user_root_direct": true, "descendant": true, "inspector_review": true, "other_orphan": true}
	for _, k := range q.ContributionKinds {
		if !validKinds[k] {
			return errors.New("invalid contribution kind")
		}
	}
	if q.Start != "" {
		if _, e := time.Parse(time.RFC3339, q.Start); e != nil {
			return errors.New("invalid start")
		}
	}
	if q.End != "" {
		if _, e := time.Parse(time.RFC3339, q.End); e != nil {
			return errors.New("invalid end")
		}
	}
	if q.Start != "" && q.End != "" {
		a, _ := time.Parse(time.RFC3339, q.Start)
		b, _ := time.Parse(time.RFC3339, q.End)
		if !a.Before(b) {
			return errors.New("start must precede end")
		}
	}
	return nil
}

type usageRow struct {
	completed, timeModel, effort, kind, root          string
	input, cached, output, reasoning, total, residual sql.NullInt64
	fidelity                                          sql.NullString
}

func calculate(ctx context.Context, db *sql.DB, epoch string, rev int64, q Query, now time.Time) (Result, error) {
	rows, err := db.QueryContext(ctx, `WITH sv AS (SELECT v.* FROM session_versions v WHERE v.epoch_id=? AND v.revision=(SELECT max(x.revision) FROM session_versions x WHERE x.epoch_id=v.epoch_id AND x.session_id=v.session_id AND x.revision<=?))
	SELECT t.completed_at,t.model,t.reasoning_effort,CASE WHEN sv.purpose='inspector_review' THEN 'inspector_review' WHEN sv.purpose='spawned' THEN 'descendant' WHEN sv.purpose='user' AND sv.session_id=sv.root_work_unit_id THEN 'user_root_direct' ELSE 'other_orphan' END,sv.root_work_unit_id,u.input_tokens,u.cached_input_tokens,u.output_tokens,u.reasoning_output_tokens,u.total_tokens,u.residual_tokens,u.fidelity
	FROM turns t JOIN sv ON sv.session_id=t.session_id LEFT JOIN turn_usage u ON u.epoch_id=t.epoch_id AND u.turn_id=t.id AND u.formula_version=1
	LEFT JOIN sv rootv ON rootv.session_id=sv.root_work_unit_id WHERE t.epoch_id=? AND t.state='completed' AND t.commit_revision<=? AND (?='' OR t.completed_at>=?) AND (?='' OR t.completed_at<?)
	AND (json_array_length(?)=0 OR rootv.project_id IN (SELECT value FROM json_each(?))) AND (json_array_length(?)=0 OR t.model IN (SELECT value FROM json_each(?))) AND (json_array_length(?)=0 OR t.reasoning_effort IN (SELECT value FROM json_each(?)))`, epoch, rev, epoch, rev, q.Start, q.Start, q.End, q.End, jsonList(q.ProjectIDs), jsonList(q.ProjectIDs), jsonList(q.Models), jsonList(q.Models), jsonList(q.ReasoningEfforts), jsonList(q.ReasoningEfforts))
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	var all []usageRow
	for rows.Next() {
		var x usageRow
		if err = rows.Scan(&x.completed, &x.timeModel, &x.effort, &x.kind, &x.root, &x.input, &x.cached, &x.output, &x.reasoning, &x.total, &x.residual, &x.fidelity); err != nil {
			return Result{}, err
		}
		all = append(all, x)
	}
	if err = rows.Err(); err != nil {
		return Result{}, err
	}
	kindAllowed := func(k string) bool {
		if len(q.ContributionKinds) == 0 {
			return true
		}
		for _, v := range q.ContributionKinds {
			if v == k {
				return true
			}
		}
		return false
	}
	eligible := 0
	observed := 0
	compositionObserved := 0
	exclusions := map[string]int{}
	var totals Totals
	var comp Composition
	roots := map[string]*Root{}
	buckets := map[string]*Bucket{}
	loc, _ := time.LoadLocation(q.Timezone)
	for _, x := range all {
		if !kindAllowed(x.kind) {
			continue
		}
		eligible++
		if !x.total.Valid {
			exclusions["missing_usage"]++
			continue
		}
		observed++
		addTotal(&totals, x.kind, x.total.Int64)
		if x.input.Valid && x.cached.Valid && x.output.Valid && x.reasoning.Valid && x.cached.Int64 <= x.input.Int64 && x.reasoning.Int64 <= x.output.Int64 && x.total.Int64 >= x.input.Int64+x.output.Int64 {
			compositionObserved++
			comp.UncachedInput += x.input.Int64 - x.cached.Int64
			comp.CachedInput += x.cached.Int64
			comp.VisibleOutput += x.output.Int64 - x.reasoning.Int64
			comp.ReasoningOutput += x.reasoning.Int64
			comp.Residual += x.total.Int64 - x.input.Int64 - x.output.Int64
		} else {
			exclusions["missing_composition"]++
		}
		if x.kind == "user_root_direct" || x.kind == "descendant" {
			rr := roots[x.root]
			if rr == nil {
				rr = &Root{RootSessionID: x.root}
				roots[x.root] = rr
			}
			rr.InclusiveTokens += x.total.Int64
			if x.kind == "descendant" {
				rr.DescendantTokens += x.total.Int64
			} else {
				rr.DirectTokens += x.total.Int64
			}
		}
		t, _ := time.Parse(time.RFC3339Nano, x.completed)
		start, end := CalendarBucket(t.In(loc), q.Grain)
		bk := start.Format(time.RFC3339)
		bb := buckets[bk]
		if bb == nil {
			bb = &Bucket{BucketStart: bk, BucketEnd: end.Format(time.RFC3339), Timezone: q.Timezone, Grain: q.Grain}
			buckets[bk] = bb
		}
		addTotal(&bb.ByKind, x.kind, x.total.Int64)
	}
	cov := coverageFor(observed, eligible, "some eligible completed turns lack usable recorded usage", "no eligible completed turn has usable recorded usage")
	if eligible == 0 {
		cov.Reason = "no eligible completed turns"
	}
	fidelity := cov.Fidelity
	indexed := indexedCoverage(ctx, db, epoch, rev)
	boundary := TimeBoundary{Timezone: q.Timezone}
	if q.Start != "" {
		boundary.RequestedStart = &q.Start
	}
	if q.End != "" {
		boundary.RequestedEnd = &q.End
	}
	boundary.EffectiveStart = boundary.RequestedStart
	boundary.EffectiveEnd = boundary.RequestedEnd
	meta := Metadata{FormulaVersion: 1, Fidelity: fidelity, Coverage: cov, IndexedCoverage: indexed, TimeBoundary: boundary, ExclusionReasons: exclusions}
	orderedBuckets := sortBuckets(buckets)
	if len(orderedBuckets) > MaxBuckets {
		return Result{}, ErrResponseTooLarge
	}
	orderedRoots := sortRoots(roots)
	latest, series, capacityCoverage, capacityExclusions, capErr := capacity(ctx, db, epoch, rev, q, now)
	if capErr != nil {
		return Result{}, capErr
	}
	items := make([]MetricItem, 0, len(q.MetricKeys))
	for _, k := range q.MetricKeys {
		m := meta
		var value any
		switch k {
		case "recorded_tokens":
			if observed > 0 {
				value = sumTotals(totals)
			}
		case "recorded_tokens_by_kind":
			value = totals
		case "recorded_tokens_over_time":
			value = orderedBuckets
		case "token_composition":
			value = comp
			m.Coverage = coverageFor(compositionObserved, eligible, "some eligible completed turns lack complete, reconcilable token components", "no eligible completed turn has complete, reconcilable token components")
			m.Fidelity = m.Coverage.Fidelity
			m.ExclusionReasons = map[string]int{"missing_or_unreconciled_composition": eligible - compositionObserved}
		case "top_root_sessions_by_tokens":
			value = orderedRoots
		case "latest_capacity_observation":
			value = latest
			m.Fidelity = capacityCoverage.Fidelity
			m.Coverage = capacityCoverage
			m.ExclusionReasons = capacityExclusions
		case "capacity_drawdown":
			value = series
			m.Fidelity = capacityCoverage.Fidelity
			m.Coverage = capacityCoverage
			m.ExclusionReasons = capacityExclusions
		}
		items = append(items, MetricItem{Key: k, Metadata: m, Value: value})
	}
	return Result{SchemaVersion: 2, DatasetEpoch: epoch, AppliedRevision: rev, Coverage: cov, Results: items}, nil
}

func coverageFor(observed, eligible int, partial, none string) Coverage {
	if eligible == 0 {
		return Coverage{Fidelity: "unavailable", Observed: 0, Eligible: 0, Reason: none}
	}
	if observed == 0 {
		return Coverage{Fidelity: "unavailable", Observed: 0, Eligible: eligible, Reason: none}
	}
	if observed < eligible {
		return Coverage{Fidelity: "derived", Observed: observed, Eligible: eligible, Reason: partial}
	}
	return Coverage{Fidelity: "exact", Observed: observed, Eligible: eligible}
}

func jsonList(v []string) string {
	if v == nil {
		return "[]"
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func addTotal(t *Totals, k string, v int64) {
	switch k {
	case "user_root_direct":
		t.UserRootDirect += v
	case "descendant":
		t.Descendant += v
	case "inspector_review":
		t.InspectorReview += v
	default:
		t.OtherOrphan += v
	}
}
func sumTotals(t Totals) int64 {
	return t.UserRootDirect + t.Descendant + t.InspectorReview + t.OtherOrphan
}
func CalendarBucket(t time.Time, g string) (time.Time, time.Time) {
	y, m, d := t.Date()
	var s time.Time
	switch g {
	case "hour":
		s = time.Date(y, m, d, t.Hour(), 0, 0, 0, t.Location())
	case "week":
		s = time.Date(y, m, d, 0, 0, 0, 0, t.Location())
		delta := (int(s.Weekday()) + 6) % 7
		s = s.AddDate(0, 0, -delta)
	case "month":
		s = time.Date(y, m, 1, 0, 0, 0, 0, t.Location())
	default:
		s = time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	}
	switch g {
	case "hour":
		return s, s.Add(time.Hour)
	case "week":
		return s, s.AddDate(0, 0, 7)
	case "month":
		return s, s.AddDate(0, 1, 0)
	default:
		return s, s.AddDate(0, 0, 1)
	}
}
func indexedCoverage(ctx context.Context, db *sql.DB, e string, r int64) IndexedCoverage {
	var a, b, c sql.NullString
	_ = db.QueryRowContext(ctx, "SELECT min(completed_at),max(completed_at),max(completed_at) FROM turns WHERE epoch_id=? AND state='completed' AND commit_revision<=?", e, r).Scan(&a, &b, &c)
	return IndexedCoverage{pointerString(a), pointerString(b), pointerString(c)}
}
func pointerString(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	return &v.String
}
func sortBuckets(m map[string]*Bucket) []Bucket {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	out := make([]Bucket, 0, len(keys))
	for _, k := range keys {
		out = append(out, *m[k])
	}
	return out
}
func sortRoots(m map[string]*Root) []Root {
	out := make([]Root, 0, len(m))
	for _, v := range m {
		out = append(out, *v)
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && (out[j].InclusiveTokens > out[j-1].InclusiveTokens || (out[j].InclusiveTokens == out[j-1].InclusiveTokens && out[j].RootSessionID < out[j-1].RootSessionID)); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	if len(out) > 200 {
		out = out[:200]
	}
	return out
}
func sortStrings(v []string) {
	for i := 1; i < len(v); i++ {
		for j := i; j > 0 && v[j] < v[j-1]; j-- {
			v[j], v[j-1] = v[j-1], v[j]
		}
	}
}
func capacity(ctx context.Context, db *sql.DB, e string, r int64, q Query, now time.Time) (*CapacityPoint, []CapacitySeries, Coverage, map[string]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT c.observed_at,c.limit_id,c.window_minutes,c.used_percent,c.remaining_percent,c.resets_at FROM capacity_observations c JOIN events ev ON ev.epoch_id=c.epoch_id AND ev.id=c.event_id WHERE c.epoch_id=? AND ev.commit_revision<=? ORDER BY c.observed_at,c.id`, e, r)
	if err != nil {
		return nil, nil, Coverage{}, nil, err
	}
	defer rows.Close()
	var latest *CapacityPoint
	groups := map[string]*CapacitySeries{}
	order := []string{}
	eligible, observedCount := 0, 0
	for rows.Next() {
		var observed, limit string
		var window sql.NullInt64
		var used, remaining sql.NullFloat64
		var reset sql.NullString
		if err = rows.Scan(&observed, &limit, &window, &used, &remaining, &reset); err != nil {
			return nil, nil, Coverage{}, nil, err
		}
		eligible++
		if !window.Valid || !used.Valid || !reset.Valid {
			continue
		}
		observedCount++
		resetRFC := reset.String
		if n, e := strconv.ParseInt(reset.String, 10, 64); e == nil {
			resetRFC = time.Unix(n, 0).UTC().Format(time.RFC3339)
		}
		p := CapacityPoint{ObservedAt: observed, LimitID: limit, WindowMinutes: window.Int64, UsedPercent: used.Float64, ResetsAt: resetRFC}
		if remaining.Valid {
			p.RemainingPercent = &remaining.Float64
		}
		rt, _ := time.Parse(time.RFC3339, resetRFC)
		p.Stale = !now.Before(rt)
		copy := p
		latest = &copy
		if q.Start != "" && observed < q.Start {
			continue
		}
		if q.End != "" && observed >= q.End {
			continue
		}
		key := fmt.Sprintf("%s|%d|%s", limit, window.Int64, resetRFC)
		g := groups[key]
		if g == nil {
			if len(order) >= MaxCapacitySeries {
				return nil, nil, Coverage{}, nil, ErrResponseTooLarge
			}
			g = &CapacitySeries{LimitID: limit, WindowMinutes: window.Int64, ResetBoundary: resetRFC}
			groups[key] = g
			order = append(order, key)
		}
		if len(g.Points) >= MaxCapacityPoints {
			return nil, nil, Coverage{}, nil, ErrResponseTooLarge
		}
		g.Points = append(g.Points, p)
	}
	out := make([]CapacitySeries, 0, len(order))
	for _, k := range order {
		out = append(out, *groups[k])
	}
	if err = rows.Err(); err != nil {
		return nil, nil, Coverage{}, nil, err
	}
	cov := coverageFor(observedCount, eligible, "some recorded capacity observations lack required fields", "no complete recorded capacity observation")
	excluded := map[string]int{}
	if eligible > observedCount {
		excluded["incomplete_capacity_observation"] = eligible - observedCount
	}
	return latest, out, cov, excluded, nil
}
