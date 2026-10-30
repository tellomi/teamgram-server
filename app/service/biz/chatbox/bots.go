package chatbox

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// In-chat bots: inline mode, callback buttons, /start, attach menu, web apps,
// and prepared inline messages.
//
// https://core.telegram.org/api/bots/inline
// https://core.telegram.org/api/bots/webapps
// https://core.telegram.org/api/bots/attach
//
// A bot is inline-capable when its profile has inline mode on (the bots table
// bot_inline_placeholder, or PutBot). getInlineBotResults then returns that
// bot's cached answer, or an empty messages.botResults when the bot has not
// answered this query. Bots that are not inline-capable return
// BOT_INLINE_DISABLED. Unknown users return BOT_INVALID.

const (
	inlineQueryTTL = 10 * time.Minute
	webSessionTTL  = 5 * time.Minute
	webSessionMax  = 24 * time.Hour
	preparedTTL    = 24 * time.Hour
	maxInlineHits  = 50
	maxResultID    = 64
	maxButtonData  = 64
	maxMessageLen  = 4096
)

var startParamRE = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// BotProfile is a bot the chat methods can address.
type BotProfile struct {
	ID          int64
	Name        string
	Username    string
	AccessHash  int64
	Inline      bool
	Placeholder string
	AttachMenu  bool
	MenuURL     string
	MainURL     string
	Description string
	Apps        []BotAppSpec
}

// BotAppSpec is one mini app on a bot.
type BotAppSpec struct {
	ID           int64
	AccessHash   int64
	ShortName    string
	Title        string
	Description  string
	URL          string
	Hash         int64
	Inactive     bool
	RequestWrite bool
	HasSettings  bool
}

