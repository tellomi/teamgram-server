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

	"github.com/teamgram/marmota/pkg/hack"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// DialogSaveDraftMessage
// dialog.saveDraftMessage user_id:long peer_type:int peer_id:long message:DraftMessage = Bool;
func (c *DialogCore) DialogSaveDraftMessage(in *dialog.TLDialogSaveDraftMessage) (*mtproto.Bool, error) {
	if in.Message != nil {
		in.Message.SanitizeEntities()
	}
	draft, _ := jsonx.Marshal(in.Message)
	in.PeerType, in.PeerId = savedDraftPeer(in.UserId, in.PeerType, in.PeerId)

	rows, err := c.svcCtx.Dao.DialogsDAO.SaveDraft(c.ctx,
		2,
		hack.String(draft),
		in.UserId,
		in.PeerType,
		in.PeerId)
	if err != nil {
		c.Logger.Errorf("dialog.saveDraftMessage - error: %v", err)
		return nil, err
	}
	if rows == 0 {
		if err = c.insertDraftDialog(in); err != nil {
			c.Logger.Errorf("dialog.saveDraftMessage - insert: %v", err)
			return nil, err
		}
		if _, err = c.svcCtx.Dao.DialogsDAO.SaveDraft(c.ctx,
			2,
			hack.String(draft),
			in.UserId,
			in.PeerType,
			in.PeerId); err != nil {
			c.Logger.Errorf("dialog.saveDraftMessage - error: %v", err)
			return nil, err
		}
	}

	return mtproto.BoolTrue, nil
}

// insertDraftDialog creates the dialog row SaveDraft updates. InsertIgnore
// leaves draft_type at 0; the caller writes the draft immediately after.
func (c *DialogCore) insertDraftDialog(in *dialog.TLDialogSaveDraftMessage) error {
	do := &dataobject.DialogsDO{
		UserId:           in.UserId,
		PeerType:         in.PeerType,
		PeerId:           in.PeerId,
		PeerDialogId:     mtproto.MakePeerDialogId(in.PeerType, in.PeerId),
		DraftMessageData: "null",
		Date2:            time.Now().Unix(),
	}
	_, _, err := c.svcCtx.Dao.ExecInvalidate(
		c.ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			_, _, err2 := c.svcCtx.Dao.DialogsDAO.InsertIgnore(ctx, do)
			return 0, 0, err2
		},
		dialog.GetCacheKeyByPeerType(do.UserId, do.PeerType))
	return err
}
