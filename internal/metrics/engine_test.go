package metrics_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	"github.com/dylanjbarth/codex-inspector/internal/metrics"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
	"github.com/getkin/kin-openapi/openapi3"
)

func indexedSynthetic(t *testing.T) *storage.Store {
	return indexedSyntheticWith(t, func(b []byte) []byte { return b })
}

func indexedSyntheticWith(t *testing.T, transform func([]byte) []byte) *storage.Store {
	t.Helper()
	root := t.TempDir()
	codex := filepath.Join(root, "codex")
	sessions := filepath.Join(codex, "sessions")
	if err := os.MkdirAll(sessions, 0700); err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	fixtures := filepath.Join(filepath.Dir(file), "..", "..", "fixtures", "synthetic")
	for _, name := range []string{"root.jsonl", "descendant.jsonl"} {
		b, err := os.ReadFile(filepath.Join(fixtures, name))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(sessions, name), transform(b), 0600); err != nil {
			t.Fatal(err)
		}
	}
	l := home.Layout{Root: filepath.Join(root, "inspector")}
	l.Queue = filepath.Join(l.Root, "queue")
	l.Run = filepath.Join(l.Root, "run")
	l.Logs = filepath.Join(l.Root, "logs")
	l.Cache = filepath.Join(l.Root, "cache")
	l.Reviews = filepath.Join(l.Root, "reviews")
	if err := home.Ensure(l); err != nil {
		t.Fatal(err)
	}
	if _, err := indexer.Run(context.Background(), indexer.Config{Layout: l, CodexHome: codex}); err != nil {
		t.Fatal(err)
	}
	s, err := storage.Open(filepath.Join(l.Root, "inspector.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestEveryFilterAndCatalogChoice(t *testing.T) {
	s := indexedSynthetic(t)
	e := metrics.New(8)
	epoch, revision, options, err := metrics.Options(context.Background(), s, 0)
	if err != nil {
		t.Fatal(err)
	}
	if epoch == "" || revision < 2 {
		t.Fatalf("catalog snapshot=%q/%d", epoch, revision)
	}
	if len(options.Projects) != 1 || len(options.Models) != 1 || options.Models[0] != "gpt-fake" || len(options.ReasoningEfforts) != 2 {
		t.Fatalf("options=%+v", options)
	}
	oldEpoch, oldRevision, oldOptions, err := metrics.Options(context.Background(), s, 1)
	if err != nil {
		t.Fatal(err)
	}
	if oldEpoch != epoch || oldRevision != 1 || len(oldOptions.Projects) != 0 || len(oldOptions.Models) != 0 || len(oldOptions.ReasoningEfforts) != 0 {
		t.Fatalf("revision-one options=%+v snapshot=%q/%d", oldOptions, oldEpoch, oldRevision)
	}
	base := metrics.Query{MetricKeys: []string{"recorded_tokens", "latest_capacity_observation"}, Timezone: "UTC", Grain: "day"}
	cases := []struct {
		name  string
		apply func(*metrics.Query)
		want  int64
	}{
		{"project", func(q *metrics.Query) { q.ProjectIDs = []string{options.Projects[0].Value} }, 2500},
		{"model", func(q *metrics.Query) { q.Models = []string{"gpt-fake"} }, 2500},
		{"reasoning", func(q *metrics.Query) { q.ReasoningEfforts = []string{"medium"} }, 500},
		{"contribution", func(q *metrics.Query) { q.ContributionKinds = []string{"descendant"} }, 500},
		{"time", func(q *metrics.Query) { q.Start = "2026-07-01T10:00:08Z"; q.End = "2026-07-01T10:01:00Z" }, 500},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := base
			tc.apply(&q)
			got, err := e.Query(context.Background(), s, q)
			if err != nil {
				t.Fatal(err)
			}
			if got.Results[0].Value.(int64) != tc.want {
				t.Fatalf("tokens=%v", got.Results[0].Value)
			}
			latest := got.Results[1].Value.([]metrics.CapacityPoint)
			if len(latest) != 1 || latest[0].UsedPercent != 45 {
				t.Fatal("capacity changed under token filter")
			}
		})
	}
}

