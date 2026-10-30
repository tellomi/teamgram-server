package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

func (s *Service) CommunitiesCreate(ctx context.Context, in *mtproto.TLCommunitiesCreate) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := peerOf(id, in.GetPeer())
	if err != nil {
		return nil, err
	}
	about := ""
	if in.GetAbout() != nil {
		about = in.GetAbout().GetValue()
	}
	return s.svcCtx.Book.Create(id, in.GetTitle(), about, in.GetHidden(), peer)
}

func (s *Service) CommunitiesGetJoinedCommunities(ctx context.Context, _ *mtproto.TLCommunitiesGetJoinedCommunities) (*mtproto.Messages_Chats, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.Joined(id), nil
}

func (s *Service) CommunitiesTogglePeerLink(ctx context.Context, in *mtproto.TLCommunitiesTogglePeerLink) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	community, hash, err := channelOf(in.GetCommunity())
	if err != nil {
		return nil, err
	}
	peer, err := peerOf(id, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Book.TogglePeerLink(id, community, hash, peer, in.GetVisible(), in.GetHidden(), in.GetDeleted()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) CommunitiesToggleCommunityCollapsedInDialogs(ctx context.Context, in *mtproto.TLCommunitiesToggleCommunityCollapsedInDialogs) (*mtproto.Updates, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	community, hash, err := channelOf(in.GetCommunity())
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.SetCollapsed(id, community, hash, in.GetCollapsed())
}

func (s *Service) CommunitiesGetPeerLinkRequests(ctx context.Context, in *mtproto.TLCommunitiesGetPeerLinkRequests) (*mtproto.Communities_PeerLinkRequests, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	community, hash, err := channelOf(in.GetCommunity())
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.Requests(id, community, hash, in.GetOffset(), in.GetLimit())
}

func (s *Service) CommunitiesTogglePeerLinkRequestApproval(ctx context.Context, in *mtproto.TLCommunitiesTogglePeerLinkRequestApproval) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	community, hash, err := channelOf(in.GetCommunity())
	if err != nil {
		return nil, err
	}
	peer, err := peerOf(id, in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Book.DecideRequest(id, community, hash, peer, in.GetReject()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) CommunitiesToggleAllPeerLinkRequestApproval(ctx context.Context, in *mtproto.TLCommunitiesToggleAllPeerLinkRequestApproval) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	community, hash, err := channelOf(in.GetCommunity())
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Book.DecideAll(id, community, hash, in.GetReject()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) CommunitiesToggleParticipantBanned(ctx context.Context, in *mtproto.TLCommunitiesToggleParticipantBanned) (*mtproto.Bool, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	community, hash, err := channelOf(in.GetCommunity())
	if err != nil {
		return nil, err
	}
	participant, err := peerOf(id, in.GetParticipant())
	if err != nil {
		return nil, err
	}
	if err := s.svcCtx.Book.SetBanned(id, community, hash, participant, in.GetUnban()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) CommunitiesGetParticipantJoinedChats(ctx context.Context, in *mtproto.TLCommunitiesGetParticipantJoinedChats) (*mtproto.Communities_ParticipantJoinedChats, error) {
	id, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	community, hash, err := channelOf(in.GetCommunity())
	if err != nil {
		return nil, err
	}
	participant, err := peerOf(id, in.GetParticipant())
	if err != nil {
		return nil, err
	}
	return s.svcCtx.Book.ParticipantChats(id, community, hash, participant)
}
