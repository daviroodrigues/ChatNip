package infrastructure

import (
	"chatnip/internal/modules/whatsapp/domain"
	"context"
	"errors"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
	"net/http"
	"strings"
	"time"
)

type WhatsMeowAdapter struct {
	client *whatsmeow.Client
}

func NewWhatsMeowAdapter(client *whatsmeow.Client) *WhatsMeowAdapter {
	return &WhatsMeowAdapter{client: client}
}

func (w *WhatsMeowAdapter) Connect() error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	return w.client.Connect()
}

func (w *WhatsMeowAdapter) Disconnect() {
	if w.client != nil {
		w.client.Disconnect()
	}
}

func (w *WhatsMeowAdapter) IsConnected() bool {
	return w.client != nil && w.client.IsConnected()
}

func (w *WhatsMeowAdapter) IsLoggedIn() bool {
	return w.client != nil && w.client.Store.ID != nil
}

func (w *WhatsMeowAdapter) GetJID() string {
	if w.client == nil || w.client.Store.ID == nil {
		return ""
	}
	return w.client.Store.ID.String()
}

func (w *WhatsMeowAdapter) Logout(ctx context.Context) error {
	if w.client == nil {
		return nil
	}
	return w.client.Logout(ctx)
}

func (w *WhatsMeowAdapter) GetQRChannel(ctx context.Context) (<-chan string, error) {
	if w.IsLoggedIn() {
		return nil, errors.New("client already logged in")
	}

	qrChan, err := w.client.GetQRChannel(ctx)
	if err != nil {
		return nil, err
	}

	out := make(chan string)

	go func() {
		defer close(out)
		for {
			select {
			case evt, ok := <-qrChan:
				if !ok {
					return
				}
				if evt.Event == "code" {
					select {
					case out <- evt.Code:
					case <-ctx.Done():
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func (w *WhatsMeowAdapter) PairPhone(ctx context.Context, phone string) (string, error) {
	if w.client == nil {
		return "", errors.New("client not initialized")
	}
	if w.IsLoggedIn() {
		return "", errors.New("client already logged in")
	}
	return w.client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
}

func (w *WhatsMeowAdapter) RegisterHandler(handler domain.EventHandler) {
	w.client.AddEventHandler(func(rawEvt interface{}) {
		if domainEvt := w.mapToDomainEvent(rawEvt); domainEvt != nil {
			handler(domainEvt)
		}
	})
}

func (w *WhatsMeowAdapter) SendPresence(ctx context.Context, jidStr string, state string) error {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return err
	}

	var pState types.ChatPresence
	var pMedia types.ChatPresenceMedia

	switch state {
	case "composing":
		pState = types.ChatPresenceComposing
		pMedia = types.ChatPresenceMediaText
	case "recording":
		pState = types.ChatPresenceComposing
		pMedia = types.ChatPresenceMediaAudio
	case "paused":
		pState = types.ChatPresencePaused
		pMedia = types.ChatPresenceMediaText
	default:
		return nil
	}

	return w.client.SendChatPresence(ctx, jid, pState, pMedia)
}

func (w *WhatsMeowAdapter) SendGlobalPresence(ctx context.Context, available bool) error {
	if w.client == nil {
		return errors.New("client not initialized")
	}
	if available {
		return w.client.SendPresence(ctx, types.PresenceAvailable)
	}
	return w.client.SendPresence(ctx, types.PresenceUnavailable)
}

func (w *WhatsMeowAdapter) SendMessage(ctx context.Context, jidStr string, input domain.TextMessageInput) (string, error) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return "", err
	}

	msg := &waE2E.Message{}

	if input.QuotedMsgID != "" {
		msg.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
			Text:        proto.String(input.Text),
			ContextInfo: buildContextInfo(input.QuotedMsgID, input.QuotedParticipant),
		}
	} else {
		msg.Conversation = proto.String(input.Text)
	}

	resp, err := w.client.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (w *WhatsMeowAdapter) EditMessage(ctx context.Context, jidStr string, messageID string, newText string) (string, error) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return "", err
	}
	editMsg := w.client.BuildEdit(jid, types.MessageID(messageID), &waE2E.Message{
		Conversation: proto.String(newText),
	})
	resp, err := w.client.SendMessage(ctx, jid, editMsg)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (w *WhatsMeowAdapter) SendReaction(ctx context.Context, jidStr string, senderJIDStr string, messageID string, reaction string) (string, error) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return "", err
	}
	var senderJID types.JID
	if senderJIDStr != "" {
		senderJID, _ = types.ParseJID(senderJIDStr)
	}
	reactionMsg := w.client.BuildReaction(jid, senderJID, types.MessageID(messageID), reaction)
	resp, err := w.client.SendMessage(ctx, jid, reactionMsg)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (w *WhatsMeowAdapter) RevokeMessage(ctx context.Context, jidStr string, senderJIDStr string, messageID string) (string, error) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return "", err
	}
	var senderJID types.JID
	if senderJIDStr != "" {
		senderJID, _ = types.ParseJID(senderJIDStr)
	}
	revokeMsg := w.client.BuildRevoke(jid, senderJID, types.MessageID(messageID))
	resp, err := w.client.SendMessage(ctx, jid, revokeMsg)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func (w *WhatsMeowAdapter) SendMedia(ctx context.Context, jidStr string, input domain.MediaMessageInput) (string, error) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return "", err
	}

	var appMedia whatsmeow.MediaType
	switch input.Type {
	case "image", "sticker":
		appMedia = whatsmeow.MediaImage
	case "video":
		appMedia = whatsmeow.MediaVideo
	case "audio":
		appMedia = whatsmeow.MediaAudio
	case "document":
		appMedia = whatsmeow.MediaDocument
	default:
		return "", errors.New("unsupported media type")
	}

	isNewsletter := jid.Server == types.NewsletterServer

	var uploaded whatsmeow.UploadResponse
	if isNewsletter {
		uploaded, err = w.client.UploadNewsletter(ctx, input.MediaData, appMedia)
	} else {
		uploaded, err = w.client.Upload(ctx, input.MediaData, appMedia)
	}
	if err != nil {
		return "", err
	}

	ctxInfo := buildContextInfo(input.QuotedMsgID, input.QuotedParticipant)

	msg := &waE2E.Message{}

	var mediaKey []byte
	var fileEncSHA256 []byte
	if !isNewsletter {
		mediaKey = uploaded.MediaKey
		fileEncSHA256 = uploaded.FileEncSHA256
	}

	switch input.Type {
	case "image":
		msg.ImageMessage = &waE2E.ImageMessage{
			Caption:       proto.String(input.Caption),
			Mimetype:      proto.String(http.DetectContentType(input.MediaData)),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      mediaKey,
			FileEncSHA256: fileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			ContextInfo:   ctxInfo,
		}

	case "video":
		msg.VideoMessage = &waE2E.VideoMessage{
			Caption:       proto.String(input.Caption),
			Mimetype:      proto.String(http.DetectContentType(input.MediaData)),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      mediaKey,
			FileEncSHA256: fileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			GifPlayback:   proto.Bool(input.IsGIF),
			ContextInfo:   ctxInfo,
		}

	case "audio":
		audioMsg := &waE2E.AudioMessage{
			Mimetype:      proto.String("audio/ogg; codecs=opus"),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      mediaKey,
			FileEncSHA256: fileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			PTT:           proto.Bool(input.IsPTT),
			ContextInfo:   ctxInfo,
		}

		if input.Duration > 0 {
			audioMsg.Seconds = proto.Uint32(input.Duration)
		}
		if len(input.Waveform) > 0 {
			audioMsg.Waveform = input.Waveform
		}

		msg.AudioMessage = audioMsg

	case "document":
		msg.DocumentMessage = &waE2E.DocumentMessage{
			Caption:       proto.String(input.Caption),
			Mimetype:      proto.String(http.DetectContentType(input.MediaData)),
			Title:         proto.String(input.FileName),
			FileName:      proto.String(input.FileName),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      mediaKey,
			FileEncSHA256: fileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			ContextInfo:   ctxInfo,
		}

	case "sticker":
		msg.StickerMessage = &waE2E.StickerMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      mediaKey,
			FileEncSHA256: fileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Mimetype:      proto.String("image/webp"),
			ContextInfo:   ctxInfo,
		}
	}

	var resp whatsmeow.SendResponse
	if isNewsletter {
		resp, err = w.client.SendMessage(ctx, jid, msg, whatsmeow.SendRequestExtra{
			MediaHandle: uploaded.Handle,
		})
	} else {
		resp, err = w.client.SendMessage(ctx, jid, msg)
	}
	if err != nil {
		return "", err
	}

	return resp.ID, nil
}

