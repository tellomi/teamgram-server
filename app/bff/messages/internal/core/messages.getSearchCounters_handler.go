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
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessagesGetSearchCounters
// messages.getSearchCounters#732eef00 peer:InputPeer filters:Vector<MessagesFilter> = Vector<messages.SearchCounter>;
func (c *MessagesCore) MessagesGetSearchCounters(in *mtproto.TLMessagesGetSearchCounters) (*mtproto.Vector_Messages_SearchCounter, error) {
	var (
		peer = mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
	)
	counters := &mtproto.Vector_Messages_SearchCounter{
		Datas: make([]*mtproto.Messages_SearchCounter, 0, len(in.GetFilters())),
	}

	if in.SavedPeerId != nil {
		// TODO: not impl
		return counters, nil
	}

	// Channel and megagroup posts live in the channel store, not in the
	// caller's private message box. Counting the box always returned 0.
	if peer.IsChannel() {
		for _, filter := range in.GetFilters() {
			n, err := channel.Shared().CountByFilter(c.ctx, c.MD.UserId, peer.PeerId, filter)
			if err != nil {
				c.Logger.Errorf("messages.getSearchCounters - error: %v", err)
				return nil, err
			}
			counters.Datas = append(counters.Datas, mtproto.MakeTLMessagesSearchCounter(&mtproto.Messages_SearchCounter{
				Inexact: false,
				Filter:  filter,
				Count:   n,
			}).To_Messages_SearchCounter())
		}
		return counters, nil
	}

	for _, filter := range in.GetFilters() {
		/*
			{
			    "constructor": "CRC32_messages_getSearchCounters",
			    "peer": {
			        "predicate_name": "inputPeerChat",
			        "constructor": "CRC32_inputPeerChat",
			        "chat_id": 10053
			    },
			    "filters": [
			        {
			            "predicate_name": "inputMessagesFilterPhotoVideo",
			            "constructor": "CRC32_inputMessagesFilterPhotoVideo"
			        },
			        {
			            "predicate_name": "inputMessagesFilterDocument",
			            "constructor": "CRC32_inputMessagesFilterDocument"
			        },
			        {
			            "predicate_name": "inputMessagesFilterRoundVoice",
			            "constructor": "CRC32_inputMessagesFilterRoundVoice"
			        },
			        {
			            "predicate_name": "inputMessagesFilterUrl",
			            "constructor": "CRC32_inputMessagesFilterUrl"
			        },
			        {
			            "predicate_name": "inputMessagesFilterMusic",
			            "constructor": "CRC32_inputMessagesFilterMusic"
			        },
			        {
			            "predicate_name": "inputMessagesFilterGif",
			            "constructor": "CRC32_inputMessagesFilterGif"
			        }
			    ]
			}
		*/

		/*
		 */
		mType, ok := searchCounterMedia(filter)
		if !ok {
			counter := mtproto.MakeTLMessagesSearchCounter(&mtproto.Messages_SearchCounter{
				Inexact: false,
				Filter:  filter,
				Count:   0,
			}).To_Messages_SearchCounter()
			counters.Datas = append(counters.Datas, counter)
			continue
		}

		sz, err := c.svcCtx.Dao.MessageClient.MessageGetSearchCounter(
			c.ctx,
			&message.TLMessageGetSearchCounter{
				UserId:    c.MD.UserId,
				PeerType:  peer.PeerType,
				PeerId:    peer.PeerId,
				MediaType: mType,
			})
		if err != nil {
			c.Logger.Errorf("messages.getSearchCounters - error: %v", err)
			return nil, err
		}

		var count int32
		if sz != nil {
			count = sz.GetV()
		}
		counter := mtproto.MakeTLMessagesSearchCounter(&mtproto.Messages_SearchCounter{
			Inexact: false,
			Filter:  filter,
			Count:   count,
		}).To_Messages_SearchCounter()
		counters.Datas = append(counters.Datas, counter)
		c.Logger.Infof("messages.getSearchCounters - result: %s", counter)
	}

	return counters, nil
}
