package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// messageForRead finds the reader's copy of maxID.
//
// Private chats and basic groups give each participant their own message box
// id. Official clients mark a basic group read with the id they were given,
// which is often the sender's id. When that id is not in the reader's box,
// map it through dialog_message_id from another participant. The returned id
// is the one stored on the reader's row.
func (c *MsgCore) messageForRead(ctx context.Context, userID int64, peerType int32, peerID int64, maxID int32) (*dataobject.MessagesDO, int32, error) {
	row, err := c.svcCtx.Dao.MessagesDAO.SelectByMessageId(ctx, userID, maxID)
	if err != nil || row != nil || peerType != mtproto.PEER_CHAT || c.svcCtx.Dao.ChatClient == nil {
		return row, maxID, err
	}
	ids, err := c.svcCtx.Dao.ChatClient.ChatGetChatParticipantIdList(ctx, &chatpb.TLChatGetChatParticipantIdList{
		ChatId: peerID,
	})
	if err != nil || ids == nil {
		c.Logger.Errorf("msg.readHistoryV2 - chat participants: %v", err)
		return nil, maxID, nil
	}
	for _, uid := range ids.GetDatas() {
		if uid == 0 || uid == userID {
			continue
		}
		mapped, err2 := c.svcCtx.Dao.MessagesDAO.SelectPeerUserMessage(ctx, userID, uid, maxID)
		if err2 != nil || mapped == nil || mapped.UserMessageBoxId <= 0 {
			continue
		}
		return mapped, mapped.UserMessageBoxId, nil
	}
	return nil, maxID, nil
}
