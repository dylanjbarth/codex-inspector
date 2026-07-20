package sources

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/facts"
	"github.com/dylanjbarth/codex-inspector/internal/phase0"
)

const (
	SupportedCodexVersion = "0.144.1"
	AdapterVersion        = "rollout-jsonl/codex-exact-cohorts/v2"
)

type wireRecord struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}
type located struct {
	Record           wireRecord
	Raw              []byte
	Start, End, Next int64
	Ordinal          int
}
type metaWire struct {
	ID                   string          `json:"id"`
	SessionID            string          `json:"session_id"`
	ParentThreadID       string          `json:"parent_thread_id"`
	ContinuationThreadID string          `json:"continued_from_thread_id"`
	CLIVersion           string          `json:"cli_version"`
	CWD                  string          `json:"cwd"`
	Originator           string          `json:"originator"`
	Source               json.RawMessage `json:"source"`
	Timestamp            string          `json:"timestamp"`
	Git                  struct {
		RepositoryURL string `json:"repository_url"`
		Branch        string `json:"branch"`
		Worktree      string `json:"worktree"`
	} `json:"git"`
}
type usageWire struct {
	Input     *int64 `json:"input_tokens"`
	Cached    *int64 `json:"cached_input_tokens"`
	Output    *int64 `json:"output_tokens"`
	Reasoning *int64 `json:"reasoning_output_tokens"`
	Total     *int64 `json:"total_tokens"`
}
type eventWire struct {
	Type        string          `json:"type"`
	TurnID      string          `json:"turn_id"`
	CompletedAt json.RawMessage `json:"completed_at"`
	Info        *struct {
		Last  *usageWire `json:"last_token_usage"`
		Total *usageWire `json:"total_token_usage"`
	} `json:"info"`
	RateLimits *struct {
		LimitID string `json:"limit_id"`
		Primary *struct {
			Used      *float64        `json:"used_percent"`
			Remaining *float64        `json:"remaining_percent"`
			Window    *int64          `json:"window_minutes"`
			Resets    json.RawMessage `json:"resets_at"`
		} `json:"primary"`
		Secondary *struct {
			Used      *float64        `json:"used_percent"`
			Remaining *float64        `json:"remaining_percent"`
			Window    *int64          `json:"window_minutes"`
			Resets    json.RawMessage `json:"resets_at"`
		} `json:"secondary"`
	} `json:"rate_limits"`
}
type turnWire struct {
	TurnID string `json:"turn_id"`
	Model  string `json:"model"`
	Effort string `json:"effort"`
	CWD    string `json:"cwd"`
}

func Parse(candidate Candidate, labels map[string]string) (facts.Batch, error) {
	return ParseContext(context.Background(), candidate, labels)
}

func ParseContext(ctx context.Context, candidate Candidate, labels map[string]string) (facts.Batch, error) {
	return ParseContextWithProofs(ctx, candidate, labels, nil)
}

type TerminalProof struct {
	ObservedAt string
}

