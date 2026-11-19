package domain

import (
	"context"
	"os"
	"time"
)

type Connector interface {
	Connect() error
	Disconnect()
	IsConnected() bool
	IsLoggedIn() bool
	GetJID() string
	GetQRChannel(ctx context.Context) (<-chan string, error)
	PairPhone(ctx context.Context, phone string) (string, error)
	Logout(ctx context.Context) error
}

type TextMessageInput struct {
	Text              string
	QuotedMsgID       string
	QuotedParticipant string
}

type MediaMessageInput struct {
	Type              string
	Caption           string
	MediaData         []byte
	MediaFilePath     string
	MediaFileSize     int64
	MimeType          string
	FileName          string
	IsPTT             bool
	IsGIF             bool
	Duration          uint32
	Waveform          []byte
	QuotedMsgID       string
	QuotedParticipant string
}

type Messenger interface {
	SendPresence(ctx context.Context, jid string, state string) error
	SendGlobalPresence(ctx context.Context, available bool) error
	SendMessage(ctx context.Context, jid string, input TextMessageInput) (string, error)
	SendMedia(ctx context.Context, jid string, input MediaMessageInput) (string, error)
	EditMessage(ctx context.Context, jid string, messageID string, newText string) (string, error)
	SendReaction(ctx context.Context, jid string, senderJID string, messageID string, reaction string) (string, error)
	RevokeMessage(ctx context.Context, jid string, senderJID string, messageID string) (string, error)
	MarkAsRead(ctx context.Context, jid string, msgID string) error
	RejectCall(ctx context.Context, callerJID string, callID string) error
}

type GroupManager interface {
	CreateGroup(ctx context.Context, name string, participants []string) (string, error)
	ListGroups(ctx context.Context) ([]Group, error)
	GetGroupInfo(ctx context.Context, groupJID string) (*GroupMetadata, error)
	SetGroupName(ctx context.Context, groupJID string, name string) error
	SetGroupTopic(ctx context.Context, groupJID string, topic string) error
	SetGroupPhoto(ctx context.Context, groupJID string, avatar []byte) (string, error)
	SetGroupAnnounce(ctx context.Context, groupJID string, announce bool) error
	SetGroupLocked(ctx context.Context, groupJID string, locked bool) error
	UpdateGroupParticipants(ctx context.Context, groupJID string, participants []string, action string) ([]GroupParticipant, error)
	LeaveGroup(ctx context.Context, groupJID string) error
	GetGroupInviteLink(ctx context.Context, groupJID string, reset bool) (string, error)
	GetGroupInfoFromLink(ctx context.Context, code string) (*GroupInviteInfo, error)
	JoinGroupWithLink(ctx context.Context, code string) (string, error)
	JoinGroupWithInvite(ctx context.Context, groupJID, inviterJID, code string, expiration int64) error
}

type ContactManager interface {
	IsOnWhatsApp(ctx context.Context, phones []string) ([]IsOnWhatsAppResult, error)
	GetUserInfo(ctx context.Context, jids []string) (map[string]UserInfoResult, error)
	GetContacts(ctx context.Context) ([]ContactInfo, error)
	GetBlocklist(ctx context.Context) ([]string, error)
	UpdateBlocklist(ctx context.Context, jid string, block bool) error
}

type AppStateSync interface {
	SetChatPin(ctx context.Context, chatJID string, pin bool) error
	SetChatMute(ctx context.Context, chatJID string, mute bool, duration time.Duration) error
	SetChatArchive(ctx context.Context, chatJID string, archive bool) error
	DeleteChat(ctx context.Context, chatJID string) error
}

type ChannelManager interface {
	CreateNewsletter(ctx context.Context, name, description string, avatar []byte) (*NewsletterInfo, error)
	GetSubscribedNewsletters(ctx context.Context) ([]NewsletterInfo, error)
	FollowNewsletter(ctx context.Context, jid string) error
	UnfollowNewsletter(ctx context.Context, jid string) error
}

type WhatsAppClient interface {
	Connector
	Messenger
	GroupManager
	ContactManager
	AppStateSync
	ChannelManager
	RegisterHandler(handler EventHandler)
	DownloadMediaToFile(ctx context.Context, desc WhatsAppMediaDescriptor, destFile *os.File) error
}

type EventHandler func(evt interface{})

type ClientRepository interface {
	GetClient(sessionID string) (WhatsAppClient, bool)
	CreateClient(ctx context.Context, session Session) (WhatsAppClient, error)
	RemoveClient(sessionID string)
	DisconnectAll()
}
