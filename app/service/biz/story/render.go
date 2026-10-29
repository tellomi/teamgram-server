package story

import (
	"context"
	"sort"

	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

func (s *Service) albumIndex(ctx context.Context, owner Owner) map[int32][]int32 {
	albums, _ := s.store.ListAlbums(ctx, owner)
	idx := map[int32][]int32{}
	for _, a := range albums {
		for _, id := range a.StoryIDs {
			idx[id] = append(idx[id], a.ID)
		}
	}
	return idx
}

func (s *Service) present(ctx context.Context, viewer int64, rec *Record, albums map[int32][]int32) *mtproto.StoryItem {
	if rec == nil || rec.Deleted || rec.Item == nil {
		id := int32(0)
		if rec != nil {
			id = rec.id()
		}
		return s.deletedItem(id)
	}
	item := protoCloneItem(rec.Item)
	if item.PredicateName == "" {
		item = mtproto.MakeTLStoryItem(item).To_StoryItem()
	}
	if albums == nil {
		albums = s.albumIndex(ctx, rec.Owner)
	}
	if ids := albums[item.Id]; len(ids) > 0 {
		item.Albums = ids
	} else {
		item.Albums = nil
	}
	manager := s.isManager(ctx, viewer, rec.Owner)
	if !manager {
		item.Privacy = nil
	}
	item.Out = viewer == rec.Sender
	item.Views = s.viewSummary(ctx, viewer, rec, manager)
	if reaction := s.reactionOf(ctx, viewer, rec); reaction != nil {
		item.SentReaction = reaction
	}
	return item
}

func (s *Service) viewSummary(ctx context.Context, viewer int64, rec *Record, manager bool) *mtproto.StoryViews {
	views, _ := s.store.ListViews(ctx, rec.Owner, rec.id())
	var count, reactions int32
	counts := map[string]int32{}
	var recent []int64
	chosen := ""
	for _, v := range views {
		if v.UserID == viewer && v.Reaction != "" {
			chosen = v.Reaction
		}
		if v.Stealth {
			continue
		}
		count++
		if v.Reaction != "" {
			reactions++
			counts[v.Reaction]++
		}
		if manager && len(recent) < 3 {
			recent = append(recent, v.UserID)
		}
	}
	sv := mtproto.MakeTLStoryViews(&mtproto.StoryViews{
		HasViewers:    manager && count > 0,
		ViewsCount:    count,
		RecentViewers: recent,
	}).To_StoryViews()
	if rec.Forwards > 0 {
		sv.ForwardsCount = &wrapperspb.Int32Value{Value: rec.Forwards}
	}
	if reactions > 0 {
		sv.ReactionsCount_FLAGINT32 = &wrapperspb.Int32Value{Value: reactions}
		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sv.Reactions = append(sv.Reactions, mtproto.MakeTLReactionCount(&mtproto.ReactionCount{
				Reaction_REACTION: mtproto.FromReaction(k),
				Count:             counts[k],
				Chosen:            k == chosen && chosen != "",
			}).To_ReactionCount())
		}
	}
	return sv
}

func (s *Service) reactionOf(ctx context.Context, viewer int64, rec *Record) *mtproto.Reaction {
	views, _ := s.store.ListViews(ctx, rec.Owner, rec.id())
	for _, v := range views {
		if v.UserID == viewer && v.Reaction != "" {
			return mtproto.FromReaction(v.Reaction)
		}
	}
	return nil
}

func (s *Service) peerStories(ctx context.Context, viewer int64, owner Owner, recs []*Record) *mtproto.PeerStories {
	albums := s.albumIndex(ctx, owner)
	items := make([]*mtproto.StoryItem, 0, len(recs))
	for _, rec := range recs {
		items = append(items, s.present(ctx, viewer, rec, albums))
	}
	ps := mtproto.MakeTLPeerStories(&mtproto.PeerStories{
		Peer:    owner.Peer(),
		Stories: items,
	}).To_PeerStories()
	if rd, _ := s.store.GetRead(ctx, viewer, owner); rd != nil && rd.MaxID > 0 {
		ps.MaxReadId = &wrapperspb.Int32Value{Value: rd.MaxID}
	}
	return ps
}

func (s *Service) storiesBox(ctx context.Context, viewer int64, owner Owner, recs []*Record) *mtproto.Stories_Stories {
	albums := s.albumIndex(ctx, owner)
	items := make([]*mtproto.StoryItem, 0, len(recs))
	for _, rec := range recs {
		items = append(items, s.present(ctx, viewer, rec, albums))
	}
	top, _ := s.store.PinnedTop(ctx, owner)
	box := mtproto.MakeTLStoriesStories(&mtproto.Stories_Stories{
		Count:       int32(len(items)),
		Stories:     items,
		PinnedToTop: top,
		Chats:       s.chats(ctx, viewer, owner),
		Users:       []*mtproto.User{},
	}).To_Stories_Stories()
	if len(top) == 0 {
		box.PinnedToTop = nil
	}
	return box
}