func ParseContextWithProofs(ctx context.Context, candidate Candidate, labels map[string]string, proofs map[string]map[string]TerminalProof) (facts.Batch, error) {
	f, err := os.Open(candidate.Path)
	if err != nil {
		return facts.Batch{}, err
	}
	defer f.Close()
	records, completeOffset, pending, prefix, err := readRecords(ctx, f)
	if err != nil {
		return facts.Batch{}, err
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return facts.Batch{}, err
	}
	decision, err := phase0.ParseRollout(&contextReader{ctx: ctx, r: f})
	if err != nil {
		return facts.Batch{}, err
	}
	// Discovery and parsing are intentionally separate. Active rollout files may
	// grow between those operations, so persist the size and timestamp observed
	// from the same open file descriptor that supplied this parsed snapshot.
	if info, statErr := f.Stat(); statErr == nil {
		candidate.Size = info.Size()
		candidate.MTimeNS = info.ModTime().UnixNano()
	}
	b := facts.Batch{Source: facts.Source{Kind: candidate.Kind, Path: candidate.Path, Size: candidate.Size, MTimeNS: candidate.MTimeNS, CompleteOffset: completeOffset, CompleteOrdinal: len(records), PendingTail: pending, PrefixSHA256: prefix, AdapterVersion: AdapterVersion, State: "unsupported"}}
	b.Source.ID = "source:" + hash([]byte(candidate.Kind+":"+prefix))
	b.Source.SegmentFingerprint = prefix
	if len(records) == 0 || records[0].Record.Type != "session_meta" {
		b.Source.StateReason = "missing_leading_session_meta"
		return b, nil
	}
	var meta metaWire
	if json.Unmarshal(records[0].Record.Payload, &meta) != nil {
		b.Source.StateReason = "invalid_session_meta"
		return b, nil
	}
	if meta.SessionID == "" {
		meta.SessionID = meta.ID
	}
	b.Source.SessionID, b.Source.DetectedVersion = meta.SessionID, meta.CLIVersion
	identityFingerprint := hash(append(append([]byte(meta.SessionID), ':'), records[0].Raw...))
	b.Source.SegmentFingerprint = identityFingerprint
	b.Source.ID = "source:" + hash([]byte(meta.SessionID+":"+identityFingerprint))
	b.Source.SegmentID = "segment:" + identityFingerprint
	if !decision.Supported {
		b.Source.StateReason = decision.Reason
		return b, nil
	}
	b.Source.State = "supported"
	b.Source.StateReason = ""
	projectKind, projectIdentity := projectIdentity(meta.Git.RepositoryURL, meta.CWD)
	projectID := "project:" + hash([]byte(projectKind+":"+projectIdentity))
	b.Project = &facts.Project{ID: projectID, Kind: projectKind, Identity: projectIdentity, DisplayName: filepath.Base(strings.TrimSuffix(projectIdentity, ".git")), Aliases: map[string]string{"cwd": meta.CWD}}
	if projectKind == "git_root" {
		b.Project.Aliases["git_root"] = projectIdentity
	}
	if meta.Git.RepositoryURL != "" {
		b.Project.Aliases["remote"] = meta.Git.RepositoryURL
	}
	if meta.Git.Branch != "" {
		b.Project.Aliases["branch"] = meta.Git.Branch
	}
	if meta.Git.Worktree != "" {
		b.Project.Aliases["worktree"] = meta.Git.Worktree
	}
	root, purpose, coverage := meta.SessionID, "user", "exact"
	lineageKind := ""
	if strings.Contains(strings.ToLower(meta.Originator), "codex-inspector") {
		purpose = "inspector_review"
	}
	if meta.ParentThreadID != "" {
		if sourceIsSubagent(meta.Source) {
			root, purpose, lineageKind = meta.ParentThreadID, "spawned", "spawned"
		} else {
			root, lineageKind = meta.SessionID, "forked_from"
		}
	}
	if meta.ContinuationThreadID != "" && meta.ParentThreadID == "" {
		root, lineageKind = meta.ContinuationThreadID, "continued_as"
	}
	sessionID := "session:" + hash([]byte(meta.SessionID))
	b.Session = &facts.Session{ID: sessionID, SourceSessionID: meta.SessionID, ProjectID: projectID, RootWorkUnitID: root, Purpose: purpose, Originator: meta.Originator, Source: string(meta.Source), Title: labels[meta.SessionID], StartedAt: records[0].Record.Timestamp, LineageCoverage: coverage}
	segmentOrderKey, err := phase0.SourceOrderKey(records[0].Record.Timestamp, identityFingerprint)
	if err != nil {
		b.Source.StateReason = "invalid_session_meta_timestamp"
		return b, nil
	}
	b.Segment = &facts.Segment{ID: b.Source.SegmentID, SessionID: sessionID, SourceID: b.Source.ID, Fingerprint: identityFingerprint, StartedAt: records[0].Record.Timestamp, SourceOrderKey: segmentOrderKey, Ordinal: 0}
	if lineageKind != "" {
		parentID := meta.ParentThreadID
		if parentID == "" {
			parentID = meta.ContinuationThreadID
		}
		// Some resumed rollout segments repeat their own logical session ID in
		// parent_thread_id. That is segment continuity, not a self-lineage edge.
		if parentID != meta.SessionID {
			b.Lineage = append(b.Lineage, facts.Lineage{ParentSessionID: "session:" + hash([]byte(parentID)), ChildSessionID: sessionID, Kind: lineageKind})
			b.Coverage = append(b.Coverage, facts.Coverage{ScopeKind: "session", ScopeID: sessionID, FieldKey: "lineage_reference", Fidelity: "exact", Observed: 1, Eligible: 1, Reason: lineageKind + ":" + parentID})
		}
	}
	b = normalize(b, records, proofs[meta.SessionID])
	if len(b.Turns) == 0 {
		b.Project, b.Session, b.Segment = nil, nil, nil
		b.Lineage, b.Events, b.Messages, b.Tools = nil, nil, nil, nil
		b.Capacity, b.Compactions, b.Evidence, b.Coverage = nil, nil, nil, nil
	}
	return b, nil
}

