package tellomiai

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	// AccountID is the official @tellomi user (migrate-20261105).
	AccountID int64 = 777001
	// Label is the visible mark on every model reply.
	Label = "AI 生成"
	// Refusal is the fixed sentence when Bailian content safety blocks a turn.
	Refusal = "这个问题我暂时不能回答"
	// DailyMessage is shown on the 101st question of the day.
	DailyMessage = "今天的提问次数已用完"
	// MinuteMessage is shown on the 6th question in a minute.
	MinuteMessage = "操作太频繁，请稍后再试"
	// BudgetMessage is shown when the monthly budget is spent.
	BudgetMessage = "本月的提问额度已用完"
)

// ErrDisabled means TELLOMI_AI is off or the API key is missing.
var ErrDisabled = errors.New("tellomi ai is disabled")

// ErrBusy means the same event id is already running.
var ErrBusy = errors.New("tellomi ai: in flight")

// Result is one reply. AIGenerated is false for quota messages.
type Result struct {
	Text        string `json:"text"`
	Model       string `json:"model,omitempty"`
	AIGenerated bool   `json:"ai_generated"`
	Refused     bool   `json:"refused"`
	Limited     string `json:"limited,omitempty"`
	// Repeated is true when event was already answered. Private chat does not send it again.
	Repeated bool `json:"-"`
}

// Sender delivers the reply into the private chat with user toUser.
type Sender interface {
	Send(ctx context.Context, toUser int64, text string, ai bool) error
}

// Recorder persists replies. A nil recorder keeps them only on the ledger.
type Recorder interface {
	Save(ctx context.Context, rec Record) error
}

// Service is the @tellomi account. enabled is false until Start.
type Service struct {
	enabled  bool
	Client   *Client
	Ledger   *Ledger
	Sender   Sender
	Recorder Recorder
	Now      func() time.Time
}

var (
	installedMu sync.RWMutex
	active      *Service
)

// Advertised is the help.getAppConfig flag. It is true only when the
// feature flag is 1 and an API key is set. The key itself is not published.
func Advertised() bool {
	return envOn(EnvEnabled) && strings.TrimSpace(getenv(EnvAPIKey)) != ""
}

// TranslateAdvertised is help.getAppConfig tellomi_translate_enabled.
// It is true only when the self-hosted translator answers /health.
// TELLOMI_AI_API_KEY does not turn it on.
func TranslateAdvertised() bool {
	if LocalReady() {
		return true
	}
	hookMu.RLock()
	hook := hookFn
	hookMu.RUnlock()
	return hook != nil && envOn(EnvTranslate)
}

// ComposeAdvertised is help.getAppConfig tellomi_ai_compose_enabled.
// It is true only when TELLOMI_AI_COMPOSE=1 and TELLOMI_AI_API_KEY is set.
// TELLOMI_AI is a separate switch for the @tellomi account.
func ComposeAdvertised() bool {
	return envOn(EnvCompose) && strings.TrimSpace(getenv(EnvAPIKey)) != ""
}

// Enabled reports whether private replies and the engine endpoint are live.
func Enabled() bool {
	installedMu.RLock()
	defer installedMu.RUnlock()
	return active != nil && active.enabled
}

// Use installs the process service. Tests pass nil to clear it.
func Use(s *Service) {
	installedMu.Lock()
	active = s
	installedMu.Unlock()
}

