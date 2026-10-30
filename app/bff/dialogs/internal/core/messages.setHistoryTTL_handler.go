// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesSetHistoryTTL
// messages.setHistoryTTL#b80e5fe4 peer:InputPeer period:int = Updates;
//
// https://core.telegram.org/method/messages.setHistoryTTL
// period is the new auto-delete timer in seconds. 0 disables it.
// Values outside 0..366 days are TTL_PERIOD_INVALID, same as
// messages.setDefaultHistoryTTL.
func (c *DialogsCore) MessagesSetHistoryTTL(in *mtproto.TLMessagesSetHistoryTTL) (*mtproto.Updates, error) {
	period := in.GetPeriod()
	if period < 0 || period > 366*86400 {
		return nil, mtproto.ErrTtlPeriodInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		return channel.Shared().SetHistoryTTL(c.ctx, c.MD.UserId, peer.PeerId, period)
	}
	if _, err := c.svcCtx.Dao.DialogClient.DialogSetHistoryTTL(c.ctx, &dialog.TLDialogSetHistoryTTL{
		UserId:    c.MD.UserId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		TtlPeriod: period,
	}); err != nil {
		c.Logger.Errorf("messages.setHistoryTTL - error: %v", err)
		return nil, err
	}
	var ttl *wrapperspb.Int32Value
	if period > 0 {
		ttl = &wrapperspb.Int32Value{Value: period}
	}
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePeerHistoryTTL(&mtproto.Update{
		Peer_PEER: peer.ToPeer(),
		TtlPeriod: ttl,
	}).To_Update()), nil
}