func readRecords(ctx context.Context, r io.Reader) ([]located, int64, int, string, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	var rows []located
	var offset int64
	pending := 0
	h := sha256.New()
	lastYield := time.Now()
	for {
		if err := ctx.Err(); err != nil {
			return nil, 0, 0, "", err
		}
		line, err := br.ReadBytes('\n')
		if len(line) > 8<<20 {
			return nil, 0, 0, "", errors.New("record exceeds 8 MiB supported input bound")
		}
		if len(line) > 0 {
			raw := bytes.TrimSuffix(line, []byte{'\n'})
			raw = bytes.TrimSuffix(raw, []byte{'\r'})
			var rec wireRecord
			if json.Unmarshal(raw, &rec) != nil {
				if errors.Is(err, io.EOF) {
					pending = len(line)
					break
				}
				return nil, 0, 0, "", errors.New("invalid complete JSONL record")
			}
			if _, e := time.Parse(time.RFC3339, rec.Timestamp); e != nil {
				return nil, 0, 0, "", errors.New("invalid record timestamp")
			}
			end := offset + int64(len(raw))
			next := offset + int64(len(line))
			h.Write(line)
			rows = append(rows, located{rec, append([]byte(nil), raw...), offset, end, next, len(rows)})
			offset = next
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, 0, 0, "", err
		}
		if time.Since(lastYield) >= 100*time.Millisecond {
			runtime.Gosched()
			lastYield = time.Now()
		}
	}
	return rows, offset, pending, hex.EncodeToString(h.Sum(nil)), nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	select {
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	default:
		return r.r.Read(p)
	}
}

