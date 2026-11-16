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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

// DialogClearAllDrafts
// dialog.clearAllDrafts user_id:long = Vector<PeerWithDraftMessage>;
func (c *DialogCore) DialogClearAllDrafts(in *dialog.TLDialogClearAllDrafts) (*dialog.Vector_PeerWithDraftMessage, error) {
	var (
		err error

		rValues = &dialog.Vector_PeerWithDraftMessage{
			Datas: []*dialog.PeerWithDraftMessage{},
		}
	)

	if _, err = c.svcCtx.Dao.DialogsDAO.SelectAllDraftsWithCB(
		c.ctx,
		in.UserId,
		func(sz, i int, v *dataobject.DialogsDO) {
			rValues.Datas = append(rValues.Datas,
				dialog.MakeTLUpdateDraftMessage(&dialog.PeerWithDraftMessage{
					Peer: mtproto.MakePeer(v.PeerType, v.PeerId),
					Draft: mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{
						Date_FLAGINT32: mtproto.MakeFlagsInt32(int32(time.Now().Unix())),
					}).To_DraftMessage(),
				}).To_PeerWithDraftMessage())
		}); err != nil {
		c.Logger.Errorf("dialog.getAllDrafts - error: %v", err)
		return nil, err
	}

	topics, err := c.loadTopicDrafts(in.UserId)
	if err != nil {
		c.Logger.Errorf("dialog.clearAllDrafts - topics: %v", err)
		return nil, err
	}
	for _, item := range topics {
		top := dialog.DraftTopicID(item.GetDraft())
		if top <= 0 {
			continue
		}
		rValues.Datas = append(rValues.Datas, dialog.MakeTLUpdateDraftMessage(&dialog.PeerWithDraftMessage{
			Peer:  item.GetPeer(),
			Draft: emptyTopicDraft(top),
		}).To_PeerWithDraftMessage())
	}

	if len(rValues.Datas) > 0 {
		_, err = c.svcCtx.Dao.DialogsDAO.ClearAllDrafts(c.ctx, in.UserId)
		if err != nil {
			c.Logger.Errorf("dialog.clearAllDrafts - error: %v", err)
			return nil, err
		}
		if c.topicDraftsReady() {
			if err = c.svcCtx.Dao.TopicDraftsDAO.ClearByUser(c.ctx, in.UserId); err != nil {
				c.Logger.Errorf("dialog.clearAllDrafts - topics: %v", err)
				return nil, err
			}
		}
	}

	return rValues, nil
}
