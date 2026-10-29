package channels_helper

import (
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/svc"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/reaction"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"

	"github.com/zeromicro/go-zero/core/logx"
)

type Config = config.Config

func New(c Config) *service.Service {
	return service.New(svc.NewServiceContext(c))
}

func NewForums() *service.ForumsService { return &service.ForumsService{} }

func NewReactions(c Config) *service.ReactionsService {
	if err := reaction.UseDSN(c.ChannelMySQL); err != nil {
		logx.Errorf("reaction mysql: %v", err)
	}
	deps := service.ReactionDeps{}
	if len(c.MessageClient.Etcd.Hosts) > 0 {
		deps.Messages = message_client.NewMessageClient(rpcx.GetCachedRpcClient(c.MessageClient))
	}
	if len(c.ChatClient.Etcd.Hosts) > 0 {
		deps.Chats = chat_client.NewChatClientHelper(rpcx.GetCachedRpcClient(c.ChatClient))
	}
	if len(c.UserClient.Etcd.Hosts) > 0 {
		deps.Users = user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient))
	}
	if len(c.IdgenClient.Etcd.Hosts) > 0 {
		gen := idgen_client.NewIDGenClient2(rpcx.GetCachedRpcClient(c.IdgenClient))
		deps.IDGen = &gen
	}
	if c.SyncClient != nil && len(c.SyncClient.Brokers) > 0 {
		deps.Sync = sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(c.SyncClient))
	}
	return service.NewReactions(deps)
}

func NewThreads() *service.ThreadsService { return &service.ThreadsService{} }
func NewStats() *service.StatsService     { return &service.StatsService{} }
