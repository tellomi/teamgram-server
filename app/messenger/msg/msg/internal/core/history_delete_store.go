package core

import (
	"context"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/zeromicro/go-zero/core/logx"
)

type msgHistoryStore struct {
	c *MsgCore
}

func (s msgHistoryStore) List(ctx context.Context, userID int64, peer *mtproto.PeerUtil, maxID, minDate, maxDate, limit int32) ([]historyRow, int32, error) {
	did := mtproto.MakeDialogId(userID, peer.PeerType, peer.PeerId)
	total, err := s.c.svcCtx.Dao.CountForHistoryDelete(ctx, userID, did.A, did.B, maxID, minDate, maxDate)
	if err != nil {
		return nil, 0, err
	}
	got, err := s.c.svcCtx.Dao.SelectForHistoryDelete(ctx, userID, did.A, did.B, maxID, minDate, maxDate, limit)
	if err != nil {
		return nil, 0, err
	}
	rows := make([]historyRow, 0, len(got))
	for _, do := range got {
		rows = append(rows, historyRow{
			BoxID:  do.UserMessageBoxId,
			DataID: do.DialogMessageId,
			Date:   int32(do.Date2),
		})
	}
	return rows, total, nil
}

func (s msgHistoryStore) TopID(ctx context.Context, userID int64, peer *mtproto.PeerUtil) (int32, error) {
	did := mtproto.MakeDialogId(userID, peer.PeerType, peer.PeerId)
	got, err := s.c.svcCtx.Dao.SelectDialogLastMessageList(ctx, userID, did.A, did.B, 1)
	if err != nil || len(got) == 0 {
		return 0, err
	}
	return got[0].UserMessageBoxId, nil
}

func (s msgHistoryStore) Revoke(ctx context.Context, fromID int64, peer *mtproto.PeerUtil, dataIDs []int64) error {
	if s.c.svcCtx.Dao.InboxClient == nil || len(dataIDs) == 0 {
		return nil
	}
	_, err := s.c.svcCtx.Dao.InboxClient.InboxDeleteMessagesToInbox(ctx, &inbox.TLInboxDeleteMessagesToInbox{
		FromId:   fromID,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
		Id:       dataIDs,
	})
	return err
}

func (s msgHistoryStore) Delete(ctx context.Context, userID int64, ids []int32) error {
	_, err := s.c.svcCtx.Dao.DeleteByMessageIdList(ctx, userID, ids)
	return err
}

func (s msgHistoryStore) Rewrite(ctx context.Context, userID int64, peer *mtproto.PeerUtil, row historyRow) (*mtproto.Message, error) {
	msg := historyClearMessage(userID, peer, row)
	raw, err := historyClearJSON(msg)
	if err != nil {
		return nil, err
	}
	if _, err = s.c.svcCtx.Dao.UpdateEditMessage(ctx, raw, "", userID, row.BoxID); err != nil {
		return nil, err
	}
	return msg, nil
}

func (s msgHistoryStore) Touch(ctx context.Context, userID int64, peer *mtproto.PeerUtil) error {
	did := mtproto.MakeDialogId(userID, peer.PeerType, peer.PeerId)
	got, err := s.c.svcCtx.Dao.SelectDialogLastMessageList(ctx, userID, did.A, did.B, 1)
	if err != nil {
		return err
	}
	top := int32(0)
	date := time.Now().Unix()
	if len(got) > 0 {
		top = got[0].UserMessageBoxId
		if got[0].Date2 > 0 {
			date = got[0].Date2
		}
	}
	_, _, err = s.c.svcCtx.Dao.ExecInvalidate(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			_, err2 := s.c.svcCtx.Dao.DialogsDAO.UpdateCustomMap(ctx, map[string]interface{}{
				"top_message": top,
				"date2":       date,
			}, userID, peer.PeerType, peer.PeerId)
			return 0, 0, err2
		},
		dialog.GetDialogCacheKeyByPeer(userID, peer.PeerType, peer.PeerId))
	return err
}

func (s msgHistoryStore) CurrentPts(ctx context.Context, userID int64) int32 {
	return s.c.svcCtx.Dao.IDGenClient2.CurrentPtsId(ctx, userID)
}

func (s msgHistoryStore) AllocPts(ctx context.Context, userID int64, n int32) (int32, error) {
	if n <= 0 {
		return s.CurrentPts(ctx, userID), nil
	}
	return s.c.svcCtx.Dao.IDGenClient2.NextNPtsId(ctx, userID, int(n)), nil
}

func (s msgHistoryStore) Persist(ctx context.Context, userID int64, pts, count int32, upd *mtproto.Update) error {
	_, err := s.c.svcCtx.Dao.AddToPtsQueueE(ctx, userID, pts, count, upd)
	return err
}

func (s msgHistoryStore) Push(ctx context.Context, userID, authKeyID int64, upds []*mtproto.Update) error {
	if s.c.svcCtx.Dao.SyncClient == nil || len(upds) == 0 {
		return nil
	}
	_, err := s.c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        userID,
		PermAuthKeyId: authKeyID,
		Updates:       mtproto.MakeUpdatesByUpdates(upds...),
	})
	// The pts rows are already stored. A push failure must not look like the
	// delete failed, or the client retries and the other device still catches
	// up through getDifference.
	if err != nil {
		logx.WithContext(ctx).Errorf("msg.deleteHistory - push updateDeleteMessages: %v", err)
	}
	return nil
}
