package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
)

// Older constructors share RPCStories with the layer-232 methods. Once the
// service is routed they must return a schema-valid result instead of an
// unimplemented gRPC status.

func (s *Service) StoriesCanSendStoryC7DFDFDD(ctx context.Context, in *mtproto.TLStoriesCanSendStoryC7DFDFDD) (*mtproto.Bool, error) {
	return s.canSendBool(ctx, in.GetPeer())
}

func (s *Service) StoriesCanSendStoryB100D45D(ctx context.Context, in *mtproto.TLStoriesCanSendStoryB100D45D) (*mtproto.Bool, error) {
	_ = in
	return s.canSendBool(ctx, mtproto.MakeTLInputPeerSelf(nil).To_InputPeer())
}

func (s *Service) canSendBool(ctx context.Context, peer *mtproto.InputPeer) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), peer)
	if err != nil {
		return nil, err
	}
	n, err := story.Shared().Remains(ctx, md.GetUserId(), owner, hash)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

func (s *Service) StoriesGetPeerMaxIDs535983C3(ctx context.Context, in *mtproto.TLStoriesGetPeerMaxIDs535983C3) (*mtproto.Vector_Int, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owners := make([]story.Owner, 0, len(in.GetId()))
	for _, peer := range in.GetId() {
		owner, _, err := resolvePeer(md.GetUserId(), peer)
		if err != nil {
			owners = append(owners, story.Owner{})
			continue
		}
		owners = append(owners, owner)
	}
	return &mtproto.Vector_Int{Datas: maxIDs(ctx, md.GetUserId(), owners)}, nil
}

func (s *Service) UsersGetStoriesMaxIDs(ctx context.Context, in *mtproto.TLUsersGetStoriesMaxIDs) (*mtproto.Vector_Int, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owners := make([]story.Owner, 0, len(in.GetId()))
	for _, user := range in.GetId() {
		if user == nil {
			owners = append(owners, story.Owner{})
			continue
		}
		id := mtproto.ToUserIdByInput(md.GetUserId(), user)
		if id == 0 {
			owners = append(owners, story.Owner{})
			continue
		}
		owners = append(owners, story.Owner{Kind: story.KindUser, ID: id})
	}
	return &mtproto.Vector_Int{Datas: maxIDs(ctx, md.GetUserId(), owners)}, nil
}

func maxIDs(ctx context.Context, viewer int64, owners []story.Owner) []int32 {
	stories := story.Shared().PeerMaxIDs(ctx, viewer, owners)
	ids := make([]int32, len(stories))
	for i, one := range stories {
		if one.GetMaxId() != nil {
			ids[i] = one.GetMaxId().GetValue()
		}
	}
	return ids
}

func (s *Service) StoriesReport1923FA8C(ctx context.Context, in *mtproto.TLStoriesReport1923FA8C) (*mtproto.Bool, error) {
	return s.reportBool(ctx, in.GetPeer(), in.GetId(), in.GetReason(), in.GetMessage())
}

func (s *Service) StoriesReportC95BE06A(ctx context.Context, in *mtproto.TLStoriesReportC95BE06A) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	if in.GetUserId() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	id := mtproto.ToUserIdByInput(md.GetUserId(), in.GetUserId())
	if id == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
		UserId:     id,
		AccessHash: in.GetUserId().GetAccessHash(),
	}).To_InputPeer()
	return s.reportBool(ctx, peer, in.GetId(), in.GetReason(), in.GetMessage())
}

func (s *Service) reportBool(ctx context.Context, peer *mtproto.InputPeer, ids []int32, reason *mtproto.ReportReason, message string) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), peer)
	if err != nil {
		return nil, err
	}
	opt := []byte("other")
	if reason != nil && reason.GetPredicateName() != "" {
		opt = []byte(reason.GetPredicateName())
	}
	if _, err = story.Shared().Report(ctx, md.GetUserId(), owner, hash, ids, opt, message); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) ContactsToggleStoriesHidden(ctx context.Context, in *mtproto.TLContactsToggleStoriesHidden) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	if in.GetId() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	id := mtproto.ToUserIdByInput(md.GetUserId(), in.GetId())
	if id == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
		UserId:     id,
		AccessHash: in.GetId().GetAccessHash(),
	}).To_InputPeer()
	return s.StoriesTogglePeerStoriesHidden(ctx, &mtproto.TLStoriesTogglePeerStoriesHidden{
		Peer:   peer,
		Hidden: in.GetHidden(),
	})
}

func (s *Service) StoriesGetUserStories(ctx context.Context, in *mtproto.TLStoriesGetUserStories) (*mtproto.Stories_UserStories, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	if in.GetUserId() == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	id := mtproto.ToUserIdByInput(md.GetUserId(), in.GetUserId())
	if id == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{
		UserId:     id,
		AccessHash: in.GetUserId().GetAccessHash(),
	}).To_InputPeer()
	got, err := s.StoriesGetPeerStories(ctx, &mtproto.TLStoriesGetPeerStories{Peer: peer})
	if err != nil {
		return nil, err
	}
	ps := got.GetStories()
	box := mtproto.MakeTLUserStories(&mtproto.UserStories{
		UserId:    id,
		MaxReadId: ps.GetMaxReadId(),
		Stories:   ps.GetStories(),
	}).To_UserStories()
	if box.Stories == nil {
		box.Stories = []*mtproto.StoryItem{}
	}
	return mtproto.MakeTLStoriesUserStories(&mtproto.Stories_UserStories{
		Stories: box,
		Users:   got.GetUsers(),
	}).To_Stories_UserStories(), nil
}

func (s *Service) StoriesGetAllReadUserStories(ctx context.Context, in *mtproto.TLStoriesGetAllReadUserStories) (*mtproto.Updates, error) {
	_ = in
	return s.StoriesGetAllReadPeerStories(ctx, &mtproto.TLStoriesGetAllReadPeerStories{})
}
