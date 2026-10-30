package story

import (
	"context"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// StoryStats is stats.getStoryStats. Only the user who owns the stories, or
// a channel admin, can read them.
func (s *Service) StoryStats(ctx context.Context, viewer int64, owner Owner, accessHash int64, id int32) (*mtproto.Stats_StoryStats, error) {
	rec, err := s.statsRecord(ctx, viewer, owner, accessHash, id)
	if err != nil {
		return nil, err
	}
	views, reactions := s.storySeries(ctx, owner, rec)
	when := int64(1)
	if rec.Item != nil && rec.Item.Date > 0 {
		when = int64(rec.Item.Date)
	}
	rx, ry := reactionAxes(reactions)
	return mtproto.MakeTLStatsStoryStats(&mtproto.Stats_StoryStats{
		ViewsGraph:              channel.MintGraph("Views", []int64{when}, []float64{float64(views)}, false),
		ReactionsByEmotionGraph: channel.MintGraph("Reactions", rx, ry, true),
	}).To_Stats_StoryStats(), nil
}

// StoryPublicForwards is stats.getStoryPublicForwards. Forwarded copies are
// not stored. count is the forward counter on the story.
func (s *Service) StoryPublicForwards(ctx context.Context, viewer int64, owner Owner, accessHash int64, id int32, offset string, limit int32) (*mtproto.Stats_PublicForwards, error) {
	rec, err := s.statsRecord(ctx, viewer, owner, accessHash, id)
	if err != nil {
		return nil, err
	}
	if limit < 0 || limit > 100 {
		return nil, mtproto.ErrLimitInvalid
	}
	if offset != "" {
		n, err := strconv.Atoi(offset)
		if err != nil || n < 0 {
			return nil, mtproto.ErrOffsetInvalid
		}
	}
	return mtproto.MakeTLStatsPublicForwards(&mtproto.Stats_PublicForwards{
		Count:    rec.Forwards,
		Forwards: []*mtproto.PublicForward{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Stats_PublicForwards(), nil
}

func (s *Service) statsRecord(ctx context.Context, viewer int64, owner Owner, accessHash int64, id int32) (*Record, error) {
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return nil, err
	}
	if !s.isManager(ctx, viewer, owner) {
		if owner.Kind == KindUser {
			return nil, mtproto.ErrPeerIdInvalid
		}
		return nil, mtproto.ErrChatAdminRequired
	}
	if id <= 0 {
		return nil, mtproto.ErrStoryIdInvalid
	}
	recs, err := s.store.ListByOwner(ctx, owner)
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, mtproto.ErrStoriesNeverCreated
	}
	for _, rec := range recs {
		if rec.id() == id && !rec.Deleted {
			return rec, nil
		}
	}
	return nil, mtproto.ErrStoryIdInvalid
}

func (s *Service) storySeries(ctx context.Context, owner Owner, rec *Record) (int, map[string]int) {
	views, _ := s.store.ListViews(ctx, owner, rec.id())
	reactions := map[string]int{}
	n := 0
	for _, v := range views {
		if v.Stealth {
			continue
		}
		n++
		if v.Reaction != "" {
			reactions[v.Reaction]++
		}
	}
	return n, reactions
}

func reactionAxes(reactions map[string]int) ([]int64, []float64) {
	if len(reactions) == 0 {
		return []int64{1}, []float64{0}
	}
	xs := make([]int64, 0, len(reactions))
	ys := make([]float64, 0, len(reactions))
	i := int64(1)
	for _, n := range reactions {
		xs = append(xs, i)
		ys = append(ys, float64(n))
		i++
	}
	return xs, ys
}
