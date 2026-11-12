package tellomiai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// EnvTranslateURL is the self-hosted translator. Empty means translation is off.
	EnvTranslateURL = "TELLOMI_TRANSLATE_URL"
	// EnvTranslateAuto allows translations_auto_enabled when the sidecar is healthy.
	// Unset keeps automatic translation off even if the model is up.
	EnvTranslateAuto = "TELLOMI_TRANSLATE_AUTO"
)

// ErrLocal is a self-hosted translator that is down or refused the request
// for a reason other than overload. Callers must not send the text to
// another provider.
var ErrLocal = errorsNew("tellomi translate: local sidecar")

// ErrSidecarBusy is HTTP 503 from the local translator. The sidecar refused
// the request immediately. Callers map this to a 400 RPC error. A 500 would
// be resent by the client transport before the UI callback runs.
var ErrSidecarBusy = errorsNew("tellomi translate: busy")

func errorsNew(s string) error { return &localError{s} }

type localError struct{ s string }

func (e *localError) Error() string { return e.s }

// LocalReady is true only when TELLOMI_TRANSLATE_URL answers /health.
// A Bailian key does not make this true.
func LocalReady() bool {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv(EnvTranslateURL)), "/")
	if base == "" {
		return false
	}
	return healthOK(base)
}

// AutoReady is LocalReady plus TELLOMI_TRANSLATE_AUTO=1.
func AutoReady() bool {
	return LocalReady() && envOn(EnvTranslateAuto)
}

type healthCache struct {
	mu    sync.Mutex
	base  string
	ok    bool
	until time.Time
}

var cachedHealth healthCache

func healthOK(base string) bool {
	cachedHealth.mu.Lock()
	defer cachedHealth.mu.Unlock()
	now := time.Now()
	if cachedHealth.base == base && now.Before(cachedHealth.until) {
		return cachedHealth.ok
	}
	ok := probeHealth(base)
	cachedHealth.base = base
	cachedHealth.ok = ok
	cachedHealth.until = now.Add(30 * time.Second)
	return ok
}

func probeHealth(base string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode == http.StatusOK
}

func translateLocal(ctx context.Context, texts []string, to string) ([]string, error) {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv(EnvTranslateURL)), "/")
	if base == "" || !healthOK(base) {
		return nil, ErrDisabled
	}
	body, err := json.Marshal(map[string]any{
		"texts": texts,
		"from":  "en",
		"to":    "zh",
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/v1/translate", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, ErrLocal
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, ErrLocal
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return nil, ErrSidecarBusy
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ErrLocal
	}
	var parsed struct {
		Texts []string `json:"texts"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || len(parsed.Texts) != len(texts) {
		return nil, ErrLocal
	}
	return parsed.Texts, nil
}

// ChineseTarget reports whether to_lang is simplified Chinese.
func ChineseTarget(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "zh", "zh-cn", "zh-hans":
		return true
	default:
		return false
	}
}
