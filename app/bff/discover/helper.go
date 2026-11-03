// Package discover_helper is the bff registration for discover.getEngineCredential.
package discover_helper

import (
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/discover/internal/server/grpc/service"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/zeromicro/go-zero/zrpc"
)

// New builds the discover RPC service. An empty user-service config still
// registers the method; issuing fails closed until the service is reachable
// and TELLOMI_DISCOVER=1 with both keys set.
func New(userConf zrpc.RpcClientConf) *service.Service {
	var users user_client.UserClient
	if len(userConf.Etcd.Hosts) > 0 || len(userConf.Endpoints) > 0 {
		users = user_client.NewUserClient(rpcx.GetCachedRpcClient(userConf))
	}
	return service.New(service.UserAccounts(users))
}
