package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

// A channel post is stored as protobuf. The send used here does not set the
// TL predicate. Before the read-side repair, layer 232 encode turned that
// post into messageEmpty and dropped the text.
func TestGetMessagesLayer232KeepsStoredPost(t *testing.T) {
	runLayer232StoredPost(t, New(NewMemoryStore()))
}

func runLayer232StoredPost(t *testing.T, svc *Service) {
	t.Helper()
	ctx := context.Background()
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Quiet", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.GetId(), ch.GetAccessHash_FLAGINT64().GetValue())
	first, err := svc.Send(ctx, 1, ch.GetId(), 0, []Outgoing{{
		RandomID: 1,
		Message:  &mtproto.Message{Message: "channel-secret", Date: 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.Send(ctx, 1, ch.GetId(), 0, []Outgoing{{
		RandomID: 2,
		Message:  &mtproto.Message{Message: "second-post", Date: 11},
	}})
	if err != nil {
		t.Fatal(err)
	}
	liveID := first.Updates[0].GetMessage_MESSAGE().GetId()
	otherID := second.Updates[0].GetMessage_MESSAGE().GetId()
	if liveID == 0 || otherID == 0 || liveID == otherID {
		t.Fatalf("ids %d %d", liveID, otherID)
	}
	stored, err := svc.store.GetMessage(ctx, ch.GetId(), liveID)
	if err != nil || stored == nil {
		t.Fatal(err)
	}
	var bare mtproto.Message
	if err = proto.Unmarshal(stored.Data, &bare); err != nil {
		t.Fatal(err)
	}
	if bare.PredicateName != "" || bare.Constructor != 0 || bare.GetMessage() != "channel-secret" {
		t.Fatalf("stored blob ctor=%#x pred=%q text=%q", uint32(bare.Constructor), bare.PredicateName, bare.GetMessage())
	}

	const neverID int32 = 1999999999
	page, err := svc.GetMessages(ctx, 1, in, []int32{otherID, neverID, liveID})
	if err != nil {
		t.Fatal(err)
	}
	decoded := decodeChannelPage(t, page)
	if len(decoded.GetMessages()) != 3 {
		t.Fatalf("len %d", len(decoded.GetMessages()))
	}
	assertLivePost(t, decoded.GetMessages()[0], otherID, "second-post", ch.GetId())
	assertEmptyPost(t, decoded.GetMessages()[1], neverID, ch.GetId())
	assertLivePost(t, decoded.GetMessages()[2], liveID, "channel-secret", ch.GetId())

	hist, err := svc.History(ctx, 1, in, HistoryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, m := range decodeChannelPage(t, hist).GetMessages() {
		if m.GetId() != liveID {
			continue
		}
		assertLivePost(t, m, liveID, "channel-secret", ch.GetId())
		found = true
	}
	if !found {
		t.Fatal("history page lost the stored post")
	}

	if _, _, err = svc.Delete(ctx, 1, ch.GetId(), 0, []int32{liveID}); err != nil {
		t.Fatal(err)
	}
	after, err := svc.GetMessages(ctx, 1, in, []int32{liveID, otherID})
	if err != nil {
		t.Fatal(err)
	}
	deleted := decodeChannelPage(t, after)
	if len(deleted.GetMessages()) != 2 {
		t.Fatalf("deleted len %d", len(deleted.GetMessages()))
	}
	assertEmptyPost(t, deleted.GetMessages()[0], liveID, ch.GetId())
	assertLivePost(t, deleted.GetMessages()[1], otherID, "second-post", ch.GetId())
	if deleted.GetMessages()[0].GetMessage() == "channel-secret" {
		t.Fatal("deleted text leaked")
	}

	if _, err = svc.GetMessages(ctx, 99, in, []int32{otherID}); err != mtproto.ErrChannelPrivate {
		t.Fatalf("outsider stored %v", err)
	}
	if _, err = svc.GetMessages(ctx, 99, in, []int32{neverID}); err != mtproto.ErrChannelPrivate {
		t.Fatalf("outsider missing %v", err)
	}
}

func TestUnmarshalFillsServicePredicateFromAction(t *testing.T) {
	rawMsg := serviceMessage(1, 50, mtproto.MakeMessageActionChannelCreate("Quiet"))
	rawMsg.PredicateName = ""
	rawMsg.Constructor = 0
	raw, err := proto.Marshal(rawMsg)
	if err != nil {
		t.Fatal(err)
	}
	got, err := unmarshalMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_messageService || got.GetAction() == nil {
		t.Fatalf("after unmarshal pred=%q action=%#v", got.GetPredicateName(), got.GetAction())
	}
	buf := mtproto.NewEncodeBuf(256)
	if err = got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	back := &mtproto.Message{}
	if err = back.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	want := mtproto.TLConstructor(mtproto.GetClazzID(mtproto.Predicate_messageService, 232))
	if back.GetPredicateName() != mtproto.Predicate_messageService || back.Constructor != want || back.GetAction() == nil || back.GetMessage() != "" {
		t.Fatalf("service roundtrip pred=%q ctor=%#x action=%v", back.GetPredicateName(), uint32(back.Constructor), back.GetAction())
	}
}

func decodeChannelPage(t *testing.T, in *mtproto.Messages_Messages) *mtproto.Messages_Messages {
	t.Helper()
	buf := mtproto.NewEncodeBuf(2048)
	if err := in.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	out := &mtproto.Messages_Messages{}
	if err := out.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	want := mtproto.TLConstructor(mtproto.GetClazzID(mtproto.Predicate_messages_channelMessages, 232))
	if out.Constructor != want {
		t.Fatalf("container %#x want %#x", uint32(out.Constructor), uint32(want))
	}
	return out
}

func assertLivePost(t *testing.T, m *mtproto.Message, id int32, text string, channelID int64) {
	t.Helper()
	if m.GetPredicateName() != mtproto.Predicate_message || m.Constructor != mtproto.CRC32_message_7600b9d3 {
		t.Fatalf("type pred=%q ctor=%#x text=%q", m.GetPredicateName(), uint32(m.Constructor), m.GetMessage())
	}
	if m.GetId() != id || m.GetMessage() != text || m.GetPeerId().GetChannelId() != channelID || !m.GetPost() {
		t.Fatalf("live id=%d text=%q peer=%d post=%v", m.GetId(), m.GetMessage(), m.GetPeerId().GetChannelId(), m.GetPost())
	}
}

func assertEmptyPost(t *testing.T, m *mtproto.Message, id int32, channelID int64) {
	t.Helper()
	if m.GetPredicateName() != mtproto.Predicate_messageEmpty || m.Constructor != mtproto.CRC32_messageEmpty {
		t.Fatalf("empty pred=%q ctor=%#x", m.GetPredicateName(), uint32(m.Constructor))
	}
	if m.GetId() != id || m.GetMessage() != "" || m.GetPeerId().GetChannelId() != channelID {
		t.Fatalf("empty id=%d text=%q peer=%v", m.GetId(), m.GetMessage(), m.GetPeerId())
	}
}
