package dao

import (
	"context"
	"fmt"

	"github.com/teamgram/proto/mtproto"
)

// phoneCallInboxUnread is the dialogs.unread_count delta for one inbox row.
// InsertOrUpdate adds this value. An answered call adds nothing. Every other
// inbox row, including a missed or declined call, still adds one.
func phoneCallInboxUnread(message *mtproto.Message) int32 {
	if message != nil && mtproto.AnsweredPhoneCall(message.GetAction()) {
		return 0
	}
	return 1
}

// AnsweredCallRead is set when the callee's read cursor moved onto an
// answered call without passing an earlier unread incoming message.
// StillUnread is the dialog's unread_count after that move. MaxID is the
// call service message. The caller's outbox is not part of this update.
type AnsweredCallRead struct {
	MaxID       int32
	StillUnread int32
}

// answeredCallCursorSQL moves read_inbox_max_id onto the new service message
// only when no incoming message sits strictly between the current cursor and
// that message. Message box ids are per user, so a gap in the id sequence is
// normal and is not itself an unread message. unread_count is cleared only
// when this message is still the dialog top. A newer row keeps the count the
// concurrent insert already added.
func answeredCallCursorSQL(table string) string {
	return fmt.Sprintf(`UPDATE dialogs
SET read_inbox_max_id = ?,
    unread_count = IF(top_message = ?, 0, unread_count),
    unread_mark = IF(top_message = ?, 0, unread_mark)
WHERE user_id = ? AND peer_type = ? AND peer_id = ?
  AND read_inbox_max_id < ?
  AND NOT EXISTS (
    SELECT 1 FROM %s m
    WHERE m.user_id = ?
      AND m.peer_type = ?
      AND m.peer_id = ?
      AND m.sender_user_id <> ?
      AND m.user_message_box_id > dialogs.read_inbox_max_id
      AND m.user_message_box_id < ?
      AND m.deleted = 0
  )`, table)
}

func (d *Dao) advanceAnsweredCallRead(ctx context.Context, userID, peerID int64, messageID int32) (*AnsweredCallRead, error) {
	if d == nil || d.MessagesDAO == nil || messageID <= 0 {
		return nil, nil
	}
	table := d.MessagesDAO.CalcTableName(userID)
	res, err := d.DB.Exec(ctx, answeredCallCursorSQL(table),
		messageID, messageID, messageID,
		userID, mtproto.PEER_USER, peerID,
		messageID,
		userID, mtproto.PEER_USER, peerID, userID, messageID,
	)
	if err != nil {
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, nil
	}
	dlg, err := d.DialogsDAO.SelectDialog(ctx, userID, mtproto.PEER_USER, peerID)
	if err != nil {
		return nil, err
	}
	if dlg == nil || dlg.ReadInboxMaxId < messageID {
		return nil, nil
	}
	return &AnsweredCallRead{MaxID: messageID, StillUnread: dlg.UnreadCount}, nil
}
