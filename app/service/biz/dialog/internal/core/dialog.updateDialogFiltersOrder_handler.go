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
	"encoding/json"
	"fmt"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// dialogFilterDefaultJSON is the dialog_filters.dialog_filter value for id 0.
// getDialogFilters turns that row back into dialogFilterDefault.
func dialogFilterDefaultJSON() (string, error) {
	raw, err := jsonx.Marshal(mtproto.MakeTLDialogFilterDefault(nil).To_DialogFilter())
	if err != nil {
		return "", err
	}
	if !json.Valid(raw) {
		return "", fmt.Errorf("dialogFilterDefault is not json")
	}
	return string(raw), nil
}

// DialogUpdateDialogFiltersOrder
// dialog.updateDialogFiltersOrder user_id:long order:Vector<long> = Bool;
//
// Id 0 is dialogFilterDefault. UPDATE matches no row until one exists, so the
// All-chats position is inserted (or revived, if a deleted id-0 row is there).
// Other ids keep the previous UPDATE, which leaves unknown ids untouched.
func (c *DialogCore) DialogUpdateDialogFiltersOrder(in *dialog.TLDialogUpdateDialogFiltersOrder) (*mtproto.Bool, error) {
	steps := dialog.FilterOrderSteps(in.GetOrder(), time.Now().Unix())
	var raw string
	for _, step := range steps {
		if !step.UpsertDefault {
			continue
		}
		var err error
		raw, err = dialogFilterDefaultJSON()
		if err != nil {
			c.Logger.Errorf("dialog.updateDialogFiltersOrder - default: %v", err)
			return nil, err
		}
		break
	}

	_, _, err := c.svcCtx.Dao.CachedConn.Exec(
		c.ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			var writeErr error
			tR := sqlx.TxWrapper(
				ctx,
				conn,
				func(tx *sqlx.Tx, result *sqlx.StoreResult) {
					for _, step := range steps {
						if step.UpsertDefault {
							_, _, writeErr = c.svcCtx.DialogFiltersDAO.InsertOrUpdateTx(tx, &dataobject.DialogFiltersDO{
								UserId:         in.UserId,
								DialogFilterId: dialog.DialogFilterDefaultID,
								DialogFilter:   raw,
								OrderValue:     step.OrderValue,
							})
						} else {
							_, writeErr = c.svcCtx.DialogFiltersDAO.UpdateOrderTx(tx, step.OrderValue, in.UserId, step.ID)
						}
						if writeErr != nil {
							result.Err = writeErr
							return
						}
					}
				})

			return 0, 0, tR.Err
		},
		dialog.GetDialogFilterCacheKey(in.UserId))
	if err != nil {
		c.Logger.Errorf("dialog.updateDialogFiltersOrder - error: %v", err)
		return nil, err
	}

	return mtproto.BoolTrue, nil
}
