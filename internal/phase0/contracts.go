package phase0

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	SupportedCodexVersion = "0.144.1"
	AdapterVersion        = "rollout-jsonl/codex-cli-0.144.1/v1"
	FormulaVersion        = 1
)

type SourceDecision struct {
	Supported       bool
	Reason          string
	SessionID       string
	ParentSessionID string
	ProjectRemote   string
	PendingTail     bool
	Turns           []Turn
	ToolRequests    int
	ToolResults     int
	Compactions     int
	Capacity        []CapacityObservation
}

type Turn struct {
	ID              string
	Completed       bool
	CompletedAt     string
	Model           string
	ReasoningEffort string
	Usage           *Usage
	Cumulative      *Usage
	Contribution    string
}

type Usage struct {
	Input     int64 `json:"input_tokens"`
	Cached    int64 `json:"cached_input_tokens"`
	Output    int64 `json:"output_tokens"`
	Reasoning int64 `json:"reasoning_output_tokens"`
	Total     int64 `json:"total_tokens"`
}

type CapacityObservation struct {
	ObservedAt   string
	LimitID      string
	UsedPercent  float64
	WindowMinute int64
	ResetsAt     int64
}

// CorpusInput is a named, synthetic rollout used to freeze normalization.
// Name is a fixture identity, never a source path.
type CorpusInput struct {
	Name string
	Data []byte
}

type NormalizedCorpus struct {
	AdapterVersion string                 `json:"adapterVersion"`
	Sources        []NormalizedSource     `json:"sources"`
	Sessions       []NormalizedSession    `json:"sessions"`
	Turns          []NormalizedTurn       `json:"turns"`
	Events         []NormalizedEvent      `json:"events"`
	Messages       []NormalizedMessage    `json:"messages"`
	ToolPhases     []NormalizedToolPhase  `json:"toolPhases"`
	Compactions    []NormalizedCompaction `json:"compactions"`
	Capacity       []NormalizedCapacity   `json:"capacity"`
	Coverage       []NormalizedCoverage   `json:"coverage"`
	Evidence       []NormalizedEvidence   `json:"evidence"`
}

type NormalizedSource struct {
	Name                string `json:"name"`
	SourceID            string `json:"sourceId"`
	IdentityFingerprint string `json:"identityFingerprint"`
	CurrentFingerprint  string `json:"currentFingerprint"`
	SegmentID           string `json:"segmentId,omitempty"`
	SegmentFingerprint  string `json:"segmentFingerprint,omitempty"`
	SessionID           string `json:"sessionId,omitempty"`
	Decision            string `json:"decision"`
	PendingTail         bool   `json:"pendingTail"`
	CompleteBytes       int64  `json:"completeBytes"`
	AdapterVersion      string `json:"adapterVersion"`
}

type NormalizedSession struct {
	SessionID       string `json:"sessionId"`
	RootWorkUnitID  string `json:"rootWorkUnitId"`
	ParentSessionID string `json:"parentSessionId,omitempty"`
	Purpose         string `json:"purpose"`
	ProjectIdentity string `json:"projectIdentity"`
	SourceID        string `json:"sourceId"`
	SegmentID       string `json:"segmentId"`
	AdapterVersion  string `json:"adapterVersion"`
}

type NormalizedTurn struct {
	TurnID            string `json:"turnId"`
	SessionID         string `json:"sessionId"`
	State             string `json:"state"`
	CompletedAt       string `json:"completedAt,omitempty"`
	Model             string `json:"model"`
	ReasoningEffort   string `json:"reasoningEffort"`
	ContributionKind  string `json:"contributionKind"`
	NormalizationKind string `json:"normalizationKind,omitempty"`
	Usage             *Usage `json:"usage,omitempty"`
	AdapterVersion    string `json:"adapterVersion"`
}

type NormalizedEvent struct {
	EventID        string `json:"eventId"`
	SourceID       string `json:"sourceId"`
	SegmentID      string `json:"segmentId"`
	RecordOrdinal  int    `json:"recordOrdinal"`
	ByteStart      int64  `json:"byteStart"`
	ByteEnd        int64  `json:"byteEnd"`
	RecordType     string `json:"recordType"`
	EventKind      string `json:"eventKind"`
	ObservedAt     string `json:"observedAt"`
	TurnID         string `json:"turnId,omitempty"`
	ContentSHA256  string `json:"contentSha256"`
	AdapterVersion string `json:"adapterVersion"`
}

type NormalizedMessage struct {
	EventID       string `json:"eventId"`
	MessageID     string `json:"messageId,omitempty"`
	TurnID        string `json:"turnId"`
	Role          string `json:"role"`
	Phase         string `json:"phase,omitempty"`
	ContentLength int    `json:"contentLength"`
	ContentSHA256 string `json:"contentSha256"`
	RecordOrdinal int    `json:"recordOrdinal"`
	ByteStart     int64  `json:"byteStart"`
	ByteEnd       int64  `json:"byteEnd"`
}

type NormalizedToolPhase struct {
	CallID        string `json:"callId"`
	SessionID     string `json:"sessionId"`
	TurnID        string `json:"turnId"`
	SemanticPhase string `json:"semanticPhase"`
	ToolName      string `json:"toolName,omitempty"`
	EventID       string `json:"eventId"`
}

