package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const recognizeTimeout = 120 * time.Second

// RecognizerFromEnv is the SenseVoice sidecar client.
// TELLOMI_ASR_URL defaults to the compose service. TELLOMI_ASR_TIMEOUT is seconds.
func RecognizerFromEnv() Recognizer {
	base := strings.TrimSpace(os.Getenv("TELLOMI_ASR_URL"))
	if base == "" {
		base = "http://asr:8080"
	}
	timeout := recognizeTimeout
	if raw := strings.TrimSpace(os.Getenv("TELLOMI_ASR_TIMEOUT")); raw != "" {
		if sec, err := time.ParseDuration(raw + "s"); err == nil && sec > 0 {
			timeout = sec
		}
	}
	return &HTTPRecognizer{
		Endpoint: strings.TrimRight(base, "/") + "/v1/transcribe",
		Client:   &http.Client{Timeout: timeout},
	}
}

// HTTPRecognizer posts audio bytes and reads {"text":"..."}.
type HTTPRecognizer struct {
	Endpoint string
	Client   *http.Client
}

type asrResponse struct {
	Text  string `json:"text"`
	Error string `json:"error"`
}

// Transcribe implements Recognizer.
func (h *HTTPRecognizer) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	if h == nil || h.Endpoint == "" {
		return "", fmt.Errorf("asr endpoint is unset")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.Endpoint, bytes.NewReader(audio))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if filename != "" {
		req.Header.Set("X-Audio-Name", filename)
	}
	client := h.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var parsed asrResponse
	_ = json.Unmarshal(body, &parsed)
	if resp.StatusCode == http.StatusBadRequest && parsed.Error == "too_long" {
		return "", ErrAudioTooLong
	}
	if resp.StatusCode != http.StatusOK {
		if parsed.Error == "" {
			parsed.Error = resp.Status
		}
		return "", fmt.Errorf("asr %s: %s", resp.Status, parsed.Error)
	}
	return parsed.Text, nil
}
