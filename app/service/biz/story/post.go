package story

import (
	"context"
	"database/sql"

	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

// SendRequest is one stories.sendStory or stories.startLive.
type SendRequest struct {
	Actor       int64
	AuthKey     int64
	Owner       Owner
	AccessHash  int64
	Media       *mtproto.MessageMedia
	Areas       []*mtproto.MediaArea
	Caption     string
	Entities    []*mtproto.MessageEntity
	Rules       []*mtproto.PrivacyRule
	RandomID    int64
	Period      int32
	Pinned      bool
	Noforwards  bool
	Albums      []int32
	FwdOwner    Owner
	FwdID       int32
	FwdModified bool
	HasFwd      bool
	Live        bool
	Music       *mtproto.Document
}

// EditRequest is stories.editStory. Nil media and nil rules keep the previous value.
type EditRequest struct {
	Actor      int64
	AuthKey    int64
	Owner      Owner
	AccessHash int64
	ID         int32
	Media      *mtproto.MessageMedia
	Areas      []*mtproto.MediaArea
	AreasSet   bool
	Caption    *string
	Entities   []*mtproto.MessageEntity
	Rules      []*mtproto.PrivacyRule
	Music      *mtproto.Document
	MusicSet   bool
}

func (s *Service) Remains(ctx context.Context, actor int64, owner Owner, accessHash int64) (int32, error) {
	if err := s.canPost(ctx, actor, owner, accessHash); err != nil {
		return 0, err
	}
	recs, err := s.store.ListByOwner(ctx, owner)
	if err != nil {
		return 0, err
	}
	now := s.now()
	var n int32
	for _, rec := range recs {
		if rec.Deleted || rec.Sender != actor {
			continue
		}
		if rec.Item != nil && rec.Item.Date > now-defaultPeriod {
			n++
		}
	}
	rem := s.storyQuota(ctx, actor) - n
	if rem < 0 {
		rem = 0
	}
	return rem, nil
}

func (s *Service) storyQuota(ctx context.Context, actor int64) int32 {
	return int32(premium.Limit(premium.FeatureStories, s.premiumMember(ctx, actor)))
}

func (s *Service) premiumMember(ctx context.Context, userID int64) bool {
	now := int64(s.now())
	ms, ok := s.store.(*mysqlStore)
	if !ok || ms == nil || ms.db == nil {
		return premium.IsMember(false, 0, now)
	}
	var stored int
	var expire int64
	err := ms.db.QueryRowContext(ctx, `SELECT premium, premium_expire_date FROM users WHERE id=? LIMIT 1`, userID).Scan(&stored, &expire)
	if err != nil {
		return premium.IsMember(false, 0, now)
	}
	return premium.IsMember(stored != 0, expire, now)
}

func (s *Service) Send(ctx context.Context, req SendRequest) (*mtproto.Updates, error) {
	if err := s.canPost(ctx, req.Actor, req.Owner, req.AccessHash); err != nil {
		return nil, err
	}
	period, err := normalizePeriod(req.Period)
	if err != nil {
		return nil, err
	}
	if !req.Live {
		if req.Media == nil || req.Media.PredicateName == "" || req.Media.PredicateName == mtproto.Predicate_messageMediaEmpty {
			return nil, mtproto.ErrMediaInvalid
		}
	}
	if req.RandomID != 0 {
		if old, err := s.store.FindByRandom(ctx, req.Owner, req.Actor, req.RandomID); err != nil {
			return nil, err
		} else if old != nil {
			return s.storyUpdates(ctx, req.Actor, old, true), nil
		}
	}
	rem, err := s.Remains(ctx, req.Actor, req.Owner, req.AccessHash)
	if err != nil {
		return nil, err
	}
	if rem == 0 {
		if req.Owner.Kind == KindChannel {
			return nil, errBoostsRequired
		}
		if !premium.Allows(premium.FeatureStories, s.premiumMember(ctx, req.Actor)) {
			return nil, mtproto.ErrPremiumAccountRequired
		}
		return nil, errStoriesTooMuch
	}
	now := s.now()
	rules := req.Rules
	if len(rules) == 0 {
		rules = []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule()}
	}
	media := req.Media
	if media == nil {
		media = mtproto.MakeTLMessageMediaEmpty(nil).To_MessageMedia()
	}
	item := &mtproto.StoryItem{
		Date:       now,
		ExpireDate: now + period,
		Pinned:     req.Pinned,
		Noforwards: req.Noforwards,
		Live:       req.Live,
		Media:      media,
		Privacy:    rules,
		Music:      req.Music,
	}
	if req.Caption != "" {
		item.Caption = &wrapperspb.StringValue{Value: req.Caption}
	}
	if len(req.Entities) > 0 {
		item.Entities = req.Entities
	}
	item.SanitizeEntities()
	if len(req.Areas) > 0 {
		item.MediaAreas = req.Areas
	}
	if req.Owner.Kind == KindChannel {
		item.FromId = mtproto.MakePeerUser(req.Actor)
	}
	if req.HasFwd && req.FwdOwner.ok() && req.FwdID != 0 {
		item.FwdFrom = mtproto.MakeTLStoryFwdHeader(&mtproto.StoryFwdHeader{
			Modified: req.FwdModified,
			From:     req.FwdOwner.Peer(),
			StoryId:  &wrapperspb.Int32Value{Value: req.FwdID},
		}).To_StoryFwdHeader()
	}
	applyPrivacyFlags(item)
	item = mtproto.MakeTLStoryItem(item).To_StoryItem()
	rec := &Record{Owner: req.Owner, Sender: req.Actor, RandomID: req.RandomID, Item: item}
	if ms, ok := s.store.(*mysqlStore); ok && ms != nil && req.Owner.Kind == KindUser {
		return s.persistUserMySQL(ctx, ms, req, rec)
	}
	// Hold this user's advertisement lock from the insert through the max
	// write. An expiry already inside SetUserMax stays outside this section,
	// so its older max cannot land after this one.
	unlock := func() {}
	if req.Owner.Kind == KindUser {
		unlock = s.lockUserAdvert(req.Owner.ID)
	}
	if err = s.store.Create(ctx, rec); err != nil {
		unlock()
		return nil, err
	}
	if err = s.attachAlbums(ctx, req.Actor, req.Owner, req.AccessHash, rec.id(), req.Albums); err != nil {
		unlock()
		return nil, err
	}
	if req.HasFwd && req.FwdOwner.ok() && req.FwdID != 0 {
		_ = s.store.AddForwards(ctx, req.FwdOwner, req.FwdID)
	}
	if req.Owner.Kind == KindUser {
		s.syncUserMaxInGate(ctx, req.Owner)
	} else {
		s.syncUserMax(ctx, req.Owner)
	}
	unlock()
	s.fanout(ctx, req.Actor, req.AuthKey, rec)
	return s.storyUpdates(ctx, req.Actor, rec, true), nil
}

