package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMegagroupMemberRank is row 191. A supergroup member can set their own
// rank, and cannot set someone else's.
func TestMegagroupMemberRank(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	owner, ownerSelf := dialPhone(t, ctx, cfg, "9996610702", "RankOwner")
	member, memberSelf := dialPhone(t, ctx, cfg, "9996610703", "RankMember")
	id, hash, err := owner.CreateMegagroup(ctx, "rank room")
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(id, hash)
	if err = owner.InviteToChannel(ctx, channel, inputUser(memberSelf.ID, memberSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	var memberHash int64
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		got, ok, err := member.ChatAccess(ctx, id)
		if err != nil || !ok || got == 0 {
			return false, err
		}
		memberHash = got
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}

	peer := inputPeerChannel(id, memberHash)
	if _, err = member.Invoke(ctx, &mtproto.TLMessagesEditChatParticipantRank{
		Constructor: mtproto.CRC32_messages_editChatParticipantRank,
		Peer:        peer,
		Participant: inputPeerSelf(),
		Rank:        "tag",
	}); err != nil {
		t.Fatal(err)
	}
	if got := channelParticipantRank(t, member, id, memberHash, memberSelf.ID, memberSelf.AccessHash); got != "tag" {
		t.Fatalf("self rank %q", got)
	}
	_, err = member.Invoke(ctx, &mtproto.TLMessagesEditChatParticipantRank{
		Constructor: mtproto.CRC32_messages_editChatParticipantRank,
		Peer:        peer,
		Participant: inputPeerUser(ownerSelf.ID, ownerSelf.AccessHash),
		Rank:        "boss",
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member edited the owner: %v", err)
	}
}

func channelParticipantRank(t *testing.T, c *Client, channelID, channelHash, userID, userHash int64) string {
	t.Helper()
	ctx, cancel := contextTimeout(20 * time.Second)
	defer cancel()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetParticipant{
		Constructor: mtproto.CRC32_channels_getParticipant,
		Channel:     inputChannel(channelID, channelHash),
		Participant: inputPeerUser(userID, userHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLChannelsChannelParticipant:
		return v.To_Channels_ChannelParticipant().GetParticipant().GetRank().GetValue()
	case *mtproto.Channels_ChannelParticipant:
		return v.GetParticipant().GetRank().GetValue()
	default:
		t.Fatalf("participant %T", obj)
		return ""
	}
}
