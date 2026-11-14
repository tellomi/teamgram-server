package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestChannelAdminRightsSheet covers the channel admin-right bugs:
// ISSUE-S-UNAME-ADMIN-001 (row 195), ISSUE-S-DISCUSSION-ADMIN-001 (row 196),
// ISSUE-S-ADMIN-HIDE-MEMBERS-001 (row 243), ISSUE-S-ADMIN-DIRECT-001 (row 244),
// ISSUE-S-CHAN-SUBLIST-001 (row 210).
// Phones 9996610451–453, code 11111.
func TestChannelAdminRightsSheet(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610451", "Own451")
	admin, _ := dialPhone(t, ctx, cfg, "9996610452", "Adm452")
	member, _ := dialPhone(t, ctx, cfg, "9996610453", "Mem453")
	adminID, adminHash := importPhone(t, ctx, owner, cfg, "9996610452", "Adm452")
	memberID, memberHash := importPhone(t, ctx, owner, cfg, "9996610453", "Mem453")

	groupID, groupHash, err := owner.CreateMegagroup(ctx, "SheetRoom")
	if err != nil {
		t.Fatal(err)
	}
	group := inputChannel(groupID, groupHash)
	if err = owner.InviteToChannel(ctx, group, inputUser(adminID, adminHash), inputUser(memberID, memberHash)); err != nil {
		t.Fatal(err)
	}
	adminGroup := waitChannel(t, ctx, admin, groupID)
	memberGroup := waitChannel(t, ctx, member, groupID)

	editChannelAdmin(t, ctx, owner, group, adminID, adminHash, &mtproto.ChatAdminRights{
		ChangeInfo:        true,
		ManageLinkedPeers: true,
	})
	name := fmt.Sprintf("sroom%d", time.Now().Unix()%100000000)
	if _, err = admin.Invoke(ctx, &mtproto.TLChannelsUpdateUsername{
		Constructor: mtproto.CRC32_channels_updateUsername,
		Channel:     adminGroup,
		Username:    name,
	}); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("admin updateUsername: %v", err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsUpdateUsername{
		Constructor: mtproto.CRC32_channels_updateUsername,
		Channel:     group,
		Username:    name,
	}); err != nil {
		t.Fatal(err)
	}
	if got := channelUsername(t, ctx, member, memberGroup); got != name {
		t.Fatalf("username %q", got)
	}

	if _, err = member.Invoke(ctx, &mtproto.TLChannelsToggleParticipantsHidden{
		Constructor: mtproto.CRC32_channels_toggleParticipantsHidden,
		Channel:     memberGroup,
		Enabled:     mtproto.ToBool(true),
	}); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("member hide: %v", err)
	}
	if _, err = admin.Invoke(ctx, &mtproto.TLChannelsToggleParticipantsHidden{
		Constructor: mtproto.CRC32_channels_toggleParticipantsHidden,
		Channel:     adminGroup,
		Enabled:     mtproto.ToBool(true),
	}); err != nil {
		t.Fatal(err)
	}
	hidden := channelParticipants(t, ctx, member, memberGroup)
	if len(hidden.GetParticipants()) != 0 {
		t.Fatalf("hidden list %d", len(hidden.GetParticipants()))
	}

	newsID, newsHash := createBroadcast(t, ctx, owner, "SheetNews")
	news := inputChannel(newsID, newsHash)
	talkID, talkHash, err := owner.CreateMegagroup(ctx, "SheetTalk")
	if err != nil {
		t.Fatal(err)
	}
	talk := inputChannel(talkID, talkHash)
	if err = owner.InviteToChannel(ctx, news, inputUser(adminID, adminHash)); err != nil {
		t.Fatal(err)
	}
	if err = owner.InviteToChannel(ctx, talk, inputUser(adminID, adminHash)); err != nil {
		t.Fatal(err)
	}
	adminNews := waitChannel(t, ctx, admin, newsID)
	adminTalk := waitChannel(t, ctx, admin, talkID)
	editChannelAdmin(t, ctx, owner, news, adminID, adminHash, &mtproto.ChatAdminRights{ChangeInfo: true})
	editChannelAdmin(t, ctx, owner, talk, adminID, adminHash, &mtproto.ChatAdminRights{
		ChangeInfo:        true,
		ManageLinkedPeers: true,
	})
	if _, err = admin.Invoke(ctx, &mtproto.TLChannelsSetDiscussionGroup{
		Constructor: mtproto.CRC32_channels_setDiscussionGroup,
		Broadcast:   adminNews,
		Group:       adminTalk,
	}); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("discussion without manage_linked_peers: %v", err)
	}
	editChannelAdmin(t, ctx, owner, news, adminID, adminHash, &mtproto.ChatAdminRights{
		ChangeInfo:        true,
		ManageLinkedPeers: true,
	})
	setDiscussion(t, ctx, admin, adminNews, adminTalk)
	if got := linkedChatID(t, ctx, owner, newsID, newsHash); got != talkID {
		t.Fatalf("linked %d", got)
	}
	setDiscussion(t, ctx, admin, adminNews, mtproto.MakeTLInputChannelEmpty(&mtproto.InputChannel{}).To_InputChannel())
	if got := linkedChatID(t, ctx, owner, newsID, newsHash); got != 0 {
		t.Fatalf("unlinked %d", got)
	}

	editChannelAdmin(t, ctx, owner, news, adminID, adminHash, &mtproto.ChatAdminRights{PostMessages: true})
	if _, err = admin.Invoke(ctx, &mtproto.TLChannelsUpdatePaidMessagesPrice{
		Constructor:              mtproto.CRC32_channels_updatePaidMessagesPrice_4b12327b,
		Channel:                  adminNews,
		SendPaidMessagesStars:    0,
		BroadcastMessagesAllowed: true,
	}); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("post admin direct: %v", err)
	}
	editChannelAdmin(t, ctx, owner, news, adminID, adminHash, &mtproto.ChatAdminRights{ManageDirectMessages: true})
	if _, err = admin.Invoke(ctx, &mtproto.TLChannelsUpdatePaidMessagesPrice{
		Constructor:              mtproto.CRC32_channels_updatePaidMessagesPrice_4b12327b,
		Channel:                  adminNews,
		SendPaidMessagesStars:    0,
		BroadcastMessagesAllowed: true,
	}); err != nil {
		t.Fatal(err)
	}
	if !channelByID(t, ctx, owner, newsID, newsHash).GetBroadcastMessagesAllowed() {
		t.Fatal("broadcast_messages_allowed")
	}

	if err = owner.InviteToChannel(ctx, news, inputUser(memberID, memberHash)); err != nil {
		t.Fatal(err)
	}
	memberNews := waitChannel(t, ctx, member, newsID)
	if _, err = member.Invoke(ctx, &mtproto.TLChannelsGetParticipants{
		Constructor: mtproto.CRC32_channels_getParticipants,
		Channel:     memberNews,
		Filter:      mtproto.MakeTLChannelParticipantsRecent(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter(),
		Limit:       50,
	}); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("subscriber list: %v", err)
	}
	ownerList := channelParticipants(t, ctx, owner, news)
	if ownerList.GetCount() < 2 {
		t.Fatalf("owner list %d", ownerList.GetCount())
	}
	full := channelFull(t, ctx, member, memberNews)
	if full.GetCanViewParticipants() {
		t.Fatal("subscriber can_view_participants")
	}
}

