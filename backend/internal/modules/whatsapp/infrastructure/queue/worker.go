package queue

import (
	"chatnip/internal/kit/queue"
	"chatnip/internal/modules/whatsapp/domain"
	"chatnip/internal/modules/whatsapp/service"
	"context"
	"encoding/json"
	"errors"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/rs/zerolog"
	"sync"
	"time"
)

const MaxRetries = 5

type WhatsAppWorker struct {
	svc     *service.WhatsAppService
	adapter *queue.RabbitMQAdapter
	logger  zerolog.Logger
}

type RabbitMQAcknowledger struct {
	Delivery amqp.Delivery
}

func (r *RabbitMQAcknowledger) Ack() error {
	return r.Delivery.Ack(false)
}

func (r *RabbitMQAcknowledger) Nack(requeue bool) error {
	return r.Delivery.Nack(false, requeue)
}

func NewWhatsAppWorker(svc *service.WhatsAppService, adapter *queue.RabbitMQAdapter, logger zerolog.Logger) *WhatsAppWorker {
	return &WhatsAppWorker{
		svc:     svc,
		adapter: adapter,
		logger:  logger,
	}
}

func (w *WhatsAppWorker) Start(ctx context.Context, queueName string, workerCount int) {
	msgs, err := w.adapter.Consume(queueName)
	if err != nil {
		w.logger.Fatal().Err(err).Str("queue", queueName).Msg("Failed to consume from queue")
	}

	closeErr := w.adapter.NotifyClose()
	go func() {
		err := <-closeErr
		if err != nil {

			w.logger.Fatal().Err(err).Msg("RabbitMQ connection closed abruptly. Restarting application...")
		}
	}()

	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			w.processMessages(ctx, msgs, workerID)
			w.logger.Warn().Int("worker_id", workerID).Msg("Worker stopped consuming")
		}(i)
	}

}

func (w *WhatsAppWorker) processMessages(ctx context.Context, msgs <-chan amqp.Delivery, workerID int) {
	for d := range msgs {
		var payload domain.SendMessagePayload
		if err := json.Unmarshal(d.Body, &payload); err != nil {
			w.logger.Error().Err(err).Msg("Invalid message body")
			if nackErr := d.Reject(false); nackErr != nil {
				w.logger.Error().Err(nackErr).Msg("Failed to reject invalid message")
			}
			continue
		}

		if payload.CorrelationID == "" {
			w.logger.Error().
				Str("session_id", payload.SessionID).
				Str("jid", payload.RemoteJID).
				Msg("Message rejected: Missing CorrelationID (UUID)")

			if nackErr := d.Nack(false, false); nackErr != nil {
				w.logger.Error().Err(nackErr).Msg("Failed to nack missing correlation ID")
			}
			continue
		}

		ackWrapper := &RabbitMQAcknowledger{Delivery: d}
		err := w.svc.ProcessBackgroundMessage(ctx, payload, ackWrapper)

		if err != nil {
			if errors.Is(err, domain.ErrSessionNotFound) {
				w.logger.Warn().Str("session_id", payload.SessionID).Msg("Session not found, discarding")
				if ackErr := d.Ack(false); ackErr != nil {
					w.logger.Error().Err(ackErr).Msg("Failed to ack on SessionNotFound")
				}
				continue
			}

			if errors.Is(err, domain.ErrSessionLoading) {
				w.logger.Debug().Str("session_id", payload.SessionID).Msg("Session loading, requeuing message...")
				time.Sleep(1 * time.Second)
				if nackErr := d.Nack(false, true); nackErr != nil {
					w.logger.Error().Err(nackErr).Msg("Failed to nack on SessionLoading")
				}
				continue
			}

			retryCount := 0
			if d.Headers != nil {
				if val, ok := d.Headers["x-retry-count"]; ok {
					switch v := val.(type) {
					case int:
						retryCount = v
					case int32:
						retryCount = int(v)
					case int64:
						retryCount = int(v)
					}
				}
			}

			if retryCount >= MaxRetries {
				w.logger.Error().
					Str("session_id", payload.SessionID).
					Int("retries", retryCount).
					Msg("Max retries reached. Moving to DLQ/Discarding")

				if nackErr := d.Nack(false, false); nackErr != nil {
					w.logger.Error().Err(nackErr).Msg("Failed to nack on MaxRetries")
				}

				if payload.CorrelationID != "" {
					failEvt := domain.MessageFailedEvent{
						EventType:     "message.failed",
						CorrelationID: payload.CorrelationID,
						Timestamp:     time.Now(),
						Data: domain.MessageFailedData{
							SessionID:    payload.SessionID,
							JID:          payload.RemoteJID,
							ErrorCode:    "MAX_RETRIES_EXCEEDED",
							ErrorMessage: err.Error(),
						},
					}

					if pubErr := w.adapter.Publish(ctx, "whatsapp.message.failed", failEvt); pubErr != nil {
						w.logger.Error().Err(pubErr).Msg("Failed to publish message.failed event")
					}
				}

			} else {
				w.logger.Info().
					Str("session_id", payload.SessionID).
					Int("retry", retryCount+1).
					Msg("Transient error. Republishing with incremented retry count")

				newHeaders := d.Headers
				if newHeaders == nil {
					newHeaders = make(amqp.Table)
				}
				newHeaders["x-retry-count"] = retryCount + 1

				pubErr := w.adapter.PublishWithHeaders(ctx, d.Exchange, d.RoutingKey, d.Body, newHeaders)

				if pubErr != nil {
					w.logger.Error().Err(pubErr).Msg("Failed to republish retry message")
					if nackErr := d.Nack(false, true); nackErr != nil {
						w.logger.Error().Err(nackErr).Msg("Failed to requeue after publish error")
					}
				} else {
					if ackErr := d.Ack(false); ackErr != nil {
						w.logger.Error().Err(ackErr).Msg("Failed to ack after republishing")
					}
				}
			}
		}
	}
}

