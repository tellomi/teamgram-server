package tellomiai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	// EnvEnabled turns the service account on. Unset or any other value is off.
	EnvEnabled = "TELLOMI_AI"
	// EnvAPIKey is the Bailian (DashScope) API key. It is not logged.
	EnvAPIKey = "TELLOMI_AI_API_KEY"
	// EnvBaseURL defaults to the China compatible-mode endpoint.
	EnvBaseURL = "TELLOMI_AI_BASE_URL"
	// EnvModelPrimary is the Qwen model. Empty means qwen-plus.
	EnvModelPrimary = "TELLOMI_AI_MODEL_PRIMARY"
	// EnvModelSmall is used after 80% of the monthly budget.
	EnvModelSmall = "TELLOMI_AI_MODEL_SMALL"
	// EnvModelFallback is DeepSeek, also called through Bailian.
	EnvModelFallback = "TELLOMI_AI_MODEL_FALLBACK"
	// EnvTimeoutSeconds is the per-call deadline. Empty means 12.
	EnvTimeoutSeconds = "TELLOMI_AI_TIMEOUT_SECONDS"

	defaultBaseURL  = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	defaultPrimary  = "qwen-plus"
	defaultSmall    = "qwen-turbo"
	defaultFallback = "deepseek-v3"
	defaultTimeout  = 12 * time.Second

	systemPrompt = "你是 Tellomi 的官方助手 @tellomi。只回复文字，不生成图片。回答简短。"
)

// Completer is the model call the assistant uses. *Client implements it.
// Fixed fallbacks must not call it. A test double should fail the test if they do.
type Completer interface {
	Complete(ctx context.Context, model, prompt string) (string, int, error)
}

// ErrSafety means Bailian content inspection rejected the prompt or the completion.
var ErrSafety = errors.New("tellomi ai: content safety")

// ErrTimeout means the model call did not finish in time.
var ErrTimeout = errors.New("tellomi ai: timeout")

// Client calls Bailian's OpenAI-compatible chat completions API.
// DeepSeek is a model name on that same API, not a second vendor.
type Client struct {
	BaseURL  string
	APIKey   string
	Primary  string
	Small    string
	Fallback string
	HTTP     *http.Client
}

// NewClient builds a client. An empty base URL uses the China endpoint.
func NewClient(baseURL, apiKey, primary, small, fallback string, timeout time.Duration) *Client {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultBaseURL
	}
	if primary == "" {
		primary = defaultPrimary
	}
	if small == "" {
		small = defaultSmall
	}
	if fallback == "" {
		fallback = defaultFallback
	}
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &Client{
		BaseURL:  strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		APIKey:   apiKey,
		Primary:  primary,
		Small:    small,
		Fallback: fallback,
		HTTP:     &http.Client{Timeout: timeout},
	}
}

// Complete returns the assistant text and total tokens.
func (c *Client) Complete(ctx context.Context, model, prompt string) (string, int, error) {
	return c.CompletePrompt(ctx, model, systemPrompt, prompt)
}

// CompletePrompt is Complete with a caller-supplied system message.
// Translation and compose use it so the chat-assistant prompt is not applied.
func (c *Client) CompletePrompt(ctx context.Context, model, system, prompt string) (string, int, error) {
	if c == nil || c.APIKey == "" || c.BaseURL == "" {
		return "", 0, errors.New("tellomi ai: client is not configured")
	}
	if model == "" {
		model = c.Primary
	}
	messages := []map[string]string{{"role": "user", "content": prompt}}
	if strings.TrimSpace(system) != "" {
		messages = []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": prompt},
		}
	}
	payload, err := json.Marshal(map[string]any{
		"model":    model,
		"messages": messages,
	})
	if err != nil {
		return "", 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		if isTimeout(err) {
			return "", 0, ErrTimeout
		}
		return "", 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", 0, err
	}
	if safetyBody(body) {
		return "", 0, ErrSafety
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, errors.New("tellomi ai: model http status")
	}
	text, tokens, err := parseCompletion(body)
	if err != nil {
		return "", 0, err
	}
	return text, tokens, nil
}

func parseCompletion(body []byte) (string, int, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Output struct {
			Text    string `json:"text"`
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		} `json:"output"`
		Usage struct {
			Total int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", 0, err
	}
	text := ""
	if len(parsed.Choices) > 0 {
		text = parsed.Choices[0].Message.Content
	}
	if text == "" && len(parsed.Output.Choices) > 0 {
		text = parsed.Output.Choices[0].Message.Content
	}
	if text == "" {
		text = parsed.Output.Text
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", 0, errors.New("tellomi ai: empty completion")
	}
	return text, parsed.Usage.Total, nil
}

func safetyBody(body []byte) bool {
	s := strings.ToLower(string(body))
	for _, needle := range []string{
		"data_inspection_failed",
		"datainspectionfailed",
		"response_inspection_failed",
		"responseinspectionfailed",
		"content_filter",
		"contentfilter",
		"content filter",
		"inappropriate content",
	} {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
