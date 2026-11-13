package story

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestExpiredStoriesStayWithOwnerAndPinned(t *testing.T) {
	svc, ctx, now := newSvc(t)
	svc.MarkContact(1, 2)
	pub := send(t, svc, ctx, 1, photoMedia(), rules("all"), 11, "live", false)
	pin := send(t, svc, ctx, 1, videoMedia(), rules("all"), 12, "pin", true)
	pubID, pinID := storyID(t, pub), storyID(t, pin)
	owner := Owner{Kind: KindUser, ID: 1}
	*now += defaultPeriod + 1

	other, err := svc.GetByID(ctx, 2, owner, 0, []int32{pubID, pinID})
	if err != nil || len(other.Stories) != 2 {
		t.Fatal(err)
	}
	if other.Stories[0].GetPredicateName() != mtproto.Predicate_storyItemDeleted {
		t.Fatal("expired story was returned to a contact")
	}
	if other.Stories[1].GetPredicateName() == mtproto.Predicate_storyItemDeleted || other.Stories[1].GetId() != pinID {
		t.Fatalf("pinned expired story hidden: %+v", other.Stories[1])
	}
	mine, err := svc.GetByID(ctx, 1, owner, 0, []int32{pubID})
	if err != nil || mine.Stories[0].GetPredicateName() == mtproto.Predicate_storyItemDeleted {
		t.Fatalf("owner archive by id: %+v %v", mine, err)
	}
	views, err := svc.GetViews(ctx, 2, owner, 0, []int32{pubID, pinID})
	if err != nil || views.Views[0].GetViewsCount() != 0 {
		t.Fatalf("views of expired story: %+v %v", views, err)
	}
	if _, err = svc.GetArchive(ctx, 2, owner, 0, 0, 10); err == nil {
		t.Fatal("archive is owner only")
	}
	arch, err := svc.GetArchive(ctx, 1, owner, 0, 0, 10)
	if err != nil || arch.Count < 1 {
		t.Fatalf("owner archive: %+v %v", arch, err)
	}
	if err = svc.AllowMediaDownload(ctx, 2, 9); err == nil || !strings.Contains(err.Error(), "FILE_REFERENCE_EXPIRED") {
		t.Fatalf("contact fetched expired story photo: %v", err)
	}
	if err = svc.AllowMediaDownload(ctx, 1, 9); err != nil {
		t.Fatalf("owner fetch: %v", err)
	}
	if err = svc.AllowMediaDownload(ctx, 2, 15); err != nil {
		t.Fatalf("pinned video: %v", err)
	}
	if err = svc.AllowMediaDownload(ctx, 9, 99); err != nil {
		t.Fatalf("unrelated file: %v", err)
	}
}

func TestStoryPrivacyDisallowAllBlockedAndLists(t *testing.T) {
	svc, ctx, _ := newSvc(t)
	svc.MarkContact(1, 2)
	svc.MarkCloseFriend(1, 3)
	svc.MarkBlocked(1, 8)
	owner := Owner{Kind: KindUser, ID: 1}

	nobody := send(t, svc, ctx, 1, photoMedia(), []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
	}, 21, "nobody", false)
	noID := storyID(t, nobody)
	got, err := svc.GetByID(ctx, 2, owner, 0, []int32{noID})
	if err != nil || got.Stories[0].GetPredicateName() != mtproto.Predicate_storyItemDeleted {
		t.Fatal("disallowAll was treated as allow")
	}
	mine, _ := svc.GetByID(ctx, 1, owner, 0, []int32{noID})
	if mine.Stories[0].GetPredicateName() == mtproto.Predicate_storyItemDeleted {
		t.Fatal("owner cannot see disallowAll")
	}

	except := send(t, svc, ctx, 1, photoMedia(), []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueAllowUsers(&mtproto.PrivacyRule{Users: []int64{4}}).To_PrivacyRule(),
	}, 22, "except", false)
	exID := storyID(t, except)
	yes, _ := svc.GetByID(ctx, 4, owner, 0, []int32{exID})
	no, _ := svc.GetByID(ctx, 2, owner, 0, []int32{exID})
	if yes.Stories[0].GetPredicateName() == mtproto.Predicate_storyItemDeleted || no.Stories[0].GetPredicateName() != mtproto.Predicate_storyItemDeleted {
		t.Fatal("allow-users exception on disallowAll")
	}

	hidden := send(t, svc, ctx, 1, photoMedia(), rules("all"), 23, "blocked", false)
	hID := storyID(t, hidden)
	blocked, _ := svc.GetByID(ctx, 8, owner, 0, []int32{hID})
	if blocked.Stories[0].GetPredicateName() != mtproto.Predicate_storyItemDeleted {
		t.Fatal("blocked user saw the story")
	}
	peer, _ := svc.GetPeerStories(ctx, 8, owner, 0)
	if len(peer.Stories.Stories) != 0 {
		t.Fatal("blocked user still has a tray")
	}

	contacts := send(t, svc, ctx, 1, photoMedia(), []*mtproto.PrivacyRule{
		mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueDisallowContacts(nil).To_PrivacyRule(),
		mtproto.MakeTLPrivacyValueDisallowUsers(&mtproto.PrivacyRule{Users: []int64{9}}).To_PrivacyRule(),
	}, 24, "lists", false)
	cID := storyID(t, contacts)
	asContact, _ := svc.GetByID(ctx, 2, owner, 0, []int32{cID})
	asUser, _ := svc.GetByID(ctx, 9, owner, 0, []int32{cID})
	stranger, _ := svc.GetByID(ctx, 7, owner, 0, []int32{cID})
	if asContact.Stories[0].GetPredicateName() != mtproto.Predicate_storyItemDeleted || asUser.Stories[0].GetPredicateName() != mtproto.Predicate_storyItemDeleted {
		t.Fatal("disallow lists did not hide the story")
	}
	if stranger.Stories[0].GetPredicateName() == mtproto.Predicate_storyItemDeleted {
		t.Fatal("stranger hidden by a contact disallow")
	}

	close := send(t, svc, ctx, 1, photoMedia(), rules("close"), 25, "close", false)
	clID := storyID(t, close)
	asClose, _ := svc.GetPeerStories(ctx, 3, owner, 0)
	asOther, _ := svc.GetByID(ctx, 2, owner, 0, []int32{clID})
	if !hasStory(asClose.Stories.Stories, clID) || asOther.Stories[0].GetPredicateName() != mtproto.Predicate_storyItemDeleted {
		t.Fatal("close friends rule")
	}
}
