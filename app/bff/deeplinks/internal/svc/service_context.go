package svc

import (
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/deeplinks/internal/config"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
)

type ServiceContext struct {
	Config config.Config
	Msg    msg_client.MsgClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	ctx := &ServiceContext{Config: c}
	if len(c.MsgClient.Etcd.Hosts) > 0 || len(c.MsgClient.Endpoints) > 0 || c.MsgClient.Target != "" {
		ctx.Msg = msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.MsgClient))
	}
	return ctx
}
