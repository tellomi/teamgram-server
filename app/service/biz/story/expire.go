package story

import (
	"context"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"

	"github.com/zeromicro/go-zero/core/logx"
)

// channelAdvert is the active-story max last shown on a channel object.
type channelAdvert struct {
	max   int32
	live  bool
	known bool
}

// WatchExpiries repairs active-story advertisements and keeps them in step
// with expire_date. The stories bff starts it once hooks are installed.
func (s *Service) WatchExpiries() {
	if s == nil {
		return
	}
	s.expireOnce.Do(func() {
		go s.expireLoop()
	})
}

func (s *Service) expireLoop() {
	ctx := context.Background()
	logx.Info("story: watching active-story expiry")
	s.ReleaseExpired(ctx)
	for {
		timer := time.NewTimer(s.untilNextExpiry(ctx))
		<-timer.C
		s.ReleaseExpired(ctx)
	}
}

func (s *Service) untilNextExpiry(ctx context.Context) time.Duration {
	const maxWait = 30 * time.Second
	next, err := s.store.NextExpiry(ctx, s.now())
	if err != nil || next <= s.now() {
		return maxWait
	}
	wait := time.Duration(int64(next)-int64(s.now())) * time.Second
	if wait <= 0 || wait > maxWait {
		return maxWait
	}
	return wait
}

// ReleaseExpired clears user.stories_max_id and channel recentStory once a
// story is no longer active. Reads such as stories.getPeerStories already
// hide expired stories; this is the advertisement those reads do not update.
func (s *Service) ReleaseExpired(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	now := s.now()
	s.repairMySQLAdverts(ctx)
	s.reconcilePersisted(ctx)

	s.mu.Lock()
	since := s.foldedThrough
	// users.stories_max_id was just compared with the active set. The first
	// scan can skip those users; it still has to refresh channel objects,
	// which are not stored on a column.
	skipUserHistory := s.haveUserTable && s.persistedDone && since == 0
	s.mu.Unlock()

	owners, err := s.store.OwnersExpiredBetween(ctx, since, now)
	if err != nil {
		logx.WithContext(ctx).Errorf("story: list expired owners: %v", err)
		return
	}
	failed := false
	for _, owner := range owners {
		switch owner.Kind {
		case KindUser:
			if skipUserHistory {
				continue
			}
			if !s.syncUserMax(ctx, owner) {
				failed = true
			}
		case KindChannel:
			if !s.syncChannelAdvert(ctx, owner) {
				failed = true
			}
		}
	}
	// A failed read must not move the watermark, or that expiry is skipped
	// until another story expires.
	if failed {
		return
	}
	s.mu.Lock()
	if now > s.foldedThrough {
		s.foldedThrough = now
	}
	s.mu.Unlock()
}

// reconcilePersisted compares users.stories_max_id with the stories that are
// still active. Send and delete already update that column; expiry did not,
// so a restarted process has to repair it from the table.
func (s *Service) reconcilePersisted(ctx context.Context) {
	s.mu.Lock()
	done := s.persistedDone
	s.mu.Unlock()
	if done {
		return
	}
	rows, ok, err := s.readPersistedMaxes(ctx)
	if err != nil {
		s.mu.Lock()
		logged := s.persistLogged
		s.persistLogged = true
		s.mu.Unlock()
		if !logged {
			logx.WithContext(ctx).Errorf("story: read users.stories_max_id: %v", err)
		}
		return
	}
	s.mu.Lock()
	s.haveUserTable = ok
	s.ensureAdvertLocked()
	var seeded []int64
	for id, max := range rows {
		if s.userMaxKnown[id] {
			continue
		}
		s.userMaxKnown[id] = true
		s.userMax[id] = max
		seeded = append(seeded, id)
	}
	s.mu.Unlock()
	failed := false
	for _, id := range seeded {
		if s.syncUserMax(ctx, Owner{Kind: KindUser, ID: id}) {
			continue
		}
		failed = true
		s.mu.Lock()
		delete(s.userMaxKnown, id)
		delete(s.userMax, id)
		s.mu.Unlock()
	}
	if !failed {
		s.mu.Lock()
		s.persistedDone = true
		s.mu.Unlock()
	}
}

