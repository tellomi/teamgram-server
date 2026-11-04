package push

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// EnvFCMEnabled turns the FCM HTTP v1 channel on. Unset or any other value is off.
	EnvFCMEnabled = "TELLOMI_FCM_PUSH"
	// EnvFCMCredentials is a path to a Firebase service-account JSON file.
	// The file is not committed and its private key is not logged.
	EnvFCMCredentials = "TELLOMI_FCM_CREDENTIALS"
	// EnvFCMProject overrides the project_id in that file. Empty uses the file.
	EnvFCMProject = "TELLOMI_FCM_PROJECT"
	// EnvFCMHost overrides the send origin. Empty uses fcm.googleapis.com.
	EnvFCMHost = "TELLOMI_FCM_HOST"
	// EnvFCMTokenURL overrides the OAuth token endpoint. Empty uses token_uri
	// from the service account, then oauth2.googleapis.com.
	EnvFCMTokenURL = "TELLOMI_FCM_TOKEN_URL"

	defaultFCMHost     = "https://fcm.googleapis.com"
	defaultFCMTokenURL = "https://oauth2.googleapis.com/token"
	fcmScope           = "https://www.googleapis.com/auth/firebase.messaging"

	// fcmHTTPTimeout bounds one HTTP call. oauth2.googleapis.com and
	// fcm.googleapis.com are often unreachable from Chengdu; without this
	// a dial can sit until the OS gives up.
	fcmHTTPTimeout = 8 * time.Second
	// fcmAttemptTimeout bounds token fetch plus send, including one 401 retry.
	fcmAttemptTimeout = 12 * time.Second
)

// fcmInFlight caps concurrent FCM attempts. Deliver never waits on a slot:
// when every slot is busy the attempt is logged and dropped. A blocked
// Google endpoint therefore cannot stall APNs, Huawei, or Xiaomi.
var fcmInFlight = make(chan struct{}, 32)

// ErrFCMNotConfigured is returned by the default FCM sender.
// Mainland China often cannot reach FCM. The sender stays off until
// TELLOMI_FCM_PUSH=1 and a service-account file are both set.
var ErrFCMNotConfigured = errors.New("fcm is not configured")

// FCMSender delivers a data payload to an FCM registration token.
// The JSON object's values are strings: loc_key, loc_args (a JSON array),
// custom (a JSON object), body (localized fallback), and p (the sealed
// loc_key object). tellomi/Telegram reads only p.
type FCMSender interface {
	Send(ctx context.Context, token string, payload []byte) error
}

type fcmUnconfigured struct{}

func (fcmUnconfigured) Send(context.Context, string, []byte) error {
	return ErrFCMNotConfigured
}

// FCMError is a non-success response from FCM HTTP v1.
// Drop is true when this registration token will never work again.
type FCMError struct {
	Status int
	Code   string
	Reason string
	drop   bool
}

func (e *FCMError) Error() string {
	if e == nil {
		return "fcm push failed"
	}
	if e.Code == "" && e.Reason == "" {
		if e.Status == 0 {
			return "fcm push failed"
		}
		return fmt.Sprintf("fcm push failed: http %d", e.Status)
	}
	if e.Reason == "" {
		return "fcm push failed: " + e.Code
	}
	if e.Code == "" {
		return "fcm push failed: " + e.Reason
	}
	return "fcm push failed: " + e.Code + ": " + e.Reason
}

// Drop reports whether the device row should be deleted.
func (e *FCMError) Drop() bool {
	return e != nil && e.drop
}

// FCM posts data messages to the FCM HTTP v1 API.
type FCM struct {
	host        string
	tokenURL    string
	projectID   string
	clientEmail string
	key         *rsa.PrivateKey
	client      *http.Client

	mu     sync.Mutex
	cached string
	expiry time.Time
}

type fcmServiceAccount struct {
	ProjectID   string `json:"project_id"`
	PrivateKey  string `json:"private_key"`
	ClientEmail string `json:"client_email"`
	TokenURI    string `json:"token_uri"`
}

// LoadFCM reads the environment. A nil client and a nil error means the
// flag is off. A non-nil error means the flag is on but the credentials
// file is missing or unusable; the process keeps running and this channel stays off.
func LoadFCM() (*FCM, error) {
	if os.Getenv(EnvFCMEnabled) != "1" {
		return nil, nil
	}
	path := strings.TrimSpace(os.Getenv(EnvFCMCredentials))
	if path == "" {
		return nil, fmt.Errorf("%w: credentials path empty", ErrFCMNotConfigured)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%w: read credentials: %v", ErrFCMNotConfigured, err)
	}
	var sa fcmServiceAccount
	if err := json.Unmarshal(raw, &sa); err != nil {
		return nil, fmt.Errorf("%w: credentials json", ErrFCMNotConfigured)
	}
	project := strings.TrimSpace(os.Getenv(EnvFCMProject))
	if project == "" {
		project = strings.TrimSpace(sa.ProjectID)
	}
	email := strings.TrimSpace(sa.ClientEmail)
	if project == "" || email == "" || strings.TrimSpace(sa.PrivateKey) == "" {
		return nil, fmt.Errorf("%w: credentials incomplete", ErrFCMNotConfigured)
	}
	key, err := parseFCMKey(sa.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("%w: private key", ErrFCMNotConfigured)
	}
	tokenURL := strings.TrimSpace(os.Getenv(EnvFCMTokenURL))
	if tokenURL == "" {
		tokenURL = strings.TrimSpace(sa.TokenURI)
	}
	if tokenURL == "" {
		tokenURL = defaultFCMTokenURL
	}
	host := strings.TrimSpace(os.Getenv(EnvFCMHost))
	if host == "" {
		host = defaultFCMHost
	}
	return NewFCM(host, tokenURL, project, email, key), nil
}

