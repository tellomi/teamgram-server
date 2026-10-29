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

	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/marmota/pkg/stores/kv"
	"github.com/teamgram/marmota/pkg/stores/sqlc"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	inbox_client "github.com/teamgram/teamgram-server/app/messenger/msg/inbox/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/internal/config"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/plugin"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	channelpb "github.com/teamgram/teamgram-server/app/service/biz/channel"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/teamgram/teamgram-server/pkg/deduplication"

	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type ServiceContext struct {
	Config config.Config
	plugin.MsgPlugin
	*dao.Dao
}

func NewServiceContext(c config.Config, plugin plugin.MsgPlugin) *ServiceContext {
	db := sqlx.NewMySQL(&c.Mysql)
	svcCtx := &ServiceContext{
		Config:    c,
		MsgPlugin: plugin,
		Dao: &dao.Dao{
			Mysql:              dao.NewMysqlDao(db, c.MessageSharding),
			CachedConn:         sqlc.NewConn(db, c.Cache),
			IDGenClient2:       idgen_client.NewIDGenClient2(rpcx.GetCachedRpcClient(c.IdgenClient)),
			UserClient:         user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
			InboxClient:        inbox_client.NewInboxMqClient(kafka.MustKafkaProducer(c.InboxClient)),
			ChatClient:         chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient)),
			SyncClient:         sync_client.NewSyncMqClient(kafka.GetCachedMQClient(c.SyncClient)),
			DialogClient:       dialog_client.NewDialogClient(rpcx.GetCachedRpcClient(c.DialogClient)),
			MessageDeDuplicate: deduplication.NewMessageDeDuplicate(kv.NewStore(c.KV)),
			Redis:              redis.MustNewRedis(c.Redis2),
		},
	}

	if plugin == nil {
		svcCtx.MsgPlugin = svcCtx.Dao
	}
	installChannelHooks(c, svcCtx.Dao)
	return svcCtx
}

func installChannelHooks(c config.Config, d *dao.Dao) {
	if err := channelpb.UseDSN(c.Mysql.DSN); err != nil {
		logx.Errorf("channel mysql: %v", err)
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
		Dialog: func(ctx context.Context, userID, channelID int64, top, readInbox, unread int32, date int64) {
			if d.DialogClient == nil {
				return
			}
			_, _ = d.DialogClient.DialogInsertOrUpdateDialog(ctx, &dialog.TLDialogInsertOrUpdateDialog{
				UserId:         userID,
				PeerType:       mtproto.PEER_CHANNEL,
				PeerId:         channelID,
				TopMessage:     &wrapperspb.Int32Value{Value: top},
				ReadInboxMaxId: &wrapperspb.Int32Value{Value: readInbox},
				UnreadCount:    &wrapperspb.Int32Value{Value: unread},
				Date2:          &wrapperspb.Int64Value{Value: date},
			})
		},
	})
}