func (s *Service) Edit(ctx context.Context, req EditRequest) (*mtproto.Updates, error) {
	if err := s.canEdit(ctx, req.Actor, req.Owner, req.AccessHash); err != nil {
		return nil, err
	}
	rec, err := s.store.Get(ctx, req.Owner, req.ID)
	if err != nil {
		return nil, err
	}
	if rec == nil || rec.Deleted {
		return nil, errStoryIDInvalid
	}
	if req.Media != nil && req.Media.PredicateName != mtproto.Predicate_messageMediaEmpty {
		rec.Item.Media = req.Media
		rec.Item.Edited = true
	}
	if req.AreasSet {
		rec.Item.MediaAreas = req.Areas
		rec.Item.Edited = true
	}
	if req.Caption != nil {
		if *req.Caption == "" {
			rec.Item.Caption = nil
		} else {
			rec.Item.Caption = &wrapperspb.StringValue{Value: *req.Caption}
		}
		rec.Item.Entities = req.Entities
		rec.Item.SanitizeEntities()
		rec.Item.Edited = true
	}
	if req.Rules != nil {
		rec.Item.Privacy = req.Rules
		applyPrivacyFlags(rec.Item)
		rec.Item.Edited = true
	}
	if req.MusicSet {
		rec.Item.Music = req.Music
		rec.Item.Edited = true
	}
	rec.Item = mtproto.MakeTLStoryItem(rec.Item).To_StoryItem()
	if err = s.store.Save(ctx, rec); err != nil {
		return nil, err
	}
	s.fanout(ctx, req.Actor, req.AuthKey, rec)
	return s.storyUpdates(ctx, req.Actor, rec, false), nil
}

