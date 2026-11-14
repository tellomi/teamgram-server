package svc

import (
	"context"
	"os"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/config"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	media_client "github.com/teamgram/teamgram-server/app/service/media/client"
	status_client "github.com/teamgram/teamgram-server/app/service/status/client"
	"github.com/teamgram/teamgram-server/app/service/status/status"
	"github.com/teamgram/teamgram-server/pkg/archive"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type ServiceContext struct {
	Config       config.Config
	UserClient   user_client.UserClient
	MediaClient  media_client.MediaClient
	DialogClient dialog_client.DialogClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	if err := channel.UseDSN(c.ChannelMySQL); err != nil {
		logx.Errorf("channel mysql: %v", err)
	}
	if err := chatbox.UseDSN(c.ChannelMySQL); err != nil {
		logx.Errorf("chatbox mysql: %v", err)
	}
	var users user_client.UserClient
	if len(c.UserClient.Etcd.Hosts) > 0 {
		users = user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient))
	}
	var mediaClient media_client.MediaClient
	if len(c.MediaClient.Etcd.Hosts) > 0 {
		mediaClient = media_client.NewMediaClient(rpcx.GetCachedRpcClient(c.MediaClient))
	}
	return &ServiceContext{
		Config:       c,
		UserClient:   users,
		MediaClient:  mediaClient,
		DialogClient: installChannelHooks(c, users),
	}
}

func installChannelHooks(c config.Config, users user_client.UserClient) dialog_client.DialogClient {
	var dialogs dialog_client.DialogClient
	if len(c.DialogClient.Etcd.Hosts) > 0 || len(c.DialogClient.Endpoints) > 0 {
		dialogs = dialog_client.NewDialogClient(rpcx.GetCachedRpcClient(c.DialogClient))
	}
	var syncer sync_client.SyncClient
	if c.SyncClient != nil && len(c.SyncClient.Brokers) > 0 {
		syncer = sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(c.SyncClient))
	}
	channel.Shared().SetHooks(channel.Hooks{
		Online: onlineMembers(c),
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
		Dialog: func(ctx context.Context, userID, channelID int64, top, readInbox, readOutbox, unread int32, date int64) {
			if dialogs == nil {
				return
			}
			_, _ = dialogs.DialogInsertOrUpdateDialog(ctx, &dialog.TLDialogInsertOrUpdateDialog{
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
		Incoming: channelIncoming(users, dialogs, syncer, channelPts(c)),
	})
	return dialogs
}

// onlineBatch is status.getUsersOnlineSessionsList's maximum.
const onlineBatch = 500

// onlineMembers asks the presence service which of the affected members
// have a session. An empty status config leaves the hook nil.
func onlineMembers(c config.Config) func(context.Context, []int64) (map[int64]bool, error) {
	if len(c.StatusClient.Etcd.Hosts) == 0 && len(c.StatusClient.Endpoints) == 0 {
		return nil
	}
	client := status_client.NewStatusClient(rpcx.GetCachedRpcClient(c.StatusClient))
	return func(ctx context.Context, ids []int64) (map[int64]bool, error) {
		out := map[int64]bool{}
		for start := 0; start < len(ids); start += onlineBatch {
			end := start + onlineBatch
			if end > len(ids) {
				end = len(ids)
			}
			res, err := client.StatusGetUsersOnlineSessionsList(ctx, &status.TLStatusGetUsersOnlineSessionsList{
				Users: ids[start:end],
			})
			if err != nil {
				return nil, err
			}
			for _, row := range res.GetDatas() {
				if row != nil && len(row.GetUserSessions()) > 0 {
					out[row.GetUserId()] = true
				}
			}
		}
		return out, nil
	}
}

func channelPts(c config.Config) archive.SeqWriter {
	var next archive.SeqPts
	if len(c.IdgenClient.Etcd.Hosts) > 0 || len(c.IdgenClient.Endpoints) > 0 {
		gen := idgen_client.NewIDGenClient2(rpcx.GetCachedRpcClient(c.IdgenClient))
		next = &gen
	}
	dsn := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(c.ChannelMySQL)
	}
	var save archive.SavePts
	if dsn != "" {
		save = archive.SavePtsSQL(mysqltls.NewMySQL(&sqlx.Config{DSN: dsn}))
	}
	return archive.SeqWriter{Next: next, Save: save}
}

func channelIncoming(users user_client.UserClient, dialogs dialog_client.DialogClient, syncer sync_client.SyncClient, pts archive.SeqWriter) func(context.Context, int64, int64, int64, bool) {
	return func(ctx context.Context, userID, channelID, sender int64, broadcast bool) {
		if users == nil || dialogs == nil {
			return
		}
		class := archive.ClassGroup
		if broadcast {
			class = archive.ClassBroadcast
		}
		ups := archive.Apply(ctx, archive.Clients{Dialog: dialogs, User: users}, pts, archive.Incoming{
			Owner:    userID,
			PeerType: mtproto.PEER_CHANNEL,
			PeerID:   channelID,
			Sender:   sender,
			Class:    class,
		})
		if len(ups) == 0 || syncer == nil {
			return
		}
		_, _ = syncer.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{
			UserId:  userID,
			Updates: mtproto.MakeUpdatesByUpdates(ups...),
		})
	}
}
