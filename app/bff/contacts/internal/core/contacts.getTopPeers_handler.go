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
	"github.com/teamgram/teamgram-server/app/bff/contacts/book"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsGetTopPeers
// contacts.getTopPeers#973478b6 flags:# correspondents:flags.0?true bots_pm:flags.1?true bots_inline:flags.2?true phone_calls:flags.3?true forward_users:flags.4?true forward_chats:flags.5?true groups:flags.10?true channels:flags.15?true bots_app:flags.16?true bots_guestchat:flags.17?true offset:int limit:int hash:long = contacts.TopPeers;
// https://core.telegram.org/method/contacts.getTopPeers
func (c *ContactsCore) ContactsGetTopPeers(in *mtproto.TLContactsGetTopPeers) (*mtproto.Contacts_TopPeers, error) {
	if err := c.requireSelf(); err != nil {
		return nil, err
	}
	cats := requestedCats(in)
	if len(cats) == 0 {
		return nil, mtproto.ErrTypesEmpty
	}
	if !c.relBook().TopEnabled(c.ctx, c.MD.UserId) {
		return mtproto.MakeTLContactsTopPeersDisabled(&mtproto.Contacts_TopPeers{}).To_Contacts_TopPeers(), nil
	}
	ranked := c.relBook().Rank(c.ctx, c.MD.UserId, cats, c.topHits(), int(in.GetOffset()), int(in.GetLimit()))
	if in.GetHash() != 0 && in.GetHash() == book.HashRanks(ranked) {
		return mtproto.MakeTLContactsTopPeersNotModified(&mtproto.Contacts_TopPeers{}).To_Contacts_TopPeers(), nil
	}
	categories := make([]*mtproto.TopPeerCategoryPeers, 0, len(ranked))
	var userIDs, chatIDs, channelIDs []int64
	for _, row := range ranked {
		peers := make([]*mtproto.TopPeer, 0, len(row.Peers))
		for _, peer := range row.Peers {
			peers = append(peers, mtproto.MakeTLTopPeer(&mtproto.TopPeer{
				Peer:   mtPeer(peer.Peer),
				Rating: peer.Rating,
			}).To_TopPeer())
			switch peer.Peer.Type {
			case mtproto.PEER_USER:
				userIDs = append(userIDs, peer.Peer.ID)
			case mtproto.PEER_CHAT:
				chatIDs = append(chatIDs, peer.Peer.ID)
			case mtproto.PEER_CHANNEL:
				channelIDs = append(channelIDs, peer.Peer.ID)
			}
		}
		categories = append(categories, mtproto.MakeTLTopPeerCategoryPeers(&mtproto.TopPeerCategoryPeers{
			Category: topCategory(row.Cat),
			Count:    int32(row.Total),
			Peers:    peers,
		}).To_TopPeerCategoryPeers())
	}
	return mtproto.MakeTLContactsTopPeers(&mtproto.Contacts_TopPeers{
		Categories: categories,
		Chats:      c.topChats(chatIDs, channelIDs),
		Users:      c.topUsers(userIDs),
	}).To_Contacts_TopPeers(), nil
}

func requestedCats(in *mtproto.TLContactsGetTopPeers) []string {
	if in == nil {
		return nil
	}
	var cats []string
	if in.GetCorrespondents() {
		cats = append(cats, book.CatCorrespondents)
	}
	if in.GetBotsPm() {
		cats = append(cats, book.CatBotsPM)
	}
	if in.GetBotsInline() {
		cats = append(cats, book.CatBotsInline)
	}
	if in.GetPhoneCalls() {
		cats = append(cats, book.CatPhoneCalls)
	}
	if in.GetForwardUsers() {
		cats = append(cats, book.CatForwardUsers)
	}
	if in.GetForwardChats() {
		cats = append(cats, book.CatForwardChats)
	}
	if in.GetGroups() {
		cats = append(cats, book.CatGroups)
	}
	if in.GetChannels() {
		cats = append(cats, book.CatChannels)
	}
	if in.GetBotsApp() {
		cats = append(cats, book.CatBotsApp)
	}
	if in.GetBotsGuestchat() {
		cats = append(cats, book.CatBotsGuest)
	}
	return cats
}

func topCategory(name string) *mtproto.TopPeerCategory {
	empty := &mtproto.TopPeerCategory{}
	switch name {
	case book.CatBotsPM:
		return mtproto.MakeTLTopPeerCategoryBotsPM(empty).To_TopPeerCategory()
	case book.CatBotsInline:
		return mtproto.MakeTLTopPeerCategoryBotsInline(empty).To_TopPeerCategory()
	case book.CatGroups:
		return mtproto.MakeTLTopPeerCategoryGroups(empty).To_TopPeerCategory()
	case book.CatChannels:
		return mtproto.MakeTLTopPeerCategoryChannels(empty).To_TopPeerCategory()
	case book.CatPhoneCalls:
		return mtproto.MakeTLTopPeerCategoryPhoneCalls(empty).To_TopPeerCategory()
	case book.CatForwardUsers:
		return mtproto.MakeTLTopPeerCategoryForwardUsers(empty).To_TopPeerCategory()
	case book.CatForwardChats:
		return mtproto.MakeTLTopPeerCategoryForwardChats(empty).To_TopPeerCategory()
	case book.CatBotsApp:
		return mtproto.MakeTLTopPeerCategoryBotsApp(empty).To_TopPeerCategory()
	case book.CatBotsGuest:
		return mtproto.MakeTLTopPeerCategoryBotsGuestChat(empty).To_TopPeerCategory()
	default:
		return mtproto.MakeTLTopPeerCategoryCorrespondents(empty).To_TopPeerCategory()
	}
}

func mtPeer(p book.Peer) *mtproto.Peer {
	return (&mtproto.PeerUtil{PeerType: p.Type, PeerId: p.ID}).ToPeer()
}

func (c *ContactsCore) topUsers(ids []int64) []*mtproto.User {
	if len(ids) == 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return []*mtproto.User{}
	}
	got, err := c.svcCtx.Dao.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
		Id: append([]int64{c.MD.UserId}, ids...),
	})
	if err != nil || got == nil {
		return []*mtproto.User{}
	}
	users := got.GetUserListByIdList(c.MD.UserId, ids...)
	if users == nil {
		return []*mtproto.User{}
	}
	return users
}

func (c *ContactsCore) topChats(chats, channels []int64) []*mtproto.Chat {
	var out []*mtproto.Chat
	if len(chats) > 0 && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.ChatClient != nil {
		got, err := c.svcCtx.Dao.ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{
			SelfId: c.MD.UserId,
			IdList: chats,
		})
		if err == nil && got != nil {
			out = append(out, got.GetChatListByIdList(c.MD.UserId, chats...)...)
		}
	}
	for _, id := range channels {
		if chat := channel.Shared().VisibleChat(c.ctx, c.MD.UserId, id); chat != nil {
			out = append(out, chat)
		}
	}
	if out == nil {
		out = []*mtproto.Chat{}
	}
	return out
}
