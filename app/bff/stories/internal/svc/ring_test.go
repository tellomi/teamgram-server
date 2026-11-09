package svc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

type fakeStoryUsers struct {
	im  *mtproto.ImmutableUser
	set []int32
}

func (f *fakeStoryUsers) UserSetStoriesMaxId(_ context.Context, in *user.TLUserSetStoriesMaxId) (*mtproto.Bool, error) {
	f.set = append(f.set, in.GetId())
	if f.im != nil && f.im.User != nil {
		f.im.User.StoriesMaxId = in.GetId()
	}
	return mtproto.BoolTrue, nil
}

func (f *fakeStoryUsers) UserGetImmutableUserV2(context.Context, *user.TLUserGetImmutableUserV2) (*mtproto.ImmutableUser, error) {
	return f.im, nil
}

func (f *fakeStoryUsers) UserGetMutableUsers(context.Context, *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error) {
	if f.im == nil {
		return &user.Vector_ImmutableUser{}, nil
	}
	return &user.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{f.im}}, nil
}

type fakeStorySync struct {
	pushes []*sync.TLSyncPushUpdates
}

func (f *fakeStorySync) SyncPushUpdates(_ context.Context, in *sync.TLSyncPushUpdates) (*mtproto.Void, error) {
	f.pushes = append(f.pushes, in)
	return mtproto.EmptyVoid, nil
}

func TestApplyStoryMaxClearsRingOnContacts(t *testing.T) {
	im := mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: mtproto.MakeTLUserData(&mtproto.UserData{
			Id:                 7,
			FirstName:          "Akis",
			StoriesUnavailable: true,
			StoriesMaxId:       4,
		}).To_UserData(),
		ReverseContacts: []*mtproto.ContactData{
			mtproto.MakeTLContactData(&mtproto.ContactData{
				UserId:        9,
				ContactUserId: 7,
				MutualContact: true,
				FirstName:     wrapperspb.String("Akis"),
			}).To_ContactData(),
		},
	}).To_ImmutableUser()
	users := &fakeStoryUsers{im: im}
	syncer := &fakeStorySync{}
	applyStoryMax(context.Background(), users, syncer, 7, 0)
	if len(users.set) != 1 || users.set[0] != 0 {
		t.Fatalf("stored max %v", users.set)
	}
	if len(syncer.pushes) != 2 {
		t.Fatalf("pushes %d", len(syncer.pushes))
	}
	byUser := map[int64]*mtproto.User{}
	for _, push := range syncer.pushes {
		if len(push.GetUpdates().GetUpdates()) != 1 || push.GetUpdates().GetUpdates()[0].GetUserId() != 7 {
			t.Fatalf("update %+v", push.GetUpdates())
		}
		if len(push.GetUpdates().GetUsers()) != 1 {
			t.Fatalf("users %+v", push.GetUpdates().GetUsers())
		}
		byUser[push.GetUserId()] = push.GetUpdates().GetUsers()[0]
	}
	self := byUser[7]
	contact := byUser[9]
	if self == nil || !self.GetSelf() || self.GetStoriesMaxId_FLAGRECENTSTORY() != nil {
		t.Fatalf("self %+v", self)
	}
	if contact == nil || !contact.GetContact() || contact.GetStoriesMaxId_FLAGRECENTSTORY() != nil || contact.GetStoriesMaxId_FLAGINT32() != nil {
		t.Fatalf("contact %+v", contact)
	}
}

// TestEmptyContactsStripAllowContacts records the render that applyStoryMax
// used to send. getImmutableUserV2 with an empty To leaves Contacts nil.
// CheckPrivacy then treats "my contacts" as deny, on master and on the
// forward-contact loader that only looks at To.
func TestEmptyContactsStripAllowContacts(t *testing.T) {
	im := storyAuthor(t, mtproto.Predicate_privacyValueAllowContacts)
	im.Contacts = nil
	got := im.ToUser(storyMutual)
	if got.GetPhoto() != nil || got.GetPhone() != nil {
		t.Fatalf("empty Contacts still exposed photo/phone: %+v", got)
	}
	if got.GetStatus().GetPredicateName() != mtproto.Predicate_userStatusRecently {
		t.Fatalf("status %s", got.GetStatus().GetPredicateName())
	}
	if !got.GetContact() {
		t.Fatal("reverse contact flag was cleared")
	}
}

