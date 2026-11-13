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
	"math"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageSearchGlobal
// message.searchGlobal user_id:long q:string offset:int limit:int = Vector<MessageBox>;
func (c *MessageCore) MessageSearchGlobal(in *message.TLMessageSearchGlobal) (*mtproto.MessageBoxList, error) {
	var (
		offset  = in.Offset
		rValues []*mtproto.MessageBox
	)

	// Offset is the global cursor date (offset_rate), not a box id.
	// A channel message id must not become user_message_box_id < ?.
	// Zero means the first page: every stored date is allowed.
	maxDate := int64(offset)
	if maxDate <= 0 {
		maxDate = math.MaxInt32
	}

	_, err := c.svcCtx.Dao.MessagesDAO.SearchGlobalTextWithCB(
		c.ctx,
		in.UserId,
		maxDate,
		in.Q,
		in.Limit,
		func(sz, i int, v *dataobject.MessagesDO) {
			rValues = append(rValues, c.svcCtx.Dao.MakeMessageBox(c.ctx, in.UserId, v))
		})
	if err != nil {
		return nil, err
	}

	if rValues == nil {
		rValues = []*mtproto.MessageBox{}
	}

	return mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{
		BoxList: rValues,
	}).To_MessageBoxList(), nil
}
