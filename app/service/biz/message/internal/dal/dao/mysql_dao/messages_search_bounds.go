package mysql_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

// dialogMessageColumns is the messages row messages.search reads back.
const dialogMessageColumns = "user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period"

// DateInWindow is messages.search min_date / max_date.
// Zero on either side means that side is unbounded. Both ends are inclusive,
// matching messages.search on a channel.
func DateInWindow(date, minDate, maxDate int32) bool {
	if minDate > 0 && date < minDate {
		return false
	}
	if maxDate > 0 && date > maxDate {
		return false
	}
	return true
}

// dialogTextQuery is one dialog's text search with optional date bounds.
// A zero date does not constrain that side, so the old query is unchanged
// when the client omits both.
func dialogTextQuery(table string) string {
	return "select " + dialogMessageColumns + " from " + table +
		" where user_id = ? and (dialog_id1 = ? and dialog_id2 = ?) and user_message_box_id < ? and deleted = 0 and message != '' and message like ?" +
		" and (? = 0 or date2 >= ?) and (? = 0 or date2 <= ?)" +
		" order by user_message_box_id desc limit ?"
}

func dialogFromQuery(table string) string {
	return "select " + dialogMessageColumns + " from " + table +
		" where user_id = ? and (dialog_id1 = ? and dialog_id2 = ?) and sender_user_id = ? and user_message_box_id < ? and deleted = 0" +
		" and (? = 0 or date2 >= ?) and (? = 0 or date2 <= ?)" +
		" order by user_message_box_id desc limit ?"
}

// SearchDialogTextWithCB is messages.search text in one private chat or basic group.
// q is the raw query. minDate and maxDate follow DateInWindow.
func (dao *MessagesDAO) SearchDialogTextWithCB(ctx context.Context, userId, dialogId1, dialogId2 int64, userMessageBoxId int32, q string, minDate, maxDate, limit int32, cb func(sz, i int, v *dataobject.MessagesDO)) (rList []dataobject.MessagesDO, err error) {
	query := dialogTextQuery(dao.CalcTableName(userId))
	pattern := "%" + q + "%"
	var values []dataobject.MessagesDO
	err = dao.db.QueryRowsPartial(ctx, &values, query, userId, dialogId1, dialogId2, userMessageBoxId, pattern, minDate, minDate, maxDate, maxDate, limit)
	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in SearchDialogText(_), error: %v", err)
		return
	}
	rList = values
	if cb != nil {
		sz := len(rList)
		for i := range sz {
			cb(sz, i, &rList[i])
		}
	}
	return
}

// SearchDialogFromWithCB is messages.search with from_id, plus the same date window.
func (dao *MessagesDAO) SearchDialogFromWithCB(ctx context.Context, userId, dialogId1, dialogId2, senderUserId int64, userMessageBoxId int32, minDate, maxDate, limit int32, cb func(sz, i int, v *dataobject.MessagesDO)) (rList []dataobject.MessagesDO, err error) {
	query := dialogFromQuery(dao.CalcTableName(userId))
	var values []dataobject.MessagesDO
	err = dao.db.QueryRowsPartial(ctx, &values, query, userId, dialogId1, dialogId2, senderUserId, userMessageBoxId, minDate, minDate, maxDate, maxDate, limit)
	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in SearchDialogFrom(_), error: %v", err)
		return
	}
	rList = values
	if cb != nil {
		sz := len(rList)
		for i := range sz {
			cb(sz, i, &rList[i])
		}
	}
	return
}
