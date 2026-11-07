package tellomiai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/teamgram/teamgram-server/pkg/contentsafety"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// EnvTranslate turns messages.translateText and messages.translateRichMessage on.
	// Unset or any other value is off.
	EnvTranslate = "TELLOMI_TRANSLATE"
	// EnvCompose turns messages.composeRichMessageWithAI on.
	// Unset or any other value is off. The app-config key tellomi_ai_compose_enabled
	// follows ComposeAdvertised, which also requires TELLOMI_AI_API_KEY.
	EnvCompose = "TELLOMI_AI_COMPOSE"

	// ModeTranslate asks the model to translate.
	ModeTranslate = "translate"
	// ModeCompose asks the model to proofread, emojify, restyle, or translate a draft.
	ModeCompose = "compose"

	// MaxRewriteRunes is the per-string cap. Longer text is INPUT_TEXT_TOO_LONG.
	MaxRewriteRunes = 16000

	rewriteSystem = "你是 Tellomi 的文本改写工具。只输出 JSON 字符串数组，不要输出其它文字，不要加「AI 生成」。数组长度和顺序必须与输入一致。"
	rewriteChunk  = 20
)

// ErrTooLong means one string is over MaxRewriteRunes.
var ErrTooLong = errors.New("tellomi ai: text too long")

// Job is one rewrite. Texts are rewritten independently and returned in order.
type Job struct {
	Mode      string
	Texts     []string
	ToLang    string
	FromLang  string
	Tone      string
	Proofread bool
	Emojify   bool
}

type hookFunc func(context.Context, Job) ([]string, error)

var (
	hookMu     sync.RWMutex
	hookFn     hookFunc
	moderateFn = contentsafety.Decide
)

// SetHook replaces the Bailian call. Tests use it. nil restores the real client.
func SetHook(fn func(context.Context, Job) ([]string, error)) {
	hookMu.Lock()
	hookFn = fn
	hookMu.Unlock()
}

// SetModerate replaces the output check. nil restores contentsafety.Decide.
// Production leaves this nil. A Blocked result fails the rewrite.
func SetModerate(fn func(context.Context, string) string) {
	hookMu.Lock()
	if fn == nil {
		moderateFn = contentsafety.Decide
	} else {
		moderateFn = fn
	}
	hookMu.Unlock()
}

