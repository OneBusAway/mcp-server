package cachedb

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Open opens (or creates) the SQLite cache database at path.
// It applies the schema and prunes already-expired entries on startup.
func Open(path string) (*Queries, *sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: one writer at a time

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, nil, err
	}

	q := New(db)
	// Discard expired rows left over from previous sessions.
	_ = q.PruneExpired(context.Background(), time.Now().Unix())

	return q, db, nil
}

// schema is the DDL applied on every open (CREATE TABLE IF NOT EXISTS is idempotent).
const schema = `
CREATE TABLE IF NOT EXISTS cache_entries (
    key        TEXT    PRIMARY KEY NOT NULL,
    data       BLOB    NOT NULL,
    expires_at INTEGER NOT NULL
);
`
