package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dylanjbarth/codex-inspector/internal/evidence"
	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/indexer"
	"github.com/dylanjbarth/codex-inspector/internal/metrics"
	"github.com/dylanjbarth/codex-inspector/internal/phase0"
	"github.com/dylanjbarth/codex-inspector/internal/sources"
	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

func TestProductionAdapterProofForEveryExactVersionCohort(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	fixture := filepath.Join(filepath.Dir(file), "..", "..", "..", "fixtures", "synthetic", "root.jsonl")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"0.142.5", "0.144.0-alpha.4", "0.144.1", "0.145.0-alpha.18"} {
		t.Run(version, func(t *testing.T) {
			candidate := []byte(strings.Replace(string(data), `"cli_version":"0.144.1"`, `"cli_version":"`+version+`"`, 1))
			path := filepath.Join(t.TempDir(), "candidate.jsonl")
			if err = os.WriteFile(path, candidate, 0o600); err != nil {
				t.Fatal(err)
			}
			result := inspectSource(path)
			if result.Version != version || !result.Recoverable || result.PendingTail || !result.AdapterVerified || !result.IdentityVerified || !result.CompletedTurns || !result.UsageFacts || !result.EvidenceFacts || !result.EvidenceOffsets || !result.LineageVerified {
				t.Fatalf("incomplete production adapter proof: %+v", result)
			}
		})
	}
}

func TestFrozenLocalCompatibilitySummaryAccountsForEverySource(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "fixtures", "local-structural", "stage-01-compatibility-summary.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var summary struct {
		SchemaVersion, Privacy, InventoryFingerprint string
		SourceCount, RolloutCount, LabelSourceCount  int
		RecoverableCount                             int
		Groups                                       []group
	}
	if err = json.Unmarshal(data, &summary); err != nil {
		t.Fatal(err)
	}
	rollouts, recoverable := 0, 0
	kinds := map[string]bool{}
	cohortProof := map[string]bool{}
	for _, item := range summary.Groups {
		if item.Count < 1 || item.Version == "" || item.Reason == "" {
			t.Fatalf("incomplete group: %+v", item)
		}
		rollouts += item.Count
		if item.Recoverable {
			recoverable += item.Count
			if item.AdapterVerified && item.IdentityVerified && item.CompletedTurns && item.UsageFacts && item.EvidenceFacts && item.EvidenceOffsets && item.LineageVerified {
				cohortProof[item.Version] = true
			}
		}
		kinds[item.SourceKind] = true
	}
	if summary.SchemaVersion != "inspector.local-compatibility-summary/v2" || summary.Privacy != "payload-safe aggregate only" || len(summary.InventoryFingerprint) != 64 || summary.SourceCount != summary.RolloutCount+summary.LabelSourceCount || rollouts != summary.RolloutCount || recoverable != summary.RecoverableCount || !kinds["active_rollout"] || !kinds["archived_rollout"] {
		t.Fatalf("summary does not account for its inventory: %+v", summary)
	}
	for _, version := range []string{"0.142.5", "0.144.0-alpha.4", "0.145.0-alpha.18"} {
		if !cohortProof[version] {
			t.Errorf("fixed local inventory lacks complete production-adapter proof for %s", version)
		}
	}
}

