package mysql_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

// HistoryDeleteFilter selects one user's live messages in a dialog.
// MaxID <= 0 has no id cap. MinDate and MaxDate <= 0 are open.
// A set bound is inclusive: date2 >= MinDate and date2 <= MaxDate.
type HistoryDeleteFilter struct {
	MaxID   int32
	MinDate int32
	MaxDate int32
	Limit   int32
}

// HistoryDeleteWhere is the SQL predicate and args for HistoryDeleteFilter.
// The caller already binds user_id, dialog_id1 and dialog_id2.
func HistoryDeleteWhere(f HistoryDeleteFilter) (string, []any) {
	clause := " and deleted = 0"
	var args []any
	if f.MaxID > 0 {
		clause += " and user_message_box_id <= ?"
		args = append(args, f.MaxID)
	}
	if f.MinDate > 0 {
		clause += " and date2 >= ?"
		args = append(args, f.MinDate)
	}
	if f.MaxDate > 0 {
		clause += " and date2 <= ?"
		args = append(args, f.MaxDate)
	}
	return clause, args
}

// SelectForHistoryDelete reads the newest matching boxes, up to limit.
func (dao *MessagesDAO) SelectForHistoryDelete(ctx context.Context, userId, dialogId1, dialogId2 int64, maxID, minDate, maxDate, limit int32) (rList []dataobject.MessagesDO, err error) {
	if limit <= 0 {
		return nil, nil
	}
	f := HistoryDeleteFilter{MaxID: maxID, MinDate: minDate, MaxDate: maxDate, Limit: limit}
	where, args := HistoryDeleteWhere(f)
	query := "select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period from " + dao.CalcTableName(userId) +
		" where user_id = ? and dialog_id1 = ? and dialog_id2 = ?" + where +
		" order by user_message_box_id desc limit ?"
	qargs := append([]any{userId, dialogId1, dialogId2}, args...)
	qargs = append(qargs, f.Limit)
	if err = dao.db.QueryRowsPartial(ctx, &rList, query, qargs...); err != nil {
		logx.WithContext(ctx).Errorf("queryx in SelectForHistoryDelete(_), error: %v", err)
		return nil, err
	}
	return rList, nil
}

// CountForHistoryDelete counts the live boxes matching the filter.
func (dao *MessagesDAO) CountForHistoryDelete(ctx context.Context, userId, dialogId1, dialogId2 int64, maxID, minDate, maxDate int32) (int32, error) {
	where, args := HistoryDeleteWhere(HistoryDeleteFilter{MaxID: maxID, MinDate: minDate, MaxDate: maxDate})
	query := "select count(*) from " + dao.CalcTableName(userId) +
		" where user_id = ? and dialog_id1 = ? and dialog_id2 = ?" + where
	qargs := append([]any{userId, dialogId1, dialogId2}, args...)
	var n int64
	if err := dao.db.QueryRowPartial(ctx, &n, query, qargs...); err != nil {
		logx.WithContext(ctx).Errorf("queryx in CountForHistoryDelete(_), error: %v", err)
		return 0, err
	}
	if n > int64(^uint32(0)>>1) {
		return int32(^uint32(0) >> 1), nil
	}
	return int32(n), nil
}
