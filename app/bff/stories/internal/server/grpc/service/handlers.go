package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
)

func (s *Service) StoriesCanSendStory30EB63F0(ctx context.Context, in *mtproto.TLStoriesCanSendStory30EB63F0) (*mtproto.Stories_CanSendStoryCount, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	n, err := story.Shared().Remains(ctx, md.GetUserId(), owner, hash)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLStoriesCanSendStoryCount(&mtproto.Stories_CanSendStoryCount{CountRemains: n}).To_Stories_CanSendStoryCount(), nil
}

func (s *Service) StoriesSendStory(ctx context.Context, in *mtproto.TLStoriesSendStory) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	media, err := s.mediaOf(ctx, md.GetPermAuthKeyId(), in.GetMedia())
	if err != nil {
		return nil, err
	}
	rules, err := privacyRules(md.GetUserId(), in.GetPrivacyRules())
	if err != nil {
		return nil, err
	}
	caption := ""
	if in.GetCaption() != nil {
		caption = in.GetCaption().GetValue()
	}
	period := int32(0)
	if in.GetPeriod() != nil {
		period = in.GetPeriod().GetValue()
	}
	req := story.SendRequest{
		Actor: md.GetUserId(), AuthKey: md.GetPermAuthKeyId(), Owner: owner, AccessHash: hash,
		Media: media, Areas: in.GetMediaAreas(), Caption: caption, Entities: in.GetEntities(),
		Rules: rules, RandomID: in.GetRandomId(), Period: period, Pinned: in.GetPinned(),
		Noforwards: in.GetNoforwards(), Albums: in.GetAlbums(), Music: s.musicOf(ctx, in.GetMusic()),
	}
	if in.GetFwdFromId() != nil && in.GetFwdFromStory() != nil {
		fwd, _, ferr := resolvePeer(md.GetUserId(), in.GetFwdFromId())
		if ferr != nil {
			return nil, ferr
		}
		req.HasFwd = true
		req.FwdOwner = fwd
		req.FwdID = in.GetFwdFromStory().GetValue()
		req.FwdModified = in.GetFwdModified()
	}
	up, err := story.Shared().Send(ctx, req)
	if err != nil {
		return nil, err
	}
	return s.withUsers(ctx, md.GetUserId(), up), nil
}

func (s *Service) StoriesEditStory(ctx context.Context, in *mtproto.TLStoriesEditStory) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	req := story.EditRequest{
		Actor: md.GetUserId(), AuthKey: md.GetPermAuthKeyId(), Owner: owner, AccessHash: hash, ID: in.GetId(),
		Entities: in.GetEntities(),
	}
	if in.GetMedia() != nil {
		media, err := s.mediaOf(ctx, md.GetPermAuthKeyId(), in.GetMedia())
		if err != nil {
			return nil, err
		}
		req.Media = media
	}
	if areas := in.GetMediaAreas(); len(areas) > 0 {
		req.Areas = areas
		req.AreasSet = true
	}
	if in.GetCaption() != nil {
		v := in.GetCaption().GetValue()
		req.Caption = &v
	}
	if rules := in.GetPrivacyRules(); len(rules) > 0 {
		req.Rules, err = privacyRules(md.GetUserId(), rules)
		if err != nil {
			return nil, err
		}
	}
	if in.GetMusic() != nil {
		req.Music = s.musicOf(ctx, in.GetMusic())
		req.MusicSet = true
	}
	up, err := story.Shared().Edit(ctx, req)
	if err != nil {
		return nil, err
	}
	return s.withUsers(ctx, md.GetUserId(), up), nil
}

func (s *Service) StoriesDeleteStories(ctx context.Context, in *mtproto.TLStoriesDeleteStories) (*mtproto.Vector_Int, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	ids, err := story.Shared().Delete(ctx, md.GetUserId(), md.GetPermAuthKeyId(), owner, hash, in.GetId())
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_Int{Datas: ids}, nil
}

func (s *Service) StoriesTogglePinned(ctx context.Context, in *mtproto.TLStoriesTogglePinned) (*mtproto.Vector_Int, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	ids, err := story.Shared().TogglePinned(ctx, md.GetUserId(), owner, hash, in.GetId(), mtproto.FromBool(in.GetPinned()))
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_Int{Datas: ids}, nil
}

