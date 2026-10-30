// Copyright 2024 Teamgram Authors
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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessageGetOutboxReadDate
// message.getOutboxReadDate user_id:long peer_type:int peer_id:long msg_id:int = Vector<ReadParticipantDate>;
func (c *MessageCore) MessageGetOutboxReadDate(in *message.TLMessageGetOutboxReadDate) (*message.Vector_ReadParticipantDate, error) {
	out := &message.Vector_ReadParticipantDate{
		Datas: []*mtproto.ReadParticipantDate{},
	}
	if in.MsgId <= 0 || in.PeerId == 0 {
		return out, nil
	}

	owner := in.UserId
	oneReader := false
	reader := int64(0)
	switch in.PeerType {
	case mtproto.PEER_USER:
		oneReader = true
		reader = in.PeerId
	case mtproto.PEER_CHAT:
	case mtproto.PEER_CHANNEL:
		// Channel checkpoints are stored under the channel id. The request
		// user_id is the member asking, not the outbox owner.
		owner = in.PeerId
	default:
		return out, nil
	}
	if owner == 0 {
		return out, nil
	}

	hits, err := c.svcCtx.Dao.MessageReadOutboxDAO.SelectOutboxReadHits(
		c.ctx,
		owner,
		mtproto.MakePeerDialogId(in.PeerType, in.PeerId),
		reader,
		in.MsgId,
		oneReader,
	)
	if err != nil {
		c.Logger.Errorf("message.getOutboxReadDate - error: %v", err)
		return nil, err
	}
	mapped := make([]readHit, 0, len(hits))
	for _, h := range hits {
		mapped = append(mapped, readHit{UserID: h.ReadUserId, Date: h.ReadDate})
	}
	out.Datas = datesFromHits(mapped, in.UserId)
	return out, nil
}
