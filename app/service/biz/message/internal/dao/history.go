// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dao

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dao/mysql_dao"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

func (d *Dao) dialogHistory(ctx context.Context, userId, dialogId1, dialogId2 int64, q mysql_dao.DialogHistory) []*mtproto.MessageBox {
	rows, err := d.MessagesDAO.SelectDialogHistory(ctx, userId, dialogId1, dialogId2, q)
	if err != nil || len(rows) == 0 {
		return nil
	}
	out := make([]*mtproto.MessageBox, 0, len(rows))
	for i := range rows {
		out = append(out, d.MakeMessageBox(ctx, userId, &rows[i]))
	}
	return out
}

// GetOffsetIdBackwardHistoryMessages offset
func (d *Dao) GetOffsetIdBackwardHistoryMessages(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetId, minId, maxId, limit int32, hash int64) (messages []*mtproto.MessageBox) {
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		did := mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		before := offsetId
		if maxId > 0 && maxId < before {
			before = maxId
		}
		messages = d.dialogHistory(ctx, userId, did.A, did.B, mysql_dao.DialogHistory{
			BeforeID:    before,
			BeforeIDSet: true,
			AfterID:     minId,
			Desc:        true,
			Limit:       limit,
		})
	case mtproto.PEER_CHANNEL:
		logx.Errorf("blocked, License key from https://teamgram.net required to unlock enterprise features.")
	}

	messages = mtproto.ToSafeMessageBoxList(messages)
	return
}

func (d *Dao) GetOffsetIdForwardHistoryMessages(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetId, minId, maxId, limit int32, hash int64) (messages []*mtproto.MessageBox) {
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		var (
			did = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		)

		after := offsetId
		if minId > 0 && minId+1 > after {
			after = minId + 1
		}
		messages = d.dialogHistory(ctx, userId, did.A, did.B, mysql_dao.DialogHistory{
			BeforeID:       maxId,
			BeforeIDSet:    maxId > 0,
			AfterInclSet:   true,
			AfterInclusive: after,
			Desc:           false,
			Limit:          limit,
		})
	case mtproto.PEER_CHANNEL:
		logx.Errorf("blocked, License key from https://teamgram.net required to unlock enterprise features.")
	}

	messages = mtproto.ToSafeMessageBoxList(messages)
	return
}

func (d *Dao) GetOffsetDateBackwardHistoryMessages(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetDate, minId, maxId, limit int32, hash int64) (messages []*mtproto.MessageBox) {
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		var (
			did = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		)

		before := maxId
		messages = d.dialogHistory(ctx, userId, did.A, did.B, mysql_dao.DialogHistory{
			BeforeID:    before,
			BeforeIDSet: before > 0,
			AfterID:     minId,
			BeforeDate:  int64(offsetDate),
			Desc:        true,
			Limit:       limit,
		})
	case mtproto.PEER_CHANNEL:
		logx.Errorf("blocked, License key from https://teamgram.net required to unlock enterprise features.")
	}

	messages = mtproto.ToSafeMessageBoxList(messages)
	return
}

func (d *Dao) GetOffsetDateForwardHistoryMessages(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetDate, minId, maxId, limit int32, hash int64) (messages []*mtproto.MessageBox) {
	switch peer.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
		var (
			did = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		)

		after := minId
		messages = d.dialogHistory(ctx, userId, did.A, did.B, mysql_dao.DialogHistory{
			BeforeID:    maxId,
			BeforeIDSet: maxId > 0,
			AfterID:     after,
			AfterDate:   int64(offsetDate),
			Desc:        false,
			Limit:       limit,
		})
	case mtproto.PEER_CHANNEL:
		logx.Errorf("blocked, License key from https://teamgram.net required to unlock enterprise features.")
	}

	messages = mtproto.ToSafeMessageBoxList(messages)
	return
}

// GetOffsetIdBackwardUnreadMentions GetOffsetIdBackwardUnreadMentions
func (d *Dao) GetOffsetIdBackwardUnreadMentions(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetId, minId, maxId, limit int32) (messages []*mtproto.MessageBox) {
	switch peer.PeerType {
	case mtproto.PEER_CHAT:
		var (
			did = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		)

		rList, _ := d.MessagesDAO.SelectBackwardUnreadMentionsByOffsetIdLimitWithCB(
			ctx,
			userId,
			did.A,
			did.B,
			offsetId,
			limit,
			func(sz, i int, v *dataobject.MessagesDO) {
				messages = append(messages, d.MakeMessageBox(ctx, userId, v))
			})
		_ = rList
	case mtproto.PEER_CHANNEL:
		logx.Errorf("blocked, License key from https://teamgram.net required to unlock enterprise features.")
	}
	return
}

func (d *Dao) GetOffsetIdForwardUnreadMentions(ctx context.Context, userId int64, peer *mtproto.PeerUtil, offsetId, minId, maxId, limit int32) (messages []*mtproto.MessageBox) {
	switch peer.PeerType {
	case mtproto.PEER_CHAT:
		var (
			did = mtproto.MakeDialogId(userId, peer.PeerType, peer.PeerId)
		)

		rList, _ := d.MessagesDAO.SelectForwardUnreadMentionsByOffsetIdLimitWithCB(
			ctx,
			userId,
			did.A,
			did.B,
			offsetId,
			limit,
			func(sz, i int, v *dataobject.MessagesDO) {
				messages = append(messages, d.MakeMessageBox(ctx, userId, v))
			})
		_ = rList
	case mtproto.PEER_CHANNEL:
		logx.Errorf("blocked, License key from https://teamgram.net required to unlock enterprise features.")
	}
	return
}