func (w *WhatsMeowAdapter) MarkAsRead(ctx context.Context, jidStr string, msgID string) error {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return err
	}

	ids := extractMessageIDs(msgID)
	if len(ids) == 0 {
		return nil
	}

	return w.client.MarkRead(ctx, ids, time.Now(), jid, jid, types.ReceiptTypeRead)
}

func extractMessageIDs(id string) []types.MessageID {
	if id == "" {
		return nil
	}
	return []types.MessageID{types.MessageID(id)}
}

func (w *WhatsMeowAdapter) mapToDomainEvent(rawEvt interface{}) domain.Event {
	switch evt := rawEvt.(type) {
	case *events.PairSuccess:
		return domain.PairSuccessEvent{
			JID:       evt.ID.String(),
			Timestamp: time.Now(),
		}
	case *events.LoggedOut:
		return domain.LoggedOutEvent{
			Reason:    evt.Reason.String(),
			Timestamp: time.Now(),
		}
	case *events.Message:
		return w.mapMessageEvent(evt)
	case *events.HistorySync:
		return w.mapHistorySync(evt)
	case *events.Receipt:
		status := "UNKNOWN"
		switch evt.Type {
		case types.ReceiptTypeDelivered:
			status = "DELIVERED"
		case types.ReceiptTypeRead, types.ReceiptTypeReadSelf:
			status = "READ"
		case types.ReceiptTypePlayed:
			status = "PLAYED"
		default:
			return nil
		}

		return domain.MessageReceiptEvent{
			ChatJID:    evt.Chat.String(),
			SenderJID:  evt.Sender.String(),
			MessageIDs: evt.MessageIDs,
			Status:     status,
			Timestamp:  evt.Timestamp,
		}
	case *events.ChatPresence:
		return domain.ChatPresenceEvent{
			ChatJID:   evt.Chat.String(),
			SenderJID: evt.Sender.String(),
			State:     string(evt.State),
			Timestamp: time.Now(),
		}
	case *events.CallOffer:
		caller := evt.CallCreator.String()
		if caller == "" {
			caller = evt.From.String()
		}
		return domain.CallOfferEvent{
			CallID:    evt.CallID,
			CallerJID: caller,
			IsGroup:   !evt.GroupJID.IsEmpty(),
			Timestamp: evt.Timestamp,
		}
	case *events.UndecryptableMessage:
		return domain.UndecryptableMessageEvent{
			MessageID: evt.Info.ID,
			ChatJID:   evt.Info.Chat.String(),
			SenderJID: evt.Info.Sender.String(),
			Timestamp: evt.Info.Timestamp,
		}
	case *events.GroupInfo:
		sender := ""
		if evt.Sender != nil {
			sender = evt.Sender.String()
		}
		return domain.GroupInfoEvent{
			GroupJID:  evt.JID.String(),
			SenderJID: sender,
			Action:    "update",
			Timestamp: evt.Timestamp,
		}
	default:
		return nil
	}
}

