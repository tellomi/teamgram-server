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
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/pagelimit"
	"github.com/teamgram/teamgram-server/pkg/searchorder"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesSearchGlobal
// messages.searchGlobal#4bc6589a flags:# folder_id:flags.0?int q:string filter:MessagesFilter min_date:int max_date:int offset_rate:int offset_peer:InputPeer offset_id:int limit:int = messages.Messages;
func (c *MessagesCore) MessagesSearchGlobal(in *mtproto.TLMessagesSearchGlobal) (*mtproto.Messages_Messages, error) {
	// 400	BOT_METHOD_INVALID	This method can't be used by a bot
	// 400	SEARCH_QUERY_EMPTY	The search query is empty
	if c.MD.IsBot {
		err := mtproto.ErrBotMethodInvalid
		c.Logger.Errorf("messages.searchGlobal - error: %v", err)
		return nil, err
	}

	q := strings.TrimSpace(in.Q)
	filterOn := globalFilterUseful(in.Filter)
	// Official clients load a media tab with an empty query. Only a text
	// search with no media filter is SEARCH_QUERY_EMPTY.
	if q == "" && !filterOn {
		err := mtproto.ErrSearchQueryEmpty
		c.Logger.Errorf("messages.searchGlobal - error: %v", err)
		return nil, err
	}

	limit := pagelimit.Cap(in.Limit)
	scope := globalQuery{
		UsersOnly:      in.GetUsersOnly(),
		GroupsOnly:     in.GetGroupsOnly(),
		BroadcastsOnly: in.GetBroadcastsOnly(),
		MinDate:        in.GetMinDate(),
		MaxDate:        in.GetMaxDate(),
	}
	cur := globalCursor{}
	if in.GetOffsetRate() != 0 || (in.GetOffsetPeer() != nil && in.GetOffsetId() != 0) {
		peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetOffsetPeer())
		cur = globalCursor{
			Active:   true,
			Rate:     in.GetOffsetRate(),
			PeerType: peer.PeerType,
			PeerID:   peer.PeerId,
			MsgID:    in.GetOffsetId(),
		}
	}
	useFolder := in.GetFolderId() != nil
	fetchLimit := limit
	if (scope.narrows() || cur.Active || useFolder || filterOn) && fetchLimit > 0 {
		fetchLimit *= 8
		if fetchLimit > 200 {
			fetchLimit = 200
		}
	}

	rValues := mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages()

	var (
		boxList *mtproto.MessageBoxList
		err     error
	)
	if q == "" {
		kind, _ := globalMediaKind(in.Filter)
		mediaOffset := in.OffsetId
		if mediaOffset == 0 {
			mediaOffset = 0x7fffffff
		}
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchByMediaType(c.ctx, &messagepb.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  messagepb.SearchGlobalPeerType,
			MediaType: kind,
			Offset:    mediaOffset,
			Limit:     fetchLimit,
		})
	} else {
		// The cursor tuple travels on the context. Offset is not a box id.
		ctx := c.ctx
		if cur.Active {
			ctx = searchorder.WithOutgoing(ctx, cur.key())
		}
		if scope.UsersOnly || scope.GroupsOnly || scope.BroadcastsOnly {
			ctx = searchorder.WithPeerScope(ctx, scope.UsersOnly, scope.GroupsOnly, scope.BroadcastsOnly)
		}
		boxList, err = c.svcCtx.Dao.MessageClient.MessageSearchGlobal(
			ctx,
			&messagepb.TLMessageSearchGlobal{
				UserId: c.MD.UserId,
				Q:      q,
				Limit:  fetchLimit,
			})
	}
	if err != nil {
		c.Logger.Errorf("messages.searchGlobal - error: %v", err)
		boxList = nil
	}
	if q != "" {
		// Private and basic-group rows live in the user message table.
		// Channel and supergroup posts live in channel_messages and are
		// not copied there, so a channel post never matched this RPC.
		boxList = c.mergeJoinedChannelHits(boxList, q, fetchLimit, cur)
	}
	if filterOn {
		boxList = keepBoxes(boxList, func(m *mtproto.Message) bool {
			return messageMatchesGlobalFilter(m, in.Filter)
		})
	}
	if scope.narrows() {
		kinds := map[int64]channelKind{}
		var channelIDs []int64
		for _, box := range boxList.GetBoxList() {
			if box != nil && box.GetPeerType() == mtproto.PEER_CHANNEL {
				channelIDs = append(channelIDs, box.GetPeerId())
			}
		}
		for _, chat := range channel.Shared().ChatsFor(c.ctx, c.MD.UserId, channelIDs...) {
			if chat == nil {
				continue
			}
			kinds[chat.GetId()] = channelKind{Broadcast: chat.GetBroadcast(), Megagroup: chat.GetMegagroup()}
		}
		boxList = filterGlobalBoxes(boxList, scope, kinds, 0)
	}
	boxList = c.dropHiddenPrehistory(boxList)
	var folder map[string]bool
	if useFolder {
		folder, useFolder = c.globalFolder(in.GetFolderId().GetValue())
	}
	if useFolder {
		folder = c.attachChannelFolders(folder, in.GetFolderId().GetValue())
	}
	sortGlobalBoxes(boxList)
	boxList = filterGlobalPage(boxList, cur, folder, useFolder, limit)
	if boxList == nil {
		return rValues, nil
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
			if chats := channel.Shared().ChatsFor(c.ctx, c.MD.UserId, channelIdList...); len(chats) > 0 {
				rValues.Chats = append(rValues.Chats, chats...)
			}
		})

	total, exact := c.globalResultCount(q, scope, boxList, useFolder || filterOn)
	return packGlobalResult(rValues, limit, total, exact), nil
}

