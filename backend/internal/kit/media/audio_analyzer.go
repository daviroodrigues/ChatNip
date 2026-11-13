package media

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type AudioMetadata struct {
	DurationSeconds uint32
	Waveform        []byte
}

func AnalyzeAudio(ctx context.Context, mediaData []byte) (*AudioMetadata, error) {

	tmpFile, err := os.CreateTemp("", "whatsapp_audio_*.ogg")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(mediaData); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}
	tmpFile.Close()

	duration, err := getDurationWithFFprobe(ctx, tmpFile.Name())
	if err != nil {
		// Logar erro se necessário, mas define fallback seguro
		duration = 0
	}

	waveform, err := generateRealWaveform(ctx, tmpFile.Name())
	if err != nil {

		waveform = make([]byte, 64)
	}

	return &AudioMetadata{
		DurationSeconds: duration,
		Waveform:        waveform,
	}, nil
}

func getDurationWithFFprobe(ctx context.Context, filePath string) (uint32, error) {
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		filePath,
	}

	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed: %w", err)
	}

	val := strings.TrimSpace(string(output))
	if val == "N/A" || val == "" {
		return 0, fmt.Errorf("duration N/A")
	}

	seconds, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, err
	}

	return uint32(math.Ceil(seconds)), nil
}

func generateRealWaveform(ctx context.Context, filePath string) ([]byte, error) {

	args := []string{
		"-i", filePath,
		"-ac", "1",
		"-ar", "4000",
		"-f", "s16le",
		"-acodec", "pcm_s16le",
		"-",
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	rawAudio, err := io.ReadAll(stdout)
	if err != nil {
		return nil, err
	}

	_ = cmd.Wait()

	if len(rawAudio) == 0 {
		return make([]byte, 64), nil
	}

	numSamples := len(rawAudio) / 2
	samples := make([]int16, numSamples)

	for i := 0; i < numSamples; i++ {

		samples[i] = int16(binary.LittleEndian.Uint16(rawAudio[i*2 : i*2+2]))
	}

	const targetBars = 64
	waveform := make([]byte, targetBars)

	blockSize := numSamples / targetBars
	if blockSize == 0 {
		blockSize = 1
	}

	for i := 0; i < targetBars; i++ {
		start := i * blockSize
		end := start + blockSize
		if end > numSamples {
			end = numSamples
		}

		var maxVal int16 = 0
		for j := start; j < end; j++ {

			val := samples[j]
			if val < 0 {
				val = -val
			}
			if val > maxVal {
				maxVal = val
			}
		}

		normalized := float64(maxVal) / 32768.0 * 100.0
		if normalized > 100 {
			normalized = 100
		}

		waveform[i] = byte(normalized)
	}

	return waveform, nil
}
