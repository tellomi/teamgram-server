package stickers_helper

import (
	"context"

	kafka "github.com/teamgram/marmota/pkg/mq"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/bff/stickers/internal/server/grpc/service"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"
)

type Config struct {
	MySQL      string                   `json:",optional"`
	SyncClient *kafka.KafkaProducerConf `json:",optional"`
}

func New(c Config) *service.Service {
	cat := catalog.Open(c.MySQL)
	client := newSync(c.SyncClient)
	if client != nil {
		cat.SetUserPush(syncUserPush{client})
	}
	return service.New(client)
}

func newSync(c *kafka.KafkaProducerConf) sync_client.SyncClient {
	if c == nil || len(c.Brokers) == 0 {
		return nil
	}
	return sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(c))
}

// syncUserPush sends updateStickerSets to one user. SyncBroadcastUpdates
// only reaches chat members, so a catalog change is one push per user.
type syncUserPush struct {
	client sync_client.SyncClient
}

func (p syncUserPush) PushUser(ctx context.Context, userID int64, updates *mtproto.Updates) error {
	if p.client == nil || userID == 0 || updates == nil {
		return nil
	}
	_, err := p.client.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{
		UserId:  userID,
		Updates: updates,
	})
	return err
}

func NewEmojiCategories() *service.EmojiCategories {
	return service.NewEmojiCategories()
}
