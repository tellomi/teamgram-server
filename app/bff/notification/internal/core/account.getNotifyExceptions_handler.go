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
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// AccountGetNotifyExceptions
// account.getNotifyExceptions#53577479 flags:# compare_sound:flags.1?true compare_stories:flags.2?true peer:flags.0?InputNotifyPeer = Updates;
func (c *NotificationCore) AccountGetNotifyExceptions(in *mtproto.TLAccountGetNotifyExceptions) (*mtproto.Updates, error) {
	// compare_sound keeps the #421 rule, including stories sounds.
	// compare_stories is not a separate filter.
	scope, err := parseNotifyExceptionScope(c.MD.UserId, in.GetPeer())
	if err != nil {
		c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
		return nil, err
	}

	settings, err := c.svcCtx.Dao.UserClient.UserGetAllNotifySettings(c.ctx, &userpb.TLUserGetAllNotifySettings{
		UserId: c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
		return nil, err
	}

	var (
		rUpdates = mtproto.MakeEmptyUpdates()
		idHelper = mtproto.NewIDListHelper()
		kinds    = map[int64]channelBroadcastKind{}
	)

	for _, setting := range settings.GetDatas() {
		// compare_sound keeps exceptions whose ringtone differs from the
		// default. A mute-only row is not a sound exception.
		if in.GetCompareSound() && !notifySoundCustom(setting.GetSettings()) {
			continue
		}
		kept, err := c.notifyExceptionKept(scope, setting, kinds)
		if err != nil {
			c.Logger.Errorf("account.getNotifyExceptions - error: %v", err)
			return nil, err
		}
		if !kept {
			continue
		}
		peer := mtproto.MakePeerUtil(setting.PeerType, setting.PeerId)
		idHelper.PickByPeerUtil(peer.PeerType, peer.PeerId)
		rUpdates.PushBackUpdate(mtproto.MakeTLUpdateNotifySettings(&mtproto.Update{
			Peer_NOTIFYPEER: mtproto.NotifyPeerWithTopic(peer, setting.GetTopMsgId()),
			NotifySettings:  setting.Settings,
		}).To_Update())
	}

	idHelper.Visit(
		func(userIdList []int64) {
			users, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
				&userpb.TLUserGetMutableUsers{
					Id: append(userIdList, c.MD.UserId),
				})
			rUpdates.Users = users.GetUserListByIdList(c.MD.UserId, userIdList...)
		},
		func(chatIdList []int64) {
			chats, _ := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx,
				&chatpb.TLChatGetChatListByIdList{
					IdList: chatIdList,
				})
			rUpdates.PushChat(chats.GetChatListByIdList(c.MD.UserId, chatIdList...)...)
		},

		func(channelIdList []int64) {
			if c.svcCtx != nil && c.svcCtx.Plugin != nil {
				rUpdates.PushChat(c.svcCtx.Plugin.GetChannelListByIdList(c.ctx, c.MD.UserId, channelIdList...)...)
				return
			}
			rUpdates.PushChat(channel.Shared().ChatsFor(c.ctx, c.MD.UserId, channelIdList...)...)
		})

	return rUpdates, nil
}

func notifySoundCustom(s *mtproto.PeerNotifySettings) bool {
	if s == nil {
		return false
	}
	return customNotifySound(s.GetIosSound()) || customNotifySound(s.GetAndroidSound()) || customNotifySound(s.GetOtherSound()) ||
		customNotifySound(s.GetStoriesIosSound()) || customNotifySound(s.GetStoriesAndroidSound()) || customNotifySound(s.GetStoriesOtherSound())
}

func customNotifySound(s *mtproto.NotificationSound) bool {
	if s == nil {
		return false
	}
	name := s.GetPredicateName()
	return name != "" && name != mtproto.Predicate_notificationSoundDefault
}

type notifyScopeKind int

const (
	notifyScopeAll notifyScopeKind = iota
	notifyScopeUsers
	notifyScopeChats
	notifyScopeBroadcasts
	notifyScopePeer
	notifyScopeTopic
)

type notifyExceptionScope struct {
	kind     notifyScopeKind
	peerType int32
	peerID   int64
	topMsgID int32
}

type channelBroadcastKind struct {
	broadcast bool
	known     bool
}

