package p1rest

import "github.com/teamgram/proto/mtproto"

// themeRec is the chat theme last set for one user and peer.
// The RPC result is updates with seq 0 and no pts-bearing update: a service
// message would need the user's pts, and a zero pts makes clients call
// getDifference and stall sync.
type themeRec struct {
	kind     string // empty, emoji, gift
	emoticon string
	slug     string
}

func (s *store) setChatTheme(self int64, in *mtproto.TLMessagesSetChatTheme) (*mtproto.Updates, error) {
	ref, _, err := peerOf(self, in.GetPeer())
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
	if next.kind == "empty" {
		delete(s.themes, k)
	} else {
		s.themes[k] = next
	}
	return mtproto.MakeEmptyUpdates(), nil
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
