package core

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

// chatMemberSpec is one user the client asked to put in a new basic group.
type chatMemberSpec struct {
	UserID     int64
	IsBot      bool
	BotNoChats bool
	Rules      []*mtproto.PrivacyRule
}

// planChatInvites decides who is actually inserted and who is reported in
// missing_invitees. Empty privacy rules mean allow-all, which is the default
// account.getPrivacy returns for chat invites. A disallow is a missing invitee,
// not a reason to fail the whole create.
func planChatInvites(self int64, members []chatMemberSpec, isContact func(owner, peer int64) bool, inChat func(user int64, chats []int64) bool) (users, bots []int64, missing []*mtproto.MissingInvitee) {
	users = make([]int64, 0)
	bots = make([]int64, 0)
	missing = make([]*mtproto.MissingInvitee, 0)
	for _, m := range members {
		if m.UserID == 0 || m.UserID == self {
			continue
		}
		if m.IsBot {
			// Preserves the historical bot filter in messages.createChat.
			if !m.BotNoChats {
				continue
			}
			bots = append(bots, m.UserID)
			continue
		}
		if !chatInviteAllowed(m.UserID, self, m.Rules, isContact, inChat) {
			missing = append(missing, mtproto.MakeTLMissingInvitee(&mtproto.MissingInvitee{
				UserId: m.UserID,
			}).To_MissingInvitee())
			continue
		}
		users = append(users, m.UserID)
	}
	return users, bots, missing
}

// chatInviteAllowed reports whether inviter may add target. No stored rules
// means the user never changed the setting, and the default is everybody.
func chatInviteAllowed(target, inviter int64, rules []*mtproto.PrivacyRule, isContact func(owner, peer int64) bool, inChat func(user int64, chats []int64) bool) bool {
	if len(rules) == 0 {
		return true
	}
	return mtproto.CheckPrivacyIsAllow(target, rules, inviter, isContact, inChat)
}

// sendFailureKeepsChat is true when the chat row already exists and the only
// failure is sending the "group created" service message. iOS shows a 403
// from that call as a privacy restriction, which is not what happened.
func sendFailureKeepsChat(err error) bool {
	if err == nil {
		return false
	}
	if st, ok := status.FromError(err); ok {
		if st.Code() == mtproto.ErrForbidden || strings.Contains(st.Message(), "CHAT_WRITE_FORBIDDEN") {
			return true
		}
	}
	return strings.Contains(err.Error(), "CHAT_WRITE_FORBIDDEN")
}

// updatesForUnsentCreate is the messages.invitedUsers payload used when the
// chat exists but the service message could not be written. Official iOS
// reads the new peer from the first message inside updates, so the result
// has to be updates#74ae4240 with updateNewMessage, not an empty updates or
// a 403.
func updatesForUnsentCreate(selfID int64, chat *mtproto.MutableChat, messages []*mtproto.Message) *mtproto.Updates {
	ups := make([]*mtproto.Update, 0, len(messages))
	for i, msg := range messages {
		if msg == nil {
			continue
		}
		if msg.Id == 0 {
			msg.Id = int32(i + 1)
		}
		ups = append(ups, mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Message_MESSAGE: msg,
			Pts_INT32:       int32(i + 1),
			PtsCount:        1,
		}).To_Update())
	}
	var chats []*mtproto.Chat
	if chat != nil {
		chats = []*mtproto.Chat{chat.ToUnsafeChat(selfID)}
	}
	return mtproto.MakeUpdatesByUpdatesUsersChats(nil, chats, ups...)
}
