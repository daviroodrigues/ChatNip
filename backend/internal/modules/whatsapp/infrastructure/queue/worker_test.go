package queue

import (
	"chatnip/internal/modules/whatsapp/domain"
	"errors"
	"testing"
)

func TestPermanentErrorCode(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantCode      string
		wantPermanent bool
	}{
		{
			name:          "recipient is not on WhatsApp",
			err:           errors.New("recipient is not on WhatsApp"),
			wantCode:      "RECIPIENT_NOT_ON_WHATSAPP",
			wantPermanent: true,
		},
		{
			name:          "no LID found",
			err:           errors.New("failed to send: no LID found"),
			wantCode:      "RECIPIENT_NOT_ON_WHATSAPP",
			wantPermanent: true,
		},
		{
			name:          "proxy required",
			err:           domain.ErrProxyRequired,
			wantCode:      "PERMANENT_ERROR",
			wantPermanent: true,
		},
		{
			name:          "transient network error",
			err:           errors.New("websocket timeout"),
			wantCode:      "",
			wantPermanent: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, permanent := permanentErrorCode(tt.err)
			if code != tt.wantCode || permanent != tt.wantPermanent {
				t.Fatalf("permanentErrorCode(%v) = (%q, %v), want (%q, %v)", tt.err, code, permanent, tt.wantCode, tt.wantPermanent)
			}
		})
	}
}

func TestNormalizeTargetJID(t *testing.T) {
	t.Run("status target defaults empty jid to status@broadcast", func(t *testing.T) {
		p := domain.SendMessagePayload{TargetType: "status"}
		normalizeTargetJID(&p)
		if p.RemoteJID != "status@broadcast" {
			t.Fatalf("expected status@broadcast, got %q", p.RemoteJID)
		}
	})

	t.Run("newsletter target appends @newsletter when missing", func(t *testing.T) {
		p := domain.SendMessagePayload{TargetType: "newsletter", RemoteJID: "12036300123456"}
		normalizeTargetJID(&p)
		if p.RemoteJID != "12036300123456@newsletter" {
			t.Fatalf("expected 12036300123456@newsletter, got %q", p.RemoteJID)
		}
	})

	t.Run("newsletter target preserves full jid", func(t *testing.T) {
		p := domain.SendMessagePayload{TargetType: "channel", RemoteJID: "12036300123456@newsletter"}
		normalizeTargetJID(&p)
		if p.RemoteJID != "12036300123456@newsletter" {
			t.Fatalf("expected 12036300123456@newsletter, got %q", p.RemoteJID)
		}
	})
}

func TestDecodeSendMessagePayload(t *testing.T) {
	t.Run("decodes standardized CommandEnvelope and maps command_type", func(t *testing.T) {
		body := []byte(`{
			"schema_version": "1.0",
			"command_type": "message.react",
			"correlation_id": "corr-123",
			"timestamp": "2026-11-18T23:00:00Z",
			"session_id": "sess-1",
			"data": {
				"jid": "5511999999999@s.whatsapp.net",
				"target_msg_id": "msg-999",
				"reaction": "👍"
			}
		}`)

		payload, ts, err := decodeSendMessagePayload(body, true)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if payload.CorrelationID != "corr-123" || payload.SessionID != "sess-1" {
			t.Fatalf("unexpected ids: %+v", payload)
		}
		if payload.Action != "react" {
			t.Fatalf("expected action react, got %q", payload.Action)
		}
		if !payload.IsPriority {
			t.Fatal("expected IsPriority to be true on fast lane")
		}
		if ts.IsZero() {
			t.Fatal("expected envelope timestamp to be parsed")
		}
	})

	t.Run("decodes legacy flat SendMessagePayload", func(t *testing.T) {
		body := []byte(`{"correlation_id":"c1","session_id":"s1","jid":"5511888888888@s.whatsapp.net","message":"oi"}`)
		payload, _, err := decodeSendMessagePayload(body, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if payload.CorrelationID != "c1" || payload.Text != "oi" || payload.IsPriority {
			t.Fatalf("unexpected payload: %+v", payload)
		}
	})
}
