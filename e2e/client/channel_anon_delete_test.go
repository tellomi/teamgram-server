package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestAnonymousAdminSender is ISSUE-S-ANON-ADMIN-001 (row 127).
// Phones 9996610451–453, code 11111.
func TestAnonymousAdminSender(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610451", "Own451")
	admin, _ := dialPhone(t, ctx, cfg, "9996610452", "Adm452")
	member, _ := dialPhone(t, ctx, cfg, "9996610453", "Mem453")
	adminID, adminHash := anonImport(t, ctx, owner, "9996610452", "Adm452")
	memberID, memberHash := anonImport(t, ctx, owner, "9996610453", "Mem453")
	groupID, groupHash, err := owner.CreateMegagroup(ctx, "AnonRoom")
	if err != nil {
		t.Fatal(err)
	}
	group := inputChannel(groupID, groupHash)
	if err = owner.InviteToChannel(ctx, group, inputUser(adminID, adminHash), inputUser(memberID, memberHash)); err != nil {
		t.Fatal(err)
	}
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsEditAdmin{
		Constructor: mtproto.CRC32_channels_editAdmin_9a98ad68,
		Channel:     group,
		UserId:      inputUser(adminID, adminHash),
		AdminRights: mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
			DeleteMessages: true,
			Anonymous:      true,
		}).To_ChatAdminRights(),
		Rank_FLAGSTRING: wrapperspb.String("mod"),
	}); err != nil {
		t.Fatal(err)
	}
	adminCh := anonWait(t, ctx, admin, groupID)
	memberCh := anonWait(t, ctx, member, groupID)
	if _, err = admin.SendText(ctx, inputPeerChannel(adminCh.GetChannelId(), adminCh.GetAccessHash()), "anon-body", nil); err != nil {
		t.Fatal(err)
	}
	adminMsg := anonFind(t, ctx, admin, adminCh, "anon-body")
	if !adminMsg.GetOut() || adminMsg.GetFromId().GetChannelId() != groupID || adminMsg.GetFromId().GetUserId() != 0 {
		t.Fatalf("admin from %+v out=%v", adminMsg.GetFromId(), adminMsg.GetOut())
	}
	memberMsg := anonFind(t, ctx, member, memberCh, "anon-body")
	if memberMsg.GetOut() || memberMsg.GetFromId().GetChannelId() != groupID || memberMsg.GetFromId().GetUserId() != 0 {
		t.Fatalf("member from %+v out=%v", memberMsg.GetFromId(), memberMsg.GetOut())
	}
	posts := messagesOfResult(t, invokeOK(t, member, &mtproto.TLChannelsSearchPosts{
		Constructor: mtproto.CRC32_channels_searchPosts_f2c4f24d,
		Query:       &wrapperspb.StringValue{Value: "anon-body"},
		OffsetPeer:  inputPeerEmpty(),
		Limit:       20,
	}))
	searched := anonMessageText(posts.GetMessages(), "anon-body")
	if searched == nil || searched.GetFromId().GetChannelId() != groupID || searched.GetFromId().GetUserId() != 0 {
		t.Fatalf("searchPosts from %+v", searched.GetFromId())
	}
	for _, user := range posts.GetUsers() {
		if user.GetId() == adminID {
			t.Fatal("searchPosts users include the anonymous admin")
		}
	}
	filtered := messagesOfResult(t, invokeOK(t, member, &mtproto.TLMessagesSearch{
		Constructor: mtproto.CRC32_messages_search_a0fda762,
		Peer:        inputPeerChannel(memberCh.GetChannelId(), memberCh.GetAccessHash()),
		Q:           "anon-body",
		FromId:      inputPeerUser(adminID, adminHash),
		Limit:       20,
	}))
	if anonMessageText(filtered.GetMessages(), "anon-body") != nil {
		t.Fatal("messages.search from the admin listed the anonymous post")
	}
}

// TestDeleteChannelPushesMembers is ISSUE-S-CHANDEL-PUSH-001 (row 156).
// Phones 9996610454–455.
func TestDeleteChannelPushesMembers(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610454", "Own454")
	member, _ := dialPhone(t, ctx, cfg, "9996610455", "Mem455")
	memberID, memberHash := anonImport(t, ctx, owner, "9996610455", "Mem455")
	groupID, groupHash, err := owner.CreateMegagroup(ctx, "GoneRoom")
	if err != nil {
		t.Fatal(err)
	}
	group := inputChannel(groupID, groupHash)
	if err = owner.InviteToChannel(ctx, group, inputUser(memberID, memberHash)); err != nil {
		t.Fatal(err)
	}
	if err = anonWaitSeen(ctx, member, groupID); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = member.State(ctx); err != nil {
		t.Fatal(err)
	}
	before := len(member.Pushed())
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsDeleteChannel{
		Constructor: mtproto.CRC32_channels_deleteChannel,
		Channel:     group,
	}); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		return sawUpdateChannel(member.Pushed()[before:], groupID), nil
	}); err != nil {
		t.Fatalf("member missed updateChannel: %v", err)
	}
	_ = groupHash
}

func anonMessageText(msgs []*mtproto.Message, text string) *mtproto.Message {
	for _, m := range msgs {
		if m != nil && m.GetMessage() == text {
			return m
		}
	}
	return nil
}

func anonImport(t *testing.T, ctx context.Context, c *Client, number, name string) (int64, int64) {
	t.Helper()
	id, hash, err := c.ImportPhone(ctx, number, name)
	if err != nil {
		t.Fatal(err)
	}
	return id, hash
}

func anonWait(t *testing.T, ctx context.Context, c *Client, channelID int64) *mtproto.InputChannel {
	t.Helper()
	var hash int64
	if err := anonWaitSeen(ctx, c, channelID); err != nil {
		t.Fatal(err)
	}
	hash, _, _ = c.ChatAccess(ctx, channelID)
	return inputChannel(channelID, hash)
}

func anonWaitSeen(ctx context.Context, c *Client, channelID int64) error {
	return waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		got, ok, err := c.ChatAccess(ctx, channelID)
		return err == nil && ok && got != 0, err
	})
}

func anonFind(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel, text string) *mtproto.Message {
	t.Helper()
	var found *mtproto.Message
	err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		msgs, err := c.History(ctx, inputPeerChannel(channel.GetChannelId(), channel.GetAccessHash()), 20)
		if err != nil {
			return false, err
		}
		for _, m := range msgs {
			if m.GetMessage() == text {
				found = m
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil || found == nil {
		t.Fatalf("history %s: %v", text, err)
	}
	return found
}
