package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestChannelForwardCount is row 206. A successful forward of a channel
// post increments message.forwards and messages.getMessagesViews.
func TestChannelForwardCount(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610712", "FwdOwner")
	peerUser, peerSelf := dialPhone(t, ctx, cfg, "9996610713", "FwdPeer")
	channelID, channelHash := createBroadcast(t, ctx, owner, "fwd room")
	postID, err := owner.SendText(ctx, inputPeerChannel(channelID, channelHash), "fwd-post", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLMessagesForwardMessages{
		Constructor: mtproto.CRC32_messages_forwardMessages_13704a7c,
		FromPeer:    inputPeerChannel(channelID, channelHash),
		Id:          []int32{postID},
		RandomId:    []int64{randomID()},
		ToPeer:      inputPeerUser(peerSelf.ID, peerSelf.AccessHash),
	}); err != nil {
		t.Fatal(err)
	}
	_ = peerUser
	viewsObj, err := owner.Invoke(ctx, &mtproto.TLMessagesGetMessagesViews{
		Constructor: mtproto.CRC32_messages_getMessagesViews,
		Peer:        inputPeerChannel(channelID, channelHash),
		Id:          []int32{postID},
		Increment:   mtproto.BoolFalse,
	})
	if err != nil {
		t.Fatal(err)
	}
	views := messageViewsOf(t, viewsObj)
	if len(views) == 0 || views[0].GetForwards().GetValue() < 1 {
		t.Fatalf("views %#v", views)
	}
}

func messageViewsOf(t *testing.T, obj mtproto.TLObject) []*mtproto.MessageViews {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesMessageViews:
		return v.To_Messages_MessageViews().GetViews()
	case *mtproto.Messages_MessageViews:
		return v.GetViews()
	default:
		t.Fatalf("views %T", obj)
		return nil
	}
}
