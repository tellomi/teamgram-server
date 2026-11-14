package mtproto

import (
	"context"
	"testing"
	"time"
)

func TestMemberListPrefetchesChatRulesOnce(t *testing.T) {
	const viewer int64 = 1
	const chat int64 = 42
	prevMember := PrivacyChatMember
	prevFetch := PrefetchPrivacy
	t.Cleanup(func() {
		PrivacyChatMember = prevMember
		PrefetchPrivacy = prevFetch
	})

	var lookups, prefetches int
	PrivacyChatMember = func(context.Context, int64, []int64) (bool, bool) {
		lookups++
		return false, false
	}
	PrefetchPrivacy = func(ctx context.Context, gotViewer int64, owners, chats []int64) {
		prefetches++
		if gotViewer != viewer {
			t.Errorf("viewer %d", gotViewer)
		}
		if len(owners) != 3 {
			t.Errorf("owners %v", owners)
		}
		NoteMembership(ctx, viewer, chats, []int64{chat})
		NoteBlocked(ctx, owners, []int64{owners[0]})
	}

	page := make([]*ImmutableUser, 0, 3)
	for _, id := range []int64{2, 3, 4} {
		page = append(page, privacyUser(id, chat))
	}
	me := privacyUser(viewer, 0)
	users := &MutableUsers{Users: append([]*ImmutableUser{me}, page...)}
	out := users.GetUserListByIdList(viewer, 2, 3, 4)
	if prefetches != 1 {
		t.Fatalf("prefetches %d, want 1", prefetches)
	}
	if lookups != 0 {
		t.Fatalf("per-user lookups %d", lookups)
	}
	if len(out) != 3 {
		t.Fatalf("users %d", len(out))
	}
	// 2 is blocked by the prefetch. 3 and 4 are in the disallow chat, so the photo is hidden.
	if out[0].GetPhoto() != nil || out[0].GetStatus().GetPredicateName() != Predicate_userStatusEmpty {
		t.Fatalf("blocked user photo %v status %s", out[0].GetPhoto(), out[0].GetStatus().GetPredicateName())
	}
	if out[1].GetPhoto() != nil || out[2].GetPhoto() != nil {
		t.Fatal("chat-participant disallow still returned the photo")
	}
	if out[1].GetStatus().GetPredicateName() == Predicate_userStatusEmpty {
		t.Fatal("a user who did not block the viewer was given an empty status")
	}
}

func TestMembershipCacheAnswersWithoutASecondQuery(t *testing.T) {
	ctx := WithPrivacyCache(context.Background())
	NoteMembership(ctx, 7, []int64{1, 2}, []int64{2})
	member, ok := Membership(ctx, 7, []int64{1})
	if !ok || member {
		t.Fatalf("chat 1 member %v known %v", member, ok)
	}
	member, ok = Membership(ctx, 7, []int64{2})
	if !ok || !member {
		t.Fatalf("chat 2 member %v known %v", member, ok)
	}
	if _, ok = Membership(ctx, 7, []int64{9}); ok {
		t.Fatal("unknown chat was treated as known")
	}
}

func privacyUser(id, disallowChat int64) *ImmutableUser {
	var rules []*PrivacyKeyRules
	if disallowChat != 0 {
		rules = []*PrivacyKeyRules{{
			Key: int32(PROFILE_PHOTO),
			Rules: []*PrivacyRule{
				{PredicateName: Predicate_privacyValueAllowAll},
				{PredicateName: Predicate_privacyValueDisallowChatParticipants, Chats: []int64{disallowChat}},
			},
		}}
	}
	return &ImmutableUser{
		User: &UserData{
			Id: id,
			ProfilePhoto: &Photo{
				Id:            id * 100,
				PredicateName: Predicate_photo,
				Sizes: []*PhotoSize{{
					PredicateName: Predicate_photoSize,
					Type:          "a",
					W:             160,
					H:             160,
				}},
			},
		},
		LastSeenAt:       time.Now().Unix(),
		KeysPrivacyRules: rules,
	}
}
