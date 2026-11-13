package service

import (
	"chatnip/internal/kit/config"
	hsemDomain "chatnip/internal/modules/hsem/domain"
	hsemService "chatnip/internal/modules/hsem/service"
	"chatnip/internal/modules/whatsapp/domain"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/rs/zerolog"
)

type mockWhatsAppClient struct {
	domain.WhatsAppClient
	loggedIn          bool
	isOnWhatsAppCalls [][]string
	isOnWhatsAppResp  []domain.IsOnWhatsAppResult
	isOnWhatsAppErr   error
	sentToJIDs        []string
	sendMessageErr    error
	presenceCalls     []string
}

func (m *mockWhatsAppClient) IsLoggedIn() bool {
	return m.loggedIn
}

func (m *mockWhatsAppClient) IsOnWhatsApp(ctx context.Context, phones []string) ([]domain.IsOnWhatsAppResult, error) {
	m.isOnWhatsAppCalls = append(m.isOnWhatsAppCalls, phones)
	if m.isOnWhatsAppErr != nil {
		return nil, m.isOnWhatsAppErr
	}
	return m.isOnWhatsAppResp, nil
}

func (m *mockWhatsAppClient) SendPresence(ctx context.Context, jid string, state string) error {
	m.presenceCalls = append(m.presenceCalls, jid+":"+state)
	return nil
}

func (m *mockWhatsAppClient) SendGlobalPresence(ctx context.Context, available bool) error {
	return nil
}

func (m *mockWhatsAppClient) SendMessage(ctx context.Context, jid string, input domain.TextMessageInput) (string, error) {
	m.sentToJIDs = append(m.sentToJIDs, jid)
	if m.sendMessageErr != nil {
		return "", m.sendMessageErr
	}
	return "msg_ok", nil
}

type mockClientRepo struct {
	domain.ClientRepository
	clients map[string]domain.WhatsAppClient
}

func (r *mockClientRepo) GetClient(sessionID string) (domain.WhatsAppClient, bool) {
	c, ok := r.clients[sessionID]
	return c, ok
}

type fastMockEngine struct {
	*hsemService.HSEMEngine
}

func newTestWhatsAppService(repo domain.ClientRepository) *WhatsAppService {
	logger := zerolog.Nop()
	mathUtils := hsemService.NewMathUtils()
	return &WhatsAppService{
		clientRepo:   repo,
		cfg:          &config.Config{},
		logger:       logger,
		hsemEngine:   hsemService.NewHSEMEngine(mathUtils),
		hsemExecutor: NewHSEMExecutor(logger),
	}
}

