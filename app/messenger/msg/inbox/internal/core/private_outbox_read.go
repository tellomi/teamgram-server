package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/readoutbox"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// privateReadOutboxUpdate is the updateReadHistoryOutbox pushed to the
// sender when reader has seen a strictly newer outgoing message.
// next is the sender's own box id. current is the sender's stored
// read_outbox_max_id. A nil update means the cursor must stay put.
func privateReadOutboxUpdate(reader int64, current, next, pts int32) *mtproto.Update {
	if reader == 0 || !readoutbox.Advance(current, next) {
		return nil
	}
	return mtproto.MakeTLUpdateReadHistoryOutbox(&mtproto.Update{
		Peer_PEER: mtproto.MakePeerUser(reader),
		MaxId:     next,
		Pts_INT32: pts,
		PtsCount:  1,
	}).To_Update()
}

// pushUserOutboxRead moves owner's private-chat read_outbox_max_id to boxID
// and pushes updateReadHistoryOutbox to owner. An older or equal box id does
// neither, so a late read cannot paint checkmarks backwards.
func (c *InboxCore) pushUserOutboxRead(owner, reader int64, boxID int32) {
	if !readoutbox.NotifySender(owner, reader) || boxID <= 0 {
		return
	}
	rows, err := c.svcCtx.Dao.DialogsDAO.SelectPeerDialogList(
		c.ctx,
		owner,
		[]int64{mtproto.MakePeerDialogId(mtproto.PEER_USER, reader)},
	)
	if err != nil {
		c.Logger.Errorf("inbox private outbox read - error: %v", err)
		return
	}
	current := int32(0)
	for i := range rows {
		if rows[i].ReadOutboxMaxId > current {
			current = rows[i].ReadOutboxMaxId
		}
	}
	if !readoutbox.Advance(current, boxID) {
		c.Logger.Infof("inbox private outbox read: keep %d, ignore %d owner=%d reader=%d", current, boxID, owner, reader)
		return
	}

	_, _ = c.svcCtx.Dao.DialogClient.DialogInsertOrUpdateDialog(
		c.ctx,
		&dialog.TLDialogInsertOrUpdateDialog{
			UserId:          owner,
			PeerType:        mtproto.PEER_USER,
			PeerId:          reader,
			TopMessage:      nil,
			ReadOutboxMaxId: &wrapperspb.Int32Value{Value: boxID},
			ReadInboxMaxId:  nil,
			UnreadCount:     nil,
			UnreadMark:      false,
			PinnedMsgId:     nil,
			Date2:           nil,
		})

	pts := c.svcCtx.Dao.IDGenClient2.NextPtsId(c.ctx, owner)
	updateReadHistoryOutbox := privateReadOutboxUpdate(reader, current, boxID, pts)
	c.persistPtsUpdate(c.ctx, owner, updateReadHistoryOutbox)
	_, _ = c.svcCtx.Dao.SyncClient.SyncPushUpdates(c.ctx, &sync.TLSyncPushUpdates{
		UserId:  owner,
		Updates: mtproto.MakeUpdatesByUpdates(updateReadHistoryOutbox),
	})
}
