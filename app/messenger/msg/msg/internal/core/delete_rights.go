package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// foreignGroupIDs lists basic groups where revoke would delete someone else's
// message. The sender's own messages are not included. A zero sender is not
// the caller's message, so a service row still needs delete_messages.
func foreignGroupIDs(userID int64, rows []dataobject.MessagesDO) []int64 {
	seen := map[int64]struct{}{}
	var ids []int64
	for _, row := range rows {
		if row.Deleted || row.PeerType != mtproto.PEER_CHAT || row.PeerId == 0 {
			continue
		}
		if row.SenderUserId == userID {
			continue
		}
		if _, ok := seen[row.PeerId]; ok {
			continue
		}
		seen[row.PeerId] = struct{}{}
		ids = append(ids, row.PeerId)
	}
	return ids
}

// mayDeleteOthersInChat is the basic-group delete_messages rule.
// The creator always may. An admin may when admin_rights.delete_messages is
// set. A regular member may not delete another member's message for everyone.
func mayDeleteOthersInChat(me *mtproto.ImmutableChatParticipant) bool {
	if me == nil || me.State != mtproto.ChatMemberStateNormal {
		return false
	}
	return me.CanAdminDeleteMessages()
}

// rejectForeignGroupRevoke stops messages.deleteMessages revoke from removing
// another member's basic-group message. Own messages stay allowed. Private
// chats are unchanged. A missing chat lookup is returned as that error.
func (c *MsgCore) rejectForeignGroupRevoke(userID int64, ids []int32) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessagesDAO == nil || len(ids) == 0 {
		return nil
	}
	rows, err := c.svcCtx.Dao.MessagesDAO.SelectByMessageIdList(c.ctx, userID, ids)
	if err != nil {
		c.Logger.Errorf("messages.deleteMessages - load: %v", err)
		return err
	}
	chats := foreignGroupIDs(userID, rows)
	if len(chats) == 0 {
		return nil
	}
	if c.svcCtx.Dao.ChatClient == nil {
		return mtproto.ErrMessageDeleteForbidden
	}
	for _, chatID := range chats {
		chat2, err := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chatpb.TLChatGetMutableChat{ChatId: chatID})
		if err != nil {
			c.Logger.Errorf("messages.deleteMessages - chat %d: %v", chatID, err)
			return err
		}
		if chat2 == nil {
			return mtproto.ErrMessageDeleteForbidden
		}
		me, _ := chat2.GetImmutableChatParticipant(userID)
		if !mayDeleteOthersInChat(me) {
			return mtproto.ErrMessageDeleteForbidden
		}
	}
	return nil
}