// NewFCM builds a sender. host is the origin, for example https://fcm.googleapis.com.
// tokenURL is the OAuth token endpoint. key is the service-account RSA key.
func NewFCM(host, tokenURL, projectID, clientEmail string, key *rsa.PrivateKey) *FCM {
	return &FCM{
		host:        strings.TrimRight(strings.TrimSpace(host), "/"),
		tokenURL:    strings.TrimSpace(tokenURL),
		projectID:   strings.TrimSpace(projectID),
		clientEmail: strings.TrimSpace(clientEmail),
		key:         key,
		client:      &http.Client{Timeout: fcmHTTPTimeout},
	}
}

// Send posts one data message. There is no notification block: the client
// decrypts p and draws the banner, and a notification block would show text
// the preview setting had already removed. A phoneCallRequested payload uses
// a one-minute TTL. Any other message is kept for a day.
func (f *FCM) Send(ctx context.Context, token string, payload []byte) error {
	if f == nil || f.key == nil || f.projectID == "" || f.clientEmail == "" || f.host == "" || f.tokenURL == "" {
		return ErrFCMNotConfigured
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrFCMNotConfigured
	}
	var data map[string]string
	if err := json.Unmarshal(payload, &data); err != nil || len(data) == 0 {
		return errors.New("fcm payload is empty")
	}
	ttl := "86400s"
	if data["loc_key"] == locKeyPhoneCallRequest {
		ttl = "60s"
	}
	body, err := json.Marshal(map[string]any{
		"message": map[string]any{
			"token": token,
			"data":  data,
			"android": map[string]any{
				"priority": "HIGH",
				"ttl":      ttl,
			},
		},
	})
	if err != nil {
		return err
	}
	return f.post(ctx, body, true)
}

func (f *FCM) post(ctx context.Context, raw []byte, retry bool) error {
	access, err := f.accessToken(ctx)
	if err != nil {
		return err
	}
	endpoint := f.host + "/v1/projects/" + url.PathEscape(f.projectID) + "/messages:send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode == http.StatusUnauthorized && retry {
		f.clearToken()
		return f.post(ctx, raw, false)
	}
	return fcmErrorFrom(resp.StatusCode, respBody)
}

func (f *FCM) accessToken(ctx context.Context) (string, error) {
	f.mu.Lock()
	if f.cached != "" && time.Now().Before(f.expiry) {
		tok := f.cached
		f.mu.Unlock()
		return tok, nil
	}
	f.mu.Unlock()
	tok, exp, err := f.fetchToken(ctx)
	if err != nil {
		return "", err
	}
	f.mu.Lock()
	f.cached = tok
	f.expiry = exp
	f.mu.Unlock()
	return tok, nil
}

func (f *FCM) clearToken() {
	f.mu.Lock()
	f.cached = ""
	f.expiry = time.Time{}
	f.mu.Unlock()
}

func (f *FCM) fetchToken(ctx context.Context) (string, time.Time, error) {
	assertion, err := f.assertion(time.Now())
	if err != nil {
		return "", time.Time{}, err
	}
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.client.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", time.Time{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return "", time.Time{}, fmt.Errorf("fcm token: http %d", resp.StatusCode)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || parsed.AccessToken == "" {
		return "", time.Time{}, errors.New("fcm token: empty access token")
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

func (f *FCM) assertion(now time.Time) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{
		"iss":   f.clientEmail,
		"scope": fcmScope,
		"aud":   f.tokenURL,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}
	signing := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func parseFCMKey(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("fcm key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		key, err2 := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err2 != nil {
			return nil, err
		}
		return key, nil
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("fcm key is not RSA")
	}
	return key, nil
}

type fcmFieldViolation struct {
	Field       string `json:"field"`
	Description string `json:"description"`
}

type fcmErrorDetail struct {
	ErrorCode       string              `json:"errorCode"`
	FieldViolations []fcmFieldViolation `json:"fieldViolations"`
}

func fcmErrorFrom(status int, body []byte) error {
	var parsed struct {
		Error struct {
			Code    int              `json:"code"`
			Message string           `json:"message"`
			Status  string           `json:"status"`
			Details []fcmErrorDetail `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil || (parsed.Error.Status == "" && parsed.Error.Message == "" && parsed.Error.Code == 0) {
		return &FCMError{Status: status, Reason: "http"}
	}
	code := parsed.Error.Status
	var violations []fcmFieldViolation
	for _, d := range parsed.Error.Details {
		if d.ErrorCode != "" {
			code = d.ErrorCode
		}
		violations = append(violations, d.FieldViolations...)
	}
	fe := &FCMError{Status: status, Code: code, Reason: clipPushReason(parsed.Error.Message)}
	switch code {
	case "UNREGISTERED", "SENDER_ID_MISMATCH", "NOT_FOUND":
		fe.drop = true
	case "INVALID_ARGUMENT":
		fe.drop = fcmTokenArgument(parsed.Error.Message, violations)
	}
	if status == http.StatusNotFound && !fe.drop {
		fe.drop = true
		if fe.Code == "" {
			fe.Code = "NOT_FOUND"
		}
	}
	return fe
}

func fcmTokenArgument(msg string, violations []fcmFieldViolation) bool {
	if strings.Contains(strings.ToLower(msg), "registration token") {
		return true
	}
	for _, v := range violations {
		f := strings.ToLower(v.Field + " " + v.Description)
		if strings.Contains(f, "token") {
			return true
		}
	}
	return false
}

func clipPushReason(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 180 {
		return s[:180]
	}
	return s
}