func TestBuildPhoneVariations(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "brazilian mobile with 9th digit",
			input: "5534999998888",
			want:  []string{"+5534999998888", "+553499998888"},
		},
		{
			name:  "brazilian mobile without 9th digit",
			input: "553499998888",
			want:  []string{"+553499998888", "+5534999998888"},
		},
		{
			name:  "brazilian landline does not add 9th digit",
			input: "553432114455",
			want:  []string{"+553432114455"},
		},
		{
			name:  "formatted input strips non-digits",
			input: "+55 (34) 99999-8888",
			want:  []string{"+5534999998888", "+553499998888"},
		},
		{
			name:  "non-brazilian number",
			input: "14155552671",
			want:  []string{"+14155552671"},
		},
		{
			name:  "empty or non-digit input",
			input: "---",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildPhoneVariations(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildPhoneVariations(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveContact(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves variation without 9th digit in single batch", func(t *testing.T) {
		client := &mockWhatsAppClient{
			loggedIn: true,
			isOnWhatsAppResp: []domain.IsOnWhatsAppResult{
				{Phone: "+5534999998888", JID: "", IsIn: false},
				{Phone: "+553499998888", JID: "553499998888@s.whatsapp.net", IsIn: true},
			},
		}
		svc := newTestWhatsAppService(&mockClientRepo{clients: map[string]domain.WhatsAppClient{"s1": client}})

		res, err := svc.ResolveContact(ctx, "s1", "5534999998888")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(client.isOnWhatsAppCalls) != 1 {
			t.Fatalf("expected 1 batch IsOnWhatsApp call, got %d", len(client.isOnWhatsAppCalls))
		}
		if !res.Exists || res.JID != "553499998888@s.whatsapp.net" || res.Phone != "553499998888" {
			t.Fatalf("unexpected resolve result: %+v", res)
		}
	})

	t.Run("prefers primary candidate if both exist", func(t *testing.T) {
		client := &mockWhatsAppClient{
			loggedIn: true,
			isOnWhatsAppResp: []domain.IsOnWhatsAppResult{
				{Phone: "+553499998888", JID: "553499998888@s.whatsapp.net", IsIn: true},
				{Phone: "+5534999998888", JID: "5534999998888@s.whatsapp.net", IsIn: true},
			},
		}
		svc := newTestWhatsAppService(&mockClientRepo{clients: map[string]domain.WhatsAppClient{"s1": client}})

		res, err := svc.ResolveContact(ctx, "s1", "5534999998888")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.JID != "5534999998888@s.whatsapp.net" {
			t.Fatalf("expected primary JID, got %q", res.JID)
		}
	})

	t.Run("returns exists=false when neither variation is on WhatsApp", func(t *testing.T) {
		client := &mockWhatsAppClient{
			loggedIn: true,
			isOnWhatsAppResp: []domain.IsOnWhatsAppResult{
				{Phone: "+5534999998888", IsIn: false},
				{Phone: "+553499998888", IsIn: false},
			},
		}
		svc := newTestWhatsAppService(&mockClientRepo{clients: map[string]domain.WhatsAppClient{"s1": client}})

		res, err := svc.ResolveContact(ctx, "s1", "5534999998888")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Exists || res.JID != "" {
			t.Fatalf("expected Exists=false, got %+v", res)
		}
	})

	t.Run("returns error when session is not connected", func(t *testing.T) {
		svc := newTestWhatsAppService(&mockClientRepo{clients: map[string]domain.WhatsAppClient{}})
		_, err := svc.ResolveContact(ctx, "missing", "5534999998888")
		if err == nil {
			t.Fatal("expected error for disconnected session")
		}
	})
}

func TestExecuteBurstStepIsPureConsumer(t *testing.T) {
	ctx := context.Background()

	t.Run("sends directly without calling IsOnWhatsApp", func(t *testing.T) {
		client := &mockWhatsAppClient{loggedIn: true}
		svc := newTestWhatsAppService(&mockClientRepo{clients: map[string]domain.WhatsAppClient{"s1": client}})

		// Use reaction action to keep test fast (300ms) while still verifying no IsOnWhatsApp call,
		// and also test a direct text execution via HSEMExecutor directly.
		payload := domain.SendMessagePayload{
			SessionID: "s1",
			RemoteJID: "553499998888@s.whatsapp.net",
			Text:      "hello",
		}
		quickPlan := hsemDomain.FingerprintPlan{
			Steps: []hsemDomain.ActionStep{
				{Type: hsemDomain.StepAction, Metadata: "send_message"},
			},
		}

		msgID, err := svc.hsemExecutor.Execute(ctx, client, quickPlan, payload, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if msgID != "msg_ok" {
			t.Fatalf("expected msg_ok, got %q", msgID)
		}
		if len(client.isOnWhatsAppCalls) != 0 {
			t.Fatalf("expected 0 IsOnWhatsApp calls, got %d", len(client.isOnWhatsAppCalls))
		}
		if len(client.sentToJIDs) != 1 || client.sentToJIDs[0] != "553499998888@s.whatsapp.net" {
			t.Fatalf("unexpected sent JIDs: %v", client.sentToJIDs)
		}
	})

	t.Run("propagates no LID found without IsOnWhatsApp fallback", func(t *testing.T) {
		client := &mockWhatsAppClient{
			loggedIn:       true,
			sendMessageErr: errors.New("no LID found"),
		}
		svc := newTestWhatsAppService(&mockClientRepo{clients: map[string]domain.WhatsAppClient{"s1": client}})
		payload := domain.SendMessagePayload{
			SessionID: "s1",
			RemoteJID: "553499998888@s.whatsapp.net",
			Text:      "hello",
		}

		_, err := svc.ExecuteBurstStep(ctx, payload, nil, true)
		if err == nil || err.Error() != "no LID found" {
			t.Fatalf("expected 'no LID found' error, got %v", err)
		}
		if len(client.isOnWhatsAppCalls) != 0 {
			t.Fatalf("expected 0 IsOnWhatsApp calls during send, got %d", len(client.isOnWhatsAppCalls))
		}
	})
}

func TestStatusAndNewsletterSkipPresence(t *testing.T) {
	ctx := context.Background()

	targets := []struct {
		name    string
		payload domain.SendMessagePayload
	}{
		{
			name: "status broadcast",
			payload: domain.SendMessagePayload{
				SessionID:  "s1",
				RemoteJID:  "status@broadcast",
				TargetType: "status",
				Action:     "status",
				Text:       "Novo story!",
			},
		},
		{
			name: "newsletter channel",
			payload: domain.SendMessagePayload{
				SessionID:  "s1",
				RemoteJID:  "12036300123456@newsletter",
				TargetType: "newsletter",
				Text:       "Novidade no canal!",
			},
		},
	}

	for _, tt := range targets {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockWhatsAppClient{loggedIn: true}
			svc := newTestWhatsAppService(&mockClientRepo{clients: map[string]domain.WhatsAppClient{"s1": client}})
			plan := hsemDomain.FingerprintPlan{
				Steps: []hsemDomain.ActionStep{
					{Type: hsemDomain.StepAction, Metadata: "composing"},
					{Type: hsemDomain.StepAction, Metadata: "send_message"},
					{Type: hsemDomain.StepAction, Metadata: "paused"},
				},
			}

			msgID, err := svc.hsemExecutor.Execute(ctx, client, plan, tt.payload, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if msgID != "msg_ok" {
				t.Fatalf("expected msg_ok, got %q", msgID)
			}
			if len(client.presenceCalls) != 0 {
				t.Fatalf("expected 0 presence calls for %s, got %v", tt.name, client.presenceCalls)
			}
			if len(client.sentToJIDs) != 1 || client.sentToJIDs[0] != tt.payload.RemoteJID {
				t.Fatalf("unexpected sentToJIDs: %v", client.sentToJIDs)
			}
		})
	}
}