func TestApplyStoryMaxKeepsPrivacyAcrossLoaders(t *testing.T) {
	for _, fillV2 := range []bool{false, true} {
		name := "master-and-224-empty-to"
		if fillV2 {
			name = "224-fills-forward-only-when-to-is-set"
		}
		t.Run(name, func(t *testing.T) {
			for _, maxID := range []int32{0, 5} {
				t.Run(storyMaxName(maxID), func(t *testing.T) {
					users := newStoryPrivacyUsers(fillV2, mtproto.Predicate_privacyValueAllowContacts, []int64{storyMutual, storyReverseOnly}, map[int64]bool{storyMutual: true})
					byUser := pushStoryPrivacy(t, users, maxID)
					assertStoryUser(t, byUser[storyMutual], maxID, true, "Local", true)
					assertStoryUser(t, byUser[storyReverseOnly], maxID, true, "Reverse", false)
				})
			}
		})
	}
}

func TestApplyStoryMaxDisallowAllStaysHidden(t *testing.T) {
	users := newStoryPrivacyUsers(true, mtproto.Predicate_privacyValueDisallowAll, []int64{storyMutual}, map[int64]bool{storyMutual: true})
	byUser := pushStoryPrivacy(t, users, 0)
	assertStoryUser(t, byUser[storyMutual], 0, true, "Local", false)
}

func TestApplyStoryMaxAllowAllStaysVisibleWithoutForwardEdge(t *testing.T) {
	users := newStoryPrivacyUsers(false, mtproto.Predicate_privacyValueAllowAll, []int64{storyReverseOnly}, nil)
	byUser := pushStoryPrivacy(t, users, 5)
	assertStoryUser(t, byUser[storyReverseOnly], 5, true, "Reverse", true)
}

func TestApplyStoryMaxPushesExtraAudience(t *testing.T) {
	SetRingAudience(func(context.Context, int64) []int64 { return []int64{15} })
	t.Cleanup(func() { SetRingAudience(nil) })
	users := newStoryPrivacyUsers(true, mtproto.Predicate_privacyValueAllowContacts, []int64{storyMutual}, map[int64]bool{storyMutual: true})
	syncer := &fakeStorySync{}
	if err := applyStoryMax(context.Background(), users, syncer, storyAuthorID, 0); err != nil {
		t.Fatal(err)
	}
	byUser := storyPushes(t, syncer)
	extra := byUser[15]
	if extra == nil || extra.GetStoriesMaxId_FLAGRECENTSTORY() != nil || !extra.GetStoriesUnavailable() {
		t.Fatalf("extra %+v", extra)
	}
	if byUser[storyMutual] == nil || byUser[storyAuthorID] == nil {
		t.Fatalf("missing self or contact %+v", byUser)
	}
}

