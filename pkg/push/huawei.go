package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// EnvHuaweiEnabled turns Huawei Push Kit on. Unset or any other value is off.
	EnvHuaweiEnabled = "TELLOMI_HUAWEI_PUSH"
	// EnvHuaweiAppID is the Push Kit client id (the app id).
	EnvHuaweiAppID = "TELLOMI_HUAWEI_APP_ID"
	// EnvHuaweiSecret is the Push Kit client secret. It is not logged.
	EnvHuaweiSecret = "TELLOMI_HUAWEI_APP_SECRET"
	// EnvHuaweiHost overrides the China send origin. Empty uses push-api.cloud.huawei.com.
	EnvHuaweiHost = "TELLOMI_HUAWEI_HOST"
	// EnvHuaweiTokenURL overrides the China OAuth endpoint.
	EnvHuaweiTokenURL = "TELLOMI_HUAWEI_TOKEN_URL"

	defaultHuaweiHost     = "https://push-api.cloud.huawei.com"
	defaultHuaweiTokenURL = "https://oauth-login.cloud.huawei.com/oauth2/v3/token"

	// huaweiHTTPTimeout bounds one HTTP call. The attempt budget covers the
	// token fetch and the send so one slow response cannot consume the
	// whole push deadline.
	huaweiHTTPTimeout    = 8 * time.Second
	huaweiAttemptTimeout = 8 * time.Second

	huaweiSuccess       = "80000000"
	huaweiPartial       = "80100000"
	huaweiAllTokensBad  = "80300007"
	huaweiTokenExpired  = "80200003"
	huaweiTokenRejected = "80200001"
)

// ErrHuaweiNotConfigured is returned until a Huawei Push Kit sender is installed.
// The Android client registers token_type 13 (PUSH_TYPE_HUAWEI). Xiaomi is
// token_type 14 (xiaomi.go). OPPO, vivo, Honor, and Meizu are not sent.
// Mainland China builds that cannot reach FCM use Huawei or Xiaomi.
var ErrHuaweiNotConfigured = errors.New("huawei push is not configured")

// HuaweiSender delivers the encrypted notification string to one HMS token.
// The string is the same "p" value FCM puts in its data map. The Tellomi
// Huawei listener base64-decodes that string directly.
type HuaweiSender interface {
	Send(ctx context.Context, token string, data string) error
}

type huaweiUnconfigured struct{}

func (huaweiUnconfigured) Send(context.Context, string, string) error {
	return ErrHuaweiNotConfigured
}

// HuaweiError is a Push Kit error response. Drop is true when the token is dead.
type HuaweiError struct {
	Code   string
	Reason string
	drop   bool
}

func (e *HuaweiError) Error() string {
	if e == nil {
		return "huawei push failed"
	}
	if e.Code == "" && e.Reason == "" {
		return "huawei push failed"
	}
	if e.Reason == "" {
		return "huawei push failed: " + e.Code
	}
	if e.Code == "" {
		return "huawei push failed: " + e.Reason
	}
	return "huawei push failed: " + e.Code + ": " + e.Reason
}

// Drop reports whether the device row should be deleted.
func (e *HuaweiError) Drop() bool {
	return e != nil && e.drop
}

// Huawei posts data messages to Push Kit.
type Huawei struct {
	host     string
	tokenURL string
	appID    string
	secret   string
	client   *http.Client

	mu     sync.Mutex
	cached string
	expiry time.Time
}

// LoadHuawei reads the environment. A nil client and a nil error means the
// flag is off. A non-nil error means the flag is on but the app id or the
// secret is missing; the process keeps running and this channel stays off.
func LoadHuawei() (*Huawei, error) {
	if os.Getenv(EnvHuaweiEnabled) != "1" {
		return nil, nil
	}
	appID := strings.TrimSpace(os.Getenv(EnvHuaweiAppID))
	secret := strings.TrimSpace(os.Getenv(EnvHuaweiSecret))
	if appID == "" || secret == "" {
		return nil, fmt.Errorf("%w: app id or secret empty", ErrHuaweiNotConfigured)
	}
	tokenURL := strings.TrimSpace(os.Getenv(EnvHuaweiTokenURL))
	if tokenURL == "" {
		tokenURL = defaultHuaweiTokenURL
	}
	host := strings.TrimSpace(os.Getenv(EnvHuaweiHost))
	if host == "" {
		host = defaultHuaweiHost
	}
	return NewHuawei(host, tokenURL, appID, secret), nil
}

// NewHuawei builds a sender. host is the origin, for example
// https://push-api.cloud.huawei.com. secret is the client secret.
func NewHuawei(host, tokenURL, appID, secret string) *Huawei {
	return &Huawei{
		host:     strings.TrimRight(strings.TrimSpace(host), "/"),
		tokenURL: strings.TrimSpace(tokenURL),
		appID:    strings.TrimSpace(appID),
		secret:   secret,
		client:   &http.Client{Timeout: huaweiHTTPTimeout},
	}
}

// Send posts one data message. category IM is Huawei's instant-message class,
// so a background delivery is not treated as marketing. The data string is
// the sealed p value; there is no notification title, so hidden previews
// cannot leak through the system tray.
func (h *Huawei) Send(ctx context.Context, token, data string) error {
	return h.send(ctx, token, data, "86400s", "IM", true)
}

