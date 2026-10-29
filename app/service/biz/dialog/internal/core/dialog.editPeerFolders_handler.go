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
	"sort"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

// DialogEditPeerFolders
// dialog.editPeerFolders user_id:long peer_dialog_list:Vector<long> folder_id:int = Vector<DialogPinnedExt>;
func (c *DialogCore) DialogEditPeerFolders(in *dialog.TLDialogEditPeerFolders) (*dialog.Vector_DialogPinnedExt, error) {
	var (
		dialogPinnedList dialog.DialogPinnedExtList
	)

	c.svcCtx.Dao.DialogsDAO.SelectPeerDialogListWithCB(c.ctx,
		in.UserId,
		in.PeerDialogList,
		func(sz, i int, v *dataobject.DialogsDO) {
			if in.FolderId == 0 {
				if v.Pinned > 0 {
					dialogPinnedList = append(dialogPinnedList, &dialog.DialogPinnedExt{
						Order:    v.Pinned,
						PeerType: v.PeerType,
						PeerId:   v.PeerId,
					})
				}
			} else {
				if v.FolderPinned > 0 {
					dialogPinnedList = append(dialogPinnedList, &dialog.DialogPinnedExt{
						Order:    v.FolderPinned,
						PeerType: v.PeerType,
						PeerId:   v.PeerId,
					})
				}
			}
		})

	if len(dialogPinnedList) > 0 {
		if in.FolderId == 0 {
			c.svcCtx.Dao.DialogsDAO.SelectPinnedDialogsWithCB(c.ctx,
				in.UserId,
				func(sz, i int, v *dataobject.DialogsDO) {
					dialogPinnedList = append(dialogPinnedList, &dialog.DialogPinnedExt{
						Order:    v.FolderPinned,
						PeerType: v.PeerType,
						PeerId:   v.PeerId,
					})
				})
		} else {
			c.svcCtx.Dao.DialogsDAO.SelectFolderPinnedDialogsWithCB(c.ctx,
				in.UserId,
				func(sz, i int, v *dataobject.DialogsDO) {
					dialogPinnedList = append(dialogPinnedList, &dialog.DialogPinnedExt{
						Order:    v.FolderPinned,
						PeerType: v.PeerType,
						PeerId:   v.PeerId,
					})
				})
		}
	}

	sd := sort.Reverse(dialogPinnedList)
	sort.Sort(sd)

	// update
	c.svcCtx.Dao.DialogsDAO.UpdatePeerDialogListFolderId(c.ctx, in.FolderId, in.UserId, in.PeerDialogList)

	if in.FolderId == 0 {
		cap := pinnedCap(c, in.UserId)
		if len(dialogPinnedList) > cap {
			unpinnedList := make([]int64, 0, len(dialogPinnedList)-cap)
			for i := cap; i < len(dialogPinnedList); i++ {
				unpinnedList = append(unpinnedList, mtproto.MakePeerDialogId(dialogPinnedList[i].PeerType, dialogPinnedList[i].PeerId))
			}

			c.svcCtx.Dao.DialogsDAO.UpdatePeerDialogListPinned(c.ctx, 0, in.UserId, unpinnedList)
			dialogPinnedList = dialogPinnedList[:cap]
		}
	}

	return &dialog.Vector_DialogPinnedExt{
		Datas: dialogPinnedList,
	}, nil
}

type userPremiumRow struct {
	Premium           int   `db:"premium"`
	PremiumExpireDate int64 `db:"premium_expire_date"`
}

func pinnedCap(c *DialogCore, userID int64) int {
	now := time.Now().Unix()
	member := premium.IsMember(false, 0, now)
	if c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.DB != nil {
		var row userPremiumRow
		err := c.svcCtx.Dao.DB.QueryRowPartial(c.ctx, &row,
			"select premium, premium_expire_date from users where id = ? limit 1", userID)
		if err == nil {
			member = premium.IsMember(row.Premium != 0, row.PremiumExpireDate, now)
		}
	}
	return premium.Limit(premium.FeaturePinnedChats, member)
}
