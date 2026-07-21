package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/facts"
	"github.com/dylanjbarth/codex-inspector/internal/phase0"
	"github.com/dylanjbarth/codex-inspector/internal/sources"
)

type cohortFixture struct {
	Version               string           `json:"version"`
	SourceKind            string           `json:"sourceKind"`
	File                  string           `json:"file"`
	RecordCount           int              `json:"recordCount"`
	StructuralFingerprint string           `json:"structuralFingerprint"`
	NormalizedSHA256      string           `json:"normalizedSha256"`
	CompletedTurnCount    int              `json:"completedTurnCount"`
	EvidenceCount         int              `json:"evidenceCount"`
	RecordedTokens        int64            `json:"recordedTokens"`
	LineageKind           string           `json:"lineageKind"`
	LineageProof          string           `json:"lineageProof"`
	Parent                *cohortCompanion `json:"parent,omitempty"`
}

type cohortCompanion struct {
	Version               string `json:"version"`
	SourceKind            string `json:"sourceKind"`
	File                  string `json:"file"`
	RecordCount           int    `json:"recordCount"`
	StructuralFingerprint string `json:"structuralFingerprint"`
	NormalizedSHA256      string `json:"normalizedSha256"`
	CompletedTurnCount    int    `json:"completedTurnCount"`
	EvidenceCount         int    `json:"evidenceCount"`
	RecordedTokens        int64  `json:"recordedTokens"`
}

type cohortFixtureManifest struct {
	SchemaVersion string          `json:"schemaVersion"`
	Privacy       string          `json:"privacy"`
	Fixtures      []cohortFixture `json:"fixtures"`
}

type provenCandidate struct {
	candidate                  sources.Candidate
	version, sessionID         string
	parentSession, lineageKind string
}

