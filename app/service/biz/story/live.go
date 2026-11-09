package story

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// liveUnboundGrace is how long a live story may exist before a group call is
// bound. stories.startLive binds the call before it returns. A story older
// than this with no call is a stream that ended, was discarded, or survived
// a process restart. The phone engine uses the same window before it gives
// up on a host who never connects.
const liveUnboundGrace int32 = 45

// LiveStarter opens the group call for one stories.startLive. The phone
// engine implements it in the bff process. audience is who may see the ring.
type LiveStarter func(ctx context.Context, host, auth int64, rtmp bool, random int32, audience []int64) (callID, access int64, up *mtproto.Updates, err error)

type liveBind struct {
	owner  Owner
	story  int32
	access int64
}

// liveBook is the in-memory link from a group call to the story it advertises.
type liveBook struct {
	byCall  map[int64]liveBind
	byStory map[Owner]map[int32]int64
	starter LiveStarter
	// alive reports whether the group call is still up. Nil means the
	// in-memory bind is enough, which is what unit tests use.
	alive func(callID int64) bool

	repairOwner int64
	repairStory int32
	repairKind  Kind
}

// SetLiveStarter installs the phone hook. A nil starter still posts the
// story; repair ends it once liveUnboundGrace has passed.
func (s *Service) SetLiveStarter(fn LiveStarter) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lives.starter = fn
	s.mu.Unlock()
}

// SetCallAlive lets the phone engine say a bound call was discarded.
// Reads then stop advertising the stub even if the end callback was missed,
// and the repair loop deletes it.
func (s *Service) SetCallAlive(fn func(callID int64) bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.lives.alive = fn
	s.mu.Unlock()
}

func (s *Service) liveStarter() LiveStarter {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lives.starter
}

// BeginLive is stories.startLive. The story is live only while its call is
// up. Empty media (the normal startLive body) is expired when the call ends,
// so the chat-list ring goes away with the stream.
func (s *Service) BeginLive(ctx context.Context, req SendRequest, rtmp bool) (*mtproto.Updates, error) {
	req.Live = true
	up, err := s.Send(ctx, req)
	if err != nil {
		return nil, err
	}
	id := storyIDOf(up)
	if id == 0 {
		return up, nil
	}
	rec, err := s.store.Get(ctx, req.Owner, id)
	if err != nil {
		return nil, err
	}
	starter := s.liveStarter()
	if starter == nil || rec == nil {
		return up, nil
	}
	viewers := s.audience(ctx, req.Actor, rec)
	callID, access, callUp, err := starter(ctx, req.Actor, req.AuthKey, rtmp, int32(req.RandomID), viewers)
	if err != nil {
		_ = s.EndLive(ctx, req.Owner, id)
		return nil, err
	}
	s.bindLive(callID, access, req.Owner, id)
	// Send ran before the call existed, so that advertisement could not be
	// live. Push again now that the call is bound.
	if req.Owner.Kind == KindUser {
		s.syncUserMax(ctx, req.Owner)
	} else {
		s.syncChannelAdvert(ctx, req.Owner)
	}
	if callUp != nil && len(callUp.Updates) > 0 {
		up.Updates = append(up.Updates, callUp.Updates...)
	}
	return up, nil
}

func storyIDOf(up *mtproto.Updates) int32 {
	if up == nil {
		return 0
	}
	for _, u := range up.Updates {
		if u.GetPredicateName() == mtproto.Predicate_updateStoryID && u.GetId_INT32() != 0 {
			return u.GetId_INT32()
		}
		if item := u.GetStory(); item != nil && item.GetId() != 0 {
			return item.GetId()
		}
	}
	return 0
}

func (s *Service) bindLive(callID, access int64, owner Owner, story int32) {
	if s == nil || callID == 0 || story == 0 || !owner.ok() {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lives.byCall == nil {
		s.lives.byCall = map[int64]liveBind{}
	}
	if s.lives.byStory == nil {
		s.lives.byStory = map[Owner]map[int32]int64{}
	}
	if s.lives.byStory[owner] == nil {
		s.lives.byStory[owner] = map[int32]int64{}
	}
	s.lives.byCall[callID] = liveBind{owner: owner, story: story, access: access}
	s.lives.byStory[owner][story] = callID
}

func (s *Service) liveBound(owner Owner, story int32) bool {
	return s.boundCall(owner, story) != 0
}

func (s *Service) boundCall(owner Owner, story int32) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lives.byStory[owner][story]
}

// onAir is a live story whose group call still exists.
func (s *Service) onAir(rec *Record) bool {
	if rec == nil || rec.Item == nil || !rec.Item.GetLive() {
		return false
	}
	callID := s.boundCall(rec.Owner, rec.id())
	if callID == 0 {
		return false
	}
	s.mu.Lock()
	alive := s.lives.alive
	s.mu.Unlock()
	if alive == nil {
		return true
	}
	return alive(callID)
}

