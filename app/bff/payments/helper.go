package payments_helper

import (
	"context"

	"github.com/teamgram/teamgram-server/app/bff/payments/internal/book"
	"github.com/teamgram/teamgram-server/app/bff/payments/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/payments/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/zeromicro/go-zero/core/logx"
)

// New builds the Stars, gift, giveaway, collectible, and payment-form RPC service.
func New() *service.Service {
	return service.New(svc.New())
}

// Start persists star gift collections on the service MySQL DSN and checks
// channel admin rights before a collection is created or edited. An empty
// DSN keeps titles in memory. A DSN that cannot be opened is logged; the
// book keeps serving from memory until the next start.
func Start(dsn string) {
	book.Shared().SetGiftAdmin(func(self, channelID int64) error {
		return channel.Shared().MayManageGiftCollection(context.Background(), self, channelID)
	})
	if err := book.Shared().OpenCollections(dsn); err != nil {
		logx.Errorf("star gift collections: mysql open failed; titles stay in memory: %v", err)
	}
}
