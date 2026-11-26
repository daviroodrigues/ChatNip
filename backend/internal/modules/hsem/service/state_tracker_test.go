package service

import (
	"chatnip/internal/kit/database"
	"chatnip/internal/modules/hsem/domain"
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionStateTracker_FatigueAccumulationAndExponentialDecay(t *testing.T) {
	now := time.Date(2026, 11, 25, 14, 0, 0, 0, time.UTC)
	tracker := NewSessionStateTracker(NewMathUtils(), nil)
	tracker.nowFn = func() time.Time { return now }

	ctx := context.Background()
	for i := 0; i < 15; i++ {
		tracker.RecordMessageSent(ctx, "sess_fatigue", "5511999999999@s.whatsapp.net", 120)
		now = now.Add(15 * time.Second)
	}

	bcPeak := tracker.Inspect(ctx, "sess_fatigue", "5511999999999@s.whatsapp.net", false)
	if bcPeak.Fatigue <= 0.25 {
		t.Fatalf("expected fatigue to accumulate after 15 messages, got %.4f", bcPeak.Fatigue)
	}
	if bcPeak.ConversationWarmth < 0.95 {
		t.Fatalf("expected hot conversation warmth (~1.0), got %.4f", bcPeak.ConversationWarmth)
	}
	if !bcPeak.TargetChatRecent {
		t.Fatalf("expected target chat to be marked recent")
	}

	now = now.Add(50 * time.Minute)
	bcAfterIdle := tracker.Inspect(ctx, "sess_fatigue", "5511999999999@s.whatsapp.net", false)
	if bcAfterIdle.Fatigue >= bcPeak.Fatigue*0.35 {
		t.Fatalf("expected exponential decay after 50m idle: peak=%.4f, after=%.4f", bcPeak.Fatigue, bcAfterIdle.Fatigue)
	}
	if bcAfterIdle.TargetChatRecent {
		t.Fatalf("expected target chat not to be recent after 50m")
	}

	now = now.Add(2 * time.Hour)
	bcCold := tracker.Inspect(ctx, "sess_fatigue", "5511999999999@s.whatsapp.net", false)
	if bcCold.ConversationWarmth != 0 {
		t.Fatalf("expected cold conversation warmth 0, got %.4f", bcCold.ConversationWarmth)
	}
}

func TestSessionStateTracker_BandwidthSleepWindowAndShiftDrift(t *testing.T) {
	now := time.Date(2026, 11, 25, 3, 30, 0, 0, time.UTC)
	tracker := NewSessionStateTracker(NewMathUtils(), nil)
	tracker.nowFn = func() time.Time { return now }

	ctx := context.Background()
	st := tracker.GetOrLoad(ctx, "sess_bio")

	if st.BandwidthKBps < domain.MinBandwidthKBps || st.BandwidthKBps > domain.MaxBandwidthKBps {
		t.Fatalf("bandwidth out of expected fiber range: %.1f", st.BandwidthKBps)
	}
	if st.CircadianOffsetMin < -25 || st.CircadianOffsetMin > 25 {
		t.Fatalf("circadian offset out of [-25,25]: %d", st.CircadianOffsetMin)
	}

	sleepIdle := tracker.NextIdleTimeout(ctx, "sess_bio")
	if sleepIdle < 45*time.Second || sleepIdle > 90*time.Second {
		t.Fatalf("expected sleep window idle timeout in [45s, 90s], got %v", sleepIdle)
	}

	now = time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	awakeIdle := tracker.NextIdleTimeout(ctx, "sess_bio")
	if awakeIdle < 3*time.Minute || awakeIdle > 8*time.Minute {
		t.Fatalf("expected awake idle timeout in [3m, 8m], got %v", awakeIdle)
	}

	oldNextShift := st.NextShiftAt
	now = now.Add(15 * time.Hour)
	_ = tracker.Inspect(ctx, "sess_bio", "", false)
	if !st.NextShiftAt.After(oldNextShift) {
		t.Fatalf("expected NextShiftAt to roll forward after 15h")
	}
}

type memoryStateStore struct {
	data map[string]*domain.SessionStateSnapshot
}

func (m *memoryStateStore) LoadSessionState(_ context.Context, sessionID string) (*domain.SessionStateSnapshot, error) {
	st, ok := m.data[sessionID]
	if !ok {
		return nil, nil
	}
	cp := cloneSnapshot(st)
	return cp, nil
}

func (m *memoryStateStore) SaveSessionState(_ context.Context, state *domain.SessionStateSnapshot) error {
	if m.data == nil {
		m.data = make(map[string]*domain.SessionStateSnapshot)
	}
	m.data[state.SessionID] = cloneSnapshot(state)
	return nil
}

func (m *memoryStateStore) DeleteExpiredStates(_ context.Context, now time.Time) error {
	for id, st := range m.data {
		if st.ExpiresAt.Before(now) {
			delete(m.data, id)
		}
	}
	return nil
}

func TestSessionStateTracker_PersistAndRestore(t *testing.T) {
	store := &memoryStateStore{}
	ctx := context.Background()

	t1 := NewSessionStateTracker(NewMathUtils(), store)
	t1.RecordMessageSent(ctx, "sess_persist", "5511888888888@s.whatsapp.net", 250)
	t1.MarkOffline(ctx, "sess_persist")

	t2 := NewSessionStateTracker(NewMathUtils(), store)
	restored := t2.GetOrLoad(ctx, "sess_persist")
	if restored.LastChatJID != "5511888888888@s.whatsapp.net" {
		t.Fatalf("expected restored LastChatJID, got %q", restored.LastChatJID)
	}
	if restored.Fatigue <= 0 {
		t.Fatalf("expected restored fatigue > 0, got %.4f", restored.Fatigue)
	}
	if !t2.ConsumeWasOffline(ctx, "sess_persist") {
		t.Fatalf("expected WasOffline to be true after MarkOffline")
	}
	if t2.ConsumeWasOffline(ctx, "sess_persist") {
		t.Fatalf("expected WasOffline to be cleared after consumption")
	}
}

func TestSQLiteSchema_CreatesHSEMStateTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.sqlite")
	db, err := database.NewSQLiteConnection(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteConnection failed: %v", err)
	}
	defer db.Close()

	var name string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='hsem_session_states'`).Scan(&name)
	if err != nil || name != "hsem_session_states" {
		t.Fatalf("expected hsem_session_states table to exist, err=%v name=%q", err, name)
	}
}
