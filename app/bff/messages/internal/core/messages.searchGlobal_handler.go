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

	return packGlobalResult(rValues, limit), nil
}

// packGlobalResult is messages.messages when this page is short of limit:
// clients treat that constructor as the end of the search. A full page may
// have more rows, so it is messages.messagesSlice with next_rate set to the
// last message date. count is one past the page because the total is not
// counted, and inexact says so. iOS stops on messages.messages. Android
// hashtag search reads next_rate; without it the next call sends offset_rate 0.
func packGlobalResult(page *mtproto.Messages_Messages, limit int32) *mtproto.Messages_Messages {
	if page == nil || limit <= 0 || int32(len(page.GetMessages())) < limit {
		return page
	}
	msgs := page.GetMessages()
	last := msgs[len(msgs)-1]
	return mtproto.MakeTLMessagesMessagesSlice(&mtproto.Messages_Messages{
		Inexact:  true,
		Count:    int32(len(msgs)) + 1,
		NextRate: &wrapperspb.Int32Value{Value: last.GetDate()},
		Messages: msgs,
		Topics:   []*mtproto.ForumTopic{},
		Chats:    page.GetChats(),
		Users:    page.GetUsers(),
	}).To_Messages_Messages()
}
