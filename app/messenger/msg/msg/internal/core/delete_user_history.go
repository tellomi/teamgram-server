package core

import (
	"math"

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
	// A had already deleted these messages for themselves, so they are not
	// in this page and the dialog-id revoke above does not see them.
	// revoke still removes the whole chat for the other participant.
	if in.GetRevoke() && in.UserId != in.PeerId && in.PeerType == mtproto.PEER_USER && !q.JustClear && !q.HasDates() && c.svcCtx.Dao.InboxClient != nil {
		mapped := int32(0)
		if in.GetMaxId() > 0 && in.GetMaxId() < math.MaxInt32 {
			var mapErr error
			mapped, mapErr = c.svcCtx.Dao.PeerBoxIncludingDeleted(c.ctx, in.PeerId, in.UserId, in.GetMaxId())
			if mapErr != nil {
				c.Logger.Errorf("msg.deleteHistory - peer box: %v", mapErr)
				mapped = 0
			}
		}
		peerMax := deletehistory.RevokePeerCap(in.GetMaxId(), mapped, len(ids) > 0)
		if _, err = c.svcCtx.Dao.InboxClient.InboxDeleteUserHistoryToInbox(c.ctx, &inbox.TLInboxDeleteUserHistoryToInbox{
			FromId:     in.UserId,
			PeerUserId: in.PeerId,
			JustClear:  false,
			MaxId:      peerMax,
		}); err != nil {
			c.Logger.Errorf("msg.deleteHistory - revoke history: %v", err)
			return nil, err
		}
	}
	if len(ids) > 0 {
		if _, err = c.svcCtx.Dao.DeleteByMessageIdList(c.ctx, in.UserId, ids); err != nil {
			c.Logger.Errorf("msg.deleteHistory - error: %v", err)
			return nil, err
		}
	}
	live, confirmErr := c.svcCtx.Dao.LiveMessageBoxIDs(c.ctx, in.UserId, ids)
	if confirmErr != nil {
		// The delete already committed. Announcing the requested ids is what
		// lets the next getDifference drop them. Returning here would leave
		// a deleted batch with no pts update.
		c.Logger.Errorf("msg.deleteHistory - confirm: %v", confirmErr)
	}
	deleted, failed := deletehistory.SplitDeleted(ids, live, confirmErr)
	offset = deletehistory.OffsetAfterFailure(offset, failed)
	if len(ids) > 0 && len(deleted) == 0 {
		err = mtproto.ErrInternalServerError
		c.Logger.Errorf("msg.deleteHistory - deleted none of %d messages: %v", len(ids), err)
		return nil, err
	}

	fullClear := !q.HasDates()
	clearAnchor := fullClear && q.JustClear && offset == 0 && anchor != nil && !isHistoryClear(anchor)
	if clearAnchor {
		cleared := historyClearMessage(in.UserId, peer, anchor)
		// Save the body before any pts is reserved. The outbox edit helper
		// also takes a pts, which would leave a gap with no stored update.
		if err = c.svcCtx.Dao.SaveEditedMessage(c.ctx, in.UserId, in.PeerType, in.PeerId, cleared); err != nil {
			c.Logger.Errorf("msg.deleteHistory - history clear: %v", err)
			clearAnchor = false
			if offset == 0 && anchor.GetId() > 0 {
				offset = anchor.GetId()
			}
		} else {
			anchor = cleared
		}
	}

	pts, updates := c.historyDeleteUpdates(in, peer, deleted, anchor, clearAnchor, fullClear && (len(deleted) > 0 || clearAnchor))
	if len(deleted) > 0 || clearAnchor {
		if topErr := c.svcCtx.Dao.SetDialogTopMessage(c.ctx, in.UserId, peer); topErr != nil {
			// pts for this batch are already stored. Failing the RPC would
			// make the client retry ids that are no longer visible.
			c.Logger.Errorf("msg.deleteHistory - top message: %v", topErr)
		}
	}
	if len(updates) > 0 {
		if _, syncErr := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
			UserId:        in.UserId,
			PermAuthKeyId: in.AuthKeyId,
			Updates:       mtproto.MakeUpdatesByUpdates(updates...),
		}); syncErr != nil {
			c.Logger.Errorf("msg.deleteHistory - sync: %v", syncErr)
		}
	}
	if clearAnchor && in.GetRevoke() && in.UserId != in.PeerId && in.PeerType == mtproto.PEER_USER {
		// max_id is the caller's box id. The peer clear compares it with
		// the peer's own box ids, which are a different sequence. Pass the
		// peer's id of this anchor. 0 and MaxInt32 stay uncapped.
		_, _ = c.svcCtx.Dao.InboxClient.InboxDeleteUserHistoryToInbox(c.ctx, &inbox.TLInboxDeleteUserHistoryToInbox{
			FromId:     in.UserId,
			PeerUserId: in.PeerId,
			JustClear:  true,
			MaxId:      c.peerRevokeMaxID(in, anchorID),
			MinDate:    q.MinDate,
			MaxDate:    q.MaxDate,
			MinDateSet: q.MinSet,
			MaxDateSet: q.MaxSet,
		})
	}

	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      pts,
		PtsCount: historyPtsCount(updates),
		Offset:   offset,
	}).To_Messages_AffectedHistory(), nil
}

// peerRevokeMaxID is the other participant's box id of anchorID.
// anchorID is the newest caller message with id <= max_id, which just_clear
// turns into historyClear and therefore does not revoke by dialog_message_id.
// Leaving the caller's box id in place drops every peer row above that
// number, including the peer's own last message, and no historyClear is sent.
func (c *MsgCore) peerRevokeMaxID(in *msg.TLMsgDeleteHistory, anchorID int32) int32 {
	callerMax := in.GetMaxId()
	if callerMax <= 0 || callerMax >= math.MaxInt32 || anchorID <= 0 {
		return callerMax
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessagesDAO == nil {
		return callerMax
	}
	mapped, err := c.svcCtx.Dao.MessagesDAO.SelectPeerUserMessage(c.ctx, in.PeerId, in.UserId, anchorID)
	if err != nil {
		c.Logger.Errorf("msg.deleteHistory - peer max_id: %v", err)
		return deletehistory.PeerRevokeMaxID(callerMax, 0)
	}
	peerBox := int32(0)
	if mapped != nil {
		peerBox = mapped.UserMessageBoxId
	}
	if peerBox <= 0 {
		c.Logger.Errorf("msg.deleteHistory - peer box for caller message %d not found", anchorID)
	}
	return deletehistory.PeerRevokeMaxID(callerMax, peerBox)
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
		cursor++
		push(mtproto.MakeTLUpdateEditMessage(&mtproto.Update{
			Message_MESSAGE: anchor,
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
