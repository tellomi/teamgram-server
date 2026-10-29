package svc

import (
	"context"
	"strings"

	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stories/internal/config"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	media_client "github.com/teamgram/teamgram-server/app/service/media/client"

	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config      config.Config
	UserClient  user_client.UserClient
	MediaClient media_client.MediaClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	if err := story.UseDSN(c.StoryMySQL); err != nil {
		logx.Errorf("story mysql: %v", err)
	}
	var users user_client.UserClient
	if len(c.UserClient.Etcd.Hosts) > 0 || len(c.UserClient.Endpoints) > 0 {
		users = user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient))
	}
	var mediaClient media_client.MediaClient
	if len(c.MediaClient.Etcd.Hosts) > 0 || len(c.MediaClient.Endpoints) > 0 {
		mediaClient = media_client.NewMediaClient(rpcx.GetCachedRpcClient(c.MediaClient))
	}
	installHooks(users, c)
	return &ServiceContext{Config: c, UserClient: users, MediaClient: mediaClient}
}

func installHooks(users user_client.UserClient, c config.Config) {
	var syncer sync_client.SyncClient
	if c.SyncClient != nil && len(c.SyncClient.Brokers) > 0 {
		syncer = sync_client.NewSyncMqClient(kafka.MustKafkaProducer(c.SyncClient))
	}
	story.Shared().SetHooks(story.Hooks{
		Push: func(ctx context.Context, userID int64, excludeAuth []int64, updates *mtproto.Updates) {
			if syncer == nil || updates == nil {
				return
			}
			if len(excludeAuth) > 0 {
				_, _ = syncer.SyncPushUpdatesIfNot(ctx, &sync.TLSyncPushUpdatesIfNot{
					UserId:   userID,
					Excludes: excludeAuth,
					Updates:  updates,
				})
				return
			}
			_, _ = syncer.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{
				UserId:  userID,
				Updates: updates,
			})
		},
		ContactIDs: func(ctx context.Context, userID int64) ([]int64, error) {
			if users == nil {
				return nil, nil
			}
			res, err := users.UserGetContactIdList(ctx, &user.TLUserGetContactIdList{UserId: userID})
			if err != nil || res == nil {
				return nil, err
			}
			return res.GetDatas(), nil
		},
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
		SetUserMax: func(ctx context.Context, userID int64, maxID int32) {
			if users == nil {
				return
			}
			_, _ = users.UserSetStoriesMaxId(ctx, &user.TLUserSetStoriesMaxId{UserId: userID, Id: maxID})
		},
		SetStoriesHidden: func(ctx context.Context, userID, peerID int64, hidden bool) {
			if users == nil {
				return
			}
			_, _ = users.UserSetStoriesHidden(ctx, &user.TLUserSetStoriesHidden{
				UserId: userID,
				Id:     peerID,
				Hidden: mtproto.ToBool(hidden),
			})
		},
		Username: func(ctx context.Context, userID int64) string {
			if users == nil {
				return ""
			}
			data, err := users.UserGetUserDataById(ctx, &user.TLUserGetUserDataById{UserId: userID})
			if err != nil || data == nil {
				return ""
			}
			return data.GetUsername()
		},
		MatchName: func(ctx context.Context, userID int64, q string) bool {
			if users == nil {
				return false
			}
			data, err := users.UserGetUserDataById(ctx, &user.TLUserGetUserDataById{UserId: userID})
			if err != nil || data == nil {
				return false
			}
			q = strings.ToLower(q)
			return strings.Contains(strings.ToLower(data.GetFirstName()), q) ||
				strings.Contains(strings.ToLower(data.GetLastName()), q) ||
				strings.Contains(strings.ToLower(data.GetUsername()), q)
		},
	})
}
