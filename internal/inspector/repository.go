package inspector

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/dylanjbarth/codex-inspector/internal/storage"
)

type Repository struct{ Store *storage.Store }

type Coverage struct {
	Fidelity string `json:"fidelity"`
	Observed int    `json:"observed"`
	Eligible int    `json:"eligible"`
	Reason   string `json:"reason,omitempty"`
}

type SessionSummary struct {
	SessionID          string         `json:"sessionId"`
	RootWorkUnitID     string         `json:"rootWorkUnitId"`
	Purpose            string         `json:"purpose"`
	Title              string         `json:"title,omitempty"`
	Project            string         `json:"project,omitempty"`
	StartedAt          string         `json:"startedAt,omitempty"`
	CompletedTurns     int            `json:"completedTurns"`
	LatestCompleted    string         `json:"latestCompleted,omitempty"`
	MatchCategories    []string       `json:"matchCategories"`
	MatchSnippets      []MatchSnippet `json:"matchSnippets"`
	DescendantSessions int            `json:"descendantSessions"`
	DirectTokens       *int64         `json:"directTokens"`
	DescendantTokens   *int64         `json:"descendantTokens"`
}

type MatchSnippet struct {
	Category string `json:"category"`
	Text     string `json:"text"`
}

type SessionPage struct {
	SchemaVersion   int              `json:"schemaVersion"`
	DatasetEpoch    string           `json:"datasetEpoch"`
	AppliedRevision int64            `json:"appliedRevision"`
	Coverage        Coverage         `json:"coverage"`
	Items           []SessionSummary `json:"-"`
	NextCursor      string           `json:"nextCursor,omitempty"`
}

type RootTurn struct {
	TurnID      string  `json:"turnId"`
	Ordinal     int     `json:"ordinal"`
	State       string  `json:"state"`
	StartedAt   string  `json:"startedAt"`
	CompletedAt *string `json:"completedAt"`
}

type MapTurn struct {
	TurnID          string  `json:"turnId"`
	SessionID       string  `json:"sessionId"`
	SessionKind     string  `json:"sessionKind"`
	Ordinal         int     `json:"ordinal"`
	State           string  `json:"state"`
	StartedAt       string  `json:"startedAt"`
	CompletedAt     *string `json:"completedAt"`
	DirectTokens    *int64  `json:"directTokens"`
	InclusiveTokens *int64  `json:"inclusiveTokens"`
	ToolCount       int     `json:"toolCount"`
	ErrorCount      int     `json:"errorCount"`
	CompactionCount int     `json:"compactionCount"`
}

type MapNode struct {
	SessionID       string `json:"sessionId"`
	Kind            string `json:"kind"`
	DirectTokens    *int64 `json:"directTokens"`
	InclusiveTokens *int64 `json:"inclusiveTokens"`
	SpawningTurnID  string `json:"spawningTurnId,omitempty"`
}

type MapEdge struct {
	ParentSessionID string `json:"parentSessionId"`
	ChildSessionID  string `json:"childSessionId"`
	Kind            string `json:"kind"`
}

type SpawnTurnTopology struct {
	ParentSessionID string `json:"parentSessionId"`
	ChildSessionID  string `json:"childSessionId"`
	SpawnTurnID     string `json:"spawnTurnId"`
	EdgeKind        string `json:"edgeKind"`
	Ordinal         int    `json:"ordinal"`
}

type SessionMap struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	DatasetEpoch    string              `json:"datasetEpoch"`
	AppliedRevision int64               `json:"appliedRevision"`
	Coverage        Coverage            `json:"coverage"`
	RootSessionID   string              `json:"rootSessionId"`
	Nodes           []MapNode           `json:"nodes"`
	Edges           []MapEdge           `json:"edges"`
	RootTurns       []RootTurn          `json:"rootTurns"`
	Turns           []MapTurn           `json:"turns"`
	SpawnTopology   []SpawnTurnTopology `json:"spawnTopology"`
}

type LedgerItem struct {
	EventID    string `json:"eventId"`
	EvidenceID string `json:"evidenceId"`
	Kind       string `json:"kind"`
	ObservedAt string `json:"observedAt"`
}

