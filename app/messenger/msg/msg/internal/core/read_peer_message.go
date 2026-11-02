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
// id. A basic-group read often names the sender's id. When that id is missing
// from this chat (it may still belong to an older dialog), map it through
// dialog_message_id from another participant. The returned id is the one
// stored on the reader's row for this chat.
func (c *MsgCore) messageForRead(ctx context.Context, userID int64, peerType int32, peerID int64, maxID int32) (*dataobject.MessagesDO, int32, error) {
	row, err := c.svcCtx.Dao.MessagesDAO.SelectByMessageId(ctx, userID, maxID)
	if err != nil || peerType != mtproto.PEER_CHAT || c.svcCtx.Dao.ChatClient == nil {
		return row, maxID, err
	}
	// Box ids are per user, across every dialog. A row with this id that
	// belongs to another peer is not the group message the client named.
	if sameReadPeer(row, peerType, peerID) {
		return row, maxID, nil
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
		if !sameReadPeer(mapped, peerType, peerID) {
			continue
		}
		return mapped, mapped.UserMessageBoxId, nil
	}
	return nil, maxID, nil
}

func sameReadPeer(row *dataobject.MessagesDO, peerType int32, peerID int64) bool {
	return row != nil && row.PeerType == peerType && row.PeerId == peerID
}
