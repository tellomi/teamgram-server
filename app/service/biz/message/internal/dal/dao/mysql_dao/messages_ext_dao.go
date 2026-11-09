/*
 * WARNING! All changes made in this file will be lost!
 *   Created from by 'dalgen'
 *
 * Copyright (c) 2024-present,  Teamgram Authors.
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package mysql_dao

import (
	"context"
	"strings"

	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

// SelectByPhotoVideoMediaType
// select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period from messages where user_id = :user_id and (dialog_id1 = :dialog_id1 and dialog_id2 = :dialog_id2) and message_filter_type in (0, 7, 8) and user_message_box_id < :user_message_box_id and deleted = 0 order by user_message_box_id desc limit :limit
func (dao *MessagesDAO) SelectByPhotoVideoMediaType(ctx context.Context, userId int64, dialogId1 int64, dialogId2 int64, userMessageBoxId int32, limit int32) (rList []dataobject.MessagesDO, err error) {
	var (
		query  = "select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period from " + dao.CalcTableName(userId) + " where user_id = ? and (dialog_id1 = ? and dialog_id2 = ?) and message_filter_type in (0, 7, 8) and user_message_box_id < ? and deleted = 0 order by user_message_box_id desc limit ?"
		values []dataobject.MessagesDO
	)
	err = dao.db.QueryRowsPartial(ctx, &values, query, userId, dialogId1, dialogId2, userMessageBoxId, limit)

	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in SelectByPhotoVideoMediaType(_), error: %v", err)
		return
	}

	rList = values

	return
}

// SelectByPhotoVideoMediaTypeWithCB
// select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period from messages where user_id = :user_id and (dialog_id1 = :dialog_id1 and dialog_id2 = :dialog_id2) and message_filter_type in (0, 7, 8) and user_message_box_id < :user_message_box_id and deleted = 0 order by user_message_box_id desc limit :limit
// The filter is fixed in the SQL (empty, photo, video). Passing it as a sixth
// argument makes go-zero reject the query: "5 arguments provided, not matching sql".
func (dao *MessagesDAO) SelectByPhotoVideoMediaTypeWithCB(ctx context.Context, userId int64, dialogId1 int64, dialogId2 int64, userMessageBoxId int32, limit int32, cb func(sz, i int, v *dataobject.MessagesDO)) (rList []dataobject.MessagesDO, err error) {
	var (
		query  = "select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period from " + dao.CalcTableName(userId) + " where user_id = ? and (dialog_id1 = ? and dialog_id2 = ?) and message_filter_type in (0, 7, 8) and user_message_box_id < ? and deleted = 0 order by user_message_box_id desc limit ?"
		values []dataobject.MessagesDO
	)
	err = dao.db.QueryRowsPartial(ctx, &values, query, userId, dialogId1, dialogId2, userMessageBoxId, limit)

	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in SelectByPhotoVideoMediaType(_), error: %v", err)
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

// SearchGlobalByMediaWithCB scans every dialog of one user.
// matchURL also keeps a message whose text contains a URL, which is how
// inputMessagesFilterUrl finds a link that was stored as plain text.
func (dao *MessagesDAO) SearchGlobalByMediaWithCB(ctx context.Context, userId int64, userMessageBoxId int32, limit int32, mediaTypes []int32, matchURL bool, cb func(sz, i int, v *dataobject.MessagesDO)) (rList []dataobject.MessagesDO, err error) {
	if len(mediaTypes) == 0 || limit <= 0 {
		return nil, nil
	}
	holders := make([]string, len(mediaTypes))
	args := []any{userId, userMessageBoxId}
	for i, kind := range mediaTypes {
		holders[i] = "?"
		args = append(args, kind)
	}
	query := "select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, pinned, has_reaction, reaction, reaction_date, reaction_unread, saved_peer_type, saved_peer_id, date2, ttl_period from " + dao.CalcTableName(userId) + " where user_id = ? and user_message_box_id < ? and deleted = 0 and (message_filter_type in (" + strings.Join(holders, ",") + ")"
	if matchURL {
		query += " OR message LIKE ? OR message LIKE ? OR message LIKE ?"
		args = append(args, "%http://%", "%https://%", "%www.%")
	}
	query += ") order by user_message_box_id desc limit ?"
	args = append(args, limit)
	var values []dataobject.MessagesDO
	err = dao.db.QueryRowsPartial(ctx, &values, query, args...)
	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in SearchGlobalByMedia(_), error: %v", err)
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
