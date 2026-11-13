package handler

import "chatnip/internal/modules/whatsapp/domain"

type LoginRequest struct {
	SessionID string `json:"session_id"`
	TenantID  string `json:"tenant_id"`
	ProxyURL  string `json:"proxy_url"`
}

type LoginResponse struct {
	Status string `json:"status"`
	Msg    string `json:"msg,omitempty"`
}

type LogoutRequest struct {
	SessionID string `json:"session_id"`
}

type UpdateProxyRequest struct {
	SessionID string `json:"session_id"`
	ProxyURL  string `json:"proxy_url"`
}

type ValidateProxyRequest struct {
	ProxyURL string `json:"proxy_url"`
}

type StatusResponse struct {
	Status    string `json:"status"`
	Connected bool   `json:"connected"`
	LoggedIn  bool   `json:"logged_in"`
	JID       string `json:"jid,omitempty"`
}

type MediaType string

const (
	MediaTypeImage    MediaType = "image"
	MediaTypeVideo    MediaType = "video"
	MediaTypeAudio    MediaType = "audio"
	MediaTypeDocument MediaType = "document"
	MediaTypeSticker  MediaType = "sticker"
)

type SendMessageRequest struct {
	CorrelationID     string    `json:"correlation_id"`
	SessionID         string    `json:"session_id"`
	RemoteJID         string    `json:"jid"`
	Message           string    `json:"message"`
	MediaURL          string    `json:"media_url,omitempty"`
	MediaType         MediaType `json:"media_type,omitempty"`
	IsPTT             bool      `json:"is_ptt,omitempty"`
	IsGIF             bool      `json:"is_gif,omitempty"`
	FileName          string    `json:"file_name,omitempty"`
	IsPriority        bool      `json:"is_priority,omitempty"`
	QuotedMsgID       string    `json:"quoted_msg_id,omitempty"`
	QuotedParticipant string    `json:"quoted_participant,omitempty"`
	IsGroup           bool      `json:"is_group"`
}

type CreateGroupRequest struct {
	SessionID    string   `json:"session_id"`
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
}

type CreateGroupResponse struct {
	GroupID string `json:"group_id"`
	Msg     string `json:"msg"`
}

type ListGroupsResponse struct {
	SessionID string         `json:"session_id"`
	Groups    []domain.Group `json:"groups"`
}

type SetGroupMessageSettingRequest struct {
	SessionID  string `json:"session_id"`
	GroupJID   string `json:"group_jid"`
	OnlyAdmins bool   `json:"only_admins"`
}

type EditMessageRequest struct {
	CorrelationID string `json:"correlation_id"`
	SessionID     string `json:"session_id"`
	RemoteJID     string `json:"jid"`
	MessageID     string `json:"message_id"`
	NewMessage    string `json:"new_message"`
	IsGroup       bool   `json:"is_group,omitempty"`
	IsPriority    bool   `json:"is_priority,omitempty"`
}

type ReactionMessageRequest struct {
	CorrelationID string `json:"correlation_id"`
	SessionID     string `json:"session_id"`
	RemoteJID     string `json:"jid"`
	SenderJID     string `json:"sender_jid,omitempty"`
	MessageID     string `json:"message_id"`
	Reaction      string `json:"reaction"` // Emoji ou vazio para remover
	IsGroup       bool   `json:"is_group,omitempty"`
	IsPriority    bool   `json:"is_priority,omitempty"`
}

type RevokeMessageRequest struct {
	CorrelationID string `json:"correlation_id"`
	SessionID     string `json:"session_id"`
	RemoteJID     string `json:"jid"`
	SenderJID     string `json:"sender_jid,omitempty"`
	MessageID     string `json:"message_id"`
	IsGroup       bool   `json:"is_group,omitempty"`
	IsPriority    bool   `json:"is_priority,omitempty"`
}

type LoginPairCodeRequest struct {
	SessionID string `json:"session_id"`
	TenantID  string `json:"tenant_id"`
	Phone     string `json:"phone"`
	ProxyURL  string `json:"proxy_url"`
}

type LoginPairCodeResponse struct {
	Status   string `json:"status"`
	PairCode string `json:"pair_code"`
}

type ReadMessageRequest struct {
	CorrelationID string `json:"correlation_id,omitempty"`
	SessionID     string `json:"session_id"`
	ChatJID       string `json:"chat_jid"`
	MessageID     string `json:"message_id"`
	IsGroup       bool   `json:"is_group,omitempty"`
}

type SetGroupNameRequest struct {
	SessionID string `json:"session_id"`
	GroupJID  string `json:"group_jid"`
	Name      string `json:"name"`
}

type SetGroupTopicRequest struct {
	SessionID string `json:"session_id"`
	GroupJID  string `json:"group_jid"`
	Topic     string `json:"topic"`
}

type SetGroupPhotoRequest struct {
	SessionID   string `json:"session_id"`
	GroupJID    string `json:"group_jid"`
	PhotoBase64 string `json:"photo_base64"`
}

type SetGroupInfoSettingRequest struct {
	SessionID  string `json:"session_id"`
	GroupJID   string `json:"group_jid"`
	OnlyAdmins bool   `json:"only_admins"`
}

type UpdateGroupParticipantsRequest struct {
	SessionID    string   `json:"session_id"`
	GroupJID     string   `json:"group_jid"`
	Participants []string `json:"participants"`
	Action       string   `json:"action"` // "add", "remove", "promote", "demote"
}

type LeaveGroupRequest struct {
	SessionID string `json:"session_id"`
	GroupJID  string `json:"group_jid"`
}

type JoinGroupLinkRequest struct {
	SessionID string `json:"session_id"`
	Code      string `json:"code"`
}

type JoinGroupInviteMessageRequest struct {
	SessionID  string `json:"session_id"`
	GroupJID   string `json:"group_jid"`
	InviterJID string `json:"inviter_jid"`
	Code       string `json:"code"`
	Expiration int64  `json:"expiration"`
}

type CheckNumbersRequest struct {
	SessionID string   `json:"session_id"`
	Phones    []string `json:"phones"`
}

type SetChatPinRequest struct {
	SessionID string `json:"session_id"`
	ChatJID   string `json:"chat_jid"`
	Pin       bool   `json:"pin"`
}

type SetChatMuteRequest struct {
	SessionID       string `json:"session_id"`
	ChatJID         string `json:"chat_jid"`
	Mute            bool   `json:"mute"`
	DurationSeconds int64  `json:"duration_seconds,omitempty"`
}

type SetChatArchiveRequest struct {
	SessionID string `json:"session_id"`
	ChatJID   string `json:"chat_jid"`
	Archive   bool   `json:"archive"`
}

type BlocklistRequest struct {
	SessionID string `json:"session_id"`
	JID       string `json:"jid"`
	Block     bool   `json:"block"`
}

type SendStatusRequest struct {
	CorrelationID string    `json:"correlation_id,omitempty"`
	SessionID     string    `json:"session_id"`
	Message       string    `json:"message,omitempty"`
	MediaURL      string    `json:"media_url,omitempty"`
	MediaType     MediaType `json:"media_type,omitempty"`
}

type CreateChannelRequest struct {
	SessionID    string `json:"session_id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	AvatarBase64 string `json:"avatar_base64,omitempty"`
}

type FollowChannelRequest struct {
	SessionID  string `json:"session_id"`
	ChannelJID string `json:"channel_jid"`
}
