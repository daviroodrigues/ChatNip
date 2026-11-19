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

CREATE INDEX IF NOT EXISTS idx_media_ingest_jobs_status_retry ON media_ingest_jobs (status, next_retry_at);
