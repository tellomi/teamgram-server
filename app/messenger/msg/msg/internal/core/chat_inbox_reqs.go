package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
)

type chatInboxSource interface {
	Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error)
	ToUnsafeChat(int64) *mtproto.Chat
}

// chatInboxReqs builds one inbox publish per current member. includeMigrated
// keeps a member who is already in the supergroup on the single-message path.
// The first send and a retry after a failed publish use this same list.
func chatInboxReqs(fromID, authKeyID, chatID int64, chat chatInboxSource, users *mtproto.MutableUsers, boxes []*mtproto.MessageBox, includeMigrated bool) []*inbox.TLInboxSendUserMessageToInboxV2 {
	if chat == nil {
		return nil
	}
	var reqs []*inbox.TLInboxSendUserMessageToInboxV2
	chat.Walk(func(userId int64, participant *mtproto.ImmutableChatParticipant) error {
		if participant == nil || (!participant.IsChatMemberStateNormal() && !(includeMigrated && participant.IsChatMemberStateMigrated())) {
			return nil
		}
		n := 0
		if users != nil {
			n = users.Length()
		}
		toUsers := make([]*mtproto.User, 0, n)
		if users != nil {
			users.Visit(func(it *mtproto.ImmutableUser) {
				toUsers = append(toUsers, it.ToUser(participant.UserId))
			})
			paintStoryUsers(participant.UserId, users, toUsers)
		}
		reqs = append(reqs, &inbox.TLInboxSendUserMessageToInboxV2{
			UserId:        participant.UserId,
			Out:           participant.UserId == fromID,
			FromId:        fromID,
			FromAuthKeyId: authKeyID,
			PeerType:      mtproto.PEER_CHAT,
			PeerId:        chatID,
			BoxList:       boxes,
			Users:         toUsers,
			Chats:         []*mtproto.Chat{chat.ToUnsafeChat(participant.UserId)},
		})
		return nil
	})
	return reqs
}
