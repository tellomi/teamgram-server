package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/wallet/internal/svc"

	"google.golang.org/grpc/status"
)

// Service implements RPCWallet and RPCWalletCard.
type Service struct {
	mtproto.UnimplementedRPCWalletServer
	mtproto.UnimplementedRPCWalletCardServer
	svcCtx *svc.ServiceContext
}

// New returns a wallet service. A nil context gets an empty ledger.
func New(sc *svc.ServiceContext) *Service {
	if sc == nil || sc.Ledger == nil {
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

func rpcErr(code string) error {
	return status.Error(mtproto.ErrBadRequest, code)
}

func fail(code string) error {
	if code == "" {
		return nil
	}
	return rpcErr(code)
}