func TestApplyStoryMaxPaintsPerViewerRing(t *testing.T) {
	prev := story.Shared()
	svc := story.New(story.NewMemoryStore())
	story.Use(svc)
	t.Cleanup(func() {
		story.Use(prev)
		mtproto.EnableStoryRings(nil)
		SetRingAudience(nil)
	})
	svc.MarkCloseFriend(storyAuthorID, 11)
	media := mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
		Photo_FLAGPHOTO: mtproto.MakeTLPhoto(&mtproto.Photo{Id: 9}).To_Photo(),
	}).To_MessageMedia()
	up, err := svc.Send(context.Background(), story.SendRequest{
		Actor: storyAuthorID, Owner: story.Owner{Kind: story.KindUser, ID: storyAuthorID},
		Media: media, RandomID: 1,
		Rules: []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowCloseFriends(nil).To_PrivacyRule()},
	})
	if err != nil {
		t.Fatal(err)
	}
	var id int32
	for _, u := range up.Updates {
		if u.GetPredicateName() == mtproto.Predicate_updateStoryID {
			id = u.GetId_INT32()
		}
	}
	if id == 0 {
		t.Fatal("missing story id")
	}
	story.EnableUserRingPaint()
	SetRingAudience(func(context.Context, int64) []int64 { return []int64{11} })
	users := newStoryPrivacyUsers(true, mtproto.Predicate_privacyValueAllowContacts, []int64{storyMutual}, map[int64]bool{storyMutual: true})
	syncer := &fakeStorySync{}
	if err = applyStoryMax(context.Background(), users, syncer, storyAuthorID, id); err != nil {
		t.Fatal(err)
	}
	byUser := storyPushes(t, syncer)
	if storyRing(byUser[storyAuthorID]) != id || byUser[storyAuthorID].GetStoriesUnavailable() {
		t.Fatalf("self %+v", byUser[storyAuthorID])
	}
	if storyRing(byUser[11]) != id || byUser[11].GetStoriesUnavailable() {
		t.Fatalf("close friend %+v", byUser[11])
	}
	other := byUser[storyMutual]
	if other == nil || storyRing(other) != 0 || !other.GetStoriesUnavailable() {
		t.Fatalf("other contact %+v", other)
	}
	if other.GetPhoto().GetPhotoId() != 42 {
		t.Fatalf("privacy photo %d", other.GetPhoto().GetPhotoId())
	}
}

func TestApplyStoryMaxSkipsViewerPushWhenContactsFail(t *testing.T) {
	users := newStoryPrivacyUsers(false, mtproto.Predicate_privacyValueAllowContacts, []int64{storyMutual}, map[int64]bool{storyMutual: true})
	users.mutableErr = errors.New("user service down")
	syncer := &fakeStorySync{}
	if err := applyStoryMax(context.Background(), users, syncer, storyAuthorID, 0); err != nil {
		t.Fatal(err)
	}
	if len(users.set) != 1 || users.set[0] != 0 {
		t.Fatalf("stored %v", users.set)
	}
	if len(syncer.pushes) != 1 || syncer.pushes[0].GetUserId() != storyAuthorID {
		t.Fatalf("pushes %+v", syncer.pushes)
	}
}

const (
	storyAuthorID    int64 = 7
	storyMutual      int64 = 9
	storyReverseOnly int64 = 11
)

type storyPrivacyUsers struct {
	fakeStoryUsers
	forward    map[int64]*mtproto.ContactData
	reverse    []*mtproto.ContactData
	rules      []*mtproto.PrivacyKeyRules
	v2         []*user.TLUserGetImmutableUserV2
	mutable    []*user.TLUserGetMutableUsers
	mutableErr error
	fillV2     bool
}

func newStoryPrivacyUsers(fillV2 bool, rule string, viewers []int64, forwardIDs map[int64]bool) *storyPrivacyUsers {
	names := map[int64]string{storyMutual: "Local", storyReverseOnly: "Reverse"}
	forward := map[int64]*mtproto.ContactData{}
	var reverse []*mtproto.ContactData
	for _, id := range viewers {
		reverse = append(reverse, storyReverseRow(id, names[id]))
		if forwardIDs[id] {
			forward[id] = storyForwardRow(id)
		}
	}
	return &storyPrivacyUsers{
		forward: forward,
		reverse: reverse,
		rules: []*mtproto.PrivacyKeyRules{
			storyRules(mtproto.PROFILE_PHOTO, rule),
			storyRules(mtproto.PHONE_NUMBER, rule),
			storyRules(mtproto.STATUS_TIMESTAMP, rule),
		},
		fillV2: fillV2,
	}
}

