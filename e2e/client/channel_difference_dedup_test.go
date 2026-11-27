package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestChannelDifferenceSkipsDeletedAndDoesNotRepeat is
// ISSUE-S-CHANDIFF-DUP-001 (问题清单行 80). After the client records pts,
// five messages, an edit, and a delete land. force getChannelDifference
// returns new_messages with the four that still exist, at the edited text.
// other_updates does not repeat those messages as updateNewChannelMessage.
// Phone 9996610941, code 11111, layer 232.
func TestChannelDifferenceSkipsDeletedAndDoesNotRepeat(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610941", "DiffDedup")
	chID, hash, err := c.CreateMegagroup(ctx, fmt.Sprintf("DiffDedup-%d", time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, hash)
	peer := inputPeerChannel(chID, hash)
	t.Cleanup(func() {
		cctx, cancel := contextTimeout(20 * time.Second)
		defer cancel()
		_, _ = c.Invoke(cctx, &mtproto.TLChannelsDeleteChannel{
			Constructor: mtproto.CRC32_channels_deleteChannel,
			Channel:     channel,
		})
	})

	caught, err := c.Invoke(ctx, channelDifferenceRequest(channel, 0, 100, true))
	if err != nil {
		t.Fatal(err)
	}
	base, ok := caught.(*mtproto.TLUpdatesChannelDifference)
	if !ok {
		t.Fatalf("create difference %T", caught)
	}
	if !base.GetFinal() || base.GetPts() == 0 {
		t.Fatalf("create difference final=%v pts=%d", base.GetFinal(), base.GetPts())
	}
	var ids []int32
	for i := 0; i < 5; i++ {
		id, err := c.SendText(ctx, peer, fmt.Sprintf("dedup-%d", i), nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	edited, deleted := ids[2], ids[4]
	invokeOK(t, c, &mtproto.TLMessagesEditMessage{
		Constructor: mtproto.CRC32_messages_editMessage_b106e66c,
		Peer:        peer,
		Id:          edited,
		Message:     &wrapperspb.StringValue{Value: "dedup-edited"},
	})
	if _, err := c.Invoke(ctx, &mtproto.TLChannelsDeleteMessages{
		Constructor: mtproto.CRC32_channels_deleteMessages,
		Channel:     channel,
		Id:          []int32{deleted},
	}); err != nil {
		t.Fatal(err)
	}
	res, err := c.Invoke(ctx, channelDifferenceRequest(channel, base.GetPts(), 100, true))
	if err != nil {
		t.Fatal(err)
	}
	diff, ok := res.(*mtproto.TLUpdatesChannelDifference)
	if !ok {
		t.Fatalf("difference %T", res)
	}
	if !diff.GetFinal() {
		t.Fatal("difference was not final")
	}
	got := map[int32]string{}
	for _, m := range diff.GetNewMessages() {
		got[m.GetId()] = m.GetMessage()
	}
	if len(got) != 4 || got[edited] != "dedup-edited" {
		t.Fatalf("new_messages %v", got)
	}
	if _, ok := got[deleted]; ok {
		t.Fatalf("deleted %d still in new_messages", deleted)
	}
	for _, id := range []int32{ids[0], ids[1], ids[3]} {
		if got[id] == "" {
			t.Fatalf("missing %d in %v", id, got)
		}
	}
	var sawDelete bool
	for _, u := range diff.GetOtherUpdates() {
		switch u.GetPredicateName() {
		case mtproto.Predicate_updateNewChannelMessage:
			t.Fatalf("other_updates repeated message %d", u.GetMessage_MESSAGE().GetId())
		case mtproto.Predicate_updateEditChannelMessage:
			if u.GetMessage_MESSAGE().GetId() == edited {
				t.Fatal("edit of a message born in this page was repeated")
			}
		case mtproto.Predicate_updateDeleteChannelMessages:
			for _, id := range u.GetMessages() {
				if id == deleted {
					sawDelete = true
				}
			}
		}
	}
	if !sawDelete {
		t.Fatal("delete of the message born in this page was dropped")
	}
}

func channelDifferenceRequest(channel *mtproto.InputChannel, pts, limit int32, force bool) *mtproto.TLUpdatesGetChannelDifference {
	return &mtproto.TLUpdatesGetChannelDifference{
		Constructor: mtproto.CRC32_updates_getChannelDifference,
		Force:       force,
		Channel:     channel,
		Filter:      mtproto.MakeTLChannelMessagesFilterEmpty(nil).To_ChannelMessagesFilter(),
		Pts:         pts,
		Limit:       limit,
	}
}
