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
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc/status"
)

// ContactsResolveUsername
// contacts.resolveUsername#f93ccba3 username:string = contacts.ResolvedPeer;
func (c *UsernamesCore) ContactsResolveUsername(in *mtproto.TLContactsResolveUsername) (*mtproto.Contacts_ResolvedPeer, error) {
	// TODO(@benqi):
	// 401	AUTH_KEY_PERM_EMPTY	The temporary auth key must be binded to the permanent auth key to use these methods.
	// 401	SESSION_PASSWORD_NEEDED	2FA is enabled, use a password to login
	// 400	USERNAME_INVALID	The provided username is not valid
	// 400	USERNAME_NOT_OCCUPIED	The provided username is not occupied
	//
	var (
		peer *mtproto.PeerUtil
	)

	if normalizeUsername(in.GetUsername()) == mtproto.BotVenueName {
		return c.resolveVenue(in)
	}

	id := userpb.GetBotIdByName(in.GetUsername())
	if id > 0 {
		peer = mtproto.MakeUserPeerUtil(id)
	} else {
		rName, err := c.svcCtx.Dao.UserClient.UserResolveUsername(c.ctx, &userpb.TLUserResolveUsername{
			Username: in.GetUsername(),
		})
		if err != nil {
			if ch, chErr := channel.Shared().ResolveUsername(c.ctx, c.MD.UserId, in.GetUsername()); chErr == nil && ch != nil {
				return mtproto.MakeTLContactsResolvedPeer(&mtproto.Contacts_ResolvedPeer{
					Peer:  mtproto.MakePeerChannel(ch.GetId()),
					Chats: []*mtproto.Chat{ch},
					Users: []*mtproto.User{},
				}).To_Contacts_ResolvedPeer(), nil
			}
			c.Logger.Errorf("contacts.resolveUsername - reply: {%v}", err)
			return nil, err
		}

		peer = mtproto.FromPeer(rName)
	}

	resolvedPeer := mtproto.MakeTLContactsResolvedPeer(&mtproto.Contacts_ResolvedPeer{
		Peer:  peer.ToPeer(),
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Contacts_ResolvedPeer()

	switch peer.PeerType {
	case mtproto.PEER_USER:
		mUsers, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: []int64{c.MD.UserId, peer.PeerId},
		})
		// .UserFacade.GetUserById(ctx, md.UserId, peer.PeerId)
		if mUsers != nil {
			resolvedPeer.Users = mUsers.GetUserListByIdList(c.MD.UserId, peer.PeerId)
		}
	case mtproto.PEER_CHAT:
		chat, _ := c.svcCtx.Dao.ChatClient.ChatGetChatBySelfId(c.ctx, &chat.TLChatGetChatBySelfId{
			SelfId: c.MD.UserId,
			ChatId: peer.PeerId,
		})
		if chat != nil {
			resolvedPeer.Chats = []*mtproto.Chat{chat.ToUnsafeChat(c.MD.UserId)}
		}
	case mtproto.PEER_CHANNEL:
		if ch, err := channel.Shared().ResolveUsername(c.ctx, c.MD.UserId, in.GetUsername()); err == nil && ch != nil {
			resolvedPeer.Chats = []*mtproto.Chat{ch}
		} else if c.svcCtx.Plugin != nil {
			resolvedPeer.Chats = c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, peer.PeerId)
		}
	}

	ensureGifBot(in.GetUsername(), resolvedPeer)
	return resolvedPeer, nil
}

// resolveVenue returns the built-in @venue bot only when the username table
// has no row for it. A real user or channel keeps the name.
func (c *UsernamesCore) resolveVenue(in *mtproto.TLContactsResolveUsername) (*mtproto.Contacts_ResolvedPeer, error) {
	looked, err := c.svcCtx.Dao.UserClient.UserResolveUsername(c.ctx, &userpb.TLUserResolveUsername{
		Username: mtproto.BotVenueName,
	})
	free, fatal := venueIsFree(err, looked)
	if fatal != nil {
		c.Logger.Errorf("contacts.resolveUsername - reply: {%v}", fatal)
		return nil, fatal
	}
	if !free {
		peer := mtproto.FromPeer(looked)
		return c.finishResolved(in.GetUsername(), peer, false)
	}
	return c.finishResolved(in.GetUsername(), mtproto.MakeUserPeerUtil(mtproto.BotVenueId), true)
}

