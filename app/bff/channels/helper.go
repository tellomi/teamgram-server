package channels_helper

import (
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/svc"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config = config.Config

func New(c Config) *service.Service {
	return service.New(svc.NewServiceContext(c))
}

func NewForums() *service.ForumsService       { return &service.ForumsService{} }
func NewReactions() *service.ReactionsService { return &service.ReactionsService{} }
func NewFragment(userConf zrpc.RpcClientConf) *service.FragmentService {
	return &service.FragmentService{
		Users: user_client.NewUserClient(rpcx.GetCachedRpcClient(userConf)),
	}
}
func NewAntiSpam() *service.AntiSpamService { return &service.AntiSpamService{} }
func NewBoosts() *service.BoostsService     { return &service.BoostsService{} }
func NewRecommendations() *service.RecommendationsService {
	return &service.RecommendationsService{}
}
func NewTakeout() *service.TakeoutService { return &service.TakeoutService{} }
func NewTranslation(messageConf zrpc.RpcClientConf) *service.TranslationService {
	return &service.TranslationService{
		Messages: message_client.NewMessageClient(rpcx.GetCachedRpcClient(messageConf)),
	}
}
func NewPaidMessages() *service.PaidMessageService { return &service.PaidMessageService{} }
func NewReports() *service.ReportsService          { return &service.ReportsService{} }

// NewThreads serves discussion-thread RPCs and contacts.blockFromReplies.
// userConf and messageConf are the biz service. msgConf is messenger.msg.
func NewThreads(userConf, messageConf, msgConf zrpc.RpcClientConf, syncConf *kafka.KafkaProducerConf) *service.ThreadsService {
	var syncClient sync_client.SyncClient
	if syncConf != nil {
		syncClient = sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(syncConf))
	}
	return &service.ThreadsService{
		Users:    user_client.NewUserClient(rpcx.GetCachedRpcClient(userConf)),
		Messages: message_client.NewMessageClient(rpcx.GetCachedRpcClient(messageConf)),
		Outbox:   msg_client.NewMsgClient(rpcx.GetCachedRpcClient(msgConf)),
		Sync:     syncClient,
	}
}
func NewStats() *service.StatsService { return &service.StatsService{} }