func pushStoryPrivacy(t *testing.T, users *storyPrivacyUsers, maxID int32) map[int64]*mtproto.User {
	t.Helper()
	syncer := &fakeStorySync{}
	if err := applyStoryMax(context.Background(), users, syncer, storyAuthorID, maxID); err != nil {
		t.Fatal(err)
	}
	if len(users.v2) != 1 || len(users.v2[0].GetTo()) != 0 || !users.v2[0].GetHasTo() {
		t.Fatalf("v2 request %+v", users.v2)
	}
	if len(users.mutable) != 1 {
		t.Fatalf("mutable calls %d", len(users.mutable))
	}
	if !sameIDs(users.mutable[0].GetTo(), viewerIDs(users.reverse)) {
		t.Fatalf("mutable to %v", users.mutable[0].GetTo())
	}
	byUser := storyPushes(t, syncer)
	if self := byUser[storyAuthorID]; self == nil || !self.GetSelf() || storyRing(self) != maxID {
		t.Fatalf("self ring %+v", self)
	}
	return byUser
}

func (f *storyPrivacyUsers) UserGetImmutableUserV2(_ context.Context, in *user.TLUserGetImmutableUserV2) (*mtproto.ImmutableUser, error) {
	f.v2 = append(f.v2, in)
	im := f.base()
	// Master leaves Contacts empty. The forward loader fills them only when
	// To names the peers, which this call does not.
	if f.fillV2 && len(in.GetTo()) > 0 && !hasID(in.GetTo(), storyAuthorID) {
		im.Contacts = f.rowsFor(in.GetTo())
	}
	return im, nil
}

func (f *storyPrivacyUsers) UserGetMutableUsers(_ context.Context, in *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error) {
	f.mutable = append(f.mutable, in)
	if f.mutableErr != nil {
		return nil, f.mutableErr
	}
	im := f.base()
	// len == 1 takes the shortcut that does not load contact rows.
	if len(in.GetId()) > 1 {
		peers := in.GetId()
		if len(in.GetTo()) > 0 && !hasID(in.GetTo(), storyAuthorID) {
			peers = in.GetTo()
		}
		im.Contacts = f.rowsFor(peers)
	}
	return &user.Vector_ImmutableUser{Datas: []*mtproto.ImmutableUser{im}}, nil
}

func (f *storyPrivacyUsers) base() *mtproto.ImmutableUser {
	im := storyAuthor(nil, "")
	im.User.StoriesMaxId = 0
	if len(f.set) > 0 {
		im.User.StoriesMaxId = f.set[len(f.set)-1]
	}
	im.Contacts = nil
	im.ReverseContacts = f.reverse
	im.KeysPrivacyRules = f.rules
	im.LastSeenAt = time.Now().Unix()
	return im
}

func (f *storyPrivacyUsers) rowsFor(peers []int64) []*mtproto.ContactData {
	var out []*mtproto.ContactData
	for _, id := range peers {
		if row := f.forward[id]; row != nil {
			out = append(out, row)
		}
	}
	return out
}

func storyAuthor(t *testing.T, rule string) *mtproto.ImmutableUser {
	if t != nil && rule == "" {
		t.Helper()
	}
	rules := []*mtproto.PrivacyKeyRules{}
	if rule != "" {
		rules = []*mtproto.PrivacyKeyRules{
			storyRules(mtproto.PROFILE_PHOTO, rule),
			storyRules(mtproto.PHONE_NUMBER, rule),
			storyRules(mtproto.STATUS_TIMESTAMP, rule),
		}
	}
	return mtproto.MakeTLImmutableUser(&mtproto.ImmutableUser{
		User: mtproto.MakeTLUserData(&mtproto.UserData{
			Id:        storyAuthorID,
			FirstName: "Akis",
			Phone:     "8613800138000",
			ProfilePhoto: mtproto.MakeTLPhoto(&mtproto.Photo{
				Id:    42,
				Sizes: []*mtproto.PhotoSize{mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{Type: "a", W: 100, H: 100, Size2: 10}).To_PhotoSize()},
			}).To_Photo(),
		}).To_UserData(),
		LastSeenAt: time.Now().Unix(),
		ReverseContacts: []*mtproto.ContactData{
			storyReverseRow(storyMutual, "Local"),
		},
		Contacts: []*mtproto.ContactData{
			storyForwardRow(storyMutual),
		},
		KeysPrivacyRules: rules,
	}).To_ImmutableUser()
}

