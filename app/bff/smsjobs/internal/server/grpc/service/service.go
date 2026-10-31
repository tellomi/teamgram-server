package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/smsjobs/internal/svc"
)

// Service implements RPCSmsjobs.
type Service struct {
	mtproto.UnimplementedRPCSmsjobsServer
	svcCtx *svc.ServiceContext
}

// New returns the smsjobs service. A nil context uses the shared ledger.
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
