package chatbox

import (
	"context"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

// Peer translation and messages.translateText.
// https://core.telegram.org/method/messages.translateText
// https://core.telegram.org/method/messages.togglePeerTranslations
//
// There is no external machine-translation provider. A valid to_lang returns
// messages.translateResult. Text that carries message entities is returned
// unchanged so entity offsets stay valid. Plain text is rewritten with a
// small built-in lexicon for en↔zh; every other supported language keeps
// the source wording. The optional tone is accepted and does not change
// the lexicon. togglePeerTranslations only stores the viewer's per-peer
// flag (userFull.translations_disabled / channelFull.translations_disabled).
// An explicit translateText call still runs when that flag is set.

type plainDoc struct {
	Text string   `json:"text"`
	Ents [][]byte `json:"ents,omitempty"`
	Seen bool     `json:"seen,omitempty"`
}

func plainKey(user int64, peer *mtproto.PeerUtil, msgID int32) string {
	return "plain:" + transKey(user, peer) + ":" + itoa(int(msgID))
}

func transKey(user int64, peer *mtproto.PeerUtil) string {
	if peer == nil {
		return userKey(user) + ":none"
	}
	if peer.PeerType == mtproto.PEER_SELF || (peer.PeerType == mtproto.PEER_USER && peer.PeerId == user) {
		return userKey(user) + ":self"
	}
	return userKey(user) + ":" + peerKey(peer.PeerType, peer.PeerId)
}

func offKey(user int64, peer *mtproto.PeerUtil) string {
	return "transoff:" + transKey(user, peer)
}

// TogglePeerTranslations stores whether this user has translations disabled
// for peer. https://core.telegram.org/method/messages.togglePeerTranslations
func (b *Book) TogglePeerTranslations(ctx context.Context, user int64, peer *mtproto.PeerUtil, disabled bool) error {
	if user == 0 || peer == nil || !peer.IsUserOrChatOrChannel() {
		return mtproto.ErrPeerIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := offKey(user, peer)
	if disabled {
		b.transOff[key] = true
	} else {
		delete(b.transOff, key)
	}
	return b.save(ctx, key, disabled)
}

// TranslationsDisabled is the flag returned on userFull and channelFull.
func (b *Book) TranslationsDisabled(user int64, peer *mtproto.PeerUtil) bool {
	if b == nil || user == 0 || peer == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := offKey(user, peer)
	if b.transOff[key] {
		return true
	}
	var disabled bool
	_ = b.load(context.Background(), key, &disabled)
	if disabled {
		b.transOff[key] = true
	}
	return disabled
}

// RememberPlain indexes message text so messages.translateText can resolve ids.
func (b *Book) RememberPlain(ctx context.Context, user int64, peer *mtproto.PeerUtil, msgID int32, text string, entities []*mtproto.MessageEntity) {
	if b == nil || user == 0 || peer == nil || msgID == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.rememberPlainLocked(ctx, user, peer, msgID, text, entities)
}

func (b *Book) rememberPlainLocked(ctx context.Context, user int64, peer *mtproto.PeerUtil, msgID int32, text string, entities []*mtproto.MessageEntity) {
	doc := &plainDoc{Text: text, Seen: true}
	for _, ent := range entities {
		if ent == nil {
			continue
		}
		raw, err := proto.Marshal(ent)
		if err != nil {
			continue
		}
		doc.Ents = append(doc.Ents, raw)
	}
	key := plainKey(user, peer, msgID)
	b.plain[key] = doc
	_ = b.save(ctx, key, doc)
}

// LookupPlain returns indexed text. ok is false when the id was never stored.
func (b *Book) LookupPlain(ctx context.Context, user int64, peer *mtproto.PeerUtil, msgID int32) (string, []*mtproto.MessageEntity, bool) {
	if b == nil || user == 0 || peer == nil || msgID <= 0 {
		return "", nil, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := plainKey(user, peer, msgID)
	doc := b.plain[key]
	if doc == nil {
		doc = &plainDoc{}
		if err := b.load(ctx, key, doc); err != nil {
			return "", nil, false
		}
		if !doc.Seen && doc.Text == "" && len(doc.Ents) == 0 {
			return "", nil, false
		}
		b.plain[key] = doc
	}
	var entities []*mtproto.MessageEntity
	for _, raw := range doc.Ents {
		ent := &mtproto.MessageEntity{}
		if err := proto.Unmarshal(raw, ent); err == nil {
			entities = append(entities, ent)
		}
	}
	return doc.Text, entities, true
}

// NoteSent indexes text and paid-media payloads from a successful send.
func (b *Book) NoteSent(ctx context.Context, user int64, peer *mtproto.PeerUtil, ups *mtproto.Updates) {
	if b == nil || ups == nil || peer == nil || user == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, u := range ups.GetUpdates() {
		msg := u.GetMessage_MESSAGE()
		if msg == nil || msg.GetId() == 0 {
			continue
		}
		b.rememberPlainLocked(ctx, user, peer, msg.GetId(), msg.GetMessage(), msg.GetEntities())
		b.notePaidLocked(ctx, user, peer, msg)
	}
}

// TranslateText builds messages.translateResult for one piece of text.
func TranslateText(text string, entities []*mtproto.MessageEntity, toLang string) (*mtproto.TextWithEntities, error) {
	if !ValidToLang(toLang) {
		return nil, mtproto.ErrToLangInvalid
	}
	out := text
	outEnt := entities
	if len(entities) == 0 {
		out = applyLexicon(text, toLang)
		outEnt = []*mtproto.MessageEntity{}
	}
	if outEnt == nil {
		outEnt = []*mtproto.MessageEntity{}
	}
	return mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
		Text:     out,
		Entities: outEnt,
	}).To_TextWithEntities(), nil
}

// ValidToLang reports whether to_lang is a language code this server accepts.
func ValidToLang(code string) bool {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" || len(code) > 8 {
		return false
	}
	_, ok := supportedLangs[code]
	return ok
}