func normalize(b facts.Batch, rows []located, proofs map[string]TerminalProof) facts.Batch {
	type turnState struct {
		turn        facts.Turn
		last, total *facts.Usage
	}
	turns := map[string]*turnState{}
	order := []string{}
	current := ""
	ensure := func(id string) *turnState {
		if turns[id] == nil {
			turns[id] = &turnState{turn: facts.Turn{ID: "turn:" + hash([]byte(b.Session.SourceSessionID+":"+id)), SessionID: b.Session.ID, SourceTurnID: id, State: "provisional", Ordinal: len(order)}}
			order = append(order, id)
		}
		return turns[id]
	}
	for _, row := range rows[1:] {
		switch row.Record.Type {
		case "turn_context":
			var w turnWire
			_ = json.Unmarshal(row.Record.Payload, &w)
			current = w.TurnID
			t := ensure(current)
			t.turn.Ordinal = row.Ordinal
			t.turn.Model = w.Model
			t.turn.ReasoningEffort = w.Effort
		case "event_msg":
			var w eventWire
			_ = json.Unmarshal(row.Record.Payload, &w)
			if w.TurnID != "" {
				current = w.TurnID
			}
			if current != "" {
				t := ensure(current)
				if w.Type == "task_started" {
					t.turn.StartedAt = row.Record.Timestamp
				}
				if w.Type == "task_complete" {
					t.turn.State = "completed"
					t.turn.TerminalAt = row.Record.Timestamp
					t.turn.CompletedAt = row.Record.Timestamp
					b.Session.EndedAt = row.Record.Timestamp
				}
				if w.Type == "turn_aborted" {
					t.turn.State = "aborted"
					t.turn.TerminalAt = row.Record.Timestamp
				}
				if w.Type == "token_count" && w.Info != nil {
					t.last = toUsage(w.Info.Last)
					t.total = toUsage(w.Info.Total)
				}
			}
		}
	}
	for turnID, proof := range proofs {
		if turn := turns[turnID]; turn != nil && turn.turn.State == "provisional" {
			turn.turn.State = "interrupted"
			if b.Source.PendingTail > 0 {
				turn.turn.State = "reconciled_truncated"
			}
			turn.turn.TerminalAt = proof.ObservedAt
		}
	}
	var cumulative *facts.Usage
	for _, id := range order {
		t := turns[id]
		t.turn.SourceOrderKey, _ = phase0.TurnOrderKey(b.Segment.SourceOrderKey, t.turn.Ordinal)
		if t.last != nil {
			t.turn.Usage = t.last
			t.turn.NormalizationKind = "last_turn"
		} else if t.total != nil && cumulative != nil {
			if delta, ok := subtract(*t.total, *cumulative); ok {
				t.turn.Usage = &delta
				t.turn.NormalizationKind = "cumulative_delta"
			}
		}
		if t.total != nil {
			copy := *t.total
			t.turn.Cumulative = &copy
			cumulative = &copy
		}
		if t.turn.State == "completed" || t.turn.State == "aborted" || t.turn.State == "interrupted" || t.turn.State == "reconciled_truncated" {
			b.Turns = append(b.Turns, t.turn)
		}
		if t.turn.State == "completed" {
			cov := facts.Coverage{ScopeKind: "turn", ScopeID: t.turn.ID, FieldKey: "usage", Eligible: 1, Fidelity: "unavailable", Reason: "usage_unavailable"}
			if usageComplete(t.turn.Usage) {
				cov.Observed = 1
				cov.Fidelity = "exact"
				cov.Reason = ""
			} else if t.turn.Usage != nil {
				cov.Reason = "missing_usage_fields"
			}
			b.Coverage = append(b.Coverage, cov)
		}
	}
	terminal := map[string]bool{}
	completed := map[string]bool{}
	for _, t := range b.Turns {
		terminal[t.SourceTurnID] = true
		completed[t.SourceTurnID] = t.State == "completed"
	}
	current = ""
	toolSeen := map[string]bool{}
	compactionTrigger := ""
	for _, row := range rows {
		kind, turnID, phase := row.Record.Type, current, row.Record.Type
		var ev eventWire
		var payload map[string]json.RawMessage
		switch row.Record.Type {
		case "turn_context":
			var w turnWire
			_ = json.Unmarshal(row.Record.Payload, &w)
			current = w.TurnID
			turnID = w.TurnID
		case "event_msg":
			_ = json.Unmarshal(row.Record.Payload, &ev)
			kind = ev.Type
			phase = ev.Type
			if ev.TurnID != "" {
				current = ev.TurnID
				turnID = ev.TurnID
			}
		case "response_item":
			_ = json.Unmarshal(row.Record.Payload, &payload)
			_ = json.Unmarshal(payload["type"], &kind)
			phase = semanticPhase(kind)
			var m struct {
				TurnID string `json:"turn_id"`
			}
			_ = json.Unmarshal(payload["internal_chat_message_metadata_passthrough"], &m)
			if m.TurnID != "" {
				turnID = m.TurnID
			}
		}
		// Frozen Phase 0 identity keys events by the exact normalized source
		// event kind; semantic phase is a separate child/tool dimension.
		eventID := "event:" + hash([]byte(fmt.Sprintf("%s:%d:%s", b.Source.SegmentFingerprint, row.Ordinal, kind)))
		contentHash := hash(row.Raw)
		internalTurn := ""
		if turnID != "" {
			internalTurn = "turn:" + hash([]byte(b.Session.SourceSessionID+":"+turnID))
		}
		eventOrderKey, _ := phase0.EventOrderKey(b.Segment.SourceOrderKey, row.Ordinal, phase)
		e := facts.Event{ID: eventID, SegmentID: b.Segment.ID, TurnID: internalTurn, SemanticPhase: phase, RecordType: row.Record.Type, Kind: kind, ObservedAt: row.Record.Timestamp, SourceOrderKey: eventOrderKey, RecordOrdinal: row.Ordinal, ByteStart: row.Start, ByteEnd: row.End, PayloadLength: int64(len(row.Raw)), ContentSHA256: contentHash, AdapterVersion: AdapterVersion}
		// Facts remain buffered until their turn reaches a supported terminal
		// state. Source-level records are admitted only when this segment has a
		// concrete completed-turn watermark, enforced again by schema v2.
		eligible := turnID != "" && terminal[turnID]
		if turnID == "" {
			for _, t := range b.Turns {
				if t.State == "completed" {
					eligible = true
					break
				}
			}
		}
		if !eligible {
			continue
		}
		b.Events = append(b.Events, e)
		evidenceID := "evidence:" + hash([]byte(eventID+":"+contentHash))
		b.Evidence = append(b.Evidence, facts.Evidence{ID: evidenceID, EventID: eventID, SourceID: b.Source.ID, EventFingerprint: contentHash, SourceFingerprint: prefixAt(rows, row.Ordinal), Availability: "available"})
		if row.Record.Type == "event_msg" && ev.Type == "context_compacted" {
			compactionTrigger = eventID
		}
		if row.Record.Type == "event_msg" && ev.Type == "token_count" && ev.RateLimits != nil {
			appendCapacity := func(window *int64, used, remaining *float64, resets json.RawMessage, ordinal string) {
				b.Capacity = append(b.Capacity, facts.Capacity{ID: "capacity:" + hash([]byte(eventID+":"+ev.RateLimits.LimitID+":"+ordinal)), EventID: eventID, LimitID: ev.RateLimits.LimitID, ObservedAt: row.Record.Timestamp, WindowMinutes: window, UsedPercent: used, RemainingPercent: remaining, ResetsAt: numberString(resets)})
			}
			if p := ev.RateLimits.Primary; p != nil {
				appendCapacity(p.Window, p.Used, p.Remaining, p.Resets, "primary")
			}
			if p := ev.RateLimits.Secondary; p != nil {
				appendCapacity(p.Window, p.Used, p.Remaining, p.Resets, "secondary")
			}
		}
		if row.Record.Type == "response_item" && kind == "message" {
			var role, id, mp string
			_ = json.Unmarshal(payload["role"], &role)
			_ = json.Unmarshal(payload["id"], &id)
			_ = json.Unmarshal(payload["phase"], &mp)
			text := responseText(payload["content"])
			if role == "user" && b.Session.Title == "" {
				b.Session.Title = friendlySessionTitle(text)
			}
			b.Messages = append(b.Messages, facts.Message{EventID: eventID, Role: role, Phase: mp, SourceMessageID: id, Readable: true, ContentLength: int64(len([]byte(text))), ContentSHA256: hash([]byte(text))})
			b.Events[len(b.Events)-1].SearchText = text
			b.Events[len(b.Events)-1].SearchCategory = "message"
		}
		if row.Record.Type == "response_item" && isTool(kind) {
			var callID, name, status string
			_ = json.Unmarshal(payload["call_id"], &callID)
			_ = json.Unmarshal(payload["name"], &name)
			_ = json.Unmarshal(payload["status"], &status)
			p := semanticPhase(kind)
			key := callID + ":" + p
			if !toolSeen[key] {
				toolSeen[key] = true
				b.Tools = append(b.Tools, facts.ToolCall{ID: "tool:" + hash([]byte(b.Session.SourceSessionID+":"+callID+":"+p)), SessionID: b.Session.ID, TurnID: internalTurn, SourceCallID: callID, Phase: p, EventID: eventID, Name: name, Family: toolFamily(name), Status: status})
			}
			text := toolSearch(payload, kind)
			if text != "" {
				b.Events[len(b.Events)-1].SearchText = text
				b.Events[len(b.Events)-1].SearchCategory = "tool_result"
			}
		}
		if row.Record.Type == "compacted" {
			var c struct {
				Message          string `json:"message"`
				FirstWindowID    string `json:"first_window_id"`
				PreviousWindowID string `json:"previous_window_id"`
				WindowID         string `json:"window_id"`
				WindowNumber     int    `json:"window_number"`
			}
			_ = json.Unmarshal(row.Record.Payload, &c)
			n := int64(len([]byte(c.Message)))
			b.Compactions = append(b.Compactions, facts.Compaction{EventID: eventID, TurnID: internalTurn, TriggerKind: compactionTrigger, SummaryLength: &n, SummarySHA256: hash([]byte(c.Message)), FirstWindowID: c.FirstWindowID, PreviousWindowID: c.PreviousWindowID, WindowID: c.WindowID, WindowNumber: c.WindowNumber})
		}
	}
	if len(b.Lineage) > 0 {
		for _, e := range b.Events {
			if e.Kind != "session_meta" {
				continue
			}
			proof := e
			proof.SemanticPhase = "lineage"
			proof.Kind = "lineage_observation"
			proof.ID = "event:" + hash([]byte(fmt.Sprintf("%s:%d:%s", b.Source.SegmentFingerprint, proof.RecordOrdinal, proof.SemanticPhase)))
			proof.SourceOrderKey, _ = phase0.EventOrderKey(b.Segment.SourceOrderKey, proof.RecordOrdinal, proof.SemanticPhase)
			b.Events = append(b.Events, proof)
			for i := range b.Lineage {
				b.Lineage[i].SourceEventID = proof.ID
			}
			break
		}
	}
	_ = completed
	return b
}