type NormalizedCompaction struct {
	EventID          string `json:"eventId"`
	TriggerEventID   string `json:"triggerEventId"`
	FirstWindowID    string `json:"firstWindowId,omitempty"`
	PreviousWindowID string `json:"previousWindowId,omitempty"`
	WindowID         string `json:"windowId,omitempty"`
	WindowNumber     int    `json:"windowNumber,omitempty"`
	ContentLength    int    `json:"contentLength"`
	ContentSHA256    string `json:"contentSha256"`
}

type NormalizedCapacity struct {
	EventID       string  `json:"eventId"`
	ObservedAt    string  `json:"observedAt"`
	LimitID       string  `json:"limitId"`
	WindowMinutes int64   `json:"windowMinutes"`
	UsedPercent   float64 `json:"usedPercent"`
	ResetsAt      int64   `json:"resetsAt"`
}

type NormalizedCoverage struct {
	ScopeKind string `json:"scopeKind"`
	ScopeID   string `json:"scopeId"`
	Field     string `json:"field"`
	Fidelity  string `json:"fidelity"`
	Observed  int    `json:"observed"`
	Eligible  int    `json:"eligible"`
	Reason    string `json:"reason,omitempty"`
}

type NormalizedEvidence struct {
	EvidenceID        string `json:"evidenceId"`
	EventID           string `json:"eventId"`
	SourceID          string `json:"sourceId"`
	SourceFingerprint string `json:"sourceFingerprint"`
	EventFingerprint  string `json:"eventFingerprint"`
	RecordOrdinal     int    `json:"recordOrdinal"`
	ByteStart         int64  `json:"byteStart"`
	ByteEnd           int64  `json:"byteEnd"`
	Availability      string `json:"availability"`
	AdapterVersion    string `json:"adapterVersion"`
}

type Metrics struct {
	FormulaVersion         int              `json:"formulaVersion"`
	RecordedTokens         int64            `json:"recordedTokens"`
	ByKind                 map[string]int64 `json:"recordedTokensByKind"`
	Composition            map[string]int64 `json:"tokenComposition"`
	TopRoots               []RootMetric     `json:"topRootSessions"`
	LatestCapacity         *CapacityMetric  `json:"latestCapacityObservation"`
	RecordedTokensOverTime []TimeBucket     `json:"recordedTokensOverTime"`
	CapacityDrawdown       []CapacityMetric `json:"capacityDrawdown"`
	Coverage               MetricCoverage   `json:"coverage"`
}
type MetricCoverage struct {
	EligibleTurns   int            `json:"eligibleTurns"`
	ObservedTurns   int            `json:"observedTurns"`
	ExcludedReasons map[string]int `json:"excludedReasons"`
}
type MetricFilter struct{ Start, End, Project, Model, Reasoning, ContributionKind string }

func CalculateMetricsFiltered(sources []SourceDecision, filter MetricFilter) (Metrics, error) {
	filtered := make([]SourceDecision, len(sources))
	copy(filtered, sources)
	for i := range filtered {
		normalized, err := normalizeTurns(sources[i].Turns)
		if err != nil {
			return Metrics{}, err
		}
		filtered[i].Turns = nil
		kind := "user_root_direct"
		if filtered[i].ParentSessionID != "" {
			kind = "descendant"
		}
		for _, turn := range normalized {
			if filter.Project != "" && sources[i].ProjectRemote != filter.Project || filter.Model != "" && turn.Model != filter.Model || filter.Reasoning != "" && turn.ReasoningEffort != filter.Reasoning || filter.ContributionKind != "" && kind != filter.ContributionKind {
				continue
			}
			if filter.Start != "" && turn.CompletedAt < filter.Start || filter.End != "" && turn.CompletedAt >= filter.End {
				continue
			}
			filtered[i].Turns = append(filtered[i].Turns, turn)
		}
		if filter.Start != "" || filter.End != "" {
			filtered[i].Capacity = nil
			for _, point := range sources[i].Capacity {
				if (filter.Start == "" || point.ObservedAt >= filter.Start) && (filter.End == "" || point.ObservedAt < filter.End) {
					filtered[i].Capacity = append(filtered[i].Capacity, point)
				}
			}
		}
	}
	result, err := CalculateMetrics(filtered)
	if err != nil {
		return Metrics{}, err
	}
	all, err := CalculateMetrics(sources)
	if err != nil {
		return Metrics{}, err
	}
	result.LatestCapacity = all.LatestCapacity
	return result, nil
}

type TimeBucket struct {
	Start  string           `json:"start"`
	ByKind map[string]int64 `json:"byKind"`
}

type RootMetric struct {
	RootSessionID    string `json:"rootSessionId"`
	InclusiveTokens  int64  `json:"inclusiveTokens"`
	DirectTokens     int64  `json:"directTokens"`
	DescendantTokens int64  `json:"descendantTokens"`
}

type CapacityMetric struct {
	ObservedAt   string  `json:"observedAt,omitempty"`
	LimitID      string  `json:"limitId"`
	UsedPercent  float64 `json:"usedPercent"`
	WindowMinute int64   `json:"windowMinutes"`
	ResetsAt     int64   `json:"resetsAt"`
}