func (s *Service) readPersistedMaxes(ctx context.Context) (map[int64]int32, bool, error) {
	ms, ok := s.store.(*mysqlStore)
	if !ok || ms == nil || ms.db == nil {
		return nil, false, nil
	}
	rows, err := ms.db.QueryContext(ctx, `SELECT id, stories_max_id FROM users WHERE stories_max_id <> 0`)
	if err != nil {
		return nil, true, err
	}
	defer rows.Close()
	out := map[int64]int32{}
	for rows.Next() {
		var id int64
		var max int32
		if err = rows.Scan(&id, &max); err != nil {
			return nil, true, err
		}
		if id != 0 && max != 0 {
			out[id] = max
		}
	}
	return out, true, rows.Err()
}

func (s *Service) ensureAdvertLocked() {
	if s.userMax == nil {
		s.userMax = map[int64]int32{}
	}
	if s.userMaxKnown == nil {
		s.userMaxKnown = map[int64]bool{}
	}
	if s.userEpoch == nil {
		s.userEpoch = map[int64]uint64{}
	}
	if s.userAdvert == nil {
		s.userAdvert = map[int64]*sync.Mutex{}
	}
	if s.channelMax == nil {
		s.channelMax = map[int64]channelAdvert{}
	}
}

// lockUserAdvert serializes advertisement writes for one user. The mutex is
// not the service lock: a hook may call out to storage and push, and that
// must not stall every other user's send or expiry. Callers must not hold
// s.mu.
func (s *Service) lockUserAdvert(id int64) func() {
	s.mu.Lock()
	s.ensureAdvertLocked()
	lk := s.userAdvert[id]
	if lk == nil {
		lk = &sync.Mutex{}
		s.userAdvert[id] = lk
	}
	s.mu.Unlock()
	lk.Lock()
	return lk.Unlock
}

// syncUserMax writes the maximum id among active stories. Zero means there
// are none, so the user object omits recentStory. It reports false when the
// active set could not be read or the stored max was not updated.
func (s *Service) syncUserMax(ctx context.Context, owner Owner) bool {
	if owner.Kind != KindUser || !owner.ok() {
		return true
	}
	unlock := s.lockUserAdvert(owner.ID)
	defer unlock()
	return s.syncUserMaxInGate(ctx, owner)
}

// syncUserMaxInGate is syncUserMax for a caller that already holds this
// user's advertisement lock. Send and delete hold that lock across the story
// row change and this write, so an expiry hook cannot commit an older max
// after a newer publish. The epoch is checked again after the hook returns:
// a stale ticket does not update memory, and the active set is recomputed
// before another external write.
func (s *Service) syncUserMaxInGate(ctx context.Context, owner Owner) bool {
	if ms, ok := s.store.(*mysqlStore); ok && ms != nil && ms.db != nil {
		return s.syncUserMaxMySQL(ctx, ms, owner)
	}
	const maxPasses = 4
	for pass := 0; pass < maxPasses; pass++ {
		max, _, err := s.activeMax(ctx, owner)
		if err != nil {
			logx.WithContext(ctx).Errorf("story: active max %d: %v", owner.ID, err)
			return false
		}
		s.mu.Lock()
		s.ensureAdvertLocked()
		if s.userMaxKnown[owner.ID] && s.userMax[owner.ID] == max {
			s.mu.Unlock()
			return true
		}
		// The users table is already 0 for anyone absent from the seed read.
		// Writing 0 again only invalidates the user cache.
		if !s.userMaxKnown[owner.ID] && max == 0 && s.haveUserTable {
			s.userMaxKnown[owner.ID] = true
			s.userMax[owner.ID] = 0
			s.mu.Unlock()
			return true
		}
		s.userEpoch[owner.ID]++
		ticket := s.userEpoch[owner.ID]
		fn := s.hooks.SetUserMax
		s.mu.Unlock()
		if fn != nil {
			if err = fn(ctx, owner.ID, max); err != nil {
				logx.WithContext(ctx).Errorf("story: set max %d: %v", owner.ID, err)
				return false
			}
		}
		again, _, err := s.activeMax(ctx, owner)
		if err != nil {
			logx.WithContext(ctx).Errorf("story: active max %d: %v", owner.ID, err)
			return false
		}
		s.mu.Lock()
		s.ensureAdvertLocked()
		if s.userEpoch[owner.ID] != ticket {
			s.mu.Unlock()
			return false
		}
		if again != max {
			s.mu.Unlock()
			continue
		}
		s.userMaxKnown[owner.ID] = true
		s.userMax[owner.ID] = max
		s.mu.Unlock()
		return true
	}
	logx.WithContext(ctx).Errorf("story: max %d changed while it was being stored", owner.ID)
	return false
}

