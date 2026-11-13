package infrastructure

import (
	"chatnip/internal/kit/config"
	"chatnip/internal/modules/whatsapp/domain"
	"context"
	"fmt"
	"github.com/rs/zerolog"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
	"strings"
	"sync"
)

var initDevicePropsOnce sync.Once

type InMemoryClientRepository struct {
	container *sqlstore.Container
	clients   map[string]domain.WhatsAppClient
	mu        sync.RWMutex
	cfg       *config.Config
	zLogger   zerolog.Logger
}

func NewClientRepository(container *sqlstore.Container, cfg *config.Config, zLogger zerolog.Logger) *InMemoryClientRepository {
	initDevicePropsOnce.Do(func() {
		store.DeviceProps.RequireFullSync = proto.Bool(true)
		store.DeviceProps.Os = proto.String("Linux")
		store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_CHROME.Enum()
		store.SetOSInfo("Linux", store.GetWAVersion())
	})

	return &InMemoryClientRepository{
		container: container,
		clients:   make(map[string]domain.WhatsAppClient),
		cfg:       cfg,
		zLogger:   zLogger,
	}
}

func (r *InMemoryClientRepository) GetClient(sessionID string) (domain.WhatsAppClient, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	client, ok := r.clients[sessionID]
	return client, ok
}

func (r *InMemoryClientRepository) CreateClient(ctx context.Context, session domain.Session) (domain.WhatsAppClient, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if client, ok := r.clients[session.ID]; ok {
		return client, nil
	}

	var jid types.JID
	if session.JID != "" {
		parsed, err := types.ParseJID(session.JID)
		if err == nil {
			jid = parsed
		}
	}

	device, err := r.container.GetDevice(ctx, jid)
	if err != nil {
		return nil, fmt.Errorf("failed to get device from store: %w", err)
	}

	if device == nil {
		device = r.container.NewDevice()
		device.Platform = domain.DefaultDeviceName
		_ = device.Save(ctx)
	}

	waLogger := NewWaLogAdapter(r.zLogger, "Client")
	client := whatsmeow.NewClient(device, waLogger)

	finalProxy := strings.TrimSpace(session.ProxyURL)
	if finalProxy == "" {
		return nil, fmt.Errorf("egress proxy is strictly required for session %s (direct connection is forbidden): %w", session.ID, domain.ErrProxyRequired)
	}

	if err := client.SetProxyAddress(finalProxy); err != nil {
		return nil, fmt.Errorf("failed to set proxy address: %w", err)
	}

	adapter := NewWhatsMeowAdapter(client)
	r.clients[session.ID] = adapter

	return adapter, nil
}

func (r *InMemoryClientRepository) RemoveClient(sessionID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if client, ok := r.clients[sessionID]; ok {
		if client.IsConnected() {
			client.Disconnect()
		}
		delete(r.clients, sessionID)
	}
}

func (r *InMemoryClientRepository) DisconnectAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, client := range r.clients {
		client.Disconnect()
	}
}
