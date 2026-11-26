package service

import (
	"chatnip/internal/modules/hsem/domain"
	"context"
	"math"
	"sync"
	"time"
)

const (
	fatigueWindowDuration = time.Hour
	fatigueDecayHalfLife  = 25 * time.Minute
	warmChatHotThreshold  = 90 * time.Second
	warmChatColdThreshold = 2 * time.Hour
	warmChatPruneAge      = 24 * time.Hour
	recentChatSwitchAge   = 2 * time.Minute
	stateTTLMin           = 6 * time.Hour
	stateTTLMax           = 12 * time.Hour
)

type SessionStateTracker struct {
	mu     sync.Mutex
	states map[string]*domain.SessionStateSnapshot
	store  domain.SessionStateStore
	math   *MathUtils
	nowFn  func() time.Time
}

func NewSessionStateTracker(math *MathUtils, store domain.SessionStateStore) *SessionStateTracker {
	if math == nil {
		math = NewMathUtils()
	}
	return &SessionStateTracker{
		states: make(map[string]*domain.SessionStateSnapshot),
		store:  store,
		math:   math,
		nowFn:  time.Now,
	}
}

func (t *SessionStateTracker) GetOrLoad(ctx context.Context, sessionID string) *domain.SessionStateSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.nowFn()
	if st, ok := t.states[sessionID]; ok {
		t.refreshPeriodicParametersLocked(st, now)
		t.decayFatigueLocked(st, now)
		return st
	}

	if t.store != nil {
		if loaded, err := t.store.LoadSessionState(ctx, sessionID); err == nil && loaded != nil {
			if loaded.ExpiresAt.IsZero() || loaded.ExpiresAt.After(now) {
				if loaded.ChatWarmth == nil {
					loaded.ChatWarmth = make(map[string]time.Time)
				}
				t.ensureDefaultsLocked(loaded, now)
				t.refreshPeriodicParametersLocked(loaded, now)
				t.decayFatigueLocked(loaded, now)
				t.states[sessionID] = loaded
				return loaded
			}
		}
	}

	st := t.newSnapshotLocked(sessionID, now)
	t.states[sessionID] = st
	return st
}

func (t *SessionStateTracker) Persist(ctx context.Context, sessionID string) error {
	if t.store == nil {
		return nil
	}
	t.mu.Lock()
	st, ok := t.states[sessionID]
	if !ok {
		t.mu.Unlock()
		return nil
	}
	now := t.nowFn()
	t.decayFatigueLocked(st, now)
	t.pruneWarmthLocked(st, now)
	st.UpdatedAt = now
	st.ExpiresAt = now.Add(t.math.UniformDuration(stateTTLMin, stateTTLMax))
	clone := cloneSnapshot(st)
	t.mu.Unlock()

	return t.store.SaveSessionState(ctx, clone)
}

func (t *SessionStateTracker) PersistAll(ctx context.Context) {
	if t.store == nil {
		return
	}
	t.mu.Lock()
	ids := make([]string, 0, len(t.states))
	for id := range t.states {
		ids = append(ids, id)
	}
	t.mu.Unlock()

	for _, id := range ids {
		_ = t.Persist(ctx, id)
	}
}

func (t *SessionStateTracker) MarkOffline(ctx context.Context, sessionID string) {
	t.mu.Lock()
	if st, ok := t.states[sessionID]; ok {
		now := t.nowFn()
		t.decayFatigueLocked(st, now)
		st.WasOffline = true
		st.UpdatedAt = now
	}
	t.mu.Unlock()

	_ = t.Persist(ctx, sessionID)
}

func (t *SessionStateTracker) ConsumeWasOffline(ctx context.Context, sessionID string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	st := t.getOrInitLocked(ctx, sessionID)
	was := st.WasOffline
	st.WasOffline = false
	return was
}

