// Copyright 2024 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package dao

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
)

// savedDeleteBatch matches saved.DeleteBatch: one deleteSavedHistory call.
const savedDeleteBatch int32 = 100

func (d *Dao) boxes(ctx context.Context, userId int64, rows []dataobject.MessagesDO) []*mtproto.MessageBox {
	out := make([]*mtproto.MessageBox, 0, len(rows))
	for i := range rows {
		out = append(out, d.MakeMessageBox(ctx, userId, &rows[i]))
	}
	return mtproto.ToSafeMessageBoxList(out)
}

// GetOffsetIdBackwardSavedHistoryMessages returns messages older than offsetId.
// minId, maxId, and offsetDate are the documented bounds; 0 means unbounded.
// Channel saved peers use the same messages.saved_peer_* columns as users and chats.
func (d *Dao) GetOffsetIdBackwardSavedHistoryMessages(ctx context.Context, userId int64, savedPeerId *mtproto.PeerUtil, offsetId, minId, maxId, offsetDate, limit int32, _ int64) []*mtproto.MessageBox {
	if savedPeerId == nil || limit <= 0 {
		return nil
	}
	before := offsetId
	if maxId > 0 && (before == 0 || maxId < before) {
		before = maxId
	}
	rows, err := d.MessagesDAO.SelectSavedHistory(ctx, userId, savedPeerId.PeerType, savedPeerId.PeerId, before, minId, offsetDate, true, limit)
	if err != nil {
		return nil
	}
	return d.boxes(ctx, userId, rows)
}

// GetOffsetIdForwardSavedHistoryMessages returns messages at or after offsetId, oldest first.
func (d *Dao) GetOffsetIdForwardSavedHistoryMessages(ctx context.Context, userId int64, savedPeerId *mtproto.PeerUtil, offsetId, minId, maxId, limit int32, _ int64) []*mtproto.MessageBox {
	if savedPeerId == nil || limit <= 0 {
		return nil
	}
	after := offsetId - 1
	if minId > after {
		after = minId
	}
	before := maxId
	rows, err := d.MessagesDAO.SelectSavedHistory(ctx, userId, savedPeerId.PeerType, savedPeerId.PeerId, before, after, 0, false, limit)
	if err != nil {
		return nil
	}
	return d.boxes(ctx, userId, rows)
}

// ListSavedForDelete returns one deleteSavedHistory batch, newest first.
func (d *Dao) ListSavedForDelete(ctx context.Context, userId int64, peerType int32, peerId int64, maxID, minDate, maxDate int32) ([]*mtproto.MessageBox, error) {
	rows, err := d.MessagesDAO.SelectSavedForDelete(ctx, userId, peerType, peerId, maxID, minDate, maxDate, savedDeleteBatch)
	if err != nil {
		return nil, err
	}
	return d.boxes(ctx, userId, rows), nil
}
