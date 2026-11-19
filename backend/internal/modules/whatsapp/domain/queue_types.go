package domain

import (
	"encoding/json"
	"strings"
	"time"
)

const QueueSchemaVersion = "1.0"

// EventEnvelope is the standardized outbound event format published to RabbitMQ.
type EventEnvelope struct {
	SchemaVersion string      `json:"schema_version"`
	EventType     string      `json:"event_type"`
	CorrelationID string      `json:"correlation_id,omitempty"`
	Timestamp     time.Time   `json:"timestamp"`
	TenantID      string      `json:"tenant_id,omitempty"`
	SessionID     string      `json:"session_id,omitempty"`
	Data          interface{} `json:"data"`
}

// CommandEnvelope is the standardized inbound command format consumed from RabbitMQ.
type CommandEnvelope struct {
	SchemaVersion string          `json:"schema_version"`
	CommandType   string          `json:"command_type"`
	CorrelationID string          `json:"correlation_id"`
	Timestamp     time.Time       `json:"timestamp"`
	TenantID      string          `json:"tenant_id,omitempty"`
	SessionID     string          `json:"session_id"`
	Data          json.RawMessage `json:"data"`
}

func NewEventEnvelope(eventType, correlationID, tenantID, sessionID string, data interface{}) EventEnvelope {
	return EventEnvelope{
		SchemaVersion: QueueSchemaVersion,
		EventType:     eventType,
		CorrelationID: correlationID,
		Timestamp:     time.Now().UTC(),
		TenantID:      tenantID,
		SessionID:     sessionID,
		Data:          data,
	}
}

type SendMessagePayload struct {
	CorrelationID     string `json:"correlation_id"`
	SessionID         string `json:"session_id"`
	RemoteJID         string `json:"jid"`
	TargetType        string `json:"target_type,omitempty"` // "contact", "group", "status", "newsletter"
	Text              string `json:"message"`
	IsGroup           bool   `json:"is_group"`
	IsPriority        bool   `json:"is_priority"`
	MediaURL          string `json:"media_url,omitempty"`
	MediaType         string `json:"media_type,omitempty"`
	IsPTT             bool   `json:"is_ptt,omitempty"`
	IsGIF             bool   `json:"is_gif,omitempty"`
	FileName          string `json:"file_name,omitempty"`
	Duration          uint32 `json:"duration,omitempty"`
	Waveform          []byte `json:"waveform,omitempty"`
	QuotedMsgID       string `json:"quoted_msg_id,omitempty"`
	QuotedParticipant string `json:"quoted_participant,omitempty"`
	Action            string `json:"action,omitempty"`
	TargetMsgID       string `json:"target_msg_id,omitempty"`
	TargetParticipant string `json:"target_participant,omitempty"`
	Reaction          string `json:"reaction,omitempty"`
	MediaFilePath     string `json:"-"`
	MediaFileSize     int64  `json:"-"`
	MediaMimeType     string `json:"-"`
}

// IsBroadcastOrChannelTarget reports whether the payload targets WhatsApp Status
// (status@broadcast) or a Newsletter/Channel (@newsletter), where chat presence
// and read receipts are not applicable.
func (p SendMessagePayload) IsBroadcastOrChannelTarget() bool {
	return p.Action == "status" ||
		p.TargetType == "status" ||
		p.TargetType == "newsletter" ||
		p.TargetType == "channel" ||
		p.RemoteJID == "status@broadcast" ||
		strings.HasSuffix(p.RemoteJID, "@newsletter")
}

type MessageSentData struct {
	MessageID string `json:"message_id"`
	SessionID string `json:"session_id"`
	JID       string `json:"jid"`
	Status    string `json:"status"`
}

type MessageSentEvent struct {
	SchemaVersion string          `json:"schema_version,omitempty"`
	EventType     string          `json:"event_type"`
	CorrelationID string          `json:"correlation_id"`
	Timestamp     time.Time       `json:"timestamp"`
	Data          MessageSentData `json:"data"`
}

type MessageFailedData struct {
	SessionID    string `json:"session_id"`
	JID          string `json:"jid"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

type MessageFailedEvent struct {
	SchemaVersion string            `json:"schema_version,omitempty"`
	EventType     string            `json:"event_type"`
	CorrelationID string            `json:"correlation_id"`
	Timestamp     time.Time         `json:"timestamp"`
	Data          MessageFailedData `json:"data"`
}

type CreateGroupPayload struct {
	CorrelationID string   `json:"correlation_id"`
	SessionID     string   `json:"session_id"`
	Name          string   `json:"name"`
	Participants  []string `json:"participants"`
}

type GroupCreatedData struct {
	GroupJID  string `json:"group_jid"`
	SessionID string `json:"session_id"`
	Name      string `json:"name"`
}

type GroupCreatedEvent struct {
	EventType     string           `json:"event_type"`
	CorrelationID string           `json:"correlation_id"`
	Timestamp     time.Time        `json:"timestamp"`
	Data          GroupCreatedData `json:"data"`
}

type GroupFailedData struct {
	SessionID    string `json:"session_id"`
	Name         string `json:"name"`
	ErrorCode    string `json:"error_code"`
	ErrorMessage string `json:"error_message"`
}

type GroupFailedEvent struct {
	EventType     string          `json:"event_type"`
	CorrelationID string          `json:"correlation_id"`
	Timestamp     time.Time       `json:"timestamp"`
	Data          GroupFailedData `json:"data"`
}
