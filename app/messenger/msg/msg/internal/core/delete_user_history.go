package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/deletehistory"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/zeromicro/go-zero/core/jsonx"
)

func (c *MsgCore) deleteUserHistory(in *msg.TLMsgDeleteHistory) (*mtproto.Messages_AffectedHistory, error) {
	q := deletehistory.Query{MaxID: in.GetMaxId(), JustClear: in.GetJustClear()}
	if dates, ok := deletehistory.RangeFrom(c.ctx); ok {
		q.MinSet, q.MinDate = dates.MinSet, dates.MinDate
		q.MaxSet, q.MaxDate = dates.MaxSet, dates.MaxDate
	}
	if err := deletehistory.Validate(q); err != nil {
		return nil, err
	}

	peer := mtproto.MakePeerUtil(in.PeerType, in.PeerId)
	var anchorID int32
	var anchor *mtproto.Message
	// just_clear without a date window keeps the newest message and turns it
	// into messageActionHistoryClear once every older message is gone.
	// A date window deletes the messages it names, including the newest.
	if q.JustClear && !q.HasDates() {
		top, err := c.svcCtx.Dao.ListDialogDeletePage(c.ctx, in.UserId, peer, deletehistory.PageSelect(deletehistory.Query{MaxID: q.MaxID}, 0, 1), true)
		if err != nil {
			c.Logger.Errorf("msg.deleteHistory - error: %v", err)
			return nil, err
		}
		if len(top) > 0 {
			anchorID = top[0].UserMessageBoxId
			anchor = messageFromDO(&top[0])
			if anchor == nil {
				anchor = &mtproto.Message{Id: anchorID, Date: int32(top[0].Date2)}
			}
		}
	}

	sel := deletehistory.PageSelect(q, anchorID, deletehistory.Batch+1)
	var rows []dataobject.MessagesDO
	var err error
	if !sel.Empty {
		rows, err = c.svcCtx.Dao.ListDialogDeletePage(c.ctx, in.UserId, peer, sel, false)
		if err != nil {
			c.Logger.Errorf("msg.deleteHistory - error: %v", err)
			return nil, err
		}
	}
	pageRows := make([]deletehistory.Row, 0, len(rows))
	for i := range rows {
		pageRows = append(pageRows, deletehistory.Row{
			ID:     rows[i].UserMessageBoxId,
			Date:   rows[i].Date2,
			DataID: rows[i].DialogMessageId,
		})
	}
	page, offset := deletehistory.Page(pageRows, deletehistory.Batch)

	ids := make([]int32, 0, len(page))
	dataIDs := make([]int64, 0, len(page))
	for _, row := range page {
		ids = append(ids, row.ID)
		if row.DataID != 0 {
			dataIDs = append(dataIDs, row.DataID)
		}
	}
	// Publish the peer delete before the local delete. A publish failure
	// leaves the caller's messages in place, so the client can retry this
	// max_id. A success that then fails locally is retried the same way;
	// deleting the peer copy again is a no-op.
	if len(ids) > 0 && in.GetRevoke() && in.UserId != in.PeerId && len(dataIDs) > 0 {
		if _, err = c.svcCtx.Dao.InboxClient.InboxDeleteMessagesToInbox(c.ctx, &inbox.TLInboxDeleteMessagesToInbox{
			FromId:   in.UserId,
			PeerType: in.PeerType,
			PeerId:   in.PeerId,
			Id:       dataIDs,
		}); err != nil {
			c.Logger.Errorf("msg.deleteHistory - revoke: %v", err)
			return nil, err
		}
	}
	if len(ids) > 0 {
		if _, err = c.svcCtx.Dao.DeleteByMessageIdList(c.ctx, in.UserId, ids); err != nil {
			c.Logger.Errorf("msg.deleteHistory - error: %v", err)
			return nil, err
		}
	}
	live, err := c.svcCtx.Dao.LiveMessageBoxIDs(c.ctx, in.UserId, ids)
	if err != nil {
		c.Logger.Errorf("msg.deleteHistory - error: %v", err)
		return nil, err
	}
	deleted := make([]int32, 0, len(ids))
	failed := make([]int32, 0)
	for _, id := range ids {
		if _, still := live[id]; still {
			failed = append(failed, id)
			continue
		}
		deleted = append(deleted, id)
	}
	offset = deletehistory.OffsetAfterFailure(offset, failed)
	if len(ids) > 0 && len(deleted) == 0 {
		err = mtproto.ErrInternalServerError
		c.Logger.Errorf("msg.deleteHistory - deleted none of %d messages: %v", len(ids), err)
		return nil, err
	}

	fullClear := !q.HasDates()
	clearAnchor := fullClear && q.JustClear && offset == 0 && anchor != nil && !isHistoryClear(anchor)
	if len(deleted) > 0 || clearAnchor {
		if err = c.svcCtx.Dao.SetDialogTopMessage(c.ctx, in.UserId, peer); err != nil {
			c.Logger.Errorf("msg.deleteHistory - top message: %v", err)
			return nil, err
		}
	}

	pts, updates := c.historyDeleteUpdates(in, peer, deleted, anchor, clearAnchor, fullClear && (len(deleted) > 0 || clearAnchor))
	if len(updates) > 0 {
		c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
			UserId:        in.UserId,
			PermAuthKeyId: in.AuthKeyId,
			Updates:       mtproto.MakeUpdatesByUpdates(updates...),
		})
	}
	if clearAnchor && in.GetRevoke() && in.UserId != in.PeerId && in.PeerType == mtproto.PEER_USER {
		// The peer's copies of the deleted messages are already gone. This
		// turns the peer's remaining top message into history clear.
		_, _ = c.svcCtx.Dao.InboxClient.InboxDeleteUserHistoryToInbox(c.ctx, &inbox.TLInboxDeleteUserHistoryToInbox{
			FromId:     in.UserId,
			PeerUserId: in.PeerId,
			JustClear:  true,
			MaxId:      0,
		})
	}

	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      pts,
		PtsCount: historyPtsCount(updates),
		Offset:   offset,
	}).To_Messages_AffectedHistory(), nil
}

