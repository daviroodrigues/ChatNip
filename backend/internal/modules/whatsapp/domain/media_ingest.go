package domain

import (
	"context"
	"time"
)

type MediaIngestStatus string

const (
	MediaStatusPendingDownload MediaIngestStatus = "pending_download"
	MediaStatusDownloaded      MediaIngestStatus = "downloaded"
	MediaStatusUploading       MediaIngestStatus = "uploading"
	MediaStatusDone            MediaIngestStatus = "done"
	MediaStatusFailed          MediaIngestStatus = "failed"
)

type MediaIngestJob struct {
	ID            string
	SessionID     string
	TenantID      string
	MessageID     string
	MediaType     string
	Mimetype      string
	FileName      string
	DirectPath    string
	MediaKey      []byte
	FileEncSHA256 []byte
	FileSHA256    []byte
	FileLength    uint64
	LocalPath     string
	MediaID       string
	EventPayload  string
	Status        MediaIngestStatus
	Attempts      int
	LastError     string
	NextRetryAt   time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (j MediaIngestJob) ToDescriptor() WhatsAppMediaDescriptor {
	return WhatsAppMediaDescriptor{
		MediaType:     j.MediaType,
		Mimetype:      j.Mimetype,
		DirectPath:    j.DirectPath,
		MediaKey:      j.MediaKey,
		FileEncSHA256: j.FileEncSHA256,
		FileSHA256:    j.FileSHA256,
		FileLength:    j.FileLength,
		FileName:      j.FileName,
	}
}

type MediaIngestRepository interface {
	Create(ctx context.Context, job *MediaIngestJob) error
	UpdateState(ctx context.Context, job *MediaIngestJob) error
	ListDueForRetry(ctx context.Context, limit int) ([]*MediaIngestJob, error)
}
