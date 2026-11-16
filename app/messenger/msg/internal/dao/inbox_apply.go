package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// ptsCurrent and ptsNext override the id generator in tests. Production
// leaves them nil and uses IDGen. The values are the user's pts counter,
// which is not the same store as user_pts_updates.
func (d *Dao) currentUserPts(ctx context.Context, userID int64) int32 {
	if d != nil && d.ptsCurrent != nil {
		return d.ptsCurrent(ctx, userID)
	}
	if d == nil {
		return 0
	}
	return d.IDGenClient2.CurrentPtsId(ctx, userID)
}

func (d *Dao) nextUserPts(ctx context.Context, userID int64) int32 {
	if d != nil && d.ptsNext != nil {
		return d.ptsNext(ctx, userID)
	}
	if d == nil {
		return 0
	}
	return d.IDGenClient2.NextPtsId(ctx, userID)
}

// firstUnwrittenPts is the next pts that has no user_pts_updates row.
// NextPtsId moves the counter before the row is inserted. A crash in
// between leaves current one ahead of the stored maximum. Reusing that
// value keeps getDifference contiguous. current == maxWritten means the
// counter is caught up and the caller must take a new value.
func firstUnwrittenPts(maxWritten, current int32) (int32, bool) {
	if current > maxWritten {
		return maxWritten + 1, true
	}
	return 0, false
}

func lockInboxUser(tx *sqlx.Tx, userID int64) error {
	if _, err := tx.Exec(`INSERT INTO inbox_pts_lock (user_id) VALUES (?) ON DUPLICATE KEY UPDATE user_id = user_id`, userID); err != nil {
		return err
	}
	var row struct {
		UserID int64 `db:"user_id"`
	}
	return tx.QueryRow(&row, `SELECT user_id FROM inbox_pts_lock WHERE user_id = ? FOR UPDATE`, userID)
}

