package core

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestPlanChatInvitesDefaultAllowsAndEmptyList(t *testing.T) {
	users, bots, missing := planChatInvites(1, nil, nil, nil)
	if len(users) != 0 || len(bots) != 0 || len(missing) != 0 {
		t.Fatalf("empty plan = %v %v %v", users, bots, missing)
	}

	users, bots, missing = planChatInvites(1, []chatMemberSpec{
		{UserID: 2},
		{UserID: 3, Rules: nil},
	}, func(int64, int64) bool { return false }, func(int64, []int64) bool { return false })
	if len(missing) != 0 || len(bots) != 0 || len(users) != 2 || users[0] != 2 || users[1] != 3 {
		t.Fatalf("default privacy users=%v missing=%v", users, missing)
	}
}

func TestPlanChatInvitesPrivacyAndSeveralMembers(t *testing.T) {
	disallow := []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule()}
	allowContacts := []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule()}
	allowAll := []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule()}

	users, _, missing := planChatInvites(10, []chatMemberSpec{
		{UserID: 11, Rules: allowAll},
		{UserID: 12, Rules: disallow},
		{UserID: 13, Rules: allowContacts},
		{UserID: 14, Rules: allowContacts},
		{UserID: 10},
	}, func(owner, peer int64) bool {
		return owner == 13 && peer == 10
	}, func(int64, []int64) bool { return false })

	if len(users) != 2 || users[0] != 11 || users[1] != 13 {
		t.Fatalf("added = %v", users)
	}
	if len(missing) != 2 || missing[0].UserId != 12 || missing[1].UserId != 14 {
		t.Fatalf("missing = %#v", missing)
	}
}

func TestSendFailureKeepsChatOnlyForWriteForbidden(t *testing.T) {
	if sendFailureKeepsChat(nil) {
		t.Fatal("nil")
	}
	if sendFailureKeepsChat(errors.New("dial tcp")) {
		t.Fatal("transport error must still fail the RPC")
	}
	if !sendFailureKeepsChat(mtproto.ErrChatWriteForbidden) {
		t.Fatal("CHAT_WRITE_FORBIDDEN should not be returned after the chat exists")
	}
	if !sendFailureKeepsChat(status.Error(mtproto.ErrForbidden, "USER_PRIVACY_RESTRICTED")) {
		t.Fatal("any 403 after create must not be forwarded")
	}
	if sendFailureKeepsChat(mtproto.ErrUsersTooFew) {
		t.Fatal("USERS_TOO_FEW is not a post-create 403")
	}
}

func TestUpdatesForUnsentCreateAreLayer232Updates(t *testing.T) {
	const self int64 = 7
	const chatID int64 = 42
	chat := &mtproto.MutableChat{
		Chat: &mtproto.ImmutableChat{Id: chatID, Title: "empty"},
		ChatParticipants: []*mtproto.ImmutableChatParticipant{{
			UserId:          self,
			ParticipantType: mtproto.ChatMemberCreator,
		}},
	}
	msg := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		FromId: mtproto.MakePeerUser(self),
		PeerId: mtproto.MakePeerChat(chatID),
		Date:   1_700_000_000,
		Action: mtproto.MakeMessageActionChatCreate("empty", nil),
	}).To_Message()

	up := updatesForUnsentCreate(self, chat, []*mtproto.Message{msg})
	buf := mtproto.NewEncodeBuf(256)
	if err := up.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if len(raw) < 4 {
		t.Fatal("short")
	}
	if binary.LittleEndian.Uint32(raw[:4]) != 0x74ae4240 {
		t.Fatalf("ctor 0x%x, want updates#74ae4240", binary.LittleEndian.Uint32(raw[:4]))
	}
	got := &mtproto.Updates{}
	if err := got.Decode(mtproto.NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if len(got.GetChats()) != 1 || got.GetChats()[0].GetId() != chatID {
		t.Fatalf("chats = %#v", got.GetChats())
	}
	if len(got.GetUpdates()) != 1 || got.GetUpdates()[0].GetMessage_MESSAGE().GetPeerId().GetChatId() != chatID {
		t.Fatalf("updates = %#v", got.GetUpdates())
	}
	if got.GetUpdates()[0].GetMessage_MESSAGE().GetId() == 0 {
		t.Fatal("message id was not filled")
	}
}
