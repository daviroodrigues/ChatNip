package database

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"time"
)

func NewSQLiteConnection(dbPath string) (*sql.DB, error) {
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	if err := db.Ping(); err != nil {
		return nil, err
	}

	query := `
	CREATE TABLE IF NOT EXISTS whatsapp_sessions (
		id TEXT PRIMARY KEY,
		tenant_id TEXT,
		jid TEXT,
		proxy_url TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS api_keys (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		key_hash TEXT NOT NULL UNIQUE,
		key_prefix TEXT NOT NULL,
		masked_key TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		revoked_at DATETIME
	);
	CREATE TABLE IF NOT EXISTS media_ingest_jobs (
		id TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		tenant_id TEXT NOT NULL,
		message_id TEXT NOT NULL,
		media_type TEXT NOT NULL,
		mimetype TEXT NOT NULL,
		file_name TEXT NOT NULL DEFAULT '',
		direct_path TEXT NOT NULL,
		media_key BLOB NOT NULL,
		file_enc_sha256 BLOB NOT NULL,
		file_sha256 BLOB NOT NULL,
		file_length INTEGER NOT NULL,
		local_path TEXT NOT NULL DEFAULT '',
		media_id TEXT NOT NULL DEFAULT '',
		event_payload TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending_download',
		attempts INTEGER NOT NULL DEFAULT 0,
		last_error TEXT NOT NULL DEFAULT '',
		next_retry_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_media_ingest_jobs_status_retry ON media_ingest_jobs (status, next_retry_at);`

	if _, err := db.Exec(query); err != nil {
		return nil, err
	}

	// Graceful migration if table existed without masked_key
	_, _ = db.Exec(`ALTER TABLE api_keys ADD COLUMN masked_key TEXT NOT NULL DEFAULT ''`)

	return db, nil
}
