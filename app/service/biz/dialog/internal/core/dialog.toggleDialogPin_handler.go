/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"context"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogToggleDialogPin
// dialog.toggleDialogPin user_id:long peer_type:int peer_id:long pinned:Bool = Int32;
func (c *DialogCore) DialogToggleDialogPin(in *dialog.TLDialogToggleDialogPin) (*mtproto.Int32, error) {
	var (
		peerDialogId = mtproto.MakePeerDialogId(in.PeerType, in.PeerId)
		pinned       int64
	)

	dlgExt, err := c.svcCtx.Dao.GetDialogByPeerDialogId(c.ctx, in.GetUserId(), peerDialogId)
	if err != nil {
		c.Logger.Errorf("dialog.toggleDialogPin - error: %v", err)
		return nil, err
	}

	folderId := dlgExt.GetDialog().GetFolderId().GetValue()
	already := dlgExt.GetDialog().GetPinned()
	pinning := mtproto.FromBool(in.Pinned)
	if pinning == already {
		return &mtproto.Int32{V: folderId}, nil
	}
	if pinning {
		// Official: you can't pin an empty chat with a user. A group or
		// channel with no messages can still be pinned; membership is the
		// dialog.
		if in.PeerType == mtproto.PEER_USER && dlgExt.GetDialog().GetTopMessage() == 0 {
			return nil, mtproto.ErrPeerHistoryEmpty
		}
		var (
			pinnedIDs []int64
			listErr   error
		)
		if folderId == 0 {
			pinnedIDs, listErr = c.svcCtx.Dao.GetNoCachePinnedDialogIdList(c.ctx, in.UserId)
		} else {
			pinnedIDs, listErr = c.svcCtx.Dao.GetNoCacheFolderPinnedDialogIdList(c.ctx, in.UserId)
		}
		if listErr != nil {
			c.Logger.Errorf("dialog.toggleDialogPin - error: %v", listErr)
			return nil, listErr
		}
		if len(pinnedIDs) >= pinnedDialogLimit(folderId) {
			return nil, mtproto.ErrPinnedDialogsTooMuch
		}
		pinned = time.Now().Unix() << 32
	} else {
		pinned = 0
	}

	if folderId == 0 {
		c.svcCtx.Dao.ExecInvalidate(
			c.ctx,
			func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
				c.svcCtx.Dao.DialogsDAO.UpdatePeerDialogListPinned(c.ctx, pinned, in.UserId, []int64{peerDialogId})
				return 0, 0, nil
			},
			dialog.GetDialogCacheKey(in.GetUserId(), peerDialogId),
			dialog.GetPinnedDialogIdListCacheKey(in.GetUserId()))
	} else {
		c.svcCtx.Dao.ExecInvalidate(
			c.ctx,
			func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
				c.svcCtx.Dao.DialogsDAO.UpdateFolderPeerDialogListPinned(c.ctx, pinned, in.UserId, []int64{peerDialogId})
				return 0, 0, nil
			},
			dialog.GetDialogCacheKey(in.GetUserId(), peerDialogId),
			dialog.GetFolderPinnedDialogIdListCacheKey(in.GetUserId()))
	}

	return &mtproto.Int32{
		V: folderId,
	}, nil
}
