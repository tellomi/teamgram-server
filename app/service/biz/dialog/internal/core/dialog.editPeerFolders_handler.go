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
	"sort"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
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
	if _, err := c.svcCtx.Dao.DialogsDAO.UpdatePeerDialogListFolderId(c.ctx, in.FolderId, in.UserId, in.PeerDialogList); err != nil {
		c.Logger.Errorf("dialog.editPeerFolders - error: %v", err)
		return nil, err
	}

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

	// getDialogs reads these lists and the dialog row from cache. Without
	// dropping them, a chat stays in the main list until the entry expires.
	// CachedConn.Exec deletes the keys whenever the callback returns a nil
	// error. The rows-affected count is not consulted, so (0, 0, nil) evicts
	// the same way (0, 1, nil) does. This runs after the pin trim so a
	// following read cannot cache the pre-trim pin list.
	c.svcCtx.Dao.ExecInvalidate(
		c.ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			return 0, 0, nil
		},
		dialogCacheKeys(in.UserId, in.PeerDialogList)...)

	return &dialog.Vector_DialogPinnedExt{
		Datas: dialogPinnedList,
	}, nil
}

type userPremiumRow struct {
	Premium           int   `db:"premium"`
	PremiumExpireDate int64 `db:"premium_expire_date"`
}

func dialogCacheKeys(userID int64, peerDialogIDs []int64) []string {
	keys := make([]string, 0, len(peerDialogIDs)+4)
	for _, id := range peerDialogIDs {
		keys = append(keys, dialog.GetDialogCacheKey(userID, id))
	}
	keys = append(keys,
		dialog.GetPinnedDialogIdListCacheKey(userID),
		dialog.GetNotPinnedDialogIdListCacheKey(userID),
		dialog.GetFolderPinnedDialogIdListCacheKey(userID),
		dialog.GetFolderNotPinnedDialogIdListCacheKey(userID),
	)
	return keys
}

func accountMember(c *DialogCore, userID int64) bool {
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
	return member
}

func pinnedCap(c *DialogCore, userID int64) int {
	return pinnedCapFor(c, userID, 0)
}

// pinnedCapFor is the main-list cap in folder 0 and the archive cap in folder 1.
// Both follow premium.Limit, including TELLOMI_PREMIUM_EVERYONE.
func pinnedCapFor(c *DialogCore, userID int64, folderID int32) int {
	feature := premium.FeaturePinnedChats
	if folderID == 1 {
		feature = premium.FeatureFolderPins
	}
	return premium.Limit(feature, accountMember(c, userID))
}
