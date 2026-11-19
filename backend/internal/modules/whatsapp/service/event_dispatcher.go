package service

import (
	"chatnip/internal/modules/whatsapp/domain"
	"context"
	"github.com/rs/zerolog"
	"time"
)

type EventHandler interface {
	Handle(ctx context.Context, evt domain.Event, sessionID string, tenantID string)
	CanHandle(evt domain.Event) bool
}

type EventDispatcher struct {
	handlers []EventHandler
}

func NewEventDispatcher(handlers ...EventHandler) *EventDispatcher {
	return &EventDispatcher{handlers: handlers}
}

func (d *EventDispatcher) Dispatch(ctx context.Context, evt domain.Event, sessionID string, tenantID string) {
	for _, h := range d.handlers {
		if h.CanHandle(evt) {
			h.Handle(ctx, evt, sessionID, tenantID)
		}
	}
}

type PairSuccessHandler struct {
	repo   domain.WhatsAppRepository
	logger zerolog.Logger
}

func NewPairSuccessHandler(repo domain.WhatsAppRepository, logger zerolog.Logger) *PairSuccessHandler {
	return &PairSuccessHandler{repo: repo, logger: logger}
}

func (h *PairSuccessHandler) CanHandle(evt domain.Event) bool {
	return evt.EventType() == domain.TypePairSuccess
}

func (h *PairSuccessHandler) Handle(ctx context.Context, evt domain.Event, sessionID string, tenantID string) {
	e, ok := evt.(domain.PairSuccessEvent)
	if !ok {
		return
	}

	time.Sleep(100 * time.Millisecond)

	existing, _ := h.repo.GetMapping(ctx, sessionID)
	proxyURL := ""
	if existing != nil {
		proxyURL = existing.ProxyURL
	}

	mapping := &domain.Session{
		ID:       sessionID,
		TenantID: tenantID,
		JID:      e.JID,
		ProxyURL: proxyURL,
	}

	err := h.repo.SaveMapping(ctx, mapping)
	if err != nil {
		h.logger.Error().Err(err).Str("session_id", sessionID).Msg("Failed to save session mapping")
		return
	}

	h.logger.Info().
		Str("session_id", sessionID).
		Str("tenant_id", tenantID).
		Str("jid", e.JID).
		Msg("Device paired successfully and saved")
}

type MessageHandler struct {
	logger      zerolog.Logger
	publisher   domain.EventPublisher
	mediaIngest *MediaIngestService
}

func NewMessageHandler(logger zerolog.Logger, publisher domain.EventPublisher, mediaIngest *MediaIngestService) *MessageHandler {
	return &MessageHandler{
		logger:      logger,
		publisher:   publisher,
		mediaIngest: mediaIngest,
	}
}

func (h *MessageHandler) CanHandle(evt domain.Event) bool {
	return evt.EventType() == domain.TypeMessage
}

func (h *MessageHandler) Handle(ctx context.Context, evt domain.Event, sessionID string, tenantID string) {
	e, ok := evt.(domain.MessageEvent)
	if !ok {
		return
	}

	e.SessionID = sessionID
	e.TenantID = tenantID

	if e.RawMedia != nil && h.mediaIngest != nil {
		h.logger.Info().
			Str("session_id", sessionID).
			Str("message_id", e.ID).
			Str("media_type", e.RawMedia.MediaType).
			Msg("Incoming media detected, starting disk download & presigned PUT pipeline")
		h.mediaIngest.IngestIncomingMedia(ctx, e)
		return
	}

	h.logger.Info().
		Str("session_id", sessionID).
		Str("sender", e.SenderJID).
		Msg("Message received, publishing to queue")

	envelope := domain.NewEventEnvelope("message.received", e.ID, tenantID, sessionID, e)
	err := h.publisher.Publish(ctx, "whatsapp.message.received", envelope)
	if err != nil {
		h.logger.Error().Err(err).
			Str("session_id", sessionID).
			Msg("Failed to publish incoming message to RabbitMQ")
	}
}

