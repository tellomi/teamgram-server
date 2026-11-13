package mysql_dao

import (
	"context"
	"strings"

	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/pkg/searchorder"

	"github.com/zeromicro/go-zero/core/logx"
)

// searchGlobalTextSelect is the column list for messages.searchGlobal.
//
// Order matches pkg/searchorder: date2 DESC, peer_type ASC, peer_id ASC,
// user_message_box_id DESC. A later page uses the tuple in
// searchGlobalTextCursor, not user_message_box_id < offset_id. A channel
// message id is a different space and is only compared when peer_type and
// peer_id are the cursor's peer.
//
// utf8mb4_unicode_ci makes the comparison case-insensitive even when the
// column collation is binary, so "Hello" matches "hello". A row whose
// plaintext column is empty still matches when the text is only in
// message_data. % _ and \ in the query are literals.
//
// A row whose ttl_period has already expired still matches until the sweep
// deletes it. This query does not hide those rows.
const searchGlobalTextSelect = "" +
	"select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period " +
	"from __TABLE__ where user_id = ? and deleted = 0 and (" +
	"message COLLATE utf8mb4_unicode_ci LIKE ? ESCAPE '\\\\' " +
	"OR (message = '' AND CAST(message_data AS CHAR) COLLATE utf8mb4_unicode_ci LIKE ? ESCAPE '\\\\')" +
	")"

// searchGlobalTextCursor is the rows strictly after the cursor.
// It is the SQL form of searchorder.Key.OlderThan.
const searchGlobalTextCursor = "" +
	" and (date2 < ? or (date2 = ? and peer_type > ?) or (date2 = ? and peer_type = ? and peer_id > ?) or (date2 = ? and peer_type = ? and peer_id = ? and user_message_box_id < ?))"

const searchGlobalTextOrder = " order by date2 desc, peer_type asc, peer_id asc, user_message_box_id desc limit ?"

// likeSubstring is a LIKE pattern that matches q anywhere.
// %, _, and \ in q are literals.
func likeSubstring(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 2)
	b.WriteByte('%')
	for _, r := range q {
		switch r {
		case '\\', '%', '_':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('%')
	return b.String()
}

// SearchGlobalTextWithCB is messages.searchGlobal over one user's message boxes.
// q is the raw query, not a LIKE pattern. There is no minimum length.
// Inbox and outbox rows are both included. active is the global cursor;
// its message id is a user box id only for the same peer.
func (dao *MessagesDAO) SearchGlobalTextWithCB(ctx context.Context, userId int64, cur searchorder.Key, active bool, q string, limit int32, cb func(sz, i int, v *dataobject.MessagesDO)) (rList []dataobject.MessagesDO, err error) {
	pattern := likeSubstring(q)
	query := strings.Replace(searchGlobalTextSelect, "__TABLE__", dao.CalcTableName(userId), -1)
	args := []any{userId, pattern, pattern}
	if active {
		query += searchGlobalTextCursor
		args = append(args,
			cur.Date,
			cur.Date, cur.PeerType,
			cur.Date, cur.PeerType, cur.PeerID,
			cur.Date, cur.PeerType, cur.PeerID, cur.MsgID,
		)
	}
	query += searchGlobalTextOrder
	args = append(args, limit)
	var values []dataobject.MessagesDO
	err = dao.db.QueryRowsPartial(ctx, &values, query, args...)
	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in SearchGlobalText(_), error: %v", err)
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