func (d *Dao) chooseInboxPts(ctx context.Context, tx *sqlx.Tx, userID int64) (int32, error) {
	if err := lockInboxUser(tx, userID); err != nil {
		return 0, err
	}
	var maxRow struct {
		Pts int32 `db:"pts"`
	}
	if err := tx.QueryRow(&maxRow, `SELECT COALESCE(MAX(pts), 0) AS pts FROM user_pts_updates WHERE user_id = ?`, userID); err != nil {
		return 0, err
	}
	if pts, ok := firstUnwrittenPts(maxRow.Pts, d.currentUserPts(ctx, userID)); ok {
		return pts, nil
	}
	next := d.nextUserPts(ctx, userID)
	if next <= 0 {
		return 0, fmt.Errorf("pts allocation failed for user %d", userID)
	}
	return next, nil
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

func (d *Dao) loadInboxApply(ctx context.Context, userID int64, boxID int32) (*inboxApplyRow, error) {
	if d == nil || d.DB == nil {
		return nil, nil
	}
	row := &inboxApplyRow{}
	err := d.DB.QueryRow(ctx, row, `SELECT user_id, box_id, peer_type, peer_id, unread_delta, mentions_delta, pts, pts_count, date2, applied FROM inbox_apply WHERE user_id = ? AND box_id = ?`, userID, boxID)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if row.BoxID == 0 {
		return nil, nil
	}
	return row, nil
}

// FinishInboxDelivery adds the dialog unread at most once. The message row
// can already be committed. applied flips 0 -> 1 in the same transaction as
// the increment, so a crash retries the increment and a second caller does
// not add it again. A read cursor already at this box, or a deleted row,
// records the attempt and does not add unread or move top_message backward.
func (d *Dao) FinishInboxDelivery(ctx context.Context, userID int64, boxID int32) (*AnsweredCallRead, error) {
	if d == nil || d.DB == nil || userID == 0 || boxID == 0 {
		return nil, nil
	}
	var answered *AnsweredCallRead
	err := d.DB.Transact(ctx, func(tx *sqlx.Tx) error {
		res, err := tx.Exec(`UPDATE inbox_apply SET applied = 1 WHERE user_id = ? AND box_id = ? AND applied = 0`, userID, boxID)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return nil
		}
		row := &inboxApplyRow{}
		if err = tx.QueryRow(row, `SELECT user_id, box_id, peer_type, peer_id, unread_delta, mentions_delta, pts, pts_count, date2, applied FROM inbox_apply WHERE user_id = ? AND box_id = ?`, userID, boxID); err != nil {
			return err
		}
		var msg struct {
			Deleted bool   `db:"deleted"`
			Message string `db:"message"`
		}
		table := "messages"
		if d.MessagesDAO != nil {
			table = d.MessagesDAO.CalcTableName(userID)
		}
		if err = tx.QueryRow(&msg, fmt.Sprintf(`SELECT deleted, message FROM %s WHERE user_id = ? AND user_message_box_id = ?`, table), userID, boxID); err != nil {
			return err
		}
		if msg.Deleted {
			return nil
		}
		read, err := dialogReadInbox(tx, userID, row.PeerType, row.PeerID)
		if err != nil {
			return err
		}
		delta := row.UnreadDelta
		mentions := row.MentionsDelta
		if read >= boxID {
			delta = 0
			mentions = 0
		}
		if err = upsertInboxDialog(tx, row, delta, mentions); err != nil {
			return err
		}
		if row.Pts > 0 {
			if err = ensurePtsRow(tx, userID, row, table, boxID); err != nil {
				return err
			}
		}
		if delta == 0 && row.UnreadDelta == 0 && row.PeerType == mtproto.PEER_USER {
			res, err = tx.Exec(answeredCallCursorSQL(table),
				boxID, boxID, boxID,
				userID, mtproto.PEER_USER, row.PeerID,
				boxID,
				userID, mtproto.PEER_USER, row.PeerID, userID, boxID,
			)
			if err != nil {
				return err
			}
			moved, err := res.RowsAffected()
			if err != nil {
				return err
			}
			if moved > 0 {
				var unread struct {
					N int32 `db:"unread_count"`
				}
				if err = tx.QueryRow(&unread, `SELECT unread_count FROM dialogs WHERE user_id = ? AND peer_type = ? AND peer_id = ?`, userID, row.PeerType, row.PeerID); err != nil {
					return err
				}
				answered = &AnsweredCallRead{MaxID: boxID, StillUnread: unread.N}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return answered, nil
}

func dialogReadInbox(tx *sqlx.Tx, userID int64, peerType int32, peerID int64) (int32, error) {
	var row struct {
		Read int32 `db:"read_inbox_max_id"`
	}
	err := tx.QueryRow(&row, `SELECT read_inbox_max_id FROM dialogs WHERE user_id = ? AND peer_type = ? AND peer_id = ? FOR UPDATE`, userID, peerType, peerID)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return row.Read, nil
}

func upsertInboxDialog(tx *sqlx.Tx, row *inboxApplyRow, unread, mentions int32) error {
	_, err := tx.Exec(`INSERT INTO dialogs (user_id, peer_type, peer_id, peer_dialog_id, top_message, unread_count, unread_mentions_count, draft_message_data, date2)
VALUES (?,?,?,?,?,?,?, 'null', ?)
ON DUPLICATE KEY UPDATE
  top_message = GREATEST(top_message, VALUES(top_message)),
  unread_count = unread_count + VALUES(unread_count),
  unread_mentions_count = unread_mentions_count + VALUES(unread_mentions_count),
  date2 = GREATEST(date2, VALUES(date2))`,
		row.UserID, row.PeerType, row.PeerID, mtproto.MakePeerDialogId(row.PeerType, row.PeerID),
		row.BoxID, unread, mentions, row.Date2)
	return err
}

func ensurePtsRow(tx *sqlx.Tx, userID int64, row *inboxApplyRow, table string, boxID int32) error {
	var existing struct {
		Pts int32 `db:"pts"`
	}
	err := tx.QueryRow(&existing, `SELECT pts FROM user_pts_updates WHERE user_id = ? AND pts = ? LIMIT 1`, userID, row.Pts)
	if err == nil && existing.Pts == row.Pts {
		return nil
	}
	if err != nil && !errors.Is(err, sqlx.ErrNotFound) {
		return err
	}
	var msg struct {
		Data string `db:"message_data"`
	}
	if err = tx.QueryRow(&msg, fmt.Sprintf(`SELECT message_data FROM %s WHERE user_id = ? AND user_message_box_id = ?`, table), userID, boxID); err != nil {
		return err
	}
	message := &mtproto.Message{}
	_ = jsonx.UnmarshalFromString(msg.Data, message)
	raw, err := jsonx.Marshal(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Message_MESSAGE: message,
		Pts_INT32:       row.Pts,
		PtsCount:        row.PtsCount,
	}).To_Update())
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO user_pts_updates (user_id, pts, pts_count, update_type, update_data, date2) VALUES (?,?,?,?,?,?)`,
		userID, row.Pts, row.PtsCount, mtproto.PTS_UPDATE_NEW_MESSAGE, string(raw), row.Date2)
	return err
}

// recordInboxApplyTx stores the unread delta and the pts chosen for this box.
// The message insert in the same transaction is what makes a later crash
// recoverable: the row exists, applied is still 0, and the pts number is the
// one already taken from the counter.
func recordInboxApplyTx(tx *sqlx.Tx, row inboxApplyRow) error {
	_, err := tx.Exec(`INSERT INTO inbox_apply (user_id, box_id, peer_type, peer_id, unread_delta, mentions_delta, pts, pts_count, date2, applied) VALUES (?,?,?,?,?,?,?,?,?,0)`,
		row.UserID, row.BoxID, row.PeerType, row.PeerID, row.UnreadDelta, row.MentionsDelta, row.Pts, row.PtsCount, row.Date2)
	return err
}
