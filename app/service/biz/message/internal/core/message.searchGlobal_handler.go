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
	"github.com/teamgram/teamgram-server/pkg/searchorder"
)

// MessageSearchGlobal
// message.searchGlobal user_id:long q:string offset:int limit:int = Vector<MessageBox>;
func (c *MessageCore) MessageSearchGlobal(in *message.TLMessageSearchGlobal) (*mtproto.MessageBoxList, error) {
	var rValues []*mtproto.MessageBox
	cur, active := searchorder.FromContext(c.ctx)
	_, err := c.svcCtx.Dao.MessagesDAO.SearchGlobalTextWithCB(
		c.ctx,
		in.UserId,
		cur,
		active,
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
	n := c.globalTextCount(in.GetUserId(), in.GetQ())
	return mtproto.MakeTLMessageBoxList(&mtproto.MessageBoxList{
		BoxList: rValues,
		Count:   n,
	}).To_MessageBoxList(), nil
}

// globalTextCount is every matching user or basic-group row, ignoring the
// page cursor. A failed count is MaxInt32 so a client that stops when the
// loaded list reaches count does not stop early.
func (c *MessageCore) globalTextCount(userID int64, q string) int32 {
	users, groups, broadcasts, limited := searchorder.PeerScope(c.ctx)
	if limited && broadcasts && !users && !groups {
		return 0
	}
	restrict := limited && (users || groups)
	n, err := c.svcCtx.Dao.MessagesDAO.CountGlobalText(c.ctx, userID, q, restrict, users, groups)
	if err != nil {
		return math.MaxInt32
	}
	return n
}
