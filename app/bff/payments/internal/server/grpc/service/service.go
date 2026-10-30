package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/payments/internal/svc"
)

// Service implements Stars balance, subscriptions, giveaways, and gift codes.
type Service struct {
	mtproto.UnimplementedRPCStarsServer
	mtproto.UnimplementedRPCStarSubscriptionsServer
	mtproto.UnimplementedRPCGiveawaysServer
	mtproto.UnimplementedRPCGiftCodesServer
	svcCtx *svc.ServiceContext
}

// New returns the payments RPC service. A nil context uses the shared book.
func New(sc *svc.ServiceContext) *Service {
	if sc == nil || sc.Book == nil {
		sc = svc.New()
	}
	return &Service{svcCtx: sc}
}

func (s *Service) user(ctx context.Context) (int64, error) {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil || md.GetUserId() == 0 {
		return 0, mtproto.ErrAuthKeyUnregistered
	}
	return md.GetUserId(), nil
}
