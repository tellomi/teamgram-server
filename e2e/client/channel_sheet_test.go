package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestChannelSheet covers direct messages, thread read state, a saved join-as
// peer, can_set_location, and the gigagroup member floor.
func TestChannelSheet(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610770", "LaneA")
	b, bSelf := dialPhone(t, ctx, cfg, "9996610771", "LaneB")

	newsID, newsHash := createBroadcast(t, ctx, a, "LaneNews")
	if err := a.InviteToChannel(ctx, inputChannel(newsID, newsHash), inputUser(bSelf.ID, bSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	var memberHash int64
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hash, ok, err := b.ChatAccess(ctx, newsID)
		if err != nil || !ok {
			return false, err
		}
		memberHash = hash
		return memberHash != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err := b.SendText(ctx, inputPeerChannel(newsID, memberHash), "before", nil)
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "CHAT_WRITE_FORBIDDEN" {
		t.Fatalf("before direct: %v", err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLChannelsUpdatePaidMessagesPrice{
		Constructor:              mtproto.CRC32_channels_updatePaidMessagesPrice_4b12327b,
		BroadcastMessagesAllowed: true,
		Channel:                  inputChannel(newsID, newsHash),
	}); err != nil {
		t.Fatal(err)
	}
	fullObj, err := a.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     inputChannel(newsID, newsHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	var news *mtproto.Chat
	if page, ok := fullObj.(*mtproto.TLMessagesChatFull); ok {
		for _, chat := range page.GetChats() {
			if chat.GetId() == newsID {
				news = chat
			}
		}
	}
	if news == nil || !news.GetMonoforum() || news.GetLinkedMonoforumId().GetValue() != newsID {
		t.Fatalf("monoforum %#v", news)
	}
	if _, err = b.SendText(ctx, inputPeerChannel(newsID, memberHash), "hello news", nil); err != nil {
		t.Fatal(err)
	}

	talkID, talkHash, err := a.CreateMegagroup(ctx, "LaneTalk")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChannel(talkID, talkHash)
	root, err := a.SendText(ctx, peer, "root", nil)
	if err != nil {
		t.Fatal(err)
	}
	reply, err := a.Invoke(ctx, &mtproto.TLMessagesSendMessage{
		Constructor:  mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:         peer,
		Message:      "reply",
		RandomId:     randomID(),
		ReplyToMsgId: wrapperspb.Int32(root),
	})
	if err != nil {
		t.Fatal(err)
	}
	replyID := messageID(reply)
	hist, err := a.History(ctx, peer, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got := messageByID(hist, root); got.GetReplies().GetReplies() != 1 {
		t.Fatalf("replies %#v", got)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLMessagesReadDiscussion{
		Constructor: mtproto.CRC32_messages_readDiscussion,
		Peer:        peer,
		MsgId:       root,
		ReadMaxId:   replyID,
	}); err != nil {
		t.Fatal(err)
	}
	hist, err = a.History(ctx, peer, 20)
	if err != nil || messageByID(hist, root).GetReplies().GetReadMaxId().GetValue() != replyID {
		t.Fatalf("read max %#v %v", hist, err)
	}

	placeID, placeHash, err := a.CreateMegagroup(ctx, "LanePlace")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLPhoneSaveDefaultGroupCallJoinAs{
		Constructor: mtproto.CRC32_phone_saveDefaultGroupCallJoinAs,
		Peer:        inputPeerChannel(placeID, placeHash),
		JoinAs:      inputPeerSelf(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLChannelsEditLocation{
		Constructor: mtproto.CRC32_channels_editLocation,
		Channel:     inputChannel(placeID, placeHash),
		GeoPoint: mtproto.MakeTLInputGeoPoint(&mtproto.InputGeoPoint{
			Lat: 31.2, Long: 121.4,
		}).To_InputGeoPoint(),
		Address: "上海",
	}); err != nil {
		t.Fatal(err)
	}
	placeFull, err := a.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     inputChannel(placeID, placeHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	card := asChatFull(t, placeFull)
	if !card.GetCanSetLocation() || card.GetGroupcallDefaultJoinAs().GetUserId() != aSelf.ID {
		t.Fatalf("place full %#v", card)
	}

	smallID, smallHash, err := a.CreateMegagroup(ctx, "LaneSmall")
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.Invoke(ctx, &mtproto.TLChannelsConvertToGigagroup{
		Constructor: mtproto.CRC32_channels_convertToGigagroup,
		Channel:     inputChannel(smallID, smallHash),
	})
	rpc, ok = asRPCError(err)
	if !ok || rpc.Message != "PARTICIPANTS_TOO_FEW" {
		t.Fatalf("gigagroup: %v", err)
	}
}

func messageByID(msgs []*mtproto.Message, id int32) *mtproto.Message {
	for _, m := range msgs {
		if m.GetId() == id {
			return m
		}
	}
	return nil
}