func writeNormalizedCohorts(candidates []sources.Candidate, directory string) error {
	wanted := map[string]bool{"0.142.5": true, "0.144.0-alpha.4": true, "0.145.0-alpha.18": true}
	sorted := append([]sources.Candidate(nil), candidates...)
	sort.Slice(sorted, func(i, j int) bool {
		left := sha256.Sum256([]byte(sorted[i].Path))
		right := sha256.Sum256([]byte(sorted[j].Path))
		return bytes.Compare(left[:], right[:]) < 0
	})
	options := map[string][]provenCandidate{}
	allBySession := map[string][]provenCandidate{}
	for _, candidate := range sorted {
		if candidate.Kind == "session_index" {
			continue
		}
		if sessionID, version := candidateIdentity(candidate); sessionID != "" {
			allBySession[sessionID] = append(allBySession[sessionID], provenCandidate{candidate: candidate, version: version, sessionID: sessionID, lineageKind: "root"})
		}
		proof := inspectCandidate(candidate)
		if !proof.Recoverable || !proof.AdapterVerified || !proof.IdentityVerified || !proof.CompletedTurns || !proof.UsageFacts || !proof.EvidenceFacts || !proof.EvidenceOffsets || !proof.LineageVerified {
			continue
		}
		info, err := os.Stat(candidate.Path)
		if err != nil {
			return err
		}
		batch, err := sources.Parse(sources.Candidate{Path: candidate.Path, Kind: candidate.Kind, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
		if err != nil || batch.Session == nil || batch.Source.State != "supported" {
			continue
		}
		item := provenCandidate{candidate: candidate, version: proof.Version, sessionID: batch.Session.SourceSessionID, lineageKind: "root"}
		if len(batch.Lineage) > 0 {
			item.lineageKind = batch.Lineage[0].Kind
			item.parentSession = batch.Session.RootWorkUnitID
		}
		if wanted[item.version] {
			options[item.version] = append(options[item.version], item)
		}
	}
	versions := []string{"0.142.5", "0.144.0-alpha.4", "0.145.0-alpha.18"}
	selected := map[string]provenCandidate{}
	selectedParents := map[string]provenCandidate{}
	canonicalizedRelationships := map[string]bool{}
	for _, version := range versions {
		for _, option := range options[version] {
			if option.lineageKind != "spawned" {
				continue
			}
			if parent, exists := usableParent(allBySession[option.parentSession]); exists {
				selected[version] = option
				selectedParents[version] = parent
				break
			}
		}
	}
	for _, version := range versions {
		if _, exists := selected[version]; exists {
			continue
		}
		for _, option := range options[version] {
			if option.lineageKind == "root" {
				selected[version] = option
				break
			}
		}
	}
	var canonicalParent provenCandidate
	for _, version := range versions {
		for _, option := range options[version] {
			if option.lineageKind == "root" {
				canonicalParent = option
				break
			}
		}
		if canonicalParent.sessionID != "" {
			break
		}
	}
	for _, version := range versions {
		if _, exists := selected[version]; exists {
			continue
		}
		for _, option := range options[version] {
			if option.lineageKind == "spawned" && canonicalParent.sessionID != "" {
				selected[version] = option
				selectedParents[version] = canonicalParent
				canonicalizedRelationships[version] = true
				break
			}
		}
	}
	if len(selected) != len(wanted) {
		return fmt.Errorf("complete cohort samples with a persisted parent-child proof unavailable: got %d want %d", len(selected), len(wanted))
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	manifest := cohortFixtureManifest{SchemaVersion: "inspector.normalized-cohort-fixtures/v2", Privacy: "structure preserved; non-discriminator leaf values canonicalized"}
	for _, version := range versions {
		selectedChild := selected[version]
		raw, err := os.ReadFile(selectedChild.candidate.Path)
		if err != nil {
			return err
		}
		ids := map[string]string{}
		var parent *cohortCompanion
		var parentSessionID string
		if selectedChild.parentSession != "" {
			selectedParent := selectedParents[version]
			if canonicalizedRelationships[version] {
				ids[selectedParent.sessionID] = "fixture-id-parent"
				ids[selectedChild.parentSession] = "fixture-id-parent"
			}
			parentRaw, readErr := os.ReadFile(selectedParent.candidate.Path)
			if readErr != nil {
				return readErr
			}
			parentNormalized, parentRecords, parentRawShape, parentNormalizedShape, sanitizeErr := sanitizeRolloutWithIDs(parentRaw, ids)
			if sanitizeErr != nil {
				return fmt.Errorf("sanitize parent for %s: %w", version, sanitizeErr)
			}
			if parentRawShape != parentNormalizedShape {
				return fmt.Errorf("sanitize parent for %s changed structural fingerprint", version)
			}
			parentName := "codex-" + strings.NewReplacer(".", "-", "+", "-", "/", "-").Replace(version) + "-parent.jsonl"
			parentPath := filepath.Join(directory, parentName)
			if writeErr := writeNormalizedFixture(parentPath, parentNormalized); writeErr != nil {
				return writeErr
			}
			parentSummary, parentBatch, summaryErr := summarizeNormalizedFixture(parentPath, parentName, selectedParent, parentNormalized, parentRecords, parentRawShape)
			if summaryErr != nil {
				return summaryErr
			}
			parent = &parentSummary
			parentSessionID = parentBatch.Session.ID
		}
		normalized, recordCount, rawShape, normalizedShape, err := sanitizeRolloutWithIDs(raw, ids)
		if err != nil {
			return fmt.Errorf("sanitize %s: %w", version, err)
		}
		if rawShape != normalizedShape {
			return fmt.Errorf("sanitize %s changed structural fingerprint", version)
		}
		name := "codex-" + strings.NewReplacer(".", "-", "+", "-", "/", "-").Replace(version) + ".jsonl"
		path := filepath.Join(directory, name)
		if err = writeNormalizedFixture(path, normalized); err != nil {
			return err
		}
		summary, batch, err := summarizeNormalizedFixture(path, name, selectedChild, normalized, recordCount, rawShape)
		if err != nil {
			return err
		}
		lineageKind := "root"
		lineageProof := "not_applicable"
		if len(batch.Lineage) > 0 {
			lineageKind = batch.Lineage[0].Kind
			lineageProof = "recorded_parent_child"
			if canonicalizedRelationships[version] {
				lineageProof = "canonicalized_link_between_real_local_sources"
			}
			if parent == nil || batch.Lineage[0].ParentSessionID != parentSessionID {
				return fmt.Errorf("normalized %s parent-child identity diverged", version)
			}
		}
		manifest.Fixtures = append(manifest.Fixtures, cohortFixture{Version: summary.Version, SourceKind: summary.SourceKind, File: summary.File, RecordCount: summary.RecordCount, StructuralFingerprint: summary.StructuralFingerprint, NormalizedSHA256: summary.NormalizedSHA256, CompletedTurnCount: summary.CompletedTurnCount, EvidenceCount: summary.EvidenceCount, RecordedTokens: summary.RecordedTokens, LineageKind: lineageKind, LineageProof: lineageProof, Parent: parent})
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	encoded = append(encoded, '\n')
	path := filepath.Join(directory, "manifest.json")
	if err = os.WriteFile(path, encoded, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o644)
}

func candidateIdentity(candidate sources.Candidate) (sessionID, version string) {
	file, err := os.Open(candidate.Path)
	if err != nil {
		return "", ""
	}
	defer file.Close()
	line, err := bufio.NewReader(file).ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", ""
	}
	var record struct {
		Type    string `json:"type"`
		Payload struct {
			ID         string `json:"id"`
			SessionID  string `json:"session_id"`
			CLIVersion string `json:"cli_version"`
		} `json:"payload"`
	}
	if json.Unmarshal(bytes.TrimSpace(line), &record) != nil || record.Type != "session_meta" {
		return "", ""
	}
	if record.Payload.ID != "" {
		return record.Payload.ID, record.Payload.CLIVersion
	}
	return record.Payload.SessionID, record.Payload.CLIVersion
}

func usableParent(candidates []provenCandidate) (provenCandidate, bool) {
	for _, candidate := range candidates {
		info, err := os.Stat(candidate.candidate.Path)
		if err != nil {
			continue
		}
		batch, err := sources.Parse(sources.Candidate{Path: candidate.candidate.Path, Kind: candidate.candidate.Kind, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
		if err == nil && batch.Source.State == "supported" && batch.Session != nil && len(batch.Turns) > 0 {
			return candidate, true
		}
	}
	return provenCandidate{}, false
}

func writeNormalizedFixture(path string, normalized []byte) error {
	if err := os.WriteFile(path, normalized, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o644)
}

func summarizeNormalizedFixture(path, name string, source provenCandidate, normalized []byte, recordCount int, rawShape string) (cohortCompanion, facts.Batch, error) {
	info, err := os.Stat(path)
	if err != nil {
		return cohortCompanion{}, facts.Batch{}, err
	}
	batch, err := sources.Parse(sources.Candidate{Path: path, Kind: source.candidate.Kind, Size: info.Size(), MTimeNS: info.ModTime().UnixNano()}, nil)
	if err != nil || batch.Source.State != "supported" || batch.Session == nil {
		return cohortCompanion{}, facts.Batch{}, fmt.Errorf("normalized %s fixture failed production adapter: %w", source.version, err)
	}
	decision, err := phase0.ParseRollout(bytes.NewReader(normalized))
	if err != nil || !decision.Supported {
		return cohortCompanion{}, facts.Batch{}, fmt.Errorf("normalized %s fixture failed structural contract: %w", source.version, err)
	}
	golden, err := phase0.CalculateMetrics([]phase0.SourceDecision{decision})
	if err != nil {
		return cohortCompanion{}, facts.Batch{}, err
	}
	completedTurns := 0
	for _, turn := range batch.Turns {
		if turn.State == "completed" {
			completedTurns++
		}
	}
	normalizedHash := sha256.Sum256(normalized)
	summary := cohortCompanion{Version: source.version, SourceKind: source.candidate.Kind, File: name, RecordCount: recordCount, StructuralFingerprint: rawShape, NormalizedSHA256: hex.EncodeToString(normalizedHash[:]), CompletedTurnCount: completedTurns, EvidenceCount: len(batch.Evidence), RecordedTokens: golden.RecordedTokens}
	return summary, batch, nil
}

func sanitizeRollout(raw []byte) ([]byte, int, string, string, error) {
	return sanitizeRolloutWithIDs(raw, map[string]string{})
}

func sanitizeRolloutWithIDs(raw []byte, ids map[string]string) ([]byte, int, string, string, error) {
	reader := bufio.NewScanner(bytes.NewReader(raw))
	reader.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var normalized bytes.Buffer
	rawShapes := make([]any, 0)
	normalizedShapes := make([]any, 0)
	record := 0
	for reader.Scan() {
		line := bytes.TrimSpace(reader.Bytes())
		if len(line) == 0 {
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(line))
		decoder.UseNumber()
		var value any
		if err := decoder.Decode(&value); err != nil {
			return nil, 0, "", "", err
		}
		rawShapes = append(rawShapes, structuralShape(value))
		safe := sanitizeValue(value, "", record, ids)
		normalizedShapes = append(normalizedShapes, structuralShape(safe))
		encoded, err := json.Marshal(safe)
		if err != nil {
			return nil, 0, "", "", err
		}
		normalized.Write(encoded)
		normalized.WriteByte('\n')
		record++
	}
	if err := reader.Err(); err != nil {
		return nil, 0, "", "", err
	}
	if record == 0 {
		return nil, 0, "", "", errors.New("empty source")
	}
	return normalized.Bytes(), record, shapeHash(rawShapes), shapeHash(normalizedShapes), nil
}

func structuralShape(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[key] = structuralShape(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, child := range typed {
			out[index] = structuralShape(child)
		}
		return out
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "boolean"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%T", value)
	}
}

func shapeHash(value any) string {
	encoded, _ := json.Marshal(value)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func sanitizeValue(value any, key string, record int, ids map[string]string) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		keys := make([]string, 0, len(typed))
		for childKey := range typed {
			keys = append(keys, childKey)
		}
		sort.Strings(keys)
		for _, childKey := range keys {
			out[childKey] = sanitizeValue(typed[childKey], childKey, record, ids)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for index, child := range typed {
			out[index] = sanitizeValue(child, key, record, ids)
		}
		return out
	case string:
		return sanitizeString(typed, key, record, ids)
	case json.Number:
		return sanitizeNumber(typed, key, record)
	case bool:
		return false
	default:
		return value
	}
}

func sanitizeString(value, key string, record int, ids map[string]string) string {
	if value == "" {
		return ""
	}
	if key == "timestamp" || key == "completed_at" {
		return time.Date(2026, 7, 1, 0, 0, record, 0, time.UTC).Format(time.RFC3339)
	}
	if key == "cli_version" || key == "type" || key == "role" || key == "phase" || key == "source" || key == "subagent" {
		return value
	}
	if key == "effort" {
		return "medium"
	}
	if key == "status" {
		return "available"
	}
	if key == "mode" {
		return "fixture"
	}
	if key == "cwd" || strings.Contains(key, "path") || key == "worktree" {
		return "/fixture/project"
	}
	if key == "repository_url" || strings.Contains(key, "remote") {
		return "https://example.invalid/fixture.git"
	}
	if key == "branch" {
		return "fixture"
	}
	if key == "model" {
		return "gpt-fixture"
	}
	if key == "originator" {
		return "codex-fixture"
	}
	if key == "name" {
		return "fixture_tool"
	}
	if key == "id" || strings.HasSuffix(key, "_id") {
		if replacement := ids[value]; replacement != "" {
			return replacement
		}
		replacement := "fixture-id-" + strconv.Itoa(len(ids)+1)
		ids[value] = replacement
		return replacement
	}
	return "fixture content"
}

func sanitizeNumber(value json.Number, key string, record int) json.Number {
	multiplier := int64(record + 1)
	switch key {
	case "input_tokens":
		return json.Number(strconv.FormatInt(100*multiplier, 10))
	case "cached_input_tokens":
		return json.Number(strconv.FormatInt(20*multiplier, 10))
	case "output_tokens":
		return json.Number(strconv.FormatInt(30*multiplier, 10))
	case "reasoning_output_tokens":
		return json.Number(strconv.FormatInt(10*multiplier, 10))
	case "total_tokens":
		return json.Number(strconv.FormatInt(130*multiplier, 10))
	case "used_percent":
		return json.Number("25")
	case "remaining_percent":
		return json.Number("75")
	case "resets_at", "completed_at":
		return json.Number(strconv.FormatInt(1780000000+int64(record), 10))
	case "window_minutes":
		return json.Number("300")
	default:
		return json.Number(strconv.Itoa(record + 1))
	}
}
