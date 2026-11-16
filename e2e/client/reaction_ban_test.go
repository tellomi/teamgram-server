package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMegagroupSendReactionsBan is ISSUE-S-BAN-REACT-001 (表格行 112).
// A default send_reactions ban and a per-member ban both reject
// messages.sendReaction with 403 CHAT_SEND_REACTIONS_FORBIDDEN. Text still
// sends. Phones 9996610642 and 9996610643, code 11111, layer 232.
func TestMegagroupSendReactionsBan(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610642", "ReactA")
	b, _ := dialPhone(t, ctx, cfg, "9996610643", "ReactB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610643", "ReactB")
	if err != nil {
		t.Fatal(err)
	}
	title := fmt.Sprintf("ReactRoom-%d", time.Now().Unix())
	chID, chHash, err := a.CreateMegagroup(ctx, title)
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, chHash)
	if err = a.InviteToChannel(ctx, channel, inputUser(bID, bHash)); err != nil {
		t.Fatal(err)
	}
	var memberHash int64
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		got, ok, err := b.ChatAccess(ctx, chID)
		if err != nil || !ok {
			return false, err
		}
		memberHash = got
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	peerA := inputPeerChannel(chID, chHash)
	peerB := inputPeerChannel(chID, memberHash)
	msgID, err := a.SendText(ctx, peerA, fmt.Sprintf("react-ban-%d", time.Now().UnixNano()), nil)
	if err != nil {
		t.Fatal(err)
	}
	noReact := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{SendReactions: true}).To_ChatBannedRights()
	if _, err = a.Invoke(ctx, &mtproto.TLMessagesEditChatDefaultBannedRights{
		Constructor:  mtproto.CRC32_messages_editChatDefaultBannedRights,
		Peer:         peerA,
		BannedRights: noReact,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = b.SendReaction(ctx, peerB, msgID, "👍")
	rpc, ok := asRPCError(err)
	if !ok || rpc.Code != 403 || rpc.Message != "CHAT_SEND_REACTIONS_FORBIDDEN" {
		t.Fatalf("default ban: %v", err)
	}
	if _, err = b.SendText(ctx, peerB, "still-text", nil); err != nil {
		t.Fatalf("text: %v", err)
	}
	if _, err = a.SendReaction(ctx, peerA, msgID, "👍"); err != nil {
		t.Fatalf("owner reaction: %v", err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLMessagesEditChatDefaultBannedRights{
		Constructor:  mtproto.CRC32_messages_editChatDefaultBannedRights,
		Peer:         peerA,
		BannedRights: mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{}).To_ChatBannedRights(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Invoke(ctx, &mtproto.TLChannelsEditBanned{
		Constructor:  mtproto.CRC32_channels_editBanned,
		Channel:      channel,
		Participant:  inputPeerUser(bID, bHash),
		BannedRights: noReact,
	}); err != nil {
		t.Fatal(err)
	}
	_, err = b.SendReaction(ctx, peerB, msgID, "👍")
	rpc, ok = asRPCError(err)
	if !ok || rpc.Code != 403 || rpc.Message != "CHAT_SEND_REACTIONS_FORBIDDEN" {
		t.Fatalf("per-user ban: %v", err)
	}
}