type LedgerPage struct {
	SchemaVersion   int          `json:"schemaVersion"`
	DatasetEpoch    string       `json:"datasetEpoch"`
	AppliedRevision int64        `json:"appliedRevision"`
	Coverage        Coverage     `json:"coverage"`
	TurnID          string       `json:"turnId"`
	Items           []LedgerItem `json:"items"`
	NextCursor      string       `json:"nextCursor,omitempty"`
}

type nodeState struct {
	id, root, purpose, lineage, spawnTurn string
	direct, inclusive                     *int64
}

func snapshot(store *storage.Store, requested int64) (string, int64, error) {
	epoch, latest, err := store.Snapshot()
	if err != nil {
		return "", 0, err
	}
	if requested == 0 {
		requested = latest
	}
	var exists int
	if requested < 1 || requested > latest || store.DB().QueryRow(`SELECT count(*) FROM index_revisions WHERE epoch_id=? AND revision=?`, epoch, requested).Scan(&exists) != nil || exists != 1 {
		return "", 0, errors.New("revision_unavailable")
	}
	return epoch, requested, nil
}

const latestSessions = `WITH sv AS (
	SELECT v.* FROM session_versions v WHERE v.epoch_id=? AND v.revision=(SELECT max(x.revision) FROM session_versions x WHERE x.epoch_id=v.epoch_id AND x.session_id=v.session_id AND x.revision<=?)
), labels AS (
	SELECT l.* FROM session_label_versions l WHERE l.epoch_id=? AND l.revision=(SELECT max(x.revision) FROM session_label_versions x WHERE x.epoch_id=l.epoch_id AND x.session_id=l.session_id AND x.revision<=?)
) `

