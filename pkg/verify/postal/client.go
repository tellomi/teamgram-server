// Package postal sends mail through a self-hosted Postal server.
//
// Delivery is Postal's HTTP API, not SMTP. Tellomi cannot use outbound TCP 25
// or 587, and Postal itself may not be deployed yet. Constructing a Client
// does not dial. SendCode fails with a configuration error when the base URL
// or API key is empty, so the process can boot before Postal exists.
//
//	POST {BaseURL}/api/v1/send/message
//	X-Server-API-Key: APIKey
//	{"to":["user@example.com"],"from":"Tellomi <noreply@tellomi.app>",...}
//
// If BaseURL already ends in /api/v1/send/message, that URL is used as-is.
package postal

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

const sendPath = "/api/v1/send/message"

// Client is a Postal HTTP API sender.
type Client struct {
	BaseURL  string
	APIKey   string
	From     string
	FromName string
	HTTP     *http.Client
}

type sendRequest struct {
	To        []string `json:"to"`
	From      string   `json:"from"`
	Sender    string   `json:"sender,omitempty"`
	Subject   string   `json:"subject"`
	PlainBody string   `json:"plain_body"`
}

type sendResponse struct {
	Status string `json:"status"`
	Data   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"data"`
}

// SendCode delivers a numeric verification code. It does not log the code or the API key.
func (c *Client) SendCode(ctx context.Context, to, code string) error {
	if c == nil {
		return fmt.Errorf("postal client is nil")
	}
	base := strings.TrimSpace(c.BaseURL)
	if base == "" {
		return fmt.Errorf("postal base url is not configured")
	}
	if strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("postal api key is not configured")
	}
	from := strings.TrimSpace(c.From)
	if from == "" {
		return fmt.Errorf("postal from address is not configured")
	}
	to = strings.TrimSpace(to)
	if to == "" || !strings.Contains(to, "@") {
		return fmt.Errorf("postal recipient is not an email address")
	}

	body, err := json.Marshal(sendRequest{
		To:        []string{to},
		From:      formatFrom(c.FromName, from),
		Sender:    from,
		Subject:   "Tellomi 验证码",
		PlainBody: fmt.Sprintf("您的验证码为：%s，请勿泄露于他人！", code),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL(base), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Server-API-Key", c.APIKey)

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
	if jerr := json.Unmarshal(raw, &parsed); jerr != nil {
		return fmt.Errorf("postal http %d: %s", resp.StatusCode, truncate(raw))
	}
	if resp.StatusCode >= 300 || !strings.EqualFold(parsed.Status, "success") {
		msg := parsed.Data.Message
		if msg == "" {
			msg = truncate(raw)
		}
		if parsed.Data.Code != "" {
			return fmt.Errorf("postal %s: %s", parsed.Data.Code, msg)
		}
		return fmt.Errorf("postal http %d: %s", resp.StatusCode, msg)
	}
	return nil
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

func sendURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, sendPath) {
		return base
	}
	return base + sendPath
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
