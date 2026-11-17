package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// ptsNext overrides the id generator in tests. Production leaves it nil
// and uses IDGen. The value is one new pts from the Redis counter. It is
// not a hole in user_pts_updates, and it is not reused.
func (d *Dao) nextUserPts(ctx context.Context, userID int64) int32 {
	if d != nil && d.ptsNext != nil {
		return d.ptsNext(ctx, userID)
	}
	if d == nil {
		return 0
	}
	return d.IDGenClient2.NextPtsId(ctx, userID)
}

type inboxApplyRow struct {
	UserID        int64 `db:"user_id"`
	BoxID         int32 `db:"box_id"`
	PeerType      int32 `db:"peer_type"`
	PeerID        int64 `db:"peer_id"`
	UnreadDelta   int32 `db:"unread_delta"`
	MentionsDelta int32 `db:"mentions_delta"`
	Pts           int32 `db:"pts"`
	PtsCount      int32 `db:"pts_count"`
	Date2         int64 `db:"date2"`
	Applied       bool  `db:"applied"`
}

// rememberInboxRandom records that this recipient already has the random id.
// The previous container allocates a new box id and inserts again. The
// messages trigger rejects that insert when this row is already committed.
// Two transactions can both pass that trigger before either commits. A
// duplicate key then belongs to the other box. ON DUPLICATE KEY UPDATE would
// let the second message commit. Returning the error rolls that transaction
// back, so the second message and its pts row are not stored. The same box
// id is the idempotent case. random_id 0 is not recorded. The table and the
// trigger are unchanged.
func rememberInboxRandom(tx *sqlx.Tx, userID, senderID, randomID int64, boxID int32) error {
	if tx == nil || randomID == 0 || userID == 0 || boxID == 0 {
		return nil
	}
	_, err := tx.Exec(`INSERT INTO inbox_random (user_id, random_id, sender_user_id, box_id) VALUES (?,?,?,?)`,
		userID, randomID, senderID, boxID)
	if err == nil || !mysqlDup(err) {
		return err
	}
	var row struct {
		Box int32 `db:"box_id"`
	}
	if qerr := tx.QueryRow(&row, `SELECT box_id FROM inbox_random WHERE user_id = ? AND random_id = ? AND sender_user_id = ?`,
		userID, randomID, senderID); qerr != nil {
		return err
	}
	if row.Box == boxID {
		return nil
	}
	return fmt.Errorf("inbox random %d already stored as box %d", randomID, row.Box)
}

// inboxBeforeRememberRandom pauses inside sendMessageToInbox after the
// message insert and before inbox_random. Production leaves it nil.
var inboxBeforeRememberRandom func()

func mysqlDup(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1062
}

