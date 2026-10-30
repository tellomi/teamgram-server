// Package resend sends mail through the Resend HTTP API.
//
// Production email OTP uses this client. Constructing a Client does not dial.
// SendCode fails with a configuration error when the API key is empty, so the
// process can boot before a key is set.
//
//	POST https://api.resend.com/emails
//	Authorization: Bearer APIKey
//	{"from":"Tellomi <noreply@tellomi.app>","to":["user@example.com"],...}
//
// The API key is read from the environment by the caller. This package does
// not embed a secret.
package resend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultEndpoint = "https://api.resend.com/emails"
	otpSubject      = "Tellomi 验证码 / Verification code"
)

// Client is a Resend HTTP API sender.
type Client struct {
	APIKey   string
	From     string
	FromName string
	// Endpoint overrides the API URL. Empty uses https://api.resend.com/emails.
	// Tests set this; production leaves it empty.
	Endpoint string
	HTTP     *http.Client
}

type sendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

type sendResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Message    string `json:"message"`
	StatusCode int    `json:"statusCode"`
}

// SendCode delivers a numeric verification code. It does not log the code or the API key.
func (c *Client) SendCode(ctx context.Context, to, code string) error {
	if c == nil {
		return fmt.Errorf("resend client is nil")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("resend api key is not configured")
	}
	from := strings.TrimSpace(c.From)
	if from == "" {
		return fmt.Errorf("resend from address is not configured")
	}
	to = strings.TrimSpace(to)
	if to == "" || !strings.Contains(to, "@") {
		return fmt.Errorf("resend recipient is not an email address")
	}

	body, err := json.Marshal(sendRequest{
		From:    formatFrom(c.FromName, from),
		To:      []string{to},
		Subject: otpSubject,
		Text:    messageText(code),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(c.APIKey))

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	var parsed sendResponse
	jerr := json.Unmarshal(raw, &parsed)
	if resp.StatusCode >= 300 {
		if jerr != nil {
			return fmt.Errorf("resend http %d: %s", resp.StatusCode, truncate(raw))
		}
		msg := parsed.Message
		if msg == "" {
			msg = truncate(raw)
		}
		if parsed.Name != "" {
			return fmt.Errorf("resend %s: %s", parsed.Name, msg)
		}
		return fmt.Errorf("resend http %d: %s", resp.StatusCode, msg)
	}
	if jerr != nil {
		return fmt.Errorf("resend http %d: %s", resp.StatusCode, truncate(raw))
	}
	if strings.TrimSpace(parsed.ID) == "" {
		return fmt.Errorf("resend http %d: missing id", resp.StatusCode)
	}
	return nil
}

func (c *Client) endpoint() string {
	if ep := strings.TrimSpace(c.Endpoint); ep != "" {
		return ep
	}
	return defaultEndpoint
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func messageText(code string) string {
	return fmt.Sprintf("您的验证码为：%s，请勿泄露于他人！\nYour verification code is %s. Do not share it.", code, code)
}

func formatFrom(name, addr string) string {
	name = strings.TrimSpace(name)
	addr = strings.TrimSpace(addr)
	if name == "" {
		return addr
	}
	name = strings.NewReplacer("\r", "", "\n", "", "<", "", ">", "", "\"", "").Replace(name)
	return name + " <" + addr + ">"
}

func truncate(b []byte) string {
	s := string(b)
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
