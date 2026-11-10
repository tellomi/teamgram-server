package story

import (
	"context"
	"sort"
	"strconv"

	"github.com/teamgram/proto/mtproto"

	"github.com/zeromicro/go-zero/core/logx"
)

// deletionReplayWindow is how long a missed storyItemDeleted stays available
// to updates.getDifference. A client offline longer than this has also passed
// the story's own expire_date.
const deletionReplayWindow int32 = 7 * 24 * 3600

// deletionPage is the most storyItemDeleted updates one getDifference returns.
const deletionPage = 100

// rememberDeletion writes the catch-up rows for a story that just became
// deleted. The live push is not durable; this is what an offline client
// reads later. A failure is logged and does not roll back the delete.
func (s *Service) rememberDeletion(ctx context.Context, rec *Record) {
	if s == nil || s.store == nil || rec == nil || !rec.Deleted || rec.id() == 0 {
		return
	}
	viewers := s.catchupAudience(ctx, rec)
	if len(viewers) == 0 {
		return
	}
	if err := s.store.SaveDeletions(ctx, rec.Owner, rec.id(), s.now(), viewers); err != nil {
		logx.WithContext(ctx).Errorf("story: remember deletion owner=%d story=%d: %v", rec.Owner.ID, rec.id(), err)
	}
}

// catchupAudience is who could have seen this story: the owner, contacts who
// pass privacy, privacy allow-lists, close friends, and, for a public story,
// private-dialog peers. That is the RingPushAudience of this story, captured
// from the record itself so a row that is already deleted still counts.
func (s *Service) catchupAudience(ctx context.Context, rec *Record) []int64 {
	if s == nil || rec == nil || rec.Item == nil || !rec.Owner.ok() {
		return nil
	}
	var ids []int64
	if rec.Owner.Kind == KindUser {
		ids = append(ids, rec.Owner.ID)
	}
	ids = append(ids, s.audience(ctx, rec.Sender, rec)...)
	if rec.Owner.Kind == KindUser && storyCloseFriends(rec) {
		for _, id := range s.closeFriendIDs(ctx, rec.Owner.ID) {
			if s.privacyAllows(ctx, rec, id) {
				ids = append(ids, id)
			}
		}
	}
	if rec.Owner.Kind == KindUser && storyReachesDialogs(rec) {
		for _, id := range s.dialogPeers(ctx, rec.Owner.ID, ringDialogCap) {
			if s.privacyAllows(ctx, rec, id) {
				ids = append(ids, id)
			}
		}
	}
	ids = unique64(ids)
	if len(ids) > ringDialogCap {
		ids = ids[:ringDialogCap]
	}
	return ids
}

func storyReachesDialogs(rec *Record) bool {
	rules := rec.Item.GetPrivacy()
	if len(rules) == 0 {
		return true
	}
	for _, rule := range rules {
		if rule.GetPredicateName() == mtproto.Predicate_privacyValueAllowAll {
			return true
		}
	}
	return false
}

func storyCloseFriends(rec *Record) bool {
	for _, rule := range rec.Item.GetPrivacy() {
		if rule.GetPredicateName() == mtproto.Predicate_privacyValueAllowCloseFriends {
			return true
		}
	}
	return false
}

// DifferenceDeletions is the updateStory{storyItemDeleted} list a viewer
// still needs after updates.getDifference(date). Rows at or before since are
// omitted so the next difference, which carries a later date, does not
// repeat them.
func (s *Service) DifferenceDeletions(ctx context.Context, viewer int64, since int32) []*mtproto.Update {
	if s == nil || s.store == nil || viewer == 0 {
		return nil
	}
	rows, err := s.store.ListDeletions(ctx, viewer, since, deletionPage)
	if err != nil {
		logx.WithContext(ctx).Errorf("story: difference deletions %d: %v", viewer, err)
		return nil
	}
	out := make([]*mtproto.Update, 0, len(rows))
	for _, row := range rows {
		if !row.Owner.ok() || row.StoryID == 0 {
			continue
		}
		out = append(out, mtproto.MakeTLUpdateStory(&mtproto.Update{
			Peer_PEER: row.Owner.Peer(),
			Story:     s.deletedItem(row.StoryID),
		}).To_Update())
	}
	return out
}

// deletionHashParts mixes each owner's deletion watermark into the
// stories.getAllStories state. An empty tray otherwise hashes to one
// constant, so a client that missed both the post and the delete stays
// allStoriesNotModified and keeps the ring it cached from a push.
func (s *Service) deletionHashParts(ctx context.Context, viewer int64) ([]string, error) {
	if s == nil || s.store == nil || viewer == 0 {
		return nil, nil
	}
	marks, err := s.store.DeletionMarks(ctx, viewer)
	if err != nil {
		return nil, err
	}
	sort.Slice(marks, func(i, j int) bool { return deletionMarkLess(marks[i], marks[j]) })
	parts := make([]string, 0, len(marks))
	for _, mark := range marks {
		if !mark.Owner.ok() || mark.Count == 0 {
			continue
		}
		parts = append(parts, "d:"+strconv.Itoa(int(mark.Owner.Kind))+":"+strconv.FormatInt(mark.Owner.ID, 10)+":"+strconv.Itoa(int(mark.Count))+":"+strconv.Itoa(int(mark.MaxID))+":"+strconv.Itoa(int(mark.MaxAt)))
	}
	return parts, nil
}

func deletionMarkLess(a, b DeletionMark) bool {
	if a.Owner.Kind != b.Owner.Kind {
		return a.Owner.Kind < b.Owner.Kind
	}
	return a.Owner.ID < b.Owner.ID
}
