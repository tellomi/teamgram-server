package login

import (
	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/passkey/persist"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	authsession_client "github.com/teamgram/teamgram-server/app/service/authsession/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"
	"github.com/teamgram/teamgram-server/pkg/geoip"
	"github.com/zeromicro/go-zero/zrpc"
)

// Clients are the bff endpoints Authorize needs.
type Clients struct {
	DSN     string
	User    zrpc.RpcClientConf
	Session zrpc.RpcClientConf
	Msg     zrpc.RpcClientConf
	Sync    *kafka.KafkaProducerConf
}

// Configure opens the passkey tables and the login clients.
func Configure(c Clients) error {
	if err := persist.Configure(c.DSN); err != nil {
		return err
	}
	SetDeps(Deps{
		Users:    user_client.NewUserClient(rpcx.GetCachedRpcClient(c.User)),
		Sessions: authsession_client.NewAuthsessionClient(rpcx.GetCachedRpcClient(c.Session)),
		Updates:  sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(c.Sync)),
		Msg:      msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.Msg)),
		Geo:      geoip.Load(""),
	})
	return nil
}
