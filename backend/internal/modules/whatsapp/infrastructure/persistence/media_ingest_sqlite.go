package persistence

import (
	"chatnip/internal/modules/whatsapp/domain"
	"context"
	"database/sql"
	"time"
)

type SQLiteMediaIngestRepository struct {
	db *sql.DB
}

func NewSQLiteMediaIngestRepository(db *sql.DB) *SQLiteMediaIngestRepository {
	return &SQLiteMediaIngestRepository{db: db}
}

func nonNilBytes(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}

func (r *SQLiteMediaIngestRepository) Create(ctx context.Context, job *domain.MediaIngestJob) error {
	query := `
		INSERT INTO media_ingest_jobs (
			id, session_id, tenant_id, message_id, media_type, mimetype, file_name,
			direct_path, media_key, file_enc_sha256, file_sha256, file_length,
			local_path, media_id, event_payload, status, attempts, last_error, next_retry_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := r.db.ExecContext(ctx, query,
		job.ID, job.SessionID, job.TenantID, job.MessageID, job.MediaType, job.Mimetype, job.FileName,
		job.DirectPath, nonNilBytes(job.MediaKey), nonNilBytes(job.FileEncSHA256), nonNilBytes(job.FileSHA256), int64(job.FileLength),
		job.LocalPath, job.MediaID, job.EventPayload, string(job.Status), job.Attempts, job.LastError, job.NextRetryAt.UTC(),
	)
	return err
}

func (r *SQLiteMediaIngestRepository) UpdateState(ctx context.Context, job *domain.MediaIngestJob) error {
	query := `
		UPDATE media_ingest_jobs
		SET local_path = ?, media_id = ?, file_length = ?, status = ?, attempts = ?, last_error = ?,
		    next_retry_at = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`
	_, err := r.db.ExecContext(ctx, query,
		job.LocalPath, job.MediaID, int64(job.FileLength), string(job.Status),
		job.Attempts, job.LastError, job.NextRetryAt.UTC(), job.ID,
	)
	return err
}

func (r *SQLiteMediaIngestRepository) ListDueForRetry(ctx context.Context, limit int) ([]*domain.MediaIngestJob, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT id, session_id, tenant_id, message_id, media_type, mimetype, file_name,
		       direct_path, media_key, file_enc_sha256, file_sha256, file_length,
		       local_path, media_id, event_payload, status, attempts, last_error, next_retry_at,
		       created_at, updated_at
		FROM media_ingest_jobs
		WHERE status IN ('pending_download', 'downloaded', 'uploading')
		  AND next_retry_at <= ?
		ORDER BY next_retry_at ASC
		LIMIT ?
	`
	rows, err := r.db.QueryContext(ctx, query, time.Now().UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*domain.MediaIngestJob
	for rows.Next() {
		var j domain.MediaIngestJob
		var status string
		var fileLen int64
		if err := rows.Scan(
			&j.ID, &j.SessionID, &j.TenantID, &j.MessageID, &j.MediaType, &j.Mimetype, &j.FileName,
			&j.DirectPath, &j.MediaKey, &j.FileEncSHA256, &j.FileSHA256, &fileLen,
			&j.LocalPath, &j.MediaID, &j.EventPayload, &status, &j.Attempts, &j.LastError,
			&j.NextRetryAt, &j.CreatedAt, &j.UpdatedAt,
		); err != nil {
			return nil, err
		}
		j.FileLength = uint64(fileLen)
		j.Status = domain.MediaIngestStatus(status)
		jobs = append(jobs, &j)
	}
	return jobs, rows.Err()
}
