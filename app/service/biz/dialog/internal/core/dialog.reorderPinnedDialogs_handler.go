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

// DialogReorderPinnedDialogs
// dialog.reorderPinnedDialogs user_id:long force:Bool folder_id:int id_list:Vector<long> = Bool;
func (c *DialogCore) DialogReorderPinnedDialogs(in *dialog.TLDialogReorderPinnedDialogs) (*mtproto.Bool, error) {
	var (
		userId      = in.GetUserId()
		force       = mtproto.FromBool(in.GetForce())
		folderId    = in.GetFolderId()
		idList      = in.GetIdList()
		orderPinned = time.Now().Unix()
		keyList     = make([]string, 0, len(idList)+1)
	)
	for _, id := range idList {
		keyList = append(keyList, dialog.GetDialogCacheKey(in.GetUserId(), id))
	}

	var (
		current []int64
		listErr error
	)
	if folderId == 0 {
		current, listErr = c.svcCtx.Dao.GetNoCachePinnedDialogIdList(c.ctx, userId)
	} else {
		current, listErr = c.svcCtx.Dao.GetNoCacheFolderPinnedDialogIdList(c.ctx, userId)
	}
	if listErr != nil {
		c.Logger.Errorf("dialog.reorderPinnedDialogs - error: %v", listErr)
		return nil, listErr
	}
	if err := reorderPinsRejected(force, current, idList, pinnedDialogLimit(folderId)); err != nil {
		return nil, err
	}

	if folderId == 0 {
		c.svcCtx.Dao.ExecInvalidate(
			c.ctx,
			func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
				tR := sqlx.TxWrapper(
					ctx,
					conn,
					func(tx *sqlx.Tx, result *sqlx.StoreResult) {
						if force {
							if len(idList) > 0 {
								_, result.Err = c.svcCtx.Dao.DialogsDAO.UpdateUnPinnedNotIdListTx(
									tx,
									userId,
									idList)
							} else {
								_, result.Err = c.svcCtx.Dao.DialogsDAO.UpdateUnPinnedNotIdListTx(
									tx,
									userId,
									[]int64{0}) // hack
							}
							if result.Err != nil {
								return
							}
						}

						for _, id := range idList {
							_, result.Err = c.svcCtx.Dao.DialogsDAO.UpdatePeerDialogListPinnedTx(
								tx,
								orderPinned<<32,
								in.UserId, []int64{id})
							if result.Err != nil {
								return
							}
							orderPinned -= 1
						}
					})
				return 0, 0, tR.Err
			},
			append(keyList, dialog.GetPinnedDialogIdListCacheKey(in.GetUserId()))...)
	} else {
		c.svcCtx.Dao.ExecInvalidate(
			c.ctx,
			func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
				tR := sqlx.TxWrapper(
					c.ctx,
					c.svcCtx.Dao.DB,
					func(tx *sqlx.Tx, result *sqlx.StoreResult) {
						if force {
							unpin := idList
							if len(unpin) == 0 {
								unpin = []int64{0} // hack: not in (0) clears the folder
							}
							_, result.Err = c.svcCtx.Dao.DialogsDAO.UpdateFolderUnPinnedNotIdListTx(
								tx,
								userId,
								unpin)
							if result.Err != nil {
								return
							}
						}

						for _, id := range idList {
							_, result.Err = c.svcCtx.Dao.DialogsDAO.UpdateFolderPeerDialogListPinnedTx(
								tx,
								orderPinned<<32,
								in.UserId,
								[]int64{id})
							if result.Err != nil {
								return
							}
							orderPinned -= 1
						}
					})
				return 0, 0, tR.Err
			},
			append(keyList, dialog.GetFolderPinnedDialogIdListCacheKey(in.GetUserId()))...)
	}

	return mtproto.BoolTrue, nil
}
