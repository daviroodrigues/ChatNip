CREATE TABLE IF NOT EXISTS hsem_session_states (
    session_id TEXT PRIMARY KEY,
    state_json TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_hsem_session_states_expires
    ON hsem_session_states (expires_at);
