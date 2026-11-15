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

	"github.com/teamgram/marmota/pkg/hack"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// DialogSaveDraftMessage
// dialog.saveDraftMessage user_id:long peer_type:int peer_id:long message:DraftMessage = Bool;
func (c *DialogCore) DialogSaveDraftMessage(in *dialog.TLDialogSaveDraftMessage) (*mtproto.Bool, error) {
	if top := dialog.DraftTopicID(in.GetMessage()); top > 0 {
		return c.saveTopicDraft(in, top)
	}
	if in.Message != nil {
		in.Message.SanitizeEntities()
	}
	draft, _ := jsonx.Marshal(in.Message)

	_, _, err := c.svcCtx.Dao.ExecInvalidate(
		c.ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			n, err := c.svcCtx.Dao.DialogsDAO.SaveDraft(ctx,
				2,
				hack.String(draft),
				in.UserId,
				in.PeerType,
				in.PeerId)
			return 0, n, err
		},
		dialog.GetDialogCacheKeyByPeer(in.UserId, in.PeerType, in.PeerId))
	if err != nil {
		c.Logger.Errorf("dialog.saveDraftMessage - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
