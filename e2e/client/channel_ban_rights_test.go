package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestChannelBanPollsAndInvites covers ISSUE-S-BAN-POLLS-001 (row 111) and
// ISSUE-S-BAN-INVITE-MEMBER-001 (row 113).
// Phones 9996610454–456, code 11111.
func TestChannelBanPollsAndInvites(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610454", "Own454")
	member, memberSelf := dialPhone(t, ctx, cfg, "9996610455", "Mem455")
	target, _ := dialPhone(t, ctx, cfg, "9996610456", "Mem456")
	memberID, memberHash := banImport(t, ctx, owner, "9996610455", "Mem455")
	targetID, targetHash := banImport(t, ctx, owner, "9996610456", "Mem456")
	_ = memberSelf

	groupID, groupHash, err := owner.CreateMegagroup(ctx, "BanRoom")
	if err != nil {
		t.Fatal(err)
	}
	group := inputChannel(groupID, groupHash)
	if err = owner.InviteToChannel(ctx, group, inputUser(memberID, memberHash)); err != nil {
		t.Fatal(err)
	}
	memberGroup := banWaitChannel(t, ctx, member, groupID)
	peer := inputPeerChannel(memberGroup.GetChannelId(), memberGroup.GetAccessHash())

	editBanned(t, ctx, owner, group, memberID, memberHash, &mtproto.ChatBannedRights{SendPolls: true})
	if _, err = member.Invoke(ctx, pollSend(peer, time.Now().UnixNano())); err == nil || !strings.Contains(err.Error(), "CHAT_SEND_POLL_FORBIDDEN") {
		t.Fatalf("send poll: %v", err)
	}
	if _, err = member.SendText(ctx, peer, "still-text", nil); err != nil {
		t.Fatalf("text: %v", err)
	}

	editBanned(t, ctx, owner, group, memberID, memberHash, &mtproto.ChatBannedRights{InviteUsers: true})
	if err = member.InviteToChannel(ctx, memberGroup, inputUser(targetID, targetHash)); err == nil || !strings.Contains(err.Error(), "CHAT_ADMIN_REQUIRED") {
		t.Fatalf("banned invite: %v", err)
	}
	if err = owner.InviteToChannel(ctx, group, inputUser(targetID, targetHash)); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		_, ok, err := target.ChatAccess(ctx, groupID)
		return ok, err
	}); err != nil {
		t.Fatal(err)
	}
}

func banImport(t *testing.T, ctx context.Context, c *Client, number, name string) (int64, int64) {
	t.Helper()
	id, hash, err := c.ImportPhone(ctx, number, name)
	if err != nil {
		t.Fatal(err)
	}
	return id, hash
}

func banWaitChannel(t *testing.T, ctx context.Context, c *Client, channelID int64) *mtproto.InputChannel {
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

func editBanned(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel, userID, userHash int64, rights *mtproto.ChatBannedRights) {
	t.Helper()
	if _, err := c.Invoke(ctx, &mtproto.TLChannelsEditBanned{
		Constructor:  mtproto.CRC32_channels_editBanned,
		Channel:      channel,
		Participant:  inputPeerUser(userID, userHash),
		BannedRights: mtproto.MakeTLChatBannedRights(rights).To_ChatBannedRights(),
	}); err != nil {
		t.Fatal(err)
	}
}

func pollSend(peer *mtproto.InputPeer, randomID int64) *mtproto.TLMessagesSendMedia {
	return &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_e25ff8e0,
		Peer:        peer,
		RandomId:    randomID,
		Message:     "",
		Media: mtproto.MakeTLInputMediaPoll(&mtproto.InputMedia{
			Poll: mtproto.MakeTLPoll(&mtproto.Poll{
				Question_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "Pick"}).To_TextWithEntities(),
				Answers: []*mtproto.PollAnswer{
					mtproto.MakePollAnswerWithText("a", []byte("0")),
					mtproto.MakePollAnswerWithText("b", []byte("1")),
				},
			}).To_Poll(),
		}).To_InputMedia(),
	}
}
