package p1rest

import (
	"time"

	"github.com/teamgram/proto/mtproto"
)

// themeRec is the chat theme last set for one user and peer.
// The service message is messageActionSetChatTheme. A wired messenger
// assigns pts; the local updates box is what tests and a process without
// messenger.msg return to the caller.
type themeRec struct {
	kind     string // empty, emoji, gift
	emoticon string
	slug     string
}

func (s *store) setChatTheme(self int64, in *mtproto.TLMessagesSetChatTheme) (*mtproto.Updates, error) {
	ref, peer, err := peerOf(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	next, err := themeOf(in)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := dialogKey{user: self, kind: ref.kind, id: ref.id}
	if sameTheme(s.themes[k], next) {
		return nil, mtproto.ErrEmojiNotModified
	}
	s.putTheme(k, next)
	return serviceUpdates(self, peer, themeAction(next)), nil
}

// putTheme stores the theme for the caller and, in a private chat, for the
// other user. Clients learn the theme from messageActionSetChatTheme.
func (s *store) putTheme(self dialogKey, next themeRec) {
	write := func(k dialogKey) {
		if next.kind == "" || next.kind == "empty" {
			delete(s.themes, k)
			return
		}
		s.themes[k] = next
	}
	write(self)
	if self.kind == "user" && self.id != 0 && self.id != self.user {
		write(dialogKey{user: self.id, kind: "user", id: self.user})
	}
}

func themeAction(next themeRec) *mtproto.MessageAction {
	switch next.kind {
	case "gift":
		return mtproto.MakeMessageActionSetChatTheme(next.slug)
	case "empty", "":
		return mtproto.MakeMessageActionSetChatTheme("")
	default:
		return mtproto.MakeMessageActionSetChatTheme(next.emoticon)
	}
}

func serviceUpdates(self int64, peer *mtproto.Peer, action *mtproto.MessageAction) *mtproto.Updates {
	msg := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		FromId: mtproto.MakePeerUser(self),
		PeerId: peer,
		Date:   int32(time.Now().Unix()),
		Action: action,
	}).To_Message()
	upd := mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Message_MESSAGE: msg,
		PtsCount:        1,
	}).To_Update()
	return mtproto.MakeUpdatesByUpdates(upd)
}

func themeOf(in *mtproto.TLMessagesSetChatTheme) (themeRec, error) {
	if in == nil {
		return themeRec{}, mtproto.ErrEmojiInvalid
	}
	theme := in.GetTheme()
	if theme == nil {
		if in.GetEmoticon() == "" {
			return themeRec{}, mtproto.ErrEmojiInvalid
		}
		if !emojiOK(in.GetEmoticon()) {
			return themeRec{}, mtproto.ErrEmojiInvalid
		}
		return themeRec{kind: "emoji", emoticon: in.GetEmoticon()}, nil
	}
	switch theme.GetPredicateName() {
	case mtproto.Predicate_inputChatThemeEmpty:
		return themeRec{kind: "empty"}, nil
	case mtproto.Predicate_inputChatTheme:
		if !emojiOK(theme.GetEmoticon()) {
			return themeRec{}, mtproto.ErrEmojiInvalid
		}
		return themeRec{kind: "emoji", emoticon: theme.GetEmoticon()}, nil
	case mtproto.Predicate_inputChatThemeUniqueGift:
		slug := theme.GetSlug()
		if slug == "" || len(slug) > 128 {
			return themeRec{}, mtproto.ErrEmojiInvalid
		}
		return themeRec{kind: "gift", slug: slug}, nil
	default:
		return themeRec{}, mtproto.ErrEmojiInvalid
	}
}

func emojiOK(s string) bool {
	if s == "" || len(s) > 64 {
		return false
	}
	return true
}

func sameTheme(cur, next themeRec) bool {
	if cur.kind == "" {
		cur.kind = "empty"
	}
	if next.kind == "" {
		next.kind = "empty"
	}
	return cur == next
}