func (w *WhatsAppWorker) StartGroupWorker(ctx context.Context, queueName string, workerCount int) {
	msgs, err := w.adapter.Consume(queueName)
	if err != nil {
		w.logger.Fatal().Err(err).Str("queue", queueName).Msg("Failed to consume group queue")
	}

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			w.processGroupMessages(ctx, msgs, workerID)
		}(i)
	}
}

func (w *WhatsAppWorker) processGroupMessages(ctx context.Context, msgs <-chan amqp.Delivery, workerID int) {
	for d := range msgs {
		var payload domain.CreateGroupPayload
		if err := json.Unmarshal(d.Body, &payload); err != nil {
			w.logger.Error().Err(err).Msg("Invalid group payload")
			d.Reject(false)
			continue
		}

		if payload.CorrelationID == "" {
			w.logger.Error().
				Str("session_id", payload.SessionID).
				Msg("Group creation rejected: Missing CorrelationID (UUID)")
			d.Nack(false, false)
			continue
		}

		groupJID, err := w.svc.CreateGroup(ctx, payload.SessionID, payload.Name, payload.Participants)

		if err == nil {

			evt := domain.GroupCreatedEvent{
				EventType:     "group.created",
				CorrelationID: payload.CorrelationID,
				Timestamp:     time.Now(),
				Data: domain.GroupCreatedData{
					GroupJID:  groupJID,
					SessionID: payload.SessionID,
					Name:      payload.Name,
				},
			}
			if pubErr := w.adapter.Publish(ctx, "whatsapp_events", evt); pubErr != nil {
				w.logger.Error().Err(pubErr).Msg("Failed to publish group.created")
			}
			d.Ack(false)

		} else {
			isPermanent := errors.Is(err, domain.ErrSessionNotFound) || errors.Is(err, domain.ErrSessionLoading)

			if isPermanent {
				w.logger.Error().Err(err).Str("session_id", payload.SessionID).Msg("Permanent failure creating group")
				d.Nack(false, false)

				failEvt := domain.GroupFailedEvent{
					EventType:     "group.failed",
					CorrelationID: payload.CorrelationID,
					Timestamp:     time.Now(),
					Data: domain.GroupFailedData{
						SessionID:    payload.SessionID,
						Name:         payload.Name,
						ErrorCode:    "GROUP_CREATE_FAILED",
						ErrorMessage: err.Error(),
					},
				}
				_ = w.adapter.Publish(ctx, "whatsapp_events", failEvt)

			} else {

				retryCount := 0
				if d.Headers != nil {
					if val, ok := d.Headers["x-retry-count"]; ok {
						switch v := val.(type) {
						case int:
							retryCount = v
						case int32:
							retryCount = int(v)
						case int64:
							retryCount = int(v)
						}
					}
				}

				if retryCount >= MaxRetries {
					w.logger.Error().
						Str("session_id", payload.SessionID).
						Int("retries", retryCount).
						Msg("Max retries reached for group creation. Moving to DLQ/Discarding")
					d.Nack(false, false)

					failEvt := domain.GroupFailedEvent{
						EventType:     "group.failed",
						CorrelationID: payload.CorrelationID,
						Timestamp:     time.Now(),
						Data: domain.GroupFailedData{
							SessionID:    payload.SessionID,
							Name:         payload.Name,
							ErrorCode:    "MAX_RETRIES_EXCEEDED",
							ErrorMessage: err.Error(),
						},
					}
					_ = w.adapter.Publish(ctx, "whatsapp_events", failEvt)

				} else {

					w.logger.Info().
						Str("session_id", payload.SessionID).
						Int("retry", retryCount+1).
						Msg("Transient error creating group. Republishing with incremented retry count")

					newHeaders := d.Headers
					if newHeaders == nil {
						newHeaders = make(amqp.Table)
					}
					newHeaders["x-retry-count"] = retryCount + 1

					pubErr := w.adapter.PublishWithHeaders(ctx, d.Exchange, d.RoutingKey, d.Body, newHeaders)

					if pubErr != nil {
						w.logger.Error().Err(pubErr).Msg("Failed to republish group retry message")
						d.Nack(false, true)
					} else {
						d.Ack(false)
					}
				}
			}
		}
	}
}