func (s *Service) chats(ctx context.Context, viewer int64, owners ...Owner) []*mtproto.Chat {
	var ids []int64
	for _, o := range owners {
		if o.Kind == KindChannel {
			ids = append(ids, o.ID)
		}
	}
	if len(ids) == 0 {
		return []*mtproto.Chat{}
	}
	return channel.Shared().ChatsFor(ctx, viewer, ids...)
}

func (s *Service) storyUpdates(ctx context.Context, viewer int64, rec *Record, withRandom bool) *mtproto.Updates {
	item := s.present(ctx, viewer, rec, nil)
	ups := []*mtproto.Update{}
	if withRandom && rec.RandomID != 0 {
		ups = append(ups, mtproto.MakeTLUpdateStoryID(&mtproto.Update{
			Id_INT32: rec.Item.Id,
			RandomId: rec.RandomID,
		}).To_Update())
	}
	ups = append(ups, mtproto.MakeTLUpdateStory(&mtproto.Update{
		Peer_PEER: rec.Owner.Peer(),
		Story:     item,
	}).To_Update())
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: ups,
		Users:   []*mtproto.User{},
		Chats:   s.chats(ctx, viewer, rec.Owner),
		Date:    s.now(),
	}).To_Updates()
}

func (s *Service) deletedItem(id int32) *mtproto.StoryItem {
	return mtproto.MakeTLStoryItemDeleted(&mtproto.StoryItem{Id: id}).To_StoryItem()
}

func (s *Service) stealthMode(ctx context.Context, userID int64) *mtproto.StoriesStealthMode {
	st, _ := s.store.GetSettings(ctx, userID)
	if st == nil {
		st = &Settings{}
	}
	sm := &mtproto.StoriesStealthMode{}
	now := s.now()
	if st.StealthActive > now {
		sm.ActiveUntilDate = &wrapperspb.Int32Value{Value: st.StealthActive}
	}
	if st.StealthCooldown > now {
		sm.CooldownUntilDate = &wrapperspb.Int32Value{Value: st.StealthCooldown}
	}
	return mtproto.MakeTLStoriesStealthMode(sm).To_StoriesStealthMode()
}

func (s *Service) hidden(ctx context.Context, viewer int64, owner Owner) bool {
	rd, _ := s.store.GetRead(ctx, viewer, owner)
	return rd != nil && rd.Hidden
}

// AttachChannel fills the story fields on a channel object.
func (s *Service) AttachChannel(ctx context.Context, viewer, channelID int64) channel.StoryAttach {
	owner := Owner{Kind: KindChannel, ID: channelID}
	if err := s.canViewPeer(ctx, viewer, owner, 0); err != nil {
		return channel.StoryAttach{}
	}
	recs, _ := s.store.ListByOwner(ctx, owner)
	var active []*Record
	var pinned bool
	for _, rec := range recs {
		if s.visible(ctx, viewer, rec, true, true) {
			pinned = true
		}
		if s.visible(ctx, viewer, rec, false, false) {
			active = append(active, rec)
		}
	}
	max, live := int32(0), false
	for _, rec := range active {
		if rec.id() >= max {
			max = rec.id()
			live = rec.Item.GetLive()
		}
	}
	var ps *mtproto.PeerStories
	if len(active) > 0 {
		ps = s.peerStories(ctx, viewer, owner, active)
	}
	return channel.StoryAttach{
		MaxID:   max,
		Live:    live,
		Hidden:  s.hidden(ctx, viewer, owner),
		Pinned:  pinned,
		Stories: ps,
	}
}

// ActivePeerStories is the userFull.stories block.
func (s *Service) ActivePeerStories(ctx context.Context, peerUser, viewer int64) *mtproto.PeerStories {
	owner := Owner{Kind: KindUser, ID: peerUser}
	recs, _ := s.store.ListByOwner(ctx, owner)
	var active []*Record
	for _, rec := range recs {
		if s.visible(ctx, viewer, rec, false, false) {
			active = append(active, rec)
		}
	}
	if len(active) == 0 {
		return nil
	}
	return s.peerStories(ctx, viewer, owner, active)
}

// PinnedAvailable reports userFull.stories_pinned_available.
func (s *Service) PinnedAvailable(ctx context.Context, peerUser, viewer int64) bool {
	owner := Owner{Kind: KindUser, ID: peerUser}
	recs, _ := s.store.ListByOwner(ctx, owner)
	for _, rec := range recs {
		if s.visible(ctx, viewer, rec, true, true) {
			return true
		}
	}
	return false
}
