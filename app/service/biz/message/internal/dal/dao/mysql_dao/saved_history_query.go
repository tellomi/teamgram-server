package mysql_dao

import (
	"context"
	"math"

	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

const savedHistoryColumns = "user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period"

// SelectSavedHistory reads one page of a saved-message dialog.
// beforeID > 0 keeps ids strictly below it. afterID > 0 keeps ids strictly above it.
// beforeDate > 0 keeps date2 strictly below it. desc orders newest first.
func (dao *MessagesDAO) SelectSavedHistory(ctx context.Context, userId int64, peerType int32, peerId int64, beforeID, afterID, beforeDate int32, desc bool, limit int32) (rList []dataobject.MessagesDO, err error) {
	if limit <= 0 {
		limit = 1
	}
	query := "select " + savedHistoryColumns + " from " + dao.CalcTableName(userId) +
		" where user_id = ? and saved_peer_type = ? and saved_peer_id = ? and deleted = 0"
	args := []interface{}{userId, peerType, peerId}
	if beforeID > 0 {
		query += " and user_message_box_id < ?"
		args = append(args, beforeID)
	}
	if afterID > 0 {
		query += " and user_message_box_id > ?"
		args = append(args, afterID)
	}
	if beforeDate > 0 {
		query += " and date2 < ?"
		args = append(args, int64(beforeDate))
	}
	if desc {
		query += " order by user_message_box_id desc limit ?"
	} else {
		query += " order by user_message_box_id asc limit ?"
	}
	args = append(args, limit)
	if err = dao.db.QueryRowsPartial(ctx, &rList, query, args...); err != nil {
		logx.WithContext(ctx).Errorf("queryx in SelectSavedHistory(_), error: %v", err)
		return nil, err
	}
	return rList, nil
}

// savedDeleteUpperBound is the inclusive message id cap.
// max_id 0 means every saved message from that peer, which is what
// messages.deleteSavedHistory(max_id=0) asks for.
func savedDeleteUpperBound(maxID int32) int32 {
	if maxID <= 0 {
		return math.MaxInt32
	}
	return maxID
}

// SelectSavedForDelete lists messages.deleteSavedHistory candidates, newest first.
// maxID <= 0 deletes the whole saved dialog. minDate/maxDate of 0 are unbounded.
func (dao *MessagesDAO) SelectSavedForDelete(ctx context.Context, userId int64, peerType int32, peerId int64, maxID, minDate, maxDate, limit int32) (rList []dataobject.MessagesDO, err error) {
	if limit <= 0 {
		limit = 1
	}
	upper := savedDeleteUpperBound(maxID)
	query := "select " + savedHistoryColumns + " from " + dao.CalcTableName(userId) +
		" where user_id = ? and saved_peer_type = ? and saved_peer_id = ? and deleted = 0 and user_message_box_id <= ?"
	args := []interface{}{userId, peerType, peerId, upper}
	if minDate > 0 {
		query += " and date2 >= ?"
		args = append(args, int64(minDate))
	}
	if maxDate > 0 {
		query += " and date2 <= ?"
		args = append(args, int64(maxDate))
	}
	query += " order by user_message_box_id desc limit ?"
	args = append(args, limit)
	if err = dao.db.QueryRowsPartial(ctx, &rList, query, args...); err != nil {
		logx.WithContext(ctx).Errorf("queryx in SelectSavedForDelete(_), error: %v", err)
		return nil, err
	}
	return rList, nil
}
