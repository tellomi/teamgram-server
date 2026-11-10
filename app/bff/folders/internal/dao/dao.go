package dao

import (
	"context"
	"os"
	"strings"

	"github.com/teamgram/teamgram-server/pkg/mysqltls"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/config"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	dialog_client "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"
	"github.com/teamgram/teamgram-server/pkg/archive"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"
)

type Dao struct {
	dialog_client.DialogClient
	user_client.UserClient
	chat_client.ChatClient
	sync_client.SyncClient
	NextPts func(ctx context.Context, userID int64) int32
	SavePts archive.SavePts
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
	if len(c.IdgenClient.Etcd.Hosts) > 0 || len(c.IdgenClient.Endpoints) > 0 {
		gen := idgen_client.NewIDGenClient2(rpcx.GetCachedRpcClient(c.IdgenClient))
		d.NextPts = gen.NextPtsId
	}
	dsn := strings.TrimSpace(os.Getenv("TEAMGRAM_MYSQL_DSN"))
	if dsn == "" {
		dsn = strings.TrimSpace(c.MysqlDSN)
	}
	if dsn != "" {
		d.SavePts = archive.SavePtsSQL(mysqltls.NewMySQL(&sqlx.Config{DSN: dsn}))
	}
	return d
}