func TestFilterOptionsRejectUnrepresentableCatalog(t *testing.T) {
	s := indexedSynthetic(t)
	epoch, revision, err := s.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < metrics.MaxFilterProjects; i++ {
		id := fmt.Sprintf("project-extra-%03d", i)
		if _, err = s.DB().Exec(`INSERT INTO projects(id,epoch_id,identity_kind,canonical_identity,display_name,created_revision) VALUES(?,?,'cwd',?,?,?)`, id, epoch, "/fake/"+id, id, revision); err != nil {
			t.Fatal(err)
		}
	}
	_, _, _, err = metrics.Options(context.Background(), s, revision)
	if !errors.Is(err, metrics.ErrResponseTooLarge) {
		t.Fatalf("options bound=%v", err)
	}
}

func TestMissingCompositionAndCapacityAreHonest(t *testing.T) {
	s := indexedSyntheticWith(t, func(b []byte) []byte {
		x := string(b)
		x = strings.ReplaceAll(x, `"cached_input_tokens":400,`, ``)
		x = strings.ReplaceAll(x, `"cached_input_tokens":500,`, ``)
		x = strings.ReplaceAll(x, `"window_minutes":300,`, ``)
		return []byte(x)
	})
	got, err := metrics.New(8).Query(context.Background(), s, metrics.Query{MetricKeys: []string{"token_composition", "latest_capacity_observation", "capacity_drawdown"}, Timezone: "UTC", Grain: "day"})
	if err != nil {
		t.Fatal(err)
	}
	composition := got.Results[0]
	if composition.Metadata.Coverage.Fidelity != "derived" || composition.Metadata.Coverage.Observed != 1 || composition.Metadata.Coverage.Eligible != 3 {
		t.Fatalf("composition=%+v", composition.Metadata.Coverage)
	}
	for _, m := range got.Results[1:] {
		if m.Metadata.Coverage.Fidelity != "unavailable" || m.Metadata.Coverage.Eligible != 2 || m.Metadata.Coverage.Observed != 0 {
			t.Fatalf("capacity=%+v", m.Metadata.Coverage)
		}
	}
}

func TestRecordedTokensUnavailableWhenNoEligibleTurnHasUsage(t *testing.T) {
	s := indexedSyntheticWith(t, func(b []byte) []byte {
		lines := strings.Split(string(b), "\n")
		kept := lines[:0]
		for _, line := range lines {
			if !strings.Contains(line, `"type":"token_count"`) {
				kept = append(kept, line)
			}
		}
		return []byte(strings.Join(kept, "\n"))
	})
	got, err := metrics.New(8).Query(context.Background(), s, metrics.Query{MetricKeys: []string{"recorded_tokens"}, Timezone: "UTC", Grain: "day"})
	if err != nil {
		t.Fatal(err)
	}
	item := got.Results[0]
	if item.Value != nil || item.Metadata.Coverage.Fidelity != "unavailable" || item.Metadata.Coverage.Observed != 0 || item.Metadata.Coverage.Eligible != 3 {
		t.Fatalf("recorded tokens=%#v coverage=%+v", item.Value, item.Metadata.Coverage)
	}
	if got.Coverage.Fidelity != "unavailable" || got.Coverage.Reason == "" {
		t.Fatalf("result coverage=%+v", got.Coverage)
	}
}

