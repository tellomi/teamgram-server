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
	// Label is the mark report-review notices still append. Assistant replies
	// (private and group) do not: the account itself is already marked as AI.
	Label = "AI 生成"
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

// GroupSender delivers one reply into the group the question came from.
// replyTo is the id of the triggering message in that chat.
type GroupSender interface {
	SendGroup(ctx context.Context, peerType int32, peerID int64, replyTo int32, text string, ai bool) error
}

// GroupAsk is one group turn that already passed the mention or reply check.
// Text is the model prompt (mention stripped, plus the one replied-to message).
// ReplyTo is the id to set on the outgoing message. Zero, with PendingReply
// set, means a basic-group inbox id is not visible yet.
type GroupAsk struct {
	From     int64
	PeerType int32
	PeerID   int64
	ReplyTo  int32
	Text     string
	Random   int64
	// PendingReply, when set, runs after the turn is decided and before SendGroup,
	// and only when ReplyTo is still 0. A zero result sends the reply with no reply_to.
	PendingReply func(ctx context.Context) int32
	// NoText is a sticker, media without a caption, or a mention with nothing left to ask.
	// The model is not called.
	NoText bool
}

// Recorder persists replies. A nil recorder keeps them only on the ledger.
type Recorder interface {
	Save(ctx context.Context, rec Record) error
}

