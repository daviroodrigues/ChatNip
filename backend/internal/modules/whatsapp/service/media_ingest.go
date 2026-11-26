package service

import (
	"bytes"
	"chatnip/internal/modules/whatsapp/domain"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

const (
	maxMediaIngestAttempts = 10
	mediaRetryInterval     = 15 * time.Second
)

type IncomingMediaRequest struct {
	TenantID   string `json:"tenant_id"`
	SessionID  string `json:"session_id"`
	MessageID  string `json:"message_id"`
	MediaType  string `json:"media_type"`
	Mimetype   string `json:"mimetype"`
	FileLength uint64 `json:"file_length"`
	FileName   string `json:"file_name,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
}

type IncomingMediaResponse struct {
	MediaID         string `json:"media_id"`
	PresignedPutURL string `json:"presigned_put_url"`
	ExpiresAt       string `json:"expires_at,omitempty"`
}

type MediaIngestService struct {
	repo               domain.MediaIngestRepository
	clientRepo         domain.ClientRepository
	publisher          domain.EventPublisher
	httpClient         *http.Client
	backendURL         string
	backendInternalKey string
	tempDir            string
	logger             zerolog.Logger
}

func NewMediaIngestService(
	repo domain.MediaIngestRepository,
	clientRepo domain.ClientRepository,
	publisher domain.EventPublisher,
	backendURL string,
	backendInternalKey string,
	tempDir string,
	logger zerolog.Logger,
) *MediaIngestService {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return &MediaIngestService{
		repo:               repo,
		clientRepo:         clientRepo,
		publisher:          publisher,
		httpClient:         &http.Client{Timeout: 2 * time.Minute},
		backendURL:         strings.TrimRight(backendURL, "/"),
		backendInternalKey: backendInternalKey,
		tempDir:            tempDir,
		logger:             logger,
	}
}

func (s *MediaIngestService) StartRetryWorker(ctx context.Context) {
	ticker := time.NewTicker(mediaRetryInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.processDueRetries(ctx)
			}
		}
	}()
}

func (s *MediaIngestService) processDueRetries(ctx context.Context) {
	jobs, err := s.repo.ListDueForRetry(ctx, 10)
	if err != nil {
		s.logger.Error().Err(err).Msg("Failed to list due media ingest jobs")
		return
	}
	for _, job := range jobs {
		s.processJob(ctx, job)
	}
}

func (s *MediaIngestService) IngestIncomingMedia(ctx context.Context, evt domain.MessageEvent) {
	if evt.RawMedia == nil {
		return
	}

	payloadBytes, err := json.Marshal(evt)
	if err != nil {
		s.logger.Error().Err(err).Str("message_id", evt.ID).Msg("Failed to marshal message event for media ingest")
		return
	}

	job := &domain.MediaIngestJob{
		ID:            uuid.NewString(),
		SessionID:     evt.SessionID,
		TenantID:      evt.TenantID,
		MessageID:     evt.ID,
		MediaType:     evt.RawMedia.MediaType,
		Mimetype:      evt.RawMedia.Mimetype,
		FileName:      evt.RawMedia.FileName,
		DirectPath:    evt.RawMedia.DirectPath,
		MediaKey:      evt.RawMedia.MediaKey,
		FileEncSHA256: evt.RawMedia.FileEncSHA256,
		FileSHA256:    evt.RawMedia.FileSHA256,
		FileLength:    evt.RawMedia.FileLength,
		EventPayload:  string(payloadBytes),
		Status:        domain.MediaStatusPendingDownload,
		Attempts:      0,
		NextRetryAt:   time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, job); err != nil {
		s.logger.Error().Err(err).Str("message_id", evt.ID).Msg("Failed to persist media ingest job")
		return
	}

	go s.processJob(context.Background(), job)
}

func (s *MediaIngestService) processJob(ctx context.Context, job *domain.MediaIngestJob) {
	if err := s.ensureDownloadedToDisk(ctx, job); err != nil {
		s.handleFailure(ctx, job, fmt.Errorf("download to disk: %w", err))
		return
	}

	if s.backendURL == "" {
		s.logger.Warn().
			Str("job_id", job.ID).
			Str("message_id", job.MessageID).
			Msg("BACKEND_API_URL not configured; skipping presigned PUT handshake and publishing message")
		s.cleanupLocalFile(job)
		s.publishMessageEvent(ctx, job, "unconfigured")
		job.Status = domain.MediaStatusDone
		_ = s.repo.UpdateState(ctx, job)
		return
	}

	handshakeResp, err := s.requestPresignedPutURL(ctx, job)
	if err != nil {
		s.handleFailure(ctx, job, fmt.Errorf("backend handshake: %w", err))
		return
	}

	job.MediaID = handshakeResp.MediaID
	job.Status = domain.MediaStatusUploading
	_ = s.repo.UpdateState(ctx, job)

	if err := s.uploadFileStreaming(ctx, job.LocalPath, handshakeResp.PresignedPutURL, job.Mimetype); err != nil {
		s.handleFailure(ctx, job, fmt.Errorf("presigned PUT upload: %w", err))
		return
	}

	s.cleanupLocalFile(job)

	if err := s.publishMessageEvent(ctx, job, "ready"); err != nil {
		s.handleFailure(ctx, job, fmt.Errorf("publish message event: %w", err))
		return
	}

	job.Status = domain.MediaStatusDone
	job.LastError = ""
	if err := s.repo.UpdateState(ctx, job); err != nil {
		s.logger.Error().Err(err).Str("job_id", job.ID).Msg("Failed to mark media ingest job done")
	}
}

func (s *MediaIngestService) ensureDownloadedToDisk(ctx context.Context, job *domain.MediaIngestJob) error {
	if job.LocalPath != "" {
		if info, err := os.Stat(job.LocalPath); err == nil && info.Size() > 0 {
			job.FileLength = uint64(info.Size())
			return nil
		}
	}

	client, ok := s.clientRepo.GetClient(job.SessionID)
	if !ok {
		return fmt.Errorf("session %s not found", job.SessionID)
	}

	tmpFile, err := os.CreateTemp(s.tempDir, fmt.Sprintf("wa_%s_%s_*.bin", job.SessionID, job.MessageID))
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	dlCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	err = client.DownloadMediaToFile(dlCtx, job.ToDescriptor(), tmpFile)
	closeErr := tmpFile.Close()
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}

	info, err := os.Stat(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	job.LocalPath = tmpPath
	job.FileLength = uint64(info.Size())
	job.Status = domain.MediaStatusDownloaded
	return s.repo.UpdateState(ctx, job)
}

func (s *MediaIngestService) requestPresignedPutURL(ctx context.Context, job *domain.MediaIngestJob) (*IncomingMediaResponse, error) {
	reqPayload := IncomingMediaRequest{
		TenantID:   job.TenantID,
		SessionID:  job.SessionID,
		MessageID:  job.MessageID,
		MediaType:  job.MediaType,
		Mimetype:   job.Mimetype,
		FileLength: job.FileLength,
		FileName:   job.FileName,
		SHA256:     hex.EncodeToString(job.FileSHA256),
	}

	body, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	endpoint := s.backendURL + "/internal/media/incoming"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.backendInternalKey != "" {
		req.Header.Set("X-Internal-Api-Key", s.backendInternalKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed IncomingMediaResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode handshake response: %w", err)
	}
	if parsed.MediaID == "" || parsed.PresignedPutURL == "" {
		return nil, fmt.Errorf("invalid handshake response: missing media_id or presigned_put_url")
	}
	return &parsed, nil
}

func (s *MediaIngestService) uploadFileStreaming(ctx context.Context, localPath, presignedURL, mimetype string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, presignedURL, file)
	if err != nil {
		return err
	}
	req.ContentLength = info.Size()
	if mimetype != "" {
		req.Header.Set("Content-Type", mimetype)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("storage PUT returned status %d: %s", resp.StatusCode, string(errBody))
	}
	return nil
}

func (s *MediaIngestService) publishMessageEvent(ctx context.Context, job *domain.MediaIngestJob, mediaStatus string) error {
	var msgEvt domain.MessageEvent
	if err := json.Unmarshal([]byte(job.EventPayload), &msgEvt); err != nil {
		return err
	}

	msgEvt.MediaType = job.MediaType
	msgEvt.Media = &domain.MediaAttachment{
		MediaID:    job.MediaID,
		MediaType:  job.MediaType,
		Mimetype:   job.Mimetype,
		FileLength: job.FileLength,
		FileName:   job.FileName,
		Status:     mediaStatus,
	}

	envelope := domain.NewEventEnvelope("message.received", msgEvt.ID, job.TenantID, job.SessionID, msgEvt)
	return s.publisher.Publish(ctx, "whatsapp.message.received", envelope)
}

func (s *MediaIngestService) handleFailure(ctx context.Context, job *domain.MediaIngestJob, err error) {
	job.Attempts++
	job.LastError = err.Error()

	if job.Attempts >= maxMediaIngestAttempts {
		s.logger.Error().Err(err).
			Str("job_id", job.ID).
			Str("message_id", job.MessageID).
			Int("attempts", job.Attempts).
			Msg("Media ingest permanently failed after max attempts")

		s.cleanupLocalFile(job)
		_ = s.publishMessageEvent(ctx, job, "failed")
		job.Status = domain.MediaStatusFailed
		_ = s.repo.UpdateState(ctx, job)
		return
	}

	backoff := time.Duration(1<<uint(job.Attempts)) * 15 * time.Second
	if backoff > 15*time.Minute {
		backoff = 15 * time.Minute
	}
	job.NextRetryAt = time.Now().UTC().Add(backoff)

	s.logger.Warn().Err(err).
		Str("job_id", job.ID).
		Str("message_id", job.MessageID).
		Int("attempt", job.Attempts).
		Time("next_retry_at", job.NextRetryAt).
		Msg("Media ingest failed, scheduled retry")

	_ = s.repo.UpdateState(ctx, job)
}

func (s *MediaIngestService) cleanupLocalFile(job *domain.MediaIngestJob) {
	if job.LocalPath != "" {
		_ = os.Remove(job.LocalPath)
		job.LocalPath = ""
	}
}
