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
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/pagelimit"
)

// MessagesSearch
// messages.search#a0fda762 flags:# peer:InputPeer q:string from_id:flags.0?InputPeer top_msg_id:flags.1?int filter:MessagesFilter min_date:int max_date:int offset_id:int add_offset:int limit:int max_id:int min_id:int hash:long = messages.Messages;
func (c *MessagesCore) MessagesSearch(in *mtproto.TLMessagesSearch) (*mtproto.Messages_Messages, error) {
	// 400	BOT_METHOD_INVALID	This method can't be used by a bot
	if c.MD.IsBot {
		err := mtproto.ErrBotMethodInvalid
		c.Logger.Errorf("messages.search - error: %v", err)
		return nil, err
	}

	var (
		rValues  *mtproto.Messages_Messages
		offsetId = in.OffsetId
		limit    = in.Limit
		boxList  *mtproto.MessageBoxList
		err      error
		fromId   *mtproto.PeerUtil
	)

	if offsetId == 0 {
		offsetId = math.MaxInt32
	}

	limit = pagelimit.Cap(limit)

	peer := mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
	if peer.IsChannel() {
		var fromUser int64
		if in.GetFromId() != nil {
			from := mtproto.FromInputPeer2(c.MD.UserId, in.GetFromId())
			if from.IsUser() {
				fromUser = from.PeerId
			}
		}
		return channel.Shared().SearchInChannel(c.ctx, c.MD.UserId, peer.PeerId, channel.ChannelSearch{
			Q:        in.Q,
			FromUser: fromUser,
			OffsetID: offsetId,
			Limit:    limit,
			MinDate:  in.GetMinDate(),
			MaxDate:  in.GetMaxDate(),
			MinID:    in.GetMinId(),
			MaxID:    in.GetMaxId(),
			TopMsgID: in.GetTopMsgId().GetValue(),
			Filter:   in.GetFilter(),
		})
	}

	if in.GetFromId() != nil {
		// 1. peer must chat and channel
		if !peer.IsChatOrChannel() {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("messages.search - error: %v", err)
			return nil, err
		}

		fromId = mtproto.FromInputPeer2(c.MD.UserId, in.FromId)
		// 2. fromId must user
		if !fromId.IsUser() {
			err = mtproto.ErrFromPeerInvalid
			c.Logger.Errorf("messages.search - error: %v", err)
			return nil, err
		}

		// TODO: check user was deleted?
		// 400	INPUT_USER_DEACTIVATED	The specified user was deleted.
	}

	rValues = mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages()

	if in.SavedPeerId != nil {
		// TODO: not impl
		return rValues, nil
	}

	filterType := mtproto.FromMessagesFilter(in.Filter)
	switch filterType {
	case mtproto.FilterPhotos:
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mtproto.MEDIA_PHOTOS_ONLY,
			Offset:    offsetId,
			Limit:     limit,
		})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
	case mtproto.FilterVideo:
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mtproto.MEDIA_VIDEOS_ONLY,
			Offset:    offsetId,
			Limit:     limit,
		})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
	case mtproto.FilterPhotoVideo:
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mtproto.MEDIA_PHOTOVIDEO,
			Offset:    offsetId,
			Limit:     limit,
		})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
	case mtproto.FilterDocument:
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mtproto.MEDIA_FILE,
			Offset:    offsetId,
			Limit:     limit,
		})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
	case mtproto.FilterUrl:
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mtproto.MEDIA_URL,
			Offset:    offsetId,
			Limit:     limit,
		})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
	case mtproto.FilterGif:
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mtproto.MEDIA_GIF,
			Offset:    offsetId,
			Limit:     limit,
		})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
	case mtproto.FilterVoice, mtproto.FilterRoundVideo:
		// Voice notes and round videos share MEDIA_AUDIO in storage; keep
		// only the kind asked for.
		boxList, err = c.searchMedia(peer, mtproto.MEDIA_AUDIO, offsetId, limit)
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
		boxList = keepBoxes(boxList, func(m *mtproto.Message) bool {
			if filterType == mtproto.FilterVoice {
				return mtproto.IsVoiceMessage(m)
			}
			return mtproto.IsRoundVideoMessage(m)
		})
	case mtproto.FilterPoll, mtproto.FilterGeo, mtproto.FilterContacts:
		mediaType := int32(mtproto.MEDIA_POLL)
		switch filterType {
		case mtproto.FilterGeo:
			mediaType = mtproto.MEDIA_GEO
		case mtproto.FilterContacts:
			mediaType = mtproto.MEDIA_CONTACT
		}
		boxList, err = c.searchMedia(peer, mediaType, offsetId, limit)
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
	case mtproto.FilterMusic:
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mtproto.MEDIA_MUSIC,
			Offset:    offsetId,
			Limit:     limit,
		})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
	case mtproto.FilterChatPhotos:
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mtproto.MEDIA_CHAT_PHOTO,
			Offset:    offsetId,
			Limit:     limit,
		})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
	case mtproto.FilterPhoneCalls:
		// The call log asks for every call, then the Missed tab sets
		// inputMessagesFilterPhoneCalls.missed. Storage has no missed
		// column, so a missed query reads a wider page and drops the
		// calls the tab does not show.
		fetch := limit
		missedOnly := in.GetFilter().GetMissed()
		if missedOnly && fetch > 0 {
			fetch *= 8
			if fetch > 200 {
				fetch = 200
			}
		}
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mtproto.MEDIA_PHONE_CALL,
			Offset:    offsetId,
			Limit:     fetch,
		})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
		if missedOnly {
			boxList = keepBoxes(boxList, incomingUnansweredCall)
			if boxList != nil && limit > 0 && int32(len(boxList.BoxList)) > limit {
				boxList.BoxList = boxList.BoxList[:limit]
			}
		}
	case mtproto.FilterRoundVoice:
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: mtproto.MEDIA_AUDIO,
			Offset:    offsetId,
			Limit:     limit,
		})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
	case mtproto.FilterMyMentions:
		c.Logger.Errorf("messages.search - invalid filter: %s", in)
		return rValues, nil
	case mtproto.FilterPinned:
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByPinned(c.ctx, &message.TLMessageSearchByPinned{
			UserId:   c.MD.UserId,
			PeerType: peer.PeerType,
			PeerId:   peer.PeerId,
		})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}
	case mtproto.FilterEmpty:
		/*
			{
			    "constructor": "CRC32_messages_search",
			    "peer": {
			        "predicate_name": "inputPeerUser",
			        "constructor": "CRC32_inputPeerUser",
			        "user_id": "136817694",
			        "access_hash": "9217169765117357414"
			    },
			    "q": "1",
			    "filter": {
			        "predicate_name": "inputMessagesFilterEmpty",
			        "constructor": "CRC32_inputMessagesFilterEmpty"
			    },
			    "limit": 21
			}
		*/
		// An empty query is a real search only for a Saved Messages tag chip.
		// Every other empty q, including a normal dialog that also sends
		// saved_reaction, is SEARCH_QUERY_EMPTY.
		if in.Q == "" && !(savedSelf(peer, c.MD.UserId) && len(in.GetSavedReaction()) > 0) {
			err = mtproto.ErrSearchQueryEmpty
			c.Logger.Errorf("messages.search - error: %v", err)
			return nil, err
		}
		if len(in.GetSavedReaction()) > 0 {
			return c.searchByReaction(peer, in, offsetId, limit)
		}

		var (
			fId int64
		)
		if fromId != nil {
			fId = fromId.PeerId
		}

		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchV2(
			c.ctx,
			&message.TLMessageSearchV2{
				UserId:    c.MD.UserId,
				PeerType:  peer.PeerType,
				PeerId:    peer.PeerId,
				Q:         in.Q,
				FromId:    fId,
				MinDate:   in.MinDate,
				MaxDate:   in.MaxDate,
				OffsetId:  offsetId,
				AddOffset: in.AddOffset,
				Limit:     limit,
				MaxId:     in.MaxId,
				MinId:     in.MinId,
				Hash:      in.Hash,
			})
		if err != nil {
			c.Logger.Errorf("messages.search - error: %v", err)
			return rValues, nil
		}

		if boxList.Count > 0 {
			rValues.Count = boxList.Count
			rValues.PredicateName = mtproto.Predicate_messages_messagesSlice
		}
	default:
		err = mtproto.ErrInputFilterInvalid
		c.Logger.Errorf("messages.search - error: %v", err)
		return nil, err
	}

	boxList.Visit(c.MD.UserId,
		func(messageList []*mtproto.Message) {
			rValues.Messages = messageList
		},
		func(userIdList []int64) {
			mUsers, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
				&userpb.TLUserGetMutableUsers{
					Id: userIdList,
				})
			rValues.Users = append(rValues.Users, mUsers.GetUserListByIdList(c.MD.UserId, userIdList...)...)
		},
		func(chatIdList []int64) {
			mChats, _ := c.svcCtx.Dao.ChatClient.Client().ChatGetChatListByIdList(c.ctx,
				&chatpb.TLChatGetChatListByIdList{
					IdList: chatIdList,
				})
			rValues.Chats = append(rValues.Chats, mChats.GetChatListByIdList(c.MD.UserId, chatIdList...)...)
		},
		func(channelIdList []int64) {
			// ignore it
		})

	return rValues, nil
}