func TestNormalizedRealCohortsReproduceAdapterMetricsEvidenceLineageAndAdversarialChecks(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	directory := filepath.Join(filepath.Dir(file), "..", "..", "..", "fixtures", "local-structural", "cohorts")
	manifestBytes, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest cohortFixtureManifest
	if err = json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != "inspector.normalized-cohort-fixtures/v2" || manifest.Privacy != "structure preserved; non-discriminator leaf values canonicalized" || len(manifest.Fixtures) != 3 {
		t.Fatalf("invalid cohort manifest: %+v", manifest)
	}
	wanted := map[string]bool{"0.142.5": true, "0.144.0-alpha.4": true, "0.145.0-alpha.18": true}
	for _, fixture := range manifest.Fixtures {
		t.Run(fixture.Version, func(t *testing.T) {
			if !wanted[fixture.Version] {
				t.Fatalf("unexpected cohort %s", fixture.Version)
			}
			delete(wanted, fixture.Version)
			path := filepath.Join(directory, fixture.File)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				t.Fatal(readErr)
			}
			digest := sha256.Sum256(data)
			if hex.EncodeToString(digest[:]) != fixture.NormalizedSHA256 {
				t.Fatal("normalized fixture hash mismatch")
			}
			if containsTrueBoolean(data) {
				t.Fatal("normalized fixture retained a true private boolean")
			}
			if (fixture.LineageKind == "root" && (fixture.Parent != nil || fixture.LineageProof != "not_applicable")) || (fixture.LineageKind != "root" && (fixture.Parent == nil || (fixture.LineageProof != "recorded_parent_child" && fixture.LineageProof != "canonicalized_link_between_real_local_sources"))) {
				t.Fatalf("invalid lineage fixture contract: %+v", fixture)
			}
			_, records, shape, normalizedShape, sanitizeErr := sanitizeRollout(data)
			if sanitizeErr != nil || records != fixture.RecordCount || shape != fixture.StructuralFingerprint || normalizedShape != shape {
				t.Fatalf("structure proof mismatch records=%d shape=%s normalized=%s err=%v", records, shape, normalizedShape, sanitizeErr)
			}
			info, statErr := os.Stat(path)
			if statErr != nil {
				t.Fatal(statErr)
			}
			batch, parseErr := sources.Parse(sources.Candidate{Path: path, Kind: fixture.SourceKind, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
			if parseErr != nil || batch.Source.State != "supported" || batch.Source.DetectedVersion != fixture.Version || batch.Source.ID == "" || batch.Source.SegmentFingerprint == "" {
				t.Fatalf("production adapter identity proof failed: source=%+v err=%v", batch.Source, parseErr)
			}
			completed := 0
			for _, turn := range batch.Turns {
				if turn.State == "completed" {
					completed++
				}
			}
			lineageKind := "root"
			if len(batch.Lineage) > 0 {
				lineageKind = batch.Lineage[0].Kind
				if batch.Lineage[0].SourceEventID == "" {
					t.Fatal("lineage has no exact source proof event")
				}
			}
			if completed != fixture.CompletedTurnCount || len(batch.Evidence) != fixture.EvidenceCount || lineageKind != fixture.LineageKind {
				t.Fatalf("lifecycle/evidence/lineage proof diverged: completed=%d evidence=%d lineage=%s", completed, len(batch.Evidence), lineageKind)
			}
			decision, decisionErr := phase0.ParseRollout(bytes.NewReader(data))
			golden, metricErr := phase0.CalculateMetrics([]phase0.SourceDecision{decision})
			if decisionErr != nil || !decision.Supported || metricErr != nil || golden.RecordedTokens != fixture.RecordedTokens {
				t.Fatalf("golden metric proof diverged: decision=%+v metrics=%+v errors=%v/%v", decision, golden, decisionErr, metricErr)
			}
			adversarial := malformedEffort(data)
			bad, badErr := phase0.ParseRollout(bytes.NewReader(adversarial))
			if badErr != nil || bad.Supported || bad.Reason != "incompatible_turn_context" {
				t.Fatalf("adversarial cohort accepted: decision=%+v err=%v", bad, badErr)
			}

			badPath := filepath.Join(t.TempDir(), "bad.jsonl")
			if err = os.WriteFile(badPath, adversarial, 0o600); err != nil {
				t.Fatal(err)
			}
			badInfo, statErr := os.Stat(badPath)
			if statErr != nil {
				t.Fatal(statErr)
			}
			badBatch, parseErr := sources.Parse(sources.Candidate{Path: badPath, Kind: fixture.SourceKind, Size: badInfo.Size(), MTimeNS: badInfo.ModTime().UnixNano()}, nil)
			if parseErr != nil || badBatch.Source.State != "unsupported" || badBatch.Source.StateReason != "incompatible_turn_context" || badBatch.Session != nil {
				t.Fatalf("production adapter did not quarantine adversarial cohort: source=%+v session=%+v err=%v", badBatch.Source, badBatch.Session, parseErr)
			}

			root := t.TempDir()
			codex := filepath.Join(root, "codex")
			if err = os.MkdirAll(filepath.Join(codex, "sessions"), 0o700); err != nil {
				t.Fatal(err)
			}
			expectedTokens := fixture.RecordedTokens
			if fixture.Parent != nil {
				parentData, parentErr := verifiedCompanion(directory, *fixture.Parent)
				if parentErr != nil {
					t.Fatal(parentErr)
				}
				if err = os.WriteFile(filepath.Join(codex, "sessions", fixture.Parent.File), parentData, 0o600); err != nil {
					t.Fatal(err)
				}
				expectedTokens += fixture.Parent.RecordedTokens
			}
			if err = os.WriteFile(filepath.Join(codex, "sessions", fixture.File), data, 0o600); err != nil {
				t.Fatal(err)
			}
			inspector := filepath.Join(root, "inspector")
			layout := home.Layout{Root: inspector, Reviews: filepath.Join(inspector, "reviews"), Queue: filepath.Join(inspector, "queue"), Run: filepath.Join(inspector, "run"), Logs: filepath.Join(inspector, "logs"), Cache: filepath.Join(inspector, "cache")}
			if _, err = indexer.Run(context.Background(), indexer.Config{Layout: layout, CodexHome: codex}); err != nil {
				t.Fatal(err)
			}
			store, openErr := storage.Open(filepath.Join(layout.Root, "inspector.db"))
			if openErr != nil {
				t.Fatal(openErr)
			}
			defer store.Close()
			var evidenceID string
			if err = store.DB().QueryRow(`SELECT id FROM evidence_refs ORDER BY id LIMIT 1`).Scan(&evidenceID); err != nil {
				t.Fatal(err)
			}
			chunk, resolveErr := evidence.ResolveAt(context.Background(), store, codex, evidenceID, 0, 0, 4096)
			if resolveErr != nil || chunk.Availability != "available" || chunk.Bytes < 1 {
				t.Fatalf("exact evidence did not resolve: chunk=%+v err=%v", chunk, resolveErr)
			}
			result, queryErr := metrics.New(2).Query(context.Background(), store, metrics.Query{MetricKeys: []string{"recorded_tokens"}, Timezone: "UTC", Grain: "day"})
			if queryErr != nil || len(result.Results) != 1 || result.Results[0].Value != expectedTokens {
				t.Fatalf("indexed metric proof diverged: result=%+v err=%v", result, queryErr)
			}
			if fixture.Parent != nil {
				var edges, spawned int
				if err = store.DB().QueryRow(`SELECT count(*) FROM lineage_edges l JOIN events e ON e.epoch_id=l.epoch_id AND e.id=l.source_event_id WHERE l.edge_kind='spawned' AND e.event_kind='lineage_observation' AND e.commit_revision=l.commit_revision`).Scan(&edges); err != nil {
					t.Fatal(err)
				}
				if err = store.DB().QueryRow(`SELECT count(*) FROM session_versions v WHERE v.purpose='spawned' AND v.lineage_coverage='exact' AND v.root_work_unit_id<>v.session_id AND v.revision=(SELECT max(x.revision) FROM session_versions x WHERE x.epoch_id=v.epoch_id AND x.session_id=v.session_id)`).Scan(&spawned); err != nil {
					t.Fatal(err)
				}
				if edges != 1 || spawned != 1 {
					t.Fatalf("persisted lineage proof diverged: edges=%d spawned=%d", edges, spawned)
				}
			}

			badRoot := t.TempDir()
			badCodex := filepath.Join(badRoot, "codex")
			if err = os.MkdirAll(filepath.Join(badCodex, "sessions"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(badCodex, "sessions", fixture.File), adversarial, 0o600); err != nil {
				t.Fatal(err)
			}
			badInspector := filepath.Join(badRoot, "inspector")
			badLayout := home.Layout{Root: badInspector, Reviews: filepath.Join(badInspector, "reviews"), Queue: filepath.Join(badInspector, "queue"), Run: filepath.Join(badInspector, "run"), Logs: filepath.Join(badInspector, "logs"), Cache: filepath.Join(badInspector, "cache")}
			if _, err = indexer.Run(context.Background(), indexer.Config{Layout: badLayout, CodexHome: badCodex}); err != nil {
				t.Fatal(err)
			}
			badStore, openErr := storage.Open(filepath.Join(badLayout.Root, "inspector.db"))
			if openErr != nil {
				t.Fatal(openErr)
			}
			status, statusErr := badStore.Status()
			_, _, badSessions, sessionsErr := badStore.Sessions(0, "", 10)
			badStore.Close()
			if statusErr != nil || sessionsErr != nil || status.Unsupported != 1 || status.Supported != 0 || len(badSessions) != 0 {
				t.Fatalf("adversarial source escaped index quarantine: status=%+v sessions=%d errors=%v/%v", status, len(badSessions), statusErr, sessionsErr)
			}
		})
	}
	if len(wanted) != 0 {
		t.Fatalf("missing normalized cohorts: %v", wanted)
	}
}

func verifiedCompanion(directory string, fixture cohortCompanion) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(directory, fixture.File))
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != fixture.NormalizedSHA256 {
		return nil, errors.New("normalized parent fixture hash mismatch")
	}
	if containsTrueBoolean(data) {
		return nil, errors.New("normalized parent fixture retained a true private boolean")
	}
	_, records, shape, normalizedShape, err := sanitizeRollout(data)
	if err != nil {
		return nil, err
	}
	if records != fixture.RecordCount || shape != fixture.StructuralFingerprint || normalizedShape != shape {
		return nil, fmt.Errorf("parent structure proof mismatch records=%d shape=%s normalized=%s", records, shape, normalizedShape)
	}
	return data, nil
}

