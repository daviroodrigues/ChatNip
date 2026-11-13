package domain

import (
	"errors"
	"strings"
	"time"
)

const (
	DefaultDeviceName = "Google Chrome (Linux)"
)

var (
	ErrSessionNotFound      = errors.New("whatsapp session not found")
	ErrSessionTimeout       = errors.New("timeout waiting for qr code")
	ErrSessionAlreadyActive = errors.New("session already active")
	ErrProxyRequired        = errors.New("proxy_url is mandatory: direct egress connection is strictly forbidden")
	ErrSessionLoading       = errors.New("session exists but is still loading")
)

type Session struct {
	ID        string
	TenantID  string
	JID       string
	ProxyURL  string
	CreatedAt time.Time
}

type SessionStatus struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Status    string    `json:"status"`
	Connected bool      `json:"connected"`
	LoggedIn  bool      `json:"logged_in"`
	JID       string    `json:"jid,omitempty"`
	QR        string    `json:"qr,omitempty"`
	ProxyURL  string    `json:"proxy_url,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ProxyValidationResult struct {
	Valid     bool   `json:"valid"`
	ExitIP    string `json:"exit_ip,omitempty"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Group struct {
	JID      string `json:"jid"`
	Name     string `json:"name"`
	OwnerJID string `json:"owner_jid,omitempty"`
	Count    int    `json:"participant_count"`
}

type GroupParticipant struct {
	JID          string `json:"jid"`
	PhoneNumber  string `json:"phone_number,omitempty"`
	IsAdmin      bool   `json:"is_admin"`
	IsSuperAdmin bool   `json:"is_super_admin"`
	DisplayName  string `json:"display_name,omitempty"`
}

type GroupMetadata struct {
	JID              string             `json:"jid"`
	OwnerJID         string             `json:"owner_jid"`
	Name             string             `json:"name"`
	Topic            string             `json:"topic"`
	TopicSetBy       string             `json:"topic_set_by,omitempty"`
	IsLocked         bool               `json:"is_locked"`
	IsAnnounce       bool               `json:"is_announce"`
	IsEphemeral      bool               `json:"is_ephemeral"`
	ParticipantCount int                `json:"participant_count"`
	Participants     []GroupParticipant `json:"participants"`
	CreatedAt        time.Time          `json:"created_at"`
}

type GroupInviteInfo struct {
	JID          string    `json:"jid"`
	GroupName    string    `json:"group_name"`
	OwnerJID     string    `json:"owner_jid"`
	ParticipantCount int   `json:"participant_count"`
	CreatedAt    time.Time `json:"created_at"`
}

type ContactInfo struct {
	JID      string `json:"jid"`
	PushName string `json:"push_name,omitempty"`
	FullName string `json:"full_name,omitempty"`
}

type IsOnWhatsAppResult struct {
	Phone string `json:"phone"`
	JID   string `json:"jid"`
	IsIn  bool   `json:"is_in"`
}

type UserInfoResult struct {
	JID          string `json:"jid"`
	Status       string `json:"status,omitempty"`
	PictureURL   string `json:"picture_url,omitempty"`
	VerifiedName string `json:"verified_name,omitempty"`
}

type NewsletterInfo struct {
	JID         string `json:"jid"`
	Name        string `json:"name"`
	Description string `json:"description"`
	InviteCode  string `json:"invite_code,omitempty"`
	Subscribers int    `json:"subscribers,omitempty"`
}

func ToDeepLink(code string) string {
	if strings.HasPrefix(code, "https://wa.me/") {
		return code
	}
	return "https://wa.me/settings/linked_devices#" + code + ",1"
}
