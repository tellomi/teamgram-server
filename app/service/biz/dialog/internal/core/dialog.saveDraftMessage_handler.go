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
	if top := dialog.DraftTopicID(in.GetMessage()); top > 0 {
		return c.saveTopicDraft(in, top)
	}
	if in.Message != nil {
		in.Message.SanitizeEntities()
	}
	draft, _ := jsonx.Marshal(in.Message)
	in.PeerType, in.PeerId = savedDraftPeer(in.UserId, in.PeerType, in.PeerId)

	// top_msg_id > 0 already returned through saveTopicDraft. A dialog
	// draft (top_msg_id 0) still invalidates the dialog cache, and a miss
	// inserts the dialog row before the draft is written again.
	_, rows, err := c.saveDialogDraft(in, draft)
	if err != nil {
		c.Logger.Errorf("dialog.saveDraftMessage - error: %v", err)
		return nil, err
	}
	if rows == 0 {
		if err = c.insertDraftDialog(in); err != nil {
			c.Logger.Errorf("dialog.saveDraftMessage - insert: %v", err)
			return nil, err
		}
		if _, _, err = c.saveDialogDraft(in, draft); err != nil {
			c.Logger.Errorf("dialog.saveDraftMessage - error: %v", err)
			return nil, err
		}
	}

	return mtproto.BoolTrue, nil
}

// saveDialogDraft writes a dialog-level draft (top_msg_id 0) and drops the
// cached dialog. The second result is the number of rows SaveDraft updated.
func (c *DialogCore) saveDialogDraft(in *dialog.TLDialogSaveDraftMessage, draft []byte) (int64, int64, error) {
	return c.svcCtx.Dao.ExecInvalidate(
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
