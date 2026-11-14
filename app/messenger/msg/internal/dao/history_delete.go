package dao

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/deletehistory"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// ListDialogDeletePage reads one deleteHistory page.
// It does not load every message in the dialog. message_data is included
// only when withBody is set, which the just_clear anchor uses.
func (d *Dao) ListDialogDeletePage(ctx context.Context, userID int64, peer *mtproto.PeerUtil, sel deletehistory.Select, withBody bool) ([]dataobject.MessagesDO, error) {
	if d == nil || d.MessagesDAO == nil || peer == nil || sel.Empty || sel.Limit <= 0 {
		return nil, nil
	}
	did := mtproto.MakeDialogId(userID, peer.PeerType, peer.PeerId)
	cols := "user_message_box_id, dialog_message_id, date2"
	if withBody {
		cols += ", message_data"
	}
	query := "select " + cols + " from " + d.MessagesDAO.CalcTableName(userID) +
		" where user_id = ? and dialog_id1 = ? and dialog_id2 = ? and deleted = 0"
	args := []any{userID, did.A, did.B}
	if sel.BeforeIDSet {
		query += " and user_message_box_id < ?"
		args = append(args, sel.BeforeID)
	}
	if sel.AfterDate > 0 {
		query += " and date2 >= ?"
		args = append(args, sel.AfterDate)
	}
	if sel.BeforeDate > 0 {
		query += " and date2 < ?"
		args = append(args, sel.BeforeDate)
	}
	query += " order by user_message_box_id desc limit ?"
	args = append(args, sel.Limit)

	var rows []dataobject.MessagesDO
	if err := d.DB.QueryRowsPartial(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

// PeerBoxIncludingDeleted is the other user's box id of callerBox.
// The caller's row may already be deleted = 1. SelectPeerUserMessage
// requires deleted = 0 on that row, so a later revoke could not see it.
func (d *Dao) PeerBoxIncludingDeleted(ctx context.Context, peerUser, caller int64, callerBox int32) (int32, error) {
	if d == nil || d.MessagesDAO == nil || d.DB == nil || peerUser == 0 || caller == 0 || callerBox <= 0 {
		return 0, nil
	}
	query := "select user_message_box_id from " + d.MessagesDAO.CalcTableName(peerUser) +
		" where user_id = ? and deleted = 0 and dialog_message_id = (select dialog_message_id from " +
		d.MessagesDAO.CalcTableName(caller) + " where user_id = ? and user_message_box_id <= ? and dialog_message_id <> 0 order by user_message_box_id desc limit 1) limit 1"
	var row dataobject.MessagesDO
	if err := d.DB.QueryRowPartial(ctx, &row, query, peerUser, caller, callerBox); err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return row.UserMessageBoxId, nil
}

// CallerMessageDate is date2 of the newest caller row at or before callerBox,
// including a row the caller already marked deleted. It bounds a revoke when
// the peer's copy of that message cannot be found.
func (d *Dao) CallerMessageDate(ctx context.Context, caller int64, callerBox int32) (int64, error) {
	if d == nil || d.MessagesDAO == nil || d.DB == nil || caller == 0 || callerBox <= 0 {
		return 0, nil
	}
	query := "select date2 from " + d.MessagesDAO.CalcTableName(caller) +
		" where user_id = ? and user_message_box_id <= ? order by user_message_box_id desc limit 1"
	var row dataobject.MessagesDO
	if err := d.DB.QueryRowPartial(ctx, &row, query, caller, callerBox); err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return row.Date2, nil
}

// LiveMessageBoxIDs returns the ids from ids that are still deleted = 0.
func (d *Dao) LiveMessageBoxIDs(ctx context.Context, userID int64, ids []int32) (map[int32]struct{}, error) {
	live := map[int32]struct{}{}
	if d == nil || d.MessagesDAO == nil || len(ids) == 0 {
		return live, nil
	}
	query := "select user_message_box_id from " + d.MessagesDAO.CalcTableName(userID) +
		" where user_id = ? and deleted = 0 and user_message_box_id in (" + int32List(ids) + ")"
	var rows []dataobject.MessagesDO
	if err := d.DB.QueryRowsPartial(ctx, &rows, query, userID); err != nil {
		return nil, err
	}
	for i := range rows {
		live[rows[i].UserMessageBoxId] = struct{}{}
	}
	return live, nil
}

// SetDialogTopMessage points the dialog at the newest message that is still
// live. It changes only top_message. Unread counts and the draft stay.
func (d *Dao) SetDialogTopMessage(ctx context.Context, userID int64, peer *mtproto.PeerUtil) error {
	if d == nil || d.MessagesDAO == nil || d.DB == nil || peer == nil {
		return nil
	}
	did := mtproto.MakeDialogId(userID, peer.PeerType, peer.PeerId)
	top, err := d.MessagesDAO.SelectDialogLastMessageId(ctx, userID, did.A, did.B)
	if err != nil {
		return err
	}
	_, err = d.DB.Exec(ctx,
		"update dialogs set top_message = ? where user_id = ? and peer_type = ? and peer_id = ?",
		top, userID, peer.PeerType, peer.PeerId)
	if err != nil {
		return err
	}
	d.invalidateDialogCache(ctx, dialog.GetDialogCacheKeyByPeer(userID, peer.PeerType, peer.PeerId))
	return nil
}

func int32List(ids []int32) string {
	var b strings.Builder
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatInt(int64(id), 10))
	}
	return b.String()
}
