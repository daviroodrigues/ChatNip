package service

import (
	hsemDomain "chatnip/internal/modules/hsem/domain"
	hsemService "chatnip/internal/modules/hsem/service"
	"chatnip/internal/modules/whatsapp/domain"
	"context"
	"errors"
	"math"
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

var ErrPreempted = errors.New("standard-lane step preempted by fast-lane")

type SessionGate struct {
	mu          sync.Mutex
	fastPending atomic.Int32

	preemptMu      sync.Mutex
	standardCancel context.CancelCauseFunc
}

func (g *SessionGate) RegisterStandardCancel(cancel context.CancelCauseFunc) {
	g.preemptMu.Lock()
	g.standardCancel = cancel
	g.preemptMu.Unlock()
}

func (g *SessionGate) ClearStandardCancel() {
	g.preemptMu.Lock()
	g.standardCancel = nil
	g.preemptMu.Unlock()
}

func (g *SessionGate) triggerPreempt() {
	g.preemptMu.Lock()
	cancel := g.standardCancel
	g.preemptMu.Unlock()
	if cancel != nil {
		cancel(ErrPreempted)
	}
}

func (g *SessionGate) AcquireFast(ctx context.Context) error {
	g.fastPending.Add(1)
	defer g.fastPending.Add(-1)

	g.triggerPreempt()

	done := make(chan struct{})
	go func() {
		g.mu.Lock()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		go func() {
			<-done
			g.mu.Unlock()
		}()
		return ctx.Err()
	}
}

func (g *SessionGate) AcquireStandard(ctx context.Context) error {
	for {
		if g.fastPending.Load() > 0 {
			if err := sleepWithContext(ctx, jitterDuration(GatePollMin, GatePollMax)); err != nil {
				return err
			}
			continue
		}

		if g.mu.TryLock() {
			if g.fastPending.Load() > 0 {
				g.mu.Unlock()
				continue
			}
			return nil
		}

		if err := sleepWithContext(ctx, jitterDuration(GatePollMin, GatePollMax)); err != nil {
			return err
		}
	}
}

func (g *SessionGate) Release() {
	g.mu.Unlock()
}

const (
	ContextSwitchRecentMin   = 800 * time.Millisecond
	ContextSwitchRecentMax   = 2 * time.Second
	ContextSwitchInactiveMin = 3 * time.Second
	ContextSwitchInactiveMax = 8 * time.Second

	GatePollMin          = 15 * time.Millisecond
	GatePollMax          = 45 * time.Millisecond
	BurstBufferWindowMin = 40 * time.Millisecond
	BurstBufferWindowMax = 95 * time.Millisecond

	MaxBatchSize      = 5
	MaxFastLaneStreak = 3
)

type BurstExecutionOptions struct {
	IsBurstContinuing    bool
	WasOffline           bool
	SkipMarkAsRead       bool
	BatchReadMsgID       string
	BatchReadingOverhead time.Duration
	ConsecutiveMsgs      int
	ContinuousComposing  *time.Duration
	ComposingPauseLimit  time.Duration
	StepCtx              context.Context
}

type SessionJob struct {
	Payload      domain.SendMessagePayload
	MediaData    []byte
	Result       chan error
	IsFast       bool
	Acknowledger domain.Acknowledger
}

type SessionWorker struct {
	ID             string
	FastQueue      chan SessionJob
	StandardQueue  chan SessionJob
	stashedJob     *SessionJob
	LastChatJID    string
	LastAction     time.Time
	FastLaneStreak int
	Quit           chan struct{}
	ctx            context.Context
	cancel         context.CancelFunc
	stopOnce       sync.Once
}

func (w *SessionWorker) Stop() {
	w.stopOnce.Do(func() {
		if w.cancel != nil {
			w.cancel()
		}
		if w.Quit != nil {
			close(w.Quit)
		}
	})
}

type SessionSerializer struct {
	workers sync.Map
	gates   sync.Map
	service *WhatsAppService
	mu      sync.Mutex
}

func NewSessionSerializer(svc *WhatsAppService) *SessionSerializer {
	return &SessionSerializer{service: svc}
}

func (s *SessionSerializer) CancelSession(sessionID string) {
	if val, ok := s.workers.LoadAndDelete(sessionID); ok {
		w := val.(*SessionWorker)
		w.Stop()
	}
	if s.service != nil && s.service.stateTracker != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.service.stateTracker.Persist(ctx, sessionID)
	}
}

