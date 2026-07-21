package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/dylanjbarth/codex-inspector/internal/home"
	"github.com/dylanjbarth/codex-inspector/internal/phase0"
	"github.com/dylanjbarth/codex-inspector/internal/sources"
)

const maxCompatibilitySourceBytes = 256 * 1024 * 1024

type inspection struct {
	Version          string `json:"version"`
	Reason           string `json:"reason"`
	Recoverable      bool   `json:"recoverable"`
	PendingTail      bool   `json:"pendingTail"`
	AdapterVerified  bool   `json:"adapterVerified"`
	IdentityVerified bool   `json:"identityVerified"`
	CompletedTurns   bool   `json:"completedTurns"`
	UsageFacts       bool   `json:"usageFacts"`
	EvidenceFacts    bool   `json:"evidenceFacts"`
	EvidenceOffsets  bool   `json:"evidenceOffsetsSafe"`
	LineageVerified  bool   `json:"lineageVerified"`
}

type group struct {
	SourceKind       string `json:"sourceKind"`
	Version          string `json:"version"`
	Reason           string `json:"reason"`
	Recoverable      bool   `json:"recoverable"`
	PendingTail      bool   `json:"pendingTail"`
	AdapterVerified  bool   `json:"adapterVerified"`
	IdentityVerified bool   `json:"identityVerified"`
	CompletedTurns   bool   `json:"completedTurns"`
	UsageFacts       bool   `json:"usageFacts"`
	EvidenceFacts    bool   `json:"evidenceFacts"`
	EvidenceOffsets  bool   `json:"evidenceOffsetsSafe"`
	LineageVerified  bool   `json:"lineageVerified"`
	Count            int    `json:"count"`
}

func inspectSource(path string) inspection {
	info, err := os.Stat(path)
	if err != nil {
		return inspection{Version: "unknown", Reason: "source_unreadable"}
	}
	return inspectCandidate(sources.Candidate{Path: path, Kind: "active_rollout", Size: info.Size(), MTimeNS: info.ModTime().UnixNano()})
}

