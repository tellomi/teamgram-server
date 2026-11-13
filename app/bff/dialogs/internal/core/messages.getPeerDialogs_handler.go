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
	"context"

	"github.com/teamgram/proto/mtproto"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"github.com/zeromicro/go-zero/core/mr"
)

// MessagesGetPeerDialogs
// messages.getPeerDialogs#e470bcfd peers:Vector<InputDialogPeer> = messages.PeerDialogs;
func (c *DialogsCore) MessagesGetPeerDialogs(in *mtproto.TLMessagesGetPeerDialogs) (*mtproto.Messages_PeerDialogs, error) {
	var (
		peerDialogIdList []int64
		folderId         int32 = -1
		peers            []*mtproto.PeerUtil
	)

	for _, peer := range in.GetPeers() {
		switch peer.GetPredicateName() {
		case mtproto.Predicate_inputDialogPeer:
			p := mtproto.FromInputPeer2(c.MD.UserId, peer.Peer)
			switch p.PeerType {
			case mtproto.PEER_SELF:
			case mtproto.PEER_USER:
			case mtproto.PEER_CHAT:
			case mtproto.PEER_CHANNEL:
			default:
				err := mtproto.ErrInputConstructorInvalid
				c.Logger.Errorf("messages.getPeerDialogs - getPeerDialogs error: %v", err)
				return nil, err
			}
			peerDialogIdList = append(peerDialogIdList, mtproto.MakePeerDialogId(p.PeerType, p.PeerId))
			peers = append(peers, p)
		case mtproto.Predicate_inputDialogPeerFolder:
			// TODO: check folderId == 1
			if folderId == -1 {
				folderId = peer.FolderId
			} else {
				// if has inputDialogPeerFolder, len(peers) == 1
				err := mtproto.ErrFolderIdInvalid
				c.Logger.Errorf("messages.getPeerDialogs - getPeerDialogs error: %v", err)
				return nil, err
			}
		default:
			err := mtproto.ErrInputConstructorInvalid
			c.Logger.Errorf("messages.getPeerDialogs - getPeerDialogs error: %v", err)
			return nil, err
		}
	}

	var (
		err                error
		dialogExtList      dialog.DialogExtList
		state              *mtproto.Updates_State
		notifySettingsList []*userpb.PeerPeerNotifySettings
	)

	// Same cursor rule as getPinnedDialogs: state is read before the dialog
	// rows so getDifference from this state still covers a write that lands
	// during the dialog load.
	state, err = c.svcCtx.Dao.UpdatesClient.UpdatesGetStateV2(c.ctx, &updates.TLUpdatesGetStateV2{
		AuthKeyId: c.MD.PermAuthKeyId,
		UserId:    c.MD.UserId,
	})
	if err != nil {
		c.Logger.Errorf("messages.getPeerDialogs - getState error: %v", err)
		return nil, err
	}

	err = mr.Finish(
		func() error {
			if len(peerDialogIdList) > 0 {
				dList, err2 := c.svcCtx.Dao.DialogClient.DialogGetDialogsByIdList(c.ctx, &dialog.TLDialogGetDialogsByIdList{
					UserId: c.MD.UserId,
					IdList: peerDialogIdList,
				})
				if err2 != nil {
					c.Logger.Errorf("messages.getPeerDialogs - getPeerDialogs error: %v", err2)
					return err2
				}
				dialogExtList = dList.GetDatas()
			} else if folderId != -1 {
				dList, err2 := c.svcCtx.Dao.DialogClient.DialogGetDialogFolder(c.ctx, &dialog.TLDialogGetDialogFolder{
					UserId:   c.MD.UserId,
					FolderId: folderId,
				})
				if err2 != nil {
					c.Logger.Errorf("messages.getPeerDialogs - getPeerDialogs error: %v", err2)
					return err2
				}
				dialogExtList = dList.GetDatas()
			}

			return nil
		},
		func() error {
			if len(peers) > 0 {
				settingsList, err2 := c.svcCtx.Dao.UserClient.UserGetNotifySettingsList(c.ctx, &userpb.TLUserGetNotifySettingsList{
					UserId: c.MD.UserId,
					Peers:  peers,
				})
				if err2 != nil {
					c.Logger.Errorf("messages.getDialogs - error: %v", err2)
					return err2
				}
				notifySettingsList = settingsList.GetDatas()
			}

			return nil
		},
	)
	if err != nil {
		c.Logger.Errorf("messages.getPeerDialogs - getPeerDialogs error: %v", err)
		return nil, err
	}

	dialogExtList = c.ensureChannelPeers(dialogExtList, peers)
	for _, dialogEx := range dialogExtList {
		peer2 := mtproto.FromPeer(dialogEx.GetDialog().GetPeer())
		dialogEx.Dialog.NotifySettings = userpb.FindPeerPeerNotifySettings(notifySettingsList, peer2)
	}

	messageDialogs := dialogExtList.DoGetMessagesDialogs(
		c.ctx,
		c.MD.UserId,
		func(ctx context.Context, selfUserId int64, id ...dialog.TopMessageId) []*mtproto.Message {
			var (
				msgList   = make([]*mtproto.Message, 0, len(id))
				msgIdList = make([]int32, 0, len(id))
			)
			var channelIDs []dialog.TopMessageId
			for _, id2 := range id {
				if id2.Peer.IsChannel() {
					channelIDs = append(channelIDs, id2)
				} else if id2.TopMessage != 0 {
					msgIdList = append(msgIdList, id2.TopMessage)
				}
			}
			msgList = append(msgList, c.channelMessages(ctx, selfUserId, channelIDs)...)
			if len(msgIdList) > 0 {
				boxList, _ := c.svcCtx.Dao.MessageClient.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
					UserId: c.MD.UserId,
					IdList: msgIdList,
				})
				boxList.Walk(func(idx int, v *mtproto.MessageBox) {
					msgList = append(msgList, v.ToMessage(c.MD.UserId))
				})
			}

			return msgList
		},
		func(ctx context.Context, selfUserId int64, id ...int64) []*mtproto.User {
			users, _ := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx,
				&userpb.TLUserGetMutableUsers{
					Id: id,
				})

			return users.GetUserListByIdList(c.MD.UserId, id...)
		},
		func(ctx context.Context, selfUserId int64, id ...int64) []*mtproto.Chat {
			chats, _ := c.svcCtx.Dao.ChatClient.ChatGetChatListByIdList(c.ctx,
				&chatpb.TLChatGetChatListByIdList{
					IdList: id,
				})

			return chats.GetChatListByIdList(c.MD.UserId, id...)
		},
		func(ctx context.Context, selfUserId int64, id ...int64) []*mtproto.Chat {
			return c.channelChats(ctx, selfUserId, id...)
		})

	return messageDialogs.ToMessagesPeerDialogs(state), nil
}