type HistorySyncHandler struct {
	logger    zerolog.Logger
	publisher domain.EventPublisher
}

func NewHistorySyncHandler(logger zerolog.Logger, publisher domain.EventPublisher) *HistorySyncHandler {
	return &HistorySyncHandler{
		logger:    logger,
		publisher: publisher,
	}
}

func (h *HistorySyncHandler) CanHandle(evt domain.Event) bool {
	return evt.EventType() == domain.TypeHistorySync
}

func (h *HistorySyncHandler) Handle(ctx context.Context, evt domain.Event, sessionID string, tenantID string) {
	e, ok := evt.(domain.HistorySyncEvent)
	if !ok {
		return
	}

	h.logger.Info().
		Str("session_id", sessionID).
		Int("msg_count", len(e.Messages)).
		Msg("Processing history sync (last 50 msgs)")

	for _, msg := range e.Messages {
		msg.SessionID = sessionID
		msg.TenantID = tenantID
		envelope := domain.NewEventEnvelope("message.received", msg.ID, tenantID, sessionID, msg)
		_ = h.publisher.Publish(ctx, "whatsapp.message.received", envelope)
	}
}

type ReceiptHandler struct {
	logger    zerolog.Logger
	publisher domain.EventPublisher
}

func NewReceiptHandler(logger zerolog.Logger, publisher domain.EventPublisher) *ReceiptHandler {
	return &ReceiptHandler{logger: logger, publisher: publisher}
}

func (h *ReceiptHandler) CanHandle(evt domain.Event) bool {
	return evt.EventType() == domain.TypeReceipt
}

func (h *ReceiptHandler) Handle(ctx context.Context, evt domain.Event, sessionID string, tenantID string) {
	e, ok := evt.(domain.MessageReceiptEvent)
	if !ok {
		return
	}

	e.SessionID = sessionID
	e.TenantID = tenantID

	envelope := domain.NewEventEnvelope("message.status", "", tenantID, sessionID, e)

	err := h.publisher.Publish(ctx, "whatsapp.message.status", envelope)
	if err != nil {
		h.logger.Error().Err(err).
			Str("session_id", sessionID).
			Msg("Failed to publish message receipt to RabbitMQ")
	} else {
		h.logger.Debug().
			Str("session_id", sessionID).
			Str("status", e.Status).
			Int("count", len(e.MessageIDs)).
			Msg("Message status published")
	}
}

type ChatPresenceHandler struct {
	logger    zerolog.Logger
	publisher domain.EventPublisher
}

func NewChatPresenceHandler(logger zerolog.Logger, publisher domain.EventPublisher) *ChatPresenceHandler {
	return &ChatPresenceHandler{logger: logger, publisher: publisher}
}

func (h *ChatPresenceHandler) CanHandle(evt domain.Event) bool {
	return evt.EventType() == domain.TypeChatPresence
}

func (h *ChatPresenceHandler) Handle(ctx context.Context, evt domain.Event, sessionID string, tenantID string) {
	e, ok := evt.(domain.ChatPresenceEvent)
	if !ok {
		return
	}
	e.SessionID = sessionID
	e.TenantID = tenantID

	envelope := domain.NewEventEnvelope("chat.presence", "", tenantID, sessionID, e)

	if err := h.publisher.Publish(ctx, "whatsapp.chat.presence", envelope); err != nil {
		h.logger.Error().Err(err).Str("session_id", sessionID).Msg("Failed to publish chat presence to RabbitMQ")
	}
}

type CallOfferHandler struct {
	logger    zerolog.Logger
	publisher domain.EventPublisher
}

func NewCallOfferHandler(logger zerolog.Logger, publisher domain.EventPublisher) *CallOfferHandler {
	return &CallOfferHandler{logger: logger, publisher: publisher}
}