// globalResultCount is the same full-result total on every page.
// User rows come back on the message-service reply. Joined channel posts
// are counted here. A folder or media filter is not subtracted, so the
// total can be high and exact is false. A failed count is 0; the packer
// then uses a value no loaded page can reach.
func (c *MessagesCore) globalResultCount(q string, scope globalQuery, boxes *mtproto.MessageBoxList, approximate bool) (int32, bool) {
	if q == "" || c == nil || c.MD == nil {
		return 0, false
	}
	restrict := scope.UsersOnly || scope.GroupsOnly || scope.BroadcastsOnly
	var userN int32
	if boxes != nil {
		userN = boxes.GetCount()
	}
	if userN == math.MaxInt32 {
		return math.MaxInt32, false
	}
	var chN int32
	onlyUsers := restrict && scope.UsersOnly && !scope.GroupsOnly && !scope.BroadcastsOnly
	if !onlyUsers {
		n, err := channel.Shared().CountJoined(c.ctx, c.MD.UserId, q, restrict, scope.BroadcastsOnly, scope.GroupsOnly)
		if err != nil {
			c.Logger.Errorf("messages.searchGlobal count: %v", err)
			return 0, false
		}
		chN = n
	}
	sum := int64(userN) + int64(chN)
	if sum > math.MaxInt32 {
		return math.MaxInt32, false
	}
	return int32(sum), !approximate
}

// packGlobalResult is messages.messages when this page is short of limit.
// iOS sets totalCount to that page's length, so the rendered count equals
// totalCount and the search completes. Android FilteredSearchView sets
// totalCount = res.count and stops when the accumulated list reaches it, so
// a full page reports the whole result, the same number on every page.
// next_rate is the last message date. inexact is set when that number is
// only a ceiling. DialogsSearchAdapter and HashtagSearchController follow
// the page size and next_rate, not this count.
func packGlobalResult(page *mtproto.Messages_Messages, limit, total int32, exact bool) *mtproto.Messages_Messages {
	if page == nil || limit <= 0 || int32(len(page.GetMessages())) < limit {
		return page
	}
	msgs := page.GetMessages()
	n := int32(len(msgs))
	if exact && total == n {
		// This page is the whole result. messages.messages completes both
		// clients without another request.
		return page
	}
	if total <= n {
		// Unknown or below this page. Stay above every list the client can
		// accumulate until a short page ends the search.
		total = math.MaxInt32
		exact = false
	}
	return mtproto.MakeTLMessagesMessagesSlice(&mtproto.Messages_Messages{
		Inexact:  !exact,
		Count:    total,
		NextRate: &wrapperspb.Int32Value{Value: msgs[len(msgs)-1].GetDate()},
		Messages: msgs,
		Topics:   []*mtproto.ForumTopic{},
		Chats:    page.GetChats(),
		Users:    page.GetUsers(),
	}).To_Messages_Messages()
}

// dropHiddenPrehistory applies channels.togglePreHistoryHidden to global
// search. The predicate is cached per channel so a page does not reload the
// same member row. A rewrite of this handler should keep the call.
func (c *MessagesCore) dropHiddenPrehistory(list *mtproto.MessageBoxList) *mtproto.MessageBoxList {
	if c == nil || c.MD == nil {
		return list
	}
	gates := map[int64]channel.PrehistoryGate{}
	return dropPrehistoryBoxes(list, func(channelID int64, date int32) bool {
		g, ok := gates[channelID]
		if !ok {
			g = channel.Shared().PrehistoryGate(c.ctx, c.MD.UserId, channelID)
			gates[channelID] = g
		}
		return g.Blocks(date)
	})
}