func TestRecordedTokensKeepKnownPartialSumLabeledPartial(t *testing.T) {
	s := indexedSyntheticWith(t, func(b []byte) []byte {
		lines := strings.Split(string(b), "\n")
		kept := lines[:0]
		for _, line := range lines {
			if strings.Contains(line, `"type":"token_count"`) && strings.Contains(line, `"total_tokens":500`) {
				continue
			}
			kept = append(kept, line)
		}
		return []byte(strings.Join(kept, "\n"))
	})
	got, err := metrics.New(8).Query(context.Background(), s, metrics.Query{MetricKeys: []string{"recorded_tokens"}, Timezone: "UTC", Grain: "day"})
	if err != nil {
		t.Fatal(err)
	}
	item := got.Results[0]
	if item.Value != int64(2000) || item.Metadata.Coverage.Fidelity != "derived" || item.Metadata.Coverage.Observed != 2 || item.Metadata.Coverage.Eligible != 3 {
		t.Fatalf("recorded tokens=%#v coverage=%+v", item.Value, item.Metadata.Coverage)
	}
}

func TestRecordedTokensTolerateMissingTurnMetadata(t *testing.T) {
	s := indexedSyntheticWith(t, func(b []byte) []byte {
		return []byte(strings.Replace(string(b), `,"model":"gpt-fake","effort":"high"`, "", 1))
	})
	got, err := metrics.New(8).Query(context.Background(), s, metrics.Query{MetricKeys: []string{"recorded_tokens"}, Timezone: "UTC", Grain: "day"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Results[0].Value != int64(500) {
		t.Fatalf("recorded tokens=%#v", got.Results[0].Value)
	}
}

func TestFrozenBucketBoundAndWallClockStaleness(t *testing.T) {
	s := indexedSynthetic(t)
	engine := metrics.New(8)
	_, err := engine.Query(context.Background(), s, metrics.Query{MetricKeys: []string{"recorded_tokens_over_time"}, Timezone: "UTC", Grain: "hour", Start: "2026-01-01T00:00:00Z", End: "2026-04-01T00:00:00Z"})
	if !errors.Is(err, metrics.ErrResponseTooLarge) {
		t.Fatalf("bound=%v", err)
	}
	now := time.Unix(1782910799, 0)
	engine.SetClockForTest(func() time.Time { return now })
	q := metrics.Query{MetricKeys: []string{"latest_capacity_observation"}, Timezone: "UTC", Grain: "day"}
	before, err := engine.Query(context.Background(), s, q)
	if err != nil {
		t.Fatal(err)
	}
	if before.Results[0].Value.([]metrics.CapacityPoint)[0].Stale {
		t.Fatal("premature stale")
	}
	now = time.Unix(1782910801, 0)
	after, err := engine.Query(context.Background(), s, q)
	if err != nil {
		t.Fatal(err)
	}
	if !after.Results[0].Value.([]metrics.CapacityPoint)[0].Stale {
		t.Fatal("cached stale")
	}
}

func TestGoldenMetricsAndFilters(t *testing.T) {
	s := indexedSynthetic(t)
	e := metrics.New(2)
	q := metrics.Query{MetricKeys: metrics.Keys, Timezone: "UTC", Grain: "day", Start: "2026-07-01T00:00:00Z", End: "2026-07-02T00:00:00Z"}
	r, err := e.Query(context.Background(), s, q)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(r)
	var got map[string]any
	if json.Unmarshal(b, &got) != nil {
		t.Fatal("invalid result")
	}
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "schemas", "internal-api.openapi.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = doc.Components.Schemas["MetricResult"].Value.VisitJSON(got); err != nil {
		t.Fatalf("result violates frozen OpenAPI: %v", err)
	}
	items := got["results"].([]any)
	values := map[string]any{}
	for _, raw := range items {
		x := raw.(map[string]any)
		values[x["key"].(string)] = x["value"]
	}
	if values["recorded_tokens"].(float64) != 2500 {
		t.Fatalf("total=%v", values["recorded_tokens"])
	}
	by := values["recorded_tokens_by_kind"].(map[string]any)
	if by["userRootDirect"].(float64) != 2000 || by["descendant"].(float64) != 500 {
		t.Fatalf("attribution=%v", by)
	}
	overTimeBucket := values["recorded_tokens_over_time"].([]any)[0].(map[string]any)
	uncachedByKind := overTimeBucket["byKindUncached"].(map[string]any)
	cachedByKind := overTimeBucket["byKindCached"].(map[string]any)
	if uncachedByKind["userRootDirect"].(float64) != 1500 || uncachedByKind["descendant"].(float64) != 400 || cachedByKind["userRootDirect"].(float64) != 500 || cachedByKind["descendant"].(float64) != 100 {
		t.Fatalf("cache attribution uncached=%v cached=%v", uncachedByKind, cachedByKind)
	}
	modelReasoningBuckets := values["recorded_tokens_by_model_reasoning_over_time"].([]any)
	if len(modelReasoningBuckets) != 1 {
		t.Fatalf("model/reasoning buckets=%v", modelReasoningBuckets)
	}
	series := modelReasoningBuckets[0].(map[string]any)["series"].([]any)
	grouped, groupedUncached, groupedCached := map[string]float64{}, map[string]float64{}, map[string]float64{}
	for _, raw := range series {
		entry := raw.(map[string]any)
		key := entry["model"].(string) + "/" + entry["reasoningEffort"].(string)
		grouped[key] = entry["tokens"].(float64)
		groupedUncached[key] = entry["uncachedTokens"].(float64)
		groupedCached[key] = entry["cachedTokens"].(float64)
	}
	if grouped["gpt-fake/high"] != 2000 || grouped["gpt-fake/medium"] != 500 || groupedUncached["gpt-fake/high"] != 1500 || groupedUncached["gpt-fake/medium"] != 400 || groupedCached["gpt-fake/high"] != 500 || groupedCached["gpt-fake/medium"] != 100 {
		t.Fatalf("model/reasoning attribution total=%v uncached=%v cached=%v", grouped, groupedUncached, groupedCached)
	}
	comp := values["token_composition"].(map[string]any)
	if comp["uncachedInput"].(float64) != 1350 || comp["cachedInput"].(float64) != 600 || comp["visibleOutput"].(float64) != 440 || comp["reasoningOutput"].(float64) != 110 {
		t.Fatalf("composition=%v", comp)
	}
	latest := values["latest_capacity_observation"].([]any)[0].(map[string]any)
	if latest["usedPercent"].(float64) != 45 {
		t.Fatalf("capacity=%v", latest)
	}
	q.MetricKeys = []string{"recorded_tokens", "latest_capacity_observation"}
	q.ContributionKinds = []string{"descendant"}
	filtered, err := e.Query(context.Background(), s, q)
	if err != nil {
		t.Fatal(err)
	}
	b, _ = json.Marshal(filtered)
	json.Unmarshal(b, &got)
	items = got["results"].([]any)
	if items[0].(map[string]any)["value"].(float64) != 500 {
		t.Fatalf("filtered=%v", items[0])
	}
	if items[1].(map[string]any)["value"].([]any)[0].(map[string]any)["usedPercent"].(float64) != 45 {
		t.Fatal("capacity incorrectly filtered")
	}
}

func TestCapacityKeepsLatestWindowsAndResetSeriesDistinct(t *testing.T) {
	s := indexedSyntheticWith(t, func(b []byte) []byte {
		return []byte(strings.Replace(
			string(b),
			`"resets_at":1782910800}}}`,
			`"resets_at":1782910800},"secondary":{"used_percent":27,"remaining_percent":73,"window_minutes":10080,"resets_at":1784930400}}}`,
			1,
		))
	})
	result, err := metrics.New(8).Query(context.Background(), s, metrics.Query{
		MetricKeys: []string{"latest_capacity_observation", "capacity_drawdown"},
		Timezone:   "UTC",
		Grain:      "day",
	})
	if err != nil {
		t.Fatal(err)
	}
	latest := result.Results[0].Value.([]metrics.CapacityPoint)
	if len(latest) != 2 {
		t.Fatalf("latest windows=%d, want 2: %+v", len(latest), latest)
	}
	if latest[0].WindowMinutes != 300 || latest[0].UsedPercent != 45 || latest[1].WindowMinutes != 10080 || latest[1].UsedPercent != 27 {
		t.Fatalf("latest deterministic order/values=%+v", latest)
	}
	if latest[0].ObservedAt != latest[1].ObservedAt {
		t.Fatalf("primary and secondary should remain latest at equal timestamp: %+v", latest)
	}
	if latest[1].RemainingPercent == nil || *latest[1].RemainingPercent != 73 {
		t.Fatalf("weekly remaining semantics=%+v", latest[1])
	}

	series := result.Results[1].Value.([]metrics.CapacitySeries)
	if len(series) != 3 {
		t.Fatalf("drawdown series=%d, want separate reset/window series: %+v", len(series), series)
	}
	for i, windowSeries := range series {
		if len(windowSeries.Points) != 1 {
			t.Fatalf("series %d connected reset boundaries: %+v", i, windowSeries)
		}
		point := windowSeries.Points[0]
		if point.LimitID != windowSeries.LimitID || point.WindowMinutes != windowSeries.WindowMinutes || point.ResetsAt != windowSeries.ResetBoundary {
			t.Fatalf("series %d mixes semantic identities: series=%+v point=%+v", i, windowSeries, point)
		}
	}
	if series[0].WindowMinutes != 300 || series[1].WindowMinutes != 300 || series[2].WindowMinutes != 10080 {
		t.Fatalf("drawdown series order=%+v", series)
	}
}

func TestCapacityDrawdownDownsamplesOversizedResetSeries(t *testing.T) {
	const firstObserved = "2026-07-01T10:00:06.000000Z"
	lastObserved := fmt.Sprintf("2026-07-01T10:00:06.%06dZ", metrics.MaxCapacityPoints+1)
	s := indexedSyntheticWith(t, func(b []byte) []byte {
		lines := strings.Split(string(b), "\n")
		out := make([]string, 0, len(lines)+metrics.MaxCapacityPoints)
		for _, line := range lines {
			if !strings.Contains(line, `"timestamp":"2026-07-01T10:00:06Z"`) {
				out = append(out, line)
				continue
			}
			for i := 0; i < metrics.MaxCapacityPoints+2; i++ {
				observed := fmt.Sprintf("2026-07-01T10:00:06.%06dZ", i)
				point := strings.Replace(line, "2026-07-01T10:00:06Z", observed, 1)
				point = strings.Replace(point, `"used_percent":40`, fmt.Sprintf(`"used_percent":%d`, 20+i%70), 1)
				out = append(out, point)
			}
		}
		return []byte(strings.Join(out, "\n"))
	})

	result, err := metrics.New(8).Query(context.Background(), s, metrics.Query{
		MetricKeys: []string{"capacity_drawdown"},
		Timezone:   "UTC",
		Grain:      "day",
		Start:      "2026-07-01T10:00:00Z",
		End:        "2026-07-01T10:01:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	series := result.Results[0].Value.([]metrics.CapacitySeries)
	if len(series) == 0 {
		t.Fatal("capacity drawdown returned no series")
	}
	if len(series[0].Points) != metrics.MaxCapacityPoints {
		t.Fatalf("series points=%d, want %d", len(series[0].Points), metrics.MaxCapacityPoints)
	}
	if series[0].Points[0].ObservedAt != firstObserved || series[0].Points[len(series[0].Points)-1].ObservedAt != lastObserved {
		t.Fatalf("downsampled endpoints=%q..%q, want %q..%q", series[0].Points[0].ObservedAt, series[0].Points[len(series[0].Points)-1].ObservedAt, firstObserved, lastObserved)
	}
}

func TestChicagoCalendarBucketAcrossDST(t *testing.T) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}
	start, end := metrics.CalendarBucket(time.Date(2026, 3, 8, 12, 0, 0, 0, loc), "day")
	if end.Sub(start) != 23*time.Hour {
		t.Fatalf("DST day=%s", end.Sub(start))
	}
}
