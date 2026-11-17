package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

func TestChannelsGetMessagesAbsentDeletedAndOutsider(t *testing.T) {
	svcCh := channel.New(channel.NewMemoryStore())
	channel.Use(svcCh)
	t.Cleanup(func() { channel.Use(channel.New(channel.NewMemoryStore())) })
	ctx := context.Background()
	created, err := svcCh.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Quiet", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  ch.GetId(),
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
	sent, err := svcCh.Send(ctx, 1, ch.GetId(), 0, []channel.Outgoing{{
		RandomID: 1,
		Message:  &mtproto.Message{Message: "channel-secret", Date: 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	liveID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if liveID == 0 {
		t.Fatal("no channel message id")
	}
	const neverID int32 = 1999999999
	member := outsiderCtx(t, 1)
	handler := &Service{svcCtx: &svc.ServiceContext{}}
	page, err := handler.ChannelsGetMessages(member, idRequest(in, liveID, neverID))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetMessages()) != 2 {
		t.Fatalf("page %#v", page.GetMessages())
	}
	if page.GetMessages()[0].GetMessage() != "channel-secret" || page.GetMessages()[0].GetId() != liveID {
		t.Fatalf("live %#v", page.GetMessages()[0])
	}
	assertChannelEmpty(t, roundTripMessage(t, page.GetMessages()[1]), neverID, ch.GetId())
	encoded := roundTripChannelMessages(t, page)
	if len(encoded.GetMessages()) != 2 {
		t.Fatalf("encoded page %#v", encoded.GetMessages())
	}
	if encoded.GetMessages()[0].GetPredicateName() != mtproto.Predicate_message ||
		encoded.GetMessages()[0].Constructor != mtproto.CRC32_message_7600b9d3 ||
		encoded.GetMessages()[0].GetId() != liveID ||
		encoded.GetMessages()[0].GetMessage() != "channel-secret" ||
		encoded.GetMessages()[0].GetPeerId().GetChannelId() != ch.GetId() {
		t.Fatalf("encoded live %#v", encoded.GetMessages()[0])
	}
	assertChannelEmpty(t, encoded.GetMessages()[1], neverID, ch.GetId())

	if _, _, err = svcCh.Delete(ctx, 1, ch.GetId(), 0, []int32{liveID}); err != nil {
		t.Fatal(err)
	}
	after, err := handler.ChannelsGetMessages(member, idRequest(in, liveID))
	if err != nil {
		t.Fatal(err)
	}
	if len(after.GetMessages()) != 1 || after.GetMessages()[0].GetMessage() == "channel-secret" {
		t.Fatalf("deleted page %#v", after.GetMessages())
	}
	assertChannelEmpty(t, roundTripMessage(t, after.GetMessages()[0]), liveID, ch.GetId())

	outsider := outsiderCtx(t, 99)
	for _, id := range []int32{liveID, neverID} {
		body, gotErr := handler.ChannelsGetMessages(outsider, idRequest(in, id))
		if body != nil {
			t.Fatalf("outsider id %d body %#v", id, body)
		}
		if gotErr != mtproto.ErrChannelPrivate {
			t.Fatalf("outsider id %d err %v", id, gotErr)
		}
	}
}

func idRequest(in *mtproto.InputChannel, ids ...int32) *mtproto.TLChannelsGetMessages {
	var list []*mtproto.InputMessage
	for _, id := range ids {
		list = append(list, mtproto.MakeTLInputMessageID(&mtproto.InputMessage{Id: id}).To_InputMessage())
	}
	return &mtproto.TLChannelsGetMessages{Channel: in, Id_VECTORINPUTMESSAGE: list}
}

func roundTripChannelMessages(t *testing.T, in *mtproto.Messages_Messages) *mtproto.Messages_Messages {
	t.Helper()
	buf := mtproto.NewEncodeBuf(1024)
	if err := in.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	out := &mtproto.Messages_Messages{}
	if err := out.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatalf("decode %v", err)
	}
	want := mtproto.TLConstructor(mtproto.GetClazzID(mtproto.Predicate_messages_channelMessages, 232))
	if out.Constructor != want {
		t.Fatalf("container ctor %#x want %#x", uint32(out.Constructor), uint32(want))
	}
	return out
}

func roundTripMessage(t *testing.T, in *mtproto.Message) *mtproto.Message {
	t.Helper()
	buf := mtproto.NewEncodeBuf(256)
	if err := in.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	out := &mtproto.Message{}
	if err := out.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	return out
}

func assertChannelEmpty(t *testing.T, m *mtproto.Message, id int32, channelID int64) {
	t.Helper()
	if m.GetPredicateName() != mtproto.Predicate_messageEmpty || m.GetId() != id || m.GetMessage() != "" {
		t.Fatalf("empty %#v", m)
	}
	if m.Constructor != mtproto.CRC32_messageEmpty {
		t.Fatalf("empty ctor %#x", uint32(m.Constructor))
	}
	if m.GetPeerId().GetChannelId() != channelID {
		t.Fatalf("peer %#v", m.GetPeerId())
	}
}