func (r Repository) Sessions(ctx context.Context, revision int64, query, projectID string, rootIDs []string, offset, limit int) (SessionPage, error) {
	epoch, revision, err := snapshot(r.Store, revision)
	if err != nil {
		return SessionPage{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	eligibleOverride, nextCursorOverride := -1, ""
	if strings.TrimSpace(query) != "" && len(rootIDs) == 0 {
		candidateIDs := make([]string, 0, 200)
		for candidateOffset := 0; ; candidateOffset += 200 {
			_, _, candidates, candidateErr := r.Store.SessionsPage(revision, query, projectID, candidateOffset, 200)
			if candidateErr != nil {
				return SessionPage{}, candidateErr
			}
			for _, candidate := range candidates {
				candidateIDs = append(candidateIDs, candidate.ID)
			}
			if len(candidates) < 200 {
				break
			}
		}
		eligibleOverride = len(candidateIDs)
		if offset > eligibleOverride {
			offset = eligibleOverride
		}
		end := min(offset+limit, eligibleOverride)
		if end < eligibleOverride {
			nextCursorOverride = intString(end)
		}
		rootIDs = candidateIDs[offset:end]
		offset = 0
		if len(rootIDs) == 0 {
			return SessionPage{SchemaVersion: 2, DatasetEpoch: epoch, AppliedRevision: revision, Coverage: Coverage{Fidelity: "exact", Observed: eligibleOverride, Eligible: eligibleOverride}, Items: []SessionSummary{}}, nil
		}
	}
	sessionQuery := latestSessions + `SELECT root.id,rv.purpose,coalesce(l.title,''),coalesce(p.canonical_identity,''),coalesce(min(t.started_at),''),count(t.id),coalesce(max(t.completed_at),max(t.terminal_at),''),count(DISTINCT member.session_id)-1
		FROM sessions root JOIN sv rv ON rv.session_id=root.id AND rv.root_work_unit_id=root.id
		LEFT JOIN labels l ON l.session_id=root.id
		LEFT JOIN projects p ON p.epoch_id=rv.epoch_id AND p.id=rv.project_id
		JOIN sv member ON member.root_work_unit_id=root.id
		JOIN turns t ON t.epoch_id=root.epoch_id AND t.session_id=member.session_id AND t.state='completed' AND t.commit_revision<=?
		WHERE root.epoch_id=? AND root.created_revision<=? AND (?='' OR rv.project_id=?)`
	args := []any{epoch, revision, epoch, revision, revision, epoch, revision, projectID, projectID}
	if len(rootIDs) > 0 {
		sessionQuery += ` AND root.id IN (` + strings.TrimRight(strings.Repeat("?,", len(rootIDs)), ",") + `)`
		for _, id := range rootIDs {
			args = append(args, id)
		}
	}
	sessionQuery += ` GROUP BY root.id ORDER BY max(t.completed_at) DESC,root.id`
	rows, err := r.Store.DB().QueryContext(ctx, sessionQuery, args...)
	if err != nil {
		return SessionPage{}, err
	}
	defer rows.Close()
	all := make([]SessionSummary, 0)
	for rows.Next() {
		var item SessionSummary
		if err = rows.Scan(&item.SessionID, &item.Purpose, &item.Title, &item.Project, &item.StartedAt, &item.CompletedTurns, &item.LatestCompleted, &item.DescendantSessions); err != nil {
			return SessionPage{}, err
		}
		item.RootWorkUnitID = item.SessionID
		item.MatchCategories, item.MatchSnippets, err = r.matches(ctx, epoch, revision, item.SessionID, query)
		if err != nil {
			return SessionPage{}, err
		}
		if query != "" && len(item.MatchCategories) == 0 {
			continue
		}
		if len(item.MatchCategories) == 0 {
			item.MatchCategories = []string{"root session"}
		}
		all = append(all, item)
	}
	if err = rows.Err(); err != nil {
		return SessionPage{}, err
	}
	totals, err := r.rootTotalsBatch(ctx, epoch, revision, all)
	if err != nil {
		return SessionPage{}, err
	}
	for index := range all {
		all[index].DirectTokens = totals[all[index].SessionID].direct
		all[index].DescendantTokens = totals[all[index].SessionID].descendant
	}
	if eligibleOverride >= 0 {
		return SessionPage{SchemaVersion: 2, DatasetEpoch: epoch, AppliedRevision: revision, Coverage: Coverage{Fidelity: "exact", Observed: eligibleOverride, Eligible: eligibleOverride}, Items: all, NextCursor: nextCursorOverride}, nil
	}
	eligible := len(all)
	if offset > eligible {
		offset = eligible
	}
	end := offset + limit
	if end > eligible {
		end = eligible
	}
	page := SessionPage{SchemaVersion: 2, DatasetEpoch: epoch, AppliedRevision: revision, Coverage: Coverage{Fidelity: "exact", Observed: eligible, Eligible: eligible}, Items: all[offset:end]}
	if end < eligible {
		page.NextCursor = intString(end)
	}
	return page, nil
}

type sessionTokenTotals struct {
	direct, descendant *int64
}

func (r Repository) rootTotalsBatch(ctx context.Context, epoch string, revision int64, sessions []SessionSummary) (map[string]sessionTokenTotals, error) {
	out := make(map[string]sessionTokenTotals, len(sessions))
	if len(sessions) == 0 {
		return out, nil
	}
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.SessionID)
	}
	query := latestSessions + `SELECT sv.root_work_unit_id,sv.session_id,count(t.id),count(u.turn_id),coalesce(sum(u.total_tokens),0) FROM sv JOIN turns t ON t.epoch_id=sv.epoch_id AND t.session_id=sv.session_id AND t.state='completed' AND t.commit_revision<=? LEFT JOIN turn_usage u ON u.epoch_id=t.epoch_id AND u.turn_id=t.id AND u.formula_version=1 AND u.total_tokens IS NOT NULL WHERE sv.epoch_id=? AND sv.root_work_unit_id IN (` + strings.TrimRight(strings.Repeat("?,", len(ids)), ",") + `) GROUP BY sv.root_work_unit_id,sv.session_id`
	args := []any{epoch, revision, epoch, revision, revision, epoch}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.Store.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type accumulator struct {
		direct, descendant                 int64
		directObserved, descendantObserved bool
		descendantEligible                 bool
	}
	values := make(map[string]*accumulator, len(ids))
	for rows.Next() {
		var rootID, sessionID string
		var eligible, observed int
		var total int64
		if err = rows.Scan(&rootID, &sessionID, &eligible, &observed, &total); err != nil {
			return nil, err
		}
		value := values[rootID]
		if value == nil {
			value = &accumulator{}
			values[rootID] = value
		}
		if observed == 0 {
			if sessionID != rootID && eligible > 0 {
				value.descendantEligible = true
			}
			continue
		}
		if sessionID == rootID {
			value.direct += total
			value.directObserved = true
		} else {
			value.descendant += total
			value.descendantObserved = true
			value.descendantEligible = true
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		value := values[id]
		if value == nil {
			out[id] = sessionTokenTotals{}
			continue
		}
		var total sessionTokenTotals
		if value.directObserved {
			direct := value.direct
			total.direct = &direct
		}
		if value.descendantObserved {
			descendant := value.descendant
			total.descendant = &descendant
		} else if !value.descendantEligible {
			zero := int64(0)
			total.descendant = &zero
		}
		out[id] = total
	}
	return out, nil
}

func (r Repository) matches(ctx context.Context, epoch string, revision int64, rootID, query string) ([]string, []MatchSnippet, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, []MatchSnippet{}, nil
	}
	categories := map[string]bool{}
	var sourceSessionID string
	if err := r.Store.DB().QueryRowContext(ctx, `SELECT source_session_id FROM sessions WHERE epoch_id=? AND id=?`, epoch, rootID).Scan(&sourceSessionID); err != nil {
		return nil, nil, err
	}
	if query == rootID || query == sourceSessionID {
		categories["root: session ID"] = true
	}
	fts := ftsSearchQuery(query)
	rows, err := r.Store.DB().QueryContext(ctx, `SELECT d.match_category,v.canonical_path,e.byte_start,e.byte_end,e.content_sha256
		FROM turns t
		JOIN events e ON e.epoch_id=t.epoch_id AND e.turn_id=t.id AND e.commit_revision<=?
		JOIN messages msg ON msg.epoch_id=e.epoch_id AND msg.event_id=e.id AND msg.role='user'
		JOIN event_search_documents d ON d.epoch_id=e.epoch_id AND d.event_id=e.id
		JOIN event_search ON event_search.rowid=d.rowid
		JOIN evidence_refs er ON er.epoch_id=e.epoch_id AND er.event_id=e.id
		JOIN source_artifact_versions v ON v.epoch_id=er.epoch_id AND v.source_id=er.source_id AND v.revision=(SELECT max(vx.revision) FROM source_artifact_versions vx WHERE vx.epoch_id=v.epoch_id AND vx.source_id=v.source_id AND vx.revision<=?)
		WHERE t.epoch_id=? AND t.session_id=? AND t.commit_revision<=? AND d.match_category='message' AND event_search MATCH ? ORDER BY e.observed_at,e.record_ordinal LIMIT 20`, revision, revision, epoch, rootID, revision, fts)
	snippets := make([]MatchSnippet, 0, 4)
	seenSnippets := map[string]bool{}
	if err == nil {
		for rows.Next() {
			var category, path, digest string
			var byteStart, byteEnd int64
			if err = rows.Scan(&category, &path, &byteStart, &byteEnd, &digest); err != nil {
				rows.Close()
				return nil, nil, err
			}
			label := "root: user message"
			categories[label] = true
			if len(snippets) < 4 {
				if text := sourceMatchSnippet(path, byteStart, byteEnd, digest, category, query); text != "" && !seenSnippets[label+"\x00"+text] {
					seenSnippets[label+"\x00"+text] = true
					snippets = append(snippets, MatchSnippet{Category: label, Text: text})
				}
			}
		}
		err = rows.Close()
	}
	if err != nil {
		// A punctuation-only FTS phrase is not a fatal discovery failure; metadata
		// matching above remains useful and deterministic.
		err = nil
	}
	out := make([]string, 0, len(categories))
	for category := range categories {
		out = append(out, category)
	}
	sort.Strings(out)
	return out, snippets, err
}

