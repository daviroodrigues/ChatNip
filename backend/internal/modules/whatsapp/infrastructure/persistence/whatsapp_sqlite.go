package persistence

import (
	"context"
	"database/sql"
	"chatnip/internal/kit/encryption"
	"chatnip/internal/modules/whatsapp/domain"
)

type sqliteWhatsAppRepository struct {
	db            *sql.DB
	encryptionKey string
}

func NewSQLiteWhatsAppRepository(db *sql.DB, encryptionKey string) domain.WhatsAppRepository {
	return &sqliteWhatsAppRepository{
		db:            db,
		encryptionKey: encryptionKey,
	}
}

func (r *sqliteWhatsAppRepository) SaveMapping(ctx context.Context, session *domain.Session) error {
	proxyToSave := session.ProxyURL
	if proxyToSave != "" {
		encrypted, err := encryption.Encrypt(proxyToSave, r.encryptionKey)
		if err != nil {
			return err
		}
		proxyToSave = encrypted
	}

	query := `
		INSERT INTO whatsapp_sessions (id, tenant_id, jid, proxy_url)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE
		SET jid=excluded.jid, tenant_id=excluded.tenant_id, proxy_url=excluded.proxy_url;
	`
	_, err := r.db.ExecContext(ctx, query, session.ID, session.TenantID, session.JID, proxyToSave)
	if err != nil {

		return err
	}
	return nil
}

func (r *sqliteWhatsAppRepository) GetMapping(ctx context.Context, sessionID string) (*domain.Session, error) {
	query := `SELECT id, tenant_id, jid, COALESCE(proxy_url, ''), created_at FROM whatsapp_sessions WHERE id = ?`

	var s domain.Session
	var rawProxy string

	err := r.db.QueryRowContext(ctx, query, sessionID).Scan(&s.ID, &s.TenantID, &s.JID, &rawProxy, &s.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	if rawProxy != "" {
		decrypted, err := encryption.Decrypt(rawProxy, r.encryptionKey)
		if err != nil {
			return nil, err
		}
		s.ProxyURL = decrypted
	}

	return &s, nil
}

func (r *sqliteWhatsAppRepository) GetSessions(ctx context.Context, tenantID string) ([]*domain.Session, error) {
	query := `SELECT id, tenant_id, jid, COALESCE(proxy_url, ''), created_at FROM whatsapp_sessions`
	var args []interface{}

	if tenantID != "" {
		query += ` WHERE tenant_id = ?`
		args = append(args, tenantID)
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []*domain.Session
	for rows.Next() {
		var s domain.Session
		var rawProxy string

		if err := rows.Scan(&s.ID, &s.TenantID, &s.JID, &rawProxy, &s.CreatedAt); err != nil {
			return nil, err
		}

		if rawProxy != "" {
			decrypted, err := encryption.Decrypt(rawProxy, r.encryptionKey)
			if err != nil {
				return nil, err
			}
			s.ProxyURL = decrypted
		}

		sessions = append(sessions, &s)
	}
	return sessions, nil
}

func (r *sqliteWhatsAppRepository) UpdateProxy(ctx context.Context, sessionID, proxyURL string) error {
	proxyToSave := proxyURL
	if proxyToSave != "" {
		encrypted, err := encryption.Encrypt(proxyToSave, r.encryptionKey)
		if err != nil {
			return err
		}
		proxyToSave = encrypted
	}

	query := `UPDATE whatsapp_sessions SET proxy_url = ? WHERE id = ?`
	result, err := r.db.ExecContext(ctx, query, proxyToSave, sessionID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *sqliteWhatsAppRepository) DeleteMapping(ctx context.Context, sessionID string) error {
	query := `DELETE FROM whatsapp_sessions WHERE id = ?`
	_, err := r.db.ExecContext(ctx, query, sessionID)
	return err
}
