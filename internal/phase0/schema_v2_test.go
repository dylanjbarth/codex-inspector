package phase0

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func openSchemaV2(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	schema, err := os.ReadFile(repoPath("docs", "contracts", "schema.sql"))
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		db.Close()
		t.Fatalf("apply schema v2: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mustExecV2(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

func fixtureEvidenceHashes(t *testing.T) (string, string) {
	t.Helper()
	data, err := os.ReadFile(repoPath("fixtures", "synthetic", "root.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	firstEnd := bytes.IndexByte(data, '\n') + 1
	secondRelative := bytes.IndexByte(data[firstEnd:], '\n')
	if firstEnd == 0 || secondRelative < 0 {
		t.Fatal("root fixture lacks two complete records")
	}
	secondEnd := firstEnd + secondRelative + 1
	prefix := sha256.Sum256(data[:secondEnd])
	record := sha256.Sum256(data[firstEnd : secondEnd-1])
	return fmt.Sprintf("%x", prefix), fmt.Sprintf("%x", record)
}

func seedRevisionScenario(t *testing.T, db *sql.DB) {
	t.Helper()
	hashA := strings.Repeat("a", 64)
	hashB := strings.Repeat("b", 64)
	hashC := strings.Repeat("c", 64)
	hashD := strings.Repeat("d", 64)
	hashE := strings.Repeat("e", 64)
	evidencePrefix, evidenceEvent := fixtureEvidenceHashes(t)
	segmentOrderKey, err := SourceOrderKey("2026-07-18T00:00:00Z", hashB)
	if err != nil {
		t.Fatal(err)
	}
	turnOrderKey, err := TurnOrderKey(segmentOrderKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	mustExecV2(t, db, `INSERT INTO dataset_epochs(id,schema_version,adapter_version,state,created_at,activated_at) VALUES('epoch-1',2,?,'active','2026-07-18T00:00:00Z','2026-07-18T00:00:00Z')`, AdapterVersion)
	mustExecV2(t, db, `INSERT INTO epoch_validations(epoch_id,validated_at,validation_sha256) VALUES('epoch-1','2026-07-18T00:00:00Z',?)`, strings.Repeat("9", 64))
	mustExecV2(t, db, `INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES('epoch-1',1,'2026-07-18T00:01:00Z','inventory')`)
	mustExecV2(t, db, `INSERT INTO source_artifacts(id,epoch_id,source_session_id,segment_fingerprint,created_revision) VALUES('source-1','epoch-1','child-001',?,1)`, hashA)
	mustExecV2(t, db, `INSERT INTO source_artifact_versions(epoch_id,source_id,revision,source_kind,canonical_path,byte_size,adapter_version,state,source_evidence_availability,availability_observed_at) VALUES('epoch-1','source-1',1,'active_rollout','/fake/active/root.jsonl',100,?,'current','available','2026-07-18T00:01:01Z')`, AdapterVersion)
	mustExecV2(t, db, `INSERT INTO source_checkpoints(source_id,epoch_id,complete_byte_offset,complete_record_ordinal,observed_size,prefix_sha256,adapter_version,updated_at,updated_revision) VALUES('source-1','epoch-1',75,2,75,?,?,'2026-07-18T00:01:01Z',1)`, hashB, AdapterVersion)
	mustExecV2(t, db, `INSERT INTO projects(id,epoch_id,identity_kind,canonical_identity,display_name,created_revision) VALUES('project-1','epoch-1','cwd','/fake/project','project',1)`)
	mustExecV2(t, db, `INSERT INTO sessions(id,epoch_id,source_session_id,created_revision) VALUES('root-001','epoch-1','root-001',1),('child-001','epoch-1','child-001',1)`)
	mustExecV2(t, db, `INSERT INTO session_versions(epoch_id,session_id,revision,project_id,root_work_unit_id,purpose,lineage_coverage,inventory_reconciled) VALUES('epoch-1','root-001',1,'project-1','root-001','user','exact',0),('epoch-1','child-001',1,'project-1','child-001','other','partial',0)`)
	mustExecV2(t, db, `INSERT INTO session_label_versions(epoch_id,session_id,revision,title,source_updated_at,source_record_ordinal) VALUES('epoch-1','child-001',1,'Old fake title','2026-07-18T00:01:00Z',1)`)
	mustExecV2(t, db, `INSERT INTO source_segments(id,epoch_id,session_id,source_id,segment_fingerprint,source_order_key,source_record_start,created_revision) VALUES('segment-1','epoch-1','child-001','source-1',?,?,0,1)`, hashB, segmentOrderKey)
	mustExecV2(t, db, `INSERT INTO turns(id,epoch_id,session_id,source_turn_id,source_order_key,state,terminal_at,completed_at,commit_revision) VALUES('turn-1','epoch-1','child-001','turn-1',?,'completed','2026-07-18T00:01:20Z','2026-07-18T00:01:20Z',1)`, turnOrderKey)
	mustExecV2(t, db, `INSERT INTO events(id,epoch_id,segment_id,turn_id,record_ordinal,semantic_phase,event_kind,observed_at,byte_start,byte_end,content_sha256,payload_length,adapter_version,commit_revision) VALUES('event-1','epoch-1','segment-1','turn-1',1,'message','message','2026-07-18T00:01:10Z',0,50,?,50,?,1)`, hashC, AdapterVersion)
	mustExecV2(t, db, `INSERT INTO events(id,epoch_id,segment_id,turn_id,record_ordinal,semantic_phase,event_kind,observed_at,byte_start,byte_end,content_sha256,payload_length,adapter_version,commit_revision) VALUES('event-terminal','epoch-1','segment-1','turn-1',2,'terminal','task_complete','2026-07-18T00:01:20Z',50,75,?,25,?,1)`, hashE, AdapterVersion)
	mustExecV2(t, db, `INSERT INTO terminal_watermarks(id,epoch_id,session_id,source_id,segment_id,terminal_event_id,terminal_revision,max_record_ordinal,observed_at) VALUES('watermark-1','epoch-1','child-001','source-1','segment-1','event-terminal',1,3,'2026-07-18T00:01:31Z')`)
	mustExecV2(t, db, `INSERT INTO messages(event_id,epoch_id,role,phase,readable,content_length,content_sha256) VALUES('event-1','epoch-1','assistant','output',1,12,?)`, hashD)
	mustExecV2(t, db, `INSERT INTO turn_usage(epoch_id,turn_id,formula_version,normalization_kind,input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,total_tokens,residual_tokens,fidelity) VALUES('epoch-1','turn-1',1,'last_turn',600,100,200,50,800,0,'exact')`)
	mustExecV2(t, db, `INSERT INTO evidence_refs(id,epoch_id,event_id,source_id,source_prefix_sha256,event_fingerprint) VALUES('evidence-1','epoch-1','event-1','source-1',?,?)`, evidencePrefix, evidenceEvent)
	mustExecV2(t, db, `INSERT INTO evidence_availability_versions(epoch_id,evidence_id,observed_at,availability,availability_revision) VALUES('epoch-1','evidence-1','2026-07-18T00:01:01Z','available',1)`)
	mustExecV2(t, db, `INSERT INTO coverage_observation_versions(epoch_id,scope_kind,scope_id,field_key,revision,fidelity,observed_count,eligible_count) VALUES('epoch-1','session','child-001','usage',1,'exact',1,1)`)
	mustExecV2(t, db, `INSERT INTO event_search_documents(rowid,epoch_id,event_id,match_category) VALUES(1,'epoch-1','event-1','message')`)
	mustExecV2(t, db, `INSERT INTO event_search(rowid,content) VALUES(1,'FAKE-RAW-PAYLOAD-DO-NOT-STORE')`)
	mustExecV2(t, db, `INSERT INTO session_label_search_documents(rowid,epoch_id,session_id,label_revision,match_category) VALUES(1,'epoch-1','child-001',1,'session_title')`)
	mustExecV2(t, db, `INSERT INTO session_label_search(rowid,content) VALUES(1,'old')`)
	mustExecV2(t, db, `INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES('epoch-1',2,'2026-07-18T00:02:00Z','lineage')`)
	mustExecV2(t, db, `INSERT INTO source_artifact_versions(epoch_id,source_id,revision,source_kind,canonical_path,byte_size,adapter_version,state,source_evidence_availability,availability_observed_at) VALUES('epoch-1','source-1',2,'archived_rollout','/fake/archive/root.jsonl',100,?,'current','source_missing','2026-07-18T00:02:01Z')`, AdapterVersion)
	mustExecV2(t, db, `UPDATE source_checkpoints SET complete_byte_offset=100,complete_record_ordinal=3,observed_size=100,updated_at='2026-07-18T00:02:01Z',updated_revision=2 WHERE source_id='source-1'`)
	mustExecV2(t, db, `INSERT INTO session_versions(epoch_id,session_id,revision,project_id,root_work_unit_id,purpose,lineage_coverage,inventory_reconciled) VALUES('epoch-1','root-001',2,'project-1','root-001','user','exact',1),('epoch-1','child-001',2,'project-1','root-001','spawned','exact',1)`)
	mustExecV2(t, db, `INSERT INTO session_label_versions(epoch_id,session_id,revision,title,source_updated_at,source_record_ordinal) VALUES('epoch-1','child-001',2,'New fake title','2026-07-18T00:02:00Z',2)`)
	mustExecV2(t, db, `INSERT INTO evidence_availability_versions(epoch_id,evidence_id,observed_at,availability,availability_revision) VALUES('epoch-1','evidence-1','2026-07-18T00:02:01Z','source_missing',2)`)
	mustExecV2(t, db, `INSERT INTO coverage_observation_versions(epoch_id,scope_kind,scope_id,field_key,revision,fidelity,observed_count,eligible_count) VALUES('epoch-1','session','child-001','usage',2,'unavailable',0,1)`)
	mustExecV2(t, db, `INSERT INTO events(id,epoch_id,segment_id,record_ordinal,semantic_phase,event_kind,observed_at,byte_start,byte_end,content_sha256,payload_length,adapter_version,terminal_watermark_id,commit_revision) VALUES('event-2','epoch-1','segment-1',3,'lineage','lineage_observation','2026-07-18T00:01:30Z',75,100,?,25,?,'watermark-1',2)`, hashD, AdapterVersion)
	mustExecV2(t, db, `INSERT INTO lineage_edges(epoch_id,parent_session_id,child_session_id,edge_kind,spawning_turn_id,source_event_id,commit_revision) VALUES('epoch-1','root-001','child-001','spawned',NULL,'event-2',2)`)
	mustExecV2(t, db, `INSERT INTO session_label_search_documents(rowid,epoch_id,session_id,label_revision,match_category) VALUES(2,'epoch-1','child-001',2,'session_title')`)
	mustExecV2(t, db, `INSERT INTO session_label_search(rowid,content) VALUES(2,'new')`)
}

func revisionSnapshot(t *testing.T, db *sql.DB, revision int) RevisionSnapshot {
	t.Helper()
	var snapshot RevisionSnapshot
	snapshot.AppliedRevision = revision
	if err := db.QueryRow(`SELECT source_kind,canonical_path FROM source_artifact_versions WHERE epoch_id='epoch-1' AND source_id='source-1' AND revision<=? ORDER BY revision DESC LIMIT 1`, revision).Scan(&snapshot.SourceKind, &snapshot.SourcePath); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT sv.root_work_unit_id,sv.purpose,slv.title FROM session_versions sv JOIN session_label_versions slv ON slv.epoch_id=sv.epoch_id AND slv.session_id=sv.session_id WHERE sv.epoch_id='epoch-1' AND sv.session_id='child-001' AND sv.revision=(SELECT max(revision) FROM session_versions WHERE epoch_id=sv.epoch_id AND session_id=sv.session_id AND revision<=?) AND slv.revision=(SELECT max(revision) FROM session_label_versions WHERE epoch_id=slv.epoch_id AND session_id=slv.session_id AND revision<=?)`, revision, revision).Scan(&snapshot.RootWorkUnitID, &snapshot.Purpose, &snapshot.SessionTitle); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT s.content FROM session_label_search s JOIN session_label_search_documents d ON d.rowid=s.rowid WHERE session_label_search MATCH 'old OR new' AND d.epoch_id='epoch-1' AND d.session_id='child-001' AND d.label_revision=(SELECT max(revision) FROM session_label_versions WHERE epoch_id=d.epoch_id AND session_id=d.session_id AND revision<=?) ORDER BY d.label_revision`, revision)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var unavailable sql.NullString
		if err := rows.Scan(&unavailable); err != nil {
			t.Fatal(err)
		}
		if revision == 1 {
			snapshot.TitleMatches = append(snapshot.TitleMatches, "old")
		} else {
			snapshot.TitleMatches = append(snapshot.TitleMatches, "new")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var availabilityRevision sql.NullInt64
	if err := db.QueryRow(`SELECT e.commit_revision,a.availability,a.observed_at,a.availability_revision FROM evidence_refs r JOIN events e ON e.epoch_id=r.epoch_id AND e.id=r.event_id JOIN evidence_availability_versions a ON a.epoch_id=r.epoch_id AND a.evidence_id=r.id WHERE r.id='evidence-1' AND e.commit_revision<=? ORDER BY a.observed_at DESC LIMIT 1`, revision).Scan(&snapshot.EvidenceFactRevision, &snapshot.EvidenceAvailability, &snapshot.AvailabilityObservedAt, &availabilityRevision); err != nil {
		t.Fatal(err)
	}
	if availabilityRevision.Valid {
		value := int(availabilityRevision.Int64)
		snapshot.AvailabilityRevision = &value
	}
	if err := db.QueryRow(`SELECT fidelity,observed_count,eligible_count FROM coverage_observation_versions WHERE epoch_id='epoch-1' AND scope_kind='session' AND scope_id='child-001' AND field_key='usage' AND revision<=? ORDER BY revision DESC LIMIT 1`, revision).Scan(&snapshot.CoverageFidelity, &snapshot.CoverageObserved, &snapshot.CoverageEligible); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestSQLiteSchemaV2RevisionGolden(t *testing.T) {
	db := openSchemaV2(t)
	seedRevisionScenario(t, db)
	var want RevisionGolden
	loadJSON(t, repoPath("fixtures", "synthetic", "expected-revisions.json"), &want)
	actual := RevisionGolden{
		PinnedRevisionOne:    revisionSnapshot(t, db, 1),
		PinnedRevisionTwo:    revisionSnapshot(t, db, 2),
		V2EpochSwapValidated: proveSeparateFileV1RebuildAndCatalogActivation(t),
	}
	if err := db.QueryRow(`SELECT schema_version FROM schema_metadata WHERE singleton=1`).Scan(&actual.SchemaVersion); err != nil {
		t.Fatal(err)
	}
	for _, input := range []struct{ start, fingerprint string }{
		{"2026-07-18T00:00:01Z", strings.Repeat("a", 64)},
		{"2026-07-18T00:00:00Z", strings.Repeat("b", 64)},
		{"2026-07-18T00:00:00Z", strings.Repeat("a", 64)},
	} {
		key, err := SourceOrderKey(input.start, input.fingerprint)
		if err != nil {
			t.Fatal(err)
		}
		actual.OrderedSourceKeys = append(actual.OrderedSourceKeys, key)
	}
	sort.Strings(actual.OrderedSourceKeys)
	for _, input := range []struct {
		segment string
		ordinal int
	}{{actual.OrderedSourceKeys[2], 1}, {actual.OrderedSourceKeys[0], 2}, {actual.OrderedSourceKeys[1], 1}} {
		key, err := TurnOrderKey(input.segment, input.ordinal)
		if err != nil {
			t.Fatal(err)
		}
		actual.OrderedTurnKeys = append(actual.OrderedTurnKeys, key)
	}
	sort.Strings(actual.OrderedTurnKeys)
	for _, input := range []struct {
		segment, phase string
		ordinal        int
	}{{actual.OrderedSourceKeys[1], "message", 1}, {actual.OrderedSourceKeys[0], "tool_result", 1}, {actual.OrderedSourceKeys[0], "message", 1}, {actual.OrderedSourceKeys[0], "message", 2}} {
		key, err := EventOrderKey(input.segment, input.ordinal, input.phase)
		if err != nil {
			t.Fatal(err)
		}
		actual.OrderedEventKeys = append(actual.OrderedEventKeys, key)
	}
	sort.Strings(actual.OrderedEventKeys)
	actual.ProjectDisplayNames = map[string]string{}
	for kind, identity := range map[string]string{"git_remote": "github.com/example/Codex-Inspector.git/", "git_root": "/work/example/repository/", "cwd": "/work/example/scratch"} {
		name, err := ProjectDisplayName(kind, identity)
		if err != nil {
			t.Fatal(err)
		}
		actual.ProjectDisplayNames[kind] = name
	}
	first := Usage{Input: 800, Cached: 200, Output: 300, Reasoning: 100, Total: 1200}
	second := Usage{Input: 1300, Cached: 400, Output: 500, Reasoning: 200, Total: 2000}
	metrics, err := CalculateMetrics([]SourceDecision{
		{Supported: true, SessionID: "resume-1", Turns: []Turn{{ID: "first", Completed: true, CompletedAt: "2026-07-18T00:00:00Z", Usage: &first, Cumulative: &first}}},
		{Supported: true, SessionID: "resume-1", Turns: []Turn{{ID: "second", Completed: true, CompletedAt: "2026-07-18T00:01:00Z", Cumulative: &second}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	actual.CrossSegmentResumeTokens = metrics.RecordedTokens - first.Total
	var validNull int
	if err := db.QueryRow(`SELECT count(*) FROM events WHERE id='event-2' AND turn_id IS NULL AND terminal_watermark_id='watermark-1'`).Scan(&validNull); err != nil {
		t.Fatal(err)
	}
	actual.NullTurnBeforeWatermarkAccepted = validNull == 1
	hash := strings.Repeat("f", 64)
	_, err = db.Exec(`INSERT INTO events(id,epoch_id,segment_id,record_ordinal,semantic_phase,event_kind,observed_at,byte_start,byte_end,content_sha256,payload_length,adapter_version,terminal_watermark_id,commit_revision) VALUES('golden-late','epoch-1','segment-1',4,'source','source_boundary','2026-07-18T00:01:30Z',100,120,?,20,?,'watermark-1',2)`, hash, AdapterVersion)
	actual.NullTurnAfterWatermarkRejected = err != nil
	_, err = db.Exec(`INSERT INTO events(id,epoch_id,segment_id,turn_id,record_ordinal,semantic_phase,event_kind,observed_at,byte_start,byte_end,content_sha256,payload_length,adapter_version,commit_revision) VALUES('golden-wrong-revision','epoch-1','segment-1','turn-1',4,'message','message','2026-07-18T00:01:11Z',120,140,?,20,?,2)`, hash, AdapterVersion)
	actual.ChildRevisionInheritanceEnforced = err != nil
	var prefix, eventFingerprint string
	if err := db.QueryRow(`SELECT source_prefix_sha256,event_fingerprint FROM evidence_refs WHERE id='evidence-1'`).Scan(&prefix, &eventFingerprint); err != nil {
		t.Fatal(err)
	}
	_, malformedHashErr := db.Exec(`INSERT INTO evidence_refs(id,epoch_id,event_id,source_id,source_prefix_sha256,event_fingerprint) VALUES('golden-bad-hash','epoch-1','event-1','source-1',?,?)`, strings.Repeat("A", 64), hash)
	actual.EvidenceFingerprintsValidated = prefix != eventFingerprint && malformedHashErr != nil
	_, err = db.Exec(`INSERT INTO turns(id,epoch_id,session_id,source_turn_id,source_order_key,state,terminal_at,commit_revision) VALUES('golden-provisional','epoch-1','child-001','golden-provisional',?,'provisional','2026-07-18T00:03:00Z',2)`, validTurnOrderKey(99))
	actual.ProvisionalTurnRejected = err != nil
	_, err = db.Exec(`UPDATE schema_metadata SET schema_version=1 WHERE singleton=1`)
	actual.V1RequiresRebuild = err != nil
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("revision scenario mismatch\n got: %#v\nwant: %#v", actual, want)
	}
	var raw sql.NullString
	if err := db.QueryRow(`SELECT content FROM event_search WHERE event_search MATCH 'payload'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw.Valid {
		t.Fatalf("contentless search exposed raw payload: %q", raw.String)
	}
}

func TestSQLiteSchemaV2Constraints(t *testing.T) {
	db := openSchemaV2(t)
	seedRevisionScenario(t, db)
	hash := strings.Repeat("f", 64)
	mustExecV2(t, db, `INSERT INTO dataset_epochs(id,schema_version,adapter_version,state,created_at) VALUES('epoch-2',2,?,'building','2026-07-18T00:03:00Z')`, AdapterVersion)
	mustExecV2(t, db, `INSERT INTO source_artifacts(id,epoch_id,source_session_id,segment_fingerprint,created_revision) VALUES('source-2','epoch-1','other-source',?,2)`, hash)
	if _, err := db.Exec(`INSERT INTO turns(id,epoch_id,session_id,source_turn_id,source_order_key,state,terminal_at,commit_revision) VALUES('provisional','epoch-1','child-001','provisional',?,'provisional','2026-07-18T00:03:00Z',2)`, validTurnOrderKey(99)); err == nil {
		t.Fatal("provisional turn was persisted")
	}
	if _, err := db.Exec(`INSERT INTO events(id,epoch_id,segment_id,record_ordinal,semantic_phase,event_kind,observed_at,byte_start,byte_end,content_sha256,payload_length,adapter_version,terminal_watermark_id,commit_revision) VALUES('late-null','epoch-1','segment-1',4,'lineage','lineage_observation','2026-07-18T00:04:00Z',100,120,?,20,?,'watermark-1',2)`, hash, AdapterVersion); err == nil {
		t.Fatal("null-turn event after terminal watermark was persisted")
	}
	if _, err := db.Exec(`INSERT INTO events(id,epoch_id,segment_id,turn_id,record_ordinal,semantic_phase,event_kind,observed_at,byte_start,byte_end,content_sha256,payload_length,adapter_version,commit_revision) VALUES('wrong-revision','epoch-1','segment-1','turn-1',4,'message','message','2026-07-18T00:01:11Z',120,140,?,20,?,2)`, hash, AdapterVersion); err == nil {
		t.Fatal("child event escaped its terminal turn revision")
	}
	if _, err := db.Exec(`INSERT INTO events(id,epoch_id,segment_id,record_ordinal,semantic_phase,event_kind,observed_at,byte_start,byte_end,content_sha256,payload_length,adapter_version,terminal_watermark_id,commit_revision) VALUES('no-watermark','epoch-1','segment-1',2,'source','source_boundary','2026-07-18T00:01:30Z',100,120,?,20,?,'missing',2)`, hash, AdapterVersion); err == nil {
		t.Fatal("null-turn event without relational terminal proof was accepted")
	}
	if _, err := db.Exec(`INSERT INTO events(id,epoch_id,segment_id,record_ordinal,semantic_phase,event_kind,observed_at,byte_start,byte_end,content_sha256,payload_length,adapter_version,terminal_watermark_id,commit_revision) VALUES('bad-kind','epoch-1','segment-1',2,'message','message','2026-07-18T00:01:30Z',100,120,?,20,?,'watermark-1',2)`, hash, AdapterVersion); err == nil {
		t.Fatal("disallowed null-turn event kind was accepted")
	}
	if _, err := db.Exec(`INSERT INTO terminal_watermarks(id,epoch_id,session_id,source_id,segment_id,terminal_event_id,terminal_revision,max_record_ordinal,observed_at) VALUES('wrong-session','epoch-1','root-001','source-1','segment-1','event-terminal',1,3,'2026-07-18T00:01:31Z')`); err == nil {
		t.Fatal("watermark crossed session ownership")
	}
	if _, err := db.Exec(`INSERT INTO terminal_watermarks(id,epoch_id,session_id,source_id,segment_id,terminal_event_id,terminal_revision,max_record_ordinal,observed_at) VALUES('wrong-source','epoch-1','child-001','source-2','segment-1','event-terminal',1,3,'2026-07-18T00:01:31Z')`); err == nil {
		t.Fatal("watermark crossed source ownership")
	}
	if _, err := db.Exec(`INSERT INTO terminal_watermarks(id,epoch_id,session_id,source_id,segment_id,terminal_event_id,terminal_revision,max_record_ordinal,observed_at) VALUES('no-terminal','epoch-1','child-001','source-1','segment-1','missing-event',1,3,'2026-07-18T00:01:31Z')`); err == nil {
		t.Fatal("watermark without a terminal turn was accepted")
	}
	segmentTwoOrder, err := SourceOrderKey("2026-07-18T00:00:01Z", hash)
	if err != nil {
		t.Fatal(err)
	}
	mustExecV2(t, db, `INSERT INTO source_segments(id,epoch_id,session_id,source_id,segment_fingerprint,source_order_key,source_record_start,created_revision) VALUES('segment-2','epoch-1','child-001','source-1',?,?,0,2)`, hash, segmentTwoOrder)
	if _, err := db.Exec(`INSERT INTO terminal_watermarks(id,epoch_id,session_id,source_id,segment_id,terminal_event_id,terminal_revision,max_record_ordinal,observed_at) VALUES('wrong-segment','epoch-1','child-001','source-1','segment-2','event-terminal',1,3,'2026-07-18T00:01:31Z')`); err == nil {
		t.Fatal("same-session terminal event from another segment proved a watermark")
	}
	if _, err := db.Exec(`INSERT INTO messages(event_id,epoch_id,role,phase,readable,content_length,content_sha256) VALUES('event-2','epoch-2','assistant','output',1,1,?)`, hash); err == nil {
		t.Fatal("message crossed its event epoch")
	}
	if _, err := db.Exec(`INSERT INTO capacity_observations(id,epoch_id,event_id,limit_id,observed_at) VALUES('cross-capacity','epoch-2','event-1','fake','2026-07-18T00:03:00Z')`); err == nil {
		t.Fatal("capacity observation crossed its event epoch")
	}
	if _, err := db.Exec(`INSERT INTO turn_usage(epoch_id,turn_id,formula_version,normalization_kind,fidelity) VALUES('epoch-2','turn-1',1,'last_turn','unavailable')`); err == nil {
		t.Fatal("usage crossed its terminal turn epoch")
	}
	if _, err := db.Exec(`INSERT INTO event_search_documents(rowid,epoch_id,event_id,match_category) VALUES(99,'epoch-2','event-1','message')`); err == nil {
		t.Fatal("search provenance crossed its event epoch")
	}
	if _, err := db.Exec(`INSERT INTO evidence_availability_versions(epoch_id,evidence_id,observed_at,availability) VALUES('epoch-2','evidence-1','2026-07-18T00:03:00Z','available')`); err == nil {
		t.Fatal("evidence availability crossed its evidence epoch")
	}
	if _, err := db.Exec(`INSERT INTO compactions(event_id,epoch_id,session_id,turn_id,trigger_kind) VALUES('event-1','epoch-1','root-001','turn-1','manual')`); err == nil {
		t.Fatal("compaction accepted a turn from the wrong session")
	}
	if _, err := db.Exec(`INSERT INTO tool_calls(id,epoch_id,session_id,turn_id,source_call_id,semantic_phase,event_id,tool_name,tool_family) VALUES('bad-tool','epoch-1','root-001','turn-1','call','request','event-1','fake','fake')`); err == nil {
		t.Fatal("tool call crossed session ownership")
	}
	if _, err := db.Exec(`INSERT INTO evidence_refs(id,epoch_id,event_id,source_id,source_prefix_sha256,event_fingerprint) VALUES('bad-evidence','epoch-1','event-1','source-1',?,?)`, strings.Repeat("A", 64), hash); err == nil {
		t.Fatal("uppercase source-prefix fingerprint was accepted")
	}
	if _, err := db.Exec(`INSERT INTO evidence_refs(id,epoch_id,event_id,source_id,source_prefix_sha256,event_fingerprint) VALUES('wrong-source-evidence','epoch-1','event-1','source-2',?,?)`, hash, strings.Repeat("e", 64)); err == nil {
		t.Fatal("evidence crossed its parent event source")
	}
	var prefix, event string
	if err := db.QueryRow(`SELECT source_prefix_sha256,event_fingerprint FROM evidence_refs WHERE id='evidence-1'`).Scan(&prefix, &event); err != nil {
		t.Fatal(err)
	}
	if prefix == event || len(prefix) != 64 || len(event) != 64 {
		t.Fatalf("evidence fingerprints not independently pinned: prefix=%q event=%q", prefix, event)
	}
	if _, err := db.Exec(`UPDATE dataset_epochs SET state='active',activated_at='2026-07-18T00:03:00Z' WHERE id='epoch-2'`); err == nil {
		t.Fatal("multiple active v2 epochs were accepted")
	}
	if _, err := db.Exec(`INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES('epoch-1',4,'2026-07-18T00:04:00Z','append')`); err == nil {
		t.Fatal("out-of-order revision allocation was accepted")
	}
	mustExecV2(t, db, `INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES('epoch-1',3,'2026-07-18T00:03:00Z','append')`)
	mustExecV2(t, db, `INSERT INTO sessions(id,epoch_id,source_session_id,created_revision) VALUES('future-session','epoch-1','future-session',3)`)
	if _, err := db.Exec(`INSERT INTO session_versions(epoch_id,session_id,revision,root_work_unit_id,purpose,lineage_coverage,inventory_reconciled) VALUES('epoch-1','future-session',2,'future-session','user','exact',0)`); err == nil {
		t.Fatal("session version referenced a future-created session")
	}
	if _, err := db.Exec(`INSERT INTO coverage_observation_versions(epoch_id,scope_kind,scope_id,field_key,revision,fidelity,observed_count,eligible_count) VALUES('epoch-1','session','future-session','usage',2,'unavailable',0,0)`); err == nil {
		t.Fatal("coverage referenced a future-created scope")
	}
	if _, err := db.Exec(`INSERT INTO coverage_observation_versions(epoch_id,scope_kind,scope_id,field_key,revision,fidelity,observed_count,eligible_count) VALUES('epoch-1','nonsense','child-001','usage',2,'unavailable',0,0)`); err == nil {
		t.Fatal("coverage accepted an unknown polymorphic scope")
	}
	if _, err := db.Exec(`UPDATE source_artifact_versions SET canonical_path='/mutated' WHERE epoch_id='epoch-1' AND source_id='source-1' AND revision=1`); err == nil {
		t.Fatal("append-only source version was updated")
	}
	if _, err := db.Exec(`DELETE FROM session_label_versions WHERE epoch_id='epoch-1' AND session_id='child-001' AND revision=1`); err == nil {
		t.Fatal("append-only label version was deleted")
	}
	if _, err := db.Exec(`UPDATE events SET event_kind='mutated' WHERE id='event-1'`); err == nil {
		t.Fatal("immutable event was updated")
	}
	if _, err := db.Exec(`DELETE FROM events WHERE id='event-1'`); err == nil {
		t.Fatal("immutable event was deleted")
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported a violation")
	}
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity_check=%q err=%v", integrity, err)
	}
}

func validTurnOrderKey(ordinal int) string {
	key, _ := TurnOrderKey(strings.Repeat("0", 20)+":"+strings.Repeat("f", 64), ordinal)
	return key
}

func TestSQLiteSchemaV2OneActiveEpochAndV1MetadataRejection(t *testing.T) {
	db := openSchemaV2(t)
	mustExecV2(t, db, `INSERT INTO dataset_epochs(id,schema_version,adapter_version,state,created_at,activated_at) VALUES('old',2,?,'active','2026-07-18T00:00:00Z','2026-07-18T00:00:00Z'),('new',2,?,'building','2026-07-18T00:01:00Z',NULL)`, AdapterVersion, AdapterVersion)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`UPDATE dataset_epochs SET state='superseded' WHERE id='old'`); err == nil {
		err = ActivateValidatedEpoch(context.Background(), tx, "new", "2026-07-18T00:01:30Z", "2026-07-18T00:02:00Z", strings.Repeat("9", 64))
	}
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var active string
	if err := db.QueryRow(`SELECT id FROM dataset_epochs WHERE state='active'`).Scan(&active); err != nil || active != "new" {
		t.Fatalf("v2 epoch swap active=%q err=%v", active, err)
	}
	if _, err := db.Exec(`UPDATE schema_metadata SET schema_version=1 WHERE singleton=1`); err == nil {
		t.Fatal("schema-v2 database accepted an in-place downgrade to schema v1")
	}
	var version int
	if err := db.QueryRow(`SELECT schema_version FROM schema_metadata WHERE singleton=1`).Scan(&version); err != nil || version != 2 {
		t.Fatalf("schema version after rejected downgrade=%d err=%v", version, err)
	}
}

func TestSeparateFileV1RebuildAndCatalogActivation(t *testing.T) {
	if !proveSeparateFileV1RebuildAndCatalogActivation(t) {
		t.Fatal("separate-file activation proof did not complete")
	}
}

func proveSeparateFileV1RebuildAndCatalogActivation(t *testing.T) bool {
	t.Helper()
	dir := t.TempDir()
	v1Path := filepath.Join(dir, "index-v1.sqlite")
	v2BadPath := filepath.Join(dir, "index-v2-bad.sqlite")
	v2Path := filepath.Join(dir, "index-v2-good.sqlite")
	catalog := filepath.Join(dir, "active-index")
	v1Schema, err := os.ReadFile(repoPath("fixtures", "synthetic", "schema-v1.sql"))
	if err != nil {
		t.Fatal(err)
	}
	v1 := openSQLiteFile(t, v1Path, string(v1Schema))
	if err := os.WriteFile(catalog, []byte(filepath.Base(v1Path)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldRead, err := v1.Begin()
	if err != nil {
		t.Fatal(err)
	}
	var oldCount int
	if err := oldRead.QueryRow(`SELECT count(*) FROM legacy_facts`).Scan(&oldCount); err != nil || oldCount != 1 {
		t.Fatalf("open v1 snapshot count=%d err=%v", oldCount, err)
	}

	schemaV2, err := os.ReadFile(repoPath("docs", "contracts", "schema.sql"))
	if err != nil {
		t.Fatal(err)
	}
	bad := openSQLiteFile(t, v2BadPath, string(schemaV2))
	expectedPrefix, expectedEvent := fixtureEvidenceHashes(t)
	if validateV2Candidate(bad, expectedPrefix, expectedEvent) == nil {
		t.Fatal("incomplete v2 candidate unexpectedly validated")
	}
	if got := readCatalog(t, catalog); got != filepath.Base(v1Path) {
		t.Fatalf("failed validation changed active catalog to %q", got)
	}

	good := openSQLiteFile(t, v2Path, string(schemaV2))
	seedRevisionScenario(t, good)
	if err := validateV2Candidate(good, expectedPrefix, expectedEvent); err != nil {
		t.Fatalf("complete v2 candidate validation: %v", err)
	}
	if err := good.Close(); err != nil {
		t.Fatal(err)
	}
	activateCatalog(t, catalog, filepath.Base(v2Path))
	newPath := filepath.Join(dir, readCatalog(t, catalog))
	newRead, err := sql.Open("sqlite", newPath)
	if err != nil {
		t.Fatal(err)
	}
	defer newRead.Close()
	var newVersion, revisions, facts int
	if err := newRead.QueryRow(`SELECT schema_version FROM schema_metadata WHERE singleton=1`).Scan(&newVersion); err != nil {
		t.Fatal(err)
	}
	if err := newRead.QueryRow(`SELECT count(*) FROM index_revisions`).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if err := newRead.QueryRow(`SELECT count(*) FROM events`).Scan(&facts); err != nil {
		t.Fatal(err)
	}
	if newVersion != 2 || revisions != 2 || facts != 3 {
		t.Fatalf("new requests did not open validated v2: version=%d revisions=%d events=%d", newVersion, revisions, facts)
	}
	if err := oldRead.QueryRow(`SELECT count(*) FROM legacy_facts`).Scan(&oldCount); err != nil || oldCount != 1 {
		t.Fatalf("old v1 read handle could not finish: count=%d err=%v", oldCount, err)
	}
	if err := oldRead.Commit(); err != nil {
		t.Fatal(err)
	}
	return true
}

func openSQLiteFile(t *testing.T, path, schema string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func validateV2Candidate(db *sql.DB, expectedPrefix, expectedEvent string) error {
	var version, epochs, revisions, sources, sessions, sourceVersions, sessionVersions, labels, turns, events, evidence, coverage int
	if err := db.QueryRow(`SELECT schema_version FROM schema_metadata WHERE singleton=1`).Scan(&version); err != nil || version != 2 {
		return errors.New("schema metadata is not v2")
	}
	if err := db.QueryRow(`SELECT count(*) FROM dataset_epochs e WHERE state IN ('active','building') AND schema_version=2 AND adapter_version=? AND (state='building' OR EXISTS (SELECT 1 FROM epoch_validations v WHERE v.epoch_id=e.id))`, AdapterVersion).Scan(&epochs); err != nil || epochs != 1 {
		return errors.New("queryable epoch or adapter mismatch")
	}
	var nonMonotonic int
	if err := db.QueryRow(`SELECT count(*) FROM (SELECT revision,row_number() OVER (PARTITION BY epoch_id ORDER BY revision) AS expected FROM index_revisions) WHERE revision<>expected`).Scan(&nonMonotonic); err != nil || nonMonotonic != 0 {
		return errors.New("revision sequence is not monotonic")
	}
	if err := db.QueryRow(`SELECT count(*) FROM index_revisions`).Scan(&revisions); err != nil || revisions != 2 {
		return errors.New("revision cardinality mismatch")
	}
	if err := db.QueryRow(`SELECT count(*) FROM source_artifacts`).Scan(&sources); err != nil || sources != 1 {
		return errors.New("source cardinality mismatch")
	}
	if err := db.QueryRow(`SELECT count(*) FROM sessions`).Scan(&sessions); err != nil || sessions != 2 {
		return errors.New("session cardinality mismatch")
	}
	if err := db.QueryRow(`SELECT count(*) FROM source_artifact_versions`).Scan(&sourceVersions); err != nil || sourceVersions != 2 {
		return errors.New("source-version cardinality mismatch")
	}
	if err := db.QueryRow(`SELECT count(*) FROM session_versions`).Scan(&sessionVersions); err != nil || sessionVersions != 4 {
		return errors.New("session-version cardinality mismatch")
	}
	if err := db.QueryRow(`SELECT count(*) FROM session_label_versions`).Scan(&labels); err != nil || labels != 2 {
		return errors.New("label cardinality mismatch")
	}
	if err := db.QueryRow(`SELECT count(*) FROM turns`).Scan(&turns); err != nil || turns != 1 {
		return errors.New("turn cardinality mismatch")
	}
	if err := db.QueryRow(`SELECT count(*) FROM events`).Scan(&events); err != nil || events != 3 {
		return errors.New("event cardinality mismatch")
	}
	if err := db.QueryRow(`SELECT count(*) FROM coverage_observation_versions`).Scan(&coverage); err != nil || coverage != 2 {
		return errors.New("coverage cardinality mismatch")
	}
	if err := db.QueryRow(`SELECT count(*) FROM evidence_refs WHERE source_prefix_sha256=? AND event_fingerprint=?`, expectedPrefix, expectedEvent).Scan(&evidence); err != nil || evidence != 1 {
		return errors.New("exact fixture evidence hash mismatch")
	}
	var currentPath, oldTitle, newTitle string
	if err := db.QueryRow(`SELECT canonical_path FROM source_artifact_versions WHERE epoch_id='epoch-1' AND source_id='source-1' AND revision<=2 ORDER BY revision DESC LIMIT 1`).Scan(&currentPath); err != nil || currentPath != "/fake/archive/root.jsonl" {
		return errors.New("latest source selector mismatch")
	}
	if err := db.QueryRow(`SELECT title FROM session_label_versions WHERE epoch_id='epoch-1' AND session_id='child-001' AND revision<=1 ORDER BY revision DESC LIMIT 1`).Scan(&oldTitle); err != nil || oldTitle != "Old fake title" {
		return errors.New("revision-one label selector mismatch")
	}
	if err := db.QueryRow(`SELECT title FROM session_label_versions WHERE epoch_id='epoch-1' AND session_id='child-001' AND revision<=2 ORDER BY revision DESC LIMIT 1`).Scan(&newTitle); err != nil || newTitle != "New fake title" {
		return errors.New("revision-two label selector mismatch")
	}
	var rootOne, purposeOne, coverageOne, rootTwo, purposeTwo, coverageTwo, liveAvailability string
	if err := db.QueryRow(`SELECT root_work_unit_id,purpose FROM session_versions WHERE epoch_id='epoch-1' AND session_id='child-001' AND revision<=1 ORDER BY revision DESC LIMIT 1`).Scan(&rootOne, &purposeOne); err != nil {
		return errors.New("revision-one session selector mismatch")
	}
	if err := db.QueryRow(`SELECT root_work_unit_id,purpose FROM session_versions WHERE epoch_id='epoch-1' AND session_id='child-001' AND revision<=2 ORDER BY revision DESC LIMIT 1`).Scan(&rootTwo, &purposeTwo); err != nil {
		return errors.New("revision-two session selector mismatch")
	}
	if err := db.QueryRow(`SELECT fidelity FROM coverage_observation_versions WHERE epoch_id='epoch-1' AND scope_kind='session' AND scope_id='child-001' AND field_key='usage' AND revision<=1 ORDER BY revision DESC LIMIT 1`).Scan(&coverageOne); err != nil {
		return errors.New("revision-one coverage selector mismatch")
	}
	if err := db.QueryRow(`SELECT fidelity FROM coverage_observation_versions WHERE epoch_id='epoch-1' AND scope_kind='session' AND scope_id='child-001' AND field_key='usage' AND revision<=2 ORDER BY revision DESC LIMIT 1`).Scan(&coverageTwo); err != nil {
		return errors.New("revision-two coverage selector mismatch")
	}
	if rootOne != "child-001" || purposeOne != "other" || coverageOne != "exact" || rootTwo != "root-001" || purposeTwo != "spawned" || coverageTwo != "unavailable" {
		return errors.New("full revision golden mismatch")
	}
	if err := db.QueryRow(`SELECT availability FROM evidence_availability_versions WHERE epoch_id='epoch-1' AND evidence_id='evidence-1' ORDER BY observed_at DESC LIMIT 1`).Scan(&liveAvailability); err != nil || liveAvailability != "source_missing" {
		return errors.New("live availability overlay mismatch")
	}
	for revision, terms := range map[int][2]string{1: {"old", "new"}, 2: {"new", "old"}} {
		var matches int
		query := `SELECT count(*) FROM session_label_search s JOIN session_label_search_documents d ON d.rowid=s.rowid WHERE session_label_search MATCH ? AND d.label_revision=(SELECT max(revision) FROM session_label_versions WHERE epoch_id=d.epoch_id AND session_id=d.session_id AND revision<=?)`
		if err := db.QueryRow(query, terms[0], revision).Scan(&matches); err != nil || matches != 1 {
			return errors.New("FTS revision visibility mismatch")
		}
		if err := db.QueryRow(query, terms[1], revision).Scan(&matches); err != nil || matches != 0 {
			return errors.New("FTS superseded or future label leaked")
		}
	}
	var leaked sql.NullString
	if err := db.QueryRow(`SELECT content FROM event_search WHERE event_search MATCH 'payload'`).Scan(&leaked); err != nil || leaked.Valid {
		return errors.New("FTS contentlessness mismatch")
	}
	if err := db.QueryRow(`SELECT content FROM session_label_search WHERE session_label_search MATCH 'new'`).Scan(&leaked); err != nil || leaked.Valid {
		return errors.New("title FTS contentlessness mismatch")
	}
	eventDigest, eventDocuments, eventTokens, err := ftsIndexDigest(db, "event")
	if err != nil {
		return fmt.Errorf("event FTS digest/cardinality validation failed: %w", err)
	}
	if eventDocuments != 1 || eventTokens != 6 || eventDigest != "4117010684892565582f3aa00fc42d0a450eb01432aaae3fa83ecf445e1875f9" {
		return fmt.Errorf("event FTS digest/cardinality mismatch: documents=%d tokens=%d digest=%s", eventDocuments, eventTokens, eventDigest)
	}
	labelDigest, labelDocuments, labelTokens, err := ftsIndexDigest(db, "label")
	if err != nil {
		return fmt.Errorf("label FTS digest/cardinality validation failed: %w", err)
	}
	if labelDocuments != 2 || labelTokens != 2 || labelDigest != "e9b5767a6b2bfc737cd3e671efcdbbab6638224fa52ea1b1878f2ed8f0f2d5ca" {
		return fmt.Errorf("label FTS digest/cardinality mismatch: documents=%d tokens=%d digest=%s", labelDocuments, labelTokens, labelDigest)
	}
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return errors.New("integrity check failed")
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("foreign key check failed")
	}
	return nil
}

func ftsIndexDigest(db *sql.DB, index string) (string, int, int, error) {
	var create, count, query string
	switch index {
	case "event":
		create = `CREATE VIRTUAL TABLE temp.event_search_vocab USING fts5vocab(main,'event_search','instance')`
		count = `SELECT count(*) FROM event_search_documents`
		query = `SELECT v.doc,d.event_id,d.match_category,v.term,v.col,v.offset FROM event_search_vocab v LEFT JOIN event_search_documents d ON d.rowid=v.doc ORDER BY v.term,v.doc,v.col,v.offset`
	case "label":
		create = `CREATE VIRTUAL TABLE temp.session_label_search_vocab USING fts5vocab(main,'session_label_search','instance')`
		count = `SELECT count(*) FROM session_label_search_documents`
		query = `SELECT v.doc,d.epoch_id,d.session_id,d.label_revision,d.match_category,v.term,v.col,v.offset FROM session_label_search_vocab v LEFT JOIN session_label_search_documents d ON d.rowid=v.doc ORDER BY v.term,v.doc,v.col,v.offset`
	default:
		return "", 0, 0, errors.New("unknown FTS index")
	}
	if _, err := db.Exec(create); err != nil && !strings.Contains(err.Error(), "already exists") {
		return "", 0, 0, err
	}
	var documents int
	if err := db.QueryRow(count).Scan(&documents); err != nil {
		return "", 0, 0, err
	}
	rows, err := db.Query(query)
	if err != nil {
		return "", 0, 0, err
	}
	defer rows.Close()
	hash := sha256.New()
	tokens := 0
	for rows.Next() {
		var rowID, offset int
		var revision sql.NullInt64
		var a, b, c, term, column sql.NullString
		if index == "event" {
			if err := rows.Scan(&rowID, &a, &b, &term, &column, &offset); err != nil {
				return "", 0, 0, err
			}
			fmt.Fprintf(hash, "%d|%s|%s|%s|%s|%d\n", rowID, a.String, b.String, term.String, column.String, offset)
		} else {
			if err := rows.Scan(&rowID, &a, &b, &revision, &c, &term, &column, &offset); err != nil {
				return "", 0, 0, err
			}
			fmt.Fprintf(hash, "%d|%s|%s|%d|%s|%s|%s|%d\n", rowID, a.String, b.String, revision.Int64, c.String, term.String, column.String, offset)
		}
		tokens++
	}
	if err := rows.Err(); err != nil {
		return "", 0, 0, err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), documents, tokens, nil
}

func TestV2CandidateValidatorRejectsEveryCorruptionClass(t *testing.T) {
	cases := []struct {
		name, mutation, want string
	}{
		{"epoch_adapter", `DROP TRIGGER dataset_epoch_state_update; UPDATE dataset_epochs SET adapter_version='wrong' WHERE id='epoch-1'`, "epoch or adapter"},
		{"revision_sequence", `DROP TRIGGER index_revisions_monotonic_insert; INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES('epoch-1',4,'2026-07-18T00:04:00Z','append')`, "revision sequence"},
		{"cardinality", `INSERT INTO source_artifacts(id,epoch_id,source_session_id,segment_fingerprint,created_revision) VALUES('extra-source','epoch-1','extra','ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff',2)`, "source cardinality"},
		{"latest_selector", `DROP TRIGGER immutable_source_versions_u; UPDATE source_artifact_versions SET canonical_path='/corrupt' WHERE epoch_id='epoch-1' AND source_id='source-1' AND revision=2`, "latest source selector"},
		{"revision_golden", `DROP TRIGGER immutable_coverage_u; UPDATE coverage_observation_versions SET fidelity='derived' WHERE epoch_id='epoch-1' AND scope_kind='session' AND scope_id='child-001' AND revision=1`, "full revision golden"},
		{"fts_visibility", `DROP TRIGGER immutable_label_search_docs_d; DELETE FROM session_label_search_documents WHERE label_revision=2`, "FTS revision visibility"},
		{"event_fts_delete", `DELETE FROM event_search WHERE rowid=1`, "FTS"},
		{"event_fts_rogue_insert", `INSERT INTO event_search(rowid,content) VALUES(99,'rogue')`, "event FTS digest/cardinality"},
		{"label_fts_delete", `DELETE FROM session_label_search WHERE rowid=2`, "FTS"},
		{"label_fts_rogue_insert", `INSERT INTO session_label_search(rowid,content) VALUES(99,'rogue')`, "label FTS digest/cardinality"},
		{"evidence_hash", `DROP TRIGGER immutable_evidence_u; UPDATE evidence_refs SET event_fingerprint='ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff' WHERE id='evidence-1'`, "fixture evidence hash"},
		{"foreign_key", `PRAGMA foreign_keys=OFF; DROP TRIGGER immutable_messages_u; UPDATE messages SET epoch_id='missing-epoch' WHERE event_id='event-1'`, "foreign key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := openSchemaV2(t)
			seedRevisionScenario(t, db)
			catalog := filepath.Join(t.TempDir(), "active-index")
			if err := os.WriteFile(catalog, []byte("index-v1.sqlite\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(tc.mutation); err != nil {
				t.Fatalf("construct corrupt candidate: %v", err)
			}
			prefix, event := fixtureEvidenceHashes(t)
			err := validateV2Candidate(db, prefix, event)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("validator error=%v, want class %q", err, tc.want)
			}
			if got := readCatalog(t, catalog); got != "index-v1.sqlite" {
				t.Fatalf("failed %s validation changed active catalog to %q", tc.name, got)
			}
		})
	}
}

func TestFTSApplicationBoundaryAndWriter(t *testing.T) {
	for _, query := range []string{
		`UPDATE event_search SET content='rogue' WHERE rowid=1`,
		`DELETE FROM event_search WHERE rowid=1`,
		`INSERT INTO event_search(rowid,content) VALUES(99,'rogue')`,
		`REPLACE INTO session_label_search(rowid,content) VALUES(99,'rogue')`,
		`DELETE FROM event_search_data`,
	} {
		if !errors.Is(GuardApplicationSQL(query), ErrDirectFTSMutation) {
			t.Fatalf("application guard accepted direct FTS mutation: %s", query)
		}
	}
	if err := GuardApplicationSQL(`SELECT rowid FROM event_search WHERE event_search MATCH 'payload'`); err != nil {
		t.Fatalf("application guard rejected FTS read: %v", err)
	}
	db := openSchemaV2(t)
	seedRevisionScenario(t, db)
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	writer := NewFTSWriter(tx)
	if err := writer.InsertEvent(context.Background(), 3, "epoch-1", "event-2", "lineage", "controlled token"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO session_label_versions(epoch_id,session_id,revision,title,source_updated_at,source_record_ordinal) VALUES('epoch-1','root-001',2,'Root title','2026-07-18T00:02:00Z',2)`); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := writer.InsertSessionLabel(context.Background(), 3, "epoch-1", "root-001", 2, "root title"); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var matches int
	if err := db.QueryRow(`SELECT count(*) FROM event_search WHERE event_search MATCH 'controlled'`).Scan(&matches); err != nil || matches != 1 {
		t.Fatalf("controlled event FTS write matches=%d err=%v", matches, err)
	}
}

func activateCatalog(t *testing.T, catalog, databaseName string) {
	t.Helper()
	temporary := catalog + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(databaseName + "\n"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, catalog); err != nil {
		t.Fatal(err)
	}
	directory, err := os.Open(filepath.Dir(catalog))
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		t.Fatal(err)
	}
}

func readCatalog(t *testing.T, catalog string) string {
	t.Helper()
	data, err := os.ReadFile(catalog)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestSQLiteSchemaV2LatestQueryPlans(t *testing.T) {
	db := openSchemaV2(t)
	seedRevisionScenario(t, db)
	cases := []struct {
		name  string
		query string
		index string
	}{
		{"source", `EXPLAIN QUERY PLAN SELECT * FROM source_artifact_versions INDEXED BY source_artifact_versions_latest WHERE epoch_id='epoch-1' AND source_id='source-1' AND revision<=2 ORDER BY revision DESC LIMIT 1`, "source_artifact_versions_latest"},
		{"session", `EXPLAIN QUERY PLAN SELECT * FROM session_versions INDEXED BY session_versions_latest WHERE epoch_id='epoch-1' AND session_id='child-001' AND revision<=2 ORDER BY revision DESC LIMIT 1`, "session_versions_latest"},
		{"label", `EXPLAIN QUERY PLAN SELECT * FROM session_label_versions INDEXED BY session_label_versions_latest WHERE epoch_id='epoch-1' AND session_id='child-001' AND revision<=2 ORDER BY revision DESC LIMIT 1`, "session_label_versions_latest"},
		{"coverage", `EXPLAIN QUERY PLAN SELECT * FROM coverage_observation_versions INDEXED BY coverage_observation_versions_latest WHERE epoch_id='epoch-1' AND scope_kind='session' AND scope_id='child-001' AND field_key='usage' AND revision<=2 ORDER BY revision DESC LIMIT 1`, "coverage_observation_versions_latest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rows, err := db.Query(tc.query)
			if err != nil {
				t.Fatal(err)
			}
			defer rows.Close()
			var plan strings.Builder
			for rows.Next() {
				var id, parent, unused int
				var detail string
				if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
					t.Fatal(err)
				}
				plan.WriteString(detail)
			}
			if !strings.Contains(plan.String(), tc.index) {
				t.Fatalf("query plan does not use %s: %s", tc.index, plan.String())
			}
		})
	}
}

func TestSQLiteTemporalFutureParentsAndImmutabilityAudit(t *testing.T) {
	db := openSchemaV2(t)
	mustExecV2(t, db, `INSERT INTO dataset_epochs(id,schema_version,adapter_version,state,created_at) VALUES('building',2,?,'building','2026-07-18T00:00:00Z')`, AdapterVersion)
	mustExecV2(t, db, `INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES('building',1,'2026-07-18T00:01:00Z','inventory'),('building',2,'2026-07-18T00:02:00Z','append')`)
	mustExecV2(t, db, `INSERT INTO sessions(id,epoch_id,source_session_id,created_revision) VALUES('future-session','building','future-session',2)`)
	if _, err := db.Exec(`INSERT INTO session_versions(epoch_id,session_id,revision,root_work_unit_id,purpose,lineage_coverage,inventory_reconciled) VALUES('building','future-session',1,'future-session','user','exact',0)`); err == nil {
		t.Fatal("building epoch accepted a session version before parent creation")
	}
	mustExecV2(t, db, `INSERT INTO source_artifacts(id,epoch_id,source_session_id,segment_fingerprint,created_revision) VALUES('future-source','building','future-source',?,2)`, strings.Repeat("a", 64))
	if _, err := db.Exec(`INSERT INTO source_artifact_versions(epoch_id,source_id,revision,source_kind,canonical_path,byte_size,state,source_evidence_availability,availability_observed_at) VALUES('building','future-source',1,'active_rollout','/fake',0,'current','available','2026-07-18T00:01:00Z')`); err == nil {
		t.Fatal("building epoch accepted a source version before parent creation")
	}
	mustExecV2(t, db, `INSERT INTO projects(id,epoch_id,identity_kind,canonical_identity,display_name,created_revision) VALUES('future-project','building','cwd','/future','future',2)`)
	if _, err := db.Exec(`INSERT INTO project_aliases(epoch_id,project_id,alias_kind,alias_value,observed_revision) VALUES('building','future-project','cwd','/future',1)`); err == nil {
		t.Fatal("building epoch accepted an alias before parent creation")
	}
	if _, err := db.Exec(`INSERT INTO coverage_observation_versions(epoch_id,scope_kind,scope_id,field_key,revision,fidelity,observed_count,eligible_count) VALUES('building','session','future-session','usage',1,'unavailable',0,0)`); err == nil {
		t.Fatal("building epoch accepted coverage before scope creation")
	}
	mustExecV2(t, db, `INSERT INTO epoch_validations(epoch_id,validated_at,validation_sha256) VALUES('building','2026-07-18T00:02:30Z',?)`, strings.Repeat("9", 64))
	mustExecV2(t, db, `UPDATE dataset_epochs SET state='active',activated_at='2026-07-18T00:03:00Z' WHERE id='building'`)
	if _, err := db.Exec(`INSERT INTO sessions(id,epoch_id,source_session_id,created_revision) VALUES('historical','building','historical',1)`); err == nil {
		t.Fatal("active epoch accepted historical backfill")
	}
	immutableTables := []string{"index_revisions", "source_artifacts", "source_artifact_versions", "projects", "project_aliases", "sessions", "session_versions", "session_label_versions", "source_segments", "turns", "terminal_watermarks", "events", "lineage_edges", "messages", "tool_calls", "turn_usage", "capacity_observations", "compactions", "evidence_refs", "evidence_availability_versions", "coverage_observation_versions", "event_search_documents", "session_label_search_documents", "schema_metadata", "epoch_validations"}
	for _, table := range immutableTables {
		var triggers int
		if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='trigger' AND tbl_name=? AND name LIKE 'immutable_%'`, table).Scan(&triggers); err != nil {
			t.Fatal(err)
		}
		if triggers != 2 {
			t.Errorf("%s immutable trigger count=%d, want update+delete", table, triggers)
		}
	}
}

func TestCheckpointEveryChangeRequiresNewRevision(t *testing.T) {
	db := openSchemaV2(t)
	seedRevisionScenario(t, db)
	mutations := []string{
		`UPDATE source_checkpoints SET prefix_sha256='ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff' WHERE source_id='source-1'`,
		`UPDATE source_checkpoints SET adapter_version='wrong' WHERE source_id='source-1'`,
		`UPDATE source_checkpoints SET pending_tail_bytes=1 WHERE source_id='source-1'`,
		`UPDATE source_checkpoints SET observed_mtime_ns=99 WHERE source_id='source-1'`,
		`UPDATE source_checkpoints SET updated_at=updated_at WHERE source_id='source-1'`,
		`DELETE FROM source_checkpoints WHERE source_id='source-1'`,
	}
	for _, mutation := range mutations {
		if _, err := db.Exec(mutation); err == nil {
			t.Fatalf("checkpoint mutation without a new revision was accepted: %s", mutation)
		}
	}
	mustExecV2(t, db, `INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES('epoch-1',3,'2026-07-18T00:03:00Z','append')`)
	mustExecV2(t, db, `UPDATE source_checkpoints SET complete_byte_offset=120,complete_record_ordinal=4,observed_size=120,observed_mtime_ns=99,prefix_sha256=?,pending_tail_bytes=2,updated_at='2026-07-18T00:03:01Z',updated_revision=3 WHERE source_id='source-1'`, strings.Repeat("f", 64))
	var revision, offset, ordinal int
	if err := db.QueryRow(`SELECT updated_revision,complete_byte_offset,complete_record_ordinal FROM source_checkpoints WHERE source_id='source-1'`).Scan(&revision, &offset, &ordinal); err != nil || revision != 3 || offset != 120 || ordinal != 4 {
		t.Fatalf("valid checkpoint advance revision=%d offset=%d ordinal=%d err=%v", revision, offset, ordinal, err)
	}
}

func TestDatasetEpochTransitionsAreOneWay(t *testing.T) {
	db := openSchemaV2(t)
	mustExecV2(t, db, `INSERT INTO dataset_epochs(id,schema_version,adapter_version,state,created_at) VALUES('candidate',2,?,'building','2026-07-18T00:00:00Z'),('failure',2,?,'building','2026-07-18T00:00:00Z')`, AdapterVersion, AdapterVersion)
	for _, mutation := range []string{
		`UPDATE dataset_epochs SET activated_at='2026-07-18T00:00:01Z' WHERE id='candidate'`,
		`UPDATE dataset_epochs SET created_at='mutated' WHERE id='candidate'`,
		`UPDATE dataset_epochs SET state='superseded',activated_at='2026-07-18T00:00:01Z' WHERE id='candidate'`,
		`DELETE FROM dataset_epochs WHERE id='candidate'`,
	} {
		if _, err := db.Exec(mutation); err == nil {
			t.Fatalf("invalid building epoch mutation accepted: %s", mutation)
		}
	}
	mustExecV2(t, db, `UPDATE dataset_epochs SET state='failed' WHERE id='failure'`)
	if _, err := db.Exec(`UPDATE dataset_epochs SET state='active',activated_at='2026-07-18T00:01:00Z' WHERE id='candidate'`); err == nil {
		t.Fatal("building epoch activated without a successful validation marker")
	}
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := ActivateValidatedEpoch(context.Background(), tx, "candidate", "2026-07-18T00:00:30Z", "2026-07-18T00:01:00Z", strings.Repeat("9", 64)); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{
		`UPDATE dataset_epochs SET activated_at='2026-07-18T00:02:00Z' WHERE id='candidate'`,
		`UPDATE dataset_epochs SET state='building',activated_at=NULL WHERE id='candidate'`,
		`UPDATE dataset_epochs SET state='active' WHERE id='candidate'`,
	} {
		if _, err := db.Exec(mutation); err == nil {
			t.Fatalf("invalid active epoch mutation accepted: %s", mutation)
		}
	}
	mustExecV2(t, db, `UPDATE dataset_epochs SET state='superseded' WHERE id='candidate'`)
	if _, err := db.Exec(`UPDATE dataset_epochs SET state='active' WHERE id='candidate'`); err == nil {
		t.Fatal("superseded epoch reactivated")
	}
}

func TestSQLiteSchemaV2FactAndCheckpointRollbackTogether(t *testing.T) {
	db := openSchemaV2(t)
	seedRevisionScenario(t, db)
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = tx.Exec(`INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES('epoch-1',3,'2026-07-18T00:03:00Z','append')`); err == nil {
		_, err = tx.Exec(`INSERT INTO source_artifact_versions(epoch_id,source_id,revision,source_kind,canonical_path,byte_size,adapter_version,state,source_evidence_availability,availability_observed_at) VALUES('epoch-1','source-1',3,'archived_rollout','/fake/archive/root.jsonl',120,?,'current','available','2026-07-18T00:03:01Z')`, AdapterVersion)
	}
	if err == nil {
		_, err = tx.Exec(`UPDATE source_checkpoints SET complete_byte_offset=120,complete_record_ordinal=3,observed_size=120,updated_at='2026-07-18T00:03:01Z',updated_revision=3 WHERE source_id='source-1'`)
	}
	if err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if _, err := tx.Exec(`INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES('epoch-1',3,'2026-07-18T00:03:02Z','append')`); err == nil {
		_ = tx.Rollback()
		t.Fatal("expected duplicate revision to abort the candidate transaction")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var revisions, versions, checkpointRevision int
	if err := db.QueryRow(`SELECT count(*) FROM index_revisions WHERE epoch_id='epoch-1'`).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM source_artifact_versions WHERE epoch_id='epoch-1' AND source_id='source-1'`).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT updated_revision FROM source_checkpoints WHERE source_id='source-1'`).Scan(&checkpointRevision); err != nil {
		t.Fatal(err)
	}
	if revisions != 2 || versions != 2 || checkpointRevision != 2 {
		t.Fatalf("partial transaction escaped rollback: revisions=%d versions=%d checkpoint=%d", revisions, versions, checkpointRevision)
	}
}

func TestCrossSegmentResumeUsesPriorCumulativeBaseline(t *testing.T) {
	first := Usage{Input: 800, Cached: 200, Output: 300, Reasoning: 100, Total: 1200}
	second := Usage{Input: 1300, Cached: 400, Output: 500, Reasoning: 200, Total: 2000}
	sources := []SourceDecision{
		{Supported: true, SessionID: "resume-1", Turns: []Turn{{ID: "first", Completed: true, CompletedAt: "2026-07-18T00:00:00Z", Usage: &first, Cumulative: &first}}},
		{Supported: true, SessionID: "resume-1", Turns: []Turn{{ID: "second", Completed: true, CompletedAt: "2026-07-18T00:01:00Z", Cumulative: &second}}},
	}
	metrics, err := CalculateMetrics(sources)
	if err != nil {
		t.Fatal(err)
	}
	delta := metrics.RecordedTokens - first.Total
	var golden RevisionGolden
	loadJSON(t, repoPath("fixtures", "synthetic", "expected-revisions.json"), &golden)
	if delta != golden.CrossSegmentResumeTokens {
		t.Fatalf("cross-segment resumed delta=%d, want %d", delta, golden.CrossSegmentResumeTokens)
	}
}

func TestSourceOrderKeyIsMoveAndIngestionOrderIndependent(t *testing.T) {
	a := strings.Repeat("a", 64)
	b := strings.Repeat("b", 64)
	first, err := SourceOrderKey("2026-07-18T00:00:00Z", a)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := SourceOrderKey("2026-07-18T00:00:00Z", a)
	if err != nil || moved != first {
		t.Fatalf("move changed source order key: %q %v", moved, err)
	}
	resume, err := SourceOrderKey("2026-07-18T00:00:00Z", b)
	if err != nil {
		t.Fatal(err)
	}
	later, err := SourceOrderKey("2026-07-18T00:00:01Z", a)
	if err != nil {
		t.Fatal(err)
	}
	reverseIngested := []string{later, resume, first}
	sort.Strings(reverseIngested)
	if !reflect.DeepEqual(reverseIngested, []string{first, resume, later}) {
		t.Fatalf("semantic order depends on ingestion or equal-time ambiguity: %v", reverseIngested)
	}
}

func TestProjectDisplayNameDerivation(t *testing.T) {
	cases := []struct{ kind, identity, want string }{
		{"git_remote", "github.com/example/Codex-Inspector.git/", "Codex-Inspector"},
		{"git_root", "/work/example/repository/", "repository"},
		{"cwd", "/work/example/scratch", "scratch"},
		{"cwd", "/", "/"},
	}
	for _, tc := range cases {
		got, err := ProjectDisplayName(tc.kind, tc.identity)
		if err != nil || got != tc.want {
			t.Fatalf("ProjectDisplayName(%q,%q)=%q,%v want %q", tc.kind, tc.identity, got, err, tc.want)
		}
	}
}