func containsTrueBoolean(data []byte) bool {
	contains := false
	var visit func(any)
	visit = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for _, child := range typed {
				visit(child)
			}
		case []any:
			for _, child := range typed {
				visit(child)
			}
		case bool:
			contains = contains || typed
		}
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var value any
		if json.Unmarshal(scanner.Bytes(), &value) == nil {
			visit(value)
		}
	}
	return contains
}

func TestCohortSanitizerIsDeterministicAndCanonicalizesPrivateLeaves(t *testing.T) {
	raw := []byte(`{"type":"event_msg","payload":{"type":"token_count","private_flag":true,"window_minutes":10080,"effort":"xhigh","status":"connected","mode":"private","secret":"do-not-retain","id":"private-id"}}` + "\n")
	var expected []byte
	for iteration := 0; iteration < 50; iteration++ {
		actual, records, before, after, err := sanitizeRollout(raw)
		if err != nil || records != 1 || before != after {
			t.Fatalf("sanitize iteration %d records=%d shape=%s/%s err=%v", iteration, records, before, after, err)
		}
		if iteration == 0 {
			expected = actual
		} else if !bytes.Equal(actual, expected) {
			t.Fatalf("sanitize iteration %d was not byte deterministic", iteration)
		}
	}
	var record struct {
		Type    string `json:"type"`
		Payload struct {
			Type          string      `json:"type"`
			PrivateFlag   bool        `json:"private_flag"`
			WindowMinutes json.Number `json:"window_minutes"`
			Effort        string      `json:"effort"`
			Status        string      `json:"status"`
			Mode          string      `json:"mode"`
			Secret        string      `json:"secret"`
			ID            string      `json:"id"`
		} `json:"payload"`
	}
	decoder := json.NewDecoder(bytes.NewReader(expected))
	decoder.UseNumber()
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record.Type != "event_msg" || record.Payload.Type != "token_count" || record.Payload.PrivateFlag || record.Payload.WindowMinutes.String() != "300" || record.Payload.Effort != "medium" || record.Payload.Status != "available" || record.Payload.Mode != "fixture" || record.Payload.Secret != "fixture content" || record.Payload.ID != "fixture-id-1" {
		t.Fatalf("privacy allowlist diverged: %+v", record)
	}
	if bytes.Contains(expected, []byte("do-not-retain")) || bytes.Contains(expected, []byte("10080")) || bytes.Contains(expected, []byte("private-id")) {
		t.Fatalf("private input survived sanitizer: %s", expected)
	}
}

