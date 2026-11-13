package mtproto

import (
	"testing"
	"time"
)

func TestChatParticipantMayChangeInfo(t *testing.T) {
	now := int32(time.Now().Unix())
	open := MakeTLChatBannedRights(&ChatBannedRights{UntilDate: now + 3600}).To_ChatBannedRights()
	banned := MakeTLChatBannedRights(&ChatBannedRights{ChangeInfo: true, UntilDate: now + 3600}).To_ChatBannedRights()
	expired := MakeTLChatBannedRights(&ChatBannedRights{ChangeInfo: true, UntilDate: now - 10}).To_ChatBannedRights()
	withRight := MakeTLChatAdminRights(&ChatAdminRights{ChangeInfo: true}).To_ChatAdminRights()
	without := MakeTLChatAdminRights(&ChatAdminRights{InviteUsers: true}).To_ChatAdminRights()

	creator := &ImmutableChatParticipant{ParticipantType: ChatMemberCreator, State: ChatMemberStateNormal}
	adminRight := &ImmutableChatParticipant{ParticipantType: ChatMemberAdmin, State: ChatMemberStateNormal, AdminRights: withRight}
	adminPlain := &ImmutableChatParticipant{ParticipantType: ChatMemberAdmin, State: ChatMemberStateNormal, AdminRights: without}
	member := &ImmutableChatParticipant{ParticipantType: ChatMemberNormal, State: ChatMemberStateNormal}
	left := &ImmutableChatParticipant{ParticipantType: ChatMemberNormal, State: ChatMemberStateLeft}

	if !ChatParticipantMayChangeInfo(creator, banned, now) {
		t.Fatal("creator")
	}
	if !ChatParticipantMayChangeInfo(adminRight, banned, now) {
		t.Fatal("admin with change_info")
	}
	if ChatParticipantMayChangeInfo(adminPlain, banned, now) {
		t.Fatal("admin without change_info is bound by the default ban")
	}
	if !ChatParticipantMayChangeInfo(adminPlain, open, now) {
		t.Fatal("admin without the right may edit when the default ban is clear")
	}
	if ChatParticipantMayChangeInfo(member, banned, now) {
		t.Fatal("member is banned by default_banned_rights.change_info")
	}
	if !ChatParticipantMayChangeInfo(member, open, now) {
		t.Fatal("member may edit when change_info is not banned")
	}
	if !ChatParticipantMayChangeInfo(member, nil, now) {
		t.Fatal("nil default banned rights does not ban change_info")
	}
	if !ChatParticipantMayChangeInfo(member, expired, now) {
		t.Fatal("an expired change_info ban does not apply")
	}
	if ChatParticipantMayChangeInfo(left, open, now) {
		t.Fatal("left member")
	}
	if ChatParticipantMayChangeInfo(nil, open, now) {
		t.Fatal("nil participant")
	}
}
