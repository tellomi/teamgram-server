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

// MessagesGetSearchResultsCalendar
// messages.getSearchResultsCalendar#6aa3f6bd flags:# peer:InputPeer saved_peer_id:flags.2?InputPeer filter:MessagesFilter offset_id:int offset_date:int = messages.SearchResultsCalendar;
// https://core.telegram.org/method/messages.getSearchResultsCalendar
//
// Groups matching messages into UTC days, newest day first. Each day carries
// its count and id range, plus the newest message that day as a preview.
// inputMessagesFilterEmpty and inputMessagesFilterMyMentions are rejected.
func (c *MessagesCore) MessagesGetSearchResultsCalendar(in *mtproto.TLMessagesGetSearchResultsCalendar) (*mtproto.Messages_SearchResultsCalendar, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	peer, err := c.peerOrInvalid(in.GetPeer())
	if err != nil {
		c.Logger.Errorf("messages.getSearchResultsCalendar - error: %v", err)
		return nil, err
	}
	kind, err := searchbox.MediaFor(in.GetFilter())
	if err != nil {
		c.Logger.Errorf("messages.getSearchResultsCalendar - error: %v", err)
		return nil, err
	}
	rows, truncated, err := c.scanMedia(peer, kind, c.savedPeer(in.GetSavedPeerId()))
	if err != nil {
		c.Logger.Errorf("messages.getSearchResultsCalendar - error: %v", err)
		return nil, err
	}
	cal := searchbox.BuildCalendar(rows, in.GetOffsetId(), in.GetOffsetDate(), truncated)
	periods := make([]*mtproto.SearchResultsCalendarPeriod, 0, len(cal.Periods))
	for _, period := range cal.Periods {
		periods = append(periods, mtproto.MakeTLSearchResultsCalendarPeriod(&mtproto.SearchResultsCalendarPeriod{
			Date:     period.Date,
			MinMsgId: period.MinMsgID,
			MaxMsgId: period.MaxMsgID,
			Count:    period.Count,
		}).To_SearchResultsCalendarPeriod())
	}
	if cal.Messages == nil {
		cal.Messages = []*mtproto.Message{}
	}
	users, chats := c.peersOf(cal.Messages)
	return mtproto.MakeTLMessagesSearchResultsCalendar(&mtproto.Messages_SearchResultsCalendar{
		Inexact:  cal.Inexact,
		Count:    cal.Count,
		MinDate:  cal.MinDate,
		MinMsgId: cal.MinMsgID,
		Periods:  periods,
		Messages: cal.Messages,
		Chats:    chats,
		Users:    users,
	}).To_Messages_SearchResultsCalendar(), nil
}
