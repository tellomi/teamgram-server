package dao

import (
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/premium/internal/config"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"

	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	user_client.UserClient
	sync_client.SyncClient
}

func New(c config.Config) *Dao {
	d := &Dao{}
	if rpcConfigured(c.UserClient) {
		d.UserClient = user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient))
	}
	if c.SyncClient != nil && len(c.SyncClient.Brokers) > 0 {
		d.SyncClient = sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(c.SyncClient))
	}
	return d
}

func rpcConfigured(c zrpc.RpcClientConf) bool {
	return len(c.Endpoints) > 0 || len(c.Etcd.Hosts) > 0 || c.Target != ""
}
