package phase0

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

var ErrDirectFTSMutation = errors.New("direct FTS mutation is forbidden; use FTSWriter")

// GuardApplicationSQL is the fail-closed boundary used before executing
// application-authored SQL. FTS tables and their SQLite shadow tables may only
// be mutated by the fixed statements in FTSWriter or by the offline rebuild
// validator.
func GuardApplicationSQL(query string) error {
	normalized := strings.ToLower(query)
	normalized = strings.NewReplacer("`", " ", `"`, " ", "[", " ", "]", " ", "(", " ", ")", " ", ",", " ", ";", " ", "\n", " ", "\t", " ").Replace(normalized)
	fields := strings.Fields(normalized)
	mutating := false
	for _, field := range fields {
		switch field {
		case "insert", "update", "delete", "replace", "drop", "alter":
			mutating = true
		}
	}
	if !mutating {
		return nil
	}
	for _, field := range fields {
		if field == "event_search" || strings.HasPrefix(field, "event_search_") ||
			field == "session_label_search" || strings.HasPrefix(field, "session_label_search_") {
			return ErrDirectFTSMutation
		}
	}
	return nil
}

// FTSWriter couples each immutable provenance document with exactly one
// contentless FTS row in the caller's transaction. It deliberately exposes no
// update or delete operation.
type FTSWriter struct {
	tx *sql.Tx
}

func NewFTSWriter(tx *sql.Tx) *FTSWriter { return &FTSWriter{tx: tx} }

func (w *FTSWriter) InsertEvent(ctx context.Context, rowID int64, epochID, eventID, category, content string) error {
	if w == nil || w.tx == nil {
		return errors.New("FTS writer requires a transaction")
	}
	if _, err := w.tx.ExecContext(ctx, `INSERT INTO event_search_documents(rowid,epoch_id,event_id,match_category) VALUES(?,?,?,?)`, rowID, epochID, eventID, category); err != nil {
		return err
	}
	_, err := w.tx.ExecContext(ctx, `INSERT INTO event_search(rowid,content) VALUES(?,?)`, rowID, content)
	return err
}

func (w *FTSWriter) InsertSessionLabel(ctx context.Context, rowID int64, epochID, sessionID string, revision int, content string) error {
	if w == nil || w.tx == nil {
		return errors.New("FTS writer requires a transaction")
	}
	if _, err := w.tx.ExecContext(ctx, `INSERT INTO session_label_search_documents(rowid,epoch_id,session_id,label_revision,match_category) VALUES(?,?,?,?,'session_title')`, rowID, epochID, sessionID, revision); err != nil {
		return err
	}
	_, err := w.tx.ExecContext(ctx, `INSERT INTO session_label_search(rowid,content) VALUES(?,?)`, rowID, content)
	return err
}
