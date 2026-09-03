package cachedb

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
)

const maxCacheDatabaseBytes = 32 << 20

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
	// The cache is disposable. Cap its database file so a misconfigured or busy
	// service cannot consume unbounded disk; expired rows are pruned on open and
	// before static cache writes.
	if _, err := db.Exec("PRAGMA max_page_count = " + strconv.Itoa(maxCacheDatabaseBytes/4096)); err != nil {
		db.Close()
		return nil, nil, err
	}

	q := New(db)
	if err := q.PruneExpired(context.Background(), time.Now().Unix()); err != nil {
		log.Printf("cache prune: %v", err)
	}

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