// ObservePrivate replies when a person messages @tellomi.
// It returns immediately when the feature is off, including when from is
// the service account itself. random is messages.sendMessage random_id;
// the same pair is answered once.
func ObservePrivate(ctx context.Context, from int64, text string, random int64) {
	installedMu.RLock()
	s := active
	installedMu.RUnlock()
	if s == nil || !s.enabled || from == 0 || from == AccountID {
		return
	}
	event := ""
	if random != 0 {
		event = "p:" + itoa(from) + ":" + itoa(random)
	}
	res, err := s.Reply(ctx, from, text, event)
	if err != nil {
		logx.WithContext(ctx).Errorf("tellomi ai: private reply failed user=%d", from)
		return
	}
	if res.Repeated || s.Sender == nil {
		if s.Sender == nil && !res.Repeated {
			logx.WithContext(ctx).Errorf("tellomi ai: sender is not configured")
		}
		return
	}
	if err = s.Sender.Send(ctx, from, res.Text, res.AIGenerated); err != nil {
		logx.WithContext(ctx).Errorf("tellomi ai: send failed user=%d", from)
	}
}

// Reply runs the quota checks and the model. event may be empty.
// A content-safety block returns Refusal and does not call the fallback model.
// A transport failure tries DeepSeek once, still through Bailian.
func (s *Service) Reply(ctx context.Context, userID int64, text, event string) (Result, error) {
	if s == nil || !s.enabled {
		return Result{}, ErrDisabled
	}
	text = clip(strings.TrimSpace(text), 4000)
	if text == "" || userID == 0 || userID == AccountID {
		return Result{}, errors.New("tellomi ai: empty turn")
	}
	if s.Client == nil {
		return Result{}, errors.New("tellomi ai: client is not configured")
	}
	if s.Ledger == nil {
		s.Ledger = NewLedger()
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}
	g := s.Ledger.Begin(userID, event, now)
	if g.Stored {
		g.Result.Repeated = true
		return g.Result, nil
	}
	if g.Busy {
		return Result{}, ErrBusy
	}
	if g.Limited != "" {
		res := Result{Text: limitText(g.Limited), Limited: g.Limited}
		s.keep(ctx, userID, event, text, res, 0, now)
		return res, nil
	}
	model := s.Client.Primary
	if g.Small {
		model = s.Client.Small
	}
	content, tokens, err := s.Client.Complete(ctx, model, text)
	if errors.Is(err, ErrSafety) {
		res := Result{Text: WithLabel(Refusal), Model: model, AIGenerated: true, Refused: true}
		s.keep(ctx, userID, event, text, res, 0, now)
		return res, nil
	}
	if err != nil && s.Client.Fallback != "" && s.Client.Fallback != model {
		content, tokens, err = s.Client.Complete(ctx, s.Client.Fallback, text)
		model = s.Client.Fallback
		if errors.Is(err, ErrSafety) {
			res := Result{Text: WithLabel(Refusal), Model: model, AIGenerated: true, Refused: true}
			s.keep(ctx, userID, event, text, res, 0, now)
			return res, nil
		}
	}
	if err != nil {
		s.Ledger.Abort(event)
		return Result{}, err
	}
	content = clip(content, 3500)
	res := Result{Text: WithLabel(content), Model: model, AIGenerated: true}
	s.keep(ctx, userID, event, text, res, tokens, now)
	return res, nil
}

func (s *Service) keep(ctx context.Context, userID int64, event, prompt string, res Result, tokens int, now time.Time) {
	s.Ledger.Finish(userID, event, res, tokens, now, prompt)
	if s.Recorder == nil {
		return
	}
	rec := Record{
		UserID: userID, EventID: event, Prompt: prompt, Reply: res.Text,
		Model: res.Model, Refused: res.Refused, Limited: res.Limited, At: now,
	}
	if err := s.Recorder.Save(ctx, rec); err != nil {
		logx.WithContext(ctx).Errorf("tellomi ai: reply log failed")
	}
}

// WithLabel appends the visible AI mark on its own line.
func WithLabel(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return Label
	}
	if strings.HasSuffix(text, Label) {
		return text
	}
	return text + "\n\n" + Label
}

func limitText(kind string) string {
	switch kind {
	case "minute":
		return MinuteMessage
	case "budget":
		return BudgetMessage
	default:
		return DailyMessage
	}
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return strings.TrimSpace(s)
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n]))
}
