CREATE TABLE IF NOT EXISTS cache_entries (
    key        TEXT    PRIMARY KEY NOT NULL,
    data       BLOB    NOT NULL,
    expires_at INTEGER NOT NULL
);
