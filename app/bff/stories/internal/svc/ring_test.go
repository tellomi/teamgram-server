package svc

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
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
