package mtproto

// ChatParticipantMayChangeInfo reports whether a basic-group member may edit
// the title, photo, or description.
//
// The creator always may. An admin may when admin_rights.change_info is set.
// Everyone else, including an admin without that right, follows
// default_banned_rights.change_info. A set change_info flag is a ban. A nil
// ban list means the right is not banned, which is the basic-group default.
func ChatParticipantMayChangeInfo(me *ImmutableChatParticipant, defaults *ChatBannedRights, now int32) bool {
	if me == nil || me.State != ChatMemberStateNormal {
		return false
	}
	if me.IsChatMemberCreator() {
		return true
	}
	if me.IsChatMemberAdmin() && me.GetAdminRights().GetChangeInfo() {
		return true
	}
	if defaults == nil {
		return true
	}
	return defaults.CanChangeInfo(now)
}
