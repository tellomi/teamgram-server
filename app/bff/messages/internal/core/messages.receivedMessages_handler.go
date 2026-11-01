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
	"github.com/teamgram/teamgram-server/pkg/notifyack"
)

// MessagesReceivedMessages
// messages.receivedMessages#5a954c0 max_id:int = Vector<ReceivedNotifyMessage>;
// https://core.telegram.org/method/messages.receivedMessages
//
// Confirms the client has the messages up to max_id and drops them from the
// pending push list that sync.pushUpdates recorded.
func (c *MessagesCore) MessagesReceivedMessages(in *mtproto.TLMessagesReceivedMessages) (*mtproto.Vector_ReceivedNotifyMessage, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	ids, err := notifyack.Shared().Take(c.ctx, c.MD.UserId, in.GetMaxId())
	if err != nil {
		c.Logger.Errorf("messages.receivedMessages - error: %v", err)
		ids = nil
	}
	out := &mtproto.Vector_ReceivedNotifyMessage{
		Datas: make([]*mtproto.ReceivedNotifyMessage, 0, len(ids)),
	}
	for _, id := range ids {
		out.Datas = append(out.Datas, mtproto.MakeTLReceivedNotifyMessage(&mtproto.ReceivedNotifyMessage{
			Id:    id,
			Flags: 0,
		}).To_ReceivedNotifyMessage())
	}
	return out, nil
}
