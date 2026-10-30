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
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
)

// DialogInsertOrUpdateDialogFilter
// dialog.insertOrUpdateDialogFilter user_id:long id:int dialog_filter:DialogFilter = Bool;
func (c *DialogCore) DialogInsertOrUpdateDialogFilter(in *dialog.TLDialogInsertOrUpdateDialogFilter) (*mtproto.Bool, error) {
	filter := in.GetDialogFilter()
	if filter == nil {
		return nil, mtproto.ErrFilterIdInvalid
	}

	existing, selErr := c.svcCtx.Dao.DialogFiltersDAO.Select(c.ctx, in.UserId, in.Id)
	if selErr != nil {
		c.Logger.Errorf("dialog.insertOrUpdateDialogFilter - select error: %v", selErr)
	}
	orderValue := time.Now().Unix() << 32
	slug := ""
	joinedBySlug := false
	if existing != nil {
		orderValue = existing.OrderValue
		slug = existing.Slug
		joinedBySlug = existing.JoinedBySlug
	} else if selErr == nil {
		// New folders sort after the ones already stored (list is order_value DESC).
		found := false
		var minOrder int64
		_, listErr := c.svcCtx.Dao.DialogFiltersDAO.SelectListWithCB(
			c.ctx,
			in.UserId,
			func(sz, i int, v *dataobject.DialogFiltersDO) {
				if !found || v.OrderValue < minOrder {
					minOrder = v.OrderValue
					found = true
				}
			})
		if listErr != nil {
			c.Logger.Errorf("dialog.insertOrUpdateDialogFilter - list error: %v", listErr)
		} else if found {
			orderValue = minOrder - 1
		}
	}
	if existing != nil && existing.DialogFilter != "" && existing.DialogFilter != "null" {
		if old, err := mtproto.UnmarshalDialogFilter(existing.DialogFilter); err == nil {
			mtproto.PreserveChatlistBlob(filter, old)
		}
	}

	dialogFilterData, err := jsonx.Marshal(filter)
	if err != nil {
		c.Logger.Errorf("dialog.insertOrUpdateDialogFilter - error: %v", err)
		return nil, err
	}
	isChatlist := filter.GetPredicateName() == mtproto.Predicate_dialogFilterChatlist

	c.svcCtx.Dao.CachedConn.Exec(
		c.ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			_, _, err2 := c.svcCtx.Dao.DialogFiltersDAO.InsertOrUpdate(
				ctx,
				&dataobject.DialogFiltersDO{
					UserId:         in.UserId,
					DialogFilterId: in.Id,
					IsChatlist:     isChatlist,
					JoinedBySlug:   joinedBySlug,
					Slug:           slug,
					DialogFilter:   string(dialogFilterData),
					OrderValue:     orderValue,
					FromSuggested:  -1,
					Deleted:        false,
				})

			return 0, 0, err2
		},
		dialog.GetDialogFilterCacheKey(in.UserId))

	return mtproto.BoolTrue, nil
}