// parseNotifyExceptionScope reads the optional peer.
// An unset peer returns every exception. inputNotifyUsers, inputNotifyChats
// and inputNotifyBroadcasts are categories. inputNotifyPeer is one dialog,
// including its topic rows. inputNotifyForumTopic is that one topic row.
func parseNotifyExceptionScope(selfID int64, peer *mtproto.InputNotifyPeer) (notifyExceptionScope, error) {
	if peer == nil || peer.GetPredicateName() == "" {
		return notifyExceptionScope{kind: notifyScopeAll}, nil
	}
	if peer.GetPredicateName() == mtproto.Predicate_inputNotifyPeer && peer.GetPeer() == nil {
		return notifyExceptionScope{}, mtproto.ErrPeerIdInvalid
	}
	parsed, top := mtproto.ParseInputNotifyPeer(selfID, peer)
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputNotifyUsers:
		return notifyExceptionScope{kind: notifyScopeUsers}, nil
	case mtproto.Predicate_inputNotifyChats:
		return notifyExceptionScope{kind: notifyScopeChats}, nil
	case mtproto.Predicate_inputNotifyBroadcasts:
		return notifyExceptionScope{kind: notifyScopeBroadcasts}, nil
	case mtproto.Predicate_inputNotifyPeer:
		pt, id, err := notifyDialogPeer(parsed)
		if err != nil {
			return notifyExceptionScope{}, err
		}
		return notifyExceptionScope{kind: notifyScopePeer, peerType: pt, peerID: id}, nil
	case mtproto.Predicate_inputNotifyForumTopic:
		if top <= 0 {
			return notifyExceptionScope{}, mtproto.ErrTopicIdInvalid
		}
		pt, id, err := notifyDialogPeer(parsed)
		if err != nil {
			return notifyExceptionScope{}, err
		}
		return notifyExceptionScope{kind: notifyScopeTopic, peerType: pt, peerID: id, topMsgID: top}, nil
	default:
		return notifyExceptionScope{}, mtproto.ErrPeerIdInvalid
	}
}

func notifyDialogPeer(p *mtproto.PeerUtil) (int32, int64, error) {
	if p == nil {
		return 0, 0, mtproto.ErrPeerIdInvalid
	}
	pt, id := p.PeerType, p.PeerId
	if pt == mtproto.PEER_SELF {
		pt = mtproto.PEER_USER
	}
	if pt != mtproto.PEER_USER && pt != mtproto.PEER_CHAT && pt != mtproto.PEER_CHANNEL {
		return 0, 0, mtproto.ErrPeerIdInvalid
	}
	if id == 0 {
		return 0, 0, mtproto.ErrPeerIdInvalid
	}
	return pt, id, nil
}

func (c *NotificationCore) notifyExceptionKept(scope notifyExceptionScope, setting *userpb.PeerPeerNotifySettings, kinds map[int64]channelBroadcastKind) (bool, error) {
	if setting == nil {
		return false, nil
	}
	switch scope.kind {
	case notifyScopeAll:
		return true, nil
	case notifyScopeUsers:
		return setting.GetPeerType() == mtproto.PEER_USER, nil
	case notifyScopeChats:
		return c.channelInChats(setting, kinds)
	case notifyScopeBroadcasts:
		return c.channelInBroadcasts(setting, kinds)
	case notifyScopePeer:
		return setting.GetPeerType() == scope.peerType && setting.GetPeerId() == scope.peerID, nil
	case notifyScopeTopic:
		return setting.GetPeerType() == scope.peerType && setting.GetPeerId() == scope.peerID && setting.GetTopMsgId() == scope.topMsgID, nil
	default:
		return false, nil
	}
}

// channelInChats keeps a basic group and a non-broadcast channel.
// The channel record's broadcast flag decides. PeerType CHANNEL alone does not.
func (c *NotificationCore) channelInChats(setting *userpb.PeerPeerNotifySettings, kinds map[int64]channelBroadcastKind) (bool, error) {
	switch setting.GetPeerType() {
	case mtproto.PEER_CHAT:
		return true, nil
	case mtproto.PEER_CHANNEL:
		broadcast, known, err := c.channelBroadcast(setting.GetPeerId(), kinds)
		if err != nil {
			return false, err
		}
		return known && !broadcast, nil
	default:
		return false, nil
	}
}

func (c *NotificationCore) channelInBroadcasts(setting *userpb.PeerPeerNotifySettings, kinds map[int64]channelBroadcastKind) (bool, error) {
	if setting.GetPeerType() != mtproto.PEER_CHANNEL {
		return false, nil
	}
	broadcast, known, err := c.channelBroadcast(setting.GetPeerId(), kinds)
	if err != nil {
		return false, err
	}
	return known && broadcast, nil
}

func (c *NotificationCore) channelBroadcast(id int64, kinds map[int64]channelBroadcastKind) (bool, bool, error) {
	if kinds != nil {
		if v, ok := kinds[id]; ok {
			return v.broadcast, v.known, nil
		}
	}
	broadcast, known, err := c.lookupChannelBroadcast(id)
	if err != nil {
		return false, false, err
	}
	if kinds != nil {
		kinds[id] = channelBroadcastKind{broadcast: broadcast, known: known}
	}
	return broadcast, known, nil
}

func (c *NotificationCore) lookupChannelBroadcast(id int64) (bool, bool, error) {
	if c.svcCtx != nil && c.svcCtx.Plugin != nil {
		chat, err := c.svcCtx.Plugin.GetChannelById(c.ctx, c.MD.UserId, id)
		if err != nil {
			if err == mtproto.ErrChannelInvalid || err == mtproto.ErrChannelPrivate || err == mtproto.ErrPeerIdInvalid {
				return false, false, nil
			}
			return false, false, err
		}
		if chat == nil || chat.GetId() == 0 {
			return false, false, nil
		}
		return chat.GetBroadcast(), true, nil
	}
	return channel.Shared().IsBroadcast(c.ctx, id)
}
