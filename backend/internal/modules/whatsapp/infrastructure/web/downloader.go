package web

import (
    "io"
    "net/http"
    "fmt"
)

type HTTPFileDownloader struct {}

func (d *HTTPFileDownloader) Download(url string) ([]byte, error) {
    resp, err := http.Get(url)
    if err != nil { return nil, err }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("status code %d", resp.StatusCode)
    }
    return io.ReadAll(resp.Body)
}
