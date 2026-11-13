package service

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stories/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
)

func TestStoryAllowUsersVisibleOnlyToListedUser(t *testing.T) {
	story.Use(story.New(story.NewMemoryStore()))
	mediaLookup = &fakeMedia{}
	t.Cleanup(func() { mediaLookup = nil })
	svc := New(&svc.ServiceContext{})

	cases := []struct {
		name string
		user *mtproto.InputUser
	}{
		{"inputUser", mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 9}).To_InputUser()},
		{"inputUserFromMessage", mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{
			UserId: 2,
			MsgId:  4,
			Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 8, AccessHash: 1}).To_InputPeer(),
		}).To_InputUser()},
		{"id without predicate", &mtproto.InputUser{UserId: 2, AccessHash: 9, Constructor: mtproto.CRC32_inputUser}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := postOnly(t, svc, int64(100+i), tc.user)
			if kind := storyKind(t, svc, 2, id); kind == mtproto.Predicate_storyItemDeleted {
				t.Fatal("listed user got storyItemDeleted")
			}
			if !peerHas(t, svc, 2, id) {
				t.Fatal("listed user peer stories omit the story")
			}
			if !trayHas(t, svc, 2, 1) {
				t.Fatal("listed user tray omits the author")
			}
			if kind := storyKind(t, svc, 3, id); kind != mtproto.Predicate_storyItemDeleted {
				t.Fatalf("stranger kind %s", kind)
			}
			if peerHas(t, svc, 3, id) {
				t.Fatal("stranger peer stories include the story")
			}
		})
	}
}

func TestStoryDisallowUsersAndCloseFriends(t *testing.T) {
	story.Use(story.New(story.NewMemoryStore()))
	mediaLookup = &fakeMedia{}
	t.Cleanup(func() { mediaLookup = nil })
	svc := New(&svc.ServiceContext{})
	story.Shared().MarkContact(1, 2)
	story.Shared().MarkContact(1, 3)
	story.Shared().MarkCloseFriend(1, 3)

	hidden := postRules(t, svc, 201, []*mtproto.InputPrivacyRule{
		mtproto.MakeTLInputPrivacyValueAllowAll(nil).To_InputPrivacyRule(),
		mtproto.MakeTLInputPrivacyValueDisallowUsers(&mtproto.InputPrivacyRule{
			Users: []*mtproto.InputUser{
				mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{UserId: 2, MsgId: 1}).To_InputUser(),
			},
		}).To_InputPrivacyRule(),
	})
	if storyKind(t, svc, 2, hidden) != mtproto.Predicate_storyItemDeleted {
		t.Fatal("disallow-users did not hide the story")
	}
	if storyKind(t, svc, 3, hidden) == mtproto.Predicate_storyItemDeleted {
		t.Fatal("disallow-users hid the story from someone else")
	}

	closeID := postRules(t, svc, 202, []*mtproto.InputPrivacyRule{
		mtproto.MakeTLInputPrivacyValueAllowCloseFriends(nil).To_InputPrivacyRule(),
	})
	if storyKind(t, svc, 3, closeID) == mtproto.Predicate_storyItemDeleted || !peerHas(t, svc, 3, closeID) {
		t.Fatal("close friend cannot see the story")
	}
	if storyKind(t, svc, 2, closeID) != mtproto.Predicate_storyItemDeleted {
		t.Fatal("ordinary contact saw a close-friends story")
	}
}

func postOnly(t *testing.T, svc *Service, random int64, user *mtproto.InputUser) int32 {
	t.Helper()
	return postRules(t, svc, random, []*mtproto.InputPrivacyRule{
		mtproto.MakeTLInputPrivacyValueAllowUsers(&mtproto.InputPrivacyRule{
			Users: []*mtproto.InputUser{user},
		}).To_InputPrivacyRule(),
	})
}

func postRules(t *testing.T, svc *Service, random int64, rules []*mtproto.InputPrivacyRule) int32 {
	t.Helper()
	up, err := svc.StoriesSendStory(withUser(1, 10), &mtproto.TLStoriesSendStory{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Media: mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{
			Id_INPUTPHOTO: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{Id: 4, AccessHash: 5}).To_InputPhoto(),
		}).To_InputMedia(),
		PrivacyRules: rules,
		RandomId:     random,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range up.GetUpdates() {
		if u.GetPredicateName() == mtproto.Predicate_updateStoryID {
			return u.GetId_INT32()
		}
	}
	t.Fatal("no story id")
	return 0
}

func storyKind(t *testing.T, svc *Service, viewer int64, id int32) string {
	t.Helper()
	got, err := svc.StoriesGetStoriesByID(withUser(viewer, viewer), &mtproto.TLStoriesGetStoriesByID{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 1, AccessHash: 1}).To_InputPeer(),
		Id:   []int32{id},
	})
	if err != nil || len(got.GetStories()) != 1 {
		t.Fatalf("getStoriesByID viewer %d: %+v %v", viewer, got, err)
	}
	return got.GetStories()[0].GetPredicateName()
}

func peerHas(t *testing.T, svc *Service, viewer int64, id int32) bool {
	t.Helper()
	got, err := svc.StoriesGetPeerStories(withUser(viewer, viewer), &mtproto.TLStoriesGetPeerStories{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 1, AccessHash: 1}).To_InputPeer(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range got.GetStories().GetStories() {
		if item.GetId() == id && item.GetPredicateName() != mtproto.Predicate_storyItemDeleted {
			return true
		}
	}
	return false
}

func trayHas(t *testing.T, svc *Service, viewer, author int64) bool {
	t.Helper()
	got, err := svc.StoriesGetAllStories(withUser(viewer, viewer), &mtproto.TLStoriesGetAllStories{})
	if err != nil {
		t.Fatal(err)
	}
	for _, ps := range got.GetPeerStories() {
		if ps.GetPeer().GetUserId() == author && len(ps.GetStories()) > 0 {
			return true
		}
	}
	return false
}