func (h *CallOfferHandler) CanHandle(evt domain.Event) bool {
	return evt.EventType() == domain.TypeCallOffer
}

func (h *CallOfferHandler) Handle(ctx context.Context, evt domain.Event, sessionID string, tenantID string) {
	e, ok := evt.(domain.CallOfferEvent)
	if !ok {
		return
	}
	e.SessionID = sessionID
	e.TenantID = tenantID

	envelope := domain.NewEventEnvelope("call.received", "", tenantID, sessionID, e)

	if err := h.publisher.Publish(ctx, "whatsapp.call.received", envelope); err != nil {
		h.logger.Error().Err(err).Str("session_id", sessionID).Msg("Failed to publish call offer to RabbitMQ")
	}
}

type CallTerminateHandler struct {
	logger    zerolog.Logger
	publisher domain.EventPublisher
}

func NewCallTerminateHandler(logger zerolog.Logger, publisher domain.EventPublisher) *CallTerminateHandler {
	return &CallTerminateHandler{logger: logger, publisher: publisher}
}

func (h *CallTerminateHandler) CanHandle(evt domain.Event) bool {
	return evt.EventType() == domain.TypeCallTerminate
}

func (h *CallTerminateHandler) Handle(ctx context.Context, evt domain.Event, sessionID string, tenantID string) {
	e, ok := evt.(domain.CallTerminateEvent)
	if !ok {
		return
	}
	e.SessionID = sessionID
	e.TenantID = tenantID

	envelope := domain.NewEventEnvelope("call.terminated", "", tenantID, sessionID, e)

	if err := h.publisher.Publish(ctx, "whatsapp.call.terminated", envelope); err != nil {
		h.logger.Error().Err(err).Str("session_id", sessionID).Msg("Failed to publish call terminate to RabbitMQ")
	}
}

type UndecryptableMessageHandler struct {
	logger    zerolog.Logger
	publisher domain.EventPublisher
}

func NewUndecryptableMessageHandler(logger zerolog.Logger, publisher domain.EventPublisher) *UndecryptableMessageHandler {
	return &UndecryptableMessageHandler{logger: logger, publisher: publisher}
}

func (h *UndecryptableMessageHandler) CanHandle(evt domain.Event) bool {
	return evt.EventType() == domain.TypeUndecryptable
}

func (h *UndecryptableMessageHandler) Handle(ctx context.Context, evt domain.Event, sessionID string, tenantID string) {
	e, ok := evt.(domain.UndecryptableMessageEvent)
	if !ok {
		return
	}
	e.SessionID = sessionID
	e.TenantID = tenantID

	envelope := domain.NewEventEnvelope("message.undecryptable", "", tenantID, sessionID, e)

	if err := h.publisher.Publish(ctx, "whatsapp.message.undecryptable", envelope); err != nil {
		h.logger.Error().Err(err).Str("session_id", sessionID).Msg("Failed to publish undecryptable message event")
	}
}

type GroupInfoHandler struct {
	logger    zerolog.Logger
	publisher domain.EventPublisher
}

func NewGroupInfoHandler(logger zerolog.Logger, publisher domain.EventPublisher) *GroupInfoHandler {
	return &GroupInfoHandler{logger: logger, publisher: publisher}
}

func (h *GroupInfoHandler) CanHandle(evt domain.Event) bool {
	return evt.EventType() == domain.TypeGroupInfo
}

func (h *GroupInfoHandler) Handle(ctx context.Context, evt domain.Event, sessionID string, tenantID string) {
	e, ok := evt.(domain.GroupInfoEvent)
	if !ok {
		return
	}
	e.SessionID = sessionID
	e.TenantID = tenantID

	envelope := domain.NewEventEnvelope("group.updated", "", tenantID, sessionID, e)

	if err := h.publisher.Publish(ctx, "whatsapp.group.updated", envelope); err != nil {
		h.logger.Error().Err(err).Str("session_id", sessionID).Msg("Failed to publish group info event")
	}
}