func CalendarBucket(timestamp, timezone, grain string) (string, error) {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return "", err
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", err
	}
	local := t.In(location)
	var start time.Time
	switch grain {
	case "hour":
		start = time.Date(local.Year(), local.Month(), local.Day(), local.Hour(), 0, 0, 0, location)
	case "day":
		start = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	case "week":
		offset := (int(local.Weekday()) + 6) % 7
		start = time.Date(local.Year(), local.Month(), local.Day()-offset, 0, 0, 0, 0, location)
	case "month":
		start = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, location)
	default:
		return "", errors.New("unsupported grain")
	}
	return start.Format(time.RFC3339), nil
}

type HookMarker struct {
	SchemaVersion   string  `json:"schemaVersion"`
	ProtocolVersion int     `json:"protocolVersion"`
	EventKind       string  `json:"eventKind"`
	SessionID       string  `json:"sessionId"`
	TurnID          string  `json:"turnId,omitempty"`
	SubagentID      string  `json:"subagentId,omitempty"`
	TranscriptPath  *string `json:"transcriptPath,omitempty"`
	ObservedAt      string  `json:"observedAt"`
}

func MapHookPayload(data []byte, observedAt time.Time) (HookMarker, string) {
	var input struct {
		SessionID      string  `json:"session_id"`
		TurnID         string  `json:"turn_id"`
		TranscriptPath *string `json:"transcript_path"`
		Event          string  `json:"hook_event_name"`
		AgentID        string  `json:"agent_id"`
	}
	if json.Unmarshal(data, &input) != nil || input.SessionID == "" {
		return HookMarker{}, "invalid_hook_payload"
	}
	kinds := map[string]string{"SessionStart": "session_start", "UserPromptSubmit": "user_prompt_submit", "PreCompact": "pre_compact", "PostCompact": "post_compact", "SubagentStart": "subagent_start", "SubagentStop": "subagent_stop", "Stop": "turn_stop"}
	kind, ok := kinds[input.Event]
	if !ok {
		return HookMarker{}, "unsupported_hook_event"
	}
	if (input.Event == "SubagentStart" || input.Event == "SubagentStop") && input.AgentID == "" {
		return HookMarker{}, "invalid_hook_payload"
	}
	return HookMarker{SchemaVersion: "inspector.hook-marker/v1", ProtocolVersion: 1, EventKind: kind, SessionID: input.SessionID, TurnID: input.TurnID, SubagentID: input.AgentID, TranscriptPath: input.TranscriptPath, ObservedAt: observedAt.UTC().Format(time.RFC3339Nano)}, "supported"
}

type record struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type metadataPayload struct {
	ID             string          `json:"id"`
	SessionID      string          `json:"session_id"`
	ParentThreadID string          `json:"parent_thread_id"`
	CLIVersion     string          `json:"cli_version"`
	CWD            string          `json:"cwd"`
	Originator     string          `json:"originator"`
	Source         json.RawMessage `json:"source"`
	Timestamp      string          `json:"timestamp"`
	Git            struct {
		RepositoryURL string `json:"repository_url"`
	} `json:"git"`
}

type turnContextPayload struct {
	TurnID string `json:"turn_id"`
	Model  string `json:"model"`
	Effort string `json:"effort"`
	CWD    string `json:"cwd"`
}

type eventPayload struct {
	Type        string          `json:"type"`
	TurnID      string          `json:"turn_id"`
	CompletedAt json.RawMessage `json:"completed_at"`
	Info        *struct {
		Last  *Usage `json:"last_token_usage"`
		Total *Usage `json:"total_token_usage"`
	} `json:"info"`
	RateLimits *struct {
		LimitID string `json:"limit_id"`
		Primary *struct {
			UsedPercent  float64 `json:"used_percent"`
			WindowMinute int64   `json:"window_minutes"`
			ResetsAt     int64   `json:"resets_at"`
		} `json:"primary"`
	} `json:"rate_limits"`
}

type responsePayload struct {
	Type   string `json:"type"`
	CallID string `json:"call_id"`
}

func ParseFixture(path string) (SourceDecision, error) {
	f, err := os.Open(path)
	if err != nil {
		return SourceDecision{}, err
	}
	defer f.Close()
	return ParseRollout(f)
}

