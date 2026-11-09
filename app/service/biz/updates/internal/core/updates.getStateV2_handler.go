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
	"github.com/teamgram/teamgram-server/app/bff/phone/secretchat"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
)

// UpdatesGetStateV2
// updates.getStateV2 auth_key_id:long user_id:long = updates.State;
func (c *UpdatesCore) UpdatesGetStateV2(in *updates.TLUpdatesGetStateV2) (*mtproto.Updates_State, error) {
	pts := c.svcCtx.Dao.IDGenClient2.CurrentPtsId(c.ctx, in.UserId)
	if pts == 0 {
		pts = c.svcCtx.Dao.IDGenClient2.NextPtsId(c.ctx, in.UserId)
	}

	// seq 0 means this auth key has not stored a seq yet. A negative seq is
	// not a counter: clients compare it with their local seq and then reload
	// state from getPeerDialogs, which is what jumped the chat list.
	seq := nonNegativeSeq(c.svcCtx.Dao.IDGenClient2.CurrentSeqId(c.ctx, in.AuthKeyId))
	qts := c.svcCtx.Dao.IDGenClient2.CurrentQtsId(c.ctx, in.AuthKeyId)
	if queued := secretchat.Current(in.UserId); queued > qts {
		qts = queued
	}
	return mtproto.MakeTLUpdatesState(&mtproto.Updates_State{
		Pts:         pts,
		Qts:         qts,
		Seq:         seq,
		Date:        int32(time.Now().Unix()), // TODO(@benqi): do.Date2???
		UnreadCount: 0,
	}).To_Updates_State(), nil
}