func (w *WhatsMeowAdapter) mapMessageEvent(evt *events.Message) domain.Event {
	if evt.Info.Timestamp.IsZero() {
		return nil
	}

	var senderAltJID string
	if !evt.Info.SenderAlt.IsEmpty() {
		senderAltJID = evt.Info.SenderAlt.String()
	}

	var chatJID string
	if !evt.Info.Chat.IsEmpty() {
		chatJID = evt.Info.Chat.String()
	}

	return domain.MessageEvent{
		ID:           evt.Info.ID,
		ChatJID:      chatJID,
		SenderJID:    evt.Info.Sender.String(),
		SenderAltJID: senderAltJID,
		SenderPhone:  extractSenderPhone(evt.Info.Sender, evt.Info.SenderAlt),
		IsGroup:      evt.Info.IsGroup,
		PushName:     evt.Info.PushName,
		IsFromMe:     evt.Info.IsFromMe,
		Timestamp:    evt.Info.Timestamp,
		Text:         extractMessageBody(evt),
	}
}

// extractSenderPhone returns the phone number digits from the sender's PN JID,
// falling back to SenderAlt when the primary Sender uses LID addressing.
func extractSenderPhone(sender, senderAlt types.JID) string {
	if sender.Server == types.DefaultUserServer && sender.User != "" {
		return sender.User
	}
	if senderAlt.Server == types.DefaultUserServer && senderAlt.User != "" {
		return senderAlt.User
	}
	return ""
}