func ParseRollout(r io.Reader) (SourceDecision, error) {
	reader := bufio.NewReader(r)
	var records []record
	pendingTail := false
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			var rec record
			if err := json.Unmarshal(line, &rec); err != nil {
				if errors.Is(readErr, io.EOF) && !bytes.HasSuffix(line, []byte{'\n'}) {
					pendingTail = true
				} else {
					return SourceDecision{}, fmt.Errorf("invalid complete JSONL record: %w", err)
				}
			} else {
				records = append(records, rec)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return SourceDecision{}, readErr
		}
	}

	if len(records) == 0 || records[0].Type != "session_meta" {
		return SourceDecision{Reason: "missing_leading_session_meta", PendingTail: pendingTail}, nil
	}
	var meta metadataPayload
	if err := json.Unmarshal(records[0].Payload, &meta); err != nil {
		return SourceDecision{Reason: "invalid_session_meta", PendingTail: pendingTail}, nil
	}
	if meta.SessionID == "" {
		meta.SessionID = meta.ID
	}
	if meta.CLIVersion != SupportedCodexVersion {
		return SourceDecision{Reason: "unsupported_codex_version", SessionID: meta.SessionID, PendingTail: pendingTail}, nil
	}
	if meta.SessionID == "" || meta.CWD == "" || meta.Originator == "" || (meta.Timestamp != "" && !isRFC3339(meta.Timestamp)) || !validSessionSource(meta.Source) {
		return SourceDecision{Reason: "missing_required_session_identity", SessionID: meta.SessionID, PendingTail: pendingTail}, nil
	}
	if reason := validateSupportedRecords(records); reason != "" {
		return SourceDecision{Reason: reason, SessionID: meta.SessionID, PendingTail: pendingTail}, nil
	}

	decision := SourceDecision{
		Supported:       true,
		Reason:          "supported",
		SessionID:       meta.SessionID,
		ParentSessionID: meta.ParentThreadID,
		ProjectRemote:   meta.Git.RepositoryURL,
		PendingTail:     pendingTail,
	}
	turns := map[string]*Turn{}
	toolRequests := map[string]bool{}
	toolResults := map[string]bool{}
	var turnOrder []string
	getTurn := func(id string) *Turn {
		if turns[id] == nil {
			turns[id] = &Turn{ID: id}
			turnOrder = append(turnOrder, id)
		}
		return turns[id]
	}
	var currentTurn string
	for _, rec := range records[1:] {
		switch rec.Type {
		case "compacted":
			decision.Compactions++
		case "turn_context":
			var payload turnContextPayload
			if json.Unmarshal(rec.Payload, &payload) != nil || payload.TurnID == "" {
				continue
			}
			currentTurn = payload.TurnID
			turn := getTurn(payload.TurnID)
			turn.Model = payload.Model
			turn.ReasoningEffort = payload.Effort
		case "event_msg":
			var payload eventPayload
			if json.Unmarshal(rec.Payload, &payload) != nil {
				continue
			}
			switch payload.Type {
			case "task_started":
				if payload.TurnID != "" {
					currentTurn = payload.TurnID
					getTurn(payload.TurnID)
				}
			case "token_count":
				if currentTurn != "" && payload.Info != nil {
					turn := getTurn(currentTurn)
					turn.Usage = payload.Info.Last
					turn.Cumulative = payload.Info.Total
				}
				if payload.RateLimits != nil && payload.RateLimits.Primary != nil {
					decision.Capacity = append(decision.Capacity, CapacityObservation{
						ObservedAt: rec.Timestamp, LimitID: payload.RateLimits.LimitID,
						UsedPercent:  payload.RateLimits.Primary.UsedPercent,
						WindowMinute: payload.RateLimits.Primary.WindowMinute,
						ResetsAt:     payload.RateLimits.Primary.ResetsAt,
					})
				}
			case "task_complete":
				if payload.TurnID != "" {
					turn := getTurn(payload.TurnID)
					turn.Completed = true
					turn.CompletedAt = rec.Timestamp
				}
			}
		case "response_item":
			var payload responsePayload
			if json.Unmarshal(rec.Payload, &payload) != nil {
				continue
			}
			switch payload.Type {
			case "custom_tool_call", "function_call":
				toolRequests[payload.CallID] = true
			case "custom_tool_call_output", "function_call_output":
				toolResults[payload.CallID] = true
			}
		}
	}
	decision.ToolRequests = len(toolRequests)
	decision.ToolResults = len(toolResults)

	for _, id := range turnOrder {
		decision.Turns = append(decision.Turns, *turns[id])
	}
	return decision, nil
}

