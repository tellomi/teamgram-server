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
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
)

// DialogGetAllDrafts
// dialog.getAllDrafts user_id:long = Vector<PeerWithDraftMessage>;
func (c *DialogCore) DialogGetAllDrafts(in *dialog.TLDialogGetAllDrafts) (*dialog.Vector_PeerWithDraftMessage, error) {
	rValues := &dialog.Vector_PeerWithDraftMessage{
		Datas: []*dialog.PeerWithDraftMessage{},
	}

	if _, err := c.svcCtx.Dao.DialogsDAO.SelectAllDraftsWithCB(
		c.ctx,
		in.UserId,
		func(sz, i int, v *dataobject.DialogsDO) {
			var err error
			rValues.Datas, err = appendStoredDraft(rValues.Datas, v)
			if err != nil {
				c.Logger.Errorf("dialog.getAllDrafts - peer (%d, %d): %v", v.PeerType, v.PeerId, err)
			}
		}); err != nil {
		c.Logger.Errorf("dialog.getAllDrafts - error: %v", err)
		return nil, err
	}

	return rValues, nil
}