func searchTerms(query string) []string {
	return strings.Fields(strings.ToLower(strings.TrimSpace(query)))
}

func ftsSearchQuery(query string) string {
	terms := searchTerms(query)
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		quoted = append(quoted, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	return strings.Join(quoted, " AND ")
}

func sourceMatchSnippet(path string, start, end int64, expectedHash, category, query string) string {
	if start < 0 || end <= start || end-start > 2*1024*1024 {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	raw, err := io.ReadAll(io.NewSectionReader(file, start, end-start))
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != expectedHash {
		return ""
	}
	var record struct {
		Payload map[string]json.RawMessage `json:"payload"`
	}
	if json.Unmarshal(raw, &record) != nil {
		return ""
	}
	var text string
	if category == "message" {
		text = readableBlocks(record.Payload["content"])
	} else if category == "tool_result" {
		if json.Unmarshal(record.Payload["output"], &text) != nil {
			text = readableBlocks(record.Payload["output"])
		}
	}
	return boundedMatchSnippet(text, query, 240)
}

func readableBlocks(raw json.RawMessage) string {
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, " ")
}

func boundedMatchSnippet(text, query string, limit int) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" || limit < 2 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	start := 0
	needle := strings.ToLower(strings.TrimSpace(query))
	at := strings.Index(strings.ToLower(text), needle)
	if at < 0 {
		for _, term := range searchTerms(query) {
			if at = strings.Index(strings.ToLower(text), term); at >= 0 {
				break
			}
		}
	}
	if at > 0 {
		start = utf8.RuneCountInString(text[:at]) - limit/3
		if start < 0 {
			start = 0
		}
	}
	end := min(len(runes), start+limit-2)
	prefix, suffix := "", ""
	if start > 0 {
		prefix = "…"
	}
	if end < len(runes) {
		suffix = "…"
	}
	return prefix + string(runes[start:end]) + suffix
}