// SendCall posts a ring. The TTL is one minute and the category is VOIP.
func (h *Huawei) SendCall(ctx context.Context, token, data string) error {
	return h.send(ctx, token, data, "60s", "VOIP", true)
}

func (h *Huawei) send(ctx context.Context, token, data, ttl, category string, retry bool) error {
	if h == nil || h.secret == "" || h.appID == "" || h.host == "" || h.tokenURL == "" {
		return ErrHuaweiNotConfigured
	}
	token = strings.TrimSpace(token)
	data = strings.TrimSpace(data)
	if token == "" || data == "" {
		return ErrHuaweiNotConfigured
	}
	if len(data) > 4096 {
		return fmt.Errorf("huawei data is %d bytes", len(data))
	}
	access, err := h.accessToken(ctx)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(map[string]any{
		"validate_only": false,
		"message": map[string]any{
			"data":  data,
			"token": []string{token},
			"android": map[string]string{
				"urgency":  "HIGH",
				"ttl":      ttl,
				"category": category,
			},
		},
	})
	if err != nil {
		return err
	}
	endpoint := h.host + "/v1/" + url.PathEscape(h.appID) + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	code, reason, illegal, success, failure, parsed := parseHuaweiBody(body)
	if huaweiAuthExpired(resp.StatusCode, code) && retry {
		h.clearToken()
		return h.send(ctx, token, data, ttl, category, false)
	}
	if resp.StatusCode == http.StatusOK && code == huaweiSuccess {
		return nil
	}
	he := &HuaweiError{Code: code, Reason: clipPushReason(reason)}
	if code == "" {
		he.Reason = clipPushReason(fmt.Sprintf("http %d", resp.StatusCode))
	}
	he.drop = huaweiShouldDrop(code, token, illegal, success, failure, parsed)
	return he
}

func huaweiAuthExpired(status int, code string) bool {
	if status == http.StatusUnauthorized {
		return true
	}
	return code == huaweiTokenExpired || code == huaweiTokenRejected
}

func huaweiShouldDrop(code, token string, illegal []string, success, failure int, parsed bool) bool {
	switch code {
	case huaweiAllTokensBad:
		return true
	case huaweiPartial:
		if !parsed {
			return true
		}
		for _, item := range illegal {
			if item == token {
				return true
			}
		}
		if len(illegal) > 0 {
			return true
		}
		return failure > 0 && success == 0
	default:
		return false
	}
}

func parseHuaweiBody(body []byte) (code, reason string, illegal []string, success, failure int, parsed bool) {
	var envelope struct {
		Code json.RawMessage `json:"code"`
		Msg  json.RawMessage `json:"msg"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return "", "", nil, 0, 0, false
	}
	code = huaweiScalar(envelope.Code)
	reason = huaweiScalar(envelope.Msg)
	if reason == "" && len(envelope.Msg) > 0 && string(bytes.TrimSpace(envelope.Msg)) != "null" {
		reason = string(envelope.Msg)
	}
	illegal, success, failure, parsed = huaweiIllegal(reason)
	if !parsed {
		illegal, success, failure, parsed = huaweiIllegal(string(envelope.Msg))
	}
	return code, reason, illegal, success, failure, parsed
}

func huaweiScalar(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var n int64
	if err := json.Unmarshal(raw, &n); err == nil {
		return strconv.FormatInt(n, 10)
	}
	return ""
}

func huaweiIllegal(msg string) (illegal []string, success, failure int, ok bool) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return nil, 0, 0, false
	}
	var obj struct {
		Success       int      `json:"success"`
		Failure       int      `json:"failure"`
		IllegalTokens []string `json:"illegal_tokens"`
	}
	if json.Unmarshal([]byte(msg), &obj) == nil && (obj.Failure != 0 || obj.Success != 0 || len(obj.IllegalTokens) > 0 || strings.Contains(msg, "illegal_tokens")) {
		return obj.IllegalTokens, obj.Success, obj.Failure, true
	}
	var inner string
	if json.Unmarshal([]byte(msg), &inner) == nil && inner != msg {
		return huaweiIllegal(inner)
	}
	return nil, 0, 0, false
}

func (h *Huawei) accessToken(ctx context.Context) (string, error) {
	h.mu.Lock()
	if h.cached != "" && time.Now().Before(h.expiry) {
		tok := h.cached
		h.mu.Unlock()
		return tok, nil
	}
	h.mu.Unlock()
	tok, exp, err := h.fetchToken(ctx)
	if err != nil {
		return "", err
	}
	h.mu.Lock()
	h.cached = tok
	h.expiry = exp
	h.mu.Unlock()
	return tok, nil
}

func (h *Huawei) clearToken() {
	h.mu.Lock()
	h.cached = ""
	h.expiry = time.Time{}
	h.mu.Unlock()
}

func (h *Huawei) fetchToken(ctx context.Context) (string, time.Time, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", h.appID)
	form.Set("client_secret", h.secret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := h.client.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("huawei token: http %d", resp.StatusCode)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.AccessToken == "" {
		return "", time.Time{}, errors.New("huawei token: empty access token")
	}
	ttl := parsed.ExpiresIn
	if ttl <= 0 {
		ttl = 300
	}
	skew := 60
	if ttl <= skew*2 {
		skew = ttl / 2
	}
	return parsed.AccessToken, time.Now().Add(time.Duration(ttl-skew) * time.Second), nil
}
