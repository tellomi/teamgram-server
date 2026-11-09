package story

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestPublicStoryRingForContactAndStranger(t *testing.T) {
	svc, ctx, now := newSvc(t)
	enableRings(t)
	up := send(t, svc, ctx, 1, photoMedia(), rules("all"), 1, "public", false)
	id := storyID(t, up)
	column := svc.liveColumn(ctx, 1)
	im := ringOwner(1, column)

	for _, viewer := range []int64{1, 2, 9} {
		rendered := paintRing(ctx, viewer, im, column)
		assertRing(t, rendered, id, false)
		assertAgrees(t, svc, ctx, viewer, 1, rendered)
	}

	*now += defaultPeriod + 1
	stale := column
	for _, viewer := range []int64{1, 2, 9} {
		rendered := paintRing(ctx, viewer, im, stale)
		assertRing(t, rendered, 0, false)
		assertAgrees(t, svc, ctx, viewer, 1, rendered)
	}
}

func TestCloseFriendStoryHidesOtherContacts(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	enableRings(t)
	svc.MarkContact(1, 2)
	svc.MarkCloseFriend(1, 3)
	id := storyID(t, send(t, svc, ctx, 1, photoMedia(), rules("close"), 1, "close", false))
	column := svc.liveColumn(ctx, 1)
	im := ringOwner(1, column)

	assertRing(t, paintRing(ctx, 3, im, column), id, false)
	assertAgrees(t, svc, ctx, 3, 1, paintRing(ctx, 3, im, column))
	for _, viewer := range []int64{2, 9} {
		rendered := paintRing(ctx, viewer, im, column)
		assertRing(t, rendered, 0, false)
		assertAgrees(t, svc, ctx, viewer, 1, rendered)
	}
	plain := im.ToUser(2)
	if plain.GetStoriesMaxId_FLAGRECENTSTORY() == nil || plain.GetStoriesUnavailable() {
		t.Fatalf("column render before paint %+v", plain)
	}
}

