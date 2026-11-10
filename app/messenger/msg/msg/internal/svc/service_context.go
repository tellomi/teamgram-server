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

package svc

import (
	"context"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/proto/mtproto"
	inbox_client "github.com/teamgram/teamgram-server/app/messenger/msg/inbox/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/internal/config"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/plugin"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	channelpb "github.com/teamgram/teamgram-server/app/service/biz/channel"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/teamgram/teamgram-server/app/service/ttlmedia"
	"github.com/teamgram/teamgram-server/pkg/archive"
	"github.com/teamgram/teamgram-server/pkg/deduplication"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type ServiceContext struct {
	Config config.Config
	plugin.MsgPlugin
	*dao.Dao
}

func NewServiceContext(c config.Config, plugin plugin.MsgPlugin) *ServiceContext {
	db := bootstrap.MustMySQL(&c.Mysql)
	svcCtx := &ServiceContext{
		Config:    c,
		MsgPlugin: plugin,
		Dao: &dao.Dao{
			Mysql:              dao.NewMysqlDao(db, c.MessageSharding),
			CachedConn:         sqlc.NewConn(db, c.Cache),
			IDGenClient2:       idgen_client.NewIDGenClient2(rpcx.GetCachedRpcClient(c.IdgenClient)),
			UserClient:         user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
			InboxClient:        inbox_client.NewInboxMqClient(bootstrap.MustKafkaProducer(c.InboxClient)),
			ChatClient:         chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient)),
			SyncClient:         sync_client.NewSyncMqClient(bootstrap.GetCachedMQClient(c.SyncClient)),
			DialogClient:       dialog_client.NewDialogClient(rpcx.GetCachedRpcClient(c.DialogClient)),
			MessageDeDuplicate: deduplication.NewMessageDeDuplicate(kv.NewStore(c.KV)),
			Redis:              bootstrap.MustRedis(c.Redis2),
		},
	}

	if plugin == nil {
		svcCtx.MsgPlugin = svcCtx.Dao
	}
	installChannelHooks(c, svcCtx.Dao)
	installStoryPaint(c, svcCtx.Dao.UserClient)
	if st, err := ttlmedia.Open(c.Mysql.DSN); err != nil {
		logx.Errorf("ttl media: %v", err)
	} else {
		ttlmedia.Start(st)
	}
	return svcCtx
}

// installStoryPaint lets message updates render recentStory from active
// stories. The user cache can still hold a max after expiry; without this,
// the next message paints the ring back on. The expiry watcher stays in the
// bff process.
func installStoryPaint(c config.Config, users user_client.UserClient) {
	if c.Mysql.DSN == "" {
		return
	}
	if err := story.UseDSN(c.Mysql.DSN); err != nil {
		logx.Errorf("story mysql: %v", err)
		return
	}
	story.Shared().SetHooks(story.Hooks{
		IsContact: func(ctx context.Context, owner, viewer int64) bool {
			if users == nil {
				return false
			}
			ok, err := users.UserCheckContact(ctx, &user.TLUserCheckContact{UserId: owner, Id: viewer})
			return err == nil && mtproto.FromBool(ok)
		},
		IsCloseFriend: func(ctx context.Context, owner, viewer int64) bool {
			if users == nil {
				return false
			}
			contact, err := users.UserGetContact(ctx, &user.TLUserGetContact{UserId: owner, Id: viewer})
			return err == nil && contact.GetCloseFriend()
		},
	})
	story.EnableUserRingPaint()
}

func installChannelHooks(c config.Config, d *dao.Dao) {
	if err := channelpb.UseDSN(c.Mysql.DSN); err != nil {
		logx.Errorf("channel mysql: %v", err)
	}
	if err := chatbox.UseDSN(c.Mysql.DSN); err != nil {
		logx.Errorf("chatbox mysql: %v", err)
	}
	channelpb.Shared().SetHooks(channelpb.Hooks{
		Push: func(ctx context.Context, userID int64, excludeAuthKeys []int64, updates *mtproto.Updates) {
			if d.SyncClient == nil || updates == nil {
				return
			}
			if len(excludeAuthKeys) > 0 {
				_, _ = d.SyncClient.SyncPushUpdatesIfNot(ctx, &sync.TLSyncPushUpdatesIfNot{
					UserId:   userID,
					Excludes: excludeAuthKeys,
					Updates:  updates,
				})
				return
			}
			_, _ = d.SyncClient.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{UserId: userID, Updates: updates})
		},
		Dialog: func(ctx context.Context, userID, channelID int64, top, readInbox, readOutbox, unread int32, date int64) {
			if d.DialogClient == nil {
				return
			}
			_, _ = d.DialogClient.DialogInsertOrUpdateDialog(ctx, &dialog.TLDialogInsertOrUpdateDialog{
				UserId:          userID,
				PeerType:        mtproto.PEER_CHANNEL,
				PeerId:          channelID,
				TopMessage:      &wrapperspb.Int32Value{Value: top},
				ReadInboxMaxId:  &wrapperspb.Int32Value{Value: readInbox},
				ReadOutboxMaxId: &wrapperspb.Int32Value{Value: readOutbox},
				UnreadCount:     &wrapperspb.Int32Value{Value: unread},
				Date2:           &wrapperspb.Int64Value{Value: date},
			})
		},
		Incoming: func(ctx context.Context, userID, channelID, sender int64, broadcast bool) {
			class := archive.ClassGroup
			if broadcast {
				class = archive.ClassBroadcast
			}
			ups := d.ArchiveIncoming(ctx, archive.Incoming{
				Owner:    userID,
				PeerType: mtproto.PEER_CHANNEL,
				PeerID:   channelID,
				Sender:   sender,
				Class:    class,
			})
			if len(ups) == 0 || d.SyncClient == nil {
				return
			}
			_, _ = d.SyncClient.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{
				UserId:  userID,
				Updates: mtproto.MakeUpdatesByUpdates(ups...),
			})
		},
	})
}