func applyLexicon(text, toLang string) string {
	toLang = strings.ToLower(toLang)
	var table map[string]string
	switch {
	case strings.HasPrefix(toLang, "zh"):
		table = enToZH
	case toLang == "en" || strings.HasPrefix(toLang, "en-"):
		table = zhToEN
	default:
		return text
	}
	if len(table) == 0 || text == "" {
		return text
	}
	if repl, ok := table[strings.ToLower(strings.TrimSpace(text))]; ok {
		return repl
	}
	return replaceWords(text, table)
}

func replaceWords(text string, table map[string]string) string {
	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	lower := strings.ToLower(text)
	var b strings.Builder
	i := 0
	for i < len(text) {
		matched := false
		for _, key := range keys {
			if !strings.HasPrefix(lower[i:], key) {
				continue
			}
			next := i + len(key)
			if asciiKey(key) && next < len(text) {
				r, _ := utf8.DecodeRuneInString(text[next:])
				if unicode.IsLetter(r) || unicode.IsDigit(r) {
					continue
				}
			}
			b.WriteString(table[key])
			i = next
			matched = true
			break
		}
		if matched {
			continue
		}
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteByte(text[i])
			i++
			continue
		}
		// Keep scanning ASCII words that the prefix search already handled
		// one key at a time. A non-matching byte is copied as itself.
		b.WriteRune(r)
		i += size
	}
	return b.String()
}

var enToZH = map[string]string{
	"hello":     "你好",
	"hi":        "你好",
	"thanks":    "谢谢",
	"thank you": "谢谢",
	"yes":       "是",
	"no":        "不是",
	"ok":        "好的",
	"okay":      "好的",
	"goodbye":   "再见",
	"bye":       "再见",
	"please":    "请",
	"sorry":     "对不起",
}

var zhToEN = map[string]string{
	"你好":  "hello",
	"谢谢":  "thanks",
	"是":   "yes",
	"不是":  "no",
	"好的":  "ok",
	"再见":  "goodbye",
	"请":   "please",
	"对不起": "sorry",
}

// supportedLangs is the to_lang set messages.translateText accepts.
// Codes outside this set return TO_LANG_INVALID.
var supportedLangs = map[string]struct{}{
	"af": {}, "am": {}, "ar": {}, "az": {}, "be": {}, "bg": {}, "bn": {},
	"bs": {}, "ca": {}, "cs": {}, "cy": {}, "da": {}, "de": {}, "el": {},
	"en": {}, "en-us": {}, "en-gb": {}, "eo": {}, "es": {}, "et": {},
	"eu": {}, "fa": {}, "fi": {}, "fr": {}, "ga": {}, "gl": {}, "gu": {},
	"he": {}, "hi": {}, "hr": {}, "hu": {}, "hy": {}, "id": {}, "is": {},
	"it": {}, "ja": {}, "ka": {}, "kk": {}, "km": {}, "kn": {}, "ko": {},
	"ky": {}, "lo": {}, "lt": {}, "lv": {}, "mk": {}, "ml": {}, "mn": {},
	"mr": {}, "ms": {}, "my": {}, "nb": {}, "ne": {}, "nl": {}, "nn": {},
	"no": {}, "pa": {}, "pl": {}, "pt": {}, "pt-br": {}, "ro": {}, "ru": {},
	"si": {}, "sk": {}, "sl": {}, "sq": {}, "sr": {}, "sv": {}, "sw": {},
	"ta": {}, "te": {}, "th": {}, "tr": {}, "uk": {}, "ur": {}, "uz": {},
	"vi": {}, "zh": {}, "zh-cn": {}, "zh-tw": {}, "zh-hans": {}, "zh-hant": {},
}

func asciiKey(key string) bool {
	for i := 0; i < len(key); i++ {
		if key[i] >= 0x80 {
			return false
		}
	}
	return true
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
