package domain

import (
	"context"
	"time"
)

type ActivityEvent struct {
	Timestamp time.Time `json:"ts"`
	Chars     int       `json:"chars"`
}

type SessionStateSnapshot struct {
	SessionID           string               `json:"session_id"`
	BasePersona         PersonaType          `json:"base_persona"`
	PersonalSpeedFactor float64              `json:"personal_speed_factor"`
	ShiftDriftFactor    float64              `json:"shift_drift_factor"`
	NextShiftAt         time.Time            `json:"next_shift_at"`
	BandwidthKBps       float64              `json:"bandwidth_kbps"`
	CircadianOffsetMin  int                  `json:"circadian_offset_min"`
	CircadianUpdatedAt  time.Time            `json:"circadian_updated_at"`
	SleepStartMin       int                  `json:"sleep_start_min"`
	SleepEndMin         int                  `json:"sleep_end_min"`
	LastChatJID         string               `json:"last_chat_jid"`
	LastActionAt        time.Time            `json:"last_action_at"`
	WasOffline          bool                 `json:"was_offline"`
	Fatigue             float64              `json:"fatigue"`
	LastFatigueUpdate   time.Time            `json:"last_fatigue_update"`
	RecentActivity      []ActivityEvent      `json:"recent_activity,omitempty"`
	ChatWarmth          map[string]time.Time `json:"chat_warmth,omitempty"`
	UpdatedAt           time.Time            `json:"updated_at"`
	ExpiresAt           time.Time            `json:"expires_at"`
}

type SessionStateStore interface {
	LoadSessionState(ctx context.Context, sessionID string) (*SessionStateSnapshot, error)
	SaveSessionState(ctx context.Context, state *SessionStateSnapshot) error
	DeleteExpiredStates(ctx context.Context, now time.Time) error
}
