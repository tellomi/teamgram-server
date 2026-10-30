package p1rest

import "github.com/teamgram/proto/mtproto"

type peerRef struct {
	kind string
	id   int64
}

func peerOf(self int64, in *mtproto.InputPeer) (peerRef, *mtproto.Peer, error) {
	if in == nil {
		return peerRef{}, nil, mtproto.ErrPeerIdInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		if self == 0 {
			return peerRef{}, nil, mtproto.ErrPeerIdInvalid
		}
		return peerRef{kind: "user", id: self}, mtproto.MakePeerUser(self), nil
	case mtproto.Predicate_inputPeerUser:
		if in.GetUserId() == 0 {
			return peerRef{}, nil, mtproto.ErrPeerIdInvalid
		}
		return peerRef{kind: "user", id: in.GetUserId()}, mtproto.MakePeerUser(in.GetUserId()), nil
	case mtproto.Predicate_inputPeerChat:
		if in.GetChatId() == 0 {
			return peerRef{}, nil, mtproto.ErrPeerIdInvalid
		}
		return peerRef{kind: "chat", id: in.GetChatId()}, mtproto.MakePeerChat(in.GetChatId()), nil
	case mtproto.Predicate_inputPeerChannel:
		if in.GetChannelId() == 0 {
			return peerRef{}, nil, mtproto.ErrPeerIdInvalid
		}
		return peerRef{kind: "channel", id: in.GetChannelId()}, mtproto.MakePeerChannel(in.GetChannelId()), nil
	default:
		return peerRef{}, nil, mtproto.ErrPeerIdInvalid
	}
}

func channelOnly(self int64, in *mtproto.InputPeer) (peerRef, error) {
	ref, _, err := peerOf(self, in)
	if err != nil {
		return peerRef{}, err
	}
	if ref.kind != "channel" {
		return peerRef{}, mtproto.ErrPeerIdInvalid
	}
	return ref, nil
}
