package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
)

// A missing id used to panic in FromPeer before the messageEmpty already
// sitting in the result could be encoded. Layer 232 then never saw
// messages.messages#1d73e7ea / messageEmpty#90a6ca84.
func TestGetMessagesAbsentDeletedAndMixedRoundTrip(t *testing.T) {
	const (
		caller  = int64(7)
		peer    = int64(8)
		liveID  = int32(10)
		goneID  = int32(11)
		neverID = int32(1999999999)
	)
	live := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		Id:      liveID,
		Message: "alive",
		PeerId:  mtproto.MakePeerUser(peer),
		FromId:  mtproto.MakePeerUser(caller),
	}).To_Message()
	// goneID is not in the store. That is what SelectByMessageIdList does
	// after deleted=1. Another user's row is not in this caller's map.
	boxes := &replyBoxes{byID: map[int32]*mtproto.MessageBox{
		liveID: {UserId: caller, MessageId: liveID, SenderUserId: caller, PeerType: mtproto.PEER_USER, PeerId: peer, Message: live},
	}}
	c := New(context.Background(), &svc.ServiceContext{Dao: &dao.Dao{MessageClient: boxes, UserClient: replyUsers{}}})
	c.MD = &metadata.RpcMetadata{UserId: caller}

	got, err := c.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{
			mtproto.MakeTLInputMessageID(&mtproto.InputMessage{Id: liveID}).To_InputMessage(),
			mtproto.MakeTLInputMessageID(&mtproto.InputMessage{Id: neverID}).To_InputMessage(),
			mtproto.MakeTLInputMessageID(&mtproto.InputMessage{Id: goneID}).To_InputMessage(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded := roundTripMessages(t, got, mtproto.CRC32_messages_messages_1d73e7ea)
	if len(decoded.GetMessages()) != 3 {
		t.Fatalf("messages %#v", decoded.GetMessages())
	}
	assertLive(t, decoded.GetMessages()[0], liveID, "alive")
	assertEmpty(t, decoded.GetMessages()[1], neverID)
	assertEmpty(t, decoded.GetMessages()[2], goneID)
	for _, m := range decoded.GetMessages() {
		if m.GetMessage() == "other-secret" {
			t.Fatal("another user's text was returned")
		}
	}

	onlyMissing, err := c.MessagesGetMessages(&mtproto.TLMessagesGetMessages{
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{
			mtproto.MakeTLInputMessageID(&mtproto.InputMessage{Id: neverID}).To_InputMessage(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	one := roundTripMessages(t, onlyMissing, mtproto.CRC32_messages_messages_1d73e7ea)
	if len(one.GetMessages()) != 1 {
		t.Fatalf("missing %#v", one.GetMessages())
	}
	assertEmpty(t, one.GetMessages()[0], neverID)

	wrapped := &mtproto.TLRpcResult{ReqMsgId: 42, Result: onlyMissing}
	buf := mtproto.NewEncodeBuf(256)
	if err = wrapped.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	obj := mtproto.NewDecodeBuf(buf.GetBuf()).Object()
	rpc, ok := obj.(*mtproto.TLRpcResult)
	if !ok {
		t.Fatalf("rpc result %#v", obj)
	}
	if _, bad := rpc.Result.(*mtproto.TLRpcError); bad {
		t.Fatalf("rpc error %#v", rpc.Result)
	}
}

func roundTripMessages(t *testing.T, in *mtproto.Messages_Messages, want mtproto.TLConstructor) *mtproto.Messages_Messages {
	t.Helper()
	buf := mtproto.NewEncodeBuf(1024)
	if err := in.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	out := &mtproto.Messages_Messages{}
	if err := out.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatalf("decode %v", err)
	}
	if out.Constructor != want {
		t.Fatalf("container ctor %#x want %#x", uint32(out.Constructor), uint32(want))
	}
	return out
}

func assertLive(t *testing.T, m *mtproto.Message, id int32, text string) {
	t.Helper()
	if m.GetPredicateName() != mtproto.Predicate_message || m.GetId() != id || m.GetMessage() != text {
		t.Fatalf("live %#v", m)
	}
	if m.Constructor != mtproto.CRC32_message_7600b9d3 {
		t.Fatalf("live ctor %#x", uint32(m.Constructor))
	}
}

func assertEmpty(t *testing.T, m *mtproto.Message, id int32) {
	t.Helper()
	if m.GetPredicateName() != mtproto.Predicate_messageEmpty || m.GetId() != id || m.GetMessage() != "" {
		t.Fatalf("empty %#v", m)
	}
	if m.Constructor != mtproto.CRC32_messageEmpty {
		t.Fatalf("empty ctor %#x", uint32(m.Constructor))
	}
	if m.GetPeerId() != nil {
		t.Fatalf("messages.getMessages has no peer to put on a missing id: %#v", m.GetPeerId())
	}
}
