package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestBasicGroupMemberCanLeaveWhenSendIsBanned covers
// ISSUE-S-BASIC-LEAVE-001 (row 162). A member whose default rights forbid
// sending used to get CHAT_WRITE_FORBIDDEN from messages.deleteChatUser(self)
// even though the membership row was the thing that had to change.
func TestBasicGroupMemberCanLeaveWhenSendIsBanned(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610448", "LeaveOwn")
	member, _ := dialPhone(t, ctx, cfg, "9996610449", "LeaveMem")
	memID, memHash, err := owner.ImportPhone(ctx, "9996610449", "LeaveMem")
	if err != nil {
		t.Fatal(err)
	}
	chatID, err := owner.CreateChat(ctx, "leave-room", inputUser(memID, memHash))
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChat(chatID)
	invokeOK(t, owner, &mtproto.TLMessagesEditChatDefaultBannedRights{
		Constructor: mtproto.CRC32_messages_editChatDefaultBannedRights,
		Peer:        peer,
		BannedRights: mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
			SendMessages: true,
		}).To_ChatBannedRights(),
	})
	_, err = member.SendText(ctx, peer, "should-fail", nil)
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "CHAT_WRITE_FORBIDDEN" {
		t.Fatalf("send while banned: %v", err)
	}
	invokeOK(t, member, &mtproto.TLMessagesDeleteChatUser{
		Constructor: mtproto.CRC32_messages_deleteChatUser,
		ChatId:      chatID,
		UserId:      inputUserSelf(),
	})
	full := asChatFull(t, invokeOK(t, member, &mtproto.TLMessagesGetFullChat{
		Constructor: mtproto.CRC32_messages_getFullChat,
		ChatId:      chatID,
	}))
	if full.GetParticipants().GetPredicateName() != mtproto.Predicate_chatParticipantsForbidden {
		t.Fatalf("leaver still sees participants %#v", full.GetParticipants())
	}
}