func (s *SessionSerializer) Shutdown() {
	s.workers.Range(func(key, value interface{}) bool {
		if w, ok := value.(*SessionWorker); ok {
			w.Stop()
		}
		s.workers.Delete(key)
		return true
	})
	if s.service != nil && s.service.stateTracker != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.service.stateTracker.PersistAll(ctx)
	}
}

func (s *SessionSerializer) getOrCreateGate(sessionID string) *SessionGate {
	if val, ok := s.gates.Load(sessionID); ok {
		return val.(*SessionGate)
	}
	gate := &SessionGate{}
	actual, _ := s.gates.LoadOrStore(sessionID, gate)
	return actual.(*SessionGate)
}

func (s *SessionSerializer) Enqueue(ctx context.Context, payload domain.SendMessagePayload, mediaData []byte, ack domain.Acknowledger) error {
	if payload.IsPriority {
		return s.executeFast(ctx, payload, mediaData, ack)
	}

	worker := s.getOrCreateWorker(payload.SessionID)
	resultChan := make(chan error, 1)

	job := SessionJob{
		Payload:      payload,
		MediaData:    mediaData,
		Result:       resultChan,
		IsFast:       false,
		Acknowledger: ack,
	}

	select {
	case worker.StandardQueue <- job:
		select {
		case err := <-resultChan:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *SessionSerializer) executeFast(ctx context.Context, payload domain.SendMessagePayload, mediaData []byte, ack domain.Acknowledger) error {
	gate := s.getOrCreateGate(payload.SessionID)
	if err := gate.AcquireFast(ctx); err != nil {
		return err
	}

	wasOffline := false
	if s.service != nil && s.service.stateTracker != nil {
		wasOffline = s.service.stateTracker.ConsumeWasOffline(ctx, payload.SessionID)
	}

	msgID, err := s.service.ExecuteBurstStepWithOpts(ctx, payload, mediaData, BurstExecutionOptions{
		WasOffline: wasOffline,
		StepCtx:    ctx,
	})
	gate.Release()

	if ack != nil && err == nil {
		s.publishSentAndAck(ctx, payload, msgID, ack)
	} else if err != nil {
		s.service.logger.Error().Err(err).
			Str("session_id", payload.SessionID).
			Msg("Failed to execute fast-lane step")
	}
	return err
}

func (s *SessionSerializer) publishSentAndAck(ctx context.Context, payload domain.SendMessagePayload, msgID string, ack domain.Acknowledger) {
	if payload.CorrelationID != "" {
		evt := domain.MessageSentEvent{
			SchemaVersion: domain.QueueSchemaVersion,
			EventType:     "message.sent",
			CorrelationID: payload.CorrelationID,
			Timestamp:     time.Now().UTC(),
			Data: domain.MessageSentData{
				MessageID: msgID,
				SessionID: payload.SessionID,
				JID:       payload.RemoteJID,
				Status:    "SENT",
			},
		}

		if pubErr := s.service.publisher.Publish(ctx, "whatsapp.message.sent", evt); pubErr != nil {
			s.service.logger.Error().Err(pubErr).Msg("Failed to publish message.sent event")
		}
	}

	_ = ack.Ack()
}

func (s *SessionSerializer) getOrCreateWorker(sessionID string) *SessionWorker {
	if val, ok := s.workers.Load(sessionID); ok {
		return val.(*SessionWorker)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if val, ok := s.workers.Load(sessionID); ok {
		return val.(*SessionWorker)
	}

	workerCtx, workerCancel := context.WithCancel(context.Background())
	worker := &SessionWorker{
		ID:            sessionID,
		FastQueue:     make(chan SessionJob, 50),
		StandardQueue: make(chan SessionJob, 100),
		Quit:          make(chan struct{}),
		ctx:           workerCtx,
		cancel:        workerCancel,
	}

	if s.service != nil && s.service.stateTracker != nil {
		snapshot := s.service.stateTracker.GetOrLoad(workerCtx, sessionID)
		if snapshot != nil {
			worker.LastChatJID = snapshot.LastChatJID
			worker.LastAction = snapshot.LastActionAt
		}
	}

	s.workers.Store(sessionID, worker)
	go s.runWorker(worker)
	return worker
}

func (s *SessionSerializer) nextIdleTimeout(w *SessionWorker) time.Duration {
	if s.service != nil && s.service.stateTracker != nil {
		return s.service.stateTracker.NextIdleTimeout(w.ctx, w.ID)
	}
	return jitterDuration(3*time.Minute, 8*time.Minute)
}

func (s *SessionSerializer) runWorker(w *SessionWorker) {
	idleTimer := time.NewTimer(s.nextIdleTimeout(w))
	defer idleTimer.Stop()

	for {
		job, received := s.pickNextJob(w, idleTimer)

		if !received {
			if s.service != nil {
				if client, ok := s.service.clientRepo.GetClient(w.ID); ok {
					_ = client.SendGlobalPresence(context.Background(), false)
				}
				if s.service.stateTracker != nil {
					persistCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					s.service.stateTracker.MarkOffline(persistCtx, w.ID)
					cancel()
				}
			}
			s.workers.Delete(w.ID)
			return
		}

		if s.service != nil {
			if client, ok := s.service.clientRepo.GetClient(w.ID); ok {
				_ = client.SendGlobalPresence(context.Background(), true)
			}
		}

		batch := s.collectBatch(w, job)
		s.processBatch(w, batch)

		if s.service != nil && s.service.stateTracker != nil {
			persistCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			_ = s.service.stateTracker.Persist(persistCtx, w.ID)
			cancel()
		}

		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(s.nextIdleTimeout(w))
	}
}

func (s *SessionSerializer) pickNextJob(w *SessionWorker, idleTimer *time.Timer) (SessionJob, bool) {
	if w.stashedJob != nil {
		job := *w.stashedJob
		w.stashedJob = nil
		return job, true
	}

	tryRead := func(ch chan SessionJob) (SessionJob, bool) {
		select {
		case j := <-ch:
			return j, true
		default:
			return SessionJob{}, false
		}
	}

	if w.FastLaneStreak < MaxFastLaneStreak {
		if job, ok := tryRead(w.FastQueue); ok {
			w.FastLaneStreak++
			return job, true
		}
	}

	if job, ok := tryRead(w.StandardQueue); ok {
		w.FastLaneStreak = 0
		return job, true
	}

	if w.FastLaneStreak >= MaxFastLaneStreak {
		if job, ok := tryRead(w.FastQueue); ok {
			w.FastLaneStreak++
			return job, true
		}
	}

	select {
	case job := <-w.FastQueue:
		w.FastLaneStreak++
		return job, true
	case job := <-w.StandardQueue:
		w.FastLaneStreak = 0
		return job, true
	case <-w.Quit:
		return SessionJob{}, false
	case <-idleTimer.C:
		return SessionJob{}, false
	}
}

func (s *SessionSerializer) collectBatch(w *SessionWorker, firstJob SessionJob) []SessionJob {
	batch := []SessionJob{firstJob}
	targetJID := firstJob.Payload.RemoteJID

	var sourceQueue chan SessionJob
	if firstJob.IsFast {
		sourceQueue = w.FastQueue
	} else {
		sourceQueue = w.StandardQueue
	}

	timeout := time.NewTimer(s.batchWindowDuration())
	defer timeout.Stop()

LoopColeta:
	for len(batch) < MaxBatchSize {
		select {
		case nextJob := <-sourceQueue:
			if nextJob.Payload.RemoteJID == targetJID {
				batch = append(batch, nextJob)
				if firstJob.IsFast {
					w.FastLaneStreak++
				}
			} else {
				stashed := nextJob
				w.stashedJob = &stashed
				break LoopColeta
			}
		case <-timeout.C:
			break LoopColeta
		}
	}
	return batch
}

func (s *SessionSerializer) batchWindowDuration() time.Duration {
	if s.service != nil && s.service.hsemEngine != nil {
		return s.service.hsemEngine.Math().UniformDuration(BurstBufferWindowMin, BurstBufferWindowMax)
	}
	return jitterDuration(BurstBufferWindowMin, BurstBufferWindowMax)
}

func (s *SessionSerializer) processBatch(w *SessionWorker, batch []SessionJob) {
	workerCtx := w.ctx
	if workerCtx == nil {
		workerCtx = context.Background()
	}
	gate := s.getOrCreateGate(w.ID)
	targetJID := batch[0].Payload.RemoteJID

	if w.LastChatJID != "" && w.LastChatJID != targetJID {
		var switchDelay time.Duration
		isRecent := false
		if s.service != nil && s.service.stateTracker != nil {
			bc := s.service.stateTracker.Inspect(workerCtx, w.ID, targetJID, false)
			isRecent = bc.TargetChatRecent
		}
		if isRecent {
			switchDelay = s.service.hsemEngine.Math().UniformDuration(ContextSwitchRecentMin, ContextSwitchRecentMax)
		} else {
			switchDelay = s.service.hsemEngine.Math().UniformDuration(ContextSwitchInactiveMin, ContextSwitchInactiveMax)
		}
		if err := sleepWithContext(workerCtx, switchDelay); err != nil {
			for _, job := range batch {
				job.Result <- err
				close(job.Result)
			}
			return
		}
	}
	w.LastChatJID = targetJID

	wasOffline := false
	if s.service != nil && s.service.stateTracker != nil {
		wasOffline = s.service.stateTracker.ConsumeWasOffline(workerCtx, w.ID)
	}

	batchReadMsgID, batchReadOverhead := computeBatchMarkAsRead(batch)

	var continuousComposing time.Duration
	composingLimit := s.service.hsemEngine.Math().UniformDuration(
		hsemDomain.MaxContinuousComposingMin,
		hsemDomain.MaxContinuousComposingMax,
	)

	for i, job := range batch {
		isLast := i == len(batch)-1

		if i > 0 {
			microDelay := s.service.hsemEngine.Math().UniformDuration(200*time.Millisecond, 600*time.Millisecond)
			if err := sleepWithContext(workerCtx, microDelay); err != nil {
				job.Result <- err
				close(job.Result)
				continue
			}
		}

		opts := BurstExecutionOptions{
			IsBurstContinuing:    !isLast,
			WasOffline:           wasOffline && i == 0,
			SkipMarkAsRead:       len(batch) > 1 && i > 0 && batchReadMsgID != "",
			BatchReadMsgID:       "",
			BatchReadingOverhead: 0,
			ConsecutiveMsgs:      i + 1,
			ContinuousComposing:  &continuousComposing,
			ComposingPauseLimit:  composingLimit,
		}
		if i == 0 && len(batch) > 1 && batchReadMsgID != "" {
			opts.BatchReadMsgID = batchReadMsgID
			opts.BatchReadingOverhead = batchReadOverhead
		}

		var msgID string
		var err error
		for {
			if acqErr := gate.AcquireStandard(workerCtx); acqErr != nil {
				err = acqErr
				break
			}

			stepCtx, stepCancel := context.WithCancelCause(workerCtx)
			gate.RegisterStandardCancel(stepCancel)
			opts.StepCtx = stepCtx

			msgID, err = s.service.ExecuteBurstStepWithOpts(workerCtx, job.Payload, job.MediaData, opts)

			gate.ClearStandardCancel()
			stepCancel(nil)
			gate.Release()

			if err != nil && errors.Is(context.Cause(stepCtx), ErrPreempted) && workerCtx.Err() == nil {
				if sleepErr := sleepWithContext(workerCtx, jitterDuration(GatePollMin, GatePollMax)); sleepErr != nil {
					err = sleepErr
					break
				}
				continue
			}
			break
		}

		if job.Acknowledger != nil {
			if err == nil {
				s.publishSentAndAck(workerCtx, job.Payload, msgID, job.Acknowledger)
			} else {
				s.service.logger.Error().Err(err).
					Str("session_id", job.Payload.SessionID).
					Msg("Failed to execute burst step")
			}
		}

		job.Result <- err
		close(job.Result)

		w.LastAction = time.Now()
	}
}

func computeBatchMarkAsRead(batch []SessionJob) (latestMsgID string, sublinearOverhead time.Duration) {
	if len(batch) <= 1 {
		return "", 0
	}
	extraChars := 0
	for i, job := range batch {
		id := job.Payload.QuotedMsgID
		if id == "" {
			id = job.Payload.TargetMsgID
		}
		if id != "" {
			latestMsgID = id
		}
		if i > 0 {
			extraChars += len([]rune(job.Payload.Text))
		}
	}
	if latestMsgID == "" || extraChars == 0 {
		return latestMsgID, 0
	}
	rawMs := float64(extraChars) * float64(hsemDomain.AvgReadingSpeedMs)
	sublinearMs := rawMs / math.Pow(float64(len(batch)), 0.35)
	return latestMsgID, time.Duration(sublinearMs) * time.Millisecond
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func jitterDuration(min, max time.Duration) time.Duration {
	delta := int64(max - min)
	if delta <= 0 {
		return min
	}
	return min + time.Duration(rand.Int64N(delta))
}

var _ *hsemService.SessionStateTracker
