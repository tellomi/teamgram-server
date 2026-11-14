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
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// MessagesGetChatInviteImporters
// messages.getChatInviteImporters#df04dd4e flags:# requested:flags.0?true peer:InputPeer link:flags.1?string q:flags.2?string offset_date:int offset_user:InputUser limit:int = messages.ChatInviteImporters;
func (c *ChatInvitesCore) MessagesGetChatInviteImporters(in *mtproto.TLMessagesGetChatInviteImporters) (*mtproto.Messages_ChatInviteImporters, error) {
	var (
		peer       = mtproto.FromInputPeer2(c.MD.UserId, in.Peer)
		offsetPeer = mtproto.FromInputUser(c.MD.UserId, in.OffsetUser)
		link       *wrapperspb.StringValue
		limit      = in.GetLimit()
	)

	if in.Link != nil {
		link = in.GetLink()
	} else {
		// link = mtproto.MakeFlagsString(in.Link)
	}

	if in.GetQ().GetValue() != "" && link.GetValue() != "" {
		// TODO: add ErrSearchWithLinkNotSupported
		err := mtproto.ErrSearchWithLinkNotSupported
		c.Logger.Errorf("messages.getChatInviteImporters - error: ", err)
		return nil, err
	}

	if peer.IsChannel() {
		rValues, err := c.channelImporters(in, peer)
		if err != nil {
			c.Logger.Errorf("messages.getChatInviteImporters - error: %v", err)
			return nil, err
		}
		return rValues, nil
	}
	if !peer.IsChat() {
		err := mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("messages.getChatInviteImporters - error: ", err)
		return nil, err
	}

	if limit == 0 {
		limit = 50
	}

	rValues := mtproto.MakeTLMessagesChatInviteImporters(&mtproto.Messages_ChatInviteImporters{
		Count:     0,
		Importers: []*mtproto.ChatInviteImporter{},
		Users:     []*mtproto.User{},
	}).To_Messages_ChatInviteImporters()

	inviteImporters, err := c.svcCtx.Dao.ChatClient.ChatGetChatInviteImporters(c.ctx, &chatpb.TLChatGetChatInviteImporters{
		SelfId:     c.MD.UserId,
		ChatId:     peer.PeerId,
		Requested:  in.Requested,
		Link:       link,
		Q:          in.Q,
		OffsetDate: in.OffsetDate,
		OffsetUser: offsetPeer.PeerId,
		Limit:      limit,
	})
	if err != nil {
		c.Logger.Errorf("messages.getChatInviteImporters - error: ", err)
		return nil, err
	}

	rValues.Importers = inviteImporters.Datas

	if len(rValues.Importers) == 0 {
		return rValues, nil
	}

	rValues.Count = int32(len(rValues.Importers))
	idHelper := mtproto.NewIDListHelper(c.MD.UserId)
	for _, a := range rValues.Importers {
		idHelper.AppendUsers(a.UserId)
	}

	idHelper.Visit(func(userIdList []int64) {
		users, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: userIdList,
			To: []int64{c.MD.UserId},
		})
		rValues.Users = users.GetUserListByIdList(c.MD.UserId, userIdList...)
	}, nil, nil)

	if q := strings.TrimSpace(in.GetQ().GetValue()); q != "" {
		byID := map[int64]*mtproto.User{}
		for _, u := range rValues.Users {
			byID[u.GetId()] = u
		}
		filtered := make([]*mtproto.ChatInviteImporter, 0, len(rValues.Importers))
		for _, row := range rValues.Importers {
			if userMatches(byID[row.GetUserId()], q) {
				filtered = append(filtered, row)
			}
		}
		page := pageImporters(filtered, in.GetOffsetDate(), offsetPeer.PeerId, limit)
		keep := map[int64]struct{}{}
		for _, row := range page {
			keep[row.GetUserId()] = struct{}{}
			if row.GetApprovedBy() != nil {
				keep[row.GetApprovedBy().GetValue()] = struct{}{}
			}
		}
		users := make([]*mtproto.User, 0, len(page))
		for _, u := range rValues.Users {
			if _, ok := keep[u.GetId()]; ok {
				users = append(users, u)
			}
		}
		rValues.Count = int32(len(filtered))
		rValues.Importers = page
		rValues.Users = users
	}

	return rValues, nil
}

func pageImporters(rows []*mtproto.ChatInviteImporter, offsetDate int32, offsetUser int64, limit int32) []*mtproto.ChatInviteImporter {
	start := 0
	if offsetUser != 0 || offsetDate != 0 {
		start = len(rows)
		for i, row := range rows {
			if row.GetUserId() == offsetUser && row.GetDate() == offsetDate {
				start = i + 1
				break
			}
		}
	}
	if limit <= 0 {
		limit = 50
	}
	if start > len(rows) {
		start = len(rows)
	}
	end := start + int(limit)
	if end > len(rows) {
		end = len(rows)
	}
	if rows == nil {
		return []*mtproto.ChatInviteImporter{}
	}
	return append([]*mtproto.ChatInviteImporter{}, rows[start:end]...)
}
