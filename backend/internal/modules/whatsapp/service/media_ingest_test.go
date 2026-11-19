package service

import (
	"chatnip/internal/kit/database"
	"chatnip/internal/modules/whatsapp/domain"
	"chatnip/internal/modules/whatsapp/infrastructure/persistence"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

type mockMediaClient struct {
	domain.WhatsAppClient
	content []byte
}

func (m *mockMediaClient) DownloadMediaToFile(ctx context.Context, desc domain.WhatsAppMediaDescriptor, destFile *os.File) error {
	_, err := destFile.Write(m.content)
	return err
}

type mockMediaClientRepo struct {
	client domain.WhatsAppClient
}

func (r *mockMediaClientRepo) GetClient(sessionID string) (domain.WhatsAppClient, bool) {
	return r.client, r.client != nil
}
func (r *mockMediaClientRepo) CreateClient(ctx context.Context, session domain.Session) (domain.WhatsAppClient, error) {
	return r.client, nil
}
func (r *mockMediaClientRepo) RemoveClient(sessionID string) {}
func (r *mockMediaClientRepo) DisconnectAll()                {}

type mockMediaPublisher struct {
	routingKey string
	payload    interface{}
	done       chan struct{}
}

func (p *mockMediaPublisher) Publish(ctx context.Context, routingKey string, payload interface{}) error {
	p.routingKey = routingKey
	p.payload = payload
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	return nil
}

func TestMediaIngestService_DiskDownloadAndPresignedPut(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	dbConn, err := database.NewSQLiteConnection(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteConnection failed: %v", err)
	}
	defer dbConn.Close()

	var putReceived []byte
	var handshakeHeader string

	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/internal/media/incoming":
			handshakeHeader = r.Header.Get("X-Internal-Api-Key")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(IncomingMediaResponse{
				MediaID:         "media-uuid-7",
				PresignedPutURL: srv.URL + "/bucket/test.jpg",
			})
		case r.Method == http.MethodPut && r.URL.Path == "/bucket/test.jpg":
			putReceived, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	mediaBytes := []byte("fake-jpeg-bytes-123456")
	repo := persistence.NewSQLiteMediaIngestRepository(dbConn)
	pub := &mockMediaPublisher{done: make(chan struct{})}
	svc := NewMediaIngestService(
		repo,
		&mockMediaClientRepo{client: &mockMediaClient{content: mediaBytes}},
		pub,
		srv.URL+"/api/v1",
		"secret-internal-key",
		tmpDir,
		zerolog.New(os.Stderr),
	)

	evt := domain.MessageEvent{
		ID:        "wamid-1",
		SessionID: "sess-1",
		TenantID:  "tenant-1",
		SenderJID: "5511999999999@s.whatsapp.net",
		Timestamp: time.Now().UTC(),
		MediaType: "image",
		RawMedia: &domain.WhatsAppMediaDescriptor{
			MediaType:  "image",
			Mimetype:   "image/jpeg",
			DirectPath: "/v/t62.7118-24/123",
			MediaKey:   []byte{1, 2, 3},
			FileLength: uint64(len(mediaBytes)),
		},
	}

	svc.IngestIncomingMedia(context.Background(), evt)

	select {
	case <-pub.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for media ingest publish")
	}

	if handshakeHeader != "secret-internal-key" {
		t.Fatalf("expected X-Internal-Api-Key header, got %q", handshakeHeader)
	}
	if string(putReceived) != string(mediaBytes) {
		t.Fatalf("uploaded bytes mismatch: got %q, want %q", string(putReceived), string(mediaBytes))
	}
	if pub.routingKey != "whatsapp.message.received" {
		t.Fatalf("expected routing key whatsapp.message.received, got %q", pub.routingKey)
	}

	env, ok := pub.payload.(domain.EventEnvelope)
	if !ok {
		t.Fatalf("expected EventEnvelope, got %T", pub.payload)
	}
	msgEvt, ok := env.Data.(domain.MessageEvent)
	if !ok || msgEvt.Media == nil || msgEvt.Media.MediaID != "media-uuid-7" || msgEvt.Media.Status != "ready" {
		t.Fatalf("unexpected published message event: %+v", env.Data)
	}

	// Verify temp file was cleaned up
	matches, _ := filepath.Glob(filepath.Join(tmpDir, "wa_*.bin"))
	if len(matches) != 0 {
		t.Fatalf("expected temp files to be cleaned up, found %v", matches)
	}
}
