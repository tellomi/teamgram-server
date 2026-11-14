package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestInboxMessagePeerPrivateChatIsTheSender(t *testing.T) {
	fromID, toID := int64(717), int64(727)
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: toID}
	got := inboxMessagePeer(fromID, peer)
	if got == nil || got.PeerType != mtproto.PEER_USER || got.PeerId != fromID {
		t.Fatalf("private inbox peer = %#v, want sender %d", got, fromID)
	}
	if peer.PeerId != toID {
		t.Fatal("inboxMessagePeer must not rewrite the input peer")
	}
}

func TestInboxMessagePeerGroupKeepsChatID(t *testing.T) {
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: 42}
	got := inboxMessagePeer(717, peer)
	if got != peer || got.PeerId != 42 || got.PeerType != mtproto.PEER_CHAT {
		t.Fatalf("group inbox peer = %#v", got)
	}
}

func TestInboxMessagePeerNil(t *testing.T) {
	if inboxMessagePeer(717, nil) != nil {
		t.Fatal("nil peer should stay nil")
	}
}

func TestReceiverKeepsACrossChatReply(t *testing.T) {
	header := &mtproto.MessageReplyHeader{
		ReplyToMsgId:       15,
		ReplyToMsgId_INT32: 15,
		ReplyToPeerId:      mtproto.MakePeerUser(9),
		Quote:              true,
		QuoteText:          &wrapperspb.StringValue{Value: "hello"},
	}
	if !crossChatReply(header) {
		t.Fatal("reply_to_peer_id should skip this-chat translation")
	}
	sameChat := &mtproto.MessageReplyHeader{ReplyToMsgId: 15, ReplyToMsgId_INT32: 15}
	if crossChatReply(sameChat) {
		t.Fatal("a reply in this chat still needs translation")
	}
	// A lookup of id 15 in the current chat would point at the wrong message.
	// The receiver leaves the other peer and the quote alone.
	if header.GetReplyToPeerId().GetUserId() != 9 || header.GetQuoteText().GetValue() != "hello" || header.GetReplyToMsgId() != 15 {
		t.Fatalf("header rewritten %+v", header)
	}
}

func TestInboxMessagePeerSavedMessagesKeepsSelf(t *testing.T) {
	self := int64(717)
	peer := &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: self}
	got := inboxMessagePeer(self, peer)
	if got == nil || got.PeerType != mtproto.PEER_USER || got.PeerId != self {
		t.Fatalf("saved-messages inbox peer = %#v, want self %d", got, self)
	}
}
