package speech

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
		Client:   asrClient(timeout),
	}
}

// asrClient keeps the long read timeout for a real transcription, but a
// sidecar that does not accept a connection fails in under a second. The
// session acks the RPC only after the handler returns, and the client
// regression gives that about two seconds.
func asrClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   500 * time.Millisecond,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          4,
			IdleConnTimeout:       30 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}

// HTTPRecognizer posts audio bytes and reads {"text":"..."}.
type HTTPRecognizer struct {
	Endpoint string
	Client   *http.Client
}

type asrResponse struct {
	Text     string `json:"text"`
	Language string `json:"language"`
	Error    string `json:"error"`
}

// Transcribe implements Recognizer. It does not set a language, so stored
// voice notes and round videos stay on SenseVoice auto-detect.
func (h *HTTPRecognizer) Transcribe(ctx context.Context, audio []byte, filename string) (string, error) {
	text, _, err := h.post(ctx, audio, filename, "")
	return text, err
}

// TranscribeDraft asks the sidecar for Chinese. A non-zh language, or a
// sidecar that does not report one, is TRANSCRIPTION_LANG_UNSUPPORTED.
func (h *HTTPRecognizer) TranscribeDraft(ctx context.Context, audio []byte, filename string) (string, error) {
	text, lang, err := h.post(ctx, audio, filename, "zh")
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", nil
	}
	if lang != "zh" {
		return "", ErrDraftLanguage
	}
	return text, nil
}

func (h *HTTPRecognizer) post(ctx context.Context, audio []byte, filename, language string) (string, string, error) {
	if h == nil || h.Endpoint == "" {
		return "", "", fmt.Errorf("asr endpoint is unset")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.Endpoint, bytes.NewReader(audio))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if filename != "" {
		req.Header.Set("X-Audio-Name", filename)
	}
	if language != "" {
		req.Header.Set("X-ASR-Language", language)
	}
	client := h.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	var parsed asrResponse
	_ = json.Unmarshal(body, &parsed)
	if resp.StatusCode == http.StatusBadRequest && parsed.Error == "too_long" {
		return "", "", ErrAudioTooLong
	}
	if parsed.Error == "language" {
		return "", parsed.Language, ErrDraftLanguage
	}
	if resp.StatusCode != http.StatusOK {
		if parsed.Error == "" {
			parsed.Error = resp.Status
		}
		return "", "", fmt.Errorf("asr %s: %s", resp.Status, parsed.Error)
	}
	return parsed.Text, parsed.Language, nil
}
