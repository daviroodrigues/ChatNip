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
	);`

	if _, err := db.Exec(query); err != nil {
		return nil, err
	}

	// Graceful migration if table existed without masked_key
	_, _ = db.Exec(`ALTER TABLE api_keys ADD COLUMN masked_key TEXT NOT NULL DEFAULT ''`)

	return db, nil
}
