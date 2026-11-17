package mysql_dao

import (
	"context"
	"strings"
	"unicode"

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
	"from __TABLE__ where user_id = ? and deleted = 0 and "

// searchGlobalTextCursor is the rows strictly after the cursor.
// It is the SQL form of searchorder.Key.OlderThan.
const searchGlobalTextCursor = "" +
	" and (date2 < ? or (date2 = ? and peer_type > ?) or (date2 = ? and peer_type = ? and peer_id > ?) or (date2 = ? and peer_type = ? and peer_id = ? and user_message_box_id < ?))"

const searchGlobalTextOrder = " order by date2 desc, peer_type asc, peer_id asc, user_message_box_id desc limit ?"

// CountGlobalText is every matching row, not one page. restrict limits the
// count to private chats (users) and/or basic groups. Channel posts are not
// in this table.
func (dao *MessagesDAO) CountGlobalText(ctx context.Context, userId int64, q string, restrict, users, groups bool) (int32, error) {
	restrictN, usersN, groupsN := 0, 0, 0
	if restrict {
		restrictN = 1
	}
	if users {
		usersN = 1
	}
	if groups {
		groupsN = 1
	}
	patterns := likeTokens(q)
	query := strings.Replace(""+
		"select count(*) as n from __TABLE__ where user_id = ? and deleted = 0 and "+textTokenWhere(len(patterns))+
		" and (? = 0 or (? = 1 and peer_type in (1, 2)) or (? = 1 and peer_type = 3))",
		"__TABLE__", dao.CalcTableName(userId), -1)
	var row struct {
		N int32 `db:"n"`
	}
	args := []any{userId}
	for _, pattern := range patterns {
		args = append(args, pattern, pattern)
	}
	args = append(args, restrictN, usersN, groupsN)
	err := dao.db.QueryRowPartial(ctx, &row, query, args...)
	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in CountGlobalText(_), error: %v", err)
		return 0, err
	}
	return row.N, nil
}

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

// textTokens splits a search into words. Order and adjacency do not matter:
// every word has to occur somewhere in the message.
func textTokens(q string) []string {
	return strings.FieldsFunc(q, unicode.IsSpace)
}

func likeTokens(q string) []string {
	parts := textTokens(q)
	if len(parts) == 0 {
		return []string{likeSubstring("")}
	}
	out := make([]string, len(parts))
	for i, part := range parts {
		out[i] = likeSubstring(part)
	}
	return out
}

func textTokenWhere(n int) string {
	if n < 1 {
		n = 1
	}
	const one = "(message COLLATE utf8mb4_unicode_ci LIKE ? ESCAPE '\\\\' OR (message = '' AND CAST(message_data AS CHAR) COLLATE utf8mb4_unicode_ci LIKE ? ESCAPE '\\\\'))"
	if n == 1 {
		return one
	}
	return strings.TrimSuffix(strings.Repeat(one+" AND ", n), " AND ")
}

// TextHasEveryToken reports whether text contains each whitespace-separated
// word in q, case-insensitively. A blank q matches everything.
func TextHasEveryToken(text, q string) bool {
	folded := strings.ToLower(text)
	for _, tok := range textTokens(strings.ToLower(q)) {
		if !strings.Contains(folded, tok) {
			return false
		}
	}
	return true
}

// SearchGlobalTextWithCB is messages.searchGlobal over one user's message boxes.
// q is the raw query, not a LIKE pattern. There is no minimum length.
// Inbox and outbox rows are both included. active is the global cursor;
// its message id is a user box id only for the same peer.
func (dao *MessagesDAO) SearchGlobalTextWithCB(ctx context.Context, userId int64, cur searchorder.Key, active bool, q string, limit int32, cb func(sz, i int, v *dataobject.MessagesDO)) (rList []dataobject.MessagesDO, err error) {
	patterns := likeTokens(q)
	query := strings.Replace(searchGlobalTextSelect, "__TABLE__", dao.CalcTableName(userId), -1) + textTokenWhere(len(patterns))
	args := []any{userId}
	for _, pattern := range patterns {
		args = append(args, pattern, pattern)
	}
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

// dialogTextFilter is the shared messages.search predicate for one dialog.
// sender 0 does not filter the sender. minDate and maxDate of 0 are
// unbounded. An empty plaintext column still matches message_data, the same
// fallback SearchGlobalText uses. Each word is its own escaped LIKE.
func dialogTextFilter(q string, sender int64, minDate, maxDate int32) (string, []any) {
	patterns := likeTokens(q)
	clause := "deleted = 0 and " + textTokenWhere(len(patterns))
	args := make([]any, 0, len(patterns)*2+3)
	for _, pattern := range patterns {
		args = append(args, pattern, pattern)
	}
	if sender != 0 {
		clause += " and sender_user_id = ?"
		args = append(args, sender)
	}
	if minDate > 0 {
		clause += " and date2 >= ?"
		args = append(args, int64(minDate))
	}
	if maxDate > 0 {
		clause += " and date2 <= ?"
		args = append(args, int64(maxDate))
	}
	return clause, args
}

// SearchDialogText is messages.search inside one dialog. Each word in q must
// appear. minDate/maxDate of 0 are unbounded. offset is an exclusive upper
// message id. sender 0 keeps every sender in that mailbox.
func (dao *MessagesDAO) SearchDialogText(ctx context.Context, userId, dialogID1, dialogID2 int64, offset int32, q string, sender int64, minDate, maxDate, limit int32) (rList []dataobject.MessagesDO, err error) {
	if limit <= 0 {
		limit = 1
	}
	if offset == 0 {
		offset = 1<<31 - 1
	}
	filter, filterArgs := dialogTextFilter(q, sender, minDate, maxDate)
	query := "select " + savedHistoryColumns + " from " + dao.CalcTableName(userId) +
		" where user_id = ? and dialog_id1 = ? and dialog_id2 = ? and user_message_box_id < ? and " +
		filter + " order by user_message_box_id desc limit ?"
	args := make([]any, 0, 4+len(filterArgs)+1)
	args = append(args, userId, dialogID1, dialogID2, offset)
	args = append(args, filterArgs...)
	args = append(args, limit)
	if err = dao.db.QueryRowsPartial(ctx, &rList, query, args...); err != nil {
		logx.WithContext(ctx).Errorf("queryx in SearchDialogText(_), error: %v", err)
		return nil, err
	}
	return rList, nil
}

// CountDialogText counts the same rows SearchDialogText would page. Offset
// and limit are not part of the total.
func (dao *MessagesDAO) CountDialogText(ctx context.Context, userId, dialogID1, dialogID2 int64, q string, sender int64, minDate, maxDate int32) (int32, error) {
	filter, filterArgs := dialogTextFilter(q, sender, minDate, maxDate)
	query := "select count(*) as n from " + dao.CalcTableName(userId) +
		" where user_id = ? and dialog_id1 = ? and dialog_id2 = ? and " + filter
	args := make([]any, 0, 3+len(filterArgs))
	args = append(args, userId, dialogID1, dialogID2)
	args = append(args, filterArgs...)
	var row struct {
		N int32 `db:"n"`
	}
	if err := dao.db.QueryRowPartial(ctx, &row, query, args...); err != nil {
		logx.WithContext(ctx).Errorf("queryx in CountDialogText(_), error: %v", err)
		return 0, err
	}
	return row.N, nil
}