func (c *MsgCore) historyDeleteUpdates(in *msg.TLMsgDeleteHistory, peer *mtproto.PeerUtil, deleted []int32, anchor *mtproto.Message, clearAnchor, markRead bool) (int32, []*mtproto.Update) {
	n := len(deleted)
	if markRead {
		n++
	}
	if clearAnchor {
		n++
	}
	if n == 0 {
		return c.svcCtx.Dao.IDGenClient2.CurrentPtsId(c.ctx, in.UserId), nil
	}
	// NextNPtsId returns the pts after these n events. They occupy
	// (end-n, end] in the order delete, read, edit.
	end := c.svcCtx.Dao.IDGenClient2.NextNPtsId(c.ctx, in.UserId, n)
	cursor := end - int32(n)
	var updates []*mtproto.Update
	push := func(upd *mtproto.Update) {
		c.svcCtx.Dao.AddToPtsQueue(c.ctx, in.UserId, upd.GetPts_INT32(), upd.GetPtsCount(), upd)
		updates = append(updates, upd)
	}
	if len(deleted) > 0 {
		cursor += int32(len(deleted))
		push(mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{
			Messages:  deleted,
			Pts_INT32: cursor,
			PtsCount:  int32(len(deleted)),
		}).To_Update())
	}
	if markRead {
		maxID := int32(0)
		if len(deleted) > 0 {
			maxID = deleted[0]
		}
		if anchor != nil && anchor.GetId() > maxID {
			maxID = anchor.GetId()
		}
		cursor++
		push(mtproto.MakeTLUpdateReadHistoryInbox(&mtproto.Update{
			Peer_PEER: peer.ToPeer(),
			MaxId:     maxID,
			Pts_INT32: cursor,
			PtsCount:  1,
		}).To_Update())
	}
	if clearAnchor {
		cleared := historyClearMessage(in.UserId, peer, anchor)
		_, _ = c.svcCtx.Dao.EditUserOutboxMessage(c.ctx, in.UserId, in.PeerId, cleared)
		cursor++
		push(mtproto.MakeTLUpdateEditMessage(&mtproto.Update{
			Message_MESSAGE: cleared,
			Pts_INT32:       cursor,
			PtsCount:        1,
		}).To_Update())
	}
	return end, updates
}

func historyPtsCount(updates []*mtproto.Update) int32 {
	var n int32
	for _, upd := range updates {
		n += upd.GetPtsCount()
	}
	return n
}

func historyClearMessage(userID int64, peer *mtproto.PeerUtil, anchor *mtproto.Message) *mtproto.Message {
	return mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		Id:     anchor.GetId(),
		FromId: mtproto.MakePeerUser(userID),
		PeerId: peer.ToPeer(),
		Date:   anchor.GetDate(),
		Action: mtproto.MakeTLMessageActionHistoryClear(nil).To_MessageAction(),
	}).To_Message()
}

func messageFromDO(row *dataobject.MessagesDO) *mtproto.Message {
	if row == nil || row.MessageData == "" {
		return nil
	}
	m := new(mtproto.Message)
	if err := jsonx.UnmarshalFromString(row.MessageData, m); err != nil {
		return nil
	}
	return m.FixData()
}

func isHistoryClear(m *mtproto.Message) bool {
	return m.GetPredicateName() == mtproto.Predicate_messageService &&
		m.GetAction().GetPredicateName() == mtproto.Predicate_messageActionHistoryClear
}