func (s *Service) unbindLive(owner Owner, story int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	callID := s.lives.byStory[owner][story]
	delete(s.lives.byStory[owner], story)
	if len(s.lives.byStory[owner]) == 0 {
		delete(s.lives.byStory, owner)
	}
	if callID != 0 {
		delete(s.lives.byCall, callID)
	}
}

// EndLiveByCall clears the story tied to a discarded or timed-out call.
func (s *Service) EndLiveByCall(ctx context.Context, callID int64) error {
	if s == nil || callID == 0 {
		return nil
	}
	s.mu.Lock()
	b := s.lives.byCall[callID]
	s.mu.Unlock()
	if b.story == 0 {
		return nil
	}
	return s.EndLive(ctx, b.owner, b.story)
}

// EndLive drops the live flag. A story that never received a recording
// expires immediately, so no chat-list ring remains. A saved recording stays
// up as an ordinary story. Viewers are pushed storyItemDeleted or the
// updated item, and the user advertisement is recomputed.
func (s *Service) EndLive(ctx context.Context, owner Owner, storyID int32) error {
	if s == nil || !owner.ok() || storyID == 0 {
		return nil
	}
	rec, err := s.store.Get(ctx, owner, storyID)
	if err != nil {
		return err
	}
	if rec == nil || rec.Deleted || rec.Item == nil {
		s.unbindLive(owner, storyID)
		return nil
	}
	if !rec.Item.Live && rec.Item.ExpireDate <= s.now() {
		s.unbindLive(owner, storyID)
		return nil
	}
	// Capture viewers before the row is deleted. RingPushAudience skips
	// deleted stories, and a public live is visible to private dialogs too.
	viewers := s.audience(ctx, rec.Sender, rec)
	if owner.Kind == KindUser {
		viewers = append(viewers, s.RingPushAudience(ctx, owner.ID)...)
	}
	rec.Item.Live = false
	if mediaEmpty(rec.Item.Media) {
		// An empty startLive body is not a story once the stream is gone.
		// Deleting it drops stories_max_id and every active-story read.
		rec.Item.ExpireDate = s.now()
		rec.Deleted = true
	}
	rec.Item = mtproto.MakeTLStoryItem(rec.Item).To_StoryItem()
	if err = s.store.Save(ctx, rec); err != nil {
		return err
	}
	s.unbindLive(owner, storyID)
	if owner.Kind == KindUser {
		s.syncUserMax(ctx, owner)
	} else {
		s.syncChannelAdvert(ctx, owner)
	}
	s.fanoutIDs(ctx, rec.Sender, 0, rec, viewers)
	return nil
}

func mediaEmpty(m *mtproto.MessageMedia) bool {
	return m == nil || m.PredicateName == "" || m.PredicateName == mtproto.Predicate_messageMediaEmpty
}

// repairUnboundLives ends live stories whose call is gone. One page per
// expiry tick. A story younger than liveUnboundGrace is left alone so
// startLive can bind the call.
func (s *Service) repairUnboundLives(ctx context.Context) {
	if s == nil || s.store == nil {
		return
	}
	now := s.now()
	s.mu.Lock()
	kind := s.lives.repairKind
	if kind == 0 {
		kind = KindUser
	}
	afterOwner, afterStory := s.lives.repairOwner, s.lives.repairStory
	s.mu.Unlock()

	recs, full, err := s.pageActive(ctx, kind, now, afterOwner, afterStory, 128)
	if err != nil {
		return
	}
	for _, rec := range recs {
		if rec == nil || rec.Item == nil || !rec.Item.GetLive() || !rec.active(now) {
			continue
		}
		if s.onAir(rec) {
			continue
		}
		// A bound call that the engine has already discarded, or a stub that
		// never got a call and is past the start window.
		if s.liveBound(rec.Owner, rec.id()) || now-rec.Item.Date >= liveUnboundGrace {
			_ = s.EndLive(ctx, rec.Owner, rec.id())
		}
	}
	s.mu.Lock()
	if full || len(recs) < 128 {
		if kind == KindUser {
			s.lives.repairKind = KindChannel
		} else {
			s.lives.repairKind = KindUser
		}
		s.lives.repairOwner = 0
		s.lives.repairStory = 0
	} else if len(recs) > 0 {
		last := recs[len(recs)-1]
		s.lives.repairKind = kind
		s.lives.repairOwner = last.Owner.ID
		s.lives.repairStory = last.id()
	}
	s.mu.Unlock()
}

// pageActive returns one page. full means the page is the rest of this kind
// (the memory store has no cursor).
func (s *Service) pageActive(ctx context.Context, kind Kind, now int32, afterOwner int64, afterStory int32, limit int) ([]*Record, bool, error) {
	if ms, ok := s.store.(*mysqlStore); ok && ms != nil {
		recs, err := ms.pageActive(ctx, kind, now, afterOwner, afterStory, limit)
		return recs, false, err
	}
	all, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, false, err
	}
	var out []*Record
	for _, rec := range all {
		if rec == nil || rec.Owner.Kind != kind || !rec.active(now) {
			continue
		}
		out = append(out, rec)
	}
	return out, true, nil
}