// syncUserMaxMySQL stores the live max in the same database transaction that
// the caller uses for a story change. The value comes from the story rows,
// not from a max this process computed earlier.
func (s *Service) syncUserMaxMySQL(ctx context.Context, ms *mysqlStore, owner Owner) bool {
	max, wrote, err := ms.commitUserAdvert(ctx, owner.ID, s.now(), nil)
	if err == errNoAdvertUser {
		return true
	}
	if err != nil {
		logx.WithContext(ctx).Errorf("story: advert %d: %v", owner.ID, err)
		return false
	}
	if !wrote {
		return false
	}
	s.rememberAdvert(owner.ID, max)
	s.pushAdvert(ctx, owner.ID, max)
	return true
}

func (s *Service) rememberAdvert(userID int64, max int32) {
	s.mu.Lock()
	s.ensureAdvertLocked()
	s.userMaxKnown[userID] = true
	s.userMax[userID] = max
	s.mu.Unlock()
}

func (s *Service) pushAdvert(ctx context.Context, userID int64, max int32) {
	s.mu.Lock()
	fn := s.hooks.SetUserMax
	s.mu.Unlock()
	if fn == nil {
		return
	}
	if err := fn(ctx, userID, max); err != nil {
		logx.WithContext(ctx).Errorf("story: push max %d: %v", userID, err)
	}
}

// repairMySQLAdverts fixes users whose stories_max_id does not match a live
// story, including a crash that left the column at 0. One pass is capped.
func (s *Service) repairMySQLAdverts(ctx context.Context) {
	ms, ok := s.store.(*mysqlStore)
	if !ok || ms == nil || ms.db == nil {
		return
	}
	ids, err := ms.advertMismatches(ctx, s.now(), 256)
	if err != nil {
		logx.WithContext(ctx).Errorf("story: advert mismatches: %v", err)
		return
	}
	for _, id := range ids {
		s.syncUserMax(ctx, Owner{Kind: KindUser, ID: id})
	}
}

func (s *Service) syncChannelAdvert(ctx context.Context, owner Owner) bool {
	if owner.Kind != KindChannel || !owner.ok() {
		return true
	}
	max, live, err := s.activeMax(ctx, owner)
	if err != nil {
		logx.WithContext(ctx).Errorf("story: channel active max %d: %v", owner.ID, err)
		return false
	}
	s.mu.Lock()
	s.ensureAdvertLocked()
	prev := s.channelMax[owner.ID]
	if prev.known && prev.max == max && prev.live == live {
		s.mu.Unlock()
		return true
	}
	s.channelMax[owner.ID] = channelAdvert{max: max, live: live, known: true}
	s.mu.Unlock()
	// A channel object is built from the active set, so a process restart
	// does not keep a stale max for the next fetch. Push only when clients
	// may still be holding the previous object: the max changed, or this
	// process woke up to find no active story.
	if !prev.known && max > 0 {
		return true
	}
	s.pushChannelRefresh(ctx, owner.ID)
	return true
}

func (s *Service) pushChannelRefresh(ctx context.Context, channelID int64) {
	ids, err := channel.Shared().MemberIDs(ctx, channelID)
	if err != nil || len(ids) == 0 {
		return
	}
	owner := Owner{Kind: KindChannel, ID: channelID}
	upd := mtproto.MakeTLUpdateChannel(&mtproto.Update{ChannelId: channelID}).To_Update()
	for _, uid := range ids {
		s.push(ctx, uid, nil, mtproto.MakeTLUpdates(&mtproto.Updates{
			Updates: []*mtproto.Update{upd},
			Users:   []*mtproto.User{},
			Chats:   s.chats(ctx, uid, owner),
			Date:    s.now(),
		}).To_Updates())
	}
}
