package service

import (
	hsemDomain "chatnip/internal/modules/hsem/domain"
	whatsappDomain "chatnip/internal/modules/whatsapp/domain"
	"context"
	"testing"

	"github.com/rs/zerolog"
)

type mockMessenger struct {
	lastAction     string
	lastMessageID  string
	lastText       string
	lastReaction   string
	lastMarkReadID string
	availableState bool
}

func (m *mockMessenger) SendPresence(ctx context.Context, jid string, state string) error {
	return nil
}

func (m *mockMessenger) SendGlobalPresence(ctx context.Context, available bool) error {
	m.availableState = available
	return nil
}

func (m *mockMessenger) SendMessage(ctx context.Context, jid string, input whatsappDomain.TextMessageInput) (string, error) {
	m.lastAction = "send_text"
	m.lastText = input.Text
	return "msg_123", nil
}

func (m *mockMessenger) SendMedia(ctx context.Context, jid string, input whatsappDomain.MediaMessageInput) (string, error) {
	m.lastAction = "send_media"
	return "media_123", nil
}

func (m *mockMessenger) EditMessage(ctx context.Context, jid string, messageID string, newText string) (string, error) {
	m.lastAction = "edit"
	m.lastMessageID = messageID
	m.lastText = newText
	return "edit_resp_id", nil
}

func (m *mockMessenger) SendReaction(ctx context.Context, jid string, senderJID string, messageID string, reaction string) (string, error) {
	m.lastAction = "reaction"
	m.lastMessageID = messageID
	m.lastReaction = reaction
	return "reaction_resp_id", nil
}

func (m *mockMessenger) RevokeMessage(ctx context.Context, jid string, senderJID string, messageID string) (string, error) {
	m.lastAction = "revoke"
	m.lastMessageID = messageID
	return "revoke_resp_id", nil
}

func (m *mockMessenger) MarkAsRead(ctx context.Context, jid string, msgID string) error {
	m.lastMarkReadID = msgID
	return nil
}

func (m *mockMessenger) RejectCall(ctx context.Context, callerJID string, callID string) error {
	m.lastAction = "reject_call"
	m.lastMessageID = callID
	return nil
}

func (m *mockMessenger) CreateGroup(ctx context.Context, name string, participants []string) (string, error) {
	return "group_123", nil
}

func (m *mockMessenger) ListGroups(ctx context.Context) ([]whatsappDomain.Group, error) {
	return nil, nil
}

func (m *mockMessenger) SetGroupAnnounce(ctx context.Context, groupJID string, announce bool) error {
	return nil
}

func (m *mockMessenger) SetGroupLocked(ctx context.Context, groupJID string, locked bool) error {
	return nil
}

func TestHSEMExecutor_Actions(t *testing.T) {
	logger := zerolog.Nop()
	executor := NewHSEMExecutor(logger)
	mock := &mockMessenger{}

	ctx := context.Background()

	// 1. Test Edit Message
	editPlan := hsemDomain.FingerprintPlan{
		Steps: []hsemDomain.ActionStep{
			{Type: hsemDomain.StepAction, Timestamp: 0, Metadata: "send_message"},
		},
	}
	editPayload := whatsappDomain.SendMessagePayload{
		SessionID:   "sess_1",
		RemoteJID:   "5511999999999@s.whatsapp.net",
		Action:      "edit",
		TargetMsgID: "orig_msg_id",
		Text:        "Novo texto editado",
	}

	id, err := executor.Execute(ctx, mock, editPlan, editPayload, nil)
	if err != nil {
		t.Fatalf("unexpected error on edit: %v", err)
	}
	if id != "edit_resp_id" || mock.lastAction != "edit" || mock.lastText != "Novo texto editado" {
		t.Errorf("edit failed to dispatch properly: %+v", mock)
	}

	// 2. Test Reaction Message
	reactPayload := whatsappDomain.SendMessagePayload{
		SessionID:   "sess_1",
		RemoteJID:   "5511999999999@s.whatsapp.net",
		Action:      "reaction",
		TargetMsgID: "orig_msg_id",
		Reaction:    "🚀",
	}
	id, err = executor.Execute(ctx, mock, editPlan, reactPayload, nil)
	if err != nil {
		t.Fatalf("unexpected error on reaction: %v", err)
	}
	if id != "reaction_resp_id" || mock.lastAction != "reaction" || mock.lastReaction != "🚀" {
		t.Errorf("reaction failed to dispatch properly: %+v", mock)
	}

	// 3. Test Revoke Message
	revokePayload := whatsappDomain.SendMessagePayload{
		SessionID:   "sess_1",
		RemoteJID:   "5511999999999@s.whatsapp.net",
		Action:      "revoke",
		TargetMsgID: "orig_msg_id",
	}
	id, err = executor.Execute(ctx, mock, editPlan, revokePayload, nil)
	if err != nil {
		t.Fatalf("unexpected error on revoke: %v", err)
	}
	if id != "revoke_resp_id" || mock.lastAction != "revoke" || mock.lastMessageID != "orig_msg_id" {
		t.Errorf("revoke failed to dispatch properly: %+v", mock)
	}

	// 4. Test MarkAsRead
	readPlan := hsemDomain.FingerprintPlan{
		Steps: []hsemDomain.ActionStep{
			{Type: hsemDomain.StepAction, Timestamp: 0, Metadata: "mark_as_read"},
			{Type: hsemDomain.StepAction, Timestamp: 0, Metadata: "send_message"},
		},
	}
	readPayload := whatsappDomain.SendMessagePayload{
		SessionID:   "sess_1",
		RemoteJID:   "5511999999999@s.whatsapp.net",
		QuotedMsgID: "received_msg_id",
		Text:        "Resposta",
	}
	_, err = executor.Execute(ctx, mock, readPlan, readPayload, nil)
	if err != nil {
		t.Fatalf("unexpected error on mark read: %v", err)
	}
	if mock.lastMarkReadID != "received_msg_id" {
		t.Errorf("expected lastMarkReadID to be received_msg_id, got %s", mock.lastMarkReadID)
	}

	// 5. Test Action Read/MarkRead explicit
	explicitReadPayload := whatsappDomain.SendMessagePayload{
		SessionID:   "sess_1",
		RemoteJID:   "5511999999999@s.whatsapp.net",
		Action:      "read",
		TargetMsgID: "msg_to_read_explicit",
	}
	id, err = executor.Execute(ctx, mock, editPlan, explicitReadPayload, nil)
	if err != nil {
		t.Fatalf("unexpected error on explicit read action: %v", err)
	}
	if id != "msg_to_read_explicit" || mock.lastMarkReadID != "msg_to_read_explicit" {
		t.Errorf("expected lastMarkReadID to be msg_to_read_explicit, got %s", mock.lastMarkReadID)
	}
}
