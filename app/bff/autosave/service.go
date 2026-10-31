package autosave

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
)

// Service implements RPCAutosave.
type Service struct {
	mtproto.UnimplementedRPCAutosaveServer
}

// New returns the autosave RPC service.
func New() *Service { return &Service{} }

func (s *Service) AccountGetAutoSaveSettings(ctx context.Context, _ *mtproto.TLAccountGetAutoSaveSettings) (*mtproto.Account_AutoSaveSettings, error) {
	return Get(userID(ctx))
}

func (s *Service) AccountSaveAutoSaveSettings(ctx context.Context, in *mtproto.TLAccountSaveAutoSaveSettings) (*mtproto.Bool, error) {
	return Save(userID(ctx), in)
}

func (s *Service) AccountDeleteAutoSaveExceptions(ctx context.Context, _ *mtproto.TLAccountDeleteAutoSaveExceptions) (*mtproto.Bool, error) {
	return DeleteExceptions(userID(ctx))
}

func userID(ctx context.Context) int64 {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return 0
	}
	return md.GetUserId()
}
