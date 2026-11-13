package domain

import "time"

type SendMessagePayload struct {
	CorrelationID     string `json:"correlation_id"`
	SessionID         string `json:"session_id"`
	RemoteJID         string `json:"jid"`
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
}

type MessageSentData struct {
	MessageID string `json:"message_id"`
	SessionID string `json:"session_id"`
	JID       string `json:"jid"`
	Status    string `json:"status"`
}

type MessageSentEvent struct {
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
