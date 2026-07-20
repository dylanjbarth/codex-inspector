package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"testing"

	"github.com/dylanjbarth/codex-inspector/internal/facts"
	"github.com/dylanjbarth/codex-inspector/internal/phase0"
)

func TestProductionAdapterUsesFrozenPhase0Discriminator(t *testing.T) {
	for _, name := range []string{"root.jsonl", "descendant.jsonl", "truncated.jsonl", "unsupported.jsonl"} {
		t.Run(name, func(t *testing.T) {
			path := fixture(t, name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			decision, err := phase0.ParseRollout(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			info, _ := os.Stat(path)
			kind := "active_rollout"
			if name == "descendant.jsonl" || name == "unsupported.jsonl" {
				kind = "archived_rollout"
			}
			batch, err := Parse(Candidate{Path: path, Kind: kind, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if got := batch.Source.State == "supported"; got != decision.Supported {
				t.Fatalf("support decision diverged: production=%t phase0=%t", got, decision.Supported)
			}
			if batch.Source.StateReason != map[bool]string{true: "", false: decision.Reason}[decision.Supported] || batch.Source.SessionID != decision.SessionID || (batch.Source.PendingTail > 0) != decision.PendingTail {
				t.Fatalf("decision projection diverged: source=%#v decision=%#v", batch.Source, decision)
			}
			terminal := 0
			for _, turn := range decision.Turns {
				if turn.Completed {
					terminal++
				}
			}
			if len(batch.Turns) != terminal {
				t.Fatalf("terminal projection diverged: production=%d phase0=%d", len(batch.Turns), terminal)
			}
		})
	}
}

func TestProductionAdapterDeepEqualsFrozenPhase0Golden(t *testing.T) {
	var expected phase0.NormalizedCorpus
	golden, err := os.ReadFile(fixture(t, "expected-facts.json"))
	if err != nil || json.Unmarshal(golden, &expected) != nil {
		t.Fatalf("load golden: %v", err)
	}
	actual := phase0.NormalizedCorpus{AdapterVersion: AdapterVersion}
	for _, name := range []string{"root.jsonl", "descendant.jsonl", "unsupported.jsonl", "truncated.jsonl"} {
		path := fixture(t, name)
		info, _ := os.Stat(path)
		kind := "active_rollout"
		if name == "descendant.jsonl" || name == "unsupported.jsonl" {
			kind = "archived_rollout"
		}
		batch, parseErr := Parse(Candidate{Path: path, Kind: kind, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		decision := batch.Source.StateReason
		if batch.Source.State == "supported" {
			decision = "supported"
		}
		segmentID, segmentFingerprint := batch.Source.SegmentID, batch.Source.SegmentFingerprint
		if batch.Source.State != "supported" {
			segmentID, segmentFingerprint = "", ""
		}
		actual.Sources = append(actual.Sources, phase0.NormalizedSource{Name: name, SourceID: batch.Source.ID, IdentityFingerprint: batch.Source.SegmentFingerprint, CurrentFingerprint: batch.Source.PrefixSHA256, SegmentID: segmentID, SegmentFingerprint: segmentFingerprint, SessionID: batch.Source.SessionID, Decision: decision, PendingTail: batch.Source.PendingTail > 0, CompleteBytes: batch.Source.CompleteOffset, AdapterVersion: batch.Source.AdapterVersion})
		if batch.Session == nil {
			continue
		}
		parent := ""
		if len(batch.Lineage) > 0 {
			parent = lineageSourceID(batch.Lineage[0].ParentSessionID, []string{"root-001", "child-001"})
		}
		projectIdentity := batch.Project.Identity
		if remote := batch.Project.Aliases["remote"]; remote != "" {
			projectIdentity = remote
		}
		actual.Sessions = append(actual.Sessions, phase0.NormalizedSession{SessionID: batch.Session.SourceSessionID, RootWorkUnitID: map[bool]string{true: parent, false: batch.Session.SourceSessionID}[parent != ""], ParentSessionID: parent, Purpose: batch.Session.Purpose, ProjectIdentity: projectIdentity, SourceID: batch.Source.ID, SegmentID: batch.Source.SegmentID, AdapterVersion: AdapterVersion})
		turnSource := map[string]string{}
		for _, turn := range batch.Turns {
			turnSource[turn.ID] = turn.SourceTurnID
			contribution := "user_root_direct"
			if batch.Session.Purpose == "spawned" {
				contribution = "descendant"
			}
			actual.Turns = append(actual.Turns, phase0.NormalizedTurn{TurnID: turn.SourceTurnID, SessionID: batch.Session.SourceSessionID, State: turn.State, CompletedAt: turn.CompletedAt, Model: turn.Model, ReasoningEffort: turn.ReasoningEffort, ContributionKind: contribution, NormalizationKind: turn.NormalizationKind, Usage: phase0Usage(turn.Usage), AdapterVersion: AdapterVersion})
		}
		eventByID := map[string]facts.Event{}
		for _, event := range batch.Events {
			eventByID[event.ID] = event
			if event.Kind == "lineage_observation" {
				continue
			}
			actual.Events = append(actual.Events, phase0.NormalizedEvent{EventID: event.ID, SourceID: batch.Source.ID, SegmentID: batch.Source.SegmentID, RecordOrdinal: event.RecordOrdinal, ByteStart: event.ByteStart, ByteEnd: event.ByteEnd, RecordType: event.RecordType, EventKind: event.Kind, ObservedAt: event.ObservedAt, TurnID: turnSource[event.TurnID], ContentSHA256: event.ContentSHA256, AdapterVersion: AdapterVersion})
		}
		for _, message := range batch.Messages {
			event := eventByID[message.EventID]
			actual.Messages = append(actual.Messages, phase0.NormalizedMessage{EventID: message.EventID, MessageID: message.SourceMessageID, TurnID: turnSource[event.TurnID], Role: message.Role, Phase: message.Phase, ContentLength: int(message.ContentLength), ContentSHA256: message.ContentSHA256, RecordOrdinal: event.RecordOrdinal, ByteStart: event.ByteStart, ByteEnd: event.ByteEnd})
		}
		for _, tool := range batch.Tools {
			actual.ToolPhases = append(actual.ToolPhases, phase0.NormalizedToolPhase{CallID: tool.SourceCallID, SessionID: batch.Session.SourceSessionID, TurnID: turnSource[tool.TurnID], SemanticPhase: tool.Phase, ToolName: tool.Name, EventID: tool.EventID})
		}
		for _, compaction := range batch.Compactions {
			actual.Compactions = append(actual.Compactions, phase0.NormalizedCompaction{EventID: compaction.EventID, TriggerEventID: compaction.TriggerKind, FirstWindowID: compaction.FirstWindowID, PreviousWindowID: compaction.PreviousWindowID, WindowID: compaction.WindowID, WindowNumber: compaction.WindowNumber, ContentLength: int(*compaction.SummaryLength), ContentSHA256: compaction.SummarySHA256})
		}
		for _, capacity := range batch.Capacity {
			reset, _ := strconv.ParseInt(capacity.ResetsAt, 10, 64)
			actual.Capacity = append(actual.Capacity, phase0.NormalizedCapacity{EventID: capacity.EventID, ObservedAt: capacity.ObservedAt, LimitID: capacity.LimitID, WindowMinutes: *capacity.WindowMinutes, UsedPercent: *capacity.UsedPercent, ResetsAt: reset})
		}
		for _, coverage := range batch.Coverage {
			if coverage.ScopeKind != "turn" {
				continue
			}
			actual.Coverage = append(actual.Coverage, phase0.NormalizedCoverage{ScopeKind: coverage.ScopeKind, ScopeID: mapID(coverage.ScopeID, turnSource), Field: coverage.FieldKey, Fidelity: coverage.Fidelity, Observed: coverage.Observed, Eligible: coverage.Eligible, Reason: coverage.Reason})
		}
		for _, evidence := range batch.Evidence {
			event := eventByID[evidence.EventID]
			actual.Evidence = append(actual.Evidence, phase0.NormalizedEvidence{EvidenceID: evidence.ID, EventID: evidence.EventID, SourceID: batch.Source.ID, SourcePrefixSHA256: evidence.SourceFingerprint, EventFingerprint: evidence.EventFingerprint, RecordOrdinal: event.RecordOrdinal, ByteStart: event.ByteStart, ByteEnd: event.ByteEnd, Availability: evidence.Availability, AdapterVersion: AdapterVersion})
		}
	}
	sections := []struct {
		name             string
		actual, expected any
	}{{"sources", actual.Sources, expected.Sources}, {"sessions", actual.Sessions, expected.Sessions}, {"turns", actual.Turns, expected.Turns}, {"events", actual.Events, expected.Events}, {"messages", actual.Messages, expected.Messages}, {"tools", actual.ToolPhases, expected.ToolPhases}, {"compactions", actual.Compactions, expected.Compactions}, {"capacity", actual.Capacity, expected.Capacity}, {"coverage", actual.Coverage, expected.Coverage}, {"evidence", actual.Evidence, expected.Evidence}}
	for i := range actual.Events {
		if !reflect.DeepEqual(actual.Events[i], expected.Events[i]) {
			t.Fatalf("production adapter event[%d] diverged: got=%#v want=%#v", i, actual.Events[i], expected.Events[i])
		}
	}
	for _, section := range sections {
		if reflect.DeepEqual(section.actual, section.expected) {
			continue
		}
		actualJSON, _ := json.MarshalIndent(actual, "", "  ")
		expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
		t.Fatalf("production adapter %s projection diverged from frozen golden\nactual=%s\nexpected=%s", section.name, actualJSON, expectedJSON)
	}
}

func phase0Usage(usage *facts.Usage) *phase0.Usage {
	if usage == nil {
		return nil
	}
	value := func(p *int64) int64 {
		if p == nil {
			return 0
		}
		return *p
	}
	return &phase0.Usage{Input: value(usage.Input), Cached: value(usage.CachedInput), Output: value(usage.Output), Reasoning: value(usage.ReasoningOutput), Total: value(usage.Total)}
}
func mapID(id string, values map[string]string) string {
	if value := values[id]; value != "" {
		return value
	}
	return id
}
func lineageSourceID(id string, candidates []string) string {
	for _, candidate := range candidates {
		if id == "session:"+hash([]byte(candidate)) {
			return candidate
		}
	}
	return ""
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "fixtures", "synthetic", name)
}

func TestFrozenAdapterGoldenFactsAndUnsupportedVersion(t *testing.T) {
	root := fixture(t, "root.jsonl")
	info, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Parse(Candidate{Path: root, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, map[string]string{"root-001": "Fake widget work"})
	if err != nil {
		t.Fatal(err)
	}
	if b.Source.State != "supported" || b.Session == nil || b.Session.Title != "Fake widget work" {
		t.Fatalf("unexpected adapter decision: %#v %#v", b.Source, b.Session)
	}
	if len(b.Turns) != 2 || b.Turns[0].Usage == nil || *b.Turns[0].Usage.Total != 1200 || b.Turns[1].NormalizationKind != "cumulative_delta" || *b.Turns[1].Usage.Total != 800 {
		t.Fatalf("usage normalization mismatch: %#v", b.Turns)
	}
	if len(b.Tools) != 2 || len(b.Compactions) != 1 || len(b.Capacity) != 2 {
		t.Fatalf("mirrored tool/compaction/capacity mismatch: tools=%d compactions=%d capacity=%d", len(b.Tools), len(b.Compactions), len(b.Capacity))
	}
	unsupported := fixture(t, "unsupported.jsonl")
	info, _ = os.Stat(unsupported)
	u, err := Parse(Candidate{Path: unsupported, Kind: "archived_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if u.Source.State != "unsupported" || u.Source.StateReason != "incompatible_record_envelope" || u.Session != nil {
		t.Fatalf("unsupported source parsed optimistically: %#v", u)
	}
}

func TestAdapterEmitsExactFrozenOrderKeysIncludingTimestampTies(t *testing.T) {
	rollout := func(marker, turn string) string {
		return fmt.Sprintf(`{"timestamp":"2026-07-01T10:00:00Z","type":"session_meta","payload":{"session_id":"order-session","timestamp":"2026-07-01T10:00:00Z","cwd":"/fake/order","originator":"codex-tui","cli_version":"0.144.1","source":"cli","test_marker":%q}}`+"\n"+
			`{"timestamp":"2026-07-01T10:00:00Z","type":"turn_context","payload":{"turn_id":%q,"model":"gpt-fake","effort":"low","cwd":"/fake/order"}}`+"\n"+
			`{"timestamp":"2026-07-01T10:00:00Z","type":"event_msg","payload":{"type":"task_started","turn_id":%q}}`+"\n"+
			`{"timestamp":"2026-07-01T10:00:00Z","type":"event_msg","payload":{"type":"task_complete","turn_id":%q}}`+"\n", marker, turn, turn, turn)
	}
	var batches []facts.Batch
	for index, marker := range []string{"a", "b"} {
		path := filepath.Join(t.TempDir(), fmt.Sprintf("tie-%d.jsonl", index))
		if err := os.WriteFile(path, []byte(rollout(marker, "turn-"+marker)), 0o600); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		batch, err := Parse(Candidate{Path: path, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
		if err != nil {
			t.Fatal(err)
		}
		wantSegment, err := phase0.SourceOrderKey("2026-07-01T10:00:00Z", batch.Segment.Fingerprint)
		if err != nil {
			t.Fatal(err)
		}
		if batch.Segment.StartedAt != "2026-07-01T10:00:00Z" || batch.Segment.SourceOrderKey != wantSegment {
			t.Fatalf("segment key=%q want=%q", batch.Segment.SourceOrderKey, wantSegment)
		}
		for _, turn := range batch.Turns {
			want, keyErr := phase0.TurnOrderKey(wantSegment, turn.Ordinal)
			if keyErr != nil || turn.SourceOrderKey != want {
				t.Fatalf("turn key=%q want=%q err=%v", turn.SourceOrderKey, want, keyErr)
			}
		}
		for _, event := range batch.Events {
			want, keyErr := phase0.EventOrderKey(wantSegment, event.RecordOrdinal, event.SemanticPhase)
			if keyErr != nil || event.SourceOrderKey != want {
				t.Fatalf("event key=%q want=%q err=%v", event.SourceOrderKey, want, keyErr)
			}
		}
		batches = append(batches, batch)
	}
	if batches[0].Segment.Fingerprint < batches[1].Segment.Fingerprint != (batches[0].Segment.SourceOrderKey < batches[1].Segment.SourceOrderKey) {
		t.Fatal("equal-timestamp segment keys do not use fingerprint tie-break")
	}
}

func TestTruncatedTailNeverCreatesCompletedFacts(t *testing.T) {
	p := fixture(t, "truncated.jsonl")
	info, _ := os.Stat(p)
	b, err := Parse(Candidate{Path: p, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if b.Source.PendingTail == 0 || len(b.Turns) != 0 || len(b.Events) != 0 || len(b.Messages) != 0 || len(b.Tools) != 0 || len(b.Capacity) != 0 || len(b.Compactions) != 0 || len(b.Evidence) != 0 || len(b.Coverage) != 0 {
		t.Fatalf("pending tail committed: %#v", b)
	}
}

func TestDiscoveryHasNoCapAndRejectsInspectorSymlink(t *testing.T) {
	root := t.TempDir()
	inspector := filepath.Join(root, "inspector")
	if err := os.MkdirAll(filepath.Join(root, "codex", "sessions"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(inspector, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 211; i++ {
		p := filepath.Join(root, "codex", "sessions", fmtName(i)+".jsonl")
		if err := os.WriteFile(p, []byte("{}\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(inspector, filepath.Join(root, "codex", "sessions", "self")); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(filepath.Join(root, "codex"), inspector)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 211 {
		t.Fatalf("discovery cap or symlink leak: got %d", len(got))
	}
}
func fmtName(i int) string { return fmt.Sprintf("rollout-%03d", i) }

func TestProjectIdentityPrecedence(t *testing.T) {
	root := t.TempDir()
	clone := filepath.Join(root, "clone")
	nested := filepath.Join(clone, "a", "b")
	if err := os.MkdirAll(filepath.Join(clone, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0700); err != nil {
		t.Fatal(err)
	}
	kind, id := projectIdentity("", nested)
	expectedClone, _ := filepath.EvalSymlinks(clone)
	if kind != "git_root" || id != expectedClone {
		t.Fatalf("git root fallback failed: %s %s", kind, id)
	}
	kind, id = projectIdentity("HTTPS://Example.Invalid/Acme/Repo.git", nested)
	if kind != "git_remote" || id != "https://example.invalid/Acme/Repo" {
		t.Fatalf("remote precedence/normalization failed: %s %s", kind, id)
	}
}

func TestMissingUsageFieldsBecomeCoverageGapNotZeros(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.jsonl")
	data := `{"timestamp":"2026-07-01T00:00:00Z","type":"session_meta","payload":{"session_id":"missing-001","cwd":"/fake","originator":"codex-tui","cli_version":"0.144.1","source":"cli"}}
{"timestamp":"2026-07-01T00:00:01Z","type":"turn_context","payload":{"turn_id":"turn-1","cwd":"/fake","model":"gpt-fake","effort":"low"}}
{"timestamp":"2026-07-01T00:00:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":"turn-1"}}
{"timestamp":"2026-07-01T00:00:03Z","type":"event_msg","payload":{"type":"token_count","info":{"last_token_usage":{"total_tokens":100}}}}
{"timestamp":"2026-07-01T00:00:04Z","type":"event_msg","payload":{"type":"task_complete","turn_id":"turn-1"}}
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	b, err := Parse(Candidate{Path: path, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Coverage) != 1 || b.Coverage[0].Fidelity != "unavailable" || b.Coverage[0].Reason != "missing_usage_fields" {
		t.Fatalf("missing usage became zero/exact: %#v", b.Coverage)
	}
	if b.Turns[0].Usage.Input != nil || b.Turns[0].Usage.Total == nil || *b.Turns[0].Usage.Total != 100 {
		t.Fatalf("missing field synthesized: %#v", b.Turns[0].Usage)
	}
}

func TestAbortedInterruptedAndReconciledTruncatedTerminalStates(t *testing.T) {
	base := func(id, terminal string, truncated bool) (string, Candidate) {
		t.Helper()
		path := filepath.Join(t.TempDir(), id+".jsonl")
		data := fmt.Sprintf(`{"timestamp":"2026-07-01T00:00:00Z","type":"session_meta","payload":{"session_id":%q,"cwd":"/fake","originator":"codex-tui","cli_version":"0.144.1","source":"cli"}}`+"\n"+
			`{"timestamp":"2026-07-01T00:00:01Z","type":"turn_context","payload":{"turn_id":%q,"cwd":"/fake","model":"gpt-fake","effort":"low"}}`+"\n"+
			`{"timestamp":"2026-07-01T00:00:02Z","type":"event_msg","payload":{"type":"task_started","turn_id":%q}}`+"\n", id, id+"-turn", id+"-turn")
		if terminal != "" {
			data += fmt.Sprintf(`{"timestamp":"2026-07-01T00:00:03Z","type":"event_msg","payload":{"type":%q,"turn_id":%q}}`+"\n", terminal, id+"-turn")
		}
		if truncated {
			data += `{"timestamp":"2026-07-01T00:00:04Z","type":"response_item"`
		}
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		info, _ := os.Stat(path)
		return id + "-turn", Candidate{Path: path, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}
	}
	abortedTurn, abortedCandidate := base("aborted", "turn_aborted", false)
	aborted, err := Parse(abortedCandidate, nil)
	if err != nil || len(aborted.Turns) != 1 || aborted.Turns[0].State != "aborted" || aborted.Turns[0].SourceTurnID != abortedTurn {
		t.Fatalf("aborted terminal mismatch: %#v err=%v", aborted.Turns, err)
	}
	for _, tc := range []struct {
		id, want  string
		truncated bool
	}{{"interrupted", "interrupted", false}, {"truncated", "reconciled_truncated", true}} {
		turnID, candidate := base(tc.id, "", tc.truncated)
		proofs := map[string]map[string]TerminalProof{tc.id: {turnID: {ObservedAt: "2026-07-01T00:00:05Z"}}}
		batch, parseErr := ParseContextWithProofs(context.Background(), candidate, nil, proofs)
		if parseErr != nil || len(batch.Turns) != 1 || batch.Turns[0].State != tc.want || batch.Turns[0].TerminalAt != "2026-07-01T00:00:05Z" {
			t.Fatalf("%s terminal mismatch: %#v err=%v", tc.id, batch.Turns, parseErr)
		}
	}
}

func TestAdapterRetainsPrimaryAndSecondaryRateLimitWindows(t *testing.T) {
	data, err := os.ReadFile(fixture(t, "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"resets_at":1782907200}}`), []byte(`"resets_at":1782907200},"secondary":{"used_percent":12,"remaining_percent":88,"window_minutes":10080,"resets_at":1783500000}}`), 1)
	path := filepath.Join(t.TempDir(), "multiple-windows.jsonl")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := Parse(Candidate{Path: path, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Capacity) != 3 {
		t.Fatalf("capacity windows=%d", len(batch.Capacity))
	}
	var weekly facts.Capacity
	for _, point := range batch.Capacity {
		if point.WindowMinutes != nil && *point.WindowMinutes == 10080 {
			weekly = point
		}
	}
	if weekly.UsedPercent == nil || *weekly.UsedPercent != 12 || weekly.RemainingPercent == nil || *weekly.RemainingPercent != 88 || weekly.ResetsAt != "1783500000" {
		t.Fatalf("weekly=%+v", weekly)
	}
}

func TestAdapterAcceptsOnlyStructurallyCompatibleVersionCohorts(t *testing.T) {
	data, err := os.ReadFile(fixture(t, "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"cli_version":"0.144.1"`), []byte(`"cli_version":"0.142.5"`), 1)
	path := filepath.Join(t.TempDir(), "compatible-cohort.jsonl")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := Parse(Candidate{Path: path, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
	if err != nil || batch.Source.State != "supported" || len(batch.Turns) == 0 || len(batch.Evidence) == 0 {
		t.Fatalf("batch=%+v turns=%d evidence=%d err=%v", batch.Source, len(batch.Turns), len(batch.Evidence), err)
	}
}
