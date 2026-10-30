package dao

import (
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/config"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"
)

type Dao struct {
	dialog_client.DialogClient
	user_client.UserClient
	sync_client.SyncClient
}

func New(c config.Config) *Dao {
	return &Dao{
		DialogClient: dialog_client.NewDialogClient(rpcx.GetCachedRpcClient(c.DialogClient)),
		UserClient:   user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		SyncClient:   sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(c.SyncClient)),
	}
}
