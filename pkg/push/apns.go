package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// APNS sends alert pushes with HTTP/2 token authentication (.p8).
type APNS struct {
	cfg        Config
	key        *ecdsa.PrivateKey
	httpClient *http.Client

	mu        sync.Mutex
	jwt       string
	jwtIssued time.Time
}

func NewAPNS(cfg Config) (*APNS, error) {
	if cfg.KeyID == "" || cfg.TeamID == "" || cfg.Topic == "" {
		return nil, fmt.Errorf("apns key id, team id, and topic are required")
	}
	pemBytes := cfg.KeyPEM
	if len(pemBytes) == 0 {
		var err error
		pemBytes, err = os.ReadFile(cfg.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("read apns key: %w", err)
		}
	}
	key, err := parseP8(pemBytes)
	if err != nil {
		return nil, err
	}
	return &APNS{
		cfg: cfg,
		key: key,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				ForceAttemptHTTP2: true,
			},
		},
	}, nil
}

// Send posts payload to deviceToken. deviceSandbox selects the sandbox host
// unless Config.Sandbox forces it for every device.
func (a *APNS) Send(ctx context.Context, deviceToken string, deviceSandbox bool, payload []byte) error {
	token := normalizeToken(deviceToken)
	if token == "" {
		return fmt.Errorf("empty device token")
	}
	jwt, err := a.bearer()
	if err != nil {
		return err
	}
	url := a.cfg.host(deviceSandbox) + "/3/device/" + token
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", a.cfg.Topic)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	// A zero expiration tells APNs to drop the notification when the device is
	// offline. Store it for a day so a suspended iOS client still gets it.
	req.Header.Set("apns-expiration", fmt.Sprintf("%d", time.Now().Add(24*time.Hour).Unix()))
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	reason := apnsReason(body)
	err = &APNSError{Status: resp.StatusCode, Reason: reason}
	if reason == "Unregistered" || reason == "BadDeviceToken" || reason == "DeviceTokenNotForTopic" {
		return err
	}
	return err
}

// APNSError is a non-2xx response from APNs.
type APNSError struct {
	Status int
	Reason string
}

func (e *APNSError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("apns status %d", e.Status)
	}
	return fmt.Sprintf("apns status %d: %s", e.Status, e.Reason)
}

// DropDevice reports whether the token should be deleted.
func (e *APNSError) DropDevice() bool {
	switch e.Reason {
	case "Unregistered", "BadDeviceToken", "DeviceTokenNotForTopic":
		return true
	default:
		return false
	}
}

func apnsReason(body []byte) string {
	var r struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(body, &r)
	return r.Reason
}

func (a *APNS) bearer() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.jwt != "" && time.Since(a.jwtIssued) < 50*time.Minute {
		return a.jwt, nil
	}
	token, err := signJWT(a.key, a.cfg.KeyID, a.cfg.TeamID, time.Now())
	if err != nil {
		return "", err
	}
	a.jwt = token
	a.jwtIssued = time.Now()
	return token, nil
}

func signJWT(key *ecdsa.PrivateKey, kid, teamID string, now time.Time) (string, error) {
	header, err := json.Marshal(map[string]string{"alg": "ES256", "kid": kid})
	if err != nil {
		return "", err
	}
	claims, err := json.Marshal(map[string]any{"iss": teamID, "iat": now.Unix()})
	if err != nil {
		return "", err
	}
	signing := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	putES256(sig[:32], r)
	putES256(sig[32:], s)
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func putES256(dst []byte, n *big.Int) {
	b := n.Bytes()
	if len(b) > len(dst) {
		b = b[len(b)-len(dst):]
	}
	copy(dst[len(dst)-len(b):], b)
}

func parseP8(pemBytes []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("apns key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		if key, err2 := x509.ParseECPrivateKey(block.Bytes); err2 == nil {
			return key, nil
		}
		return nil, fmt.Errorf("parse apns key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("apns key is %T, want ECDSA P-256", parsed)
	}
	return key, nil
}

func normalizeToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, "<>")
	token = strings.ReplaceAll(token, " ", "")
	return strings.ToLower(token)
}
