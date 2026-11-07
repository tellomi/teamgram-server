package chatbox

import (
	"context"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

// Peer translation and messages.translateText.
// https://core.telegram.org/method/messages.translateText
// https://core.telegram.org/method/messages.togglePeerTranslations
//
// Translation calls the Bailian client in pkg/tellomiai when
// TELLOMI_TRANSLATE=1 and TELLOMI_AI_API_KEY are set. Otherwise, and when
// the model call fails, the result is TRANSLATE_REQ_FAILED. Entity offsets
// are UTF-16 and are moved with the translated spans. Code, URLs, mentions,
// and custom emoji stay as written. togglePeerTranslations only stores the
// viewer's per-peer flag. An explicit translateText call still runs when
// that flag is set.

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

// TranslateInput is one messages.translateText fragment.
type TranslateInput struct {
	Text     string
	Entities []*mtproto.MessageEntity
	ToLang   string
	FromLang string
	Tone     string
}

// TranslateText builds messages.translateResult for one piece of text.
// https://core.telegram.org/method/messages.translateText
func TranslateText(ctx context.Context, in TranslateInput) (*mtproto.TextWithEntities, error) {
	if !ValidToLang(in.ToLang) {
		return nil, mtproto.ErrToLangInvalid
	}
	return rewritePlain(ctx, in.Text, in.Entities, translateJob(in))
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

// supportedLangs is English to simplified Chinese only.
// Any other to_lang is TO_LANG_INVALID. There is no third-party fallback.
var supportedLangs = map[string]struct{}{
	"zh": {}, "zh-cn": {}, "zh-hans": {},
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