// NormalizeCorpus freezes the complete metadata-only fact projection used by
// Phase 0. It intentionally retains hashes and byte locators, never payloads.
func NormalizeCorpus(inputs []CorpusInput) (NormalizedCorpus, error) {
	corpus := NormalizedCorpus{AdapterVersion: AdapterVersion}
	sequence := 0
	for _, input := range inputs {
		decision, err := ParseRollout(bytes.NewReader(input.Data))
		if err != nil {
			return NormalizedCorpus{}, fmt.Errorf("%s: %w", input.Name, err)
		}
		completeRecords, completeBytes := completeJSONLRecords(input.Data)
		identityMaterial := []byte(decision.SessionID)
		if len(completeRecords) > 0 && completeRecords[0].Record.Type == "session_meta" {
			identityMaterial = append(identityMaterial, ':')
			identityMaterial = append(identityMaterial, completeRecords[0].Content...)
		}
		identityFingerprint := sha256Hex(identityMaterial)
		sourceID := "source:" + sha256Hex([]byte(decision.SessionID+":"+identityFingerprint))
		currentFingerprint := CheckpointFingerprint(input.Data, completeBytes)
		segmentFingerprint := identityFingerprint
		segmentID := ""
		if decision.Supported {
			segmentID = "segment:" + segmentFingerprint
		}
		corpus.Sources = append(corpus.Sources, NormalizedSource{
			Name: input.Name, SourceID: sourceID, IdentityFingerprint: identityFingerprint, CurrentFingerprint: currentFingerprint,
			SegmentID: segmentID, SegmentFingerprint: emptyUnless(decision.Supported, segmentFingerprint),
			SessionID: decision.SessionID, Decision: decision.Reason, PendingTail: decision.PendingTail,
			CompleteBytes: completeBytes, AdapterVersion: AdapterVersion,
		})
		if !decision.Supported || completedTurnCount(decision.Turns) == 0 {
			continue
		}
		rootID, purpose, contribution := decision.SessionID, "user", "user_root_direct"
		if decision.ParentSessionID != "" {
			rootID, purpose, contribution = decision.ParentSessionID, "spawned", "descendant"
		}
		corpus.Sessions = append(corpus.Sessions, NormalizedSession{
			SessionID: decision.SessionID, RootWorkUnitID: rootID, ParentSessionID: decision.ParentSessionID,
			Purpose: purpose, ProjectIdentity: decision.ProjectRemote, SourceID: sourceID,
			SegmentID: segmentID, AdapterVersion: AdapterVersion,
		})
		normalizedTurns, err := normalizeTurns(decision.Turns)
		if err != nil {
			return NormalizedCorpus{}, err
		}
		for i, turn := range normalizedTurns {
			if !turn.Completed {
				continue
			}
			kind := "last_turn"
			if decision.Turns[i].Usage == nil && turn.Usage != nil {
				kind = "cumulative_delta"
			}
			corpus.Turns = append(corpus.Turns, NormalizedTurn{
				TurnID: turn.ID, SessionID: decision.SessionID, State: "completed", CompletedAt: turn.CompletedAt,
				Model: turn.Model, ReasoningEffort: turn.ReasoningEffort, ContributionKind: contribution,
				NormalizationKind: kind, Usage: turn.Usage, AdapterVersion: AdapterVersion,
			})
			coverage := NormalizedCoverage{ScopeKind: "turn", ScopeID: turn.ID, Field: "usage", Eligible: 1}
			if turn.Usage == nil {
				coverage.Fidelity, coverage.Reason = "unavailable", "usage_unavailable"
			} else {
				coverage.Fidelity, coverage.Observed = "exact", 1
			}
			corpus.Coverage = append(corpus.Coverage, coverage)
		}

		currentTurn, compactionTrigger := "", ""
		toolSeen := map[string]bool{}
		for ordinal, located := range completeRecords {
			rec := located.Record
			sequence++
			eventKind, turnID := rec.Type, currentTurn
			var eventPayloadValue eventPayload
			var response map[string]json.RawMessage
			switch rec.Type {
			case "turn_context":
				var context turnContextPayload
				_ = json.Unmarshal(rec.Payload, &context)
				currentTurn, turnID = context.TurnID, context.TurnID
			case "event_msg":
				_ = json.Unmarshal(rec.Payload, &eventPayloadValue)
				eventKind = eventPayloadValue.Type
				if eventPayloadValue.TurnID != "" {
					currentTurn, turnID = eventPayloadValue.TurnID, eventPayloadValue.TurnID
				}
			case "response_item":
				_ = json.Unmarshal(rec.Payload, &response)
				_ = json.Unmarshal(response["type"], &eventKind)
				var metadata struct {
					TurnID string `json:"turn_id"`
				}
				_ = json.Unmarshal(response["internal_chat_message_metadata_passthrough"], &metadata)
				if metadata.TurnID != "" {
					turnID = metadata.TurnID
				}
			}
			eventFingerprint := sha256Hex(located.Content)
			eventID := "event:" + sha256Hex([]byte(fmt.Sprintf("%s:%d:%s", segmentFingerprint, ordinal, eventKind)))
			corpus.Events = append(corpus.Events, NormalizedEvent{
				EventID: eventID, SourceID: sourceID, SegmentID: segmentID, RecordOrdinal: ordinal,
				ByteStart: located.Start, ByteEnd: located.End, RecordType: rec.Type, EventKind: eventKind,
				ObservedAt: rec.Timestamp, TurnID: turnID, ContentSHA256: eventFingerprint, AdapterVersion: AdapterVersion,
			})
			evidenceID := "evidence:" + sha256Hex([]byte(eventID+":"+eventFingerprint))
			corpus.Evidence = append(corpus.Evidence, NormalizedEvidence{
				EvidenceID: evidenceID, EventID: eventID, SourceID: sourceID,
				SourceFingerprint: CheckpointFingerprint(input.Data, located.End), EventFingerprint: eventFingerprint,
				RecordOrdinal: ordinal, ByteStart: located.Start, ByteEnd: located.End,
				Availability: "available", AdapterVersion: AdapterVersion,
			})
			if rec.Type == "event_msg" && eventPayloadValue.Type == "context_compacted" {
				compactionTrigger = eventID
			}
			if rec.Type == "event_msg" && eventPayloadValue.Type == "token_count" && eventPayloadValue.RateLimits != nil && eventPayloadValue.RateLimits.Primary != nil {
				corpus.Capacity = append(corpus.Capacity, NormalizedCapacity{
					EventID: eventID, ObservedAt: rec.Timestamp, LimitID: eventPayloadValue.RateLimits.LimitID,
					WindowMinutes: eventPayloadValue.RateLimits.Primary.WindowMinute,
					UsedPercent:   eventPayloadValue.RateLimits.Primary.UsedPercent,
					ResetsAt:      eventPayloadValue.RateLimits.Primary.ResetsAt,
				})
			}
			if rec.Type == "response_item" && eventKind == "message" {
				var messageID, role, phase string
				_ = json.Unmarshal(response["id"], &messageID)
				_ = json.Unmarshal(response["role"], &role)
				_ = json.Unmarshal(response["phase"], &phase)
				content := responseText(response["content"])
				corpus.Messages = append(corpus.Messages, NormalizedMessage{
					EventID: eventID, MessageID: messageID, TurnID: turnID, Role: role, Phase: phase,
					ContentLength: len([]byte(content)), ContentSHA256: sha256Hex([]byte(content)),
					RecordOrdinal: ordinal, ByteStart: located.Start, ByteEnd: located.End,
				})
			}
			if rec.Type == "response_item" && (eventKind == "custom_tool_call" || eventKind == "custom_tool_call_output" || eventKind == "function_call" || eventKind == "function_call_output") {
				var callID, toolName string
				_ = json.Unmarshal(response["call_id"], &callID)
				_ = json.Unmarshal(response["name"], &toolName)
				phase := "request"
				if strings.HasSuffix(eventKind, "_output") {
					phase = "result"
				}
				key := callID + ":" + phase
				if !toolSeen[key] {
					toolSeen[key] = true
					corpus.ToolPhases = append(corpus.ToolPhases, NormalizedToolPhase{CallID: callID, SessionID: decision.SessionID, TurnID: turnID, SemanticPhase: phase, ToolName: toolName, EventID: eventID})
				}
			}
			if rec.Type == "compacted" {
				var compacted struct {
					Message          string `json:"message"`
					FirstWindowID    string `json:"first_window_id"`
					PreviousWindowID string `json:"previous_window_id"`
					WindowID         string `json:"window_id"`
					WindowNumber     int    `json:"window_number"`
				}
				_ = json.Unmarshal(rec.Payload, &compacted)
				corpus.Compactions = append(corpus.Compactions, NormalizedCompaction{
					EventID: eventID, TriggerEventID: compactionTrigger, FirstWindowID: compacted.FirstWindowID,
					PreviousWindowID: compacted.PreviousWindowID, WindowID: compacted.WindowID,
					WindowNumber: compacted.WindowNumber, ContentLength: len([]byte(compacted.Message)),
					ContentSHA256: sha256Hex([]byte(compacted.Message)),
				})
			}
		}
	}
	_ = sequence // iteration order is represented by source order and record ordinal.
	return corpus, nil
}

