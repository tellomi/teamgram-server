package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestChannelDifferenceSkipsDeletedAndDoesNotRepeat is 行 80 /
// ISSUE-S-CHANDIFF-DUP-001. new_messages keeps the edited text and omits a
// message deleted in the same window. other_updates does not repeat those
// new messages.
// Phone 9996610658, code 11111.
func TestChannelDifferenceSkipsDeletedAndDoesNotRepeat(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610658", "DiffDedup")
	chID, hash, err := c.CreateMegagroup(ctx, fmt.Sprintf("DiffRoom-%d", time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, hash)
	peer := inputPeerChannel(chID, hash)
	caught, err := channelDifference(ctx, c, channel, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	base, ok := caught.(*mtproto.TLUpdatesChannelDifference)
	if !ok {
		t.Fatalf("create difference %T", caught)
	}
	var ids []int32
	for i := 0; i < 3; i++ {
		id, err := c.SendText(ctx, peer, fmt.Sprintf("dedup-%d", i), nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	invokeOK(t, c, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_b106e66c,
		Peer:        peer,
		Id:          ids[0],
		Message:     &wrapperspb.StringValue{Value: "dedup-edited"},
	})
	if _, err := c.Invoke(ctx, &mtproto.TLChannelsDeleteMessages{
		Constructor: mtproto.CRC32_channels_deleteMessages,
		Channel:     channel,
		Id:          []int32{ids[1]},
	}); err != nil {
		t.Fatal(err)
	}
	res, err := channelDifference(ctx, c, channel, base.GetPts(), 100)
	if err != nil {
		t.Fatal(err)
	}
	diff, ok := res.(*mtproto.TLUpdatesChannelDifference)
	if !ok {
		t.Fatalf("difference %T", res)
	}
	var sawEdited, sawDeleted bool
	for _, m := range diff.GetNewMessages() {
		if m.GetId() == ids[0] {
			sawEdited = m.GetMessage() == "dedup-edited"
		}
		if m.GetId() == ids[1] {
			sawDeleted = true
		}
	}
	if !sawEdited || sawDeleted || len(diff.GetNewMessages()) != 2 {
		t.Fatalf("messages %d edited=%v deleted=%v", len(diff.GetNewMessages()), sawEdited, sawDeleted)
	}
	for _, u := range diff.GetOtherUpdates() {
		if u.GetPredicateName() == mtproto.Predicate_updateNewChannelMessage {
			t.Fatal("other_updates repeated a new message")
		}
	}
}