func inspectCandidate(candidate sources.Candidate) inspection {
	file, err := os.Open(candidate.Path)
	if err != nil {
		return inspection{Version: "unknown", Reason: "source_unreadable"}
	}
	defer file.Close()
	if candidate.Size > maxCompatibilitySourceBytes {
		return inspection{Version: "unknown", Reason: "source_too_large"}
	}

	reader := bufio.NewReader(file)
	first, readErr := reader.ReadBytes('\n')
	if len(first) == 0 {
		return inspection{Version: "unknown", Reason: "missing_leading_session_meta"}
	}
	hadNewline := first[len(first)-1] == '\n'
	first = bytes.TrimSuffix(first, []byte{'\n'})
	var envelope map[string]json.RawMessage
	if json.Unmarshal(first, &envelope) != nil {
		return inspection{Version: "unknown", Reason: "invalid_leading_session_meta", PendingTail: errors.Is(readErr, io.EOF) && !hadNewline}
	}
	var recordType string
	if json.Unmarshal(envelope["type"], &recordType) != nil || recordType != "session_meta" {
		return inspection{Version: "unknown", Reason: "missing_leading_session_meta"}
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(envelope["payload"], &payload) != nil {
		return inspection{Version: "unknown", Reason: "invalid_session_meta"}
	}
	var version string
	if json.Unmarshal(payload["cli_version"], &version) != nil || version == "" {
		return inspection{Version: "missing", Reason: "missing_codex_version"}
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return inspection{Version: version, Reason: "source_unreadable"}
	}
	decision, parseErr := phase0.ParseRollout(file)
	if parseErr != nil {
		return inspection{Version: version, Reason: "invalid_complete_record"}
	}
	if !decision.Supported {
		return inspection{Version: version, Reason: decision.Reason, PendingTail: decision.PendingTail}
	}
	reason := "structurally_compatible_candidate"
	if version == phase0.SupportedCodexVersion {
		reason = "supported_current"
	}
	batch, adapterErr := sources.ParseContextWithProofs(context.Background(), candidate, nil, nil)
	if adapterErr != nil || batch.Source.State != "supported" {
		return inspection{Version: version, Reason: "production_adapter_failed", PendingTail: decision.PendingTail}
	}
	proof := inspection{Version: version, Reason: reason, Recoverable: true, PendingTail: decision.PendingTail, AdapterVerified: true, LineageVerified: true}
	proof.IdentityVerified = batch.Source.ID != "" && batch.Source.SessionID != "" && batch.Source.SegmentFingerprint != ""
	for _, turn := range batch.Turns {
		if turn.State == "completed" {
			proof.CompletedTurns = true
		}
		if turn.Usage != nil {
			proof.UsageFacts = true
		}
	}
	proof.EvidenceFacts = len(batch.Evidence) > 0
	proof.EvidenceOffsets = proof.EvidenceFacts
	for _, event := range batch.Events {
		if event.ByteStart < 0 || event.ByteEnd <= event.ByteStart || event.RecordOrdinal < 0 {
			proof.EvidenceOffsets = false
		}
	}
	return proof
}

func main() {
	codexHomeFlag := flag.String("codex-home", "", "Codex home to inspect")
	fixtureDirectory := flag.String("write-normalized-cohorts", "", "write payload-safe normalized cohort fixtures")
	flag.Parse()
	layout, err := home.Resolve()
	if err != nil {
		fatal("inspector_home_unavailable")
	}
	codexHome := *codexHomeFlag
	if codexHome == "" {
		codexHome, err = sources.CodexHome()
		if err != nil {
			fatal("source_home_unavailable")
		}
	}
	candidates, err := sources.Discover(codexHome, layout.Root)
	if err != nil {
		fatal("source_inventory_failed")
	}
	if *fixtureDirectory != "" {
		if err = writeNormalizedCohorts(candidates, *fixtureDirectory); err != nil {
			fatal("cohort_fixture_generation_failed")
		}
	}

	grouped := map[string]*group{}
	inventoryKeys := make([]string, 0, len(candidates))
	recoverable := 0
	rollouts := 0
	labelSources := 0
	for _, candidate := range candidates {
		pathHash := sha256.Sum256([]byte(candidate.Path))
		inventoryKeys = append(inventoryKeys, candidate.Kind+"\x00"+hex.EncodeToString(pathHash[:])+"\x00"+strconv.FormatInt(candidate.Size, 10)+"\x00"+strconv.FormatInt(candidate.MTimeNS, 10))
		if candidate.Kind == "session_index" {
			labelSources++
			continue
		}
		rollouts++
		result := inspectCandidate(candidate)
		if result.Recoverable {
			recoverable++
		}
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%t\x00%t\x00%t\x00%t\x00%t\x00%t\x00%t\x00%t\x00%t", candidate.Kind, result.Version, result.Reason, result.Recoverable, result.PendingTail, result.AdapterVerified, result.IdentityVerified, result.CompletedTurns, result.UsageFacts, result.EvidenceFacts, result.EvidenceOffsets, result.LineageVerified)
		if grouped[key] == nil {
			grouped[key] = &group{SourceKind: candidate.Kind, Version: result.Version, Reason: result.Reason, Recoverable: result.Recoverable, PendingTail: result.PendingTail, AdapterVerified: result.AdapterVerified, IdentityVerified: result.IdentityVerified, CompletedTurns: result.CompletedTurns, UsageFacts: result.UsageFacts, EvidenceFacts: result.EvidenceFacts, EvidenceOffsets: result.EvidenceOffsets, LineageVerified: result.LineageVerified}
		}
		grouped[key].Count++
	}
	sort.Strings(inventoryKeys)
	inventoryHash := sha256.Sum256([]byte(strings.Join(inventoryKeys, "\n")))
	groups := make([]group, 0, len(grouped))
	for _, value := range grouped {
		groups = append(groups, *value)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Version != groups[j].Version {
			return groups[i].Version < groups[j].Version
		}
		if groups[i].SourceKind != groups[j].SourceKind {
			return groups[i].SourceKind < groups[j].SourceKind
		}
		if groups[i].Reason != groups[j].Reason {
			return groups[i].Reason < groups[j].Reason
		}
		return !groups[i].PendingTail && groups[j].PendingTail
	})
	output := map[string]any{
		"inventoryFingerprint": hex.EncodeToString(inventoryHash[:]),
		"sourceCount":          len(candidates),
		"rolloutCount":         rollouts,
		"labelSourceCount":     labelSources,
		"recoverableCount":     recoverable,
		"groups":               groups,
	}
	if err = json.NewEncoder(os.Stdout).Encode(output); err != nil {
		fatal("report_write_failed")
	}
}

func fatal(code string) {
	fmt.Fprintln(os.Stderr, code)
	os.Exit(1)
}
