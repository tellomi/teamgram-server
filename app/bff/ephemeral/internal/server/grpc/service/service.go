package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/ephemeral/internal/svc"
)

// Service implements RPCEphemeral.
type Service struct {
	mtproto.UnimplementedRPCEphemeralServer
	svcCtx *svc.ServiceContext
}

// New returns an ephemeral service. A nil context uses the shared box.
func New(sc *svc.ServiceContext) *Service {
	if sc == nil || sc.Box == nil {
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

func peerOf(self int64, in *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	p := mtproto.FromInputPeer2(self, in)
	if p.PeerType == mtproto.PEER_SELF {
		p.PeerType = mtproto.PEER_USER
		p.PeerId = self
	}
	if p.PeerId == 0 || (p.PeerType != mtproto.PEER_USER && p.PeerType != mtproto.PEER_CHAT && p.PeerType != mtproto.PEER_CHANNEL) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return p, nil
}

func userOf(self int64, in *mtproto.InputUser) (int64, error) {
	if in == nil {
		return 0, mtproto.ErrUserIdInvalid
	}
	p := mtproto.FromInputUser(self, in)
	switch p.PeerType {
	case mtproto.PEER_SELF:
		return self, nil
	case mtproto.PEER_USER:
		if p.PeerId == 0 {
			return 0, mtproto.ErrUserIdInvalid
		}
		return p.PeerId, nil
	default:
		return 0, mtproto.ErrUserIdInvalid
	}
}
