package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/communities/internal/svc"
)

// Service implements RPCCommunities.
type Service struct {
	mtproto.UnimplementedRPCCommunitiesServer
	svcCtx *svc.ServiceContext
}

// New returns a communities service. A nil context uses the shared book.
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

func channelOf(in *mtproto.InputChannel) (int64, int64, error) {
	if in == nil || in.GetPredicateName() == mtproto.Predicate_inputChannelEmpty || in.GetChannelId() == 0 {
		return 0, 0, mtproto.ErrChannelInvalid
	}
	return in.GetChannelId(), in.GetAccessHash(), nil
}
