package mysql_dao

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

// forwardSet builds one SET clause. Message cursors only move forward so two
// overlapping sends cannot put an older top_message back on the dialog.
// unread_count is absolute and is allowed to fall when a chat is read.
func forwardSet(column string) string {
	switch column {
	case "top_message", "date2", "read_inbox_max_id", "read_outbox_max_id":
		return column + " = GREATEST(" + column + ", ?)"
	default:
		return column + " = ?"
	}
}

// UpdateForwardOnly is UpdateCustomMap with forward-only cursors.
func (dao *DialogsDAO) UpdateForwardOnly(ctx context.Context, cMap map[string]interface{}, userId int64, peerType int32, peerId int64) (rowsAffected int64, err error) {
	names := make([]string, 0, len(cMap))
	aValues := make([]interface{}, 0, len(cMap))
	for k, v := range cMap {
		names = append(names, forwardSet(k))
		aValues = append(aValues, v)
	}
	query := fmt.Sprintf("update dialogs set %s where user_id = ? and peer_type = ? and peer_id = ?", strings.Join(names, ", "))
	aValues = append(aValues, userId, peerType, peerId)

	var rResult sql.Result
	rResult, err = dao.db.Exec(ctx, query, aValues...)
	if err != nil {
		logx.WithContext(ctx).Errorf("exec in UpdateForwardOnly(_), error: %v", err)
		return
	}
	rowsAffected, err = rResult.RowsAffected()
	if err != nil {
		logx.WithContext(ctx).Errorf("rowsAffected in UpdateForwardOnly(_), error: %v", err)
	}
	return
}

// insertOrUpdateForwardSQL keeps unread increments and refuses to move
// top_message or date2 backwards when two inbox applies land out of order.
const insertOrUpdateForwardSQL = "insert into dialogs(user_id, peer_type, peer_id, peer_dialog_id, top_message, pinned_msg_id, unread_count, unread_mentions_count, draft_message_data, date2) values (:user_id, :peer_type, :peer_id, :peer_dialog_id, :top_message, :pinned_msg_id, :unread_count, :unread_mentions_count, :draft_message_data, :date2) on duplicate key update top_message = GREATEST(top_message, VALUES(top_message)), unread_count = unread_count + VALUES(unread_count), unread_mentions_count = unread_mentions_count + VALUES(unread_mentions_count), date2 = GREATEST(date2, VALUES(date2))"

// InsertOrUpdateForward is InsertOrUpdate for a newly delivered message.
func (dao *DialogsDAO) InsertOrUpdateForward(ctx context.Context, do *dataobject.DialogsDO) (lastInsertId, rowsAffected int64, err error) {
	r, err := dao.db.NamedExec(ctx, insertOrUpdateForwardSQL, do)
	if err != nil {
		logx.WithContext(ctx).Errorf("namedExec in InsertOrUpdateForward(%v), error: %v", do, err)
		return
	}
	lastInsertId, err = r.LastInsertId()
	if err != nil {
		logx.WithContext(ctx).Errorf("lastInsertId in InsertOrUpdateForward(%v), error: %v", do, err)
		return
	}
	rowsAffected, err = r.RowsAffected()
	if err != nil {
		logx.WithContext(ctx).Errorf("rowsAffected in InsertOrUpdateForward(%v), error: %v", do, err)
	}
	return
}