func (s *Service) persistUserMySQL(ctx context.Context, ms *mysqlStore, req SendRequest, rec *Record) (*mtproto.Updates, error) {
	unlock := s.lockUserAdvert(req.Owner.ID)
	if rec.Item.Id == 0 {
		id, err := ms.alloc(ctx, req.Owner, "next_story")
		if err != nil {
			unlock()
			return nil, err
		}
		rec.Item.Id = id
	}
	max, wrote, err := ms.commitUserAdvert(ctx, req.Owner.ID, s.now(), func(tx *sql.Tx) error {
		return upsertExec(ctx, tx, rec)
	})
	if err == errNoAdvertUser {
		unlock()
		if err = s.store.Create(ctx, rec); err != nil {
			return nil, err
		}
		s.syncUserMax(ctx, req.Owner)
		if err = s.attachAlbums(ctx, req.Actor, req.Owner, req.AccessHash, rec.id(), req.Albums); err != nil {
			return nil, err
		}
		if req.HasFwd && req.FwdOwner.ok() && req.FwdID != 0 {
			_ = s.store.AddForwards(ctx, req.FwdOwner, req.FwdID)
		}
		s.fanout(ctx, req.Actor, req.AuthKey, rec)
		return s.storyUpdates(ctx, req.Actor, rec, true), nil
	}
	if err != nil {
		unlock()
		return nil, err
	}
	if wrote {
		s.rememberAdvert(req.Owner.ID, max)
	}
	unlock()
	if wrote {
		s.pushAdvert(ctx, req.Owner.ID, max)
	}
	if err = s.attachAlbums(ctx, req.Actor, req.Owner, req.AccessHash, rec.id(), req.Albums); err != nil {
		return nil, err
	}
	if req.HasFwd && req.FwdOwner.ok() && req.FwdID != 0 {
		_ = s.store.AddForwards(ctx, req.FwdOwner, req.FwdID)
	}
	s.fanout(ctx, req.Actor, req.AuthKey, rec)
	return s.storyUpdates(ctx, req.Actor, rec, true), nil
}

func (s *Service) deleteUserMySQL(ctx context.Context, ms *mysqlStore, actor, auth int64, owner Owner, ids []int32) ([]int32, error) {
	unlock := s.lockUserAdvert(owner.ID)
	var removed []*Record
	max, wrote, err := ms.commitUserAdvert(ctx, owner.ID, s.now(), func(tx *sql.Tx) error {
		for _, id := range ids {
			rec, err := scanRecord(tx.QueryRowContext(ctx, `SELECT `+storyCols+` FROM stories WHERE owner_kind=? AND owner_id=? AND story_id=?`, owner.Kind, owner.ID, id))
			if err == sql.ErrNoRows {
				continue
			}
			if err != nil {
				return err
			}
			if rec == nil || rec.Deleted {
				continue
			}
			rec.Deleted = true
			if err = upsertExec(ctx, tx, rec); err != nil {
				return err
			}
			removed = append(removed, rec)
		}
		return nil
	})
	if err != nil && err != errNoAdvertUser {
		unlock()
		return nil, err
	}
	if wrote {
		s.rememberAdvert(owner.ID, max)
	}
	unlock()
	if wrote {
		s.pushAdvert(ctx, owner.ID, max)
	}
	deleted := make([]int32, 0, len(removed))
	for _, rec := range removed {
		deleted = append(deleted, rec.id())
		s.rememberDeletion(ctx, rec)
		s.fanout(ctx, actor, auth, rec)
	}
	return deleted, nil
}