func TestLiveStoryRing(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	enableRings(t)
	up, err := svc.Send(ctx, SendRequest{
		Actor: 1, Owner: Owner{Kind: KindUser, ID: 1}, Media: photoMedia(),
		Rules: rules("all"), RandomID: 7, Live: true, Caption: "live",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := storyID(t, up)
	column := svc.liveColumn(ctx, 1)
	rendered := paintRing(ctx, 9, ringOwner(1, column), column)
	assertRing(t, rendered, id, true)
	assertAgrees(t, svc, ctx, 9, 1, rendered)
}

func TestZeroColumnSkipsLiveQuery(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	enableRings(t)
	send(t, svc, ctx, 1, photoMedia(), rules("all"), 1, "public", false)
	rendered := paintRing(ctx, 9, ringOwner(1, 0), 0)
	assertRing(t, rendered, 0, false)
	maxes := svc.PeerMaxIDs(ctx, 9, []Owner{{Kind: KindUser, ID: 1}})
	if maxes[0].GetMaxId().GetValue() == 0 {
		t.Fatal("getPeerMaxIDs dropped a live public story")
	}
}

func TestChannelWithoutStorySetsUnavailable(t *testing.T) {
	svc, ctx, now := newSvc(t)
	created, err := channel.Shared().Create(ctx, channel.CreateRequest{CreatorID: 7, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	got := channel.Shared().ChatsFor(ctx, 7, ch.Id)
	if len(got) != 1 || !got[0].GetStoriesUnavailable() || got[0].GetStoriesMaxId_FLAGRECENTSTORY() != nil {
		t.Fatalf("empty channel %+v", got)
	}
	owner := Owner{Kind: KindChannel, ID: ch.Id}
	if _, err = svc.Send(ctx, SendRequest{
		Actor: 7, Owner: owner, AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
		Media: photoMedia(), Rules: rules("all"), RandomID: 3,
	}); err != nil {
		t.Fatal(err)
	}
	active := channel.Shared().ChatsFor(ctx, 7, ch.Id)
	if len(active) != 1 || active[0].GetStoriesUnavailable() || active[0].GetStoriesMaxId_FLAGRECENTSTORY().GetMaxId().GetValue() == 0 {
		t.Fatalf("active channel %+v", active)
	}
	*now += defaultPeriod + 1
	expired := channel.Shared().ChatsFor(ctx, 7, ch.Id)
	if len(expired) != 1 || !expired[0].GetStoriesUnavailable() || expired[0].GetStoriesMaxId_FLAGRECENTSTORY() != nil {
		t.Fatalf("expired channel %+v", expired)
	}
}

func TestRingPushAudience(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	svc.MarkContact(1, 2)
	svc.MarkCloseFriend(1, 3)
	svc.SetHooks(Hooks{DialogPeers: func(context.Context, int64, int) ([]int64, error) {
		return []int64{8}, nil
	}})
	send(t, svc, ctx, 1, photoMedia(), rules("all"), 1, "public", false)
	send(t, svc, ctx, 1, photoMedia(), rules("close"), 2, "close", false)
	send(t, svc, ctx, 1, photoMedia(), allowUsers(4), 3, "picked", false)
	got := map[int64]bool{}
	for _, id := range svc.RingPushAudience(ctx, 1) {
		got[id] = true
	}
	for _, id := range []int64{3, 4, 8} {
		if !got[id] {
			t.Fatalf("audience missing %d in %v", id, got)
		}
	}
	if got[2] {
		t.Fatal("plain contact was added beside reverse contacts")
	}
}

func TestExpiredStoryUpdateReachesOnlyOwner(t *testing.T) {
	svc, ctx, now := newSvc(t)
	svc.MarkContact(1, 2)
	id := storyID(t, send(t, svc, ctx, 1, photoMedia(), rules("all"), 1, "today", false))
	var pushes []pushedStory
	svc.SetHooks(Hooks{Push: func(_ context.Context, userID int64, _ []int64, updates *mtproto.Updates) {
		pushes = append(pushes, pushedStory{userID, updates})
	}})
	if _, err := svc.TogglePinned(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0, []int32{id}, true); err != nil {
		t.Fatal(err)
	}
	if kind := storyPushKind(t, pushes, 2); kind != mtproto.Predicate_storyItem {
		t.Fatalf("active viewer update %s", kind)
	}
	pushes = nil
	*now += defaultPeriod + 1
	if _, err := svc.TogglePinned(ctx, 1, Owner{Kind: KindUser, ID: 1}, 0, []int32{id}, false); err != nil {
		t.Fatal(err)
	}
	if kind := storyPushKind(t, pushes, 1); kind != mtproto.Predicate_storyItem {
		t.Fatalf("owner archive update %s", kind)
	}
	if kind := storyPushKind(t, pushes, 2); kind != mtproto.Predicate_storyItemDeleted {
		t.Fatalf("viewer expired update %s", kind)
	}
	for _, push := range pushes {
		if push.userID != 1 && push.userID != 2 {
			t.Fatalf("unexpected viewer %d", push.userID)
		}
	}

	rec, err := svc.store.Get(ctx, Owner{Kind: KindUser, ID: 1}, id)
	if err != nil || rec == nil {
		t.Fatal(err)
	}
	pushes = nil
	got, err := svc.React(ctx, 2, 0, rec.Owner, 0, id, mtproto.FromReaction("❤"))
	if err != nil {
		t.Fatal(err)
	}
	if itemKind(got) != mtproto.Predicate_storyItemDeleted {
		t.Fatalf("viewer reaction %s", itemKind(got))
	}
	if kind := storyPushKind(t, pushes, 1); kind != mtproto.Predicate_storyItem {
		t.Fatalf("owner reaction %s", kind)
	}
}

func TestBotStoriesStayUnavailable(t *testing.T) {
	im := mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: mtproto.MakeTLUserData(&mtproto.UserData{
			Id:                 5,
			FirstName:          "Bot",
			StoriesMaxId:       4,
			StoriesUnavailable: false,
			Bot:                &mtproto.BotData{},
		}).To_UserData(),
	}).To_ImmutableUser()
	user := im.ToUser(9)
	if !user.GetBot() || !user.GetStoriesUnavailable() {
		t.Fatalf("bot %+v", user)
	}
	self := im.ToSelfUser()
	if !self.GetStoriesUnavailable() {
		t.Fatal("self bot lost stories_unavailable")
	}
}

type pushedStory struct {
	userID int64
	up     *mtproto.Updates
}

func enableRings(t *testing.T) {
	t.Helper()
	EnableUserRingPaint()
	t.Cleanup(func() { mtproto.EnableStoryRings(nil) })
}

func (s *Service) liveColumn(ctx context.Context, userID int64) int32 {
	max, _, err := s.activeMax(ctx, Owner{Kind: KindUser, ID: userID})
	if err != nil {
		return 0
	}
	return max
}

func ringOwner(id int64, column int32) *mtproto.ImmutableUser {
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: mtproto.MakeTLUserData(&mtproto.UserData{
			Id:                 id,
			FirstName:          "Owner",
			StoriesMaxId:       column,
			StoriesUnavailable: column == 0,
		}).To_UserData(),
		ReverseContacts: []*mtproto.ContactData{
			mtproto.MakeTLContactData(&mtproto.ContactData{
				UserId:        2,
				ContactUserId: id,
				MutualContact: true,
				FirstName:     wrapperspb.String("Contact"),
			}).To_ContactData(),
		},
	}).To_ImmutableUser()
}

