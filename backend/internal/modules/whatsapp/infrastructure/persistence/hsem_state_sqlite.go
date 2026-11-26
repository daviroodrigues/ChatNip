package persistence

import (
	hsemDomain "chatnip/internal/modules/hsem/domain"
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

type sqliteHSEMStateStore struct {
	db *sql.DB
}

func NewSQLiteHSEMStateStore(db *sql.DB) hsemDomain.SessionStateStore {
	return &sqliteHSEMStateStore{db: db}
}

func (s *sqliteHSEMStateStore) LoadSessionState(ctx context.Context, sessionID string) (*hsemDomain.SessionStateSnapshot, error) {
	const query = `SELECT state_json, expires_at FROM hsem_session_states WHERE session_id = ?`
	var raw string
	var expiresAt time.Time
	err := s.db.QueryRowContext(ctx, query, sessionID).Scan(&raw, &expiresAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if !expiresAt.IsZero() && expiresAt.Before(time.Now()) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM hsem_session_states WHERE session_id = ?`, sessionID)
		return nil, nil
	}

	var snapshot hsemDomain.SessionStateSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		return nil, err
	}
	if snapshot.ExpiresAt.IsZero() {
		snapshot.ExpiresAt = expiresAt
	}
	return &snapshot, nil
}

func (s *sqliteHSEMStateStore) SaveSessionState(ctx context.Context, state *hsemDomain.SessionStateSnapshot) error {
	if state == nil || state.SessionID == "" {
		return nil
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	expiresAt := state.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(8 * time.Hour)
	}

	const query = `
		INSERT INTO hsem_session_states (session_id, state_json, expires_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(session_id) DO UPDATE
		SET state_json = excluded.state_json,
		    expires_at = excluded.expires_at,
		    updated_at = CURRENT_TIMESTAMP;
	`
	_, err = s.db.ExecContext(ctx, query, state.SessionID, string(raw), expiresAt.UTC())
	return err
}

func (s *sqliteHSEMStateStore) DeleteExpiredStates(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM hsem_session_states WHERE expires_at < ?`, now.UTC())
	return err
}
