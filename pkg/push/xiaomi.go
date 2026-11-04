package push

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	// EnvXiaomiEnabled turns the Xiaomi channel on. Unset or any other value is off.
	EnvXiaomiEnabled = "TELLOMI_XIAOMI_PUSH"
	// EnvXiaomiSecret is the MiPush AppSecret. It is not logged.
	EnvXiaomiSecret = "TELLOMI_XIAOMI_APP_SECRET"
	// EnvXiaomiPackage is restricted_package_name (the Android application id).
	EnvXiaomiPackage = "TELLOMI_XIAOMI_PACKAGE"
	// EnvXiaomiHost overrides the China endpoint. Empty uses api.xmpush.xiaomi.com.
	EnvXiaomiHost = "TELLOMI_XIAOMI_HOST"

	defaultXiaomiHost = "https://api.xmpush.xiaomi.com"
	xiaomiRegIDPath   = "/v3/message/regid"
	// XiaomiInvalidRegID is the MiPush code for a registration id that no longer exists.
	XiaomiInvalidRegID = 20301
)

// ErrXiaomiNotConfigured is returned until a Xiaomi sender is installed.
var ErrXiaomiNotConfigured = errors.New("xiaomi push is not configured")

// XiaomiMessage is one notification-bar message. Payload is the same "p"
// ciphertext Huawei sends when the device registered a secret, otherwise the
// FCM-shaped JSON. The system tray shows Title and Description.
type XiaomiMessage struct {
	Title       string
	Description string
	Payload     string
	Silent      bool
}

// XiaomiSender delivers one message to a MiPush registration id (token_type 14).
type XiaomiSender interface {
	Send(ctx context.Context, token string, msg XiaomiMessage) error
}

type xiaomiUnconfigured struct{}

func (xiaomiUnconfigured) Send(context.Context, string, XiaomiMessage) error {
	return ErrXiaomiNotConfigured
}

// XiaomiError is a MiPush error response. Drop is true when the regid is dead.
type XiaomiError struct {
	Code   int
	Reason string
}

func (e *XiaomiError) Error() string {
	if e == nil {
		return "xiaomi push failed"
	}
	if e.Reason == "" {
		return "xiaomi push failed"
	}
	return "xiaomi push failed: " + e.Reason
}

// Drop reports whether the device row should be deleted.
func (e *XiaomiError) Drop() bool {
	return e != nil && e.Code == XiaomiInvalidRegID
}

// Xiaomi posts notification messages to MiPush.
type Xiaomi struct {
	host   string
	secret string
	pkg    string
	client *http.Client
}

// LoadXiaomi reads the environment. A nil client and a nil error means the
// flag is off. A non-nil error means the flag is on but the secret or the
// package name is missing; the process keeps running and this channel stays off.
func LoadXiaomi() (*Xiaomi, error) {
	if os.Getenv(EnvXiaomiEnabled) != "1" {
		return nil, nil
	}
	secret := strings.TrimSpace(os.Getenv(EnvXiaomiSecret))
	pkg := strings.TrimSpace(os.Getenv(EnvXiaomiPackage))
	if secret == "" || pkg == "" {
		return nil, ErrXiaomiNotConfigured
	}
	host := strings.TrimSpace(os.Getenv(EnvXiaomiHost))
	if host == "" {
		host = defaultXiaomiHost
	}
	return NewXiaomi(host, secret, pkg), nil
}

// NewXiaomi builds a sender. host is the origin, for example
// https://api.xmpush.xiaomi.com. secret is the AppSecret.
func NewXiaomi(host, secret, pkg string) *Xiaomi {
	return &Xiaomi{
		host:   strings.TrimRight(strings.TrimSpace(host), "/"),
		secret: secret,
		pkg:    pkg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Send posts one regid message. pass_through is 0 so the phone shows a
// notification when the process is not running. Other brands receive this
// only when they have the Xiaomi Push SDK; that limit is the client's.
func (x *Xiaomi) Send(ctx context.Context, token string, msg XiaomiMessage) error {
	if x == nil || x.secret == "" || x.pkg == "" || x.host == "" {
		return ErrXiaomiNotConfigured
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.TrimSpace(msg.Payload) == "" {
		return ErrXiaomiNotConfigured
	}
	form := url.Values{}
	form.Set("registration_id", token)
	form.Set("restricted_package_name", x.pkg)
	form.Set("payload", msg.Payload)
	form.Set("title", msg.Title)
	form.Set("description", msg.Description)
	form.Set("pass_through", "0")
	form.Set("notify_type", "-1")
	if msg.Silent {
		form.Set("notify_type", "0")
	}
	form.Set("time_to_live", "86400000")
	form.Set("extra.notify_foreground", "0")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, x.host+xiaomiRegIDPath, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded;charset=UTF-8")
	req.Header.Set("Authorization", "key="+x.secret)
	resp, err := x.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return &XiaomiError{Code: resp.StatusCode, Reason: "http"}
	}
	var parsed struct {
		Result string `json:"result"`
		Code   int    `json:"code"`
		Reason string `json:"reason"`
		Desc   string `json:"description"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return err
	}
	if parsed.Code == 0 && (parsed.Result == "" || parsed.Result == "ok") {
		return nil
	}
	reason := parsed.Reason
	if reason == "" {
		reason = parsed.Desc
	}
	return &XiaomiError{Code: parsed.Code, Reason: reason}
}
