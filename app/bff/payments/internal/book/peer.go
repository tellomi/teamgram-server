package book

import (
	"strings"
	"unicode"

	"github.com/teamgram/proto/mtproto"
)

type peerRef struct {
	kind string
	id   int64
}

func (p peerRef) key() string {
	return p.kind + ":" + itoa(p.id)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
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

func parsePeer(self int64, in *mtproto.InputPeer) (peerRef, *mtproto.Peer, error) {
	if in == nil {
		return peerRef{}, nil, fail(CodePeerInvalid)
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		if self == 0 {
			return peerRef{}, nil, fail(CodePeerInvalid)
		}
		return peerRef{kind: "user", id: self}, mtproto.MakePeerUser(self), nil
	case mtproto.Predicate_inputPeerUser:
		if in.GetUserId() == 0 {
			return peerRef{}, nil, fail(CodePeerInvalid)
		}
		return peerRef{kind: "user", id: in.GetUserId()}, mtproto.MakePeerUser(in.GetUserId()), nil
	case mtproto.Predicate_inputPeerChat:
		if in.GetChatId() == 0 {
			return peerRef{}, nil, fail(CodePeerInvalid)
		}
		return peerRef{kind: "chat", id: in.GetChatId()}, mtproto.MakePeerChat(in.GetChatId()), nil
	case mtproto.Predicate_inputPeerChannel:
		if in.GetChannelId() == 0 {
			return peerRef{}, nil, fail(CodePeerInvalid)
		}
		return peerRef{kind: "channel", id: in.GetChannelId()}, mtproto.MakePeerChannel(in.GetChannelId()), nil
	default:
		return peerRef{}, nil, fail(CodePeerInvalid)
	}
}

func parseUser(self int64, in *mtproto.InputUser) (int64, error) {
	if in == nil {
		return 0, fail(CodeUserInvalid)
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputUserSelf:
		if self == 0 {
			return 0, fail(CodeUserInvalid)
		}
		return self, nil
	case mtproto.Predicate_inputUser:
		if in.GetUserId() == 0 {
			return 0, fail(CodeUserInvalid)
		}
		return in.GetUserId(), nil
	default:
		return 0, fail(CodeUserInvalid)
	}
}

func requireID(raw, emptyCode string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxIDLen {
		return fail(emptyCode)
	}
	for _, r := range raw {
		if unicode.IsControl(r) {
			return fail(emptyCode)
		}
	}
	return nil
}

func pageLimitOf(limit int32) (int32, error) {
	if limit == 0 {
		return pageLimit, nil
	}
	if limit < 0 || limit > maxLimit {
		return 0, fail(CodeLimitInvalid)
	}
	return limit, nil
}
