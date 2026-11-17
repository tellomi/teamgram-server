package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
)

func TestForeignGroupIDs(t *testing.T) {
	rows := []dataobject.MessagesDO{
		{PeerType: mtproto.PEER_USER, PeerId: 9, SenderUserId: 2},
		{PeerType: mtproto.PEER_CHAT, PeerId: 4, SenderUserId: 1},
		{PeerType: mtproto.PEER_CHAT, PeerId: 4, SenderUserId: 3},
		{PeerType: mtproto.PEER_CHAT, PeerId: 8, SenderUserId: 0},
		{PeerType: mtproto.PEER_CHAT, PeerId: 8, SenderUserId: 5, Deleted: true},
		{PeerType: mtproto.PEER_CHANNEL, PeerId: 99, SenderUserId: 2},
	}
	got := foreignGroupIDs(1, rows)
	if len(got) != 2 || got[0] != 4 || got[1] != 8 {
		t.Fatalf("chats %v", got)
	}
	if len(foreignGroupIDs(2, rows[:1])) != 0 {
		t.Fatal("a private chat is not a group revoke")
	}
}

func TestMayDeleteOthersInChat(t *testing.T) {
	if mayDeleteOthersInChat(nil) {
		t.Fatal("nil")
	}
	member := &mtproto.ImmutableChatParticipant{ParticipantType: mtproto.ChatMemberNormal, State: mtproto.ChatMemberStateNormal}
	if mayDeleteOthersInChat(member) {
		t.Fatal("member")
	}
	creator := &mtproto.ImmutableChatParticipant{ParticipantType: mtproto.ChatMemberCreator, State: mtproto.ChatMemberStateNormal}
	if !mayDeleteOthersInChat(creator) {
		t.Fatal("creator")
	}
	admin := &mtproto.ImmutableChatParticipant{
		ParticipantType: mtproto.ChatMemberAdmin,
		State:           mtproto.ChatMemberStateNormal,
		AdminRights:     mtproto.MakeDefaultChatAdminRights(),
	}
	if !mayDeleteOthersInChat(admin) {
		t.Fatal("admin with delete_messages")
	}
	plain := &mtproto.ImmutableChatParticipant{
		ParticipantType: mtproto.ChatMemberAdmin,
		State:           mtproto.ChatMemberStateNormal,
		AdminRights:     mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{InviteUsers: true}).To_ChatAdminRights(),
	}
	if mayDeleteOthersInChat(plain) {
		t.Fatal("admin without delete_messages")
	}
	left := &mtproto.ImmutableChatParticipant{ParticipantType: mtproto.ChatMemberCreator, State: mtproto.ChatMemberStateLeft}
	if mayDeleteOthersInChat(left) {
		t.Fatal("left creator")
	}
}