type locatedRecord struct {
	Record  record
	Content []byte
	Start   int64
	End     int64
}

func completeJSONLRecords(data []byte) ([]locatedRecord, int64) {
	var result []locatedRecord
	var offset int64
	for len(data) > 0 {
		lineEnd := bytes.IndexByte(data, '\n')
		consumed := len(data)
		line := data
		if lineEnd >= 0 {
			line, consumed = data[:lineEnd], lineEnd+1
		}
		var rec record
		if json.Unmarshal(line, &rec) != nil {
			break
		}
		result = append(result, locatedRecord{Record: rec, Content: append([]byte(nil), line...), Start: offset, End: offset + int64(len(line))})
		offset += int64(consumed)
		data = data[consumed:]
	}
	return result, offset
}

func responseText(raw json.RawMessage) string {
	var parts []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var builder strings.Builder
	for _, part := range parts {
		builder.WriteString(part.Text)
	}
	return builder.String()
}

func sha256Hex(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

// CheckpointFingerprint hashes an already-validated complete prefix. Appends
// preserve prior checkpoint fingerprints; rewriting any indexed byte changes
// the fingerprint and requires rebuild.
func CheckpointFingerprint(data []byte, completeBytes int64) string {
	if completeBytes < 0 || completeBytes > int64(len(data)) {
		return ""
	}
	return sha256Hex(data[:completeBytes])
}
func emptyUnless(condition bool, value string) string {
	if condition {
		return value
	}
	return ""
}
func completedTurnCount(turns []Turn) int {
	count := 0
	for _, turn := range turns {
		if turn.Completed {
			count++
		}
	}
	return count
}

func validateSupportedRecords(records []record) string {
	outer := map[string]bool{"session_meta": true, "turn_context": true, "event_msg": true, "response_item": true, "compacted": true, "world_state": true, "inter_agent_communication_metadata": true}
	events := map[string]bool{"agent_message": true, "context_compacted": true, "entered_review_mode": true, "exited_review_mode": true, "image_generation_end": true, "mcp_tool_call_end": true, "patch_apply_end": true, "sub_agent_activity": true, "task_complete": true, "task_started": true, "thread_rolled_back": true, "thread_settings_applied": true, "token_count": true, "turn_aborted": true, "user_message": true, "web_search_end": true}
	responses := map[string]bool{"agent_message": true, "custom_tool_call": true, "custom_tool_call_output": true, "function_call": true, "function_call_output": true, "message": true, "reasoning": true}
	turns := map[string]bool{}
	for _, rec := range records {
		if rec.Type != "turn_context" {
			continue
		}
		var p turnContextPayload
		var raw map[string]json.RawMessage
		if json.Unmarshal(rec.Payload, &p) != nil || json.Unmarshal(rec.Payload, &raw) != nil || p.TurnID == "" || p.Model == "" || p.Effort == "" || p.CWD == "" || raw["cwd"] == nil || turns[p.TurnID] {
			return "incompatible_turn_context"
		}
		turns[p.TurnID] = true
	}
	for i, rec := range records {
		if _, err := time.Parse(time.RFC3339, rec.Timestamp); err != nil || !outer[rec.Type] || (rec.Type == "session_meta" && i != 0) {
			return "incompatible_record_envelope"
		}
		switch rec.Type {
		case "turn_context":
			continue
		case "event_msg":
			var p eventPayload
			if json.Unmarshal(rec.Payload, &p) != nil || !events[p.Type] {
				return "incompatible_event_record"
			}
			if (p.Type == "task_started" || p.Type == "task_complete") && (p.TurnID == "" || !turns[p.TurnID]) {
				return "incompatible_turn_identity"
			}
			if p.Type == "task_complete" && len(p.CompletedAt) != 0 && !validJSONNumber(p.CompletedAt) {
				return "incompatible_event_record"
			}
			if p.Type == "token_count" && p.Info == nil {
				return "incompatible_token_record"
			}
			if p.Type == "token_count" && ((p.Info.Last != nil && !validUsage(*p.Info.Last)) || (p.Info.Total != nil && !validUsage(*p.Info.Total))) {
				return "incompatible_token_record"
			}
		case "response_item":
			var p responsePayload
			if json.Unmarshal(rec.Payload, &p) != nil || !responses[p.Type] {
				return "incompatible_response_record"
			}
			if (p.Type == "custom_tool_call" || p.Type == "custom_tool_call_output" || p.Type == "function_call" || p.Type == "function_call_output") && p.CallID == "" {
				return "incompatible_tool_identity"
			}
		case "compacted", "world_state", "inter_agent_communication_metadata":
			var p map[string]json.RawMessage
			if json.Unmarshal(rec.Payload, &p) != nil {
				return "incompatible_record_shape"
			}
		}
	}
	return ""
}

func validJSONNumber(raw json.RawMessage) bool {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return false
	}
	var number json.Number
	if err := json.Unmarshal(raw, &number); err != nil {
		return false
	}
	_, err := strconv.ParseFloat(string(number), 64)
	return err == nil
}

