package phase0

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/santhosh-tekuri/jsonschema/v5"
	_ "modernc.org/sqlite"
)

func repoPath(parts ...string) string {
	all := append([]string{"..", ".."}, parts...)
	return filepath.Join(all...)
}

func TestSourceDecisions(t *testing.T) {
	root, err := ParseFixture(repoPath("fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !root.Supported || root.PendingTail || root.SessionID != "root-001" {
		t.Fatalf("unexpected root decision: %+v", root)
	}
	if got := completedTurns(root); got != 2 {
		t.Fatalf("completed root turns = %d, want 2", got)
	}
	if root.ToolRequests != 1 || root.ToolResults != 1 {
		t.Fatalf("tool phases = %d/%d, want 1/1", root.ToolRequests, root.ToolResults)
	}

	unsupported, err := ParseFixture(repoPath("fixtures", "synthetic", "unsupported.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if unsupported.Supported || unsupported.Reason != "incompatible_record_envelope" {
		t.Fatalf("unexpected unsupported decision: %+v", unsupported)
	}

	truncated, err := ParseFixture(repoPath("fixtures", "synthetic", "truncated.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !truncated.Supported || !truncated.PendingTail || completedTurns(truncated) != 0 {
		t.Fatalf("unexpected truncated decision: %+v", truncated)
	}
}

func TestSourceDecisionUsesActualThreadIdentityAndFamilyRoot(t *testing.T) {
	data, err := os.ReadFile(repoPath("fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	candidate := strings.Replace(string(data), `"id":"root-001","session_id":"root-001"`, `"id":"child-001","session_id":"root-001"`, 1)
	candidate = strings.Replace(candidate, `"source":"cli"`, `"source":{"subagent":{"thread_spawn":{"parent_thread_id":"root-001","depth":1}}}`, 1)
	decision, err := ParseRollout(strings.NewReader(candidate))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Supported || decision.SessionID != "child-001" || decision.RootSessionID != "root-001" || decision.ParentSessionID != "root-001" || !decision.Spawned {
		t.Fatalf("modern thread identity was not preserved: %+v", decision)
	}
}

func TestHistoricalCodexVersionsReachStructuralValidation(t *testing.T) {
	data, err := os.ReadFile(repoPath("fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"0.100.0-alpha.10", "0.125.0", "0.142.4", "0.142.5", "0.143.0", "0.144.0-alpha.4", "0.144.1", "0.145.0-alpha.18", "0.145.0-alpha.19", "1.0.0"} {
		t.Run(version, func(t *testing.T) {
			candidate := strings.Replace(string(data), `"cli_version":"0.144.1"`, `"cli_version":"`+version+`"`, 1)
			decision, parseErr := ParseRollout(strings.NewReader(candidate))
			if parseErr != nil || !decision.Supported {
				t.Fatalf("recent structurally compatible source rejected: decision=%+v err=%v", decision, parseErr)
			}
		})
	}
	invalid := strings.Replace(string(data), `"cli_version":"0.144.1"`, `"cli_version":"not-a-version"`, 1)
	decision, err := ParseRollout(strings.NewReader(invalid))
	if err != nil || decision.Supported || decision.Reason != "unsupported_codex_version" {
		t.Fatalf("invalid version accepted: decision=%+v err=%v", decision, err)
	}
}

func TestCompatibilityProbeStopsAfterBoundedPrefix(t *testing.T) {
	b, err := os.ReadFile(repoPath("fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	first, _, found := bytes.Cut(b, []byte{'\n'})
	if !found {
		t.Fatal("fixture has no complete leading record")
	}
	data := append(append(append([]byte(nil), first...), '\n'), bytes.Repeat([]byte{'x'}, 2*maxCompatibilityProbeBytes)...)
	reader := bytes.NewReader(data)
	before := reader.Len()
	decision, err := ProbeRollout(reader)
	if err != nil || !decision.Supported || decision.SessionID != "root-001" {
		t.Fatalf("probe=%+v err=%v", decision, err)
	}
	if consumed := before - reader.Len(); consumed > maxCompatibilityProbeBytes+1 {
		t.Fatalf("compatibility probe consumed %d bytes", consumed)
	}

	tooLarge := strings.Repeat("x", maxCompatibilityProbeBytes+1)
	decision, err = ProbeRollout(strings.NewReader(tooLarge))
	if err != nil || decision.Supported || decision.Reason != "leading_record_too_large" {
		t.Fatalf("oversized probe=%+v err=%v", decision, err)
	}
}

func TestSourceFingerprintRejectsMalformedSameVersion(t *testing.T) {
	data, err := os.ReadFile(repoPath("fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	malformed := strings.Replace(string(data), `"effort":"high"`, `"effort":7`, 1)
	decision, err := ParseRollout(strings.NewReader(malformed))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Supported || decision.Reason != "incompatible_turn_context" {
		t.Fatalf("malformed same-version source accepted: %+v", decision)
	}
	unknown := string(data) + "{\"timestamp\":\"2026-07-01T12:00:00Z\",\"type\":\"future_record\",\"payload\":{}}\n"
	decision, err = ParseRollout(strings.NewReader(unknown))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Supported || decision.Reason != "incompatible_record_envelope" {
		t.Fatalf("unknown record accepted: %+v", decision)
	}
	mirrored := string(data) + "{\"timestamp\":\"2026-07-01T12:00:00Z\",\"type\":\"response_item\",\"payload\":{\"type\":\"function_call\",\"call_id\":\"call-1\"}}\n{\"timestamp\":\"2026-07-01T12:00:01Z\",\"type\":\"response_item\",\"payload\":{\"type\":\"function_call_output\",\"call_id\":\"call-1\"}}\n"
	decision, err = ParseRollout(strings.NewReader(mirrored))
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Supported || decision.ToolRequests != 1 || decision.ToolResults != 1 {
		t.Fatalf("mirrored tool phases not deduplicated: %+v", decision)
	}

	for name, mutation := range map[string]string{
		"not-rfc3339":    strings.Replace(string(data), `"timestamp":"2026-07-01T10:00:03Z"`, `"timestamp":"not-rfc3339"`, 1),
		"not-numeric":    strings.Replace(string(data), `"completed_at":1782900007`, `"completed_at":"not-numeric"`, 1),
		"non-string-cwd": strings.Replace(string(data), `"cwd":"/fake/acme","model"`, `"cwd":42,"model"`, 1),
		"negative-usage": strings.Replace(string(data), `"input_tokens":1000`, `"input_tokens":-1`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ParseRollout(strings.NewReader(mutation))
			if err != nil {
				t.Fatal(err)
			}
			if got.Supported {
				t.Fatalf("malformed source accepted: %+v", got)
			}
		})
	}
	minimalCompletion := strings.Replace(string(data), `,"completed_at":1782900007`, ``, 1)
	decision, err = ParseRollout(strings.NewReader(minimalCompletion))
	if err != nil || !decision.Supported {
		t.Fatalf("observed payload-minimal completion variant rejected: decision=%+v err=%v", decision, err)
	}

	completeMalformed := string(data) + "{malformed-complete-record}\n"
	if _, err := ParseRollout(strings.NewReader(completeMalformed)); err == nil {
		t.Fatal("newline-terminated malformed record was treated as a retryable tail")
	}
	unterminatedMalformed := string(data) + "{malformed-pending-tail"
	pending, err := ParseRollout(strings.NewReader(unterminatedMalformed))
	if err != nil {
		t.Fatal(err)
	}
	if !pending.Supported || !pending.PendingTail {
		t.Fatalf("unterminated final tail not retained as retryable: %+v", pending)
	}
}

func TestStructuralValidatorRetainsCompatibleHistoricalVariants(t *testing.T) {
	data, err := os.ReadFile(repoPath("fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	variants := map[string]string{
		"missing-reasoning-effort":  strings.Replace(string(data), `"effort":"high"`, `"effort":null`, 1),
		"rate-limit-only-token":     strings.Replace(string(data), `"info":{"last_token_usage":null,"total_token_usage":{"input_tokens":1600,"cached_input_tokens":500,"output_tokens":400,"reasoning_output_tokens":80,"total_tokens":2000},"model_context_window":99999}`, `"info":null`, 1),
		"repeated-turn-context":     string(data) + "{\"timestamp\":\"2026-07-01T12:00:00Z\",\"type\":\"turn_context\",\"payload\":{\"turn_id\":\"turn-root-2\",\"cwd\":\"/fake/acme\",\"model\":\"gpt-fake\",\"effort\":\"high\"}}\n",
		"generic-event":             string(data) + "{\"timestamp\":\"2026-07-01T12:00:00Z\",\"type\":\"event_msg\",\"payload\":{\"type\":\"future_observation\"}}\n",
		"generic-response":          string(data) + "{\"timestamp\":\"2026-07-01T12:00:00Z\",\"type\":\"response_item\",\"payload\":{\"type\":\"future_response\"}}\n",
		"repeated-session-metadata": string(data) + strings.SplitN(string(data), "\n", 2)[0] + "\n",
	}
	for name, variant := range variants {
		t.Run(name, func(t *testing.T) {
			decision, parseErr := ParseRollout(strings.NewReader(variant))
			if parseErr != nil || !decision.Supported {
				t.Fatalf("compatible historical variant rejected: decision=%+v err=%v", decision, parseErr)
			}
		})
	}
}

func TestMetricGolden(t *testing.T) {
	root, err := ParseFixture(repoPath("fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	child, err := ParseFixture(repoPath("fixtures", "synthetic", "descendant.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := CalculateMetrics([]SourceDecision{root, child})
	if err != nil {
		t.Fatal(err)
	}

	var expected Metrics
	loadJSON(t, repoPath("fixtures", "synthetic", "expected-metrics.json"), &expected)
	if !reflect.DeepEqual(metrics, expected) {
		got, _ := json.MarshalIndent(metrics, "", "  ")
		want, _ := json.MarshalIndent(expected, "", "  ")
		t.Fatalf("metric golden mismatch\ngot: %s\nwant: %s", got, want)
	}
}

func TestCalendarBucketsAcrossDSTAndMissingCoverage(t *testing.T) {
	before, err := CalendarBucket("2026-03-08T07:30:00Z", "America/Chicago", "day")
	if err != nil {
		t.Fatal(err)
	}
	after, err := CalendarBucket("2026-03-09T06:30:00Z", "America/Chicago", "day")
	if err != nil {
		t.Fatal(err)
	}
	if before != "2026-03-08T00:00:00-06:00" || after != "2026-03-09T00:00:00-05:00" {
		t.Fatalf("DST buckets %q %q", before, after)
	}
	metrics, err := CalculateMetrics([]SourceDecision{{Supported: true, SessionID: "missing", Turns: []Turn{{ID: "turn", Completed: true, CompletedAt: "2026-03-08T07:30:00Z"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.RecordedTokens != 0 || metrics.Coverage.EligibleTurns != 1 || metrics.Coverage.ObservedTurns != 0 || metrics.Coverage.ExcludedReasons["usage_unavailable"] != 1 {
		t.Fatalf("missing coverage: %+v", metrics)
	}
}

func TestMetricFilterConsistencyAndCapacityIndependence(t *testing.T) {
	root, _ := ParseFixture(repoPath("fixtures", "synthetic", "root.jsonl"))
	child, _ := ParseFixture(repoPath("fixtures", "synthetic", "descendant.jsonl"))
	filtered, err := CalculateMetricsFiltered([]SourceDecision{root, child}, MetricFilter{ContributionKind: "descendant", Project: "https://example.invalid/acme/widgets.git", Model: "gpt-fake", Reasoning: "medium", Start: "2026-07-01T09:00:00Z", End: "2026-07-01T10:30:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.RecordedTokens != 500 || filtered.ByKind["descendant"] != 500 || len(filtered.TopRoots) != 1 || filtered.TopRoots[0].DescendantTokens != 500 {
		t.Fatalf("filter mismatch: %+v", filtered)
	}
	if filtered.LatestCapacity == nil || filtered.LatestCapacity.UsedPercent != 45 || len(filtered.CapacityDrawdown) != 1 || filtered.CapacityDrawdown[0].UsedPercent != 40 {
		t.Fatalf("capacity independence/range mismatch: %+v", filtered)
	}
}

func TestCumulativeNormalizationPrecedesFilters(t *testing.T) {
	base := Usage{Input: 900, Cached: 300, Output: 300, Reasoning: 100, Total: 1200}
	next := Usage{Input: 1500, Cached: 400, Output: 500, Reasoning: 130, Total: 2000}
	source := SourceDecision{Supported: true, SessionID: "root", ProjectRemote: "project", Turns: []Turn{
		{ID: "first", Completed: true, CompletedAt: "2026-07-01T10:00:00Z", Model: "model-a", ReasoningEffort: "high", Usage: &base, Cumulative: &base},
		{ID: "second", Completed: true, CompletedAt: "2026-07-01T11:00:00Z", Model: "model-b", ReasoningEffort: "medium", Cumulative: &next},
	}}
	filtered, err := CalculateMetricsFiltered([]SourceDecision{source}, MetricFilter{Model: "model-b"})
	if err != nil {
		t.Fatal(err)
	}
	if filtered.RecordedTokens != 800 || filtered.Coverage.ObservedTurns != 1 {
		t.Fatalf("filtered cumulative delta lost baseline: %+v", filtered)
	}
}

func TestProvisionalSnapshotEstablishesCumulativeBaseline(t *testing.T) {
	first := Usage{Input: 900, Cached: 300, Output: 300, Reasoning: 100, Total: 1200}
	provisional := Usage{Input: 1200, Cached: 350, Output: 400, Reasoning: 115, Total: 1600}
	last := Usage{Input: 1500, Cached: 400, Output: 500, Reasoning: 130, Total: 2000}
	source := SourceDecision{Supported: true, SessionID: "root", Turns: []Turn{
		{ID: "first", Completed: true, CompletedAt: "2026-07-01T10:00:00Z", Usage: &first, Cumulative: &first},
		{ID: "aborted", Completed: false, Cumulative: &provisional},
		{ID: "last", Completed: true, CompletedAt: "2026-07-01T11:00:00Z", Cumulative: &last},
	}}
	metrics, err := CalculateMetrics([]SourceDecision{source})
	if err != nil {
		t.Fatal(err)
	}
	if metrics.RecordedTokens != 1600 || metrics.Coverage.EligibleTurns != 2 || metrics.Coverage.ObservedTurns != 2 {
		t.Fatalf("provisional baseline or eligibility incorrect: %+v", metrics)
	}
}

func TestFactGolden(t *testing.T) {
	paths := []string{"root.jsonl", "descendant.jsonl", "unsupported.jsonl", "truncated.jsonl"}
	inputs := make([]CorpusInput, 0, len(paths))
	for _, name := range paths {
		data, err := os.ReadFile(repoPath("fixtures", "synthetic", name))
		if err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, CorpusInput{Name: name, Data: data})
	}
	got, err := NormalizeCorpus(inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sources) != 4 || len(got.Sessions) != 2 || len(got.Turns) != 3 || len(got.Events) != 23 || len(got.Messages) != 3 || len(got.ToolPhases) != 2 || len(got.Compactions) != 1 || len(got.Capacity) != 2 || len(got.Coverage) != 3 || len(got.Evidence) != len(got.Events) {
		t.Fatalf("incomplete normalized corpus projection: sources=%d sessions=%d turns=%d events=%d messages=%d tools=%d compactions=%d capacity=%d coverage=%d evidence=%d", len(got.Sources), len(got.Sessions), len(got.Turns), len(got.Events), len(got.Messages), len(got.ToolPhases), len(got.Compactions), len(got.Capacity), len(got.Coverage), len(got.Evidence))
	}
	eventKinds := map[string]int{}
	for _, event := range got.Events {
		eventKinds[event.EventKind]++
		if len(event.ContentSHA256) != 64 || event.ByteEnd <= event.ByteStart || event.AdapterVersion != AdapterVersion {
			t.Fatalf("incomplete event provenance: %+v", event)
		}
	}
	if eventKinds["custom_tool_call"] != 1 || eventKinds["function_call"] != 1 || eventKinds["custom_tool_call_output"] != 1 || eventKinds["function_call_output"] != 1 {
		t.Fatalf("mirrored source events not represented: %v", eventKinds)
	}
	if got.ToolPhases[0].SemanticPhase != "request" || got.ToolPhases[1].SemanticPhase != "result" {
		t.Fatalf("mirrored tool phases not deduplicated semantically: %+v", got.ToolPhases)
	}
	var expected NormalizedCorpus
	loadJSON(t, repoPath("fixtures", "synthetic", "expected-facts.json"), &expected)
	if !reflect.DeepEqual(got, expected) {
		actual, _ := json.MarshalIndent(got, "", "  ")
		want, _ := json.MarshalIndent(expected, "", "  ")
		t.Fatalf("fact golden mismatch\ngot: %s\nwant: %s", actual, want)
	}
}

func TestNormalizedIdentitiesAreAppendStable(t *testing.T) {
	base, err := os.ReadFile(repoPath("fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	appendix := strings.Join([]string{
		`{"timestamp":"2026-07-01T12:00:01Z","type":"turn_context","payload":{"turn_id":"turn-root-3","cwd":"/fake/acme","model":"gpt-fake","effort":"high"}}`,
		`{"timestamp":"2026-07-01T12:00:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-root-3","started_at":"2026-07-01T12:00:02Z","model_context_window":99999,"collaboration_mode_kind":"default"}}`,
		`{"timestamp":"2026-07-01T12:00:03Z","type":"response_item","payload":{"type":"message","id":"msg-root-user-3","role":"user","content":[{"type":"input_text","text":"Run one appended fake check."}],"internal_chat_message_metadata_passthrough":{"turn_id":"turn-root-3"}}}`,
		`{"timestamp":"2026-07-01T12:00:04Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":null,"total_token_usage":{"input_tokens":1800,"cached_input_tokens":550,"output_tokens":500,"reasoning_output_tokens":100,"total_tokens":2300},"model_context_window":99999},"rate_limits":null}}`,
		`{"timestamp":"2026-07-01T12:00:05Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"turn-root-3","completed_at":1782907205,"last_agent_message":"Appended fake check complete."}}`,
	}, "\n") + "\n"
	appended := append(append([]byte(nil), base...), []byte(appendix)...)

	before, err := NormalizeCorpus([]CorpusInput{{Name: "active.jsonl", Data: base}})
	if err != nil {
		t.Fatal(err)
	}
	after, err := NormalizeCorpus([]CorpusInput{{Name: "active.jsonl", Data: appended}})
	if err != nil {
		t.Fatal(err)
	}
	if before.Sources[0].SourceID != after.Sources[0].SourceID || before.Sources[0].IdentityFingerprint != after.Sources[0].IdentityFingerprint || before.Sources[0].SegmentID != after.Sources[0].SegmentID || before.Sources[0].SegmentFingerprint != after.Sources[0].SegmentFingerprint {
		t.Fatalf("append changed source/segment identity: before=%+v after=%+v", before.Sources[0], after.Sources[0])
	}
	if before.Sources[0].CurrentFingerprint == after.Sources[0].CurrentFingerprint {
		t.Fatal("append did not advance mutable current fingerprint")
	}
	assertStablePrefix := func(name string, old, current any) {
		t.Helper()
		oldValue, currentValue := reflect.ValueOf(old), reflect.ValueOf(current)
		if oldValue.Len() > currentValue.Len() {
			t.Fatalf("%s shrank", name)
		}
		if !reflect.DeepEqual(oldValue.Interface(), currentValue.Slice(0, oldValue.Len()).Interface()) {
			t.Fatalf("append changed pre-existing %s", name)
		}
	}
	assertStablePrefix("sessions", before.Sessions, after.Sessions)
	assertStablePrefix("turns", before.Turns, after.Turns)
	assertStablePrefix("events and locators", before.Events, after.Events)
	assertStablePrefix("messages and locators", before.Messages, after.Messages)
	assertStablePrefix("tool phases", before.ToolPhases, after.ToolPhases)
	assertStablePrefix("compactions", before.Compactions, after.Compactions)
	assertStablePrefix("capacity", before.Capacity, after.Capacity)
	assertStablePrefix("coverage", before.Coverage, after.Coverage)
	assertStablePrefix("evidence identities and locators", before.Evidence, after.Evidence)
	if len(after.Turns) != len(before.Turns)+1 || len(after.Events) != len(before.Events)+5 || len(after.Evidence) != len(before.Evidence)+5 {
		t.Fatalf("append did not add only new facts: before turns/events/evidence=%d/%d/%d after=%d/%d/%d", len(before.Turns), len(before.Events), len(before.Evidence), len(after.Turns), len(after.Events), len(after.Evidence))
	}
	if len(after.Sessions) != len(before.Sessions) || len(after.Messages) != len(before.Messages)+1 || len(after.ToolPhases) != len(before.ToolPhases) || len(after.Compactions) != len(before.Compactions) || len(after.Capacity) != len(before.Capacity) || len(after.Coverage) != len(before.Coverage)+1 {
		t.Fatalf("append changed unrelated fact cardinality: sessions %d→%d messages %d→%d tools %d→%d compactions %d→%d capacity %d→%d coverage %d→%d", len(before.Sessions), len(after.Sessions), len(before.Messages), len(after.Messages), len(before.ToolPhases), len(after.ToolPhases), len(before.Compactions), len(after.Compactions), len(before.Capacity), len(after.Capacity), len(before.Coverage), len(after.Coverage))
	}
	beforeDecision, _ := ParseRollout(bytes.NewReader(base))
	afterDecision, _ := ParseRollout(bytes.NewReader(appended))
	beforeMetrics, err := CalculateMetrics([]SourceDecision{beforeDecision})
	if err != nil {
		t.Fatal(err)
	}
	afterMetrics, err := CalculateMetrics([]SourceDecision{afterDecision})
	if err != nil {
		t.Fatal(err)
	}
	if afterMetrics.RecordedTokens-beforeMetrics.RecordedTokens != 300 {
		t.Fatalf("appended contribution delta=%d, want 300", afterMetrics.RecordedTokens-beforeMetrics.RecordedTokens)
	}

	oldCheckpoint := CheckpointFingerprint(base, int64(len(base)))
	if CheckpointFingerprint(appended, int64(len(base))) != oldCheckpoint {
		t.Fatal("append changed the fingerprint of the previously indexed prefix")
	}
	changed := append([]byte(nil), base...)
	changed[bytes.Index(changed, []byte("root-001"))] = 'R'
	if CheckpointFingerprint(changed, int64(len(changed))) == oldCheckpoint {
		t.Fatal("changed indexed prefix was not detected")
	}
}

func TestArchiveMoveIdentityFixture(t *testing.T) {
	var inventory struct {
		Active struct {
			Path               string `json:"path"`
			SourceKind         string `json:"sourceKind"`
			SessionID          string `json:"sessionId"`
			SegmentFingerprint string `json:"segmentFingerprint"`
		} `json:"active"`
		AfterArchiveMove struct {
			Path               string `json:"path"`
			SourceKind         string `json:"sourceKind"`
			SessionID          string `json:"sessionId"`
			SegmentFingerprint string `json:"segmentFingerprint"`
		} `json:"afterArchiveMove"`
		ExpectedLogicalArtifacts int `json:"expectedLogicalArtifacts"`
	}
	loadJSON(t, repoPath("fixtures", "synthetic", "source-inventory.json"), &inventory)
	identities := map[string]bool{
		inventory.Active.SessionID + ":" + inventory.Active.SegmentFingerprint:                     true,
		inventory.AfterArchiveMove.SessionID + ":" + inventory.AfterArchiveMove.SegmentFingerprint: true,
	}
	if len(identities) != inventory.ExpectedLogicalArtifacts {
		t.Fatalf("logical artifacts = %d, want %d", len(identities), inventory.ExpectedLogicalArtifacts)
	}
	data, err := os.ReadFile(repoPath("fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	active, err := NormalizeCorpus([]CorpusInput{{Name: "sessions/root.jsonl", Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := NormalizeCorpus([]CorpusInput{{Name: "archived_sessions/root.jsonl", Data: data}})
	if err != nil {
		t.Fatal(err)
	}
	if active.Sources[0].SourceID != archived.Sources[0].SourceID || active.Sources[0].SegmentID != archived.Sources[0].SegmentID || active.Sources[0].SegmentFingerprint != archived.Sources[0].SegmentFingerprint {
		t.Fatalf("active-to-archive move changed normalized identity: active=%+v archived=%+v", active.Sources[0], archived.Sources[0])
	}
}

func TestSessionIndexFixtureShape(t *testing.T) {
	f, err := os.Open(repoPath("fixtures", "synthetic", "session_index.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	count := 0
	for {
		var entry struct {
			ID         string `json:"id"`
			ThreadName string `json:"thread_name"`
			UpdatedAt  string `json:"updated_at"`
		}
		if err := decoder.Decode(&entry); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if entry.ID == "" || entry.ThreadName == "" || entry.UpdatedAt == "" {
			t.Fatalf("incomplete session index entry: %+v", entry)
		}
		count++
	}
	if count != 2 {
		t.Fatalf("session index entries = %d, want 2", count)
	}
}

func TestNegativeCumulativeDeltaRejected(t *testing.T) {
	_, err := subtractUsage(Usage{Total: 9}, Usage{Total: 10})
	if err == nil {
		t.Fatal("negative cumulative delta was accepted")
	}
}

func TestExecThreadStartedContract(t *testing.T) {
	f, err := os.Open(repoPath("fixtures", "codex-exec", "thread-started.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	id, err := ParseThreadStarted(f)
	if err != nil {
		t.Fatal(err)
	}
	if id != "fake-review-thread" {
		t.Fatalf("thread id = %q", id)
	}
}

func TestReviewSchemas(t *testing.T) {
	metric := func(value any, fidelity string) map[string]any {
		return map[string]any{"formulaVersion": float64(1), "fidelity": fidelity, "value": value}
	}
	manifest := map[string]any{
		"schemaVersion":      "inspector.review/v1",
		"reviewId":           "review-001",
		"createdAt":          "2026-07-18T12:00:00Z",
		"datasetEpoch":       "epoch-001",
		"indexRevision":      float64(1),
		"scope":              map[string]any{"kind": "single_session", "rootSessionId": "root-001", "descendantSessionIds": []any{"child-001"}, "appliedRevision": float64(1)},
		"includedSessionIds": []any{"root-001", "child-001"},
		"includedTurnIds":    []any{"turn-root-1"},
		"sources":            []any{map[string]any{"sourceId": "source-001", "sourceKind": "active_rollout", "locator": "/fake/root.jsonl", "fingerprint": strings.Repeat("a", 64)}},
		"aggregateMetrics": map[string]any{
			"recorded_tokens": metric(float64(2500), "exact"), "recorded_tokens_by_kind": metric(map[string]any{"user_root_direct": float64(2000), "descendant": float64(500), "inspector_review": float64(0), "other_orphan": float64(0)}, "exact"),
			"recorded_tokens_over_time": metric([]any{}, "exact"), "token_composition": metric(map[string]any{"uncached_input": float64(1350), "cached_input": float64(600), "visible_output": float64(440), "reasoning_output": float64(110), "residual": float64(0)}, "exact"), "top_root_sessions_by_tokens": metric([]any{}, "exact"),
			"latest_capacity_observation": metric(map[string]any{"observedAt": "2026-07-01T11:00:04Z", "limitId": "fake-plan", "windowMinutes": float64(300), "usedPercent": float64(45), "resetsAt": float64(1782910800)}, "exact"), "capacity_drawdown": metric([]any{}, "exact"),
		},
		"coverageGaps":      []any{},
		"evidenceRules":     map[string]any{"citationShape": "manifest_evidence_id", "treatAsUntrusted": true},
		"rubric":            []any{"task_framing_and_steering", "execution_efficiency", "delegation_and_workflow", "reusable_leverage"},
		"model":             "gpt-fake",
		"reasoning":         "medium",
		"evidence":          []any{map[string]any{"evidenceId": "evidence-001", "sourceId": "source-001", "sourcePrefixSha256": strings.Repeat("a", 64), "eventFingerprint": strings.Repeat("b", 64), "adapterVersion": AdapterVersion, "recordOrdinal": float64(3), "byteStart": float64(100), "byteEnd": float64(200), "sessionId": "root-001", "turnId": "turn-root-1", "eventKind": "message", "observedAt": "2026-07-01T10:00:03Z", "availability": "available", "availabilityObservedAt": "2026-07-18T12:00:00Z", "availabilityRevision": nil}},
		"reportDestination": "./review.json",
		"reportSchema":      "./report.schema.json",
		"limits":            map[string]any{"maxFindings": float64(5), "maxReportBytes": float64(1048576)},
	}
	run := map[string]any{
		"schemaVersion": "inspector.review/v1", "reviewId": "review-001", "status": "running",
		"createdAt": "2026-07-18T12:00:00Z", "startedAt": "2026-07-18T12:00:01Z", "command": []any{"codex", "exec", "--json"}, "threadId": "fake-review-thread", "pid": float64(1234), "launchPromptVersion": "inspector.review-launch/v2",
		"review": map[string]any{"reviewId": "review-001", "datasetEpoch": "epoch-001", "indexRevision": float64(1), "scope": map[string]any{"kind": "single_session", "rootSessionId": "root-001"}, "model": "gpt-fake", "reasoning": "medium"},
	}
	report := map[string]any{
		"schemaVersion": "inspector.review/v1", "reviewId": "review-001", "summary": "A fake review summary.",
		"scope": map[string]any{"kind": "single_session", "summary": "One fake root and descendant.", "datasetEpoch": "epoch-001", "indexRevision": float64(1)},
		"model": "gpt-fake", "reasoning": "medium", "completedAt": "2026-07-18T12:05:00Z",
		"findings": []any{map[string]any{
			"findingId": "finding-001", "kind": "strength", "lens": "task_framing_and_steering", "title": "Clear fake task",
			"observation": "The fake prompt named one output.", "impact": "The fake scope stayed bounded.", "support": "directly_observed", "evidenceSummary": "The fake prompt named one output.", "citations": []any{"evidence-001"},
			"recommendation": "Keep using bounded fake scopes.",
		}},
	}
	validateSchema(t, "manifest.schema.json", manifest)
	validateSchema(t, "run.schema.json", run)
	validateSchema(t, "report.schema.json", report)
	badManifest := cloneMap(t, manifest)
	badEvidence := badManifest["evidence"].([]any)[0].(map[string]any)
	delete(badEvidence, "availabilityObservedAt")
	if err := schemaFor(t, "manifest.schema.json").Validate(badManifest); err == nil {
		t.Fatal("manifest accepted live availability without availabilityObservedAt")
	}

	badReport := cloneMap(t, report)
	badReport["findings"] = []any{report["findings"].([]any)[0], report["findings"].([]any)[0], report["findings"].([]any)[0], report["findings"].([]any)[0], report["findings"].([]any)[0], report["findings"].([]any)[0]}
	if err := schemaFor(t, "report.schema.json").Validate(badReport); err == nil {
		t.Fatal("report schema accepted more than five findings")
	}
}

func TestOpenAPIContractCoverage(t *testing.T) {
	contractPath := repoPath("schemas", "internal-api.openapi.json")
	document, err := openapi3.NewLoader().LoadFromFile(contractPath)
	if err != nil {
		t.Fatalf("load OpenAPI: %v", err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatalf("validate OpenAPI: %v", err)
	}
	var doc struct {
		OpenAPI    string                     `json:"openapi"`
		Paths      map[string]json.RawMessage `json:"paths"`
		Components struct {
			Schemas map[string]map[string]any `json:"schemas"`
		} `json:"components"`
	}
	data, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.OpenAPI != "3.1.0" {
		t.Fatalf("OpenAPI version = %q", doc.OpenAPI)
	}
	required := []string{"/v1/health", "/v1/heartbeat", "/v1/status", "/v1/sync", "/v1/metrics/catalog", "/v1/metrics/query", "/v1/sessions", "/v1/sessions/{sessionId}/map", "/v1/sessions/{sessionId}/turns/{turnId}/ledger", "/v1/evidence/{evidenceId}", "/v1/context/{evidenceId}", "/v1/review-plans", "/v1/reviews", "/v1/reviews/{reviewId}", "/v1/events"}
	for _, path := range required {
		if _, ok := doc.Paths[path]; !ok {
			t.Errorf("OpenAPI missing %s", path)
		}
	}
	for _, name := range []string{"StatusChangedData", "RevisionAvailableData", "SyncProgressData", "ReviewChangedData", "HeartbeatData", "StatusEvent"} {
		if document.Components.Schemas[name] == nil {
			t.Errorf("OpenAPI missing SSE schema %s", name)
		}
	}
	assertSchemaRequired(t, doc.Components.Schemas, "ProcessStatus", "state", "inspectorVersion", "cliVersion", "cliCompatibility", "pluginVersion", "pluginProtocolVersion", "pid", "startedAt")
	assertSchemaRequired(t, doc.Components.Schemas, "Snapshot", "schemaVersion", "datasetEpoch", "appliedRevision", "coverage")
	assertSchemaRequired(t, doc.Components.Schemas, "IndexStatus", "state", "datasetEpoch", "appliedRevision", "schemaVersion", "databaseBytes", "sourceCount", "supportedSourceCount", "unsupportedSourceCount", "pendingTailCount", "queuedSessionChanges", "processedCount", "queuedCount", "skippedCount", "failedCount", "requiresRebuildCount", "diagnosticGroups", "reverseScanBoundary", "completedWatermark")
	assertSchemaRequired(t, doc.Components.Schemas, "HookStatus", "state", "registeredEvents", "lastMarker", "diagnostics")
	assertSchemaRequired(t, doc.Components.Schemas, "MetricMetadata", "formulaVersion", "fidelity", "coverage", "indexedCoverage", "timeBoundary", "exclusionReasons")
	assertSchemaRequired(t, doc.Components.Schemas, "IndexedCoverage", "indexedStart", "indexedEnd", "completedWatermark")
	assertSchemaRequired(t, doc.Components.Schemas, "MetricTimeBoundary", "requestedStart", "requestedEnd", "effectiveStart", "effectiveEnd", "timezone")
	assertSchemaRequired(t, doc.Components.Schemas, "CapacityPoint", "observedAt", "limitId", "windowMinutes", "usedPercent", "remainingPercent", "resetsAt", "stale")
	assertSchemaRequired(t, doc.Components.Schemas, "SessionMap", "rootSessionId", "nodes", "edges", "rootTurns", "spawnTopology")
	assertSchemaRequired(t, doc.Components.Schemas, "RootTurn", "turnId", "ordinal", "state", "startedAt")
	if properties, _ := doc.Components.Schemas["MapTurn"]["properties"].(map[string]any); properties["promptPreview"] == nil {
		t.Error("MapTurn is missing its optional exact prompt preview")
	}
	assertSchemaRequired(t, doc.Components.Schemas, "SpawnTurnTopology", "parentSessionId", "childSessionId", "spawnTurnId", "edgeKind", "ordinal")
	assertSchemaRequired(t, doc.Components.Schemas, "ReviewManifestPreview", "schemaVersion", "reviewId", "createdAt", "datasetEpoch", "indexRevision", "scope", "includedSessionIds", "includedTurnIds", "sources", "aggregateMetrics", "coverageGaps", "evidenceRules", "rubric", "model", "reasoning", "evidence", "reportDestination", "reportSchema", "limits")
	assertSchemaRequired(t, doc.Components.Schemas, "ManifestEvidence", "evidenceId", "sourceId", "sourcePrefixSha256", "eventFingerprint", "availability", "availabilityObservedAt", "availabilityRevision")
	assertSchemaRequired(t, doc.Components.Schemas, "ManifestAggregateMetrics", "recorded_tokens", "recorded_tokens_by_kind", "recorded_tokens_over_time", "token_composition", "top_root_sessions_by_tokens", "latest_capacity_observation", "capacity_drawdown")
	assertSchemaRequired(t, doc.Components.Schemas, "ReviewSpec", "reviewId", "datasetEpoch", "indexRevision", "scope", "model", "reasoning")
	assertSchemaRequired(t, doc.Components.Schemas, "ReviewDetail", "summary", "review", "run", "reportState", "acceptedReport")
	assertSchemaRequired(t, doc.Components.Schemas, "ReviewPlan", "schemaVersion", "datasetEpoch", "appliedRevision", "planId", "review", "launchPrompt")
	assertSchemaRequired(t, doc.Components.Schemas, "ReviewSourceByteCounts", "sourceCount", "discoveredBytes", "indexedBytes", "includedBytes")
	assertSchemaRequired(t, doc.Components.Schemas, "ReviewProjectSummary", "projectCount", "projects")
	assertSchemaRequired(t, doc.Components.Schemas, "EvidenceLocator", "sourceId", "recordOrdinal", "byteStart", "byteEnd")
	assertSchemaRequired(t, doc.Components.Schemas, "EvidenceAvailability", "availability", "availabilityObservedAt", "availabilityRevision")
	assertSchemaRequired(t, doc.Components.Schemas, "EvidenceChunk", "evidenceId", "locator", "sourcePrefixSha256", "eventFingerprint", "offset", "bytes", "complete")
	assertSchemaRequired(t, doc.Components.Schemas, "ContextBlock", "evidenceId", "kind", "locator", "sourcePrefixSha256", "eventFingerprint")
	assertSchemaRequired(t, doc.Components.Schemas, "ReviewCitationState", "evidenceId", "sourcePrefixSha256", "eventFingerprint", "rootSessionId", "turnId")
	assertSchemaRequired(t, doc.Components.Schemas, "RevisionAvailableData", "schemaVersion", "datasetEpoch", "revision", "fullRefreshRequired")
	metricItems, ok := doc.Components.Schemas["MetricItem"]["oneOf"].([]any)
	if !ok || len(metricItems) != 8 {
		t.Fatalf("MetricItem variants = %d, want all eight", len(metricItems))
	}
	wantMetricRefs := map[string]bool{"#/components/schemas/RecordedTokensMetric": true, "#/components/schemas/RecordedTokensByKindMetric": true, "#/components/schemas/RecordedTokensOverTimeMetric": true, "#/components/schemas/RecordedTokensByModelReasoningOverTimeMetric": true, "#/components/schemas/TokenCompositionMetric": true, "#/components/schemas/TopRootSessionsMetric": true, "#/components/schemas/LatestCapacityMetric": true, "#/components/schemas/CapacityDrawdownMetric": true}
	for _, item := range metricItems {
		object, _ := item.(map[string]any)
		ref, _ := object["$ref"].(string)
		if !wantMetricRefs[ref] {
			t.Errorf("MetricItem has unexpected variant %q", ref)
		}
		delete(wantMetricRefs, ref)
	}
	if len(wantMetricRefs) != 0 {
		t.Errorf("MetricItem missing variants: %v", wantMetricRefs)
	}
	for _, name := range []string{"RecordedTokensMetric", "RecordedTokensByKindMetric", "RecordedTokensOverTimeMetric", "RecordedTokensByModelReasoningOverTimeMetric", "TokenCompositionMetric", "TopRootSessionsMetric", "LatestCapacityMetric", "CapacityDrawdownMetric"} {
		allOf, _ := doc.Components.Schemas[name]["allOf"].([]any)
		if len(allOf) == 0 {
			t.Errorf("%s does not inherit exact metric metadata", name)
			continue
		}
		base, _ := allOf[0].(map[string]any)
		if base["$ref"] != "#/components/schemas/MetricMetadata" {
			t.Errorf("%s metadata ref=%v", name, base["$ref"])
		}
	}
	capacityProperties, _ := doc.Components.Schemas["CapacityPoint"]["properties"].(map[string]any)
	remaining, _ := capacityProperties["remainingPercent"].(map[string]any)
	remainingTypes, _ := remaining["type"].([]any)
	if !reflect.DeepEqual(remainingTypes, []any{"number", "null"}) {
		t.Errorf("CapacityPoint.remainingPercent type=%v, want honest nullable number", remainingTypes)
	}
	latestAllOf, _ := doc.Components.Schemas["LatestCapacityMetric"]["allOf"].([]any)
	latestShape, _ := latestAllOf[1].(map[string]any)
	latestProperties, _ := latestShape["properties"].(map[string]any)
	latestValue, _ := latestProperties["value"].(map[string]any)
	latestItems, _ := latestValue["items"].(map[string]any)
	if latestValue["type"] != "array" || latestValue["maxItems"] != float64(100) || latestItems["$ref"] != "#/components/schemas/CapacityPoint" {
		t.Errorf("LatestCapacityMetric.value=%v, want bounded CapacityPoint array", latestValue)
	}
	diagnosticProperties, _ := doc.Components.Schemas["SourceDiagnosticGroup"]["properties"].(map[string]any)
	for _, field := range []string{"reason", "detectedVersion", "remediation"} {
		property, _ := diagnosticProperties[field].(map[string]any)
		if property["maxLength"] == nil {
			t.Errorf("SourceDiagnosticGroup.%s is not string-bounded", field)
		}
	}
	diagnosticReason, _ := diagnosticProperties["reason"].(map[string]any)
	diagnosticVersion, _ := diagnosticProperties["detectedVersion"].(map[string]any)
	if diagnosticReason["enum"] == nil || diagnosticVersion["pattern"] == nil {
		t.Errorf("SourceDiagnosticGroup must constrain reason codes and detected versions: reason=%v version=%v", diagnosticReason, diagnosticVersion)
	}
	reasonEnum, _ := diagnosticReason["enum"].([]any)
	if !slices.Contains(reasonEnum, any("unsupported_codex_version")) {
		t.Errorf("SourceDiagnosticGroup.reason must publish unsupported_codex_version: %v", reasonEnum)
	}
	indexProperties, _ := doc.Components.Schemas["IndexStatus"]["properties"].(map[string]any)
	diagnosticGroups, _ := indexProperties["diagnosticGroups"].(map[string]any)
	if diagnosticGroups["maxItems"] != float64(50) {
		t.Errorf("IndexStatus.diagnosticGroups maxItems=%v, want 50", diagnosticGroups["maxItems"])
	}
	rootTurnProperties, _ := doc.Components.Schemas["RootTurn"]["properties"].(map[string]any)
	stateSchema, _ := rootTurnProperties["state"].(map[string]any)
	states, _ := stateSchema["enum"].([]any)
	wantStates := map[string]bool{"completed": true, "aborted": true, "interrupted": true, "reconciled_truncated": true}
	if len(states) != len(wantStates) {
		t.Fatalf("RootTurn states=%v, want only committed/terminal states", states)
	}
	for _, state := range states {
		text, _ := state.(string)
		if !wantStates[text] || text == "provisional" || text == "active" {
			t.Errorf("RootTurn exposes forbidden state %q", text)
		}
		delete(wantStates, text)
	}
	if len(wantStates) != 0 {
		t.Errorf("RootTurn missing states %v", wantStates)
	}
}

func TestGeneratedTypeScriptContractExists(t *testing.T) {
	data, err := os.ReadFile(repoPath("web", "src", "generated", "internal-api.ts"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"auto-generated by openapi-typescript", "streamEvents", "StatusChangedEvent", "RevisionAvailableEvent", "ProcessStatus", "IndexStatus", "schemaVersion: 2", "queuedSessionChanges", "requiresRebuildCount", "HookDiagnostic", "RecordedTokensMetric", "indexedCoverage", "timeBoundary", "CapacityDrawdownMetric", "remainingPercent", "RootTurn", "SpawnTurnTopology", "EvidenceLocator", "EvidenceAvailability", "sourcePrefixSha256", "availabilityObservedAt", "ReviewSpec", "launchPrompt", "AcceptedReviewReport", "ReviewCitationState", "rootSessionId", "turnId"} {
		if !strings.Contains(text, required) {
			t.Errorf("generated TypeScript missing %q", required)
		}
	}
	command := exec.Command("scripts/phase0/generate-internal-api.sh", "--check")
	command.Dir = repoPath()
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated TypeScript is not reproducible: %v\n%s", err, output)
	}
}

func assertSchemaRequired(t *testing.T, schemas map[string]map[string]any, name string, fields ...string) {
	t.Helper()
	schema, ok := schemas[name]
	if !ok {
		t.Fatalf("OpenAPI missing schema %s", name)
	}
	required := map[string]bool{}
	properties := map[string]bool{}
	var collect func(map[string]any)
	collect = func(value map[string]any) {
		if list, ok := value["required"].([]any); ok {
			for _, field := range list {
				if text, ok := field.(string); ok {
					required[text] = true
				}
			}
		}
		if fields, ok := value["properties"].(map[string]any); ok {
			for field := range fields {
				properties[field] = true
			}
		}
		if all, ok := value["allOf"].([]any); ok {
			for _, item := range all {
				if object, ok := item.(map[string]any); ok {
					collect(object)
				}
			}
		}
	}
	collect(schema)
	for _, field := range fields {
		if !required[field] {
			t.Errorf("OpenAPI schema %s does not require %s", name, field)
		}
		if !properties[field] {
			t.Errorf("OpenAPI schema %s does not define %s", name, field)
		}
	}
}

func completedTurns(source SourceDecision) int {
	count := 0
	for _, turn := range source.Turns {
		if turn.Completed {
			count++
		}
	}
	return count
}

func loadJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func schemaFor(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	return compileSchema(t, repoPath("schemas", "reviews", name), name)
}

func compileSchema(t *testing.T, path, resourceName string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := compiler.AddResource(resourceName, bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(resourceName)
	if err != nil {
		t.Fatalf("compile %s: %v", resourceName, err)
	}
	return schema
}

func validateSchema(t *testing.T, name string, value any) {
	t.Helper()
	if err := schemaFor(t, name).Validate(value); err != nil {
		t.Fatalf("validate %s: %v", name, err)
	}
}

func cloneMap(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(data, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}

func TestHookFixturesContainOnlyExpectedShape(t *testing.T) {
	markerSchema := compileSchema(t, repoPath("schemas", "hook-marker.schema.json"), "hook-marker.schema.json")
	entries, err := os.ReadDir(repoPath("fixtures", "hooks"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(repoPath("fixtures", "hooks", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if !json.Valid(data) {
			t.Errorf("%s is invalid JSON", entry.Name())
		}
		if strings.Contains(string(data), "secret") {
			t.Errorf("%s contains forbidden payload", entry.Name())
		}
		if entry.Name() == "marker.json" {
			var marker any
			if err := json.Unmarshal(data, &marker); err != nil {
				t.Fatal(err)
			}
			if err := markerSchema.Validate(marker); err != nil {
				t.Fatalf("validate marker: %v", err)
			}
		} else {
			marker, decision := MapHookPayload(data, mustTime(t, "2026-07-18T12:00:00Z"))
			if decision != "supported" {
				t.Fatalf("%s decision=%s", entry.Name(), decision)
			}
			encoded, err := json.Marshal(marker)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(encoded), "Fake prompt") || strings.Contains(string(encoded), "gpt-fake") || strings.Contains(string(encoded), "permission_mode") {
				t.Fatalf("%s leaked non-marker hook fields: %s", entry.Name(), encoded)
			}
			var value any
			if err := json.Unmarshal(encoded, &value); err != nil {
				t.Fatal(err)
			}
			if err := markerSchema.Validate(value); err != nil {
				t.Fatalf("mapped %s: %v", entry.Name(), err)
			}
		}
	}
	_, decision := MapHookPayload([]byte(`{"session_id":"fake","hook_event_name":"FutureHook"}`), mustTime(t, "2026-07-18T12:00:00Z"))
	if decision != "unsupported_hook_event" {
		t.Fatalf("future hook decision=%s", decision)
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}
