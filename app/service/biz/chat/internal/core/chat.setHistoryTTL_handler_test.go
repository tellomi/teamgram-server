package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestHistoryTTLAllowed(t *testing.T) {
	chat := &mtproto.MutableChat{
		Chat: &mtproto.ImmutableChat{Id: 30, Creator: 1},
		ChatParticipants: []*mtproto.ImmutableChatParticipant{
			{UserId: 1, State: mtproto.ChatMemberStateNormal, ParticipantType: mtproto.ChatMemberCreator},
			{UserId: 2, State: mtproto.ChatMemberStateNormal, ParticipantType: mtproto.ChatMemberAdmin},
			{UserId: 3, State: mtproto.ChatMemberStateNormal, ParticipantType: mtproto.ChatMemberNormal},
			{UserId: 4, State: mtproto.ChatMemberStateLeft, ParticipantType: mtproto.ChatMemberNormal},
		},
	}
	if err := historyTTLAllowed(chat, 1); err != nil {
		t.Fatal(err)
	}
	if err := historyTTLAllowed(chat, 2); err != nil {
		t.Fatal(err)
	}
	if status.Convert(historyTTLAllowed(chat, 3)).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatal(historyTTLAllowed(chat, 3))
	}
	if status.Convert(historyTTLAllowed(chat, 4)).Message() != "PEER_ID_INVALID" {
		t.Fatal(historyTTLAllowed(chat, 4))
	}
	if status.Convert(historyTTLAllowed(chat, 9)).Message() != "PEER_ID_INVALID" {
		t.Fatal(historyTTLAllowed(chat, 9))
	}
}