// incomingUnansweredCall is a call the Missed tab shows: an incoming
// call that was not connected. Outgoing calls stay on the All tab.
// Missed, busy, and a missing reason are unanswered. A hangup or
// disconnect with a duration was picked up.
func incomingUnansweredCall(m *mtproto.Message) bool {
	if m == nil || m.GetOut() {
		return false
	}
	action := m.GetAction()
	if action.GetPredicateName() != mtproto.Predicate_messageActionPhoneCall {
		return false
	}
	switch action.GetReason().GetPredicateName() {
	case mtproto.Predicate_phoneCallDiscardReasonMissed,
		mtproto.Predicate_phoneCallDiscardReasonBusy,
		"":
		return true
	default:
		return false
	}
}

func (c *MessagesCore) searchMedia(peer *mtproto.PeerUtil, mediaType, offsetId, limit int32) (*mtproto.MessageBoxList, error) {
	return c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &message.TLMessageSearchByMediaType{
		UserId:    c.MD.UserId,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		MediaType: mediaType,
		Offset:    offsetId,
		Limit:     limit,
	})
}

// searchByReaction pages the caller's reaction document. The message ids
// that carry one of saved_reaction are the whole result. offset and limit
// cut that list. count is the number of tagged messages, and a short page
// is messages.messagesSlice.
func (c *MessagesCore) searchByReaction(peer *mtproto.PeerUtil, in *mtproto.TLMessagesSearch, offsetID, limit int32) (*mtproto.Messages_Messages, error) {
	rValues := mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages()
	ids := chatbox.Shared().ReactionMessageIDs(c.ctx, c.MD.UserId, peer, in.GetSavedReaction())
	ids = narrowReactionIDs(ids, offsetID, in.GetMinId(), in.GetMaxId())
	if in.Q != "" {
		loaded := c.boxesByID(ids)
		var kept []int32
		q := strings.ToLower(in.Q)
		for _, id := range ids {
			box := loaded[id]
			if box == nil || box.GetMessage() == nil {
				continue
			}
			if strings.Contains(strings.ToLower(box.GetMessage().GetMessage()), q) {
				kept = append(kept, id)
			}
		}
		ids = kept
	}
	total := int32(len(ids))
	page := pageReactionIDs(ids, in.GetAddOffset(), limit)
	loaded := c.boxesByID(page)
	for _, id := range page {
		box := loaded[id]
		if box == nil || box.GetMessage() == nil {
			continue
		}
		rValues.Messages = append(rValues.Messages, box.GetMessage())
	}
	if rValues.Messages == nil {
		rValues.Messages = []*mtproto.Message{}
	}
	if int32(len(rValues.Messages)) < total {
		rValues.Count = total
		rValues.PredicateName = mtproto.Predicate_messages_messagesSlice
	}
	c.attachSearchUsers(rValues)
	return rValues, nil
}