func isRFC3339(value string) bool {
	_, err := time.Parse(time.RFC3339, value)
	return err == nil
}

func validUsage(usage Usage) bool {
	return usage.Input >= 0 && usage.Cached >= 0 && usage.Output >= 0 && usage.Reasoning >= 0 && usage.Total >= 0
}

func validSessionSource(raw json.RawMessage) bool {
	var sourceName string
	if err := json.Unmarshal(raw, &sourceName); err == nil {
		return sourceName != ""
	}
	var sourceObject map[string]json.RawMessage
	if err := json.Unmarshal(raw, &sourceObject); err != nil || len(sourceObject) != 1 {
		return false
	}
	subagent, ok := sourceObject["subagent"]
	if !ok {
		return false
	}
	var kind string
	if err := json.Unmarshal(subagent, &kind); err == nil {
		return kind == "other"
	}
	var spawn map[string]json.RawMessage
	if err := json.Unmarshal(subagent, &spawn); err != nil || len(spawn) != 1 {
		return false
	}
	if other, ok := spawn["other"]; ok {
		var label string
		return json.Unmarshal(other, &label) == nil && label != ""
	}
	threadSpawn, ok := spawn["thread_spawn"]
	if !ok {
		return false
	}
	var threadSpawnFields map[string]json.RawMessage
	return json.Unmarshal(threadSpawn, &threadSpawnFields) == nil && threadSpawnFields["parent_thread_id"] != nil
}

func ensureTurn(turns map[string]*Turn, id string) *Turn {
	if turns[id] == nil {
		turns[id] = &Turn{ID: id}
	}
	return turns[id]
}