func (t *SessionStateTracker) RecordMessageSent(ctx context.Context, sessionID, remoteJID string, charCount int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.nowFn()
	st := t.getOrInitLocked(ctx, sessionID)
	t.decayFatigueLocked(st, now)

	if charCount < 0 {
		charCount = 0
	}
	st.RecentActivity = append(st.RecentActivity, domain.ActivityEvent{
		Timestamp: now,
		Chars:     charCount,
	})
	t.pruneActivityLocked(st, now)

	msgDelta := 0.012 + (float64(charCount)/180.0)*0.015
	hourlyMsgs := len(st.RecentActivity)
	hourlyChars := 0
	for _, ev := range st.RecentActivity {
		hourlyChars += ev.Chars
	}
	volumeBoost := 1.0 + math.Min(1.0, float64(hourlyMsgs)/80.0+float64(hourlyChars)/12000.0)
	st.Fatigue = math.Min(1.0, st.Fatigue+msgDelta*volumeBoost)
	st.LastFatigueUpdate = now

	if remoteJID != "" {
		st.LastChatJID = remoteJID
		if st.ChatWarmth == nil {
			st.ChatWarmth = make(map[string]time.Time)
		}
		st.ChatWarmth[remoteJID] = now
	}
	st.LastActionAt = now
	st.WasOffline = false
	st.UpdatedAt = now
}

func (t *SessionStateTracker) RecordInboundChatActivity(ctx context.Context, sessionID, remoteJID string) {
	if sessionID == "" || remoteJID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.nowFn()
	st := t.getOrInitLocked(ctx, sessionID)
	if st.ChatWarmth == nil {
		st.ChatWarmth = make(map[string]time.Time)
	}
	st.ChatWarmth[remoteJID] = now
}

type BehavioralContext struct {
	EffectivePersona    domain.PersonaType
	PersonalSpeedFactor float64
	BandwidthKBps       float64
	CircadianOffset     time.Duration
	InSleepWindow       bool
	Fatigue             float64
	ConversationWarmth  float64
	LastChatJID         string
	TargetChatRecent    bool
}

func (t *SessionStateTracker) Inspect(ctx context.Context, sessionID, remoteJID string, isPriority bool) BehavioralContext {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.nowFn()
	st := t.getOrInitLocked(ctx, sessionID)
	t.refreshPeriodicParametersLocked(st, now)
	t.decayFatigueLocked(st, now)

	warmth := computeWarmth(st.ChatWarmth, remoteJID, now)
	inSleep := isInSleepWindow(now, st.CircadianOffsetMin, st.SleepStartMin, st.SleepEndMin)

	recentChat := false
	if ts, ok := st.ChatWarmth[remoteJID]; ok && now.Sub(ts) <= recentChatSwitchAge {
		recentChat = true
	}

	persona := st.BasePersona
	if persona == "" {
		persona = domain.PersonaStandard
	}
	if isPriority {
		persona = domain.PersonaRushed
	} else if inSleep || st.Fatigue > 0.78 {
		persona = domain.PersonaDistracted
	}

	effectiveSpeed := st.PersonalSpeedFactor * st.ShiftDriftFactor
	if effectiveSpeed <= 0 {
		effectiveSpeed = 1.0
	}

	return BehavioralContext{
		EffectivePersona:    persona,
		PersonalSpeedFactor: effectiveSpeed,
		BandwidthKBps:       st.BandwidthKBps,
		CircadianOffset:     time.Duration(st.CircadianOffsetMin) * time.Minute,
		InSleepWindow:       inSleep,
		Fatigue:             st.Fatigue,
		ConversationWarmth:  warmth,
		LastChatJID:         st.LastChatJID,
		TargetChatRecent:    recentChat,
	}
}

