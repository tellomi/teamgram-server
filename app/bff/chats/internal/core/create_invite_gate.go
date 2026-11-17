package core

import "github.com/teamgram/proto/mtproto"

// omitRestrictedCreateMembers applies the same gate as messages.addChatUser:
// a member who blocked the creator, or who the creator blocked, or whose
// chat-invite privacy rejects the creator, is not inserted. They stay in
// missing_invitees. channels.createChannel has no users vector, so this
// gate is only for messages.createChat.
func (c *ChatsCore) omitRestrictedCreateMembers(users []int64, missing []*mtproto.MissingInvitee) ([]int64, []*mtproto.MissingInvitee) {
	if c == nil || c.MD == nil || len(users) == 0 {
		return users, missing
	}
	kept := make([]int64, 0, len(users))
	for _, id := range users {
		if miss := c.chatInviteRestricted(c.MD.UserId, id); miss != nil {
			missing = append(missing, miss)
			continue
		}
		kept = append(kept, id)
	}
	return kept, missing
}

// legacyCreatePrivacyError is USER_PRIVACY_RESTRICTED for the pre-layer-232
// messages.createChat results, which are Updates and cannot carry
// missing_invitees. The chat is not created when every requested member is
// skipped. A layer 232 messages.invitedUsers result keeps the creator-only
// chat and the missing list instead.
func legacyCreatePrivacyError(updatesOnly bool, users, bots []int64, missing []*mtproto.MissingInvitee) error {
	if updatesOnly && len(users) == 0 && len(bots) == 0 && len(missing) > 0 {
		return mtproto.ErrUserPrivacyRestricted
	}
	return nil
}
