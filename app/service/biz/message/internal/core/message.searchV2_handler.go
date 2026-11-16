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
	"math"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

/*
{
    "constructor": "CRC32_messages_search",
    "peer": {
        "predicate_name": "inputPeerChat",
        "constructor": "CRC32_inputPeerChat",
        "chat_id": "10053"
    },
    "from_id": {
        "predicate_name": "inputPeerUser",
        "constructor": "CRC32_inputPeerUser",
        "user_id": "136817689",
        "access_hash": "4630984627545308247"
    },
    "filter": {
        "predicate_name": "inputMessagesFilterEmpty",
        "constructor": "CRC32_inputMessagesFilterEmpty"
    },
    "offset_id": 94,
    "limit": 21
}
*/

// MessageSearchV2
// message.searchV2 user_id:long peer_type:int peer_id:long q:string from_id:long min_date:int max_date:int offset_id:int add_offset:int limit:int max_id:int min_id:int hash:long = Vector<MessageBox>;
func (c *MessageCore) MessageSearchV2(in *message.TLMessageSearchV2) (*mtproto.MessageBoxList, error) {
	if in.FromId == 0 {
		if in.MinDate != 0 || in.MaxDate != 0 {
			return c.searchDialogDated(in)
		}
		return c.MessageSearch(&message.TLMessageSearch{
			UserId:   in.UserId,
			PeerType: in.PeerType,
			PeerId:   in.PeerId,
			Q:        in.Q,
			Offset:   in.OffsetId,
			Limit:    in.Limit,
		})
	}

	// from_id used to ignore q and the date bounds and count every message
	// from that sender. The page and the total now use one filter.
	return c.searchDialogFiltered(in, in.FromId, true)
}

func (c *MessageCore) searchDialogDated(in *message.TLMessageSearchV2) (*mtproto.MessageBoxList, error) {
	return c.searchDialogFiltered(in, 0, false)
}

// searchDialogFiltered reads one private chat or basic group. withCount is
// the from_id path: the total uses the same sender, words, dates, dialog,
// and deleted filter as the page. Offset only turns the page.
func (c *MessageCore) searchDialogFiltered(in *message.TLMessageSearchV2, sender int64, withCount bool) (*mtproto.MessageBoxList, error) {
	switch in.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT:
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
	offset := in.OffsetId
	if offset == 0 {
		offset = math.MaxInt32
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	dialogId := mtproto.MakeDialogId(in.UserId, in.PeerType, in.PeerId)
	rows, err := c.svcCtx.Dao.MessagesDAO.SearchDialogText(
		c.ctx, in.UserId, dialogId.A, dialogId.B, offset, in.Q, sender, in.MinDate, in.MaxDate, limit)
	if err != nil {
		return nil, err
	}
	boxList := make([]*mtproto.MessageBox, 0, len(rows))
	for i := range rows {
		boxList = append(boxList, c.svcCtx.Dao.MakeMessageBox(c.ctx, in.UserId, &rows[i]))
	}
	out := &mtproto.MessageBoxList{BoxList: boxList}
	if !withCount {
		return mtproto.MakeTLMessageBoxList(out).To_MessageBoxList(), nil
	}
	count, err := c.svcCtx.Dao.MessagesDAO.CountDialogText(
		c.ctx, in.UserId, dialogId.A, dialogId.B, in.Q, sender, in.MinDate, in.MaxDate)
	if err != nil {
		return nil, err
	}
	out.Count = count
	return mtproto.MakeTLMessageBoxListSlice(out).To_MessageBoxList(), nil
}
