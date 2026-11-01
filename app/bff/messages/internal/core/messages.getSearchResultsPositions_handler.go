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
)

// MessagesGetSearchResultsPositions
// messages.getSearchResultsPositions#9c7f2f10 flags:# peer:InputPeer saved_peer_id:flags.2?InputPeer filter:MessagesFilter offset_id:int limit:int = messages.SearchResultsPositions;
// https://core.telegram.org/method/messages.getSearchResultsPositions
//
// Returns sparse scrollbar positions for one media filter, newest first.
// offset is the index from the newest match. Empty and mention filters are rejected.
func (c *MessagesCore) MessagesGetSearchResultsPositions(in *mtproto.TLMessagesGetSearchResultsPositions) (*mtproto.Messages_SearchResultsPositions, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	peer, err := c.peerOrInvalid(in.GetPeer())
	if err != nil {
		c.Logger.Errorf("messages.getSearchResultsPositions - error: %v", err)
		return nil, err
	}
	kind, err := searchbox.MediaFor(in.GetFilter())
	if err != nil {
		c.Logger.Errorf("messages.getSearchResultsPositions - error: %v", err)
		return nil, err
	}
	rows, _, err := c.scanMedia(peer, kind, c.savedPeer(in.GetSavedPeerId()))
	if err != nil {
		c.Logger.Errorf("messages.getSearchResultsPositions - error: %v", err)
		return nil, err
	}
	count, samples := searchbox.BuildPositions(rows, in.GetOffsetId(), in.GetLimit())
	positions := make([]*mtproto.SearchResultsPosition, 0, len(samples))
	for _, sample := range samples {
		positions = append(positions, mtproto.MakeTLSearchResultPosition(&mtproto.SearchResultsPosition{
			MsgId:  sample.MsgID,
			Date:   sample.Date,
			Offset: sample.Offset,
		}).To_SearchResultsPosition())
	}
	return mtproto.MakeTLMessagesSearchResultsPositions(&mtproto.Messages_SearchResultsPositions{
		Count:     count,
		Positions: positions,
	}).To_Messages_SearchResultsPositions(), nil
}