func CalculateMetrics(sources []SourceDecision) (Metrics, error) {
	metrics := Metrics{
		FormulaVersion: FormulaVersion,
		ByKind:         map[string]int64{"user_root_direct": 0, "descendant": 0, "inspector_review": 0, "other_orphan": 0},
		Composition:    map[string]int64{"uncached_input": 0, "cached_input": 0, "visible_output": 0, "reasoning_output": 0, "residual": 0},
		Coverage:       MetricCoverage{ExcludedReasons: map[string]int{}},
	}
	rootTotals := map[string]*RootMetric{}
	buckets := map[string]map[string]int64{}
	latestCapacityObservedAt := ""
	for _, source := range sources {
		if !source.Supported {
			continue
		}
		rootID := source.SessionID
		kind := "user_root_direct"
		if source.ParentSessionID != "" {
			rootID = source.ParentSessionID
			kind = "descendant"
		}
		if rootTotals[rootID] == nil {
			rootTotals[rootID] = &RootMetric{RootSessionID: rootID}
		}
		turns, err := normalizeTurns(source.Turns)
		if err != nil {
			return Metrics{}, fmt.Errorf("session %s: %w", source.SessionID, err)
		}
		for _, turn := range turns {
			if !turn.Completed {
				continue
			}
			metrics.Coverage.EligibleTurns++
			usage := turn.Usage
			if usage == nil {
				metrics.Coverage.ExcludedReasons["usage_unavailable"]++
				continue
			}
			metrics.Coverage.ObservedTurns++
			if err := addUsage(&metrics, *usage, kind, rootTotals[rootID]); err != nil {
				return Metrics{}, fmt.Errorf("turn %s: %w", turn.ID, err)
			}
			bucket, err := CalendarBucket(turn.CompletedAt, "UTC", "day")
			if err != nil {
				return Metrics{}, err
			}
			if buckets[bucket] == nil {
				buckets[bucket] = map[string]int64{}
			}
			buckets[bucket][kind] += usage.Total
		}
		for _, capacity := range source.Capacity {
			point := CapacityMetric{ObservedAt: capacity.ObservedAt, LimitID: capacity.LimitID, UsedPercent: capacity.UsedPercent, WindowMinute: capacity.WindowMinute, ResetsAt: capacity.ResetsAt}
			metrics.CapacityDrawdown = append(metrics.CapacityDrawdown, point)
			if metrics.LatestCapacity == nil || capacity.ObservedAt > latestCapacityObservedAt {
				latest := point
				latest.ObservedAt = ""
				metrics.LatestCapacity = &latest
				latestCapacityObservedAt = capacity.ObservedAt
			}
		}
	}
	for start, kinds := range buckets {
		metrics.RecordedTokensOverTime = append(metrics.RecordedTokensOverTime, TimeBucket{Start: start, ByKind: kinds})
	}
	sort.Slice(metrics.RecordedTokensOverTime, func(i, j int) bool {
		return metrics.RecordedTokensOverTime[i].Start < metrics.RecordedTokensOverTime[j].Start
	})
	sort.Slice(metrics.CapacityDrawdown, func(i, j int) bool {
		return metrics.CapacityDrawdown[i].ObservedAt < metrics.CapacityDrawdown[j].ObservedAt
	})
	for _, root := range rootTotals {
		root.InclusiveTokens = root.DirectTokens + root.DescendantTokens
		metrics.TopRoots = append(metrics.TopRoots, *root)
	}
	sort.Slice(metrics.TopRoots, func(i, j int) bool {
		if metrics.TopRoots[i].InclusiveTokens == metrics.TopRoots[j].InclusiveTokens {
			return metrics.TopRoots[i].RootSessionID < metrics.TopRoots[j].RootSessionID
		}
		return metrics.TopRoots[i].InclusiveTokens > metrics.TopRoots[j].InclusiveTokens
	})
	return metrics, nil
}

func normalizeTurns(turns []Turn) ([]Turn, error) {
	normalized := append([]Turn(nil), turns...)
	var previous *Usage
	for i := range normalized {
		if normalized[i].Usage == nil && normalized[i].Cumulative != nil && previous != nil {
			delta, err := subtractUsage(*normalized[i].Cumulative, *previous)
			if err != nil {
				return nil, fmt.Errorf("turn %s: %w", normalized[i].ID, err)
			}
			normalized[i].Usage = &delta
		}
		if normalized[i].Cumulative != nil {
			copy := *normalized[i].Cumulative
			previous = &copy
		}
	}
	return normalized, nil
}

func subtractUsage(current, prior Usage) (Usage, error) {
	delta := Usage{Input: current.Input - prior.Input, Cached: current.Cached - prior.Cached, Output: current.Output - prior.Output, Reasoning: current.Reasoning - prior.Reasoning, Total: current.Total - prior.Total}
	if delta.Input < 0 || delta.Cached < 0 || delta.Output < 0 || delta.Reasoning < 0 || delta.Total < 0 {
		return Usage{}, errors.New("negative cumulative token delta")
	}
	return delta, nil
}

func addUsage(metrics *Metrics, usage Usage, kind string, root *RootMetric) error {
	if usage.Cached > usage.Input || usage.Reasoning > usage.Output {
		return errors.New("inclusive token components are inconsistent")
	}
	uncached := usage.Input - usage.Cached
	visible := usage.Output - usage.Reasoning
	residual := usage.Total - uncached - usage.Cached - visible - usage.Reasoning
	if residual < 0 {
		return errors.New("negative token residual")
	}
	metrics.RecordedTokens += usage.Total
	metrics.ByKind[kind] += usage.Total
	metrics.Composition["uncached_input"] += uncached
	metrics.Composition["cached_input"] += usage.Cached
	metrics.Composition["visible_output"] += visible
	metrics.Composition["reasoning_output"] += usage.Reasoning
	metrics.Composition["residual"] += residual
	if kind == "descendant" {
		root.DescendantTokens += usage.Total
	} else {
		root.DirectTokens += usage.Total
	}
	return nil
}

func ParseThreadStarted(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		var event struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return "", err
		}
		if event.Type == "thread.started" {
			if event.ThreadID == "" {
				return "", errors.New("thread.started missing thread_id")
			}
			return event.ThreadID, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("thread.started not found")
}