// Service is the @tellomi account. enabled is false until Start.
// A disabled service still answers with DisabledMessage when someone messages it.
type Service struct {
	enabled  bool
	Client   *Client
	Models   Completer
	Ledger   *Ledger
	Sender   Sender
	Group    GroupSender
	Recorder Recorder
	Allow    *Allow
	Now      func() time.Time
	notices  noticeLog
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

// Ready is an enabled service for an in-process caller. Start builds the same
// shape from the environment.
func Ready(client *Client, ledger *Ledger) *Service {
	if ledger == nil {
		ledger = NewLedger()
	}
	return &Service{enabled: true, Client: client, Ledger: ledger}
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

// Installed reports whether a service, including a disabled one, is in this process.
func Installed() bool {
	installedMu.RLock()
	defer installedMu.RUnlock()
	return active != nil
}

// ObservePrivate replies when a person messages @tellomi.
// random is messages.sendMessage random_id; the same pair is answered once.
// A switched-off service still sends DisabledMessage and does not call the model.
func ObservePrivate(ctx context.Context, from int64, text string, random int64) {
	installedMu.RLock()
	s := active
	installedMu.RUnlock()
	if s == nil || from == 0 || from == AccountID {
		return
	}
	if !s.enabled {
		s.emit(ctx, from, "", false, GroupAsk{}, Result{Text: Text(KindDisabled), Limited: KindDisabled})
		return
	}
	event := ""
	if random != 0 {
		event = "p:" + itoa(from) + ":" + itoa(random)
	}
	res, err := s.Reply(ctx, from, text, event)
	if err != nil {
		if errors.Is(err, ErrBusy) || errors.Is(err, ErrDisabled) {
			return
		}
		logx.WithContext(ctx).Errorf("tellomi ai: private reply failed user=%d", from)
		s.emit(ctx, from, "", false, GroupAsk{}, Result{Text: Text(KindBusy), Limited: KindBusy})
		return
	}
	s.emit(ctx, from, "", false, GroupAsk{}, res)
}

// ObserveGroup replies in the same basic group or supergroup. The caller has
// already decided this message mentions @tellomi or replies to 777001, and
// that 777001 can post there. PendingReply, when ReplyTo is still 0, runs
// after the turn is decided and before SendGroup. The model call does not
// wait on the inbox row. NoText is a sticker, other media without a caption,
// or a mention that left no text.
func ObserveGroup(ctx context.Context, ask GroupAsk) {
	installedMu.RLock()
	s := active
	installedMu.RUnlock()
	if s == nil || ask.From == 0 || ask.From == AccountID || ask.PeerID == 0 {
		return
	}
	chat := ChatKey(ask.PeerType, ask.PeerID)
	if !s.enabled {
		s.emit(ctx, ask.From, chat, true, ask, Result{Text: Text(KindDisabled), Limited: KindDisabled})
		return
	}
	event := ""
	if ask.Random != 0 {
		event = "g:" + itoa(ask.From) + ":" + itoa(int64(ask.PeerType)) + ":" + itoa(ask.PeerID) + ":" + itoa(ask.Random)
	}
	res, err := s.reply(ctx, ask.From, ask.Text, event, chat, ask.NoText)
	if err != nil {
		if errors.Is(err, ErrBusy) || errors.Is(err, ErrDisabled) {
			return
		}
		logx.WithContext(ctx).Errorf("tellomi ai: group reply failed user=%d peer=%d", ask.From, ask.PeerID)
		s.emit(ctx, ask.From, chat, true, ask, Result{Text: Text(KindBusy), Limited: KindBusy})
		return
	}
	s.emit(ctx, ask.From, chat, true, ask, res)
}

// Reply runs the allowlist, the quota checks, and the model. event may be empty.
// A content-safety block returns the blocked sentence and does not call the fallback model.
// A transport failure tries the fallback model once, still through Bailian.
// Fixed fallbacks are returned as a Result and do not call the model.
// A model or timeout failure is still an error so the engine can retry; chat delivery
// turns that error into BusyMessage without another model call.
func (s *Service) Reply(ctx context.Context, userID int64, text, event string) (Result, error) {
	return s.reply(ctx, userID, text, event, "", false)
}

// ReplyChat is Reply with the per-chat minute cap. An empty chat is a private turn.
func (s *Service) ReplyChat(ctx context.Context, userID int64, text, event, chat string) (Result, error) {
	return s.reply(ctx, userID, text, event, chat, false)
}

func (s *Service) reply(ctx context.Context, userID int64, text, event, chat string, noText bool) (Result, error) {
	if s == nil || !s.enabled {
		return Result{}, ErrDisabled
	}
	if userID == 0 || userID == AccountID {
		return Result{}, errors.New("tellomi ai: empty turn")
	}
	allowed, err := s.permitted(ctx, userID)
	if err != nil {
		return Result{Text: Text(KindBusy), Limited: KindBusy}, nil
	}
	if !allowed {
		return Result{Text: Text(KindAllow), Limited: KindAllow}, nil
	}
	text = clip(strings.TrimSpace(text), 4000)
	if noText || text == "" {
		return Result{Text: Text(KindInput), Limited: KindInput}, nil
	}
	if s.Ledger == nil {
		s.Ledger = NewLedger()
	}
	now := s.now()
	g := s.Ledger.BeginChat(userID, event, chat, now)
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
	primary, small, fallback := s.modelNames()
	model := primary
	if g.Small {
		model = small
	}
	content, tokens, err := s.complete(ctx, model, text)
	if errors.Is(err, ErrSafety) {
		res := Result{Text: Text(KindBlocked), Model: model, AIGenerated: true, Refused: true}
		s.keep(ctx, userID, event, text, res, 0, now)
		return res, nil
	}
	if err != nil && fallback != "" && fallback != model {
		content, tokens, err = s.complete(ctx, fallback, text)
		model = fallback
		if errors.Is(err, ErrSafety) {
			res := Result{Text: Text(KindBlocked), Model: model, AIGenerated: true, Refused: true}
			s.keep(ctx, userID, event, text, res, 0, now)
			return res, nil
		}
	}
	if err != nil {
		s.Ledger.Abort(event)
		return Result{}, err
	}
	content = clip(content, 3500)
	res := Result{Text: content, Model: model, AIGenerated: true}
	s.keep(ctx, userID, event, text, res, tokens, now)
	return res, nil
}

func (s *Service) permitted(ctx context.Context, userID int64) (bool, error) {
	if c := currentRealName(); c != nil {
		return c.Verified(ctx, userID)
	}
	if s == nil || s.Allow == nil {
		return true, nil
	}
	return s.Allow.Allowed(ctx, userID)
}

func (s *Service) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) modelNames() (primary, small, fallback string) {
	if s != nil && s.Client != nil {
		return s.Client.Primary, s.Client.Small, s.Client.Fallback
	}
	return defaultPrimary, defaultSmall, defaultFallback
}

func (s *Service) complete(ctx context.Context, model, prompt string) (string, int, error) {
	if s != nil && s.Models != nil {
		return s.Models.Complete(ctx, model, prompt)
	}
	if s == nil || s.Client == nil {
		return "", 0, errors.New("tellomi ai: client is not configured")
	}
	return s.Client.Complete(ctx, model, prompt)
}

func (s *Service) emit(ctx context.Context, from int64, chat string, group bool, ask GroupAsk, res Result) {
	if res.Repeated || res.Text == "" {
		return
	}
	if !s.notices.admit(res.Limited, from, chat, group, s.now()) {
		return
	}
	if group {
		if s.Group == nil {
			logx.WithContext(ctx).Errorf("tellomi ai: group sender is not configured")
			return
		}
		replyTo := ask.ReplyTo
		if replyTo == 0 && ask.PendingReply != nil {
			replyTo = ask.PendingReply(ctx)
		}
		if err := s.Group.SendGroup(ctx, ask.PeerType, ask.PeerID, replyTo, res.Text, res.AIGenerated); err != nil {
			logx.WithContext(ctx).Errorf("tellomi ai: group send failed user=%d peer=%d", from, ask.PeerID)
		}
		return
	}
	if s.Sender == nil {
		logx.WithContext(ctx).Errorf("tellomi ai: sender is not configured")
		return
	}
	if err := s.Sender.Send(ctx, from, res.Text, res.AIGenerated); err != nil {
		logx.WithContext(ctx).Errorf("tellomi ai: send failed user=%d", from)
	}
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
// Assistant chat replies do not call it. Report-review notices still do.
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

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return strings.TrimSpace(s)
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:n]))
}