func (s *Service) StoriesGetAllStories(ctx context.Context, in *mtproto.TLStoriesGetAllStories) (*mtproto.Stories_AllStories, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	state := ""
	if in.GetState() != nil {
		state = in.GetState().GetValue()
	}
	res, err := story.Shared().GetAll(ctx, md.GetUserId(), in.GetHidden(), in.GetNext(), state)
	if err != nil {
		return nil, err
	}
	if res.Users != nil {
		res.Users = s.users(ctx, md.GetUserId(), append(idsOfPeers(res.PeerStories), md.GetUserId())...)
	}
	return res, nil
}

func (s *Service) StoriesGetPinnedStories(ctx context.Context, in *mtproto.TLStoriesGetPinnedStories) (*mtproto.Stories_Stories, error) {
	return s.storiesPage(ctx, in.GetPeer(), in.GetOffsetId(), in.GetLimit(), story.Shared().GetPinned)
}

func (s *Service) StoriesGetStoriesArchive(ctx context.Context, in *mtproto.TLStoriesGetStoriesArchive) (*mtproto.Stories_Stories, error) {
	return s.storiesPage(ctx, in.GetPeer(), in.GetOffsetId(), in.GetLimit(), story.Shared().GetArchive)
}

func (s *Service) storiesPage(ctx context.Context, peer *mtproto.InputPeer, offset, limit int32, fn func(context.Context, int64, story.Owner, int64, int32, int32) (*mtproto.Stories_Stories, error)) (*mtproto.Stories_Stories, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), peer)
	if err != nil {
		return nil, err
	}
	res, err := fn(ctx, md.GetUserId(), owner, hash, offset, limit)
	if err != nil {
		return nil, err
	}
	res.Users = s.users(ctx, md.GetUserId(), append(idsOfItems(res.Stories), md.GetUserId())...)
	return res, nil
}

func (s *Service) StoriesGetStoriesByID(ctx context.Context, in *mtproto.TLStoriesGetStoriesByID) (*mtproto.Stories_Stories, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	res, err := story.Shared().GetByID(ctx, md.GetUserId(), owner, hash, in.GetId())
	if err != nil {
		return nil, err
	}
	res.Users = s.users(ctx, md.GetUserId(), idsOfItems(res.Stories)...)
	return res, nil
}

func (s *Service) StoriesToggleAllStoriesHidden(ctx context.Context, in *mtproto.TLStoriesToggleAllStoriesHidden) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	if err = story.Shared().ToggleAllHidden(ctx, md.GetUserId(), mtproto.FromBool(in.GetHidden())); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) StoriesReadStories(ctx context.Context, in *mtproto.TLStoriesReadStories) (*mtproto.Vector_Int, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	ids, err := story.Shared().Read(ctx, md.GetUserId(), md.GetPermAuthKeyId(), owner, hash, in.GetMaxId())
	if err != nil {
		return nil, err
	}
	return &mtproto.Vector_Int{Datas: ids}, nil
}

func (s *Service) StoriesIncrementStoryViews(ctx context.Context, in *mtproto.TLStoriesIncrementStoryViews) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = story.Shared().IncrementViews(ctx, md.GetUserId(), owner, hash, in.GetId()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) StoriesGetStoryViewsList(ctx context.Context, in *mtproto.TLStoriesGetStoryViewsList) (*mtproto.Stories_StoryViewsList, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	q := ""
	if in.GetQ() != nil {
		q = in.GetQ().GetValue()
	}
	res, err := story.Shared().ViewsList(ctx, story.ViewsQuery{
		Actor: md.GetUserId(), Owner: owner, AccessHash: hash, ID: in.GetId(),
		Offset: in.GetOffset(), Limit: in.GetLimit(), Q: q,
		JustContacts: in.GetJustContacts(), ReactionsFirst: in.GetReactionsFirst(), ForwardsFirst: in.GetForwardsFirst(),
	})
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, v := range res.Views {
		ids = append(ids, v.GetUserId())
	}
	res.Users = s.users(ctx, md.GetUserId(), ids...)
	return res, nil
}

func (s *Service) StoriesGetStoriesViews(ctx context.Context, in *mtproto.TLStoriesGetStoriesViews) (*mtproto.Stories_StoryViews, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	res, err := story.Shared().GetViews(ctx, md.GetUserId(), owner, hash, in.GetId())
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, v := range res.Views {
		ids = append(ids, v.GetRecentViewers()...)
	}
	res.Users = s.users(ctx, md.GetUserId(), ids...)
	return res, nil
}