func (r Repository) rootTotals(ctx context.Context, epoch string, revision int64, rootID string) (*int64, *int64, error) {
	rows, err := r.Store.DB().QueryContext(ctx, latestSessions+`SELECT sv.session_id,count(t.id),count(u.turn_id),coalesce(sum(u.total_tokens),0) FROM sv JOIN turns t ON t.epoch_id=sv.epoch_id AND t.session_id=sv.session_id AND t.state='completed' AND t.commit_revision<=? LEFT JOIN turn_usage u ON u.epoch_id=t.epoch_id AND u.turn_id=t.id AND u.formula_version=1 AND u.total_tokens IS NOT NULL WHERE sv.epoch_id=? AND sv.root_work_unit_id=? GROUP BY sv.session_id`, epoch, revision, epoch, revision, revision, epoch, rootID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var direct, descendant int64
	directObserved, descendantObserved := false, false
	descendantEligible := false
	for rows.Next() {
		var id string
		var eligible, observed int
		var total int64
		if err = rows.Scan(&id, &eligible, &observed, &total); err != nil {
			return nil, nil, err
		}
		if observed == 0 {
			if id != rootID && eligible > 0 {
				descendantEligible = true
			}
			continue
		}
		if id == rootID {
			direct += total
			directObserved = true
		} else {
			descendant += total
			descendantObserved = true
			descendantEligible = true
		}
	}
	var directPtr, descendantPtr *int64
	if directObserved {
		directPtr = &direct
	}
	if descendantObserved {
		descendantPtr = &descendant
	} else if !descendantEligible {
		zero := int64(0)
		descendantPtr = &zero
	}
	return directPtr, descendantPtr, rows.Err()
}

func (r Repository) Map(ctx context.Context, revision int64, requestedSessionID string) (SessionMap, error) {
	epoch, revision, err := snapshot(r.Store, revision)
	if err != nil {
		return SessionMap{}, err
	}
	var rootID string
	err = r.Store.DB().QueryRowContext(ctx, latestSessions+`SELECT sv.root_work_unit_id FROM sv WHERE sv.epoch_id=? AND sv.session_id=?`, epoch, revision, epoch, revision, epoch, requestedSessionID).Scan(&rootID)
	if err != nil {
		return SessionMap{}, err
	}
	rows, err := r.Store.DB().QueryContext(ctx, latestSessions+`SELECT sv.session_id,sv.root_work_unit_id,sv.purpose,sv.lineage_coverage,count(t.id),count(u.turn_id),coalesce(sum(u.total_tokens),0) FROM sv LEFT JOIN turns t ON t.epoch_id=sv.epoch_id AND t.session_id=sv.session_id AND t.state='completed' AND t.commit_revision<=? LEFT JOIN turn_usage u ON u.epoch_id=t.epoch_id AND u.turn_id=t.id AND u.formula_version=1 AND u.total_tokens IS NOT NULL WHERE sv.epoch_id=? AND sv.root_work_unit_id=? GROUP BY sv.session_id ORDER BY sv.session_id`, epoch, revision, epoch, revision, revision, epoch, rootID)
	if err != nil {
		return SessionMap{}, err
	}
	states := map[string]*nodeState{}
	for rows.Next() {
		var n nodeState
		var eligible, observed int
		var total int64
		if err = rows.Scan(&n.id, &n.root, &n.purpose, &n.lineage, &eligible, &observed, &total); err != nil {
			rows.Close()
			return SessionMap{}, err
		}
		if observed > 0 {
			value := total
			n.direct = &value
		}
		states[n.id] = &n
	}
	if err = rows.Close(); err != nil {
		return SessionMap{}, err
	}
	if len(states) == 0 {
		return SessionMap{}, sql.ErrNoRows
	}
	edges := make([]MapEdge, 0)
	spawn := make([]SpawnTurnTopology, 0)
	attached := map[string]bool{}
	rows, err = r.Store.DB().QueryContext(ctx, `SELECT parent_session_id,child_session_id,edge_kind,coalesce(spawning_turn_id,'') FROM lineage_edges WHERE epoch_id=? AND commit_revision<=? AND (parent_session_id IN (SELECT session_id FROM session_versions WHERE epoch_id=? AND root_work_unit_id=? AND revision=(SELECT max(x.revision) FROM session_versions x WHERE x.epoch_id=session_versions.epoch_id AND x.session_id=session_versions.session_id AND x.revision<=?)) OR child_session_id IN (SELECT session_id FROM session_versions WHERE epoch_id=? AND root_work_unit_id=? AND revision=(SELECT max(x.revision) FROM session_versions x WHERE x.epoch_id=session_versions.epoch_id AND x.session_id=session_versions.session_id AND x.revision<=?))) ORDER BY parent_session_id,child_session_id,edge_kind`, epoch, revision, epoch, rootID, revision, epoch, rootID, revision)
	if err != nil {
		return SessionMap{}, err
	}
	for rows.Next() {
		var parent, child, kind, turn string
		if err = rows.Scan(&parent, &child, &kind, &turn); err != nil {
			rows.Close()
			return SessionMap{}, err
		}
		if states[parent] == nil || states[child] == nil {
			// Only fork neighbors are included outside the work-unit tree.
			if kind != "forked_from" {
				continue
			}
			external := parent
			if states[parent] != nil {
				external = child
			}
			if states[external] == nil {
				states[external] = &nodeState{id: external, root: external, purpose: "fork"}
			}
		}
		edges = append(edges, MapEdge{ParentSessionID: parent, ChildSessionID: child, Kind: kind})
		attached[child] = true
		if turn != "" {
			spawn = append(spawn, SpawnTurnTopology{ParentSessionID: parent, ChildSessionID: child, SpawnTurnID: turn, EdgeKind: kind, Ordinal: len(spawn)})
			states[child].spawnTurn = turn
		}
	}
	if err = rows.Close(); err != nil {
		return SessionMap{}, err
	}
	for id, state := range states {
		if id != rootID && state.purpose != "fork" && !attached[id] {
			edges = append(edges, MapEdge{ParentSessionID: rootID, ChildSessionID: id, Kind: "unknown"})
		}
	}
	childSets := map[string]map[string]bool{}
	for _, edge := range edges {
		if edge.Kind != "forked_from" && states[edge.ParentSessionID] != nil && states[edge.ChildSessionID] != nil {
			if childSets[edge.ParentSessionID] == nil {
				childSets[edge.ParentSessionID] = map[string]bool{}
			}
			childSets[edge.ParentSessionID][edge.ChildSessionID] = true
		}
	}
	children := map[string][]string{}
	for parent, set := range childSets {
		for child := range set {
			children[parent] = append(children[parent], child)
		}
		sort.Strings(children[parent])
	}
	var inclusive func(string, map[string]bool) (*int64, bool)
	inclusive = func(id string, seen map[string]bool) (*int64, bool) {
		if seen[id] {
			return nil, false
		}
		seen[id] = true
		var total int64
		observed := false
		if states[id].direct != nil {
			total += *states[id].direct
			observed = true
		}
		for _, child := range children[id] {
			if value, ok := inclusive(child, seen); ok {
				total += *value
				observed = true
			}
		}
		delete(seen, id)
		if !observed {
			return nil, false
		}
		return &total, true
	}
	mapNodes := make([]MapNode, 0, len(states))
	partial := false
	for id, state := range states {
		kind := "descendant"
		switch {
		case id == rootID:
			kind = "root"
		case state.purpose == "fork":
			kind = "fork"
		case state.purpose == "orphan":
			kind = "orphan"
		}
		inc, _ := inclusive(id, map[string]bool{})
		state.inclusive = inc
		if state.lineage != "" && state.lineage != "exact" {
			partial = true
		}
		mapNodes = append(mapNodes, MapNode{SessionID: id, Kind: kind, DirectTokens: state.direct, InclusiveTokens: inc, SpawningTurnID: state.spawnTurn})
	}
	sort.Slice(mapNodes, func(i, j int) bool {
		if mapNodes[i].Kind == "root" || mapNodes[j].Kind == "root" {
			return mapNodes[i].Kind == "root"
		}
		if mapNodes[i].Kind != mapNodes[j].Kind {
			return mapNodes[i].Kind < mapNodes[j].Kind
		}
		return mapNodes[i].SessionID < mapNodes[j].SessionID
	})
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].ParentSessionID+edges[i].ChildSessionID < edges[j].ParentSessionID+edges[j].ChildSessionID
	})
	turns, err := r.rootTurns(ctx, epoch, revision, rootID)
	if err != nil {
		return SessionMap{}, err
	}
	mapTurns, err := r.mapTurns(ctx, epoch, revision, rootID, states, spawn)
	if err != nil {
		return SessionMap{}, err
	}
	coverage := Coverage{Fidelity: "exact", Observed: len(mapNodes), Eligible: len(mapNodes)}
	if partial {
		coverage.Fidelity = "derived"
		coverage.Reason = "some lineage relationships are unresolved"
	}
	return SessionMap{SchemaVersion: 2, DatasetEpoch: epoch, AppliedRevision: revision, Coverage: coverage, RootSessionID: rootID, Nodes: mapNodes, Edges: edges, RootTurns: turns, Turns: mapTurns, SpawnTopology: spawn}, nil
}