// applyInboxInTx writes the dialog unread, the pts row, and inbox_apply
// in the caller's transaction. The caller inserts the message in that same
// transaction. applied is stored as 1. A crash rolls all of it back, so an
// older process never sees a message whose unread was left for a later
// pass. The pts value is whatever NextPtsId just returned. A gap left by a
// rolled-back NextPtsId is not filled: that number may belong to a read,
// edit, pin, or archive call that has not written its row yet.
func (d *Dao) applyInboxInTx(ctx context.Context, tx *sqlx.Tx, row inboxApplyRow, message *mtproto.Message) (int32, *AnsweredCallRead, error) {
	if d == nil || tx == nil {
		return 0, nil, fmt.Errorf("inbox apply is missing a transaction")
	}
	pts := d.nextUserPts(ctx, row.UserID)
	if pts <= 0 {
		return 0, nil, fmt.Errorf("pts allocation failed for user %d", row.UserID)
	}
	row.Pts = pts
	if row.PtsCount <= 0 {
		row.PtsCount = 1
	}
	if _, err := tx.Exec(`INSERT INTO dialogs (user_id, peer_type, peer_id, peer_dialog_id, top_message, unread_count, unread_mentions_count, draft_message_data, date2)
VALUES (?,?,?,?,?,?,?, 'null', ?)
ON DUPLICATE KEY UPDATE
  top_message = GREATEST(top_message, VALUES(top_message)),
  unread_count = unread_count + IF(read_inbox_max_id >= VALUES(top_message), 0, VALUES(unread_count)),
  unread_mentions_count = unread_mentions_count + IF(read_inbox_max_id >= VALUES(top_message), 0, VALUES(unread_mentions_count)),
  date2 = GREATEST(date2, VALUES(date2))`,
		row.UserID, row.PeerType, row.PeerID, mtproto.MakePeerDialogId(row.PeerType, row.PeerID),
		row.BoxID, row.UnreadDelta, row.MentionsDelta, row.Date2); err != nil {
		return 0, nil, err
	}
	raw, err := jsonx.Marshal(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Message_MESSAGE: message,
		Pts_INT32:       pts,
		PtsCount:        row.PtsCount,
	}).To_Update())
	if err != nil {
		return 0, nil, err
	}
	if _, err = tx.Exec(`INSERT INTO user_pts_updates (user_id, pts, pts_count, update_type, update_data, date2) VALUES (?,?,?,?,?,?)`,
		row.UserID, pts, row.PtsCount, mtproto.PTS_UPDATE_NEW_MESSAGE, string(raw), row.Date2); err != nil {
		return 0, nil, err
	}
	if _, err = tx.Exec(`INSERT INTO inbox_apply (user_id, box_id, peer_type, peer_id, unread_delta, mentions_delta, pts, pts_count, date2, applied) VALUES (?,?,?,?,?,?,?,?,?,1)`,
		row.UserID, row.BoxID, row.PeerType, row.PeerID, row.UnreadDelta, row.MentionsDelta, pts, row.PtsCount, row.Date2); err != nil {
		return 0, nil, err
	}
	var answered *AnsweredCallRead
	if row.UnreadDelta == 0 && row.PeerType == mtproto.PEER_USER {
		table := "messages"
		if d.MessagesDAO != nil {
			table = d.MessagesDAO.CalcTableName(row.UserID)
		}
		res, err := tx.Exec(answeredCallCursorSQL(table),
			row.BoxID, row.BoxID, row.BoxID,
			row.UserID, mtproto.PEER_USER, row.PeerID,
			row.BoxID,
			row.UserID, mtproto.PEER_USER, row.PeerID, row.UserID, row.BoxID,
		)
		if err != nil {
			return 0, nil, err
		}
		moved, err := res.RowsAffected()
		if err != nil {
			return 0, nil, err
		}
		if moved > 0 {
			var unread struct {
				N int32 `db:"unread_count"`
			}
			if err = tx.QueryRow(&unread, `SELECT unread_count FROM dialogs WHERE user_id = ? AND peer_type = ? AND peer_id = ?`, row.UserID, row.PeerType, row.PeerID); err != nil {
				return 0, nil, err
			}
			answered = &AnsweredCallRead{MaxID: row.BoxID, StillUnread: unread.N}
		}
	}
	return pts, answered, nil
}

// invalidateDialogKeys drops the dialog cache after the unread transaction
// commits. A zero cache means this process has no Redis client; the SQL
// write is already durable. An in-flight getDialogs can put the old object
// back, so the second delete uses the same delay as the other dialog writes.
func (d *Dao) invalidateDialogKeys(ctx context.Context, keys ...string) {
	if d == nil || len(keys) == 0 || d.CachedConn == (sqlc.CachedConn{}) {
		return
	}
	_, _, _ = d.ExecInvalidate(ctx, func(context.Context, *sqlx.DB) (int64, int64, error) {
		return 0, 0, nil
	}, keys...)
}

func dialogCacheKey(userID int64, peerType int32, peerID int64) string {
	return dialog.GetDialogCacheKey(userID, mtproto.MakePeerDialogId(peerType, peerID))
}