func friendlySessionTitle(text string) string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return ""
	}
	const maxRunes = 96
	title := strings.Join(words, " ")
	runes := []rune(title)
	if len(runes) <= maxRunes {
		return title
	}
	short := strings.TrimSpace(string(runes[:maxRunes]))
	if split := strings.LastIndex(short, " "); split >= maxRunes/2 {
		short = short[:split]
	}
	return strings.TrimSpace(short) + "…"
}

func validate(rows []located) string {
	outer := map[string]bool{"session_meta": true, "turn_context": true, "event_msg": true, "response_item": true, "compacted": true, "world_state": true, "inter_agent_communication_metadata": true}
	events := map[string]bool{"agent_message": true, "context_compacted": true, "entered_review_mode": true, "exited_review_mode": true, "image_generation_end": true, "mcp_tool_call_end": true, "patch_apply_end": true, "sub_agent_activity": true, "task_complete": true, "task_started": true, "thread_rolled_back": true, "thread_settings_applied": true, "token_count": true, "turn_aborted": true, "user_message": true, "web_search_end": true}
	responses := map[string]bool{"agent_message": true, "custom_tool_call": true, "custom_tool_call_output": true, "function_call": true, "function_call_output": true, "message": true, "reasoning": true}
	turns := map[string]bool{}
	for _, r := range rows {
		if !outer[r.Record.Type] {
			return "incompatible_record_envelope"
		}
		if r.Record.Type == "turn_context" {
			var w turnWire
			if json.Unmarshal(r.Record.Payload, &w) != nil || w.TurnID == "" || w.Model == "" || w.Effort == "" || w.CWD == "" || turns[w.TurnID] {
				return "incompatible_turn_context"
			}
			turns[w.TurnID] = true
		}
	}
	for _, r := range rows {
		switch r.Record.Type {
		case "event_msg":
			var w eventWire
			if json.Unmarshal(r.Record.Payload, &w) != nil || !events[w.Type] {
				return "incompatible_event_record"
			}
			if (w.Type == "task_started" || w.Type == "task_complete") && (!turns[w.TurnID]) {
				return "incompatible_turn_identity"
			}
			if w.Type == "token_count" && w.Info == nil {
				return "incompatible_token_record"
			}
		case "response_item":
			var p struct {
				Type   string `json:"type"`
				CallID string `json:"call_id"`
			}
			if json.Unmarshal(r.Record.Payload, &p) != nil || !responses[p.Type] {
				return "incompatible_response_record"
			}
			if isTool(p.Type) && p.CallID == "" {
				return "incompatible_tool_identity"
			}
		}
	}
	return ""
}
func validSource(raw json.RawMessage) bool {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s != ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	return m["subagent"] != nil
}
func sourceIsSubagent(raw json.RawMessage) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(raw, &m) == nil && m["subagent"] != nil
}
func projectIdentity(remote, cwd string) (string, string) {
	if remote != "" {
		u, e := url.Parse(remote)
		if e == nil && u.Host != "" {
			u.Scheme = strings.ToLower(u.Scheme)
			u.Host = strings.ToLower(u.Host)
			u.Path = strings.TrimSuffix(strings.TrimSuffix(u.Path, "/"), ".git")
			return "git_remote", u.String()
		}
		trimmed := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(remote), "/"), ".git")
		if at := strings.Index(trimmed, "@"); at >= 0 {
			if colon := strings.Index(trimmed[at:], ":"); colon >= 0 {
				cut := at + colon
				trimmed = trimmed[:at+1] + strings.ToLower(trimmed[at+1:cut]) + trimmed[cut:]
			}
		}
		return "git_remote", trimmed
	}
	abs, e := filepath.Abs(cwd)
	if e == nil {
		if canonical, ce := filepath.EvalSymlinks(abs); ce == nil {
			abs = canonical
		}
		if root := findGitRoot(abs); root != "" {
			return "git_root", root
		}
		return "cwd", filepath.Clean(abs)
	}
	return "cwd", filepath.Clean(cwd)
}
func findGitRoot(start string) string {
	p := filepath.Clean(start)
	for {
		if info, e := os.Stat(filepath.Join(p, ".git")); e == nil && (info.IsDir() || info.Mode().IsRegular()) {
			return p
		}
		next := filepath.Dir(p)
		if next == p {
			return ""
		}
		p = next
	}
}
func toUsage(w *usageWire) *facts.Usage {
	if w == nil {
		return nil
	}
	return &facts.Usage{Input: w.Input, CachedInput: w.Cached, Output: w.Output, ReasoningOutput: w.Reasoning, Total: w.Total}
}
func usageComplete(u *facts.Usage) bool {
	return u != nil && u.Input != nil && u.CachedInput != nil && u.Output != nil && u.ReasoningOutput != nil && u.Total != nil
}
func subtract(a, b facts.Usage) (facts.Usage, bool) {
	fields := []struct{ x, y *int64 }{{a.Input, b.Input}, {a.CachedInput, b.CachedInput}, {a.Output, b.Output}, {a.ReasoningOutput, b.ReasoningOutput}, {a.Total, b.Total}}
	out := make([]*int64, 5)
	for i, f := range fields {
		if f.x == nil || f.y == nil {
			continue
		}
		v := *f.x - *f.y
		if v < 0 {
			return facts.Usage{}, false
		}
		vv := v
		out[i] = &vv
	}
	return facts.Usage{Input: out[0], CachedInput: out[1], Output: out[2], ReasoningOutput: out[3], Total: out[4]}, true
}
func semanticPhase(k string) string {
	if strings.HasSuffix(k, "_output") {
		return "result"
	}
	if isTool(k) {
		return "request"
	}
	return k
}
func isTool(k string) bool {
	return k == "custom_tool_call" || k == "custom_tool_call_output" || k == "function_call" || k == "function_call_output"
}
func responseText(raw json.RawMessage) string {
	var p []struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(raw, &p)
	var s strings.Builder
	for _, x := range p {
		s.WriteString(x.Text)
	}
	return s.String()
}
func toolSearch(p map[string]json.RawMessage, k string) string {
	if !strings.HasSuffix(k, "_output") {
		return ""
	}
	var s string
	if json.Unmarshal(p["output"], &s) == nil {
		return s
	}
	return responseText(p["output"])
}
func toolFamily(name string) string {
	switch {
	case strings.Contains(name, "exec"):
		return "shell"
	case strings.Contains(name, "search"):
		return "search"
	case strings.Contains(name, "patch"):
		return "patch"
	default:
		return "other"
	}
}
func numberString(raw json.RawMessage) string {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	return string(raw)
}
func hash(b []byte) string { v := sha256.Sum256(b); return hex.EncodeToString(v[:]) }
func prefixAt(rows []located, ordinal int) string {
	h := sha256.New()
	for i := 0; i <= ordinal && i < len(rows); i++ {
		h.Write(rows[i].Raw)
		if i < ordinal && rows[i].Next > rows[i].End {
			h.Write([]byte{'\n'})
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