// Rewrite translates with the self-hosted sidecar only. Compose that is not
// a translation still uses the configured assistant. A translation is never
// sent to Bailian, and a sidecar failure does not fall back to it.
func Rewrite(ctx context.Context, job Job) ([]string, error) {
	if isTranslate(job) {
		if !translateReady() {
			return nil, ErrDisabled
		}
	} else if !advertised(job.Mode) {
		return nil, ErrDisabled
	}
	if ctx == nil {
		ctx = context.Background()
	}
	for _, text := range job.Texts {
		if utf8.RuneCountInString(text) > MaxRewriteRunes {
			return nil, ErrTooLong
		}
	}
	if len(job.Texts) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(job.Texts))
	for start := 0; start < len(job.Texts); start += rewriteChunk {
		end := start + rewriteChunk
		if end > len(job.Texts) {
			end = len(job.Texts)
		}
		part, err := rewriteChunked(ctx, job, job.Texts[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	if err := moderate(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func advertised(mode string) bool {
	if mode == ModeCompose {
		return ComposeAdvertised()
	}
	return TranslateAdvertised()
}

func isTranslate(job Job) bool {
	return job.Mode == ModeTranslate || ChineseTarget(job.ToLang)
}

func translateReady() bool {
	hookMu.RLock()
	hook := hookFn
	hookMu.RUnlock()
	if hook != nil && envOn(EnvTranslate) {
		return true
	}
	return LocalReady()
}

func rewriteChunked(ctx context.Context, job Job, texts []string) ([]string, error) {
	if isTranslate(job) {
		hookMu.RLock()
		hook := hookFn
		hookMu.RUnlock()
		if hook != nil {
			chunk := job
			chunk.Texts = texts
			got, err := hook(ctx, chunk)
			if err != nil {
				return nil, err
			}
			if len(got) != len(texts) {
				return nil, errors.New("tellomi ai: rewrite shape")
			}
			return got, nil
		}
		return translateLocal(ctx, texts, job.ToLang)
	}
	hookMu.RLock()
	hook := hookFn
	hookMu.RUnlock()
	if hook != nil {
		chunk := job
		chunk.Texts = texts
		got, err := hook(ctx, chunk)
		if err != nil {
			return nil, err
		}
		if len(got) != len(texts) {
			return nil, errors.New("tellomi ai: rewrite shape")
		}
		return got, nil
	}
	client := NewClient(
		getenv(EnvBaseURL),
		getenv(EnvAPIKey),
		getenv(EnvModelPrimary),
		getenv(EnvModelSmall),
		getenv(EnvModelFallback),
		envDurationSeconds(EnvTimeoutSeconds, defaultTimeout),
	)
	prompt := userPrompt(job, texts)
	text, err := completeRewrite(ctx, client, client.Primary, prompt)
	if err != nil && !errors.Is(err, ErrSafety) && client.Fallback != "" && client.Fallback != client.Primary {
		text, err = completeRewrite(ctx, client, client.Fallback, prompt)
	}
	if err != nil {
		logx.WithContext(ctx).Errorf("tellomi ai: rewrite failed mode=%s", job.Mode)
		return nil, err
	}
	got, err := parseStringArray(text, len(texts))
	if err != nil {
		logx.WithContext(ctx).Errorf("tellomi ai: rewrite shape mode=%s", job.Mode)
		return nil, err
	}
	return got, nil
}

func completeRewrite(ctx context.Context, client *Client, model, prompt string) (string, error) {
	text, _, err := client.CompletePrompt(ctx, model, rewriteSystem, prompt)
	return text, err
}

func moderate(ctx context.Context, texts []string) error {
	hookMu.RLock()
	fn := moderateFn
	hookMu.RUnlock()
	if fn == nil {
		fn = contentsafety.Decide
	}
	for _, text := range texts {
		if strings.TrimSpace(text) == "" {
			continue
		}
		if fn(ctx, text) == contentsafety.Blocked {
			logx.WithContext(ctx).Errorf("tellomi ai: rewrite output blocked")
			return ErrSafety
		}
	}
	return nil
}

func userPrompt(job Job, texts []string) string {
	raw, err := json.Marshal(texts)
	if err != nil {
		raw = []byte("[]")
	}
	return instruction(job) + "\n" + string(raw)
}

func instruction(job Job) string {
	var b strings.Builder
	b.WriteString("Rewrite each input string. Return only a JSON array of strings with the same length and order. Do not add a label. Do not explain.")
	if job.Mode == ModeTranslate || strings.TrimSpace(job.ToLang) != "" {
		b.WriteString(" Translate each string into ")
		b.WriteString(oneLine(job.ToLang, 32))
		b.WriteString(".")
	}
	if from := oneLine(job.FromLang, 32); from != "" {
		b.WriteString(" Source language: ")
		b.WriteString(from)
		b.WriteString(".")
	}
	if job.Proofread {
		b.WriteString(" Proofread spelling and grammar. Keep the language unless a translation was also requested.")
	}
	if job.Emojify {
		b.WriteString(" Add a few relevant emoji.")
	}
	if tone := oneLine(job.Tone, 1024); tone != "" {
		b.WriteString(" Use this tone: ")
		b.WriteString(tone)
		b.WriteString(".")
	}
	b.WriteString(" Keep URLs, emoji, and code unchanged when they are the whole string.")
	return b.String()
}

func oneLine(s string, n int) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
	return clip(strings.TrimSpace(s), n)
}

func parseStringArray(raw string, n int) ([]string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```JSON")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	if i := strings.Index(raw, "["); i >= 0 {
		if j := strings.LastIndex(raw, "]"); j > i {
			raw = raw[i : j+1]
		}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil || len(out) != n {
		return nil, errors.New("tellomi ai: rewrite shape")
	}
	return out, nil
}
