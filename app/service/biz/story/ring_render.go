package story

import (
	"context"

	"github.com/teamgram/proto/mtproto"

	"github.com/zeromicro/go-zero/core/logx"
)

// UserRing is the highest active story one viewer may see.
type UserRing struct {
	MaxID int32
	Live  bool
}

const ringDialogCap = 5000

// EnableUserRingPaint makes user objects in this process use the live
// per-viewer max instead of users.stories_max_id.
func EnableUserRingPaint() {
	mtproto.EnableStoryRings(func(ctx context.Context, viewer int64, ids []int64) (map[int64]mtproto.StoryRing, error) {
		rings, err := Shared().PeerMaxIDsBatch(ctx, viewer, ids)
		if err != nil {
			return nil, err
		}
		out := make(map[int64]mtproto.StoryRing, len(rings))
		for id, ring := range rings {
			out[id] = mtproto.StoryRing{MaxID: ring.MaxID, Live: ring.Live}
		}
		return out, nil
	})
}

func (s *Service) visibleMax(ctx context.Context, viewer int64, recs []*Record) UserRing {
	var ring UserRing
	for _, rec := range recs {
		if rec == nil || rec.Item == nil || !s.privacyAllows(ctx, rec, viewer) {
			continue
		}
		if rec.id() >= ring.MaxID {
			ring.MaxID = rec.id()
			ring.Live = rec.Item.GetLive()
		}
	}
	return ring
}

// ViewerRings is the live ring of one user for each viewer. column is
// users.stories_max_id; zero skips the read and returns no rings.
func (s *Service) ViewerRings(ctx context.Context, owner int64, column int32, viewers []int64) (map[int64]UserRing, error) {
	out := make(map[int64]UserRing, len(viewers))
	if s == nil || column <= 0 || len(viewers) == 0 {
		return out, nil
	}
	recs, err := s.store.ListActiveByOwners(ctx, KindUser, []int64{owner}, s.now())
	if err != nil {
		return nil, err
	}
	for _, viewer := range viewers {
		ring := s.visibleMax(ctx, viewer, recs)
		if ring.MaxID > 0 {
			out[viewer] = ring
		}
	}
	return out, nil
}

// RingPushAudience is who could have seen a ring besides reverse contacts:
// privacy allow-users, close friends, and, for a public story, private dialogs.
func (s *Service) RingPushAudience(ctx context.Context, owner int64) []int64 {
	if s == nil || owner == 0 {
		return nil
	}
	recs, err := s.store.ListByOwner(ctx, Owner{Kind: KindUser, ID: owner})
	if err != nil {
		logx.WithContext(ctx).Errorf("story: ring audience %d: %v", owner, err)
		return nil
	}
	var ids []int64
	public := false
	closeFriends := false
	for _, rec := range recs {
		if rec == nil || rec.Deleted || rec.Item == nil {
			continue
		}
		rules := rec.Item.GetPrivacy()
		if len(rules) == 0 {
			public = true
		}
		for _, rule := range rules {
			switch rule.GetPredicateName() {
			case mtproto.Predicate_privacyValueAllowAll:
				public = true
			case mtproto.Predicate_privacyValueAllowCloseFriends:
				closeFriends = true
			case mtproto.Predicate_privacyValueAllowUsers:
				ids = append(ids, rule.Users...)
			}
		}
	}
	if closeFriends {
		ids = append(ids, s.closeFriendIDs(ctx, owner)...)
	}
	if public {
		ids = append(ids, s.dialogPeers(ctx, owner, ringDialogCap)...)
	}
	return unique64(ids)
}

func (s *Service) closeFriendIDs(ctx context.Context, owner int64) []int64 {
	s.mu.Lock()
	var ids []int64
	for id := range s.closeFriends[owner] {
		ids = append(ids, id)
	}
	s.mu.Unlock()
	ms, ok := s.store.(*mysqlStore)
	if !ok || ms == nil || ms.db == nil {
		return ids
	}
	rows, err := ms.closeFriendIDs(ctx, owner)
	if err != nil {
		logx.WithContext(ctx).Errorf("story: close friends %d: %v", owner, err)
		return ids
	}
	return append(ids, rows...)
}

func (s *Service) dialogPeers(ctx context.Context, owner int64, limit int) []int64 {
	s.mu.Lock()
	hook := s.hooks.DialogPeers
	s.mu.Unlock()
	if hook != nil {
		ids, err := hook(ctx, owner, limit)
		if err != nil {
			logx.WithContext(ctx).Errorf("story: dialog audience %d: %v", owner, err)
			return nil
		}
		return ids
	}
	ms, ok := s.store.(*mysqlStore)
	if !ok || ms == nil || ms.db == nil {
		return nil
	}
	ids, err := ms.dialogPeers(ctx, owner, limit)
	if err != nil {
		logx.WithContext(ctx).Errorf("story: dialog audience %d: %v", owner, err)
		return nil
	}
	return ids
}
