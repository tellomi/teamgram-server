package svc

import (
	"context"

	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/config"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	media_client "github.com/teamgram/teamgram-server/app/service/media/client"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type ServiceContext struct {
	Config      config.Config
	UserClient  user_client.UserClient
	MediaClient media_client.MediaClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	if err := channel.UseDSN(c.ChannelMySQL); err != nil {
		logx.Errorf("channel mysql: %v", err)
	}
	var users user_client.UserClient
	if len(c.UserClient.Etcd.Hosts) > 0 {
		users = user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient))
	}
	var mediaClient media_client.MediaClient
	if len(c.MediaClient.Etcd.Hosts) > 0 {
		mediaClient = media_client.NewMediaClient(rpcx.GetCachedRpcClient(c.MediaClient))
	}
	installChannelHooks(c)
	return &ServiceContext{Config: c, UserClient: users, MediaClient: mediaClient}
}

func installChannelHooks(c config.Config) {
	var dialogs dialog_client.DialogClient
	if len(c.DialogClient.Etcd.Hosts) > 0 {
		dialogs = dialog_client.NewDialogClient(rpcx.GetCachedRpcClient(c.DialogClient))
	}
	var syncer sync_client.SyncClient
	if c.SyncClient != nil && len(c.SyncClient.Brokers) > 0 {
		syncer = sync_client.NewSyncMqClient(kafka.MustKafkaProducer(c.SyncClient))
	}
	channel.Shared().SetHooks(channel.Hooks{
		Push: func(ctx context.Context, userID int64, excludeAuthKeys []int64, updates *mtproto.Updates) {
			if syncer == nil || updates == nil {
				return
			}
			if len(excludeAuthKeys) > 0 {
				_, _ = syncer.SyncPushUpdatesIfNot(ctx, &sync.TLSyncPushUpdatesIfNot{
					UserId:   userID,
					Excludes: excludeAuthKeys,
					Updates:  updates,
				})
				return
			}
			_, _ = syncer.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{
				UserId:  userID,
				Updates: updates,
			})
		},
		Dialog: func(ctx context.Context, userID, channelID int64, top, readInbox, unread int32, date int64) {
			if dialogs == nil {
				return
			}
			_, _ = dialogs.DialogInsertOrUpdateDialog(ctx, &dialog.TLDialogInsertOrUpdateDialog{
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