func (r Repository) mapTurns(ctx context.Context, epoch string, revision int64, rootID string, states map[string]*nodeState, spawn []SpawnTurnTopology) ([]MapTurn, error) {
	ids := make([]string, 0, len(states))
	for id := range states {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return []MapTurn{}, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	query := `SELECT t.session_id,t.id,t.state,coalesce(t.started_at,t.terminal_at),t.completed_at,u.total_tokens,
		count(DISTINCT CASE WHEN tc.semantic_phase='request' THEN tc.id END),
		count(DISTINCT CASE WHEN tc.semantic_phase='result' AND (coalesce(tc.exit_code,0)<>0 OR lower(coalesce(tc.status,'')) IN ('error','failed','failure','cancelled','timed_out')) THEN tc.id END),
		count(DISTINCT c.event_id)
		FROM turns t
		LEFT JOIN turn_usage u ON u.epoch_id=t.epoch_id AND u.turn_id=t.id AND u.formula_version=1
		LEFT JOIN tool_calls tc ON tc.epoch_id=t.epoch_id AND tc.turn_id=t.id
		LEFT JOIN compactions c ON c.epoch_id=t.epoch_id AND c.turn_id=t.id
		WHERE t.epoch_id=? AND t.commit_revision<=? AND t.session_id IN (` + placeholders + `)
		GROUP BY t.session_id,t.id,t.state,t.started_at,t.terminal_at,t.completed_at,u.total_tokens,t.source_order_key
		ORDER BY t.session_id,t.source_order_key,t.id`
	args := []any{epoch, revision}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := r.Store.DB().QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	childrenByTurn := map[string]map[string]bool{}
	for _, relation := range spawn {
		if relation.SpawnTurnID == "" || states[relation.ChildSessionID] == nil {
			continue
		}
		if childrenByTurn[relation.SpawnTurnID] == nil {
			childrenByTurn[relation.SpawnTurnID] = map[string]bool{}
		}
		childrenByTurn[relation.SpawnTurnID][relation.ChildSessionID] = true
	}
	ordinals := map[string]int{}
	out := make([]MapTurn, 0)
	for rows.Next() {
		var turn MapTurn
		var completed sql.NullString
		var direct sql.NullInt64
		if err = rows.Scan(&turn.SessionID, &turn.TurnID, &turn.State, &turn.StartedAt, &completed, &direct, &turn.ToolCount, &turn.ErrorCount, &turn.CompactionCount); err != nil {
			return nil, err
		}
		turn.Ordinal = ordinals[turn.SessionID]
		ordinals[turn.SessionID]++
		turn.SessionKind = "descendant"
		if turn.SessionID == rootID {
			turn.SessionKind = "root"
		} else if state := states[turn.SessionID]; state != nil && state.purpose == "fork" {
			turn.SessionKind = "fork"
		} else if state := states[turn.SessionID]; state != nil && state.purpose == "orphan" {
			turn.SessionKind = "orphan"
		}
		if completed.Valid {
			turn.CompletedAt = &completed.String
		}
		var inclusive int64
		observed := false
		if direct.Valid {
			value := direct.Int64
			turn.DirectTokens = &value
			inclusive += value
			observed = true
		}
		for childID := range childrenByTurn[turn.TurnID] {
			if child := states[childID]; child != nil && child.inclusive != nil {
				inclusive += *child.inclusive
				observed = true
			}
		}
		if observed {
			value := inclusive
			turn.InclusiveTokens = &value
		}
		out = append(out, turn)
	}
	return out, rows.Err()
}

func (r Repository) rootTurns(ctx context.Context, epoch string, revision int64, rootID string) ([]RootTurn, error) {
	rows, err := r.Store.DB().QueryContext(ctx, `SELECT id,state,coalesce(started_at,terminal_at),completed_at FROM turns WHERE epoch_id=? AND session_id=? AND commit_revision<=? ORDER BY source_order_key,id`, epoch, rootID, revision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RootTurn{}
	for rows.Next() {
		var x RootTurn
		var completed sql.NullString
		if err = rows.Scan(&x.TurnID, &x.State, &x.StartedAt, &completed); err != nil {
			return nil, err
		}
		x.Ordinal = len(out)
		if completed.Valid {
			x.CompletedAt = &completed.String
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (r Repository) Ledger(ctx context.Context, revision int64, rootSessionID, turnID string, offset, limit int) (LedgerPage, error) {
	epoch, revision, err := snapshot(r.Store, revision)
	if err != nil {
		return LedgerPage{}, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var canonicalRoot string
	err = r.Store.DB().QueryRowContext(ctx, latestSessions+`SELECT sv.root_work_unit_id FROM sv JOIN turns t ON t.epoch_id=sv.epoch_id AND t.session_id=sv.session_id WHERE sv.epoch_id=? AND sv.root_work_unit_id=? AND t.id=? AND t.commit_revision<=?`, epoch, revision, epoch, revision, epoch, rootSessionID, turnID, revision).Scan(&canonicalRoot)
	if err != nil {
		return LedgerPage{}, err
	}
	rows, err := r.Store.DB().QueryContext(ctx, `SELECT e.id,r.id,e.event_kind,e.observed_at FROM events e JOIN source_segments sg ON sg.epoch_id=e.epoch_id AND sg.id=e.segment_id JOIN evidence_refs r ON r.epoch_id=e.epoch_id AND r.event_id=e.id WHERE e.epoch_id=? AND e.turn_id=? AND e.commit_revision<=? ORDER BY sg.source_order_key,e.record_ordinal,e.semantic_phase LIMIT ? OFFSET ?`, epoch, turnID, revision, limit+1, offset)
	if err != nil {
		return LedgerPage{}, err
	}
	defer rows.Close()
	items := []LedgerItem{}
	for rows.Next() {
		var x LedgerItem
		if err = rows.Scan(&x.EventID, &x.EvidenceID, &x.Kind, &x.ObservedAt); err != nil {
			return LedgerPage{}, err
		}
		items = append(items, x)
	}
	if len(items) == 0 {
		return LedgerPage{}, sql.ErrNoRows
	}
	page := LedgerPage{SchemaVersion: 2, DatasetEpoch: epoch, AppliedRevision: revision, Coverage: Coverage{Fidelity: "exact", Observed: len(items), Eligible: len(items)}, TurnID: turnID, Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = intString(offset + limit)
		page.Coverage.Observed = limit
	}
	return page, rows.Err()
}

func (r Repository) EvidenceKind(ctx context.Context, revision int64, evidenceID string) (string, error) {
	epoch, revision, err := snapshot(r.Store, revision)
	if err != nil {
		return "", err
	}
	var kind string
	err = r.Store.DB().QueryRowContext(ctx, `SELECT e.event_kind FROM evidence_refs r JOIN events e ON e.epoch_id=r.epoch_id AND e.id=r.event_id WHERE r.epoch_id=? AND r.id=? AND e.commit_revision<=?`, epoch, evidenceID, revision).Scan(&kind)
	return kind, err
}

func intString(value int) string {
	if value == 0 {
		return "0"
	}
	const digits = "0123456789"
	b := make([]byte, 0, 20)
	for value > 0 {
		b = append(b, digits[value%10])
		value /= 10
	}
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	return string(b)
}
