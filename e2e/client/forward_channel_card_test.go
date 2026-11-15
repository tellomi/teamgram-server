package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestForwardedChannelCardReachesNonMember is 行 157 / ISSUE-S-FWD-CHAN-ENTITY-001.
// A channel post forwarded into a private chat must carry the channel in
// chats for a recipient who is not a subscriber, including a min channel
// when the channel is private.
// Phones 9996610655 / 9996610656, code 11111.
func TestForwardedChannelCardReachesNonMember(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	a, aSelf := dialPhone(t, ctx, cfg, "9996610655", "FwdA")
	b, _ := dialPhone(t, ctx, cfg, "9996610656", "FwdB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610656", "FwdB")
	if err != nil {
		t.Fatal(err)
	}
	aID, aHash, err := b.ImportPhone(ctx, "9996610655", "FwdA")
	if err != nil {
		t.Fatal(err)
	}
	_ = aSelf

	const title = "FwdNews"
	created, err := a.Invoke(ctx, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Broadcast:   true,
		Title:       title,
	})
	if err != nil {
		t.Fatal(err)
	}
	var chID, chHash int64
	for _, chat := range chatsOf(created) {
		if chat != nil && chat.GetBroadcast() && chat.GetTitle() == title {
			chID, chHash = chat.GetId(), chatAccessHash(chat)
		}
	}
	if chID == 0 {
		t.Fatalf("create channel %T", created)
	}
	channelPeer := inputPeerChannel(chID, chHash)
	text := fmt.Sprintf("fwd-card-%d", time.Now().UnixNano())
	postID, err := a.SendText(ctx, channelPeer, text, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Invoke(ctx, &mtproto.TLMessagesForwardMessages{
		Constructor: mtproto.CRC32_messages_forwardMessages_13704a7c,
		FromPeer:    channelPeer,
		Id:          []int32{postID},
		RandomId:    []int64{randomID()},
		ToPeer:      inputPeerUser(bID, bHash),
	}); err != nil {
		t.Fatal(err)
	}

	peerA := inputPeerUser(aID, aHash)
	var boxID int32
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		page := messagesOfResult(t, invokeOK(t, b, &mtproto.TLMessagesGetHistory{
			Constructor: mtproto.CRC32_messages_getHistory,
			Peer:        peerA,
			Limit:       20,
		}))
		for _, m := range page.GetMessages() {
			if m.GetMessage() != text {
				continue
			}
			if m.GetFwdFrom().GetFromId().GetChannelId() != chID {
				return false, fmt.Errorf("fwd_from channel %d", m.GetFwdFrom().GetFromId().GetChannelId())
			}
			if !channelCard(page.GetChats(), chID, title) {
				return false, fmt.Errorf("getHistory chats missing %d", chID)
			}
			boxID = m.GetId()
			return true, nil
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}

	got := messagesOfResult(t, invokeOK(t, b, &mtproto.TLMessagesGetMessages{
		Constructor:           mtproto.CRC32_messages_getMessages_63c66506,
		Id_VECTORINPUTMESSAGE: []*mtproto.InputMessage{mtproto.MakeTLInputMessageID(&mtproto.InputMessage{Id: boxID}).To_InputMessage()},
	}))
	if !channelCard(got.GetChats(), chID, title) {
		t.Fatalf("getMessages chats %#v", got.GetChats())
	}
	page, err := b.dialogPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !channelCard(page.Chats, chID, title) {
		t.Fatalf("getDialogs chats %#v", page.Chats)
	}
}

func channelCard(chats []*mtproto.Chat, id int64, title string) bool {
	for _, ch := range chats {
		if ch.GetId() == id && ch.GetPredicateName() == mtproto.Predicate_channel && ch.GetTitle() == title {
			return true
		}
	}
	return false
}
