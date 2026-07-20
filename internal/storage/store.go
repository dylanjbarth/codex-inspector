package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dylanjbarth/codex-inspector/internal/facts"
	"github.com/dylanjbarth/codex-inspector/internal/phase0"
	"github.com/dylanjbarth/codex-inspector/internal/sources"
	_ "modernc.org/sqlite"
)

//go:embed migrations/001_schema.sql
var migrations embed.FS

const AdapterVersion = "rollout-jsonl/codex-recent-structural/v4"

type Store struct {
	db        *sql.DB
	path      string
	requested string
	catalog   string
}

var ErrSchemaV1RequiresRebuild = errors.New("schema v1 requires separate-file rebuild")
var validationTestHook func(*sql.DB)

const retainedPreviousSnapshots = 2

var immutableSnapshotName = regexp.MustCompile(`^index-v2-[0-9a-f]{24}\.sqlite$`)

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	catalog := filepath.Join(filepath.Dir(path), "active-index")
	actual, err := resolveCatalog(catalog)
	if err != nil {
		return nil, err
	}
	if actual == "" {
		if _, statErr := os.Stat(path); statErr == nil {
			version, versionErr := schemaVersionAt(path)
			if versionErr != nil || version != 2 {
				return nil, ErrSchemaV1RequiresRebuild
			}
			actual = path
			if err = activateCatalog(catalog, filepath.Base(actual)); err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(statErr) {
			return nil, statErr
		} else {
			actual = immutablePath(path)
			candidate, createErr := openBuilding(actual, path, catalog)
			if createErr != nil {
				return nil, createErr
			}
			expected, expectationErr := candidate.canonicalProjectionDigest(context.Background())
			if expectationErr != nil {
				candidate.Close()
				return nil, expectationErr
			}
			if createErr = candidate.validateAndActivate(context.Background(), expected); createErr != nil {
				candidate.Close()
				return nil, createErr
			}
			if createErr = candidate.Close(); createErr != nil {
				return nil, createErr
			}
			if createErr = activateCatalog(catalog, filepath.Base(actual)); createErr != nil {
				return nil, createErr
			}
		}
	}
	db, err := openDB(actual)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, path: actual, requested: path, catalog: catalog}
	if err = s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err = os.Chmod(actual, 0600); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func openDB(path string) (*sql.DB, error) {
	dsn := "file:" + filepath.ToSlash(path) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err == nil {
		db.SetMaxOpenConns(4)
		db.SetMaxIdleConns(4)
	}
	return db, err
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) migrate() error {
	var n int
	err := s.db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='schema_metadata'").Scan(&n)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrSchemaV1RequiresRebuild
	}
	var v int
	if e := s.db.QueryRow("SELECT schema_version FROM schema_metadata WHERE singleton=1").Scan(&v); e != nil || v != 2 {
		if e == nil && v == 1 {
			return ErrSchemaV1RequiresRebuild
		}
		return fmt.Errorf("unsupported index schema: %d: %w", v, e)
	}
	return nil
}
func openBuilding(path, requested, catalog string) (*Store, error) {
	db, err := openDB(path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db, path: path, requested: requested, catalog: catalog}
	b, err := migrations.ReadFile("migrations/001_schema.sql")
	if err != nil {
		db.Close()
		return nil, err
	}
	if _, err = db.Exec(string(b)); err != nil {
		db.Close()
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := "epoch:" + hash([]byte("schema:2:"+AdapterVersion+":"+now+":"+path))
	tx, err := s.db.Begin()
	if err != nil {
		db.Close()
		return nil, err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO dataset_epochs(id,schema_version,adapter_version,state,created_at) VALUES(?,2,?,'building',?)", id, AdapterVersion, now); err != nil {
		db.Close()
		return nil, err
	}
	if _, err = tx.Exec("INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES(?,1,?,'inventory')", id, now); err != nil {
		db.Close()
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		db.Close()
		return nil, err
	}
	_ = os.Chmod(path, 0600)
	return s, nil
}
func (s *Store) Snapshot() (string, int64, error) {
	var e string
	var r int64
	err := s.db.QueryRow("SELECT d.id,coalesce(max(r.revision),0) FROM dataset_epochs d LEFT JOIN index_revisions r ON r.epoch_id=d.id WHERE d.state IN ('active','building') ORDER BY CASE d.state WHEN 'active' THEN 0 ELSE 1 END LIMIT 1").Scan(&e, &r)
	return e, r, err
}

func schemaVersionAt(path string) (int, error) {
	db, err := openDB(path)
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var version int
	if err = db.QueryRow(`SELECT schema_version FROM schema_metadata WHERE singleton=1`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func resolveCatalog(catalog string) (string, error) {
	data, err := os.ReadFile(catalog)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(data))
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." {
		return "", errors.New("invalid active-index catalog")
	}
	return filepath.Join(filepath.Dir(catalog), name), nil
}

func immutablePath(requested string) string {
	stamp := fmt.Sprintf("%d", time.Now().UnixNano())
	return filepath.Join(filepath.Dir(requested), "index-v2-"+hash([]byte(requested + ":" + stamp))[:24]+".sqlite")
}

// PruneSnapshots bounds derived index storage while retaining two previous
// epochs for readers that were open across a recent catalog swap. Callers must
// hold the Inspector writer lock so an in-progress candidate cannot be removed.
func PruneSnapshots(requested string) error {
	dir := filepath.Dir(requested)
	active, err := resolveCatalog(filepath.Join(dir, "active-index"))
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	type snapshot struct {
		path    string
		modTime time.Time
	}
	var stale []snapshot
	for _, entry := range entries {
		if entry.IsDir() || !immutableSnapshotName.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if path == active {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		stale = append(stale, snapshot{path: path, modTime: info.ModTime()})
	}
	sort.Slice(stale, func(i, j int) bool { return stale[i].modTime.After(stale[j].modTime) })
	for _, candidate := range stale[retainedCount(len(stale)):] {
		if err = removeSQLiteFamily(candidate.path); err != nil {
			return err
		}
	}
	return nil
}

func retainedCount(count int) int {
	if count < retainedPreviousSnapshots {
		return count
	}
	return retainedPreviousSnapshots
}

func removeSQLiteFamily(path string) error {
	for _, suffix := range []string{"-wal", "-shm", ""} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func activateCatalog(catalog, databaseName string) error {
	if filepath.Base(databaseName) != databaseName || databaseName == "." || databaseName == ".." {
		return errors.New("catalog requires immutable database basename")
	}
	temporary := catalog + fmt.Sprintf(".tmp-%d", time.Now().UnixNano())
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	cleanup := func() { _ = file.Close(); _ = os.Remove(temporary) }
	if _, err = file.WriteString(databaseName + "\n"); err != nil {
		cleanup()
		return err
	}
	if err = file.Sync(); err != nil {
		cleanup()
		return err
	}
	if err = file.Close(); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err = os.Rename(temporary, catalog); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	directory, err := os.Open(filepath.Dir(catalog))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

var canonicalValidationTables = []string{
	"schema_metadata", "dataset_epochs", "epoch_validations", "index_revisions",
	"source_artifacts", "source_artifact_versions", "source_checkpoints", "projects", "project_aliases",
	"sessions", "session_versions", "session_label_versions", "source_segments", "turns", "terminal_watermarks",
	"events", "lineage_edges", "messages", "tool_calls", "turn_usage", "capacity_observations", "compactions",
	"evidence_refs", "evidence_availability_versions", "coverage_observation_versions",
	"event_search_documents", "session_label_search_documents",
}

func (s *Store) canonicalProjectionDigest(ctx context.Context) (string, error) {
	h := sha256.New()
	for _, table := range canonicalValidationTables {
		columns, err := tableColumns(ctx, s.db, table)
		if err != nil {
			return "", err
		}
		if len(columns) == 0 {
			return "", fmt.Errorf("candidate projection missing table %s", table)
		}
		query := `SELECT * FROM ` + table + ` ORDER BY ` + strings.Join(columns, ",")
		rows, err := s.db.QueryContext(ctx, query)
		if err != nil {
			return "", err
		}
		_, _ = io.WriteString(h, "table:"+table+"\n")
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err = rows.Scan(pointers...); err != nil {
				rows.Close()
				return "", err
			}
			for _, value := range values {
				writeCanonicalValue(h, value)
			}
			_, _ = io.WriteString(h, "\n")
		}
		if err = rows.Close(); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func tableColumns(ctx context.Context, db *sql.DB, table string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var cid, notnull, pk int
		var name, kind string
		var defaultValue any
		if err = rows.Scan(&cid, &name, &kind, &notnull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns = append(columns, name)
	}
	return columns, rows.Err()
}

func writeCanonicalValue(w io.Writer, value any) {
	if value == nil {
		_, _ = io.WriteString(w, "n;")
		return
	}
	var text string
	switch typed := value.(type) {
	case []byte:
		text = string(typed)
	case string:
		text = typed
	default:
		text = fmt.Sprint(typed)
	}
	_, _ = io.WriteString(w, fmt.Sprintf("%T:%d:%s;", value, len(text), text))
}

func (s *Store) validateSelectors(ctx context.Context, epoch string, applied int64) error {
	checks := []struct{ name, table, group, match string }{
		{"source", "source_artifact_versions", "source_id", "x.source_id=v.source_id"},
		{"session", "session_versions", "session_id", "x.session_id=v.session_id"},
		{"label", "session_label_versions", "session_id", "x.session_id=v.session_id"},
		{"coverage", "coverage_observation_versions", "scope_kind,scope_id,field_key", "x.scope_kind=v.scope_kind AND x.scope_id=v.scope_id AND x.field_key=v.field_key"},
	}
	for _, check := range checks {
		query := fmt.Sprintf(`SELECT count(*) FROM %s v WHERE v.epoch_id=? AND v.revision<=? AND v.revision=(SELECT max(x.revision) FROM %s x WHERE x.epoch_id=v.epoch_id AND %s AND x.revision<=?)`, check.table, check.table, check.match)
		var selected, eligible int
		if err := s.db.QueryRowContext(ctx, query, epoch, applied, applied).Scan(&selected); err != nil {
			return err
		}
		groupQuery := fmt.Sprintf(`SELECT count(*) FROM (SELECT %s FROM %s WHERE epoch_id=? AND revision<=? GROUP BY %s)`, check.group, check.table, check.group)
		if err := s.db.QueryRowContext(ctx, groupQuery, epoch, applied).Scan(&eligible); err != nil {
			return err
		}
		if selected != eligible {
			return fmt.Errorf("candidate %s latest-at-applied selector invalid", check.name)
		}
	}
	return nil
}

func (s *Store) expectedFTS(ctx context.Context, epoch string) (*sql.DB, error) {
	expected, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	expected.SetMaxOpenConns(1)
	ddl := `CREATE TABLE event_search_documents(rowid INTEGER PRIMARY KEY,epoch_id TEXT,event_id TEXT,match_category TEXT); CREATE VIRTUAL TABLE event_search USING fts5(content,content='',contentless_delete=1,tokenize='unicode61'); CREATE TABLE session_label_search_documents(rowid INTEGER PRIMARY KEY,epoch_id TEXT,session_id TEXT,label_revision INTEGER,match_category TEXT); CREATE VIRTUAL TABLE session_label_search USING fts5(content,content='',contentless_delete=1,tokenize='unicode61');`
	if _, err = expected.ExecContext(ctx, ddl); err != nil {
		expected.Close()
		return nil, err
	}
	texts := map[string]string{}
	rows, err := s.db.QueryContext(ctx, `SELECT v.canonical_path,v.source_kind,v.byte_size,coalesce(v.mtime_ns,0) FROM source_artifact_versions v WHERE v.epoch_id=? AND v.revision=(SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id)`, epoch)
	if err != nil {
		expected.Close()
		return nil, err
	}
	for rows.Next() {
		var candidate sources.Candidate
		if err = rows.Scan(&candidate.Path, &candidate.Kind, &candidate.Size, &candidate.MTimeNS); err != nil {
			rows.Close()
			expected.Close()
			return nil, err
		}
		if candidate.Kind == "session_index" {
			continue
		}
		batch, parseErr := sources.Parse(candidate, nil)
		if parseErr != nil {
			rows.Close()
			expected.Close()
			return nil, parseErr
		}
		for _, event := range batch.Events {
			if event.SearchText != "" {
				texts[scoped(epoch, event.ID)+"\x00"+event.SearchCategory] = event.SearchText
			}
		}
	}
	if err = rows.Close(); err != nil {
		expected.Close()
		return nil, err
	}
	tx, err := expected.BeginTx(ctx, nil)
	if err != nil {
		expected.Close()
		return nil, err
	}
	writer := phase0.NewFTSWriter(tx)
	rows, err = s.db.QueryContext(ctx, `SELECT rowid,event_id,match_category FROM event_search_documents ORDER BY rowid`)
	if err != nil {
		tx.Rollback()
		expected.Close()
		return nil, err
	}
	for rows.Next() {
		var rowid int64
		var eventID, category string
		if err = rows.Scan(&rowid, &eventID, &category); err != nil {
			rows.Close()
			tx.Rollback()
			expected.Close()
			return nil, err
		}
		text, ok := texts[eventID+"\x00"+category]
		if !ok {
			rows.Close()
			tx.Rollback()
			expected.Close()
			return nil, errors.New("candidate event FTS provenance has no source-derived text")
		}
		if err = writer.InsertEvent(ctx, rowid, epoch, eventID, category, text); err != nil {
			rows.Close()
			tx.Rollback()
			expected.Close()
			return nil, err
		}
	}
	if err = rows.Close(); err != nil {
		tx.Rollback()
		expected.Close()
		return nil, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT d.rowid,d.session_id,d.label_revision,v.title FROM session_label_search_documents d JOIN session_label_versions v ON v.epoch_id=d.epoch_id AND v.session_id=d.session_id AND v.revision=d.label_revision ORDER BY d.rowid`)
	if err != nil {
		tx.Rollback()
		expected.Close()
		return nil, err
	}
	for rows.Next() {
		var rowid, revision int64
		var sessionID, title string
		if err = rows.Scan(&rowid, &sessionID, &revision, &title); err != nil {
			rows.Close()
			tx.Rollback()
			expected.Close()
			return nil, err
		}
		if err = writer.InsertSessionLabel(ctx, rowid, epoch, sessionID, int(revision), title); err != nil {
			rows.Close()
			tx.Rollback()
			expected.Close()
			return nil, err
		}
	}
	if err = rows.Close(); err != nil {
		tx.Rollback()
		expected.Close()
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		expected.Close()
		return nil, err
	}
	return expected, nil
}

func ftsProjection(ctx context.Context, db *sql.DB, kind string) (int, string, error) {
	fts, docs, provenance := "event_search", "event_search_documents", `d.epoch_id,d.event_id,d.match_category`
	if kind == "label" {
		fts, docs, provenance = "session_label_search", "session_label_search_documents", `d.epoch_id,d.session_id,cast(d.label_revision AS TEXT),d.match_category`
	}
	vocab := kind + "_validation_vocab"
	_, _ = db.ExecContext(ctx, `DROP TABLE IF EXISTS temp.`+vocab)
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE temp.`+vocab+` USING fts5vocab(main,'`+fts+`','instance')`); err != nil {
		return 0, "", err
	}
	defer db.ExecContext(context.Background(), `DROP TABLE IF EXISTS temp.`+vocab)
	query := `SELECT v.term,v.col,v.offset,` + provenance + ` FROM temp.` + vocab + ` v LEFT JOIN ` + docs + ` d ON d.rowid=v.doc ORDER BY ` + provenance + `,v.term,v.col,v.offset`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	h, count := sha256.New(), 0
	columns, _ := rows.Columns()
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err = rows.Scan(pointers...); err != nil {
			return 0, "", err
		}
		for _, value := range values {
			writeCanonicalValue(h, value)
		}
		_, _ = io.WriteString(h, "\n")
		count++
	}
	return count, hex.EncodeToString(h.Sum(nil)), rows.Err()
}

func (s *Store) validateAndActivate(ctx context.Context, expectedProjection string) error {
	epoch, applied, err := s.Snapshot()
	if err != nil {
		return err
	}
	fail := func(cause error) error {
		_, _ = s.db.ExecContext(ctx, `UPDATE dataset_epochs SET state='failed' WHERE id=? AND state='building'`, epoch)
		return cause
	}
	var schemaVersion, epochCount int
	var adapter, state string
	if err = s.db.QueryRowContext(ctx, `SELECT schema_version FROM schema_metadata WHERE singleton=1`).Scan(&schemaVersion); err != nil || schemaVersion != 2 {
		return fail(fmt.Errorf("candidate schema metadata: %w", err))
	}
	if err = s.db.QueryRowContext(ctx, `SELECT count(*),coalesce(max(adapter_version),''),coalesce(max(state),'') FROM dataset_epochs`).Scan(&epochCount, &adapter, &state); err != nil || epochCount != 1 || adapter != AdapterVersion || state != "building" {
		return fail(errors.New("candidate epoch metadata invalid"))
	}
	rows, err := s.db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fail(err)
	}
	foreignViolation := rows.Next()
	_ = rows.Close()
	if foreignViolation {
		return fail(errors.New("candidate foreign key validation failed"))
	}
	var integrity string
	if err = s.db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return fail(errors.New("candidate integrity validation failed"))
	}
	var gaps int
	if err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM index_revisions r WHERE epoch_id=? AND revision<>(SELECT count(*) FROM index_revisions x WHERE x.epoch_id=r.epoch_id AND x.revision<=r.revision)`, epoch).Scan(&gaps); err != nil || gaps != 0 {
		return fail(errors.New("candidate revisions are not gap-free"))
	}
	if err = s.validateSelectors(ctx, epoch, applied); err != nil {
		return fail(err)
	}
	projection, projectionErr := s.canonicalProjectionDigest(ctx)
	if projectionErr != nil || projection != expectedProjection {
		return fail(errors.New("candidate identity/revision projection differs from candidate-derived expectation"))
	}
	expectedFTS, expectationErr := s.expectedFTS(ctx, epoch)
	if expectationErr != nil {
		return fail(expectationErr)
	}
	defer expectedFTS.Close()
	eventCount, eventDigest, projectionErr := ftsProjection(ctx, s.db, "event")
	if projectionErr != nil {
		return fail(projectionErr)
	}
	wantEventCount, wantEventDigest, projectionErr := ftsProjection(ctx, expectedFTS, "event")
	if projectionErr != nil {
		return fail(projectionErr)
	}
	labelCount, labelDigest, projectionErr := ftsProjection(ctx, s.db, "label")
	if projectionErr != nil {
		return fail(projectionErr)
	}
	wantLabelCount, wantLabelDigest, projectionErr := ftsProjection(ctx, expectedFTS, "label")
	if projectionErr != nil {
		return fail(projectionErr)
	}
	if eventCount != wantEventCount || eventDigest != wantEventDigest || labelCount != wantLabelCount || labelDigest != wantLabelDigest {
		return fail(errors.New("candidate FTS token/provenance projection differs from source-derived expectation"))
	}
	evidenceRows, err := s.db.QueryContext(ctx, `SELECT v.canonical_path,e.byte_start,e.byte_end,r.source_prefix_sha256,r.event_fingerprint FROM evidence_refs r JOIN events e ON e.epoch_id=r.epoch_id AND e.id=r.event_id JOIN source_artifact_versions v ON v.epoch_id=r.epoch_id AND v.source_id=r.source_id AND v.revision=(SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id)`)
	if err != nil {
		return fail(err)
	}
	for evidenceRows.Next() {
		var path, prefix, event string
		var start, end int64
		if err = evidenceRows.Scan(&path, &start, &end, &prefix, &event); err != nil {
			evidenceRows.Close()
			return fail(err)
		}
		file, openErr := os.Open(path)
		if openErr != nil {
			evidenceRows.Close()
			return fail(errors.New("candidate evidence source unavailable"))
		}
		prefixHash, eventHash := sha256.New(), sha256.New()
		_, prefixErr := io.Copy(prefixHash, io.NewSectionReader(file, 0, end))
		_, eventErr := io.Copy(eventHash, io.NewSectionReader(file, start, end-start))
		_ = file.Close()
		if prefixErr != nil || eventErr != nil || hex.EncodeToString(prefixHash.Sum(nil)) != prefix || hex.EncodeToString(eventHash.Sum(nil)) != event {
			evidenceRows.Close()
			return fail(errors.New("candidate evidence hash validation failed"))
		}
	}
	if err = evidenceRows.Close(); err != nil {
		return fail(err)
	}
	var events, evidence int
	if err = s.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM events WHERE adapter_version=? AND event_kind<>'lineage_observation'),(SELECT count(*) FROM evidence_refs)`, AdapterVersion).Scan(&events, &evidence); err != nil || events != evidence {
		return fail(errors.New("candidate identity/cardinality validation failed"))
	}
	digest := hash([]byte(fmt.Sprintf("projection=%s;event_tokens=%d:%s;label_tokens=%d:%s", projection, eventCount, eventDigest, labelCount, labelDigest)))
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fail(err)
	}
	if err = phase0.ActivateValidatedEpoch(ctx, tx, epoch, now, now, digest); err != nil {
		_ = tx.Rollback()
		return fail(err)
	}
	if err = tx.Commit(); err != nil {
		return fail(err)
	}
	_, err = s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	return err
}
func scoped(epoch, id string) string {
	if id == "" {
		return ""
	}
	return strings.Split(id, ":")[0] + ":" + hash([]byte(epoch+":"+id))
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func pointer[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

func (s *Store) Apply(ctx context.Context, b facts.Batch, reason string) (int64, error) {
	if err := validateBatch(b); err != nil {
		return 0, err
	}
	epoch, latest, err := s.Snapshot()
	if err != nil {
		return 0, err
	}
	rev := latest + 1
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	ftsWriter := phase0.NewFTSWriter(tx)
	insertLabelFTS := func(sessionID, title string) error {
		var rowid int64
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(rowid),0)+1 FROM session_label_search_documents`).Scan(&rowid); err != nil {
			return err
		}
		return ftsWriter.InsertSessionLabel(ctx, rowid, epoch, sessionID, int(rev), title)
	}
	reason = revisionReason(reason)
	if _, err = tx.ExecContext(ctx, "INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES(?,?,?,?)", epoch, rev, now, reason); err != nil {
		return 0, err
	}
	sourceID, sessionID, segmentID := scoped(epoch, b.Source.ID), "", ""
	if b.Session != nil {
		sessionID = scoped(epoch, b.Session.ID)
	}
	if b.Segment != nil {
		segmentID = scoped(epoch, b.Segment.ID)
	}
	_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO source_artifacts(id,epoch_id,source_session_id,segment_fingerprint,created_revision) VALUES(?,?,?,?,?)`, sourceID, epoch, b.Source.SessionID, b.Source.SegmentFingerprint, rev)
	if err != nil {
		return 0, err
	}
	state := b.Source.State
	if state == "supported" {
		state = map[bool]string{true: "indexing", false: "current"}[b.Source.PendingTail > 0]
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO source_artifact_versions(epoch_id,source_id,revision,source_kind,canonical_path,byte_size,mtime_ns,detected_codex_version,adapter_version,state,state_reason,source_evidence_availability,availability_observed_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, epoch, sourceID, rev, b.Source.Kind, b.Source.Path, b.Source.Size, b.Source.MTimeNS, nullable(b.Source.DetectedVersion), nullable(b.Source.AdapterVersion), state, nullable(b.Source.StateReason), "available", now)
	if err != nil {
		return 0, err
	}
	if b.Source.State == "unsupported" || b.Source.State == "failed" {
		_, err = tx.ExecContext(ctx, `INSERT INTO source_checkpoints(source_id,epoch_id,complete_byte_offset,complete_record_ordinal,observed_size,observed_mtime_ns,prefix_sha256,adapter_version,pending_tail_bytes,updated_at,updated_revision) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(source_id) DO UPDATE SET complete_byte_offset=excluded.complete_byte_offset,complete_record_ordinal=excluded.complete_record_ordinal,observed_size=excluded.observed_size,observed_mtime_ns=excluded.observed_mtime_ns,prefix_sha256=excluded.prefix_sha256,adapter_version=excluded.adapter_version,pending_tail_bytes=excluded.pending_tail_bytes,updated_at=excluded.updated_at,updated_revision=excluded.updated_revision`, sourceID, epoch, b.Source.CompleteOffset, b.Source.CompleteOrdinal, b.Source.Size, b.Source.MTimeNS, b.Source.PrefixSHA256, b.Source.AdapterVersion, b.Source.PendingTail, now, rev)
		if err != nil {
			return 0, err
		}
		return rev, tx.Commit()
	}
	for sourceSessionID, title := range b.SessionLabels {
		var labelSessionID string
		lookupErr := tx.QueryRowContext(ctx, `SELECT id FROM sessions WHERE epoch_id=? AND source_session_id=?`, epoch, sourceSessionID).Scan(&labelSessionID)
		if errors.Is(lookupErr, sql.ErrNoRows) {
			continue
		}
		if lookupErr != nil {
			return 0, lookupErr
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO session_label_versions(epoch_id,session_id,revision,title,source_updated_at,source_record_ordinal) VALUES(?,?,?,?,?,0)`, epoch, labelSessionID, rev, title, now)
		if err != nil {
			return 0, err
		}
		if err = insertLabelFTS(labelSessionID, title); err != nil {
			return 0, err
		}
	}
	if b.Project != nil {
		pid := scoped(epoch, b.Project.ID)
		_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO projects(id,epoch_id,identity_kind,canonical_identity,display_name,created_revision) VALUES(?,?,?,?,?,?)", pid, epoch, b.Project.Kind, b.Project.Identity, b.Project.DisplayName, rev)
		if err != nil {
			return 0, err
		}
		for k, v := range b.Project.Aliases {
			if v != "" {
				_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO project_aliases(epoch_id,project_id,alias_kind,alias_value,observed_revision) VALUES(?,?,?,?,?)", epoch, pid, k, v, rev)
				if err != nil {
					return 0, err
				}
			}
		}
	}
	if b.Session != nil {
		pid := scoped(epoch, b.Session.ProjectID)
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO sessions(id,epoch_id,source_session_id,created_revision) VALUES(?,?,?,?)`, sessionID, epoch, b.Session.SourceSessionID, rev)
		if err != nil {
			return 0, err
		}
		rootID, purpose, coverage := sessionID, b.Session.Purpose, b.Session.LineageCoverage
		if b.Session.RootWorkUnitID != "" && b.Session.RootWorkUnitID != b.Session.SourceSessionID {
			var proved string
			if tx.QueryRowContext(ctx, "SELECT id FROM sessions WHERE epoch_id=? AND source_session_id=?", epoch, b.Session.RootWorkUnitID).Scan(&proved) == nil {
				rootID = proved
			} else {
				purpose, coverage = "other", "partial"
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO session_versions(epoch_id,session_id,revision,project_id,root_work_unit_id,purpose,source,originator,earliest_proven_start,latest_terminal_end,lineage_coverage,inventory_reconciled) VALUES(?,?,?,?,?,?,?,?,?,?,?,0)`, epoch, sessionID, rev, nullable(pid), rootID, purpose, nullable(b.Session.Source), nullable(b.Session.Originator), nullable(b.Session.StartedAt), nullable(b.Session.EndedAt), coverage)
		if err != nil {
			return 0, err
		}
		if b.Session.Title != "" {
			_, err = tx.ExecContext(ctx, `INSERT INTO session_label_versions(epoch_id,session_id,revision,title,source_updated_at,source_record_ordinal) VALUES(?,?,?,?,?,0)`, epoch, sessionID, rev, b.Session.Title, now)
			if err != nil {
				return 0, err
			}
			if err = insertLabelFTS(sessionID, b.Session.Title); err != nil {
				return 0, err
			}
		}
	}
	if b.Segment != nil {
		orderKey := b.Segment.SourceOrderKey
		if orderKey == "" {
			orderKey, err = phase0.SourceOrderKey(b.Segment.StartedAt, b.Segment.Fingerprint)
			if err != nil {
				return 0, err
			}
		}
		_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO source_segments(id,epoch_id,session_id,source_id,segment_fingerprint,source_order_key,source_record_start,created_revision) VALUES(?,?,?,?,?,?,0,?)", segmentID, epoch, sessionID, sourceID, b.Segment.Fingerprint, orderKey, rev)
		if err != nil {
			return 0, err
		}
	}
	insertedTurns := map[string]bool{}
	for _, t := range b.Turns {
		tid := scoped(epoch, t.ID)
		turnOrder := t.SourceOrderKey
		if turnOrder == "" {
			segmentOrder := b.Segment.SourceOrderKey
			if segmentOrder == "" {
				segmentOrder, err = phase0.SourceOrderKey(b.Segment.StartedAt, b.Segment.Fingerprint)
				if err != nil {
					return 0, err
				}
			}
			turnOrder, err = phase0.TurnOrderKey(segmentOrder, t.Ordinal)
			if err != nil {
				return 0, err
			}
		}
		terminalAt := t.TerminalAt
		if terminalAt == "" {
			terminalAt = t.CompletedAt
		}
		res, insertErr := tx.ExecContext(ctx, `INSERT OR IGNORE INTO turns(id,epoch_id,session_id,source_turn_id,source_order_key,state,started_at,terminal_at,completed_at,model,reasoning_effort,commit_revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, tid, epoch, sessionID, t.SourceTurnID, turnOrder, t.State, nullable(t.StartedAt), terminalAt, nullable(t.CompletedAt), nullable(t.Model), nullable(t.ReasoningEffort), rev)
		err = insertErr
		if err != nil {
			return 0, err
		}
		inserted, _ := res.RowsAffected()
		if inserted > 0 {
			insertedTurns[t.ID] = true
		}
		if t.Usage != nil && inserted > 0 {
			fidelity := "exact"
			if t.Usage.Input == nil || t.Usage.CachedInput == nil || t.Usage.Output == nil || t.Usage.ReasoningOutput == nil || t.Usage.Total == nil {
				fidelity = "unavailable"
			}
			if t.NormalizationKind == "cumulative_delta" {
				fidelity = "derived"
			}
			_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO turn_usage(epoch_id,turn_id,formula_version,normalization_kind,input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,total_tokens,residual_tokens,fidelity) VALUES(?,?,1,?,?,?,?,?,?,NULL,?)", epoch, tid, t.NormalizationKind, pointer(t.Usage.Input), pointer(t.Usage.CachedInput), pointer(t.Usage.Output), pointer(t.Usage.ReasoningOutput), pointer(t.Usage.Total), fidelity)
			if err != nil {
				return 0, err
			}
		}
	}
	terminal := map[string]bool{}
	for _, t := range b.Turns {
		if insertedTurns[t.ID] {
			terminal[t.ID] = true
		}
	}
	var terminalEvent *facts.Event
	for i := range b.Events {
		if b.Events[i].Kind == "task_complete" && terminal[b.Events[i].TurnID] {
			terminalEvent = &b.Events[i]
		}
	}
	newEvents := map[string]bool{}
	insertEvent := func(e facts.Event, watermark string) error {
		// A resumed segment can repeat an already-committed logical turn. Its
		// immutable turn and events retain their original terminal revision;
		// repeated source records only advance this source's checkpoint.
		if e.TurnID != "" && !terminal[e.TurnID] {
			return nil
		}
		eid := scoped(epoch, e.ID)
		var exists int
		if er := tx.QueryRowContext(ctx, "SELECT count(*) FROM events WHERE epoch_id=? AND id=?", epoch, eid).Scan(&exists); er != nil {
			return er
		}
		if exists > 0 {
			return nil
		}
		tid := ""
		if e.TurnID != "" && terminal[e.TurnID] {
			tid = scoped(epoch, e.TurnID)
		}
		_, er := tx.ExecContext(ctx, "INSERT INTO events(id,epoch_id,segment_id,turn_id,record_ordinal,semantic_phase,event_kind,observed_at,byte_start,byte_end,content_sha256,payload_length,adapter_version,terminal_watermark_id,commit_revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", eid, epoch, segmentID, nullable(tid), e.RecordOrdinal, e.SemanticPhase, e.Kind, e.ObservedAt, e.ByteStart, e.ByteEnd, e.ContentSHA256, e.PayloadLength, e.AdapterVersion, nullable(watermark), rev)
		if er == nil {
			newEvents[e.ID] = true
		}
		return er
	}
	if terminalEvent != nil {
		if err = insertEvent(*terminalEvent, ""); err != nil {
			return 0, err
		}
	}
	watermarkID := ""
	if terminalEvent != nil && newEvents[terminalEvent.ID] {
		watermarkID = scoped(epoch, "watermark:"+hash([]byte(b.Source.ID+":"+terminalEvent.ID)))
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO terminal_watermarks(id,epoch_id,session_id,source_id,segment_id,terminal_event_id,terminal_revision,max_record_ordinal,observed_at) VALUES(?,?,?,?,?,?,?,?,?)`, watermarkID, epoch, sessionID, sourceID, segmentID, scoped(epoch, terminalEvent.ID), rev, terminalEvent.RecordOrdinal, terminalEvent.ObservedAt)
		if err != nil {
			return 0, err
		}
	}
	for _, e := range b.Events {
		if terminalEvent != nil && e.ID == terminalEvent.ID {
			continue
		}
		wm := ""
		if e.TurnID == "" {
			if watermarkID == "" || e.RecordOrdinal > terminalEvent.RecordOrdinal || !nullEventKind(e.Kind) {
				continue
			}
			wm = watermarkID
		}
		if err = insertEvent(e, wm); err != nil {
			return 0, err
		}
		if !newEvents[e.ID] {
			continue
		}
		eid := scoped(epoch, e.ID)
		if err != nil {
			return 0, err
		}
		if e.SearchText != "" {
			var rowid int64
			if er := tx.QueryRowContext(ctx, `SELECT coalesce(max(rowid),0)+1 FROM event_search_documents`).Scan(&rowid); er != nil {
				return 0, er
			}
			if er := ftsWriter.InsertEvent(ctx, rowid, epoch, eid, e.SearchCategory, e.SearchText); er != nil {
				return 0, er
			}
		}
	}
	for _, edge := range b.Lineage {
		if edge.SourceEventID == "" || !newEvents[edge.SourceEventID] {
			continue
		}
		parentID := scoped(epoch, edge.ParentSessionID)
		var exists int
		if tx.QueryRowContext(ctx, "SELECT count(*) FROM sessions WHERE epoch_id=? AND id=?", epoch, parentID).Scan(&exists) != nil || exists == 0 {
			continue
		}
		_, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO lineage_edges(epoch_id,parent_session_id,child_session_id,edge_kind,spawning_turn_id,source_event_id,commit_revision) VALUES(?,?,?,?,?,?,?)`, epoch, parentID, sessionID, edge.Kind, nullable(scoped(epoch, edge.SpawningTurnID)), scoped(epoch, edge.SourceEventID), rev)
		if err != nil {
			return 0, err
		}
	}
	for _, m := range b.Messages {
		if !newEvents[m.EventID] {
			continue
		}
		_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO messages(event_id,epoch_id,role,phase,source_message_id,readable,content_length,content_sha256) VALUES(?,?,?,?,?,?,?,?)", scoped(epoch, m.EventID), epoch, m.Role, m.Phase, nullable(m.SourceMessageID), boolInt(m.Readable), m.ContentLength, m.ContentSHA256)
		if err != nil {
			return 0, err
		}
	}
	for _, t := range b.Tools {
		if !newEvents[t.EventID] {
			continue
		}
		tid := scoped(epoch, t.TurnID)
		_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO tool_calls(id,epoch_id,session_id,turn_id,source_call_id,semantic_phase,event_id,tool_name,tool_family,status,exit_code,duration_ms) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", scoped(epoch, t.ID), epoch, sessionID, tid, t.SourceCallID, t.Phase, scoped(epoch, t.EventID), t.Name, t.Family, nullable(t.Status), pointer(t.ExitCode), pointer(t.DurationMS))
		if err != nil {
			return 0, err
		}
	}
	for _, c := range b.Capacity {
		if !newEvents[c.EventID] {
			continue
		}
		_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO capacity_observations(id,epoch_id,event_id,limit_id,window_minutes,used_percent,remaining_percent,resets_at,observed_at) VALUES(?,?,?,?,?,?,?,?,?)", scoped(epoch, c.ID), epoch, scoped(epoch, c.EventID), c.LimitID, pointer(c.WindowMinutes), pointer(c.UsedPercent), pointer(c.RemainingPercent), nullable(c.ResetsAt), c.ObservedAt)
		if err != nil {
			return 0, err
		}
	}
	for _, c := range b.Compactions {
		if !newEvents[c.EventID] {
			continue
		}
		tid := scoped(epoch, c.TurnID)
		_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO compactions(event_id,epoch_id,session_id,turn_id,trigger_kind,recorded_summary_length,recorded_summary_sha256) VALUES(?,?,?,?,?,?,?)", scoped(epoch, c.EventID), epoch, sessionID, nullable(tid), nullable(c.TriggerKind), pointer(c.SummaryLength), nullable(c.SummarySHA256))
		if err != nil {
			return 0, err
		}
	}
	for _, e := range b.Evidence {
		if !newEvents[e.EventID] {
			continue
		}
		evidenceID := scoped(epoch, e.ID)
		_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO evidence_refs(id,epoch_id,event_id,source_id,source_prefix_sha256,event_fingerprint) VALUES(?,?,?,?,?,?)", evidenceID, epoch, scoped(epoch, e.EventID), sourceID, e.SourceFingerprint, e.EventFingerprint)
		if err != nil {
			return 0, err
		}
		_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO evidence_availability_versions(epoch_id,evidence_id,observed_at,availability,availability_revision) VALUES(?,?,?,?,?)", epoch, evidenceID, now, e.Availability, rev)
		if err != nil {
			return 0, err
		}
	}
	for _, c := range b.Coverage {
		_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO coverage_observation_versions(epoch_id,scope_kind,scope_id,field_key,revision,fidelity,observed_count,eligible_count,reason) VALUES(?,?,?,?,?,?,?,?,?)", epoch, c.ScopeKind, scoped(epoch, c.ScopeID), c.FieldKey, rev, c.Fidelity, c.Observed, c.Eligible, nullable(c.Reason))
		if err != nil {
			return 0, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO source_checkpoints(source_id,epoch_id,complete_byte_offset,complete_record_ordinal,observed_size,observed_mtime_ns,prefix_sha256,adapter_version,pending_tail_bytes,updated_at,updated_revision) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(source_id) DO UPDATE SET complete_byte_offset=excluded.complete_byte_offset,complete_record_ordinal=excluded.complete_record_ordinal,observed_size=excluded.observed_size,observed_mtime_ns=excluded.observed_mtime_ns,prefix_sha256=excluded.prefix_sha256,adapter_version=excluded.adapter_version,pending_tail_bytes=excluded.pending_tail_bytes,updated_at=excluded.updated_at,updated_revision=excluded.updated_revision`, sourceID, epoch, b.Source.CompleteOffset, b.Source.CompleteOrdinal, b.Source.Size, b.Source.MTimeNS, b.Source.PrefixSHA256, b.Source.AdapterVersion, b.Source.PendingTail, now, rev)
	if err != nil {
		return 0, err
	}
	if failpointMatches("apply_before_commit", b.Source.SessionID) {
		os.Exit(86)
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	if failpointMatches("apply_after_commit", b.Source.SessionID) {
		os.Exit(87)
	}
	return rev, nil
}

func failpointMatches(name, sourceSessionID string) bool {
	if os.Getenv("CODEX_INSPECTOR_FAILPOINT") != name {
		return false
	}
	target := os.Getenv("CODEX_INSPECTOR_FAILPOINT_SESSION")
	return target == "" || target == sourceSessionID
}

func revisionReason(reason string) string {
	switch reason {
	case "inventory", "normalize", "append", "source_move", "label", "lineage", "availability", "reconcile", "rebuild":
		return reason
	case "session_index_labels":
		return "label"
	case "rebuild_source":
		return "rebuild"
	default:
		return "normalize"
	}
}
func nullEventKind(kind string) bool {
	switch kind {
	case "session_meta", "source_boundary", "capacity_observation", "compaction_boundary", "lineage_observation":
		return true
	}
	return false
}
func turnOrdinal(t facts.Turn) int64 {
	stamp := t.StartedAt
	if stamp == "" {
		stamp = t.CompletedAt
	}
	parsed, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return int64(t.Ordinal)
	}
	return parsed.UnixMilli()*1000 + int64(t.Ordinal%1000)
}
func validateBatch(b facts.Batch) error {
	if b.Source.ID == "" || b.Source.Path == "" || b.Source.SessionID == "" || b.Source.SegmentFingerprint == "" {
		return errors.New("normalized source identity is incomplete")
	}
	if b.Source.Kind != "active_rollout" && b.Source.Kind != "archived_rollout" && b.Source.Kind != "session_index" {
		return errors.New("normalized source kind is invalid")
	}
	if (b.Session == nil) != (b.Segment == nil) {
		return errors.New("normalized session and segment must be present together")
	}
	return nil
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func hash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

type Checkpoint struct {
	Size, Offset, MTimeNS int64
	Prefix, Path, Kind    string
	State, AdapterVersion string
	SourceSessionID       string
}

func (s *Store) Checkpoint(sourceStableID string) (Checkpoint, error) {
	epoch, _, e := s.Snapshot()
	if e != nil {
		return Checkpoint{}, e
	}
	var c Checkpoint
	e = s.db.QueryRow(`SELECT c.observed_size,c.complete_byte_offset,c.observed_mtime_ns,c.prefix_sha256,v.canonical_path,v.source_kind,v.state,c.adapter_version,a.source_session_id FROM source_checkpoints c JOIN source_artifacts a ON a.epoch_id=c.epoch_id AND a.id=c.source_id JOIN source_artifact_versions v ON v.epoch_id=c.epoch_id AND v.source_id=c.source_id WHERE c.source_id=? ORDER BY v.revision DESC LIMIT 1`, scoped(epoch, sourceStableID)).Scan(&c.Size, &c.Offset, &c.MTimeNS, &c.Prefix, &c.Path, &c.Kind, &c.State, &c.AdapterVersion, &c.SourceSessionID)
	return c, e
}

// CheckpointsByPath supports the indexer's metadata-only startup reconciliation.
// It selects only the latest version of each source in the active epoch so old
// archived paths cannot make moved or replaced sources look unchanged.
func (s *Store) CheckpointsByPath() (map[string]Checkpoint, error) {
	epoch, _, e := s.Snapshot()
	if e != nil {
		return nil, e
	}
	rows, e := s.db.Query(`SELECT c.observed_size,c.complete_byte_offset,c.observed_mtime_ns,c.prefix_sha256,v.canonical_path,v.source_kind,v.state,c.adapter_version,a.source_session_id
		FROM source_artifact_versions v
		JOIN source_checkpoints c ON c.epoch_id=v.epoch_id AND c.source_id=v.source_id
		JOIN source_artifacts a ON a.epoch_id=v.epoch_id AND a.id=v.source_id
		WHERE v.epoch_id=?
		AND v.revision=(SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id)`, epoch)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	checkpoints := map[string]Checkpoint{}
	for rows.Next() {
		var checkpoint Checkpoint
		if e = rows.Scan(&checkpoint.Size, &checkpoint.Offset, &checkpoint.MTimeNS, &checkpoint.Prefix, &checkpoint.Path, &checkpoint.Kind, &checkpoint.State, &checkpoint.AdapterVersion, &checkpoint.SourceSessionID); e != nil {
			return nil, e
		}
		checkpoints[checkpoint.Path] = checkpoint
	}
	return checkpoints, rows.Err()
}
func (s *Store) HasOtherSessionSegment(sourceSessionID, sourceStableID string) bool {
	epoch, _, err := s.Snapshot()
	if err != nil {
		return false
	}
	var count int
	return s.db.QueryRow(`SELECT count(*) FROM source_artifacts WHERE epoch_id=? AND source_session_id=? AND id<>?`, epoch, sourceSessionID, scoped(epoch, sourceStableID)).Scan(&count) == nil && count > 0
}
func (s *Store) MarkRequiresRebuild(sourceStableID, reason string) error {
	epoch, latest, e := s.Snapshot()
	if e != nil {
		return e
	}
	tx, e := s.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	rev := latest + 1
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, e = tx.Exec("INSERT INTO index_revisions VALUES(?,?,?,?)", epoch, rev, now, "reconcile"); e != nil {
		return e
	}
	if _, e = tx.Exec(`INSERT INTO source_artifact_versions(epoch_id,source_id,revision,source_kind,canonical_path,inode,byte_size,mtime_ns,detected_codex_version,adapter_version,state,state_reason,source_evidence_availability,availability_observed_at)
		SELECT epoch_id,source_id,?,source_kind,canonical_path,inode,byte_size,mtime_ns,detected_codex_version,adapter_version,'requires_rebuild',?,source_evidence_availability,?
		FROM source_artifact_versions WHERE epoch_id=? AND source_id=? ORDER BY revision DESC LIMIT 1`, rev, reason, now, epoch, scoped(epoch, sourceStableID)); e != nil {
		return e
	}
	return tx.Commit()
}

// ReconcileLineage resolves reverse-scan children after inventory completion
// and classifies still-unresolved spawned sessions as explicit orphans.
func (s *Store) ReconcileLineage(ctx context.Context) error {
	epoch, latest, err := s.Snapshot()
	if err != nil {
		return err
	}
	type candidate struct {
		child, parent, kind, project, source, originator, start, end, parentRoot, parentPurpose, proofEvent, segment, watermark, observed, content, adapter, spawningSourceTurn string
		ordinal                                                                                                                                                                 int
		byteStart, byteEnd, payload                                                                                                                                             int64
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id,coalesce(p.id,''),substr(cv.reason,1,instr(cv.reason,':')-1),coalesce(sv.project_id,''),coalesce(sv.source,''),coalesce(sv.originator,''),coalesce(sv.earliest_proven_start,''),coalesce(sv.latest_terminal_end,''),coalesce(pv.root_work_unit_id,''),coalesce(pv.purpose,''),coalesce(e.id,''),coalesce(e.segment_id,''),coalesce(e.terminal_watermark_id,''),coalesce(e.observed_at,''),coalesce(e.content_sha256,''),coalesce(e.adapter_version,''),coalesce((SELECT sc.reason FROM coverage_observation_versions sc WHERE sc.epoch_id=cv.epoch_id AND sc.scope_kind='session' AND sc.scope_id=c.id AND sc.field_key='spawning_turn_reference' ORDER BY sc.revision DESC LIMIT 1),''),coalesce(e.record_ordinal,0),coalesce(e.byte_start,0),coalesce(e.byte_end,0),coalesce(e.payload_length,0)
		FROM coverage_observation_versions cv JOIN sessions c ON c.epoch_id=cv.epoch_id AND c.id=cv.scope_id
		JOIN session_versions sv ON sv.epoch_id=c.epoch_id AND sv.session_id=c.id AND sv.revision=(SELECT max(x.revision) FROM session_versions x WHERE x.epoch_id=sv.epoch_id AND x.session_id=sv.session_id AND x.revision<=?)
		LEFT JOIN sessions p ON p.epoch_id=c.epoch_id AND p.source_session_id=substr(cv.reason,instr(cv.reason,':')+1)
		LEFT JOIN session_versions pv ON pv.epoch_id=p.epoch_id AND pv.session_id=p.id AND pv.revision=(SELECT max(x.revision) FROM session_versions x WHERE x.epoch_id=pv.epoch_id AND x.session_id=pv.session_id AND x.revision<=?)
		LEFT JOIN source_segments sg ON sg.epoch_id=c.epoch_id AND sg.session_id=c.id
		LEFT JOIN events e ON e.epoch_id=sg.epoch_id AND e.segment_id=sg.id AND e.event_kind='lineage_observation'
		WHERE cv.epoch_id=? AND cv.scope_kind='session' AND cv.field_key='lineage_reference' AND instr(cv.reason,':')>1
		AND cv.revision=(SELECT max(x.revision) FROM coverage_observation_versions x WHERE x.epoch_id=cv.epoch_id AND x.scope_kind=cv.scope_kind AND x.scope_id=cv.scope_id AND x.field_key=cv.field_key AND x.revision<=?)
		AND NOT EXISTS(SELECT 1 FROM lineage_edges l WHERE l.epoch_id=cv.epoch_id AND l.child_session_id=c.id) GROUP BY c.id`, latest, latest, epoch, latest)
	if err != nil {
		return err
	}
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.child, &c.parent, &c.kind, &c.project, &c.source, &c.originator, &c.start, &c.end, &c.parentRoot, &c.parentPurpose, &c.proofEvent, &c.segment, &c.watermark, &c.observed, &c.content, &c.adapter, &c.spawningSourceTurn, &c.ordinal, &c.byteStart, &c.byteEnd, &c.payload); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, c)
	}
	if err = rows.Close(); err != nil {
		return err
	}
	if len(candidates) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rev, now := latest+1, time.Now().UTC().Format(time.RFC3339Nano)
	if _, err = tx.ExecContext(ctx, "INSERT INTO index_revisions(epoch_id,revision,committed_at,reason) VALUES(?,?,?,'reconcile')", epoch, rev, now); err != nil {
		return err
	}
	for _, c := range candidates {
		root, purpose, coverage := c.child, "orphan", "partial"
		if c.parent != "" {
			root, purpose, coverage = c.parentRoot, "spawned", "exact"
			if c.parentPurpose == "inspector_review" {
				purpose = "inspector_review"
			}
			if c.kind == "forked_from" {
				root, purpose = c.child, "user"
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO session_versions(epoch_id,session_id,revision,project_id,root_work_unit_id,purpose,source,originator,earliest_proven_start,latest_terminal_end,lineage_coverage,inventory_reconciled) VALUES(?,?,?,?,?,?,?,?,?,?,?,1)`, epoch, c.child, rev, nullable(c.project), root, purpose, nullable(c.source), nullable(c.originator), nullable(c.start), nullable(c.end), coverage)
		if err != nil {
			return err
		}
		if c.parent == "" || c.proofEvent == "" || c.watermark == "" {
			continue
		}
		proofID := "event:" + hash([]byte(epoch+":"+c.proofEvent+":reconcile"))
		_, err = tx.ExecContext(ctx, `INSERT INTO events(id,epoch_id,segment_id,record_ordinal,semantic_phase,event_kind,observed_at,byte_start,byte_end,content_sha256,payload_length,adapter_version,terminal_watermark_id,commit_revision) VALUES(?,?,?,?, 'lineage_reconcile','lineage_observation',?,?,?,?,?,?,?,?)`, proofID, epoch, c.segment, c.ordinal, c.observed, c.byteStart, c.byteEnd, c.content, c.payload, c.adapter, c.watermark, rev)
		if err != nil {
			return err
		}
		var spawningTurn any
		if c.spawningSourceTurn != "" {
			var turnID string
			if tx.QueryRowContext(ctx, `SELECT id FROM turns WHERE epoch_id=? AND session_id=? AND source_turn_id=?`, epoch, c.parent, c.spawningSourceTurn).Scan(&turnID) == nil {
				spawningTurn = turnID
			}
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO lineage_edges(epoch_id,parent_session_id,child_session_id,edge_kind,spawning_turn_id,source_event_id,commit_revision) VALUES(?,?,?,?,?,?,?)`, epoch, c.parent, c.child, c.kind, spawningTurn, proofID, rev)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

type Status struct {
	Epoch                                                                                         string
	Revision                                                                                      int64
	DatabaseBytes                                                                                 int64
	Sources, Supported, Unsupported, Pending, Processed, Queued, Skipped, Failed, RequiresRebuild int
	Boundary, Watermark                                                                           *string
}

type SourceDiagnosticGroup struct {
	State, Reason, DetectedVersion string
	Count                          int
}

const MaxSourceDiagnosticGroups = 50

var diagnosticVersionPattern = regexp.MustCompile(`^[0-9]{1,5}\.[0-9]{1,5}\.[0-9]{1,5}(?:[-+][0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)

var diagnosticReasonCodes = map[string]bool{
	"leading_record_too_large":          true,
	"missing_leading_session_meta":      true,
	"invalid_leading_session_meta":      true,
	"invalid_session_meta":              true,
	"invalid_session_meta_timestamp":    true,
	"missing_required_session_identity": true,
	"unsupported_codex_version":         true,
	"incompatible_turn_context":         true,
	"incompatible_record_envelope":      true,
	"incompatible_event_record":         true,
	"incompatible_turn_identity":        true,
	"incompatible_token_record":         true,
	"incompatible_response_record":      true,
	"incompatible_tool_identity":        true,
	"incompatible_record_shape":         true,
	"foreign_key_constraint":            true,
	"identity_constraint":               true,
	"indexed_prefix_changed_or_shrank":  true,
	"fact_check_constraint":             true,
	"storage_busy":                      true,
	"normalization_failed":              true,
	"parse_failed":                      true,
	"rebuild_failed":                    true,
	"unspecified":                       true,
}

func normalizeDiagnosticReason(value string) string {
	value = strings.TrimSpace(value)
	if diagnosticReasonCodes[value] {
		return value
	}
	return "unclassified_source_state"
}

func normalizeDiagnosticVersion(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 64 && diagnosticVersionPattern.MatchString(value) {
		return value
	}
	return "unknown"
}

// SourceDiagnosticGroups returns payload-free cohorts from the latest version
// of each source artifact. Paths and source content are deliberately excluded.
func (s *Store) SourceDiagnosticGroups(ctx context.Context) ([]SourceDiagnosticGroup, error) {
	epoch, _, err := s.Snapshot()
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT v.state,substr(coalesce(v.state_reason,'unspecified'),1,65),substr(coalesce(v.detected_codex_version,'unknown'),1,65),count(*)
		FROM source_artifact_versions v
		WHERE v.epoch_id=? AND v.state IN ('unsupported','failed','requires_rebuild')
		AND v.revision=(SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id)
		GROUP BY v.state,substr(coalesce(v.state_reason,'unspecified'),1,65),substr(coalesce(v.detected_codex_version,'unknown'),1,65)
		ORDER BY v.state,count(*) DESC,coalesce(v.state_reason,'unspecified'),coalesce(v.detected_codex_version,'unknown')`, epoch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	groups := make([]SourceDiagnosticGroup, 0, MaxSourceDiagnosticGroups)
	groupIndexes := map[string]int{}
	overflowCount := 0
	for rows.Next() {
		var group SourceDiagnosticGroup
		if err = rows.Scan(&group.State, &group.Reason, &group.DetectedVersion, &group.Count); err != nil {
			return nil, err
		}
		group.Reason = normalizeDiagnosticReason(group.Reason)
		group.DetectedVersion = normalizeDiagnosticVersion(group.DetectedVersion)
		key := group.State + "\x00" + group.Reason + "\x00" + group.DetectedVersion
		if index, ok := groupIndexes[key]; ok {
			groups[index].Count += group.Count
			continue
		}
		if len(groups) < MaxSourceDiagnosticGroups-1 {
			groupIndexes[key] = len(groups)
			groups = append(groups, group)
			continue
		}
		overflowCount += group.Count
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if overflowCount > 0 {
		groups = append(groups, SourceDiagnosticGroup{State: "multiple", Reason: "additional_diagnostic_groups", DetectedVersion: "multiple", Count: overflowCount})
	}
	return groups, nil
}

type SourceInventory struct {
	Total, Current, Discovered, Supported, Indexing, Unsupported, Failed, RequiresRebuild, Missing int
}

func (s *Store) SourceInventoryAt(ctx context.Context, epoch string, revision int64, sessionIDs []string) (SourceInventory, error) {
	query := `SELECT v.state,count(*)
 FROM source_artifact_versions v
 WHERE v.epoch_id=? AND v.source_kind<>'session_index'
 AND v.revision=(SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id AND x.revision<=?)
	`
	args := []any{epoch, revision}
	if len(sessionIDs) > 0 {
		query += ` AND EXISTS (SELECT 1 FROM source_artifacts a JOIN sessions s ON s.epoch_id=a.epoch_id AND s.source_session_id=a.source_session_id WHERE a.epoch_id=v.epoch_id AND a.id=v.source_id AND s.id IN (` + strings.TrimRight(strings.Repeat("?,", len(sessionIDs)), ",") + `))`
		for _, id := range sessionIDs {
			args = append(args, id)
		}
	}
	query += ` GROUP BY v.state`
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return SourceInventory{}, err
	}
	defer rows.Close()
	var out SourceInventory
	for rows.Next() {
		var state string
		var count int
		if err = rows.Scan(&state, &count); err != nil {
			return SourceInventory{}, err
		}
		out.Total += count
		switch state {
		case "current":
			out.Current = count
		case "discovered":
			out.Discovered = count
		case "supported":
			out.Supported = count
		case "indexing":
			out.Indexing = count
		case "unsupported":
			out.Unsupported = count
		case "failed":
			out.Failed = count
		case "requires_rebuild":
			out.RequiresRebuild = count
		case "missing":
			out.Missing = count
		}
	}
	return out, rows.Err()
}

func (s *Store) Status() (Status, error) {
	epoch, rev, e := s.Snapshot()
	if e != nil {
		return Status{}, e
	}
	st := Status{Epoch: epoch, Revision: rev}
	_ = s.db.QueryRow(`SELECT count(*),coalesce(sum(state NOT IN ('unsupported','failed')),0),coalesce(sum(state='unsupported'),0),coalesce(sum(state='indexing'),0),coalesce(sum(state='current'),0),coalesce(sum(state='discovered'),0),coalesce(sum(state='failed'),0),coalesce(sum(state='requires_rebuild'),0) FROM source_artifact_versions v WHERE epoch_id=? AND revision=(SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id)`, epoch).Scan(&st.Sources, &st.Supported, &st.Unsupported, &st.Pending, &st.Processed, &st.Queued, &st.Failed, &st.RequiresRebuild)
	if info, e := os.Stat(s.path); e == nil {
		st.DatabaseBytes = info.Size()
	}
	var w sql.NullString
	_ = s.db.QueryRow("SELECT max(completed_at) FROM turns WHERE epoch_id=? AND state='completed'", epoch).Scan(&w)
	if w.Valid {
		st.Watermark = &w.String
	}
	return st, nil
}

type SessionSummary struct {
	ID, SourceSessionID, RootWorkUnitID, Purpose, Title, StartedAt string
	CompletedTurns                                                 int
}

// Rebuild creates and validates a complete replacement before atomically
// swapping its database file. Existing readers retain the old inode/snapshot;
// subsequent readers see only the replacement epoch.
func (s *Store) Rebuild(ctx context.Context, batches []facts.Batch) error {
	oldEpoch, _, err := s.Snapshot()
	if err != nil {
		return err
	}
	candidatePath := immutablePath(s.requested)
	replacement, err := openBuilding(candidatePath, s.requested, s.catalog)
	if err != nil {
		return err
	}
	keepCandidate := false
	defer func() {
		_ = replacement.Close()
		if !keepCandidate {
			_ = removeSQLiteFamily(candidatePath)
		}
	}()
	for _, batch := range batches {
		if _, err = replacement.Apply(ctx, batch, "rebuild_source"); err != nil {
			_, _ = replacement.db.ExecContext(ctx, `UPDATE dataset_epochs SET state='failed' WHERE state='building'`)
			return err
		}
	}
	if err = replacement.ReconcileLineage(ctx); err != nil {
		return err
	}
	expected, err := replacement.canonicalProjectionDigest(ctx)
	if err != nil {
		return err
	}
	if validationTestHook != nil {
		validationTestHook(replacement.db)
	}
	if err = replacement.validateAndActivate(ctx, expected); err != nil {
		return err
	}
	if _, err = replacement.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return err
	}
	if err = activateCatalog(s.catalog, filepath.Base(candidatePath)); err != nil {
		return err
	}
	keepCandidate = true
	// This transition happens only after the durable pointer swap. Existing
	// read transactions retain their SQLite snapshot while new opens resolve
	// the immutable replacement filename through active-index.
	_, _ = s.db.ExecContext(ctx, `UPDATE dataset_epochs SET state='superseded' WHERE id=? AND state='active'`, oldEpoch)
	return PruneSnapshots(s.requested)
}

func BuildAndActivate(ctx context.Context, requested string, batches []facts.Batch) error {
	catalog := filepath.Join(filepath.Dir(requested), "active-index")
	candidatePath := immutablePath(requested)
	candidate, err := openBuilding(candidatePath, requested, catalog)
	if err != nil {
		return err
	}
	keepCandidate := false
	defer func() {
		_ = candidate.Close()
		if !keepCandidate {
			_ = removeSQLiteFamily(candidatePath)
		}
	}()
	for _, batch := range batches {
		if _, err = candidate.Apply(ctx, batch, "rebuild_source"); err != nil {
			_, _ = candidate.db.ExecContext(ctx, `UPDATE dataset_epochs SET state='failed' WHERE state='building'`)
			return err
		}
	}
	if err = candidate.ReconcileLineage(ctx); err != nil {
		return err
	}
	expected, err := candidate.canonicalProjectionDigest(ctx)
	if err != nil {
		return err
	}
	if validationTestHook != nil {
		validationTestHook(candidate.db)
	}
	if err = candidate.validateAndActivate(ctx, expected); err != nil {
		return err
	}
	if _, err = candidate.db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return err
	}
	if err = activateCatalog(catalog, filepath.Base(candidatePath)); err != nil {
		return err
	}
	keepCandidate = true
	return PruneSnapshots(requested)
}

func (s *Store) Sessions(revision int64, query string, limit int) (string, int64, []SessionSummary, error) {
	return s.SessionsPage(revision, query, "", 0, limit)
}

func (s *Store) SessionsPage(revision int64, query, projectID string, offset, limit int) (string, int64, []SessionSummary, error) {
	epoch, latest, e := s.Snapshot()
	if e != nil {
		return "", 0, nil, e
	}
	if revision == 0 {
		revision = latest
	}
	if revision > latest || revision < 1 || !s.hasRevision(epoch, revision) {
		return "", 0, nil, errors.New("revision_unavailable")
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query)
	like := "%" + escaped + "%"
	fts := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	if query == "" {
		fts = `"__inspector_no_query__"`
	}
	rows, e := s.db.Query(`WITH sv AS (
		SELECT v.* FROM session_versions v WHERE v.epoch_id=? AND v.revision=(SELECT max(x.revision) FROM session_versions x WHERE x.epoch_id=v.epoch_id AND x.session_id=v.session_id AND x.revision<=?)
	), labels AS (
		SELECT l.* FROM session_label_versions l WHERE l.epoch_id=? AND l.revision=(SELECT max(x.revision) FROM session_label_versions x WHERE x.epoch_id=l.epoch_id AND x.session_id=l.session_id AND x.revision<=?)
	)
	SELECT r.id,r.source_session_id,rv.root_work_unit_id,rv.purpose,coalesce(rl.title,''),coalesce(rv.earliest_proven_start,''),count(t.id)
	FROM sessions r JOIN sv rv ON rv.session_id=r.id AND rv.root_work_unit_id=r.id
	JOIN sv cv ON cv.root_work_unit_id=r.id JOIN sessions c ON c.id=cv.session_id
	LEFT JOIN labels rl ON rl.session_id=r.id
	LEFT JOIN turns t ON t.session_id=c.id AND t.state='completed' AND t.commit_revision<=?
	LEFT JOIN projects p ON p.id=cv.project_id AND p.created_revision<=?
	WHERE r.epoch_id=? AND r.created_revision<=? AND (?='' OR EXISTS(SELECT 1 FROM sv pv WHERE pv.root_work_unit_id=r.id AND pv.project_id=?)) AND (?='' OR EXISTS(SELECT 1 FROM sv mv JOIN sessions m ON m.id=mv.session_id LEFT JOIN labels ml ON ml.session_id=m.id LEFT JOIN projects mp ON mp.id=mv.project_id WHERE mv.root_work_unit_id=r.id AND (m.source_session_id LIKE ? ESCAPE '\' OR coalesce(ml.title,'') LIKE ? ESCAPE '\' OR coalesce(mp.canonical_identity,'') LIKE ? ESCAPE '\' OR EXISTS(
		SELECT 1 FROM turns st JOIN events ev ON ev.turn_id=st.id JOIN event_search_documents d ON d.epoch_id=ev.epoch_id AND d.event_id=ev.id JOIN event_search ON event_search.rowid=d.rowid WHERE st.session_id=m.id AND ev.commit_revision<=? AND event_search MATCH ?))))
	GROUP BY r.id HAVING count(t.id)>0 ORDER BY max(t.completed_at) DESC,r.id LIMIT ? OFFSET ?`, epoch, revision, epoch, revision, revision, revision, epoch, revision, projectID, projectID, query, like, like, like, revision, fts, limit, offset)
	if e != nil {
		return "", 0, nil, e
	}
	defer rows.Close()
	var out []SessionSummary
	for rows.Next() {
		var x SessionSummary
		if e = rows.Scan(&x.ID, &x.SourceSessionID, &x.RootWorkUnitID, &x.Purpose, &x.Title, &x.StartedAt, &x.CompletedTurns); e != nil {
			return "", 0, nil, e
		}
		out = append(out, x)
	}
	return epoch, revision, out, rows.Err()
}

func (s *Store) hasRevision(epoch string, revision int64) bool {
	var n int
	return s.db.QueryRow("SELECT count(*) FROM index_revisions WHERE epoch_id=? AND revision=?", epoch, revision).Scan(&n) == nil && n == 1
}

type EvidenceLocator struct {
	Epoch                                       string
	Revision                                    int64
	ID, Path, EventHash, SourcePrefix, SourceID string
	Start, End                                  int64
	RecordOrdinal                               int
	Availability                                string
	AvailabilityObservedAt                      string
	AvailabilityRevision                        sql.NullInt64
	SourceSessionID, SegmentFingerprint         string
}

func (s *Store) Evidence(id string, revision int64) (EvidenceLocator, error) {
	epoch, latest, e := s.Snapshot()
	if e != nil {
		return EvidenceLocator{}, e
	}
	if revision == 0 {
		revision = latest
	}
	if revision > latest || revision < 1 || !s.hasRevision(epoch, revision) {
		return EvidenceLocator{}, errors.New("revision_unavailable")
	}
	var x EvidenceLocator
	x.Epoch = epoch
	x.Revision = revision
	x.ID = id
	e = s.db.QueryRow(`SELECT v.canonical_path,r.event_fingerprint,e.byte_start,e.byte_end,coalesce(av.availability,'available'),a.source_session_id,a.segment_fingerprint,r.source_prefix_sha256,r.source_id,e.record_ordinal,coalesce(av.observed_at,v.availability_observed_at),av.availability_revision
		FROM evidence_refs r JOIN events e ON e.epoch_id=r.epoch_id AND e.id=r.event_id JOIN source_artifacts a ON a.epoch_id=r.epoch_id AND a.id=r.source_id
		JOIN source_artifact_versions v ON v.epoch_id=a.epoch_id AND v.source_id=a.id AND v.revision=(SELECT max(x.revision) FROM source_artifact_versions x WHERE x.epoch_id=v.epoch_id AND x.source_id=v.source_id)
		LEFT JOIN evidence_availability_versions av ON av.epoch_id=r.epoch_id AND av.evidence_id=r.id AND av.observed_at=(SELECT max(x.observed_at) FROM evidence_availability_versions x WHERE x.epoch_id=av.epoch_id AND x.evidence_id=av.evidence_id)
		WHERE r.epoch_id=? AND r.id=? AND e.commit_revision<=?`, epoch, id, revision).Scan(&x.Path, &x.EventHash, &x.Start, &x.End, &x.Availability, &x.SourceSessionID, &x.SegmentFingerprint, &x.SourcePrefix, &x.SourceID, &x.RecordOrdinal, &x.AvailabilityObservedAt, &x.AvailabilityRevision)
	return x, e
}
func (s *Store) DB() *sql.DB  { return s.db }
func (s *Store) Path() string { return s.path }
func ParseRevision(v string) (int64, error) {
	if v == "" {
		return 0, nil
	}
	return strconv.ParseInt(v, 10, 64)
}
