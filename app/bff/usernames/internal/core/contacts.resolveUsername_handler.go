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
	name := normalizeUsername(in.GetUsername())
	if name == "" {
		return nil, mtproto.ErrUsernameInvalid
	}
	if id := userpb.GetBotIdByName(name); id > 0 {
		return c.resolveBuiltin(name, id)
	}
	return c.resolveOccupied(name)
}

// resolveBuiltin returns the reserved bot. A real row is used only when that
// row is the bot's own user id. A placeholder (peer_type 0, which resolve
// reports as USERNAME_NOT_OCCUPIED) or any other user or channel still
// resolves to the official bot, so @bing cannot be given away.
//
// @venue is the one historical exception: a peer that already holds it keeps
// the name. New claims of venue, and of every other reserved bot, are rejected.
func (c *UsernamesCore) resolveBuiltin(name string, botID int64) (*mtproto.Contacts_ResolvedPeer, error) {
	looked, err := c.svcCtx.Dao.UserClient.UserResolveUsername(c.ctx, &userpb.TLUserResolveUsername{
		Username: name,
	})
	if err != nil && !usernameNotOccupied(err) {
		c.Logger.Errorf("contacts.resolveUsername - reply: {%v}", err)
		return nil, err
	}
	if err == nil && looked.GetUserId() == botID && botID != 0 {
		return c.finishResolved(name, mtproto.FromPeer(looked))
	}
	if name == mtproto.BotVenueName && err == nil && (looked.GetUserId() != 0 || looked.GetChannelId() != 0) {
		return c.finishResolved(name, mtproto.FromPeer(looked))
	}
	return c.finishResolved(name, mtproto.MakeUserPeerUtil(botID))
}

func (c *UsernamesCore) resolveOccupied(name string) (*mtproto.Contacts_ResolvedPeer, error) {
	rName, err := c.svcCtx.Dao.UserClient.UserResolveUsername(c.ctx, &userpb.TLUserResolveUsername{
		Username: name,
	})
	if err != nil {
		if ch, chErr := channel.Shared().ResolveUsername(c.ctx, c.MD.UserId, name); chErr == nil && ch != nil {
			return mtproto.MakeTLContactsResolvedPeer(&mtproto.Contacts_ResolvedPeer{
				Peer:  mtproto.MakePeerChannel(ch.GetId()),
				Chats: []*mtproto.Chat{ch},
				Users: []*mtproto.User{},
			}).To_Contacts_ResolvedPeer(), nil
		}
		c.Logger.Errorf("contacts.resolveUsername - reply: {%v}", err)
		return nil, err
	}
	return c.finishResolved(name, mtproto.FromPeer(rName))
}

func (c *UsernamesCore) finishResolved(username string, peer *mtproto.PeerUtil) (*mtproto.Contacts_ResolvedPeer, error) {
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
		if mUsers != nil {
			resolvedPeer.Users = mUsers.GetUserListByIdList(c.MD.UserId, peer.PeerId)
		}
		if bot := catalog.BuiltinBotUser(peer.PeerId); bot != nil {
			ensureBuiltinBot(resolvedPeer, bot)
		}
		if !userListed(resolvedPeer, peer.PeerId) {
			return nil, mtproto.ErrUsernameNotOccupied
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
	return resolvedPeer, nil
}

func normalizeUsername(username string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(username)), "@")
}

// usernameIsFree reports whether nobody holds the name. A lookup failure
// other than USERNAME_NOT_OCCUPIED is fatal: a built-in bot must not take
// the name when the table cannot be read.
func usernameIsFree(err error, peer *mtproto.Peer) (bool, error) {
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

// rejectReservedBotClaim blocks a new claim of any reserved bot username.
// Matching is case-insensitive. The peer that already holds the name can
// keep it. A free name, a placeholder, and a name held by someone else are
// USERNAME_OCCUPIED. A lookup failure is returned and is not treated as free.
func (c *UsernamesCore) rejectReservedBotClaim(username string, holderType int32, holderID int64) error {
	if userpb.GetBotIdByName(username) == 0 {
		return nil
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return mtproto.ErrUsernameOccupied
	}
	peer, err := c.svcCtx.Dao.UserClient.UserResolveUsername(c.ctx, &userpb.TLUserResolveUsername{
		Username: normalizeUsername(username),
	})
	if err == nil && holderID != 0 && peerHolds(peer, holderType, holderID) {
		return nil
	}
	_, fatal := usernameIsFree(err, peer)
	if fatal != nil {
		return fatal
	}
	return mtproto.ErrUsernameOccupied
}

func peerHolds(peer *mtproto.Peer, holderType int32, holderID int64) bool {
	if peer == nil || holderID == 0 {
		return false
	}
	switch holderType {
	case mtproto.PEER_USER:
		return peer.GetUserId() == holderID
	case mtproto.PEER_CHANNEL:
		return peer.GetChannelId() == holderID
	default:
		return false
	}
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

func userListed(peer *mtproto.Contacts_ResolvedPeer, id int64) bool {
	if peer == nil || id == 0 {
		return false
	}
	for _, u := range peer.Users {
		if u.GetId() == id && u.GetPredicateName() != mtproto.Predicate_userEmpty {
			return true
		}
	}
	return false
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