func paintRing(ctx context.Context, viewer int64, im *mtproto.ImmutableUser, column int32) *mtproto.User {
	var user *mtproto.User
	if viewer == im.Id() {
		user = im.ToSelfUser()
	} else {
		user = im.ToUser(viewer)
	}
	mtproto.PaintUserStoryRings(ctx, viewer, []*mtproto.User{user}, map[int64]int32{im.Id(): column})
	return user
}

func assertRing(t *testing.T, user *mtproto.User, max int32, live bool) {
	t.Helper()
	recent := user.GetStoriesMaxId_FLAGRECENTSTORY()
	if max == 0 {
		if recent != nil || user.GetStoriesMaxId_FLAGINT32() != nil || !user.GetStoriesUnavailable() {
			t.Fatalf("ring still set %+v unavailable=%v", recent, user.GetStoriesUnavailable())
		}
		return
	}
	if user.GetStoriesUnavailable() || recent.GetMaxId().GetValue() != max || recent.GetLive() != live {
		t.Fatalf("ring max=%d live=%v unavailable=%v", recent.GetMaxId().GetValue(), recent.GetLive(), user.GetStoriesUnavailable())
	}
}

func assertAgrees(t *testing.T, svc *Service, ctx context.Context, viewer, owner int64, rendered *mtproto.User) {
	t.Helper()
	maxes := svc.PeerMaxIDs(ctx, viewer, []Owner{{Kind: KindUser, ID: owner}})
	want := maxes[0].GetMaxId().GetValue()
	got := rendered.GetStoriesMaxId_FLAGRECENTSTORY().GetMaxId().GetValue()
	if got != want || (want > 0 && rendered.GetStoriesMaxId_FLAGRECENTSTORY().GetLive() != maxes[0].GetLive()) {
		t.Fatalf("viewer %d render %d live=%v peer %+v", viewer, got, rendered.GetStoriesMaxId_FLAGRECENTSTORY().GetLive(), maxes[0])
	}
}

func storyPushKind(t *testing.T, pushes []pushedStory, userID int64) string {
	t.Helper()
	for _, push := range pushes {
		if push.userID == userID {
			return itemKind(push.up)
		}
	}
	t.Fatalf("no push for %d", userID)
	return ""
}

func itemKind(up *mtproto.Updates) string {
	if up == nil {
		return ""
	}
	for _, u := range up.Updates {
		if u.GetPredicateName() == mtproto.Predicate_updateStory {
			return u.GetStory().GetPredicateName()
		}
	}
	return ""
}
