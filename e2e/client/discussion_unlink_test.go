package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestDiscussionUnlinkClearsGroupSide is channels.setDiscussionGroup.
// Rebind and unlink must clear the previous group's linked_chat_id, and a
// member of that group receives updateChannel.
func TestDiscussionUnlinkClearsGroupSide(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "disc_a", "DiscA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "disc_b", "DiscB")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	phone, err := cfg.Phone("disc_b")
	if err != nil {
		t.Fatal(err)
	}
	bID, bHash, err := a.ImportPhone(ctx, phone.Number, "DiscB")
	if err != nil {
		t.Fatal(err)
	}
	channelID, channelHash := createBroadcast(t, ctx, a, "NewsDesk")
	hallID, hallHash, err := a.CreateMegagroup(ctx, "HallChat")
	if err != nil {
		t.Fatal(err)
	}
	yardID, yardHash, err := a.CreateMegagroup(ctx, "YardChat")
	if err != nil {
		t.Fatal(err)
	}
	if err = a.InviteToChannel(ctx, inputChannel(hallID, hallHash), inputUser(bID, bHash)); err != nil {
		t.Fatal(err)
	}
	var memberHash int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hash, ok, err := b.ChatAccess(ctx, hallID)
		if err != nil || !ok {
			return false, err
		}
		memberHash = hash
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}

	setDiscussion(t, ctx, a, inputChannel(channelID, channelHash), inputChannel(hallID, hallHash))
	if got := linkedChatID(t, ctx, a, channelID, channelHash); got != hallID {
		t.Fatalf("channel linked %d", got)
	}
	if got := linkedChatID(t, ctx, b, hallID, memberHash); got != channelID {
		t.Fatalf("hall linked %d", got)
	}

	if _, _, _, err = b.State(ctx); err != nil {
		t.Fatal(err)
	}
	before := len(b.Pushed())
	setDiscussion(t, ctx, a, inputChannel(channelID, channelHash), inputChannel(yardID, yardHash))
	if got := linkedChatID(t, ctx, a, channelID, channelHash); got != yardID {
		t.Fatalf("rebind channel %d", got)
	}
	if got := linkedChatID(t, ctx, a, yardID, yardHash); got != channelID {
		t.Fatalf("yard linked %d", got)
	}
	if got := linkedChatID(t, ctx, b, hallID, memberHash); got != 0 {
		t.Fatalf("hall still linked %d", got)
	}
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		return sawUpdateChannel(b.Pushed()[before:], hallID), nil
	}); err != nil {
		t.Fatalf("member missed updateChannel for hall: %v; pushed=%s", err, pushedSummary(b.Pushed()[before:]))
	}

	if err = a.InviteToChannel(ctx, inputChannel(yardID, yardHash), inputUser(bID, bHash)); err != nil {
		t.Fatal(err)
	}
	var yardMember int64
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		hash, ok, err := b.ChatAccess(ctx, yardID)
		if err != nil || !ok {
			return false, err
		}
		yardMember = hash
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = b.State(ctx); err != nil {
		t.Fatal(err)
	}
	before = len(b.Pushed())
	setDiscussion(t, ctx, a, inputChannel(channelID, channelHash), mtproto.MakeTLInputChannelEmpty(&mtproto.InputChannel{}).To_InputChannel())
	if got := linkedChatID(t, ctx, a, channelID, channelHash); got != 0 {
		t.Fatalf("unlinked channel %d", got)
	}
	if got := linkedChatID(t, ctx, a, yardID, yardHash); got != 0 {
		t.Fatalf("yard still linked %d", got)
	}
	if got := linkedChatID(t, ctx, b, yardID, yardMember); got != 0 {
		t.Fatalf("member yard link %d", got)
	}
	if got := linkedChatID(t, ctx, b, hallID, memberHash); got != 0 {
		t.Fatalf("hall changed %d", got)
	}
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		return sawUpdateChannel(b.Pushed()[before:], yardID), nil
	}); err != nil {
		t.Fatalf("member missed updateChannel for yard: %v; pushed=%s", err, pushedSummary(b.Pushed()[before:]))
	}
}

func createBroadcast(t *testing.T, ctx context.Context, c *Client, title string) (int64, int64) {
	t.Helper()
	created, err := c.Invoke(ctx, &mtproto.TLChannelsCreateChannel{
		Constructor: mtproto.CRC32_channels_createChannel_91006707,
		Broadcast:   true,
		Title:       title,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, chat := range chatsOf(created) {
		if chat != nil && chat.GetBroadcast() {
			return chat.GetId(), chatAccessHash(chat)
		}
	}
	t.Fatalf("createChannel %T", created)
	return 0, 0
}

func setDiscussion(t *testing.T, ctx context.Context, c *Client, broadcast, group *mtproto.InputChannel) {
	t.Helper()
	if _, err := c.Invoke(ctx, &mtproto.TLChannelsSetDiscussionGroup{
		Constructor: mtproto.CRC32_channels_setDiscussionGroup,
		Broadcast:   broadcast,
		Group:       group,
	}); err != nil {
		t.Fatal(err)
	}
}

func linkedChatID(t *testing.T, ctx context.Context, c *Client, channelID, accessHash int64) int64 {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     inputChannel(channelID, accessHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	return asChatFull(t, obj).GetLinkedChatId().GetValue()
}

func sawUpdateChannel(objs []mtproto.TLObject, channelID int64) bool {
	for _, obj := range objs {
		for _, upd := range updatesOf(obj) {
			if upd.GetPredicateName() == mtproto.Predicate_updateChannel && upd.GetChannelId() == channelID {
				return true
			}
		}
	}
	return false
}
