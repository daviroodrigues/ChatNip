package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type HTTPFileDownloader struct{}

var streamHTTPClient = &http.Client{
	Timeout: 10 * time.Minute,
}

func (d *HTTPFileDownloader) Download(url string) ([]byte, error) {
	resp, err := streamHTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// DownloadToTempFile streams the response body directly to a temporary file on
// disk so large files (e.g. 1 GB documents) never spike container RAM.
func (d *HTTPFileDownloader) DownloadToTempFile(ctx context.Context, url string, tempDir string) (string, int64, string, error) {
	if tempDir == "" {
		tempDir = os.TempDir()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", 0, "", err
	}

	resp, err := streamHTTPClient.Do(req)
	if err != nil {
		return "", 0, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", 0, "", fmt.Errorf("status code %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp(tempDir, "wa_out_*.bin")
	if err != nil {
		return "", 0, "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	written, copyErr := io.Copy(tmpFile, resp.Body)
	if copyErr != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return "", 0, "", copyErr
	}

	mimeType := strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if mimeType == "" || mimeType == "application/octet-stream" {
		if _, seekErr := tmpFile.Seek(0, io.SeekStart); seekErr == nil {
			head := make([]byte, 512)
			n, _ := io.ReadFull(tmpFile, head)
			if n > 0 {
				mimeType = http.DetectContentType(head[:n])
			}
		}
	}

	if closeErr := tmpFile.Close(); closeErr != nil {
		_ = os.Remove(tmpPath)
		return "", 0, "", closeErr
	}

	return tmpPath, written, mimeType, nil
}