func TestCohortGeneratorIsByteIdempotent(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	directory := filepath.Join(filepath.Dir(file), "..", "..", "..", "fixtures", "local-structural", "cohorts")
	manifestBytes, err := os.ReadFile(filepath.Join(directory, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest cohortFixtureManifest
	if err = json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	var candidates []sources.Candidate
	add := func(name, kind string) {
		path := filepath.Join(directory, name)
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatal(statErr)
		}
		candidates = append(candidates, sources.Candidate{Path: path, Kind: kind, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()})
	}
	for _, fixture := range manifest.Fixtures {
		add(fixture.File, fixture.SourceKind)
		if fixture.Parent != nil {
			add(fixture.Parent.File, fixture.Parent.SourceKind)
		}
	}
	first, second := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")
	if err = writeNormalizedCohorts(candidates, first); err != nil {
		t.Fatal(err)
	}
	if err = writeNormalizedCohorts(candidates, second); err != nil {
		t.Fatal(err)
	}
	firstEntries, err := os.ReadDir(first)
	if err != nil {
		t.Fatal(err)
	}
	secondEntries, err := os.ReadDir(second)
	if err != nil {
		t.Fatal(err)
	}
	if len(firstEntries) != len(secondEntries) {
		t.Fatalf("generated file counts differ: %d/%d", len(firstEntries), len(secondEntries))
	}
	for index := range firstEntries {
		if firstEntries[index].Name() != secondEntries[index].Name() {
			t.Fatalf("generated names differ: %s/%s", firstEntries[index].Name(), secondEntries[index].Name())
		}
		left, leftErr := os.ReadFile(filepath.Join(first, firstEntries[index].Name()))
		right, rightErr := os.ReadFile(filepath.Join(second, secondEntries[index].Name()))
		if leftErr != nil || rightErr != nil || !bytes.Equal(left, right) {
			t.Fatalf("generated %s was not byte idempotent: %v/%v", firstEntries[index].Name(), leftErr, rightErr)
		}
	}
}

func malformedEffort(data []byte) []byte {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var out bytes.Buffer
	mutated := false
	for scanner.Scan() {
		var record map[string]any
		_ = json.Unmarshal(scanner.Bytes(), &record)
		if !mutated && record["type"] == "turn_context" {
			if payload, ok := record["payload"].(map[string]any); ok {
				payload["effort"] = float64(7)
				mutated = true
			}
		}
		encoded, _ := json.Marshal(record)
		out.Write(encoded)
		out.WriteByte('\n')
	}
	return out.Bytes()
}