func (t *SessionStateTracker) NextIdleTimeout(ctx context.Context, sessionID string) time.Duration {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.nowFn()
	st := t.getOrInitLocked(ctx, sessionID)
	t.refreshPeriodicParametersLocked(st, now)
	if isInSleepWindow(now, st.CircadianOffsetMin, st.SleepStartMin, st.SleepEndMin) {
		return t.math.UniformDuration(45*time.Second, 90*time.Second)
	}
	return t.math.UniformDuration(3*time.Minute, 8*time.Minute)
}

func (t *SessionStateTracker) getOrInitLocked(ctx context.Context, sessionID string) *domain.SessionStateSnapshot {
	now := t.nowFn()
	if st, ok := t.states[sessionID]; ok {
		return st
	}
	if t.store != nil {
		if loaded, err := t.store.LoadSessionState(ctx, sessionID); err == nil && loaded != nil {
			if loaded.ExpiresAt.IsZero() || loaded.ExpiresAt.After(now) {
				if loaded.ChatWarmth == nil {
					loaded.ChatWarmth = make(map[string]time.Time)
				}
				t.ensureDefaultsLocked(loaded, now)
				t.states[sessionID] = loaded
				return loaded
			}
		}
	}
	st := t.newSnapshotLocked(sessionID, now)
	t.states[sessionID] = st
	return st
}

func (t *SessionStateTracker) newSnapshotLocked(sessionID string, now time.Time) *domain.SessionStateSnapshot {
	st := &domain.SessionStateSnapshot{
		SessionID:           sessionID,
		BasePersona:         domain.PersonaStandard,
		PersonalSpeedFactor: t.math.UniformFloat(0.88, 1.12),
		ShiftDriftFactor:    t.math.UniformFloat(0.92, 1.08),
		NextShiftAt:         now.Add(t.math.UniformDuration(8*time.Hour, 14*time.Hour)),
		BandwidthKBps:       t.math.UniformFloat(domain.MinBandwidthKBps, domain.MaxBandwidthKBps),
		CircadianOffsetMin:  int(math.Round(t.math.UniformFloat(-25, 25))),
		CircadianUpdatedAt:  now,
		SleepStartMin:       90 + int(math.Round(t.math.UniformFloat(-30, 30))),
		SleepEndMin:         390 + int(math.Round(t.math.UniformFloat(-30, 30))),
		WasOffline:          true,
		LastFatigueUpdate:   now,
		ChatWarmth:          make(map[string]time.Time),
		UpdatedAt:           now,
		ExpiresAt:           now.Add(t.math.UniformDuration(stateTTLMin, stateTTLMax)),
	}
	return st
}

func (t *SessionStateTracker) ensureDefaultsLocked(st *domain.SessionStateSnapshot, now time.Time) {
	if st.BasePersona == "" {
		st.BasePersona = domain.PersonaStandard
	}
	if st.PersonalSpeedFactor <= 0 {
		st.PersonalSpeedFactor = t.math.UniformFloat(0.88, 1.12)
	}
	if st.ShiftDriftFactor <= 0 {
		st.ShiftDriftFactor = t.math.UniformFloat(0.92, 1.08)
	}
	if st.NextShiftAt.IsZero() {
		st.NextShiftAt = now.Add(t.math.UniformDuration(8*time.Hour, 14*time.Hour))
	}
	if st.BandwidthKBps <= 0 {
		st.BandwidthKBps = t.math.UniformFloat(domain.MinBandwidthKBps, domain.MaxBandwidthKBps)
	}
	if st.SleepStartMin == 0 && st.SleepEndMin == 0 {
		st.SleepStartMin = 90 + int(math.Round(t.math.UniformFloat(-30, 30)))
		st.SleepEndMin = 390 + int(math.Round(t.math.UniformFloat(-30, 30)))
	}
	if st.LastFatigueUpdate.IsZero() {
		st.LastFatigueUpdate = now
	}
}

