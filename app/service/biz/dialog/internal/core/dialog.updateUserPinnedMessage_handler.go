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

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogUpdateUserPinnedMessage
// dialog.updateUserPinnedMessage user_id:long peer_type:int peer_id:long pinned_msg_id:int = Bool;
//
// Writes pinned_msg_id on a dialog that is still in the chat list. A deleted
// dialog is left deleted, so pinning a channel post does not bring it back.
func (c *DialogCore) DialogUpdateUserPinnedMessage(in *dialog.TLDialogUpdateUserPinnedMessage) (*mtproto.Bool, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogsDAO == nil || in.GetUserId() == 0 || in.GetPeerId() == 0 {
		return mtproto.BoolTrue, nil
	}
	_, _, err := c.svcCtx.Dao.ExecInvalidate(
		c.ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			n, err := c.svcCtx.Dao.DialogsDAO.UpdatePinnedMsgId(ctx, in.GetPinnedMsgId(), in.GetUserId(), mtproto.MakePeerDialogId(in.GetPeerType(), in.GetPeerId()))
			return 0, n, err
		},
		dialog.GetDialogCacheKeyByPeer(in.UserId, in.PeerType, in.PeerId))
	if err != nil {
		c.Logger.Errorf("dialog.updateUserPinnedMessage - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
