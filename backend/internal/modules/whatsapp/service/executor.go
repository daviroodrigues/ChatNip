package service

import (
	hsemDomain "chatnip/internal/modules/hsem/domain"
	whatsappDomain "chatnip/internal/modules/whatsapp/domain"
	"context"
	"github.com/rs/zerolog"
	"math/rand"
	"time"
)

type HSEMExecutor struct {
	logger zerolog.Logger
}

func NewHSEMExecutor(logger zerolog.Logger) *HSEMExecutor {
	return &HSEMExecutor{logger: logger}
}

func (e *HSEMExecutor) Execute(ctx context.Context, client whatsappDomain.Messenger, plan hsemDomain.FingerprintPlan, payload whatsappDomain.SendMessagePayload, mediaData []byte) (string, error) {
	startTime := time.Now()
	var messageID string

	for _, step := range plan.Steps {
		targetTime := startTime.Add(step.Timestamp)
		waitDuration := time.Until(targetTime)

		if waitDuration > 0 {
			select {
			case <-time.After(waitDuration):
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}

		switch step.Metadata {
		case "mark_as_read":
			msgToRead := payload.QuotedMsgID
			if msgToRead == "" {
				msgToRead = payload.TargetMsgID
			}
			if msgToRead != "" {
				if err := client.MarkAsRead(ctx, payload.RemoteJID, msgToRead); err != nil {
					e.logger.Warn().Err(err).Str("jid", payload.RemoteJID).Str("msg_id", msgToRead).Msg("Failed to mark message as read")
				} else {
					e.logger.Debug().Str("jid", payload.RemoteJID).Str("msg_id", msgToRead).Msg("HSEM: Mark as Read sent")
				}
			}

		case "composing":
			if !payload.IsBroadcastOrChannelTarget() {
				if err := client.SendPresence(ctx, payload.RemoteJID, "composing"); err != nil {
					e.logger.Warn().Err(err).Msg("Failed to send composing presence")
				}
			}

		case "recording":
			if !payload.IsBroadcastOrChannelTarget() {
				if err := client.SendPresence(ctx, payload.RemoteJID, "recording"); err != nil {
					e.logger.Warn().Err(err).Msg("Failed to send recording presence")
				}
			}

		case "paused":
			if !payload.IsBroadcastOrChannelTarget() {
				if err := client.SendPresence(ctx, payload.RemoteJID, "paused"); err != nil {
					e.logger.Warn().Err(err).Msg("Failed to send paused presence")
				}
			}

		case "send_message":
			switch payload.Action {
			case "edit":
				id, err := client.EditMessage(ctx, payload.RemoteJID, payload.TargetMsgID, payload.Text)
				if err != nil {
					e.logger.Error().Err(err).Msg("Failed to edit message")
					return "", err
				}
				messageID = id

			case "reaction", "react":
				id, err := client.SendReaction(ctx, payload.RemoteJID, payload.TargetParticipant, payload.TargetMsgID, payload.Reaction)
				if err != nil {
					e.logger.Error().Err(err).Msg("Failed to send reaction")
					return "", err
				}
				messageID = id

			case "revoke", "delete":
				id, err := client.RevokeMessage(ctx, payload.RemoteJID, payload.TargetParticipant, payload.TargetMsgID)
				if err != nil {
					e.logger.Error().Err(err).Msg("Failed to revoke message")
					return "", err
				}
				messageID = id

			case "presence":
				if !payload.IsBroadcastOrChannelTarget() {
					state := payload.PresenceState
					if state == "" {
						state = payload.Text
					}
					if state == "" {
						state = "composing"
					}
					if err := client.SendPresence(ctx, payload.RemoteJID, state); err != nil {
						e.logger.Error().Err(err).Msg("Failed to send chat presence")
						return "", err
					}
				}

			case "reject_call":
				if err := client.RejectCall(ctx, payload.RemoteJID, payload.TargetMsgID); err != nil {
					e.logger.Error().Err(err).Str("call_id", payload.TargetMsgID).Msg("Failed to reject call")
					return "", err
				}
				messageID = payload.TargetMsgID

			case "read", "mark_read":
				msgToRead := payload.TargetMsgID
				if msgToRead == "" {
					msgToRead = payload.QuotedMsgID
				}
				if err := client.MarkAsRead(ctx, payload.RemoteJID, msgToRead); err != nil {
					e.logger.Error().Err(err).Msg("Failed to mark message as read")
					return "", err
				}
				messageID = msgToRead

			default:
				if payload.MediaURL != "" && (len(mediaData) > 0 || payload.MediaFilePath != "") {
					finalDuration := payload.Duration
					finalWaveform := payload.Waveform

					if payload.MediaType == "audio" && payload.IsPTT {
						if finalDuration == 0 {
							finalDuration = 10
						}
						if len(finalWaveform) == 0 {
							finalWaveform = make([]byte, 64)
							for i := range finalWaveform {
								finalWaveform[i] = byte(rand.Intn(100))
							}
						}
					}

					mediaInput := whatsappDomain.MediaMessageInput{
						Type:              payload.MediaType,
						Caption:           payload.Text,
						MediaData:         mediaData,
						MediaFilePath:     payload.MediaFilePath,
						MediaFileSize:     payload.MediaFileSize,
						MimeType:          payload.MediaMimeType,
						FileName:          payload.FileName,
						IsPTT:             payload.IsPTT,
						IsGIF:             payload.IsGIF,
						Duration:          finalDuration,
						Waveform:          finalWaveform,
						QuotedMsgID:       payload.QuotedMsgID,
						QuotedParticipant: payload.QuotedParticipant,
					}
					id, err := client.SendMedia(ctx, payload.RemoteJID, mediaInput)
					if err != nil {
						e.logger.Error().Err(err).Msg("Failed to send media message")
						return "", err
					}
					messageID = id

				} else {
					textInput := whatsappDomain.TextMessageInput{
						Text:              payload.Text,
						QuotedMsgID:       payload.QuotedMsgID,
						QuotedParticipant: payload.QuotedParticipant,
					}
					id, err := client.SendMessage(ctx, payload.RemoteJID, textInput)
					if err != nil {
						e.logger.Error().Err(err).Msg("Failed to send text message")
						return "", err
					}
					messageID = id
				}
			}

			if !payload.IsBroadcastOrChannelTarget() {
				_ = client.SendPresence(ctx, payload.RemoteJID, "paused")
			}
		}
	}

	return messageID, nil
}
