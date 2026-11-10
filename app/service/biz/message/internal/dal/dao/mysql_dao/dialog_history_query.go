package mysql_dao

import (
	"context"

	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

// DialogHistory is one messages.getHistory page for a private chat or basic group.
// BeforeID, when set, keeps ids strictly below it (offset_id / max_id).
// AfterID, when set, keeps ids strictly above it (min_id).
// AfterInclusive, when set, keeps ids greater than or equal (forward offset_id).
// BeforeDate / AfterDate use date2. Zero dates are unbounded.
// Desc orders newest first, which is the backward page.
type DialogHistory struct {
	BeforeID       int32
	BeforeIDSet    bool
	AfterID        int32
	AfterInclusive int32
	AfterInclSet   bool
	BeforeDate     int64
	AfterDate      int64
	Desc           bool
	Limit          int32
}

// SelectDialogHistory reads one history page.
// min_id is AfterID (id > min_id). max_id is folded into BeforeID by the caller
// (id < max_id). A missing bound is not added, so a plain backward read stays
// "id < offset AND deleted = 0 ORDER BY id DESC".
func (dao *MessagesDAO) SelectDialogHistory(ctx context.Context, userId, dialogId1, dialogId2 int64, q DialogHistory) (rList []dataobject.MessagesDO, err error) {
	if q.Limit <= 0 {
		return nil, nil
	}
	query := "select " + savedHistoryColumns + " from " + dao.CalcTableName(userId) +
		" where user_id = ? and dialog_id1 = ? and dialog_id2 = ? and deleted = 0"
	args := []any{userId, dialogId1, dialogId2}
	if q.BeforeIDSet {
		query += " and user_message_box_id < ?"
		args = append(args, q.BeforeID)
	}
	if q.AfterID > 0 {
		query += " and user_message_box_id > ?"
		args = append(args, q.AfterID)
	}
	if q.AfterInclSet {
		query += " and user_message_box_id >= ?"
		args = append(args, q.AfterInclusive)
	}
	if q.BeforeDate > 0 {
		query += " and date2 < ?"
		args = append(args, q.BeforeDate)
	}
	if q.AfterDate > 0 {
		query += " and date2 >= ?"
		args = append(args, q.AfterDate)
	}
	if q.Desc {
		query += " order by user_message_box_id desc limit ?"
	} else {
		query += " order by user_message_box_id asc limit ?"
	}
	args = append(args, q.Limit)
	if err = dao.db.QueryRowsPartial(ctx, &rList, query, args...); err != nil {
		logx.WithContext(ctx).Errorf("queryx in SelectDialogHistory(_), error: %v", err)
		return nil, err
	}
	return rList, nil
}