type botApp struct {
	ID           int64  `json:"id"`
	AccessHash   int64  `json:"access_hash"`
	ShortName    string `json:"short_name"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	URL          string `json:"url"`
	Hash         int64  `json:"hash"`
	Inactive     bool   `json:"inactive"`
	RequestWrite bool   `json:"request_write"`
	HasSettings  bool   `json:"has_settings"`
}

type botProfile struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Username    string   `json:"username"`
	AccessHash  int64    `json:"access_hash"`
	Inline      bool     `json:"inline"`
	Placeholder string   `json:"placeholder"`
	Attach      bool     `json:"attach"`
	MenuURL     string   `json:"menu_url"`
	MainURL     string   `json:"main_url"`
	Description string   `json:"description"`
	Apps        []botApp `json:"apps"`
}

type cachedAnswer struct {
	Gallery    bool     `json:"gallery"`
	CacheTime  int32    `json:"cache_time"`
	Expires    int64    `json:"expires"`
	NextOffset string   `json:"next_offset"`
	SwitchText string   `json:"switch_text"`
	SwitchPM   string   `json:"switch_pm"`
	WebText    string   `json:"web_text"`
	WebURL     string   `json:"web_url"`
	Results    [][]byte `json:"results"`
}

type botState struct {
	Profile botProfile               `json:"profile"`
	Answers map[string]*cachedAnswer `json:"answers"`
}

type inlineQuery struct {
	ID         int64    `json:"id"`
	BotID      int64    `json:"bot_id"`
	UserID     int64    `json:"user_id"`
	PeerType   int32    `json:"peer_type"`
	PeerID     int64    `json:"peer_id"`
	Query      string   `json:"query"`
	Expires    int64    `json:"expires"`
	Gallery    bool     `json:"gallery"`
	CacheTime  int32    `json:"cache_time"`
	NextOffset string   `json:"next_offset"`
	SwitchText string   `json:"switch_text"`
	SwitchPM   string   `json:"switch_pm"`
	WebText    string   `json:"web_text"`
	WebURL     string   `json:"web_url"`
	Results    [][]byte `json:"results"`
}

type attachItem struct {
	ID    int64 `json:"id"`
	Write bool  `json:"write"`
}

type attachDoc struct {
	Bots []attachItem `json:"bots"`
}

type preparedDoc struct {
	ID        string   `json:"id"`
	BotID     int64    `json:"bot_id"`
	UserID    int64    `json:"user_id"`
	Expires   int64    `json:"expires"`
	Result    []byte   `json:"result"`
	PeerTypes [][]byte `json:"peer_types"`
}

type webSession struct {
	ID       int64  `json:"id"`
	BotID    int64  `json:"bot_id"`
	UserID   int64  `json:"user_id"`
	PeerType int32  `json:"peer_type"`
	PeerID   int64  `json:"peer_id"`
	Created  int64  `json:"created"`
	Expires  int64  `json:"expires"`
	URL      string `json:"url"`
}

type callbackAnswer struct {
	Alert     bool   `json:"alert"`
	Message   string `json:"message"`
	URL       string `json:"url"`
	CacheTime int32  `json:"cache_time"`
}

// SendReq is a message the BFF should hand to messenger.msg.
// Fallback is the schema-valid Updates used when the messenger client is unset.
type SendReq struct {
	Peer       *mtproto.PeerUtil
	RandomID   int64
	NoWebpage  bool
	Background bool
	Message    *mtproto.Message
	Fallback   *mtproto.Updates
}

func botKey(id int64) string { return "bot:" + userKey(id) }

func (p botProfile) displayName() string {
	if p.Name != "" {
		return p.Name
	}
	if p.Username != "" {
		return p.Username
	}
	return "bot"
}

func (p botProfile) accessHash() int64 {
	if p.AccessHash != 0 {
		return p.AccessHash
	}
	return p.ID
}

func (p botProfile) user(enabled bool) *mtproto.User {
	return mtproto.MakeTLUser(&mtproto.User{
		Id:                   p.ID,
		Bot:                  true,
		BotAttachMenu:        p.Attach,
		AttachMenuEnabled:    enabled,
		BotHasMainApp:        p.MainURL != "",
		AccessHash:           mtproto.MakeFlagsInt64(p.accessHash()),
		FirstName:            mtproto.MakeFlagsString(p.displayName()),
		Username:             mtproto.MakeFlagsString(p.Username),
		BotInlinePlaceholder: mtproto.MakeFlagsString(p.Placeholder),
	}).To_User()
}

func (p botProfile) hosts() map[string]struct{} {
	out := map[string]struct{}{}
	add := func(raw string) {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Host == "" {
			return
		}
		out[strings.ToLower(u.Hostname())] = struct{}{}
	}
	add(p.MenuURL)
	add(p.MainURL)
	for _, a := range p.Apps {
		add(a.URL)
	}
	return out
}

func (s sqlDocs) lookupBot(ctx context.Context, id int64) (botProfile, bool) {
	if s.db == nil || id <= 0 {
		return botProfile{}, false
	}
	var placeholder, menu, main, description string
	var attach, hasMain int
	err := s.db.QueryRowContext(ctx, `
SELECT IFNULL(bot_inline_placeholder,''), IFNULL(bot_attach_menu,0), IFNULL(menu_button_url,''),
       IFNULL(main_app_url,''), IFNULL(description,''), IFNULL(bot_has_main_app,0)
FROM bots WHERE bot_id=?`, id).Scan(&placeholder, &attach, &menu, &main, &description, &hasMain)
	if err != nil {
		return botProfile{}, false
	}
	p := botProfile{
		ID:          id,
		Inline:      placeholder != "",
		Placeholder: placeholder,
		Attach:      attach != 0,
		MenuURL:     menu,
		MainURL:     main,
		Description: description,
	}
	if hasMain != 0 && p.MainURL == "" {
		p.MainURL = menu
	}
	var name, username string
	var isBot int
	var access int64
	err = s.db.QueryRowContext(ctx, `
SELECT IFNULL(first_name,''), IFNULL(username,''), IFNULL(is_bot,0), IFNULL(access_hash,0)
FROM users WHERE id=?`, id).Scan(&name, &username, &isBot, &access)
	if err == nil {
		if isBot == 0 {
			return botProfile{}, false
		}
		p.Name = name
		p.Username = username
		p.AccessHash = access
	}
	return p, true
}

func (b *Book) lookupStoredBot(ctx context.Context, id int64) (botProfile, bool) {
	type finder interface {
		lookupBot(context.Context, int64) (botProfile, bool)
	}
	f, ok := b.db.(finder)
	if !ok || f == nil {
		return botProfile{}, false
	}
	return f.lookupBot(ctx, id)
}

func (b *Book) botLocked(ctx context.Context, id int64) *botState {
	if id <= 0 {
		return nil
	}
	if st := b.bots[id]; st != nil {
		return st
	}
	var st botState
	if err := b.load(ctx, botKey(id), &st); err == nil && st.Profile.ID == id {
		if st.Answers == nil {
			st.Answers = map[string]*cachedAnswer{}
		}
		b.bots[id] = &st
		return &st
	}
	p, ok := b.lookupStoredBot(ctx, id)
	if !ok {
		return nil
	}
	st = botState{Profile: p, Answers: map[string]*cachedAnswer{}}
	b.bots[id] = &st
	return b.bots[id]
}

func (b *Book) saveBot(ctx context.Context, st *botState) error {
	if st == nil {
		return nil
	}
	return b.save(ctx, botKey(st.Profile.ID), st)
}

// PutBot registers a bot profile. Tests and local configuration use it.
// A row in the bots table is loaded on demand when this was not called.
func (b *Book) PutBot(ctx context.Context, in BotProfile) error {
	if in.ID <= 0 {
		return mtproto.ErrBotInvalid
	}
	apps := make([]botApp, 0, len(in.Apps))
	seen := map[string]bool{}
	for _, a := range in.Apps {
		if a.ShortName == "" || a.ID == 0 {
			return mtproto.ErrBotAppInvalid
		}
		key := strings.ToLower(a.ShortName)
		if seen[key] {
			return mtproto.ErrBotAppInvalid
		}
		seen[key] = true
		if _, err := allowConfigured(a.URL); err != nil && a.URL != "" {
			return err
		}
		hash := a.Hash
		if hash == 0 {
			hash = hashText(a.ShortName + "\n" + a.URL)
		}
		apps = append(apps, botApp{
			ID: a.ID, AccessHash: a.AccessHash, ShortName: a.ShortName, Title: a.Title,
			Description: a.Description, URL: a.URL, Hash: hash, Inactive: a.Inactive,
			RequestWrite: a.RequestWrite, HasSettings: a.HasSettings,
		})
	}
	if in.MenuURL != "" {
		if _, err := allowConfigured(in.MenuURL); err != nil {
			return err
		}
	}
	if in.MainURL != "" {
		if _, err := allowConfigured(in.MainURL); err != nil {
			return err
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.botLocked(ctx, in.ID)
	if st == nil {
		st = &botState{Answers: map[string]*cachedAnswer{}}
		b.bots[in.ID] = st
	}
	if st.Answers == nil {
		st.Answers = map[string]*cachedAnswer{}
	}
	st.Profile = botProfile{
		ID: in.ID, Name: in.Name, Username: in.Username, AccessHash: in.AccessHash,
		Inline: in.Inline, Placeholder: in.Placeholder, Attach: in.AttachMenu,
		MenuURL: in.MenuURL, MainURL: in.MainURL, Description: in.Description, Apps: apps,
	}
	return b.saveBot(ctx, st)
}

func allowConfigured(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", mtproto.ErrUrlInvalid
	}
	return u.String(), nil
}

func allowURL(raw string, hosts map[string]struct{}) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return "", mtproto.ErrUrlInvalid
	}
	h := strings.ToLower(u.Hostname())
	if _, ok := hosts[h]; ok {
		return u.String(), nil
	}
	for host := range hosts {
		if strings.HasSuffix(h, "."+host) {
			return u.String(), nil
		}
	}
	return "", mtproto.ErrUrlInvalid
}

func checkStart(param string, allowEmpty bool) error {
	if param == "" {
		if allowEmpty {
			return nil
		}
		return mtproto.ErrStartParamEmpty
	}
	if !startParamRE.MatchString(param) {
		return mtproto.ErrStartParamInvalid
	}
	return nil
}

func utf16Len(s string) int { return len(utf16.Encode([]rune(s))) }

func (b *Book) freshID() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return b.now().UnixNano() & 0x7fffffffffffffff
	}
	v := int64(binary.BigEndian.Uint64(buf[:]) & 0x7fffffffffffffff)
	if v == 0 {
		return 1
	}
	return v
}

func hashText(s string) int64 {
	var h int64 = 1125899906842597
	for _, c := range s {
		h = h*31 + int64(c)
	}
	if h < 0 {
		h = -h
	}
	if h == 0 {
		h = 1
	}
	return h
}

func normalizePeer(self int64, p *mtproto.PeerUtil) *mtproto.PeerUtil {
	if p == nil {
		return nil
	}
	if p.IsSelf() {
		p.PeerType = mtproto.PEER_USER
		p.PeerId = self
	}
	return p
}

func resolvePeer(self int64, in *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	p := normalizePeer(self, mtproto.FromInputPeer2(self, in))
	if p == nil || !p.IsUserOrChatOrChannel() || p.PeerId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return p, nil
}

func resolveUser(self int64, in *mtproto.InputUser) (int64, error) {
	if in == nil {
		return 0, mtproto.ErrBotInvalid
	}
	p := mtproto.FromInputUser(self, in)
	if p == nil || p.PeerId <= 0 || (p.PeerType != mtproto.PEER_USER && p.PeerType != mtproto.PEER_SELF) {
		return 0, mtproto.ErrBotInvalid
	}
	return p.PeerId, nil
}

// PutBot is above. Inline answers are stored with SetInlineAnswer or
// AnswerInlineQuery (messages.setInlineBotResults).

// SetInlineAnswer caches the results messages.getInlineBotResults returns
// for this query text. cacheTime 0 keeps the answer until it is replaced.
// A positive cacheTime is how long a later fetch may reuse it. Query "*"
// matches any text that does not have its own answer.
func (b *Book) SetInlineAnswer(ctx context.Context, botID int64, query string, gallery bool, cacheTime int32, results []*mtproto.BotInlineResult) error {
	raw, err := packResults(results)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.botLocked(ctx, botID)
	if st == nil {
		return mtproto.ErrBotInvalid
	}
	if !st.Profile.Inline {
		return mtproto.ErrBotInlineDisabled
	}
	if st.Answers == nil {
		st.Answers = map[string]*cachedAnswer{}
	}
	exp := int64(0)
	if cacheTime > 0 {
		exp = b.now().Add(time.Duration(cacheTime) * time.Second).Unix()
	}
	st.Answers[query] = &cachedAnswer{
		Gallery: gallery, CacheTime: cacheTime, Expires: exp, Results: raw,
	}
	return b.saveBot(ctx, st)
}

func packResults(results []*mtproto.BotInlineResult) ([][]byte, error) {
	if len(results) > maxInlineHits {
		return nil, mtproto.ErrResultsTooMuch
	}
	seen := map[string]bool{}
	out := make([][]byte, 0, len(results))
	for _, r := range results {
		if r == nil || r.Id == "" {
			return nil, mtproto.ErrResultIdEmpty
		}
		if len(r.Id) > maxResultID || strings.ContainsAny(r.Id, "\x00") {
			return nil, mtproto.ErrResultIdInvalid
		}
		if seen[r.Id] {
			return nil, mtproto.ErrResultIdDuplicate
		}
		seen[r.Id] = true
		raw, err := proto.Marshal(r)
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
	}
	return out, nil
}

func unpackResults(raw [][]byte) ([]*mtproto.BotInlineResult, error) {
	out := make([]*mtproto.BotInlineResult, 0, len(raw))
	for _, b := range raw {
		r := &mtproto.BotInlineResult{}
		if err := proto.Unmarshal(b, r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// InlineResults implements messages.getInlineBotResults.
func (b *Book) InlineResults(ctx context.Context, self int64, bot *mtproto.InputUser, peer *mtproto.InputPeer, query, offset string) (*mtproto.Messages_BotResults, error) {
	if utf16Len(query) > 256 {
		return nil, mtproto.ErrMessageTooLong
	}
	botID, err := resolveUser(self, bot)
	if err != nil {
		return nil, err
	}
	p, err := resolvePeer(self, peer)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.botLocked(ctx, botID)
	if st == nil {
		return nil, mtproto.ErrBotInvalid
	}
	if !st.Profile.Inline {
		return nil, mtproto.ErrBotInlineDisabled
	}
	ans := b.pickAnswer(st, query)
	if offset != "" && (ans == nil || offset != ans.NextOffset) {
		ans = &cachedAnswer{}
	}
	q := b.storeQueryLocked(ctx, self, st.Profile.ID, p, query, ans)
	results, err := unpackResults(q.Results)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesBotResults(&mtproto.Messages_BotResults{
		Gallery:       q.Gallery,
		QueryId:       q.ID,
		NextOffset:    mtproto.MakeFlagsString(q.NextOffset),
		SwitchPm:      switchPM(q.SwitchText, q.SwitchPM),
		SwitchWebview: switchWeb(q.WebText, q.WebURL),
		Results:       results,
		CacheTime:     q.CacheTime,
		Users:         []*mtproto.User{st.Profile.user(false)},
	}).To_Messages_BotResults(), nil
}

func (b *Book) pickAnswer(st *botState, query string) *cachedAnswer {
	if st == nil || st.Answers == nil {
		return nil
	}
	now := b.now().Unix()
	if a := st.Answers[query]; a != nil && (a.Expires == 0 || a.Expires > now) {
		return a
	}
	if a := st.Answers["*"]; a != nil && (a.Expires == 0 || a.Expires > now) {
		return a
	}
	return nil
}

func (b *Book) storeQueryLocked(ctx context.Context, self, botID int64, peer *mtproto.PeerUtil, query string, ans *cachedAnswer) *inlineQuery {
	if ans == nil {
		ans = &cachedAnswer{}
	}
	q := &inlineQuery{
		ID: b.freshID(), BotID: botID, UserID: self,
		PeerType: peer.PeerType, PeerID: peer.PeerId, Query: query,
		Expires:    b.now().Add(inlineQueryTTL).Unix(),
		Gallery:    ans.Gallery,
		CacheTime:  ans.CacheTime,
		NextOffset: ans.NextOffset,
		SwitchText: ans.SwitchText,
		SwitchPM:   ans.SwitchPM,
		WebText:    ans.WebText,
		WebURL:     ans.WebURL,
		Results:    ans.Results,
	}
	b.queries[q.ID] = q
	_ = b.save(ctx, queryKey(q.ID), q)
	return q
}

func queryKey(id int64) string { return "iq:" + userKey(id) }

func (b *Book) queryLocked(ctx context.Context, id int64) *inlineQuery {
	if id == 0 {
		return nil
	}
	if q := b.queries[id]; q != nil {
		return q
	}
	var q inlineQuery
	if err := b.load(ctx, queryKey(id), &q); err != nil || q.ID == 0 {
		return nil
	}
	b.queries[id] = &q
	return &q
}

func switchPM(text, param string) *mtproto.InlineBotSwitchPM {
	if text == "" {
		return nil
	}
	return mtproto.MakeTLInlineBotSwitchPM(&mtproto.InlineBotSwitchPM{
		Text: text, StartParam: param,
	}).To_InlineBotSwitchPM()
}

func switchWeb(text, raw string) *mtproto.InlineBotWebView {
	if text == "" || raw == "" {
		return nil
	}
	return mtproto.MakeTLInlineBotWebView(&mtproto.InlineBotWebView{
		Text: text, Url: raw,
	}).To_InlineBotWebView()
}

// AnswerInlineQuery implements messages.setInlineBotResults.
// The caller must be the bot that owns the query. cache_time stores the
// answer for later identical queries; private skips that cache.
func (b *Book) AnswerInlineQuery(ctx context.Context, botID, queryID int64, gallery, private bool, cacheTime int32, next, switchText, switchParam, webText, webURL string, results []*mtproto.InputBotInlineResult) error {
	raw, err := convertInputResults(results)
	if err != nil {
		return err
	}
	if webURL != "" {
		if _, err = allowConfigured(webURL); err != nil {
			return err
		}
	}
	if err = checkStart(switchParam, switchText == ""); err != nil && switchText != "" {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	q := b.queryLocked(ctx, queryID)
	if q == nil || q.BotID != botID || q.Expires < b.now().Unix() {
		return mtproto.ErrQueryIdInvalid
	}
	q.Gallery = gallery
	q.CacheTime = cacheTime
	q.NextOffset = next
	q.SwitchText = switchText
	q.SwitchPM = switchParam
	q.WebText = webText
	q.WebURL = webURL
	q.Results = raw
	q.Expires = b.now().Add(inlineQueryTTL).Unix()
	b.queries[q.ID] = q
	if err = b.save(ctx, queryKey(q.ID), q); err != nil {
		return err
	}
	if private || cacheTime <= 0 {
		return nil
	}
	st := b.botLocked(ctx, botID)
	if st == nil {
		return nil
	}
	if st.Answers == nil {
		st.Answers = map[string]*cachedAnswer{}
	}
	st.Answers[q.Query] = &cachedAnswer{
		Gallery: gallery, CacheTime: cacheTime,
		Expires:    b.now().Add(time.Duration(cacheTime) * time.Second).Unix(),
		NextOffset: next, SwitchText: switchText, SwitchPM: switchParam,
		WebText: webText, WebURL: webURL, Results: raw,
	}
	return b.saveBot(ctx, st)
}

func convertInputResults(in []*mtproto.InputBotInlineResult) ([][]byte, error) {
	out := make([]*mtproto.BotInlineResult, 0, len(in))
	for _, r := range in {
		if r == nil || r.Id == "" {
			return nil, mtproto.ErrResultIdEmpty
		}
		send, err := convertInlineMessage(r.SendMessage)
		if err != nil {
			return nil, err
		}
		typ := r.Type
		if typ == "" {
			typ = "article"
		}
		out = append(out, mtproto.MakeTLBotInlineResult(&mtproto.BotInlineResult{
			Id:          r.Id,
			Type:        typ,
			Title:       r.Title,
			Description: r.Description,
			Url:         r.Url,
			SendMessage: send,
		}).To_BotInlineResult())
	}
	return packResults(out)
}

func convertInlineMessage(in *mtproto.InputBotInlineMessage) (*mtproto.BotInlineMessage, error) {
	if in == nil {
		return nil, mtproto.ErrMessageEmpty
	}
	switch in.PredicateName {
	case "", mtproto.Predicate_inputBotInlineMessageText:
		if utf16Len(in.Message) > maxMessageLen {
			return nil, mtproto.ErrMessageTooLong
		}
		return mtproto.MakeTLBotInlineMessageText(&mtproto.BotInlineMessage{
			NoWebpage:   in.NoWebpage,
			InvertMedia: in.InvertMedia,
			Message:     in.Message,
			Entities:    in.Entities,
			ReplyMarkup: in.ReplyMarkup,
		}).To_BotInlineMessage(), nil
	default:
		// Media, venue, contact, and invoice payloads are returned as text
		// when they carry a message. Other shapes are rejected rather than
		// sent as an empty bubble.
		if strings.TrimSpace(in.Message) == "" && strings.TrimSpace(in.Title) == "" {
			return nil, mtproto.ErrMessageEmpty
		}
		text := in.Message
		if text == "" {
			text = in.Title
		}
		return mtproto.MakeTLBotInlineMessageText(&mtproto.BotInlineMessage{
			NoWebpage:   in.NoWebpage,
			InvertMedia: in.InvertMedia,
			Message:     text,
			Entities:    in.Entities,
			ReplyMarkup: in.ReplyMarkup,
		}).To_BotInlineMessage(), nil
	}
}

// TakeInlineResult implements the lookup half of messages.sendInlineBotResult.
func (b *Book) TakeInlineResult(ctx context.Context, self int64, peer *mtproto.InputPeer, queryID int64, resultID string, random int64, hideVia, silent, background bool, schedule int32, reply *mtproto.InputReplyTo) (*SendReq, *mtproto.Updates, error) {
	if resultID == "" {
		return nil, nil, mtproto.ErrResultIdEmpty
	}
	p, err := resolvePeer(self, peer)
	if err != nil {
		return nil, nil, err
	}
	b.mu.Lock()
	q := b.queryLocked(ctx, queryID)
	if q == nil || q.UserID != self || q.Expires < b.now().Unix() {
		b.mu.Unlock()
		return nil, nil, mtproto.ErrInlineResultExpired
	}
	results, err := unpackResults(q.Results)
	if err != nil {
		b.mu.Unlock()
		return nil, nil, err
	}
	var hit *mtproto.BotInlineResult
	for _, r := range results {
		if r.GetId() == resultID {
			hit = r
			break
		}
	}
	botID := q.BotID
	var users []*mtproto.User
	if st := b.botLocked(ctx, botID); st != nil {
		users = []*mtproto.User{st.Profile.user(false)}
	}
	b.mu.Unlock()
	if hit == nil || hit.SendMessage == nil {
		return nil, nil, mtproto.ErrInlineResultExpired
	}
	text := hit.SendMessage.Message
	if strings.TrimSpace(text) == "" {
		return nil, nil, mtproto.ErrMessageEmpty
	}
	if utf16Len(text) > maxMessageLen {
		return nil, nil, mtproto.ErrMessageTooLong
	}
	msg := b.textMessage(self, p, text, silent)
	msg.Entities = hit.SendMessage.Entities
	msg.ReplyMarkup = hit.SendMessage.ReplyMarkup
	msg.InvertMedia = hit.SendMessage.InvertMedia
	if !hideVia {
		msg.ViaBotId = wrapperspb.Int64(botID)
	}
	msg.ReplyTo = replyHeader(reply)
	if schedule > 0 {
		ups, err := b.Schedule(ctx, self, p, schedule, []*mtproto.Message{msg})
		return nil, ups, err
	}
	if random == 0 {
		random = b.freshID()
	}
	return &SendReq{
		Peer: p, RandomID: random, NoWebpage: hit.SendMessage.NoWebpage,
		Background: background, Message: msg, Fallback: localUpdates(msg, users),
	}, nil, nil
}

func (b *Book) textMessage(self int64, peer *mtproto.PeerUtil, text string, silent bool) *mtproto.Message {
	return mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		Silent:  silent,
		Date:    int32(b.now().Unix()),
		Message: text,
		FromId:  mtproto.MakePeerUser(self),
		PeerId:  peer.ToPeer(),
	}).To_Message()
}

func localUpdates(msg *mtproto.Message, users []*mtproto.User) *mtproto.Updates {
	if msg.Id == 0 {
		msg.Id = 1
	}
	up := mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Message_MESSAGE: msg,
		Pts_INT32:       msg.Id,
		PtsCount:        1,
	}).To_Update()
	return mtproto.MakeUpdatesByUpdatesUsers(users, up)
}

func replyHeader(r *mtproto.InputReplyTo) *mtproto.MessageReplyHeader {
	if r == nil || r.GetPredicateName() != mtproto.Predicate_inputReplyToMessage || r.GetReplyToMsgId() == 0 {
		return nil
	}
	id := r.GetReplyToMsgId()
	return mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId:           id,
		ReplyToMsgId_INT32:     id,
		ReplyToMsgId_FLAGINT32: mtproto.MakeFlagsInt32(id),
	}).To_MessageReplyHeader()
}

// SeenRandom returns a previous successful send for this random_id.
func (b *Book) SeenRandom(ctx context.Context, user, random int64) (*mtproto.Updates, bool) {
	if user == 0 || random == 0 {
		return nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	raw := b.sent[randomKey(user, random)]
	if raw == nil {
		var stored []byte
		if err := b.load(ctx, randomKey(user, random), &stored); err != nil || len(stored) == 0 {
			return nil, false
		}
		raw = stored
	}
	ups := &mtproto.Updates{}
	if err := proto.Unmarshal(raw, ups); err != nil {
		return nil, false
	}
	return ups, true
}

// KeepRandom remembers a successful send so a client retry does not duplicate it.
func (b *Book) KeepRandom(ctx context.Context, user, random int64, ups *mtproto.Updates) {
	if user == 0 || random == 0 || ups == nil {
		return
	}
	raw, err := proto.Marshal(ups)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent[randomKey(user, random)] = raw
	_ = b.save(ctx, randomKey(user, random), raw)
}

func randomKey(user, random int64) string {
	return "rnd:" + userKey(user) + ":" + userKey(random)
}

// StartBot implements messages.startBot. The outgoing text is /start or
// /start <param>. start_param must match [A-Za-z0-9_-]{1,64}.
func (b *Book) StartBot(ctx context.Context, self int64, bot *mtproto.InputUser, peer *mtproto.InputPeer, random int64, startParam string) (*SendReq, error) {
	if err := checkStart(startParam, true); err != nil {
		return nil, err
	}
	botID, err := resolveUser(self, bot)
	if err != nil {
		return nil, err
	}
	p, err := resolvePeer(self, peer)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	st := b.botLocked(ctx, botID)
	b.mu.Unlock()
	if st == nil {
		return nil, mtproto.ErrBotInvalid
	}
	text := "/start"
	if startParam != "" {
		text = "/start " + startParam
	}
	if random == 0 {
		random = b.freshID()
	}
	msg := b.textMessage(self, p, text, false)
	return &SendReq{
		Peer: p, RandomID: random, Message: msg,
		Fallback: localUpdates(msg, []*mtproto.User{st.Profile.user(false)}),
	}, nil
}

// CallbackAnswer implements messages.getBotCallbackAnswer.
// A stored answer (SetCallbackAnswer or messages.setBotCallbackAnswer) is
// returned when one exists. Otherwise the result is an empty
// messages.botCallbackAnswer, which is what a bot sends when it calls
// answerCallbackQuery with no text. data longer than 64 bytes is
// BUTTON_DATA_INVALID. A password cannot be checked here and returns
// PASSWORD_HASH_INVALID.
func (b *Book) CallbackAnswer(ctx context.Context, self int64, peer *mtproto.InputPeer, msgID int32, data []byte, password bool) (*mtproto.Messages_BotCallbackAnswer, error) {
	p, err := resolvePeer(self, peer)
	if err != nil {
		return nil, err
	}
	if msgID <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if len(data) > maxButtonData {
		return nil, mtproto.ErrButtonDataInvalid
	}
	if password {
		return nil, mtproto.ErrPasswordHashInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	ans := b.callbacks[callbackKey(p.PeerType, p.PeerId, msgID, data)]
	if ans == nil {
		ans = &callbackAnswer{}
	}
	return mtproto.MakeTLMessagesBotCallbackAnswer(&mtproto.Messages_BotCallbackAnswer{
		Alert:     ans.Alert,
		HasUrl:    ans.URL != "",
		Message:   mtproto.MakeFlagsString(ans.Message),
		Url:       mtproto.MakeFlagsString(ans.URL),
		CacheTime: ans.CacheTime,
	}).To_Messages_BotCallbackAnswer(), nil
}

func callbackKey(peerType int32, peerID int64, msgID int32, data []byte) string {
	return "cb:" + peerKey(peerType, peerID) + ":" + userKey(int64(msgID)) + ":" + hex.EncodeToString(data)
}

// SetCallbackAnswer stores the alert a later getBotCallbackAnswer returns.
func (b *Book) SetCallbackAnswer(ctx context.Context, self int64, peer *mtproto.InputPeer, msgID int32, data []byte, alert bool, message, rawURL string, cacheTime int32) error {
	p, err := resolvePeer(self, peer)
	if err != nil {
		return err
	}
	if msgID <= 0 {
		return mtproto.ErrMessageIdInvalid
	}
	if len(data) > maxButtonData {
		return mtproto.ErrButtonDataInvalid
	}
	if rawURL != "" {
		if _, err = allowConfigured(rawURL); err != nil {
			return err
		}
	}
	if utf16Len(message) > 256 {
		return mtproto.ErrMessageTooLong
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.callbacks[callbackKey(p.PeerType, p.PeerId, msgID, data)] = &callbackAnswer{
		Alert: alert, Message: message, URL: rawURL, CacheTime: cacheTime,
	}
	return b.save(ctx, callbackKey(p.PeerType, p.PeerId, msgID, data), b.callbacks[callbackKey(p.PeerType, p.PeerId, msgID, data)])
}

// BindCallbackQuery records query_id -> peer/msg/data so a bot can answer
// with messages.setBotCallbackAnswer. The client method does not wait.
func (b *Book) NoteCallback(ctx context.Context, self int64, peer *mtproto.InputPeer, msgID int32, data []byte) (int64, error) {
	p, err := resolvePeer(self, peer)
	if err != nil {
		return 0, err
	}
	if msgID <= 0 {
		return 0, mtproto.ErrMessageIdInvalid
	}
	id := b.freshID()
	bind := &callbackBind{
		ID: id, PeerType: p.PeerType, PeerID: p.PeerId, MsgID: msgID, Data: append([]byte(nil), data...),
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cbBind[id] = bind
	_ = b.save(ctx, "cbq:"+userKey(id), bind)
	return id, nil
}

type callbackBind struct {
	ID       int64  `json:"id"`
	PeerType int32  `json:"peer_type"`
	PeerID   int64  `json:"peer_id"`
	MsgID    int32  `json:"msg_id"`
	Data     []byte `json:"data"`
}

// AnswerCallback implements messages.setBotCallbackAnswer.
func (b *Book) AnswerCallback(ctx context.Context, queryID int64, alert bool, message, rawURL string, cacheTime int32) error {
	if queryID == 0 {
		return mtproto.ErrQueryIdInvalid
	}
	if rawURL != "" {
		if _, err := allowConfigured(rawURL); err != nil {
			return err
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	bind := b.cbBind[queryID]
	if bind == nil {
		var loaded callbackBind
		if err := b.load(ctx, "cbq:"+userKey(queryID), &loaded); err != nil || loaded.ID == 0 {
			return mtproto.ErrQueryIdInvalid
		}
		bind = &loaded
		b.cbBind[queryID] = bind
	}
	b.callbacks[callbackKey(bind.PeerType, bind.PeerID, bind.MsgID, bind.Data)] = &callbackAnswer{
		Alert: alert, Message: message, URL: rawURL, CacheTime: cacheTime,
	}
	return b.save(ctx, callbackKey(bind.PeerType, bind.PeerID, bind.MsgID, bind.Data), b.callbacks[callbackKey(bind.PeerType, bind.PeerID, bind.MsgID, bind.Data)])
}

// RequestedPeers implements messages.sendBotRequestedPeer.
func (b *Book) RequestedPeers(ctx context.Context, self int64, peer *mtproto.InputPeer, msgID int32, buttonID int32, peers []*mtproto.InputPeer) (*SendReq, error) {
	p, err := resolvePeer(self, peer)
	if err != nil {
		return nil, err
	}
	if msgID < 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if len(peers) == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	out := make([]*mtproto.Peer, 0, len(peers))
	for _, in := range peers {
		rp, err := resolvePeer(self, in)
		if err != nil {
			return nil, err
		}
		out = append(out, rp.ToPeer())
	}
	msg := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		Date:   int32(b.now().Unix()),
		FromId: mtproto.MakePeerUser(self),
		PeerId: p.ToPeer(),
		Action: mtproto.MakeTLMessageActionRequestedPeer(&mtproto.MessageAction{
			ButtonId:         buttonID,
			Peers_VECTORPEER: out,
		}).To_MessageAction(),
	}).To_Message()
	return &SendReq{
		Peer: p, RandomID: b.freshID(), Message: msg, Fallback: localUpdates(msg, nil),
	}, nil
}

// WebViewData implements messages.sendWebViewData. The user sees a service
// message messageActionWebViewDataSent whose text is the button text.
func (b *Book) WebViewData(ctx context.Context, self int64, bot *mtproto.InputUser, random int64, button, data string) (*SendReq, error) {
	if strings.TrimSpace(button) == "" {
		return nil, mtproto.ErrMessageEmpty
	}
	if utf16Len(data) > maxMessageLen || utf16Len(button) > 256 {
		return nil, mtproto.ErrMessageTooLong
	}
	botID, err := resolveUser(self, bot)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	st := b.botLocked(ctx, botID)
	b.mu.Unlock()
	if st == nil {
		return nil, mtproto.ErrBotInvalid
	}
	peer := mtproto.MakePeerUtil(mtproto.PEER_USER, botID)
	if random == 0 {
		random = b.freshID()
	}
	msg := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		Date:   int32(b.now().Unix()),
		FromId: mtproto.MakePeerUser(self),
		PeerId: peer.ToPeer(),
		Action: mtproto.MakeTLMessageActionWebViewDataSent(&mtproto.MessageAction{
			Text: button,
			Data: data,
		}).To_MessageAction(),
	}).To_Message()
	return &SendReq{
		Peer: peer, RandomID: random, Message: msg,
		Fallback: localUpdates(msg, []*mtproto.User{st.Profile.user(false)}),
	}, nil
}

func attachKey(user int64) string { return "attach:" + userKey(user) }

func (b *Book) attachLocked(ctx context.Context, user int64) *attachDoc {
	if doc := b.attach[user]; doc != nil {
		return doc
	}
	var doc attachDoc
	if err := b.load(ctx, attachKey(user), &doc); err == nil && doc.Bots != nil {
		b.attach[user] = &doc
		return &doc
	}
	doc = attachDoc{Bots: []attachItem{}}
	b.attach[user] = &doc
	return &doc
}

func attachHash(items []attachItem) int64 {
	if len(items) == 0 {
		return 0
	}
	h := int64(0)
	for _, it := range items {
		h = h*16777619 + it.ID
		if it.Write {
			h++
		}
	}
	if h < 0 {
		h = -h
	}
	if h == 0 {
		h = 1
	}
	return h
}

func menuPeerTypes() []*mtproto.AttachMenuPeerType {
	return []*mtproto.AttachMenuPeerType{
		mtproto.MakeTLAttachMenuPeerTypePM(&mtproto.AttachMenuPeerType{}).To_AttachMenuPeerType(),
		mtproto.MakeTLAttachMenuPeerTypeChat(&mtproto.AttachMenuPeerType{}).To_AttachMenuPeerType(),
		mtproto.MakeTLAttachMenuPeerTypeBroadcast(&mtproto.AttachMenuPeerType{}).To_AttachMenuPeerType(),
	}
}

func (p botProfile) menuBot(inactive, write bool) *mtproto.AttachMenuBot {
	short := p.Username
	if short == "" {
		short = "bot"
	}
	return mtproto.MakeTLAttachMenuBot(&mtproto.AttachMenuBot{
		Inactive:           inactive,
		RequestWriteAccess: !write,
		ShowInAttachMenu:   true,
		BotId:              p.ID,
		ShortName:          short,
		PeerTypes:          menuPeerTypes(),
		Icons:              []*mtproto.AttachMenuBotIcon{},
	}).To_AttachMenuBot()
}

// AttachBots implements messages.getAttachMenuBots.
func (b *Book) AttachBots(ctx context.Context, self int64, hash int64) (*mtproto.AttachMenuBots, error) {
	if self <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc := b.attachLocked(ctx, self)
	sum := attachHash(doc.Bots)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLAttachMenuBotsNotModified(&mtproto.AttachMenuBots{}).To_AttachMenuBots(), nil
	}
	bots := make([]*mtproto.AttachMenuBot, 0, len(doc.Bots))
	users := make([]*mtproto.User, 0, len(doc.Bots))
	for _, it := range doc.Bots {
		st := b.botLocked(ctx, it.ID)
		if st == nil || !st.Profile.Attach {
			continue
		}
		bots = append(bots, st.Profile.menuBot(false, it.Write))
		users = append(users, st.Profile.user(true))
	}
	return mtproto.MakeTLAttachMenuBots(&mtproto.AttachMenuBots{
		Hash:  sum,
		Bots:  bots,
		Users: users,
	}).To_AttachMenuBots(), nil
}

// AttachBot implements messages.getAttachMenuBot.
func (b *Book) AttachBot(ctx context.Context, self int64, bot *mtproto.InputUser) (*mtproto.AttachMenuBotsBot, error) {
	botID, err := resolveUser(self, bot)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.botLocked(ctx, botID)
	if st == nil || !st.Profile.Attach {
		return nil, mtproto.ErrBotInvalid
	}
	write := false
	if self > 0 {
		for _, it := range b.attachLocked(ctx, self).Bots {
			if it.ID == botID {
				write = it.Write
				break
			}
		}
	}
	enabled := false
	if self > 0 {
		for _, it := range b.attach[self].Bots {
			if it.ID == botID {
				enabled = true
				break
			}
		}
	}
	return mtproto.MakeTLAttachMenuBotsBot(&mtproto.AttachMenuBotsBot{
		Bot:   st.Profile.menuBot(!enabled, write),
		Users: []*mtproto.User{st.Profile.user(enabled)},
	}).To_AttachMenuBotsBot(), nil
}

// ToggleAttach implements messages.toggleBotInAttachMenu.
func (b *Book) ToggleAttach(ctx context.Context, self int64, bot *mtproto.InputUser, enabled, write bool) (*mtproto.Bool, error) {
	if self <= 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	botID, err := resolveUser(self, bot)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	st := b.botLocked(ctx, botID)
	if st == nil || !st.Profile.Attach {
		return nil, mtproto.ErrBotInvalid
	}
	doc := b.attachLocked(ctx, self)
	next := doc.Bots[:0]
	for _, it := range doc.Bots {
		if it.ID != botID {
			next = append(next, it)
		}
	}
	if enabled {
		next = append(next, attachItem{ID: botID, Write: write})
	}
	doc.Bots = next
	b.attach[self] = doc
	if err := b.save(ctx, attachKey(self), doc); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// WebOpen is the shared input for messages.requestWebView and the other
// mini-app openers. Kind "app" puts the app short name in URL. Kind "join"
// puts the inline query id in BotID.
type WebOpen struct {
	Self       int64
	BotID      int64
	Peer       *mtproto.PeerUtil
	Fullscreen bool
	Compact    bool
	StartParam string
	URL        string
	Kind       string
}

// OpenWebView implements the messages.request*WebView family.
// kind is "menu", "simple", "main", "app", or "join".
func (b *Book) OpenWebView(ctx context.Context, in WebOpen) (*mtproto.WebViewResult, error) {
	if err := checkStart(in.StartParam, true); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var (
		raw   string
		botID = in.BotID
		hosts map[string]struct{}
		prof  botProfile
	)
	switch in.Kind {
	case "join":
		q := b.queryLocked(ctx, in.BotID)
		if q == nil || q.UserID != in.Self || q.Expires < b.now().Unix() {
			return nil, mtproto.ErrQueryIdInvalid
		}
		botID = q.BotID
		if in.Peer == nil {
			in.Peer = mtproto.MakePeerUtil(q.PeerType, q.PeerID)
		}
		st := b.botLocked(ctx, botID)
		if st == nil {
			return nil, mtproto.ErrBotInvalid
		}
		prof = st.Profile
		hosts = prof.hosts()
		raw = q.WebURL
		if raw == "" {
			raw = prof.MenuURL
		}
		if raw == "" {
			raw = prof.MainURL
		}
	default:
		st := b.botLocked(ctx, botID)
		if st == nil {
			return nil, mtproto.ErrBotInvalid
		}
		prof = st.Profile
		hosts = prof.hosts()
		switch in.Kind {
		case "main":
			if prof.MainURL == "" {
				return nil, mtproto.ErrBotAppInvalid
			}
			raw = prof.MainURL
		case "app":
			app := findApp(prof, in.URL, 0)
			if app == nil {
				return nil, mtproto.ErrBotAppInvalid
			}
			raw = app.URL
		default:
			if in.URL != "" {
				raw = in.URL
			} else if in.Kind == "simple" && prof.MainURL != "" {
				raw = prof.MainURL
			} else {
				raw = prof.MenuURL
				if raw == "" {
					raw = prof.MainURL
				}
			}
		}
	}
	if raw == "" {
		if in.Kind == "app" || in.Kind == "main" {
			return nil, mtproto.ErrBotAppInvalid
		}
		return nil, mtproto.ErrUrlInvalid
	}
	opened, err := allowURL(raw, hosts)
	if err != nil {
		return nil, err
	}
	opened = withParam(opened, in.StartParam)
	peerType, peerID := int32(mtproto.PEER_USER), botID
	if in.Peer != nil {
		peerType, peerID = in.Peer.PeerType, in.Peer.PeerId
	}
	now := b.now().Unix()
	sess := &webSession{
		ID: b.freshID(), BotID: botID, UserID: in.Self,
		PeerType: peerType, PeerID: peerID,
		Created: now, Expires: b.now().Add(webSessionTTL).Unix(), URL: opened,
	}
	b.webviews[sess.ID] = sess
	if err := b.save(ctx, webKey(sess.ID), sess); err != nil {
		return nil, err
	}
	return mtproto.MakeTLWebViewResultUrl(&mtproto.WebViewResult{
		Fullsize:          !in.Compact,
		Fullscreen:        in.Fullscreen,
		SameOrigin:        true,
		QueryId_FLAGINT64: wrapperspb.Int64(sess.ID),
		Url:               opened,
	}).To_WebViewResult(), nil
}

func withParam(raw, param string) string {
	if param == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	if q.Get("tgWebAppStartParam") == "" {
		q.Set("tgWebAppStartParam", param)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func webKey(id int64) string { return "wv:" + userKey(id) }

func (b *Book) webLocked(ctx context.Context, id int64) *webSession {
	if id == 0 {
		return nil
	}
	if s := b.webviews[id]; s != nil {
		return s
	}
	var s webSession
	if err := b.load(ctx, webKey(id), &s); err != nil || s.ID == 0 {
		return nil
	}
	b.webviews[id] = &s
	return &s
}

// ProlongWebView implements messages.prolongWebView.
func (b *Book) ProlongWebView(ctx context.Context, self, botID, queryID int64, peer *mtproto.PeerUtil) (*mtproto.Bool, error) {
	if queryID == 0 {
		return nil, mtproto.ErrQueryIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.webLocked(ctx, queryID)
	if s == nil || s.UserID != self || s.BotID != botID {
		return nil, mtproto.ErrQueryIdInvalid
	}
	if peer != nil && (s.PeerType != peer.PeerType || s.PeerID != peer.PeerId) {
		return nil, mtproto.ErrQueryIdInvalid
	}
	now := b.now()
	if now.Unix() > s.Created+int64(webSessionMax/time.Second) || now.Unix() > s.Expires {
		return nil, mtproto.ErrQueryIdInvalid
	}
	s.Expires = now.Add(webSessionTTL).Unix()
	b.webviews[s.ID] = s
	if err := b.save(ctx, webKey(s.ID), s); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func findApp(p botProfile, short string, id int64) *botApp {
	for i := range p.Apps {
		a := &p.Apps[i]
		if id != 0 && a.ID == id {
			return a
		}
		if short != "" && strings.EqualFold(a.ShortName, short) {
			return a
		}
	}
	return nil
}

// GetBotApp implements messages.getBotApp.
func (b *Book) GetBotApp(ctx context.Context, self int64, app *mtproto.InputBotApp, hash int64) (*mtproto.Messages_BotApp, error) {
	if app == nil {
		return nil, mtproto.ErrBotAppInvalid
	}
	var (
		botID int64
		id    int64
		short string
		err   error
	)
	switch app.PredicateName {
	case mtproto.Predicate_inputBotAppID:
		id = app.Id
		if id == 0 {
			return nil, mtproto.ErrBotAppInvalid
		}
	case mtproto.Predicate_inputBotAppShortName:
		botID, err = resolveUser(self, app.BotId)
		if err != nil {
			return nil, mtproto.ErrBotAppInvalid
		}
		short = app.ShortName
		if short == "" {
			return nil, mtproto.ErrBotAppInvalid
		}
	default:
		return nil, mtproto.ErrBotAppInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	var found *botApp
	var prof botProfile
	if botID != 0 {
		st := b.botLocked(ctx, botID)
		if st == nil {
			return nil, mtproto.ErrBotInvalid
		}
		prof = st.Profile
		found = findApp(prof, short, 0)
	} else {
		for _, st := range b.bots {
			if a := findApp(st.Profile, "", id); a != nil {
				found = a
				prof = st.Profile
				break
			}
		}
	}
	if found == nil {
		return nil, mtproto.ErrBotAppInvalid
	}
	if hash != 0 && hash == found.Hash {
		return mtproto.MakeTLMessagesBotApp(&mtproto.Messages_BotApp{
			App: mtproto.MakeTLBotAppNotModified(&mtproto.BotApp{}).To_BotApp(),
		}).To_Messages_BotApp(), nil
	}
	return mtproto.MakeTLMessagesBotApp(&mtproto.Messages_BotApp{
		Inactive:           found.Inactive,
		RequestWriteAccess: found.RequestWrite,
		HasSettings:        found.HasSettings,
		App: mtproto.MakeTLBotApp(&mtproto.BotApp{
			Id:          found.ID,
			AccessHash:  found.AccessHash,
			ShortName:   found.ShortName,
			Title:       found.Title,
			Description: found.Description,
			Photo:       mtproto.MakeTLPhotoEmpty(&mtproto.Photo{}).To_Photo(),
			Hash:        found.Hash,
		}).To_BotApp(),
	}).To_Messages_BotApp(), nil
}

// AppByRef resolves an inputBotApp to the bot id and app URL key used by OpenWebView.
func (b *Book) AppByRef(ctx context.Context, self int64, app *mtproto.InputBotApp) (botID int64, short string, err error) {
	if app == nil {
		return 0, "", mtproto.ErrBotAppInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	switch app.PredicateName {
	case mtproto.Predicate_inputBotAppShortName:
		botID, err = resolveUser(self, app.BotId)
		if err != nil {
			return 0, "", mtproto.ErrBotAppInvalid
		}
		st := b.botLocked(ctx, botID)
		if st == nil {
			return 0, "", mtproto.ErrBotInvalid
		}
		if findApp(st.Profile, app.ShortName, 0) == nil {
			return 0, "", mtproto.ErrBotAppInvalid
		}
		return botID, app.ShortName, nil
	case mtproto.Predicate_inputBotAppID:
		for _, st := range b.bots {
			if a := findApp(st.Profile, "", app.Id); a != nil {
				return st.Profile.ID, a.ShortName, nil
			}
		}
		return 0, "", mtproto.ErrBotAppInvalid
	default:
		return 0, "", mtproto.ErrBotAppInvalid
	}
}

func prepKey(id string) string { return "prep:" + id }

// SavePrepared implements messages.savePreparedInlineMessage.
// The caller is the bot. The returned id is what the user passes to
// messages.getPreparedInlineMessage.
func (b *Book) SavePrepared(ctx context.Context, botID, forUser int64, result *mtproto.InputBotInlineResult, peers []*mtproto.InlineQueryPeerType) (*mtproto.Messages_BotPreparedInlineMessage, error) {
	raw, err := convertInputResults([]*mtproto.InputBotInlineResult{result})
	if err != nil {
		return nil, err
	}
	if len(raw) != 1 {
		return nil, mtproto.ErrResultIdEmpty
	}
	peerRaw := make([][]byte, 0, len(peers))
	for _, p := range peers {
		if p == nil {
			continue
		}
		buf, err := proto.Marshal(p)
		if err != nil {
			return nil, err
		}
		peerRaw = append(peerRaw, buf)
	}
	id := hex.EncodeToString(binaryID(b.freshID()))
	exp := b.now().Add(preparedTTL).Unix()
	doc := &preparedDoc{
		ID: id, BotID: botID, UserID: forUser, Expires: exp, Result: raw[0], PeerTypes: peerRaw,
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.botLocked(ctx, botID) == nil {
		return nil, mtproto.ErrBotInvalid
	}
	b.prepared[id] = doc
	if err := b.save(ctx, prepKey(id), doc); err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesBotPreparedInlineMessage(&mtproto.Messages_BotPreparedInlineMessage{
		Id: id, ExpireDate: int32(exp),
	}).To_Messages_BotPreparedInlineMessage(), nil
}

func binaryID(v int64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(v))
	return buf
}

func (b *Book) preparedLocked(ctx context.Context, id string) *preparedDoc {
	if id == "" {
		return nil
	}
	if d := b.prepared[id]; d != nil {
		return d
	}
	var d preparedDoc
	if err := b.load(ctx, prepKey(id), &d); err != nil || d.ID == "" {
		return nil
	}
	b.prepared[id] = &d
	return &d
}

// PreparedInline implements messages.getPreparedInlineMessage.
func (b *Book) PreparedInline(ctx context.Context, self int64, bot *mtproto.InputUser, id string) (*mtproto.Messages_PreparedInlineMessage, error) {
	if id == "" {
		return nil, mtproto.ErrInlineResultExpired
	}
	botID, err := resolveUser(self, bot)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc := b.preparedLocked(ctx, id)
	if doc == nil || doc.BotID != botID || doc.Expires < b.now().Unix() {
		return nil, mtproto.ErrInlineResultExpired
	}
	if doc.UserID != 0 && doc.UserID != self {
		return nil, mtproto.ErrInlineResultExpired
	}
	st := b.botLocked(ctx, botID)
	if st == nil {
		return nil, mtproto.ErrBotInvalid
	}
	res := &mtproto.BotInlineResult{}
	if err := proto.Unmarshal(doc.Result, res); err != nil {
		return nil, err
	}
	types := make([]*mtproto.InlineQueryPeerType, 0, len(doc.PeerTypes))
	for _, raw := range doc.PeerTypes {
		t := &mtproto.InlineQueryPeerType{}
		if err := proto.Unmarshal(raw, t); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	q := b.storeQueryLocked(ctx, self, botID, mtproto.MakePeerUtil(mtproto.PEER_USER, botID), "", &cachedAnswer{
		Results: [][]byte{doc.Result},
	})
	_ = q
	return mtproto.MakeTLMessagesPreparedInlineMessage(&mtproto.Messages_PreparedInlineMessage{
		QueryId:   q.ID,
		Result:    res,
		PeerTypes: types,
		CacheTime: int32(preparedTTL / time.Second),
		Users:     []*mtproto.User{st.Profile.user(false)},
	}).To_Messages_PreparedInlineMessage(), nil
}
