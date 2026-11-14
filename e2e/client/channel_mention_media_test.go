package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMegagroupMentionFlag is ISSUE-S-MENTION-MEGAGROUP-001 (row 76).
// Voice and ttl media_unread (row 296) is covered by
// TestMegagroupMentionFlagAndVoiceUnread, which sends the stored media
// through channel.Send and channels.readMessageContents.
// Phones 9996610451–452, code 11111.
func TestMegagroupMentionFlag(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610451", "Own451")
	member, _ := dialPhone(t, ctx, cfg, "9996610452", "Mem452")
	ownerID, ownerHash, err := member.ImportPhone(ctx, "9996610451", "Own451")
	if err != nil {
		t.Fatal(err)
	}
	groupID, groupHash, err := owner.CreateMegagroup(ctx, "MentionRoom")
	if err != nil {
		t.Fatal(err)
	}
	memberID, memberHash, err := owner.ImportPhone(ctx, "9996610452", "Mem452")
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.InviteToChannel(ctx, inputChannel(groupID, groupHash), inputUser(memberID, memberHash)); err != nil {
		t.Fatal(err)
	}
	memberCh := waitChannelAccess(t, ctx, member, groupID)
	id, err := member.SendText(ctx, inputPeerChannel(memberCh.GetChannelId(), memberCh.GetAccessHash()), "hi", []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityMentionName(&mtproto.MessageEntity{
			Offset: 0, Length: 2, UserId_INT64: ownerID,
		}).To_MessageEntity(),
	})
	if err != nil || id == 0 {
		t.Fatal(err)
	}
	_ = ownerHash
	ownerCh := waitChannelAccess(t, ctx, owner, groupID)
	msg := historyMessage(t, ctx, owner, ownerCh, id)
	if !msg.GetMentioned() {
		t.Fatal("mentioned is false")
	}
}

func waitChannelAccess(t *testing.T, ctx context.Context, c *Client, channelID int64) *mtproto.InputChannel {
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

func historyMessage(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel, id int32) *mtproto.Message {
	t.Helper()
	var found *mtproto.Message
	err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		msgs, err := c.History(ctx, inputPeerChannel(channel.GetChannelId(), channel.GetAccessHash()), 20)
		if err != nil {
			return false, err
		}
		for _, m := range msgs {
			if m.GetId() == id {
				found = m
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil || found == nil {
		t.Fatalf("history %d: %v", id, err)
	}
	return found
}
