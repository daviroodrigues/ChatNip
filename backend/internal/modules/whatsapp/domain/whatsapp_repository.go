package domain

import (
	"context"
)

type WhatsAppRepository interface {
	SaveMapping(ctx context.Context, session *Session) error
	GetMapping(ctx context.Context, sessionID string) (*Session, error)
	GetSessions(ctx context.Context, tenantID string) ([]*Session, error)
	UpdateProxy(ctx context.Context, sessionID, proxyURL string) error
	DeleteMapping(ctx context.Context, sessionID string) error
}
