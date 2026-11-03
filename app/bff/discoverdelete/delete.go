// Package discoverdelete tells content-engine that a teamgram account is gone.
//
// DELETE /internal/users/{sub} runs after the account row is already deleted.
// A failure is retried in the background. It never returns an error to the
// delete-account RPC.
package discoverdelete

import (
	"context"
	"crypto/hmac"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/discoverauth"
)

const (
	// EnvEnabled turns the callback on. Unset or any other value is off.
	EnvEnabled = "TELLOMI_DISCOVER_DELETE"
	// EnvEngineURL is the engine origin, no path. https, or http on loopback.
	EnvEngineURL = "TELLOMI_DISCOVER_ENGINE_URL"
	// EnvServiceKey is standard base64 of the engine ENGINE_SERVICE_KEY bytes.
	EnvServiceKey = "TELLOMI_DISCOVER_SERVICE_KEY"

	maxAttempts = 8
)

// Outcome is one HTTP attempt.
type Outcome int

const (
	OutcomeDeleted Outcome = iota
	OutcomeRetry
	OutcomeAuth
	OutcomePermanent
)

// Result is the status of one attempt. Status 0 means there was no response.
type Result struct {
	Outcome Outcome
	Status  int
}

var (
	errConfig = errors.New("discoverdelete: config")

	// secondDeleteAfter is when a second DELETE is sent. A credential issued
	// just before deletion stays valid for an hour.
	secondDeleteAfter = 70 * time.Minute
	requestTimeout    = 10 * time.Second
)

type settings struct {
	base      *url.URL
	bearer    string
	pseudonym []byte
}

func enabled() bool {
	return strings.TrimSpace(osGetenv(EnvEnabled)) == "1"
}

// osGetenv is a var so tests can avoid the process environment.
var osGetenv = func(key string) string {
	return getenv(key)
}

func load() (settings, error) {
	rawURL := strings.TrimSpace(osGetenv(EnvEngineURL))
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return settings{}, errConfig
	}
	if u.Path != "" && u.Path != "/" {
		return settings{}, errConfig
	}
	u.Path = ""
	host := strings.ToLower(u.Hostname())
	loopback := host == "localhost" || host == "127.0.0.1" || host == "::1"
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && !(scheme == "http" && loopback) {
		return settings{}, errConfig
	}
	key, err := decodeKey(osGetenv(EnvServiceKey))
	if err != nil {
		return settings{}, err
	}
	pseudo, err := decodeKey(osGetenv(discoverauth.EnvPseudonymKey))
	if err != nil {
		return settings{}, err
	}
	if hmac.Equal(key, pseudo) {
		return settings{}, errConfig
	}
	return settings{
		base:      u,
		bearer:    "Bearer " + base64.StdEncoding.EncodeToString(key),
		pseudonym: pseudo,
	}, nil
}

func decodeKey(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errConfig
	}
	buf, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		buf, err = base64.RawStdEncoding.DecodeString(raw)
	}
	if err != nil || len(buf) < 32 {
		return nil, errConfig
	}
	return buf, nil
}

// Call performs one DELETE. Redirects are not followed. The body is discarded
// and not logged.
func Call(ctx context.Context, client *http.Client, cfg settings, sub []byte) Result {
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	client = redirectStop(client)
	endpoint := cfg.base.String() + "/internal/users/" + base64.RawURLEncoding.EncodeToString(sub)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return Result{Outcome: OutcomeRetry}
	}
	req.Header.Set("Authorization", cfg.bearer)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "tellomi-teamgram")
	resp, err := client.Do(req)
	if err != nil {
		return Result{Outcome: OutcomeRetry}
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	_ = resp.Body.Close()
	return classify(resp.StatusCode)
}

func redirectStop(client *http.Client) *http.Client {
	clone := *client
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	if clone.Timeout == 0 {
		clone.Timeout = requestTimeout
	}
	return &clone
}

func classify(status int) Result {
	// deletion.go: 200 is success and idempotent. 503 service_disabled and
	// other 5xx are retryable. 401 is a bad service key. 400 invalid_sub
	// will not succeed on retry. 404 is not the unconfigured-key response.
	switch {
	case status == http.StatusOK:
		return Result{Outcome: OutcomeDeleted, Status: status}
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return Result{Outcome: OutcomeAuth, Status: status}
	case status == http.StatusServiceUnavailable || status >= 500 || status == http.StatusRequestTimeout || status == http.StatusTooManyRequests || status == http.StatusTooEarly:
		return Result{Outcome: OutcomeRetry, Status: status}
	default:
		return Result{Outcome: OutcomePermanent, Status: status}
	}
}

func getenv(key string) string {
	return realGetenv(key)
}
