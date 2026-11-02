package dao

import (
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/premium/internal/config"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	dfs_client "github.com/teamgram/teamgram-server/app/service/dfs/client"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"

	"github.com/zeromicro/go-zero/zrpc"
)

type Dao struct {
	user_client.UserClient
	sync_client.SyncClient
	message_client.MessageClient
	dfs_client.DfsClient
}

func New(c config.Config) *Dao {
	d := &Dao{}
	if rpcConfigured(c.UserClient) {
		d.UserClient = user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient))
	}
	if rpcConfigured(c.MessageClient) {
		d.MessageClient = message_client.NewMessageClient(rpcx.GetCachedRpcClient(c.MessageClient))
	}
	if rpcConfigured(c.DfsClient) {
		d.DfsClient = dfs_client.NewDfsClient(rpcx.GetCachedRpcClient(c.DfsClient))
	}
	if c.SyncClient != nil && len(c.SyncClient.Brokers) > 0 {
		d.SyncClient = sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(c.SyncClient))
	}
	return d
}

func rpcConfigured(c zrpc.RpcClientConf) bool {
	return len(c.Endpoints) > 0 || len(c.Etcd.Hosts) > 0 || c.Target != ""
}
