package service

import (
	"chatnip/internal/modules/whatsapp/domain"
	"context"
	"sync"
	"time"
)

const (
	ContextSwitchMin  = 2 * time.Second
	ContextSwitchMax  = 5 * time.Second
	BurstBufferWindow = 50 * time.Millisecond
	MaxBatchSize      = 5
	MaxFastLaneStreak = 3
)

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
}

type SessionSerializer struct {
	workers sync.Map
	service *WhatsAppService
	mu      sync.Mutex
}

func NewSessionSerializer(svc *WhatsAppService) *SessionSerializer {
	return &SessionSerializer{service: svc}
}

func (s *SessionSerializer) Enqueue(ctx context.Context, payload domain.SendMessagePayload, mediaData []byte, ack domain.Acknowledger) error {
	worker := s.getOrCreateWorker(payload.SessionID)
	resultChan := make(chan error, 1)

	job := SessionJob{
		Payload:      payload,
		MediaData:    mediaData,
		Result:       resultChan,
		IsFast:       payload.IsPriority,
		Acknowledger: ack,
	}

	var targetQueue chan SessionJob
	if payload.IsPriority {
		targetQueue = worker.FastQueue
	} else {
		targetQueue = worker.StandardQueue
	}

	select {
	case targetQueue <- job:
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

func (s *SessionSerializer) getOrCreateWorker(sessionID string) *SessionWorker {
	if val, ok := s.workers.Load(sessionID); ok {
		return val.(*SessionWorker)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if val, ok := s.workers.Load(sessionID); ok {
		return val.(*SessionWorker)
	}

	worker := &SessionWorker{
		ID:            sessionID,
		FastQueue:     make(chan SessionJob, 50),
		StandardQueue: make(chan SessionJob, 100),
		Quit:          make(chan struct{}),
	}
	s.workers.Store(sessionID, worker)
	go s.runWorker(worker)
	return worker
}

func (s *SessionSerializer) runWorker(w *SessionWorker) {
	idleTimer := time.NewTimer(5 * time.Minute)
	defer idleTimer.Stop()

	for {
		job, received := s.pickNextJob(w, idleTimer)

		if !received {
			if client, ok := s.service.clientRepo.GetClient(w.ID); ok {
				_ = client.SendGlobalPresence(context.Background(), false)
			}
			s.workers.Delete(w.ID)
			return
		}

		if client, ok := s.service.clientRepo.GetClient(w.ID); ok {
			_ = client.SendGlobalPresence(context.Background(), true)
		}

		batch := s.collectBatch(w, job)
		s.processBatch(w, batch)

		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(5 * time.Minute)
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

	timeout := time.NewTimer(BurstBufferWindow)
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

func (s *SessionSerializer) processBatch(w *SessionWorker, batch []SessionJob) {
	ctx := context.Background()

	if w.LastChatJID != "" && w.LastChatJID != batch[0].Payload.RemoteJID {
		delay := s.service.hsemEngine.Math().UniformDuration(ContextSwitchMin, ContextSwitchMax)
		time.Sleep(delay)
	}
	w.LastChatJID = batch[0].Payload.RemoteJID

	for i, job := range batch {
		isLast := i == len(batch)-1

		if i > 0 {
			microDelay := s.service.hsemEngine.Math().UniformDuration(200*time.Millisecond, 600*time.Millisecond)
			time.Sleep(microDelay)
		}

		msgID, err := s.service.ExecuteBurstStep(ctx, job.Payload, job.MediaData, !isLast)

		if job.Acknowledger != nil {
			if err == nil {

				if job.Payload.CorrelationID != "" {
					evt := domain.MessageSentEvent{
						EventType:     "message.sent",
						CorrelationID: job.Payload.CorrelationID,
						Timestamp:     time.Now(),
						Data: domain.MessageSentData{
							MessageID: msgID,
							SessionID: job.Payload.SessionID,
							JID:       job.Payload.RemoteJID,
							Status:    "SENT",
						},
					}

					if pubErr := s.service.publisher.Publish(ctx, "whatsapp.message.sent", evt); pubErr != nil {
						s.service.logger.Error().Err(pubErr).Msg("Failed to publish message.sent event")
					}
				}

				_ = job.Acknowledger.Ack()
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
