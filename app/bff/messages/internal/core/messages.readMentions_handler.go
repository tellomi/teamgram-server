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
	"github.com/teamgram/teamgram-server/app/bff/messages/searchbox"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesReadMentions
// messages.readMentions#36e5bf4d flags:# peer:InputPeer top_msg_id:flags.0?int = messages.AffectedHistory;
// https://core.telegram.org/method/messages.readMentions
//
// Marks unread mentions in the dialog as read. top_msg_id limits the call to
// one forum topic. offset is the number still unread when this call stops
// after one page, so the client invokes the method again.
func (c *MessagesCore) MessagesReadMentions(in *mtproto.TLMessagesReadMentions) (*mtproto.Messages_AffectedHistory, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	peer, err := c.peerOrInvalid(in.GetPeer())
	if err != nil {
		c.Logger.Errorf("messages.readMentions - error: %v", err)
		return nil, err
	}
	if peer.IsChannel() {
		if chat := channel.Shared().VisibleChat(c.ctx, c.MD.UserId, peer.PeerId); chat != nil && chat.GetLeft() {
			return nil, mtproto.ErrChannelPrivate
		}
	}
	topID := int32(0)
	if in.GetTopMsgId() != nil {
		topID = in.GetTopMsgId().GetValue()
		if topID < 0 {
			return nil, mtproto.ErrMsgIdInvalid
		}
	}
	rows, total := c.unreadMentions(peer)
	ids := searchbox.MentionIDs(rows, topID)
	if len(ids) > 100 {
		ids = ids[:100]
	}
	remaining := total - int32(len(ids))
	if remaining < 0 {
		remaining = 0
	}
	pts, ptsCount := c.markMentionsRead(peer, rows, ids)
	if remaining == 0 {
		c.clearMentionCounter(peer)
	}
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      pts,
		PtsCount: ptsCount,
		Offset:   remaining,
	}).To_Messages_AffectedHistory(), nil
}

func (c *MessagesCore) unreadMentions(peer *mtproto.PeerUtil) ([]searchbox.Row, int32) {
	if testMediaRows != nil {
		rows, _, _ := testMediaRows(peer.PeerType, peer.PeerId, searchbox.MediaKind{})
		return rows, int32(len(rows))
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, 0
	}
	got, err := c.svcCtx.Dao.MessageGetUnreadMentions(c.ctx, &messagepb.TLMessageGetUnreadMentions{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
		OffsetId: 0,
		Limit:    100,
	})
	if err != nil {
		c.Logger.Errorf("messages.readMentions - error: %v", err)
		return nil, 0
	}
	var rows []searchbox.Row
	for _, box := range got.GetDatas() {
		rows = append(rows, rowFromBox(box))
	}
	total := int32(len(rows))
	count, err := c.svcCtx.Dao.MessageGetUnreadMentionsCount(c.ctx, &messagepb.TLMessageGetUnreadMentionsCount{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
	})
	if err == nil && count != nil && count.GetV() > total {
		total = count.GetV()
	}
	return rows, total
}

func (c *MessagesCore) markMentionsRead(peer *mtproto.PeerUtil, rows []searchbox.Row, ids []int32) (pts, ptsCount int32) {
	if c.svcCtx != nil && c.svcCtx.Dao != nil {
		pts = c.svcCtx.Dao.CurrentPtsId(c.ctx, c.MD.UserId)
	}
	if len(ids) == 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MsgClient == nil {
		return pts, 0
	}
	want := map[int32]bool{}
	for _, id := range ids {
		want[id] = true
	}
	contents := make([]*msgpb.ContentMessage, 0, len(ids))
	for _, row := range rows {
		if !want[row.ID] || row.Message == nil {
			continue
		}
		contents = append(contents, &msgpb.ContentMessage{
			Id:         row.ID,
			SendUserId: row.Sender,
			Mentioned:  true,
		})
	}
	if len(contents) == 0 {
		return pts, int32(len(ids))
	}
	affected, err := c.svcCtx.Dao.MsgReadMessageContents(c.ctx, &msgpb.TLMsgReadMessageContents{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		Id:        contents,
	})
	if err != nil {
		c.Logger.Errorf("messages.readMentions - error: %v", err)
		return pts, int32(len(ids))
	}
	return affected.GetPts(), affected.GetPtsCount()
}

func (c *MessagesCore) clearMentionCounter(peer *mtproto.PeerUtil) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return
	}
	dlg, err := c.svcCtx.Dao.DialogGetDialogById(c.ctx, &dialog.TLDialogGetDialogById{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
	})
	unread, reactions := int32(0), int32(0)
	if err == nil && dlg != nil && dlg.GetDialog() != nil {
		unread = dlg.GetDialog().GetUnreadCount()
		reactions = dlg.GetDialog().GetUnreadReactionsCount()
	}
	_, err = c.svcCtx.Dao.DialogUpdateUnreadCount(c.ctx, &dialog.TLDialogUpdateUnreadCount{
		UserId:               c.MD.UserId,
		PeerType:             peer.PeerType,
		PeerId:               peer.PeerId,
		UnreadCount:          wrapperspb.Int32(unread),
		UnreadMentionsCount:  wrapperspb.Int32(0),
		UnreadReactionsCount: wrapperspb.Int32(reactions),
	})
	if err != nil {
		c.Logger.Errorf("messages.readMentions - counter: %v", err)
	}
}
