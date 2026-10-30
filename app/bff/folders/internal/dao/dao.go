package dao

import (
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/config"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"
)

type Dao struct {
	dialog_client.DialogClient
	user_client.UserClient
	chat_client.ChatClient
	sync_client.SyncClient
}

func New(c config.Config) *Dao {
	d := &Dao{
		DialogClient: dialog_client.NewDialogClient(rpcx.GetCachedRpcClient(c.DialogClient)),
		UserClient:   user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		SyncClient:   sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(c.SyncClient)),
	}
	// Basic-group cards in chatlist invite previews. Empty config must not panic
	// the standalone folders process; the in-process bff always sets this.
	if c.ChatClient.Etcd.Key != "" || len(c.ChatClient.Endpoints) > 0 {
		d.ChatClient = chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient))
	}
	return d
}
