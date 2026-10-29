package svc

import (
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/relay"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/sfu"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	"github.com/teamgram/teamgram-server/app/service/biz/phonecall"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/zrpc"
)

type Clients struct {
	User zrpc.RpcClientConf
	Chat zrpc.RpcClientConf
	Msg  zrpc.RpcClientConf
	Sync *kafka.KafkaProducerConf
}

type ServiceContext struct {
	Config config.Config
	Users  user_client.UserClient
	Chat   chat_client.ChatClient
	Msg    msgclient.MsgClient
	Sync   sync_client.SyncClient
	Relay  *relay.Reflector
	SFU    *sfu.Server
}

func New(c config.Config, clients Clients) *ServiceContext {
	c = c.Normalized()
	sc := &ServiceContext{Config: c}
	if !c.DisableReflect {
		r, err := relay.Listen(c.ReflectorPort)
		if err != nil {
			logx.Errorf("phone reflector: %v", err)
		} else {
			sc.Relay = r
			logx.Infof("phone reflector udp :%d", r.Addr().Port)
		}
	}
	if !c.DisableSFU {
		s, err := sfu.Listen(c.PublicIP, c.SFUPort)
		if err != nil {
			logx.Errorf("phone sfu: %v", err)
		} else {
			sc.SFU = s
			logx.Infof("phone sfu udp :%d", s.Port())
		}
	}
	if err := phonecall.UseDSN(c.MySQL); err != nil {
		logx.Errorf("phone mysql: %v", err)
	}
	if len(clients.User.Etcd.Hosts) > 0 || len(clients.User.Endpoints) > 0 {
		sc.Users = user_client.NewUserClient(rpcx.GetCachedRpcClient(clients.User))
	}
	if len(clients.Chat.Etcd.Hosts) > 0 || len(clients.Chat.Endpoints) > 0 {
		sc.Chat = chat_client.NewChatClient(rpcx.GetCachedRpcClient(clients.Chat))
	}
	if len(clients.Msg.Etcd.Hosts) > 0 || len(clients.Msg.Endpoints) > 0 {
		sc.Msg = msgclient.NewMsgClient(rpcx.GetCachedRpcClient(clients.Msg))
	}
	if clients.Sync != nil && len(clients.Sync.Brokers) > 0 {
		sc.Sync = sync_client.NewSyncMqClient(kafka.MustKafkaProducer(clients.Sync))
	}
	return sc
}
