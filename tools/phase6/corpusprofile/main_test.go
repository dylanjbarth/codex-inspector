package main

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestCountSourceStatesReportsFrozenSupportedCurrentProjection(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`CREATE TABLE source_artifact_versions(epoch_id TEXT, source_id TEXT, revision INTEGER, state TEXT);
		INSERT INTO source_artifact_versions VALUES
		('epoch','supported-1',1,'supported'),
		('epoch','supported-1',2,'supported'),
		('epoch','session-index',1,'current'),
		('epoch','unsupported-1',1,'unsupported'),
		('epoch','failed-1',1,'failed'),
		('epoch','rebuild-1',1,'requires_rebuild');`); err != nil {
		t.Fatal(err)
	}
	total, supported, unsupported, failed, rebuild, err := countSourceStates(db, "epoch")
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 || supported != 2 || unsupported != 1 || failed != 1 || rebuild != 1 {
		t.Fatalf("projection total=%d supported=%d unsupported=%d failed=%d rebuild=%d", total, supported, unsupported, failed, rebuild)
	}
}