func (t *SessionStateTracker) refreshPeriodicParametersLocked(st *domain.SessionStateSnapshot, now time.Time) {
	if st.CircadianUpdatedAt.IsZero() || now.Sub(st.CircadianUpdatedAt) >= 24*time.Hour {
		st.CircadianOffsetMin = int(math.Round(t.math.UniformFloat(-25, 25)))
		st.SleepStartMin = 90 + int(math.Round(t.math.UniformFloat(-30, 30)))
		st.SleepEndMin = 390 + int(math.Round(t.math.UniformFloat(-30, 30)))
		st.CircadianUpdatedAt = now
	}

	if st.NextShiftAt.IsZero() || !now.Before(st.NextShiftAt) {
		st.ShiftDriftFactor = t.math.UniformFloat(0.92, 1.08)
		st.NextShiftAt = now.Add(t.math.UniformDuration(8*time.Hour, 14*time.Hour))
	}
}

func (t *SessionStateTracker) decayFatigueLocked(st *domain.SessionStateSnapshot, now time.Time) {
	t.pruneActivityLocked(st, now)
	if st.LastFatigueUpdate.IsZero() {
		st.LastFatigueUpdate = now
		return
	}
	elapsed := now.Sub(st.LastFatigueUpdate)
	if elapsed <= 0 || st.Fatigue <= 0 {
		st.LastFatigueUpdate = now
		return
	}
	lambda := math.Ln2 / fatigueDecayHalfLife.Seconds()
	st.Fatigue *= math.Exp(-lambda * elapsed.Seconds())
	if st.Fatigue < 0.001 {
		st.Fatigue = 0
	}
	st.LastFatigueUpdate = now
}

func (t *SessionStateTracker) pruneActivityLocked(st *domain.SessionStateSnapshot, now time.Time) {
	cutoff := now.Add(-fatigueWindowDuration)
	kept := st.RecentActivity[:0]
	for _, ev := range st.RecentActivity {
		if ev.Timestamp.After(cutoff) {
			kept = append(kept, ev)
		}
	}
	st.RecentActivity = kept
}

func (t *SessionStateTracker) pruneWarmthLocked(st *domain.SessionStateSnapshot, now time.Time) {
	cutoff := now.Add(-warmChatPruneAge)
	for jid, ts := range st.ChatWarmth {
		if ts.Before(cutoff) {
			delete(st.ChatWarmth, jid)
		}
	}
}

func computeWarmth(warmthMap map[string]time.Time, remoteJID string, now time.Time) float64 {
	if remoteJID == "" || warmthMap == nil {
		return 0
	}
	last, ok := warmthMap[remoteJID]
	if !ok || last.IsZero() {
		return 0
	}
	elapsed := now.Sub(last)
	if elapsed <= warmChatHotThreshold {
		return 1.0
	}
	if elapsed >= warmChatColdThreshold {
		return 0.0
	}
	ratio := float64(elapsed-warmChatHotThreshold) / float64(warmChatColdThreshold-warmChatHotThreshold)
	return 1.0 - ratio
}

func isInSleepWindow(now time.Time, offsetMin, startMin, endMin int) bool {
	adj := now.Add(time.Duration(offsetMin) * time.Minute)
	minuteOfDay := adj.Hour()*60 + adj.Minute()
	start := ((startMin % 1440) + 1440) % 1440
	end := ((endMin % 1440) + 1440) % 1440
	if start < end {
		return minuteOfDay >= start && minuteOfDay < end
	}
	return minuteOfDay >= start || minuteOfDay < end
}

func cloneSnapshot(src *domain.SessionStateSnapshot) *domain.SessionStateSnapshot {
	cp := *src
	if src.RecentActivity != nil {
		cp.RecentActivity = make([]domain.ActivityEvent, len(src.RecentActivity))
		copy(cp.RecentActivity, src.RecentActivity)
	}
	if src.ChatWarmth != nil {
		cp.ChatWarmth = make(map[string]time.Time, len(src.ChatWarmth))
		for k, v := range src.ChatWarmth {
			cp.ChatWarmth[k] = v
		}
	}
	return &cp
}
