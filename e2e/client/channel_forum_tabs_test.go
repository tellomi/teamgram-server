package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestForumTabsAndViewAsMessages is ISSUE-S-FORUM-VIEW-001 (row 109).
// Phone 9996610459, code 11111.
func TestForumTabsAndViewAsMessages(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610459", "Own459")
	id, hash, err := owner.CreateMegagroup(ctx, "TabRoom")
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(id, hash)
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsToggleForum{
		Constructor: mtproto.CRC32_channels_toggleForum_3ff75734,
		Channel:     channel,
		Enabled:     mtproto.ToBool(true),
		Tabs:        mtproto.ToBool(true),
	}); err != nil {
		t.Fatal(err)
	}
	chats := forumChats(t, owner, ctx, channel)
	var chat *mtproto.Chat
	for _, c := range chats {
		if c.GetId() == id {
			chat = c
		}
	}
	if chat == nil || !chat.GetForum() || !chat.GetForumTabs() {
		t.Fatalf("chat forum/tabs %+v", chat)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsToggleViewForumAsMessages{
		Constructor: mtproto.CRC32_channels_toggleViewForumAsMessages,
		Channel:     channel,
		Enabled:     mtproto.ToBool(true),
	}); err != nil {
		t.Fatal(err)
	}
	obj, err := owner.Invoke(ctx, &mtproto.TLMessagesGetPeerDialogs{
		Constructor: mtproto.CRC32_messages_getPeerDialogs,
		Peers: []*mtproto.InputDialogPeer{
			mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
				Peer: inputPeerChannel(id, hash),
			}).To_InputDialogPeer(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var seen bool
	for _, d := range dialogsOf(obj) {
		if d.GetPeer().GetChannelId() == id && d.GetViewForumAsMessages() {
			seen = true
		}
	}
	if !seen {
		t.Fatal("getPeerDialogs view_forum_as_messages is false")
	}
}

func forumChats(t *testing.T, c *Client, ctx context.Context, channel *mtproto.InputChannel) []*mtproto.Chat {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetChannels{
		Constructor: mtproto.CRC32_channels_getChannels,
		Id:          []*mtproto.InputChannel{channel},
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesChats:
		return v.GetChats()
	case *mtproto.Messages_Chats:
		return v.GetChats()
	default:
		t.Fatalf("chats %T", obj)
		return nil
	}
}
