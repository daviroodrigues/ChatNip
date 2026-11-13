package domain

import "time"

type EventType string

const (
	TypeMessage       EventType = "message"
	TypePairSuccess   EventType = "pair_success"
	TypeLoggedOut     EventType = "logged_out"
	TypeHistorySync   EventType = "history_sync"
	TypeReceipt       EventType = "receipt"
	TypeChatPresence  EventType = "chat_presence"
	TypeCallOffer     EventType = "call_offer"
	TypeUndecryptable EventType = "undecryptable"
	TypeGroupInfo     EventType = "group_info"
)

type Event interface {
	EventType() EventType
}

type PairSuccessEvent struct {
	JID       string
	Timestamp time.Time
}

func (e PairSuccessEvent) EventType() EventType { return TypePairSuccess }

type LoggedOutEvent struct {
	Reason    string
	Timestamp time.Time
}

func (e LoggedOutEvent) EventType() EventType { return TypeLoggedOut }

type MessageEvent struct {
	ID           string    `json:"id"`
	SessionID    string    `json:"session_id"`
	TenantID     string    `json:"tenant_id"`
	ChatJID      string    `json:"chat_jid,omitempty"`
	SenderJID    string    `json:"sender_jid"`
	SenderAltJID string    `json:"sender_alt_jid,omitempty"`
	SenderPhone  string    `json:"sender_phone,omitempty"`
	IsGroup      bool      `json:"is_group,omitempty"`
	PushName     string    `json:"push_name"`
	Text         string    `json:"text"`
	IsFromMe     bool      `json:"is_from_me"`
	Timestamp    time.Time `json:"timestamp"`
	MediaType    string    `json:"media_type,omitempty"`
	MediaURL     string    `json:"media_url,omitempty"`
}

func (e MessageEvent) EventType() EventType { return TypeMessage }

type HistorySyncEvent struct {
	Messages []MessageEvent
}

func (e HistorySyncEvent) EventType() EventType { return TypeHistorySync }

type MessageReceiptEvent struct {
	SessionID  string    `json:"session_id"`
	TenantID   string    `json:"tenant_id"`
	ChatJID    string    `json:"chat_jid"`
	SenderJID  string    `json:"sender_jid"`
	MessageIDs []string  `json:"message_ids"`
	Status     string    `json:"status"`
	Timestamp  time.Time `json:"timestamp"`
}

func (e MessageReceiptEvent) EventType() EventType { return TypeReceipt }

type ChatPresenceEvent struct {
	SessionID string    `json:"session_id"`
	TenantID  string    `json:"tenant_id"`
	ChatJID   string    `json:"chat_jid"`
	SenderJID string    `json:"sender_jid"`
	State     string    `json:"state"` // "composing", "recording", "paused"
	Timestamp time.Time `json:"timestamp"`
}

func (e ChatPresenceEvent) EventType() EventType { return TypeChatPresence }

type CallOfferEvent struct {
	SessionID string    `json:"session_id"`
	TenantID  string    `json:"tenant_id"`
	CallID    string    `json:"call_id"`
	CallerJID string    `json:"caller_jid"`
	IsGroup   bool      `json:"is_group"`
	Timestamp time.Time `json:"timestamp"`
}

func (e CallOfferEvent) EventType() EventType { return TypeCallOffer }

type UndecryptableMessageEvent struct {
	SessionID string    `json:"session_id"`
	TenantID  string    `json:"tenant_id"`
	MessageID string    `json:"message_id"`
	ChatJID   string    `json:"chat_jid"`
	SenderJID string    `json:"sender_jid"`
	Timestamp time.Time `json:"timestamp"`
}

func (e UndecryptableMessageEvent) EventType() EventType { return TypeUndecryptable }

type GroupInfoEvent struct {
	SessionID string    `json:"session_id"`
	TenantID  string    `json:"tenant_id"`
	GroupJID  string    `json:"group_jid"`
	SenderJID string    `json:"sender_jid,omitempty"`
	Action    string    `json:"action"` // e.g. "update", "join", "leave", "promote", "demote"
	Timestamp time.Time `json:"timestamp"`
}

func (e GroupInfoEvent) EventType() EventType { return TypeGroupInfo }