func (s *Service) Delete(ctx context.Context, actor, auth int64, owner Owner, accessHash int64, ids []int32) ([]int32, error) {
	if len(ids) == 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	if err := s.canDelete(ctx, actor, owner, accessHash); err != nil {
		return nil, err
	}
	if ms, ok := s.store.(*mysqlStore); ok && ms != nil && owner.Kind == KindUser {
		return s.deleteUserMySQL(ctx, ms, actor, auth, owner, ids)
	}
	unlock := func() {}
	if owner.Kind == KindUser {
		unlock = s.lockUserAdvert(owner.ID)
	}
	var deleted []int32
	var removed []*Record
	for _, id := range ids {
		rec, err := s.store.Get(ctx, owner, id)
		if err != nil {
			unlock()
			return nil, err
		}
		if rec == nil || rec.Deleted {
			continue
		}
		rec.Deleted = true
		if err = s.store.Save(ctx, rec); err != nil {
			unlock()
			return nil, err
		}
		deleted = append(deleted, id)
		removed = append(removed, rec)
	}
	if owner.Kind == KindUser {
		s.syncUserMaxInGate(ctx, owner)
	} else {
		s.syncUserMax(ctx, owner)
	}
	unlock()
	for _, rec := range removed {
		s.rememberDeletion(ctx, rec)
		s.fanout(ctx, actor, auth, rec)
	}
	if deleted == nil {
		deleted = []int32{}
	}
	return deleted, nil
}

func (s *Service) TogglePinned(ctx context.Context, actor int64, owner Owner, accessHash int64, ids []int32, pinned bool) ([]int32, error) {
	if len(ids) == 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	if err := s.canEdit(ctx, actor, owner, accessHash); err != nil {
		return nil, err
	}
	var changed []int32
	for _, id := range ids {
		rec, err := s.store.Get(ctx, owner, id)
		if err != nil {
			return nil, err
		}
		if rec == nil || rec.Deleted {
			continue
		}
		rec.Item.Pinned = pinned
		if err = s.store.Save(ctx, rec); err != nil {
			return nil, err
		}
		changed = append(changed, id)
		s.fanout(ctx, actor, 0, rec)
	}
	if changed == nil {
		changed = []int32{}
	}
	return changed, nil
}

func (s *Service) TogglePinnedTop(ctx context.Context, actor int64, owner Owner, accessHash int64, ids []int32) error {
	if err := s.canEdit(ctx, actor, owner, accessHash); err != nil {
		return err
	}
	if len(ids) > maxPinnedTop {
		return errStoriesTooMuch
	}
	for _, id := range ids {
		rec, err := s.store.Get(ctx, owner, id)
		if err != nil {
			return err
		}
		if rec == nil || rec.Deleted || !rec.Item.GetPinned() {
			return errStoryIDInvalid
		}
	}
	return s.store.SetPinnedTop(ctx, owner, ids)
}

func (s *Service) attachAlbums(ctx context.Context, actor int64, owner Owner, accessHash int64, storyID int32, albumIDs []int32) error {
	if len(albumIDs) == 0 {
		return nil
	}
	if err := s.canEdit(ctx, actor, owner, accessHash); err != nil {
		return err
	}
	for _, id := range albumIDs {
		album, err := s.store.GetAlbum(ctx, owner, id)
		if err != nil || album == nil {
			continue
		}
		if contains32(album.StoryIDs, storyID) {
			continue
		}
		album.StoryIDs = append(album.StoryIDs, storyID)
		if err = s.store.SaveAlbum(ctx, album); err != nil {
			return err
		}
	}
	return nil
}