func (s *Service) StoriesExportStoryLink(ctx context.Context, in *mtproto.TLStoriesExportStoryLink) (*mtproto.ExportedStoryLink, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	link, err := story.Shared().ExportLink(ctx, md.GetUserId(), owner, hash, in.GetId())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLExportedStoryLink(&mtproto.ExportedStoryLink{Link: link}).To_ExportedStoryLink(), nil
}

func (s *Service) StoriesReport19D8EB45(ctx context.Context, in *mtproto.TLStoriesReport19D8EB45) (*mtproto.ReportResult, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	return story.Shared().Report(ctx, md.GetUserId(), owner, hash, in.GetId(), in.GetOption(), in.GetMessage())
}

func (s *Service) StoriesActivateStealthMode(ctx context.Context, in *mtproto.TLStoriesActivateStealthMode) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return story.Shared().Stealth(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetPast(), in.GetFuture())
}

func (s *Service) StoriesSendReaction(ctx context.Context, in *mtproto.TLStoriesSendReaction) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	up, err := story.Shared().React(ctx, md.GetUserId(), md.GetPermAuthKeyId(), owner, hash, in.GetStoryId(), in.GetReaction())
	if err != nil {
		return nil, err
	}
	return s.withUsers(ctx, md.GetUserId(), up), nil
}

func (s *Service) StoriesGetPeerStories(ctx context.Context, in *mtproto.TLStoriesGetPeerStories) (*mtproto.Stories_PeerStories, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	res, err := story.Shared().GetPeerStories(ctx, md.GetUserId(), owner, hash)
	if err != nil {
		return nil, err
	}
	res.Users = s.users(ctx, md.GetUserId(), idsOfPeers([]*mtproto.PeerStories{res.Stories})...)
	return res, nil
}

func (s *Service) StoriesGetAllReadPeerStories(ctx context.Context, in *mtproto.TLStoriesGetAllReadPeerStories) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	return story.Shared().AllRead(ctx, md.GetUserId())
}

func (s *Service) StoriesGetPeerMaxIDs78499170(ctx context.Context, in *mtproto.TLStoriesGetPeerMaxIDs78499170) (*mtproto.Vector_RecentStory, error) {
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
	return &mtproto.Vector_RecentStory{Datas: story.Shared().PeerMaxIDs(ctx, md.GetUserId(), owners)}, nil
}

func (s *Service) StoriesGetChatsToSend(ctx context.Context, in *mtproto.TLStoriesGetChatsToSend) (*mtproto.Messages_Chats, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	chats := story.Shared().ChatsToSend(ctx, md.GetUserId())
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats(), nil
}

func (s *Service) StoriesTogglePeerStoriesHidden(ctx context.Context, in *mtproto.TLStoriesTogglePeerStoriesHidden) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = story.Shared().TogglePeerHidden(ctx, md.GetUserId(), owner, hash, mtproto.FromBool(in.GetHidden())); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) StoriesGetStoryReactionsList(ctx context.Context, in *mtproto.TLStoriesGetStoryReactionsList) (*mtproto.Stories_StoryReactionsList, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	offset := ""
	if in.GetOffset() != nil {
		offset = in.GetOffset().GetValue()
	}
	reaction := ""
	if in.GetReaction() != nil {
		reaction = in.GetReaction().ToString()
	}
	res, err := story.Shared().ReactionsList(ctx, story.ViewsQuery{
		Actor: md.GetUserId(), Owner: owner, AccessHash: hash, ID: in.GetId(),
		Offset: offset, Limit: in.GetLimit(), Reaction: reaction, ForwardsFirst: in.GetForwardsFirst(),
	})
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, r := range res.Reactions {
		if r.GetPeerId().GetUserId() != 0 {
			ids = append(ids, r.GetPeerId().GetUserId())
		}
	}
	res.Users = s.users(ctx, md.GetUserId(), ids...)
	return res, nil
}

