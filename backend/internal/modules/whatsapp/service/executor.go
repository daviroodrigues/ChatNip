package service

import (
	hsemDomain "chatnip/internal/modules/hsem/domain"
	whatsappDomain "chatnip/internal/modules/whatsapp/domain"
	"context"
	"math/rand"
	"time"

	"github.com/rs/zerolog"
)

type HSEMExecutor struct {
	logger zerolog.Logger
}

func NewHSEMExecutor(logger zerolog.Logger) *HSEMExecutor {
	return &HSEMExecutor{logger: logger}
}

func (e *HSEMExecutor) Execute(ctx context.Context, client whatsappDomain.Messenger, plan hsemDomain.FingerprintPlan, payload whatsappDomain.SendMessagePayload, mediaData []byte) (string, error) {
	return e.ExecuteWithPreemption(ctx, ctx, client, plan, payload, mediaData)
}

func (e *HSEMExecutor) ExecuteWithPreemption(sendCtx, stepCtx context.Context, client whatsappDomain.Messenger, plan hsemDomain.FingerprintPlan, payload whatsappDomain.SendMessagePayload, mediaData []byte) (string, error) {
	startTime := time.Now()
	var messageID string
	activePresence := ""

	cleanupPresence := func() {
		if activePresence == "" || payload.IsBroadcastOrChannelTarget() {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := client.SendPresence(cleanupCtx, payload.RemoteJID, "paused"); err != nil {
			e.logger.Debug().Err(err).Str("jid", payload.RemoteJID).Msg("Failed to send cleanup paused presence on abort")
		}
		activePresence = ""
	}

	for _, step := range plan.Steps {
		targetTime := startTime.Add(step.Timestamp)
		waitDuration := time.Until(targetTime)

		if waitDuration > 0 {
			timer := time.NewTimer(waitDuration)
			select {
			case <-timer.C:
			case <-stepCtx.Done():
				timer.Stop()
				cleanupPresence()
				return "", stepCtx.Err()
			}
		}

		if err := stepCtx.Err(); err != nil {
			cleanupPresence()
			return "", err
		}

		switch step.Metadata {
		case "mark_as_read":
			msgToRead := payload.QuotedMsgID
			if msgToRead == "" {
				msgToRead = payload.TargetMsgID
			}
			if msgToRead != "" {
				if err := client.MarkAsRead(stepCtx, payload.RemoteJID, msgToRead); err != nil {
					e.logger.Warn().Err(err).Str("jid", payload.RemoteJID).Str("msg_id", msgToRead).Msg("Failed to mark message as read")
				} else {
					e.logger.Debug().Str("jid", payload.RemoteJID).Str("msg_id", msgToRead).Msg("HSEM: Mark as Read sent")
				}
			}

		case "composing":
			if !payload.IsBroadcastOrChannelTarget() {
				if err := client.SendPresence(stepCtx, payload.RemoteJID, "composing"); err != nil {
					e.logger.Warn().Err(err).Msg("Failed to send composing presence")
				} else {
					activePresence = "composing"
				}
			}

		case "recording":
			if !payload.IsBroadcastOrChannelTarget() {
				if err := client.SendPresence(stepCtx, payload.RemoteJID, "recording"); err != nil {
					e.logger.Warn().Err(err).Msg("Failed to send recording presence")
				} else {
					activePresence = "recording"
				}
			}

		case "paused":
			if !payload.IsBroadcastOrChannelTarget() {
				if err := client.SendPresence(stepCtx, payload.RemoteJID, "paused"); err != nil {
					e.logger.Warn().Err(err).Msg("Failed to send paused presence")
				}
				activePresence = ""
			}

		case "send_message":
			switch payload.Action {
			case "edit":
				id, err := client.EditMessage(sendCtx, payload.RemoteJID, payload.TargetMsgID, payload.Text)
				if err != nil {
					e.logger.Error().Err(err).Msg("Failed to edit message")
					cleanupPresence()
					return "", err
				}
				messageID = id

			case "reaction", "react":
				id, err := client.SendReaction(sendCtx, payload.RemoteJID, payload.TargetParticipant, payload.TargetMsgID, payload.Reaction)
				if err != nil {
					e.logger.Error().Err(err).Msg("Failed to send reaction")
					cleanupPresence()
					return "", err
				}
				messageID = id

			case "revoke", "delete":
				id, err := client.RevokeMessage(sendCtx, payload.RemoteJID, payload.TargetParticipant, payload.TargetMsgID)
				if err != nil {
					e.logger.Error().Err(err).Msg("Failed to revoke message")
					cleanupPresence()
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
					if err := client.SendPresence(sendCtx, payload.RemoteJID, state); err != nil {
						e.logger.Error().Err(err).Msg("Failed to send chat presence")
						return "", err
					}
				}

			case "reject_call":
				if err := client.RejectCall(sendCtx, payload.RemoteJID, payload.TargetMsgID); err != nil {
					e.logger.Error().Err(err).Str("call_id", payload.TargetMsgID).Msg("Failed to reject call")
					return "", err
				}
				messageID = payload.TargetMsgID

			case "read", "mark_read":
				msgToRead := payload.TargetMsgID
				if msgToRead == "" {
					msgToRead = payload.QuotedMsgID
				}
				if err := client.MarkAsRead(sendCtx, payload.RemoteJID, msgToRead); err != nil {
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
					id, err := client.SendMedia(sendCtx, payload.RemoteJID, mediaInput)
					if err != nil {
						e.logger.Error().Err(err).Msg("Failed to send media message")
						cleanupPresence()
						return "", err
					}
					messageID = id

				} else {
					textInput := whatsappDomain.TextMessageInput{
						Text:              payload.Text,
						QuotedMsgID:       payload.QuotedMsgID,
						QuotedParticipant: payload.QuotedParticipant,
					}
					id, err := client.SendMessage(sendCtx, payload.RemoteJID, textInput)
					if err != nil {
						e.logger.Error().Err(err).Msg("Failed to send text message")
						cleanupPresence()
						return "", err
					}
					messageID = id
				}
			}

			if !payload.IsBroadcastOrChannelTarget() {
				_ = client.SendPresence(sendCtx, payload.RemoteJID, "paused")
			}
			activePresence = ""
		}
	}

	return messageID, nil
}
