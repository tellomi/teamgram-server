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
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

/*
 Android client's account.updateNotifySettings#84be5b93 source code:

	TLRPC.TL_account_updateNotifySettings req = new TLRPC.TL_account_updateNotifySettings();
	req.settings = new TLRPC.TL_inputPeerNotifySettings();
	req.settings.sound = "default";
	int mute_type = preferences.getInt("notify2_" + dialog_id, 0);
	if (mute_type == 3) {
		req.settings.mute_until = preferences.getInt("notifyuntil_" + dialog_id, 0);
	} else {
		req.settings.mute_until = mute_type != 2 ? 0 : Integer.MAX_VALUE;
	}
	req.settings.show_previews = preferences.getBoolean("preview_" + dialog_id, true);
	req.settings.silent = preferences.getBoolean("silent_" + dialog_id, false);
	req.peer = new TLRPC.TL_inputNotifyPeer();
	((TLRPC.TL_inputNotifyPeer) req.peer).peer = MessagesController.getInputPeer((int) dialog_id);
*/

// AccountUpdateNotifySettings
// account.updateNotifySettings#84be5b93 peer:InputNotifyPeer settings:InputPeerNotifySettings = Bool;
func (c *NotificationCore) AccountUpdateNotifySettings(in *mtproto.TLAccountUpdateNotifySettings) (*mtproto.Bool, error) {
	// account.updateNotifySettings
	var (
		err         error
		peerUser    *mtproto.User
		peerChat    *mtproto.Chat
		peerChannel *mtproto.Chat
	)

	settings, err := mtproto.MakePeerNotifySettings(in.GetSettings(), mtproto.NotifySoundSlot(c.MD.GetLangpack(), c.MD.GetClient()))
	if err != nil {
		c.Logger.Errorf("account.updateNotifySettings - error: %v", err)
		return nil, err
	}

	peer := mtproto.FromInputNotifyPeer(c.MD.UserId, in.GetPeer())
	switch peer.PeerType {
	case mtproto.PEER_USERS:
	case mtproto.PEER_CHATS:
	case mtproto.PEER_BROADCASTS:
	case mtproto.PEER_USER:
		if users, err2 := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{
			Id: []int64{c.MD.UserId, peer.PeerId},
		}); err2 != nil {
			c.Logger.Errorf("account.updateNotifySettings - error: %v", err)
			err2 = mtproto.ErrPeerIdInvalid
			return nil, err2
		} else {
			peerUser, _ = users.GetUnsafeUser(c.MD.UserId, peer.PeerId)
			_ = peerUser
		}
	case mtproto.PEER_CHAT:
		peerChat2, err2 := c.svcCtx.Dao.ChatClient.ChatGetMutableChat(c.ctx, &chat.TLChatGetMutableChat{
			ChatId: peer.PeerId,
		})
		if err2 != nil {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("account.updateNotifySettings - error: %v", err)
			return nil, err
		}
		me, _ := peerChat2.GetImmutableChatParticipant(c.MD.UserId)
		if me == nil || !me.IsChatMemberStateNormal() {
			err = mtproto.ErrPeerIdInvalid
			c.Logger.Errorf("account.updateNotifySettings - error: %v", err)
			return nil, err
		}

		peerChat = peerChat2.ToUnsafeChat(c.MD.UserId)
	case mtproto.PEER_CHANNEL:
		peerChannel, err = c.channelForNotify(peer.PeerId)
		if err != nil {
			c.Logger.Errorf("account.updateNotifySettings - error: %v", err)
			return nil, err
		}
	default:
		err = mtproto.ErrPeerIdInvalid
		c.Logger.Errorf("account.updateNotifySettings - error: %v", err)
		return nil, err
	}

	_, err = c.svcCtx.Dao.UserClient.UserSetNotifySettings(c.ctx, &userpb.TLUserSetNotifySettings{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
		Settings: settings,
	})
	if err != nil {
		c.Logger.Errorf("account.updateNotifySettings - error: %v", err)
		return nil, err
	}
	stored, err := c.svcCtx.Dao.UserClient.UserGetNotifySettings(c.ctx, &userpb.TLUserGetNotifySettings{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
	})
	if err != nil {
		c.Logger.Errorf("account.updateNotifySettings - error: %v", err)
		return nil, err
	}
	if stored != nil {
		settings = stored
	}

	// Other sessions of this user. The current session already applied the
	// change locally. updateNotifySettings has no pts, so the live push is
	// what an online device uses; a later getNotifySettings reads the row.
	syncNotMeUpdates := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNotifySettings(&mtproto.Update{
		Peer_NOTIFYPEER: peer.ToNotifyPeer(),
		NotifySettings:  settings,
	}).To_Update())

	switch peer.PeerType {
	case mtproto.PEER_USER:
		syncNotMeUpdates.AddSafeUser(peerUser)
	case mtproto.PEER_CHAT:
		syncNotMeUpdates.AddSafeChat(peerChat)
	case mtproto.PEER_CHANNEL:
		syncNotMeUpdates.AddSafeChat(peerChannel)
	}

	if _, err = c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       syncNotMeUpdates,
	}); err != nil {
		c.Logger.Errorf("account.updateNotifySettings - error: %v", err)
		return nil, err
	}

	// return
	return mtproto.BoolTrue, nil
}

// channelForNotify loads the channel card attached to updateNotifySettings.
// The plugin is the usual path. Without it the channel service is used, so
// a missing enterprise plugin does not fail the RPC.
func (c *NotificationCore) channelForNotify(id int64) (*mtproto.Chat, error) {
	if c.svcCtx != nil && c.svcCtx.Plugin != nil {
		chat, err := c.svcCtx.Plugin.GetChannelById(c.ctx, c.MD.UserId, id)
		if err != nil {
			return nil, err
		}
		if chat.GetPredicateName() == mtproto.Predicate_channelForbidden {
			return nil, mtproto.ErrChannelPrivate
		}
		return chat, nil
	}
	chat := channel.Shared().VisibleChat(c.ctx, c.MD.UserId, id)
	if chat == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if chat.GetPredicateName() == mtproto.Predicate_channelForbidden {
		return nil, mtproto.ErrChannelPrivate
	}
	return chat, nil
}
