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
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// MessagesDeleteHistory
// messages.deleteHistory#b08f922a flags:# just_clear:flags.0?true revoke:flags.1?true peer:InputPeer max_id:int min_date:flags.2?int max_date:flags.3?int = messages.AffectedHistory;
func (c *MessagesCore) MessagesDeleteHistory(in *mtproto.TLMessagesDeleteHistory) (*mtproto.Messages_AffectedHistory, error) {
	var (
		peer = mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	)

	minDate, maxDate, hasDate, err := deleteHistoryDates(in)
	if err != nil {
		c.Logger.Errorf("messages.deleteHistory - error: %v", err)
		return nil, err
	}
	// Official clients only send min_date / max_date for private chats.
	if hasDate && !peer.IsUser() {
		c.Logger.Errorf("messages.deleteHistory - error: %v", mtproto.ErrChatRevokeDateUnsupported)
		return nil, mtproto.ErrChatRevokeDateUnsupported
	}

	if peer.IsChannel() {
		return c.deleteChannelHistory(peer, in)
	}

	if !peer.IsChatOrUser() {
		err = mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.deleteHistory - error: %v", err)
		return nil, err
	}

	affectedHistory, err := c.svcCtx.Dao.MsgClient.MsgDeleteHistory(c.ctx, &msgpb.TLMsgDeleteHistory{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		JustClear: in.GetJustClear(),
		Revoke:    in.Revoke,
		MaxId:     in.MaxId,
		MinDate:   minDate,
		MaxDate:   maxDate,
	})

	if err != nil {
		c.Logger.Errorf("messages.deleteHistory - error: %v", err)
		return nil, err
	}

	// A date range leaves the chat in place. A full delete removes the dialog
	// only after the last slice (offset 0); earlier slices still have messages.
	if !in.GetJustClear() && !hasDate && affectedHistory.GetOffset() == 0 && peer.IsUser() {
		c.svcCtx.Dao.DialogDeleteDialog(c.ctx, &dialog.TLDialogDeleteDialog{
			UserId:   c.MD.UserId,
			PeerType: peer.PeerType,
			PeerId:   peer.PeerId,
		})
		if in.Revoke && !peer.IsSelf() {
			c.svcCtx.Dao.DialogDeleteDialog(c.ctx, &dialog.TLDialogDeleteDialog{
				UserId:   peer.PeerId,
				PeerType: peer.PeerType,
				PeerId:   c.MD.UserId,
			})
		}
	}

	return affectedHistory, nil
}

// deleteHistoryDates reads the optional inclusive range.
// A missing flag or a zero value is an open bound.
func deleteHistoryDates(in *mtproto.TLMessagesDeleteHistory) (minDate, maxDate int32, hasDate bool, err error) {
	if in.GetMinDate() != nil {
		minDate = in.GetMinDate().GetValue()
		hasDate = true
		if minDate < 0 {
			return 0, 0, true, mtproto.ErrMinDateInvalid
		}
	}
	if in.GetMaxDate() != nil {
		maxDate = in.GetMaxDate().GetValue()
		hasDate = true
		if maxDate < 0 {
			return 0, 0, true, mtproto.ErrMaxDateInvalid
		}
	}
	if minDate > 0 && maxDate > 0 && minDate > maxDate {
		return 0, 0, true, mtproto.ErrMaxDateInvalid
	}
	return minDate, maxDate, hasDate, nil
}

func (c *MessagesCore) deleteChannelHistory(peer *mtproto.PeerUtil, in *mtproto.TLMessagesDeleteHistory) (*mtproto.Messages_AffectedHistory, error) {
	input := &mtproto.InputChannel{
		PredicateName: mtproto.Predicate_inputChannel,
		ChannelId:     peer.PeerId,
		AccessHash:    peer.AccessHash,
	}
	var (
		pts, count int32
		err        error
	)
	if in.GetMinDate() != nil || in.GetMaxDate() != nil {
		pts, count, err = channel.Shared().DeleteByDate(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, input, in.GetMinDate().GetValue(), in.GetMaxDate().GetValue())
	} else {
		pts, count, err = channel.Shared().DeleteHistory(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, input, in.GetMaxId(), in.GetRevoke())
		if err == mtproto.ErrChatAdminRequired && in.GetRevoke() {
			pts, count, err = channel.Shared().DeleteHistory(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, input, in.GetMaxId(), false)
		}
	}
	if err != nil {
		c.Logger.Errorf("messages.deleteHistory - error: %v", err)
		return nil, err
	}
	if !in.GetJustClear() {
		_, _ = channel.Shared().Leave(c.ctx, c.MD.UserId, c.MD.PermAuthKeyId, input)
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      pts,
		PtsCount: count,
		Offset:   0,
	}).To_Messages_AffectedHistory(), nil
}
