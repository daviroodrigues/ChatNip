package domain

import "context"

type FileDownloader interface {
	Download(url string) ([]byte, error)
	DownloadToTempFile(ctx context.Context, url string, tempDir string) (filePath string, size int64, mimeType string, err error)
}

type EventPublisher interface {
	Publish(ctx context.Context, routingKey string, payload interface{}) error
}

type Acknowledger interface {
	Ack() error
	Nack(requeue bool) error
}
