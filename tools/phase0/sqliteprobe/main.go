package main

import (
	"database/sql"
	"fmt"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "file:phase0-proof?mode=memory&cache=shared")
	if err != nil {
		fail(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE proof(id INTEGER PRIMARY KEY, value TEXT NOT NULL); INSERT INTO proof(value) VALUES('self-contained');`); err != nil {
		fail(err)
	}
	var value string
	if err := db.QueryRow(`SELECT value FROM proof WHERE id = 1`).Scan(&value); err != nil {
		fail(err)
	}
	fmt.Printf("sqlite_driver=modernc.org/sqlite cgo=not-required round_trip=%s\n", value)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
