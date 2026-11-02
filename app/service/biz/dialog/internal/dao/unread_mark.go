package dao

import "context"

type unreadMarkRow struct {
	PeerType int32 `db:"peer_type"`
	PeerId   int64 `db:"peer_id"`
}

// SetUnreadMark stores messages.markDialogUnread on the dialog row.
// A dialog that does not exist is left alone; the caller still returns success.
func (d *Dao) SetUnreadMark(ctx context.Context, userID int64, peerType int32, peerID int64, unread bool) error {
	mark := 0
	if unread {
		mark = 1
	}
	_, err := d.Mysql.Exec(ctx, "update dialogs set unread_mark = ? where user_id = ? and peer_type = ? and peer_id = ? and deleted = 0", mark, userID, peerType, peerID)
	return err
}

// ListUnreadMarks returns dialogs the user has marked unread.
func (d *Dao) ListUnreadMarks(ctx context.Context, userID int64) ([]unreadMarkRow, error) {
	var rows []unreadMarkRow
	err := d.Mysql.QueryRowsPartial(ctx, &rows, "select peer_type, peer_id from dialogs where user_id = ? and unread_mark = 1 and deleted = 0", userID)
	if err != nil {
		return nil, err
	}
	return rows, nil
}