func narrowReactionIDs(ids []int32, offsetID, minID, maxID int32) []int32 {
	if offsetID == 0 || offsetID == math.MaxInt32 {
		offsetID = 0
	}
	var out []int32
	for _, id := range ids {
		if offsetID != 0 && id >= offsetID {
			continue
		}
		if minID > 0 && id <= minID {
			continue
		}
		if maxID > 0 && id >= maxID {
			continue
		}
		out = append(out, id)
	}
	return out
}

func pageReactionIDs(ids []int32, addOffset, limit int32) []int32 {
	if limit < 0 {
		limit = 0
	}
	start := int(addOffset)
	if start < 0 {
		start = 0
	}
	if start > len(ids) {
		return nil
	}
	end := start + int(limit)
	if end > len(ids) {
		end = len(ids)
	}
	return ids[start:end]
}

func (c *MessagesCore) boxesByID(ids []int32) map[int32]*mtproto.MessageBox {
	out := map[int32]*mtproto.MessageBox{}
	if len(ids) == 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return out
	}
	found, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
		UserId: c.MD.UserId,
		IdList: ids,
	})
	if err != nil || found == nil {
		return out
	}
	for _, box := range found.GetDatas() {
		if box == nil {
			continue
		}
		id := box.GetMessageId()
		if box.GetMessage() != nil && box.GetMessage().GetId() != 0 {
			id = box.GetMessage().GetId()
		}
		out[id] = box
	}
	return out
}

func (c *MessagesCore) attachSearchUsers(rValues *mtproto.Messages_Messages) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return
	}
	seen := map[int64]bool{}
	var ids []int64
	for _, m := range rValues.GetMessages() {
		id := m.GetFromId().GetUserId()
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return
	}
	mUsers, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: ids})
	if mUsers != nil {
		rValues.Users = append(rValues.Users, mUsers.GetUserListByIdList(c.MD.UserId, ids...)...)
	}
}

func savedSelf(peer *mtproto.PeerUtil, self int64) bool {
	if peer == nil {
		return false
	}
	if peer.PeerType == mtproto.PEER_SELF {
		return true
	}
	return peer.PeerType == mtproto.PEER_USER && (peer.PeerId == 0 || peer.PeerId == self)
}

// keepBoxes drops boxes whose message does not satisfy ok.
func keepBoxes(list *mtproto.MessageBoxList, ok func(*mtproto.Message) bool) *mtproto.MessageBoxList {
	if list == nil {
		return list
	}
	kept := list.BoxList[:0]
	for _, box := range list.BoxList {
		if box != nil && ok(box.GetMessage()) {
			kept = append(kept, box)
		}
	}
	list.BoxList = kept
	return list
}
