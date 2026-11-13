package infrastructure

import (
	"chatnip/internal/modules/whatsapp/domain"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func TestMapMessageEvent_IncludesSenderAndPhone(t *testing.T) {
	adapter := &WhatsMeowAdapter{}
	now := time.Now().UTC()

	t.Run("PN sender populates SenderJID and SenderPhone", func(t *testing.T) {
		evt := &events.Message{
			Info: types.MessageInfo{
				ID:        "msg-1",
				Timestamp: now,
				PushName:  "Alice",
				MessageSource: types.MessageSource{
					Chat:   types.NewJID("553499998888", types.DefaultUserServer),
					Sender: types.NewJID("553499998888", types.DefaultUserServer),
				},
			},
			Message: &waE2E.Message{Conversation: proto.String("Oi")},
		}

		mapped, ok := adapter.mapMessageEvent(evt).(domain.MessageEvent)
		if !ok {
			t.Fatal("expected domain.MessageEvent")
		}
		if mapped.SenderJID != "553499998888@s.whatsapp.net" {
			t.Fatalf("unexpected SenderJID: %q", mapped.SenderJID)
		}
		if mapped.SenderPhone != "553499998888" {
			t.Fatalf("unexpected SenderPhone: %q", mapped.SenderPhone)
		}
		if mapped.ChatJID != "553499998888@s.whatsapp.net" {
			t.Fatalf("unexpected ChatJID: %q", mapped.ChatJID)
		}
	})

	t.Run("LID sender extracts SenderPhone from SenderAlt", func(t *testing.T) {
		evt := &events.Message{
			Info: types.MessageInfo{
				ID:        "msg-2",
				Timestamp: now,
				PushName:  "Bob",
				MessageSource: types.MessageSource{
					Chat:      types.NewJID("123456789", types.HiddenUserServer),
					Sender:    types.NewJID("123456789", types.HiddenUserServer),
					SenderAlt: types.NewJID("553499998888", types.DefaultUserServer),
				},
			},
			Message: &waE2E.Message{Conversation: proto.String("Olá")},
		}

		mapped, ok := adapter.mapMessageEvent(evt).(domain.MessageEvent)
		if !ok {
			t.Fatal("expected domain.MessageEvent")
		}
		if mapped.SenderJID != "123456789@lid" {
			t.Fatalf("unexpected SenderJID: %q", mapped.SenderJID)
		}
		if mapped.SenderAltJID != "553499998888@s.whatsapp.net" {
			t.Fatalf("unexpected SenderAltJID: %q", mapped.SenderAltJID)
		}
		if mapped.SenderPhone != "553499998888" {
			t.Fatalf("unexpected SenderPhone: %q", mapped.SenderPhone)
		}
	})
}