func (c *UsernamesCore) finishResolved(username string, peer *mtproto.PeerUtil, claimVenue bool) (*mtproto.Contacts_ResolvedPeer, error) {
	resolvedPeer := mtproto.MakeTLContactsResolvedPeer(&mtproto.Contacts_ResolvedPeer{
		Peer:  peer.ToPeer(),
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Contacts_ResolvedPeer()

	switch peer.PeerType {
	case mtproto.PEER_USER:
		if claimVenue && peer.PeerId == mtproto.BotVenueId {
			resolvedPeer.Users = []*mtproto.User{catalog.VenueBotUser()}
			return resolvedPeer, nil
		}
		mUsers, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: []int64{c.MD.UserId, peer.PeerId},
		})
		if mUsers != nil {
			resolvedPeer.Users = mUsers.GetUserListByIdList(c.MD.UserId, peer.PeerId)
		}
	case mtproto.PEER_CHAT:
		chat, _ := c.svcCtx.Dao.ChatClient.ChatGetChatBySelfId(c.ctx, &chat.TLChatGetChatBySelfId{
			SelfId: c.MD.UserId,
			ChatId: peer.PeerId,
		})
		if chat != nil {
			resolvedPeer.Chats = []*mtproto.Chat{chat.ToUnsafeChat(c.MD.UserId)}
		}
	case mtproto.PEER_CHANNEL:
		if ch, err := channel.Shared().ResolveUsername(c.ctx, c.MD.UserId, username); err == nil && ch != nil {
			resolvedPeer.Chats = []*mtproto.Chat{ch}
		} else if c.svcCtx.Plugin != nil {
			resolvedPeer.Chats = c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, peer.PeerId)
		}
	}
	if claimVenue {
		ensureBuiltinBot(resolvedPeer, catalog.VenueBotUser())
	}
	return resolvedPeer, nil
}

func normalizeUsername(username string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(username)), "@")
}

// venueIsFree reports whether nobody holds username venue. A lookup failure
// other than USERNAME_NOT_OCCUPIED is fatal: the bot must not take the name
// when the table cannot be read.
func venueIsFree(err error, peer *mtproto.Peer) (bool, error) {
	if usernameNotOccupied(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if peer == nil || (peer.GetUserId() == 0 && peer.GetChannelId() == 0) {
		return true, nil
	}
	return false, nil
}

// rejectVenueClaim blocks a new claim of username venue. The user who
// already holds it can keep it. A free name is reserved for the bot.
// A lookup outage is returned so the caller does not guess.
func (c *UsernamesCore) rejectVenueClaim(username string, self int64) error {
	if normalizeUsername(username) != mtproto.BotVenueName {
		return nil
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil
	}
	peer, err := c.svcCtx.Dao.UserClient.UserResolveUsername(c.ctx, &userpb.TLUserResolveUsername{
		Username: mtproto.BotVenueName,
	})
	if err == nil && peer != nil && self != 0 && peer.GetUserId() == self {
		return nil
	}
	_, fatal := venueIsFree(err, peer)
	if fatal != nil {
		return fatal
	}
	return mtproto.ErrUsernameOccupied
}

func usernameNotOccupied(err error) bool {
	if err == nil {
		return false
	}
	if st, ok := status.FromError(err); ok && st.Message() == "USERNAME_NOT_OCCUPIED" {
		return true
	}
	return strings.Contains(err.Error(), "USERNAME_NOT_OCCUPIED")
}

// ensureGifBot puts the built-in @gif user on contacts.resolvedPeer.
// iOS will not open the GIF panel when resolveUsername has no bot user.
func ensureGifBot(username string, peer *mtproto.Contacts_ResolvedPeer) {
	name := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(username)), "@")
	if peer == nil || name != mtproto.BotGifName {
		return
	}
	bot := catalog.GifBotUser()
	for i, u := range peer.Users {
		if u.GetId() == bot.GetId() {
			if !u.GetBot() {
				peer.Users[i] = bot
			}
			return
		}
	}
	peer.Users = append(peer.Users, bot)
}

func ensureBuiltinBot(peer *mtproto.Contacts_ResolvedPeer, bot *mtproto.User) {
	if peer == nil || bot == nil {
		return
	}
	for i, u := range peer.Users {
		if u.GetId() == bot.GetId() {
			if !u.GetBot() {
				peer.Users[i] = bot
			}
			return
		}
	}
	peer.Users = append(peer.Users, bot)
}