func (s *Service) StoriesTogglePinnedToTop(ctx context.Context, in *mtproto.TLStoriesTogglePinnedToTop) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = story.Shared().TogglePinnedTop(ctx, md.GetUserId(), owner, hash, in.GetId()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) StoriesSearchPosts(ctx context.Context, in *mtproto.TLStoriesSearchPosts) (*mtproto.Stories_FoundStories, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	tag := ""
	if in.GetHashtag() != nil {
		tag = in.GetHashtag().GetValue()
	}
	var peer *story.Owner
	if in.GetPeer() != nil {
		owner, _, err := resolvePeer(md.GetUserId(), in.GetPeer())
		if err != nil {
			return nil, err
		}
		peer = &owner
	}
	res, err := story.Shared().Search(ctx, md.GetUserId(), tag, in.GetOffset(), in.GetLimit(), peer)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for _, found := range res.Stories {
		if found.GetPeer().GetUserId() != 0 {
			ids = append(ids, found.GetPeer().GetUserId())
		}
		ids = append(ids, idsOfItems([]*mtproto.StoryItem{found.GetStory()})...)
	}
	res.Users = s.users(ctx, md.GetUserId(), ids...)
	return res, nil
}

func (s *Service) StoriesCreateAlbum(ctx context.Context, in *mtproto.TLStoriesCreateAlbum) (*mtproto.StoryAlbum, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	return story.Shared().CreateAlbum(ctx, md.GetUserId(), owner, hash, in.GetTitle(), in.GetStories())
}

func (s *Service) StoriesUpdateAlbum(ctx context.Context, in *mtproto.TLStoriesUpdateAlbum) (*mtproto.StoryAlbum, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	var title *string
	if in.GetTitle() != nil {
		v := in.GetTitle().GetValue()
		title = &v
	}
	return story.Shared().UpdateAlbum(ctx, md.GetUserId(), owner, hash, in.GetAlbumId(), title, in.GetDeleteStories(), in.GetAddStories(), in.GetOrder())
}

func (s *Service) StoriesReorderAlbums(ctx context.Context, in *mtproto.TLStoriesReorderAlbums) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = story.Shared().ReorderAlbums(ctx, md.GetUserId(), owner, hash, in.GetOrder()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) StoriesDeleteAlbum(ctx context.Context, in *mtproto.TLStoriesDeleteAlbum) (*mtproto.Bool, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	if err = story.Shared().DeleteAlbum(ctx, md.GetUserId(), owner, hash, in.GetAlbumId()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) StoriesGetAlbums(ctx context.Context, in *mtproto.TLStoriesGetAlbums) (*mtproto.Stories_Albums, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	return story.Shared().GetAlbums(ctx, md.GetUserId(), owner, hash, in.GetHash())
}

func (s *Service) StoriesGetAlbumStories(ctx context.Context, in *mtproto.TLStoriesGetAlbumStories) (*mtproto.Stories_Stories, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	res, err := story.Shared().GetAlbumStories(ctx, md.GetUserId(), owner, hash, in.GetAlbumId(), in.GetOffset(), in.GetLimit())
	if err != nil {
		return nil, err
	}
	res.Users = s.users(ctx, md.GetUserId(), idsOfItems(res.Stories)...)
	return res, nil
}

func (s *Service) StoriesStartLive(ctx context.Context, in *mtproto.TLStoriesStartLive) (*mtproto.Updates, error) {
	md, err := s.md(ctx)
	if err != nil {
		return nil, err
	}
	owner, hash, err := resolvePeer(md.GetUserId(), in.GetPeer())
	if err != nil {
		return nil, err
	}
	rules, err := privacyRules(md.GetUserId(), in.GetPrivacyRules())
	if err != nil {
		return nil, err
	}
	caption := ""
	if in.GetCaption() != nil {
		caption = in.GetCaption().GetValue()
	}
	up, err := story.Shared().Send(ctx, story.SendRequest{
		Actor: md.GetUserId(), AuthKey: md.GetPermAuthKeyId(), Owner: owner, AccessHash: hash,
		Caption: caption, Entities: in.GetEntities(), Rules: rules, RandomID: in.GetRandomId(),
		Pinned: in.GetPinned(), Noforwards: in.GetNoforwards(), Live: true,
	})
	if err != nil {
		return nil, err
	}
	return s.withUsers(ctx, md.GetUserId(), up), nil
}

func (s *Service) withUsers(ctx context.Context, self int64, up *mtproto.Updates) *mtproto.Updates {
	if up == nil {
		return up
	}
	var ids []int64
	ids = append(ids, self)
	for _, one := range up.Updates {
		if one.GetStory() != nil {
			ids = append(ids, idsOfItems([]*mtproto.StoryItem{one.GetStory()})...)
		}
		if one.GetPeer_PEER().GetUserId() != 0 {
			ids = append(ids, one.GetPeer_PEER().GetUserId())
		}
	}
	if users := s.users(ctx, self, ids...); len(users) > 0 {
		up.Users = users
	}
	return up
}
