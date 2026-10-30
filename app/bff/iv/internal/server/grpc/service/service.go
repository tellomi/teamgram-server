package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/iv/rich"
)

// Service implements RPCIv. Android's TL_iv.getRichMessage constructor is
// messages.getRichMessage and is served there. This method is the same load
// for callers that reach RPCIv directly.
type Service struct {
	mtproto.UnimplementedRPCIvServer
}

// New returns the iv service.
func New() *Service { return &Service{} }

func (s *Service) IvGetRichMessage(ctx context.Context, in *mtproto.TLIvGetRichMessage) (*mtproto.Messages_Messages, error) {
	var self int64
	if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
		self = md.GetUserId()
	}
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return rich.Load(ctx, self, in.GetPeer(), in.GetId())
}