func importPhone(t *testing.T, ctx context.Context, c *Client, cfg Config, number, name string) (int64, int64) {
	t.Helper()
	id, hash, err := c.ImportPhone(ctx, number, name)
	if err != nil {
		t.Fatal(err)
	}
	return id, hash
}

func waitChannel(t *testing.T, ctx context.Context, c *Client, channelID int64) *mtproto.InputChannel {
	t.Helper()
	var hash int64
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		got, ok, err := c.ChatAccess(ctx, channelID)
		if err != nil || !ok || got == 0 {
			return false, err
		}
		hash = got
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	return inputChannel(channelID, hash)
}

func editChannelAdmin(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel, userID, userHash int64, rights *mtproto.ChatAdminRights) {
	t.Helper()
	if _, err := c.Invoke(ctx, &mtproto.TLChannelsEditAdmin{
		Constructor:     mtproto.CRC32_channels_editAdmin_9a98ad68,
		Channel:         channel,
		UserId:          inputUser(userID, userHash),
		AdminRights:     mtproto.MakeTLChatAdminRights(rights).To_ChatAdminRights(),
		Rank_FLAGSTRING: wrapperspb.String("mod"),
	}); err != nil {
		t.Fatal(err)
	}
}

func channelFull(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel) *mtproto.ChatFull {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     channel,
	})
	if err != nil {
		t.Fatal(err)
	}
	return asChatFull(t, obj)
}

func channelUsername(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel) string {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     channel,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, chat := range channelFullChats(t, obj) {
		if chat != nil && chat.GetId() == channel.GetChannelId() {
			return chat.GetUsername().GetValue()
		}
	}
	t.Fatal("channel missing from full")
	return ""
}

func channelByID(t *testing.T, ctx context.Context, c *Client, id, hash int64) *mtproto.Chat {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     inputChannel(id, hash),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, chat := range channelFullChats(t, obj) {
		if chat != nil && chat.GetId() == id {
			return chat
		}
	}
	t.Fatal("channel missing")
	return nil
}

func channelFullChats(t *testing.T, obj mtproto.TLObject) []*mtproto.Chat {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesChatFull:
		return v.To_Messages_ChatFull().GetChats()
	case *mtproto.Messages_ChatFull:
		return v.GetChats()
	default:
		t.Fatalf("full chats %T", obj)
		return nil
	}
}

func channelParticipants(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel) *mtproto.Channels_ChannelParticipants {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetParticipants{
		Constructor: mtproto.CRC32_channels_getParticipants,
		Channel:     channel,
		Filter:      mtproto.MakeTLChannelParticipantsRecent(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter(),
		Limit:       50,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLChannelsChannelParticipants:
		return v.To_Channels_ChannelParticipants()
	case *mtproto.Channels_ChannelParticipants:
		return v
	default:
		t.Fatalf("participants %T", obj)
		return nil
	}
}
