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
	keys := draftCacheKeys(in.UserId, in.PeerType, in.PeerId)
	if rows == 0 {
		_, _, err = c.svcCtx.Dao.DialogsDAO.InsertIgnore(c.ctx, &dataobject.DialogsDO{
			UserId:           in.UserId,
			PeerType:         in.PeerType,
			PeerId:           in.PeerId,
			PeerDialogId:     mtproto.MakePeerDialogId(in.PeerType, in.PeerId),
			DraftType:        2,
			DraftMessageData: hack.String(draft),
			Date2:            time.Now().Unix(),
		})
		if err != nil {
			c.Logger.Errorf("dialog.saveDraftMessage - insert: %v", err)
			return nil, err
		}
		if k := dialog.GetCacheKeyByPeerType(in.UserId, in.PeerType); k != "" {
			keys = append(keys, k)
		}
	}
	if _, _, err = c.svcCtx.Dao.ExecInvalidate(c.ctx, func(context.Context, *sqlx.DB) (int64, int64, error) {
		return 0, rows, nil
	}, keys...); err != nil {
		c.Logger.Errorf("dialog.saveDraftMessage - cache: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}

func draftCacheKeys(userID int64, peerType int32, peerID int64) []string {
	keys := []string{
		dialog.GetDialogCacheKeyByPeer(userID, peerType, peerID),
		dialog.GetAllDraftIdListCacheKey(userID),
	}
	if k := dialog.GetDraftCacheKey(userID, mtproto.MakePeerDialogId(peerType, peerID)); k != "" {
		keys = append(keys, k)
	}
	return keys
}
