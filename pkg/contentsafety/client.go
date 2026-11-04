package contentsafety

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/verify/aliyun"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	envEnabled  = "TELLOMI_CONTENT_SAFETY"
	envEndpoint = "TELLOMI_CONTENT_SAFETY_ENDPOINT"
	envService  = "TELLOMI_CONTENT_SAFETY_SERVICE"
	envTimeout  = "TELLOMI_CONTENT_SAFETY_TIMEOUT"
	envKeyID    = "TELLOMI_CONTENT_SAFETY_ACCESS_KEY_ID"
	envSecret   = "TELLOMI_CONTENT_SAFETY_ACCESS_KEY_SECRET"

	apiVersion = "2022-03-02"
)

// defaultTimeout bounds one moderation call so a stalled endpoint cannot
// hold account.updateProfile open.
const defaultTimeout = 3 * time.Second

// Decide reports whether text may be saved. Allowed and Fallback both mean
// the caller saves it. Blocked means the service refused it. Fallback is
// logged. The returned string never contains the text or a credential.
func Decide(ctx context.Context, text string) string {
	text = strings.TrimSpace(text)
	if text == "" || !containsHan(text) {
		return Allowed
	}
	action, service, endpoint, on := settings()
	if !on {
		return Allowed
	}
	keyID, secret := credentials()
	if keyID == "" || secret == "" {
		logx.WithContext(ctx).Errorf("contentsafety: fallback allow reason=missing_access_key")
		return Fallback
	}
	timeout := callTimeout()
	outcome, labels, risk, code, requestID := call(ctx, endpoint, action, service, keyID, secret, text, timeout)
	switch outcome {
	case Blocked:
		logx.WithContext(ctx).Errorf("contentsafety: blocked risk=%s labels=%s code=%s request_id=%s", risk, labels, code, requestID)
	case Fallback:
		logx.WithContext(ctx).Errorf("contentsafety: fallback allow code=%s request_id=%s", code, requestID)
	}
	return outcome
}

func settings() (action, service, endpoint string, on bool) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envEnabled))) {
	case "", "0", "off", "false", "no":
		return "", "", "", false
	case "bailian", "plus":
		action = "TextModerationPlus"
		service = "nickname_detection_pro"
		endpoint = "https://green-cip.cn-shanghai.aliyuncs.com/"
		on = true
	default:
		action = "TextModeration"
		service = "nickname_detection"
		endpoint = "https://green-cip.cn-chengdu.aliyuncs.com/"
		on = true
	}
	if v := strings.TrimSpace(os.Getenv(envService)); v != "" {
		service = v
	}
	if v := strings.TrimSpace(os.Getenv(envEndpoint)); v != "" {
		endpoint = v
	}
	return action, service, endpoint, on
}

func credentials() (string, string) {
	id := strings.TrimSpace(os.Getenv(envKeyID))
	if id == "" {
		id = strings.TrimSpace(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID"))
	}
	secret := strings.TrimSpace(os.Getenv(envSecret))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET"))
	}
	return id, secret
}

func callTimeout() time.Duration {
	v := strings.TrimSpace(os.Getenv(envTimeout))
	if v == "" {
		return defaultTimeout
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return defaultTimeout
	}
	return d
}

func call(ctx context.Context, endpoint, action, service, keyID, secret, text string, timeout time.Duration) (outcome, labels, risk, code, requestID string) {
	payload, err := json.Marshal(map[string]string{"content": text})
	if err != nil {
		return Fallback, "", "", "", ""
	}
	params := map[string]string{
		"Format":            "JSON",
		"Version":           apiVersion,
		"AccessKeyId":       keyID,
		"SignatureMethod":   "HMAC-SHA1",
		"SignatureVersion":  "1.0",
		"SignatureNonce":    nonce(),
		"Timestamp":         time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Action":            action,
		"Service":           service,
		"ServiceParameters": string(payload),
	}
	body := aliyun.FormBody(params, secret)
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return Fallback, "", "", "", ""
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	transport := transportFor(timeout)
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return Fallback, "", "", "", ""
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Fallback, "", "", "", ""
	}
	return interpret(resp.StatusCode, raw)
}

func nonce() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func transportFor(timeout time.Duration) *http.Transport {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	dial := timeout
	if dial > 2*time.Second {
		dial = 2 * time.Second
	}
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   dial,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   dial,
		ResponseHeaderTimeout: timeout,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
}
