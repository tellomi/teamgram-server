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
	"container/list"

	"github.com/teamgram/proto/mtproto"
)

func equalPeer(p, o *mtproto.Peer) bool {
	if p.GetPredicateName() != o.GetPredicateName() {
		return false
	}
	switch p.GetPredicateName() {
	case mtproto.Predicate_peerUser:
		return p.GetUserId() == o.GetUserId()
	case mtproto.Predicate_peerChat:
		return p.GetChatId() == o.GetChatId()
	case mtproto.Predicate_peerChannel:
		return p.GetChannelId() == o.GetChannelId()
	default:
		return false
	}
}

type mergeUpdatesHelper struct {
	newMessages           []*mtproto.Message
	deleteMessages        *list.Element
	readHistoryInBoxList  []*list.Element
	readHistoryOutBoxList []*list.Element
	readMessagesContents  *list.Element
	phoneCallRequest      *list.Element
	otherUpdates          *list.List
}

func newMergeUpdatesHelper() *mergeUpdatesHelper {
	return &mergeUpdatesHelper{
		newMessages:  []*mtproto.Message{},
		otherUpdates: list.New(),
	}
}

// merge combines one getDifference window.
//
// Official clients apply a pts update only when localPts + pts_count == pts.
// The update's own pts and pts_count stay. Same-type rows are folded only
// when they are adjacent and the later pts continues the earlier one
// (earlier.pts + later.pts_count == later.pts): the result keeps the later
// pts and the sum of the counts. Folding across another update, or across
// a hole, makes that walk fail.
func (m *mergeUpdatesHelper) merge(update *mtproto.Update) {
	switch update.PredicateName {
	case mtproto.Predicate_updateNewMessage:
		// updateNewMessage#1f2b0afd message:Message pts:int pts_count:int = Update;
		m.newMessages = append(m.newMessages, update.Message_MESSAGE)
	case mtproto.Predicate_updateDeleteMessages:
		// updateDeleteMessages#a20db0e5 messages:Vector<int> pts:int pts_count:int = Update;
		m.foldMessageIDs(&m.deleteMessages, update)
	case mtproto.Predicate_updateReadHistoryInbox:
		// updateReadHistoryInbox#9c974fdf flags:# folder_id:flags.0?int peer:Peer max_id:int still_unread_count:int pts:int pts_count:int = Update;
		m.foldReadHistory(&m.readHistoryInBoxList, update, true)
	case mtproto.Predicate_updateReadHistoryOutbox:
		// updateReadHistoryOutbox#2f2f21bf peer:Peer max_id:int pts:int pts_count:int = Update;
		m.foldReadHistory(&m.readHistoryOutBoxList, update, false)
	case mtproto.Predicate_updateWebPage:
		// updateWebPage#7f891213 webpage:WebPage pts:int pts_count:int = Update;
		m.otherUpdates.PushBack(update)
	case mtproto.Predicate_updateReadMessagesContents:
		// updateReadMessagesContents#68c13933 messages:Vector<int> pts:int pts_count:int = Update;
		m.foldMessageIDs(&m.readMessagesContents, update)
	case mtproto.Predicate_updateNewChannelMessage:
		// updateNewChannelMessage#62ba04d9 message:Message pts:int pts_count:int = Update;
		// ignore
	case mtproto.Predicate_updateDeleteChannelMessages:
		// updateDeleteChannelMessages#c37521c9 channel_id:int messages:Vector<int> pts:int pts_count:int = Update;
		// ignore
	case mtproto.Predicate_updateEditChannelMessage:
		// updateEditChannelMessage#1b3f4df7 message:Message pts:int pts_count:int = Update;
		// ignore
	case mtproto.Predicate_updateEditMessage:
		// updateEditMessage#e40370a3 message:Message pts:int pts_count:int = Update;
		m.otherUpdates.PushBack(update)
	case mtproto.Predicate_updateChannelWebPage:
		// updateChannelWebPage#40771900 channel_id:int webpage:WebPage pts:int pts_count:int = Update;
		// ignore
	case mtproto.Predicate_updateFolderPeers:
		// updateFolderPeers#19360dc0 folder_peers:Vector<FolderPeer> pts:int pts_count:int = Update;
		m.otherUpdates.PushBack(update)
	case mtproto.Predicate_updatePinnedMessages:
		// updatePinnedMessages#ed85eab5 flags:# pinned:flags.0?true peer:Peer messages:Vector<int> pts:int pts_count:int = Update;
		m.otherUpdates.PushBack(update)
	case mtproto.Predicate_updatePinnedChannelMessages:
		// updatePinnedChannelMessages#8588878b flags:# pinned:flags.0?true channel_id:int messages:Vector<int> pts:int pts_count:int = Update;
		// ignore
	case mtproto.Predicate_updatePhoneCall:
		switch update.GetPhoneCall().GetPredicateName() {
		case mtproto.Predicate_phoneCallRequested:
			if m.phoneCallRequest != nil {
				m.otherUpdates.Remove(m.phoneCallRequest)
			}
			m.phoneCallRequest = m.otherUpdates.PushBack(update)
		case mtproto.Predicate_phoneCallDiscarded:
			if m.phoneCallRequest != nil {
				m.otherUpdates.Remove(m.phoneCallRequest)
				m.phoneCallRequest = nil
			}
		}
	default:
		// TODO: merge
		m.otherUpdates.PushBack(update)
	}
}