func storyRules(key int32, rule string) *mtproto.PrivacyKeyRules {
	return mtproto.MakeTLPrivacyKeyRules(&mtproto.PrivacyKeyRules{
		Key:   key,
		Rules: []*mtproto.PrivacyRule{{PredicateName: rule}},
	}).To_PrivacyKeyRules()
}

func viewerIDs(rows []*mtproto.ContactData) []int64 {
	out := make([]int64, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.GetUserId())
	}
	return out
}

func storyForwardRow(viewer int64) *mtproto.ContactData {
	return mtproto.MakeTLContactData(&mtproto.ContactData{
		UserId:        storyAuthorID,
		ContactUserId: viewer,
	}).To_ContactData()
}

func storyReverseRow(viewer int64, name string) *mtproto.ContactData {
	return mtproto.MakeTLContactData(&mtproto.ContactData{
		UserId:        viewer,
		ContactUserId: storyAuthorID,
		FirstName:     wrapperspb.String(name),
	}).To_ContactData()
}

func storyPushes(t *testing.T, syncer *fakeStorySync) map[int64]*mtproto.User {
	t.Helper()
	byUser := map[int64]*mtproto.User{}
	for _, push := range syncer.pushes {
		updates := push.GetUpdates().GetUpdates()
		if len(updates) != 1 || updates[0].GetPredicateName() != mtproto.Predicate_updateUser || updates[0].GetUserId() != storyAuthorID {
			t.Fatalf("update %+v", push.GetUpdates())
		}
		users := push.GetUpdates().GetUsers()
		if len(users) != 1 {
			t.Fatalf("users %d", len(users))
		}
		byUser[push.GetUserId()] = users[0]
	}
	return byUser
}

func assertStoryUser(t *testing.T, got *mtproto.User, maxID int32, contact bool, name string, visible bool) {
	t.Helper()
	if got == nil {
		t.Fatal("missing user")
	}
	if got.GetMin() {
		t.Fatal("update carried a min user")
	}
	if got.GetContact() != contact || got.GetFirstName().GetValue() != name {
		t.Fatalf("contact=%v name=%q", got.GetContact(), got.GetFirstName().GetValue())
	}
	if storyRing(got) != maxID {
		t.Fatalf("ring %d", storyRing(got))
	}
	if visible {
		if got.GetPhoto().GetPhotoId() != 42 || got.GetPhone().GetValue() != "8613800138000" {
			t.Fatalf("visible profile photo=%d phone=%q", got.GetPhoto().GetPhotoId(), got.GetPhone().GetValue())
		}
		if got.GetStatus().GetPredicateName() != mtproto.Predicate_userStatusOnline {
			t.Fatalf("status %s", got.GetStatus().GetPredicateName())
		}
		return
	}
	if got.GetPhoto() != nil || got.GetPhone() != nil {
		t.Fatalf("hidden profile was sent photo=%v phone=%v", got.GetPhoto(), got.GetPhone())
	}
	if got.GetStatus().GetPredicateName() != mtproto.Predicate_userStatusRecently {
		t.Fatalf("status %s", got.GetStatus().GetPredicateName())
	}
}

func storyRing(user *mtproto.User) int32 {
	recent := user.GetStoriesMaxId_FLAGRECENTSTORY()
	if recent == nil || recent.GetMaxId() == nil {
		return 0
	}
	return recent.GetMaxId().GetValue()
}

func storyMaxName(maxID int32) string {
	if maxID == 0 {
		return "expired"
	}
	return "still-active"
}

func sameIDs(got, want []int64) bool {
	if len(got) != len(want) {
		return false
	}
	seen := map[int64]int{}
	for _, id := range got {
		seen[id]++
	}
	for _, id := range want {
		seen[id]--
		if seen[id] < 0 {
			return false
		}
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}

func hasID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