func (w *WhatsMeowAdapter) mapHistorySync(evt *events.HistorySync) domain.Event {
	var messages []domain.MessageEvent
	const maxMessagesPerChat = 50

	for _, conv := range evt.Data.GetConversations() {
		rawMsgs := conv.GetMessages()
		if len(rawMsgs) == 0 {
			continue
		}

		startIndex := 0
		if len(rawMsgs) > maxMessagesPerChat {
			startIndex = len(rawMsgs) - maxMessagesPerChat
		}
		recentMsgs := rawMsgs[startIndex:]

		for _, historyMsg := range recentMsgs {
			ts := time.Unix(int64(historyMsg.GetMessage().GetMessageTimestamp()), 0)

			rawEvt := &events.Message{
				Info: types.MessageInfo{
					ID: historyMsg.GetMessage().GetKey().GetID(),
					MessageSource: types.MessageSource{
						Chat:     types.NewJID(conv.GetID(), types.DefaultUserServer),
						Sender:   types.NewJID(historyMsg.GetMessage().GetKey().GetParticipant(), types.DefaultUserServer),
						IsFromMe: historyMsg.GetMessage().GetKey().GetFromMe(),
					},
					Timestamp: ts,
				},
				Message: historyMsg.GetMessage().GetMessage(),
			}

			if domainEvt := w.mapMessageEvent(rawEvt); domainEvt != nil {
				if msg, ok := domainEvt.(domain.MessageEvent); ok {
					messages = append(messages, msg)
				}
			}
		}
	}

	if len(messages) == 0 {
		return nil
	}

	return domain.HistorySyncEvent{Messages: messages}
}

func extractMessageBody(evt *events.Message) string {
	msg := evt.Message
	if msg == nil {
		return ""
	}
	if len(msg.GetConversation()) > 0 {
		return msg.GetConversation()
	}
	if extended := msg.GetExtendedTextMessage(); extended != nil {
		return extended.GetText()
	}
	if img := msg.GetImageMessage(); img != nil {
		return img.GetCaption()
	}
	if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	}
	return "[Mídia]"
}

func (w *WhatsMeowAdapter) CreateGroup(ctx context.Context, name string, participants []string) (string, error) {
	if w.client == nil {
		return "", errors.New("client not initialized")
	}

	var participantJIDs []types.JID
	for _, p := range participants {
		jidStr := p
		if !strings.Contains(jidStr, "@") {
			jidStr += "@s.whatsapp.net"
		}
		parsed, err := types.ParseJID(jidStr)
		if err != nil {
			continue
		}
		participantJIDs = append(participantJIDs, parsed)
	}

	req := whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: participantJIDs,
	}

	groupInfo, err := w.client.CreateGroup(ctx, req)
	if err != nil {
		return "", err
	}

	return groupInfo.JID.String(), nil
}

func (w *WhatsMeowAdapter) ListGroups(ctx context.Context) ([]domain.Group, error) {
	if w.client == nil {
		return nil, errors.New("client not initialized")
	}

	groups, err := w.client.GetJoinedGroups(context.Background())
	if err != nil {
		return nil, err
	}

	var result []domain.Group
	for _, g := range groups {
		result = append(result, domain.Group{
			JID:      g.JID.String(),
			Name:     g.GroupName.Name,
			OwnerJID: g.OwnerJID.String(),
			Count:    len(g.Participants),
		})
	}

	return result, nil
}

func buildContextInfo(quotedID, quotedParticipant string) *waE2E.ContextInfo {
	if quotedID == "" {
		return nil
	}
	return &waE2E.ContextInfo{
		StanzaID:    proto.String(quotedID),
		Participant: proto.String(quotedParticipant),
		// Nota: Para um preview completo, precisaríamos do objeto da mensagem original,
		// mas apenas ID e Participant já criam o link de resposta ("Reply").
	}
}