// ptsContinues reports that next starts at the pts prev ends on.
func ptsContinues(prev, next *mtproto.Update) bool {
	if prev.GetPtsCount() <= 0 || next.GetPtsCount() <= 0 {
		return false
	}
	return prev.GetPts_INT32()+next.GetPtsCount() == next.GetPts_INT32()
}

func applyLaterPts(dst, src *mtproto.Update) {
	dst.Pts_INT32 = src.Pts_INT32
	dst.PtsCount += src.PtsCount
}

// foldMessageIDs appends message ids when the previous update of this kind
// is still the tail and its pts continues. Otherwise the update is kept.
func (m *mergeUpdatesHelper) foldMessageIDs(slot **list.Element, update *mtproto.Update) {
	if *slot != nil && *slot == m.otherUpdates.Back() && ptsContinues((*slot).Value.(*mtproto.Update), update) {
		kept := (*slot).Value.(*mtproto.Update)
		kept.Messages = append(kept.Messages, update.Messages...)
		applyLaterPts(kept, update)
		return
	}
	*slot = m.otherUpdates.PushBack(update)
}

// foldReadHistory reduces adjacent reads of one peer. inbox also carries
// still_unread_count. The first list entry is included; a match at index 0
// is the same fold as any later one.
func (m *mergeUpdatesHelper) foldReadHistory(slots *[]*list.Element, update *mtproto.Update, inbox bool) {
	var matched *list.Element
	for _, e := range *slots {
		prev := e.Value.(*mtproto.Update)
		if inbox && prev.GetFolderId().GetValue() != update.GetFolderId().GetValue() {
			continue
		}
		if equalPeer(prev.GetPeer_PEER(), update.GetPeer_PEER()) {
			matched = e
		}
	}
	if matched != nil && matched == m.otherUpdates.Back() && ptsContinues(matched.Value.(*mtproto.Update), update) {
		kept := matched.Value.(*mtproto.Update)
		if update.GetMaxId() > kept.GetMaxId() {
			kept.MaxId = update.GetMaxId()
		}
		if inbox {
			kept.StillUnreadCount = update.StillUnreadCount
		}
		applyLaterPts(kept, update)
		return
	}
	*slots = append(*slots, m.otherUpdates.PushBack(update))
}

func (m *mergeUpdatesHelper) toUpdates() []*mtproto.Update {
	updates := make([]*mtproto.Update, 0, m.otherUpdates.Len())
	for e := m.otherUpdates.Front(); e != nil; e = e.Next() {
		updates = append(updates, e.Value.(*mtproto.Update))
	}
	return updates
}
